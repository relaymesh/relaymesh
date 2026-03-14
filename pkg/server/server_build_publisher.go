package server

import (
	"errors"
	"fmt"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/core"
	driverspkg "github.com/relaymesh/relaymesh/pkg/drivers"
)

func buildPublisher(cfg core.Config, driverCache *driverspkg.Cache) (core.Publisher, error) {
	hasBaseDrivers := strings.TrimSpace(cfg.Relaybus.Driver) != "" || len(cfg.Relaybus.Drivers) > 0
	var basePublisher core.Publisher
	if hasBaseDrivers {
		var err error
		basePublisher, err = core.NewPublisher(cfg.Relaybus)
		if err != nil {
			return nil, fmt.Errorf("publisher: %w", err)
		}
	}

	switch {
	case driverCache != nil:
		return driverspkg.NewTenantPublisher(driverCache, basePublisher), nil
	case basePublisher != nil:
		return basePublisher, nil
	default:
		return nil, errors.New("relaybus publisher not configured")
	}
}
