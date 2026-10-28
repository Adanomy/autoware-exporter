// Package monitor maintains independent ROS contexts and immutable telemetry snapshots.
package monitor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Adanomy/autoware-exporter/internal/ros2"
	"github.com/Adanomy/autoware-exporter/internal/telemetry"
)

type Client interface {
	Subscribe(ros2.Subscription) error
	Poll(time.Duration) ([]ros2.Message, error)
	PublisherCount(string) (int, error)
	PublisherFingerprint(string) (string, error)
	Close() error
}

type Dial func(ros2.Config) (Client, error)

var errPublisherChanged = errors.New("notification publisher changed")

type Options struct {
	PollInterval  time.Duration
	RetryInterval time.Duration
}

type Manager struct {
	mu        sync.RWMutex
	vehicles  []ros2.Config
	snapshots []telemetry.VehicleSnapshot
	options   Options
	dial      Dial
	logger    *slog.Logger
}

func New(vehicles []ros2.Config, sources []telemetry.Source, options Options, logger *slog.Logger) (*Manager, error) {
	return newManager(vehicles, sources, options, logger, func(cfg ros2.Config) (Client, error) { return ros2.New(cfg) })
}

func newManager(vehicles []ros2.Config, sources []telemetry.Source, options Options, logger *slog.Logger, dial Dial) (*Manager, error) {
	if len(vehicles) == 0 || len(sources) == 0 {
		return nil, fmt.Errorf("at least one vehicle and source are required")
	}
	if options.PollInterval <= 0 || options.RetryInterval <= 0 {
		return nil, fmt.Errorf("poll and retry intervals must be positive")
	}
	if logger == nil {
		logger = slog.Default()
	}
	m := &Manager{vehicles: slices.Clone(vehicles), options: options, dial: dial, logger: logger}
	identities := make(map[string]bool)
	for _, vehicle := range vehicles {
		if vehicle.Name == "" || identities[vehicle.Name] || vehicle.DomainID < 0 || vehicle.DomainID > 232 {
			return nil, fmt.Errorf("invalid or duplicate vehicle %q", vehicle.Name)
		}
		identities[vehicle.Name] = true
		snapshot := telemetry.VehicleSnapshot{Name: vehicle.Name, DomainID: vehicle.DomainID, Sources: make(map[string]telemetry.SourceSnapshot)}
		topics := make(map[string]bool)
		for _, source := range sources {
			if _, exists := snapshot.Sources[source.Key]; exists || source.Key == "" || source.Topic == "" || source.Type == "" {
				return nil, fmt.Errorf("invalid or duplicate source %q", source.Key)
			}
			source.Topic = resolveTopic(vehicle.Namespace, source.Topic)
			if topics[source.Topic] {
				return nil, fmt.Errorf("duplicate topic %q for vehicle %q", source.Topic, vehicle.Name)
			}
			topics[source.Topic] = true
			snapshot.Sources[source.Key] = telemetry.SourceSnapshot{Spec: source}
		}
		m.snapshots = append(m.snapshots, snapshot)
	}
	return m, nil
}

func resolveTopic(namespace, topic string) string {
	return "/" + strings.Trim(strings.Trim(namespace, "/")+"/"+strings.TrimPrefix(topic, "/"), "/")
}

// Snapshots returns a consistent copy. Message data is immutable after reception.
func (m *Manager) Snapshots() []telemetry.VehicleSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]telemetry.VehicleSnapshot, len(m.snapshots))
	for i, snapshot := range m.snapshots {
		snapshot.Sources = maps.Clone(snapshot.Sources)
		result[i] = snapshot
	}
	return result
}

func (m *Manager) Run(ctx context.Context) {
	var workers sync.WaitGroup
	for i := range m.vehicles {
		workers.Go(func() { m.runVehicle(ctx, i) })
	}
	workers.Wait()
}

