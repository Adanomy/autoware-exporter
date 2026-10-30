# Autoware compatibility tests

Push and pull-request CI runs the Go/tooling checks and four native runtime
smoke tests (Humble/Jazzy on amd64/arm64), without the compatibility matrix or
reproducibility rebuilds. Each smoke test builds once and checks the build
metadata, CLI version, HTTP endpoint and native ROS context in the minimal image.
Startup checks exercise the shipping image's default DDS configuration. In
particular, the default empty `CYCLONEDDS_URI` must allow ROS initialization
without inheriting the build image's deployment-specific Cyclone profile.

Manual CI runs and release validation run the full compatibility matrix below
and the reproducible native release previews. Release builds wait for this
validation before publishing containers. Run the **CI** workflow manually to
check the full matrix for a branch without creating a release.

CI tests Universe branches 0.51, 0.52 and 0.53 against ROS 2 Humble and Jazzy
on native Linux amd64 and arm64 runners. The twelve jobs build against their
own ROS C headers and runtime libraries, run
`go vet` and the Go tests with the race detector, and exercise real DDS
publication, subscription, C introspection and Prometheus collection.

`releases.json` records exact upstream commits for the message repositories and
the Universe control-performance messages. The 0.51 and 0.52 sets follow the
[Autoware 1.8.0](https://github.com/autowarefoundation/autoware/blob/1.8.0/repositories/autoware.repos)
and [1.9.0](https://github.com/autowarefoundation/autoware/blob/1.9.0/repositories/autoware.repos)
manifests. The 0.53 set follows the component versions selected for Core 1.10.0
and Universe 0.53.1 in the upstream development manifest as of 2026-10-28;
it does not imply that an Autoware meta-release 1.10.0 has been published.
Core versions are reference context, not an additional binary built by this test.

`prepare_interfaces.py` selects every message type from the production topic
list and its transitive message dependencies. It copies their upstream `.msg`
contents unchanged into interface-only CMake packages. Standard ROS interfaces
come from the selected ROS distribution. No Autoware algorithms, launch files,
GPU stack or vehicle hardware are needed. Upstream message definitions retain
their upstream licensing; the generated packages are only CI build fixtures.

`publish.py` creates Python messages from the generated upstream type support,
fills `messages.json`, and publishes into DDS domain 200 under `/compatibility`.
Fixtures must cover every production source, and unknown fields or missing enum
constants fail message construction. The Go test requires valid received data
for every source and checks representative metric values, enum labels, nested
messages, arrays and unit conversions. A normal test run skips this publisher
unless `AUTOWARE_EXPORTER_COMPATIBILITY_TESTS=1` is explicitly enabled.

Each job also builds the production Dockerfile against its pinned Autoware ROS
image and compiles the compatibility test in that same build environment.
`check_runtime.py` runs the real exporter and the compiled test inside the
minimal runtime as UID/GID 65532 with a read-only filesystem and dropped
capabilities. A separate container publishes the selected upstream fixtures.
`AUTOWARE_EXPORTER_EXTERNAL_PUBLISHER=1` skips starting Python inside the
compatibility test; all source and metric-value assertions still run.

The publisher has `--network none`; the exporter joins that isolated network
namespace. Fast DDS uses an explicit UDP-only loopback profile with
`ROS_LOCALHOST_ONLY=0`: middleware localhost-only mode did not deliver user data
between the isolated sidecars in this setup. Synthetic messages never reach
existing ROS graphs.
The check also verifies the actual exporter's HTTP health, all source
subscriptions, received data, error counters and metric series. Results and logs
are uploaded as CI artifacts.

Run a matrix entry locally from the repository root:

```sh
docker build --build-arg ROS_DISTRO=humble --build-arg AUTOWARE_BRANCH=0.51 \
  -f tests/compatibility/Dockerfile -t autoware-exporter-compatibility .
docker run --rm --network none autoware-exporter-compatibility
```

The container has loopback networking for DDS; `--network none` isolates it from
existing ROS graphs. Synthetic publishers are test helpers only and are absent
from the production image.

When a new stable branch is released, advance the three branches in the CI
matrix, update the pinned revisions and documented matrix, and run all twelve jobs.
Patch updates within a branch should use its latest validated patch. A passing
test establishes message/transport compatibility; it does not prove that a
deployment publishes every optional topic or that upstream evaluator semantics
have stayed unchanged. Check changed topics, enum meanings and evaluator keys
in the upstream release before advancing the pins.

These files define tested coverage only. Production code has no Autoware version
range, minimum version or release-specific startup gate.
