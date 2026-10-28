package exporter

import (
	"math"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Adanomy/autoware-exporter/internal/telemetry"
	"github.com/prometheus/client_golang/prometheus"
)

// Snapshotter returns immutable observations, without performing ROS I/O during a scrape.
type Snapshotter interface {
	Snapshots() []telemetry.VehicleSnapshot
}

type metric struct {
	desc *prometheus.Desc
	kind prometheus.ValueType
}

// Collector presents each configured ROS domain with a separate vehicle label.
type Collector struct {
	provider Snapshotter
	version  string
	revision string
	metrics  map[string]metric
	now      func() time.Time
}

func NewCollector(provider Snapshotter, version, revision string) *Collector {
	c := &Collector{provider: provider, version: version, revision: revision, metrics: make(map[string]metric), now: time.Now}
	for _, d := range metricDefinitions {
		c.metrics[d.name] = metric{desc: prometheus.NewDesc(d.name, d.help, d.labels, nil), kind: d.kind}
	}
	return c
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range c.metrics {
		ch <- m.desc
	}
}

func (c *Collector) emit(ch chan<- prometheus.Metric, name string, value float64, labels ...string) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}
	m := c.metrics["autoware_"+name]
	ch <- prometheus.MustNewConstMetric(m.desc, m.kind, value, labels...)
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.emit(ch, "exporter_build_info", 1, c.version, c.revision, runtime.Version())
	now := c.now()
	for _, v := range c.provider.Snapshots() {
		c.collectVehicle(ch, v, now)
	}
}

