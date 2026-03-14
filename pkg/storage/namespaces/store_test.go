package namespaces

import (
	"context"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/storage"
)

func TestNamespacesStoreCRUD(t *testing.T) {
	store, err := Open(Config{Driver: "sqlite", DSN: ":memory:", AutoMigrate: true})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := storage.WithTenant(context.Background(), "tenant-a")
	record := storage.NamespaceRecord{
		Provider: "github",
		RepoID:   "1",
		Owner:    "org",
		RepoName: "repo",
		FullName: "org/repo",
	}
	if err := store.UpsertNamespace(ctx, record); err != nil {
		t.Fatalf("upsert namespace: %v", err)
	}
	got, err := store.GetNamespace(ctx, "github", "1", "")
	if err != nil || got == nil {
		t.Fatalf("get namespace: %v", err)
	}
	list, err := store.ListNamespaces(ctx, storage.NamespaceFilter{Provider: "github"})
	if err != nil || len(list) != 1 {
		t.Fatalf("list namespaces: %v", err)
	}
	if err := store.DeleteNamespace(ctx, "github", "1", ""); err != nil {
		t.Fatalf("delete namespace: %v", err)
	}
}
