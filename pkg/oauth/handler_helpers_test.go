package oauth

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

func TestIsProviderInstanceHash(t *testing.T) {
	if !isProviderInstanceHash(strings.Repeat("a", 64)) {
		t.Fatalf("expected valid hash")
	}
	if isProviderInstanceHash("short") {
		t.Fatalf("expected invalid hash")
	}
	if isProviderInstanceHash(strings.Repeat("z", 64)) {
		t.Fatalf("expected invalid hex hash")
	}
}

func TestProviderConfigJSON(t *testing.T) {
	cfg := auth.ProviderConfig{Key: "secret", Webhook: auth.WebhookConfig{Secret: "x"}}
	raw, ok := providerConfigJSON(cfg)
	if !ok {
		t.Fatalf("expected config json")
	}
	if strings.Contains(raw, "\"key\"") {
		t.Fatalf("expected key to be cleared")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("unmarshal config json: %v", err)
	}
}

func TestExpiryFromToken(t *testing.T) {
	if expiryFromToken(oauthToken{ExpiresIn: 0}) != nil {
		t.Fatalf("expected nil expiry")
	}
	expires := expiryFromToken(oauthToken{ExpiresIn: 1})
	if expires == nil || expires.Before(time.Now().UTC()) {
		t.Fatalf("expected future expiry")
	}
}

func TestOAuthTokenMetadataJSON(t *testing.T) {
	token := oauthToken{TokenType: "bearer", Scope: "read"}
	raw := token.MetadataJSON()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if parsed["token_type"] != "bearer" || parsed["scope"] != "read" {
		t.Fatalf("unexpected metadata: %v", parsed)
	}
}
