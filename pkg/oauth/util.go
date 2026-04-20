package oauth

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"
)

func callbackURL(r *http.Request, provider, endpoint string) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint != "" {
		return fmt.Sprintf("%s/auth/%s/callback", endpoint, provider)
	}
	scheme := forwardedProto(r)
	host := forwardedHost(r)
	if scheme == "" {
		scheme = "http"
	}
	if host == "" {
		host = r.Host
	}
	if host == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s/auth/%s/callback", scheme, host, provider)
}

func forwardedProto(r *http.Request) string {
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	return ""
}

func forwardedHost(r *http.Request) string {
	if host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); host != "" {
		return host
	}
	return ""
}

func providerFromPath(path string) string {
	path = strings.TrimRight(path, "/")
	switch {
	case strings.HasSuffix(path, "/auth/github/callback"):
		return "github"
	case strings.HasSuffix(path, "/auth/gitlab/callback"):
		return "gitlab"
	case strings.HasSuffix(path, "/auth/bitbucket/callback"):
		return "bitbucket"
	case strings.HasSuffix(path, "/auth/slack/callback"):
		return "slack"
	default:
		return ""
	}
}

func randomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return fmt.Sprintf("%x", buf)
}
