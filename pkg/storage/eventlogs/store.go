package eventlogs

import (
	"errors"
	"time"

	"github.com/relaymesh/relaymesh/pkg/storage"

	"gorm.io/gorm"
)

// Config mirrors the storage configuration for the event logs table.
type Config struct {
	Driver      string
	DSN         string
	Dialect     string
	Table       string
	AutoMigrate bool
	Pool        storage.PoolConfig
}

// Store implements storage.EventLogStore on top of GORM.
type Store struct {
	db    *gorm.DB
	table string
}

type row struct {
	ID              string    `gorm:"column:id;size:64;primaryKey;index:idx_el_tenant_created_id,priority:3,sort:desc"`
	TenantID        string    `gorm:"column:tenant_id;size:64;not null;default:'';index;index:idx_event_logs_tenant_created,priority:1;index:idx_event_logs_tenant_provider_created,priority:1;index:idx_event_logs_tenant_status_created,priority:1;index:idx_event_logs_tenant_matched_created,priority:1;index:idx_el_tenant_rule_id,priority:1;index:idx_el_tenant_installation,priority:1;index:idx_el_tenant_namespace,priority:1;index:idx_el_tenant_name_created,priority:1;index:idx_el_tenant_topic_created,priority:1;index:idx_el_tenant_request,priority:1;index:idx_el_tenant_created_id,priority:1"`
	Provider        string    `gorm:"column:provider;size:32;not null;index;index:idx_event_logs_tenant_provider_created,priority:2"`
	Name            string    `gorm:"column:name;size:128;not null;index;index:idx_el_tenant_name_created,priority:2"`
	RequestID       string    `gorm:"column:request_id;size:128;index;index:idx_el_tenant_request,priority:2"`
	StateID         string    `gorm:"column:state_id;size:128;index"`
	InstallationID  string    `gorm:"column:installation_id;size:128;index;index:idx_el_tenant_installation,priority:2"`
	NamespaceID     string    `gorm:"column:namespace_id;size:128;index;index:idx_el_tenant_namespace,priority:2"`
	NamespaceName   string    `gorm:"column:namespace_name;size:256;index"`
	Topic           string    `gorm:"column:topic;size:128;index;index:idx_el_tenant_topic_created,priority:2"`
	RuleID          string    `gorm:"column:rule_id;size:64;index;index:idx_el_tenant_rule_id,priority:2"`
	RuleWhen        string    `gorm:"column:rule_when;type:text"`
	DriversJSON     string    `gorm:"column:drivers_json;type:text"`
	HeadersJSON     string    `gorm:"column:headers_json;type:text"`
	Body            string    `gorm:"column:body;type:text"`
	TransformedBody string    `gorm:"column:transformed_body;type:text"`
	BodyHash        string    `gorm:"column:body_hash;size:64;index"`
	Matched         bool      `gorm:"column:matched;not null;default:false;index;index:idx_event_logs_tenant_matched_created,priority:2"`
	Status          string    `gorm:"column:status;size:32;not null;default:'queued';index;index:idx_event_logs_tenant_status_created,priority:2"`
	ErrorMessage    string    `gorm:"column:error_message;type:text"`
	LatencyMS       int64     `gorm:"column:latency_ms;not null;default:0;index"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime;index;index:idx_event_logs_tenant_created,priority:2,sort:desc;index:idx_event_logs_tenant_provider_created,priority:3,sort:desc;index:idx_event_logs_tenant_status_created,priority:3,sort:desc;index:idx_event_logs_tenant_matched_created,priority:3,sort:desc;index:idx_el_tenant_name_created,priority:3,sort:desc;index:idx_el_tenant_topic_created,priority:3,sort:desc;index:idx_el_tenant_created_id,priority:2,sort:desc"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

// Open creates a GORM-backed event logs store.
func Open(cfg Config) (*Store, error) {
	if cfg.Driver == "" && cfg.Dialect == "" {
		return nil, errors.New("storage driver or dialect is required")
	}
	if cfg.DSN == "" {
		return nil, errors.New("storage dsn is required")
	}
	driver, err := storage.ResolveSQLDriver(cfg.Driver, cfg.Dialect)
	if err != nil {
		return nil, err
	}
	gormDB, err := storage.OpenGorm(driver, cfg.DSN)
	if err != nil {
		return nil, err
	}
	if err := storage.ApplyPoolConfig(gormDB, cfg.Pool); err != nil {
		return nil, err
	}
	table := cfg.Table
	if table == "" {
		table = "githook_event_logs"
	}
	store := &Store{db: gormDB, table: table}
	if cfg.AutoMigrate {
		if err := store.migrate(); err != nil {
			return nil, err
		}
	}
	return store, nil
}

// Close closes the underlying DB connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *Store) migrate() error {
	return s.tableDB().AutoMigrate(&row{})
}

func (s *Store) tableDB() *gorm.DB {
	return s.db.Session(&gorm.Session{NewDB: true}).Table(s.table)
}
