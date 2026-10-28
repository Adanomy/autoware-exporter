//go:build linux && cgo && ros2

package compatibility

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Adanomy/autoware-exporter/internal/exporter"
	"github.com/Adanomy/autoware-exporter/internal/monitor"
	"github.com/Adanomy/autoware-exporter/internal/ros2"
	"github.com/Adanomy/autoware-exporter/internal/telemetry"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type publisherSource struct {
	Key     string
	Topic   string
	Type    string
	Latched bool
}

// TestAutowareMessages uses upstream-generated Python publishers and the real
// C subscriber, introspection decoder and Prometheus collector. No Autoware
// version is supplied to production code. Run only in the isolated CI image.
func TestAutowareMessages(t *testing.T) {
	if os.Getenv("AUTOWARE_EXPORTER_COMPATIBILITY_TESTS") != "1" {
		t.Skip("set AUTOWARE_EXPORTER_COMPATIBILITY_TESTS=1 inside the compatibility image")
	}
	sources := telemetry.Sources()
	plan := make([]publisherSource, len(sources))
	for i, source := range sources {
		plan[i] = publisherSource{Key: source.Key, Topic: source.Topic, Type: source.Type, Latched: source.Latched}
	}
	payload, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "publisher.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logFile.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	t.Cleanup(cancel)
	publisher := exec.CommandContext(ctx, "python3", "publish.py")
	publisher.Stdin = bytes.NewReader(payload)
	publisher.Stdout, publisher.Stderr = logFile, logFile
	if err := publisher.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = publisher.Process.Kill()
		_ = publisher.Wait()
		if t.Failed() {
			logs, _ := os.ReadFile(logPath)
			t.Logf("publisher output:\n%s", logs)
		}
	})
	manager, err := monitor.New([]ros2.Config{{Name: "compatibility", DomainID: 200, Namespace: "/compatibility"}},
		sources, monitor.Options{PollInterval: 10 * time.Millisecond, RetryInterval: 100 * time.Millisecond},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	managerCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { manager.Run(managerCtx); close(done) }()
	t.Cleanup(func() { stop(); <-done })
	deadline := time.Now().Add(40 * time.Second)
	for {
		snapshots := manager.Snapshots()
		ready := len(snapshots) == 1 && snapshots[0].Connected && len(snapshots[0].Sources) == len(sources)
		for _, snapshot := range snapshots {
			for _, source := range snapshot.Sources {
				ready = ready && source.Fresh(time.Now()) && source.MessagesTotal > 0
			}
		}
		if ready {
			for key, source := range snapshots[0].Sources {
				if source.ErrorsTotal != 0 {
					t.Errorf("%s: %d receive/decode errors", key, source.ErrorsTotal)
				}
			}
			break
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			for _, snapshot := range snapshots {
				for key, source := range snapshot.Sources {
					if !source.Fresh(time.Now()) {
						t.Logf("%s: subscribed=%t publishers=%d messages=%d errors=%d data=%v", key,
							source.Subscribed, source.Publishers, source.MessagesTotal, source.ErrorsTotal, source.Data)
					}
				}
			}
			t.Fatal("not every production source received a valid upstream-generated message")
		}
		time.Sleep(50 * time.Millisecond)
	}
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(exporter.NewCollector(manager, "compatibility", "test"))
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	metrics := make(map[string]*dto.MetricFamily, len(families))
	for _, family := range families {
		metrics[family.GetName()] = family
	}
	checks := []struct {
		name   string
		value  float64
		labels map[string]string
	}{
		{"operation_mode", 1, map[string]string{"mode": "autonomous"}},
		{"localization_state", 1, map[string]string{"state": "initialized"}},
		{"route_state", 1, map[string]string{"state": "set"}},
		{"motion_state", 1, map[string]string{"state": "moving"}},
		{"mrm_state", 1, map[string]string{"state": "normal"}},
		{"mrm_behavior", 1, map[string]string{"behavior": "none"}},
		{"control_enabled", 1, nil},
		{"heartbeat_sequence", 42, nil},
		{"ros_time_seconds", 12.5, nil},
		{"vehicle_speed_meters_per_second", 5, nil},
		{"vehicle_position_meters", 12, map[string]string{"axis": "x"}},
		{"vehicle_acceleration_meters_per_second_squared", 1.5, map[string]string{"axis": "x"}},
		{"vehicle_steering_angle_radians", 0.25, nil},
		{"vehicle_gear", 1, map[string]string{"gear": "drive"}},
		{"vehicle_turn_indicator", 1, map[string]string{"state": "left"}},
		{"vehicle_hazard_lights", 1, map[string]string{"state": "disabled"}},
		{"vehicle_energy_ratio", 0.75, nil},
		{"diagnostic_level", 2, map[string]string{"path": "/compatibility/component"}},
		{"command_velocity_meters_per_second", 6, nil},
		{"command_acceleration_meters_per_second_squared", 1.25, nil},
		{"command_steering_angle_radians", 0.5, nil},
		{"command_throttle_ratio", 0.4, nil},
		{"command_jerk_meters_per_second_cubed", 0.75, nil},
		{"control_lateral_error_meters", 0.125, nil},
		{"control_jerk_meters_per_second_cubed", 0.5, map[string]string{"axis": "longitudinal"}},
		{"localization_ndt_score", 1.5, map[string]string{"kind": "transform_probability"}},
		{"localization_ndt_iterations", 7, nil},
		{"localization_ndt_execution_seconds", 0.025, nil},
		{"perception_objects_by_class", 1, map[string]string{"class": "car"}},
		{"processing_duration_seconds", 0.01, map[string]string{"module": "lateral_controller"}},
		{"pipeline_latency_seconds", 0.1, nil},
		{"route_remaining_distance_meters", 125, nil},
		{"route_estimated_remaining_seconds", 30, nil},
		{"goal_deviation_meters", -2.5, map[string]string{"axis": "longitudinal", "kind": "signed"}},
		{"planning_steering_factor_distance_meters", 4, map[string]string{"point": "end"}},
		{"planning_velocity_factor_distance_meters", 3, nil},
		{"velocity_limit_meters_per_second", 8, map[string]string{"scope": "global"}},
		{"velocity_limit_meters_per_second", 7, map[string]string{"scope": "local"}},
		{"emergency_command", 0, nil},
		{"emergency_holding", 1, nil},
		{"hazard_level", 1, nil},
		{"perception_latency_seconds", 0.05, map[string]string{"stage": "total"}},
		{"planning_trajectory_length_meters", 20, map[string]string{"statistic": "mean"}},
		{"traffic_signal_confidence_ratio", 0.75, map[string]string{"color": "red"}},
		{"door_state", 1, map[string]string{"state": "closed"}},
		{"aeb_decision", 1, map[string]string{"decision": "brake"}},
		{"aeb_rss_distance_meters", -2.5, nil},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			for _, metric := range metrics["autoware_"+check.name].GetMetric() {
				matches := true
				for name, value := range check.labels {
					found := false
					for _, label := range metric.Label {
						found = found || label.GetName() == name && label.GetValue() == value
					}
					matches = matches && found
				}
				if matches && metric.Gauge != nil && math.Abs(metric.Gauge.GetValue()-check.value) <= 1e-6 {
					return
				}
			}
			t.Fatalf("missing autoware_%s%v = %g", check.name, check.labels, check.value)
		})
	}
	t.Logf("received %d ROS sources; checked %d metric observations", len(sources), len(checks))
}
