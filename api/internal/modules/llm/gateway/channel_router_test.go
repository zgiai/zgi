package gateway

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/config"
	channelmodel "github.com/zgiai/ginext/internal/modules/llm/channel/model"
	credentialmodel "github.com/zgiai/ginext/internal/modules/llm/credential/model"
	llmmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/ginext/internal/modules/llm/shared"
)

type stubCryptoService struct{}

func (stubCryptoService) Encrypt(plaintext string) (string, error) {
	return plaintext, nil
}

func (stubCryptoService) Decrypt(ciphertext string) (string, error) {
	return "test-api-key", nil
}

func TestBuildChannelSelection_UsesModelListAsSourceOfTruth(t *testing.T) {
	router := &ChannelRouter{}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "deepseek",
		APIBaseURL:      "https://api.agicto.cn/v1",
		Models:          []string{"gpt-4o", "deepseek-v3"},
		IsEnabled:       true,
	}
	model := &llmmodel.LLMModel{
		ID:        uuid.New(),
		Model:     "gpt-4o",
		ModelName: "gpt-4o",
		Provider:  "openai",
		IsActive:  true,
	}

	selection, err := router.buildChannelSelection(context.Background(), route, model, nil, "gpt-4o", false, "chat")
	if err != nil {
		t.Fatalf("buildChannelSelection returned error: %v", err)
	}
	if selection == nil {
		t.Fatal("buildChannelSelection returned nil selection")
	}
	if selection.ModelName != "gpt-4o" {
		t.Fatalf("selection.ModelName = %q, want %q", selection.ModelName, "gpt-4o")
	}
	if selection.ChannelProvider != "deepseek" {
		t.Fatalf("selection.ChannelProvider = %q, want %q", selection.ChannelProvider, "deepseek")
	}
	if selection.ModelSource != channelModelSourceGlobal {
		t.Fatalf("selection.ModelSource = %q, want %q", selection.ModelSource, channelModelSourceGlobal)
	}
}

func TestBuildChannelSelection_UsesRouteChannelProviderForPrivateRoutes(t *testing.T) {
	router := &ChannelRouter{
		cryptoService: stubCryptoService{},
	}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "agicto",
		APIBaseURL:      "https://api.agicto.cn/v1",
		Models:          []string{"qwen2.5-14b-instruct"},
		IsEnabled:       true,
		TenantCredential: &credentialmodel.TenantCredential{
			ChannelProvider:  "openai",
			APIKeyCiphertext: "ciphertext",
			APIBaseURL:       "https://api.agicto.cn/v1",
			IsActive:         true,
		},
	}
	model := &llmmodel.LLMModel{
		ID:        uuid.New(),
		Model:     "qwen2.5-14b-instruct",
		ModelName: "Qwen2.5-14B-Instruct",
		Provider:  "qwen",
		IsActive:  true,
	}

	selection, err := router.buildChannelSelection(context.Background(), route, model, nil, model.Model, false, "chat")
	if err != nil {
		t.Fatalf("buildChannelSelection returned error: %v", err)
	}
	if selection == nil {
		t.Fatal("buildChannelSelection returned nil selection")
	}
	if selection.ChannelProvider != "agicto" {
		t.Fatalf("selection.ChannelProvider = %q, want %q", selection.ChannelProvider, "agicto")
	}
	if selection.APIKey != "test-api-key" {
		t.Fatalf("selection.APIKey = %q, want decrypted API key", selection.APIKey)
	}
	if selection.ModelSource != channelModelSourceGlobal {
		t.Fatalf("selection.ModelSource = %q, want %q", selection.ModelSource, channelModelSourceGlobal)
	}
}

func TestBuildChannelSelection_PassthroughModelSource(t *testing.T) {
	router := &ChannelRouter{}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "openai-compatible",
		APIBaseURL:      "https://example.com/v1",
		Models:          []string{"custom-model"},
		IsEnabled:       true,
	}

	selection, err := router.buildChannelSelection(context.Background(), route, nil, nil, "custom-model", true, "chat")
	if err != nil {
		t.Fatalf("buildChannelSelection returned error: %v", err)
	}
	if selection.ModelSource != channelModelSourcePassthrough {
		t.Fatalf("selection.ModelSource = %q, want %q", selection.ModelSource, channelModelSourcePassthrough)
	}
}

