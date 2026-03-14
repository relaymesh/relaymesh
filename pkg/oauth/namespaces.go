package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/auth"
	ghprovider "github.com/relaymesh/relaymesh/pkg/providers/github"
	"github.com/relaymesh/relaymesh/pkg/storage"

	gh "github.com/google/go-github/v57/github"
)

// SyncGitHubNamespaces fetches repositories for an installation and upserts them into the namespace store.
func SyncGitHubNamespaces(ctx context.Context, store storage.NamespaceStore, cfg auth.ProviderConfig, installationID, accountID, instanceKey string) error {
	if !namespaceStoreAvailable(store) {
		return nil
	}
	if installationID == "" {
		return nil
	}
	if cfg.App.AppID == 0 || (cfg.App.PrivateKeyPath == "" && cfg.App.PrivateKeyPEM == "") {
		return nil
	}
	installationInt, err := strconv.ParseInt(installationID, 10, 64)
	if err != nil {
		return err
	}
	client, err := ghprovider.NewAppClient(ctx, ghprovider.AppConfig{
		AppID:          cfg.App.AppID,
		PrivateKeyPath: cfg.App.PrivateKeyPath,
		PrivateKeyPEM:  cfg.App.PrivateKeyPEM,
		BaseURL:        cfg.API.BaseURL,
	}, installationInt)
	if err != nil {
		return err
	}
	opts := &gh.ListOptions{PerPage: 100}
	for {
		repos, resp, err := client.Apps.ListRepos(ctx, opts)
		if err != nil {
			return err
		}
		for _, repo := range repos.Repositories {
			if repo == nil || repo.ID == nil {
				continue
			}
			repoID := strconv.FormatInt(repo.GetID(), 10)
			existing, err := store.GetNamespace(ctx, "github", repoID, instanceKey)
			if err != nil {
				return err
			}
			owner := ""
			if repo.Owner != nil {
				owner = repo.Owner.GetLogin()
			}
			visibility := "public"
			if repo.GetPrivate() {
				visibility = "private"
			}
			record := storage.NamespaceRecord{
				Provider:            "github",
				ProviderInstanceKey: instanceKey,
				AccountID:           accountID,
				InstallationID:      installationID,
				RepoID:              repoID,
				Owner:               owner,
				RepoName:            repo.GetName(),
				FullName:            repo.GetFullName(),
				Visibility:          visibility,
				DefaultBranch:       repo.GetDefaultBranch(),
				HTTPURL:             repo.GetHTMLURL(),
				SSHURL:              repo.GetSSHURL(),
				WebhooksEnabled:     existingWebhooks(existing, true),
			}
			if err := store.UpsertNamespace(ctx, record); err != nil {
				return err
			}
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return nil
}

// SyncGitLabNamespaces fetches repositories and upserts them into the namespace store.
func SyncGitLabNamespaces(ctx context.Context, store storage.NamespaceStore, cfg auth.ProviderConfig, accessToken, accountID, installationID, instanceKey string) error {
	if !namespaceStoreAvailable(store) {
		return nil
	}
	if accessToken == "" {
		return nil
	}
	baseURL := strings.TrimRight(cfg.API.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://gitlab.com/api/v4"
	}

	page := 1
	for {
		endpoint := fmt.Sprintf("%s/projects?membership=true&per_page=100&page=%d", baseURL, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		var payload []struct {
			ID                int64  `json:"id"`
			Name              string `json:"name"`
			PathWithNamespace string `json:"path_with_namespace"`
			Visibility        string `json:"visibility"`
			DefaultBranch     string `json:"default_branch"`
			WebURL            string `json:"web_url"`
			SSHURL            string `json:"ssh_url_to_repo"`
			HTTPURL           string `json:"http_url_to_repo"`
			Namespace         struct {
				Name string `json:"name"`
				Path string `json:"path"`
			} `json:"namespace"`
		}
		err = json.NewDecoder(resp.Body).Decode(&payload)
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("gitlab namespace list close failed: %v", cerr)
		}
		if err != nil {
			return err
		}
		for _, repo := range payload {
			existing, err := store.GetNamespace(ctx, "gitlab", strconv.FormatInt(repo.ID, 10), instanceKey)
			if err != nil {
				return err
			}
			record := storage.NamespaceRecord{
				Provider:            "gitlab",
				ProviderInstanceKey: instanceKey,
				AccountID:           accountID,
				InstallationID:      installationID,
				RepoID:              strconv.FormatInt(repo.ID, 10),
				Owner:               repo.Namespace.Path,
				RepoName:            repo.Name,
				FullName:            repo.PathWithNamespace,
				Visibility:          repo.Visibility,
				DefaultBranch:       repo.DefaultBranch,
				HTTPURL:             repo.HTTPURL,
				SSHURL:              repo.SSHURL,
				WebhooksEnabled:     existingWebhooks(existing, false),
			}
			if err := store.UpsertNamespace(ctx, record); err != nil {
				return err
			}
		}
		if resp.Header.Get("X-Next-Page") == "" {
			break
		}
		page++
	}
	return nil
}

// SyncBitbucketNamespaces fetches repositories and upserts them into the namespace store.
func SyncBitbucketNamespaces(ctx context.Context, store storage.NamespaceStore, cfg auth.ProviderConfig, accessToken, accountID, installationID, instanceKey string) error {
	if !namespaceStoreAvailable(store) {
		return nil
	}
	if accessToken == "" {
		return nil
	}
	baseURL := strings.TrimRight(cfg.API.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.bitbucket.org/2.0"
	}
	nextURL := baseURL + "/repositories?role=member"
	for nextURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		var payload struct {
			Next   string `json:"next"`
			Values []struct {
				UUID      string `json:"uuid"`
				Name      string `json:"name"`
				Slug      string `json:"slug"`
				FullName  string `json:"full_name"`
				Workspace struct {
					Slug string `json:"slug"`
				} `json:"workspace"`
				Owner struct {
					Username    string `json:"username"`
					DisplayName string `json:"display_name"`
				} `json:"owner"`
				MainBranch struct {
					Name string `json:"name"`
				} `json:"mainbranch"`
				Links struct {
					HTML struct {
						Href string `json:"href"`
					} `json:"html"`
					Clone []struct {
						Href string `json:"href"`
						Name string `json:"name"`
					} `json:"clone"`
				} `json:"links"`
				IsPrivate bool `json:"is_private"`
			} `json:"values"`
		}
		err = json.NewDecoder(resp.Body).Decode(&payload)
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("bitbucket repo list close failed: %v", cerr)
		}
		if err != nil {
			return err
		}
		for _, repo := range payload.Values {
			owner := strings.TrimSpace(repo.Workspace.Slug)
			if owner == "" {
				owner = strings.TrimSpace(repo.Owner.Username)
			}
			if owner == "" {
				owner = strings.TrimSpace(repo.Owner.DisplayName)
			}
			repoName := strings.TrimSpace(repo.Slug)
			if repoName == "" {
				repoName = strings.TrimSpace(repo.Name)
			}
			fullName := strings.TrimSpace(repo.FullName)
			if fullName == "" && owner != "" && repoName != "" {
				fullName = owner + "/" + repoName
			}
			visibility := "public"
			if repo.IsPrivate {
				visibility = "private"
			}
			sshURL := ""
			for _, clone := range repo.Links.Clone {
				if clone.Name == "ssh" {
					sshURL = clone.Href
					break
				}
			}
			existing, err := store.GetNamespace(ctx, "bitbucket", repo.UUID, instanceKey)
			if err != nil {
				return err
			}
			record := storage.NamespaceRecord{
				Provider:            "bitbucket",
				ProviderInstanceKey: instanceKey,
				AccountID:           accountID,
				InstallationID:      installationID,
				RepoID:              repo.UUID,
				Owner:               owner,
				RepoName:            repoName,
				FullName:            fullName,
				Visibility:          visibility,
				DefaultBranch:       repo.MainBranch.Name,
				HTTPURL:             repo.Links.HTML.Href,
				SSHURL:              sshURL,
				WebhooksEnabled:     existingWebhooks(existing, false),
			}
			if err := store.UpsertNamespace(ctx, record); err != nil {
				return err
			}
		}
		nextURL = payload.Next
	}
	return nil
}

func existingWebhooks(record *storage.NamespaceRecord, defaultValue bool) bool {
	if record == nil {
		return defaultValue
	}
	return record.WebhooksEnabled
}
