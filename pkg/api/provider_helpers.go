package api

import "github.com/relaymesh/relaymesh/pkg/auth"

func providerConfigFromAuthConfig(cfg auth.Config, provider string) auth.ProviderConfig {
	switch auth.NormalizeProviderName(provider) {
	case auth.ProviderGitLab:
		return cfg.GitLab
	case auth.ProviderBitbucket:
		return cfg.Bitbucket
	case auth.ProviderSlack:
		return cfg.Slack
	case auth.ProviderAtlassian:
		if hasProviderConfig(cfg.Atlassian) {
			return cfg.Atlassian
		}
		return cfg.Jira
	case auth.ProviderJira:
		if hasProviderConfig(cfg.Atlassian) {
			return cfg.Atlassian
		}
		return cfg.Jira
	default:
		return cfg.GitHub
	}
}

func hasProviderConfig(cfg auth.ProviderConfig) bool {
	if cfg.Enabled {
		return true
	}
	if cfg.Key != "" || cfg.Webhook.Path != "" || cfg.Webhook.Secret != "" {
		return true
	}
	if cfg.App.AppID != 0 || cfg.App.PrivateKeyPath != "" || cfg.App.PrivateKeyPEM != "" || cfg.App.AppSlug != "" {
		return true
	}
	if cfg.OAuth.ClientID != "" || cfg.OAuth.ClientSecret != "" || len(cfg.OAuth.Scopes) > 0 {
		return true
	}
	if cfg.API.BaseURL != "" || cfg.API.WebBaseURL != "" {
		return true
	}
	return false
}
