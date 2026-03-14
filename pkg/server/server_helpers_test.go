package server

import (
	"context"
	"strings"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/storage"
)

func TestRuleKey(t *testing.T) {
	key := ruleKey(" action ", []string{"b", "a", " "}, "driver-x")
	if !strings.HasPrefix(key, "action") || !strings.Contains(key, "a,b") {
		t.Fatalf("unexpected rule key: %q", key)
	}
}

func TestResolveRuleDrivers(t *testing.T) {
	ctx := storage.WithTenant(context.Background(), "tenant-a")
	store := storage.NewMockDriverStore()
	driver, err := store.UpsertDriver(ctx, storage.DriverRecord{Name: "amqp", Enabled: true})
	if err != nil {
		t.Fatalf("upsert driver: %v", err)
	}
	name, err := resolveRuleDriverName(ctx, store, driver.ID)
	if err != nil {
		t.Fatalf("resolve name: %v", err)
	}
	if name != "amqp" {
		t.Fatalf("unexpected driver name: %q", name)
	}
}
