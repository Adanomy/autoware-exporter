import importlib.util
import os
from pathlib import Path
import subprocess
import unittest


SOURCE = Path(__file__).resolve().parent.parent / "scripts/prepare-runtime.py"
SPEC = importlib.util.spec_from_file_location("prepare_runtime", SOURCE)
runtime = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(runtime)


class RuntimeEnvironmentTests(unittest.TestCase):
    def source_environment(self, overrides):
        defaults = runtime.runtime_defaults({
            "CYCLONEDDS_URI": "file:///home/aw/cyclonedds.xml",
            "RMW_IMPLEMENTATION": "rmw_cyclonedds_cpp",
            "FASTRTPS_DEFAULT_PROFILES_FILE": "/build-image/fastdds.xml",
        })
        script = runtime.runtime_environment_script({"ROS_DISTRO": "jazzy"}, defaults)
        script += '\nprintf "%s\\0" "$CYCLONEDDS_URI" "$RMW_IMPLEMENTATION" "$FASTRTPS_DEFAULT_PROFILES_FILE"'
        environment = dict(os.environ)
        for name in defaults:
            environment.pop(name, None)
        environment.update(overrides)
        result = subprocess.check_output(["bash", "-eu", "-c", script], env=environment)
        return result.decode().split("\0")[:-1]

    def test_build_image_cyclone_profile_is_disabled_by_default(self):
        uri, middleware, _ = self.source_environment({})
        self.assertEqual(uri, "")
        self.assertEqual(middleware, "rmw_cyclonedds_cpp")

    def test_explicit_empty_cyclone_uri_is_preserved(self):
        uri, _, _ = self.source_environment({"CYCLONEDDS_URI": ""})
        self.assertEqual(uri, "")

    def test_explicit_cyclone_profile_is_preserved(self):
        for value in ("file:///etc/autoware-exporter/cyclonedds.xml", '<CycloneDDS><Domain id="any"/></CycloneDDS>'):
            with self.subTest(uri=value):
                uri, _, _ = self.source_environment({"CYCLONEDDS_URI": value})
                self.assertEqual(uri, value)

    def test_empty_overrides_are_distinct_from_unset_defaults(self):
        _, middleware, profile = self.source_environment({
            "RMW_IMPLEMENTATION": "", "FASTRTPS_DEFAULT_PROFILES_FILE": "",
        })
        self.assertEqual(middleware, "")
        self.assertEqual(profile, "")

    def test_explicit_middleware_and_fastdds_profile_are_preserved(self):
        _, middleware, profile = self.source_environment({
            "RMW_IMPLEMENTATION": "rmw_fastrtps_cpp",
            "FASTRTPS_DEFAULT_PROFILES_FILE": "/deployment/fastdds.xml",
        })
        self.assertEqual(middleware, "rmw_fastrtps_cpp")
        self.assertEqual(profile, "/deployment/fastdds.xml")


if __name__ == "__main__":
    unittest.main()
