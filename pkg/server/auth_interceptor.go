package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	oidchelper "github.com/relaymesh/relaymesh/pkg/auth/oidc"
)

type authContextKey struct{}

// AuthClaimsFromContext returns JWT claims from a request context if available.
func AuthClaimsFromContext(ctx context.Context) (*oidchelper.Claims, bool) {
	claims, ok := ctx.Value(authContextKey{}).(*oidchelper.Claims)
	return claims, ok
}

func newAuthInterceptor(verifier *oidchelper.Verifier, logger *log.Logger) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if verifier == nil {
				return next(ctx, req)
			}

			token, err := bearerToken(req.Header())
			if err != nil {
				logAuthError(logger, err)
				return nil, connect.NewError(connect.CodeUnauthenticated, err)
			}
			claims, err := verifier.Verify(ctx, token)
			if err != nil {
				logAuthError(logger, err)
				return nil, connect.NewError(connect.CodeUnauthenticated, err)
			}
			ctx = context.WithValue(ctx, authContextKey{}, claims)
			return next(ctx, req)
		}
	}
}

func bearerToken(header http.Header) (string, error) {
	raw := strings.TrimSpace(header.Get("Authorization"))
	if raw == "" {
		return "", errors.New("missing authorization header")
	}
	parts := strings.SplitN(raw, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", errors.New("invalid authorization header")
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", errors.New("missing bearer token")
	}
	return token, nil
}

func logAuthError(logger *log.Logger, err error) {
	if logger == nil || err == nil {
		return
	}
	logger.Printf("auth failed: %v", err)
}
