# Contributing

Open an [issue](https://github.com/Adanomy/autoware-exporter/issues) for bugs and
feature proposals. For vulnerabilities, follow [SECURITY.md](SECURITY.md).

## Checks

Use **Go 1.27.3** and a C compiler for the race detector. Run from the repository
root:

```sh
gofmt -w cmd internal tests
go vet ./...
go test -race ./...
python3 -m unittest discover -s ci
bash -n scripts/build-native.sh scripts/entrypoint.sh
python3 -m json.tool grafana/autoware-exporter.json > /dev/null
```

Default Go tests require no ROS installation. Native checks need Linux and
matching ROS C headers, libraries, and message type support. Run an isolated
compatibility environment as described in
[tests/compatibility](tests/compatibility/README.md). Live integration checks
subscribe to existing publishers only; see [building and testing](docs/building.md).

Message changes need upstream definitions and regression tests. Keep the current
and two previous stable Universe branches covered on Humble and Jazzy. The
matrix defines tested coverage; do not add a runtime Autoware version gate.

## Pull requests

- Keep each PR focused and describe the problem, resulting behavior, and checks run.
- Add tests for behavior changes and update affected documentation.
- Keep code comments, logs, errors, documentation, and PR descriptions in English.
- Preserve read-only ROS access; add no publishers, service calls, or vehicle commands.
- Keep synthetic publishers inside isolated test environments.
- Describe user-visible changes and compatibility impacts in the PR. Release
  notes are published in [GitHub Releases](https://github.com/Adanomy/autoware-exporter/releases).

Use [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/)
for commit messages and PR titles, with a short English description:

```text
feat(telemetry): collect another Autoware source
fix(exporter): discard stale vehicle metrics
docs: clarify ROS domain configuration
```

Mark breaking changes with `!` and explain them in the description. Contributions
are distributed under the project's [MIT license](LICENSE).
