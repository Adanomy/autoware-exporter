#!/usr/bin/env python3
"""Select a runtime filesystem from the exact ROS installation used to build.

ELF dependencies alone miss dlopen-loaded RMW, logging and message type support.
Keep those explicit roots, close over their ELF dependencies, and preserve the
ament resource indexes used by ROS to discover them. No packages are rebuilt.
"""

import argparse
import ctypes
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess


def runtime_defaults(environ):
    defaults = {name: environ[name] for name in (
        "RMW_IMPLEMENTATION", "FASTRTPS_DEFAULT_PROFILES_FILE",
        "FASTDDS_DEFAULT_PROFILES_FILE", "ROS_AUTOMATIC_DISCOVERY_RANGE",
        "ROS_LOCALHOST_ONLY", "ROS_STATIC_PEERS",
    ) if environ.get(name)}
    # The production Dockerfile deliberately disables the build image's Cyclone
    # profile. It may require host-specific interfaces or large socket buffers.
    defaults["CYCLONEDDS_URI"] = ""
    return defaults


def runtime_environment_script(environment, defaults):
    return (
        "".join(f"export {name}={shlex.quote(value)}\n" for name, value in environment.items()) +
        "".join(f'if [[ -z ${{{name}+x}} ]]; then export {name}={shlex.quote(value)}; fi\n'
                for name, value in defaults.items()))


