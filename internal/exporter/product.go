package exporter

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	maxFactorEntries   = 64
	maxTrafficGroups   = 128
	maxTrafficElements = 16
	maxDoorEntries     = 32
)

// MetricArray names are public strings, but only these exact, version-checked
// names are allowed into exported measurements. They never become free labels.
type metricUnit []string

var controlEvaluatorUnits = map[string]metricUnit{
	"goal_longitudinal_deviation":               {"", "m"},
	"goal_longitudinal_deviation_abs":           {"", "m"},
	"goal_lateral_deviation":                    {"", "m"},
	"goal_lateral_deviation_abs":                {"", "m"},
	"goal_yaw_deviation":                        {"", "rad"},
	"goal_yaw_deviation_abs":                    {"", "rad"},
	"lateral_deviation_centerline":              {"", "m"},
	"lateral_deviation_centerline_abs":          {"", "m"},
	"left_boundary_distance":                    {"", "m"},
	"right_boundary_distance":                   {"", "m"},
	"left_uncrossable_boundary_distance":        {"", "m"},
	"right_uncrossable_boundary_distance":       {"", "m"},
	"left_uncrossable_boundary_distance_count":  {"", "count"},
	"right_uncrossable_boundary_distance_count": {"", "count"},
	"lateral_deviation":                         {"", "m"},
	"yaw_deviation":                             {"", "rad"},
	"longitudinal_velocity_deviation":           {"", "m/s"},
	"jerk":                                      {"", "m/s^3"},
}

var planningEvaluatorUnits = func() map[string]metricUnit {
	units := make(map[string]metricUnit)
	for _, name := range []string{"stability", "stability_frechet", "length"} {
		for _, statistic := range []string{"min", "max", "mean"} {
			units[name+"/"+statistic] = metricUnit{"", "m"}
		}
	}
	for _, statistic := range []string{"min", "max", "mean"} {
		units["duration/"+statistic] = metricUnit{"", "s"}
	}
	units["steer_change_count/count_in_duration"] = metricUnit{"", "count"}
	units["blinker_change_count/count_in_duration"] = metricUnit{"", "count"}
	units["obstacle_ttc/worst"] = metricUnit{"", "s"}
	return units
}()

var perceptionAnalyticsUnits = map[string]metricUnit{
	"meas_to_tracked_latency": {"ms"},
	"prediction_latency":      {"ms"},
	"total_latency":           {"ms"},
}

