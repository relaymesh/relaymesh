package oauth

import (
	"net/http"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

// DefaultRegistry registers the built-in OAuth providers.
func DefaultRegistry() *Registry {
	registry := NewRegistry()
	_ = registry.Register(gitHubProvider{})
	_ = registry.Register(gitLabProvider{})
	_ = registry.Register(bitbucketProvider{})
	_ = registry.Register(slackProvider{})
	return registry
}

type gitHubProvider struct{}

type gitLabProvider struct{}

type bitbucketProvider struct{}

type slackProvider struct{}

func (gitHubProvider) Name() string {
	return "github"
}

func (gitHubProvider) CallbackPath() string {
	return "/auth/github/callback"
}

func (gitHubProvider) AuthorizeURL(_ *http.Request, cfg auth.ProviderConfig, state, _ string) (string, error) {
	return githubInstallURL(cfg, state)
}

func (gitHubProvider) NewHandler(cfg auth.ProviderConfig, opts HandlerOptions) http.Handler {
	return &Handler{
		Provider:              "github",
		Config:                cfg,
		Providers:             opts.Providers,
		Store:                 opts.Store,
		NamespaceStore:        opts.NamespaceStore,
		ProviderInstanceStore: opts.ProviderInstanceStore,
		ProviderInstanceCache: opts.ProviderInstanceCache,
		Logger:                opts.Logger,
		RedirectBase:          opts.RedirectBase,
		Endpoint:              opts.Endpoint,
	}
}

func (gitLabProvider) Name() string {
	return "gitlab"
}

func (gitLabProvider) CallbackPath() string {
	return "/auth/gitlab/callback"
}

func (gitLabProvider) AuthorizeURL(r *http.Request, cfg auth.ProviderConfig, state, endpoint string) (string, error) {
	redirectURL := callbackURL(r, "gitlab", endpoint)
	return gitlabAuthorizeURL(cfg, state, redirectURL)
}

func (gitLabProvider) NewHandler(cfg auth.ProviderConfig, opts HandlerOptions) http.Handler {
	return &Handler{
		Provider:              "gitlab",
		Config:                cfg,
		Providers:             opts.Providers,
		Store:                 opts.Store,
		NamespaceStore:        opts.NamespaceStore,
		ProviderInstanceStore: opts.ProviderInstanceStore,
		ProviderInstanceCache: opts.ProviderInstanceCache,
		Logger:                opts.Logger,
		RedirectBase:          opts.RedirectBase,
		Endpoint:              opts.Endpoint,
	}
}

func (bitbucketProvider) Name() string {
	return "bitbucket"
}

func (bitbucketProvider) CallbackPath() string {
	return "/auth/bitbucket/callback"
}

func (bitbucketProvider) AuthorizeURL(r *http.Request, cfg auth.ProviderConfig, state, endpoint string) (string, error) {
	redirectURL := callbackURL(r, "bitbucket", endpoint)
	return bitbucketAuthorizeURL(cfg, state, redirectURL)
}

func (bitbucketProvider) NewHandler(cfg auth.ProviderConfig, opts HandlerOptions) http.Handler {
	return &Handler{
		Provider:              "bitbucket",
		Config:                cfg,
		Providers:             opts.Providers,
		Store:                 opts.Store,
		NamespaceStore:        opts.NamespaceStore,
		ProviderInstanceStore: opts.ProviderInstanceStore,
		ProviderInstanceCache: opts.ProviderInstanceCache,
		Logger:                opts.Logger,
		RedirectBase:          opts.RedirectBase,
		Endpoint:              opts.Endpoint,
	}
}

func (slackProvider) Name() string {
	return "slack"
}

func (slackProvider) CallbackPath() string {
	return "/auth/slack/callback"
}

func (slackProvider) AuthorizeURL(r *http.Request, cfg auth.ProviderConfig, state, endpoint string) (string, error) {
	redirectURL := callbackURL(r, "slack", endpoint)
	return slackAuthorizeURL(cfg, state, redirectURL)
}

func (slackProvider) NewHandler(cfg auth.ProviderConfig, opts HandlerOptions) http.Handler {
	return &Handler{
		Provider:              "slack",
		Config:                cfg,
		Providers:             opts.Providers,
		Store:                 opts.Store,
		NamespaceStore:        opts.NamespaceStore,
		ProviderInstanceStore: opts.ProviderInstanceStore,
		ProviderInstanceCache: opts.ProviderInstanceCache,
		Logger:                opts.Logger,
		RedirectBase:          opts.RedirectBase,
		Endpoint:              opts.Endpoint,
	}
}
