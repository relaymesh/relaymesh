package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"connectrpc.com/connect"

	"github.com/relaymesh/relaymesh/pkg/auth"
	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
	"github.com/relaymesh/relaymesh/pkg/oauth"
	"github.com/relaymesh/relaymesh/pkg/providerinstance"
	ghprovider "github.com/relaymesh/relaymesh/pkg/providers/github"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

// SCMService returns per-installation SCM client credentials.
type SCMService struct {
	Store                 storage.Store
	ProviderInstanceStore storage.ProviderInstanceStore
	ProviderInstanceCache *providerinstance.Cache
	Providers             auth.Config
	Logger                *log.Logger
}

func (s *SCMService) GetSCMClient(
	ctx context.Context,
	req *connect.Request[cloudv1.GetSCMClientRequest],
) (*connect.Response[cloudv1.GetSCMClientResponse], error) {
	provider := auth.NormalizeProviderName(req.Msg.GetProvider())
	installationID := strings.TrimSpace(req.Msg.GetInstallationId())
	instanceKey := strings.TrimSpace(req.Msg.GetProviderInstanceKey())
	resolved, err := s.GetProviderClient(ctx, provider, installationID, instanceKey)
	if err != nil {
		logError(s.Logger, "provider client lookup failed", err)
		if strings.Contains(err.Error(), "required") {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		if strings.Contains(err.Error(), "not found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		if strings.Contains(err.Error(), "storage not configured") {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	if resolved == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("installation not found"))
	}

	resp := &cloudv1.GetSCMClientResponse{
		Client: &cloudv1.SCMClient{
			Provider:            resolved.Provider,
			ApiBaseUrl:          resolved.APIBaseURL,
			AccessToken:         resolved.AccessToken,
			ExpiresAt:           resolved.ExpiresAt,
			ProviderInstanceKey: resolved.ProviderInstanceKey,
		},
	}
	return connect.NewResponse(resp), nil
}

func (s *SCMService) resolveInstallation(
	ctx context.Context,
	provider string,
	installationID string,
	instanceKey string,
) (*storage.InstallRecord, error) {
	if instanceKey != "" {
		return s.Store.GetInstallationByInstallationIDAndInstanceKey(ctx, provider, installationID, instanceKey)
	}
	return s.Store.GetInstallationByInstallationID(ctx, provider, installationID)
}

func (s *SCMService) providerConfigFor(ctx context.Context, provider, instanceKey string) (auth.ProviderConfig, error) {
	instanceKey = strings.TrimSpace(instanceKey)
	if instanceKey != "" {
		if s.ProviderInstanceCache != nil {
			if cfg, ok, err := s.ProviderInstanceCache.ConfigFor(ctx, provider, instanceKey); err == nil && ok {
				return cfg, nil
			}
		}
		if s.ProviderInstanceStore != nil {
			record, err := s.ProviderInstanceStore.GetProviderInstance(ctx, provider, instanceKey)
			if err != nil {
				return auth.ProviderConfig{}, err
			}
			if record != nil {
				return providerinstance.ProviderConfigFromRecord(*record)
			}
		}
		return auth.ProviderConfig{}, fmt.Errorf("provider instance not found: %s", instanceKey)
	}
	return providerConfigFromAuthConfig(s.Providers, provider), nil
}

func (s *SCMService) clientForInstallation(
	ctx context.Context,
	provider string,
	providerCfg auth.ProviderConfig,
	record *storage.InstallRecord,
) (*cloudv1.SCMClient, error) {
	switch provider {
	case auth.ProviderGitHub:
		if providerCfg.App.AppID == 0 || (providerCfg.App.PrivateKeyPath == "" && providerCfg.App.PrivateKeyPEM == "") {
			return nil, errors.New("github app credentials missing")
		}
		installationID, err := strconv.ParseInt(strings.TrimSpace(record.InstallationID), 10, 64)
		if err != nil || installationID == 0 {
			return nil, errors.New("github installation id invalid")
		}
		token, err := ghprovider.FetchInstallationToken(ctx, ghprovider.AppConfig{
			AppID:          providerCfg.App.AppID,
			PrivateKeyPath: providerCfg.App.PrivateKeyPath,
			PrivateKeyPEM:  providerCfg.App.PrivateKeyPEM,
			BaseURL:        strings.TrimSpace(providerCfg.API.BaseURL),
		}, installationID)
		if err != nil {
			return nil, fmt.Errorf("github token exchange failed: %w", err)
		}
		return &cloudv1.SCMClient{
			Provider:            provider,
			ApiBaseUrl:          resolveAPIBase(provider, providerCfg),
			AccessToken:         token.Token,
			ExpiresAt:           toProtoTimestampPtr(token.ExpiresAt),
			ProviderInstanceKey: record.ProviderInstanceKey,
		}, nil
	case auth.ProviderGitLab:
		return s.tokenClient(ctx, provider, providerCfg, record)
	case auth.ProviderBitbucket:
		return s.tokenClient(ctx, provider, providerCfg, record)
	default:
		return nil, errors.New("unsupported provider")
	}
}

func (s *SCMService) tokenClient(
	ctx context.Context,
	provider string,
	providerCfg auth.ProviderConfig,
	record *storage.InstallRecord,
) (*cloudv1.SCMClient, error) {
	if record == nil {
		return nil, errors.New("installation not found")
	}
	if record.AccessToken == "" {
		return nil, errors.New("access token missing")
	}
	accessToken := record.AccessToken
	if shouldRefresh(record.ExpiresAt) && record.RefreshToken != "" {
		var refreshed oauth.TokenResult
		var err error
		switch provider {
		case auth.ProviderGitLab:
			refreshed, err = oauth.RefreshGitLabToken(ctx, providerCfg, record.RefreshToken)
		case auth.ProviderBitbucket:
			refreshed, err = oauth.RefreshBitbucketToken(ctx, providerCfg, record.RefreshToken)
		default:
			err = errors.New("unsupported provider for refresh")
		}
		if err != nil {
			return nil, fmt.Errorf("token refresh failed: %w", err)
		}
		accessToken = refreshed.AccessToken
		record.AccessToken = refreshed.AccessToken
		record.RefreshToken = refreshed.RefreshToken
		record.ExpiresAt = refreshed.ExpiresAt
		if err := s.Store.UpsertInstallation(ctx, *record); err != nil {
			logError(s.Logger, "token refresh persist failed", err)
		}
	}
	return &cloudv1.SCMClient{
		Provider:            provider,
		ApiBaseUrl:          resolveAPIBase(provider, providerCfg),
		AccessToken:         accessToken,
		ExpiresAt:           toProtoTimestampPtr(record.ExpiresAt),
		ProviderInstanceKey: record.ProviderInstanceKey,
	}, nil
}

func resolveAPIBase(provider string, cfg auth.ProviderConfig) string {
	base := strings.TrimSpace(cfg.API.BaseURL)
	if base != "" {
		return base
	}
	switch provider {
	case auth.ProviderGitHub:
		return "https://api.github.com"
	case auth.ProviderGitLab:
		return "https://gitlab.com/api/v4"
	case auth.ProviderBitbucket:
		return "https://api.bitbucket.org/2.0"
	default:
		return ""
	}
}
