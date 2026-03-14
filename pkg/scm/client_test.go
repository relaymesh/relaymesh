package scm

import (
	"context"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

func TestFactoryNewClientUnsupported(t *testing.T) {
	factory := NewFactory(auth.Config{})
	if _, err := factory.NewClient(context.Background(), auth.AuthContext{Provider: "unknown"}); err == nil {
		t.Fatalf("expected error for unsupported provider")
	}
}

func TestFactoryNewClientTokenProviders(t *testing.T) {
	factory := NewFactory(auth.Config{})

	client, err := factory.NewClient(context.Background(), auth.AuthContext{
		Provider: "gitlab",
		Token:    "dummy-token",
	})
	if err != nil || client == nil {
		t.Fatalf("expected gitlab client, got err=%v", err)
	}

	client, err = factory.NewClient(context.Background(), auth.AuthContext{
		Provider: "bitbucket",
		Token:    "dummy-token",
	})
	if err != nil || client == nil {
		t.Fatalf("expected bitbucket client, got err=%v", err)
	}
}
