#!/usr/bin/env python3
"""Build CI-only interface packages from unchanged, pinned upstream .msg files."""

import argparse
from concurrent.futures import ThreadPoolExecutor
import io
import json
from pathlib import Path, PurePosixPath
import re
import tarfile
import urllib.error
import urllib.request

PRIMITIVES = set("bool byte char float32 float64 int8 uint8 int16 uint16 int32 uint32 int64 uint64 string wstring".split())
ANALYSIS_MESSAGES = ("Error", "ErrorStamped", "DrivingMonitorStamped", "FloatStamped")


def download(url):
    request = urllib.request.Request(url, headers={"User-Agent": "autoware-exporter-compatibility"})
    for attempt in range(3):
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                return response.read()
        except (urllib.error.URLError, TimeoutError):
            if attempt == 2:
                raise


def repository_messages(item):
    repository, revision = item
    commit = revision["commit"]
    if not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise ValueError(f"Expected a pinned commit for {repository}")
    messages = {}
    if repository.endswith("/autoware_universe"):
        package = "autoware_control_performance_analysis"
        for name in ANALYSIS_MESSAGES:
            url = f"https://raw.githubusercontent.com/{repository}/{commit}/control/{package}/msg/{name}.msg"
            messages[f"{package}/{name}"] = download(url).decode()
    else:
        archive = download(f"https://codeload.github.com/{repository}/tar.gz/{commit}")
        with tarfile.open(fileobj=io.BytesIO(archive), mode="r:gz") as source:
            for member in source.getmembers():
                path = PurePosixPath(member.name)
                if member.isfile() and path.suffix == ".msg" and path.parent.name == "msg":
                    package = path.parts[1]
                    messages[f"{package}/{path.stem}"] = source.extractfile(member).read().decode()
    print(f"{repository}@{revision['tag']} ({commit}): {len(messages)} message definitions", flush=True)
    return messages


def dependencies(package, text):
    for line in text.splitlines():
        tokens = line.split("#", 1)[0].split()
        if len(tokens) < 2 or "=" in tokens[1]:
            continue
        field_type = tokens[0].split("[", 1)[0].split("<=", 1)[0]
        if field_type not in PRIMITIVES:
            yield field_type if "/" in field_type else f"{package}/{field_type}"


def prepare(branch, output, source_file):
    releases = json.loads(Path(__file__).with_name("releases.json").read_text())
    release = releases[branch]
    definitions = {}
    with ThreadPoolExecutor(max_workers=5) as pool:
        for messages in pool.map(repository_messages, release["repositories"].items()):
            definitions.update(messages)
    roots = set(re.findall(r'"([a-z][a-z0-9_]*)/msg/([A-Za-z][A-Za-z0-9_]*)"', source_file.read_text()))
    if not roots:
        raise ValueError("No exporter message types found")
    selected = {}

    def include(name):
        if name in selected:
            return
        package = name.split("/", 1)[0]
        if name not in definitions:
            if package.startswith(("autoware_", "tier4_")):
                raise ValueError(f"{name} missing from Universe {release['universe']} interface sources")
            return  # Standard ROS interfaces come from the selected ROS distribution.
        selected[name] = definitions[name]
        for dependency in dependencies(package, definitions[name]):
            include(dependency)

    for package, name in sorted(roots):
        include(f"{package}/{name}")
    packages = sorted({name.split("/", 1)[0] for name in selected})
    for package in packages:
        directory = output / "src" / package
        directory.mkdir(parents=True)
        files, required = [], set()
        for name, text in sorted(selected.items()):
            prefix, message = name.split("/")
            if prefix != package:
                continue
            relative = f"msg/{message}.msg"
            (directory / "msg").mkdir(exist_ok=True)
            (directory / relative).write_text(text)
            files.append(relative)
            required.update(item.split("/", 1)[0] for item in dependencies(package, text))
        required.discard(package)
        required = sorted(required)
        cmake = ["cmake_minimum_required(VERSION 3.14)", f"project({package})",
                 "find_package(ament_cmake REQUIRED)", "find_package(rosidl_default_generators REQUIRED)"]
        cmake += [f"find_package({name} REQUIRED)" for name in required]
        cmake += ["rosidl_generate_interfaces(${PROJECT_NAME}", *[f'  "{name}"' for name in files]]
        if required:
            cmake.append("  DEPENDENCIES " + " ".join(required))
        cmake += [")", "ament_export_dependencies(rosidl_default_runtime)", "ament_package()"]
        (directory / "CMakeLists.txt").write_text("\n".join(cmake) + "\n")
        manifest = [f'<package format="3"><name>{package}</name><version>0.0.0</version>',
                    '<description>CI-only build of pinned upstream message definitions</description>',
                    '<maintainer email="noreply@example.com">Autoware Exporter</maintainer>',
                    '<license>Apache-2.0</license><buildtool_depend>ament_cmake</buildtool_depend>',
                    '<buildtool_depend>rosidl_default_generators</buildtool_depend>',
                    '<exec_depend>rosidl_default_runtime</exec_depend>',
                    *[f"<depend>{name}</depend>" for name in required],
                    '<member_of_group>rosidl_interface_packages</member_of_group>',
                    '<export><build_type>ament_cmake</build_type></export></package>']
        (directory / "package.xml").write_text("\n".join(manifest) + "\n")
    (output / "upstream-revisions.json").write_text(json.dumps(release, indent=2) + "\n")
    print(f"Universe {release['universe']} / Core {release['core']}: {len(roots)} source types, {len(selected)} upstream messages in {len(packages)} packages", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--branch", required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--sources", type=Path, required=True)
    args = parser.parse_args()
    prepare(args.branch, args.output, args.sources)
