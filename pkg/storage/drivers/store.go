package drivers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/relaymesh/relaymesh/pkg/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Config mirrors the storage configuration for the drivers table.
type Config struct {
	Driver      string
	DSN         string
	Dialect     string
	Table       string
	AutoMigrate bool
	Pool        storage.PoolConfig
}

// Store implements storage.DriverStore on top of GORM.
type Store struct {
	db    *gorm.DB
	table string
}

const globalTenantID = ""

type row struct {
	TenantID   string    `gorm:"column:tenant_id;size:64;not null;default:'';uniqueIndex:idx_driver,priority:1;uniqueIndex:idx_driver_id,priority:1;index:idx_driver_tenant,priority:1;index:idx_driver_tenant_name,priority:1"`
	ID         string    `gorm:"column:id;size:64;not null;default:'';uniqueIndex:idx_driver_id,priority:2"`
	Name       string    `gorm:"column:name;size:64;not null;uniqueIndex:idx_driver,priority:2;index:idx_driver_tenant_name,priority:2"`
	ConfigJSON string    `gorm:"column:config_json;type:text"`
	Enabled    bool      `gorm:"column:enabled;not null;default:true"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt  time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// Open creates a GORM-backed drivers store.
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
		table = "githook_drivers"
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

// ListDrivers returns all stored drivers for the current tenant context.
func (s *Store) ListDrivers(ctx context.Context) ([]storage.DriverRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	tenantID := tenantIDFromContext(ctx)
	query := s.tableDB().WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("name asc")
	var data []row
	if err := query.Find(&data).Error; err != nil {
		return nil, err
	}
	out := make([]storage.DriverRecord, 0, len(data))
	for _, item := range data {
		out = append(out, fromRow(item))
	}
	return out, nil
}

// GetDriver returns a driver configuration by name.
func (s *Store) GetDriver(ctx context.Context, name string) (*storage.DriverRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	tenantID := tenantIDFromContext(ctx)
	query := s.tableDB().WithContext(ctx).
		Where("name = ?", name).
		Where("tenant_id = ?", tenantID)
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

// GetDriverByID returns a driver configuration by ID.
func (s *Store) GetDriverByID(ctx context.Context, id string) (*storage.DriverRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("id is required")
	}
	tenantID := tenantIDFromContext(ctx)
	query := s.tableDB().WithContext(ctx).
		Where("id = ?", id).
		Where("tenant_id = ?", tenantID)
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

// UpsertDriver inserts or updates a driver record.
func (s *Store) UpsertDriver(ctx context.Context, record storage.DriverRecord) (*storage.DriverRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	if record.Name == "" {
		return nil, errors.New("name is required")
	}
	tenantID := tenantIDFromContext(ctx)
	record.TenantID = tenantID
	record.ID = strings.TrimSpace(record.ID)
	if record.ID == "" {
		if tenantID != "" {
			record.ID = fmt.Sprintf("%s:%s", tenantID, record.Name)
		} else {
			record.ID = record.Name
		}
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	data := toRow(record)
	if err := s.tableDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "name"}},
				DoUpdates: clause.AssignmentColumns([]string{"config_json", "enabled", "updated_at"}),
			}).
			Create(&data).Error
	}); err != nil {
		return nil, err
	}
	out := fromRow(data)
	return &out, nil
}

// DeleteDriver removes a driver configuration.
func (s *Store) DeleteDriver(ctx context.Context, name string) error {
	if s == nil || s.db == nil {
		return errors.New("store is not initialized")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name is required")
	}
	tenantID := tenantIDFromContext(ctx)
	query := s.tableDB().WithContext(ctx).
		Where("name = ?", name).
		Where("tenant_id = ?", tenantID)
	return query.Delete(&row{}).Error
}

func (s *Store) migrate() error {
	return s.tableDB().AutoMigrate(&row{})
}

func (s *Store) tableDB() *gorm.DB {
	return s.db.Session(&gorm.Session{NewDB: true}).Table(s.table)
}

func tenantIDFromContext(ctx context.Context) string {
	return storage.ResolveTenant(ctx, globalTenantID)
}

func toRow(record storage.DriverRecord) row {
	return row{
		TenantID:   record.TenantID,
		ID:         record.ID,
		Name:       record.Name,
		ConfigJSON: record.ConfigJSON,
		Enabled:    record.Enabled,
		CreatedAt:  record.CreatedAt,
		UpdatedAt:  record.UpdatedAt,
	}
}

func fromRow(data row) storage.DriverRecord {
	return storage.DriverRecord{
		TenantID:   data.TenantID,
		ID:         data.ID,
		Name:       data.Name,
		ConfigJSON: data.ConfigJSON,
		Enabled:    data.Enabled,
		CreatedAt:  data.CreatedAt,
		UpdatedAt:  data.UpdatedAt,
	}
}
