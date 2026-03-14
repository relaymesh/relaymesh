package provider_instances

import (
	"context"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/storage"
)

func TestProviderInstanceStoreCRUD(t *testing.T) {
	store, err := Open(Config{Driver: "sqlite", DSN: ":memory:", AutoMigrate: true})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := storage.WithTenant(context.Background(), "tenant-a")
	record, err := store.UpsertProviderInstance(ctx, storage.ProviderInstanceRecord{
		Provider:   "github",
		Key:        "default",
		ConfigJSON: "{}",
		Enabled:    true,
	})
	if err != nil || record == nil {
		t.Fatalf("upsert provider instance: %v", err)
	}
	got, err := store.GetProviderInstance(ctx, "github", "default")
	if err != nil || got == nil {
		t.Fatalf("get provider instance: %v", err)
	}
	list, err := store.ListProviderInstances(ctx, "github")
	if err != nil || len(list) != 1 {
		t.Fatalf("list provider instances: %v", err)
	}
	if err := store.DeleteProviderInstance(ctx, "github", "default"); err != nil {
		t.Fatalf("delete provider instance: %v", err)
	}
}
