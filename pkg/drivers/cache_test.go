package drivers

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

type stubDriverStore struct {
	listDriversFn func(ctx context.Context) ([]storage.DriverRecord, error)
}

func (s *stubDriverStore) ListDrivers(ctx context.Context) ([]storage.DriverRecord, error) {
	if s.listDriversFn == nil {
		return nil, nil
	}
	return s.listDriversFn(ctx)
}

func (s *stubDriverStore) GetDriver(ctx context.Context, name string) (*storage.DriverRecord, error) {
	return nil, errors.New("not implemented")
}

func (s *stubDriverStore) GetDriverByID(ctx context.Context, id string) (*storage.DriverRecord, error) {
	return nil, errors.New("not implemented")
}

func (s *stubDriverStore) UpsertDriver(ctx context.Context, record storage.DriverRecord) (*storage.DriverRecord, error) {
	return nil, errors.New("not implemented")
}

func (s *stubDriverStore) DeleteDriver(ctx context.Context, name string) error {
	return errors.New("not implemented")
}

func (s *stubDriverStore) Close() error {
	return nil
}

type stubPublisher struct {
	published bool
	closed    bool
	err       error
}

func (s *stubPublisher) Publish(ctx context.Context, topic string, event core.Event) error {
	s.published = true
	return nil
}

func (s *stubPublisher) PublishForDrivers(ctx context.Context, topic string, event core.Event, drivers []string) error {
	s.published = true
	return nil
}

func (s *stubPublisher) Close() error {
	s.closed = true
	return s.err
}

func TestCacheKeyFromContext(t *testing.T) {
	ctx := context.Background()
	if key := cacheKeyFromContext(ctx); key != globalDriverKey {
		t.Fatalf("expected global key")
	}
	ctx = storage.WithTenant(ctx, "tenant-a")
	if key := cacheKeyFromContext(ctx); key != "tenant-a" {
		t.Fatalf("expected tenant key")
	}
}

func TestCachePublisherForMissingStore(t *testing.T) {
	cache := NewCache(nil, core.RelaybusConfig{}, nil)
	if _, err := cache.PublisherFor(context.Background()); err == nil {
		t.Fatalf("expected error for missing store")
	}
}

func TestTenantPublisherFallback(t *testing.T) {
	fallback := &stubPublisher{}
	tenantPub := NewTenantPublisher(NewCache(nil, core.RelaybusConfig{}, nil), fallback)
	if err := tenantPub.Publish(context.Background(), "topic", core.Event{}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !fallback.published {
		t.Fatalf("expected fallback publish")
	}
}

func TestCacheClose(t *testing.T) {
	cache := NewCache(nil, core.RelaybusConfig{}, nil)
	cache.pub.Set(globalDriverKey, &stubPublisher{})
	cache.Close()
	pub, ok := cache.pub.Get(globalDriverKey)
	if ok && pub != nil {
		t.Fatalf("expected publisher removed")
	}
}

func TestTenantPublisherNoFallback(t *testing.T) {
	tenantPub := NewTenantPublisher(NewCache(nil, core.RelaybusConfig{}, nil), nil)
	if err := tenantPub.Publish(context.Background(), "topic", core.Event{}); err == nil {
		t.Fatalf("expected error")
	}
	if err := tenantPub.PublishForDrivers(context.Background(), "topic", core.Event{}, nil); err == nil {
		t.Fatalf("expected error")
	}
	if err := tenantPub.Close(); err != nil {
		t.Fatalf("unexpected close error")
	}
}

func TestCacheRefreshNoStoreAndListError(t *testing.T) {
	cache := NewCache(nil, core.RelaybusConfig{}, nil)
	if err := cache.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh with nil store: %v", err)
	}

	expectedErr := errors.New("list drivers failed")
	cache = NewCache(&stubDriverStore{listDriversFn: func(ctx context.Context) ([]storage.DriverRecord, error) {
		return nil, expectedErr
	}}, core.RelaybusConfig{}, nil)
	if err := cache.Refresh(context.Background()); !errors.Is(err, expectedErr) {
		t.Fatalf("expected list error, got %v", err)
	}
}

func TestCacheRefreshTenantClearsPublisherWhenNoDrivers(t *testing.T) {
	cache := NewCache(nil, core.RelaybusConfig{}, nil)
	pub := &stubPublisher{}
	cache.pub.Set(globalDriverKey, pub)
	cache.config.Set(globalDriverKey, core.RelaybusConfig{Driver: "amqp"})

	if err := cache.refreshTenant(context.Background(), globalDriverKey, nil); err != nil {
		t.Fatalf("refresh tenant: %v", err)
	}
	if !pub.closed {
		t.Fatalf("expected existing publisher to be closed")
	}
	if got, ok := cache.pub.Get(globalDriverKey); ok && got != nil {
		t.Fatalf("expected publisher removed after refresh")
	}
	if got, ok := cache.config.Get(globalDriverKey); ok && (got.Driver != "" || len(got.Drivers) > 0) {
		t.Fatalf("expected tenant config removed")
	}
}

func TestCachePublisherForUsesCachedPublisherAndRefreshError(t *testing.T) {
	cache := NewCache(nil, core.RelaybusConfig{}, nil)
	cached := &stubPublisher{}
	cache.pub.Set(globalDriverKey, cached)

	pub, err := cache.PublisherFor(context.Background())
	if err != nil {
		t.Fatalf("publisher for cached tenant: %v", err)
	}
	if pub != cached {
		t.Fatalf("expected cached publisher")
	}

	refreshErr := errors.New("refresh failed")
	cache = NewCache(&stubDriverStore{listDriversFn: func(ctx context.Context) ([]storage.DriverRecord, error) {
		return nil, refreshErr
	}}, core.RelaybusConfig{}, nil)
	if _, err := cache.PublisherFor(context.Background()); !errors.Is(err, refreshErr) {
		t.Fatalf("expected refresh error, got %v", err)
	}
}

func TestCacheRefreshTenantConfigError(t *testing.T) {
	cache := NewCache(nil, core.RelaybusConfig{}, nil)
	err := cache.refreshTenant(context.Background(), globalDriverKey, []storage.DriverRecord{{
		Name:       "unsupported",
		Enabled:    true,
		ConfigJSON: `{}`,
	}})
	if err == nil {
		t.Fatalf("expected refresh tenant to fail for unsupported driver")
	}
}

func TestTenantPublisherCloseReturnsFallbackError(t *testing.T) {
	fallbackErr := errors.New("close failed")
	fallback := &stubPublisher{err: fallbackErr}
	tenantPub := NewTenantPublisher(nil, fallback)
	if err := tenantPub.Close(); !errors.Is(err, fallbackErr) {
		t.Fatalf("expected fallback close error, got %v", err)
	}
}

func TestCacheCloseIgnoresPublisherCloseErrors(t *testing.T) {
	cache := NewCache(nil, core.RelaybusConfig{}, nil)
	cache.pub.Set(globalDriverKey, &stubPublisher{err: fmt.Errorf("boom")})
	cache.Close()
	if got, ok := cache.pub.Get(globalDriverKey); ok && got != nil {
		t.Fatalf("expected cache close to remove publishers")
	}
}
