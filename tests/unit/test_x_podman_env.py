# SPDX-License-Identifier: GPL-2.0

import unittest

from podman_compose import PodmanCompose


class TestXPodmanEnvOverrides(unittest.TestCase):
    def test_env_overrides_compose_and_coerces_types(self) -> None:
        compose = PodmanCompose()
        compose._parse_x_podman_settings(
            {
                "x-podman": {
                    "in_pod": True,
                    "default_net_behavior_compat": False,
                    "pod_args": ["--infra=false", "--share="],
                    "not_a_setting": 1,
                }
            },
            {
                "PODMAN_COMPOSE_IN_POD": "0",
                "PODMAN_COMPOSE_DEFAULT_NET_BEHAVIOR_COMPAT": "false",
                "PODMAN_COMPOSE_DEFAULT_NET_NAME_COMPAT": "true",
                "PODMAN_COMPOSE_POD_ARGS": "--infra=false --share= --cpus=1",
                "PODMAN_COMPOSE_PROVIDER": "podman",
                "PODMAN_COMPOSE_UNKNOWN": "x",
            },
        )
        key = PodmanCompose.XPodmanSettingKey
        self.assertEqual(compose.x_podman[key.IN_POD], "0")
        self.assertIs(compose.x_podman[key.DEFAULT_NET_BEHAVIOR_COMPAT], False)
        self.assertIs(compose.x_podman[key.DEFAULT_NET_NAME_COMPAT], True)
        self.assertEqual(
            compose.x_podman[key.POD_ARGS],
            ["--infra=false", "--share=", "--cpus=1"],
        )
        self.assertNotIn("not_a_setting", {k.value for k in compose.x_podman})

    def test_yaml_list_pod_args(self) -> None:
        compose = PodmanCompose()
        compose._parse_x_podman_settings(
            {},
            {"PODMAN_COMPOSE_POD_ARGS": '["--infra=true", "--share="]'},
        )
        self.assertEqual(
            compose.x_podman[PodmanCompose.XPodmanSettingKey.POD_ARGS],
            ["--infra=true", "--share="],
        )
