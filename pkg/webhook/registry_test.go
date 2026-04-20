package webhook

import (
	"net/http"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/auth"
	providerspkg "github.com/relaymesh/relaymesh/pkg/providers"
)

type webhookProviderStub struct {
	name string
}

func (w webhookProviderStub) Name() string { return w.name }
func (w webhookProviderStub) Definition() providerspkg.Definition {
	return providerspkg.Definition{Name: w.name}
}
func (w webhookProviderStub) WebhookPath(cfg auth.ProviderConfig) string { return cfg.Webhook.Path }
func (w webhookProviderStub) NewHandler(cfg auth.ProviderConfig, opts HandlerOptions) (http.Handler, error) {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { rw.WriteHeader(http.StatusNoContent) }), nil
}
func (w webhookProviderStub) WebhookLogFields(cfg auth.ProviderConfig) string { return "" }

func TestWebhookRegistry(t *testing.T) {
	var nilRegistry *Registry
	if _, ok := nilRegistry.Provider("github"); ok {
		t.Fatalf("expected nil registry lookup false")
	}
	if providers := nilRegistry.Providers(); providers != nil {
		t.Fatalf("expected nil providers for nil registry")
	}

	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Fatalf("expected nil provider error")
	}
	if err := r.Register(webhookProviderStub{name: ""}); err == nil {
		t.Fatalf("expected provider name error")
	}
	if err := r.Register(webhookProviderStub{name: "github"}); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := r.Register(webhookProviderStub{name: "github"}); err == nil {
		t.Fatalf("expected duplicate provider error")
	}
	if _, ok := r.Provider("github"); !ok {
		t.Fatalf("expected provider lookup to succeed")
	}
	if len(r.Providers()) != 1 {
		t.Fatalf("expected one provider")
	}

	var bad *Registry
	if err := bad.Register(webhookProviderStub{name: "x"}); err == nil {
		t.Fatalf("expected nil registry register error")
	}
}

func TestWebhookDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	if _, ok := r.Provider("github"); !ok {
		t.Fatalf("expected github provider")
	}
	if _, ok := r.Provider("gitlab"); !ok {
		t.Fatalf("expected gitlab provider")
	}
	if _, ok := r.Provider("bitbucket"); !ok {
		t.Fatalf("expected bitbucket provider")
	}
	if _, ok := r.Provider("slack"); !ok {
		t.Fatalf("expected slack provider")
	}
	if _, ok := r.Provider("atlassian"); !ok {
		t.Fatalf("expected atlassian provider")
	}
}
