//go:build linux && cgo && ros2

package integration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Adanomy/autoware-exporter/internal/exporter"
	"github.com/Adanomy/autoware-exporter/internal/monitor"
	"github.com/Adanomy/autoware-exporter/internal/ros2"
	"github.com/Adanomy/autoware-exporter/internal/telemetry"
	"github.com/prometheus/client_golang/prometheus"
)

// TestLiveVehicles reads existing ROS publishers and never creates a publisher,
// service request, route, or vehicle command. It is opt-in for a live deployment.
func TestLiveVehicles(t *testing.T) {
	domains := os.Getenv("AUTOWARE_EXPORTER_LIVE_DOMAINS")
	if domains == "" {
		t.Skip("set AUTOWARE_EXPORTER_LIVE_DOMAINS=51,52,53 to inspect existing vehicles")
	}
	var vehicles []ros2.Config
	seen := make(map[int]bool)
	for raw := range strings.SplitSeq(domains, ",") {
		domain, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || domain < 0 || domain > 232 || seen[domain] {
			t.Fatalf("invalid or duplicate live domain %q", raw)
		}
		seen[domain] = true
		vehicles = append(vehicles, ros2.Config{Name: fmt.Sprintf("vehicle-%d", domain), DomainID: domain})
	}
	manager, err := monitor.New(vehicles, telemetry.Sources(), monitor.Options{PollInterval: 20 * time.Millisecond, RetryInterval: time.Second}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { manager.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	requiredSources := []string{"operation_mode", "kinematics", "vehicle_status", "diagnostics_struct", "diagnostics_status"}
	requiredMetrics := []string{"autoware_vehicle_speed_meters_per_second", "autoware_operation_mode", "autoware_diagnostic_level"}
	if os.Getenv("AUTOWARE_EXPORTER_LIVE_PRODUCT_SOURCES") == "1" {
		// This stricter check targets the evaluator-enabled deployment. Empty
		// event arrays (stops, traffic lights and doors) remain valid and are
		// deliberately not required to produce measurements.
		requiredSources = append(requiredSources, "control_evaluator", "planning_evaluator", "perception_analytics", "velocity_limit", "closest_velocity_limit", "emergency_command", "emergency_holding", "hazard_status")
		requiredMetrics = append(requiredMetrics, "autoware_velocity_limit_meters_per_second", "autoware_emergency_command", "autoware_emergency_holding", "autoware_hazard_level", "autoware_perception_latency_seconds")
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		ready := true
		for _, snapshot := range manager.Snapshots() {
			for _, key := range requiredSources {
				ready = ready && snapshot.Connected && snapshot.Sources[key].Fresh(time.Now())
			}
		}
		if ready {
			break
		}
		if time.Now().After(deadline) {
			for _, snapshot := range manager.Snapshots() {
				for key, source := range snapshot.Sources {
					t.Logf("%s %s subscribed=%t publishers=%d messages=%d errors=%d fresh=%t", snapshot.Name, key, source.Subscribed, source.Publishers, source.MessagesTotal, source.ErrorsTotal, source.Fresh(time.Now()))
				}
			}
			t.Fatal("required data did not become available for every configured vehicle")
		}
		time.Sleep(100 * time.Millisecond)
	}
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(exporter.NewCollector(manager, "integration", "uncommitted"))
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, metricName := range requiredMetrics {
		present := make(map[string]bool)
		for _, family := range families {
			if family.GetName() != metricName {
				continue
			}
			for _, metric := range family.Metric {
				for _, label := range metric.Label {
					if label.GetName() == "vehicle" {
						present[label.GetValue()] = true
					}
				}
			}
		}
		for _, vehicle := range vehicles {
			if !present[vehicle.Name] {
				t.Errorf("%s missing for %s", metricName, vehicle.Name)
			}
		}
	}
	for _, snapshot := range manager.Snapshots() {
		available := 0
		for _, source := range snapshot.Sources {
			if source.Fresh(time.Now()) {
				available++
			}
		}
		t.Logf("%s: %d/%d sources available", snapshot.Name, available, len(snapshot.Sources))
	}
}
