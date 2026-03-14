package scm

import (
	"context"
	"errors"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/providers/bitbucket"
	"github.com/relaymesh/relaymesh/pkg/providers/github"
	"github.com/relaymesh/relaymesh/pkg/providers/gitlab"
)

// Client is a provider-specific API client instance.
// It is returned as an interface so callers can type-assert to the provider client
// without constructing it themselves.
type Client interface{}

// Factory builds SCM clients using resolved auth contexts.
type Factory struct {
	cfg auth.Config
}

// NewFactory creates a new Factory.
func NewFactory(cfg auth.Config) *Factory {
	return &Factory{cfg: cfg}
}

// NewClient creates a provider-specific client from an AuthContext.
func (f *Factory) NewClient(ctx context.Context, authCtx auth.AuthContext) (Client, error) {
	provider := auth.NormalizeProviderName(authCtx.Provider)
	switch provider {
	case auth.ProviderGitHub:
		cfg := f.cfg.GitHub
		return github.NewAppClient(ctx, github.AppConfig{
			AppID:          cfg.App.AppID,
			PrivateKeyPath: cfg.App.PrivateKeyPath,
			PrivateKeyPEM:  cfg.App.PrivateKeyPEM,
			BaseURL:        cfg.API.BaseURL,
		}, authCtx.InstallationID)
	case auth.ProviderGitLab:
		return gitlab.NewTokenClient(f.cfg.GitLab, authCtx.Token)
	case auth.ProviderBitbucket:
		return bitbucket.NewTokenClient(f.cfg.Bitbucket, authCtx.Token)
	default:
		return nil, errors.New("unsupported provider for scm client")
	}
}
