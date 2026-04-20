package core

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadConfigDefaults tests that the default values are applied correctly when loading a config.
func TestLoadConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.yaml")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write app config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Fatalf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Providers.GitHub.Webhook.Path != "/webhooks/github" {
		t.Fatalf("expected default github path, got %q", cfg.Providers.GitHub.Webhook.Path)
	}
	if cfg.Providers.GitLab.Webhook.Path != "/webhooks/gitlab" {
		t.Fatalf("expected default gitlab path, got %q", cfg.Providers.GitLab.Webhook.Path)
	}
	if cfg.Providers.Bitbucket.Webhook.Path != "/webhooks/bitbucket" {
		t.Fatalf("expected default bitbucket path, got %q", cfg.Providers.Bitbucket.Webhook.Path)
	}
	if cfg.Providers.Slack.Webhook.Path != "/webhooks/slack" {
		t.Fatalf("expected default slack path, got %q", cfg.Providers.Slack.Webhook.Path)
	}
	if cfg.Relaybus.Driver != "" {
		t.Fatalf("expected empty default relaybus driver, got %q", cfg.Relaybus.Driver)
	}
	if len(cfg.Relaybus.Drivers) != 0 {
		t.Fatalf("expected no default drivers, got %v", cfg.Relaybus.Drivers)
	}
	if cfg.Storage.MaxOpenConns != 2 {
		t.Fatalf("expected default max_open_conns 2, got %d", cfg.Storage.MaxOpenConns)
	}
	if cfg.Storage.MaxIdleConns != 1 {
		t.Fatalf("expected default max_idle_conns 1, got %d", cfg.Storage.MaxIdleConns)
	}
	if cfg.Storage.ConnMaxLifetimeMS != 300000 {
		t.Fatalf("expected default conn_max_lifetime_ms 300000, got %d", cfg.Storage.ConnMaxLifetimeMS)
	}
	if cfg.Storage.ConnMaxIdleTimeMS != 60000 {
		t.Fatalf("expected default conn_max_idle_time_ms 60000, got %d", cfg.Storage.ConnMaxIdleTimeMS)
	}
}

// TestLoadConfigInvalidRule tests that loading a config with an invalid rule returns an error.
func TestLoadConfigInvalidRule(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "rules:\n  - when: action == \"opened\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write rules config: %v", err)
	}

	if _, err := LoadConfig(path); err == nil {
		t.Fatalf("expected error for missing emit")
	}
}

// TestLoadConfigTrimsFields tests that the fields in a rule are trimmed correctly.
func TestLoadConfigTrimsFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "rules:\n  - when: \"  action == \\\"opened\\\"  \"\n    emit: \"  pr.opened.ready  \"\n    driver_id: amqp\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write rules config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load rules config: %v", err)
	}
	if cfg.Rules[0].When != "action == \"opened\"" {
		t.Fatalf("expected trimmed when, got %q", cfg.Rules[0].When)
	}
	if len(cfg.Rules[0].Emit) != 1 || cfg.Rules[0].Emit[0] != "pr.opened.ready" {
		t.Fatalf("expected trimmed emit, got %v", cfg.Rules[0].Emit)
	}
}

func TestLoadConfigEmitList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "rules:\n  - when: action == \"opened\"\n    emit: [\"pr.opened\", \"audit.pr.opened\"]\n    driver_id: amqp\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write rules config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load rules config: %v", err)
	}
	if len(cfg.Rules[0].Emit) != 2 {
		t.Fatalf("expected 2 emits, got %v", cfg.Rules[0].Emit)
	}
}
