#!/usr/bin/env bash
set -eo pipefail
source "/opt/ros/${ROS_DISTRO:-jazzy}/setup.bash"
if [[ -f /opt/autoware/setup.bash ]]; then
  source /opt/autoware/setup.bash
fi
exec /usr/local/bin/autoware-exporter "$@"
