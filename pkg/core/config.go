package core

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/auth"

	"gopkg.in/yaml.v3"
)

// AppConfig represents the main application configuration.
type AppConfig struct {
	// Server holds server-specific configuration.
	Server struct {
		Port                      int      `yaml:"port"`
		PublicBaseURL             string   `yaml:"public_base_url"` // Deprecated: use Endpoint.
		ReadTimeoutMS             int64    `yaml:"read_timeout_ms"`
		WriteTimeoutMS            int64    `yaml:"write_timeout_ms"`
		IdleTimeoutMS             int64    `yaml:"idle_timeout_ms"`
		ReadHeaderMS              int64    `yaml:"read_header_timeout_ms"`
		MaxBodyBytes              int64    `yaml:"max_body_bytes"`
		DebugEvents               bool     `yaml:"debug_events"`
		CORSAllowedOrigins        []string `yaml:"cors_allowed_origins"`
		CORSAllowedHeaders        []string `yaml:"cors_allowed_headers"`
		AllowTenantHeaderFallback bool     `yaml:"allow_tenant_header_fallback"`
		MaxReplayConcurrency      int      `yaml:"max_replay_concurrency"`
	} `yaml:"server"`
	// Providers contains configuration for each Git provider.
	Providers auth.Config `yaml:"providers"`
	// Relaybus holds configuration for the Relaybus message router.
	Relaybus RelaybusConfig `yaml:"relaybus"`
	// Storage holds configuration for installation storage.
	Storage StorageConfig `yaml:"storage"`
	// RedirectBaseURL is where users are redirected after OAuth completion.
	RedirectBaseURL string `yaml:"redirect_base_url"`
	// OAuth holds legacy callback configuration for provider integrations.
	OAuth OAuthConfig `yaml:"oauth"`
	// Auth holds API authentication configuration.
	Auth auth.AuthConfig `yaml:"auth"`
	// Endpoint is the base URL for Connect RPC calls.
	Endpoint string `yaml:"endpoint"`
}

// Config represents the application configuration including rules.
type Config struct {
	AppConfig   `yaml:",inline"`
	Rules       []Rule `yaml:"rules"`
	RulesStrict bool   `yaml:"rules_strict"`
}

// ProviderConfig represents the configuration for a single Git provider.
type ProviderConfig = auth.ProviderConfig

// RelaybusConfig holds the configuration for Relaybus, which handles messaging.
type RelaybusConfig struct {
	Driver       string             `yaml:"driver"`
	Drivers      []string           `yaml:"drivers"`
	Kafka        KafkaConfig        `yaml:"kafka"`
	NATS         NATSConfig         `yaml:"nats"`
	AMQP         AMQPConfig         `yaml:"amqp"`
	HTTP         HTTPConfig         `yaml:"http"`
	PublishRetry PublishRetryConfig `yaml:"publish_retry"`
	DLQDriver    string             `yaml:"dlq_driver"`
}

// KafkaConfig holds configuration for the Kafka pub/sub.
type KafkaConfig struct {
	Brokers     []string `yaml:"brokers" json:"brokers"`
	Broker      string   `yaml:"broker" json:"broker"`
	TopicPrefix string   `yaml:"topic_prefix" json:"topic_prefix"`
	RetryCount  int      `yaml:"retry_count" json:"retry_count"`
}

// NATSConfig holds configuration for the NATS pub/sub.
type NATSConfig struct {
	URL           string `yaml:"url" json:"url"`
	SubjectPrefix string `yaml:"subject_prefix" json:"subject_prefix"`
	RetryCount    int    `yaml:"retry_count" json:"retry_count"`
}

// AMQPConfig holds configuration for the AMQP pub/sub.
type AMQPConfig struct {
	URL                string `yaml:"url" json:"url"`
	Exchange           string `yaml:"exchange" json:"exchange"`
	RoutingKeyTemplate string `yaml:"routing_key_template" json:"routing_key_template"`
	Mandatory          bool   `yaml:"mandatory" json:"mandatory"`
	Immediate          bool   `yaml:"immediate" json:"immediate"`
	RetryCount         int    `yaml:"retry_count" json:"retry_count"`
}

type HTTPConfig struct {
	Endpoint     string `yaml:"endpoint" json:"endpoint"`
	RetryCount   int    `yaml:"retry_count" json:"retry_count"`
	WebhookToken string `yaml:"webhook_token" json:"webhook_token"`
}

type PublishRetryConfig struct {
	Attempts int `yaml:"attempts"`
	DelayMS  int `yaml:"delay_ms"`
}

// StorageConfig holds configuration for SQL-backed installation storage.
type StorageConfig struct {
	Driver            string `yaml:"driver"`
	DSN               string `yaml:"dsn"`
	Dialect           string `yaml:"dialect"`
	AutoMigrate       bool   `yaml:"auto_migrate"`
	MaxOpenConns      int    `yaml:"max_open_conns"`
	MaxIdleConns      int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeMS int64  `yaml:"conn_max_lifetime_ms"`
	ConnMaxIdleTimeMS int64  `yaml:"conn_max_idle_time_ms"`
}

// OAuthConfig holds configuration for OAuth callbacks.
type OAuthConfig struct {
	RedirectBaseURL string `yaml:"redirect_base_url"`
}

// LoadAppConfig loads the main application configuration from a YAML file.
// It applies default values.
func LoadAppConfig(path string) (AppConfig, error) {
	var cfg AppConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	applyDefaults(&cfg)
	return cfg, nil
}

