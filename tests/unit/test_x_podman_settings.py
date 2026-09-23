# SPDX-License-Identifier: GPL-2.0
# pylint: disable=protected-access

from __future__ import annotations

import argparse
import os
import tempfile
import unittest
from typing import Any
from unittest import mock

import yaml
from parameterized import parameterized

from podman_compose import PodmanCompose
from podman_compose import default_network_name_for_project
from podman_compose import try_parse_bool

Key = PodmanCompose.XPodmanSettingKey


def parse_settings(x_podman: Any, environ: dict[str, Any]) -> PodmanCompose:
    compose = PodmanCompose()
    compose._parse_x_podman_settings({"x-podman": x_podman}, environ)
    return compose


class TestTryParseBool(unittest.TestCase):
    @parameterized.expand([
        (True, True),
        (False, False),
        (1, True),
        (0, False),
        ("1", True),
        ("0", False),
        ("true", True),
        ("True", True),
        (" TRUE ", True),
        ("false", False),
        ("yes", True),
        ("no", False),
        ("on", True),
        ("off", False),
        ("maybe", None),
        ("", None),
        (2, None),
        (None, None),
        ([], None),
    ])
    def test_try_parse_bool(self, value: Any, expected: bool | None) -> None:
        self.assertIs(try_parse_bool(value), expected)


class TestParseXPodmanSettings(unittest.TestCase):
    def test_no_settings(self) -> None:
        compose = PodmanCompose()
        compose._parse_x_podman_settings({}, {})
        self.assertEqual(compose.x_podman, {})

    def test_empty_x_podman_section(self) -> None:
        self.assertEqual(parse_settings(None, {}).x_podman, {})

    def test_compose_file_only(self) -> None:
        compose = parse_settings(
            {
                "in_pod": False,
                "pod_args": ["--infra=false", "--share=", "--cpus=1"],
                "default_net_name_compat": True,
                "default_net_behavior_compat": True,
            },
            {},
        )
        self.assertEqual(
            compose.x_podman,
            {
                Key.IN_POD: False,
                Key.POD_ARGS: ["--infra=false", "--share=", "--cpus=1"],
                Key.DEFAULT_NET_NAME_COMPAT: True,
                Key.DEFAULT_NET_BEHAVIOR_COMPAT: True,
            },
        )

    def test_environment_only(self) -> None:
        compose = parse_settings(
            {},
            {
                "PODMAN_COMPOSE_IN_POD": "custom_pod",
                "PODMAN_COMPOSE_POD_ARGS": "--infra=false --share= --cpus=2",
                "PODMAN_COMPOSE_DEFAULT_NET_NAME_COMPAT": "true",
                "PODMAN_COMPOSE_DEFAULT_NET_BEHAVIOR_COMPAT": "1",
            },
        )
        self.assertEqual(
            compose.x_podman,
            {
                Key.IN_POD: "custom_pod",
                Key.POD_ARGS: ["--infra=false", "--share=", "--cpus=2"],
                Key.DEFAULT_NET_NAME_COMPAT: True,
                Key.DEFAULT_NET_BEHAVIOR_COMPAT: True,
            },
        )

    @parameterized.expand([
        ("in_pod", Key.IN_POD, True, "PODMAN_COMPOSE_IN_POD", "false", "false"),
        ("in_pod_name", Key.IN_POD, False, "PODMAN_COMPOSE_IN_POD", "env_pod", "env_pod"),
        (
            "pod_args",
            Key.POD_ARGS,
            ["--infra=false", "--share="],
            "PODMAN_COMPOSE_POD_ARGS",
            "--infra=true --share=net",
            ["--infra=true", "--share=net"],
        ),
        (
            "default_net_name_compat",
            Key.DEFAULT_NET_NAME_COMPAT,
            True,
            "PODMAN_COMPOSE_DEFAULT_NET_NAME_COMPAT",
            "0",
            False,
        ),
        (
            "default_net_behavior_compat",
            Key.DEFAULT_NET_BEHAVIOR_COMPAT,
            False,
            "PODMAN_COMPOSE_DEFAULT_NET_BEHAVIOR_COMPAT",
            "yes",
            True,
        ),
    ])
    def test_environment_overrides_compose_file(
        self,
        name: str,
        key: PodmanCompose.XPodmanSettingKey,
        file_value: Any,
        env_name: str,
        env_value: str,
        expected: Any,
    ) -> None:
        compose = parse_settings({key.value: file_value}, {env_name: env_value})
        self.assertEqual(compose.x_podman, {key: expected})

    def test_environment_variable_name_is_case_insensitive(self) -> None:
        compose = parse_settings({}, {"PODMAN_COMPOSE_in_pod": "false"})
        self.assertEqual(compose.x_podman, {Key.IN_POD: "false"})

    def test_empty_environment_value_is_ignored(self) -> None:
        compose = parse_settings(
            {"in_pod": False, "default_net_behavior_compat": True},
            {
                "PODMAN_COMPOSE_IN_POD": "",
                "PODMAN_COMPOSE_DEFAULT_NET_BEHAVIOR_COMPAT": "",
                "PODMAN_COMPOSE_POD_ARGS": None,
            },
        )
        self.assertEqual(
            compose.x_podman, {Key.IN_POD: False, Key.DEFAULT_NET_BEHAVIOR_COMPAT: True}
        )

    def test_invalid_environment_value_keeps_compose_file_value(self) -> None:
        with self.assertLogs("podman_compose", level="WARNING") as logs:
            compose = parse_settings(
                {"default_net_behavior_compat": True},
                {"PODMAN_COMPOSE_DEFAULT_NET_BEHAVIOR_COMPAT": "maybe"},
            )
        self.assertEqual(compose.x_podman, {Key.DEFAULT_NET_BEHAVIOR_COMPAT: True})
        self.assertIn("PODMAN_COMPOSE_DEFAULT_NET_BEHAVIOR_COMPAT", logs.output[0])

    @parameterized.expand([
        ("bool_true", True, True),
        ("bool_false", False, False),
        ("str_true", "true", True),
        ("str_false", "false", False),
    ])
    def test_compose_file_bool_values_are_normalized(
        self, name: str, value: Any, expected: bool
    ) -> None:
        compose = parse_settings({"default_net_behavior_compat": value}, {})
        self.assertIs(compose.x_podman[Key.DEFAULT_NET_BEHAVIOR_COMPAT], expected)

    def test_compose_file_pod_args_string_is_split(self) -> None:
        compose = parse_settings({"pod_args": "--infra=false --share="}, {})
        self.assertEqual(compose.x_podman, {Key.POD_ARGS: ["--infra=false", "--share="]})

    def test_compose_file_empty_pod_args(self) -> None:
        compose = parse_settings({"pod_args": []}, {})
        self.assertEqual(compose.x_podman, {Key.POD_ARGS: []})

    def test_unknown_compose_file_key_warns(self) -> None:
        with self.assertLogs("podman_compose", level="WARNING") as logs:
            compose = parse_settings({"in_pod": False, "unknown_key": 1}, {})
        self.assertEqual(compose.x_podman, {Key.IN_POD: False})
        self.assertIn("unknown_key", logs.output[0])

    def test_unknown_environment_variable_warns(self) -> None:
        with self.assertLogs("podman_compose", level="WARNING") as logs:
            compose = parse_settings({}, {"PODMAN_COMPOSE_UNKNOWN": "1"})
        self.assertEqual(compose.x_podman, {})
        self.assertIn("PODMAN_COMPOSE_UNKNOWN", logs.output[0])

    def test_podman_compose_provider_variables_are_ignored(self) -> None:
        with mock.patch("podman_compose.log") as log_mock:
            compose = parse_settings(
                {},
                {
                    "PODMAN_COMPOSE_PROVIDER": "/usr/bin/podman-compose",
                    "PODMAN_COMPOSE_WARNING_LOGS": "false",
                    "PODMAN_USERNS": "keep-id",
                },
            )
        self.assertEqual(compose.x_podman, {})
        log_mock.warning.assert_not_called()

    def test_keys_are_interchangeable_with_strings(self) -> None:
        compose = parse_settings({"in_pod": False}, {})
        x_podman: dict[Any, Any] = compose.x_podman
        self.assertIs(x_podman["in_pod"], False)
        self.assertIs(x_podman[Key.IN_POD], False)


