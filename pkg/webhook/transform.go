package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/relaymesh/relaymesh/pkg/core"
)

const maxTransformRuntime = 2 * time.Second

func applyRuleTransform(event core.Event, transformJS string) (core.Event, error) {
	transformJS = strings.TrimSpace(transformJS)
	if transformJS == "" {
		return event, nil
	}

	payload, err := eventPayloadJSON(event)
	if err != nil {
		return event, err
	}

	vm := goja.New()
	if _, err := vm.RunString("var transform = " + transformJS); err != nil {
		return event, fmt.Errorf("compile transform_js: %w", err)
	}
	transform, ok := goja.AssertFunction(vm.Get("transform"))
	if !ok {
		return event, fmt.Errorf("transform_js must define a function")
	}

	var input any
	if err := json.Unmarshal(payload, &input); err != nil {
		return event, err
	}
	ctx := transformEventContext(event, input)
	if err := vm.Set("event", ctx); err != nil {
		return event, fmt.Errorf("set transform event context: %w", err)
	}
	timer := time.AfterFunc(maxTransformRuntime, func() {
		vm.Interrupt(errors.New("transform_js execution timed out"))
	})
	defer timer.Stop()

	result, err := transform(goja.Undefined(), vm.ToValue(input), vm.ToValue(ctx))
	if err != nil {
		return event, fmt.Errorf("execute transform_js: %w", err)
	}
	outPayload := transformOutputPayload(result.Export())

	out, err := json.Marshal(outPayload)
	if err != nil {
		return event, fmt.Errorf("marshal transformed payload: %w", err)
	}

	event.RawPayload = out
	event.RawObject = nil
	return event, nil
}

func ApplyRuleTransform(event core.Event, transformJS string) (core.Event, error) {
	return applyRuleTransform(event, transformJS)
}

func transformEventContext(event core.Event, payload any) map[string]any {
	ctx := map[string]any{
		"provider":              event.Provider,
		"name":                  event.Name,
		"request_id":            event.RequestID,
		"state_id":              event.StateID,
		"tenant_id":             event.TenantID,
		"installation_id":       event.InstallationID,
		"provider_instance_key": event.ProviderInstanceKey,
		"namespace_id":          event.NamespaceID,
		"namespace_name":        event.NamespaceName,
		"headers":               event.Headers,
		"data":                  event.Data,
		"payload":               payload,
	}
	return ctx
}

func transformOutputPayload(result any) any {
	if m, ok := result.(map[string]any); ok {
		if payload, exists := m["payload"]; exists {
			return payload
		}
	}
	return result
}

func eventPayloadJSON(event core.Event) ([]byte, error) {
	if len(event.RawPayload) > 0 {
		return event.RawPayload, nil
	}
	if event.RawObject != nil {
		return json.Marshal(event.RawObject)
	}
	if event.Data != nil {
		return json.Marshal(event.Data)
	}
	return []byte("{}"), nil
}
