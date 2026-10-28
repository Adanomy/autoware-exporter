package exporter

import (
	"encoding/json/v2"
	"math"
	"testing"
	"time"

	"github.com/Adanomy/autoware-exporter/internal/telemetry"
	dto "github.com/prometheus/client_model/go"
)

type evaluatorEntry struct {
	Name  string `json:"name"`
	Unit  string `json:"unit"`
	Value string `json:"value"`
}

func evaluatorSample(t *testing.T, key string, entries ...evaluatorEntry) telemetry.SourceSnapshot {
	t.Helper()
	if entries == nil {
		entries = []evaluatorEntry{}
	}
	data, err := json.Marshal(map[string]any{"metric_array": entries})
	if err != nil {
		t.Fatal(err)
	}
	return sample(t, key, string(data))
}

func TestProductGoalCenterlineAndRawBoundarySemantics(t *testing.T) {
	control := evaluatorSample(t, "control_evaluator",
		evaluatorEntry{"goal_longitudinal_deviation", "", "-2.5"},
		evaluatorEntry{"goal_longitudinal_deviation_abs", "", "3.5"},
		evaluatorEntry{"goal_lateral_deviation", "m", "0"},
		evaluatorEntry{"goal_yaw_deviation", "rad", "-0.3"},
		evaluatorEntry{"goal_yaw_deviation_abs", "", "0.4"},
		evaluatorEntry{"lateral_deviation_centerline", "", "-1.1"},
		evaluatorEntry{"lateral_deviation_centerline_abs", "", "1.2"},
		evaluatorEntry{"left_boundary_distance", "", "2.1"},
		evaluatorEntry{"left_uncrossable_boundary_distance", "", "8.79"},
		evaluatorEntry{"left_uncrossable_boundary_distance_count", "", "0"},
		evaluatorEntry{"right_uncrossable_boundary_distance", "", "-0.2"},
		evaluatorEntry{"right_uncrossable_boundary_distance_count", "", "2"})
	metrics := gather(t, vehicle("v", control))
	expect(t, metrics, "goal_deviation_meters", -2.5, map[string]string{"vehicle": "v", "axis": "longitudinal", "kind": "signed"})
	// Absolute input is deliberately different from abs(signed): no exporter math.
	expect(t, metrics, "goal_deviation_meters", 3.5, map[string]string{"vehicle": "v", "axis": "longitudinal", "kind": "absolute"})
	expect(t, metrics, "goal_deviation_meters", 0, map[string]string{"vehicle": "v", "axis": "lateral", "kind": "signed"})
	expect(t, metrics, "goal_yaw_deviation_radians", -.3, map[string]string{"vehicle": "v", "kind": "signed"})
	expect(t, metrics, "goal_yaw_deviation_radians", .4, map[string]string{"vehicle": "v", "kind": "absolute"})
	expect(t, metrics, "centerline_deviation_meters", -1.1, map[string]string{"vehicle": "v", "kind": "signed"})
	expect(t, metrics, "centerline_deviation_meters", 1.2, map[string]string{"vehicle": "v", "kind": "absolute"})
	expect(t, metrics, "boundary_distance_meters", 8.79, map[string]string{"vehicle": "v", "side": "left", "boundary": "uncrossable"})
	expect(t, metrics, "boundary_crossings", 0, map[string]string{"vehicle": "v", "side": "left", "boundary": "uncrossable"})
	expect(t, metrics, "boundary_distance_meters", -.2, map[string]string{"vehicle": "v", "side": "right", "boundary": "uncrossable"})
	expect(t, metrics, "boundary_crossings", 2, map[string]string{"vehicle": "v", "side": "right", "boundary": "uncrossable"})
	absent(t, metrics, "boundary_crossings", map[string]string{"vehicle": "v", "side": "left", "boundary": "lane"})
	if metrics["autoware_boundary_crossings"].GetType() != dto.MetricType_GAUGE {
		t.Fatal("upstream count snapshot was converted into an exporter counter")
	}
}

