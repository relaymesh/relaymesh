package oidc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

func TestAudienceUnmarshalJSON(t *testing.T) {
	var a Audience
	if err := json.Unmarshal([]byte(`"api://githook"`), &a); err != nil {
		t.Fatalf("unmarshal single string: %v", err)
	}
	if len(a) != 1 || a[0] != "api://githook" {
		t.Fatalf("expected single audience, got %v", a)
	}

	if err := json.Unmarshal([]byte(`["a","b"]`), &a); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(a) != 2 || a[0] != "a" || a[1] != "b" {
		t.Fatalf("expected list audience, got %v", a)
	}

	if err := json.Unmarshal([]byte(`""`), &a); err != nil {
		t.Fatalf("unmarshal empty string: %v", err)
	}
	if a != nil {
		t.Fatalf("expected nil audience for empty string, got %v", a)
	}

	if err := json.Unmarshal([]byte(`123`), &a); err == nil {
		t.Fatalf("expected error for invalid audience value")
	}
}

func TestAudienceUnmarshalJSONNilTarget(t *testing.T) {
	var a *Audience
	if err := a.UnmarshalJSON([]byte(`"value"`)); err == nil {
		t.Fatalf("expected error for nil audience target")
	}
}

func TestValidateScopes(t *testing.T) {
	claims := Claims{
		Scope:  "read write",
		Scopes: []string{"admin"},
	}
	if err := validateScopes(claims, []string{"write", "admin"}); err != nil {
		t.Fatalf("expected scopes to pass, got %v", err)
	}
	if err := validateScopes(claims, []string{"missing"}); err == nil {
		t.Fatalf("expected missing scope error")
	}
}

func TestValidateList(t *testing.T) {
	if err := validateList([]string{"ops", "admin"}, []string{"admin"}, "roles"); err != nil {
		t.Fatalf("expected roles to pass, got %v", err)
	}
	if err := validateList([]string{"ops"}, []string{"admin"}, "roles"); err == nil {
		t.Fatalf("expected missing role error")
	}
}

func TestNewVerifierValidation(t *testing.T) {
	if _, err := NewVerifier(context.Background(), auth.OAuth2Config{}); err == nil {
		t.Fatalf("expected issuer required error")
	}

	if _, err := NewVerifier(context.Background(), auth.OAuth2Config{Issuer: "https://issuer.example.com"}); err == nil {
		t.Fatalf("expected audience required error")
	}
}

func TestVerifyValidation(t *testing.T) {
	var v *Verifier
	if _, err := v.Verify(context.Background(), "token"); err == nil {
		t.Fatalf("expected verifier not configured error")
	}

	v = &Verifier{}
	if _, err := v.Verify(context.Background(), "token"); err == nil {
		t.Fatalf("expected verifier not configured error")
	}

	v = &Verifier{verifier: nil}
	if _, err := v.Verify(context.Background(), ""); err == nil {
		t.Fatalf("expected verifier not configured error")
	}
}
