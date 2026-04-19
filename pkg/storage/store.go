package storage

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

// InstallRecord stores SCM installation or token metadata.
type InstallRecord struct {
	ID                  string
	TenantID            string
	Provider            string
	AccountID           string
	AccountName         string
	InstallationID      string
	ProviderInstanceKey string
	EnterpriseID        string
	EnterpriseSlug      string
	EnterpriseName      string
	AccessToken         string
	RefreshToken        string
	ExpiresAt           *time.Time
	MetadataJSON        string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// NamespaceRecord stores provider repository metadata.
type NamespaceRecord struct {
	ID                  string
	TenantID            string
	Provider            string
	AccountID           string
	InstallationID      string
	ProviderInstanceKey string
	RepoID              string
	Owner               string
	RepoName            string
	FullName            string
	Visibility          string
	DefaultBranch       string
	HTTPURL             string
	SSHURL              string
	WebhooksEnabled     bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// RuleRecord stores rule metadata.
type RuleRecord struct {
	TenantID         string
	ID               string
	When             string
	Emit             []string
	DriverID         string
	TransformJS      string
	DriverName       string
	DriverConfigJSON string
	DriverEnabled    bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// DriverRecord stores Relaybus driver config (per-tenant).
type DriverRecord struct {
	TenantID   string
	ID         string
	Name       string
	ConfigJSON string
	Enabled    bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// EventLogRecord stores metadata about webhook events and rule matches.
type EventLogRecord struct {
	TenantID        string
	ID              string
	Provider        string
	Name            string
	RequestID       string
	StateID         string
	InstallationID  string
	NamespaceID     string
	NamespaceName   string
	Topic           string
	RuleID          string
	RuleWhen        string
	Drivers         []string
	Headers         map[string][]string
	Body            []byte
	TransformedBody []byte
	BodyHash        string
	Matched         bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Status          string
	ErrorMessage    string
	LatencyMS       int64
}

// EventLogFilter selects event log rows.
type EventLogFilter struct {
	TenantID        string
	Provider        string
	Name            string
	RequestID       string
	StateID         string
	InstallationID  string
	NamespaceID     string
	NamespaceName   string
	Topic           string
	RuleID          string
	RuleWhen        string
	Matched         *bool
	StartTime       time.Time
	EndTime         time.Time
	Limit           int
	Offset          int
	CursorCreatedAt time.Time
	CursorID        string
}

// EventLogCount represents an aggregate count bucket.
type EventLogCount struct {
	Key   string
	Count int64
}

// EventLogAnalytics contains aggregate data for event logs.
type EventLogAnalytics struct {
	Total       int64
	Matched     int64
	DistinctReq int64
	ByProvider  []EventLogCount
	ByEvent     []EventLogCount
	ByTopic     []EventLogCount
	ByRule      []EventLogCount
	ByInstall   []EventLogCount
	ByNamespace []EventLogCount
}

// EventLogInterval defines the time-series bucket granularity.
type EventLogInterval string

const (
	EventLogIntervalHour EventLogInterval = "hour"
	EventLogIntervalDay  EventLogInterval = "day"
	EventLogIntervalWeek EventLogInterval = "week"
)

// EventLogTimeseriesBucket represents a time-series aggregate bucket.
type EventLogTimeseriesBucket struct {
	Start        time.Time
	End          time.Time
	EventCount   int64
	MatchedCount int64
	DistinctReq  int64
	FailureCount int64
}

// EventLogBreakdownGroup defines supported breakdown dimensions.
type EventLogBreakdownGroup string

const (
	EventLogBreakdownProvider      EventLogBreakdownGroup = "provider"
	EventLogBreakdownEvent         EventLogBreakdownGroup = "event"
	EventLogBreakdownRuleID        EventLogBreakdownGroup = "rule_id"
	EventLogBreakdownRuleWhen      EventLogBreakdownGroup = "rule_when"
	EventLogBreakdownTopic         EventLogBreakdownGroup = "topic"
	EventLogBreakdownNamespaceID   EventLogBreakdownGroup = "namespace_id"
	EventLogBreakdownNamespaceName EventLogBreakdownGroup = "namespace_name"
	EventLogBreakdownInstallation  EventLogBreakdownGroup = "installation_id"
)

// EventLogBreakdownSort defines supported sort keys.
type EventLogBreakdownSort string

const (
	EventLogBreakdownSortCount      EventLogBreakdownSort = "count"
	EventLogBreakdownSortMatched    EventLogBreakdownSort = "matched"
	EventLogBreakdownSortFailed     EventLogBreakdownSort = "failed"
	EventLogBreakdownSortLatencyP95 EventLogBreakdownSort = "latency_p95"
)

// EventLogBreakdown represents aggregated counts for a breakdown dimension.
type EventLogBreakdown struct {
	Key          string
	EventCount   int64
	MatchedCount int64
	FailureCount int64
	LatencyP50MS float64
	LatencyP95MS float64
	LatencyP99MS float64
}

// ProviderInstanceRecord stores per-tenant provider instance config.
type ProviderInstanceRecord struct {
	ID              string
	TenantID        string
	Provider        string
	Key             string
	ConfigJSON      string
	RedirectBaseURL string
	Enabled         bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NamespaceRecordID returns a deterministic identifier for a namespace.
func NamespaceRecordID(record NamespaceRecord) string {
	return hashID(
		record.TenantID,
		record.Provider,
		record.ProviderInstanceKey,
		record.RepoID,
	)
}

// InstallRecordID returns a deterministic identifier for an installation.
func InstallRecordID(record InstallRecord) string {
	return hashID(
		record.TenantID,
		record.Provider,
		record.AccountID,
		record.InstallationID,
	)
}

// ProviderInstanceRecordID returns a deterministic identifier for a provider instance.
func ProviderInstanceRecordID(record ProviderInstanceRecord) string {
	return hashID(
		record.TenantID,
		record.Provider,
		record.Key,
	)
}

func hashID(parts ...string) string {
	buf := sha256.New()
	for _, part := range parts {
		buf.Write([]byte(strings.TrimSpace(part)))
		buf.Write([]byte{':'})
	}
	return fmt.Sprintf("%x", buf.Sum(nil))
}

// NamespaceFilter selects namespace rows.
type NamespaceFilter struct {
	TenantID            string
	Provider            string
	AccountID           string
	InstallationID      string
	ProviderInstanceKey string
	RepoID              string
	Owner               string
	RepoName            string
	FullName            string
	Limit               int
}

// Store defines the persistence interface for installation records.
type Store interface {
	UpsertInstallation(ctx context.Context, record InstallRecord) error
	GetInstallation(ctx context.Context, provider, accountID, installationID string) (*InstallRecord, error)
	GetInstallationByInstallationID(ctx context.Context, provider, installationID string) (*InstallRecord, error)
	GetInstallationByInstallationIDAndInstanceKey(ctx context.Context, provider, installationID, instanceKey string) (*InstallRecord, error)
	// ListInstallations lists installations for a provider, optionally filtered by accountID.
	ListInstallations(ctx context.Context, provider, accountID string) ([]InstallRecord, error)
	DeleteInstallation(ctx context.Context, provider, accountID, installationID, instanceKey string) error
	Close() error
}

// NamespaceStore defines persistence for provider repository metadata.
type NamespaceStore interface {
	UpsertNamespace(ctx context.Context, record NamespaceRecord) error
	GetNamespace(ctx context.Context, provider, repoID, instanceKey string) (*NamespaceRecord, error)
	ListNamespaces(ctx context.Context, filter NamespaceFilter) ([]NamespaceRecord, error)
	DeleteNamespace(ctx context.Context, provider, repoID, instanceKey string) error
	Close() error
}

// RuleStore defines persistence for rules.
type RuleStore interface {
	ListRules(ctx context.Context) ([]RuleRecord, error)
	GetRule(ctx context.Context, id string) (*RuleRecord, error)
	CreateRule(ctx context.Context, record RuleRecord) (*RuleRecord, error)
	UpdateRule(ctx context.Context, record RuleRecord) (*RuleRecord, error)
	DeleteRule(ctx context.Context, id string) error
	Close() error
}

// ProviderInstanceStore defines persistence for provider instance configs.
type ProviderInstanceStore interface {
	ListProviderInstances(ctx context.Context, provider string) ([]ProviderInstanceRecord, error)
	GetProviderInstance(ctx context.Context, provider, key string) (*ProviderInstanceRecord, error)
	UpsertProviderInstance(ctx context.Context, record ProviderInstanceRecord) (*ProviderInstanceRecord, error)
	DeleteProviderInstance(ctx context.Context, provider, key string) error
	Close() error
}

// DriverStore defines persistence for driver configs.
type DriverStore interface {
	ListDrivers(ctx context.Context) ([]DriverRecord, error)
	GetDriver(ctx context.Context, name string) (*DriverRecord, error)
	GetDriverByID(ctx context.Context, id string) (*DriverRecord, error)
	UpsertDriver(ctx context.Context, record DriverRecord) (*DriverRecord, error)
	DeleteDriver(ctx context.Context, name string) error
	Close() error
}

// EventLogStore defines persistence for webhook event logs.
type EventLogStore interface {
	CreateEventLogs(ctx context.Context, records []EventLogRecord) error
	ListEventLogs(ctx context.Context, filter EventLogFilter) ([]EventLogRecord, error)
	GetEventLog(ctx context.Context, id string) (*EventLogRecord, error)
	GetEventLogAnalytics(ctx context.Context, filter EventLogFilter) (EventLogAnalytics, error)
	GetEventLogTimeseries(ctx context.Context, filter EventLogFilter, interval EventLogInterval) ([]EventLogTimeseriesBucket, error)
	GetEventLogBreakdown(ctx context.Context, filter EventLogFilter, groupBy EventLogBreakdownGroup, sortBy EventLogBreakdownSort, sortDesc bool, pageSize int, pageToken string, includeLatency bool) ([]EventLogBreakdown, string, error)
	UpdateEventLogTransformedPayload(ctx context.Context, id string, transformedBody []byte) error
	UpdateEventLogStatus(ctx context.Context, id, status, errorMessage string) error
	Close() error
}
