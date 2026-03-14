package oidc

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

func TestCacheKey(t *testing.T) {
	cfg := auth.OAuth2Config{
		Issuer:   " https://issuer ",
		Audience: " api ",
		ClientID: " client ",
	}
	if key := CacheKey(cfg); key != "https://issuer|api|client" {
		t.Fatalf("unexpected cache key: %q", key)
	}
}

func TestStoreLoadCachedToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")
	key := "cache-key"
	expiresAt := time.Now().Add(30 * time.Minute)

	if err := StoreCachedToken(path, key, "token-value", expiresAt); err != nil {
		t.Fatalf("store cached token: %v", err)
	}

	token, expiry, ok, err := LoadCachedToken(path, key)
	if err != nil {
		t.Fatalf("load cached token: %v", err)
	}
	if !ok {
		t.Fatalf("expected cached token")
	}
	if token != "token-value" {
		t.Fatalf("expected token-value, got %q", token)
	}
	if expiry.IsZero() {
		t.Fatalf("expected expiry to be set")
	}
}

func TestLoadCachedTokenExpired(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")
	key := "cache-key"
	expiresAt := time.Now().Add(-1 * time.Minute)

	if err := StoreCachedToken(path, key, "token-value", expiresAt); err != nil {
		t.Fatalf("store cached token: %v", err)
	}

	token, _, ok, err := LoadCachedToken(path, key)
	if err != nil {
		t.Fatalf("load cached token: %v", err)
	}
	if ok || token != "" {
		t.Fatalf("expected expired token to be cleared")
	}
}

func TestLoadCachedTokenPathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	if _, _, _, err := LoadCachedToken(dir, "key"); err == nil {
		t.Fatalf("expected error for directory cache path")
	}
}

func TestDefaultCachePathAndValidation(t *testing.T) {
	if path, err := DefaultCachePath(); err != nil || path == "" {
		t.Fatalf("expected default cache path, got path=%q err=%v", path, err)
	}

	if _, _, _, err := LoadCachedToken("", "k"); err == nil {
		t.Fatalf("expected empty path load error")
	}
	if err := StoreCachedToken("", "k", "token", time.Now().Add(time.Minute)); err == nil {
		t.Fatalf("expected empty path store error")
	}
}

func TestEnsureCachePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")
	if err := os.WriteFile(path, []byte(`{"entries":{}}`), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	if err := ensureCachePermissions(path); err != nil {
		t.Fatalf("ensure cache permissions: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat cache file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected cache file mode 0600, got %#o", info.Mode().Perm())
	}
}
