# Grafana

Import [autoware-exporter.json](autoware-exporter.json) through
**Dashboards → New → Import** and choose any folder. Select a **Prometheus**
datasource, **Job**, **Instance**, and **Vehicles**. Jobs are discovered from
`autoware_exporter_build_info`; the saved defaults select datasource UID
`prometheus`, job `autoware-exporter`, and all instances and vehicles.

[Full dashboard preview](../docs/images/dashboard-full.png).

The dashboard refreshes every 15 seconds by default. It uses built-in Grafana
panels only. The repository stores the finished browser export directly.

## Vehicle and source state

The overview shows operation mode, localization, routing, motion, component
issues, and fail-safe state. Motion panels compare measured and commanded
values with published limits, blockers, pipeline latency, and tracking errors.
Separate sections cover control, localization, planning, perception, source
diagnostics, optional vehicle data, and advanced inspection.

Current-state panels require a successful scrape and use instant queries.
Missing measurements remain **No data**. Optional topics, empty event arrays,
and unavailable publishers are distinct conditions. `/healthz` checks only the
HTTP process; an initialized ROS context does not prove fresh vehicle data.

## Time and interpretation

ROS timestamps and `/clock` remain in their source clocks. ROS clock progression
shows the observed one-minute slope; heartbeat changes count observed value
changes rather than publisher frequency. Processing durations and pipeline
latency are the last reported samples, not percentiles.

NDT scores describe matching, covariance describes estimated uncertainty, and
goal deviations describe the evaluator's current target. None of these proves
ground-truth accuracy, mission completion, or safe motion. See
[metric interpretation](../docs/metrics.md#interpretation) and
[source topics and freshness](../docs/sources.md).

## Updates and compatibility

Edit the dashboard in Grafana, then use **Export dashboard → Export as code →
Download file** to replace `autoware-exporter.json`. Keep the title
**Autoware Exporter** and review the exported changes before committing.
The saved UID is `adanomy-autoware`; no generator is needed.

Rendered with Grafana **12.4.1** and Prometheus. CI checks JSON syntax; validate
panel rendering and queries in Grafana after editing. Standard Go/process
metrics vary by operating system and Go version; unavailable metrics remain
**No data**. With host networking, process network counters include traffic
from the shared host network namespace.
