package channelprovider

import "testing"

func TestResolve_ZGICloudUsesDedicatedAdapter(t *testing.T) {
	spec, err := Resolve("zgi-cloud")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if spec.Name != "zgi-cloud" {
		t.Fatalf("spec.Name = %q, want %q", spec.Name, "zgi-cloud")
	}
	if spec.AdapterKey != "zgi-cloud" {
		t.Fatalf("spec.AdapterKey = %q, want %q", spec.AdapterKey, "zgi-cloud")
	}
	if !spec.RequiresBaseURL {
		t.Fatalf("spec.RequiresBaseURL = false, want true")
	}
}

func TestResolve_DoubaoUsesDedicatedAdapter(t *testing.T) {
	spec, err := Resolve("doubao")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if spec.Name != "doubao" {
		t.Fatalf("spec.Name = %q, want %q", spec.Name, "doubao")
	}
	if spec.AdapterKey != "doubao" {
		t.Fatalf("spec.AdapterKey = %q, want %q", spec.AdapterKey, "doubao")
	}
	if spec.LookupProvider != "doubao" {
		t.Fatalf("spec.LookupProvider = %q, want %q", spec.LookupProvider, "doubao")
	}
}

func TestResolve_OpenAICompatibleUsesOpenAIAdapter(t *testing.T) {
	spec, err := Resolve("openai-compatible")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if spec.Name != "openai-compatible" {
		t.Fatalf("spec.Name = %q, want %q", spec.Name, "openai-compatible")
	}
	if spec.AdapterKey != "openai-compatible" {
		t.Fatalf("spec.AdapterKey = %q, want %q", spec.AdapterKey, "openai-compatible")
	}
	if spec.LookupProvider != "openai" {
		t.Fatalf("spec.LookupProvider = %q, want %q", spec.LookupProvider, "openai")
	}
	if !spec.RequiresBaseURL {
		t.Fatalf("spec.RequiresBaseURL = false, want true")
	}
}

func TestResolve_AgictoUsesDedicatedAdapter(t *testing.T) {
	spec, err := Resolve("agicto")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if spec.Name != "agicto" {
		t.Fatalf("spec.Name = %q, want %q", spec.Name, "agicto")
	}
	if spec.AdapterKey != "agicto" {
		t.Fatalf("spec.AdapterKey = %q, want %q", spec.AdapterKey, "agicto")
	}
}

func TestResolve_OllamaUsesDedicatedPrivateAdapter(t *testing.T) {
	spec, err := Resolve("ollama")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if spec.Name != "ollama" {
		t.Fatalf("spec.Name = %q, want %q", spec.Name, "ollama")
	}
	if spec.AdapterKey != "ollama" {
		t.Fatalf("spec.AdapterKey = %q, want %q", spec.AdapterKey, "ollama")
	}
	if spec.LookupProvider != "ollama" {
		t.Fatalf("spec.LookupProvider = %q, want %q", spec.LookupProvider, "ollama")
	}
	if !spec.RequiresBaseURL {
		t.Fatalf("spec.RequiresBaseURL = false, want true")
	}
	if !spec.AllowsEmptyKey {
		t.Fatalf("spec.AllowsEmptyKey = false, want true")
	}
}

func TestValidateAPIKey_ProviderSpecificRules(t *testing.T) {
	ollama, err := Resolve("ollama")
	if err != nil {
		t.Fatalf("Resolve(ollama) error = %v", err)
	}
	if err := ValidateAPIKey(ollama, ""); err != nil {
		t.Fatalf("ValidateAPIKey(ollama, empty) error = %v, want nil", err)
	}

	openAICompatible, err := Resolve("openai-compatible")
	if err != nil {
		t.Fatalf("Resolve(openai-compatible) error = %v", err)
	}
	if err := ValidateAPIKey(openAICompatible, ""); err == nil {
		t.Fatalf("ValidateAPIKey(openai-compatible, empty) error = nil, want error")
	}
}

