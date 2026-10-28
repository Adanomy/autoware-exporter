# Metrics

Vehicle and source metrics carry a `vehicle` label. Other labels are shown in
the metric names below. `autoware_exporter_build_info` describes the exporter
process and has no vehicle label. See [source topics and freshness](sources.md)
for the complete ROS subscription list.

## Exporter and source diagnostics

| Metric | Type | Meaning |
| --- | --- | --- |
| `autoware_exporter_build_info{version,revision,go_version}` | Gauge | Exporter build information; value 1 |
| `autoware_vehicle_info{ros_domain_id}` | Gauge | Configured vehicle and ROS domain; value 1 |
| `autoware_ros_context_up` | Gauge | ROS client context initialized; does not imply that vehicle data is fresh |
| `autoware_topic_subscribed{topic}` | Gauge | Subscription created for this configured topic |
| `autoware_topic_publishers{topic}` | Gauge | Number of discovered publishers for this topic |
| `autoware_topic_data_available{topic}` | Gauge | A message is available from an active publisher within the configured freshness policy |
| `autoware_topic_messages_received_total{topic}` | Counter | Messages received by this exporter, including messages that failed decoding |
| `autoware_topic_receive_errors_total{topic}` | Counter | Subscription, publisher discovery, or message decoding errors observed by this exporter |
| `autoware_topic_last_received_timestamp_seconds{topic}` | Gauge | Exporter-host Unix time when the latest valid decoded message was received |
| `autoware_topic_last_received_age_seconds{topic}` | Gauge | Monotonic real time elapsed since the latest valid decoded message was received |
| `autoware_topic_source_timestamp_seconds{topic,component}` | Gauge | Timestamp carried by the ROS message in its source clock; not necessarily Unix time |

## Vehicle states and ROS time

| Metric | Type | Meaning |
| --- | --- | --- |
| `autoware_ros_time_seconds` | Gauge | Latest observed ROS /clock time; can pause or move backwards |
| `autoware_heartbeat_sequence` | Gauge | Last observed AD API heartbeat sequence, wrapping at 65535 |
| `autoware_operation_mode{mode}` | Gauge | Current AD API operation mode as a fixed one-hot state |
| `autoware_control_enabled` | Gauge | Autoware control is enabled |
| `autoware_operation_mode_in_transition` | Gauge | An operation mode transition is in progress |
| `autoware_operation_mode_available{mode}` | Gauge | Switching to the specified operation mode is available |
| `autoware_localization_state{state}` | Gauge | Localization initialization state as a fixed one-hot state |
| `autoware_route_state{state}` | Gauge | Route state as a fixed one-hot state |
| `autoware_motion_state{state}` | Gauge | Motion state as a fixed one-hot state |
| `autoware_mrm_state{state}` | Gauge | Minimal risk maneuver state as a fixed one-hot state |
| `autoware_mrm_behavior{behavior}` | Gauge | Known AD API minimal risk maneuver behavior as a fixed one-hot state; unrecognized extensible behavior IDs are omitted |

## Measured vehicle telemetry

| Metric | Type | Meaning |
| --- | --- | --- |
| `autoware_vehicle_position_meters{axis}` | Gauge | Vehicle pose position in the message coordinate frame |
| `autoware_vehicle_yaw_radians` | Gauge | Vehicle pose yaw extracted from its orientation quaternion |
| `autoware_vehicle_velocity_meters_per_second{axis}` | Gauge | Signed vehicle linear velocity components in the twist message frame |
| `autoware_vehicle_speed_meters_per_second` | Gauge | Magnitude of the vehicle linear velocity vector |
| `autoware_vehicle_acceleration_meters_per_second_squared{axis}` | Gauge | Vehicle linear acceleration components in the acceleration message frame |
| `autoware_vehicle_angular_velocity_radians_per_second{axis}` | Gauge | Vehicle angular velocity components in the twist message frame |
| `autoware_vehicle_yaw_rate_radians_per_second` | Gauge | Vehicle angular velocity about the z axis |
| `autoware_vehicle_steering_angle_radians` | Gauge | Reported vehicle tire steering angle |
| `autoware_vehicle_gear{gear}` | Gauge | Reported vehicle gear as a fixed one-hot state |
| `autoware_vehicle_turn_indicator{state}` | Gauge | Reported turn indicator as a fixed one-hot state |
| `autoware_vehicle_hazard_lights{state}` | Gauge | Reported hazard light status as a fixed one-hot state |
| `autoware_vehicle_energy_ratio` | Gauge | Remaining vehicle fuel or battery divided by maximum capacity |