func TestMetricArrayRejectsMalformedNonfiniteDuplicatesAndWrongUnits(t *testing.T) {
	control := sample(t, "control_evaluator", `{"metric_array":[
		{"name":"goal_longitudinal_deviation","unit":"","value":"1"},
		{"name":"goal_longitudinal_deviation","unit":"","value":"not-a-number"},
		{"name":"goal_lateral_deviation","unit":"","value":"NaN"},
		{"name":"goal_yaw_deviation","unit":"","value":"+Inf"},
		{"name":"goal_yaw_deviation_abs","unit":"","value":"-1"},
		{"name":"lateral_deviation_centerline","unit":"km","value":"10"},
		{"name":"lateral_deviation_centerline_abs","value":"20"},
		{"name":"right_boundary_distance","unit":"","value":""},
		{"name":"left_uncrossable_boundary_distance_count","unit":"","value":"1.5"},
		{"name":"right_uncrossable_boundary_distance_count","unit":"","value":"-1"},
		{"name":"uuid/free-form-metric","unit":"m","value":"50"},
		{"name":"left_boundary_distance","unit":"","value":" -2e-1 "},
		null,7,{"name":{},"unit":"","value":"7"}]}`)
	metrics := gather(t, vehicle("v", control))
	for _, name := range []string{"goal_deviation_meters", "goal_yaw_deviation_radians", "centerline_deviation_meters", "boundary_crossings"} {
		absent(t, metrics, name, nil)
	}
	expect(t, metrics, "boundary_distance_meters", -.2, map[string]string{"vehicle": "v", "side": "left", "boundary": "lane"})
	if count := len(parseMetricArray(control.Data, controlEvaluatorUnits)); count != 4 {
		t.Fatalf("accepted%d finite values, expected one distance plus three domain-invalid values rejected by the collector", count)
	}
	metrics = gather(t, vehicle("v", evaluatorSample(t, "control_evaluator")))
	absent(t, metrics, "goal_deviation_meters", nil)
}

func TestProductFactorIndexesPreserveDuplicatesAndBoundText(t *testing.T) {
	steering := sample(t, "steering_factors", `{"factors":[
		{"behavior":"start-planner","direction":1,"status":1,"distance":[2,4],"detail":"private-free-form","sequence":"uuid-value"},
		{"behavior":"start-planner","direction":1,"status":1,"distance":[3,5]},
		{"behavior":"new-extension-with-secret","direction":99,"status":99,"distance":[-1,8]},
		{"behavior":"another-extension","direction":99,"status":99,"distance":[9,10]}]}`)
	velocity := sample(t, "velocity_factors", `{"factors":[
		{"behavior":"traffic-signal","status":2,"distance":1},
		{"behavior":"traffic-signal","status":1,"distance":2},
		{"behavior":"","status":0,"distance":0}]}`)
	metrics := gather(t, vehicle("v", steering, velocity))
	expect(t, metrics, "planning_steering_factor", 1, map[string]string{"vehicle": "v", "index": "0", "behavior": "start-planner", "direction": "left", "status": "approaching"})
	expect(t, metrics, "planning_steering_factor_distance_meters", 5, map[string]string{"vehicle": "v", "index": "1", "point": "end"})
	expect(t, metrics, "planning_steering_factor_distance_meters", -1, map[string]string{"vehicle": "v", "index": "2", "behavior": "unknown", "direction": "unknown", "status": "unknown", "point": "start"})
	expect(t, metrics, "planning_steering_factor_distance_meters", 10, map[string]string{"vehicle": "v", "index": "3", "point": "end"})
	expect(t, metrics, "planning_velocity_factor_distance_meters", 1, map[string]string{"vehicle": "v", "index": "0", "behavior": "traffic-signal", "status": "stopped"})
	expect(t, metrics, "planning_velocity_factor_distance_meters", 2, map[string]string{"vehicle": "v", "index": "1", "status": "approaching"})
	expect(t, metrics, "planning_velocity_factor", 1, map[string]string{"vehicle": "v", "index": "2", "behavior": "unknown", "status": "unknown"})
	if len(metrics["autoware_planning_steering_factor"].Metric) != 4 {
		t.Fatal("repeated or unknown factors collapsed into one series")
	}
	for _, family := range metrics {
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				switch label.GetValue() {
				case "private-free-form", "uuid-value", "new-extension-with-secret", "another-extension":
					t.Error("unbounded factor text leaked into labels")
				}
			}
		}
	}
}

