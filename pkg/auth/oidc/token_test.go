package oidc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

func TestResolveEndpointsConfigured(t *testing.T) {
	cfg := auth.OAuth2Config{
		AuthorizeURL: "https://issuer/auth",
		TokenURL:     "https://issuer/token",
		JWKSURL:      "https://issuer/jwks",
	}
	authURL, tokenURL, jwksURL, err := ResolveEndpoints(context.Background(), cfg)
	if err != nil {
		t.Fatalf("resolve endpoints: %v", err)
	}
	if authURL != cfg.AuthorizeURL || tokenURL != cfg.TokenURL || jwksURL != cfg.JWKSURL {
		t.Fatalf("unexpected endpoints: %q %q %q", authURL, tokenURL, jwksURL)
	}
}

func TestResolveEndpointsRequiresIssuer(t *testing.T) {
	cfg := auth.OAuth2Config{}
	if _, _, _, err := ResolveEndpoints(context.Background(), cfg); err == nil {
		t.Fatalf("expected issuer required error")
	}
}

func TestResolveEndpointsDiscovery(t *testing.T) {
	discoveryCacheMu.Lock()
	discoveryCache = make(map[string]cacheEntry)
	discoveryCacheMu.Unlock()

	t.Run("fills missing endpoints from discovery", func(t *testing.T) {
		issuer := "https://issuer.example.com"
		payload := `{"authorization_endpoint":"https://issuer/auth","token_endpoint":"https://issuer/token","jwks_uri":"https://issuer/jwks"}`
		originalTransport := http.DefaultTransport
		http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != issuer+"/.well-known/openid-configuration" {
				t.Fatalf("unexpected discovery url: %s", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(payload)),
			}, nil
		})
		t.Cleanup(func() { http.DefaultTransport = originalTransport })

		authURL, tokenURL, jwksURL, err := ResolveEndpoints(context.Background(), auth.OAuth2Config{
			Issuer:       issuer,
			TokenURL:     "https://custom/token",
			AuthorizeURL: "",
		})
		if err != nil {
			t.Fatalf("resolve endpoints via discovery: %v", err)
		}
		if authURL != "https://issuer/auth" || tokenURL != "https://custom/token" || jwksURL != "https://issuer/jwks" {
			t.Fatalf("unexpected resolved endpoints: %q %q %q", authURL, tokenURL, jwksURL)
		}
	})

	t.Run("discovery failure bubbles up", func(t *testing.T) {
		discoveryCacheMu.Lock()
		discoveryCache = make(map[string]cacheEntry)
		discoveryCacheMu.Unlock()

		originalTransport := http.DefaultTransport
		http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("network down")
		})
		t.Cleanup(func() { http.DefaultTransport = originalTransport })

		if _, _, _, err := ResolveEndpoints(context.Background(), auth.OAuth2Config{Issuer: "https://issuer.example.com"}); err == nil {
			t.Fatalf("expected discovery error")
		}
	})
}

