package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
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

func TestGitlabImporterUsesDefaultLoginCredential(t *testing.T) {
	var mu sync.Mutex
	var receivedTokens []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedTokens = append(receivedTokens, r.Header.Get("Private-Token"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"username":"bob"}`))
	}))
	defer server.Close()

	baseURL := server.URL + "/"

	repo := repository.CreateTestRepo(false)
	defer repository.CleanupTestRepos(t, repo)

	backend, err := cache.NewRepoCache(&memGlobalConfigRepo{repo, repository.NewMemConfig()})
	require.NoError(t, err)
	defer backend.Close()

	for _, login := range []string{"alice", "bob", "carol"} {
		token := auth.NewToken(target, "token-of-"+login)
		token.SetMetadata(auth.MetaKeyLogin, login)
		token.SetMetadata(auth.MetaKeyBaseURL, baseURL)
		require.NoError(t, auth.Store(backend, token))
	}

	conf := core.Configuration{
		confKeyProjectID:     "1",
		confKeyGitlabBaseUrl: baseURL,
		confKeyDefaultLogin:  "bob",
	}

	// credentials are listed in random order, a single run could pass by chance
	const runs = 20
	for i := 0; i < runs; i++ {
		importer := &gitlabImporter{}
		require.NoError(t, importer.Init(context.Background(), backend, conf))

		_, _, err := importer.client.Users.CurrentUser()
		require.NoError(t, err)
	}

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, receivedTokens, runs)
	for _, token := range receivedTokens {
		require.Equal(t, "token-of-bob", token)
	}
}