func (c *Collector) collectVehicle(ch chan<- prometheus.Metric, v telemetry.VehicleSnapshot, now time.Time) {
	c.emit(ch, "vehicle_info", 1, v.Name, strconv.Itoa(v.DomainID))
	c.emit(ch, "ros_context_up", boolValue(v.Connected), v.Name)
	for _, s := range v.Sources {
		c.emit(ch, "topic_subscribed", boolValue(s.Subscribed), v.Name, s.Spec.Topic)
		c.emit(ch, "topic_publishers", float64(s.Publishers), v.Name, s.Spec.Topic)
		c.emit(ch, "topic_data_available", boolValue(v.Connected && s.Fresh(now)), v.Name, s.Spec.Topic)
		c.emit(ch, "topic_messages_received_total", float64(s.MessagesTotal), v.Name, s.Spec.Topic)
		c.emit(ch, "topic_receive_errors_total", float64(s.ErrorsTotal), v.Name, s.Spec.Topic)
		if !s.ReceivedAt.IsZero() {
			c.emit(ch, "topic_last_received_timestamp_seconds", float64(s.ReceivedAt.UnixNano())/1e9, v.Name, s.Spec.Topic)
			c.emit(ch, "topic_last_received_age_seconds", max(0, now.Sub(s.ReceivedAt).Seconds()), v.Name, s.Spec.Topic)
		}
	}
	if !v.Connected {
		return
	}
	fresh := func(key string) map[string]any {
		s := v.Sources[key]
		if s.Fresh(now) {
			return s.Data
		}
		return nil
	}
	for key, s := range v.Sources {
		if !s.Fresh(now) {
			continue
		}
		c.sourceTime(ch, v.Name, s.Spec.Topic, "message", s.Data)
		if key == "kinematics" {
			for _, component := range []string{"pose", "twist", "accel"} {
				if data, ok := field(s.Data, component).(map[string]any); ok {
					c.sourceTime(ch, v.Name, s.Spec.Topic, component, data)
				}
			}
		}
	}
	mode := fresh("operation_mode")
	c.state(ch, "operation_mode", "mode", mode, v.Name, []string{"unknown", "stop", "autonomous", "local", "remote"})
	c.boolean(ch, "control_enabled", mode, v.Name, "is_autoware_control_enabled")
	c.boolean(ch, "operation_mode_in_transition", mode, v.Name, "is_in_transition")
	for _, name := range []string{"stop", "autonomous", "local", "remote"} {
		if b, ok := field(mode, "is_"+name+"_mode_available").(bool); ok {
			c.emit(ch, "operation_mode_available", boolValue(b), v.Name, name)
		}
	}
	c.state(ch, "localization_state", "state", fresh("localization_state"), v.Name, []string{"unknown", "uninitialized", "initializing", "initialized"})
	c.state(ch, "route_state", "state", fresh("route_state"), v.Name, []string{"unknown", "unset", "set", "arrived", "changing"})
	c.state(ch, "motion_state", "state", fresh("motion_state"), v.Name, []string{"unknown", "stopped", "starting", "moving"})
	mrm := fresh("mrm_state")
	c.state(ch, "mrm_state", "state", mrm, v.Name, []string{"unknown", "normal", "operating", "succeeded", "failed"})
	c.state(ch, "mrm_behavior", "behavior", mrm, v.Name, []string{"unknown", "none", "emergency_stop", "comfortable_stop", "pull_over"})
	c.numeric(ch, "heartbeat_sequence", fresh("heartbeat"), v.Name, "seq")
	if clock, ok := timestamp(field(fresh("clock"), "clock")); ok {
		c.emit(ch, "ros_time_seconds", clock, v.Name)
	}
	c.kinematics(ch, v.Name, fresh("kinematics"))
	status := fresh("vehicle_status")
	c.numeric(ch, "vehicle_steering_angle_radians", status, v.Name, "steering_tire_angle")
	c.state(ch, "vehicle_gear", "gear.status", status, v.Name, []string{"unknown", "neutral", "drive", "reverse", "park", "low"})
	c.state(ch, "vehicle_turn_indicator", "turn_indicators.status", status, v.Name, []string{"unknown", "disabled", "left", "right"})
	c.state(ch, "vehicle_hazard_lights", "hazard_lights.status", status, v.Name, []string{"unknown", "disabled", "enabled"})
	if energy, ok := number(field(fresh("energy"), "energy")); ok && energy >= 0 && energy <= 1 {
		c.emit(ch, "vehicle_energy_ratio", energy, v.Name)
	}
	c.diagnostics(ch, v.Name, fresh("diagnostics_struct"), fresh("diagnostics_status"))
	c.commands(ch, v.Name, fresh)
	c.collectProduct(ch, v.Name, fresh)
	c.localization(ch, v.Name, fresh)
	c.objects(ch, v.Name, fresh("objects"))
	for key, s := range v.Sources {
		if !s.Fresh(now) || !strings.HasPrefix(key, "processing_") {
			continue
		}
		if value, ok := number(field(s.Data, "data")); ok && value >= 0 {
			c.emit(ch, "processing_duration_seconds", value/1000, v.Name, strings.TrimPrefix(key, "processing_"))
		}
	}
	if value, ok := number(field(fresh("pipeline_latency"), "data")); ok && value >= 0 {
		c.emit(ch, "pipeline_latency_seconds", value/1000, v.Name)
	}
	if state, ok := integer(field(fresh("route_state"), "state")); ok && (state == 2 || state == 3) {
		route := fresh("remaining_route")
		for _, m := range []struct{ name, field string }{{"route_remaining_distance_meters", "remaining_distance"}, {"route_estimated_remaining_seconds", "remaining_time"}} {
			if value, ok := number(field(route, m.field)); ok && value >= 0 {
				c.emit(ch, m.name, value, v.Name)
			}
		}
	}
}

func (c *Collector) sourceTime(ch chan<- prometheus.Metric, vehicle, topic, component string, data map[string]any) {
	stamp := field(data, "stamp")
	if stamp == nil {
		stamp = field(data, "header", "stamp")
	}
	if value, ok := timestamp(stamp); ok {
		c.emit(ch, "topic_source_timestamp_seconds", value, vehicle, topic, component)
	}
}

