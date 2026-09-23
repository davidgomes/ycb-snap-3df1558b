# SPDX-License-Identifier: GPL-2.0

import argparse
import os
import tempfile
import unittest
from typing import Any
from unittest import mock

from parameterized import parameterized

from podman_compose import PodmanCompose
from podman_compose import default_network_name_for_project
from podman_compose import try_parse_bool

Key = PodmanCompose.XPodmanSettingKey


class TestParseXPodmanSettings(unittest.TestCase):
    def test_compose_file_values(self) -> None:
        c = PodmanCompose()
        c._parse_x_podman_settings(
            {"x-podman": {"in_pod": False, "pod_args": ["--infra=true"]}}, {}
        )
        self.assertEqual(c.x_podman, {Key.IN_POD: False, Key.POD_ARGS: ["--infra=true"]})

    def test_env_overrides_compose_file(self) -> None:
        c = PodmanCompose()
        c._parse_x_podman_settings(
            {"x-podman": {"in_pod": True, "default_net_behavior_compat": True}},
            {
                "PODMAN_COMPOSE_IN_POD": "0",
                "PODMAN_COMPOSE_DEFAULT_NET_BEHAVIOR_COMPAT": "false",
            },
        )
        self.assertEqual(c.x_podman, {Key.IN_POD: "0", Key.DEFAULT_NET_BEHAVIOR_COMPAT: "false"})

    def test_env_without_compose_file_value(self) -> None:
        c = PodmanCompose()
        c._parse_x_podman_settings({}, {"PODMAN_COMPOSE_DEFAULT_NET_NAME_COMPAT": "1"})
        self.assertEqual(c.x_podman, {Key.DEFAULT_NET_NAME_COMPAT: "1"})

    def test_reparse_does_not_keep_stale_values(self) -> None:
        c = PodmanCompose()
        c._parse_x_podman_settings({}, {"PODMAN_COMPOSE_IN_POD": "0"})
        c._parse_x_podman_settings({}, {})
        self.assertEqual(c.x_podman, {})

    def test_unrelated_env_vars_are_ignored(self) -> None:
        c = PodmanCompose()
        with self.assertNoLogs("podman_compose", level="WARNING"):
            c._parse_x_podman_settings(
                {},
                {
                    "PODMAN_COMPOSE_PROVIDER": "podman-compose",
                    "PODMAN_COMPOSE_WARNING_LOGS": "false",
                    "COMPOSE_IN_POD": "0",
                    "IN_POD": "0",
                },
            )
        self.assertEqual(c.x_podman, {})

    def test_unknown_keys_warn(self) -> None:
        c = PodmanCompose()
        with self.assertLogs("podman_compose", level="WARNING") as logs:
            c._parse_x_podman_settings(
                {"x-podman": {"bogus": 1}}, {"PODMAN_COMPOSE_OTHER_BOGUS": "1"}
            )
        self.assertEqual(c.x_podman, {})
        self.assertEqual(len(logs.records), 2)
        self.assertIn("bogus", logs.output[0])
        self.assertIn("other_bogus", logs.output[1])


class TestXPodmanSettingConsumers(unittest.TestCase):
    def _compose(self, x_podman: dict[Key, Any]) -> PodmanCompose:
        c = PodmanCompose()
        c.global_args = argparse.Namespace(in_pod=None, pod_args=None)
        c.project_name = "my-project"
        c.x_podman = x_podman
        return c

    def test_pod_args_from_env_string_is_split(self) -> None:
        c = self._compose({Key.POD_ARGS: "--infra=false --share= --cpus=1"})
        self.assertEqual(c.resolve_pod_args(), ["--infra=false", "--share=", "--cpus=1"])

    def test_pod_args_command_line_wins(self) -> None:
        c = self._compose({Key.POD_ARGS: "--cpus=1"})
        c.global_args.pod_args = "--infra=true"
        self.assertEqual(c.resolve_pod_args(), ["--infra=true"])

    def test_in_pod_command_line_wins(self) -> None:
        c = self._compose({Key.IN_POD: "0"})
        c.global_args.in_pod = "custom_pod"
        self.assertEqual(c.resolve_in_pod(), "custom_pod")

    @parameterized.expand([
        ("bool_true", True, "myproject_net"),
        ("bool_false", False, "my-project_net"),
        ("str_1", "1", "myproject_net"),
        ("str_true", "True", "myproject_net"),
        ("str_0", "0", "my-project_net"),
        ("str_false", "false", "my-project_net"),
    ])
    def test_default_net_name_compat(self, _: str, value: Any, expected: str) -> None:
        c = self._compose({Key.DEFAULT_NET_NAME_COMPAT: value})
        self.assertEqual(default_network_name_for_project(c, "net", False), expected)


class TestTryParseBool(unittest.TestCase):
    @parameterized.expand([
        (True, True),
        (False, False),
        (1, True),
        (0, False),
        ("1", True),
        ("TRUE", True),
        ("0", False),
        ("False", False),
        ("", False),
        ("maybe", None),
        (None, None),
    ])
    def test_try_parse_bool(self, value: Any, expected: bool | None) -> None:
        self.assertIs(try_parse_bool(value), expected)


class TestParseComposeFileEnvOverrides(unittest.TestCase):
    COMPOSE = """
services:
  web:
    image: busybox
    networks: [net0]
networks:
  net0: {}
x-podman:
  in_pod: true
  default_net_behavior_compat: false
"""

    def _parse(self, environ: dict[str, str]) -> PodmanCompose:
        with tempfile.TemporaryDirectory() as tmpdir:
            path = os.path.join(tmpdir, "docker-compose.yml")
            with open(path, "w", encoding="utf-8") as f:
                f.write(self.COMPOSE)
            c = PodmanCompose()
            c.global_args = argparse.Namespace(
                file=[path],
                project_name="proj",
                env_file=None,
                profile=[],
                in_pod=None,
                pod_args=None,
            )
            cwd = os.getcwd()
            with mock.patch.dict(os.environ, environ):
                try:
                    c._parse_compose_file()
                finally:
                    os.chdir(cwd)
            return c

    def test_compose_file_values_without_env(self) -> None:
        c = self._parse({})
        self.assertEqual(c.global_args.in_pod, True)
        self.assertEqual([p["name"] for p in c.pods], ["pod_proj"])
        self.assertNotIn("default", c.networks)
        self.assertEqual(c.global_args.pod_arg_list, ["--infra=false", "--share="])

    def test_env_overrides_compose_file(self) -> None:
        c = self._parse({
            "PODMAN_COMPOSE_IN_POD": "0",
            "PODMAN_COMPOSE_DEFAULT_NET_BEHAVIOR_COMPAT": "1",
            "PODMAN_COMPOSE_POD_ARGS": "--infra=false --share= --cpus=1",
        })
        self.assertEqual(c.global_args.in_pod, "0")
        self.assertEqual(c.pods, [])
        self.assertIn("default", c.networks)
        self.assertEqual(c.global_args.pod_arg_list, ["--infra=false", "--share=", "--cpus=1"])

    def test_env_false_disables_compose_file_true(self) -> None:
        self.COMPOSE = self.COMPOSE.replace(
            "default_net_behavior_compat: false", "default_net_behavior_compat: true"
        )
        self.assertIn("default", self._parse({}).networks)
        c = self._parse({"PODMAN_COMPOSE_DEFAULT_NET_BEHAVIOR_COMPAT": "false"})
        self.assertNotIn("default", c.networks)
