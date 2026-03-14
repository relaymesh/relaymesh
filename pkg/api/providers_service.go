package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"strings"

	"connectrpc.com/connect"

	"github.com/relaymesh/relaymesh/pkg/auth"
	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
	"github.com/relaymesh/relaymesh/pkg/providerinstance"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

const (
	providerInstanceHashBytes    = 32
	providerInstanceHashAttempts = 5
)

// ProvidersService handles CRUD for provider instances.
type ProvidersService struct {
	Store  storage.ProviderInstanceStore
	Cache  *providerinstance.Cache
	Logger *log.Logger
}

func (s *ProvidersService) ListProviders(
	ctx context.Context,
	req *connect.Request[cloudv1.ListProvidersRequest],
) (*connect.Response[cloudv1.ListProvidersResponse], error) {
	_ = req
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	provider := auth.NormalizeProviderName(req.Msg.GetProvider())
	records, err := s.Store.ListProviderInstances(ctx, provider)
	if err != nil {
		logError(s.Logger, "list provider instances failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("list provider instances failed"))
	}
	resp := &cloudv1.ListProvidersResponse{
		Providers: toProtoProviderRecords(records),
	}
	return connect.NewResponse(resp), nil
}

func (s *ProvidersService) GetProvider(
	ctx context.Context,
	req *connect.Request[cloudv1.GetProviderRequest],
) (*connect.Response[cloudv1.GetProviderResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	provider := auth.NormalizeProviderName(req.Msg.GetProvider())
	hash := strings.TrimSpace(req.Msg.GetHash())
	record, err := s.Store.GetProviderInstance(ctx, provider, hash)
	if err != nil {
		logError(s.Logger, "get provider instance failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("get provider instance failed"))
	}
	resp := &cloudv1.GetProviderResponse{
		Provider: toProtoProviderRecord(record),
	}
	return connect.NewResponse(resp), nil
}

func (s *ProvidersService) UpsertProvider(
	ctx context.Context,
	req *connect.Request[cloudv1.UpsertProviderRequest],
) (*connect.Response[cloudv1.UpsertProviderResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	provider := req.Msg.GetProvider()
	providerName := auth.NormalizeProviderName(provider.GetProvider())
	hash := strings.TrimSpace(provider.GetHash())
	configJSON := strings.TrimSpace(provider.GetConfigJson())
	if providerName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider is required"))
	}
	existing, err := s.Store.ListProviderInstances(ctx, providerName)
	if err != nil {
		logError(s.Logger, "list provider instances failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("list provider instances failed"))
	}
	if hash == "" {
		if len(existing) > 0 {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("provider instance already exists"))
		}
		var err error
		hash, err = generateProviderInstanceHash(ctx, s.Store, providerName)
		if err != nil {
			logError(s.Logger, "generate provider instance hash failed", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("generate provider instance hash failed"))
		}
	} else if len(existing) > 0 {
		found := false
		for _, item := range existing {
			if item.Key == hash {
				found = true
				break
			}
		}
		if !found {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("provider instance already exists"))
		}
	}
	if configJSON == "" {
		record, err := s.Store.GetProviderInstance(ctx, providerName, hash)
		if err != nil {
			logError(s.Logger, "get provider instance failed", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("get provider instance failed"))
		}
		if record != nil {
			configJSON = strings.TrimSpace(record.ConfigJSON)
		}
	}
	record, err := s.Store.UpsertProviderInstance(ctx, storage.ProviderInstanceRecord{
		Provider:        providerName,
		Key:             hash,
		ConfigJSON:      configJSON,
		RedirectBaseURL: strings.TrimSpace(provider.GetRedirectBaseUrl()),
		Enabled:         provider.GetEnabled(),
	})
	if err != nil {
		logError(s.Logger, "upsert provider instance failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("upsert provider instance failed"))
	}
	if s.Cache != nil {
		if err := s.Cache.Refresh(ctx); err != nil {
			logError(s.Logger, "provider instance cache refresh failed", err)
		}
	}
	resp := &cloudv1.UpsertProviderResponse{
		Provider: toProtoProviderRecord(record),
	}
	return connect.NewResponse(resp), nil
}

func (s *ProvidersService) DeleteProvider(
	ctx context.Context,
	req *connect.Request[cloudv1.DeleteProviderRequest],
) (*connect.Response[cloudv1.DeleteProviderResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	provider := auth.NormalizeProviderName(req.Msg.GetProvider())
	hash := strings.TrimSpace(req.Msg.GetHash())
	if err := s.Store.DeleteProviderInstance(ctx, provider, hash); err != nil {
		logError(s.Logger, "delete provider instance failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("delete provider instance failed"))
	}
	if s.Cache != nil {
		if err := s.Cache.Refresh(ctx); err != nil {
			logError(s.Logger, "provider instance cache refresh failed", err)
		}
	}
	return connect.NewResponse(&cloudv1.DeleteProviderResponse{}), nil
}

func generateProviderInstanceHash(ctx context.Context, store storage.ProviderInstanceStore, provider string) (string, error) {
	for i := 0; i < providerInstanceHashAttempts; i++ {
		hash, err := randomHex(providerInstanceHashBytes)
		if err != nil {
			return "", err
		}
		existing, err := store.GetProviderInstance(ctx, provider, hash)
		if err != nil {
			return "", err
		}
		if existing == nil {
			return hash, nil
		}
	}
	return "", errors.New("unable to generate unique provider instance hash")
}

func randomHex(size int) (string, error) {
	if size <= 0 {
		return "", errors.New("random hex size must be positive")
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
