package providerinstance

import (
	"context"
	"errors"
	"log"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/cache"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

// Cache maintains per-tenant provider instance configs keyed by provider+key.
type Cache struct {
	store  storage.ProviderInstanceStore
	logger *log.Logger
	items  *cache.TenantCache[map[string]auth.ProviderConfig]
}

// NewCache creates a new provider instance cache.
func NewCache(store storage.ProviderInstanceStore, logger *log.Logger) *Cache {
	if logger == nil {
		logger = log.Default()
	}
	return &Cache{
		store:  store,
		logger: logger,
		items:  cache.NewTenantCache[map[string]auth.ProviderConfig](),
	}
}

// Refresh reloads provider instances for the tenant in the context.
func (c *Cache) Refresh(ctx context.Context) error {
	if c == nil || c.store == nil {
		return nil
	}
	tenantID := storage.TenantFromContext(ctx)
	records, err := c.store.ListProviderInstances(ctx, "")
	if err != nil {
		return err
	}
	if tenantID != "" {
		return c.refreshTenant(tenantID, records)
	}
	grouped := make(map[string][]storage.ProviderInstanceRecord)
	for _, record := range records {
		grouped[record.TenantID] = append(grouped[record.TenantID], record)
	}
	for id, group := range grouped {
		if err := c.refreshTenant(id, group); err != nil {
			return err
		}
	}
	existing := c.items.Keys()
	for _, id := range existing {
		if _, ok := grouped[id]; ok {
			continue
		}
		c.items.Delete(id)
	}
	return nil
}

func (c *Cache) refreshTenant(tenantID string, records []storage.ProviderInstanceRecord) error {
	configs := make(map[string]auth.ProviderConfig, len(records))
	for _, record := range records {
		cfg, err := ProviderConfigFromRecord(record)
		if err != nil {
			return err
		}
		key := record.Provider + ":" + record.Key
		configs[key] = cfg
	}
	if len(configs) == 0 {
		c.items.Delete(tenantID)
		return nil
	}
	c.items.Set(tenantID, configs)
	return nil
}

// ConfigFor returns a provider config for the tenant, provider, and key.
func (c *Cache) ConfigFor(ctx context.Context, provider, key string) (auth.ProviderConfig, bool, error) {
	if c == nil {
		return auth.ProviderConfig{}, false, errors.New("provider instance cache not configured")
	}
	tenantID := storage.TenantFromContext(ctx)
	if configs, ok := c.items.Get(tenantID); ok {
		cfg, ok := configs[provider+":"+key]
		return cfg, ok, nil
	}
	if c.store == nil {
		return auth.ProviderConfig{}, false, errors.New("provider instance store not configured")
	}
	if err := c.Refresh(ctx); err != nil {
		return auth.ProviderConfig{}, false, err
	}
	if configs, ok := c.items.Get(tenantID); ok {
		cfg, ok := configs[provider+":"+key]
		return cfg, ok, nil
	}
	return auth.ProviderConfig{}, false, nil
}

// Close clears cached configs.
func (c *Cache) Close() {
	if c == nil {
		return
	}
	for _, tenantID := range c.items.Keys() {
		c.items.Delete(tenantID)
	}
}
