package channelprovider

import (
	"context"
	"fmt"
	"strings"

	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/llm/internal/urlguard"
)

// Spec defines the canonical behavior for a channel_provider.
type Spec struct {
	Name               string
	AdapterKey         string
	LookupProvider     string
	RequiresBaseURL    bool
	AllowsEmptyKey     bool
	NativeCapabilities NativeCapabilities
}

// NativeCapabilities describes provider-native protocol support.
type NativeCapabilities struct {
	OpenAIResponses   NativeProtocolCapability
	AnthropicMessages NativeProtocolCapability
}

type NativeProtocolCapability struct {
	Supported              bool
	RequiresExplicitConfig bool
}

var specs = map[string]Spec{
	"zgi-cloud":         {Name: "zgi-cloud", AdapterKey: "zgi-cloud", LookupProvider: "zgi-cloud", RequiresBaseURL: true, NativeCapabilities: NativeCapabilities{OpenAIResponses: supportedNativeProtocol(), AnthropicMessages: supportedNativeProtocol()}},
	"openai":            {Name: "openai", AdapterKey: "openai", LookupProvider: "openai", NativeCapabilities: NativeCapabilities{OpenAIResponses: supportedNativeProtocol()}},
	"openai-compatible": {Name: "openai-compatible", AdapterKey: "openai-compatible", LookupProvider: "openai", RequiresBaseURL: true, NativeCapabilities: NativeCapabilities{OpenAIResponses: explicitNativeProtocol(), AnthropicMessages: explicitNativeProtocol()}},
	"ollama":            {Name: "ollama", AdapterKey: "ollama", LookupProvider: "ollama", RequiresBaseURL: true, AllowsEmptyKey: true, NativeCapabilities: NativeCapabilities{OpenAIResponses: supportedNativeProtocol(), AnthropicMessages: supportedNativeProtocol()}},
	"agicto":            {Name: "agicto", AdapterKey: "agicto", LookupProvider: "openai", NativeCapabilities: NativeCapabilities{OpenAIResponses: supportedNativeProtocol(), AnthropicMessages: supportedNativeProtocol()}},
	"glm":               {Name: "glm", AdapterKey: "glm", LookupProvider: "glm", NativeCapabilities: NativeCapabilities{AnthropicMessages: supportedNativeProtocol()}},
	"zhipu":             {Name: "glm", AdapterKey: "glm", LookupProvider: "glm", NativeCapabilities: NativeCapabilities{AnthropicMessages: supportedNativeProtocol()}},
	"z.ai":              {Name: "glm", AdapterKey: "glm", LookupProvider: "glm", NativeCapabilities: NativeCapabilities{AnthropicMessages: supportedNativeProtocol()}},
	"zai":               {Name: "glm", AdapterKey: "glm", LookupProvider: "glm", NativeCapabilities: NativeCapabilities{AnthropicMessages: supportedNativeProtocol()}},
	"bigmodel":          {Name: "glm", AdapterKey: "glm", LookupProvider: "glm", NativeCapabilities: NativeCapabilities{AnthropicMessages: supportedNativeProtocol()}},
	"minimax":           {Name: "minimax", AdapterKey: "minimax", LookupProvider: "minimax", NativeCapabilities: NativeCapabilities{AnthropicMessages: supportedNativeProtocol()}},
	"minmax":            {Name: "minimax", AdapterKey: "minimax", LookupProvider: "minimax", NativeCapabilities: NativeCapabilities{AnthropicMessages: supportedNativeProtocol()}},
	"deepseek":          {Name: "deepseek", AdapterKey: "deepseek", LookupProvider: "deepseek", NativeCapabilities: NativeCapabilities{AnthropicMessages: supportedNativeProtocol()}},
	"siliconflow":       {Name: "siliconflow", AdapterKey: "siliconflow", LookupProvider: "deepseek", NativeCapabilities: NativeCapabilities{AnthropicMessages: supportedNativeProtocol()}},
	"mistral":           {Name: "mistral", AdapterKey: "mistral", LookupProvider: "mistral"},
	"cohere":            {Name: "cohere", AdapterKey: "cohere", LookupProvider: "cohere"},
	"anthropic":         {Name: "anthropic", AdapterKey: "claude", LookupProvider: "anthropic", NativeCapabilities: NativeCapabilities{AnthropicMessages: supportedNativeProtocol()}},
	"claude":            {Name: "anthropic", AdapterKey: "claude", LookupProvider: "anthropic", NativeCapabilities: NativeCapabilities{AnthropicMessages: supportedNativeProtocol()}},
	"openrouter":        {Name: "openrouter", AdapterKey: "openrouter", LookupProvider: "openrouter", NativeCapabilities: NativeCapabilities{OpenAIResponses: supportedNativeProtocol(), AnthropicMessages: supportedNativeProtocol()}},
	"qwen":              {Name: "qwen", AdapterKey: "dashscope", LookupProvider: "qwen", NativeCapabilities: NativeCapabilities{OpenAIResponses: supportedNativeProtocol(), AnthropicMessages: supportedNativeProtocol()}},
	"alibaba":           {Name: "qwen", AdapterKey: "dashscope", LookupProvider: "qwen", NativeCapabilities: NativeCapabilities{OpenAIResponses: supportedNativeProtocol(), AnthropicMessages: supportedNativeProtocol()}},
	"dashscope":         {Name: "qwen", AdapterKey: "dashscope", LookupProvider: "qwen", NativeCapabilities: NativeCapabilities{OpenAIResponses: supportedNativeProtocol(), AnthropicMessages: supportedNativeProtocol()}},
	"aliyun":            {Name: "qwen", AdapterKey: "dashscope", LookupProvider: "qwen", NativeCapabilities: NativeCapabilities{OpenAIResponses: supportedNativeProtocol(), AnthropicMessages: supportedNativeProtocol()}},
	"moonshot":          {Name: "moonshot", AdapterKey: "moonshotai", LookupProvider: "moonshot", NativeCapabilities: NativeCapabilities{AnthropicMessages: supportedNativeProtocol()}},
	"moonshotai":        {Name: "moonshot", AdapterKey: "moonshotai", LookupProvider: "moonshot", NativeCapabilities: NativeCapabilities{AnthropicMessages: supportedNativeProtocol()}},
	"moonshotai-cn":     {Name: "moonshotai-cn", AdapterKey: "moonshotai-cn", LookupProvider: "moonshot", NativeCapabilities: NativeCapabilities{AnthropicMessages: supportedNativeProtocol()}},
	"kimi":              {Name: "moonshot", AdapterKey: "moonshotai", LookupProvider: "moonshot", NativeCapabilities: NativeCapabilities{AnthropicMessages: supportedNativeProtocol()}},
	"volcengine":        {Name: "volcengine", AdapterKey: "volcengine", LookupProvider: "doubao", NativeCapabilities: NativeCapabilities{OpenAIResponses: supportedNativeProtocol()}},
	"doubao":            {Name: "doubao", AdapterKey: "doubao", LookupProvider: "doubao", NativeCapabilities: NativeCapabilities{OpenAIResponses: supportedNativeProtocol()}},
	"ark":               {Name: "doubao", AdapterKey: "doubao", LookupProvider: "doubao", NativeCapabilities: NativeCapabilities{OpenAIResponses: supportedNativeProtocol()}},
	"google":            {Name: "google", AdapterKey: "gcp-imagen", LookupProvider: "google"},
	"gemini":            {Name: "google", AdapterKey: "gcp-imagen", LookupProvider: "google"},
	"gcp-imagen":        {Name: "google", AdapterKey: "gcp-imagen", LookupProvider: "google"},
	"midjourney":        {Name: "midjourney", AdapterKey: "midjourney", LookupProvider: "openai"},
	"mj-proxy":          {Name: "midjourney", AdapterKey: "midjourney", LookupProvider: "openai"},
}

