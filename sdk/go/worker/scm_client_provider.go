package worker

import (
	"container/list"
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

const (
	defaultSCMCacheSize = 10
	defaultSCMSkew      = 30 * time.Second
)

// RemoteSCMClientProvider resolves SCM clients via the server API.
type RemoteSCMClientProvider struct {
	endpoint   string
	apiKey     string
	oauth2     *auth.OAuth2Config
	httpClient *http.Client
	cache      *scmClientCache
	skew       time.Duration
}

// RemoteSCMClientProviderOption configures a RemoteSCMClientProvider.
type RemoteSCMClientProviderOption func(*RemoteSCMClientProvider)

// NewRemoteSCMClientProvider creates a provider that fetches credentials from the server.
func NewRemoteSCMClientProvider(opts ...RemoteSCMClientProviderOption) *RemoteSCMClientProvider {
	provider := &RemoteSCMClientProvider{
		cache: newSCMClientCache(defaultSCMCacheSize),
		skew:  defaultSCMSkew,
	}
	for _, opt := range opts {
		opt(provider)
	}
	return provider
}

// WithSCMEndpoint overrides the API endpoint used for SCM client resolution.
func WithSCMEndpoint(endpoint string) RemoteSCMClientProviderOption {
	return func(p *RemoteSCMClientProvider) {
		p.endpoint = strings.TrimSpace(endpoint)
	}
}

// WithSCMAPIKey sets the API key used for SCM client resolution.
func WithSCMAPIKey(apiKey string) RemoteSCMClientProviderOption {
	return func(p *RemoteSCMClientProvider) {
		p.apiKey = strings.TrimSpace(apiKey)
	}
}

// WithSCMOAuth2Config sets OAuth2 credentials used for SCM client resolution.
func WithSCMOAuth2Config(cfg *auth.OAuth2Config) RemoteSCMClientProviderOption {
	return func(p *RemoteSCMClientProvider) {
		p.oauth2 = cfg
	}
}

// WithSCMHTTPClient overrides the HTTP client used for SCM client resolution.
func WithSCMHTTPClient(client *http.Client) RemoteSCMClientProviderOption {
	return func(p *RemoteSCMClientProvider) {
		p.httpClient = client
	}
}

// WithSCMCacheSize sets the LRU cache size (default 10).
func WithSCMCacheSize(size int) RemoteSCMClientProviderOption {
	return func(p *RemoteSCMClientProvider) {
		if size < 1 {
			size = 1
		}
		p.cache = newSCMClientCache(size)
	}
}

// WithSCMCacheSkew sets how early to refresh tokens before expiry.
func WithSCMCacheSkew(skew time.Duration) RemoteSCMClientProviderOption {
	return func(p *RemoteSCMClientProvider) {
		if skew < 0 {
			skew = 0
		}
		p.skew = skew
	}
}

// BindAPIClient configures the provider with the worker's resolved API settings.
func (p *RemoteSCMClientProvider) BindAPIClient(cfg apiClientConfig) {
	if cfg.BaseURL != "" {
		p.endpoint = cfg.BaseURL
	}
	if cfg.APIKey != "" {
		p.apiKey = cfg.APIKey
	}
	if cfg.OAuth2 != nil {
		p.oauth2 = cfg.OAuth2
	}
	if cfg.HTTPClient != nil {
		p.httpClient = cfg.HTTPClient
	}
}

// Client returns a cached SCM client or fetches new credentials from the server.
func (p *RemoteSCMClientProvider) Client(ctx context.Context, evt *Event) (interface{}, error) {
	if evt == nil {
		return nil, errors.New("event is required")
	}
	provider := strings.TrimSpace(evt.Provider)
	if provider == "" {
		return nil, errors.New("provider is required")
	}
	installationID := strings.TrimSpace(evt.Metadata[MetadataKeyInstallationID])
	if installationID == "" {
		return nil, errors.New("installation_id missing from metadata")
	}
	instanceKey := strings.TrimSpace(evt.Metadata[MetadataKeyProviderInstanceKey])

	cacheKey := strings.Join([]string{provider, installationID, instanceKey}, "|")
	if cached := p.cache.Get(cacheKey, p.skew); cached != nil {
		return cached, nil
	}

	client, err := p.fetchClient(ctx, provider, installationID, instanceKey)
	if err != nil {
		return nil, err
	}
	if client.client != nil {
		p.cache.Add(cacheKey, client.client, client.expiresAt)
	}
	return client.client, nil
}

type scmClientResult struct {
	client    interface{}
	expiresAt time.Time
}

func (p *RemoteSCMClientProvider) fetchClient(
	ctx context.Context,
	provider string,
	installationID string,
	instanceKey string,
) (*scmClientResult, error) {
	endpoint := resolveEndpoint(p.endpoint)
	client := &SCMClientsClient{
		BaseURL:    endpoint,
		HTTPClient: p.httpClient,
		APIKey:     p.apiKey,
		OAuth2:     p.oauth2,
	}
	record, err := client.GetSCMClient(ctx, provider, installationID, instanceKey)
	if err != nil {
		return nil, err
	}
	if record.AccessToken == "" {
		return nil, errors.New("scm access token missing")
	}

	created, err := newProviderClient(record.Provider, record.AccessToken, record.APIBaseURL)
	if err != nil {
		return nil, err
	}
	return &scmClientResult{client: created, expiresAt: record.ExpiresAt}, nil
}

type scmClientCache struct {
	mu    sync.Mutex
	size  int
	order *list.List
	items map[string]*list.Element
}

type scmCacheEntry struct {
	key       string
	client    interface{}
	expiresAt time.Time
}

func newSCMClientCache(size int) *scmClientCache {
	if size < 1 {
		size = 1
	}
	return &scmClientCache{
		size:  size,
		order: list.New(),
		items: make(map[string]*list.Element, size),
	}
}

func (c *scmClientCache) Get(key string, skew time.Duration) interface{} {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	elem := c.items[key]
	if elem == nil {
		return nil
	}
	entry := elem.Value.(*scmCacheEntry)
	if expired(entry.expiresAt, skew) {
		c.order.Remove(elem)
		delete(c.items, key)
		return nil
	}
	c.order.MoveToFront(elem)
	return entry.client
}

func (c *scmClientCache) Add(key string, client interface{}, expiresAt time.Time) {
	if c == nil || client == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem := c.items[key]; elem != nil {
		entry := elem.Value.(*scmCacheEntry)
		entry.client = client
		entry.expiresAt = expiresAt
		c.order.MoveToFront(elem)
		return
	}
	elem := c.order.PushFront(&scmCacheEntry{key: key, client: client, expiresAt: expiresAt})
	c.items[key] = elem
	for c.order.Len() > c.size {
		last := c.order.Back()
		if last == nil {
			break
		}
		entry := last.Value.(*scmCacheEntry)
		delete(c.items, entry.key)
		c.order.Remove(last)
	}
}

func expired(expiresAt time.Time, skew time.Duration) bool {
	if expiresAt.IsZero() {
		return false
	}
	return time.Now().UTC().After(expiresAt.Add(-skew))
}
