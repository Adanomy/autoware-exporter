ARG ROS_DISTRO=jazzy
FROM golang:1.27.3-bookworm AS go-toolchain
FROM ros:${ROS_DISTRO}-ros-base AS dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential ca-certificates python3-colcon-common-extensions \
    ros-${ROS_DISTRO}-rosidl-default-generators \
    ros-${ROS_DISTRO}-rosidl-typesupport-introspection-c \
    ros-${ROS_DISTRO}-sensor-msgs ros-${ROS_DISTRO}-geographic-msgs && \
    rm -rf /var/lib/apt/lists/*
COPY --from=go-toolchain /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}" GOTOOLCHAIN=local GOWORK=off GOENV=off
SHELL ["/bin/bash", "-eo", "pipefail", "-c"]

FROM dependencies AS compatibility
WORKDIR /interfaces
COPY tests/compatibility/prepare_interfaces.py tests/compatibility/releases.json /prepare/
COPY internal/telemetry/sources.go /prepare/sources.go
ARG AUTOWARE_BRANCH=0.53
RUN python3 /prepare/prepare_interfaces.py --branch "$AUTOWARE_BRANCH" \
    --output /interfaces --sources /prepare/sources.go && \
    source /opt/ros/${ROS_DISTRO}/setup.bash && \
    colcon build --merge-install --install-base /opt/autoware \
      --parallel-workers 2 --cmake-args -DBUILD_TESTING=OFF -DCMAKE_BUILD_TYPE=Release && \
    rm -rf /interfaces/build /interfaces/log
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY cmd ./cmd
COPY internal ./internal
COPY scripts ./scripts
COPY tests ./tests
RUN bash scripts/build-native.sh
ENV AUTOWARE_EXPORTER_ROS_TESTS=1 AUTOWARE_EXPORTER_COMPATIBILITY_TESTS=1 ROS_LOCALHOST_ONLY=1
CMD ["bash", "-eo", "pipefail", "-c", "source scripts/build-native.sh && go vet -tags 'ros2,ros2test' ./... && go test -race -count=1 -timeout=3m -tags 'ros2,ros2test' ./..."]