func TestProductFactorEmptyMalformedAndOversizeArrays(t *testing.T) {
	for _, raw := range []string{`{"factors":[]}`, `{"factors":[null,{"status":"bad","behavior":"crosswalk","distance":1}]}`, `{"factors":{}}`} {
		metrics := gather(t, vehicle("v", sample(t, "velocity_factors", raw)))
		absent(t, metrics, "planning_velocity_factor", nil)
		absent(t, metrics, "planning_velocity_factor_distance_meters", nil)
	}
	tooMany := make([]any, maxFactorEntries+1)
	for i := range tooMany {
		tooMany[i] = map[string]any{"behavior": "crosswalk", "status": float64(1), "distance": float64(5)}
	}
	s := sample(t, "velocity_factors", `{}`)
	s.Data["factors"] = tooMany
	metrics := gather(t, vehicle("v", s))
	absent(t, metrics, "planning_velocity_factor", nil)
}

func TestProductLimitsEmergencyHazardAndSignedRSS(t *testing.T) {
	metrics := gather(t, vehicle("v",
		sample(t, "velocity_limit", `{"max_velocity":0,"sender":"never-a-label"}`),
		sample(t, "closest_velocity_limit", `{"data":5.5}`),
		sample(t, "emergency_command", `{"emergency":false}`),
		sample(t, "emergency_holding", `{"is_holding":true}`),
		sample(t, "hazard_status", `{"status":{"level":0,"emergency":false,"emergency_holding":true}}`),
		sample(t, "aeb_rss_distance", `{"data":-2.5}`)))
	expect(t, metrics, "velocity_limit_meters_per_second", 0, map[string]string{"vehicle": "v", "scope": "global"})
	expect(t, metrics, "velocity_limit_meters_per_second", 5.5, map[string]string{"vehicle": "v", "scope": "local"})
	expect(t, metrics, "emergency_command", 0, map[string]string{"vehicle": "v"})
	expect(t, metrics, "emergency_holding", 1, map[string]string{"vehicle": "v"})
	expect(t, metrics, "hazard_level", 0, map[string]string{"vehicle": "v"})
	expect(t, metrics, "hazard_flag", 0, map[string]string{"vehicle": "v", "flag": "emergency"})
	expect(t, metrics, "hazard_flag", 1, map[string]string{"vehicle": "v", "flag": "emergency_holding"})
	expect(t, metrics, "aeb_rss_distance_meters", -2.5, map[string]string{"vehicle": "v"})
	metrics = gather(t, vehicle("v", sample(t, "closest_velocity_limit", `{"max_velocity":9}`), sample(t, "hazard_status", `{"status":{"level":9}}`)))
	absent(t, metrics, "velocity_limit_meters_per_second", nil)
	absent(t, metrics, "hazard_level", nil)
}

