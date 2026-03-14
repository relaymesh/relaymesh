package rules

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/relaymesh/relaymesh/pkg/storage"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Config mirrors the storage configuration for the rules table.
type Config struct {
	Driver      string
	DSN         string
	Dialect     string
	Table       string
	AutoMigrate bool
	Pool        storage.PoolConfig
}

// Store implements storage.RuleStore on top of GORM.
type Store struct {
	db    *gorm.DB
	table string
}

type row struct {
	ID          string    `gorm:"column:id;size:64;primaryKey;index:idx_rules_tenant_id,priority:2"`
	TenantID    string    `gorm:"column:tenant_id;size:64;not null;default:'';index;index:idx_rules_tenant_created,priority:1;index:idx_rules_tenant_id,priority:1"`
	When        string    `gorm:"column:when;type:text;not null"`
	EmitJSON    string    `gorm:"column:emit_json;type:text"`
	Emit        string    `gorm:"column:emit;type:text"`
	DriverID    string    `gorm:"column:driver_id;size:64"`
	TransformJS string    `gorm:"column:transform_js;type:text"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime;index:idx_rules_tenant_created,priority:2,sort:asc"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

type ruleWithDriver struct {
	ID            string    `gorm:"column:id;size:64;primaryKey"`
	TenantID      string    `gorm:"column:tenant_id;size:64;not null;default:'';index;index:idx_rules_tenant_created,priority:1"`
	When          string    `gorm:"column:when;type:text;not null"`
	EmitJSON      string    `gorm:"column:emit_json;type:text"`
	Emit          string    `gorm:"column:emit;type:text"`
	DriverID      string    `gorm:"column:driver_id;size:64"`
	TransformJS   string    `gorm:"column:transform_js;type:text"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime;index:idx_rules_tenant_created,priority:2,sort:asc"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime"`
	DriverName    string    `gorm:"column:driver_name"`
	DriverConfig  string    `gorm:"column:driver_config_json"`
	DriverEnabled bool      `gorm:"column:driver_enabled"`
}

// Open creates a GORM-backed rules store.
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
		table = "githook_rules"
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

// ListRules returns all stored rules.
func (s *Store) ListRules(ctx context.Context) ([]storage.RuleRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	var data []ruleWithDriver
	join := "LEFT JOIN githook_drivers ON githook_rules.driver_id = githook_drivers.id AND githook_rules.tenant_id = githook_drivers.tenant_id"
	query := s.tableDB().WithContext(ctx).
		Select("githook_rules.*, githook_drivers.name as driver_name, githook_drivers.config_json as driver_config_json, githook_drivers.enabled as driver_enabled").
		Joins(join).
		Order("githook_rules.created_at asc")
	query = query.Scopes(storage.TenantScope(ctx, "", "githook_rules.tenant_id"))
	if err := query.Find(&data).Error; err != nil {
		return nil, err
	}
	out := make([]storage.RuleRecord, 0, len(data))
	for _, item := range data {
		out = append(out, fromJoinedRow(item))
	}
	return out, nil
}

// GetRule fetches a rule by ID.
func (s *Store) GetRule(ctx context.Context, id string) (*storage.RuleRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	var data ruleWithDriver
	join := "LEFT JOIN githook_drivers ON githook_rules.driver_id = githook_drivers.id AND githook_rules.tenant_id = githook_drivers.tenant_id"
	query := s.tableDB().WithContext(ctx).
		Select("githook_rules.*, githook_drivers.name as driver_name, githook_drivers.config_json as driver_config_json, githook_drivers.enabled as driver_enabled").
		Joins(join).
		Where("githook_rules.id = ?", id)
	query = query.Scopes(storage.TenantScope(ctx, "", "githook_rules.tenant_id"))
	err := query.Take(&data).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	record := fromJoinedRow(data)
	return &record, nil
}

// CreateRule inserts a new rule.
func (s *Store) CreateRule(ctx context.Context, record storage.RuleRecord) (*storage.RuleRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	if record.When == "" {
		return nil, errors.New("when is required")
	}
	if record.TenantID == "" {
		record.TenantID = storage.TenantFromContext(ctx)
	}
	if record.ID == "" {
		record.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	data, err := toRow(record)
	if err != nil {
		return nil, err
	}
	if err := s.tableDB().WithContext(ctx).Create(&data).Error; err != nil {
		return nil, err
	}
	out := fromRow(data)
	return &out, nil
}

// UpdateRule updates an existing rule.
func (s *Store) UpdateRule(ctx context.Context, record storage.RuleRecord) (*storage.RuleRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	if record.ID == "" {
		return nil, errors.New("id is required")
	}
	if record.When == "" {
		return nil, errors.New("when is required")
	}
	if record.TenantID == "" {
		record.TenantID = storage.TenantFromContext(ctx)
	}
	record.UpdatedAt = time.Now().UTC()
	data, err := toRow(record)
	if err != nil {
		return nil, err
	}
	query := s.tableDB().WithContext(ctx).Where("id = ?", record.ID)
	query = query.Scopes(storage.TenantScope(ctx, "", "tenant_id"))
	if err := query.Updates(&data).Error; err != nil {
		return nil, err
	}
	out := fromRow(data)
	return &out, nil
}

// DeleteRule removes a rule.
func (s *Store) DeleteRule(ctx context.Context, id string) error {
	if s == nil || s.db == nil {
		return errors.New("store is not initialized")
	}
	query := s.tableDB().WithContext(ctx).Where("id = ?", id)
	query = query.Scopes(storage.TenantScope(ctx, "", "tenant_id"))
	return query.Delete(&row{}).Error
}

func (s *Store) migrate() error {
	return s.tableDB().AutoMigrate(&row{})
}

func (s *Store) tableDB() *gorm.DB {
	return s.db.Session(&gorm.Session{NewDB: true}).Table(s.table)
}

func toRow(record storage.RuleRecord) (row, error) {
	emitJSON, err := json.Marshal(record.Emit)
	if err != nil {
		return row{}, err
	}
	return row{
		ID:          record.ID,
		TenantID:    record.TenantID,
		When:        record.When,
		EmitJSON:    string(emitJSON),
		DriverID:    strings.TrimSpace(record.DriverID),
		TransformJS: strings.TrimSpace(record.TransformJS),
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}, nil
}

func fromRow(data row) storage.RuleRecord {
	record := storage.RuleRecord{
		ID:          data.ID,
		TenantID:    data.TenantID,
		When:        data.When,
		DriverID:    strings.TrimSpace(data.DriverID),
		TransformJS: strings.TrimSpace(data.TransformJS),
		CreatedAt:   data.CreatedAt,
		UpdatedAt:   data.UpdatedAt,
	}
	record.Emit = parseEmit(data.EmitJSON, data.Emit)
	return record
}

func fromJoinedRow(data ruleWithDriver) storage.RuleRecord {
	base := row{
		ID:          data.ID,
		TenantID:    data.TenantID,
		When:        data.When,
		EmitJSON:    data.EmitJSON,
		Emit:        data.Emit,
		DriverID:    data.DriverID,
		TransformJS: data.TransformJS,
		CreatedAt:   data.CreatedAt,
		UpdatedAt:   data.UpdatedAt,
	}
	record := fromRow(base)
	record.DriverName = strings.TrimSpace(data.DriverName)
	record.DriverConfigJSON = strings.TrimSpace(data.DriverConfig)
	record.DriverEnabled = data.DriverEnabled
	return record
}

func parseEmit(jsonSource, legacy string) []string {
	switch {
	case jsonSource != "":
		var emit []string
		if err := json.Unmarshal([]byte(jsonSource), &emit); err == nil {
			return emit
		}
		trimmed := strings.TrimSpace(jsonSource)
		if trimmed != "" {
			return []string{trimmed}
		}
	case legacy != "":
		trimmed := strings.TrimSpace(legacy)
		if trimmed != "" {
			return []string{trimmed}
		}
	}
	return nil
}