## ROS diagnostics

| Metric | Type | Meaning |
| --- | --- | --- |
| `autoware_diagnostic_level{path}` | Gauge | AD API diagnostic graph node level: 0 OK, 1 WARN, 2 ERROR, 3 STALE |
| `autoware_diagnostics{kind,level}` | Gauge | Number of diagnostic graph entries by kind and level |

## Commands and tracking errors

| Metric | Type | Meaning |
| --- | --- | --- |
| `autoware_command_velocity_meters_per_second` | Gauge | Commanded longitudinal velocity; negative is reverse |
| `autoware_command_acceleration_meters_per_second_squared` | Gauge | Commanded longitudinal acceleration |
| `autoware_command_jerk_meters_per_second_cubed` | Gauge | Commanded longitudinal jerk when explicitly defined |
| `autoware_command_steering_angle_radians` | Gauge | Commanded tire steering angle |
| `autoware_command_steering_rate_radians_per_second` | Gauge | Commanded tire steering angular velocity |
| `autoware_command_throttle_ratio` | Gauge | Commanded normalized throttle pedal position |
| `autoware_command_brake_ratio` | Gauge | Commanded normalized brake pedal position |
| `autoware_control_lateral_error_meters` | Gauge | Reported lateral tracking error; control performance analysis has priority over the control evaluator fallback |
| `autoware_control_longitudinal_error_meters` | Gauge | Longitudinal tracking error reported by control performance analysis |
| `autoware_control_heading_error_radians` | Gauge | Reported heading tracking error; control performance analysis has priority over the control evaluator fallback |
| `autoware_control_velocity_error_meters_per_second` | Gauge | Reported velocity tracking error; control performance analysis has priority over the control evaluator fallback |
| `autoware_control_jerk_meters_per_second_cubed{axis}` | Gauge | Reported measured driving jerk; control performance analysis has priority, with longitudinal control evaluator fallback |

## Localization

| Metric | Type | Meaning |
| --- | --- | --- |
| `autoware_localization_position_stddev_meters{source,axis}` | Gauge | Square root of pose covariance diagonal; estimated uncertainty, not ground-truth error |
| `autoware_localization_yaw_stddev_radians{source}` | Gauge | Square root of yaw covariance; estimated uncertainty, not ground-truth error |
| `autoware_localization_ndt_score{kind}` | Gauge | NDT matching score; not a calibrated probability of localization correctness |
| `autoware_localization_ndt_iterations` | Gauge | Number of iterations used by the most recent NDT match |
| `autoware_localization_ndt_execution_seconds` | Gauge | Execution time of the most recent NDT match |

## Perception and performance

| Metric | Type | Meaning |
| --- | --- | --- |
| `autoware_perception_objects` | Gauge | Number of objects in the latest predicted object array |
| `autoware_perception_objects_by_class{class}` | Gauge | Object count by the highest-probability finite classification |
| `autoware_processing_duration_seconds{module}` | Gauge | Latest reported module processing duration; not a histogram |
| `autoware_pipeline_latency_seconds` | Gauge | Latest total pipeline latency reported by Autoware |
| `autoware_perception_latency_seconds{stage}` | Gauge | Latency reported by perception analytics, converted from milliseconds |
| `autoware_traffic_signal{group,index,color,shape,status}` | Gauge | Raw reported traffic light element; bounded group/element indexes, value 1. This does not assert traffic-rule compliance |
| `autoware_traffic_signal_confidence_ratio{group,index,color,shape,status}` | Gauge | Reported traffic light element classification confidence |

