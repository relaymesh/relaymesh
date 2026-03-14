package server

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/storage"
)

func resolveRuleDriverName(ctx context.Context, store storage.DriverStore, driverID string) (string, error) {
	if driverID == "" {
		return "", errors.New("driver_id is required")
	}
	if store == nil {
		return "", errors.New("driver store not configured")
	}
	trimmed := strings.TrimSpace(driverID)
	if trimmed == "" {
		return "", errors.New("driver_id is required")
	}
	record, err := store.GetDriverByID(ctx, trimmed)
	if err != nil {
		return "", err
	}
	if record == nil {
		return "", fmt.Errorf("driver not found: %s", trimmed)
	}
	name := strings.TrimSpace(record.Name)
	if name == "" {
		return "", fmt.Errorf("driver %s has empty name", trimmed)
	}
	return name, nil
}

func ruleKey(when string, emit []string, driverID string) string {
	emitKey := normalizeRuleSlice(emit)
	driverKey := strings.TrimSpace(driverID)
	return strings.TrimSpace(when) + "|" + emitKey + "|" + driverKey
}

func normalizeRuleSlice(values []string) string {
	clean := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		clean = append(clean, trimmed)
	}
	sort.Strings(clean)
	return strings.Join(clean, ",")
}
