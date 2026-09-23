package gitlab

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xanzy/go-gitlab"

	"github.com/MichaelMure/git-bug/bridge/core"
	"github.com/MichaelMure/git-bug/bridge/core/auth"
)

const (
	target = "gitlab"

	metaKeyGitlabId      = "gitlab-id"
	metaKeyGitlabUrl     = "gitlab-url"
	metaKeyGitlabLogin   = "gitlab-login"
	metaKeyGitlabProject = "gitlab-project-id"
	metaKeyGitlabBaseUrl = "gitlab-base-url"

	keyProjectID     = "project-id"
	keyGitlabBaseUrl = "base-url"

	defaultBaseURL = "https://gitlab.com/"
	defaultTimeout = 60 * time.Second
)

type Gitlab struct{}

func (*Gitlab) Target() string {
	return target
}

func (*Gitlab) NewImporter() core.Importer {
	return &gitlabImporter{}
}

func (*Gitlab) NewExporter() core.Exporter {
	return &gitlabExporter{}
}

func buildClient(baseURL string, token *auth.Token) (*gitlab.Client, error) {
	normalized, err := normalizeGitlabBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Timeout: defaultTimeout,
	}

	client := gitlab.NewClient(httpClient, token.Value)
	if err := client.SetBaseURL(normalized); err != nil {
		return nil, err
	}

	return client, nil
}

// normalizeGitlabBaseURL reduces a project URL or API base URL to scheme://host/.
// Self-hosted project URLs include a path, but GitLab API calls must target the
// instance root. Values without a trailing slash are accepted. An empty value
// falls back to gitlab.com so previously configured public bridges keep working.
func normalizeGitlabBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = defaultBaseURL
	}

	clean := strings.TrimSuffix(raw, ".git")
	clean = strings.Replace(clean, "git@", "https://", 1)

	parsed, err := url.Parse(clean)
	if err != nil {
		return "", err
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("invalid GitLab base URL %q", raw)
	}

	scheme := parsed.Scheme
	if scheme != "http" && scheme != "https" {
		scheme = "https"
	}

	return scheme + "://" + parsed.Host + "/", nil
}

func sameGitlabInstance(a, b string) (bool, error) {
	left, err := normalizeGitlabBaseURL(a)
	if err != nil {
		return false, err
	}
	right, err := normalizeGitlabBaseURL(b)
	if err != nil {
		return false, err
	}
	return left == right, nil
}
