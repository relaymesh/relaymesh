package provider_instances

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/relaymesh/relaymesh/pkg/storage"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Config mirrors the storage configuration for the provider instances table.
type Config struct {
	Driver      string
	DSN         string
	Dialect     string
	Table       string
	AutoMigrate bool
	Pool        storage.PoolConfig
}

// Store implements storage.ProviderInstanceStore on top of GORM.
type Store struct {
	db    *gorm.DB
	table string
}

type row struct {
	ID              string    `gorm:"column:id;size:64;primaryKey"`
	TenantID        string    `gorm:"column:tenant_id;size:64;not null;default:'';uniqueIndex:idx_provider_instance,priority:1;index:idx_pi_tenant_provider,priority:1"`
	Provider        string    `gorm:"column:provider;size:32;not null;uniqueIndex:idx_provider_instance,priority:2;index:idx_pi_tenant_provider,priority:2"`
	Key             string    `gorm:"column:instance_key;size:64;not null;uniqueIndex:idx_provider_instance,priority:3"`
	ConfigJSON      string    `gorm:"column:config_json;type:text"`
	RedirectBaseURL string    `gorm:"column:redirect_base_url;type:text"`
	Enabled         bool      `gorm:"column:enabled;not null;default:true"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// Open creates a GORM-backed provider instances store.
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
		table = "githook_provider_instances"
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

// ListProviderInstances returns all instances for a provider.
func (s *Store) ListProviderInstances(ctx context.Context, provider string) ([]storage.ProviderInstanceRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	query := s.tableDB().WithContext(ctx).Order("instance_key asc")
	if provider = strings.TrimSpace(provider); provider != "" {
		query = query.Where("provider = ?", provider)
	}
	query = query.Scopes(storage.TenantScope(ctx, "", "tenant_id"))
	var data []row
	if err := query.Find(&data).Error; err != nil {
		return nil, err
	}
	out := make([]storage.ProviderInstanceRecord, 0, len(data))
	for _, item := range data {
		out = append(out, fromRow(item))
	}
	return out, nil
}

// GetProviderInstance returns a provider instance by key.
func (s *Store) GetProviderInstance(ctx context.Context, provider, key string) (*storage.ProviderInstanceRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	provider = strings.TrimSpace(provider)
	key = strings.TrimSpace(key)
	if provider == "" || key == "" {
		return nil, errors.New("provider and key are required")
	}
	query := s.tableDB().WithContext(ctx).Where("provider = ? AND instance_key = ?", provider, key)
	query = query.Scopes(storage.TenantScope(ctx, "", "tenant_id"))
	var data row
	err := query.Take(&data).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	record := fromRow(data)
	return &record, nil
}

// UpsertProviderInstance inserts or updates a provider instance.
func (s *Store) UpsertProviderInstance(ctx context.Context, record storage.ProviderInstanceRecord) (*storage.ProviderInstanceRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	if record.Provider == "" || record.Key == "" {
		return nil, errors.New("provider and key are required")
	}
	if record.TenantID == "" {
		record.TenantID = storage.TenantFromContext(ctx)
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	if record.ID == "" {
		record.ID = storage.ProviderInstanceRecordID(record)
	}
	data := toRow(record)
	err := s.tableDB().
		WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "provider"}, {Name: "instance_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"config_json", "redirect_base_url", "enabled", "updated_at"}),
		}).
		Create(&data).Error
	if err != nil {
		return nil, err
	}
	out := fromRow(data)
	return &out, nil
}

// DeleteProviderInstance removes a provider instance.
func (s *Store) DeleteProviderInstance(ctx context.Context, provider, key string) error {
	if s == nil || s.db == nil {
		return errors.New("store is not initialized")
	}
	provider = strings.TrimSpace(provider)
	key = strings.TrimSpace(key)
	if provider == "" || key == "" {
		return errors.New("provider and key are required")
	}
	query := s.tableDB().WithContext(ctx).Where("provider = ? AND instance_key = ?", provider, key)
	query = query.Scopes(storage.TenantScope(ctx, "", "tenant_id"))
	return query.Delete(&row{}).Error
}

// UpdateProviderInstanceKey updates the instance key for a provider and tenant.
func (s *Store) UpdateProviderInstanceKey(ctx context.Context, provider, oldKey, newKey, tenantID string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("store is not initialized")
	}
	provider = strings.TrimSpace(provider)
	oldKey = strings.TrimSpace(oldKey)
	newKey = strings.TrimSpace(newKey)
	tenantID = strings.TrimSpace(tenantID)
	if provider == "" || oldKey == "" || newKey == "" {
		return 0, errors.New("provider and keys are required")
	}
	query := s.tableDB().
		WithContext(ctx).
		Where("provider = ? AND instance_key = ?", provider, oldKey).
		Where("tenant_id = ?", tenantID)
	result := query.Updates(map[string]interface{}{
		"instance_key": newKey,
		"updated_at":   time.Now().UTC(),
	})
	return result.RowsAffected, result.Error
}

// DeleteProviderInstanceForTenant removes a provider instance for a specific tenant.
func (s *Store) DeleteProviderInstanceForTenant(ctx context.Context, provider, key, tenantID string) error {
	if s == nil || s.db == nil {
		return errors.New("store is not initialized")
	}
	provider = strings.TrimSpace(provider)
	key = strings.TrimSpace(key)
	tenantID = strings.TrimSpace(tenantID)
	if provider == "" || key == "" {
		return errors.New("provider and key are required")
	}
	query := s.tableDB().
		WithContext(ctx).
		Where("provider = ? AND instance_key = ?", provider, key).
		Where("tenant_id = ?", tenantID)
	return query.Delete(&row{}).Error
}

func (s *Store) migrate() error {
	if err := s.tableDB().AutoMigrate(&row{}); err != nil {
		return err
	}
	if err := s.backfillIDs(); err != nil {
		return err
	}
	if err := s.ensurePrimaryKey(); err != nil {
		return err
	}
	return nil
}

func (s *Store) tableDB() *gorm.DB {
	return s.db.Session(&gorm.Session{NewDB: true}).Table(s.table)
}

func (s *Store) backfillIDs() error {
	if s == nil || s.db == nil {
		return nil
	}
	var data []row
	if err := s.tableDB().Where("id = '' OR id IS NULL").Find(&data).Error; err != nil {
		return err
	}
	for _, item := range data {
		record := fromRow(item)
		if strings.TrimSpace(record.ID) == "" {
			record.ID = storage.ProviderInstanceRecordID(record)
		}
		if strings.TrimSpace(record.ID) == "" {
			continue
		}
		err := s.tableDB().
			Where("tenant_id = ? AND provider = ? AND instance_key = ?", item.TenantID, item.Provider, item.Key).
			Update("id", record.ID).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensurePrimaryKey() error {
	if s == nil || s.db == nil {
		return nil
	}
	dialect := s.db.Name()
	hasPK, err := hasPrimaryKey(s.db, s.table, dialect)
	if err != nil {
		return err
	}
	if hasPK {
		return nil
	}
	if dialect == "sqlite" {
		return nil
	}
	table := quoteQualifiedIdent(dialect, s.table)
	return s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY (id)", table)).Error
}

func hasPrimaryKey(db *gorm.DB, table, dialect string) (bool, error) {
	switch dialect {
	case "postgres":
		var one int
		err := db.Raw(
			`SELECT 1 FROM information_schema.table_constraints
WHERE table_schema = current_schema()
  AND table_name = ?
  AND constraint_type = 'PRIMARY KEY'
LIMIT 1`,
			table,
		).Row().Scan(&one)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		return one == 1, nil
	case "mysql":
		var one int
		err := db.Raw(
			`SELECT 1 FROM information_schema.table_constraints
WHERE table_schema = database()
  AND table_name = ?
  AND constraint_type = 'PRIMARY KEY'
LIMIT 1`,
			table,
		).Row().Scan(&one)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		return one == 1, nil
	case "sqlite":
		type columnInfo struct {
			CID     int
			Name    string
			Type    string
			NotNull int
			Default *string
			Primary int
		}
		var cols []columnInfo
		if err := db.Raw(fmt.Sprintf("PRAGMA table_info(%s)", quoteQualifiedIdent(dialect, table))).Scan(&cols).Error; err != nil {
			return false, err
		}
		for _, col := range cols {
			if col.Primary == 1 {
				return true, nil
			}
		}
		return false, nil
	default:
		return false, nil
	}
}

func quoteQualifiedIdent(dialect, value string) string {
	quote := `"`
	if dialect == "mysql" {
		quote = "`"
	}
	parts := strings.Split(value, ".")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parts[i] = quote + part + quote
	}
	return strings.Join(parts, ".")
}

func toRow(record storage.ProviderInstanceRecord) row {
	return row{
		ID:              record.ID,
		TenantID:        record.TenantID,
		Provider:        record.Provider,
		Key:             record.Key,
		ConfigJSON:      record.ConfigJSON,
		RedirectBaseURL: record.RedirectBaseURL,
		Enabled:         record.Enabled,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
	}
}

func fromRow(data row) storage.ProviderInstanceRecord {
	return storage.ProviderInstanceRecord{
		ID:              data.ID,
		TenantID:        data.TenantID,
		Provider:        data.Provider,
		Key:             data.Key,
		ConfigJSON:      data.ConfigJSON,
		RedirectBaseURL: data.RedirectBaseURL,
		Enabled:         data.Enabled,
		CreatedAt:       data.CreatedAt,
		UpdatedAt:       data.UpdatedAt,
	}
}
