package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

func TestRefreshGitLabToken(t *testing.T) {
	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		payload := map[string]interface{}{
			"access_token":  "token",
			"refresh_token": "",
			"expires_in":    60,
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

	cfg := auth.ProviderConfig{API: auth.APIConfig{BaseURL: "https://gitlab.example.com/api/v4"}}
	result, err := RefreshGitLabToken(context.Background(), cfg, "refresh")
	if err != nil {
		t.Fatalf("refresh gitlab token: %v", err)
	}
	if result.AccessToken != "token" || result.RefreshToken != "refresh" {
		t.Fatalf("unexpected token result: %+v", result)
	}
	if result.ExpiresAt == nil || result.ExpiresAt.Before(time.Now()) {
		t.Fatalf("expected expiry")
	}
}

func TestRefreshBitbucketToken(t *testing.T) {
	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		payload := map[string]interface{}{
			"access_token":  "token",
			"refresh_token": "refresh-new",
			"expires_in":    60,
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

	cfg := auth.ProviderConfig{OAuth: auth.OAuthConfig{ClientID: "id", ClientSecret: "secret"}}
	result, err := RefreshBitbucketToken(context.Background(), cfg, "refresh-old")
	if err != nil {
		t.Fatalf("refresh bitbucket token: %v", err)
	}
	if result.AccessToken != "token" || result.RefreshToken != "refresh-new" {
		t.Fatalf("unexpected token result: %+v", result)
	}
}

func TestRefreshSlackToken(t *testing.T) {
	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		payload := map[string]interface{}{
			"ok":            true,
			"access_token":  "xoxb-new",
			"refresh_token": "xoxe-new",
			"expires_in":    120,
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
	result, err := RefreshSlackToken(context.Background(), cfg, "xoxe-old")
	if err != nil {
		t.Fatalf("refresh slack token: %v", err)
	}
	if result.AccessToken != "xoxb-new" || result.RefreshToken != "xoxe-new" {
		t.Fatalf("unexpected token result: %+v", result)
	}
	if result.ExpiresAt == nil || result.ExpiresAt.Before(time.Now()) {
		t.Fatalf("expected expiry")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