func supportedNativeProtocol() NativeProtocolCapability {
	return NativeProtocolCapability{Supported: true}
}

func explicitNativeProtocol() NativeProtocolCapability {
	return NativeProtocolCapability{Supported: true, RequiresExplicitConfig: true}
}

// Resolve returns the canonical provider spec.
func Resolve(raw string) (Spec, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return Spec{}, fmt.Errorf("channel_provider is required")
	}

	spec, ok := specs[key]
	if !ok {
		return Spec{}, fmt.Errorf("unsupported channel_provider: %s", raw)
	}
	return spec, nil
}

// Normalize returns the canonical channel_provider value.
func Normalize(raw string) (string, error) {
	spec, err := Resolve(raw)
	if err != nil {
		return "", err
	}
	return spec.Name, nil
}

// LookupProvider resolves the provider name used by llm_providers lookup.
func LookupProvider(raw string) (string, error) {
	spec, err := Resolve(raw)
	if err != nil {
		return "", err
	}
	return spec.LookupProvider, nil
}

// ValidateConnectionSpec validates provider-specific connection invariants.
func ValidateConnectionSpec(spec Spec, apiBaseURL string) error {
	if spec.RequiresBaseURL && strings.TrimSpace(apiBaseURL) == "" {
		return fmt.Errorf("api_base_url is required for channel_provider %q", spec.Name)
	}
	if err := ValidateBaseURLForSpec(spec, "api_base_url", apiBaseURL); err != nil {
		return err
	}
	return nil
}

