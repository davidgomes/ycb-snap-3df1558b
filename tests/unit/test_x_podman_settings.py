# SPDX-License-Identifier: GPL-2.0

import unittest

from podman_compose import PodmanCompose


class TestParseXPodmanSettings(unittest.TestCase):
    def setUp(self) -> None:
        self.compose = PodmanCompose()
        self.key = PodmanCompose.XPodmanSettingKey

    def test_env_overrides_compose_file(self) -> None:
        self.compose._parse_x_podman_settings(
            {
                "x-podman": {
                    "in_pod": True,
                    "default_net_name_compat": False,
                    "default_net_behavior_compat": False,
                    "pod_args": ["--infra=true"],
                }
            },
            {
                "PODMAN_COMPOSE_IN_POD": "0",
                "PODMAN_COMPOSE_DEFAULT_NET_NAME_COMPAT": "true",
                "PODMAN_COMPOSE_DEFAULT_NET_BEHAVIOR_COMPAT": "1",
                "PODMAN_COMPOSE_POD_ARGS": "--infra=false --share=",
                "PODMAN_COMPOSE_PROVIDER": "/usr/bin/podman-compose",
                "PODMAN_COMPOSE_WARNING_LOGS": "false",
            },
        )

        self.assertEqual(self.compose.x_podman[self.key.IN_POD], "0")
        self.assertIs(self.compose.x_podman[self.key.DEFAULT_NET_NAME_COMPAT], True)
        self.assertIs(self.compose.x_podman[self.key.DEFAULT_NET_BEHAVIOR_COMPAT], True)
        self.assertEqual(
            self.compose.x_podman[self.key.POD_ARGS], ["--infra=false", "--share="]
        )
        self.assertEqual(len(self.compose.x_podman), 4)

    def test_compose_file_used_when_env_absent(self) -> None:
        self.compose._parse_x_podman_settings(
            {"x-podman": {"in_pod": False, "pod_args": ["--cpus=1"]}},
            {},
        )
        self.assertIs(self.compose.x_podman[self.key.IN_POD], False)
        self.assertEqual(self.compose.x_podman[self.key.POD_ARGS], ["--cpus=1"])

    def test_command_line_overrides_env_and_file(self) -> None:
        self.compose._parse_x_podman_settings(
            {"x-podman": {"in_pod": True, "pod_args": ["--infra=true"]}},
            {"PODMAN_COMPOSE_IN_POD": "0", "PODMAN_COMPOSE_POD_ARGS": "--cpus=1"},
        )
        self.compose.global_args.in_pod = "custom-pod"
        self.compose.global_args.pod_args = "--infra=false"
        self.assertEqual(self.compose.resolve_in_pod(), "custom-pod")
        self.assertEqual(self.compose.resolve_pod_args(), ["--infra=false"])
