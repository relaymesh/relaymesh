package server

import (
	"context"
	"strings"

	"connectrpc.com/connect"

	"github.com/relaymesh/relaymesh/pkg/storage"
)

func newTenantInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			tenantID := tenantIDFromRequest(ctx, req)
			if tenantID != "" {
				ctx = storage.WithTenant(ctx, tenantID)
			}
			return next(ctx, req)
		}
	}
}

func tenantIDFromRequest(ctx context.Context, req connect.AnyRequest) string {
	if claims, ok := AuthClaimsFromContext(ctx); ok && claims != nil {
		if trimmed := strings.TrimSpace(claims.TenantID); trimmed != "" {
			return trimmed
		}
	}
	if req == nil {
		return ""
	}
	header := req.Header()
	if header == nil {
		return ""
	}
	if tenantID := strings.TrimSpace(header.Get("X-Tenant-ID")); tenantID != "" {
		return tenantID
	}
	if tenantID := strings.TrimSpace(header.Get("X-Githooks-Tenant-ID")); tenantID != "" {
		return tenantID
	}
	return ""
}
