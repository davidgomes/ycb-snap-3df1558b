package gitlab

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/MichaelMure/git-bug/bridge/core/auth"
)

func TestProjectPath(t *testing.T) {
	type args struct {
		url string
	}
	type want struct {
		path string
		err  error
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "default url",
			args: args{
				url: "https://gitlab.com/MichaelMure/git-bug",
			},
			want: want{
				path: "MichaelMure/git-bug",
				err:  nil,
			},
		},
		{
			name: "multiple sub groups",
			args: args{
				url: "https://gitlab.com/MichaelMure/group/subgroup/git-bug",
			},
			want: want{
				path: "MichaelMure/group/subgroup/git-bug",
				err:  nil,
			},
		},
		{
			name: "default url with git extension",
			args: args{
				url: "https://gitlab.com/MichaelMure/git-bug.git",
			},
			want: want{
				path: "MichaelMure/git-bug",
				err:  nil,
			},
		},
		{
			name: "url with git protocol",
			args: args{
				url: "git://gitlab.com/MichaelMure/git-bug.git",
			},
			want: want{
				path: "MichaelMure/git-bug",
				err:  nil,
			},
		},
		{
			name: "ssh url",
			args: args{
				url: "git@gitlab.com/MichaelMure/git-bug.git",
			},
			want: want{
				path: "MichaelMure/git-bug",
				err:  nil,
			},
		},
		{
			name: "bad url",
			args: args{
				url: "---,%gitlab.com/MichaelMure/git-bug.git",
			},
			want: want{
				err: ErrBadProjectURL,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := getProjectPath(tt.args.url)
			assert.Equal(t, tt.want.path, path)
			assert.Equal(t, tt.want.err, err)
		})
	}
}

func TestInstanceProjectPath(t *testing.T) {
	tests := []struct {
		name       string
		baseUrl    string
		projectUrl string
		path       string
		err        bool
	}{
		{
			name:       "gitlab.com",
			baseUrl:    defaultBaseURL,
			projectUrl: "https://gitlab.com/MichaelMure/git-bug",
			path:       "MichaelMure/git-bug",
		},
		{
			name:       "self-hosted",
			baseUrl:    "https://gitlab.example.com",
			projectUrl: "https://gitlab.example.com/group/subgroup/git-bug.git",
			path:       "group/subgroup/git-bug",
		},
		{
			name:       "self-hosted ssh url",
			baseUrl:    "https://gitlab.example.com/",
			projectUrl: "git@gitlab.example.com/group/git-bug.git",
			path:       "group/git-bug",
		},
		{
			name:       "self-hosted with sub-path",
			baseUrl:    "https://example.com/gitlab/",
			projectUrl: "https://example.com/gitlab/group/git-bug",
			path:       "group/git-bug",
		},
		{
			name:       "project on another instance",
			baseUrl:    defaultBaseURL,
			projectUrl: "https://gitlab.example.com/group/git-bug",
			err:        true,
		},
		{
			name:       "project outside of the sub-path",
			baseUrl:    "https://example.com/gitlab",
			projectUrl: "https://example.com/gitlabfoo/git-bug",
			err:        true,
		},
		{
			name:       "base url without scheme",
			baseUrl:    "gitlab.example.com",
			projectUrl: "https://gitlab.example.com/group/git-bug",
			err:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := getInstanceProjectPath(tt.baseUrl, tt.projectUrl)
			if tt.err {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.path, path)
		})
	}
}

func TestValidGitlabRemoteURLs(t *testing.T) {
	remotes := map[string]string{
		"origin":   "git@gitlab.example.com/group/git-bug.git",
		"gitlab":   "https://gitlab.com/MichaelMure/git-bug.git",
		"upstream": "https://github.com/MichaelMure/git-bug.git",
	}

	assert.Equal(t, []string{"https://gitlab.example.com/group/git-bug"},
		getValidGitlabRemoteURLs("https://gitlab.example.com/", remotes))
	assert.Equal(t, []string{"https://gitlab.com/MichaelMure/git-bug"},
		getValidGitlabRemoteURLs(defaultBaseURL, remotes))
}

func TestValidateProjectURLSelfHosted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/gitlab/api/v4/projects/group%2Fgit-bug" ||
			r.Header.Get("PRIVATE-TOKEN") != "secret" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"id": 42}`))
	}))
	defer server.Close()

	baseUrl := server.URL + "/gitlab/"
	token := auth.NewToken("", "secret", target)

	id, err := validateProjectURL(baseUrl, baseUrl+"group/git-bug", token)
	require.NoError(t, err)
	assert.Equal(t, 42, id)
}