func (c *Collector) state(ch chan<- prometheus.Metric, metric, key string, data map[string]any, vehicle string, states []string) {
	value, ok := integer(field(data, strings.Split(key, ".")...))
	if !ok || value < 0 || value >= len(states) {
		return
	}
	for i, state := range states {
		c.emit(ch, metric, boolValue(i == value), vehicle, state)
	}
}

func (c *Collector) boolean(ch chan<- prometheus.Metric, metric string, data map[string]any, vehicle string, keys ...string) {
	if value, ok := field(data, keys...).(bool); ok {
		c.emit(ch, metric, boolValue(value), vehicle)
	}
}

func (c *Collector) numeric(ch chan<- prometheus.Metric, metric string, data map[string]any, vehicle string, keys ...string) {
	if value, ok := number(field(data, keys...)); ok {
		c.emit(ch, metric, value, vehicle)
	}
}

func (c *Collector) kinematics(ch chan<- prometheus.Metric, vehicle string, data map[string]any) {
	for _, vector := range []struct {
		metric string
		path   []string
	}{
		{"vehicle_position_meters", []string{"pose", "pose", "pose", "position"}},
		{"vehicle_velocity_meters_per_second", []string{"twist", "twist", "twist", "linear"}},
		{"vehicle_acceleration_meters_per_second_squared", []string{"accel", "accel", "accel", "linear"}},
		{"vehicle_angular_velocity_radians_per_second", []string{"twist", "twist", "twist", "angular"}},
	} {
		for _, axis := range []string{"x", "y", "z"} {
			if value, ok := number(field(data, append(vector.path, axis)...)); ok {
				c.emit(ch, vector.metric, value, vehicle, axis)
			}
		}
	}
	if yawRate, ok := number(field(data, "twist", "twist", "twist", "angular", "z")); ok {
		c.emit(ch, "vehicle_yaw_rate_radians_per_second", yawRate, vehicle)
	}
	x, xOK := number(field(data, "twist", "twist", "twist", "linear", "x"))
	y, yOK := number(field(data, "twist", "twist", "twist", "linear", "y"))
	z, zOK := number(field(data, "twist", "twist", "twist", "linear", "z"))
	if xOK && yOK && zOK {
		c.emit(ch, "vehicle_speed_meters_per_second", math.Hypot(math.Hypot(x, y), z), vehicle)
	}
	qx, xOK := number(field(data, "pose", "pose", "pose", "orientation", "x"))
	qy, yOK := number(field(data, "pose", "pose", "pose", "orientation", "y"))
	qz, zOK := number(field(data, "pose", "pose", "pose", "orientation", "z"))
	qw, wOK := number(field(data, "pose", "pose", "pose", "orientation", "w"))
	norm := math.Hypot(math.Hypot(qx, qy), math.Hypot(qz, qw))
	if xOK && yOK && zOK && wOK && norm > 1e-12 && !math.IsInf(norm, 0) {
		qx, qy, qz, qw = qx/norm, qy/norm, qz/norm, qw/norm
		c.emit(ch, "vehicle_yaw_radians", math.Atan2(2*(qw*qz+qx*qy), 1-2*(qy*qy+qz*qz)), vehicle)
	}
	c.covariance(ch, vehicle, "kinematics", field(data, "pose", "pose", "covariance"))
}

