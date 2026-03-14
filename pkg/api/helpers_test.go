package api

import (
	"testing"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

func TestBitbucketOwnerRepo(t *testing.T) {
	owner, repo := bitbucketOwnerRepo(storage.NamespaceRecord{Owner: "o", RepoName: "r"})
	if owner != "o" || repo != "r" {
		t.Fatalf("unexpected owner/repo: %q/%q", owner, repo)
	}

	owner, repo = bitbucketOwnerRepo(storage.NamespaceRecord{Owner: "old", RepoName: "old", FullName: " new-owner / new-repo "})
	if owner != "new-owner" || repo != "new-repo" {
		t.Fatalf("unexpected full_name parse: %q/%q", owner, repo)
	}
}

func TestResolveAPIBase(t *testing.T) {
	if got := resolveAPIBase(auth.ProviderGitHub, auth.ProviderConfig{}); got != "https://api.github.com" {
		t.Fatalf("unexpected github api base: %q", got)
	}
	if got := resolveAPIBase(auth.ProviderGitLab, auth.ProviderConfig{}); got != "https://gitlab.com/api/v4" {
		t.Fatalf("unexpected gitlab api base: %q", got)
	}
	if got := resolveAPIBase(auth.ProviderBitbucket, auth.ProviderConfig{}); got != "https://api.bitbucket.org/2.0" {
		t.Fatalf("unexpected bitbucket api base: %q", got)
	}
	if got := resolveAPIBase("custom", auth.ProviderConfig{API: auth.APIConfig{BaseURL: "https://custom.example"}}); got != "https://custom.example" {
		t.Fatalf("unexpected custom api base: %q", got)
	}
	if got := resolveAPIBase("custom", auth.ProviderConfig{}); got != "" {
		t.Fatalf("unexpected unknown api base: %q", got)
	}
}