func TestBuildChannelSelection_OfficialRouteUsesRuntimeConsoleAPIURL(t *testing.T) {
	setGatewayConsoleAPIURL(t, "https://console-api.zgi.im")

	router := &ChannelRouter{}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		Type:            shared.RouteTypeZGICloud,
		ChannelProvider: "zgi-cloud",
		APIBaseURL:      "http://zgi-console-api.zeabur.internal:2625/v1/internal",
		Models:          []string{"gpt-5"},
		IsEnabled:       true,
		IsOfficial:      true,
	}
	model := &llmmodel.LLMModel{
		ID:        uuid.New(),
		Model:     "gpt-5",
		ModelName: "gpt-5",
		Provider:  "openai",
		IsActive:  true,
	}

	selection, err := router.buildChannelSelection(context.Background(), route, model, nil, model.Model, false, "chat")
	if err != nil {
		t.Fatalf("buildChannelSelection returned error: %v", err)
	}
	if selection.APIBaseURL != "https://console-api.zgi.im/v1/internal" {
		t.Fatalf("selection.APIBaseURL = %q, want runtime console URL", selection.APIBaseURL)
	}
	if selection.BillingLane != UsageBillingLanePlatform || !selection.UseSystemProvider {
		t.Fatalf("selection lane = %s use_system_provider=%t, want platform/true", selection.BillingLane, selection.UseSystemProvider)
	}
	if route.APIBaseURL != "http://zgi-console-api.zeabur.internal:2625/v1/internal" {
		t.Fatalf("route.APIBaseURL mutated to %q", route.APIBaseURL)
	}
}

func TestBuildChannelSelection_OfficialRouteRequiresRuntimeConsoleAPIURL(t *testing.T) {
	setGatewayConsoleAPIURL(t, "")

	router := &ChannelRouter{}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		Type:            shared.RouteTypeZGICloud,
		ChannelProvider: "zgi-cloud",
		APIBaseURL:      "http://zgi-console-api.zeabur.internal:2625/v1/internal",
		Models:          []string{"gpt-5"},
		IsEnabled:       true,
		IsOfficial:      true,
	}
	model := &llmmodel.LLMModel{
		ID:        uuid.New(),
		Model:     "gpt-5",
		ModelName: "gpt-5",
		Provider:  "openai",
		IsActive:  true,
	}

	_, err := router.buildChannelSelection(context.Background(), route, model, nil, model.Model, false, "chat")
	if err == nil || !strings.Contains(err.Error(), "console api url") {
		t.Fatalf("buildChannelSelection error = %v, want console api url error", err)
	}
}

func TestBuildChannelSelection_OfficialRouteRejectsInvalidRuntimeConsoleAPIURL(t *testing.T) {
	setGatewayConsoleAPIURL(t, "console-api.zgi.im")

	router := &ChannelRouter{}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		Type:            shared.RouteTypeZGICloud,
		ChannelProvider: "zgi-cloud",
		APIBaseURL:      "http://zgi-console-api.zeabur.internal:2625/v1/internal",
		Models:          []string{"gpt-5"},
		IsEnabled:       true,
		IsOfficial:      true,
	}
	model := &llmmodel.LLMModel{
		ID:        uuid.New(),
		Model:     "gpt-5",
		ModelName: "gpt-5",
		Provider:  "openai",
		IsActive:  true,
	}

	_, err := router.buildChannelSelection(context.Background(), route, model, nil, model.Model, false, "chat")
	if err == nil || !strings.Contains(err.Error(), "invalid console api url") {
		t.Fatalf("buildChannelSelection error = %v, want invalid console api url error", err)
	}
}

func TestBuildChannelSelection_PrivateRouteKeepsRouteAPIBaseURL(t *testing.T) {
	setGatewayConsoleAPIURL(t, "https://console-api.zgi.im")

	router := &ChannelRouter{}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "openai-compatible",
		APIBaseURL:      "https://proxy.example.com/v1",
		Models:          []string{"gpt-4o"},
		IsEnabled:       true,
	}
	model := &llmmodel.LLMModel{
		ID:        uuid.New(),
		Model:     "gpt-4o",
		ModelName: "gpt-4o",
		Provider:  "openai",
		IsActive:  true,
	}

	selection, err := router.buildChannelSelection(context.Background(), route, model, nil, model.Model, false, "chat")
	if err != nil {
		t.Fatalf("buildChannelSelection returned error: %v", err)
	}
	if selection.APIBaseURL != "https://proxy.example.com/v1" {
		t.Fatalf("selection.APIBaseURL = %q, want private route URL", selection.APIBaseURL)
	}
	if selection.BillingLane != UsageBillingLanePrivate || selection.UseSystemProvider {
		t.Fatalf("selection lane = %s use_system_provider=%t, want private/false", selection.BillingLane, selection.UseSystemProvider)
	}
}

func TestFilterRoutesForNativeProtocol_ResponsesSkipsChatOnlyRoutes(t *testing.T) {
	routes := []*channelmodel.LLMRoute{
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "openai-compatible",
			NativeProtocols: channelmodel.NativeProtocolConfig{
				OpenAIResponses: channelmodel.NativeProtocolEndpoint{Enabled: true},
			},
		},
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "deepseek",
		},
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypeZGICloud,
			ChannelProvider: "zgi-cloud",
			IsOfficial:      true,
		},
	}
	model := &llmmodel.LLMModel{
		Model:     "gpt-4.1-mini",
		Provider:  "openai",
		Responses: true,
	}

	filtered := filterRoutesForNativeProtocol(routes, model, modelCategoryResponses)
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
	if filtered[0].ChannelProvider != "openai-compatible" || filtered[1].ChannelProvider != "zgi-cloud" {
		t.Fatalf("filtered providers = [%q, %q], want [openai-compatible, zgi-cloud]", filtered[0].ChannelProvider, filtered[1].ChannelProvider)
	}
}

