package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/relaymesh/relaymesh/pkg/core"
)

// rawObjectAndFlatten unmarshals a raw JSON byte slice into both an interface{}
// and a flattened map[string]interface{}. This is useful for both preserving the
// original structure and for easy access to nested fields.
func rawObjectAndFlatten(raw []byte) (interface{}, map[string]interface{}) {
	var out interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, map[string]interface{}{}
	}
	objectMap, ok := out.(map[string]interface{})
	if !ok {
		return out, map[string]interface{}{}
	}
	return out, core.Flatten(objectMap)
}

func annotatePayload(rawObject interface{}, data map[string]interface{}, provider, eventName string) (interface{}, normalizedEventFields) {
	provider = strings.TrimSpace(provider)
	eventName = strings.TrimSpace(eventName)
	normalized := deriveNormalizedEventFields(provider, eventName, data)
	var refValue string
	var hasRef bool
	if data != nil {
		if provider != "" {
			data["provider"] = provider
		}
		if eventName != "" {
			data["event"] = eventName
			data["event_type"] = normalized.EventType
		}
		if normalized.ProviderType != "" {
			data["provider_type"] = normalized.ProviderType
		}
		if normalized.Action != "" {
			data["action"] = normalized.Action
		}
		if normalized.ResourceType != "" {
			data["resource_type"] = normalized.ResourceType
		}
		if normalized.ResourceID != "" {
			data["resource_id"] = normalized.ResourceID
		}
		if normalized.ResourceName != "" {
			data["resource_name"] = normalized.ResourceName
		}
		if normalized.ActorID != "" {
			data["actor_id"] = normalized.ActorID
		}
		if normalized.ActorName != "" {
			data["actor_name"] = normalized.ActorName
		}
		if normalized.OccurredAt != "" {
			data["occurred_at"] = normalized.OccurredAt
		}
		if ref, ok := deriveGitRef(data); ok {
			refValue = ref
			hasRef = true
			data["ref"] = ref
		}
	}
	if obj, ok := rawObject.(map[string]interface{}); ok {
		if provider != "" {
			obj["provider"] = provider
		}
		if eventName != "" {
			obj["event"] = eventName
			obj["event_type"] = normalized.EventType
		}
		if normalized.ProviderType != "" {
			obj["provider_type"] = normalized.ProviderType
		}
		if normalized.Action != "" {
			obj["action"] = normalized.Action
		}
		if normalized.ResourceType != "" {
			obj["resource_type"] = normalized.ResourceType
		}
		if normalized.ResourceID != "" {
			obj["resource_id"] = normalized.ResourceID
		}
		if normalized.ResourceName != "" {
			obj["resource_name"] = normalized.ResourceName
		}
		if normalized.ActorID != "" {
			obj["actor_id"] = normalized.ActorID
		}
		if normalized.ActorName != "" {
			obj["actor_name"] = normalized.ActorName
		}
		if normalized.OccurredAt != "" {
			obj["occurred_at"] = normalized.OccurredAt
		}
		if hasRef {
			obj["ref"] = refValue
		} else if ref, ok := deriveGitRef(data); ok {
			obj["ref"] = ref
		}
		return obj, normalized
	}
	return rawObject, normalized
}

func deriveGitRef(data map[string]interface{}) (string, bool) {
	if data == nil {
		return "", false
	}
	if value, ok := data["ref"]; ok {
		if normalized, valid := normalizeGitRef(fmt.Sprintf("%v", value)); valid {
			return normalized, true
		}
	}
	candidates := []string{
		"check_suite.head_branch",
		"check_suite.head_ref",
		"workflow_run.head_branch",
		"workflow_run.head_ref",
		"push.ref",
	}
	for _, key := range candidates {
		if value := data[key]; value != nil {
			if normalized, valid := normalizeGitRef(fmt.Sprintf("%v", value)); valid {
				return normalized, true
			}
		}
	}
	return "", false
}

func normalizeGitRef(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if strings.HasPrefix(value, "refs/") {
		return value, true
	}
	return fmt.Sprintf("refs/heads/%s", strings.TrimPrefix(value, "refs/heads/")), true
}

func requestID(r *http.Request) string {
	if r == nil {
		return uuid.NewString()
	}
	if id := r.Header.Get("X-Request-Id"); id != "" {
		return id
	}
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	if id := r.Header.Get("X-Correlation-Id"); id != "" {
		return id
	}
	return uuid.NewString()
}

func prepareWebhookRequest(w http.ResponseWriter, r *http.Request, maxBody int64, logger *log.Logger) (*http.Request, *log.Logger, string, []byte, bool) {
	if maxBody > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	}
	reqID := requestID(r)
	w.Header().Set("X-Request-Id", reqID)
	reqLogger := core.WithRequestID(logger, reqID)
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return nil, reqLogger, reqID, nil, false
	}
	r.Body = io.NopCloser(bytes.NewReader(rawBody))
	return r, reqLogger, reqID, rawBody, true
}

func cloneHeaders(headers http.Header) map[string][]string {
	if headers == nil {
		return nil
	}
	out := make(map[string][]string, len(headers))
	for key, values := range headers {
		if len(values) == 0 {
			continue
		}
		out[key] = append([]string(nil), values...)
	}
	return out
}
