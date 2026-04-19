package server

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/relaymesh/relaymesh/pkg/auth/oidc"
)

func TestTenantIDFromRequest(t *testing.T) {
	req := connect.NewRequest(&struct{}{})
	req.Header().Set("X-Tenant-ID", "header-id")

	ctx := context.WithValue(context.Background(), authContextKey{}, &oidc.Claims{TenantID: " claim-id "})
	if tenantID := tenantIDFromRequest(ctx, req, false); tenantID != "claim-id" {
		t.Fatalf("expected claim tenant id, got %q", tenantID)
	}

	ctx = context.Background()
	if tenantID := tenantIDFromRequest(ctx, req, false); tenantID != "" {
		t.Fatalf("expected empty tenant id when header fallback disabled, got %q", tenantID)
	}
	if tenantID := tenantIDFromRequest(ctx, req, true); tenantID != "header-id" {
		t.Fatalf("expected header tenant id, got %q", tenantID)
	}

	req.Header().Del("X-Tenant-ID")
	req.Header().Set("X-Githooks-Tenant-ID", "fallback-id")
	if tenantID := tenantIDFromRequest(ctx, req, true); tenantID != "fallback-id" {
		t.Fatalf("expected fallback tenant id, got %q", tenantID)
	}
}

func TestTenantIDFromRequestNilRequest(t *testing.T) {
	if tenantID := tenantIDFromRequest(context.Background(), nil, true); tenantID != "" {
		t.Fatalf("expected empty tenant id, got %q", tenantID)
	}
}