func (m *Manager) runVehicle(ctx context.Context, index int) {
	// Keep rcl calls and its thread-local error state on one thread per vehicle.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cfg := m.vehicles[index]
	logger := m.logger.With("vehicle", cfg.Name, "domain_id", cfg.DomainID)
	lastError := ""
	for ctx.Err() == nil {
		client, err := m.dial(cfg)
		if err != nil {
			if err.Error() != lastError {
				logger.Warn("ROS context unavailable", "error", err)
				lastError = err.Error()
			}
			if !wait(ctx, m.options.RetryInterval) {
				return
			}
			continue
		}
		lastError = ""
		m.mu.Lock()
		m.snapshots[index].Connected = true
		m.snapshots[index].StartedAt = time.Now()
		m.mu.Unlock()
		logger.Info("ROS context started")
		err = m.collect(ctx, index, client, logger)
		if closeErr := client.Close(); closeErr != nil {
			logger.Warn("close ROS context", "error", closeErr)
		}
		m.mu.Lock()
		m.snapshots[index].Connected = false
		for key, source := range m.snapshots[index].Sources {
			source.Subscribed, source.Publishers, source.Data = false, 0, nil
			m.snapshots[index].Sources[key] = source
		}
		m.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, errPublisherChanged) {
			logger.Info("refreshing notification subscriptions", "reason", err)
			continue
		}
		logger.Warn("ROS context failed; retrying", "error", err)
		if !wait(ctx, m.options.RetryInterval) {
			return
		}
	}
}

func (m *Manager) collect(ctx context.Context, index int, client Client, logger *slog.Logger) error {
	nextSubscriptions, nextGraph := time.Time{}, time.Time{}
	subscriptionErrors := make(map[string]string)
	messageErrors := make(map[string]string)
	publisherFingerprints := make(map[string]string)
	for ctx.Err() == nil {
		now := time.Now()
		if !now.Before(nextSubscriptions) {
			for key, source := range m.Snapshots()[index].Sources {
				if source.Subscribed {
					continue
				}
				err := client.Subscribe(ros2.Subscription{Key: key, Topic: source.Spec.Topic, Type: source.Spec.Type, Latched: source.Spec.Latched})
				m.mu.Lock()
				current := m.snapshots[index].Sources[key]
				if err == nil {
					current.Subscribed = true
				} else {
					current.ErrorsTotal++
				}
				m.snapshots[index].Sources[key] = current
				m.mu.Unlock()
				if err != nil && subscriptionErrors[key] != err.Error() {
					logger.Debug("source unavailable", "source", key, "topic", source.Spec.Topic, "error", err)
					subscriptionErrors[key] = err.Error()
				} else if err == nil {
					delete(subscriptionErrors, key)
				}
			}
			nextSubscriptions = now.Add(m.options.RetryInterval)
		}
		if !now.Before(nextGraph) {
			for key, source := range m.Snapshots()[index].Sources {
				if !source.Subscribed {
					continue
				}
				count, err := client.PublisherCount(source.Spec.Topic)
				if err == nil && source.Spec.Latched && count > 0 {
					var fingerprint string
					fingerprint, err = client.PublisherFingerprint(source.Spec.Topic)
					if err == nil && fingerprint != "" {
						previous := publisherFingerprints[key]
						if previous != "" && previous != fingerprint {
							// Recreate the context to request the new publisher's retained
							// samples, including any already received before this graph poll.
							return fmt.Errorf("%w: %s", errPublisherChanged, source.Spec.Topic)
						}
						publisherFingerprints[key] = fingerprint
					}
				}
				m.mu.Lock()
				current := m.snapshots[index].Sources[key]
				if err != nil {
					current.ErrorsTotal++
					count = 0
				}
				if count == 0 && current.Publishers > 0 {
					current.Data = nil
				}
				current.Publishers = count
				m.snapshots[index].Sources[key] = current
				m.mu.Unlock()
			}
			nextGraph = now.Add(time.Second)
		}
		messages, err := client.Poll(m.options.PollInterval)
		if err != nil {
			return err
		}
		for _, message := range messages {
			messageErr := message.Error
			if messageErr == nil && message.Data == nil {
				messageErr = errors.New("received an empty decoded ROS message")
			}
			m.mu.Lock()
			current, known := m.snapshots[index].Sources[message.Key]
			if known {
				current.MessagesTotal++
				if messageErr != nil {
					current.ErrorsTotal++
					current.Data = nil
				} else {
					current.Data = message.Data
					current.ReceivedAt = time.Now()
				}
				m.snapshots[index].Sources[message.Key] = current
			}
			m.mu.Unlock()
			if known && messageErr != nil && messageErrors[message.Key] != messageErr.Error() {
				logger.Debug("source message unavailable", "source", message.Key, "error", messageErr)
				messageErrors[message.Key] = messageErr.Error()
			} else if known && messageErr == nil {
				delete(messageErrors, message.Key)
			}
		}
	}
	return ctx.Err()
}

func wait(ctx context.Context, duration time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(duration):
		return true
	}
}
