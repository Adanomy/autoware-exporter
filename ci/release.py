#!/usr/bin/env python3
"""Build reproducible native ROS release archives with Docker."""

import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time


ROOT = Path(__file__).resolve().parent.parent
PLATFORMS = (
    ("linux", "amd64", "humble"),
    ("linux", "arm64", "humble"),
    ("linux", "amd64", "jazzy"),
    ("linux", "arm64", "jazzy"),
)
RELEASE_TAG = re.compile(r"v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)", re.ASCII)


def command(*args, env=None):
    return subprocess.check_output(args, cwd=ROOT, env=env, text=True).strip()


def release_version(revision):
    ref_type = os.environ.get("GITHUB_REF_TYPE")
    if ref_type:
        if ref_type != "tag":
            raise ValueError("release builds require a Git tag, not a branch")
        tag = os.environ.get("GITHUB_REF_NAME", "")
    else:
        tags = [
            tag for tag in command("git", "tag", "--points-at", "HEAD").splitlines()
            if RELEASE_TAG.fullmatch(tag)
        ]
        if len(tags) != 1:
            raise ValueError("release builds require exactly one vMAJOR.MINOR.PATCH tag at HEAD")
        tag = tags[0]
    if not RELEASE_TAG.fullmatch(tag):
        raise ValueError(f"invalid release tag {tag!r}; expected vMAJOR.MINOR.PATCH")
    if command("git", "rev-parse", f"refs/tags/{tag}^{{commit}}") != revision:
        raise ValueError(f"release tag {tag!r} must point to HEAD")
    return tag.removeprefix("v")


def release_metadata(snapshot):
    match = re.search(r"^go (\S+)$", (ROOT / "go.mod").read_text(), re.MULTILINE)
    if not match:
        raise ValueError("go.mod has no Go version")
    toolchain = "go" + match[1]
    env = dict(os.environ, GOTOOLCHAIN=toolchain, GOWORK="off", GOENV="off", GOFLAGS="")
    if command("go", "env", "GOVERSION", env=env) != toolchain:
        raise ValueError(f"release builds require exactly {toolchain}")

    revision = "uncommitted"
    epoch = 0
    head = subprocess.run(
        ["git", "rev-parse", "--verify", "HEAD"], cwd=ROOT,
        stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True,
    )
    if head.returncode == 0:
        revision = head.stdout.strip()
        epoch = int(command("git", "show", "-s", "--format=%ct", "HEAD"))
        dirty = bool(command("git", "status", "--porcelain", "--untracked-files=normal"))
        if dirty:
            if not snapshot:
                raise ValueError("release builds require a clean working tree; use --snapshot for a preview")
            revision += "-dirty"
    elif not snapshot:
        raise ValueError("release builds require a committed, tagged source; use --snapshot for a preview")

    return {
        "version": "dev" if snapshot else release_version(revision),
        "revision": revision,
        "go_version": toolchain,
        "source_date_epoch": epoch,
        "snapshot": snapshot,
        "platforms": [f"{goos}/{goarch}/{ros}" for goos, goarch, ros in PLATFORMS],
    }


def archive(path, members, epoch):
    with path.open("wb") as raw:
        with gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=epoch, compresslevel=9) as compressed:
            with tarfile.open(fileobj=compressed, mode="w", format=tarfile.USTAR_FORMAT) as output:
                for name, content, mode in sorted(members):
                    info = tarfile.TarInfo(name)
                    info.size, info.mode, info.mtime = len(content), mode, epoch
                    info.uid = info.gid = 0
                    info.uname = info.gname = ""
                    output.addfile(info, io.BytesIO(content))


def checksums(directory):
    return {
        path.name: hashlib.sha256(path.read_bytes()).hexdigest()
        for path in sorted(directory.iterdir()) if path.name != "SHA256SUMS"
    }


def image_for(ros):
    return json.loads((ROOT / "ci/ros-images.json").read_text())[ros]


