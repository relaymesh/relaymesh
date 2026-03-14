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

// EventLogsClient updates event log status via the server API.
type EventLogsClient struct {
	BaseURL    string
	HTTPClient *http.Client
	APIKey     string
	OAuth2     *auth.OAuth2Config
}

// UpdateStatus updates the status for a single event log entry.
func (c *EventLogsClient) UpdateStatus(ctx context.Context, logID, status, errorMessage string) error {
	logID = strings.TrimSpace(logID)
	status = strings.TrimSpace(status)
	if logID == "" {
		return errors.New("log id is required")
	}
	if status == "" {
		return errors.New("status is required")
	}
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return errors.New("base url is required")
	}

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	interceptor := validate.NewInterceptor()
	connectClient := cloudv1connect.NewEventLogsServiceClient(
		client,
		base,
		connect.WithInterceptors(interceptor),
	)
	req := connect.NewRequest(&cloudv1.UpdateEventLogStatusRequest{
		LogId:        logID,
		Status:       status,
		ErrorMessage: strings.TrimSpace(errorMessage),
	})
	setAuthHeaders(ctx, req.Header(), c.APIKey, c.OAuth2)
	if tenantID := TenantIDFromContext(ctx); tenantID != "" {
		req.Header().Set("X-Tenant-ID", tenantID)
	}
	if _, err := connectClient.UpdateEventLogStatus(ctx, req); err != nil {
		return fmt.Errorf("event logs api failed: %w", err)
	}
	return nil
}
