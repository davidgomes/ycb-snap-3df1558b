package github

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MichaelMure/git-bug/bridge/core"
	"github.com/MichaelMure/git-bug/bridge/core/auth"
	"github.com/MichaelMure/git-bug/cache"
	"github.com/MichaelMure/git-bug/repository"
)

func TestSelectsConfiguredLogin(t *testing.T) {
	dir, err := ioutil.TempDir("", "git-bug-auth")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	prevGlobal := os.Getenv("GIT_CONFIG_GLOBAL")
	prevNoSystem := os.Getenv("GIT_CONFIG_NOSYSTEM")
	require.NoError(t, os.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, "gitconfig")))
	require.NoError(t, os.Setenv("GIT_CONFIG_NOSYSTEM", "1"))
	defer func() {
		_ = os.Setenv("GIT_CONFIG_GLOBAL", prevGlobal)
		_ = os.Setenv("GIT_CONFIG_NOSYSTEM", prevNoSystem)
	}()

	repo := repository.CreateTestRepo(false)
	defer repository.CleanupTestRepos(t, repo)

	backend, err := cache.NewRepoCache(repo)
	require.NoError(t, err)
	defer backend.Close()

	for _, login := range []string{"alice", "bob"} {
		token := auth.NewToken(target, login+"-token")
		token.SetMetadata(auth.MetaKeyLogin, login)
		require.NoError(t, auth.Store(repo, token))

		user, err := backend.NewIdentity(login, login+"@example.com")
		require.NoError(t, err)
		user.SetMetadata(metaKeyGithubLogin, login)
		require.NoError(t, user.Commit())
	}

	importer := &githubImporter{}
	err = importer.Init(context.Background(), backend, core.Configuration{
		confKeyOwner:        "octocat",
		confKeyProject:      "hello",
		confKeyDefaultLogin: "nobody",
	})
	require.Equal(t, ErrMissingIdentityToken, err)

	err = importer.Init(context.Background(), backend, core.Configuration{
		confKeyOwner:        "octocat",
		confKeyProject:      "hello",
		confKeyDefaultLogin: "bob",
	})
	require.NoError(t, err)

	alice := &githubExporter{}
	err = alice.Init(context.Background(), backend, core.Configuration{
		confKeyOwner:        "octocat",
		confKeyProject:      "hello",
		confKeyDefaultLogin: "alice",
	})
	require.NoError(t, err)
	require.Equal(t, "alice-token", alice.defaultToken.Value)

	bob := &githubExporter{}
	err = bob.Init(context.Background(), backend, core.Configuration{
		confKeyOwner:        "octocat",
		confKeyProject:      "hello",
		confKeyDefaultLogin: "bob",
	})
	require.NoError(t, err)
	require.Equal(t, "bob-token", bob.defaultToken.Value)
}
