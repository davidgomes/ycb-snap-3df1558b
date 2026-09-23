package gitlab

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/MichaelMure/git-bug/bridge/core/auth"
	"github.com/MichaelMure/git-bug/entity"
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

func TestProjectBaseURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "gitlab.com project",
			url:  "https://gitlab.com/MichaelMure/git-bug",
			want: "https://gitlab.com/",
		},
		{
			name: "self-hosted project path",
			url:  "https://gitlab.internal.corp/devops/infrastructure",
			want: "https://gitlab.internal.corp/",
		},
		{
			name: "self-hosted with port and git suffix",
			url:  "https://gitlab.internal.corp:8443/group/sub/proj.git",
			want: "https://gitlab.internal.corp:8443/",
		},
		{
			name: "ssh url",
			url:  "git@gitlab.internal.corp/devops/infrastructure.git",
			want: "https://gitlab.internal.corp/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := projectBaseURL(tt.url)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildClientBaseURL(t *testing.T) {
	token := auth.NewToken(entity.Id("user"), "token", target)

	client, err := buildClient("https://gitlab.com", token)
	require.NoError(t, err)
	assert.Equal(t, "https://gitlab.com/api/v4/", client.BaseURL().String())

	base, err := projectBaseURL("https://gitlab.internal.corp/devops/infrastructure")
	require.NoError(t, err)
	client, err = buildClient(base, token)
	require.NoError(t, err)
	assert.Equal(t, "https://gitlab.internal.corp/api/v4/", client.BaseURL().String())

	client, err = buildClient("", token)
	require.NoError(t, err)
	assert.Equal(t, "https://gitlab.com/api/v4/", client.BaseURL().String())
}
