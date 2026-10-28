# Build and runtime must use matching ROS libraries and message type supports.
ARG SOURCE_DATE_EPOCH=0
ARG ROS_IMAGE=ghcr.io/autowarefoundation/autoware:universe-jazzy@sha256:e7904537b72333967a4a9c3849f7f1c7facd7dea6dd9b617d7c505ebce598d86
FROM golang:1.27.3-bookworm@sha256:d9b6a1beb9474adc2a1ce842573639b44b2a0dd49b6669f51d3821baf9ec37c9 AS go-toolchain

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
COPY scripts ./scripts
ARG VERSION=dev
ARG REVISION=unknown
RUN test "$(go env GOVERSION)" = "go$(awk '/^go / {print $2}' go.mod)" && \
    VERSION="$VERSION" REVISION="$REVISION" OUTPUT_DIR=/out bash scripts/build-native.sh

FROM ${ROS_IMAGE}
USER root
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.source="https://github.com/Adanomy/autoware-exporter" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$REVISION" \
      org.opencontainers.image.title="Autoware Exporter"
COPY --from=build /out/autoware-exporter /usr/local/bin/autoware-exporter
COPY scripts/entrypoint.sh /usr/local/bin/autoware-exporter-entrypoint
RUN chmod 0755 /usr/local/bin/autoware-exporter /usr/local/bin/autoware-exporter-entrypoint
WORKDIR /tmp
ENV HOME=/tmp CYCLONEDDS_URI=""
USER 65532:65532
EXPOSE 9841
ENTRYPOINT ["/usr/local/bin/autoware-exporter-entrypoint"]
CMD []
