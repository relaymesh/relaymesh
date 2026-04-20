package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/google/go-github/v57/github"
	"github.com/ktrysmt/go-bitbucket"
	"github.com/xanzy/go-gitlab"
	"golang.org/x/oauth2"
)

// SlackAPIClient is a minimal Slack Web API client used by workers.
type SlackAPIClient struct {
	token   string
	baseURL string
	client  *http.Client
}

// GitHubClient returns the GitHub SDK client from an event, if available.
func GitHubClient(evt *Event) (*github.Client, bool) {
	if evt == nil || evt.Client == nil {
		return nil, false
	}
	client, ok := evt.Client.(*github.Client)
	return client, ok
}

// GitLabClient returns the GitLab SDK client from an event, if available.
//
//nolint:staticcheck // legacy go-gitlab client retained for compatibility
func GitLabClient(evt *Event) (*gitlab.Client, bool) {
	if evt == nil || evt.Client == nil {
		return nil, false
	}
	client, ok := evt.Client.(*gitlab.Client)
	return client, ok
}

// BitbucketClient returns the Bitbucket SDK client from an event, if available.
func BitbucketClient(evt *Event) (*bitbucket.Client, bool) {
	if evt == nil || evt.Client == nil {
		return nil, false
	}
	client, ok := evt.Client.(*bitbucket.Client)
	return client, ok
}

// SlackClient returns the Slack SDK-like client from an event, if available.
func SlackClientFromEvent(evt *Event) (*SlackAPIClient, bool) {
	if evt == nil || evt.Client == nil {
		return nil, false
	}
	client, ok := evt.Client.(*SlackAPIClient)
	return client, ok
}

// SlackClient returns the Slack client from an event, if available.
func SlackClient(evt *Event) (*SlackAPIClient, bool) {
	return SlackClientFromEvent(evt)
}

func GitHubClientFromEvent(evt *Event) (*github.Client, bool) {
	return GitHubClient(evt)
}

//nolint:staticcheck // legacy go-gitlab client retained for compatibility
func GitLabClientFromEvent(evt *Event) (*gitlab.Client, bool) {
	return GitLabClient(evt)
}

func BitbucketClientFromEvent(evt *Event) (*bitbucket.Client, bool) {
	return BitbucketClient(evt)
}

func NewProviderClient(provider, token, baseURL string) (interface{}, error) {
	return newProviderClient(provider, token, baseURL)
}

func newProviderClient(provider, token, baseURL string) (interface{}, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "github":
		return newGitHubClient(token, baseURL)
	case "gitlab":
		return newGitLabClient(token, baseURL)
	case "bitbucket":
		return newBitbucketClient(token, baseURL)
	case "slack":
		return newSlackClient(token, baseURL)
	default:
		return nil, errors.New("unsupported provider")
	}
}

func newGitHubClient(token, baseURL string) (*github.Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("github token is required")
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(context.Background(), ts)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	client := github.NewClient(httpClient)
	if baseURL == "" || baseURL == defaultGitHubAPIBase {
		return client, nil
	}
	uploadURL := enterpriseUploadURL(baseURL)
	return client.WithEnterpriseURLs(baseURL, uploadURL)
}

//nolint:staticcheck
func newGitLabClient(token, baseURL string) (*gitlab.Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("gitlab token is required")
	}
	opts := []gitlab.ClientOptionFunc{}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultGitLabAPIBase
	}
	if baseURL != "" {
		opts = append(opts, gitlab.WithBaseURL(baseURL))
	}
	return gitlab.NewClient(token, opts...) //nolint:staticcheck // legacy go-gitlab client retained for compatibility
}

func newBitbucketClient(token, baseURL string) (*bitbucket.Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("bitbucket token is required")
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultBitbucketAPIBase
	}
	if baseURL != "" {
		_ = os.Setenv("BITBUCKET_API_BASE_URL", baseURL)
	}
	return bitbucket.NewOAuthbearerToken(token)
}

func newSlackClient(token, baseURL string) (*SlackAPIClient, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("slack token is required")
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultSlackAPIBase
	}
	return &SlackAPIClient{
		token:   token,
		baseURL: baseURL,
		client:  &http.Client{},
	}, nil
}

// Request performs an authenticated HTTP request against Slack API.
func (c *SlackAPIClient) Request(ctx context.Context, method, path string, body any, headers map[string]string) (*http.Response, error) {
	if c == nil {
		return nil, errors.New("slack client is nil")
	}
	url := resolveProviderURL(c.baseURL, path)
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(strings.TrimSpace(method)), url, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	httpClient := c.client
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return httpClient.Do(req)
}

// RequestJSON performs an authenticated Slack API request and decodes the JSON response.
func (c *SlackAPIClient) RequestJSON(ctx context.Context, method, path string, body any, headers map[string]string) (map[string]any, error) {
	resp, err := c.Request(ctx, method, path, body, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("slack request failed: %d %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func resolveProviderURL(baseURL, path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	normalizedBase := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.TrimSpace(path) == "" {
		return normalizedBase
	}
	normalizedPath := path
	if !strings.HasPrefix(normalizedPath, "/") {
		normalizedPath = "/" + normalizedPath
	}
	return normalizedBase + normalizedPath
}

func enterpriseUploadURL(apiBase string) string {
	base := strings.TrimRight(apiBase, "/")
	switch {
	case strings.HasSuffix(base, "/api/v3"):
		return strings.TrimSuffix(base, "/api/v3") + "/api/uploads"
	case strings.HasSuffix(base, "/api"):
		return strings.TrimSuffix(base, "/api") + "/api/uploads"
	default:
		return base
	}
}

const (
	defaultGitHubAPIBase    = "https://api.github.com"
	defaultGitLabAPIBase    = "https://gitlab.com/api/v4"
	defaultBitbucketAPIBase = "https://api.bitbucket.org/2.0"
	defaultSlackAPIBase     = "https://slack.com/api"
)
