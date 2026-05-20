package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

func TestDeepSeekAdapterUnsupportedCapabilities(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	a, err := NewDeepSeekAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewDeepSeekAdapter() error = %v", err)
	}

	t.Run("responses", func(t *testing.T) {
		_, err := a.CreateResponse(context.Background(), &adapter.CreateResponseRequest{Model: "deepseek-chat"})
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("CreateResponse() error = %v, want ErrCapabilityUnsupported", err)
		}
	})

	t.Run("embeddings", func(t *testing.T) {
		_, err := a.CreateEmbeddings(context.Background(), &adapter.EmbeddingsRequest{
			Model: "deepseek-chat",
			Input: "hello",
		})
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("CreateEmbeddings() error = %v, want ErrCapabilityUnsupported", err)
		}
	})

	t.Run("image", func(t *testing.T) {
		_, err := a.CreateImage(context.Background(), &adapter.ImageRequest{
			Model:  "deepseek-chat",
			Prompt: "hello",
		})
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("CreateImage() error = %v, want ErrCapabilityUnsupported", err)
		}
	})

	t.Run("rerank", func(t *testing.T) {
		_, err := a.Rerank(context.Background(), &adapter.RerankRequest{
			Model:     "deepseek-chat",
			Query:     "hello",
			Documents: []string{"a", "b"},
		})
		if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
			t.Fatalf("Rerank() error = %v, want ErrCapabilityUnsupported", err)
		}
	})
}

func TestDeepSeekAdapterListModels_NormalizesOfficialModels(t *testing.T) {
	t.Helper()

	var gotAuth string
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path

		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/models")
		}

		fmt.Fprint(w, `{
			"object": "list",
			"data": [
				{"id": "deepseek-chat", "object": "model", "owned_by": "deepseek"},
				{"id": "deepseek-reasoner", "object": "model", "owned_by": "deepseek"}
			]
		}`)
	}))
	defer server.Close()

	a, err := NewDeepSeekAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewDeepSeekAdapter() error = %v", err)
	}

	models, err := a.ListModels(context.Background(), "runtime-key")
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}

	if gotAuth != "Bearer runtime-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer runtime-key")
	}
	if gotPath != "/models" {
		t.Fatalf("path = %q, want %q", gotPath, "/models")
	}
	if len(models) != 2 {
		t.Fatalf("len(models) = %d, want 2", len(models))
	}

	chatModel := models[0]
	if chatModel.ID != "deepseek-chat" {
		t.Fatalf("models[0].ID = %q, want %q", chatModel.ID, "deepseek-chat")
	}
	if chatModel.Type != "chat" {
		t.Fatalf("models[0].Type = %q, want %q", chatModel.Type, "chat")
	}
	if chatModel.ContextLength != 128000 {
		t.Fatalf("models[0].ContextLength = %d, want %d", chatModel.ContextLength, 128000)
	}
	if !containsDeepSeekValue(chatModel.Capabilities, "chat") {
		t.Fatalf("models[0].Capabilities = %#v, want to contain %q", chatModel.Capabilities, "chat")
	}
	if !containsDeepSeekValue(chatModel.Capabilities, "stream") {
		t.Fatalf("models[0].Capabilities = %#v, want to contain %q", chatModel.Capabilities, "stream")
	}
	if containsDeepSeekValue(chatModel.Capabilities, "embedding") || containsDeepSeekValue(chatModel.Capabilities, "image") || containsDeepSeekValue(chatModel.Capabilities, "rerank") {
		t.Fatalf("models[0].Capabilities = %#v, should not expose unsupported capabilities", chatModel.Capabilities)
	}

	reasonerModel := models[1]
	if reasonerModel.ID != "deepseek-reasoner" {
		t.Fatalf("models[1].ID = %q, want %q", reasonerModel.ID, "deepseek-reasoner")
	}
	if reasonerModel.Type != "chat" {
		t.Fatalf("models[1].Type = %q, want %q", reasonerModel.Type, "chat")
	}
	if reasonerModel.ContextLength != 128000 {
		t.Fatalf("models[1].ContextLength = %d, want %d", reasonerModel.ContextLength, 128000)
	}
	if !containsDeepSeekValue(reasonerModel.Capabilities, "chat") {
		t.Fatalf("models[1].Capabilities = %#v, want to contain %q", reasonerModel.Capabilities, "chat")
	}
	if !containsDeepSeekValue(reasonerModel.Capabilities, "stream") {
		t.Fatalf("models[1].Capabilities = %#v, want to contain %q", reasonerModel.Capabilities, "stream")
	}
	if containsDeepSeekValue(reasonerModel.Capabilities, "embedding") || containsDeepSeekValue(reasonerModel.Capabilities, "image") || containsDeepSeekValue(reasonerModel.Capabilities, "rerank") {
		t.Fatalf("models[1].Capabilities = %#v, should not expose unsupported capabilities", reasonerModel.Capabilities)
	}
}

