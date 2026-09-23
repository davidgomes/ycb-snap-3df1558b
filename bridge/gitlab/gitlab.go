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
	httpClient := &http.Client{
		Timeout: defaultTimeout,
	}

	normalized, err := normalizeGitlabBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	gitlabClient := gitlab.NewClient(httpClient, token.Value)
	if err := gitlabClient.SetBaseURL(normalized); err != nil {
		return nil, err
	}

	return gitlabClient, nil
}

// normalizeGitlabBaseURL reduces a GitLab URL to scheme+host with a trailing slash.
// Project paths must not be part of the API base: go-gitlab appends api/v4 itself.
// An empty value falls back to gitlab.com so existing bridges keep working.
func normalizeGitlabBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultBaseURL, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid gitlab base url: %q", raw)
	}

	return fmt.Sprintf("%s://%s/", parsed.Scheme, parsed.Host), nil
}

// projectBaseURL derives the API base (scheme+host) from a project or remote URL.
func projectBaseURL(projectURL string) (string, error) {
	clean := strings.TrimSuffix(strings.TrimSpace(projectURL), ".git")
	clean = strings.Replace(clean, "git@", "https://", 1)
	return normalizeGitlabBaseURL(clean)
}
