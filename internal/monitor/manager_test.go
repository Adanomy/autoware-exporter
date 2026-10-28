package monitor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Adanomy/autoware-exporter/internal/ros2"
	"github.com/Adanomy/autoware-exporter/internal/telemetry"
)

type fakeClient struct {
	batches          chan []ros2.Message
	closed           atomic.Bool
	subscriptions    atomic.Int64
	failFirst        bool
	lastTopic        chan string
	fingerprint      atomic.Pointer[string]
	countError       atomic.Bool
	fingerprintError atomic.Bool
	publisherCount   atomic.Pointer[int]
	retained         map[string]any
}

func (c *fakeClient) Subscribe(subscription ros2.Subscription) error {
	if c.subscriptions.Add(1) == 1 && c.failFirst {
		return errors.New("type temporarily unavailable")
	}
	select {
	case c.lastTopic <- subscription.Topic:
	default:
	}
	if c.retained != nil {
		c.batches <- []ros2.Message{{Key: subscription.Key, Data: c.retained}}
	}
	return nil
}

func (c *fakeClient) Poll(timeout time.Duration) ([]ros2.Message, error) {
	select {
	case batch := <-c.batches:
		return batch, nil
	case <-time.After(timeout):
		return nil, nil
	}
}

func (c *fakeClient) PublisherCount(string) (int, error) {
	if c.countError.Load() {
		return 0, errors.New("publisher count unavailable")
	}
	if count := c.publisherCount.Load(); count != nil {
		return *count, nil
	}
	return 1, nil
}
func (c *fakeClient) PublisherFingerprint(string) (string, error) {
	if c.fingerprintError.Load() {
		return "", errors.New("publisher fingerprint unavailable")
	}
	if fingerprint := c.fingerprint.Load(); fingerprint != nil {
		return *fingerprint, nil
	}
	return "initial-publisher", nil
}
func (c *fakeClient) Close() error { c.closed.Store(true); return nil }

