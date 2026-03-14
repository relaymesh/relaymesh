package oauth

import (
	"log"
	"net/http"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/providerinstance"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

// Handler handles OAuth callbacks and persists installation data.
type Handler struct {
	Provider              string
	Config                auth.ProviderConfig
	Providers             auth.Config
	Store                 storage.Store
	NamespaceStore        storage.NamespaceStore
	ProviderInstanceStore storage.ProviderInstanceStore
	ProviderInstanceCache *providerinstance.Cache
	Logger                *log.Logger
	RedirectBase          string
	Endpoint              string
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := h.Logger
	if logger == nil {
		logger = log.Default()
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	provider := h.Provider
	config := h.Config
	if provider == "" {
		provider = providerFromPath(r.URL.Path)
		switch provider {
		case "github":
			config = h.Providers.GitHub
		case "gitlab":
			config = h.Providers.GitLab
		case "bitbucket":
			config = h.Providers.Bitbucket
		}
	}

	switch provider {
	case "gitlab":
		h.handleGitLab(w, r, logger, config)
	case "bitbucket":
		h.handleBitbucket(w, r, logger, config)
	case "github":
		h.handleGitHubApp(w, r, logger, config)
	default:
		http.Error(w, "unsupported provider", http.StatusBadRequest)
	}
}
