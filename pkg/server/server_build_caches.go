package server

import (
	"context"
	"fmt"
	"log"

	"github.com/relaymesh/relaymesh/pkg/core"
	driverspkg "github.com/relaymesh/relaymesh/pkg/drivers"
	"github.com/relaymesh/relaymesh/pkg/providerinstance"
)

func buildCaches(ctx context.Context, stores serverStores, cfg core.Config, logger *log.Logger, addCloser func(func())) (serverCaches, error) {
	caches := serverCaches{
		dynamicDriverCache: driverspkg.NewDynamicPublisherCache(),
	}
	addCloser(func() { _ = caches.dynamicDriverCache.Close() })

	if stores.driverStore != nil {
		caches.driverCache = driverspkg.NewCache(stores.driverStore, cfg.Relaybus, logger)
		if err := caches.driverCache.Refresh(ctx); err != nil {
			return caches, fmt.Errorf("drivers cache: %w", err)
		}
		addCloser(caches.driverCache.Close)
	}

	if stores.instanceStore != nil {
		caches.instanceCache = providerinstance.NewCache(stores.instanceStore, logger)
		if err := caches.instanceCache.Refresh(ctx); err != nil {
			return caches, fmt.Errorf("provider instances cache: %w", err)
		}
		addCloser(caches.instanceCache.Close)
	}

	return caches, nil
}