// parseMetricArray rejects duplicate allowlisted names as ambiguous, even if
// one of their values is invalid. Unknown keys, units, nonnumeric values and
// non-finite numbers are omitted independently rather than becoming zero.
func parseMetricArray(data map[string]any, allow map[string]metricUnit) map[string]float64 {
	entries, ok := field(data, "metric_array").([]any)
	if !ok || len(entries) > 4096 {
		return nil
	}
	values := make(map[string]float64)
	seen := make(map[string]bool)
	for _, entry := range entries {
		name, ok := fieldMap(entry, "name").(string)
		if !ok {
			continue
		}
		units, allowed := allow[name]
		if !allowed {
			continue
		}
		if seen[name] {
			delete(values, name)
			continue
		}
		seen[name] = true
		unit, valid := fieldMap(entry, "unit").(string)
		if !valid || !unitAllowed(unit, units) {
			continue
		}
		text, valid := fieldMap(entry, "value").(string)
		if !valid || strings.TrimSpace(text) == "" {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		values[name] = value
	}
	return values
}

func unitAllowed(unit string, units metricUnit) bool { return slices.Contains(units, unit) }

func (c *Collector) collectProduct(ch chan<- prometheus.Metric, vehicle string, fresh func(string) map[string]any) {
	control := parseMetricArray(fresh("control_evaluator"), controlEvaluatorUnits)
	for _, axis := range []string{"longitudinal", "lateral"} {
		for _, kind := range []string{"signed", "absolute"} {
			key := "goal_" + axis + "_deviation"
			if kind == "absolute" {
				key += "_abs"
			}
			if value, ok := control[key]; ok && (kind != "absolute" || value >= 0) {
				c.emit(ch, "goal_deviation_meters", value, vehicle, axis, kind)
			}
		}
	}
	for _, kind := range []string{"signed", "absolute"} {
		suffix := ""
		if kind == "absolute" {
			suffix = "_abs"
		}
		if value, ok := control["goal_yaw_deviation"+suffix]; ok && (kind != "absolute" || value >= 0) {
			c.emit(ch, "goal_yaw_deviation_radians", value, vehicle, kind)
		}
		if value, ok := control["lateral_deviation_centerline"+suffix]; ok && (kind != "absolute" || value >= 0) {
			c.emit(ch, "centerline_deviation_meters", value, vehicle, kind)
		}
	}
	for _, side := range []string{"left", "right"} {
		for _, boundary := range []string{"lane", "uncrossable"} {
			key := side + "_boundary_distance"
			if boundary == "uncrossable" {
				key = side + "_uncrossable_boundary_distance"
			}
			if value, ok := control[key]; ok {
				c.emit(ch, "boundary_distance_meters", value, vehicle, side, boundary)
			}
			if value, ok := control[key+"_count"]; ok && value >= 0 && math.Trunc(value) == value {
				c.emit(ch, "boundary_crossings", value, vehicle, side, boundary)
			}
		}
	}
	c.collectFactors(ch, vehicle, fresh("steering_factors"), true)
	c.collectFactors(ch, vehicle, fresh("velocity_factors"), false)
	for _, scope := range []string{"global", "local"} {
		key := "velocity_limit"
		if scope == "local" {
			key = "closest_velocity_limit"
		}
		fieldName := "max_velocity"
		if scope == "local" {
			fieldName = "data"
		}
		if value, ok := number(field(fresh(key), fieldName)); ok && value >= 0 {
			c.emit(ch, "velocity_limit_meters_per_second", value, vehicle, scope)
		}
	}
	c.boolean(ch, "emergency_command", fresh("emergency_command"), vehicle, "emergency")
	c.boolean(ch, "emergency_holding", fresh("emergency_holding"), vehicle, "is_holding")
	hazard := fresh("hazard_status")
	if level, ok := integer(field(hazard, "status", "level")); ok && level >= 0 && level <= 3 {
		c.emit(ch, "hazard_level", float64(level), vehicle)
	}
	for _, flag := range []string{"emergency", "emergency_holding"} {
		if value, ok := field(hazard, "status", flag).(bool); ok {
			c.emit(ch, "hazard_flag", boolValue(value), vehicle, flag)
		}
	}
	analytics := parseMetricArray(fresh("perception_analytics"), perceptionAnalyticsUnits)
	for key, stage := range map[string]string{"meas_to_tracked_latency": "measurement_to_tracked", "prediction_latency": "prediction", "total_latency": "total"} {
		if value, ok := analytics[key]; ok && value >= 0 {
			c.emit(ch, "perception_latency_seconds", value/1000, vehicle, stage)
		}
	}
	planning := parseMetricArray(fresh("planning_evaluator"), planningEvaluatorUnits)
	for _, statistic := range []string{"min", "max", "mean"} {
		for _, kind := range []string{"stability", "frechet"} {
			key := "stability"
			if kind == "frechet" {
				key = "stability_frechet"
			}
			if value, ok := planning[key+"/"+statistic]; ok && value >= 0 {
				c.emit(ch, "planning_stability_meters", value, vehicle, kind, statistic)
			}
		}
		if value, ok := planning["length/"+statistic]; ok && value >= 0 {
			c.emit(ch, "planning_trajectory_length_meters", value, vehicle, statistic)
		}
		if value, ok := planning["duration/"+statistic]; ok && value >= 0 {
			c.emit(ch, "planning_trajectory_duration_seconds", value, vehicle, statistic)
		}
	}
	for key, signal := range map[string]string{"steer_change_count/count_in_duration": "steering", "blinker_change_count/count_in_duration": "blinker"} {
		if value, ok := planning[key]; ok && value >= 0 && math.Trunc(value) == value {
			c.emit(ch, "planning_changes", value, vehicle, signal)
		}
	}
	if value, ok := planning["obstacle_ttc/worst"]; ok && value >= 0 {
		c.emit(ch, "planning_time_to_collision_seconds", value, vehicle, "worst")
	}
	c.collectTrafficSignals(ch, vehicle, fresh("traffic_signals"))
	c.collectDoors(ch, vehicle, fresh("doors"))
	c.collectAEBDecision(ch, vehicle, fresh("aeb_metrics"))
	c.numeric(ch, "aeb_rss_distance_meters", fresh("aeb_rss_distance"), vehicle, "data")
}

// Array indexes identify only slots in this received message, not persistent
// maneuver, map-object or door identities. All textual labels use fixed enums.
func (c *Collector) collectFactors(ch chan<- prometheus.Metric, vehicle string, data map[string]any, steering bool) {
	factors, ok := field(data, "factors").([]any)
	if !ok || len(factors) > maxFactorEntries {
		return
	}
	for index, factor := range factors {
		behavior, valid := factorBehavior(fieldMap(factor, "behavior"))
		if !valid {
			continue
		}
		status, valid := factorState(fieldMap(factor, "status"), steering)
		if !valid {
			continue
		}
		labels := []string{vehicle, strconv.Itoa(index), behavior}
		if steering {
			direction, valid := fixedEnum(fieldMap(factor, "direction"), map[int]string{0: "unknown", 1: "left", 2: "right", 3: "straight"})
			if !valid {
				continue
			}
			labels = append(labels, direction, status)
			c.emit(ch, "planning_steering_factor", 1, labels...)
			distances, valid := fieldMap(factor, "distance").([]any)
			if !valid || len(distances) != 2 {
				continue
			}
			for i, point := range []string{"start", "end"} {
				if value, valid := number(distances[i]); valid {
					c.emit(ch, "planning_steering_factor_distance_meters", value, append(labels, point)...)
				}
			}
		} else {
			labels = append(labels, status)
			c.emit(ch, "planning_velocity_factor", 1, labels...)
			if value, valid := number(fieldMap(factor, "distance")); valid {
				c.emit(ch, "planning_velocity_factor_distance_meters", value, labels...)
			}
		}
	}
}

func factorState(value any, steering bool) (string, bool) {
	if steering {
		return fixedEnum(value, map[int]string{0: "unknown", 1: "approaching", 3: "turning"})
	}
	return fixedEnum(value, map[int]string{0: "unknown", 1: "approaching", 2: "stopped"})
}

func fixedEnum(value any, labels map[int]string) (string, bool) {
	code, ok := integer(value)
	if !ok || code < 0 || code > 65535 {
		return "", false
	}
	if label, exists := labels[code]; exists {
		return label, true
	}
	return "unknown", true
}

func factorBehavior(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	// PlanningBehavior.UNKNOWN is the empty string in Autoware 0.53.
	switch text {
	case "":
		return "unknown", true
	case "avoidance", "crosswalk", "goal-planner", "intersection", "lane-change", "merge", "no-drivable-lane", "no-stopping-area", "rear-check", "route-obstacle", "sidewalk", "start-planner", "stop-sign", "surrounding-obstacle", "traffic-signal", "user-defined-attention-area", "virtual-traffic-light", "run-out", "adaptive-cruise", "roundabout":
		return text, true
	default:
		return "unknown", true
	}
}

func (c *Collector) collectTrafficSignals(ch chan<- prometheus.Metric, vehicle string, data map[string]any) {
	groups, ok := field(data, "traffic_light_groups").([]any)
	if !ok || len(groups) > maxTrafficGroups {
		return
	}
	for groupIndex, group := range groups {
		elements, valid := fieldMap(group, "elements").([]any)
		if !valid || len(elements) > maxTrafficElements {
			continue
		}
		for index, element := range elements {
			color, colorOK := fixedEnum(fieldMap(element, "color"), map[int]string{0: "unknown", 1: "red", 2: "amber", 3: "green", 4: "white"})
			shape, shapeOK := fixedEnum(fieldMap(element, "shape"), map[int]string{0: "unknown", 1: "circle", 2: "left_arrow", 3: "right_arrow", 4: "up_arrow", 5: "up_left_arrow", 6: "up_right_arrow", 7: "down_arrow", 8: "down_left_arrow", 9: "down_right_arrow", 10: "cross"})
			status, statusOK := fixedEnum(fieldMap(element, "status"), map[int]string{0: "unknown", 1: "solid_off", 2: "solid_on", 3: "flashing"})
			if !colorOK || !shapeOK || !statusOK {
				continue
			}
			labels := []string{vehicle, strconv.Itoa(groupIndex), strconv.Itoa(index), color, shape, status}
			c.emit(ch, "traffic_signal", 1, labels...)
			if confidence, valid := number(fieldMap(element, "confidence")); valid && confidence >= 0 && confidence <= 1 {
				c.emit(ch, "traffic_signal_confidence_ratio", confidence, labels...)
			}
		}
	}
}

func (c *Collector) collectDoors(ch chan<- prometheus.Metric, vehicle string, data map[string]any) {
	doors, ok := field(data, "doors").([]any)
	if !ok || len(doors) > maxDoorEntries {
		return
	}
	states := []string{"unknown", "not_available", "opened", "closed", "opening", "closing"}
	for index, door := range doors {
		state, ok := fixedEnum(fieldMap(door, "status"), map[int]string{0: "unknown", 1: "not_available", 2: "opened", 3: "closed", 4: "opening", 5: "closing"})
		if !ok {
			continue
		}
		for _, label := range states {
			c.emit(ch, "door_state", boolValue(state == label), vehicle, strconv.Itoa(index), label)
		}
	}
}

func (c *Collector) collectAEBDecision(ch chan<- prometheus.Metric, vehicle string, data map[string]any) {
	entries, ok := field(data, "metric_array").([]any)
	if !ok || len(entries) > 4096 {
		return
	}
	seen, brake := false, false
	for _, entry := range entries {
		if fieldMap(entry, "name") != "decision" {
			continue
		}
		if seen {
			return
		}
		seen = true
		unit, valid := fieldMap(entry, "unit").(string)
		value, valueOK := fieldMap(entry, "value").(string)
		brake = valid && (unit == "" || unit == "string") && valueOK && value == "brake"
	}
	if brake {
		c.emit(ch, "aeb_decision", 1, vehicle, "brake")
	}
}
