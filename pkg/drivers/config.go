package drivers

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

// RecordsFromConfig converts a Relaybus config into driver records.
func RecordsFromConfig(cfg core.RelaybusConfig) ([]storage.DriverRecord, error) {
	drivers := cfg.Drivers
	if len(drivers) == 0 && cfg.Driver != "" {
		drivers = []string{cfg.Driver}
	}
	if len(drivers) == 0 {
		return nil, nil
	}
	out := make([]storage.DriverRecord, 0, len(drivers))
	for _, name := range drivers {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		configJSON, err := marshalDriverConfig(name, cfg)
		if err != nil {
			return nil, err
		}
		out = append(out, storage.DriverRecord{
			Name:       strings.ToLower(name),
			ConfigJSON: configJSON,
			Enabled:    true,
		})
	}
	return out, nil
}

// ConfigFromRecords builds a Relaybus config from stored driver records.
func ConfigFromRecords(base core.RelaybusConfig, records []storage.DriverRecord) (core.RelaybusConfig, error) {
	cfg := base
	cfg.Drivers = nil
	cfg.Driver = ""
	for _, record := range records {
		if !record.Enabled {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(record.Name))
		if name == "" {
			continue
		}
		if err := applyDriverConfig(&cfg, name, record.ConfigJSON); err != nil {
			return core.RelaybusConfig{}, err
		}
		cfg.Drivers = append(cfg.Drivers, name)
	}
	return cfg, nil
}

func marshalDriverConfig(name string, cfg core.RelaybusConfig) (string, error) {
	switch strings.ToLower(name) {
	case "amqp":
		return marshalJSON(cfg.AMQP)
	case "nats":
		return marshalJSON(cfg.NATS)
	case "kafka":
		return marshalJSON(cfg.Kafka)
	case "http":
		return marshalJSON(cfg.HTTP)
	default:
		return "", errors.New("unsupported driver: " + name)
	}
}

func applyDriverConfig(cfg *core.RelaybusConfig, name, raw string) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	switch strings.ToLower(name) {
	case "amqp":
		return unmarshalJSON(raw, &cfg.AMQP)
	case "nats":
		return unmarshalJSON(raw, &cfg.NATS)
	case "kafka":
		return unmarshalJSON(raw, &cfg.Kafka)
	case "http":
		return unmarshalJSON(raw, &cfg.HTTP)
	default:
		return errors.New("unsupported driver: " + name)
	}
}

// ConfigFromDriver builds a Relaybus config for a single driver from its JSON payload.
func ConfigFromDriver(driverName, configJSON string) (core.RelaybusConfig, error) {
	name := strings.ToLower(strings.TrimSpace(driverName))
	if name == "" {
		return core.RelaybusConfig{}, errors.New("driver name is required")
	}
	cfg := core.RelaybusConfig{
		Driver:  name,
		Drivers: []string{name},
	}
	if err := applyDriverConfig(&cfg, name, configJSON); err != nil {
		return core.RelaybusConfig{}, err
	}
	return cfg, nil
}

func marshalJSON(value interface{}) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func unmarshalJSON(raw string, target interface{}) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), target)
}