func TestClientCredentialsToken(t *testing.T) {
	payload := map[string]interface{}{
		"access_token": "token-value",
		"expires_in":   3600,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", req.Method)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if values.Get("grant_type") != "client_credentials" {
			t.Fatalf("expected client_credentials grant, got %q", values.Get("grant_type"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     http.StatusText(http.StatusOK),
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(raw)),
		}, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	cfg := auth.OAuth2Config{
		ClientID:     "client",
		ClientSecret: "secret",
		TokenURL:     "https://issuer.example.com/oauth/token",
		AuthorizeURL: "https://issuer/auth",
		JWKSURL:      "https://issuer/jwks",
		RequiredScopes: []string{
			"github.com/relaymesh/relaymesh.read",
		},
	}
	token, err := ClientCredentialsToken(context.Background(), cfg)
	if err != nil {
		t.Fatalf("client credentials token: %v", err)
	}
	if token.AccessToken != "token-value" {
		t.Fatalf("expected token-value, got %q", token.AccessToken)
	}
}

func TestReadErrorBodySanitizesTokens(t *testing.T) {
	body := `{"error":"access_denied","access_token":"secret","refresh_token":"refresh","id_token":"id"}`
	value := readErrorBody(strings.NewReader(body))
	if strings.Contains(value, "access_token") || strings.Contains(value, "refresh_token") || strings.Contains(value, "id_token") {
		t.Fatalf("expected tokens to be stripped, got %q", value)
	}
	if !strings.Contains(value, "access_denied") {
		t.Fatalf("expected error value to be preserved, got %q", value)
	}
}

func TestReadErrorBodyTrimsWhitespace(t *testing.T) {
	body := "{\n\"error\":\"bad\"}\n"
	value := readErrorBody(strings.NewReader(body))
	if strings.Contains(value, "\n") || strings.Contains(value, "\r") {
		t.Fatalf("expected trimmed whitespace, got %q", value)
	}
}

func TestClientCredentialsTokenUsesAudienceAndScopes(t *testing.T) {
	var got url.Values
	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		got, err = url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse form: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     http.StatusText(http.StatusOK),
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"access_token":"ok","expires_in":1}`)),
		}, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	cfg := auth.OAuth2Config{
		ClientID:       "client",
		ClientSecret:   "secret",
		TokenURL:       "https://issuer.example.com/oauth/token",
		AuthorizeURL:   "https://issuer/auth",
		JWKSURL:        "https://issuer/jwks",
		Audience:       "https://api",
		RequiredScopes: []string{"scope-a", "scope-b"},
	}
	if _, err := ClientCredentialsToken(context.Background(), cfg); err != nil {
		t.Fatalf("client credentials token: %v", err)
	}
	if got.Get("audience") != "https://api" {
		t.Fatalf("expected audience to be set, got %q", got.Get("audience"))
	}
	if got.Get("scope") != "scope-a scope-b" {
		t.Fatalf("expected scope to be set, got %q", got.Get("scope"))
	}
}

func TestClientCredentialsTokenMissingCredentials(t *testing.T) {
	cfg := auth.OAuth2Config{}
	if _, err := ClientCredentialsToken(context.Background(), cfg); err == nil {
		t.Fatalf("expected missing credentials error")
	}
}

func TestClientCredentialsTokenErrorResponses(t *testing.T) {
	t.Run("non-2xx response includes sanitized body", func(t *testing.T) {
		originalTransport := http.DefaultTransport
		http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"error":"access_denied","access_token":"secret"}`)),
			}, nil
		})
		t.Cleanup(func() { http.DefaultTransport = originalTransport })

		_, err := ClientCredentialsToken(context.Background(), auth.OAuth2Config{
			ClientID:     "client",
			ClientSecret: "secret",
			TokenURL:     "https://issuer.example.com/oauth/token",
			AuthorizeURL: "https://issuer/auth",
			JWKSURL:      "https://issuer/jwks",
		})
		if err == nil || !strings.Contains(err.Error(), "token exchange failed") {
			t.Fatalf("expected token exchange failure, got %v", err)
		}
		if strings.Contains(err.Error(), "access_token") {
			t.Fatalf("expected sensitive token removed from error: %v", err)
		}
	})

	t.Run("missing access token", func(t *testing.T) {
		originalTransport := http.DefaultTransport
		http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"expires_in":3600}`)),
			}, nil
		})
		t.Cleanup(func() { http.DefaultTransport = originalTransport })

		_, err := ClientCredentialsToken(context.Background(), auth.OAuth2Config{
			ClientID:     "client",
			ClientSecret: "secret",
			TokenURL:     "https://issuer.example.com/oauth/token",
			AuthorizeURL: "https://issuer/auth",
			JWKSURL:      "https://issuer/jwks",
		})
		if err == nil || !strings.Contains(err.Error(), "access_token missing") {
			t.Fatalf("expected missing access token error, got %v", err)
		}
	})
}

func TestReadErrorBodyReaderError(t *testing.T) {
	value := readErrorBody(errReader{err: errors.New("boom")})
	if value != "" {
		t.Fatalf("expected empty body for read error, got %q", value)
	}
}

type errReader struct {
	err error
}

func (e errReader) Read(p []byte) (int, error) {
	return 0, e.err
}
