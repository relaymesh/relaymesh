package oauth

import (
	"errors"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/providerinstance"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

// HandlerOptions holds dependencies used to build OAuth handlers.
type HandlerOptions struct {
	Providers             auth.Config
	Store                 storage.Store
	NamespaceStore        storage.NamespaceStore
	ProviderInstanceStore storage.ProviderInstanceStore
	ProviderInstanceCache *providerinstance.Cache
	Logger                *log.Logger
	RedirectBase          string
	Endpoint              string
}

// Provider is a plugin interface for OAuth integrations.
type Provider interface {
	Name() string
	CallbackPath() string
	AuthorizeURL(r *http.Request, cfg auth.ProviderConfig, state, endpoint string) (string, error)
	NewHandler(cfg auth.ProviderConfig, opts HandlerOptions) http.Handler
}

// Registry holds OAuth providers.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry creates an empty OAuth provider registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds an OAuth provider to the registry.
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

// Provider returns an OAuth provider by name.
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
