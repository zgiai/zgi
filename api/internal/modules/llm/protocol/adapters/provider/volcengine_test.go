package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

func TestVolcengineAdapterGetProviderInfo_IsCVOnly(t *testing.T) {
	t.Helper()

	a, err := NewVolcengineAdapter(&adapter.AdapterConfig{
		APIKey: "ak|sk",
	})
	if err != nil {
		t.Fatalf("NewVolcengineAdapter() error = %v", err)
	}

	info := a.GetProviderInfo()
	if info == nil {
		t.Fatal("GetProviderInfo() = nil, want non-nil")
	}
	if info.BaseURL != "https://visual.volcengineapi.com" {
		t.Fatalf("BaseURL = %q, want %q", info.BaseURL, "https://visual.volcengineapi.com")
	}
	if len(info.Capabilities) != 1 || info.Capabilities[0] != "image" {
		t.Fatalf("Capabilities = %#v, want image only", info.Capabilities)
	}
}

func TestNewVolcengineAdapter_RejectsArkStyleAPIKey(t *testing.T) {
	t.Helper()

	_, err := NewVolcengineAdapter(&adapter.AdapterConfig{
		APIKey: "ark-runtime-key",
	})
	if err == nil {
		t.Fatal("NewVolcengineAdapter() error = nil, want invalid config")
	}
	if !errors.Is(err, adapter.ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
}

func TestVolcengineAdapterCreateImage_UsesSignedCVRequest(t *testing.T) {
	t.Helper()

	var (
		gotAuthHeader string
		gotPath       string
		gotAction     string
		gotVersion    string
		gotPayload    map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		gotAuthHeader = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotAction = r.URL.Query().Get("Action")
		gotVersion = r.URL.Query().Get("Version")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"code":10000,
			"message":"success",
			"data":{"result":"https://cdn.volcengine.com/fake.png"}
		}`)
	}))
	defer server.Close()

	a, err := NewVolcengineAdapter(&adapter.AdapterConfig{
		APIKey:  "access-key|secret-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewVolcengineAdapter() error = %v", err)
	}

	resp, err := a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:  "high_aes_general_v21_L",
		Prompt: "a cat astronaut",
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if gotPath != "/" {
		t.Fatalf("path = %q, want %q", gotPath, "/")
	}
	if gotAction != "CVProcess" {
		t.Fatalf("Action = %q, want %q", gotAction, "CVProcess")
	}
	if gotVersion != "2022-08-31" {
		t.Fatalf("Version = %q, want %q", gotVersion, "2022-08-31")
	}
	if !strings.Contains(gotAuthHeader, "Credential=access-key/") {
		t.Fatalf("Authorization = %q, want AWS v4 signed header", gotAuthHeader)
	}
	if got := gotPayload["req_key"]; got != "high_aes_general_v21_L" {
		t.Fatalf("payload.req_key = %#v, want %q", got, "high_aes_general_v21_L")
	}
	if len(resp.Data) != 1 || resp.Data[0].URL != "https://cdn.volcengine.com/fake.png" {
		t.Fatalf("response data = %#v, want one generated image url", resp.Data)
	}
}

func TestVolcengineAdapterRejectsDoubaoSeedreamModels(t *testing.T) {
	t.Helper()

	a, err := NewVolcengineAdapter(&adapter.AdapterConfig{
		APIKey: "ak|sk",
	})
	if err != nil {
		t.Fatalf("NewVolcengineAdapter() error = %v", err)
	}

	_, err = a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:  "doubao-seedream-5-0-lite-260128",
		Prompt: "a cat",
	})
	if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
		t.Fatalf("CreateImage() error = %v, want ErrCapabilityUnsupported", err)
	}
}

func TestVolcengineAdapterUnsupportedCapabilities(t *testing.T) {
	t.Helper()

	a, err := NewVolcengineAdapter(&adapter.AdapterConfig{
		APIKey: "ak|sk",
	})
	if err != nil {
		t.Fatalf("NewVolcengineAdapter() error = %v", err)
	}

	t.Run("chat", func(t *testing.T) {
		_, err := a.ChatCompletion(context.Background(), &adapter.ChatRequest{Model: "doubao-seed"})
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("ChatCompletion() error = %v, want ErrCapabilityUnsupported", err)
		}
	})

	t.Run("stream", func(t *testing.T) {
		_, err := a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{Model: "doubao-seed"})
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("ChatCompletionStream() error = %v, want ErrCapabilityUnsupported", err)
		}
	})

	t.Run("responses", func(t *testing.T) {
		_, err := a.CreateResponse(context.Background(), &adapter.CreateResponseRequest{Model: "doubao-seed"})
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("CreateResponse() error = %v, want ErrCapabilityUnsupported", err)
		}
	})

	t.Run("embeddings", func(t *testing.T) {
		_, err := a.CreateEmbeddings(context.Background(), &adapter.EmbeddingsRequest{Model: "embed", Input: "hello"})
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("CreateEmbeddings() error = %v, want ErrCapabilityUnsupported", err)
		}
	})

	t.Run("rerank", func(t *testing.T) {
		_, err := a.Rerank(context.Background(), &adapter.RerankRequest{Model: "rerank", Query: "q", Documents: []string{"a"}})
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("Rerank() error = %v, want ErrCapabilityUnsupported", err)
		}
	})

	t.Run("list_models", func(t *testing.T) {
		_, err := a.ListModels(context.Background(), "runtime-key")
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("ListModels() error = %v, want ErrCapabilityUnsupported", err)
		}
	})

	t.Run("balance", func(t *testing.T) {
		_, err := a.GetBalance(context.Background(), "runtime-key")
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("GetBalance() error = %v, want ErrCapabilityUnsupported", err)
		}
	})
}