func TestIndependentDomainsAndSubscriptionRetry(t *testing.T) {
	client := &fakeClient{batches: make(chan []ros2.Message, 1), lastTopic: make(chan string, 1), failFirst: true}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager, err := newManager([]ros2.Config{{Name: "working", DomainID: 51, Namespace: "/robot"}, {Name: "offline", DomainID: 52}},
		[]telemetry.Source{{Key: "state", Topic: "/api/state", Type: "pkg/msg/State", Latched: true}},
		Options{PollInterval: time.Millisecond, RetryInterval: 5 * time.Millisecond}, logger,
		func(cfg ros2.Config) (Client, error) {
			if cfg.DomainID == 52 {
				return nil, errors.New("domain unavailable")
			}
			return client, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { manager.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case topic := <-client.lastTopic:
		if topic != "/robot/api/state" {
			t.Fatalf("topic=%q", topic)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscription was not retried")
	}
	client.batches <- []ros2.Message{{Key: "state", Data: map[string]any{"state": 2}}}
	eventually(t, func() bool { return manager.Snapshots()[0].Sources["state"].MessagesTotal == 1 })
	snapshots := manager.Snapshots()
	if !snapshots[0].Connected || snapshots[1].Connected || snapshots[1].Sources["state"].Data != nil {
		t.Fatalf("domains were not isolated: %+v", snapshots)
	}
	if snapshots[0].Sources["state"].ErrorsTotal != 1 {
		t.Fatalf("subscription errors=%d", snapshots[0].Sources["state"].ErrorsTotal)
	}
	delete(snapshots[0].Sources, "state")
	if len(manager.Snapshots()[0].Sources) != 1 {
		t.Fatal("snapshot map aliases manager state")
	}
}

func TestMissingMessageInvalidatesDataAndShutdownClearsSession(t *testing.T) {
	client := &fakeClient{batches: make(chan []ros2.Message, 2), lastTopic: make(chan string, 1)}
	manager, err := newManager([]ros2.Config{{Name: "robot", DomainID: 0}},
		[]telemetry.Source{{Key: "state", Topic: "/state", Type: "pkg/msg/State", Latched: true}},
		Options{PollInterval: time.Millisecond, RetryInterval: time.Millisecond}, slog.New(slog.NewTextHandler(io.Discard, nil)),
		func(ros2.Config) (Client, error) { return client, nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { manager.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	client.batches <- []ros2.Message{{Key: "state", Data: map[string]any{"state": 1}}}
	eventually(t, func() bool { return manager.Snapshots()[0].Sources["state"].Data != nil })
	client.batches <- []ros2.Message{{Key: "state"}, {Key: "unknown", Data: map[string]any{}}}
	eventually(t, func() bool { return manager.Snapshots()[0].Sources["state"].ErrorsTotal == 1 })
	source := manager.Snapshots()[0].Sources["state"]
	if source.Data != nil || source.MessagesTotal != 2 || source.ReceivedAt.IsZero() {
		t.Fatalf("invalid message state=%+v", source)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("manager did not stop")
	}
	snapshot := manager.Snapshots()[0]
	if snapshot.Connected || snapshot.Sources["state"].Subscribed || !client.closed.Load() {
		t.Fatal("shutdown retained an active session")
	}
}

func TestSourceFailureDoesNotRestartVehicleOrDiscardOtherMessages(t *testing.T) {
	client := &fakeClient{batches: make(chan []ros2.Message, 1), lastTopic: make(chan string, 2)}
	manager, err := newManager([]ros2.Config{{Name: "robot", DomainID: 51}},
		[]telemetry.Source{{Key: "state", Topic: "/state", Type: "pkg/msg/State", Latched: true}, {Key: "objects", Topic: "/objects", Type: "pkg/msg/Objects", MaxAge: time.Second}},
		Options{PollInterval: time.Millisecond, RetryInterval: time.Millisecond}, slog.New(slog.NewTextHandler(io.Discard, nil)),
		func(ros2.Config) (Client, error) { return client, nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { manager.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	client.batches <- []ros2.Message{{Key: "objects", Error: errors.New("message exceeds native conversion limit")}, {Key: "state", Data: map[string]any{"state": 2}}}
	eventually(t, func() bool { return manager.Snapshots()[0].Sources["state"].MessagesTotal == 1 })
	snapshot := manager.Snapshots()[0]
	if !snapshot.Connected || !snapshot.Sources["state"].Fresh(time.Now()) || client.closed.Load() {
		t.Fatalf("source failure interrupted vehicle context: %+v", snapshot)
	}
	if failed := snapshot.Sources["objects"]; failed.ErrorsTotal != 1 || failed.Data != nil {
		t.Fatalf("failed source state=%+v", failed)
	}
}

func TestPublisherReplacementInvalidatesRetainedStateEvenWhenCountStaysOne(t *testing.T) {
	first := &fakeClient{batches: make(chan []ros2.Message, 1), lastTopic: make(chan string, 1)}
	second := &fakeClient{batches: make(chan []ros2.Message, 1), lastTopic: make(chan string, 1)}
	var dials atomic.Int64
	manager, err := newManager([]ros2.Config{{Name: "robot", DomainID: 51}},
		[]telemetry.Source{{Key: "state", Topic: "/state", Type: "pkg/msg/State", Latched: true}},
		Options{PollInterval: time.Millisecond, RetryInterval: time.Millisecond}, slog.New(slog.NewTextHandler(io.Discard, nil)),
		func(ros2.Config) (Client, error) {
			if dials.Add(1) == 1 {
				return first, nil
			}
			return second, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { manager.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	first.batches <- []ros2.Message{{Key: "state", Data: map[string]any{"state": 3}}}
	eventually(t, func() bool { return manager.Snapshots()[0].Sources["state"].Fresh(time.Now()) })
	first.fingerprint.Store(new("replacement-publisher"))
	eventually(t, func() bool { return dials.Load() >= 2 && first.closed.Load() })
	if manager.Snapshots()[0].Sources["state"].Data != nil {
		t.Fatal("previous publisher's state survived its replacement")
	}
}

func TestPublisherDiscoveryRecovery(t *testing.T) {
	for _, test := range []struct {
		name       string
		latched    bool
		fail       func(*fakeClient)
		recover    func(*fakeClient)
		wantErrors uint64
	}{
		{"notification count error", true, func(c *fakeClient) { c.countError.Store(true) }, func(c *fakeClient) { c.countError.Store(false) }, 1},
		{"notification fingerprint error", true, func(c *fakeClient) { c.fingerprintError.Store(true) }, func(c *fakeClient) { c.fingerprintError.Store(false) }, 1},
		{"notification publisher disappears", true, func(c *fakeClient) { c.publisherCount.Store(new(0)) }, func(c *fakeClient) { c.publisherCount.Store(new(1)) }, 0},
		{"periodic count error", false, func(c *fakeClient) { c.countError.Store(true) }, func(c *fakeClient) { c.countError.Store(false) }, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				first := &fakeClient{batches: make(chan []ros2.Message, 1), retained: map[string]any{"state": 1}}
				second := &fakeClient{batches: make(chan []ros2.Message, 1), retained: map[string]any{"state": 2}}
				var dials atomic.Int64
				manager, err := newManager([]ros2.Config{{Name: "robot", DomainID: 51}},
					[]telemetry.Source{{Key: "state", Topic: "/state", Type: "pkg/msg/State", Latched: test.latched, MaxAge: 10 * time.Second}},
					Options{PollInterval: 50 * time.Millisecond, RetryInterval: time.Second}, slog.New(slog.NewTextHandler(io.Discard, nil)),
					func(ros2.Config) (Client, error) {
						if dials.Add(1) == 1 {
							return first, nil
						}
						return second, nil
					})
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(t.Context())
				done := make(chan struct{})
				go func() { manager.Run(ctx); close(done) }()
				t.Cleanup(func() { cancel(); <-done })
				synctest.Wait()
				initial := manager.Snapshots()[0].Sources["state"]
				if !initial.Fresh(time.Now()) || initial.MessagesTotal != 1 {
					t.Fatalf("initial state unavailable: %+v", initial)
				}

				test.fail(first)
				time.Sleep(time.Second)
				synctest.Wait()
				failed := manager.Snapshots()[0].Sources["state"]
				if failed.Data != nil || failed.Publishers != 0 || failed.ErrorsTotal != test.wantErrors {
					t.Fatalf("discovery failure did not invalidate state: %+v", failed)
				}
				if dials.Load() != 1 || first.closed.Load() {
					t.Fatal("discovery failure restarted the ROS context before recovery")
				}

				test.recover(first)
				time.Sleep(time.Second)
				synctest.Wait()
				if test.latched {
					if dials.Load() != 2 || !first.closed.Load() || second.subscriptions.Load() != 1 {
						t.Fatal("notification subscription was not recreated for the unchanged publisher")
					}
				} else {
					if dials.Load() != 1 || first.closed.Load() {
						t.Fatal("periodic source recovery unnecessarily restarted the ROS context")
					}
					first.batches <- []ros2.Message{{Key: "state", Data: map[string]any{"state": 2}}}
					synctest.Wait()
				}
				recovered := manager.Snapshots()[0].Sources["state"]
				if !recovered.Fresh(time.Now()) || recovered.Data["state"] != 2 || recovered.MessagesTotal != 2 || !recovered.ReceivedAt.After(initial.ReceivedAt) {
					t.Fatalf("current state was not received after recovery: %+v", recovered)
				}
				time.Sleep(2 * time.Second)
				synctest.Wait()
				if second.closed.Load() || second.subscriptions.Load() > 1 {
					t.Fatal("successful recovery caused repeated subscription refreshes")
				}
			})
		})
	}
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition did not become true")
		}
		time.Sleep(time.Millisecond)
	}
}
