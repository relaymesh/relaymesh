package provider_instances

import (
	"context"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/storage"
)

func TestProviderInstancesStoreValidationAndOperations(t *testing.T) {
	store, err := Open(Config{Driver: "sqlite", DSN: ":memory:", AutoMigrate: true})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctxA := storage.WithTenant(context.Background(), "tenant-a")
	ctxB := storage.WithTenant(context.Background(), "tenant-b")

	if _, err := store.UpsertProviderInstance(ctxA, storage.ProviderInstanceRecord{Provider: "github"}); err == nil {
		t.Fatalf("expected key validation error")
	}
	if _, err := store.UpsertProviderInstance(ctxA, storage.ProviderInstanceRecord{Key: "default"}); err == nil {
		t.Fatalf("expected provider validation error")
	}

	if _, err := store.GetProviderInstance(ctxA, "", "default"); err == nil {
		t.Fatalf("expected get validation error")
	}

	if _, err := store.UpsertProviderInstance(ctxA, storage.ProviderInstanceRecord{Provider: "github", Key: "default", Enabled: true}); err != nil {
		t.Fatalf("upsert tenant-a: %v", err)
	}
	if _, err := store.UpsertProviderInstance(ctxB, storage.ProviderInstanceRecord{Provider: "github", Key: "default", Enabled: true}); err != nil {
		t.Fatalf("upsert tenant-b: %v", err)
	}

	listA, err := store.ListProviderInstances(ctxA, "github")
	if err != nil || len(listA) != 1 {
		t.Fatalf("expected tenant-a list size 1, got %d err=%v", len(listA), err)
	}

	rows, err := store.UpdateProviderInstanceKey(ctxA, "github", "default", "renamed", "tenant-a")
	if err != nil {
		t.Fatalf("update key: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 updated row, got %d", rows)
	}

	if _, err := store.GetProviderInstance(ctxA, "github", "renamed"); err != nil {
		t.Fatalf("get renamed instance: %v", err)
	}

	if err := store.DeleteProviderInstanceForTenant(ctxA, "github", "renamed", "tenant-a"); err != nil {
		t.Fatalf("delete for tenant: %v", err)
	}
	if got, err := store.GetProviderInstance(ctxA, "github", "renamed"); err != nil || got != nil {
		t.Fatalf("expected deleted instance nil, err=%v", err)
	}

	if err := store.DeleteProviderInstance(ctxB, "github", "default"); err != nil {
		t.Fatalf("delete default for tenant-b: %v", err)
	}
}

func TestProviderInstancesHelpers(t *testing.T) {
	if got := quoteQualifiedIdent("postgres", "public.table"); got != `"public"."table"` {
		t.Fatalf("unexpected postgres quote: %s", got)
	}
	if got := quoteQualifiedIdent("mysql", "db.table"); got != "`db`.`table`" {
		t.Fatalf("unexpected mysql quote: %s", got)
	}

	store, err := Open(Config{Driver: "sqlite", DSN: ":memory:", AutoMigrate: true})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	hasPK, err := hasPrimaryKey(store.db, store.table, "sqlite")
	if err != nil {
		t.Fatalf("hasPrimaryKey sqlite: %v", err)
	}
	_ = hasPK

	unknown, err := hasPrimaryKey(store.db, store.table, "unknown")
	if err != nil {
		t.Fatalf("hasPrimaryKey unknown: %v", err)
	}
	if unknown {
		t.Fatalf("expected unknown dialect to return false")
	}
}

func TestProviderInstancesNilStoreBranches(t *testing.T) {
	var s *Store
	ctx := context.Background()

	if err := s.Close(); err != nil {
		t.Fatalf("nil close should be nil: %v", err)
	}
	if _, err := s.ListProviderInstances(ctx, "github"); err == nil {
		t.Fatalf("expected list on nil store error")
	}
	if _, err := s.GetProviderInstance(ctx, "github", "default"); err == nil {
		t.Fatalf("expected get on nil store error")
	}
	if _, err := s.UpsertProviderInstance(ctx, storage.ProviderInstanceRecord{Provider: "github", Key: "default"}); err == nil {
		t.Fatalf("expected upsert on nil store error")
	}
	if err := s.DeleteProviderInstance(ctx, "github", "default"); err == nil {
		t.Fatalf("expected delete on nil store error")
	}
	if _, err := s.UpdateProviderInstanceKey(ctx, "github", "old", "new", "tenant-a"); err == nil {
		t.Fatalf("expected update key on nil store error")
	}
	if err := s.DeleteProviderInstanceForTenant(ctx, "github", "default", "tenant-a"); err == nil {
		t.Fatalf("expected delete for tenant on nil store error")
	}
	if err := s.backfillIDs(); err != nil {
		t.Fatalf("nil backfill should be nil: %v", err)
	}
	if err := s.ensurePrimaryKey(); err != nil {
		t.Fatalf("nil ensurePrimaryKey should be nil: %v", err)
	}
}
