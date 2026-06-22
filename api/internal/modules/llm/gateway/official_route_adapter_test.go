package gateway

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	_ "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters/provider"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
)

func TestCreateAdapterConfig_OfficialRouteUsesZGICloudTransport(t *testing.T) {
	setGatewayTestConfig(t)

	svc := &llmGatewayServiceImpl{}
	orgID := uuid.New()
	selection := &ProviderSelection{
		Provider: providermodel.LLMProvider{
			ID:       uuid.New(),
			Provider: "cohere",
		},
		UseSystemProvider: true,
		ChannelProvider:   "zgi-cloud",
		APIBaseURL:        "http://console.internal/v1/internal",
	}

	cfg := svc.createAdapterConfig(selection, orgID)

	if cfg.ProviderName != "zgi-cloud" {
		t.Fatalf("ProviderName = %q, want %q for official transport", cfg.ProviderName, "zgi-cloud")
	}
	if cfg.ProviderID != "" {
		t.Fatalf("ProviderID = %q, want empty so official transport bypasses provider protocol lookup", cfg.ProviderID)
	}
	if cfg.BaseURL != selection.APIBaseURL {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, selection.APIBaseURL)
	}
	if cfg.AuthHook == nil {
		t.Fatal("AuthHook = nil, want HMAC auth hook for official transport")
	}
	if !cfg.GuardOutboundURL {
		t.Fatal("GuardOutboundURL = false, want official transport guarded")
	}
	if !cfg.AllowPrivateBaseURL {
		t.Fatal("AllowPrivateBaseURL = false, want official transport to allow console-api")
	}
}

func TestCreateAdapterConfig_OfficialImageRouteUsesZGICloudTransport(t *testing.T) {
	setGatewayTestConfig(t)

	svc := &llmGatewayServiceImpl{}
	cfg := svc.createAdapterConfig(&ProviderSelection{
		Provider: providermodel.LLMProvider{
			ID:       uuid.New(),
			Provider: "qwen",
		},
		UseSystemProvider: true,
		ChannelProvider:   "zgi-cloud",
		APIBaseURL:        "http://console.internal/v1/internal",
		Model: llmmodel.LLMModel{
			ID:              uuid.New(),
			Model:           "qwen-image-2.0",
			ModelName:       "qwen-image-2.0",
			Provider:        "qwen",
			UseCases:        llmmodel.StringArray{"image-gen"},
			ImageGeneration: true,
		},
	}, uuid.New())

	if cfg.ProviderName != "zgi-cloud" {
		t.Fatalf("ProviderName = %q, want %q for official image transport", cfg.ProviderName, "zgi-cloud")
	}
	if cfg.BaseURL != "http://console.internal/v1/internal" {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, "http://console.internal/v1/internal")
	}
	if cfg.AuthHook == nil {
		t.Fatal("AuthHook = nil, want HMAC auth hook for official image transport")
	}
	if !cfg.GuardOutboundURL || !cfg.AllowPrivateBaseURL {
		t.Fatal("official image transport should be guarded and allow console-api")
	}
}

func TestCreateAdapterConfig_OfficialRouteCanCreateAdapterWithoutBearerAPIKey(t *testing.T) {
	setGatewayTestConfig(t)

	svc := &llmGatewayServiceImpl{}
	cfg := svc.createAdapterConfig(&ProviderSelection{
		Provider: providermodel.LLMProvider{
			ID:       uuid.New(),
			Provider: "cohere",
		},
		UseSystemProvider: true,
		ChannelProvider:   "zgi-cloud",
		APIBaseURL:        "http://console.internal/v1/internal",
	}, uuid.New())

	if _, err := adapter.GlobalFactory.CreateAdapter(cfg); err != nil {
		t.Fatalf("CreateAdapter err = %v, want nil", err)
	}
}

func setGatewayTestConfig(t *testing.T) {
	t.Helper()
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Console: config.ConsoleConfig{InternalAPIKey: "test-internal-key"},
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
	})
}

func TestCreateAdapterConfig_PrivateImageRouteDoesNotInferAdapterFromModelName(t *testing.T) {
	svc := &llmGatewayServiceImpl{}
	cfg := svc.createAdapterConfig(&ProviderSelection{
		Provider: providermodel.LLMProvider{
			ID:       uuid.New(),
			Provider: "openai",
		},
		UseSystemProvider: false,
		ChannelProvider:   "agicto",
		APIBaseURL:        "https://api.agicto.cn/v1",
		Model: llmmodel.LLMModel{
			ID:              uuid.New(),
			Model:           "seedream-3.0",
			ModelName:       "seedream-3.0",
			Provider:        "doubao",
			UseCases:        llmmodel.StringArray{"image-gen"},
			ImageGeneration: true,
		},
	}, uuid.New())

	if cfg.ProviderName != "agicto" {
		t.Fatalf("ProviderName = %q, want adapter resolved from channel_provider only", cfg.ProviderName)
	}
	if !cfg.GuardOutboundURL {
		t.Fatal("GuardOutboundURL = false, want private transport guarded")
	}
	if cfg.AllowPrivateBaseURL {
		t.Fatal("AllowPrivateBaseURL = true, want private transport public-only by default")
	}
}
