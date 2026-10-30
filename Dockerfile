# Build and runtime must use matching ROS libraries and message type supports.
ARG SOURCE_DATE_EPOCH=0
ARG ROS_IMAGE=ghcr.io/autowarefoundation/autoware:universe-jazzy@sha256:e7904537b72333967a4a9c3849f7f1c7facd7dea6dd9b617d7c505ebce598d86
ARG GO_IMAGE=golang:1.27.3-bookworm@sha256:d9b6a1beb9474adc2a1ce842573639b44b2a0dd49b6669f51d3821baf9ec37c9
FROM ${GO_IMAGE} AS go-toolchain

FROM ${ROS_IMAGE} AS build
USER root
RUN if ! command -v gcc >/dev/null || ! command -v make >/dev/null || \
       [ ! -s /etc/ssl/certs/ca-certificates.crt ]; then \
      apt-get update && apt-get install -y --no-install-recommends build-essential ca-certificates && \
      rm -rf /var/lib/apt/lists/*; \
    fi
COPY --from=go-toolchain /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}" GOTOOLCHAIN=local GOWORK=off GOENV=off
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY cmd ./cmd
COPY internal ./internal
COPY tests ./tests
COPY scripts/build-native.sh ./scripts/build-native.sh
ARG VERSION=dev
ARG REVISION=unknown
RUN test "$(go env GOVERSION)" = "go$(awk '/^go / {print $2}' go.mod)" && \
    VERSION="$VERSION" REVISION="$REVISION" OUTPUT_DIR=/out bash scripts/build-native.sh
COPY scripts/prepare-runtime.py ./scripts/prepare-runtime.py
RUN bash -eo pipefail -c 'source "/opt/ros/${ROS_DISTRO:-jazzy}/setup.bash"; \
    if [[ -f /opt/autoware/setup.bash ]]; then source /opt/autoware/setup.bash; fi; \
    python3 scripts/prepare-runtime.py --binary /out/autoware-exporter \
      --sources internal/telemetry/sources.go --output /runtime'

# Ubuntu userspace and ROS/DDS libraries come from the pinned build image.
# Copy their complete runtime closure without inheriting Autoware's layers.
FROM scratch
ARG ROS_DISTRO
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.source="https://github.com/Adanomy/autoware-exporter" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$REVISION" \
      org.opencontainers.image.title="Autoware Exporter"
COPY --from=build /runtime/ /
COPY --chmod=0755 scripts/entrypoint.sh /usr/local/bin/autoware-exporter-entrypoint
WORKDIR /tmp
ENV HOME=/tmp CYCLONEDDS_URI="" LANG=C.UTF-8 LC_ALL=C.UTF-8 \
    PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin ROS_DISTRO=${ROS_DISTRO}
RUN ["/bin/bash", "-ec", "expected_distro=$ROS_DISTRO; source /usr/local/share/autoware-exporter/runtime-env.sh; if [[ -n $expected_distro && $ROS_DISTRO != $expected_distro ]]; then echo 'ROS_DISTRO must match ROS_IMAGE' >&2; exit 1; fi"]
USER 65532:65532
EXPOSE 9841
ENTRYPOINT ["/usr/local/bin/autoware-exporter-entrypoint"]
CMD []
