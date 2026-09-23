package github

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"testing"
	"unsafe"

	"github.com/shurcooL/githubv4"
	"github.com/shurcooL/graphql"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/MichaelMure/git-bug/bridge/core"
	"github.com/MichaelMure/git-bug/bridge/core/auth"
	"github.com/MichaelMure/git-bug/cache"
	"github.com/MichaelMure/git-bug/repository"
)

func TestImportSelectsCredentialForDefaultLogin(t *testing.T) {
	repo, backend := newCredentialSelectRepo(t)
	defer backend.Close()

	storeToken(t, repo, "wrong-token", "other-user")
	ctx := context.Background()

	importer := &githubImporter{}
	err := importer.Init(ctx, backend, core.Configuration{
		confKeyOwner:        "owner",
		confKeyProject:      "project",
		confKeyDefaultLogin: "wanted-user",
	})
	require.Error(t, err)

	storeToken(t, repo, "right-token", "wanted-user")

	importer = &githubImporter{}
	err = importer.Init(ctx, backend, core.Configuration{
		confKeyOwner:        "owner",
		confKeyProject:      "project",
		confKeyDefaultLogin: "wanted-user",
	})
	require.NoError(t, err)
	require.Equal(t, "right-token", tokenValue(t, importer.client))
}

func TestExportSelectsCredentialForDefaultLogin(t *testing.T) {
	repo, backend := newCredentialSelectRepo(t)
	defer backend.Close()

	other, err := backend.NewIdentityRaw("other", "other@example.com", "", map[string]string{
		metaKeyGithubLogin: "other-user",
	})
	require.NoError(t, err)
	wanted, err := backend.NewIdentityRaw("wanted", "wanted@example.com", "", map[string]string{
		metaKeyGithubLogin: "wanted-user",
	})
	require.NoError(t, err)

	storeToken(t, repo, "wrong-token", "other-user")
	storeToken(t, repo, "right-token", "wanted-user")

	exporter := &githubExporter{}
	err = exporter.Init(context.Background(), backend, core.Configuration{
		confKeyOwner:        "owner",
		confKeyProject:      "project",
		confKeyDefaultLogin: "wanted-user",
	})
	require.NoError(t, err)
	require.Equal(t, "right-token", exporter.defaultToken.Value)
	require.Equal(t, "right-token", tokenValue(t, exporter.defaultClient))
	require.Equal(t, "right-token", tokenValue(t, exporter.identityClient[wanted.Id()]))
	require.Equal(t, "wrong-token", tokenValue(t, exporter.identityClient[other.Id()]))
}

func newCredentialSelectRepo(t *testing.T) (*repository.GitRepo, *cache.RepoCache) {
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

func storeToken(t *testing.T, repo repository.RepoConfig, value, login string) {
	t.Helper()
	token := auth.NewToken(target, value)
	token.SetMetadata(auth.MetaKeyLogin, login)
	require.NoError(t, auth.Store(repo, token))
}

func tokenValue(t *testing.T, client *githubv4.Client) string {
	t.Helper()
	gql := unexportedField(reflect.ValueOf(client).Elem(), "client").Interface().(*graphql.Client)
	httpClient := unexportedField(reflect.ValueOf(gql).Elem(), "httpClient").Interface().(*http.Client)
	transport, ok := httpClient.Transport.(*oauth2.Transport)
	require.True(t, ok)
	tok, err := transport.Source.Token()
	require.NoError(t, err)
	return tok.AccessToken
}

func unexportedField(v reflect.Value, name string) reflect.Value {
	f := v.FieldByName(name)
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
}

func TestGithubValidateConfigRequiresDefaultLogin(t *testing.T) {
	err := (&Github{}).ValidateConfig(core.Configuration{
		core.ConfigKeyTarget: target,
		confKeyOwner:         "owner",
		confKeyProject:       "project",
	})
	require.Error(t, err)

	err = (&Github{}).ValidateConfig(core.Configuration{
		core.ConfigKeyTarget: target,
		confKeyOwner:         "owner",
		confKeyProject:       "project",
		confKeyDefaultLogin:  "wanted-user",
	})
	require.NoError(t, err)
}
