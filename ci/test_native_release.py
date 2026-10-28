import hashlib
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest
from unittest.mock import patch

import publish_release
import release


class NativeReleaseTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="autoware-native-release-")
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.metadata = {
            "version": "0.1.0", "revision": "a" * 40, "snapshot": False,
            "go_version": "go1.27.3", "source_date_epoch": 1700000000,
            "platforms": [f"{system}/{arch}/{ros}" for system, arch, ros in release.PLATFORMS],
        }

    def make_parts(self):
        source = self.root / "parts"
        source.mkdir()
        for system, arch, ros in release.PLATFORMS:
            metadata = release.target_metadata(self.metadata, ros, arch, release.image_for(ros))
            with patch.object(release, "build_binary", return_value=f"native {ros}/{arch}".encode()):
                part = self.root / f"{ros}-{arch}"
                release.build(part, metadata, "unpublished-preview")
            for artifact in part.iterdir():
                artifact.rename(source / artifact.name)
        return source

    def test_archive_is_stable_and_preserves_executable_mode(self):
        members = [("README.md", b"usage\n", 0o644), ("autoware-exporter", b"native binary", 0o755)]
        first, second = self.root / "a.tar.gz", self.root / "b.tar.gz"
        release.archive(first, members, 1700000000)
        release.archive(second, list(reversed(members)), 1700000000)
        self.assertEqual(first.read_bytes(), second.read_bytes())
        with tarfile.open(first) as archive:
            binary = archive.getmember("autoware-exporter")
            self.assertEqual(binary.mode, 0o755)
            self.assertEqual(binary.mtime, 1700000000)
            self.assertEqual(binary.uid, 0)
            self.assertEqual(archive.extractfile(binary).read(), b"native binary")

    def test_native_archive_records_its_ros_library_baseline(self):
        metadata = release.target_metadata(self.metadata, "humble", "arm64", release.image_for("humble"))
        with patch.object(release, "build_binary", return_value=b"native ROS binary"):
            release.build(self.root / "native", metadata, "unpublished-preview")
        path = self.root / "native/autoware-exporter_0.1.0_linux_arm64_humble.tar.gz"
        with tarfile.open(path) as archive:
            recorded = json.load(archive.extractfile("build-metadata.json"))
            self.assertEqual(recorded["target"]["ros_distro"], "humble")
            self.assertEqual(recorded["target"]["ros_image"], release.image_for("humble"))
            self.assertEqual(archive.extractfile("autoware-exporter").read(), b"native ROS binary")

    def test_release_rejects_an_unpinned_ros_image(self):
        with self.assertRaisesRegex(ValueError, "pinned by digest"):
            release.target_metadata(self.metadata, "jazzy", "amd64", "custom-autoware:latest")

    def test_preview_accepts_a_matching_local_ros_image(self):
        metadata = dict(self.metadata, snapshot=True)
        result = release.target_metadata(metadata, "humble", "arm64", "custom-autoware:local")
        self.assertEqual(result["target"]["ros_image"], "custom-autoware:local")

    def test_container_check_waits_for_an_initialized_ros_context(self):
        pending = subprocess.CompletedProcess([], 0, stdout='autoware_ros_context_up{vehicle="release_check"} 0\n')
        ready = subprocess.CompletedProcess([], 0, stdout='autoware_ros_context_up{vehicle="release_check"} 1\n')
        with patch.object(release, "command", return_value="container-id"):
            with patch.object(release.subprocess, "run", side_effect=[pending, ready, subprocess.CompletedProcess([], 0)]) as run:
                with patch.object(release.time, "sleep"):
                    release.verify_container("native-image")
        self.assertEqual(run.call_args_list[-1].args[0], ["docker", "rm", "-f", "container-id"])

    def test_container_check_rejects_http_health_without_ros_and_cleans_up(self):
        pending = subprocess.CompletedProcess([], 0, stdout='autoware_ros_context_up{vehicle="release_check"} 0\n')
        logs = subprocess.CompletedProcess([], 0, stdout="", stderr="DDS configuration is unreadable")
        with patch.object(release, "command", return_value="container-id"):
            with patch.object(release.subprocess, "run", side_effect=[pending] * 40 + [logs, subprocess.CompletedProcess([], 0)]) as run:
                with patch.object(release.time, "sleep"):
                    with self.assertRaisesRegex(ValueError, "DDS configuration is unreadable"):
                        release.verify_container("native-image")
        self.assertEqual(run.call_args_list[-1].args[0], ["docker", "rm", "-f", "container-id"])

    def test_assembly_produces_a_complete_verified_release(self):
        source = self.make_parts()
        destination = self.root / "release"
        release.assemble(source, destination)
        artifacts = publish_release.load_artifacts(destination, "v0.1.0", self.metadata["revision"])
        self.assertEqual(len(artifacts), 7)
        metadata = json.loads(artifacts["build-metadata.json"].read_text())
        self.assertEqual(len(metadata["builds"]), 4)
        for path in destination.iterdir():
            if path.name != "SHA256SUMS":
                self.assertIn(hashlib.sha256(path.read_bytes()).hexdigest(), artifacts["SHA256SUMS"].read_text())

    def test_assembly_rejects_missing_platforms(self):
        source = self.make_parts()
        (source / "metadata_humble_arm64.json").unlink()
        with self.assertRaisesRegex(ValueError, "all four"):
            release.assemble(source, self.root / "release")

    def test_assembly_rejects_a_missing_native_archive(self):
        source = self.make_parts()
        (source / "autoware-exporter_0.1.0_linux_arm64_humble.tar.gz").unlink()
        with self.assertRaisesRegex(ValueError, "archive is missing"):
            release.assemble(source, self.root / "release")

    def test_assembly_rejects_mixed_source_revisions(self):
        source = self.make_parts()
        path = source / "metadata_jazzy_arm64.json"
        metadata = json.loads(path.read_text())
        metadata["revision"] = "b" * 40
        path.write_text(json.dumps(metadata))
        with self.assertRaisesRegex(ValueError, "same source release"):
            release.assemble(source, self.root / "release")

    def test_release_notes_identify_both_ros_images(self):
        image = "ghcr.io/adanomy/autoware-exporter"
        jazzy, humble = "sha256:" + "a" * 64, "sha256:" + "b" * 64
        body = publish_release.release_body("Release notes", image, "v0.1.0", jazzy, humble)
        self.assertIn(image + ":v0.1.0-humble", body)
        self.assertIn(image + "@" + jazzy, body)
        self.assertIn(image + "@" + humble, body)
        self.assertIn("Release notes", body)


if __name__ == "__main__":
    unittest.main()
