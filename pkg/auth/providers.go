package auth

import "strings"

const (
	ProviderGitHub    = "github"
	ProviderGitLab    = "gitlab"
	ProviderBitbucket = "bitbucket"
	ProviderSlack     = "slack"
	ProviderAtlassian = "atlassian"
	ProviderJira      = "jira"
)

// NormalizeProviderName normalizes provider identifiers for comparisons.
func NormalizeProviderName(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	provider = strings.ReplaceAll(provider, "-", "_")
	if provider == ProviderJira {
		return ProviderAtlassian
	}
	return provider
}

// IsGitHubProvider reports whether the provider is GitHub.
func IsGitHubProvider(provider string) bool {
	switch NormalizeProviderName(provider) {
	case ProviderGitHub:
		return true
	default:
		return false
	}
}
