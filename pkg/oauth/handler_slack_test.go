package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

func TestExchangeSlackToken(t *testing.T) {
	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		payload := map[string]interface{}{
			"ok":            true,
			"access_token":  "xoxb-token",
			"refresh_token": "xoxe-refresh",
			"expires_in":    3600,
			"token_type":    "bot",
			"scope":         "chat:write,channels:history",
			"bot_user_id":   "U123",
			"app_id":        "A123",
			"team": map[string]interface{}{
				"id":   "T123",
				"name": "Relaymesh Team",
			},
		}
		raw, _ := json.Marshal(payload)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     http.StatusText(http.StatusOK),
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(raw)),
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = originalTransport })

	cfg := auth.ProviderConfig{
		OAuth: auth.OAuthConfig{ClientID: "id", ClientSecret: "secret"},
		API:   auth.APIConfig{BaseURL: "https://slack.com/api"},
	}
	token, accountID, accountName, metadata, err := exchangeSlackToken(context.Background(), cfg, "code-1", "https://example.com/auth/slack/callback")
	if err != nil {
		t.Fatalf("exchange slack token: %v", err)
	}
	if token.AccessToken != "xoxb-token" {
		t.Fatalf("unexpected access token: %q", token.AccessToken)
	}
	if accountID != "T123" || accountName != "Relaymesh Team" {
		t.Fatalf("unexpected account info id=%q name=%q", accountID, accountName)
	}
	if metadata == "" {
		t.Fatalf("expected metadata json")
	}
}