class TestXPodmanSettingsPrecedence(unittest.TestCase):
    def setUp(self) -> None:
        tmp_dir = tempfile.TemporaryDirectory()
        self.addCleanup(tmp_dir.cleanup)
        self.dir = os.path.join(tmp_dir.name, "x-podman-proj")
        os.mkdir(self.dir)

    def parse(
        self,
        x_podman: dict[str, Any] | None,
        env: dict[str, str],
        in_pod: str | None = None,
        pod_args: str | None = None,
        networks: dict[str, Any] | None = None,
    ) -> PodmanCompose:
        content: dict[str, Any] = {"services": {"web": {"image": "busybox"}}}
        if x_podman is not None:
            content["x-podman"] = x_podman
        if networks is not None:
            content["networks"] = networks
            content["services"]["web"]["networks"] = list(networks)
        compose_path = os.path.join(self.dir, "docker-compose.yml")
        with open(compose_path, "w", encoding="utf-8") as f:
            yaml.safe_dump(content, f)

        compose = PodmanCompose()
        compose.global_args = argparse.Namespace(
            file=[compose_path],
            project_name=None,
            env_file=None,
            profile=[],
            in_pod=in_pod,
            pod_args=pod_args,
            no_normalize=True,
        )
        environ = {k: v for k, v in os.environ.items() if not k.startswith("PODMAN_COMPOSE_")}
        environ.pop("COMPOSE_PROJECT_NAME", None)
        environ.pop("COMPOSE_PROJECT_DIR", None)
        environ.update(env)
        with mock.patch.dict(os.environ, environ, clear=True):
            compose._parse_compose_file()
        return compose

    def test_in_pod_default(self) -> None:
        compose = self.parse(None, {})
        self.assertEqual(compose.pods, [{"name": "pod_x-podman-proj"}])

    def test_in_pod_compose_file(self) -> None:
        compose = self.parse({"in_pod": False}, {})
        self.assertEqual(compose.pods, [])

    def test_in_pod_environment_overrides_compose_file(self) -> None:
        compose = self.parse({"in_pod": True}, {"PODMAN_COMPOSE_IN_POD": "false"})
        self.assertEqual(compose.pods, [])
        self.assertEqual(compose.containers[0]["pod"], None)

    def test_in_pod_environment_custom_pod_name(self) -> None:
        compose = self.parse({"in_pod": False}, {"PODMAN_COMPOSE_IN_POD": "env_pod"})
        self.assertEqual(compose.pods, [{"name": "env_pod"}])
        self.assertEqual(compose.containers[0]["pod"], "env_pod")

    def test_in_pod_command_line_overrides_environment(self) -> None:
        compose = self.parse({"in_pod": False}, {"PODMAN_COMPOSE_IN_POD": "false"}, in_pod="true")
        self.assertEqual(compose.pods, [{"name": "pod_x-podman-proj"}])

    def test_pod_args_default(self) -> None:
        compose = self.parse(None, {})
        self.assertEqual(compose.global_args.pod_arg_list, ["--infra=false", "--share="])

    def test_pod_args_environment_overrides_compose_file(self) -> None:
        compose = self.parse(
            {"pod_args": ["--infra=false", "--share=", "--cpus=1"]},
            {"PODMAN_COMPOSE_POD_ARGS": "--infra=false --share= --cpus=2"},
        )
        self.assertEqual(
            compose.global_args.pod_arg_list, ["--infra=false", "--share=", "--cpus=2"]
        )

    def test_pod_args_command_line_overrides_environment(self) -> None:
        compose = self.parse(
            {"pod_args": ["--cpus=1"]},
            {"PODMAN_COMPOSE_POD_ARGS": "--cpus=2"},
            pod_args="--cpus=3",
        )
        self.assertEqual(compose.global_args.pod_arg_list, ["--cpus=3"])

    def test_default_net_behavior_compat_from_environment(self) -> None:
        networks: dict[str, Any] = {"net0": {}, "net1": {}}
        compose = self.parse(None, {}, networks=networks)
        self.assertIsNone(compose.default_net)
        self.assertNotIn("default", compose.networks)

        compose = self.parse(
            None, {"PODMAN_COMPOSE_DEFAULT_NET_BEHAVIOR_COMPAT": "true"}, networks=networks
        )
        self.assertEqual(compose.default_net, "default")
        self.assertIn("default", compose.networks)

    def test_default_net_behavior_compat_environment_disables_compose_file(self) -> None:
        compose = self.parse(
            {"default_net_behavior_compat": True},
            {"PODMAN_COMPOSE_DEFAULT_NET_BEHAVIOR_COMPAT": "false"},
            networks={"net0": {}, "net1": {}},
        )
        self.assertIsNone(compose.default_net)
        self.assertNotIn("default", compose.networks)

    def test_default_net_name_compat_from_environment(self) -> None:
        compose = self.parse(None, {})
        self.assertEqual(
            default_network_name_for_project(compose, "net1", False), "x-podman-proj_net1"
        )

        compose = self.parse(None, {"PODMAN_COMPOSE_DEFAULT_NET_NAME_COMPAT": "true"})
        self.assertEqual(
            default_network_name_for_project(compose, "net1", False), "xpodmanproj_net1"
        )

    def test_environment_is_read_from_env_file(self) -> None:
        env_path = os.path.join(self.dir, "custom.env")
        with open(env_path, "w", encoding="utf-8") as f:
            f.write("PODMAN_COMPOSE_IN_POD=false\n")
        compose_path = os.path.join(self.dir, "docker-compose.yml")
        with open(compose_path, "w", encoding="utf-8") as f:
            yaml.safe_dump({"services": {"web": {"image": "busybox"}}}, f)

        compose = PodmanCompose()
        compose.global_args = argparse.Namespace(
            file=[compose_path],
            project_name=None,
            env_file=env_path,
            profile=[],
            in_pod=None,
            pod_args=None,
            no_normalize=True,
        )
        environ = {k: v for k, v in os.environ.items() if not k.startswith("PODMAN_COMPOSE_")}
        with mock.patch.dict(os.environ, environ, clear=True):
            compose._parse_compose_file()
        self.assertEqual(compose.pods, [])
