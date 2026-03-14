package installations

import (
	"context"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/storage"
)

func TestInstallationsStoreCRUD(t *testing.T) {
	store, err := Open(Config{Driver: "sqlite", DSN: ":memory:", AutoMigrate: true})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := storage.WithTenant(context.Background(), "tenant-a")
	record := storage.InstallRecord{
		Provider:       "github",
		AccountID:      "acct",
		InstallationID: "inst",
	}
	if err := store.UpsertInstallation(ctx, record); err != nil {
		t.Fatalf("upsert installation: %v", err)
	}

	got, err := store.GetInstallation(ctx, "github", "acct", "inst")
	if err != nil || got == nil {
		t.Fatalf("get installation: %v", err)
	}
	gotByID, err := store.GetInstallationByInstallationID(ctx, "github", "inst")
	if err != nil || gotByID == nil {
		t.Fatalf("get installation by id: %v", err)
	}

	list, err := store.ListInstallations(ctx, "github", "")
	if err != nil || len(list) != 1 {
		t.Fatalf("list installations: %v", err)
	}

	if err := store.DeleteInstallation(ctx, "github", "acct", "inst", ""); err != nil {
		t.Fatalf("delete installation: %v", err)
	}
}
