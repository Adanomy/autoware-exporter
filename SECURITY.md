# Security

Report vulnerabilities privately to the maintainers. Use
[GitHub private vulnerability reporting](https://github.com/Adanomy/autoware-exporter/security/advisories/new)
when available. Include the affected version, impact, reproduction steps, and a
minimal example. Keep credentials and private vehicle data out of reports.

Please keep vulnerability details out of public issues and pull requests until
maintainers have coordinated a fix and disclosure with you.

This is a community project; response and remediation times are not guaranteed.

## Deployment boundary

The exporter subscribes to ROS 2 topics. It never publishes commands, calls
vehicle services, changes routes, or advances a simulation. Its HTTP endpoint
has no authentication or TLS; bind it to a trusted network or place it behind
an authenticated proxy. Metrics can include vehicle names, ROS domains,
diagnostic paths, coordinates, and motion data.

ROS discovery and received messages are part of the deployment's trust boundary.
Use the middleware's network and access controls for the monitored ROS graphs.
Decoder limits apply after DDS deserialization and do not bound middleware
allocations or total process memory.