func TestFilterRoutesForNativeProtocol_OpenAICompatibleRequiresExplicitResponsesConfig(t *testing.T) {
	routes := []*channelmodel.LLMRoute{
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "openai-compatible",
		},
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "openai-compatible",
			NativeProtocols: channelmodel.NativeProtocolConfig{
				OpenAIResponses: channelmodel.NativeProtocolEndpoint{Enabled: true},
			},
		},
	}
	model := &llmmodel.LLMModel{
		Model:     "custom-model",
		Provider:  "openai",
		Responses: true,
	}

	filtered := filterRoutesForNativeProtocol(routes, model, modelCategoryResponses)
	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}
	if filtered[0].ID != routes[1].ID {
		t.Fatalf("filtered route = %s, want explicitly configured route %s", filtered[0].ID, routes[1].ID)
	}
}

func TestFilterRoutesForNativeProtocol_ResponsesRequiresModelCapabilityForKnownModels(t *testing.T) {
	routes := []*channelmodel.LLMRoute{
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "openai-compatible",
			NativeProtocols: channelmodel.NativeProtocolConfig{
				OpenAIResponses: channelmodel.NativeProtocolEndpoint{Enabled: true},
			},
		},
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypeZGICloud,
			ChannelProvider: "zgi-cloud",
			IsOfficial:      true,
		},
	}
	model := &llmmodel.LLMModel{
		Model:     "chat-only-model",
		Provider:  "openai",
		Responses: false,
	}

	filtered := filterRoutesForNativeProtocol(routes, model, modelCategoryResponses)
	if len(filtered) != 0 {
		t.Fatalf("len(filtered) = %d, want 0", len(filtered))
	}
}

func TestFilterRoutesForNativeProtocol_AnthropicSkipsOpenAIOnlyRoutes(t *testing.T) {
	routes := []*channelmodel.LLMRoute{
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "openai-compatible",
		},
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "agicto",
		},
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "claude",
		},
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypeZGICloud,
			ChannelProvider: "zgi-cloud",
			IsOfficial:      true,
		},
	}
	model := &llmmodel.LLMModel{
		Model:    "claude-sonnet-4-0",
		Provider: "anthropic",
	}

	filtered := filterRoutesForNativeProtocol(routes, model, modelCategoryAnthropicMessages)
	if len(filtered) != 3 {
		t.Fatalf("len(filtered) = %d, want 3", len(filtered))
	}
	if filtered[0].ChannelProvider != "agicto" || filtered[1].ChannelProvider != "claude" || filtered[2].ChannelProvider != "zgi-cloud" {
		t.Fatalf("filtered providers = [%q, %q, %q], want [agicto, claude, zgi-cloud]", filtered[0].ChannelProvider, filtered[1].ChannelProvider, filtered[2].ChannelProvider)
	}
}

func TestBuildChannelSelection_NativeProtocolBaseURLOverridesPrivateRouteBaseURL(t *testing.T) {
	router := &ChannelRouter{}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "qwen",
		APIBaseURL:      "https://dashscope.aliyuncs.com/compatible-mode/v1",
		NativeProtocols: channelmodel.NativeProtocolConfig{
			OpenAIResponses: channelmodel.NativeProtocolEndpoint{
				Enabled: true,
				BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1/",
			},
			AnthropicMessages: channelmodel.NativeProtocolEndpoint{
				Enabled: true,
				BaseURL: "https://dashscope.aliyuncs.com/api/v2/apps/anthropic",
			},
		},
		Models:    []string{"qwen3-coder"},
		IsEnabled: true,
	}
	model := &llmmodel.LLMModel{
		ID:        uuid.New(),
		Model:     "qwen3-coder",
		ModelName: "qwen3-coder",
		Provider:  "qwen",
		Responses: true,
		IsActive:  true,
	}

	selection, err := router.buildChannelSelection(context.Background(), route, model, nil, model.Model, false, modelCategoryResponses)
	if err != nil {
		t.Fatalf("buildChannelSelection returned error: %v", err)
	}
	if selection.APIBaseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("selection.APIBaseURL = %q, want responses base URL", selection.APIBaseURL)
	}
}

func setGatewayConsoleAPIURL(t *testing.T, apiURL string) {
	t.Helper()
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Console: config.ConsoleConfig{APIURL: apiURL},
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
	})
}
