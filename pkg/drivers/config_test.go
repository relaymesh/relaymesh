package drivers

import (
	"encoding/json"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

func TestRecordsFromConfig(t *testing.T) {
	cfg := core.RelaybusConfig{
		Driver: "amqp",
		AMQP: core.AMQPConfig{
			URL:      "amqp://localhost",
			Exchange: "events",
		},
	}
	records, err := RecordsFromConfig(cfg)
	if err != nil {
		t.Fatalf("records from config: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Name != "amqp" || !records[0].Enabled {
		t.Fatalf("unexpected record: %+v", records[0])
	}
}

func TestRecordsFromConfigUnsupported(t *testing.T) {
	cfg := core.RelaybusConfig{
		Drivers: []string{"unsupported"},
	}
	if _, err := RecordsFromConfig(cfg); err == nil {
		t.Fatalf("expected unsupported driver error")
	}
}

func TestConfigFromRecords(t *testing.T) {
	base := core.RelaybusConfig{
		AMQP: core.AMQPConfig{URL: "amqp://base"},
	}
	raw, err := json.Marshal(core.AMQPConfig{URL: "amqp://custom"})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	records := []storage.DriverRecord{
		{
			Name:       "amqp",
			ConfigJSON: string(raw),
			Enabled:    true,
		},
		{
			Name:       "nats",
			ConfigJSON: "{}",
			Enabled:    false,
		},
	}
	cfg, err := ConfigFromRecords(base, records)
	if err != nil {
		t.Fatalf("config from records: %v", err)
	}
	if len(cfg.Drivers) != 1 || cfg.Drivers[0] != "amqp" {
		t.Fatalf("unexpected drivers list: %v", cfg.Drivers)
	}
	if cfg.AMQP.URL != "amqp://custom" {
		t.Fatalf("expected updated amqp url, got %q", cfg.AMQP.URL)
	}
}

func TestMarshalDriverConfigUnsupported(t *testing.T) {
	if _, err := marshalDriverConfig("unknown", core.RelaybusConfig{}); err == nil {
		t.Fatalf("expected unsupported driver error")
	}
}

func TestApplyDriverConfigNil(t *testing.T) {
	if err := applyDriverConfig(nil, "amqp", "{}"); err == nil {
		t.Fatalf("expected error for nil config")
	}
}

func TestConfigFromDriver(t *testing.T) {
	t.Run("missing name", func(t *testing.T) {
		if _, err := ConfigFromDriver("", "{}"); err == nil {
			t.Fatalf("expected missing driver name error")
		}
	})

	t.Run("unsupported driver", func(t *testing.T) {
		if _, err := ConfigFromDriver("custom", "{}"); err == nil {
			t.Fatalf("expected unsupported driver error")
		}
	})

	t.Run("valid amqp", func(t *testing.T) {
		cfg, err := ConfigFromDriver("amqp", `{"url":"amqp://localhost","exchange":"events"}`)
		if err != nil {
			t.Fatalf("config from driver: %v", err)
		}
		if cfg.Driver != "amqp" {
			t.Fatalf("expected amqp driver, got %q", cfg.Driver)
		}
		if len(cfg.Drivers) != 1 || cfg.Drivers[0] != "amqp" {
			t.Fatalf("unexpected drivers: %v", cfg.Drivers)
		}
		if cfg.AMQP.URL != "amqp://localhost" || cfg.AMQP.Exchange != "events" {
			t.Fatalf("unexpected amqp config: %+v", cfg.AMQP)
		}
	})

	t.Run("valid http", func(t *testing.T) {
		cfg, err := ConfigFromDriver("http", `{"endpoint":"http://localhost:8088/{topic}","retry_count":5,"webhook_token":"abc123"}`)
		if err != nil {
			t.Fatalf("config from driver: %v", err)
		}
		if cfg.Driver != "http" {
			t.Fatalf("expected http driver, got %q", cfg.Driver)
		}
		if cfg.HTTP.Endpoint != "http://localhost:8088/{topic}" {
			t.Fatalf("unexpected http config: %+v", cfg.HTTP)
		}
		if cfg.HTTP.RetryCount != 5 {
			t.Fatalf("expected http retry_count=5, got %+v", cfg.HTTP)
		}
		if cfg.HTTP.WebhookToken != "abc123" {
			t.Fatalf("expected webhook token mapping, got %+v", cfg.HTTP)
		}
	})
}

func TestRecordsFromConfigHTTP(t *testing.T) {
	cfg := core.RelaybusConfig{
		Driver: "http",
		HTTP: core.HTTPConfig{
			Endpoint:     "http://localhost:8088/{topic}",
			RetryCount:   3,
			WebhookToken: "tok-1",
		},
	}
	records, err := RecordsFromConfig(cfg)
	if err != nil {
		t.Fatalf("records from config: %v", err)
	}
	if len(records) != 1 || records[0].Name != "http" {
		t.Fatalf("unexpected records: %+v", records)
	}
	parsed, err := ConfigFromDriver("http", records[0].ConfigJSON)
	if err != nil {
		t.Fatalf("config from stored http driver: %v", err)
	}
	if parsed.HTTP.RetryCount != 3 || parsed.HTTP.WebhookToken != "tok-1" {
		t.Fatalf("expected stored http retry/token, got %+v", parsed.HTTP)
	}
}
