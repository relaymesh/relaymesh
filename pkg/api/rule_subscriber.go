package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/storage"
	"github.com/relaymesh/relaymesh/sdk/go/worker"
)

// validateRuleSubscriber builds a subscriber for the stored driver configuration so the
// rule creation flow verifies that the referenced driver is usable before persisting.
func validateRuleSubscriber(ctx context.Context, logger *log.Logger, store storage.DriverStore, driverID string) error {
	if store == nil {
		return errors.New("driver store not configured")
	}
	trimmed := strings.TrimSpace(driverID)
	if trimmed == "" {
		return errors.New("driver_id is required")
	}
	record, err := store.GetDriverByID(ctx, trimmed)
	if err != nil {
		return err
	}
	if record == nil {
		return fmt.Errorf("driver not found: %s", trimmed)
	}
	if !record.Enabled {
		return fmt.Errorf("driver %s is disabled", trimmed)
	}
	if logger != nil {
		logger.Printf("validating subscriber for driver=%s tenant=%s", trimmed, storage.TenantFromContext(ctx))
	}
	if err := worker.ValidateSubscriber(record.Name, record.ConfigJSON); err != nil {
		return err
	}
	if logger != nil {
		logger.Printf("validated subscriber for driver=%s", trimmed)
	}
	return nil
}
