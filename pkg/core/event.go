package core

// Event represents a webhook event from a Git provider.
type Event struct {
	// Provider is the name of the Git provider (e.g., "github", "gitlab").
	Provider string `json:"provider"`
	// ProviderType is the normalized provider category (e.g. "scm").
	ProviderType string `json:"provider_type,omitempty"`
	// Name is the name of the event (e.g., "pull_request", "push").
	Name string `json:"name"`
	// EventType is the normalized event type. For current SCM providers this mirrors Name.
	EventType string `json:"event_type,omitempty"`
	// Action is the normalized action from the payload when available (e.g. opened, closed).
	Action string `json:"action,omitempty"`
	// ResourceType is the normalized resource/entity type for the event.
	ResourceType string `json:"resource_type,omitempty"`
	// ResourceID is the normalized stable id of the affected resource.
	ResourceID string `json:"resource_id,omitempty"`
	// ResourceName is a human-friendly resource identifier (e.g. owner/repo).
	ResourceName string `json:"resource_name,omitempty"`
	// ActorID identifies the principal that triggered the event.
	ActorID string `json:"actor_id,omitempty"`
	// ActorName is the human-friendly principal name/login.
	ActorName string `json:"actor_name,omitempty"`
	// OccurredAt is the source event timestamp when available.
	OccurredAt string `json:"occurred_at,omitempty"`
	// RequestID links the event back to the inbound webhook request.
	RequestID string `json:"request_id,omitempty"`
	// LogID links the event to a stored event log entry.
	LogID string `json:"-"`
	// Headers contains the inbound webhook request headers.
	Headers map[string][]string `json:"-"`
	// Data is the flattened JSON payload of the event.
	Data map[string]interface{} `json:"data"`
	// RawPayload is the raw JSON payload from the webhook.
	RawPayload []byte `json:"-"`
	// RawObject is the unmarshalled JSON payload.
	RawObject interface{} `json:"-"`
	// StateID maps the event to an installation/account id for token lookup.
	StateID  string `json:"-"`
	TenantID string `json:"-"`
	// InstallationID maps the event to a provider installation for token lookup.
	InstallationID string `json:"-"`
	// ProviderInstanceKey identifies the provider instance configuration.
	ProviderInstanceKey string `json:"-"`
	// NamespaceID identifies the repository/namespace for the event.
	NamespaceID string `json:"-"`
	// NamespaceName is the human-readable namespace (owner/repo).
	NamespaceName string `json:"-"`
}
