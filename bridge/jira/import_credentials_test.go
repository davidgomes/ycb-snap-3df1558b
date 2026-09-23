package jira

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MichaelMure/git-bug/bridge/core"
	"github.com/MichaelMure/git-bug/bridge/core/auth"
	"github.com/MichaelMure/git-bug/cache"
	"github.com/MichaelMure/git-bug/repository"
)

// memGlobalConfigRepo keeps the stored credentials out of the user's global git config
type memGlobalConfigRepo struct {
	*repository.GitRepo
	globalConfig *repository.MemConfig
}

func (r *memGlobalConfigRepo) GlobalConfig() repository.Config {
	return r.globalConfig
}

func TestJiraImporterUsesDefaultLoginCredential(t *testing.T) {
	const baseURL = "https://jira.example.com"

	repo := repository.CreateTestRepo(false)
	defer repository.CleanupTestRepos(t, repo)

	backend, err := cache.NewRepoCache(&memGlobalConfigRepo{repo, repository.NewMemConfig()})
	require.NoError(t, err)
	defer backend.Close()

	for _, login := range []string{"alice", "bob", "carol"} {
		lp := auth.NewLoginPassword(target, login, "password-of-"+login)
		lp.SetMetadata(auth.MetaKeyLogin, login)
		lp.SetMetadata(auth.MetaKeyBaseURL, baseURL)
		require.NoError(t, auth.Store(backend, lp))
	}

	// TOKEN credentials are only attached to the transport, no request is made
	conf := core.Configuration{
		confKeyBaseUrl:        baseURL,
		confKeyProject:        "TEST",
		confKeyCredentialType: "TOKEN",
		confKeyDefaultLogin:   "bob",
	}

	expected := base64.StdEncoding.EncodeToString([]byte("bob:password-of-bob"))

	// credentials are listed in random order, a single run could pass by chance
	for i := 0; i < 20; i++ {
		importer := &jiraImporter{}
		require.NoError(t, importer.Init(context.Background(), backend, conf))

		transport, ok := importer.client.Transport.(*ClientTransport)
		require.True(t, ok)
		require.Equal(t, expected, transport.basicAuthString)
	}
}
