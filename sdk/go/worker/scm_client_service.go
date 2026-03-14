package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/validate"

	"github.com/relaymesh/relaymesh/pkg/auth"
	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
	cloudv1connect "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1/cloudv1connect"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

// SCMClientRecord holds server-issued credentials for building SCM clients.
type SCMClientRecord struct {
	Provider            string
	APIBaseURL          string
	AccessToken         string
	ExpiresAt           time.Time
	ProviderInstanceKey string
}

// SCMClientsClient fetches SCM client credentials from the server API.
type SCMClientsClient struct {
	BaseURL    string
	HTTPClient *http.Client
	APIKey     string
	OAuth2     *auth.OAuth2Config
}

// GetSCMClient fetches SCM client credentials for a provider installation.
func (c *SCMClientsClient) GetSCMClient(
	ctx context.Context,
	provider string,
	installationID string,
	instanceKey string,
) (*SCMClientRecord, error) {
	provider = strings.TrimSpace(provider)
	installationID = strings.TrimSpace(installationID)
	if provider == "" || installationID == "" {
		return nil, errors.New("provider and installation_id are required")
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
	connectClient := cloudv1connect.NewSCMServiceClient(
		client,
		base,
		connect.WithInterceptors(interceptor),
	)
	req := connect.NewRequest(&cloudv1.GetSCMClientRequest{
		Provider:            provider,
		InstallationId:      installationID,
		ProviderInstanceKey: strings.TrimSpace(instanceKey),
	})
	setAuthHeaders(ctx, req.Header(), c.APIKey, c.OAuth2)
	if tenantID := storage.TenantFromContext(ctx); tenantID != "" {
		req.Header().Set("X-Tenant-ID", tenantID)
	}
	log.Printf("scm api request GetSCMClient base=%s tenant=%s provider=%s installation_id=%s", base, storage.TenantFromContext(ctx), provider, installationID)
	resp, err := connectClient.GetSCMClient(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("scm api failed: %w", err)
	}
	record := resp.Msg.GetClient()
	if record == nil {
		return nil, errors.New("scm client missing in response")
	}
	out := &SCMClientRecord{
		Provider:            record.GetProvider(),
		APIBaseURL:          strings.TrimSpace(record.GetApiBaseUrl()),
		AccessToken:         record.GetAccessToken(),
		ProviderInstanceKey: strings.TrimSpace(record.GetProviderInstanceKey()),
	}
	if ts := record.GetExpiresAt(); ts != nil {
		out.ExpiresAt = ts.AsTime()
	}
	return out, nil
}
