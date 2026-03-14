package installations

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/relaymesh/relaymesh/pkg/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Config mirrors the storage configuration for the installations table.
type Config struct {
	Driver      string
	DSN         string
	Dialect     string
	Table       string
	AutoMigrate bool
	Pool        storage.PoolConfig
}

// Store implements storage.Store on top of GORM.
type Store struct {
	db    *gorm.DB
	table string
}

type row struct {
	ID             string     `gorm:"column:id;size:64;primaryKey"`
	TenantID       string     `gorm:"column:tenant_id;size:64;not null;default:'';uniqueIndex:idx_installation,priority:1;index:idx_installation_provider_installation_updated,priority:1;index:idx_installation_provider_installation_instance_updated,priority:1;index:idx_installation_tenant_provider,priority:1"`
	Provider       string     `gorm:"column:provider;size:32;not null;uniqueIndex:idx_installation,priority:2;index:idx_installation_provider_installation_updated,priority:2;index:idx_installation_provider_installation_instance_updated,priority:2;index:idx_installation_tenant_provider,priority:2"`
	AccountID      string     `gorm:"column:account_id;size:128;not null;uniqueIndex:idx_installation,priority:3"`
	AccountName    string     `gorm:"column:account_name;size:255"`
	InstallationID string     `gorm:"column:installation_id;size:128;not null;uniqueIndex:idx_installation,priority:4;index:idx_installation_provider_installation_updated,priority:3;index:idx_installation_provider_installation_instance_updated,priority:3"`
	InstanceKey    string     `gorm:"column:provider_instance_key;size:64;uniqueIndex:idx_installation,priority:5;index:idx_installation_provider_installation_instance_updated,priority:4"`
	EnterpriseID   string     `gorm:"column:enterprise_id;size:128"`
	EnterpriseSlug string     `gorm:"column:enterprise_slug;size:255"`
	EnterpriseName string     `gorm:"column:enterprise_name;size:255"`
	AccessToken    string     `gorm:"column:access_token"`
	RefreshToken   string     `gorm:"column:refresh_token"`
	ExpiresAt      *time.Time `gorm:"column:expires_at"`
	MetadataJSON   string     `gorm:"column:metadata_json;type:text"`
	CreatedAt      time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;autoUpdateTime;index:idx_installation_provider_installation_updated,priority:4,sort:desc;index:idx_installation_provider_installation_instance_updated,priority:5,sort:desc"`
}

// Open creates a GORM-backed installations store.
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
		table = "githook_installations"
	}
	store := &Store{
		db:    gormDB,
		table: table,
	}
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

// UpsertInstallation inserts or updates an installation record.
func (s *Store) UpsertInstallation(ctx context.Context, record storage.InstallRecord) error {
	if s == nil || s.db == nil {
		return errors.New("store is not initialized")
	}
	if record.Provider == "" {
		return errors.New("provider is required")
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
		record.ID = storage.InstallRecordID(record)
	}

	data := toRow(record)
	return s.tableDB().
		WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "provider"}, {Name: "account_id"}, {Name: "installation_id"}, {Name: "provider_instance_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"account_name", "provider_instance_key", "enterprise_id", "enterprise_slug", "enterprise_name", "access_token", "refresh_token", "expires_at", "metadata_json", "updated_at"}),
		}).
		Create(&data).Error
}