func (c *Collector) commands(ch chan<- prometheus.Metric, vehicle string, fresh func(string) map[string]any) {
	c.numeric(ch, "command_velocity_meters_per_second", fresh("command_velocity"), vehicle, "velocity")
	c.numeric(ch, "command_acceleration_meters_per_second_squared", fresh("command_acceleration"), vehicle, "acceleration")
	steering := fresh("command_steering")
	c.numeric(ch, "command_steering_angle_radians", steering, vehicle, "steering_tire_angle")
	c.numeric(ch, "command_steering_rate_radians_per_second", steering, vehicle, "steering_tire_velocity")
	pedals := fresh("command_pedals")
	for _, name := range []string{"throttle", "brake"} {
		if value, ok := number(field(pedals, name)); ok && value >= 0 && value <= 1 {
			c.emit(ch, "command_"+name+"_ratio", value, vehicle)
		}
	}
	control := fresh("control_command")
	if defined, ok := field(control, "longitudinal", "is_defined_jerk").(bool); ok && defined {
		c.numeric(ch, "command_jerk_meters_per_second_cubed", control, vehicle, "longitudinal", "jerk")
	}
	errors := fresh("control_errors")
	evaluator := parseMetricArray(fresh("control_evaluator"), controlEvaluatorUnits)
	for _, m := range []struct{ name, field, fallback string }{
		{"control_lateral_error_meters", "lateral_error", "lateral_deviation"},
		{"control_longitudinal_error_meters", "longitudinal_error", ""},
		{"control_heading_error_radians", "heading_error", "yaw_deviation"},
		{"control_velocity_error_meters_per_second", "vehicle_velocity_error", "longitudinal_velocity_deviation"},
	} {
		if value, ok := number(field(errors, "error", m.field)); ok {
			c.emit(ch, m.name, value, vehicle)
		} else if value, ok := evaluator[m.fallback]; m.fallback != "" && ok {
			c.emit(ch, m.name, value, vehicle)
		}
	}
	driving := fresh("driving_status")
	for _, axis := range []string{"longitudinal", "lateral"} {
		if value, ok := number(field(driving, axis+"_jerk", "data")); ok {
			c.emit(ch, "control_jerk_meters_per_second_cubed", value, vehicle, axis)
		} else if value, ok := evaluator["jerk"]; axis == "longitudinal" && ok {
			c.emit(ch, "control_jerk_meters_per_second_cubed", value, vehicle, axis)
		}
	}
}

func (c *Collector) localization(ch chan<- prometheus.Metric, vehicle string, fresh func(string) map[string]any) {
	for _, score := range []struct{ key, kind string }{{"ndt_score", "transform_probability"}, {"ndt_voxel_score", "nearest_voxel_likelihood"}} {
		if value, ok := number(field(fresh(score.key), "data")); ok {
			c.emit(ch, "localization_ndt_score", value, vehicle, score.kind)
		}
	}
	if value, ok := integer(field(fresh("ndt_iterations"), "data")); ok && value >= 0 {
		c.emit(ch, "localization_ndt_iterations", float64(value), vehicle)
	}
	if value, ok := number(field(fresh("ndt_execution"), "data")); ok && value >= 0 {
		c.emit(ch, "localization_ndt_execution_seconds", value/1000, vehicle)
	}
	c.covariance(ch, vehicle, "ndt", field(fresh("ndt_covariance"), "pose", "covariance"))
}

func (c *Collector) covariance(ch chan<- prometheus.Metric, vehicle, source string, value any) {
	values, ok := value.([]any)
	if !ok || len(values) != 36 {
		return
	}
	for i, axis := range []string{"x", "y", "z"} {
		if covariance, ok := number(values[i*7]); ok && covariance >= 0 {
			c.emit(ch, "localization_position_stddev_meters", math.Sqrt(covariance), vehicle, source, axis)
		}
	}
	if covariance, ok := number(values[35]); ok && covariance >= 0 {
		c.emit(ch, "localization_yaw_stddev_radians", math.Sqrt(covariance), vehicle, source)
	}
}

