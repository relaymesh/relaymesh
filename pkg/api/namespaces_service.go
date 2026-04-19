package api

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/relaymesh/relaymesh/pkg/auth"
	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
	"github.com/relaymesh/relaymesh/pkg/oauth"
	"github.com/relaymesh/relaymesh/pkg/providerinstance"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

const maxNamespaceListSize = 1000

// NamespacesService implements the Connect/GRPC NamespacesService.
type NamespacesService struct {
	Store                 storage.NamespaceStore
	InstallStore          storage.Store
	ProviderInstanceStore storage.ProviderInstanceStore
	ProviderInstanceCache *providerinstance.Cache
	Providers             auth.Config
	Endpoint              string
	Logger                *log.Logger
}

func (s *NamespacesService) ListNamespaces(
	ctx context.Context,
	req *connect.Request[cloudv1.ListNamespacesRequest],
) (*connect.Response[cloudv1.ListNamespacesResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	stateID := strings.TrimSpace(req.Msg.GetStateId())
	provider := auth.NormalizeProviderName(req.Msg.GetProvider())

	filter := storage.NamespaceFilter{
		Provider: provider,
		Owner:    strings.TrimSpace(req.Msg.GetOwner()),
		RepoName: strings.TrimSpace(req.Msg.GetRepo()),
		FullName: strings.TrimSpace(req.Msg.GetFullName()),
		Limit:    maxNamespaceListSize,
	}
	if stateID != "" {
		filter.AccountID = stateID
	}
	records, err := s.Store.ListNamespaces(ctx, filter)
	if err != nil {
		logError(s.Logger, "list namespaces failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("list namespaces failed"))
	}

	resp := &cloudv1.ListNamespacesResponse{
		Namespaces: toProtoNamespaces(records),
	}
	return connect.NewResponse(resp), nil
}

func (s *NamespacesService) SyncNamespaces(
	ctx context.Context,
	req *connect.Request[cloudv1.SyncNamespacesRequest],
) (*connect.Response[cloudv1.SyncNamespacesResponse], error) {
	if s.InstallStore == nil || s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	stateID := strings.TrimSpace(req.Msg.GetStateId())
	provider := auth.NormalizeProviderName(req.Msg.GetProvider())

	installations, err := installationsForSync(ctx, s.InstallStore, provider, stateID)
	if err != nil {
		logError(s.Logger, "installation lookup failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("installation lookup failed"))
	}
	if len(installations) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("installation not found"))
	}

	for i := range installations {
		record := installations[i]
		providerCfg, cfgErr := s.providerConfigFor(ctx, provider, record.ProviderInstanceKey)
		if cfgErr != nil {
			logError(s.Logger, "provider config lookup failed", cfgErr)
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("provider config missing"))
		}
		if provider != auth.ProviderGitHub && record.AccessToken == "" {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("access token missing"))
		}
		accessToken := record.AccessToken
		if provider != auth.ProviderGitHub && shouldRefresh(record.ExpiresAt) && record.RefreshToken != "" {
			switch provider {
			case "gitlab":
				refreshed, err := oauth.RefreshGitLabToken(ctx, providerCfg, record.RefreshToken)
				if err != nil {
					logError(s.Logger, "gitlab token refresh failed", err)
					return nil, connect.NewError(connect.CodeInternal, errors.New("token refresh failed"))
				}
				accessToken = refreshed.AccessToken
				record.AccessToken = refreshed.AccessToken
				record.RefreshToken = refreshed.RefreshToken
				record.ExpiresAt = refreshed.ExpiresAt
			case "bitbucket":
				refreshed, err := oauth.RefreshBitbucketToken(ctx, providerCfg, record.RefreshToken)
				if err != nil {
					logError(s.Logger, "bitbucket token refresh failed", err)
					return nil, connect.NewError(connect.CodeInternal, errors.New("token refresh failed"))
				}
				accessToken = refreshed.AccessToken
				record.AccessToken = refreshed.AccessToken
				record.RefreshToken = refreshed.RefreshToken
				record.ExpiresAt = refreshed.ExpiresAt
			}
			if err := s.InstallStore.UpsertInstallation(ctx, record); err != nil {
				logError(s.Logger, "token refresh persist failed", err)
			}
		}

		switch provider {
		case auth.ProviderGitHub:
			// No remote sync for GitHub; namespaces come from install webhooks.
		case "gitlab":
			if err := oauth.SyncGitLabNamespaces(ctx, s.Store, providerCfg, accessToken, record.AccountID, record.InstallationID, record.ProviderInstanceKey); err != nil {
				logError(s.Logger, "gitlab namespace sync failed", err)
				return nil, connect.NewError(connect.CodeInternal, errors.New("namespace sync failed"))
			}
		case "bitbucket":
			if err := oauth.SyncBitbucketNamespaces(ctx, s.Store, providerCfg, accessToken, record.AccountID, record.InstallationID, record.ProviderInstanceKey); err != nil {
				logError(s.Logger, "bitbucket namespace sync failed", err)
				return nil, connect.NewError(connect.CodeInternal, errors.New("namespace sync failed"))
			}
		}
	}

	filter := storage.NamespaceFilter{
		Provider: provider,
		Limit:    maxNamespaceListSize,
	}
	if stateID != "" {
		filter.AccountID = stateID
	}
	records, err := s.Store.ListNamespaces(ctx, filter)
	if err != nil {
		logError(s.Logger, "list namespaces failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("list namespaces failed"))
	}
	resp := &cloudv1.SyncNamespacesResponse{
		Namespaces: toProtoNamespaces(records),
	}
	return connect.NewResponse(resp), nil
}

