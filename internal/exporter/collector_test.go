package exporter

import (
	"encoding/json/v2"
	"math"
	"testing"
	"time"

	"github.com/Adanomy/autoware-exporter/internal/telemetry"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type fixtureProvider []telemetry.VehicleSnapshot

func (p fixtureProvider) Snapshots() []telemetry.VehicleSnapshot { return p }

var fixtureTime = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func sample(t *testing.T, key, raw string) telemetry.SourceSnapshot {
	t.Helper()
	for _, spec := range telemetry.Sources() {
		if spec.Key != key {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(raw), &data); err != nil {
			t.Fatal(err)
		}
		return telemetry.SourceSnapshot{Spec: spec, Publishers: 1, Subscribed: true, ReceivedAt: fixtureTime, MessagesTotal: 4, Data: data}
	}
	t.Fatalf("unknown fixture source %q", key)
	return telemetry.SourceSnapshot{}
}

func vehicle(name string, samples ...telemetry.SourceSnapshot) telemetry.VehicleSnapshot {
	v := telemetry.VehicleSnapshot{Name: name, DomainID: 51, Connected: true, Sources: make(map[string]telemetry.SourceSnapshot)}
	for _, s := range samples {
		v.Sources[s.Spec.Key] = s
	}
	return v
}

func gather(t *testing.T, vehicles ...telemetry.VehicleSnapshot) map[string]*dto.MetricFamily {
	t.Helper()
	c := NewCollector(fixtureProvider(vehicles), "test", "test-revision")
	c.now = func() time.Time { return fixtureTime }
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(c)
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[string]*dto.MetricFamily)
	for _, family := range families {
		result[family.GetName()] = family
	}
	return result
}

func value(families map[string]*dto.MetricFamily, name string, labels map[string]string) (float64, bool) {
	for _, m := range families["autoware_"+name].GetMetric() {
		matches := true
		for key, expected := range labels {
			found := false
			for _, label := range m.Label {
				if label.GetName() == key && label.GetValue() == expected {
					found = true
				}
			}
			if !found {
				matches = false
			}
		}
		if matches {
			if m.Gauge != nil {
				return m.Gauge.GetValue(), true
			}
			if m.Counter != nil {
				return m.Counter.GetValue(), true
			}
		}
	}
	return 0, false
}

func expect(t *testing.T, metrics map[string]*dto.MetricFamily, name string, want float64, labels map[string]string) {
	t.Helper()
	got, ok := value(metrics, name, labels)
	if !ok || math.Abs(got-want) > 1e-9 {
		t.Errorf("%s %v = %v, available=%v; want %v", name, labels, got, ok, want)
	}
}

func absent(t *testing.T, metrics map[string]*dto.MetricFamily, name string, labels map[string]string) {
	t.Helper()
	if got, ok := value(metrics, name, labels); ok {
		t.Errorf("%s %v unexpectedly available: %v", name, labels, got)
	}
}

func TestCollectorSeparatesVehiclesAndMissingValues(t *testing.T) {
	first := vehicle("first", sample(t, "operation_mode", `{"mode":1,"is_autoware_control_enabled":false}`), sample(t, "command_velocity", `{"velocity":0}`))
	second := vehicle("second", sample(t, "operation_mode", `{"mode":2,"is_autoware_control_enabled":true}`), sample(t, "command_velocity", `{"velocity":-3.5}`))
	missing := vehicle("missing")
	metrics := gather(t, first, second, missing)
	expect(t, metrics, "operation_mode", 1, map[string]string{"vehicle": "first", "mode": "stop"})
	expect(t, metrics, "operation_mode", 0, map[string]string{"vehicle": "first", "mode": "autonomous"})
	expect(t, metrics, "operation_mode", 1, map[string]string{"vehicle": "second", "mode": "autonomous"})
	expect(t, metrics, "control_enabled", 0, map[string]string{"vehicle": "first"})
	expect(t, metrics, "control_enabled", 1, map[string]string{"vehicle": "second"})
	expect(t, metrics, "command_velocity_meters_per_second", 0, map[string]string{"vehicle": "first"})
	expect(t, metrics, "command_velocity_meters_per_second", -3.5, map[string]string{"vehicle": "second"})
	absent(t, metrics, "command_velocity_meters_per_second", map[string]string{"vehicle": "missing"})
	absent(t, metrics, "operation_mode", map[string]string{"vehicle": "missing"})
}

