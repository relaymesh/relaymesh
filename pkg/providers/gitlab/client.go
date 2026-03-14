package gitlab

import (
	"errors"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/auth"

	gl "github.com/xanzy/go-gitlab"
)

//nolint:staticcheck // legacy go-gitlab client retained for compatibility
type Client = gl.Client

// NewTokenClient returns a GitLab SDK client.
func NewTokenClient(cfg auth.ProviderConfig, token string) (*Client, error) {
	if token == "" {
		return nil, errors.New("gitlab access token is required")
	}
	opts := []gl.ClientOptionFunc{}
	if base := normalizeBaseURL(cfg.API.BaseURL); base != "" {
		opts = append(opts, gl.WithBaseURL(base))
	}
	return gl.NewClient(token, opts...) //nolint:staticcheck // legacy go-gitlab client retained for compatibility
}

func normalizeBaseURL(base string) string {
	if base == "" {
		return "https://gitlab.com/api/v4"
	}
	return strings.TrimRight(base, "/")
}