func (c *Collector) diagnostics(ch chan<- prometheus.Metric, vehicle string, structure, status map[string]any) {
	id, ok := field(structure, "id").(string)
	if !ok || id == "" || id != field(status, "id") {
		return
	}
	nodeStructs, structsOK := field(structure, "nodes").([]any)
	nodeStatuses, statusesOK := field(status, "nodes").([]any)
	if !structsOK || !statusesOK || len(nodeStructs) != len(nodeStatuses) || len(nodeStructs) > 512 {
		return
	}
	seen := make(map[string]bool)
	counts := [2][4]int{}
	for i, s := range nodeStatuses {
		level, valid := integer(fieldMap(s, "level"))
		if !valid || level < 0 || level > 3 {
			continue
		}
		counts[0][level]++
		path, valid := fieldMap(nodeStructs[i], "path").(string)
		if !valid || path == "" || len(path) > 256 || seen[path] {
			continue
		}
		seen[path] = true
		c.emit(ch, "diagnostic_level", float64(level), vehicle, path)
	}
	leafStructs, leafStructsOK := field(structure, "diags").([]any)
	leafStatuses, leafStatusesOK := field(status, "diags").([]any)
	if leafStructsOK && leafStatusesOK && len(leafStructs) == len(leafStatuses) && len(leafStructs) <= 4096 {
		for _, s := range leafStatuses {
			if level, ok := integer(fieldMap(s, "level")); ok && level >= 0 && level <= 3 {
				counts[1][level]++
			}
		}
	}
	for kind, name := range []string{"node", "leaf"} {
		if kind == 1 && !(leafStructsOK && leafStatusesOK && len(leafStructs) == len(leafStatuses) && len(leafStructs) <= 4096) {
			continue
		}
		for level, label := range []string{"ok", "warn", "error", "stale"} {
			c.emit(ch, "diagnostics", float64(counts[kind][level]), vehicle, name, label)
		}
	}
}

func (c *Collector) objects(ch chan<- prometheus.Metric, vehicle string, data map[string]any) {
	objects, ok := field(data, "objects").([]any)
	if !ok || len(objects) > 10000 {
		return
	}
	classes := []string{"unknown", "car", "truck", "bus", "trailer", "motorcycle", "bicycle", "pedestrian", "animal", "hazard", "over_drivable", "under_drivable"}
	counts := make([]int, len(classes))
	for _, object := range objects {
		best, probability := 0, -1.0
		classifications, _ := fieldMap(object, "classification").([]any)
		for _, classification := range classifications {
			label, valid := integer(fieldMap(classification, "label"))
			p, pOK := number(fieldMap(classification, "probability"))
			if valid && pOK && label >= 0 && label < len(classes) && p >= 0 && p <= 1 && p > probability {
				best, probability = label, p
			}
		}
		counts[best]++
	}
	c.emit(ch, "perception_objects", float64(len(objects)), vehicle)
	for i, label := range classes {
		c.emit(ch, "perception_objects_by_class", float64(counts[i]), vehicle, label)
	}
}

func field(data map[string]any, keys ...string) any {
	var current any = data
	for _, key := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[key]
	}
	return current
}

func fieldMap(data any, key string) any {
	m, _ := data.(map[string]any)
	return m[key]
}

func number(value any) (float64, bool) {
	var result float64
	switch n := value.(type) {
	case float64:
		result = n
	case float32:
		result = float64(n)
	case int:
		result = float64(n)
	case int32:
		result = float64(n)
	case int64:
		result = float64(n)
	case uint:
		result = float64(n)
	case uint8:
		result = float64(n)
	case uint16:
		result = float64(n)
	case uint32:
		result = float64(n)
	case uint64:
		result = float64(n)
	default:
		return 0, false
	}
	return result, !math.IsNaN(result) && !math.IsInf(result, 0)
}

func integer(value any) (int, bool) {
	v, ok := number(value)
	if !ok || math.Trunc(v) != v || v < math.MinInt32 || v > math.MaxInt32 {
		return 0, false
	}
	return int(v), true
}

func timestamp(value any) (float64, bool) {
	sec, secOK := number(fieldMap(value, "sec"))
	nanos, nanoOK := number(fieldMap(value, "nanosec"))
	if !secOK || !nanoOK || math.Trunc(sec) != sec || math.Trunc(nanos) != nanos || nanos < 0 || nanos >= 1e9 {
		return 0, false
	}
	return sec + nanos/1e9, true
}

func boolValue(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
