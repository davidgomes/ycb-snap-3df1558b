# SPDX-License-Identifier: GPL-2.0

import unittest

from parameterized import parameterized

from podman_compose import is_context_git_url


class TestIsContextGitUrl(unittest.TestCase):
    @parameterized.expand([
        ("with_url_fragment", "http://host.xz/path/to/repo.git#fragment", True),
        ("suffix_and_prefix", "git://host.xz/path/to/repo.git", True),
        ("empty_url_path", "http://#fragment", True),
        ("no_prefix", "http://host.xz/path/to/repo", True),
        ("wrong_prefix_git", "gitt://host.xz/path/to/repo", False),
        ("wrong_prefix_http", "htt://host.xz/path/to/repo.git", False),
        ("user_path_ending_with_git", "path/to/workdir.git", False),
        ("absolute_workdir_git", "/path/to/workdir.git", False),
        ("absolute_colon", "/path/to:workdir.git", False),
        ("absolute_at", "/path/to@workdir.git", False),
        ("home_relative", "~/path/to/workdir.git", False),
        ("https_not_suffix", "https://github.com/test_repo.git/git_not_suffix", True),
        ("github_scp", "github.com:containers/podman-compose.git", True),
        ("github_alias", "github:containers/podman-compose.git", True),
    ])
    def test_is_context_git_url(self, test_name: str, path: str, result: bool) -> None:
        self.assertEqual(is_context_git_url(path), result)
