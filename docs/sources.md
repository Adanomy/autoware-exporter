# ROS sources

These are the unprefixed topic names. A configured vehicle namespace prefixes each topic.
Type-support libraries for the message types must be installed in the native runtime.
Optional missing sources are retried independently.

| Key | Group | Topic | Message type | Freshness |
| --- | --- | --- | --- | --- |
| `operation_mode` | `state` | `/api/operation_mode/state` | `autoware_adapi_v1_msgs/msg/OperationModeState` | Notification; no receive-age expiry |
| `localization_state` | `state` | `/api/localization/initialization_state` | `autoware_adapi_v1_msgs/msg/LocalizationInitializationState` | Notification; no receive-age expiry |
| `route_state` | `state` | `/api/routing/state` | `autoware_adapi_v1_msgs/msg/RouteState` | Notification; no receive-age expiry |
| `motion_state` | `state` | `/api/motion/state` | `autoware_adapi_v1_msgs/msg/MotionState` | Notification; no receive-age expiry |
| `mrm_state` | `state` | `/api/fail_safe/mrm_state` | `autoware_adapi_v1_msgs/msg/MrmState` | Notification; no receive-age expiry |
| `emergency_holding` | `state` | `/system/emergency_holding` | `tier4_system_msgs/msg/EmergencyHoldingState` | `--source.max-age` |
| `clock` | `state` | `/clock` | `rosgraph_msgs/msg/Clock` | `--source.max-age` |
| `heartbeat` | `state` | `/api/system/heartbeat` | `autoware_adapi_v1_msgs/msg/Heartbeat` | `--source.max-age` |
| `kinematics` | `vehicle` | `/api/vehicle/kinematics` | `autoware_adapi_v1_msgs/msg/VehicleKinematics` | `--source.max-age` |
| `vehicle_status` | `vehicle` | `/api/vehicle/status` | `autoware_adapi_v1_msgs/msg/VehicleStatus` | `--source.max-age` |
| `doors` | `vehicle` | `/api/vehicle/doors/status` | `autoware_adapi_v1_msgs/msg/DoorStatusArray` | Notification; no receive-age expiry |
| `diagnostics_struct` | `diagnostics` | `/api/system/diagnostics/struct` | `autoware_adapi_v1_msgs/msg/DiagGraphStruct` | Notification; no receive-age expiry |
| `diagnostics_status` | `diagnostics` | `/api/system/diagnostics/status` | `autoware_adapi_v1_msgs/msg/DiagGraphStatus` | `--source.max-age` |
| `hazard_status` | `diagnostics` | `/system/emergency/hazard_status` | `autoware_system_msgs/msg/HazardStatusStamped` | `--source.max-age` |
| `command_velocity` | `control` | `/api/control/command/velocity` | `autoware_adapi_v1_msgs/msg/VelocityCommand` | `--source.max-age` |
| `command_acceleration` | `control` | `/api/control/command/acceleration` | `autoware_adapi_v1_msgs/msg/AccelerationCommand` | `--source.max-age` |
| `command_steering` | `control` | `/api/control/command/steering` | `autoware_adapi_v1_msgs/msg/SteeringCommand` | `--source.max-age` |
| `command_pedals` | `control` | `/api/control/command/pedals` | `autoware_adapi_v1_msgs/msg/PedalsCommand` | `--source.max-age` |
| `control_command` | `control` | `/control/command/control_cmd` | `autoware_control_msgs/msg/Control` | `--source.max-age` |
| `control_errors` | `control` | `/control_performance/performance_vars` | `autoware_control_performance_analysis/msg/ErrorStamped` | `--source.max-age` |
| `driving_status` | `control` | `/control_performance/driving_status` | `autoware_control_performance_analysis/msg/DrivingMonitorStamped` | `--source.max-age` |
| `control_evaluator` | `control` | `/control/control_evaluator/metrics` | `tier4_metric_msgs/msg/MetricArray` | `--source.max-age` |
| `emergency_command` | `control` | `/control/command/emergency_cmd` | `tier4_vehicle_msgs/msg/VehicleEmergencyStamped` | `--source.max-age` |
| `aeb_metrics` | `control` | `/control/autonomous_emergency_braking/metrics` | `tier4_metric_msgs/msg/MetricArray` | `--source.max-age` |
| `aeb_rss_distance` | `control` | `/control/autonomous_emergency_braking/debug/rss_distance` | `tier4_debug_msgs/msg/Float32Stamped` | `--source.max-age` |
| `ndt_score` | `localization` | `/localization/pose_estimator/transform_probability` | `autoware_internal_debug_msgs/msg/Float32Stamped` | `--source.max-age` |
| `ndt_voxel_score` | `localization` | `/localization/pose_estimator/nearest_voxel_transformation_likelihood` | `autoware_internal_debug_msgs/msg/Float32Stamped` | `--source.max-age` |
| `ndt_iterations` | `localization` | `/localization/pose_estimator/iteration_num` | `autoware_internal_debug_msgs/msg/Int32Stamped` | `--source.max-age` |
| `ndt_execution` | `localization` | `/localization/pose_estimator/exe_time_ms` | `autoware_internal_debug_msgs/msg/Float32Stamped` | `--source.max-age` |
| `ndt_covariance` | `localization` | `/localization/pose_estimator/pose_with_covariance` | `geometry_msgs/msg/PoseWithCovarianceStamped` | `--source.max-age` |
| `objects` | `perception` | `/perception/object_recognition/objects` | `autoware_perception_msgs/msg/PredictedObjects` | `--source.max-age` |
| `perception_analytics` | `perception` | `/perception/perception_analytics_publisher/perception_analytics` | `tier4_metric_msgs/msg/MetricArray` | `--source.max-age` |
| `traffic_signals` | `perception` | `/perception/traffic_light_recognition/traffic_signals` | `autoware_perception_msgs/msg/TrafficLightGroupArray` | `--source.max-age` |
| `processing_localization_filter` | `performance` | `/localization/pose_twist_fusion_filter/debug/processing_time_ms` | `autoware_internal_debug_msgs/msg/Float64Stamped` | `--source.max-age` |
| `processing_lateral_controller` | `performance` | `/control/trajectory_follower/controller_node_exe/lateral/debug/processing_time_ms` | `autoware_internal_debug_msgs/msg/Float64Stamped` | `--source.max-age` |
| `processing_longitudinal_controller` | `performance` | `/control/trajectory_follower/controller_node_exe/longitudinal/debug/processing_time_ms` | `autoware_internal_debug_msgs/msg/Float64Stamped` | `--source.max-age` |
| `processing_mission_planner` | `performance` | `/planning/mission_planning/mission_planner/debug/processing_time_ms` | `autoware_internal_debug_msgs/msg/Float64Stamped` | `--source.max-age` |
| `processing_object_tracker` | `performance` | `/perception/object_recognition/tracking/multi_object_tracker/debug/processing_time_ms` | `autoware_internal_debug_msgs/msg/Float64Stamped` | `--source.max-age` |
| `processing_object_prediction` | `performance` | `/perception/object_recognition/prediction/map_based_prediction/debug/processing_time_ms` | `autoware_internal_debug_msgs/msg/Float64Stamped` | `--source.max-age` |
| `pipeline_latency` | `performance` | `/system/pipeline_latency_monitor/output/total_latency_ms` | `autoware_internal_debug_msgs/msg/Float64Stamped` | `--source.max-age` |
| `remaining_route` | `routing` | `/planning/mission_remaining_distance_time` | `autoware_internal_msgs/msg/MissionRemainingDistanceTime` | `--source.max-age` |
| `planning_evaluator` | `routing` | `/planning/planning_evaluator/metrics` | `tier4_metric_msgs/msg/MetricArray` | `--source.max-age` |
| `velocity_factors` | `routing` | `/api/planning/velocity_factors` | `autoware_adapi_v1_msgs/msg/VelocityFactorArray` | `--source.max-age` |
| `steering_factors` | `routing` | `/api/planning/steering_factors` | `autoware_adapi_v1_msgs/msg/SteeringFactorArray` | `--source.max-age` |
| `velocity_limit` | `routing` | `/planning/scenario_planning/current_max_velocity` | `autoware_internal_planning_msgs/msg/VelocityLimit` | Notification; no receive-age expiry |
| `closest_velocity_limit` | `routing` | `/planning/scenario_planning/velocity_smoother/closest_max_velocity` | `autoware_internal_debug_msgs/msg/Float32Stamped` | `--source.max-age` |
| `energy` | `energy` | `/api/vehicle/metrics` | `autoware_adapi_v1_msgs/msg/VehicleMetrics` | `--source.max-age` |

See [metric definitions and interpretation](metrics.md) for the exported measurements.
