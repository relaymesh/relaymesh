package auth

import (
	"context"
	"errors"

	"github.com/relaymesh/relaymesh/pkg/providers/github"
)

// AuthContext contains the resolved authentication data for a webhook event.
type AuthContext struct {
	Provider       string
	InstallationID int64
	Token          string
}

// EventContext captures the webhook context used for auth resolution.
type EventContext struct {
	Provider string
	Payload  []byte
}

// Resolver resolves authentication for a webhook event.
type Resolver interface {
	Resolve(ctx context.Context, event EventContext) (AuthContext, error)
}

// DefaultResolver resolves auth using configuration and webhook payload data.
type DefaultResolver struct {
	cfg Config
}

// NewResolver constructs a DefaultResolver.
func NewResolver(cfg Config) *DefaultResolver {
	return &DefaultResolver{cfg: cfg}
}

// Resolve builds an AuthContext from the webhook event.
func (r *DefaultResolver) Resolve(_ context.Context, event EventContext) (AuthContext, error) {
	provider := NormalizeProviderName(event.Provider)
	switch provider {
	case ProviderGitHub:
		if r.cfg.GitHub.App.AppID == 0 ||
			(r.cfg.GitHub.App.PrivateKeyPath == "" && r.cfg.GitHub.App.PrivateKeyPEM == "") {
			return AuthContext{}, errors.New("github app_id and private key are required")
		}
		installationID, ok, err := github.InstallationIDFromPayload(event.Payload)
		if err != nil {
			return AuthContext{}, err
		}
		if !ok {
			return AuthContext{}, errors.New("github installation id not found in payload")
		}
		return AuthContext{
			Provider:       ProviderGitHub,
			InstallationID: installationID,
		}, nil
	default:
		return AuthContext{}, errors.New("unsupported provider for auth resolution")
	}
}
