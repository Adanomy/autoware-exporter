package exporter

import "github.com/prometheus/client_golang/prometheus"

type metricDefinition struct {
	name   string
	help   string
	labels []string
	kind   prometheus.ValueType
}

func gauge(name, help string, labels ...string) metricDefinition {
	return metricDefinition{name: "autoware_" + name, help: help, labels: append([]string{"vehicle"}, labels...), kind: prometheus.GaugeValue}
}

func counter(name, help string, labels ...string) metricDefinition {
	d := gauge(name, help, labels...)
	d.kind = prometheus.CounterValue
	return d
}

var metricDefinitions = []metricDefinition{
	{name: "autoware_exporter_build_info", help: "Exporter build information; value 1.", labels: []string{"version", "revision", "go_version"}, kind: prometheus.GaugeValue},
	gauge("vehicle_info", "Configured vehicle and ROS domain; value 1.", "ros_domain_id"),
	gauge("ros_context_up", "ROS client context initialized; does not imply that vehicle data is fresh."),
	gauge("topic_subscribed", "Subscription created for this configured topic.", "topic"),
	gauge("topic_publishers", "Number of discovered publishers for this topic.", "topic"),
	gauge("topic_data_available", "A message is available from an active publisher within the configured freshness policy.", "topic"),
	counter("topic_messages_received_total", "Messages received by this exporter, including messages that failed decoding.", "topic"),
	counter("topic_receive_errors_total", "Subscription, publisher discovery, or message decoding errors observed by this exporter.", "topic"),
	gauge("topic_last_received_timestamp_seconds", "Exporter-host Unix time when the latest valid decoded message was received.", "topic"),
	gauge("topic_last_received_age_seconds", "Monotonic real time elapsed since the latest valid decoded message was received.", "topic"),
	gauge("topic_source_timestamp_seconds", "Timestamp carried by the ROS message in its source clock; not necessarily Unix time.", "topic", "component"),
	gauge("ros_time_seconds", "Latest observed ROS /clock time; can pause or move backwards."),
	gauge("heartbeat_sequence", "Last observed AD API heartbeat sequence, wrapping at 65535."),
	gauge("operation_mode", "Current AD API operation mode as a fixed one-hot state.", "mode"),
	gauge("control_enabled", "Autoware control is enabled."),
	gauge("operation_mode_in_transition", "An operation mode transition is in progress."),
	gauge("operation_mode_available", "Switching to the specified operation mode is available.", "mode"),
	gauge("localization_state", "Localization initialization state as a fixed one-hot state.", "state"),
	gauge("route_state", "Route state as a fixed one-hot state.", "state"),
	gauge("motion_state", "Motion state as a fixed one-hot state.", "state"),
	gauge("mrm_state", "Minimal risk maneuver state as a fixed one-hot state.", "state"),
	gauge("mrm_behavior", "Known AD API minimal risk maneuver behavior as a fixed one-hot state; unrecognized extensible behavior IDs are omitted.", "behavior"),
	gauge("vehicle_position_meters", "Vehicle pose position in the message coordinate frame.", "axis"),
	gauge("vehicle_yaw_radians", "Vehicle pose yaw extracted from its orientation quaternion."),
	gauge("vehicle_velocity_meters_per_second", "Signed vehicle linear velocity components in the twist message frame.", "axis"),
	gauge("vehicle_speed_meters_per_second", "Magnitude of the vehicle linear velocity vector."),
	gauge("vehicle_acceleration_meters_per_second_squared", "Vehicle linear acceleration components in the acceleration message frame.", "axis"),
	gauge("vehicle_angular_velocity_radians_per_second", "Vehicle angular velocity components in the twist message frame.", "axis"),
	gauge("vehicle_yaw_rate_radians_per_second", "Vehicle angular velocity about the z axis."),
	gauge("vehicle_steering_angle_radians", "Reported vehicle tire steering angle."),
	gauge("vehicle_gear", "Reported vehicle gear as a fixed one-hot state.", "gear"),
	gauge("vehicle_turn_indicator", "Reported turn indicator as a fixed one-hot state.", "state"),
	gauge("vehicle_hazard_lights", "Reported hazard light status as a fixed one-hot state.", "state"),
	gauge("vehicle_energy_ratio", "Remaining vehicle fuel or battery divided by maximum capacity."),
	gauge("diagnostic_level", "AD API diagnostic graph node level: 0 OK, 1 WARN, 2 ERROR, 3 STALE.", "path"),
	gauge("diagnostics", "Number of diagnostic graph entries by kind and level.", "kind", "level"),
	gauge("command_velocity_meters_per_second", "Commanded longitudinal velocity; negative is reverse."),
	gauge("command_acceleration_meters_per_second_squared", "Commanded longitudinal acceleration."),
	gauge("command_jerk_meters_per_second_cubed", "Commanded longitudinal jerk when explicitly defined."),
	gauge("command_steering_angle_radians", "Commanded tire steering angle."),
	gauge("command_steering_rate_radians_per_second", "Commanded tire steering angular velocity."),
	gauge("command_throttle_ratio", "Commanded normalized throttle pedal position."),
	gauge("command_brake_ratio", "Commanded normalized brake pedal position."),
	gauge("control_lateral_error_meters", "Reported lateral tracking error; control performance analysis has priority over the control evaluator fallback."),
	gauge("control_longitudinal_error_meters", "Longitudinal tracking error reported by control performance analysis."),
	gauge("control_heading_error_radians", "Reported heading tracking error; control performance analysis has priority over the control evaluator fallback."),
	gauge("control_velocity_error_meters_per_second", "Reported velocity tracking error; control performance analysis has priority over the control evaluator fallback."),
	gauge("control_jerk_meters_per_second_cubed", "Reported measured driving jerk; control performance analysis has priority, with longitudinal control evaluator fallback.", "axis"),
	gauge("localization_position_stddev_meters", "Square root of pose covariance diagonal; estimated uncertainty, not ground-truth error.", "source", "axis"),
	gauge("localization_yaw_stddev_radians", "Square root of yaw covariance; estimated uncertainty, not ground-truth error.", "source"),
	gauge("localization_ndt_score", "NDT matching score; not a calibrated probability of localization correctness.", "kind"),
	gauge("localization_ndt_iterations", "Number of iterations used by the most recent NDT match."),
	gauge("localization_ndt_execution_seconds", "Execution time of the most recent NDT match."),
	gauge("perception_objects", "Number of objects in the latest predicted object array."),
	gauge("perception_objects_by_class", "Object count by the highest-probability finite classification.", "class"),
	gauge("processing_duration_seconds", "Latest reported module processing duration; not a histogram.", "module"),
	gauge("pipeline_latency_seconds", "Latest total pipeline latency reported by Autoware."),
	gauge("route_remaining_distance_meters", "Remaining route distance reported by Autoware; unavailable without an active route."),
	gauge("route_estimated_remaining_seconds", "Remaining route time estimate reported by Autoware; not a traffic-aware arrival forecast."),
	gauge("goal_deviation_meters", "Goal deviation reported by the control evaluator; absolute values are supplied upstream, not computed by the exporter.", "axis", "kind"),
	gauge("goal_yaw_deviation_radians", "Goal yaw deviation reported by the control evaluator; signed or upstream absolute value.", "kind"),
	gauge("centerline_deviation_meters", "Centerline deviation reported by the control evaluator; signed or upstream absolute value.", "kind"),
	gauge("boundary_distance_meters", "Raw boundary distance reported by the evaluator; may be capped at its search distance when no boundary is found.", "side", "boundary"),
	gauge("boundary_crossings", "Snapshot of upstream boundary crossings since the evaluator started; resets with its process and is not an exporter counter.", "side", "boundary"),
	gauge("planning_steering_factor", "Reported steering factor; one series per bounded array index, value 1.", "index", "behavior", "direction", "status"),
	gauge("planning_steering_factor_distance_meters", "Reported steering factor start or end distance; negative values may be behind the vehicle.", "index", "behavior", "direction", "status", "point"),
	gauge("planning_velocity_factor", "Reported velocity factor; one series per bounded array index, value 1.", "index", "behavior", "status"),
	gauge("planning_velocity_factor_distance_meters", "Reported velocity factor distance; negative values may be behind the vehicle.", "index", "behavior", "status"),
	gauge("velocity_limit_meters_per_second", "Reported global or closest local maximum velocity; not an inferred map speed limit.", "scope"),
	gauge("emergency_command", "Emergency command flag reported by the command gate."),
	gauge("emergency_holding", "Emergency holding state reported by the command gate."),
	gauge("hazard_level", "Reported hazard level: 0 no fault, 1 safe fault, 2 latent fault, 3 single-point fault."),
	gauge("hazard_flag", "Reported hazard state flag.", "flag"),
	gauge("perception_latency_seconds", "Latency reported by perception analytics, converted from milliseconds.", "stage"),
	gauge("planning_stability_meters", "Upstream trajectory stability summary; not a percentile computed from exporter scrapes.", "kind", "statistic"),
	gauge("planning_changes", "Upstream count of steering or blinker changes within its configured duration window; not a cumulative counter.", "signal"),
	gauge("planning_trajectory_length_meters", "Upstream summary of planned trajectory length.", "statistic"),
	gauge("planning_trajectory_duration_seconds", "Upstream summary of planned trajectory duration.", "statistic"),
	gauge("traffic_signal", "Raw reported traffic light element; bounded group/element indexes, value 1. This does not assert traffic-rule compliance.", "group", "index", "color", "shape", "status"),
	gauge("traffic_signal_confidence_ratio", "Reported traffic light element classification confidence.", "group", "index", "color", "shape", "status"),
	gauge("door_state", "Reported door state as a fixed one-hot state, identified by bounded array index.", "index", "state"),
	gauge("planning_time_to_collision_seconds", "Reported obstacle time-to-collision summary from the planning evaluator; absent when upstream supplies no finite value.", "statistic"),
	gauge("aeb_decision", "Current decision explicitly reported by AEB metrics; an empty array is unavailable, not a no-braking assertion.", "decision"),
	gauge("aeb_rss_distance_meters", "Required longitudinal separation reported by the AEB RSS model; not measured obstacle clearance and may be signed."),
}
