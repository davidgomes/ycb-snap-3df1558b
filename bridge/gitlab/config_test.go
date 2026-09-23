package gitlab

import (
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

func TestNormalizeGitlabBaseURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "project url drops path",
			raw:  "https://gitlab.internal.corp/devops/infrastructure",
			want: "https://gitlab.internal.corp/",
		},
		{
			name: "private instance project url",
			raw:  "https://private.instance/username/semrel-changelog-mr",
			want: "https://private.instance/",
		},
		{
			name: "gitlab.com without trailing slash",
			raw:  "https://gitlab.com",
			want: "https://gitlab.com/",
		},
		{
			name: "gitlab.com with trailing slash",
			raw:  "https://gitlab.com/",
			want: "https://gitlab.com/",
		},
		{
			name: "gitlab.com project url",
			raw:  "https://gitlab.com/MichaelMure/git-bug",
			want: "https://gitlab.com/",
		},
		{
			name: "http kept",
			raw:  "http://gitlab.local/group/proj",
			want: "http://gitlab.local/",
		},
		{
			name: "custom port",
			raw:  "https://gitlab.internal.corp:8443/group/proj",
			want: "https://gitlab.internal.corp:8443/",
		},
		{
			name: "empty defaults to gitlab.com",
			raw:  "",
			want: "https://gitlab.com/",
		},
		{
			name: "git protocol becomes https",
			raw:  "git://gitlab.com/MichaelMure/git-bug.git",
			want: "https://gitlab.com/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeGitlabBaseURL(tt.raw)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildClientUsesInstanceRoot(t *testing.T) {
	token := auth.NewToken("tester", "glpat-test", target)

	client, err := buildClient("https://gitlab.internal.corp/devops/infrastructure", token)
	require.NoError(t, err)

	base := client.BaseURL()
	assert.Equal(t, "https", base.Scheme)
	assert.Equal(t, "gitlab.internal.corp", base.Host)
	assert.Equal(t, "/api/v4/", base.Path)

	client, err = buildClient("https://gitlab.com", token)
	require.NoError(t, err)
	assert.Equal(t, "https://gitlab.com/api/v4/", client.BaseURL().String())
}