def target_metadata(metadata, ros, arch, image):
    if ("linux", arch, ros) not in PLATFORMS:
        raise ValueError("release target must be Linux amd64/arm64 with Humble or Jazzy")
    if not metadata["snapshot"] and not re.search(r"@sha256:[0-9a-f]{64}$", image):
        raise ValueError("release ROS_IMAGE must be pinned by digest")
    return dict(metadata, target={"os": "linux", "arch": arch, "ros_distro": ros, "ros_image": image})


def verify_container(image_name):
    # Exercise the shipping image's default middleware/configuration so a baked
    # deployment-specific DDS profile cannot be hidden by the smoke test.
    container = command(
        "docker", "run", "-d", "--network", "none", "--read-only", "--cap-drop", "ALL",
        "--security-opt", "no-new-privileges:true", "--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=64m",
        "-e", "ROS_LOCALHOST_ONLY=1", image_name, "--vehicle=release_check:200",
    )
    try:
        for _ in range(40):
            result = subprocess.run(
                ["docker", "exec", container, "curl", "--fail", "--silent", "--max-time", "2",
                 "http://127.0.0.1:9841/metrics"], capture_output=True, text=True,
            )
            if result.returncode == 0 and 'autoware_ros_context_up{vehicle="release_check"} 1' in result.stdout.splitlines():
                return
            time.sleep(0.5)
        logs = subprocess.run(["docker", "logs", "--tail", "30", container], capture_output=True, text=True)
        raise ValueError(f"native container did not initialize ROS: {logs.stdout}{logs.stderr}")
    finally:
        subprocess.run(["docker", "rm", "-f", container], stdout=subprocess.DEVNULL, check=True)


def build_binary(metadata, image_name, fresh_cache=False):
    target = metadata["target"]
    args = ["docker", "buildx", "build", "--load", "--platform", f"linux/{target['arch']}",
            "--build-arg", f"ROS_IMAGE={target['ros_image']}",
            "--build-arg", f"ROS_DISTRO={target['ros_distro']}",
            "--build-arg", f"VERSION={metadata['version']}",
            "--build-arg", f"REVISION={metadata['revision']}",
            "--build-arg", f"SOURCE_DATE_EPOCH={metadata['source_date_epoch']}", "-t", image_name]
    if fresh_cache:
        args += ["--no-cache-filter", "build"]
    subprocess.run(args + [str(ROOT)], cwd=ROOT, check=True)
    environment = command("docker", "image", "inspect", image_name, "--format", "{{range .Config.Env}}{{println .}}{{end}}")
    if f"ROS_DISTRO={target['ros_distro']}" not in environment.splitlines():
        raise ValueError("built image ROS_DISTRO does not match the release target")
    version = command("docker", "run", "--rm", "--network", "none", image_name, "--version")
    expected = f"autoware-exporter {metadata['version']} revision={metadata['revision']} go={metadata['go_version']} native_ros2=true"
    if version != expected:
        raise ValueError(f"native release binary reports unexpected build metadata: {version}")
    verify_container(image_name)
    container = command("docker", "create", image_name)
    try:
        with tempfile.TemporaryDirectory(prefix="autoware-exporter-binary-") as temporary:
            binary = Path(temporary) / "autoware-exporter"
            subprocess.run(["docker", "cp", f"{container}:/usr/local/bin/autoware-exporter", str(binary)], check=True)
            return binary.read_bytes()
    finally:
        subprocess.run(["docker", "rm", container], stdout=subprocess.DEVNULL, check=True)


def build(directory, metadata, image_name, fresh_cache=False):
    directory.mkdir()
    binary = build_binary(metadata, image_name, fresh_cache)
    target = metadata["target"]
    shared = [(name, (ROOT / name).read_bytes(), 0o644)
              for name in ("LICENSE", "README.md", "docs/building.md")]
    shared += [("build-metadata.json", (json.dumps(metadata, indent=2) + "\n").encode(), 0o644)]
    stem = f"autoware-exporter_{metadata['version']}_linux_{target['arch']}_{target['ros_distro']}"
    archive(directory / (stem + ".tar.gz"), shared + [("autoware-exporter", binary, 0o755)], metadata["source_date_epoch"])
    (directory / f"metadata_{target['ros_distro']}_{target['arch']}.json").write_text(json.dumps(metadata, indent=2) + "\n")


