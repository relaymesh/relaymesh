package server

import (
	"fmt"
	"log"

	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/storage"
	driversstore "github.com/relaymesh/relaymesh/pkg/storage/drivers"
	"github.com/relaymesh/relaymesh/pkg/storage/eventlogs"
	"github.com/relaymesh/relaymesh/pkg/storage/installations"
	"github.com/relaymesh/relaymesh/pkg/storage/namespaces"
	providerinstancestore "github.com/relaymesh/relaymesh/pkg/storage/provider_instances"
	"github.com/relaymesh/relaymesh/pkg/storage/rules"
)

func openStores(cfg core.Config, logger *log.Logger, addCloser func(func())) (serverStores, error) {
	var stores serverStores
	if cfg.Storage.Driver == "" || cfg.Storage.DSN == "" {
		logger.Printf("storage disabled (missing storage.driver or storage.dsn)")
		return stores, nil
	}

	pool := storage.PoolConfig{
		MaxOpenConns:      cfg.Storage.MaxOpenConns,
		MaxIdleConns:      cfg.Storage.MaxIdleConns,
		ConnMaxLifetimeMS: cfg.Storage.ConnMaxLifetimeMS,
		ConnMaxIdleTimeMS: cfg.Storage.ConnMaxIdleTimeMS,
	}
	installStore, err := installations.Open(installations.Config{
		Driver:      cfg.Storage.Driver,
		DSN:         cfg.Storage.DSN,
		Dialect:     cfg.Storage.Dialect,
		AutoMigrate: cfg.Storage.AutoMigrate,
		Pool:        pool,
	})
	if err != nil {
		return stores, fmt.Errorf("storage: %w", err)
	}
	stores.installStore = installStore
	addCloser(func() { _ = installStore.Close() })
	logger.Printf("storage enabled driver=%s dialect=%s table=githook_installations", cfg.Storage.Driver, cfg.Storage.Dialect)

	namespaceStore, err := namespaces.Open(namespaces.Config{
		Driver:      cfg.Storage.Driver,
		DSN:         cfg.Storage.DSN,
		Dialect:     cfg.Storage.Dialect,
		AutoMigrate: cfg.Storage.AutoMigrate,
		Pool:        pool,
	})
	if err != nil {
		return stores, fmt.Errorf("namespaces storage: %w", err)
	}
	stores.namespaceStore = namespaceStore
	addCloser(func() { _ = namespaceStore.Close() })
	logger.Printf("namespaces enabled driver=%s dialect=%s table=%s", cfg.Storage.Driver, cfg.Storage.Dialect, namespaceStore.TableName())

	ruleStore, err := rules.Open(rules.Config{
		Driver:      cfg.Storage.Driver,
		DSN:         cfg.Storage.DSN,
		Dialect:     cfg.Storage.Dialect,
		AutoMigrate: cfg.Storage.AutoMigrate,
		Pool:        pool,
	})
	if err != nil {
		return stores, fmt.Errorf("rules storage: %w", err)
	}
	stores.ruleStore = ruleStore
	addCloser(func() { _ = ruleStore.Close() })
	logger.Printf("rules enabled driver=%s dialect=%s table=githook_rules", cfg.Storage.Driver, cfg.Storage.Dialect)

	logStore, err := eventlogs.Open(eventlogs.Config{
		Driver:      cfg.Storage.Driver,
		DSN:         cfg.Storage.DSN,
		Dialect:     cfg.Storage.Dialect,
		AutoMigrate: cfg.Storage.AutoMigrate,
		Pool:        pool,
	})
	if err != nil {
		return stores, fmt.Errorf("event logs storage: %w", err)
	}
	stores.logStore = logStore
	addCloser(func() { _ = logStore.Close() })
	logger.Printf("event logs enabled driver=%s dialect=%s table=githook_event_logs", cfg.Storage.Driver, cfg.Storage.Dialect)

	driverStore, err := driversstore.Open(driversstore.Config{
		Driver:      cfg.Storage.Driver,
		DSN:         cfg.Storage.DSN,
		Dialect:     cfg.Storage.Dialect,
		AutoMigrate: cfg.Storage.AutoMigrate,
		Pool:        pool,
	})
	if err != nil {
		return stores, fmt.Errorf("driver storage: %w", err)
	}
	stores.driverStore = driverStore
	addCloser(func() { _ = driverStore.Close() })
	logger.Printf("drivers enabled driver=%s dialect=%s table=githook_drivers", cfg.Storage.Driver, cfg.Storage.Dialect)

	instanceStore, err := providerinstancestore.Open(providerinstancestore.Config{
		Driver:      cfg.Storage.Driver,
		DSN:         cfg.Storage.DSN,
		Dialect:     cfg.Storage.Dialect,
		AutoMigrate: cfg.Storage.AutoMigrate,
		Pool:        pool,
	})
	if err != nil {
		return stores, fmt.Errorf("provider instances storage: %w", err)
	}
	stores.instanceStore = instanceStore
	addCloser(func() { _ = instanceStore.Close() })
	logger.Printf("provider instances enabled driver=%s dialect=%s table=githook_provider_instances", cfg.Storage.Driver, cfg.Storage.Dialect)

	return stores, nil
}
