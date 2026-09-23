# SPDX-License-Identifier: GPL-2.0

import unittest

from parameterized import parameterized

from podman_compose import is_context_git_url


class TestIsPathGitUrl(unittest.TestCase):
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
        ("http_example", "http://example.com/my-project.git", True),
        ("file_absolute", "file:///absolute/path/to/my-project.git", True),
        ("ssh_user", "ssh:user@example.com:my-project", True),
        ("git_at_github", "git@github.com:user/project.git", True),
        ("https_github", "https://github.com/user/project.git", True),
        ("http_github", "http://github.com/user/project.git", True),
        ("git_at_ip", "git@192.168.101.127:user/project.git", True),
        ("https_ip", "https://192.168.101.127/user/project.git", True),
        ("http_ip", "http://192.168.101.127/user/project.git", True),
        ("ssh_port", "ssh://user@host.xz:port/path/to/repo.git/", True),
        ("ssh_user_path", "ssh://user@host.xz/path/to/repo.git/", True),
        ("ssh_host_port", "ssh://host.xz:port/path/to/repo.git/", True),
        ("ssh_host", "ssh://host.xz/path/to/repo.git/", True),
        ("ssh_user_tilde", "ssh://user@host.xz/~user/path/to/repo.git/", True),
        ("ssh_tilde", "ssh://host.xz/~user/path/to/repo.git/", True),
        ("ssh_home", "ssh://user@host.xz/~/path/to/repo.git", True),
        ("ssh_home_host", "ssh://host.xz/~/path/to/repo.git", True),
        ("git_scheme", "git://host.xz/path/to/repo.git/", True),
        ("git_tilde", "git://host.xz/~user/path/to/repo.git/", True),
        ("http_suffix", "http://host.xz/path/to/repo.git/", True),
        ("https_suffix", "https://host.xz/path/to/repo.git/", True),
        ("custom_gitlab", "git@custom-gitlab:my-group/myrepo.git", True),
        ("scp_user_abs", "user@host.xz:/path/to/repo.git/", True),
        ("scp_host_abs", "host.xz:/path/to/repo.git/", True),
        ("scp_host_rel_git", "host:path/to/repo.git", True),
        ("scp_host_rel", "host:path/to/repo", True),
        ("scp_user_tilde", "user@host.xz:~user/path/to/repo.git/", True),
        ("scp_host_tilde", "host.xz:~user/path/to/repo.git/", True),
        ("scp_user_rel", "user@host.xz:path/to/repo.git", True),
        ("user_at_path", "user@path/to/repo", True),
        ("scp_host_git", "host.xz:path/to/repo.git", True),
        ("rsync", "rsync://host.xz/path/to/repo.git/", True),
        ("file_repo", "file:///path/to/repo.git/", True),
        ("file_home", "file://~/path/to/repo.git/", True),
        ("github_scp", "github.com:containers/podman-compose.git", True),
        ("github_alias", "github:containers/podman-compose.git", True),
    ])
    def test_is_path_git_url(self, test_name: str, path: str, result: bool) -> None:
        self.assertEqual(is_context_git_url(path), result)