func TestDeepSeekAdapterGetBalance_UsesOfficialEndpoint(t *testing.T) {
	t.Helper()

	var gotPath string
	var gotAccept string
	var gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		gotAuth = r.Header.Get("Authorization")

		if r.URL.Path != "/user/balance" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/user/balance")
		}

		fmt.Fprint(w, `{
			"is_available": true,
			"balance_infos": [
				{
					"currency": "CNY",
					"total_balance": "110.00",
					"granted_balance": "10.00",
					"topped_up_balance": "100.00"
				}
			]
		}`)
	}))
	defer server.Close()

	a, err := NewDeepSeekAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewDeepSeekAdapter() error = %v", err)
	}

	balance, err := a.GetBalance(context.Background(), "runtime-key")
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}

	if gotPath != "/user/balance" {
		t.Fatalf("path = %q, want %q", gotPath, "/user/balance")
	}
	if gotAccept != "application/json" {
		t.Fatalf("Accept = %q, want %q", gotAccept, "application/json")
	}
	if gotAuth != "Bearer runtime-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer runtime-key")
	}
	if got := balance.Currency; got != "CNY" {
		t.Fatalf("Currency = %q, want %q", got, "CNY")
	}
	if got := balance.Total.String(); got != "110" {
		t.Fatalf("Total = %q, want %q", got, "110")
	}
	if got := balance.Remaining.String(); got != "110" {
		t.Fatalf("Remaining = %q, want %q", got, "110")
	}
}

func TestDeepSeekAdapterGetProviderInfo_DeclaresDocumentedCapabilities(t *testing.T) {
	t.Helper()

	a, err := NewDeepSeekAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: "https://api.deepseek.com/v1",
	})
	if err != nil {
		t.Fatalf("NewDeepSeekAdapter() error = %v", err)
	}

	info := a.GetProviderInfo()
	if info == nil {
		t.Fatal("GetProviderInfo() = nil, want non-nil")
	}

	if !containsDeepSeekValue(info.Capabilities, "chat") {
		t.Fatalf("Capabilities = %#v, want to contain %q", info.Capabilities, "chat")
	}
	if !containsDeepSeekValue(info.Capabilities, "stream") {
		t.Fatalf("Capabilities = %#v, want to contain %q", info.Capabilities, "stream")
	}
	if !containsDeepSeekValue(info.Capabilities, "model_listing") {
		t.Fatalf("Capabilities = %#v, want to contain %q", info.Capabilities, "model_listing")
	}
	if containsDeepSeekValue(info.Capabilities, "embedding") || containsDeepSeekValue(info.Capabilities, "image") || containsDeepSeekValue(info.Capabilities, "rerank") || containsDeepSeekValue(info.Capabilities, "completion") {
		t.Fatalf("Capabilities = %#v, should not expose unsupported adapter capabilities", info.Capabilities)
	}
}

func containsDeepSeekValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
