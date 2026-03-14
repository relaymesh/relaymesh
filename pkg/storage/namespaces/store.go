package namespaces

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/relaymesh/relaymesh/pkg/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Config mirrors the storage configuration for the namespaces table.
type Config struct {
	Driver      string
	DSN         string
	Dialect     string
	Table       string
	AutoMigrate bool
	Pool        storage.PoolConfig
}

// Store implements storage.NamespaceStore on top of GORM.
type Store struct {
	db    *gorm.DB
	table string
}

type row struct {
	ID              string    `gorm:"column:id;size:64;primaryKey"`
	TenantID        string    `gorm:"column:tenant_id;size:64;not null;default:'';uniqueIndex:idx_namespace,priority:1;index:idx_ns_tenant_provider,priority:1;index:idx_ns_tenant_instance_key,priority:1;index:idx_ns_tenant_account,priority:1;index:idx_ns_tenant_installation,priority:1;index:idx_ns_tenant_owner,priority:1;index:idx_ns_tenant_repo_name,priority:1;index:idx_ns_tenant_full_name,priority:1"`
	Provider        string    `gorm:"column:provider;size:32;not null;uniqueIndex:idx_namespace,priority:2;index:idx_ns_tenant_provider,priority:2;index:idx_ns_provider_only,priority:1"`
	InstanceKey     string    `gorm:"column:provider_instance_key;size:64;uniqueIndex:idx_namespace,priority:3;index:idx_ns_tenant_instance_key,priority:2"`
	RepoID          string    `gorm:"column:repo_id;size:128;not null;uniqueIndex:idx_namespace,priority:4"`
	AccountID       string    `gorm:"column:account_id;size:128;not null;index:idx_ns_tenant_account,priority:2"`
	InstallationID  string    `gorm:"column:installation_id;size:128;index:idx_ns_tenant_installation,priority:2"`
	Owner           string    `gorm:"column:owner;size:255;index:idx_ns_tenant_owner,priority:2"`
	RepoName        string    `gorm:"column:repo_name;size:255;index:idx_ns_tenant_repo_name,priority:2"`
	FullName        string    `gorm:"column:full_name;size:255;index:idx_ns_tenant_full_name,priority:2"`
	Visibility      string    `gorm:"column:visibility;size:32"`
	DefaultBranch   string    `gorm:"column:default_branch;size:255"`
	HTTPURL         string    `gorm:"column:http_url;size:512"`
	SSHURL          string    `gorm:"column:ssh_url;size:512"`
	WebhooksEnabled bool      `gorm:"column:webhooks_enabled"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// Open creates a GORM-backed namespaces store.
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
		table = "githook_namespaces"
		migrator := gormDB.Migrator()
		if migrator.HasTable("git_namespaces") && !migrator.HasTable(table) {
			table = "git_namespaces"
		}
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

// TableName returns the resolved table name.
func (s *Store) TableName() string {
	if s == nil {
		return ""
	}
	return s.table
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

// UpsertNamespace inserts or updates a namespace record.
func (s *Store) UpsertNamespace(ctx context.Context, record storage.NamespaceRecord) error {
	if s == nil || s.db == nil {
		return errors.New("store is not initialized")
	}
	if record.Provider == "" || record.RepoID == "" {
		return errors.New("provider and repo_id are required")
	}
	if record.TenantID == "" {
		record.TenantID = storage.TenantFromContext(ctx)
	}
	if record.ID == "" {
		record.ID = storage.NamespaceRecordID(record)
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now

	data := toRow(record)
	return s.tableDB().
		WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "provider"}, {Name: "provider_instance_key"}, {Name: "repo_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"account_id", "installation_id", "provider_instance_key", "owner", "repo_name", "full_name", "visibility", "default_branch", "http_url", "ssh_url", "webhooks_enabled", "updated_at"}),
		}).
		Create(&data).Error
}

// GetNamespace fetches a namespace by provider/repo ID.
func (s *Store) GetNamespace(ctx context.Context, provider, repoID, instanceKey string) (*storage.NamespaceRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	var data row
	query := s.tableDB().
		WithContext(ctx).
		Where("provider = ? AND repo_id = ?", provider, repoID)
	if instanceKey != "" {
		query = query.Where("provider_instance_key = ?", instanceKey)
	}
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

// ListNamespaces lists namespaces by filter.
func (s *Store) ListNamespaces(ctx context.Context, filter storage.NamespaceFilter) ([]storage.NamespaceRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	query := s.tableDB().WithContext(ctx)
	query = query.Scopes(storage.TenantScope(ctx, filter.TenantID, "tenant_id"))
	if filter.Provider != "" {
		query = query.Where("provider = ?", filter.Provider)
	}
	if filter.ProviderInstanceKey != "" {
		query = query.Where("provider_instance_key = ?", filter.ProviderInstanceKey)
	}
	if filter.AccountID != "" {
		query = query.Where("account_id = ?", filter.AccountID)
	}
	if filter.InstallationID != "" {
		query = query.Where("installation_id = ?", filter.InstallationID)
	}
	if filter.RepoID != "" {
		query = query.Where("repo_id = ?", filter.RepoID)
	}
	if filter.Owner != "" {
		query = query.Where("owner = ?", filter.Owner)
	}
	if filter.RepoName != "" {
		query = query.Where("repo_name = ?", filter.RepoName)
	}
	if filter.FullName != "" {
		query = query.Where("full_name = ?", filter.FullName)
	}
	var data []row
	err := query.Find(&data).Error
	if err != nil {
		return nil, err
	}
	records := make([]storage.NamespaceRecord, 0, len(data))
	for _, item := range data {
		records = append(records, fromRow(item))
	}
	return records, nil
}

// DeleteNamespace removes a namespace record by provider and repo ID.
func (s *Store) DeleteNamespace(ctx context.Context, provider, repoID, instanceKey string) error {
	if s == nil || s.db == nil {
		return errors.New("store is not initialized")
	}
	provider = strings.TrimSpace(provider)
	repoID = strings.TrimSpace(repoID)
	instanceKey = strings.TrimSpace(instanceKey)
	if provider == "" || repoID == "" {
		return errors.New("provider and repo_id are required")
	}
	query := s.tableDB().
		WithContext(ctx).
		Where("provider = ? AND repo_id = ?", provider, repoID)
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

func toRow(record storage.NamespaceRecord) row {
	return row{
		ID:              record.ID,
		TenantID:        record.TenantID,
		Provider:        record.Provider,
		RepoID:          record.RepoID,
		AccountID:       record.AccountID,
		InstallationID:  record.InstallationID,
		InstanceKey:     record.ProviderInstanceKey,
		Owner:           record.Owner,
		RepoName:        record.RepoName,
		FullName:        record.FullName,
		Visibility:      record.Visibility,
		DefaultBranch:   record.DefaultBranch,
		HTTPURL:         record.HTTPURL,
		SSHURL:          record.SSHURL,
		WebhooksEnabled: record.WebhooksEnabled,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
	}
}

func fromRow(data row) storage.NamespaceRecord {
	return storage.NamespaceRecord{
		ID:                  data.ID,
		TenantID:            data.TenantID,
		Provider:            data.Provider,
		RepoID:              data.RepoID,
		AccountID:           data.AccountID,
		InstallationID:      data.InstallationID,
		ProviderInstanceKey: data.InstanceKey,
		Owner:               data.Owner,
		RepoName:            data.RepoName,
		FullName:            data.FullName,
		Visibility:          data.Visibility,
		DefaultBranch:       data.DefaultBranch,
		HTTPURL:             data.HTTPURL,
		SSHURL:              data.SSHURL,
		WebhooksEnabled:     data.WebhooksEnabled,
		CreatedAt:           data.CreatedAt,
		UpdatedAt:           data.UpdatedAt,
	}
}