func ValidateBaseURLForSpec(spec Spec, fieldName, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if err := urlguard.ValidateBaseURL(context.Background(), raw, URLGuardPolicy(spec)); err != nil {
		return fmt.Errorf("invalid %s for channel_provider %q: %w", fieldName, spec.Name, err)
	}
	return nil
}

// ValidateAPIKey validates provider-specific API key requirements.
func ValidateAPIKey(spec Spec, apiKey string) error {
	if spec.AllowsEmptyKey {
		return nil
	}
	if strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("api_key is required for channel_provider %q", spec.Name)
	}
	return nil
}

// ValidateConnectionFields resolves the provider and validates provider-specific connection fields.
func ValidateConnectionFields(rawProvider, apiBaseURL string) (Spec, error) {
	spec, err := Resolve(rawProvider)
	if err != nil {
		return Spec{}, err
	}
	if err := ValidateConnectionSpec(spec, apiBaseURL); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

func URLGuardPolicy(spec Spec) urlguard.Policy {
	return urlguard.Policy{AllowPrivate: AllowsPrivateBaseURL(spec.Name)}
}

func AllowsPrivateBaseURL(rawProvider string) bool {
	spec, err := Resolve(rawProvider)
	if err != nil {
		return false
	}
	return spec.Name == "ollama" && appconfig.Current().LLM.AllowPrivateBaseURL
}

// SupportsOpenAIResponses reports whether the channel provider supports native OpenAI Responses.
func SupportsOpenAIResponses(raw string) bool {
	spec, err := Resolve(raw)
	if err != nil {
		return false
	}
	return spec.NativeCapabilities.OpenAIResponses.Supported
}

// OpenAIResponsesCapability returns the native OpenAI Responses capability for a provider.
func OpenAIResponsesCapability(raw string) NativeProtocolCapability {
	spec, err := Resolve(raw)
	if err != nil {
		return NativeProtocolCapability{}
	}
	return spec.NativeCapabilities.OpenAIResponses
}

// SupportsAnthropicMessages reports whether the channel provider supports native Anthropic Messages.
func SupportsAnthropicMessages(raw string) bool {
	spec, err := Resolve(raw)
	if err != nil {
		return false
	}
	return spec.NativeCapabilities.AnthropicMessages.Supported
}

// AnthropicMessagesCapability returns the native Anthropic Messages capability for a provider.
func AnthropicMessagesCapability(raw string) NativeProtocolCapability {
	spec, err := Resolve(raw)
	if err != nil {
		return NativeProtocolCapability{}
	}
	return spec.NativeCapabilities.AnthropicMessages
}
