package gitlab

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/MichaelMure/git-bug/bridge/core"
	"github.com/MichaelMure/git-bug/bridge/core/auth"
)

func TestBuildClient(t *testing.T) {
	client, err := buildClient("https://gitlab.example.com/gitlab", auth.NewToken("", "secret", target))
	require.NoError(t, err)
	assert.Equal(t, "https://gitlab.example.com/gitlab/api/v4/", client.BaseURL().String())
}

func TestConfBaseURL(t *testing.T) {
	assert.Equal(t, defaultBaseURL, confBaseURL(core.Configuration{keyProjectID: "1"}))
	assert.Equal(t, "https://gitlab.example.com/", confBaseURL(core.Configuration{
		keyProjectID:     "1",
		keyGitlabBaseUrl: "https://gitlab.example.com/",
	}))
}
