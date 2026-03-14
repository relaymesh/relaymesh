package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

func TestSupportsAuthCode(t *testing.T) {
	if !supportsAuthCode("") || !supportsAuthCode("auto") || !supportsAuthCode("auth_code") {
		t.Fatalf("expected auth code modes to be supported")
	}
	if supportsAuthCode("client_credentials") {
		t.Fatalf("expected client_credentials to be unsupported")
	}
}

func TestRandomStringAndCodeChallenge(t *testing.T) {
	value := randomString(16)
	if value == "" {
		t.Fatalf("expected random string")
	}
	challenge := codeChallenge("verifier")
	if challenge == "" || strings.Contains(challenge, "verifier") {
		t.Fatalf("unexpected challenge")
	}
}

func TestTokenCachePath(t *testing.T) {
	t.Setenv("github.com/relaymesh/relaymesh_TOKEN_CACHE", "/tmp/token.json")
	if got := tokenCachePath(); got != "/tmp/token.json" {
		t.Fatalf("unexpected token cache path: %q", got)
	}
}

func TestOAuth2HandlerLoginErrors(t *testing.T) {
	handler := newOAuth2Handler(auth.OAuth2Config{Enabled: false}, nil)
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	resp := httptest.NewRecorder()
	handler.Login(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected not found status")
	}

	handler = newOAuth2Handler(auth.OAuth2Config{Enabled: true, Mode: "client_credentials"}, nil)
	resp = httptest.NewRecorder()
	handler.Login(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for auth_code disabled")
	}

	handler = newOAuth2Handler(auth.OAuth2Config{Enabled: true}, nil)
	req = httptest.NewRequest(http.MethodPost, "/login", nil)
	resp = httptest.NewRecorder()
	handler.Login(resp, req)
	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected method not allowed")
	}
}

func TestOAuth2HandlerLoginRedirect(t *testing.T) {
	cfg := auth.OAuth2Config{
		Enabled:      true,
		Mode:         "auth_code",
		ClientID:     "client",
		ClientSecret: "secret",
		Audience:     "api",
		AuthorizeURL: "https://auth.example.com/authorize",
		TokenURL:     "https://auth.example.com/token",
		JWKSURL:      "https://auth.example.com/jwks",
		RedirectURL:  "https://app.example.com/callback",
	}
	handler := newOAuth2Handler(cfg, nil)
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	resp := httptest.NewRecorder()
	handler.Login(resp, req)
	if resp.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d", resp.Code)
	}
	location := resp.Header().Get("Location")
	if !strings.HasPrefix(location, cfg.AuthorizeURL) {
		t.Fatalf("unexpected redirect location: %s", location)
	}
	if len(handler.state) == 0 {
		t.Fatalf("expected state to be stored")
	}
}

func TestOAuth2HandlerCallbackErrors(t *testing.T) {
	handler := newOAuth2Handler(auth.OAuth2Config{Enabled: false}, nil)
	req := httptest.NewRequest(http.MethodGet, "/callback", nil)
	resp := httptest.NewRecorder()
	handler.Callback(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected not found status")
	}

	handler = newOAuth2Handler(auth.OAuth2Config{Enabled: true, Mode: "client_credentials"}, nil)
	resp = httptest.NewRecorder()
	handler.Callback(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for auth_code disabled")
	}

	handler = newOAuth2Handler(auth.OAuth2Config{Enabled: true}, nil)
	req = httptest.NewRequest(http.MethodPost, "/callback", nil)
	resp = httptest.NewRecorder()
	handler.Callback(resp, req)
	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected method not allowed")
	}

	req = httptest.NewRequest(http.MethodGet, "/callback?code=&state=", nil)
	resp = httptest.NewRecorder()
	handler.Callback(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected missing code/state error")
	}
}

func TestConsumeState(t *testing.T) {
	handler := newOAuth2Handler(auth.OAuth2Config{Enabled: true}, nil)
	handler.state["valid"] = oauthState{
		codeVerifier: "code",
		expiresAt:    time.Now().Add(1 * time.Minute),
	}
	handler.state["expired"] = oauthState{
		codeVerifier: "code",
		expiresAt:    time.Now().Add(-1 * time.Minute),
	}
	if _, err := handler.consumeState("expired"); err == nil {
		t.Fatalf("expected expired state error")
	}
	val, err := handler.consumeState("valid")
	if err != nil || val != "code" {
		t.Fatalf("expected valid state, got %q err=%v", val, err)
	}
}