func (s *NamespacesService) GetNamespaceWebhook(
	ctx context.Context,
	req *connect.Request[cloudv1.GetNamespaceWebhookRequest],
) (*connect.Response[cloudv1.GetNamespaceWebhookResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	provider := auth.NormalizeProviderName(req.Msg.GetProvider())
	repoID := strings.TrimSpace(req.Msg.GetRepoId())
	stateID := strings.TrimSpace(req.Msg.GetStateId())

	record, err := s.Store.GetNamespace(ctx, provider, repoID, "")
	if err != nil {
		logError(s.Logger, "namespace lookup failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("namespace lookup failed"))
	}
	if record == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("namespace not found"))
	}
	if stateID != "" && record.AccountID != stateID {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("state_id mismatch"))
	}
	return connect.NewResponse(&cloudv1.GetNamespaceWebhookResponse{
		Enabled: record.WebhooksEnabled,
	}), nil
}

func (s *NamespacesService) SetNamespaceWebhook(
	ctx context.Context,
	req *connect.Request[cloudv1.SetNamespaceWebhookRequest],
) (*connect.Response[cloudv1.SetNamespaceWebhookResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	if s.InstallStore == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("installation storage not configured"))
	}
	provider := auth.NormalizeProviderName(req.Msg.GetProvider())
	repoID := strings.TrimSpace(req.Msg.GetRepoId())
	stateID := strings.TrimSpace(req.Msg.GetStateId())

	record, err := s.Store.GetNamespace(ctx, provider, repoID, "")
	if err != nil {
		logError(s.Logger, "namespace lookup failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("namespace lookup failed"))
	}
	if record == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("namespace not found"))
	}
	if stateID != "" && record.AccountID != stateID {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("state_id mismatch"))
	}
	if provider == auth.ProviderGitHub {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("github webhooks are always enabled"))
	}
	webhookURL, err := webhookURL(s.Endpoint, provider)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	install, err := installationForNamespace(ctx, s.InstallStore, provider, record, stateID)
	if err != nil {
		logError(s.Logger, "installation lookup failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("installation lookup failed"))
	}
	if install == nil || install.AccessToken == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("access token missing"))
	}
	providerCfg, cfgErr := s.providerConfigFor(ctx, provider, install.ProviderInstanceKey)
	if cfgErr != nil {
		logError(s.Logger, "provider config lookup failed", cfgErr)
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("provider config missing"))
	}

	if req.Msg.GetEnabled() {
		if err := enableProviderWebhook(ctx, provider, providerCfg, install.AccessToken, *record, webhookURL); err != nil {
			logError(s.Logger, "webhook enable failed", err)
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("webhook enable failed"))
		}
		record.WebhooksEnabled = true
	} else {
		if err := disableProviderWebhook(ctx, provider, providerCfg, install.AccessToken, *record, webhookURL); err != nil {
			logError(s.Logger, "webhook disable failed", err)
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("webhook disable failed"))
		}
		record.WebhooksEnabled = false
	}
	if err := s.Store.UpsertNamespace(ctx, *record); err != nil {
		logError(s.Logger, "namespace update failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("namespace update failed"))
	}

	return connect.NewResponse(&cloudv1.SetNamespaceWebhookResponse{
		Enabled: record.WebhooksEnabled,
	}), nil
}

func (s *NamespacesService) providerConfigFor(ctx context.Context, provider, instanceKey string) (auth.ProviderConfig, error) {
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
	}
	return providerConfigFromAuthConfig(s.Providers, provider), nil
}

func installationsForSync(ctx context.Context, store storage.Store, provider, stateID string) ([]storage.InstallRecord, error) {
	if store == nil {
		return nil, errors.New("store is not initialized")
	}
	if stateID != "" {
		record, err := latestInstallation(ctx, store, provider, stateID)
		if err != nil || record == nil {
			return nil, err
		}
		return []storage.InstallRecord{*record}, nil
	}
	return store.ListInstallations(ctx, provider, "")
}

func installationForNamespace(ctx context.Context, store storage.Store, provider string, record *storage.NamespaceRecord, stateID string) (*storage.InstallRecord, error) {
	if store == nil || record == nil {
		return nil, nil
	}
	if record.InstallationID != "" {
		found, err := store.GetInstallationByInstallationID(ctx, provider, record.InstallationID)
		if err != nil {
			return nil, err
		}
		if found != nil {
			return found, nil
		}
	}
	if record.AccountID != "" {
		return latestInstallation(ctx, store, provider, record.AccountID)
	}
	if stateID != "" {
		return latestInstallation(ctx, store, provider, stateID)
	}
	return nil, nil
}

func latestInstallation(ctx context.Context, store storage.Store, provider, accountID string) (*storage.InstallRecord, error) {
	records, err := store.ListInstallations(ctx, provider, accountID)
	if err != nil {
		return nil, err
	}
	var latest *storage.InstallRecord
	for i := range records {
		item := records[i]
		if latest == nil || item.UpdatedAt.After(latest.UpdatedAt) {
			copy := item
			latest = &copy
		}
	}
	return latest, nil
}

func shouldRefresh(expiresAt *time.Time) bool {
	if expiresAt == nil {
		return false
	}
	return time.Now().UTC().After(expiresAt.Add(-1 * time.Minute))
}

func webhookURL(endpoint, provider string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	endpoint = strings.TrimRight(endpoint, "/")
	if endpoint == "" {
		return "", errors.New("endpoint is required for webhook management")
	}
	switch provider {
	case "gitlab":
		return endpoint + "/webhooks/gitlab", nil
	case "bitbucket":
		return endpoint + "/webhooks/bitbucket", nil
	default:
		return "", errors.New("unsupported provider for webhook management")
	}
}
