# SPDX-License-Identifier: GPL-2.0

import json
import os
import subprocess
import unittest

from tests.integration.test_utils import RunSubprocessMixin
from tests.integration.test_utils import podman_compose_path
from tests.integration.test_utils import test_path


class TestPodmanCompose(unittest.TestCase, RunSubprocessMixin):
    def test_selinux(self) -> None:
        # test if when using volumes type:bind with selinux:z option, container ackquires a
        # respective host:source:z mapping in CreateCommand list
        compose_path = os.path.join(test_path(), "selinux", "docker-compose.yml")
        host_path = os.path.join(os.path.realpath(test_path()), "selinux", "host_test_text.txt")
        try:
            subprocess.run([
                podman_compose_path(),
                "-f",
                compose_path,
                "up",
                "-d",
                "container1",
                "container2",
            ])
            out, _ = self.run_subprocess_assert_returncode([
                "podman",
                "inspect",
                "selinux_container1_1",
            ])
            inspect_out = json.loads(out)
            create_command_list = inspect_out[0].get("Config", []).get("CreateCommand", [])
            self.assertIn(f'{host_path}:/test_text.txt:z', create_command_list)

            out, _ = self.run_subprocess_assert_returncode([
                "podman",
                "inspect",
                "selinux_container2_1",
            ])
            inspect_out = json.loads(out)
            create_command_list = inspect_out[0].get("Config", []).get("CreateCommand", [])
            self.assertIn(f'{host_path}:/test_text.txt', create_command_list)
        finally:
            out, _ = self.run_subprocess_assert_returncode([
                podman_compose_path(),
                "-f",
                compose_path,
                "down",
                "-t",
                "0",
            ])
