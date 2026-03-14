package oidc

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

type tokenCacheFile struct {
	Entries map[string]tokenCacheEntry `json:"entries"`
}

type tokenCacheEntry struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// CacheKey returns a stable cache key for an OAuth2 config.
func CacheKey(cfg auth.OAuth2Config) string {
	return strings.TrimSpace(cfg.Issuer) + "|" + strings.TrimSpace(cfg.Audience) + "|" + strings.TrimSpace(cfg.ClientID)
}

// DefaultCachePath returns a default cache path.
func DefaultCachePath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "github.com/relaymesh/relaymesh", "token.json"), nil
}

// LoadCachedToken reads a cached token if it exists and is not expired.
func LoadCachedToken(path, key string) (string, time.Time, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", time.Time{}, false, errors.New("cache path is required")
	}
	if err := ensureCachePermissions(path); err != nil {
		return "", time.Time{}, false, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", time.Time{}, false, nil
		}
		return "", time.Time{}, false, err
	}
	var cache tokenCacheFile
	if err := json.Unmarshal(raw, &cache); err != nil {
		return "", time.Time{}, false, err
	}
	if cache.Entries == nil {
		return "", time.Time{}, false, nil
	}
	entry, ok := cache.Entries[key]
	if !ok {
		return "", time.Time{}, false, nil
	}
	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		delete(cache.Entries, key)
		_ = StoreCachedToken(path, key, "", time.Time{})
		return "", time.Time{}, false, nil
	}
	return entry.AccessToken, entry.ExpiresAt, true, nil
}

// StoreCachedToken writes a cached token to disk.
func StoreCachedToken(path, key, token string, expiresAt time.Time) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("cache path is required")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := ensureCachePermissions(path); err != nil {
		return err
	}
	cache := tokenCacheFile{Entries: map[string]tokenCacheEntry{}}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &cache)
	}
	if cache.Entries == nil {
		cache.Entries = map[string]tokenCacheEntry{}
	}
	now := time.Now()
	for name, entry := range cache.Entries {
		if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
			delete(cache.Entries, name)
		}
	}
	if token != "" {
		cache.Entries[key] = tokenCacheEntry{
			AccessToken: token,
			ExpiresAt:   expiresAt,
		}
	} else {
		delete(cache.Entries, key)
	}
	raw, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func ensureCachePermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("token cache path must be a file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(path, 0o600); err != nil {
			return errors.New("token cache permissions too open")
		}
	}
	return nil
}
