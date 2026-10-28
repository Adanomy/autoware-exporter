package telemetry

import "time"

// Groups returns the supported collector groups in a stable order.
func Groups() []string {
	return []string{"state", "vehicle", "diagnostics", "control", "localization", "perception", "performance", "routing", "energy"}
}

// Sources returns a new copy of the explicit topic allowlist. Compatibility
// depends on the installed message interfaces, not an Autoware version check.
// Missing optional packages/topics do not prevent other sources.
func Sources() []Source {
	const age = 10 * time.Second
	return []Source{
		{"operation_mode", "/api/operation_mode/state", "autoware_adapi_v1_msgs/msg/OperationModeState", "state", true, 0},
		{"localization_state", "/api/localization/initialization_state", "autoware_adapi_v1_msgs/msg/LocalizationInitializationState", "state", true, 0},
		{"route_state", "/api/routing/state", "autoware_adapi_v1_msgs/msg/RouteState", "state", true, 0},
		{"motion_state", "/api/motion/state", "autoware_adapi_v1_msgs/msg/MotionState", "state", true, 0},
		{"mrm_state", "/api/fail_safe/mrm_state", "autoware_adapi_v1_msgs/msg/MrmState", "state", true, 0},
		{"emergency_holding", "/system/emergency_holding", "tier4_system_msgs/msg/EmergencyHoldingState", "state", false, age},
		{"clock", "/clock", "rosgraph_msgs/msg/Clock", "state", false, age},
		{"heartbeat", "/api/system/heartbeat", "autoware_adapi_v1_msgs/msg/Heartbeat", "state", false, age},
		{"kinematics", "/api/vehicle/kinematics", "autoware_adapi_v1_msgs/msg/VehicleKinematics", "vehicle", false, age},
		{"vehicle_status", "/api/vehicle/status", "autoware_adapi_v1_msgs/msg/VehicleStatus", "vehicle", false, age},
		{"doors", "/api/vehicle/doors/status", "autoware_adapi_v1_msgs/msg/DoorStatusArray", "vehicle", true, 0},
		{"diagnostics_struct", "/api/system/diagnostics/struct", "autoware_adapi_v1_msgs/msg/DiagGraphStruct", "diagnostics", true, 0},
		{"diagnostics_status", "/api/system/diagnostics/status", "autoware_adapi_v1_msgs/msg/DiagGraphStatus", "diagnostics", false, age},
		{"hazard_status", "/system/emergency/hazard_status", "autoware_system_msgs/msg/HazardStatusStamped", "diagnostics", false, age},
		{"command_velocity", "/api/control/command/velocity", "autoware_adapi_v1_msgs/msg/VelocityCommand", "control", false, age},
		{"command_acceleration", "/api/control/command/acceleration", "autoware_adapi_v1_msgs/msg/AccelerationCommand", "control", false, age},
		{"command_steering", "/api/control/command/steering", "autoware_adapi_v1_msgs/msg/SteeringCommand", "control", false, age},
		{"command_pedals", "/api/control/command/pedals", "autoware_adapi_v1_msgs/msg/PedalsCommand", "control", false, age},
		{"control_command", "/control/command/control_cmd", "autoware_control_msgs/msg/Control", "control", false, age},
		{"control_errors", "/control_performance/performance_vars", "autoware_control_performance_analysis/msg/ErrorStamped", "control", false, age},
		{"driving_status", "/control_performance/driving_status", "autoware_control_performance_analysis/msg/DrivingMonitorStamped", "control", false, age},
		{"control_evaluator", "/control/control_evaluator/metrics", "tier4_metric_msgs/msg/MetricArray", "control", false, age},
		{"emergency_command", "/control/command/emergency_cmd", "tier4_vehicle_msgs/msg/VehicleEmergencyStamped", "control", false, age},
		{"aeb_metrics", "/control/autonomous_emergency_braking/metrics", "tier4_metric_msgs/msg/MetricArray", "control", false, age},
		{"aeb_rss_distance", "/control/autonomous_emergency_braking/debug/rss_distance", "tier4_debug_msgs/msg/Float32Stamped", "control", false, age},
		{"ndt_score", "/localization/pose_estimator/transform_probability", "autoware_internal_debug_msgs/msg/Float32Stamped", "localization", false, age},
		{"ndt_voxel_score", "/localization/pose_estimator/nearest_voxel_transformation_likelihood", "autoware_internal_debug_msgs/msg/Float32Stamped", "localization", false, age},
		{"ndt_iterations", "/localization/pose_estimator/iteration_num", "autoware_internal_debug_msgs/msg/Int32Stamped", "localization", false, age},
		{"ndt_execution", "/localization/pose_estimator/exe_time_ms", "autoware_internal_debug_msgs/msg/Float32Stamped", "localization", false, age},
		{"ndt_covariance", "/localization/pose_estimator/pose_with_covariance", "geometry_msgs/msg/PoseWithCovarianceStamped", "localization", false, age},
		{"objects", "/perception/object_recognition/objects", "autoware_perception_msgs/msg/PredictedObjects", "perception", false, age},
		{"perception_analytics", "/perception/perception_analytics_publisher/perception_analytics", "tier4_metric_msgs/msg/MetricArray", "perception", false, age},
		{"traffic_signals", "/perception/traffic_light_recognition/traffic_signals", "autoware_perception_msgs/msg/TrafficLightGroupArray", "perception", false, age},
		{"processing_localization_filter", "/localization/pose_twist_fusion_filter/debug/processing_time_ms", "autoware_internal_debug_msgs/msg/Float64Stamped", "performance", false, age},
		{"processing_lateral_controller", "/control/trajectory_follower/controller_node_exe/lateral/debug/processing_time_ms", "autoware_internal_debug_msgs/msg/Float64Stamped", "performance", false, age},
		{"processing_longitudinal_controller", "/control/trajectory_follower/controller_node_exe/longitudinal/debug/processing_time_ms", "autoware_internal_debug_msgs/msg/Float64Stamped", "performance", false, age},
		{"processing_mission_planner", "/planning/mission_planning/mission_planner/debug/processing_time_ms", "autoware_internal_debug_msgs/msg/Float64Stamped", "performance", false, age},
		{"processing_object_tracker", "/perception/object_recognition/tracking/multi_object_tracker/debug/processing_time_ms", "autoware_internal_debug_msgs/msg/Float64Stamped", "performance", false, age},
		{"processing_object_prediction", "/perception/object_recognition/prediction/map_based_prediction/debug/processing_time_ms", "autoware_internal_debug_msgs/msg/Float64Stamped", "performance", false, age},
		{"pipeline_latency", "/system/pipeline_latency_monitor/output/total_latency_ms", "autoware_internal_debug_msgs/msg/Float64Stamped", "performance", false, age},
		{"remaining_route", "/planning/mission_remaining_distance_time", "autoware_internal_msgs/msg/MissionRemainingDistanceTime", "routing", false, age},
		{"planning_evaluator", "/planning/planning_evaluator/metrics", "tier4_metric_msgs/msg/MetricArray", "routing", false, age},
		{"velocity_factors", "/api/planning/velocity_factors", "autoware_adapi_v1_msgs/msg/VelocityFactorArray", "routing", false, age},
		{"steering_factors", "/api/planning/steering_factors", "autoware_adapi_v1_msgs/msg/SteeringFactorArray", "routing", false, age},
		{"velocity_limit", "/planning/scenario_planning/current_max_velocity", "autoware_internal_planning_msgs/msg/VelocityLimit", "routing", true, 0},
		{"closest_velocity_limit", "/planning/scenario_planning/velocity_smoother/closest_max_velocity", "autoware_internal_debug_msgs/msg/Float32Stamped", "routing", false, age},
		{"energy", "/api/vehicle/metrics", "autoware_adapi_v1_msgs/msg/VehicleMetrics", "energy", false, age},
	}
}