func TestProductPublishedPlanningSummariesWindowsAndLatencies(t *testing.T) {
	planning := evaluatorSample(t, "planning_evaluator",
		evaluatorEntry{"stability/min", "", "0"}, evaluatorEntry{"stability/max", "", "3"}, evaluatorEntry{"stability/mean", "", "1"},
		evaluatorEntry{"stability_frechet/min", "", "1"}, evaluatorEntry{"stability_frechet/max", "", "5"}, evaluatorEntry{"stability_frechet/mean", "", "2"},
		evaluatorEntry{"length/min", "m", "10"}, evaluatorEntry{"length/max", "m", "50"}, evaluatorEntry{"length/mean", "m", "20"},
		evaluatorEntry{"duration/min", "s", "1"}, evaluatorEntry{"duration/max", "s", "9"}, evaluatorEntry{"duration/mean", "s", "4"},
		evaluatorEntry{"steer_change_count/count_in_duration", "", "0"}, evaluatorEntry{"blinker_change_count/count_in_duration", "", "2"},
		evaluatorEntry{"obstacle_ttc/worst", "", "1.25"}, evaluatorEntry{"obstacle_ttc/uuid-123", "", "0.1"},
		evaluatorEntry{"obstacle_distance/worst", "", "0.2"})
	analytics := evaluatorSample(t, "perception_analytics", evaluatorEntry{"meas_to_tracked_latency", "ms", "20"}, evaluatorEntry{"prediction_latency", "ms", "30"}, evaluatorEntry{"total_latency", "ms", "50"}, evaluatorEntry{"all_object_count", "count", "9"})
	metrics := gather(t, vehicle("v", planning, analytics))
	expect(t, metrics, "planning_stability_meters", 0, map[string]string{"vehicle": "v", "kind": "stability", "statistic": "min"})
	expect(t, metrics, "planning_stability_meters", 5, map[string]string{"vehicle": "v", "kind": "frechet", "statistic": "max"})
	expect(t, metrics, "planning_trajectory_length_meters", 20, map[string]string{"vehicle": "v", "statistic": "mean"})
	expect(t, metrics, "planning_trajectory_duration_seconds", 9, map[string]string{"vehicle": "v", "statistic": "max"})
	expect(t, metrics, "planning_changes", 0, map[string]string{"vehicle": "v", "signal": "steering"})
	expect(t, metrics, "planning_changes", 2, map[string]string{"vehicle": "v", "signal": "blinker"})
	expect(t, metrics, "planning_time_to_collision_seconds", 1.25, map[string]string{"vehicle": "v", "statistic": "worst"})
	expect(t, metrics, "perception_latency_seconds", .02, map[string]string{"vehicle": "v", "stage": "measurement_to_tracked"})
	expect(t, metrics, "perception_latency_seconds", .03, map[string]string{"vehicle": "v", "stage": "prediction"})
	expect(t, metrics, "perception_latency_seconds", .05, map[string]string{"vehicle": "v", "stage": "total"})
	if metrics["autoware_planning_changes"].GetType() != dto.MetricType_GAUGE {
		t.Fatal("window count must remain a Gauge")
	}
	if got := len(metrics["autoware_planning_time_to_collision_seconds"].Metric); got != 1 {
		t.Fatalf("per-object UUIDs leaked intoTTC: %d series", got)
	}
	if got := len(parseMetricArray(planning.Data, planningEvaluatorUnits)); got != 15 {
		t.Fatalf("planning whitelist accepted %d keys, want15", got)
	}
}

func TestProductTrafficSignalsAndDoorsKeepBoundedSlots(t *testing.T) {
	traffic := sample(t, "traffic_signals", `{"traffic_light_groups":[
		{"traffic_light_group_id":123456789,"elements":[{"color":1,"shape":1,"status":2,"confidence":0.9},{"color":3,"shape":2,"status":3,"confidence":0}]},
		{"traffic_light_group_id":987654321,"elements":[{"color":99,"shape":99,"status":99,"confidence":1.2}]}]}`)
	doors := sample(t, "doors", `{"doors":[{"status":2},{"status":3},{"status":99}]}`)
	metrics := gather(t, vehicle("v", traffic, doors))
	expect(t, metrics, "traffic_signal", 1, map[string]string{"vehicle": "v", "group": "0", "index": "0", "color": "red", "shape": "circle", "status": "solid_on"})
	expect(t, metrics, "traffic_signal_confidence_ratio", 0, map[string]string{"vehicle": "v", "group": "0", "index": "1", "color": "green", "shape": "left_arrow", "status": "flashing"})
	expect(t, metrics, "traffic_signal", 1, map[string]string{"vehicle": "v", "group": "1", "index": "0", "color": "unknown", "shape": "unknown", "status": "unknown"})
	absent(t, metrics, "traffic_signal_confidence_ratio", map[string]string{"vehicle": "v", "group": "1", "index": "0"})
	expect(t, metrics, "door_state", 1, map[string]string{"vehicle": "v", "index": "0", "state": "opened"})
	expect(t, metrics, "door_state", 1, map[string]string{"vehicle": "v", "index": "1", "state": "closed"})
	expect(t, metrics, "door_state", 1, map[string]string{"vehicle": "v", "index": "2", "state": "unknown"})
	expect(t, metrics, "door_state", 0, map[string]string{"vehicle": "v", "index": "2", "state": "opened"})
	if len(metrics["autoware_traffic_signal"].Metric) != 3 {
		t.Fatal("traffic elements collapsed")
	}
	for _, family := range metrics {
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetValue() == "123456789" || label.GetValue() == "987654321" {
					t.Fatal("map identifiers became labels")
				}
			}
		}
	}
	metrics = gather(t, vehicle("v", sample(t, "traffic_signals", `{"traffic_light_groups":[]}`), sample(t, "doors", `{"doors":[]}`)))
	absent(t, metrics, "traffic_signal", nil)
	absent(t, metrics, "door_state", nil)
}

