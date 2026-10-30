#!/usr/bin/env bash
set -eo pipefail
if [[ -f /usr/local/share/autoware-exporter/runtime-env.sh ]]; then
  source /usr/local/share/autoware-exporter/runtime-env.sh
else
  source "/opt/ros/${ROS_DISTRO:-jazzy}/setup.bash"
  if [[ -f /opt/autoware/setup.bash ]]; then
    source /opt/autoware/setup.bash
  fi
fi
exec /usr/local/bin/autoware-exporter "$@"