func TestNativeCapabilities(t *testing.T) {
	tests := []struct {
		name                  string
		provider              string
		wantOpenAIResponses   bool
		wantAnthropicMessages bool
	}{
		{
			name:                "openai supports responses",
			provider:            "openai",
			wantOpenAIResponses: true,
		},
		{
			name:                  "openai compatible requires explicit native protocol config",
			provider:              "openai-compatible",
			wantOpenAIResponses:   true,
			wantAnthropicMessages: true,
		},
		{
			name:                  "agicto supports responses and anthropic messages",
			provider:              "agicto",
			wantOpenAIResponses:   true,
			wantAnthropicMessages: true,
		},
		{
			name:                  "claude supports anthropic messages",
			provider:              "claude",
			wantAnthropicMessages: true,
		},
		{
			name:                  "anthropic alias supports anthropic messages",
			provider:              "anthropic",
			wantAnthropicMessages: true,
		},
		{
			name:                  "zgi cloud supports both native protocols",
			provider:              "zgi-cloud",
			wantOpenAIResponses:   true,
			wantAnthropicMessages: true,
		},
		{
			name:                  "qwen supports responses and anthropic messages",
			provider:              "qwen",
			wantOpenAIResponses:   true,
			wantAnthropicMessages: true,
		},
		{
			name:                  "deepseek supports anthropic messages only",
			provider:              "deepseek",
			wantAnthropicMessages: true,
		},
		{
			name:                  "siliconflow supports anthropic messages only",
			provider:              "siliconflow",
			wantAnthropicMessages: true,
		},
		{
			name:                  "minimax alias supports anthropic messages",
			provider:              "minmax",
			wantAnthropicMessages: true,
		},
		{
			name:                  "openrouter supports both native protocols",
			provider:              "openrouter",
			wantOpenAIResponses:   true,
			wantAnthropicMessages: true,
		},
		{
			name:                  "glm supports anthropic messages only",
			provider:              "glm",
			wantAnthropicMessages: true,
		},
		{
			name:                  "zhipu alias supports anthropic messages only",
			provider:              "zhipu",
			wantAnthropicMessages: true,
		},
		{
			name:                  "bigmodel alias supports anthropic messages only",
			provider:              "bigmodel",
			wantAnthropicMessages: true,
		},
		{
			name:                  "z.ai alias supports anthropic messages only",
			provider:              "z.ai",
			wantAnthropicMessages: true,
		},
		{
			name:                  "zai alias supports anthropic messages only",
			provider:              "zai",
			wantAnthropicMessages: true,
		},
		{
			name:                "doubao supports responses only",
			provider:            "doubao",
			wantOpenAIResponses: true,
		},
		{
			name:                "volcengine supports responses only",
			provider:            "volcengine",
			wantOpenAIResponses: true,
		},
		{
			name:                "ark alias supports responses only",
			provider:            "ark",
			wantOpenAIResponses: true,
		},
		{
			name:                  "kimi alias supports anthropic messages only",
			provider:              "kimi",
			wantAnthropicMessages: true,
		},
		{
			name:                  "ollama supports both native protocols",
			provider:              "ollama",
			wantOpenAIResponses:   true,
			wantAnthropicMessages: true,
		},
		{
			name:     "unknown provider supports nothing",
			provider: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportsOpenAIResponses(tt.provider); got != tt.wantOpenAIResponses {
				t.Fatalf("SupportsOpenAIResponses(%q) = %v, want %v", tt.provider, got, tt.wantOpenAIResponses)
			}
			if got := SupportsAnthropicMessages(tt.provider); got != tt.wantAnthropicMessages {
				t.Fatalf("SupportsAnthropicMessages(%q) = %v, want %v", tt.provider, got, tt.wantAnthropicMessages)
			}
		})
	}
}

func TestNativeCapabilities_OpenAICompatibleRequiresExplicitConfig(t *testing.T) {
	got := OpenAIResponsesCapability("openai-compatible")
	if !got.Supported {
		t.Fatal("OpenAIResponsesCapability(openai-compatible).Supported = false, want true")
	}
	if !got.RequiresExplicitConfig {
		t.Fatal("OpenAIResponsesCapability(openai-compatible).RequiresExplicitConfig = false, want true")
	}

	got = AnthropicMessagesCapability("openai-compatible")
	if !got.Supported {
		t.Fatal("AnthropicMessagesCapability(openai-compatible).Supported = false, want true")
	}
	if !got.RequiresExplicitConfig {
		t.Fatal("AnthropicMessagesCapability(openai-compatible).RequiresExplicitConfig = false, want true")
	}
}
