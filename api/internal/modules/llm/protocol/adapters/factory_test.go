package adapter

import (
	"context"
	"testing"
)

type stubAdapter struct {
	name string
}

func (a *stubAdapter) Name() string { return a.name }
func (a *stubAdapter) ChatCompletion(context.Context, *ChatRequest) (*ChatResponse, error) {
	return nil, nil
}
func (a *stubAdapter) ChatCompletionStream(context.Context, *ChatRequest) (<-chan StreamResponse, error) {
	return nil, nil
}
func (a *stubAdapter) CreateEmbeddings(context.Context, *EmbeddingsRequest) (*EmbeddingsResponse, error) {
	return nil, nil
}
func (a *stubAdapter) CreateImage(context.Context, *ImageRequest) (*ImageResponse, error) {
	return nil, nil
}
func (a *stubAdapter) CreateResponse(context.Context, *CreateResponseRequest) (*CreateResponseResponse, error) {
	return nil, nil
}
func (a *stubAdapter) Rerank(context.Context, *RerankRequest) (*RerankResponse, error) {
	return nil, nil
}
func (a *stubAdapter) ListModels(context.Context, string) ([]Model, error) {
	return nil, nil
}
func (a *stubAdapter) GetBalance(context.Context, string) (*Balance, error) {
	return nil, nil
}
func (a *stubAdapter) ValidateConfig(*AdapterConfig) error { return nil }
func (a *stubAdapter) GetProviderInfo() *ProviderInfo      { return nil }

func TestCreateAdapter_UsesProviderNameAsSingleSelectionSource(t *testing.T) {
	factory := NewDefaultAdapterFactory()
	factory.Register("openai", func(config *AdapterConfig) (LLMProviderAdapter, error) {
		return &stubAdapter{name: "openai"}, nil
	})

	adapter, err := factory.CreateAdapter(&AdapterConfig{
		ProviderName: "openai",
	})
	if err != nil {
		t.Fatalf("CreateAdapter returned error: %v", err)
	}
	got, ok := adapter.(*stubAdapter)
	if !ok {
		t.Fatalf("adapter type = %T, want *stubAdapter", adapter)
	}
	if got.name != "openai" {
		t.Fatalf("adapter name = %q, want %q", got.name, "openai")
	}
}

func TestCreateAdapter_RejectsUnregisteredProvider(t *testing.T) {
	factory := NewDefaultAdapterFactory()
	factory.Register("openai", func(config *AdapterConfig) (LLMProviderAdapter, error) {
		return &stubAdapter{name: "openai"}, nil
	})

	if _, err := factory.CreateAdapter(&AdapterConfig{ProviderName: "agicto"}); err == nil {
		t.Fatal("CreateAdapter returned nil error, want unsupported provider")
	}
}