## Routing and planning

| Metric | Type | Meaning |
| --- | --- | --- |
| `autoware_route_remaining_distance_meters` | Gauge | Remaining route distance reported by Autoware; unavailable without an active route |
| `autoware_route_estimated_remaining_seconds` | Gauge | Remaining route time estimate reported by Autoware; not a traffic-aware arrival forecast |
| `autoware_goal_deviation_meters{axis,kind}` | Gauge | Goal deviation reported by the control evaluator; absolute values are supplied upstream, not computed by the exporter |
| `autoware_goal_yaw_deviation_radians{kind}` | Gauge | Goal yaw deviation reported by the control evaluator; signed or upstream absolute value |
| `autoware_centerline_deviation_meters{kind}` | Gauge | Centerline deviation reported by the control evaluator; signed or upstream absolute value |
| `autoware_boundary_distance_meters{side,boundary}` | Gauge | Raw boundary distance reported by the evaluator; may be capped at its search distance when no boundary is found |
| `autoware_boundary_crossings{side,boundary}` | Gauge | Snapshot of upstream boundary crossings since the evaluator started; resets with its process and is not an exporter counter |
| `autoware_planning_steering_factor{index,behavior,direction,status}` | Gauge | Reported steering factor; one series per bounded array index, value 1 |
| `autoware_planning_steering_factor_distance_meters{index,behavior,direction,status,point}` | Gauge | Reported steering factor start or end distance; negative values may be behind the vehicle |
| `autoware_planning_velocity_factor{index,behavior,status}` | Gauge | Reported velocity factor; one series per bounded array index, value 1 |
| `autoware_planning_velocity_factor_distance_meters{index,behavior,status}` | Gauge | Reported velocity factor distance; negative values may be behind the vehicle |
| `autoware_velocity_limit_meters_per_second{scope}` | Gauge | Reported global or closest local maximum velocity; not an inferred map speed limit |
| `autoware_planning_stability_meters{kind,statistic}` | Gauge | Upstream trajectory stability summary; not a percentile computed from exporter scrapes |
| `autoware_planning_changes{signal}` | Gauge | Upstream count of steering or blinker changes within its configured duration window; not a cumulative counter |
| `autoware_planning_trajectory_length_meters{statistic}` | Gauge | Upstream summary of planned trajectory length |
| `autoware_planning_trajectory_duration_seconds{statistic}` | Gauge | Upstream summary of planned trajectory duration |
| `autoware_planning_time_to_collision_seconds{statistic}` | Gauge | Reported obstacle time-to-collision summary from the planning evaluator; absent when upstream supplies no finite value |

## Emergency and safety

| Metric | Type | Meaning |
| --- | --- | --- |
| `autoware_emergency_command` | Gauge | Emergency command flag reported by the command gate |
| `autoware_emergency_holding` | Gauge | Emergency holding state reported by the command gate |
| `autoware_hazard_level` | Gauge | Reported hazard level: 0 no fault, 1 safe fault, 2 latent fault, 3 single-point fault |
| `autoware_hazard_flag{flag}` | Gauge | Reported hazard state flag |
| `autoware_door_state{index,state}` | Gauge | Reported door state as a fixed one-hot state, identified by bounded array index |
| `autoware_aeb_decision{decision}` | Gauge | Current decision explicitly reported by AEB metrics; an empty array is unavailable, not a no-braking assertion |
| `autoware_aeb_rss_distance_meters` | Gauge | Required longitudinal separation reported by the AEB RSS model; not measured obstacle clearance and may be signed |

## Standard collectors

| Metric | Type | Meaning |
| --- | --- | --- |
| `go_*` | Gauge / Counter / Summary | Go runtime metrics for the exporter process |
| `process_*` | Gauge / Counter | Resource usage and lifecycle metrics for the exporter process |

## Interpretation