// LoadConfig loads the full application configuration, including rules, from a YAML file.
// It applies defaults and normalizes rules.
func LoadConfig(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	applyDefaults(&cfg.AppConfig)
	normalized, err := normalizeRules(cfg.Rules)
	if err != nil {
		return cfg, err
	}
	cfg.Rules = normalized
	cfg.RulesStrict = cfg.RulesStrict || false

	return cfg, nil
}

// RulesConfig represents the rule-specific parts of the configuration.
type RulesConfig struct {
	Rules    []Rule `yaml:"rules"`
	Strict   bool   `yaml:"rules_strict"`
	TenantID string `yaml:"-"`
	Logger   *log.Logger
}

// LoadRulesConfig loads only the rules from a YAML configuration file.
func LoadRulesConfig(path string) (RulesConfig, error) {
	var cfg RulesConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	normalized, err := normalizeRules(cfg.Rules)
	if err != nil {
		return cfg, err
	}
	cfg.Rules = normalized
	return cfg, nil
}

// NormalizeRules trims and validates rule definitions.
func NormalizeRules(rules []Rule) ([]Rule, error) {
	return normalizeRules(rules)
}

func applyDefaults(cfg *AppConfig) {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.ReadTimeoutMS == 0 {
		cfg.Server.ReadTimeoutMS = 5000
	}
	if cfg.Server.WriteTimeoutMS == 0 {
		cfg.Server.WriteTimeoutMS = 10000
	}
	if cfg.Server.IdleTimeoutMS == 0 {
		cfg.Server.IdleTimeoutMS = 60000
	}
	if cfg.Server.ReadHeaderMS == 0 {
		cfg.Server.ReadHeaderMS = 5000
	}
	if cfg.Server.MaxBodyBytes == 0 {
		cfg.Server.MaxBodyBytes = 1 << 20
	}
	if cfg.Endpoint == "" && cfg.Server.PublicBaseURL != "" {
		cfg.Endpoint = cfg.Server.PublicBaseURL
	}
	if cfg.RedirectBaseURL == "" && cfg.OAuth.RedirectBaseURL != "" {
		cfg.RedirectBaseURL = cfg.OAuth.RedirectBaseURL
	}
	if cfg.Providers.GitHub.Webhook.Path == "" {
		cfg.Providers.GitHub.Webhook.Path = "/webhooks/github"
	}
	if cfg.Providers.GitLab.Webhook.Path == "" {
		cfg.Providers.GitLab.Webhook.Path = "/webhooks/gitlab"
	}
	if cfg.Providers.Bitbucket.Webhook.Path == "" {
		cfg.Providers.Bitbucket.Webhook.Path = "/webhooks/bitbucket"
	}
	if cfg.Relaybus.PublishRetry.Attempts == 0 {
		cfg.Relaybus.PublishRetry.Attempts = 3
	}
	if cfg.Relaybus.PublishRetry.DelayMS == 0 {
		cfg.Relaybus.PublishRetry.DelayMS = 500
	}
	if cfg.Storage.MaxOpenConns == 0 {
		cfg.Storage.MaxOpenConns = 2
	}
	if cfg.Storage.MaxIdleConns == 0 {
		cfg.Storage.MaxIdleConns = 1
	}
	if cfg.Storage.ConnMaxLifetimeMS == 0 {
		cfg.Storage.ConnMaxLifetimeMS = 300000
	}
	if cfg.Storage.ConnMaxIdleTimeMS == 0 {
		cfg.Storage.ConnMaxIdleTimeMS = 60000
	}
	if len(cfg.Server.CORSAllowedOrigins) == 0 {
		cfg.Server.CORSAllowedOrigins = defaultCORSAllowedOrigins(cfg.Endpoint)
	}
	if len(cfg.Server.CORSAllowedHeaders) == 0 {
		cfg.Server.CORSAllowedHeaders = []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"Connect-Protocol-Version",
			"Connect-Timeout-Ms",
			"X-API-Key",
			"X-Tenant-ID",
			"X-Githooks-Tenant-ID",
		}
	}
	if cfg.Server.MaxReplayConcurrency <= 0 {
		cfg.Server.MaxReplayConcurrency = 8
	}
	applyAuthDefaults(cfg)
}

func defaultCORSAllowedOrigins(endpoint string) []string {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return []string{parsed.Scheme + "://" + parsed.Host}
	}
	return []string{
		"http://localhost:3000",
		"http://127.0.0.1:3000",
	}
}

func applyAuthDefaults(cfg *AppConfig) {
	if !cfg.Auth.OAuth2.Enabled {
		return
	}
	oauth2 := &cfg.Auth.OAuth2
	if oauth2.Mode == "" {
		oauth2.Mode = "auto"
	}
	if oauth2.RedirectURL == "" && cfg.Endpoint != "" {
		oauth2.RedirectURL = strings.TrimRight(cfg.Endpoint, "/") + "/auth/callback"
	}
	if len(oauth2.Scopes) == 0 {
		oauth2.Scopes = []string{"openid", "profile", "email"}
	}
}

func normalizeRules(rules []Rule) ([]Rule, error) {
	out := make([]Rule, 0, len(rules))
	for i := range rules {
		rule := rules[i]
		rule.When = strings.TrimSpace(rule.When)
		rule.Emit = EmitList(rule.Emit.Values())
		rule.DriverID = strings.TrimSpace(rule.DriverID)
		if rule.DriverID == "" {
			return nil, fmt.Errorf("rule %d is missing driver_id", i)
		}
		if rule.When == "" || len(rule.Emit) == 0 {
			return nil, fmt.Errorf("rule %d is missing when or emit", i)
		}
		out = append(out, rule)
	}
	return out, nil
}