// GetInstallation fetches a single installation record.
func (s *Store) GetInstallation(ctx context.Context, provider, accountID, installationID string) (*storage.InstallRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	var data row
	query := s.tableDB().
		WithContext(ctx).
		Where("provider = ? AND account_id = ? AND installation_id = ?", provider, accountID, installationID)
	query = query.Scopes(storage.TenantScope(ctx, "", "tenant_id"))
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

// GetInstallationByInstallationID fetches the latest installation record for a provider.
func (s *Store) GetInstallationByInstallationID(ctx context.Context, provider, installationID string) (*storage.InstallRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	var data row
	query := s.tableDB().
		WithContext(ctx).
		Where("provider = ? AND installation_id = ?", provider, installationID)
	query = query.Scopes(storage.TenantScope(ctx, "", "tenant_id"))
	err := query.Order("updated_at desc").Take(&data).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	record := fromRow(data)
	return &record, nil
}

// GetInstallationByInstallationIDAndInstanceKey fetches the latest installation record for a provider instance.
func (s *Store) GetInstallationByInstallationIDAndInstanceKey(ctx context.Context, provider, installationID, instanceKey string) (*storage.InstallRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	provider = strings.TrimSpace(provider)
	installationID = strings.TrimSpace(installationID)
	instanceKey = strings.TrimSpace(instanceKey)
	if provider == "" || installationID == "" || instanceKey == "" {
		return nil, errors.New("provider, installation_id, and provider_instance_key are required")
	}
	var data row
	query := s.tableDB().
		WithContext(ctx).
		Where("provider = ? AND installation_id = ? AND provider_instance_key = ?", provider, installationID, instanceKey)
	query = query.Scopes(storage.TenantScope(ctx, "", "tenant_id"))
	err := query.Order("updated_at desc").Take(&data).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	record := fromRow(data)
	return &record, nil
}

// ListInstallations lists installations for a provider, optionally filtered by account.
func (s *Store) ListInstallations(ctx context.Context, provider, accountID string) ([]storage.InstallRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	var data []row
	query := s.tableDB().
		WithContext(ctx).
		Where("provider = ?", provider)
	if accountID != "" {
		query = query.Where("account_id = ?", accountID)
	}
	query = query.Scopes(storage.TenantScope(ctx, "", "tenant_id"))
	err := query.Find(&data).Error
	if err != nil {
		return nil, err
	}
	records := make([]storage.InstallRecord, 0, len(data))
	for _, item := range data {
		records = append(records, fromRow(item))
	}
	return records, nil
}

// DeleteInstallation removes an installation record.
func (s *Store) DeleteInstallation(ctx context.Context, provider, accountID, installationID, instanceKey string) error {
	if s == nil || s.db == nil {
		return errors.New("store is not initialized")
	}
	provider = strings.TrimSpace(provider)
	accountID = strings.TrimSpace(accountID)
	installationID = strings.TrimSpace(installationID)
	instanceKey = strings.TrimSpace(instanceKey)
	if provider == "" || accountID == "" || installationID == "" {
		return errors.New("provider, account_id, and installation_id are required")
	}
	query := s.tableDB().
		WithContext(ctx).
		Where("provider = ? AND account_id = ? AND installation_id = ?", provider, accountID, installationID)
	if instanceKey != "" {
		query = query.Where("provider_instance_key = ?", instanceKey)
	}
	query = query.Scopes(storage.TenantScope(ctx, "", "tenant_id"))
	return query.Delete(&row{}).Error
}

// UpdateProviderInstanceKey updates the provider instance key for a provider and tenant.
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
		Where("provider = ? AND provider_instance_key = ?", provider, oldKey).
		Where("tenant_id = ?", tenantID)
	result := query.Updates(map[string]interface{}{
		"provider_instance_key": newKey,
		"updated_at":            time.Now().UTC(),
	})
	return result.RowsAffected, result.Error
}

func (s *Store) migrate() error {
	return s.tableDB().AutoMigrate(&row{})
}

func (s *Store) tableDB() *gorm.DB {
	return s.db.Session(&gorm.Session{NewDB: true}).Table(s.table)
}

func toRow(record storage.InstallRecord) row {
	return row{
		ID:             record.ID,
		TenantID:       record.TenantID,
		Provider:       record.Provider,
		AccountID:      record.AccountID,
		AccountName:    record.AccountName,
		InstallationID: record.InstallationID,
		InstanceKey:    record.ProviderInstanceKey,
		EnterpriseID:   record.EnterpriseID,
		EnterpriseSlug: record.EnterpriseSlug,
		EnterpriseName: record.EnterpriseName,
		AccessToken:    record.AccessToken,
		RefreshToken:   record.RefreshToken,
		ExpiresAt:      record.ExpiresAt,
		MetadataJSON:   record.MetadataJSON,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}
}

func fromRow(data row) storage.InstallRecord {
	return storage.InstallRecord{
		ID:                  data.ID,
		TenantID:            data.TenantID,
		Provider:            data.Provider,
		AccountID:           data.AccountID,
		AccountName:         data.AccountName,
		InstallationID:      data.InstallationID,
		ProviderInstanceKey: data.InstanceKey,
		EnterpriseID:        data.EnterpriseID,
		EnterpriseSlug:      data.EnterpriseSlug,
		EnterpriseName:      data.EnterpriseName,
		AccessToken:         data.AccessToken,
		RefreshToken:        data.RefreshToken,
		ExpiresAt:           data.ExpiresAt,
		MetadataJSON:        data.MetadataJSON,
		CreatedAt:           data.CreatedAt,
		UpdatedAt:           data.UpdatedAt,
	}
}
