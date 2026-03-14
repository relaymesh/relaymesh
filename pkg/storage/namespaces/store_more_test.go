package namespaces

import (
	"context"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/storage"
)

func TestNamespacesStoreValidationFiltersAndTenant(t *testing.T) {
	store, err := Open(Config{Driver: "sqlite", DSN: ":memory:", AutoMigrate: true})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctxA := storage.WithTenant(context.Background(), "tenant-a")
	ctxB := storage.WithTenant(context.Background(), "tenant-b")

	if err := store.UpsertNamespace(ctxA, storage.NamespaceRecord{Provider: "github"}); err == nil {
		t.Fatalf("expected repo_id validation error")
	}
	if err := store.UpsertNamespace(ctxA, storage.NamespaceRecord{RepoID: "1"}); err == nil {
		t.Fatalf("expected provider validation error")
	}

	if got, err := store.GetNamespace(ctxA, "github", "404", ""); err != nil || got != nil {
		t.Fatalf("expected missing namespace nil, err=%v", err)
	}

	if err := store.UpsertNamespace(ctxA, storage.NamespaceRecord{Provider: "github", RepoID: "1", AccountID: "acc-1", InstallationID: "inst-1", ProviderInstanceKey: "k1", Owner: "org", RepoName: "repo", FullName: "org/repo"}); err != nil {
		t.Fatalf("upsert tenant-a: %v", err)
	}
	if err := store.UpsertNamespace(ctxB, storage.NamespaceRecord{Provider: "github", RepoID: "1", AccountID: "acc-2", InstallationID: "inst-2", ProviderInstanceKey: "k1", Owner: "orgb", RepoName: "repo", FullName: "orgb/repo"}); err != nil {
		t.Fatalf("upsert tenant-b: %v", err)
	}

	listA, err := store.ListNamespaces(ctxA, storage.NamespaceFilter{Provider: "github", AccountID: "acc-1", InstallationID: "inst-1", RepoID: "1", Owner: "org", RepoName: "repo", FullName: "org/repo", ProviderInstanceKey: "k1"})
	if err != nil || len(listA) != 1 {
		t.Fatalf("expected filtered list size 1, got %d err=%v", len(listA), err)
	}

	listB, err := store.ListNamespaces(ctxB, storage.NamespaceFilter{Provider: "github"})
	if err != nil || len(listB) != 1 {
		t.Fatalf("expected tenant-b list size 1, got %d err=%v", len(listB), err)
	}

	if err := store.DeleteNamespace(ctxA, "", "1", ""); err == nil {
		t.Fatalf("expected delete validation error")
	}

	rows, err := store.UpdateProviderInstanceKey(ctxA, "github", "k1", "k2", "tenant-a")
	if err != nil {
		t.Fatalf("update provider instance key: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 updated row, got %d", rows)
	}

	updated, err := store.GetNamespace(ctxA, "github", "1", "k2")
	if err != nil || updated == nil {
		t.Fatalf("expected updated namespace key, err=%v", err)
	}
}
