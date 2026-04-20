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
	default:
		return cfg.GitHub
	}
}
