#!/usr/bin/env python3
"""Test a production runtime against pinned messages in an isolated sidecar."""

import argparse
import json
from pathlib import Path
import re
import subprocess
import tempfile
import time
import uuid


ROOT = Path(__file__).resolve().parents[2]
MIDDLEWARE = "rmw_fastrtps_cpp"


def docker(*args, timeout=120):
    return subprocess.check_output(["docker", *args], text=True, timeout=timeout).strip()


def metrics(body):
    values = {}
    for line in body.splitlines():
        if not line.startswith("autoware_"):
            continue
        match = re.fullmatch(r"(\w+)(?:\{(.*)\})?\s+(\S+)(?:\s+\S+)?", line)
        if not match:
            raise ValueError(f"Invalid metric line: {line}")
        labels = tuple(sorted((name, json.loads('"' + value + '"')) for name, value in
                              re.findall(r'(\w+)="((?:\\.|[^"\\])*)"', match[2] or "")))
        if dict(labels).get("vehicle") == "compatibility":
            values[(match[1], labels)] = float(match[3])
    return values


def check(args):
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    runtime = json.loads(docker("image", "inspect", args.runtime_image))[0]
    fixture = json.loads(docker("image", "inspect", args.fixture_image))[0]
    fixture_environment = dict(item.split("=", 1) for item in fixture["Config"]["Env"])
    if fixture_environment.get("ROS_DISTRO") != args.ros_distro:
        raise ValueError("Fixture ROS distribution does not match the matrix entry")
    if runtime["Architecture"] != args.arch:
        raise ValueError("Runtime architecture does not match the requested matrix entry")
    machine = int.from_bytes(args.test_binary.read_bytes()[18:20], "little")
    if machine != {"amd64": 62, "arm64": 183}[args.arch]:
        raise ValueError("Compatibility test binary has the wrong ELF architecture")
    sources = ROOT / "internal/telemetry/sources.go"
    plan = [dict(Key=key, Topic=topic, Type=message, Latched=latched == "true")
            for key, topic, message, latched in re.findall(
                r'\{"([^\"]+)", "([^\"]+)", "([^\"]+)", "[^\"]+", (true|false),', sources.read_text())]
    fixtures = json.loads((Path(__file__).with_name("messages.json")).read_text())
    if not plan or {item["Key"] for item in plan} != set(fixtures):
        raise ValueError("Fixture coverage does not match the production source list")
    expected_upstream = json.loads(Path(__file__).with_name("releases.json").read_text())[args.branch]
    prefix = "aw-runtime-" + uuid.uuid4().hex[:12]
    publisher, exporter = prefix + "-publisher", prefix + "-exporter"
    created = []
    result = {"passed": False, "ros_distro": args.ros_distro, "universe_branch": args.branch,
              "arch": args.arch, "runtime_image": args.runtime_image, "runtime_id": runtime["Id"],
              "fixture_image": args.fixture_image, "fixture_id": fixture["Id"],
              "fixture_arch": fixture["Architecture"], "middleware": MIDDLEWARE,
              "transport": "UDPv4/loopback"}
    try:
        with tempfile.TemporaryDirectory(prefix="aw-runtime-fixtures-") as temporary:
            directory = Path(temporary)
            directory.chmod(0o755)
            (directory / "plan.json").write_text(json.dumps(plan))
            for filename in ("publish.py", "messages.json", "fastrtps.xml"):
                (directory / filename).write_bytes(Path(__file__).with_name(filename).read_bytes())
            docker("create", "--name", publisher, "--pull=never", "--network", "none",
                   "--platform", "linux/" + fixture["Architecture"], "--cpus=2", "--memory=1g",
                   "--user", "65532:65532", "-e", "HOME=/tmp",
                   "-e", "RMW_IMPLEMENTATION=" + MIDDLEWARE, "-e", "ROS_LOCALHOST_ONLY=0",
                   "-e", "ROS_AUTOMATIC_DISCOVERY_RANGE=SUBNET",
                   "-e", "FASTRTPS_DEFAULT_PROFILES_FILE=/tmp/runtime-fixtures/fastrtps.xml",
                   "-e", "FASTDDS_DEFAULT_PROFILES_FILE=/tmp/runtime-fixtures/fastrtps.xml",
                   "--entrypoint", "/bin/bash", args.fixture_image, "-ec",
                   'source /opt/ros/$ROS_DISTRO/setup.bash; source /opt/autoware/setup.bash; '
                   'if [[ -f /opt/runtime-fixtures/setup.bash ]]; then source /opt/runtime-fixtures/setup.bash; fi; '
                   'if [[ -f /opt/runtime-fixtures-update/setup.bash ]]; then source /opt/runtime-fixtures-update/setup.bash; fi; '
                   'exec python3 /tmp/runtime-fixtures/publish.py < /tmp/runtime-fixtures/plan.json')
            created.append(publisher)
            docker("cp", str(directory), publisher + ":/tmp/runtime-fixtures")
            docker("start", publisher)
            deadline = time.monotonic() + 60
            while "READY" not in docker("logs", publisher):
                if docker("inspect", publisher, "--format", "{{.State.Running}}") != "true":
                    raise ValueError("Pinned message publisher exited before becoming ready")
                if time.monotonic() >= deadline:
                    raise TimeoutError("Pinned message publisher did not become ready")
                time.sleep(1)
        upstream = json.loads(docker("exec", publisher, "python3", "-c",
                                    "from pathlib import Path; print(Path('/interfaces/upstream-revisions.json').read_text())"))
        if upstream != expected_upstream:
            raise ValueError("Fixture image does not contain the declared pinned upstream revisions")
        result["upstream"] = upstream
        docker("create", "--name", exporter, "--pull=never", "--platform", "linux/" + args.arch,
               "--network", "container:" + publisher,
               "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
               "--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=64m", "--cpus=2", "--memory=1g",
               "--mount", f"type=bind,source={args.test_binary.resolve()},target=/runtime-tests/compatibility.test,readonly",
               "--mount", f"type=bind,source={Path(__file__).with_name('fastrtps.xml')},target=/runtime-tests/fastrtps.xml,readonly",
               "-e", "RMW_IMPLEMENTATION=" + MIDDLEWARE, "-e", "ROS_LOCALHOST_ONLY=0",
               "-e", "ROS_AUTOMATIC_DISCOVERY_RANGE=SUBNET",
               "-e", "FASTRTPS_DEFAULT_PROFILES_FILE=/runtime-tests/fastrtps.xml",
               "-e", "FASTDDS_DEFAULT_PROFILES_FILE=/runtime-tests/fastrtps.xml",
               args.runtime_image, "--web.listen-address=127.0.0.1:9841",
               "--vehicle=compatibility:200:/compatibility", "--log.level=debug", "--log.format=json")
        created.append(exporter)
        docker("start", exporter)
        with (output / "go-test.log").open("w") as log:
            subprocess.run(["docker", "exec", "-e", "AUTOWARE_EXPORTER_COMPATIBILITY_TESTS=1",
                            "-e", "AUTOWARE_EXPORTER_EXTERNAL_PUBLISHER=1", exporter,
                            "/bin/bash", "-ec", "source /usr/local/share/autoware-exporter/runtime-env.sh; "
                            "exec /runtime-tests/compatibility.test -test.v -test.run '^TestAutowareMessages$' -test.timeout=90s"],
                           stdout=log, stderr=subprocess.STDOUT, timeout=120, check=True)
        test_log = (output / "go-test.log").read_text()
        checked = re.search(r"received (\d+) ROS sources; checked (\d+) metric observations", test_log)
        if not checked or int(checked[1]) != len(plan):
            raise ValueError("The runtime compatibility test did not cover every source")
        result["go_metric_checks"] = int(checked[2])
        result["sources"] = int(checked[1])
        # Check the actual production exporter process as well as the test binary.
        deadline = time.monotonic() + 30
        samples = []
        for index in range(3):
            while True:
                body = docker("exec", publisher, "python3", "-c",
                              "import urllib.request; print(urllib.request.urlopen('http://127.0.0.1:9841/metrics',timeout=5).read().decode())")
                values = metrics(body)
                available = sum(value == 1 for key, value in values.items() if key[0] == "autoware_topic_data_available")
                if available == len(plan):
                    break
                if time.monotonic() >= deadline:
                    raise ValueError(f"Only {available}/{len(plan)} production sources received data")
                time.sleep(1)
            (output / f"metrics-{index}.prom").write_text(body)
            errors = [key for key, value in values.items() if key[0] == "autoware_topic_receive_errors_total" and value != 0]
            subscriptions = sum(value == 1 for key, value in values.items() if key[0] == "autoware_topic_subscribed")
            if errors or subscriptions != len(plan):
                raise ValueError(f"Production subscription/decoding failure: {errors}")
            samples.append(values)
            time.sleep(1)
        if samples[0].keys() != samples[-1].keys():
            raise ValueError("Production metric series changed under fixed fixtures")
        stalled = [dict(key[1])["topic"] for key, value in samples[0].items()
                   if key[0] == "autoware_topic_messages_received_total" and samples[-1][key] <= value]
        if stalled:
            raise ValueError(f"Production sources stopped receiving messages: {stalled}")
        result["streaming_sources"] = len(plan)
        product = {key: value for key, value in samples[-1].items() if not key[0].startswith("autoware_topic_")}
        result["metric_families_with_vehicle"] = len({key[0] for key in samples[-1]})
        result["series_with_vehicle"] = len(samples[-1])
        result["product_values"] = [{"metric": key[0], "labels": dict(key[1]), "value": value}
                                    for key, value in sorted(product.items())]
        health = docker("exec", exporter, "curl", "--fail", "--silent", "http://127.0.0.1:9841/healthz")
        if health != "ok":
            raise ValueError(f"Unhealthy production exporter: {health}")
        config = json.loads(docker("inspect", exporter))[0]
        if config["Config"]["User"] != "65532:65532" or not config["HostConfig"]["ReadonlyRootfs"]:
            raise ValueError("Runtime test lost its non-root/read-only constraints")
        docker("cp", exporter + ":/usr/local/share/autoware-exporter/runtime.json", str(output / "runtime.json"))
        manifest = json.loads((output / "runtime.json").read_text())
        if manifest["ros_distro"] != args.ros_distro or not {item["Type"] for item in plan} <= set(manifest["message_types"]):
            raise ValueError("Prepared runtime does not cover the requested ROS distribution and message types")
        host = docker("info", "--format", "{{.Architecture}}")
        native = {"aarch64": "arm64", "x86_64": "amd64"}.get(host, host)
        result["execution"] = "native" if native == args.arch else "emulated"
        result["health"] = health
        result["passed"] = True
    except BaseException as error:
        result["error"] = str(error)
        raise
    finally:
        for name in created:
            logs = subprocess.run(["docker", "logs", name], capture_output=True, text=True)
            (output / ("publisher.log" if name == publisher else "exporter.log")).write_text(logs.stdout + logs.stderr)
        (output / "result.json").write_text(json.dumps(result, indent=2, allow_nan=False) + "\n")
        if created:
            subprocess.run(["docker", "rm", "-f", *reversed(created)], check=False, stdout=subprocess.DEVNULL)
    print(json.dumps({key: value for key, value in result.items() if key not in ("product_values", "upstream")}, indent=2))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ros-distro", choices=("humble", "jazzy"), required=True)
    parser.add_argument("--branch", choices=("0.51", "0.52", "0.53"), required=True)
    parser.add_argument("--arch", choices=("amd64", "arm64"), required=True)
    parser.add_argument("--runtime-image", required=True)
    parser.add_argument("--fixture-image", required=True)
    parser.add_argument("--test-binary", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    check(parser.parse_args())
