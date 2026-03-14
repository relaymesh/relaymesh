package worker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/validate"

	"github.com/relaymesh/relaymesh/pkg/auth"
	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
	cloudv1connect "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1/cloudv1connect"
)

// InstallationRecord mirrors the server installation response.
type InstallationRecord struct {
	Provider            string     `json:"provider"`
	AccountID           string     `json:"account_id"`
	AccountName         string     `json:"account_name"`
	InstallationID      string     `json:"installation_id"`
	ProviderInstanceKey string     `json:"provider_instance_key"`
	EnterpriseID        string     `json:"enterprise_id"`
	EnterpriseSlug      string     `json:"enterprise_slug"`
	EnterpriseName      string     `json:"enterprise_name"`
	AccessToken         string     `json:"access_token"`
	RefreshToken        string     `json:"refresh_token"`
	ExpiresAt           *time.Time `json:"expires_at"`
}

// InstallationsClient fetches installation records from the server API.
type InstallationsClient struct {
	BaseURL    string
	HTTPClient *http.Client
	APIKey     string
	OAuth2     *auth.OAuth2Config
}

// GetByInstallationID fetches the latest installation record by provider + installation_id.
func (c *InstallationsClient) GetByInstallationID(ctx context.Context, provider, installationID string) (*InstallationRecord, error) {
	if installationID == "" {
		return nil, errors.New("installation_id is required")
	}
	if provider == "" {
		return nil, errors.New("provider is required")
	}
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return nil, errors.New("base url is required")
	}

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	interceptor := validate.NewInterceptor()
	connectClient := cloudv1connect.NewInstallationsServiceClient(
		client,
		base,
		connect.WithInterceptors(interceptor),
	)
	req := connect.NewRequest(&cloudv1.GetInstallationByIDRequest{
		Provider:       provider,
		InstallationId: installationID,
	})
	setAuthHeaders(ctx, req.Header(), c.APIKey, c.OAuth2)
	if tenantID := TenantIDFromContext(ctx); tenantID != "" {
		req.Header().Set("X-Tenant-ID", tenantID)
	}
	resp, err := connectClient.GetInstallationByID(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("installations api failed: %w", err)
	}
	if resp.Msg.GetInstallation() == nil {
		return nil, nil
	}
	record := fromProtoInstallation(resp.Msg.GetInstallation())
	return &record, nil
}

func fromProtoInstallation(record *cloudv1.InstallRecord) InstallationRecord {
	if record == nil {
		return InstallationRecord{}
	}
	expiresAt := record.GetExpiresAt()
	var expiresTime *time.Time
	if expiresAt != nil {
		t := expiresAt.AsTime()
		expiresTime = &t
	}
	return InstallationRecord{
		Provider:            record.GetProvider(),
		AccountID:           record.GetAccountId(),
		AccountName:         record.GetAccountName(),
		InstallationID:      record.GetInstallationId(),
		ProviderInstanceKey: record.GetProviderInstanceKey(),
		EnterpriseID:        record.GetEnterpriseId(),
		EnterpriseSlug:      record.GetEnterpriseSlug(),
		EnterpriseName:      record.GetEnterpriseName(),
		AccessToken:         record.GetAccessToken(),
		RefreshToken:        record.GetRefreshToken(),
		ExpiresAt:           expiresTime,
	}
}

// IsEnterprise reports whether the installation is tied to an enterprise account.
func (r InstallationRecord) IsEnterprise() bool {
	return r.EnterpriseID != "" || r.EnterpriseSlug != "" || r.EnterpriseName != ""
}