def assemble(source, destination):
    parts = sorted(source.glob("metadata_*.json"))
    if len(parts) != len(PLATFORMS):
        raise ValueError("release assembly requires all four native platform metadata files")
    common, builds, seen = None, [], set()
    artifacts = []
    for path in parts:
        metadata = json.loads(path.read_text())
        target = metadata.pop("target")
        key = (target["os"], target["arch"], target["ros_distro"])
        if key not in PLATFORMS or key in seen:
            raise ValueError("duplicate or unsupported native release target")
        seen.add(key)
        if common is None:
            common = metadata
        elif metadata != common:
            raise ValueError("native build metadata does not describe the same source release")
        name = f"autoware-exporter_{metadata['version']}_linux_{target['arch']}_{target['ros_distro']}.tar.gz"
        archive_path = source / name
        if archive_path.is_symlink() or not archive_path.is_file():
            raise ValueError(f"native release archive is missing: {name}")
        artifacts.append(archive_path)
        builds.append(target)
    destination.mkdir(parents=True, exist_ok=True)
    for path in artifacts:
        shutil.copyfile(path, destination / path.name)
    common["builds"] = builds
    (destination / "build-metadata.json").write_text(json.dumps(common, indent=2) + "\n")
    (destination / "autoware-exporter-dashboard.json").write_bytes((ROOT / "grafana/autoware-exporter.json").read_bytes())
    manifest = checksums(destination)
    (destination / "SHA256SUMS").write_text("".join(f"{digest}  {name}\n" for name, digest in manifest.items()))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--snapshot", action="store_true", help="build an unpublished dev preview without requiring a tag")
    parser.add_argument("--verify-reproducible", action="store_true", help="rebuild with a fresh build stage and compare artifact digests")
    parser.add_argument("--ros-distro", choices=("humble", "jazzy"), default="jazzy")
    parser.add_argument("--arch", choices=("amd64", "arm64"), default="amd64")
    parser.add_argument("--ros-image", help="override the pinned ROS/Autoware image")
    parser.add_argument("--container-image", help="local image tag for the native build")
    parser.add_argument("--assemble", type=Path, metavar="DIRECTORY", help="assemble all four native builds")
    parser.add_argument("--output", type=Path, help="empty output directory")
    args = parser.parse_args()
    destination = (args.output or ROOT / "dist" / ("preview" if args.snapshot else "release")).resolve()
    if destination.exists() and (not destination.is_dir() or any(destination.iterdir())):
        raise ValueError(f"output must be an empty directory: {destination}")
    if args.assemble:
        assemble(args.assemble, destination)
        return
    metadata = target_metadata(release_metadata(args.snapshot), args.ros_distro, args.arch, args.ros_image or image_for(args.ros_distro))
    image_name = args.container_image or f"autoware-exporter:preview-{args.ros_distro}-{args.arch}"
    with tempfile.TemporaryDirectory(prefix="autoware-exporter-release-") as temporary:
        first = Path(temporary) / "first"
        build(first, metadata, image_name)
        if args.verify_reproducible:
            second = Path(temporary) / "second"
            build(second, metadata, image_name, fresh_cache=True)
            if checksums(first) != checksums(second):
                raise ValueError("reproducibility check failed: artifact digests differ")
            print("Reproducibility verified for native release artifacts", flush=True)
        destination.mkdir(parents=True, exist_ok=True)
        for artifact in first.iterdir():
            shutil.copyfile(artifact, destination / artifact.name)
    print(f"{'Unpublished preview' if args.snapshot else 'Release'} artifacts: {destination}")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"release: {error}", file=sys.stderr)
        raise SystemExit(1)
