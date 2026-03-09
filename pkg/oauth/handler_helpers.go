package oauth

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/relaymesh/githook/pkg/auth"
	"github.com/relaymesh/githook/pkg/providerinstance"
	"github.com/relaymesh/githook/pkg/storage"
)

func (h *Handler) redirectOrJSON(w http.ResponseWriter, r *http.Request, params map[string]string, instanceRedirect string) {
	redirect := strings.TrimSpace(instanceRedirect)
	if redirect == "" {
		redirect = strings.TrimSpace(h.RedirectBase)
	}
	if redirect == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(params)
		return
	}
	target, err := url.Parse(redirect)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(params)
		return
	}
	values := target.Query()
	for key, value := range params {
		if value == "" {
			continue
		}
		values.Set(key, value)
	}
	target.RawQuery = values.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func (h *Handler) resolveInstanceConfig(ctx context.Context, provider, instanceKey string, fallback auth.ProviderConfig) (auth.ProviderConfig, string, string) {
	provider = strings.TrimSpace(provider)
	instanceKey = strings.TrimSpace(instanceKey)
	if h.ProviderInstanceCache == nil && h.ProviderInstanceStore == nil {
		return fallback, instanceKey, ""
	}
	if instanceKey == "" {
		if h.ProviderInstanceStore == nil {
			return fallback, instanceKey, ""
		}
		records, err := h.ProviderInstanceStore.ListProviderInstances(ctx, provider)
		if err != nil || len(records) == 0 {
			return fallback, instanceKey, ""
		}
		if len(records) == 1 {
			cfg, err := providerinstance.ProviderConfigFromRecord(records[0])
			if err != nil {
				return fallback, instanceKey, ""
			}
			return cfg, records[0].Key, records[0].RedirectBaseURL
		}
		match, ok := matchProviderConfigRecord(records, fallback)
		if !ok {
			return fallback, instanceKey, ""
		}
		cfg, err := providerinstance.ProviderConfigFromRecord(match)
		if err != nil {
			return fallback, instanceKey, ""
		}
		return cfg, match.Key, match.RedirectBaseURL
	}
	if h.ProviderInstanceCache != nil {
		if cfg, ok, err := h.ProviderInstanceCache.ConfigFor(ctx, provider, instanceKey); err == nil && ok {
			// Cache doesn't have redirect URL, fetch from store if available
			if h.ProviderInstanceStore != nil {
				record, err := h.ProviderInstanceStore.GetProviderInstance(ctx, provider, instanceKey)
				if err == nil && record != nil {
					return cfg, instanceKey, record.RedirectBaseURL
				}
			}
			return cfg, instanceKey, ""
		}
	}
	if h.ProviderInstanceStore == nil {
		return fallback, instanceKey, ""
	}
	record, err := h.ProviderInstanceStore.GetProviderInstance(ctx, provider, instanceKey)
	if err != nil || record == nil {
		return fallback, instanceKey, ""
	}
	cfg, err := providerinstance.ProviderConfigFromRecord(*record)
	if err != nil {
		return fallback, instanceKey, ""
	}
	return cfg, instanceKey, record.RedirectBaseURL
}

func resolveExistingInstallationID(ctx context.Context, store storage.Store, provider, accountID, instanceKey string) string {
	if store == nil {
		return ""
	}
	provider = strings.TrimSpace(provider)
	accountID = strings.TrimSpace(accountID)
	instanceKey = strings.TrimSpace(instanceKey)
	if provider == "" || accountID == "" {
		return ""
	}
	records, err := store.ListInstallations(ctx, provider, accountID)
	if err != nil || len(records) == 0 {
		return ""
	}
	var bestID string
	var bestTime time.Time
	for _, record := range records {
		if instanceKey != "" && record.ProviderInstanceKey != instanceKey {
			continue
		}
		if record.InstallationID == "" {
			continue
		}
		if bestID == "" || record.UpdatedAt.After(bestTime) {
			bestID = record.InstallationID
			bestTime = record.UpdatedAt
		}
	}
	return bestID
}

func dedupeInstallations(ctx context.Context, store storage.Store, provider, accountID, instanceKey, keepID string) {
	if store == nil {
		return
	}
	provider = strings.TrimSpace(provider)
	accountID = strings.TrimSpace(accountID)
	keepID = strings.TrimSpace(keepID)
	instanceKey = strings.TrimSpace(instanceKey)
	if provider == "" || accountID == "" || keepID == "" {
		return
	}
	records, err := store.ListInstallations(ctx, provider, accountID)
	if err != nil {
		return
	}
	for _, record := range records {
		if record.InstallationID == "" || record.InstallationID == keepID {
			continue
		}
		if instanceKey != "" && record.ProviderInstanceKey != "" && record.ProviderInstanceKey != instanceKey {
			continue
		}
		_ = store.DeleteInstallation(ctx, provider, accountID, record.InstallationID, record.ProviderInstanceKey)
	}
}

func matchProviderConfigRecord(records []storage.ProviderInstanceRecord, fallback auth.ProviderConfig) (storage.ProviderInstanceRecord, bool) {
	configJSON, ok := providerConfigJSON(fallback)
	if !ok {
		return storage.ProviderInstanceRecord{}, false
	}
	var match *storage.ProviderInstanceRecord
	for i := range records {
		record := records[i]
		normalized, ok := providerinstance.NormalizeProviderConfigJSON(record.ConfigJSON)
		if !ok || normalized != configJSON {
			continue
		}
		if !isProviderInstanceHash(record.Key) {
			continue
		}
		if match != nil {
			return storage.ProviderInstanceRecord{}, false
		}
		match = &record
	}
	if match == nil {
		return storage.ProviderInstanceRecord{}, false
	}
	return *match, true
}

func providerConfigJSON(cfg auth.ProviderConfig) (string, bool) {
	cfg.Key = ""
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(raw)), true
}

func isProviderInstanceHash(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

type oauthToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	CreatedAt    int64  `json:"created_at"`
	ExpiresAt    *time.Time
}

func (t oauthToken) MetadataJSON() string {
	payload := map[string]interface{}{
		"token_type": t.TokenType,
		"scope":      t.Scope,
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func storeAvailable(store storage.Store) bool {
	if store == nil {
		return false
	}
	value := reflect.ValueOf(store)
	switch value.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func, reflect.Chan:
		return !value.IsNil()
	default:
		return true
	}
}

func namespaceStoreAvailable(store storage.NamespaceStore) bool {
	if store == nil {
		return false
	}
	value := reflect.ValueOf(store)
	switch value.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func, reflect.Chan:
		return !value.IsNil()
	default:
		return true
	}
}

func asyncNamespaceSync(ctx context.Context, logger *log.Logger, provider string, syncFn func(context.Context) error) {
	go func() {
		if logger == nil {
			logger = log.Default()
		}
		if err := syncFn(ctx); err != nil {
			logger.Printf("%s namespaces sync failed: %v", provider, err)
		}
	}()
}

func logUpsertAttempt(logger *log.Logger, record storage.InstallRecord, accessToken string) {
	if logger == nil {
		return
	}
	tokenState := "empty"
	if accessToken != "" {
		tokenState = "present"
	}
	logger.Printf("oauth upsert attempt provider=%s account_id=%s installation_id=%s token=%s", record.Provider, record.AccountID, record.InstallationID, tokenState)
}

func expiryFromToken(token oauthToken) *time.Time {
	if token.ExpiresIn <= 0 {
		return nil
	}
	expires := time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second)
	return &expires
}
