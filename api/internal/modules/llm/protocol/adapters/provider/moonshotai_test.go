package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestMoonshotAIAdapterListModelsEnrichesLatestFlagships(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want /models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"kimi-k2.7-code","owned_by":"moonshot"},{"id":"kimi-k2.6","owned_by":"moonshot"}]}`)
	}))
	defer server.Close()

	client, err := NewMoonshotAIAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewMoonshotAIAdapter returned error: %v", err)
	}
	models, err := client.ListModels(context.Background(), "runtime-key")
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}

	for _, modelID := range []string{"kimi-k2.7-code", "kimi-k2.6"} {
		model := findMoonshotModel(t, models, modelID)
		if model.ContextLength != 262144 {
			t.Fatalf("%s context = %d, want 262144", modelID, model.ContextLength)
		}
		if model.Architecture == nil || !containsGLMValue(model.Architecture.InputModalities, "image") || !containsGLMValue(model.Architecture.InputModalities, "video") {
			t.Fatalf("%s architecture = %#v, want image and video input", modelID, model.Architecture)
		}
		if !containsGLMValue(model.Capabilities, "function_calling") {
			t.Fatalf("%s capabilities = %#v, want function_calling", modelID, model.Capabilities)
		}
	}
}

func findMoonshotModel(t *testing.T, models []adapter.Model, modelID string) adapter.Model {
	t.Helper()
	for _, model := range models {
		if model.ID == modelID {
			return model
		}
	}
	t.Fatalf("model %q not found", modelID)
	return adapter.Model{}
}
