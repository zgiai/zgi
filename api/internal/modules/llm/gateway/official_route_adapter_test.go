package gateway

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/requesttimeout"
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

	cfg := svc.createAdapterConfig(context.Background(), selection, orgID)

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
	cfg := svc.createAdapterConfig(context.Background(), &ProviderSelection{
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
	cfg := svc.createAdapterConfig(context.Background(), &ProviderSelection{
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
	cfg := svc.createAdapterConfig(context.Background(), &ProviderSelection{
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

func TestCreateAdapterConfig_QwenImageOpenAIProtocolUsesCompatibleAdapter(t *testing.T) {
	svc := &llmGatewayServiceImpl{}
	cfg := svc.createAdapterConfig(context.Background(), &ProviderSelection{
		Provider: providermodel.LLMProvider{
			ID:       uuid.New(),
			Provider: "qwen",
		},
		UseSystemProvider:       false,
		ChannelProvider:         "qwen",
		AdapterProviderOverride: "openai-compatible",
		APIBaseURL:              "https://api.agicto.cn/v1",
		NativeProtocols: channelmodel.NativeProtocolConfig{
			OpenAIResponses: channelmodel.NativeProtocolEndpoint{Enabled: true},
		},
		Model: llmmodel.LLMModel{
			ID:              uuid.New(),
			Model:           "qwen-image-2.0",
			ModelName:       "qwen-image-2.0",
			Provider:        "qwen",
			UseCases:        llmmodel.StringArray{"image-gen"},
			ImageGeneration: true,
		},
	}, uuid.New())

	if cfg.ProviderName != "openai-compatible" {
		t.Fatalf("ProviderName = %q, want openai-compatible", cfg.ProviderName)
	}
	if cfg.BaseURL != "https://api.agicto.cn/v1" {
		t.Fatalf("BaseURL = %q, want AGICTO base URL", cfg.BaseURL)
	}
	if cfg.ProviderID == "" {
		t.Fatal("ProviderID is empty, want qwen model provider retained for billing/config context")
	}
}

func TestAdapterProviderOverrideForRoute_QwenImageOpenAIProtocolOnly(t *testing.T) {
	route := &channelmodel.LLMRoute{
		ChannelProvider: "qwen",
		NativeProtocols: channelmodel.NativeProtocolConfig{
			OpenAIResponses: channelmodel.NativeProtocolEndpoint{Enabled: true},
		},
	}
	if got := adapterProviderOverrideForRoute(route, modelCategoryImage); got != "openai-compatible" {
		t.Fatalf("adapterProviderOverrideForRoute() = %q, want openai-compatible", got)
	}
	if got := adapterProviderOverrideForRoute(route, modelCategoryResponses); got != "" {
		t.Fatalf("responses override = %q, want empty", got)
	}
	route.NativeProtocols.OpenAIResponses.Enabled = false
	if got := adapterProviderOverrideForRoute(route, modelCategoryImage); got != "" {
		t.Fatalf("native qwen override = %q, want empty", got)
	}
}

func TestCreateAdapterConfig_OfficialRouteUsesCloudDefaultTimeout(t *testing.T) {
	setGatewayTestConfig(t)
	svc := &llmGatewayServiceImpl{}
	cfg := svc.createAdapterConfig(context.Background(), &ProviderSelection{
		UseSystemProvider: true,
		ChannelProvider:   "zgi-cloud",
		APIBaseURL:        "http://console.internal/v1/internal",
		ParamOverride: map[string]interface{}{
			"upstream_timeout_seconds": json.Number("15"),
		},
	}, uuid.New())
	// The pre-side override is ignored: Console's channel override governs the
	// provider call. The hop to Console follows the request default plus margin.
	if want := requesttimeout.Default() + officialAdapterMargin; cfg.Timeout != want {
		t.Fatalf("official timeout = %s, want %s", cfg.Timeout, want)
	}
}

func TestCreateAdapterConfig_OfficialRouteFollowsRequestTimeout(t *testing.T) {
	setGatewayTestConfig(t)
	svc := &llmGatewayServiceImpl{}
	ctx := requesttimeout.WithTimeout(context.Background(), 2400*time.Second)
	cfg := svc.createAdapterConfig(ctx, &ProviderSelection{
		UseSystemProvider: true,
		ChannelProvider:   "zgi-cloud",
		APIBaseURL:        "http://console.internal/v1/internal",
	}, uuid.New())
	// Above both old caps (120s official, 600s overall).
	if want := 2400*time.Second + officialAdapterMargin; cfg.Timeout != want {
		t.Fatalf("official timeout = %s, want %s", cfg.Timeout, want)
	}
}

func TestCreateAdapterConfig_PrivateRouteTimeoutPriority(t *testing.T) {
	setGatewayTestConfig(t)
	svc := &llmGatewayServiceImpl{}
	selection := func(override interface{}) *ProviderSelection {
		sel := &ProviderSelection{ChannelProvider: "openai", APIBaseURL: "https://api.openai.com/v1"}
		if override != nil {
			sel.ParamOverride = map[string]interface{}{"upstream_timeout_seconds": override}
		}
		return sel
	}
	requested := requesttimeout.WithTimeout(context.Background(), 1800*time.Second)
	cases := []struct {
		name string
		ctx  context.Context
		sel  *ProviderSelection
		want time.Duration
	}{
		{"header beats override", requested, selection(json.Number("90")), 1800 * time.Second},
		{"override without header", context.Background(), selection(json.Number("900")), 900 * time.Second},
		{"default without either", context.Background(), selection(nil), defaultPrivateAdapterTimeout},
		{"override bounded by request max", context.Background(), selection(json.Number("99999")), requesttimeout.Max()},
	}
	for _, tc := range cases {
		if got := svc.createAdapterConfig(tc.ctx, tc.sel, uuid.New()).Timeout; got != tc.want {
			t.Fatalf("%s: timeout = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestCreateAdapterConfig_PrivateRouteReadsUpstreamTimeoutOverride(t *testing.T) {
	setGatewayTestConfig(t)
	svc := &llmGatewayServiceImpl{}
	cfg := svc.createAdapterConfig(context.Background(), &ProviderSelection{
		UseSystemProvider: false,
		ChannelProvider:   "openai",
		APIBaseURL:        "https://api.openai.com/v1",
		ParamOverride: map[string]interface{}{
			"upstream_timeout_seconds": json.Number("15"),
		},
	}, uuid.New())
	if cfg.Timeout != 15*time.Second {
		t.Fatalf("private timeout = %s, want 15s", cfg.Timeout)
	}
}