func TestFreshnessAndDisconnectNeverInventHealthyZero(t *testing.T) {
	oldState := sample(t, "route_state", `{"state":2}`)
	oldState.ReceivedAt = fixtureTime.Add(-time.Hour)
	oldVelocity := sample(t, "command_velocity", `{"velocity":0}`)
	oldVelocity.ReceivedAt = fixtureTime.Add(-time.Hour)
	noPublisher := sample(t, "motion_state", `{"state":1}`)
	noPublisher.Publishers = 0
	v := vehicle("active", oldState, oldVelocity, noPublisher)
	metrics := gather(t, v)
	expect(t, metrics, "route_state", 1, map[string]string{"vehicle": "active", "state": "set"})
	absent(t, metrics, "command_velocity_meters_per_second", map[string]string{"vehicle": "active"})
	absent(t, metrics, "motion_state", map[string]string{"vehicle": "active"})
	expect(t, metrics, "topic_data_available", 0, map[string]string{"vehicle": "active", "topic": oldVelocity.Spec.Topic})
	expect(t, metrics, "topic_last_received_age_seconds", 3600, map[string]string{"vehicle": "active", "topic": oldVelocity.Spec.Topic})
	expect(t, metrics, "topic_messages_received_total", 4, map[string]string{"vehicle": "active", "topic": oldVelocity.Spec.Topic})
	v.Connected = false
	metrics = gather(t, v)
	absent(t, metrics, "route_state", map[string]string{"vehicle": "active"})
	expect(t, metrics, "ros_context_up", 0, map[string]string{"vehicle": "active"})
	expect(t, metrics, "topic_data_available", 0, map[string]string{"vehicle": "active", "topic": oldState.Spec.Topic})
}

func TestInvalidEnumsAndNonFiniteNumbersAreOmitted(t *testing.T) {
	mode := sample(t, "operation_mode", `{"mode":255}`)
	velocity := sample(t, "command_velocity", `{"velocity":0}`)
	velocity.Data["velocity"] = math.NaN()
	metrics := gather(t, vehicle("v", mode, velocity,
		sample(t, "mrm_state", `{"state":1,"behavior":99}`),
		sample(t, "energy", `{"energy":-1}`)))
	absent(t, metrics, "operation_mode", nil)
	absent(t, metrics, "command_velocity_meters_per_second", nil)
	absent(t, metrics, "vehicle_energy_ratio", nil)
	absent(t, metrics, "mrm_behavior", nil)
	expect(t, metrics, "mrm_state", 1, map[string]string{"vehicle": "v", "state": "normal"})
	absent(t, metrics, "control_enabled", nil)
}

func TestKinematicsUnitsQuaternionAndCovariance(t *testing.T) {
	data := sample(t, "kinematics", `{
		"pose":{"header":{"stamp":{"sec":12,"nanosec":500000000}},"pose":{"pose":{"position":{"x":3,"y":4,"z":0},"orientation":{"x":0,"y":0,"z":1,"w":1}},"covariance":[4,0,0,0,0,0,0,9,0,0,0,0,0,0,16,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0.25]}},
		"twist":{"twist":{"twist":{"linear":{"x":-3,"y":4,"z":0},"angular":{"x":0,"y":0,"z":0.3}}}},
		"accel":{"accel":{"accel":{"linear":{"x":0,"y":1,"z":0}}}}}`)
	metrics := gather(t, vehicle("v", data))
	expect(t, metrics, "vehicle_velocity_meters_per_second", -3, map[string]string{"vehicle": "v", "axis": "x"})
	expect(t, metrics, "vehicle_speed_meters_per_second", 5, map[string]string{"vehicle": "v"})
	expect(t, metrics, "vehicle_acceleration_meters_per_second_squared", 0, map[string]string{"vehicle": "v", "axis": "x"})
	expect(t, metrics, "vehicle_yaw_radians", math.Pi/2, map[string]string{"vehicle": "v"})
	expect(t, metrics, "vehicle_yaw_rate_radians_per_second", .3, map[string]string{"vehicle": "v"})
	expect(t, metrics, "localization_position_stddev_meters", 2, map[string]string{"vehicle": "v", "source": "kinematics", "axis": "x"})
	expect(t, metrics, "localization_yaw_stddev_radians", .5, map[string]string{"vehicle": "v", "source": "kinematics"})
	expect(t, metrics, "topic_source_timestamp_seconds", 12.5, map[string]string{"vehicle": "v", "component": "pose"})
}

