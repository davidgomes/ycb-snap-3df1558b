package github

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
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

type authCapturingTransport struct {
	authorizations []string
}

func (t *authCapturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.authorizations = append(t.authorizations, req.Header.Get("Authorization"))
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       ioutil.NopCloser(bytes.NewBufferString(`{"data":{"viewer":{"login":"bob"}}}`)),
		Request:    req,
	}, nil
}

func TestGithubImporterUsesDefaultLoginCredential(t *testing.T) {
	repo := repository.CreateTestRepo(false)
	defer repository.CleanupTestRepos(t, repo)

	backend, err := cache.NewRepoCache(&memGlobalConfigRepo{repo, repository.NewMemConfig()})
	require.NoError(t, err)
	defer backend.Close()

	for _, login := range []string{"alice", "bob", "carol"} {
		token := auth.NewToken(target, "token-of-"+login)
		token.SetMetadata(auth.MetaKeyLogin, login)
		require.NoError(t, auth.Store(backend, token))
	}

	// the oauth2 client built by the importer falls back to http.DefaultTransport
	transport := &authCapturingTransport{}
	defaultTransport := http.DefaultTransport
	http.DefaultTransport = transport
	defer func() { http.DefaultTransport = defaultTransport }()

	conf := core.Configuration{
		confKeyOwner:        "MichaelMure",
		confKeyProject:      "git-bug",
		confKeyDefaultLogin: "bob",
	}

	// credentials are listed in random order, a single run could pass by chance
	const runs = 20
	for i := 0; i < runs; i++ {
		importer := &githubImporter{}
		require.NoError(t, importer.Init(context.Background(), backend, conf))

		var q loginQuery
		require.NoError(t, importer.client.Query(context.Background(), &q, nil))
	}

	require.Len(t, transport.authorizations, runs)
	for _, authorization := range transport.authorizations {
		require.Equal(t, "Bearer token-of-bob", authorization)
	}
}
