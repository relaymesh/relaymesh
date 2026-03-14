package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/relaymesh/relaymesh/pkg/auth"
	oidchelper "github.com/relaymesh/relaymesh/pkg/auth/oidc"

	"golang.org/x/oauth2"
)

type oauth2Handler struct {
	cfg    auth.OAuth2Config
	logger *log.Logger
	mu     sync.Mutex
	state  map[string]oauthState
}

type oauthState struct {
	codeVerifier string
	expiresAt    time.Time
}

func newOAuth2Handler(cfg auth.OAuth2Config, logger *log.Logger) *oauth2Handler {
	return &oauth2Handler{
		cfg:    cfg,
		logger: logger,
		state:  make(map[string]oauthState),
	}
}

func (h *oauth2Handler) Login(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.Enabled {
		http.Error(w, "auth disabled", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !supportsAuthCode(h.cfg.Mode) {
		http.Error(w, "auth_code flow not enabled", http.StatusBadRequest)
		return
	}
	state := randomString(16)
	verifier := randomString(32)
	challenge := codeChallenge(verifier)

	h.mu.Lock()
	h.pruneStatesLocked()
	h.state[state] = oauthState{codeVerifier: verifier, expiresAt: time.Now().Add(10 * time.Minute)}
	h.mu.Unlock()

	authorizeURL, tokenURL, _, err := oidchelper.ResolveEndpoints(r.Context(), h.cfg)
	if err != nil {
		h.logf("auth_code discovery failed: %v", err)
		http.Error(w, "oidc discovery failed", http.StatusBadRequest)
		return
	}
	oauthConfig := oauth2.Config{
		ClientID:     h.cfg.ClientID,
		ClientSecret: h.cfg.ClientSecret,
		RedirectURL:  h.cfg.RedirectURL,
		Scopes:       h.cfg.Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  authorizeURL,
			TokenURL: tokenURL,
		},
	}
	url := oauthConfig.AuthCodeURL(
		state,
		oauth2.AccessTypeOnline,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("audience", h.cfg.Audience),
	)
	http.Redirect(w, r, url, http.StatusFound)
}

func (h *oauth2Handler) Callback(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.Enabled {
		http.Error(w, "auth disabled", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !supportsAuthCode(h.cfg.Mode) {
		http.Error(w, "auth_code flow not enabled", http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		http.Error(w, "missing code or state", http.StatusBadRequest)
		return
	}
	verifier, err := h.consumeState(state)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	authorizeURL, tokenURL, _, err := oidchelper.ResolveEndpoints(r.Context(), h.cfg)
	if err != nil {
		h.logf("auth_code discovery failed: %v", err)
		http.Error(w, "oidc discovery failed", http.StatusBadRequest)
		return
	}
	oauthConfig := oauth2.Config{
		ClientID:     h.cfg.ClientID,
		ClientSecret: h.cfg.ClientSecret,
		RedirectURL:  h.cfg.RedirectURL,
		Scopes:       h.cfg.Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  authorizeURL,
			TokenURL: tokenURL,
		},
	}
	token, err := oauthConfig.Exchange(
		r.Context(),
		code,
		oauth2.SetAuthURLParam("code_verifier", verifier),
		oauth2.SetAuthURLParam("audience", h.cfg.Audience),
	)
	if err != nil {
		h.logf("auth_code exchange failed: %v", err)
		http.Error(w, "token exchange failed", http.StatusBadRequest)
		return
	}
	h.storeToken(token.AccessToken, token.Expiry)
	payload := map[string]interface{}{
		"token_type": token.TokenType,
		"expiry":     token.Expiry,
		"status":     "ok",
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *oauth2Handler) consumeState(state string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pruneStatesLocked()
	item, ok := h.state[state]
	if !ok || time.Now().After(item.expiresAt) {
		delete(h.state, state)
		return "", errors.New("invalid state")
	}
	delete(h.state, state)
	return item.codeVerifier, nil
}

func (h *oauth2Handler) logf(format string, args ...interface{}) {
	if h.logger == nil {
		return
	}
	h.logger.Printf(format, args...)
}

func (h *oauth2Handler) storeToken(token string, expiry time.Time) {
	if token == "" {
		return
	}
	cachePath := tokenCachePath()
	if cachePath == "" {
		return
	}
	if expiry.IsZero() {
		expiry = time.Now().Add(30 * time.Minute)
	}
	key := oidchelper.CacheKey(h.cfg)
	if key == "||" {
		key = "default"
	}
	if err := oidchelper.StoreCachedToken(cachePath, key, token, expiry); err != nil {
		h.logf("token cache write failed: %v", err)
	}
}

func supportsAuthCode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto", "auth_code":
		return true
	default:
		return false
	}
}

func (h *oauth2Handler) pruneStatesLocked() {
	now := time.Now()
	for key, item := range h.state {
		if now.After(item.expiresAt) {
			delete(h.state, key)
		}
	}
}

func randomString(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func codeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func tokenCachePath() string {
	if path := strings.TrimSpace(os.Getenv("github.com/relaymesh/relaymesh_TOKEN_CACHE")); path != "" {
		return path
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "github.com/relaymesh/relaymesh", "token.json")
}
