package provider

import adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"

func init() {
	// Register adapters by adapter_type (not provider name)
	// This allows multiple providers to share the same adapter

	// OpenAI adapter - used by OpenAI, DeepSeek (compatible mode), Moonshot, etc.
	adapter.GlobalFactory.Register("openai", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewOpenAIAdapter(config)
	})
	adapter.GlobalFactory.Register("openai-compatible", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewOpenAIAdapter(config)
	})
	adapter.GlobalFactory.Register("agicto", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewAgictoAdapter(config)
	})
	adapter.GlobalFactory.Register("siliconflow", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewSiliconFlowAdapter(config)
	})
	adapter.GlobalFactory.Register("mistral", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewMistralAdapter(config)
	})
	adapter.GlobalFactory.Register("ollama", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewOllamaAdapter(config)
	})
	adapter.GlobalFactory.Register("zgi-cloud", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewZGICloudAdapter(config)
	})

	// DeepSeek native adapter - used by DeepSeek (native mode)
	adapter.GlobalFactory.Register("deepseek", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewDeepSeekAdapter(config)
	})

	// OpenRouter adapter
	adapter.GlobalFactory.Register("openrouter", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewOpenRouterAdapter(config)
	})

	// GLM adapter
	adapter.GlobalFactory.Register("glm", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewGLMAdapter(config)
	})

	// MiniMax adapter
	adapter.GlobalFactory.Register("minimax", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewMiniMaxAdapter(config)
	})

	// Cohere adapter
	adapter.GlobalFactory.Register("cohere", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewCohereAdapter(config)
	})

	// Moonshot AI adapter
	adapter.GlobalFactory.Register("moonshotai", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewMoonshotAIAdapter(config)
	})

	// Moonshot AI CN adapter
	adapter.GlobalFactory.Register("moonshotai-cn", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewMoonshotAICNAdapter(config)
	})

	// Claude adapter - used by Anthropic
	adapter.GlobalFactory.Register("claude", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewClaudeAdapter(config)
	})

	// Aliyun DashScope adapter
	adapter.GlobalFactory.Register("dashscope", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewAliyunAdapter(config)
	})

	// Google Vertex AI adapter
	adapter.GlobalFactory.Register("gcp-imagen", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewGoogleAdapter(config)
	})

	// Midjourney adapter
	adapter.GlobalFactory.Register("midjourney", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewMidjourneyAdapter(config)
	})
	adapter.GlobalFactory.Register("mj-proxy", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewMidjourneyAdapter(config)
	})

	// Doubao Ark adapter
	adapter.GlobalFactory.Register("doubao", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewDoubaoAdapter(config)
	})

	// Volcengine adapter
	adapter.GlobalFactory.Register("volcengine", func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return NewVolcengineAdapter(config)
	})
}
