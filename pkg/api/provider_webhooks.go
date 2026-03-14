package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

func enableProviderWebhook(ctx context.Context, provider string, cfg auth.ProviderConfig, token string, record storage.NamespaceRecord, hookURL string) error {
	switch provider {
	case "gitlab":
		return ensureGitLabWebhook(ctx, cfg, token, record.RepoID, hookURL, true)
	case "bitbucket":
		owner, repo := bitbucketOwnerRepo(record)
		return ensureBitbucketWebhook(ctx, cfg, token, owner, repo, hookURL, true)
	default:
		return fmt.Errorf("unsupported provider: %s", provider)
	}
}

func disableProviderWebhook(ctx context.Context, provider string, cfg auth.ProviderConfig, token string, record storage.NamespaceRecord, hookURL string) error {
	switch provider {
	case "gitlab":
		return ensureGitLabWebhook(ctx, cfg, token, record.RepoID, hookURL, false)
	case "bitbucket":
		owner, repo := bitbucketOwnerRepo(record)
		return ensureBitbucketWebhook(ctx, cfg, token, owner, repo, hookURL, false)
	default:
		return fmt.Errorf("unsupported provider: %s", provider)
	}
}

func bitbucketOwnerRepo(record storage.NamespaceRecord) (string, string) {
	owner := strings.TrimSpace(record.Owner)
	repo := strings.TrimSpace(record.RepoName)
	full := strings.TrimSpace(record.FullName)
	if full != "" {
		parts := strings.SplitN(full, "/", 2)
		if len(parts) == 2 {
			if strings.TrimSpace(parts[0]) != "" {
				owner = strings.TrimSpace(parts[0])
			}
			if strings.TrimSpace(parts[1]) != "" {
				repo = strings.TrimSpace(parts[1])
			}
		}
	}
	return owner, repo
}

func ensureGitLabWebhook(ctx context.Context, cfg auth.ProviderConfig, token, repoID, hookURL string, enable bool) error {
	if repoID == "" {
		return fmt.Errorf("gitlab repo_id missing")
	}
	baseURL := strings.TrimRight(cfg.API.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://gitlab.com/api/v4"
	}
	hooksURL := fmt.Sprintf("%s/projects/%s/hooks", baseURL, url.PathEscape(repoID))

	existingID, err := gitlabHookID(ctx, hooksURL, token, hookURL)
	if err != nil {
		return err
	}
	if enable {
		if existingID != 0 {
			return nil
		}
		body := map[string]interface{}{
			"url":                     hookURL,
			"push_events":             true,
			"merge_requests_events":   true,
			"enable_ssl_verification": true,
		}
		return gitlabCreateHook(ctx, hooksURL, token, body)
	}
	if existingID == 0 {
		return nil
	}
	deleteURL := fmt.Sprintf("%s/%d", hooksURL, existingID)
	return gitlabDeleteHook(ctx, deleteURL, token)
}

func gitlabHookID(ctx context.Context, hooksURL, token, targetURL string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hooksURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return 0, fmt.Errorf("gitlab hook list failed: %s body=%s", resp.Status, strings.TrimSpace(string(body)))
	}
	var hooks []struct {
		ID  int64  `json:"id"`
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&hooks); err != nil {
		return 0, err
	}
	for _, hook := range hooks {
		if hook.URL == targetURL {
			return hook.ID, nil
		}
	}
	return 0, nil
}

func gitlabCreateHook(ctx context.Context, hooksURL, token string, payload map[string]interface{}) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hooksURL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("gitlab hook create failed: %s body=%s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func gitlabDeleteHook(ctx context.Context, hookURL, token string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, hookURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("gitlab hook delete failed: %s body=%s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func ensureBitbucketWebhook(ctx context.Context, cfg auth.ProviderConfig, token, owner, repo, hookURL string, enable bool) error {
	if owner == "" || repo == "" {
		return fmt.Errorf("bitbucket owner/repo missing")
	}
	baseURL := strings.TrimRight(cfg.API.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.bitbucket.org/2.0"
	}
	hooksURL := fmt.Sprintf("%s/repositories/%s/%s/hooks", baseURL, url.PathEscape(owner), url.PathEscape(repo))

	existingID, err := bitbucketHookID(ctx, hooksURL, token, hookURL)
	if err != nil {
		return err
	}
	if enable {
		if existingID != "" {
			return nil
		}
		body := map[string]interface{}{
			"description": "github.com/relaymesh/relaymesh",
			"url":         hookURL,
			"active":      true,
			"events": []string{
				"repo:push",
				"pullrequest:created",
				"pullrequest:updated",
				"pullrequest:fulfilled",
			},
		}
		return bitbucketCreateHook(ctx, hooksURL, token, body)
	}
	if existingID == "" {
		return nil
	}
	deleteURL := fmt.Sprintf("%s/%s", hooksURL, existingID)
	return bitbucketDeleteHook(ctx, deleteURL, token)
}

func bitbucketHookID(ctx context.Context, hooksURL, token, targetURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hooksURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("bitbucket hook list failed: %s body=%s", resp.Status, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Values []struct {
			UUID string `json:"uuid"`
			URL  string `json:"url"`
		} `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	for _, hook := range payload.Values {
		if hook.URL == targetURL {
			return hook.UUID, nil
		}
	}
	return "", nil
}

func bitbucketCreateHook(ctx context.Context, hooksURL, token string, payload map[string]interface{}) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hooksURL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("bitbucket hook create failed: %s body=%s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func bitbucketDeleteHook(ctx context.Context, hookURL, token string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, hookURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("bitbucket hook delete failed: %s body=%s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}