- ROS messages are decoded directly into immutable Go maps through the native type introspection metadata. There is no intermediate JSON representation. Decode limits apply after DDS deserialization: a 4 MiB payload accounting budget, 4096 elements per array, 32 nested message levels, and 65536 values and containers in total. These limits do not cap DDS allocations or total Go heap usage.
- Fixed enum states are one-hot gauges: the active state is 1 and other known states are 0.
- Message counters count what this exporter receives, not the total number published in ROS.
- Receive-age measurements use monotonic real time; source timestamps retain their ROS clock.
- The `component` label distinguishes embedded kinematics stamps from the containing message stamp.
- A periodic source is unavailable after its receive-age limit; state notification topics do not expire merely because their value is unchanged.
- Notification publisher endpoint identities are checked as well as counts. A replacement refreshes that vehicle's ROS context and requests new retained states, even when the publisher count remains one.
- Missing publishers, missing messages, invalid numeric values, and mismatched diagnostic graph IDs do not produce fabricated zero measurements.
- Diagnostic labels contain graph paths, not free-form messages, hardware IDs, or arbitrary key/value data.
- Perception uses a fixed classification set; object UUIDs never become labels.
- NDT scores are matching scores, and covariance-derived standard deviations estimate uncertainty. Neither is ground-truth localization error.
- Processing duration and pipeline latency convert the reported milliseconds to seconds. They describe the latest sample; no percentile or histogram is inferred from sparse samples.
- Commanded and measured vehicle values are separate metrics. Controller commands do not confirm application by the vehicle.
- Route distance/time requires an active route and the optional calculator; it is not a traffic-aware ETA.
- Energy is only available when the vehicle supplies energy messages. A publisher without messages is not a zero battery.

- Evaluator `MetricArray` values are parsed only for exact fixed keys and expected units. Empty, malformed, non-finite, duplicate, and unsupported entries are omitted independently. Unknown metric names, object UUIDs, sender strings, and free-form factor detail/sequence fields never become labels.
- Goal and centerline absolute deviations are the values supplied upstream. The exporter does not calculate absolute values or new window summaries.
- Boundary distances are raw evaluator observations and may equal the search-distance cap when no matching boundary is found. They do not prove a boundary was found. `boundary_crossings` is a Gauge snapshot of the upstream evaluator's cumulative non-positive uncrossable-boundary transitions; zero crossings does not invalidate the distance. Regular left/right lane-boundary distances have no corresponding crossing-count field.
- Steering and velocity factors preserve separate entries by array slot; steering start/end distances remain separate. At most 64 factors are accepted per array. `behavior`, `direction`, and `status` use fixed vocabularies; unsupported enum extensions normalize to `unknown`, with indexes preventing duplicate series. `PlanningBehavior.UNKNOWN` is an empty string upstream.
- `velocity_limit_meters_per_second{scope="global"}` uses the retained `VelocityLimit.max_velocity`; `scope="local"` uses the smoother's latest closest-limit scalar. Neither is a reconstructed map speed limit.
- Perception analytics latencies are the three reported measurement-to-tracked, prediction, and total values, converted from milliseconds. Planning stability/Frechet, horizon length/time summaries, and recent-window steering/blinker change counts are exported exactly as reported. The counts remain Gauges and use the upstream configured window, not an exporter-selected window.
- Existing lateral/heading/velocity error metrics prefer a valid control-performance-analysis field, falling back individually to the control evaluator. Both use actual-minus-reference conventions, but legacy interpolation/filtering and the evaluator's nearest-point reference can differ. Longitudinal position error has no goal-deviation fallback. Measured jerk falls back only on the longitudinal axis; no lateral jerk is derived.
- Traffic signal groups and elements use bounded array slots (up to 128 groups and 16 elements each); door states use up to 32 door slots. Empty arrays emit no state measurements. These slots are not stable map, maneuver, or hardware identities. Traffic light classification/confidence does not assert traffic-rule compliance.
- Obstacle TTC uses only the exact precomputed `obstacle_ttc/worst` summary. The exporter neither reduces per-object metrics nor exports object identifiers. AEB RSS distance is the model's required longitudinal separation, may be signed, and is not measured obstacle clearance. An empty AEB decision array does not prove that braking is unnecessary and emits no decision series.
