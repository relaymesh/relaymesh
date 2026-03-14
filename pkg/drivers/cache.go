package drivers

import (
	"context"
	"errors"
	"log"

	"github.com/relaymesh/relaymesh/pkg/cache"
	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

// Cache maintains driver configs and publishers.
type Cache struct {
	store  storage.DriverStore
	base   core.RelaybusConfig
	logger *log.Logger
	config *cache.TenantCache[core.RelaybusConfig]
	pub    *cache.TenantCache[core.Publisher]
}

const globalDriverKey = "global"

// NewCache creates a new driver cache.
func NewCache(store storage.DriverStore, base core.RelaybusConfig, logger *log.Logger) *Cache {
	if logger == nil {
		logger = log.Default()
	}
	return &Cache{
		store:  store,
		base:   base,
		logger: logger,
		config: cache.NewTenantCache[core.RelaybusConfig](),
		pub:    cache.NewTenantCache[core.Publisher](),
	}
}

// Refresh reloads drivers for the tenant in the context and rebuilds the publisher.
func (c *Cache) Refresh(ctx context.Context) error {
	if c == nil || c.store == nil {
		return nil
	}
	tenantKey := cacheKeyFromContext(ctx)
	records, err := c.store.ListDrivers(ctx)
	if err != nil {
		return err
	}
	return c.refreshTenant(ctx, tenantKey, records)
}

func (c *Cache) refreshTenant(ctx context.Context, tenantID string, records []storage.DriverRecord) error {
	cfg, err := ConfigFromRecords(c.base, records)
	if err != nil {
		return err
	}
	if len(cfg.Drivers) == 0 && cfg.Driver == "" {
		if existing, ok := c.pub.Get(tenantID); ok && existing != nil {
			_ = existing.Close()
		}
		c.pub.Delete(tenantID)
		c.config.Delete(tenantID)
		return nil
	}
	pub, err := core.NewPublisherWithContext(ctx, cfg)
	if err != nil {
		return err
	}
	if existing, ok := c.pub.Get(tenantID); ok && existing != nil {
		_ = existing.Close()
	}
	c.config.Set(tenantID, cfg)
	c.pub.Set(tenantID, pub)
	return nil
}

// PublisherFor returns a publisher for the tenant in the context.
func (c *Cache) PublisherFor(ctx context.Context) (core.Publisher, error) {
	if c == nil {
		return nil, errors.New("driver cache not configured")
	}
	tenantKey := cacheKeyFromContext(ctx)
	if pub, ok := c.pub.Get(tenantKey); ok && pub != nil {
		return pub, nil
	}
	if c.store == nil {
		return nil, errors.New("driver store not configured")
	}
	if err := c.Refresh(ctx); err != nil {
		if c.logger != nil {
			c.logger.Printf("driver cache refresh failed tenant=%s err=%v", tenantKey, err)
		}
		return nil, err
	}
	pub, _ := c.pub.Get(tenantKey)
	if pub == nil {
		if c.logger != nil {
			c.logger.Printf("driver cache missing publisher tenant=%s drivers=%v", tenantKey, c.pub.Keys())
		}
		return nil, errors.New("no publisher available")
	}
	return pub, nil
}

// Close closes all cached publishers.
func (c *Cache) Close() {
	if c == nil {
		return
	}
	keys := make([]string, 0)
	c.pub.Range(func(key string, pub core.Publisher) {
		if pub != nil {
			_ = pub.Close()
		}
		keys = append(keys, key)
	})
	for _, key := range keys {
		c.pub.Delete(key)
		c.config.Delete(key)
	}
}

// TenantPublisher routes publish calls to the cached publisher for each tenant.
type TenantPublisher struct {
	cache    *Cache
	fallback core.Publisher
}

// NewTenantPublisher creates a publisher that routes by tenant when possible.
func NewTenantPublisher(cache *Cache, fallback core.Publisher) core.Publisher {
	return &TenantPublisher{cache: cache, fallback: fallback}
}

func (p *TenantPublisher) Publish(ctx context.Context, topic string, event core.Event) error {
	pub, err := p.publisherFor(ctx)
	if err != nil {
		return err
	}
	return pub.Publish(ctx, topic, event)
}

func (p *TenantPublisher) PublishForDrivers(ctx context.Context, topic string, event core.Event, drivers []string) error {
	pub, err := p.publisherFor(ctx)
	if err != nil {
		return err
	}
	return pub.PublishForDrivers(ctx, topic, event, drivers)
}

func (p *TenantPublisher) Close() error {
	if p.cache != nil {
		p.cache.Close()
	}
	if p.fallback != nil {
		return p.fallback.Close()
	}
	return nil
}

func (p *TenantPublisher) publisherFor(ctx context.Context) (core.Publisher, error) {
	if p.cache == nil {
		if p.fallback == nil {
			return nil, errors.New("no publisher configured")
		}
		return p.fallback, nil
	}
	pub, err := p.cache.PublisherFor(ctx)
	if err == nil {
		return pub, nil
	}
	if p.fallback != nil && storage.TenantFromContext(ctx) == "" {
		return p.fallback, nil
	}
	return nil, err
}

func cacheKeyFromContext(ctx context.Context) string {
	if tenantID := storage.TenantFromContext(ctx); tenantID != "" {
		return tenantID
	}
	return globalDriverKey
}
