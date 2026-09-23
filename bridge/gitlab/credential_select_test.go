package gitlab

import (
	"context"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"github.com/xanzy/go-gitlab"

	"github.com/MichaelMure/git-bug/bridge/core"
	"github.com/MichaelMure/git-bug/bridge/core/auth"
	"github.com/MichaelMure/git-bug/cache"
	"github.com/MichaelMure/git-bug/repository"
)

func TestGitlabImportSelectsCredentialForDefaultLogin(t *testing.T) {
	repo, backend := newGitlabCredentialRepo(t)
	defer backend.Close()

	storeGitlabToken(t, repo, "wrong-token", "other-user")

	importer := &gitlabImporter{}
	err := importer.Init(context.Background(), backend, core.Configuration{
		confKeyProjectID:     "1",
		confKeyGitlabBaseUrl: defaultBaseURL,
		confKeyDefaultLogin:  "wanted-user",
	})
	require.Error(t, err)

	storeGitlabToken(t, repo, "right-token", "wanted-user")

	importer = &gitlabImporter{}
	err = importer.Init(context.Background(), backend, core.Configuration{
		confKeyProjectID:     "1",
		confKeyGitlabBaseUrl: defaultBaseURL,
		confKeyDefaultLogin:  "wanted-user",
	})
	require.NoError(t, err)
	require.Equal(t, "right-token", gitlabToken(t, importer.client))
}

func TestGitlabExportUsesMatchingCredential(t *testing.T) {
	repo, backend := newGitlabCredentialRepo(t)
	defer backend.Close()

	other, err := backend.NewIdentityRaw("other", "other@example.com", "", map[string]string{
		metaKeyGitlabLogin: "other-user",
	})
	require.NoError(t, err)
	wanted, err := backend.NewIdentityRaw("wanted", "wanted@example.com", "", map[string]string{
		metaKeyGitlabLogin: "wanted-user",
	})
	require.NoError(t, err)

	storeGitlabToken(t, repo, "wrong-token", "other-user")
	storeGitlabToken(t, repo, "right-token", "wanted-user")

	exporter := &gitlabExporter{}
	err = exporter.Init(context.Background(), backend, core.Configuration{
		confKeyProjectID:     "1",
		confKeyGitlabBaseUrl: defaultBaseURL,
		confKeyDefaultLogin:  "wanted-user",
	})
	require.NoError(t, err)
	require.Equal(t, "wrong-token", gitlabToken(t, exporter.identityClient[other.Id()]))
	require.Equal(t, "right-token", gitlabToken(t, exporter.identityClient[wanted.Id()]))
}

func TestGitlabValidateConfigRequiresDefaultLogin(t *testing.T) {
	err := (&Gitlab{}).ValidateConfig(core.Configuration{
		core.ConfigKeyTarget: target,
		confKeyGitlabBaseUrl: defaultBaseURL,
		confKeyProjectID:     "1",
	})
	require.Error(t, err)
}

func newGitlabCredentialRepo(t *testing.T) (*repository.GitRepo, *cache.RepoCache) {
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

func storeGitlabToken(t *testing.T, repo repository.RepoConfig, value, login string) {
	t.Helper()
	token := auth.NewToken(target, value)
	token.SetMetadata(auth.MetaKeyLogin, login)
	token.SetMetadata(auth.MetaKeyBaseURL, defaultBaseURL)
	require.NoError(t, auth.Store(repo, token))
}

func gitlabToken(t *testing.T, client *gitlab.Client) string {
	t.Helper()
	require.NotNil(t, client)
	f := reflect.ValueOf(client).Elem().FieldByName("token")
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().String()
}
