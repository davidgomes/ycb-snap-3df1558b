# SPDX-License-Identifier: GPL-2.0

import unittest

from parameterized import parameterized

from podman_compose import is_context_git_url
from podman_compose import is_path_git_url


class TestIsContextGitUrl(unittest.TestCase):
    @parameterized.expand([
        ("prefix_git", "git://host.xz/path/to/repo", True),
        ("prefix_almost_git", "gitt://host.xz/path/to/repo", False),
        ("prefix_invalid_scheme", "not_prefix://github.com/test_repo", False),
        ("http_without_suffix", "http://host.xz/path/to/repo", True),
        ("https_without_suffix", "https://github.com/containers/podman-compose", True),
        ("suffix_git", "http://host.xz/path/to/repo.git", True),
        ("suffix_with_url_fragment", "http://host.xz/path/to/repo.git#fragment", True),
        ("suffix_and_prefix", "git://host.xz/path/to/repo.git", True),
        ("scheme_uppercase", "HTTPS://github.com/containers/podman-compose.git", True),
        ("ssh", "ssh://host.xz/path/to/repo.git", True),
        ("ssh_with_user_and_port", "ssh://user@host.xz:22/path/to/repo.git", True),
        ("git_over_ssh", "git+ssh://user@host.xz/path/to/repo.git", True),
        ("file", "file:///path/to/repo.git", True),
        ("scp_like", "host.xz:path/to/repo.git", True),
        ("scp_like_with_user", "git@github.com:containers/podman-compose.git", True),
        ("scp_like_home_dir", "user@host.xz:/~user/path/to/repo.git", True),
        ("scp_like_ssh_config_host", "github:containers/podman-compose.git", True),
        ("user_without_colon", "git@github.com/containers/podman-compose.git", True),
        ("empty", "", False),
        ("current_dir", ".", False),
        ("relative", "./relative", False),
        ("relative_suffix_git", "test.git", False),
        ("relative_parent_suffix_git", "../test.git", False),
        ("absolute_suffix_git", "/home/user/test.git", False),
        ("absolute_suffix_git_with_slash", "/home/user/test.git/", False),
        ("relative_colon_after_slash", "./foo:bar", False),
        ("absolute_colon", "/path/with:colon", False),
        ("windows_drive", "C:\\path\\to\\repo.git", False),
        ("windows_drive_forward_slashes", "C:/path/to/repo.git", False),
        ("at_sign_after_slash", "path/to/user@host", False),
        ("at_sign_leading", "@scope/package", False),
    ])
    def test_is_context_git_url(self, test_name: str, path: str, result: bool) -> None:
        self.assertEqual(is_context_git_url(path), result)

    def test_is_path_git_url_alias(self) -> None:
        self.assertIs(is_path_git_url, is_context_git_url)
