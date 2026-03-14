package webhook

import (
	"fmt"
	"net/http"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

type gitHubProvider struct{}

type gitLabProvider struct{}

type bitbucketProvider struct{}

// DefaultRegistry registers the built-in webhook providers.
func DefaultRegistry() *Registry {
	registry := NewRegistry()
	_ = registry.Register(gitHubProvider{})
	_ = registry.Register(gitLabProvider{})
	_ = registry.Register(bitbucketProvider{})
	return registry
}

func (gitHubProvider) Name() string {
	return "github"
}

func (gitHubProvider) WebhookPath(cfg auth.ProviderConfig) string {
	return cfg.Webhook.Path
}

func (gitHubProvider) NewHandler(cfg auth.ProviderConfig, opts HandlerOptions) (http.Handler, error) {
	return NewGitHubHandler(
		cfg.Webhook.Secret,
		opts.Rules,
		opts.Publisher,
		opts.Logger,
		opts.MaxBodyBytes,
		opts.DebugEvents,
		opts.InstallStore,
		opts.NamespaceStore,
		opts.EventLogStore,
		opts.RuleStore,
		opts.DriverStore,
		opts.RulesStrict,
		opts.DynamicDriverCache,
		cfg,
	)
}

func (gitHubProvider) WebhookLogFields(cfg auth.ProviderConfig) string {
	return fmt.Sprintf("app_id=%d", cfg.App.AppID)
}

func (gitLabProvider) Name() string {
	return "gitlab"
}

func (gitLabProvider) WebhookPath(cfg auth.ProviderConfig) string {
	return cfg.Webhook.Path
}

func (gitLabProvider) NewHandler(cfg auth.ProviderConfig, opts HandlerOptions) (http.Handler, error) {
	return NewGitLabHandler(
		cfg.Webhook.Secret,
		opts.Rules,
		opts.Publisher,
		opts.Logger,
		opts.MaxBodyBytes,
		opts.DebugEvents,
		opts.NamespaceStore,
		opts.EventLogStore,
		opts.RuleStore,
		opts.DriverStore,
		opts.RulesStrict,
		opts.DynamicDriverCache,
	)
}

func (gitLabProvider) WebhookLogFields(cfg auth.ProviderConfig) string {
	_ = cfg
	return ""
}

func (bitbucketProvider) Name() string {
	return "bitbucket"
}

func (bitbucketProvider) WebhookPath(cfg auth.ProviderConfig) string {
	return cfg.Webhook.Path
}

func (bitbucketProvider) NewHandler(cfg auth.ProviderConfig, opts HandlerOptions) (http.Handler, error) {
	return NewBitbucketHandler(
		cfg.Webhook.Secret,
		opts.Rules,
		opts.Publisher,
		opts.Logger,
		opts.MaxBodyBytes,
		opts.DebugEvents,
		opts.NamespaceStore,
		opts.EventLogStore,
		opts.RuleStore,
		opts.DriverStore,
		opts.RulesStrict,
		opts.DynamicDriverCache,
	)
}

func (bitbucketProvider) WebhookLogFields(cfg auth.ProviderConfig) string {
	_ = cfg
	return ""
}
