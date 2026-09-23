# SPDX-License-Identifier: GPL-2.0

import unittest

from parameterized import parameterized

from podman_compose import container_to_args
from tests.unit.test_container_to_args import create_compose_mock
from tests.unit.test_container_to_args import get_minimal_container


class TestContainerToArgsHealthcheck(unittest.IsolatedAsyncioTestCase):
    async def _healthcheck_args(self, healthcheck):
        c = create_compose_mock()
        cnt = get_minimal_container()
        cnt["healthcheck"] = healthcheck
        return await container_to_args(c, cnt)

    @parameterized.expand([
        ("string", "cmd arg1 arg2", '["CMD-SHELL", "cmd arg1 arg2"]'),
        ("cmd", ["CMD", "cmd", "arg1", "arg2"], '["cmd", "arg1", "arg2"]'),
        ("cmd_shell", ["CMD-SHELL", "cmd arg1 arg2"], '["cmd arg1 arg2"]'),
        (
            "cmd_keeps_arg_with_whitespace",
            ["CMD", "pg_isready", "-U", "my user"],
            '["pg_isready", "-U", "my user"]',
        ),
        (
            "cmd_does_not_expand_variables",
            ["CMD", "echo", "$HOME"],
            '["echo", "$HOME"]',
        ),
        (
            "cmd_shell_keeps_quotes",
            ["CMD-SHELL", "curl -f 'http://localhost/?a=1&b=2' || exit 1"],
            '["curl -f \'http://localhost/?a=1&b=2\' || exit 1"]',
        ),
    ])
    async def test_healthcheck_command(self, _, test, expected_command):
        args = await self._healthcheck_args({"test": test})
        self.assertEqual(
            args,
            [
                "--name=project_name_service_name1",
                "-d",
                "--network=bridge:alias=service_name",
                "--healthcheck-command",
                expected_command,
                "busybox",
            ],
        )

    async def test_healthcheck_does_not_modify_service_definition(self):
        test = ["CMD", "cmd", "arg1"]
        await self._healthcheck_args({"test": test})
        self.assertEqual(test, ["CMD", "cmd", "arg1"])

    @parameterized.expand([
        ("none", {"test": ["NONE"]}),
        ("disable", {"test": "cmd arg1 arg2", "disable": True}),
    ])
    async def test_healthcheck_disabled(self, _, healthcheck):
        args = await self._healthcheck_args(healthcheck)
        self.assertEqual(
            args,
            [
                "--name=project_name_service_name1",
                "-d",
                "--network=bridge:alias=service_name",
                "--no-healthcheck",
                "busybox",
            ],
        )

    async def test_healthcheck_options(self):
        args = await self._healthcheck_args({
            "test": ["CMD", "cmd", "arg1"],
            "interval": "1m",
            "timeout": "10s",
            "start_period": "5s",
            "retries": 3,
        })
        self.assertEqual(
            args,
            [
                "--name=project_name_service_name1",
                "-d",
                "--network=bridge:alias=service_name",
                "--healthcheck-command",
                '["cmd", "arg1"]',
                "--healthcheck-interval",
                "1m",
                "--healthcheck-timeout",
                "10s",
                "--healthcheck-start-period",
                "5s",
                "--healthcheck-retries",
                "3",
                "busybox",
            ],
        )

    async def test_healthcheck_cmd_shell_requires_single_string(self):
        with self.assertRaisesRegex(ValueError, "'CMD-SHELL' takes a single string after it"):
            await self._healthcheck_args({"test": ["CMD-SHELL", "cmd arg1", "arg2"]})

    async def test_healthcheck_unknown_test_type(self):
        with self.assertRaisesRegex(ValueError, r"unknown healthcheck test type \[TEST\]"):
            await self._healthcheck_args({"test": ["TEST", "arg1"]})

    async def test_healthcheck_test_not_string_or_list(self):
        with self.assertRaisesRegex(ValueError, "'healthcheck.test' either a string or a list"):
            await self._healthcheck_args({"test": 2})

    async def test_healthcheck_not_a_mapping(self):
        with self.assertRaisesRegex(ValueError, "'healthcheck' must be a key-value mapping"):
            await self._healthcheck_args([])
