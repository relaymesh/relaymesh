package worker

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
)

func TestDriversClientValidation(t *testing.T) {
	client := DriversClient{}
	if _, err := client.GetDriver(context.Background(), ""); err == nil {
		t.Fatalf("expected driver name error")
	}
	client.BaseURL = ""
	if _, err := client.GetDriver(context.Background(), "amqp"); err == nil {
		t.Fatalf("expected base url error")
	}
}

func TestRulesClientValidation(t *testing.T) {
	client := RulesClient{}
	if _, err := client.ListRuleTopics(context.Background()); err == nil {
		t.Fatalf("expected base url error")
	}
	if _, err := client.GetRule(context.Background(), ""); err == nil {
		t.Fatalf("expected rule id error")
	}
	client.BaseURL = ""
	if _, err := client.GetRule(context.Background(), "id"); err == nil {
		t.Fatalf("expected base url error")
	}
}

func TestEventLogsClientValidation(t *testing.T) {
	client := EventLogsClient{}
	if err := client.UpdateStatus(context.Background(), "", "success", ""); err == nil {
		t.Fatalf("expected log id error")
	}
	if err := client.UpdateStatus(context.Background(), "id", "", ""); err == nil {
		t.Fatalf("expected status error")
	}
	client.BaseURL = ""
	if err := client.UpdateStatus(context.Background(), "id", "success", ""); err == nil {
		t.Fatalf("expected base url error")
	}
}

func TestInstallationsClientValidation(t *testing.T) {
	client := InstallationsClient{}
	if _, err := client.GetByInstallationID(context.Background(), "", "id"); err == nil {
		t.Fatalf("expected provider error")
	}
	if _, err := client.GetByInstallationID(context.Background(), "github", ""); err == nil {
		t.Fatalf("expected installation id error")
	}
	if _, err := client.GetByInstallationID(context.Background(), "github", "id"); err == nil {
		t.Fatalf("expected base url error")
	}
}

func TestFromProtoInstallation(t *testing.T) {
	record := fromProtoInstallation(nil)
	if record.Provider != "" {
		t.Fatalf("expected empty record")
	}
	now := time.Now()
	proto := &cloudv1.InstallRecord{
		Provider:       "github",
		InstallationId: "id",
		ExpiresAt:      timestamppb.New(now),
	}
	record = fromProtoInstallation(proto)
	if record.Provider != "github" || record.InstallationID != "id" {
		t.Fatalf("unexpected record: %+v", record)
	}
	if record.ExpiresAt == nil {
		t.Fatalf("expected expires at")
	}
}
