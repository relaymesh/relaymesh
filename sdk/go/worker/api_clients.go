package worker

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

const (
	defaultAPIEndpoint = "http://localhost:8080"
	defaultTenantID    = "default"
)

type apiClientConfig struct {
	BaseURL    string
	APIKey     string
	OAuth2     *auth.OAuth2Config
	HTTPClient *http.Client
}

type apiClientBinder interface {
	BindAPIClient(cfg apiClientConfig)
}

func resolveEndpoint(explicit string) string {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return strings.TrimRight(trimmed, "/")
	}
	if endpoint := envEndpoint("RELAYMESH_ENDPOINT"); endpoint != "" {
		return endpoint
	}
	if endpoint := envEndpoint("RELAYMESH_API_BASE_URL"); endpoint != "" {
		return endpoint
	}
	return defaultAPIEndpoint
}

func envEndpoint(key string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return strings.TrimRight(value, "/")
	}
	return ""
}

func apiKeyFromEnv() string {
	return strings.TrimSpace(os.Getenv("RELAYMESH_API_KEY"))
}

func envTenantID() string {
	return strings.TrimSpace(os.Getenv("RELAYMESH_TENANT_ID"))
}

func (w *Worker) apiBaseURL() string {
	explicit := ""
	if w != nil {
		explicit = w.endpoint
	}
	return resolveEndpoint(explicit)
}

func (w *Worker) apiKeyValue() string {
	if w != nil {
		if key := strings.TrimSpace(w.apiKey); key != "" {
			return key
		}
	}
	return apiKeyFromEnv()
}

func (w *Worker) oauth2Value() *auth.OAuth2Config {
	if w == nil {
		return nil
	}
	return w.oauth2Config
}

func setAuthHeaders(ctx context.Context, header http.Header, apiKey string, oauth2Config *auth.OAuth2Config) {
	if header == nil {
		return
	}
	key := strings.TrimSpace(apiKey)
	if key == "" {
		key = apiKeyFromEnv()
	}
	if key != "" {
		header.Set("x-api-key", key)
		return
	}
	if oauth2Config != nil {
		if token, err := oauth2TokenFromConfig(ctx, *oauth2Config); err == nil && token != "" {
			header.Set("Authorization", "Bearer "+token)
		}
		return
	}
	if token, err := oauth2Token(ctx); err == nil && token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
}

func (w *Worker) rulesClient() *RulesClient {
	return &RulesClient{
		BaseURL: w.apiBaseURL(),
		APIKey:  w.apiKeyValue(),
		OAuth2:  w.oauth2Value(),
	}
}

func (w *Worker) driversClient() *DriversClient {
	return &DriversClient{
		BaseURL: w.apiBaseURL(),
		APIKey:  w.apiKeyValue(),
		OAuth2:  w.oauth2Value(),
	}
}

func (w *Worker) eventLogsClient() *EventLogsClient {
	return &EventLogsClient{
		BaseURL: w.apiBaseURL(),
		APIKey:  w.apiKeyValue(),
		OAuth2:  w.oauth2Value(),
	}
}

func (w *Worker) tenantIDValue() string {
	if w != nil {
		if trimmed := strings.TrimSpace(w.tenantID); trimmed != "" {
			return trimmed
		}
	}
	if env := envTenantID(); env != "" {
		return env
	}
	return defaultTenantID
}
