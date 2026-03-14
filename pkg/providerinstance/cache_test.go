package providerinstance

import (
	"context"
	"errors"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/storage"
)

type providerStoreStub struct {
	listFn func(ctx context.Context, provider string) ([]storage.ProviderInstanceRecord, error)
}

func (s *providerStoreStub) ListProviderInstances(ctx context.Context, provider string) ([]storage.ProviderInstanceRecord, error) {
	if s.listFn == nil {
		return nil, nil
	}
	return s.listFn(ctx, provider)
}

func (s *providerStoreStub) GetProviderInstance(ctx context.Context, provider, key string) (*storage.ProviderInstanceRecord, error) {
	return nil, errors.New("not implemented")
}

func (s *providerStoreStub) UpsertProviderInstance(ctx context.Context, record storage.ProviderInstanceRecord) (*storage.ProviderInstanceRecord, error) {
	return nil, errors.New("not implemented")
}

func (s *providerStoreStub) DeleteProviderInstance(ctx context.Context, provider, key string) error {
	return errors.New("not implemented")
}

func (s *providerStoreStub) Close() error {
	return nil
}

func TestCacheConfigForAndRefresh(t *testing.T) {
	t.Run("nil cache and missing store", func(t *testing.T) {
		var c *Cache
		if _, _, err := c.ConfigFor(context.Background(), "github", ""); err == nil {
			t.Fatalf("expected error for nil cache")
		}

		c = NewCache(nil, nil)
		if _, _, err := c.ConfigFor(context.Background(), "github", ""); err == nil {
			t.Fatalf("expected missing store error")
		}
	})

	t.Run("refresh stores and retrieves tenant config", func(t *testing.T) {
		store := &providerStoreStub{listFn: func(ctx context.Context, provider string) ([]storage.ProviderInstanceRecord, error) {
			return []storage.ProviderInstanceRecord{{
				TenantID:   "tenant-a",
				Provider:   "github",
				Key:        "",
				Enabled:    true,
				ConfigJSON: `{"oauth":{"client_id":"abc"}}`,
			}}, nil
		}}
		c := NewCache(store, nil)
		ctx := storage.WithTenant(context.Background(), "tenant-a")

		if err := c.Refresh(ctx); err != nil {
			t.Fatalf("refresh: %v", err)
		}

		cfg, ok, err := c.ConfigFor(ctx, "github", "")
		if err != nil {
			t.Fatalf("config for: %v", err)
		}
		if !ok {
			t.Fatalf("expected tenant config to be cached")
		}
		if cfg.OAuth.ClientID != "abc" || !cfg.Enabled {
			t.Fatalf("unexpected config: %+v", cfg)
		}
	})

	t.Run("refresh propagates list and parse errors", func(t *testing.T) {
		expected := errors.New("list failed")
		c := NewCache(&providerStoreStub{listFn: func(ctx context.Context, provider string) ([]storage.ProviderInstanceRecord, error) {
			return nil, expected
		}}, nil)
		ctx := storage.WithTenant(context.Background(), "tenant-a")
		if err := c.Refresh(ctx); !errors.Is(err, expected) {
			t.Fatalf("expected list error, got %v", err)
		}

		c = NewCache(&providerStoreStub{listFn: func(ctx context.Context, provider string) ([]storage.ProviderInstanceRecord, error) {
			return []storage.ProviderInstanceRecord{{TenantID: "tenant-a", Provider: "github", ConfigJSON: "{"}}, nil
		}}, nil)
		if err := c.Refresh(ctx); err == nil {
			t.Fatalf("expected invalid config parse error")
		}
	})
}
