package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestCohereAdapterListModels_NormalizesCapabilities(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/models")
		}
		fmt.Fprint(w, `{
			"models": [
				{
					"name": "embed-v4.0",
					"endpoints": ["embed", "embed_image", "rerank"],
					"finetuned": false,
					"context_length": 128000,
					"features": ["vision"]
				},
				{
					"name": "command-r",
					"endpoints": ["chat", "generate"],
					"finetuned": false,
					"context_length": 128000
				}
			]
		}`)
	}))
	defer server.Close()

	a, err := NewCohereAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewCohereAdapter() error = %v", err)
	}

	models, err := a.ListModels(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("len(models) = %d, want 2", len(models))
	}

	embedModel := models[0]
	if !containsCohereValue(embedModel.Capabilities, "embedding") {
		t.Fatalf("embed model capabilities = %#v, want to contain %q", embedModel.Capabilities, "embedding")
	}
	if !containsCohereValue(embedModel.Capabilities, "rerank") {
		t.Fatalf("embed model capabilities = %#v, want to contain %q", embedModel.Capabilities, "rerank")
	}
	if containsCohereValue(embedModel.Capabilities, "embed") || containsCohereValue(embedModel.Capabilities, "embed_image") {
		t.Fatalf("embed model capabilities = %#v, should not expose raw cohere endpoint names", embedModel.Capabilities)
	}
	if !containsCohereValue(embedModel.Endpoints, "embed") || !containsCohereValue(embedModel.Endpoints, "embed_image") {
		t.Fatalf("embed model endpoints = %#v, want raw upstream endpoints preserved", embedModel.Endpoints)
	}

	chatModel := models[1]
	if !containsCohereValue(chatModel.Capabilities, "chat") {
		t.Fatalf("chat model capabilities = %#v, want to contain %q", chatModel.Capabilities, "chat")
	}
	if containsCohereValue(chatModel.Capabilities, "generate") {
		t.Fatalf("chat model capabilities = %#v, should not expose raw endpoint %q", chatModel.Capabilities, "generate")
	}
}

func containsCohereValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
