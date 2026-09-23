# SPDX-License-Identifier: GPL-2.0

import unittest

from parameterized import parameterized

from podman_compose import is_context_git_url


class TestIsContextGitUrl(unittest.TestCase):
    @parameterized.expand([
        ("prefix_git", "git://host.xz/path/to/repo", True),
        ("prefix_almost_git", "gitt://host.xz/path/to/repo", False),
        ("prefix_almost_http", "htt://host.xz/path/to/repo.git", False),
        ("prefix_http_no_suffix", "http://host.xz/path/to/repo", True),
        ("suffix_git", "http://host.xz/path/to/repo.git", True),
        ("suffix_with_url_fragment", "http://host.xz/path/to/repo.git#fragment", True),
        ("suffix_and_prefix", "git://host.xz/path/to/repo.git", True),
        ("empty_url_path", "http://#fragment", True),
        ("git_not_suffix", "https://github.com/test_repo.git/git_not_suffix", True),
        ("https", "https://github.com/user/project.git", True),
        ("https_ip", "https://192.168.101.127/user/project.git", True),
        ("ssh_user_port", "ssh://user@host.xz:port/path/to/repo.git/", True),
        ("ssh_no_user", "ssh://host.xz/~/path/to/repo.git", True),
        ("ssh_scp_like", "ssh:user@example.com:my-project", True),
        ("rsync", "rsync://host.xz/path/to/repo.git/", True),
        ("file", "file:///absolute/path/to/my-project.git", True),
        ("file_home", "file://~/path/to/repo.git/", True),
        ("scp_like_user", "git@github.com:user/project.git", True),
        ("scp_like_user_ip", "git@192.168.101.127:user/project.git", True),
        ("scp_like_user_custom_host", "git@custom-gitlab:my-group/myrepo.git", True),
        ("scp_like_user_abs_path", "user@host.xz:/path/to/repo.git/", True),
        ("scp_like_user_home", "user@host.xz:~user/path/to/repo.git/", True),
        ("scp_like_user_no_colon", "user@path/to/repo", True),
        ("scp_like_host", "host.xz:path/to/repo.git", True),
        ("scp_like_host_abs_path", "host.xz:/path/to/repo.git/", True),
        ("scp_like_host_no_suffix", "host:path/to/repo", True),
        ("scp_like_github", "github.com:containers/podman-compose.git", True),
        ("scp_like_ssh_alias", "github:containers/podman-compose.git", True),
        ("dir_ending_with_git", "workdir.git", False),
        ("relative_dir_ending_with_git", "path/to/workdir.git", False),
        ("absolute_dir_ending_with_git", "/path/to/workdir.git", False),
        ("absolute_dir_with_colon", "/path/to:workdir.git", False),
        ("absolute_dir_with_at", "/path/to@workdir.git", False),
        ("home_dir_ending_with_git", "~/path/to/workdir.git", False),
        ("current_dir", ".", False),
        ("parent_dir", "../context", False),
        ("unix_path", "/path/to/containerfile/context", False),
        ("windows_path_backslash", "C:\\path\\to\\containerfile\\context", False),
        ("windows_path_slash", "C:/path/to/containerfile/context", False),
    ])
    def test_is_context_git_url(self, test_name: str, path: str, result: bool) -> None:
        self.assertEqual(is_context_git_url(path), result)
