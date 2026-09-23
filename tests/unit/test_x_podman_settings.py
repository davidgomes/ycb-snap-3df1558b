# SPDX-License-Identifier: GPL-2.0

import argparse
import unittest

from podman_compose import PodmanCompose
from podman_compose import default_network_name_for_project


class TestXPodmanSettings(unittest.TestCase):
    def _compose(self) -> PodmanCompose:
        compose = PodmanCompose()
        compose.global_args = argparse.Namespace(in_pod=None, pod_args=None)
        compose.project_name = "microservice-a"
        return compose

    def test_env_overrides_compose_file(self) -> None:
        compose = self._compose()
        compose._parse_x_podman_settings(
            {
                "x-podman": {
                    "default_net_name_compat": False,
                    "default_net_behavior_compat": False,
                    "in_pod": True,
                    "pod_args": ["--infra=true", "--share=net"],
                }
            },
            {
                "PODMAN_COMPOSE_DEFAULT_NET_NAME_COMPAT": "true",
                "PODMAN_COMPOSE_DEFAULT_NET_BEHAVIOR_COMPAT": "1",
                "PODMAN_COMPOSE_IN_POD": "0",
                "PODMAN_COMPOSE_POD_ARGS": "--infra=false --share=",
                "PODMAN_COMPOSE_PROVIDER": "docker-compose",
                "PODMAN_COMPOSE_WARNING_LOGS": "false",
            },
        )

        key = PodmanCompose.XPodmanSettingKey
        self.assertIs(compose.x_podman[key.DEFAULT_NET_NAME_COMPAT], True)
        self.assertIs(compose.x_podman[key.DEFAULT_NET_BEHAVIOR_COMPAT], True)
        self.assertEqual(compose.x_podman[key.IN_POD], "0")
        self.assertEqual(compose.x_podman[key.POD_ARGS], ["--infra=false", "--share="])
        self.assertEqual(
            default_network_name_for_project(compose, "default", False),
            "microservicea_default",
        )
        self.assertEqual(compose.resolve_in_pod(), "0")
        self.assertEqual(
            compose.resolve_pod_args(),
            ["--infra=false", "--share="],
        )

    def test_compose_file_used_when_env_absent(self) -> None:
        compose = self._compose()
        compose._parse_x_podman_settings(
            {
                "x-podman": {
                    "default_net_name_compat": False,
                    "in_pod": False,
                    "pod_args": ["--cpus=1"],
                }
            },
            {},
        )
        key = PodmanCompose.XPodmanSettingKey
        self.assertIs(compose.x_podman[key.DEFAULT_NET_NAME_COMPAT], False)
        self.assertEqual(
            default_network_name_for_project(compose, "default", False),
            "microservice-a_default",
        )
        self.assertIs(compose.resolve_in_pod(), False)
        self.assertEqual(compose.resolve_pod_args(), ["--cpus=1"])

    def test_command_line_overrides_env(self) -> None:
        compose = self._compose()
        compose._parse_x_podman_settings(
            {"x-podman": {"in_pod": False}},
            {"PODMAN_COMPOSE_IN_POD": "0", "PODMAN_COMPOSE_POD_ARGS": "--cpus=1"},
        )
        compose.global_args.in_pod = "custom-pod"
        compose.global_args.pod_args = "--cpus=2"
        self.assertEqual(compose.resolve_in_pod(), "custom-pod")
        self.assertEqual(compose.resolve_pod_args(), ["--cpus=2"])

    def test_string_true_enables_compat_network_name(self) -> None:
        compose = self._compose()
        compose.x_podman = {
            PodmanCompose.XPodmanSettingKey.DEFAULT_NET_NAME_COMPAT: "true",
        }
        self.assertEqual(
            default_network_name_for_project(compose, "default", False),
            "microservicea_default",
        )

    def test_unknown_keys_are_ignored(self) -> None:
        compose = self._compose()
        with self.assertLogs("podman_compose", level="WARNING") as logs:
            compose._parse_x_podman_settings(
                {"x-podman": {"not_a_setting": True, "in_pod": False}},
                {"PODMAN_COMPOSE_ALSO_UNKNOWN": "1"},
            )
        self.assertIs(compose.x_podman[PodmanCompose.XPodmanSettingKey.IN_POD], False)
        self.assertEqual(len(compose.x_podman), 1)
        self.assertTrue(any("not_a_setting" in line for line in logs.output))
        self.assertTrue(any("also_unknown" in line for line in logs.output))