func TestProductAEBEmptyArrayDoesNotAssertNoBraking(t *testing.T) {
	for _, entries := range [][]evaluatorEntry{nil, {{"decision", "", "unknown"}}, {{"decision", "", "brake"}, {"decision", "", "brake"}}} {
		metrics := gather(t, vehicle("v", evaluatorSample(t, "aeb_metrics", entries...)))
		absent(t, metrics, "aeb_decision", nil)
	}
	metrics := gather(t, vehicle("v", evaluatorSample(t, "aeb_metrics", evaluatorEntry{"decision", "", "brake"})))
	expect(t, metrics, "aeb_decision", 1, map[string]string{"vehicle": "v", "decision": "brake"})
}

func TestControlEvaluatorFallbackPriorityAndReferenceBoundaries(t *testing.T) {
	evaluator := evaluatorSample(t, "control_evaluator", evaluatorEntry{"lateral_deviation", "", "2"}, evaluatorEntry{"yaw_deviation", "", "0.5"}, evaluatorEntry{"longitudinal_velocity_deviation", "", "3"}, evaluatorEntry{"jerk", "", "4"}, evaluatorEntry{"goal_longitudinal_deviation", "", "100"})
	legacy := sample(t, "control_errors", `{"error":{"lateral_error":0,"heading_error":0.2}}`)
	driving := sample(t, "driving_status", `{"longitudinal_jerk":{"data":0},"lateral_jerk":{"data":-1.5}}`)
	metrics := gather(t, vehicle("v", evaluator, legacy, driving))
	expect(t, metrics, "control_lateral_error_meters", 0, map[string]string{"vehicle": "v"})
	expect(t, metrics, "control_heading_error_radians", .2, map[string]string{"vehicle": "v"})
	expect(t, metrics, "control_velocity_error_meters_per_second", 3, map[string]string{"vehicle": "v"})
	expect(t, metrics, "control_jerk_meters_per_second_cubed", 0, map[string]string{"vehicle": "v", "axis": "longitudinal"})
	expect(t, metrics, "control_jerk_meters_per_second_cubed", -1.5, map[string]string{"vehicle": "v", "axis": "lateral"})
	absent(t, metrics, "control_longitudinal_error_meters", nil)
	// An invalid field falls back independently; finite fields retain priority.
	legacy.Data["error"].(map[string]any)["lateral_error"] = math.NaN()
	metrics = gather(t, vehicle("v", evaluator, legacy))
	expect(t, metrics, "control_lateral_error_meters", 2, map[string]string{"vehicle": "v"})
	expect(t, metrics, "control_heading_error_radians", .2, map[string]string{"vehicle": "v"})
	expect(t, metrics, "control_jerk_meters_per_second_cubed", 4, map[string]string{"vehicle": "v", "axis": "longitudinal"})
	absent(t, metrics, "control_jerk_meters_per_second_cubed", map[string]string{"vehicle": "v", "axis": "lateral"})
}

func TestProductStalenessDisconnectAndRetainedLimits(t *testing.T) {
	control := evaluatorSample(t, "control_evaluator", evaluatorEntry{"goal_lateral_deviation", "", "1"}, evaluatorEntry{"lateral_deviation", "", "2"})
	control.ReceivedAt = fixtureTime.Add(-time.Hour)
	global := sample(t, "velocity_limit", `{"max_velocity":5}`)
	global.ReceivedAt = control.ReceivedAt
	local := sample(t, "closest_velocity_limit", `{"data":6}`)
	local.ReceivedAt = control.ReceivedAt
	v := vehicle("v", control, global, local)
	metrics := gather(t, v)
	absent(t, metrics, "goal_deviation_meters", nil)
	absent(t, metrics, "control_lateral_error_meters", nil)
	expect(t, metrics, "velocity_limit_meters_per_second", 5, map[string]string{"vehicle": "v", "scope": "global"})
	absent(t, metrics, "velocity_limit_meters_per_second", map[string]string{"vehicle": "v", "scope": "local"})
	v.Connected = false
	metrics = gather(t, v)
	absent(t, metrics, "velocity_limit_meters_per_second", nil)
}
