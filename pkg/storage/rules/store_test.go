package rules

import (
	"context"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/storage"
	"github.com/relaymesh/relaymesh/pkg/storage/drivers"
)

func TestRulesStoreCRUD(t *testing.T) {
	dsn := "file::memory:?cache=shared"
	driverStore, err := drivers.Open(drivers.Config{Driver: "sqlite", DSN: dsn, AutoMigrate: true})
	if err != nil {
		t.Fatalf("open driver store: %v", err)
	}
	t.Cleanup(func() { _ = driverStore.Close() })

	store, err := Open(Config{Driver: "sqlite", DSN: dsn, AutoMigrate: true})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := storage.WithTenant(context.Background(), "tenant-a")
	created, err := store.CreateRule(ctx, storage.RuleRecord{
		When:        "action == \"opened\"",
		Emit:        []string{"topic"},
		DriverID:    "driver",
		TransformJS: "function transform(payload){ return payload; }",
	})
	if err != nil || created == nil {
		t.Fatalf("create rule: %v", err)
	}
	got, err := store.GetRule(ctx, created.ID)
	if err != nil || got == nil {
		t.Fatalf("get rule: %v", err)
	}
	if got.TransformJS == "" {
		t.Fatalf("expected transform_js to persist")
	}
	list, err := store.ListRules(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list rules: %v", err)
	}
	_, err = store.UpdateRule(ctx, storage.RuleRecord{
		ID:          created.ID,
		When:        "action == \"closed\"",
		Emit:        []string{"topic"},
		DriverID:    "driver",
		TransformJS: "function transform(payload){ payload.closed = true; return payload; }",
		CreatedAt:   created.CreatedAt,
	})
	if err != nil {
		t.Fatalf("update rule: %v", err)
	}
	if err := store.DeleteRule(ctx, created.ID); err != nil {
		t.Fatalf("delete rule: %v", err)
	}
}

func TestRulesStoreListRulesByTenantQualifiesTenantID(t *testing.T) {
	dsn := "file::memory:?cache=shared"
	driverStore, err := drivers.Open(drivers.Config{Driver: "sqlite", DSN: dsn, AutoMigrate: true})
	if err != nil {
		t.Fatalf("open driver store: %v", err)
	}
	t.Cleanup(func() { _ = driverStore.Close() })

	ruleStore, err := Open(Config{Driver: "sqlite", DSN: dsn, AutoMigrate: true})
	if err != nil {
		t.Fatalf("open rule store: %v", err)
	}
	t.Cleanup(func() { _ = ruleStore.Close() })

	tenantID := "tenant-qualified"
	ctx := storage.WithTenant(context.Background(), tenantID)

	driver := storage.DriverRecord{
		ID:         tenantID + ":amqp",
		Name:       "amqp",
		ConfigJSON: "{}",
	}
	if _, err := driverStore.UpsertDriver(ctx, driver); err != nil {
		t.Fatalf("upsert driver: %v", err)
	}
	record, err := driverStore.GetDriverByID(ctx, driver.ID)
	if err != nil {
		t.Fatalf("get driver: %v", err)
	}
	if record == nil {
		t.Fatalf("driver not found")
	}

	created, err := ruleStore.CreateRule(ctx, storage.RuleRecord{
		When:     "action == \"opened\"",
		Emit:     []string{"topic"},
		DriverID: record.ID,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	var stored struct {
		DriverID string
	}
	if err := ruleStore.tableDB().WithContext(ctx).Select("driver_id").Take(&stored).Error; err != nil {
		t.Fatalf("peek rule row: %v", err)
	}
	if stored.DriverID == "" {
		t.Fatalf("driver_id missing in persisted row")
	}
	if stored.DriverID != record.ID {
		t.Fatalf("persisted driver id mismatch: %s vs %s", stored.DriverID, record.ID)
	}

	list, err := ruleStore.ListRules(ctx)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(list))
	}

	if err := ruleStore.DeleteRule(ctx, created.ID); err != nil {
		t.Fatalf("cleanup rule: %v", err)
	}
}
