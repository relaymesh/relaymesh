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
	"github.com/relaymesh/relaymesh/pkg/storage"
)

// InstallationsService implements the Connect/GRPC InstallationsService.
type InstallationsService struct {
	Store     storage.Store
	Providers auth.Config
	Logger    *log.Logger
}

func (s *InstallationsService) ListInstallations(
	ctx context.Context,
	req *connect.Request[cloudv1.ListInstallationsRequest],
) (*connect.Response[cloudv1.ListInstallationsResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	stateID := strings.TrimSpace(req.Msg.GetStateId())
	provider := auth.NormalizeProviderName(req.Msg.GetProvider())
	providers := []string{provider}
	if provider == "" {
		providers = []string{
			auth.ProviderGitHub,
			auth.ProviderGitLab,
			auth.ProviderBitbucket,
		}
	}
	if s.Logger != nil {
		s.Logger.Printf("installations list provider=%s state_id=%s tenant=%s", provider, stateID, storage.TenantFromContext(ctx))
	}

	var records []storage.InstallRecord
	for _, item := range providers {
		if strings.TrimSpace(item) == "" {
			continue
		}
		items, err := s.Store.ListInstallations(ctx, item, stateID)
		if err != nil {
			logError(s.Logger, "list installations failed", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("list installations failed"))
		}
		records = append(records, items...)
	}

	resp := &cloudv1.ListInstallationsResponse{
		Installations: toProtoInstallations(records),
	}
	return connect.NewResponse(resp), nil
}

func (s *InstallationsService) GetInstallationByID(
	ctx context.Context,
	req *connect.Request[cloudv1.GetInstallationByIDRequest],
) (*connect.Response[cloudv1.GetInstallationByIDResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	provider := auth.NormalizeProviderName(req.Msg.GetProvider())
	installationID := strings.TrimSpace(req.Msg.GetInstallationId())
	record, err := s.Store.GetInstallationByInstallationID(ctx, provider, installationID)
	if err != nil {
		logError(s.Logger, "get installation failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("get installation failed"))
	}
	if record == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("installation not found"))
	}
	resp := &cloudv1.GetInstallationByIDResponse{
		Installation: toProtoInstallation(*record),
	}
	return connect.NewResponse(resp), nil
}

func (s *InstallationsService) UpsertInstallation(
	ctx context.Context,
	req *connect.Request[cloudv1.UpsertInstallationRequest],
) (*connect.Response[cloudv1.UpsertInstallationResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	install := req.Msg.GetInstallation()
	provider := auth.NormalizeProviderName(install.GetProvider())
	record := storage.InstallRecord{
		Provider:            provider,
		AccountID:           strings.TrimSpace(install.GetAccountId()),
		AccountName:         strings.TrimSpace(install.GetAccountName()),
		InstallationID:      strings.TrimSpace(install.GetInstallationId()),
		ProviderInstanceKey: strings.TrimSpace(install.GetProviderInstanceKey()),
		EnterpriseID:        strings.TrimSpace(install.GetEnterpriseId()),
		EnterpriseSlug:      strings.TrimSpace(install.GetEnterpriseSlug()),
		EnterpriseName:      strings.TrimSpace(install.GetEnterpriseName()),
		AccessToken:         strings.TrimSpace(install.GetAccessToken()),
		RefreshToken:        strings.TrimSpace(install.GetRefreshToken()),
		ExpiresAt:           fromProtoTimestampPtr(install.GetExpiresAt()),
		MetadataJSON:        strings.TrimSpace(install.GetMetadataJson()),
	}
	if record.EnterpriseID == "" && record.EnterpriseSlug == "" && record.EnterpriseName == "" {
		existing, err := s.Store.GetInstallationByInstallationID(ctx, provider, record.InstallationID)
		if err == nil && existing != nil {
			record.EnterpriseID = existing.EnterpriseID
			record.EnterpriseSlug = existing.EnterpriseSlug
			record.EnterpriseName = existing.EnterpriseName
		}
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	record.UpdatedAt = time.Now().UTC()
	if err := s.Store.UpsertInstallation(ctx, record); err != nil {
		logError(s.Logger, "upsert installation failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("upsert installation failed"))
	}
	resp := &cloudv1.UpsertInstallationResponse{
		Installation: toProtoInstallation(record),
	}
	return connect.NewResponse(resp), nil
}

func (s *InstallationsService) DeleteInstallation(
	ctx context.Context,
	req *connect.Request[cloudv1.DeleteInstallationRequest],
) (*connect.Response[cloudv1.DeleteInstallationResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	provider := auth.NormalizeProviderName(req.Msg.GetProvider())
	accountID := strings.TrimSpace(req.Msg.GetAccountId())
	installationID := strings.TrimSpace(req.Msg.GetInstallationId())
	instanceKey := strings.TrimSpace(req.Msg.GetProviderInstanceKey())
	if err := s.Store.DeleteInstallation(ctx, provider, accountID, installationID, instanceKey); err != nil {
		logError(s.Logger, "delete installation failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("delete installation failed"))
	}
	return connect.NewResponse(&cloudv1.DeleteInstallationResponse{}), nil
}