func TestDiagnosticsRequiresMatchingGraphAndBoundsLabels(t *testing.T) {
	structure := sample(t, "diagnostics_struct", `{"id":"a","nodes":[{"path":"/localization"},{"path":"/planning"}],"diags":[{"parent":0,"name":"arbitrary hardware"}]}`)
	status := sample(t, "diagnostics_status", `{"id":"b","nodes":[{"level":0},{"level":2}],"diags":[{"level":1,"message":"unbounded text","hardware_id":"secret-ish"}]}`)
	metrics := gather(t, vehicle("v", structure, status))
	absent(t, metrics, "diagnostic_level", nil)
	absent(t, metrics, "diagnostics", nil)
	status.Data["id"] = "a"
	metrics = gather(t, vehicle("v", structure, status))
	expect(t, metrics, "diagnostic_level", 2, map[string]string{"vehicle": "v", "path": "/planning"})
	expect(t, metrics, "diagnostics", 1, map[string]string{"vehicle": "v", "kind": "node", "level": "ok"})
	expect(t, metrics, "diagnostics", 1, map[string]string{"vehicle": "v", "kind": "leaf", "level": "warn"})
	for _, family := range metrics {
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetValue() == "unbounded text" || label.GetValue() == "arbitrary hardware" || label.GetValue() == "secret-ish" {
					t.Error("diagnostic text leaked into metric label")
				}
			}
		}
	}
}

func TestCommandsRespectDefinedFieldsAndConvertMilliseconds(t *testing.T) {
	control := sample(t, "control_command", `{"longitudinal":{"jerk":0,"is_defined_jerk":false}}`)
	metrics := gather(t, vehicle("v", control,
		sample(t, "command_acceleration", `{"acceleration":0}`),
		sample(t, "command_pedals", `{"throttle":0.4,"brake":1.2}`),
		sample(t, "ndt_execution", `{"data":2.5}`),
		sample(t, "processing_lateral_controller", `{"data":10}`),
		sample(t, "pipeline_latency", `{"data":420}`)))
	absent(t, metrics, "command_jerk_meters_per_second_cubed", nil)
	expect(t, metrics, "command_acceleration_meters_per_second_squared", 0, map[string]string{"vehicle": "v"})
	expect(t, metrics, "command_throttle_ratio", .4, map[string]string{"vehicle": "v"})
	absent(t, metrics, "command_brake_ratio", nil)
	expect(t, metrics, "localization_ndt_execution_seconds", .0025, map[string]string{"vehicle": "v"})
	expect(t, metrics, "processing_duration_seconds", .01, map[string]string{"vehicle": "v", "module": "lateral_controller"})
	expect(t, metrics, "pipeline_latency_seconds", .42, map[string]string{"vehicle": "v"})
	control.Data["longitudinal"].(map[string]any)["is_defined_jerk"] = true
	metrics = gather(t, vehicle("v", control))
	expect(t, metrics, "command_jerk_meters_per_second_cubed", 0, map[string]string{"vehicle": "v"})
}

func TestObjectsUseOneClassificationPerObject(t *testing.T) {
	objects := sample(t, "objects", `{"objects":[
		{"object_id":"never-a-label","classification":[{"label":1,"probability":0.4},{"label":7,"probability":0.9}]},
		{"classification":[]},
		{"classification":[{"label":99,"probability":1}]}]}`)
	metrics := gather(t, vehicle("v", objects))
	expect(t, metrics, "perception_objects", 3, map[string]string{"vehicle": "v"})
	expect(t, metrics, "perception_objects_by_class", 1, map[string]string{"vehicle": "v", "class": "pedestrian"})
	expect(t, metrics, "perception_objects_by_class", 2, map[string]string{"vehicle": "v", "class": "unknown"})
	expect(t, metrics, "perception_objects_by_class", 0, map[string]string{"vehicle": "v", "class": "car"})
	metrics = gather(t, vehicle("v", sample(t, "objects", `{}`)))
	absent(t, metrics, "perception_objects", nil)
}

func TestRouteEstimatesRequireValidRoute(t *testing.T) {
	estimates := sample(t, "remaining_route", `{"remaining_distance":0,"remaining_time":10}`)
	route := sample(t, "route_state", `{"state":1}`)
	metrics := gather(t, vehicle("v", estimates, route))
	absent(t, metrics, "route_remaining_distance_meters", nil)
	route.Data["state"] = float64(2)
	metrics = gather(t, vehicle("v", estimates, route))
	expect(t, metrics, "route_remaining_distance_meters", 0, map[string]string{"vehicle": "v"})
	expect(t, metrics, "route_estimated_remaining_seconds", 10, map[string]string{"vehicle": "v"})
}
