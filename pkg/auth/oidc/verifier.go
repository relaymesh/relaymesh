package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/auth"

	oidclib "github.com/coreos/go-oidc/v3/oidc"
)

type Claims struct {
	Subject  string   `json:"sub"`
	Issuer   string   `json:"iss"`
	Audience Audience `json:"aud"`
	Scope    string   `json:"scope"`
	Scopes   []string `json:"scp"`
	Roles    []string `json:"roles"`
	Groups   []string `json:"groups"`
	TenantID string   `json:"tenant_id"`
}

type Audience []string

func (a *Audience) UnmarshalJSON(data []byte) error {
	if a == nil {
		return errors.New("audience target is nil")
	}
	if string(data) == "null" {
		*a = nil
		return nil
	}
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		if strings.TrimSpace(single) == "" {
			*a = nil
			return nil
		}
		*a = Audience{single}
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*a = Audience(list)
		return nil
	}
	return fmt.Errorf("invalid audience value: %s", string(data))
}

type Verifier struct {
	issuer         string
	requiredScopes []string
	requiredRoles  []string
	requiredGroups []string
	verifier       *oidclib.IDTokenVerifier
}

func NewVerifier(ctx context.Context, cfg auth.OAuth2Config) (*Verifier, error) {
	if strings.TrimSpace(cfg.Issuer) == "" {
		return nil, errors.New("issuer is required")
	}
	if strings.TrimSpace(cfg.Audience) == "" {
		return nil, errors.New("audience is required")
	}
	var keySet oidclib.KeySet
	if strings.TrimSpace(cfg.JWKSURL) != "" {
		keySet = oidclib.NewRemoteKeySet(ctx, cfg.JWKSURL)
	} else {
		provider, err := oidclib.NewProvider(ctx, cfg.Issuer)
		if err != nil {
			return nil, err
		}
		return &Verifier{
			issuer:         cfg.Issuer,
			requiredScopes: cfg.RequiredScopes,
			requiredRoles:  cfg.RequiredRoles,
			requiredGroups: cfg.RequiredGroups,
			verifier:       provider.Verifier(&oidclib.Config{ClientID: cfg.Audience}),
		}, nil
	}
	verifier := oidclib.NewVerifier(cfg.Issuer, keySet, &oidclib.Config{ClientID: cfg.Audience})
	return &Verifier{
		issuer:         cfg.Issuer,
		requiredScopes: cfg.RequiredScopes,
		requiredRoles:  cfg.RequiredRoles,
		requiredGroups: cfg.RequiredGroups,
		verifier:       verifier,
	}, nil
}

func (v *Verifier) Verify(ctx context.Context, token string) (*Claims, error) {
	if v == nil || v.verifier == nil {
		return nil, errors.New("verifier not configured")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("token is required")
	}
	idToken, err := v.verifier.Verify(ctx, token)
	if err != nil {
		return nil, err
	}
	var claims Claims
	if err := idToken.Claims(&claims); err != nil {
		return nil, err
	}
	if err := validateScopes(claims, v.requiredScopes); err != nil {
		return nil, err
	}
	if err := validateList(claims.Roles, v.requiredRoles, "roles"); err != nil {
		return nil, err
	}
	if err := validateList(claims.Groups, v.requiredGroups, "groups"); err != nil {
		return nil, err
	}
	return &claims, nil
}

func validateScopes(claims Claims, required []string) error {
	if len(required) == 0 {
		return nil
	}
	available := map[string]struct{}{}
	if claims.Scope != "" {
		for _, val := range strings.Fields(claims.Scope) {
			available[val] = struct{}{}
		}
	}
	for _, val := range claims.Scopes {
		if val != "" {
			available[val] = struct{}{}
		}
	}
	for _, requiredScope := range required {
		if _, ok := available[requiredScope]; !ok {
			return errors.New("missing required scope: " + requiredScope)
		}
	}
	return nil
}

func validateList(values []string, required []string, name string) error {
	if len(required) == 0 {
		return nil
	}
	available := map[string]struct{}{}
	for _, value := range values {
		if value != "" {
			available[value] = struct{}{}
		}
	}
	for _, requiredValue := range required {
		if _, ok := available[requiredValue]; !ok {
			return errors.New("missing required " + name + ": " + requiredValue)
		}
	}
	return nil
}
