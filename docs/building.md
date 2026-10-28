# Building and releases

## Native build

Use Linux, **Go 1.27.3**, CGO, and the ROS/Autoware headers and libraries from
the target deployment. The exporter links directly to ROS 2 C libraries and
loads C and introspection type support for subscribed message packages.

```sh
bash scripts/build-native.sh
bin/autoware-exporter --vehicle=vehicle-01:0
```

The script sources `/opt/ros/${ROS_DISTRO:-jazzy}/setup.bash` and, when present,
`/opt/autoware/setup.bash`, then sets include and library search paths. Supply
matching `CGO_CFLAGS`, `CGO_LDFLAGS`, and runtime library paths for a different
installation layout. Build and runtime libraries must agree.

An ordinary build without `-tags ros2` provides a stub transport for unit tests
and CLI help. It cannot monitor ROS. Native builds use `CGO_ENABLED=1` and
`-tags ros2`; a Humble binary is not interchangeable with a Jazzy binary.

## Container build

The default Dockerfile uses a public Autoware Jazzy image pinned by digest.
Override it with a compatible image for your deployment:

```sh
docker build --build-arg ROS_IMAGE=your-autoware-image:humble \
  -t autoware-exporter:local .
```

The image must contain ROS 2 and the message type-support libraries needed by
your selected sources. Missing optional packages disable only their own sources.
The entrypoint sources the image's ROS/Autoware environment before executing
the exporter. The final image runs as UID/GID 65532.

## Testing

Default checks require no ROS installation:

```sh
go test -race ./...
go vet ./...
```

After sourcing the build script in a matching Linux environment:

```sh
source scripts/build-native.sh
AUTOWARE_EXPORTER_ROS_TESTS=1 go test -race -tags 'ros2,ros2test' ./...
AUTOWARE_EXPORTER_LIVE_DOMAINS=51,52,53 go test -race -tags ros2 -v ./tests/integration
```

The first command checks the native transport and introspection. The second
subscribes to existing ROS publishers and does not create vehicle commands or
test publishers. `AUTOWARE_EXPORTER_LIVE_PRODUCT_SOURCES=1` also requires the
configured evaluator and performance sources. Run synthetic publishers only
inside [the isolated compatibility containers](../tests/compatibility/README.md).

## Release artifacts

Version tags use `vMAJOR.MINOR.PATCH`. CI builds Linux amd64/arm64 artifacts for
Humble and Jazzy, verifies reproducibility, and publishes checksummed archives,
the Grafana dashboard, build metadata, and matching container images.

Archives contain a native binary, its build metadata, the license, and usage
documentation. They need the matching ROS/Autoware environment; the recorded
image digest identifies the build/runtime library baseline. No macOS, Windows,
or static ROS-independent binary is produced.

Release and unpublished preview commands are documented by
`python3 ci/release.py --help`. A preview uses `--snapshot`; a published release
requires a clean, committed source tree with one stable version tag at HEAD.
Do not create a version file or edit the source to bump the exporter version.

GitHub Releases retain the release notes. Container tags distinguish ROS
distributions: `vX.Y.Z-jazzy` and `vX.Y.Z-humble`; `latest-jazzy` and
`latest-humble` track the newest stable exporter release. The unsuffixed
`vX.Y.Z`, `X.Y.Z`, and `latest` aliases select Jazzy. Use an image digest to
keep a deployment fixed.
