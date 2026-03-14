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

// DriverRecord mirrors the server driver response.
type DriverRecord struct {
	Name       string `json:"name"`
	ConfigJSON string `json:"config_json"`
	Enabled    bool   `json:"enabled"`
	ID         string `json:"id"`
}

// DriversClient fetches driver records from the server API.
type DriversClient struct {
	BaseURL    string
	HTTPClient *http.Client
	APIKey     string
	OAuth2     *auth.OAuth2Config
}

// ListDrivers fetches all driver records.
func (c *DriversClient) ListDrivers(ctx context.Context) ([]DriverRecord, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return nil, errors.New("base url is required")
	}

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	interceptor := validate.NewInterceptor()
	connectClient := cloudv1connect.NewDriversServiceClient(
		client,
		base,
		connect.WithInterceptors(interceptor),
	)
	req := connect.NewRequest(&cloudv1.ListDriversRequest{})
	setAuthHeaders(ctx, req.Header(), c.APIKey, c.OAuth2)
	if tenantID := storage.TenantFromContext(ctx); tenantID != "" {
		req.Header().Set("X-Tenant-ID", tenantID)
	}
	log.Printf("drivers api request ListDrivers base=%s tenant=%s body=%#v", base, storage.TenantFromContext(ctx), req.Msg)
	resp, err := connectClient.ListDrivers(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("drivers api failed: %w", err)
	}

	out := make([]DriverRecord, 0, len(resp.Msg.GetDrivers()))
	for _, record := range resp.Msg.GetDrivers() {
		if record == nil {
			continue
		}
		out = append(out, DriverRecord{
			Name:       record.GetName(),
			ConfigJSON: record.GetConfigJson(),
			Enabled:    record.GetEnabled(),
			ID:         record.GetId(),
		})
	}
	return out, nil
}

// GetDriver fetches the driver record by name.
func (c *DriversClient) GetDriver(ctx context.Context, name string) (*DriverRecord, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil, errors.New("driver name is required")
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
	connectClient := cloudv1connect.NewDriversServiceClient(
		client,
		base,
		connect.WithInterceptors(interceptor),
	)
	req := connect.NewRequest(&cloudv1.GetDriverRequest{
		Name: name,
	})
	setAuthHeaders(ctx, req.Header(), c.APIKey, c.OAuth2)
	if tenantID := storage.TenantFromContext(ctx); tenantID != "" {
		req.Header().Set("X-Tenant-ID", tenantID)
	}
	log.Printf("drivers api request GetDriver base=%s tenant=%s name=%s body=%#v", base, storage.TenantFromContext(ctx), name, req.Msg)
	resp, err := connectClient.GetDriver(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("drivers api failed: %w", err)
	}
	if resp.Msg.GetDriver() == nil {
		return nil, nil
	}
	record := resp.Msg.GetDriver()
	return &DriverRecord{
		Name:       record.GetName(),
		ConfigJSON: record.GetConfigJson(),
		Enabled:    record.GetEnabled(),
		ID:         record.GetId(),
	}, nil
}

// GetDriverByID fetches the driver record by ID.
func (c *DriversClient) GetDriverByID(ctx context.Context, id string) (*DriverRecord, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("driver id is required")
	}
	records, err := c.ListDrivers(ctx)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		if strings.TrimSpace(record.ID) == id {
			copy := record
			return &copy, nil
		}
	}
	return nil, nil
}
