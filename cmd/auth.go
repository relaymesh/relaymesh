package cmd

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/relaymesh/relaymesh/pkg/auth/oidc"
	"github.com/relaymesh/relaymesh/pkg/core"

	"golang.org/x/oauth2"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication helper",
		Long: "Fetch or store OAuth2 tokens for CLI use. " +
			"When auth_code is configured, it runs the browser login flow; otherwise it fetches a client-credentials token.",
		Example: "  githook auth",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadCLIConfig()
			if err != nil {
				return err
			}
			if !cfg.Auth.OAuth2.Enabled {
				return errors.New("auth.oauth2 is not enabled")
			}
			if strings.ToLower(strings.TrimSpace(cfg.Auth.OAuth2.Mode)) == "auth_code" {
				return printLoginURL(cfg)
			}
			return fetchToken(cfg)
		},
	}
	return cmd
}

func printLoginURL(cfg core.AppConfig) error {
	if cfg.Auth.OAuth2.ClientID == "" {
		return errors.New("client_id is required for auth_code")
	}
	codeVerifier := randomString(32)
	state := randomString(16)
	redirectURL, codeCh, err := startLocalCallback(state, codeVerifier)
	if err != nil {
		return err
	}
	authURL, err := buildAuthCodeURL(context.Background(), cfg, redirectURL, state, codeVerifier)
	if err != nil {
		return err
	}
	if err := printStdoutf("Open this URL in your browser:\n%s\n", authURL); err != nil {
		return err
	}
	_ = openBrowser(authURL)
	result, err := waitForAuthCode(codeCh)
	if err != nil {
		return err
	}
	token, err := exchangeAuthCode(context.Background(), cfg, redirectURL, result)
	if err != nil {
		return err
	}
	return storeToken(cfg, token.AccessToken, int64(time.Until(token.Expiry).Seconds()))
}

func fetchToken(cfg core.AppConfig) error {
	resp, err := oidc.ClientCredentialsToken(context.Background(), cfg.Auth.OAuth2)
	if err != nil {
		return err
	}
	return storeToken(cfg, resp.AccessToken, resp.ExpiresIn)
}

func storeToken(cfg core.AppConfig, token string, expiresIn int64) error {
	cachePath := tokenCachePath()
	if cachePath == "" {
		return errors.New("token cache path unavailable")
	}
	expiry := time.Now().Add(time.Duration(expiresIn) * time.Second)
	if expiresIn == 0 {
		expiry = time.Now().Add(30 * time.Minute)
	}
	key := oidc.CacheKey(cfg.Auth.OAuth2)
	if key == "||" {
		key = "default"
	}
	if err := oidc.StoreCachedToken(cachePath, key, token, expiry); err != nil {
		return err
	}
	if err := printStdoutf("Stored token at %s\n", cachePath); err != nil {
		return err
	}
	return nil
}

type authCodeResult struct {
	code         string
	codeVerifier string
}

func startLocalCallback(state, codeVerifier string) (string, <-chan authCodeResult, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	resultCh := make(chan authCodeResult, 1)
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/auth/callback" {
				http.NotFound(w, r)
				return
			}
			if strings.TrimSpace(r.URL.Query().Get("state")) != state {
				http.Error(w, "invalid state", http.StatusBadRequest)
				return
			}
			code := strings.TrimSpace(r.URL.Query().Get("code"))
			if code == "" {
				http.Error(w, "missing code", http.StatusBadRequest)
				return
			}
			if _, err := fmt.Fprintln(w, "Authentication complete. You can close this window."); err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "auth callback response write failed:", err)
			}
			resultCh <- authCodeResult{code: code, codeVerifier: codeVerifier}
		}),
	}
	go func() {
		_ = srv.Serve(listener)
	}()
	redirectURL := fmt.Sprintf("http://%s/auth/callback", listener.Addr().String())
	return redirectURL, resultCh, nil
}

func waitForAuthCode(ch <-chan authCodeResult) (authCodeResult, error) {
	select {
	case result := <-ch:
		return result, nil
	case <-time.After(5 * time.Minute):
		return authCodeResult{}, errors.New("login timed out")
	}
}

func buildAuthCodeURL(ctx context.Context, cfg core.AppConfig, redirectURL, state, codeVerifier string) (string, error) {
	authorizeURL, tokenURL, _, err := oidc.ResolveEndpoints(ctx, cfg.Auth.OAuth2)
	if err != nil {
		return "", err
	}
	_ = tokenURL
	codeChallenge := pkceChallenge(codeVerifier)
	oauthConfig := oauth2.Config{
		ClientID:     cfg.Auth.OAuth2.ClientID,
		ClientSecret: cfg.Auth.OAuth2.ClientSecret,
		RedirectURL:  redirectURL,
		Scopes:       cfg.Auth.OAuth2.Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL: authorizeURL,
		},
	}
	return oauthConfig.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	), nil
}

func exchangeAuthCode(ctx context.Context, cfg core.AppConfig, redirectURL string, result authCodeResult) (*oauth2.Token, error) {
	authorizeURL, tokenURL, _, err := oidc.ResolveEndpoints(ctx, cfg.Auth.OAuth2)
	if err != nil {
		return nil, err
	}
	oauthConfig := oauth2.Config{
		ClientID:     cfg.Auth.OAuth2.ClientID,
		ClientSecret: cfg.Auth.OAuth2.ClientSecret,
		RedirectURL:  redirectURL,
		Scopes:       cfg.Auth.OAuth2.Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  authorizeURL,
			TokenURL: tokenURL,
		},
	}
	return oauthConfig.Exchange(ctx, result.code, oauth2.SetAuthURLParam("code_verifier", result.codeVerifier))
}

func randomString(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
