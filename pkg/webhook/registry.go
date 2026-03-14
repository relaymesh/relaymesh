package webhook

import (
	"errors"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/drivers"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

// HandlerOptions holds dependencies used to build webhook handlers.
type HandlerOptions struct {
	Rules              *core.RuleEngine
	Publisher          core.Publisher
	Logger             *log.Logger
	MaxBodyBytes       int64
	DebugEvents        bool
	InstallStore       storage.Store
	NamespaceStore     storage.NamespaceStore
	EventLogStore      storage.EventLogStore
	RuleStore          storage.RuleStore
	DriverStore        storage.DriverStore
	RulesStrict        bool
	DynamicDriverCache *drivers.DynamicPublisherCache
}

// Provider is a plugin interface for webhook integrations.
type Provider interface {
	Name() string
	WebhookPath(cfg auth.ProviderConfig) string
	NewHandler(cfg auth.ProviderConfig, opts HandlerOptions) (http.Handler, error)
	WebhookLogFields(cfg auth.ProviderConfig) string
}

// Registry holds webhook providers.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry creates an empty webhook provider registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds a webhook provider to the registry.
func (r *Registry) Register(provider Provider) error {
	if r == nil {
		return errors.New("registry is nil")
	}
	if provider == nil {
		return errors.New("provider is nil")
	}
	name := strings.ToLower(strings.TrimSpace(provider.Name()))
	if name == "" {
		return errors.New("provider name is required")
	}
	if _, exists := r.providers[name]; exists {
		return errors.New("provider already registered")
	}
	r.providers[name] = provider
	return nil
}

// Provider returns a webhook provider by name.
func (r *Registry) Provider(name string) (Provider, bool) {
	if r == nil {
		return nil, false
	}
	provider, ok := r.providers[strings.ToLower(strings.TrimSpace(name))]
	return provider, ok
}

// Providers returns all registered providers in name order.
func (r *Registry) Providers() []Provider {
	if r == nil {
		return nil
	}
	out := make([]Provider, 0, len(r.providers))
	for _, provider := range r.providers {
		out = append(out, provider)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name() < out[j].Name()
	})
	return out
}
