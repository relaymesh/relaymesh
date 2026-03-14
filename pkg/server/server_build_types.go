package server

import (
	driverspkg "github.com/relaymesh/relaymesh/pkg/drivers"
	"github.com/relaymesh/relaymesh/pkg/providerinstance"
	driversstore "github.com/relaymesh/relaymesh/pkg/storage/drivers"
	"github.com/relaymesh/relaymesh/pkg/storage/eventlogs"
	"github.com/relaymesh/relaymesh/pkg/storage/installations"
	"github.com/relaymesh/relaymesh/pkg/storage/namespaces"
	providerinstancestore "github.com/relaymesh/relaymesh/pkg/storage/provider_instances"
	"github.com/relaymesh/relaymesh/pkg/storage/rules"
)

type serverStores struct {
	installStore   *installations.Store
	namespaceStore *namespaces.Store
	ruleStore      *rules.Store
	logStore       *eventlogs.Store
	driverStore    *driversstore.Store
	instanceStore  *providerinstancestore.Store
}

type serverCaches struct {
	driverCache        *driverspkg.Cache
	instanceCache      *providerinstance.Cache
	dynamicDriverCache *driverspkg.DynamicPublisherCache
}
