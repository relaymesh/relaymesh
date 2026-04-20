package api

import (
	"context"
	"errors"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/auth"
	providerspkg "github.com/relaymesh/relaymesh/pkg/providers"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ProviderClient is a provider-agnostic authenticated client descriptor.
// It is intentionally generic so non-SCM providers can reuse the same contract.
type ProviderClient struct {
	Provider            string
	ProviderType        string
	APIBaseURL          string
	AccessToken         string
	ExpiresAt           *timestamppb.Timestamp
	ProviderInstanceKey string
}

// ProviderClientResolver defines provider-agnostic client resolution.
type ProviderClientResolver interface {
	GetProviderClient(ctx context.Context, provider, installationID, providerInstanceKey string) (*ProviderClient, error)
}

// GetProviderClient resolves an authenticated provider client for the installation.
func (s *SCMService) GetProviderClient(ctx context.Context, provider, installationID, providerInstanceKey string) (*ProviderClient, error) {
	if s.Store == nil {
		return nil, errors.New("storage not configured")
	}
	provider = auth.NormalizeProviderName(provider)
	installationID = strings.TrimSpace(installationID)
	providerInstanceKey = strings.TrimSpace(providerInstanceKey)
	if provider == "" || installationID == "" {
		return nil, errors.New("provider and installation_id are required")
	}

	record, err := s.resolveInstallation(ctx, provider, installationID, providerInstanceKey)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, errors.New("installation not found")
	}
	if providerInstanceKey == "" {
		providerInstanceKey = strings.TrimSpace(record.ProviderInstanceKey)
	}

	providerCfg, cfgErr := s.providerConfigFor(ctx, provider, providerInstanceKey)
	if cfgErr != nil {
		return nil, cfgErr
	}

	client, err := s.clientForInstallation(ctx, provider, providerCfg, record)
	if err != nil {
		return nil, err
	}

	return &ProviderClient{
		Provider:            provider,
		ProviderType:        string(providerspkg.ProviderTypeFor(provider)),
		APIBaseURL:          client.GetApiBaseUrl(),
		AccessToken:         client.GetAccessToken(),
		ExpiresAt:           client.GetExpiresAt(),
		ProviderInstanceKey: client.GetProviderInstanceKey(),
	}, nil
}
