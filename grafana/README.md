# Grafana

Import dashboard ID **[25976](https://grafana.com/grafana/dashboards/25976)** or
[autoware-exporter.json](autoware-exporter.json) through **Dashboards → New →
Import** and choose any folder. Select a **Datasource**, **Job**, **Instance**,
and **Vehicles**. Jobs are discovered from `autoware_exporter_build_info`;
no scrape job name, datasource UID, vehicle name, or vehicle count is prescribed.
The default All selection expands only the discovered Autoware jobs. Vehicle
panels display vehicle names. Queries identify vehicles by **Job + vehicle name**.
Keep both names stable across deployments, and keep **Instance = All** to include
history across exporter replacements. Different jobs remain separate internally;
use the Job filter to select a source when vehicle names overlap.
Renaming a job or vehicle creates a different logical source; unrelated vehicles
must not share the same name within one job.

[Full dashboard preview](../docs/images/dashboard-full.png) ·
[Localization history preview](../docs/images/dashboard-localization.png).

Set the Prometheus datasource's scrape interval to match the actual Prometheus
job. Query resolution and rate windows follow Grafana's datasource settings;
the explicitly labeled ROS clock and heartbeat windows remain one minute.
The dashboard refreshes every 15 seconds by default and uses built-in Grafana
panels only. The repository stores the finished dashboard JSON directly.

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

Vehicle panels discard exporter instance and deployment labels only after
computing each source's value and checking its scrape status. Concurrent reports
are combined only when they agree. Categorical panels, including **Localization
status history**, show **CONFLICT** on disagreement; numeric history has a gap.
**Conflicting exporter reports** in ROS source diagnostics lists current raw
disagreements by metric and semantic labels. Optional distances can remain absent
even when a factor is present. An empty conflict or issue table does not prove
that data is available. Select an Instance to inspect one exporter.

Topic, diagnostic path, covariance source, axis, factor, signal and other metric
dimensions remain distinct. Rates, clock progression and heartbeat changes are
computed on the original exporter series before combining them. Exporter and Go
runtime panels retain their per-process identity.

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
**Autoware / Prometheus Exporter** and UID `autoware-exporter`. Clear saved
datasource and deployment-specific variable selections, preserve discovery
queries, and review the exported changes before committing. Publish a new
revision under dashboard ID **25976** when updating the catalog copy.

Rendered with Grafana **12.4.1** and Prometheus. CI checks JSON syntax; validate
panel rendering and queries in Grafana after editing. Standard Go/process
metrics vary by operating system and Go version; unavailable metrics remain
**No data**. With host networking, process network counters include traffic
from the shared host network namespace.