def prepare(binary, sources, output):
    if output.exists():
        raise ValueError(f"Runtime destination already exists: {output}")
    output.mkdir(parents=True)
    # Preserve Ubuntu's merged-/usr aliases before copying leaf symlinks.
    # Otherwise /lib/.../libfoo.so -> libfoo.so.1 could point beside a target
    # copied under /usr/lib instead, leaving a dangling runtime symlink.
    for name in ("bin", "sbin", "lib", "lib64"):
        original = Path("/") / name
        if original.is_symlink():
            (output / str(original.resolve()).lstrip("/")).mkdir(parents=True, exist_ok=True)
            (output / name).symlink_to(os.readlink(original))
    copied, libraries, packages = set(), set(), set()
    symbols = {}
    library_dirs = [Path(item) for item in os.environ["LD_LIBRARY_PATH"].split(":") if item]
    prefixes = [Path(item) for item in os.environ["AMENT_PREFIX_PATH"].split(":") if item]
    message_types = sorted(set(re.findall(r'"([a-z][a-z0-9_]*/msg/[A-Za-z][A-Za-z0-9_]*)"', sources.read_text())))
    if not message_types:
        raise ValueError("No production message types found")

    def copy(source, destination=None):
        source = Path(source)
        destination = output / str(destination or source).lstrip("/")
        if str(destination) in copied:
            return
        copied.add(str(destination))
        destination.parent.mkdir(parents=True, exist_ok=True)
        if source.is_symlink():
            destination.symlink_to(os.readlink(source))
            copy(source.resolve())
        else:
            shutil.copy2(source, destination)

    def find(name):
        for directory in library_dirs:
            candidate = directory / name
            if candidate.is_file():
                return candidate
        return None

    def add_elf(source, destination=None):
        source = Path(source)
        copy(source, destination)
        resolved = source.resolve()
        if resolved in libraries:
            return
        libraries.add(resolved)
        result = subprocess.run(["ldd", str(source)], capture_output=True, text=True, check=True)
        if "not found" in result.stdout:
            raise ValueError(f"Unresolved dependencies of {source}: {result.stdout}")
        for line in result.stdout.splitlines():
            match = re.search(r"(?:=>\s+)?(/[^\s]+)\s+\(", line)
            if match:
                add_elf(Path(match[1]))
        match = re.match(r"lib(.+)__rosidl_", source.name)
        if match and match[1] not in packages:
            packages.add(match[1])
            # Generic type support loads the selected middleware's support at
            # runtime. Include both C and C++ implementations when installed,
            # including those of nested message packages discovered by ldd.
            for directory in library_dirs:
                for support in sorted(directory.glob(f"lib{match[1]}__rosidl_*.so")):
                    if "generator_py" not in support.name:
                        add_elf(support)

    add_elf(binary, "/usr/local/bin/autoware-exporter")
    for tool in ("bash", "sh", "curl", "env"):
        executable = shutil.which(tool)
        if not executable:
            raise ValueError(f"Required runtime tool is unavailable: {tool}")
        add_elf(executable)
    # The entrypoint and existing Compose healthchecks use these stable paths.
    copy(shutil.which("bash"), "/bin/bash")
    copy(shutil.which("curl"), "/usr/bin/curl")
    copy(shutil.which("env"), "/usr/bin/env")
    for directory in library_dirs:
        for pattern in ("librmw*.so", "librcl_logging_*.so"):
            for plugin in sorted(directory.glob(pattern)):
                add_elf(plugin)
    missing_types = []
    available_types = []
    for message in message_types:
        package = message.split("/")[0]
        support = find(f"lib{package}__rosidl_typesupport_c.so")
        introspection = find(f"lib{package}__rosidl_typesupport_introspection_c.so")
        if support is None or introspection is None:
            # Preserve the full image's optional-package availability exactly.
            missing_types.append(message)
            continue
        name = message.split("/")[2]
        supported = True
        for kind, library in (("c", support), ("introspection_c", introspection)):
            if library not in symbols:
                symbols[library] = {line.split()[-1] for line in subprocess.check_output(
                    ["nm", "-D", "--defined-only", str(library)], text=True).splitlines() if line.split()}
            supported = supported and f"rosidl_typesupport_{kind}__get_message_type_support_handle__{package}__msg__{name}" in symbols[library]
        if not supported:
            # Older interface packages can exist without a newer optional type.
            missing_types.append(message)
            continue
        add_elf(support)
        add_elf(introspection)
        available_types.append(message)

    # glibc loads NSS implementations outside the ELF dependency graph.
    cache = subprocess.check_output(["ldconfig", "-p"], text=True)
    for line in cache.splitlines():
        if re.search(r"libnss_(dns|files)\.so\.2\s", line):
            add_elf(Path(line.split("=>", 1)[1].strip()))
    for filename in ("/etc/os-release", "/etc/ssl/certs/ca-certificates.crt", "/etc/localtime", "/usr/share/zoneinfo/UTC"):
        if Path(filename).is_file():
            copy(filename)
    locale = Path("/usr/lib/locale/C.utf8")
    if locale.is_dir():
        for filename in sorted(locale.rglob("*")):
            if filename.is_file():
                copy(filename)
    # Keep only runtime resource indexes and package metadata, not setup hooks
    # that may reference Python, build tools or removed Autoware components.
    runtime_prefixes = []
    for prefix in prefixes:
        if not (output / str(prefix).lstrip("/")).exists():
            continue
        runtime_prefixes.append(str(prefix))
        index = prefix / "share/ament_index/resource_index"
        if index.is_dir():
            for resource in sorted(index.rglob("*")):
                if resource.is_file():
                    copy(resource)
        for package in sorted(packages):
            metadata = prefix / "share" / package
            for pattern in ("package.xml", "*LICENSE*", "*COPYING*", "*copyright*"):
                for filename in sorted(metadata.glob(pattern)):
                    if filename.is_file():
                        copy(filename)
    # Retain distribution license notices for the selected system libraries.
    owners = set()
    queries = {str(library) for library in libraries}
    queries.update(filename.removeprefix("/usr") for filename in list(queries) if filename.startswith(("/usr/lib/", "/usr/bin/")))
    result = subprocess.run(["dpkg-query", "-S", *sorted(queries)], capture_output=True, text=True)
    for line in result.stdout.splitlines():
        if ": /" in line:
            owners.update(line.split(": /", 1)[0].split(", "))
    for owner in sorted(owners):
        copyright_file = Path("/usr/share/doc") / owner.split(":")[0] / "copyright"
        if copyright_file.is_file():
            copy(copyright_file)

    environment = {
        "ROS_DISTRO": os.environ["ROS_DISTRO"], "ROS_VERSION": "2",
        "AMENT_PREFIX_PATH": ":".join(runtime_prefixes),
        "LD_LIBRARY_PATH": ":".join(str(directory) for directory in library_dirs if (output / str(directory).lstrip("/")).is_dir()),
    }
    defaults = runtime_defaults(os.environ)
    # Autoware setup hooks can select DDS and its configuration. Preserve those
    # defaults while allowing the deployment's environment to override them.
    for name in ("FASTRTPS_DEFAULT_PROFILES_FILE", "FASTDDS_DEFAULT_PROFILES_FILE"):
        if defaults.get(name):
            copy(Path(defaults[name]))
    share = output / "usr/local/share/autoware-exporter"
    share.mkdir(parents=True)
    (share / "runtime-env.sh").write_text(runtime_environment_script(environment, defaults))
    manifest = {"ros_distro": environment["ROS_DISTRO"], "message_types": available_types,
                "unavailable_message_types": missing_types,
                "libraries": sorted("/usr/local/bin/autoware-exporter" if item == binary.resolve() else str(item) for item in libraries),
                "environment": environment, "defaults": defaults}
    (share / "runtime.json").write_text(json.dumps(manifest, indent=2) + "\n")
    etc = output / "etc"
    etc.mkdir(exist_ok=True)
    (etc / "passwd").write_text("root:x:0:0:root:/root:/bin/bash\nexporter:x:65532:65532:Exporter:/tmp:/bin/bash\n")
    (etc / "group").write_text("root:x:0:\nexporter:x:65532:\n")
    (etc / "nsswitch.conf").write_text("passwd: files\ngroup: files\nhosts: files dns\n")
    (output / "tmp").mkdir(mode=0o1777)
    (output / "tmp").chmod(0o1777)
    (output / "dev").mkdir()
    (output / "proc").mkdir()

    # Check all selected dlopen roots/getters inside the trimmed filesystem,
    # before testing real subscriptions in the running final container.
    child = os.fork()
    if child == 0:
        try:
            os.chroot(output)
            os.chdir("/")
            for message in available_types:
                package, _, name = message.split("/")
                for kind in ("c", "introspection_c"):
                    library = ctypes.CDLL(f"lib{package}__rosidl_typesupport_{kind}.so")
                    getter = getattr(library, f"rosidl_typesupport_{kind}__get_message_type_support_handle__{package}__msg__{name}")
                    getter.restype = ctypes.c_void_p
                    if not getter():
                        raise ValueError(f"Empty type support for {message}")
        except BaseException as error:
            print(f"Runtime validation failed: {error}", flush=True)
            os._exit(1)
        os._exit(0)
    _, status = os.waitpid(child, 0)
    if status != 0:
        raise ValueError("Trimmed runtime cannot load all original message types")
    subprocess.run(["chroot", str(output), "/bin/bash", "-ec",
                    "source /usr/local/share/autoware-exporter/runtime-env.sh; /usr/local/bin/autoware-exporter --version; /usr/bin/curl --version"], check=True)
    print(f"Runtime: {len(libraries)} ELF files, {len(available_types)}/{len(message_types)} message types; unavailable in source: {missing_types}", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--sources", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    prepare(args.binary, args.sources, args.output)
