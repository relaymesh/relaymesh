package webhook

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"testing"

	gh "github.com/google/go-github/v57/github"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

func TestVerifyGitHubSHA1(t *testing.T) {
	secret := "secret"
	body := []byte("payload")
	expected := "sha1=" + hex.EncodeToString(hmacSHA1(secret, body))
	if !verifyGitHubSHA1(secret, body, expected) {
		t.Fatalf("expected signature to verify")
	}
	if verifyGitHubSHA1(secret, body, "sha1=bad") {
		t.Fatalf("expected signature to fail")
	}
}

func TestGithubNamespaceInfo(t *testing.T) {
	raw := []byte(`{"repository":{"id":123,"full_name":"org/repo","name":"repo","owner":{"login":"org"}}}`)
	id, name := githubNamespaceInfo(raw)
	if id != "123" || name != "org/repo" {
		t.Fatalf("unexpected namespace info: %q %q", id, name)
	}

	raw = []byte(`{"repository":{"id":0,"name":"repo","owner":{"login":"org"}}}`)
	id, name = githubNamespaceInfo(raw)
	if id != "" || name != "org/repo" {
		t.Fatalf("unexpected fallback namespace info: %q %q", id, name)
	}
}

func TestRecordHelpers(t *testing.T) {
	record := &storage.InstallRecord{TenantID: "tenant", AccountName: "acct", ProviderInstanceKey: "key"}
	if got := recordAccountName(record, "github"); got != "acct" {
		t.Fatalf("expected account name, got %q", got)
	}
	if got := recordTenantID(record); got != "tenant" {
		t.Fatalf("expected tenant id, got %q", got)
	}
	if got := recordInstanceKey(record); got != "key" {
		t.Fatalf("expected instance key, got %q", got)
	}
	if got := recordAccountName(nil, "github"); got != "github" {
		t.Fatalf("expected provider fallback, got %q", got)
	}
	if recordTenantID(nil) != "" || recordInstanceKey(nil) != "" {
		t.Fatalf("expected empty values for nil record")
	}
}

func TestEnsureGitHubRepoOwner(t *testing.T) {
	repo := githubRepo{FullName: "org/repo"}
	result := ensureGitHubRepoOwner(repo)
	if result.Owner != "org" {
		t.Fatalf("expected owner org, got %q", result.Owner)
	}
	repo = githubRepo{FullName: "", Owner: "kept"}
	if ensureGitHubRepoOwner(repo).Owner != "kept" {
		t.Fatalf("expected owner to remain when already set")
	}
}

func TestApplyGitHubRepoDefaults(t *testing.T) {
	repo := githubRepo{FullName: "org/repo"}
	result := applyGitHubRepoDefaults(repo, "https://github.example.com")
	if result.HTMLURL != "https://github.example.com/org/repo" {
		t.Fatalf("unexpected html url %q", result.HTMLURL)
	}
	if result.SSHURL != "git@github.example.com:org/repo.git" {
		t.Fatalf("unexpected ssh url %q", result.SSHURL)
	}
}

func TestNeedsGitHubRepoEnrichment(t *testing.T) {
	if !needsGitHubRepoEnrichment(githubRepo{}) {
		t.Fatalf("expected empty repo to need enrichment")
	}
	repo := githubRepo{Owner: "org", DefaultBranch: "main", HTMLURL: "https://x", SSHURL: "git@x"}
	if needsGitHubRepoEnrichment(repo) {
		t.Fatalf("expected complete repo to skip enrichment")
	}
}

func TestMergeGitHubRepo(t *testing.T) {
	repo := githubRepo{ID: "1"}
	login := "org"
	full := "org/repo"
	ghRepo := &gh.Repository{}
	ghRepo.Owner = &gh.User{Login: &login}
	ghRepo.Name = gh.String("repo")
	ghRepo.FullName = &full
	ghRepo.DefaultBranch = gh.String("main")
	ghRepo.HTMLURL = gh.String("https://example.com/org/repo")
	ghRepo.SSHURL = gh.String("git@example.com:org/repo.git")
	ghRepo.Private = gh.Bool(true)
	merged := mergeGitHubRepo(repo, ghRepo)
	if merged.Owner != "org" || merged.DefaultBranch != "main" || merged.Visibility != "private" {
		t.Fatalf("expected merged repo to copy metadata: %+v", merged)
	}
}

func hmacSHA1(secret string, body []byte) []byte {
	mac := hmac.New(sha1.New, []byte(secret))
	_, _ = mac.Write(body)
	return mac.Sum(nil)
}
