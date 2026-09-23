package jira

import (
	"context"
	"encoding/base64"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MichaelMure/git-bug/bridge/core"
	"github.com/MichaelMure/git-bug/bridge/core/auth"
	"github.com/MichaelMure/git-bug/cache"
	"github.com/MichaelMure/git-bug/repository"
)

func TestJiraImportSelectsCredentialForDefaultLogin(t *testing.T) {
	repo, backend := newJiraCredentialRepo(t)
	defer backend.Close()

	storeJiraCred(t, repo, "other-user", "wrong-secret")
	storeJiraCred(t, repo, "wanted-user", "right-secret")

	importer := &jiraImporter{}
	err := importer.Init(context.Background(), backend, core.Configuration{
		confKeyBaseUrl:        "https://jira.example.com",
		confKeyProject:        "PROJ",
		confKeyCredentialType: "TOKEN",
		confKeyDefaultLogin:   "wanted-user",
	})
	require.NoError(t, err)

	transport, ok := importer.client.Transport.(*ClientTransport)
	require.True(t, ok)
	raw, err := base64.StdEncoding.DecodeString(transport.basicAuthString)
	require.NoError(t, err)
	require.Equal(t, "wanted-user:right-secret", string(raw))
}

func TestJiraImportIgnoresOtherAccount(t *testing.T) {
	repo, backend := newJiraCredentialRepo(t)
	defer backend.Close()

	storeJiraCred(t, repo, "other-user", "wrong-secret")

	importer := &jiraImporter{}
	err := importer.Init(context.Background(), backend, core.Configuration{
		confKeyBaseUrl:        "https://jira.example.com",
		confKeyProject:        "PROJ",
		confKeyCredentialType: "TOKEN",
		confKeyDefaultLogin:   "wanted-user",
	})
	require.Error(t, err)
}

func TestJiraValidateConfigRequiresDefaultLogin(t *testing.T) {
	err := (&Jira{}).ValidateConfig(core.Configuration{
		core.ConfigKeyTarget:  target,
		confKeyBaseUrl:        "https://jira.example.com",
		confKeyProject:        "PROJ",
		confKeyCredentialType: "TOKEN",
	})
	require.Error(t, err)
}

func newJiraCredentialRepo(t *testing.T) (*repository.GitRepo, *cache.RepoCache) {
	t.Helper()

	home, err := ioutil.TempDir("", "git-bug-home-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(home) })

	prevHome := os.Getenv("HOME")
	require.NoError(t, os.Setenv("HOME", home))
	t.Cleanup(func() { _ = os.Setenv("HOME", prevHome) })

	repo := repository.CreateTestRepo(false)
	t.Cleanup(func() { repository.CleanupTestRepos(t, repo) })

	backend, err := cache.NewRepoCache(repo)
	require.NoError(t, err)
	return repo, backend
}

func storeJiraCred(t *testing.T, repo repository.RepoConfig, login, password string) {
	t.Helper()
	cred := auth.NewLoginPassword(target, login, password)
	cred.SetMetadata(auth.MetaKeyLogin, login)
	cred.SetMetadata(auth.MetaKeyBaseURL, "https://jira.example.com")
	require.NoError(t, auth.Store(repo, cred))
}
