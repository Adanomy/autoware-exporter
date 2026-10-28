#!/usr/bin/env bash
# Compile against the headers and libraries of the ROS installation used at runtime.
set -eo pipefail

ros_distro="${ROS_DISTRO:-jazzy}"
ros_prefix="/opt/ros/$ros_distro"
if [[ ! -f "$ros_prefix/setup.bash" ]]; then
  echo "ROS setup not found: $ros_prefix/setup.bash" >&2
  exit 1
fi
source "$ros_prefix/setup.bash"
if [[ -f /opt/autoware/setup.bash ]]; then
  source /opt/autoware/setup.bash
fi

export CGO_ENABLED=1
export GOWORK=off GOENV=off GOFLAGS= GOEXPERIMENT=
export CGO_CFLAGS="${CGO_CFLAGS:-}"
export CGO_LDFLAGS="${CGO_LDFLAGS:-}"
for prefix in "$ros_prefix" /opt/autoware /opt/autoware/*; do
  [[ -d "$prefix" ]] || continue
  for include in "$prefix/include" "$prefix"/include/*; do
    if [[ -d "$include" ]]; then
      # ROS dependency headers such as dds/features.h must not shadow libc headers.
      CGO_CFLAGS+=" -idirafter $include"
    fi
  done
  if [[ -d "$prefix/lib" ]]; then
    CGO_LDFLAGS+=" -L$prefix/lib -Wl,-rpath,$prefix/lib"
  fi
done

mkdir -p "${OUTPUT_DIR:-bin}"
go build -mod=readonly -trimpath -buildvcs=false -tags ros2 \
  -ldflags="-s -w -buildid= -X main.version=${VERSION:-dev} -X main.revision=${REVISION:-unknown}" \
  -o "${OUTPUT_DIR:-bin}/autoware-exporter" ./cmd/autoware-exporter
