package installations

import (
	"context"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/storage"
)

func TestInstallationsStoreValidationAndTenantScoping(t *testing.T) {
	if _, err := Open(Config{DSN: ":memory:"}); err == nil {
		t.Fatalf("expected missing driver error")
	}
	if _, err := Open(Config{Driver: "sqlite"}); err == nil {
		t.Fatalf("expected missing dsn error")
	}

	store, err := Open(Config{Driver: "sqlite", DSN: ":memory:", AutoMigrate: true})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctxA := storage.WithTenant(context.Background(), "tenant-a")
	ctxB := storage.WithTenant(context.Background(), "tenant-b")

	if err := store.UpsertInstallation(ctxA, storage.InstallRecord{}); err == nil {
		t.Fatalf("expected provider required error")
	}

	if got, err := store.GetInstallation(ctxA, "github", "missing", "missing"); err != nil || got != nil {
		t.Fatalf("expected missing installation nil, err=%v", err)
	}

	recA := storage.InstallRecord{Provider: "github", AccountID: "acct", InstallationID: "inst", ProviderInstanceKey: "k1"}
	if err := store.UpsertInstallation(ctxA, recA); err != nil {
		t.Fatalf("upsert tenant-a: %v", err)
	}
	recB := storage.InstallRecord{Provider: "github", AccountID: "acct", InstallationID: "inst", ProviderInstanceKey: "k1"}
	if err := store.UpsertInstallation(ctxB, recB); err != nil {
		t.Fatalf("upsert tenant-b: %v", err)
	}

	listA, err := store.ListInstallations(ctxA, "github", "")
	if err != nil || len(listA) != 1 {
		t.Fatalf("expected tenant-a list size 1, got %d err=%v", len(listA), err)
	}
	listB, err := store.ListInstallations(ctxB, "github", "")
	if err != nil || len(listB) != 1 {
		t.Fatalf("expected tenant-b list size 1, got %d err=%v", len(listB), err)
	}

	if _, err := store.GetInstallationByInstallationIDAndInstanceKey(ctxA, "", "inst", "k1"); err == nil {
		t.Fatalf("expected validation error for empty provider")
	}
	if _, err := store.GetInstallationByInstallationIDAndInstanceKey(ctxA, "github", "", "k1"); err == nil {
		t.Fatalf("expected validation error for empty installation id")
	}
	if _, err := store.GetInstallationByInstallationIDAndInstanceKey(ctxA, "github", "inst", ""); err == nil {
		t.Fatalf("expected validation error for empty key")
	}

	if err := store.DeleteInstallation(ctxA, "", "acct", "inst", "k1"); err == nil {
		t.Fatalf("expected validation error for delete")
	}

	rows, err := store.UpdateProviderInstanceKey(ctxA, "github", "k1", "k2", "tenant-a")
	if err != nil {
		t.Fatalf("update provider instance key: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 updated row, got %d", rows)
	}

	got, err := store.GetInstallationByInstallationIDAndInstanceKey(ctxA, "github", "inst", "k2")
	if err != nil || got == nil {
		t.Fatalf("expected updated key record, err=%v", err)
	}
}
