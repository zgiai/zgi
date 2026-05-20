package llm

import shared_model "github.com/zgiai/ginext/internal/modules/shared/model"

// ModelType identifies the model family used by the workflow node.
type ModelType string

const (
	ModelTypeLLM           ModelType = "llm"
	ModelTypeTextEmbedding ModelType = "text-embedding"
	ModelTypeRerank        ModelType = "rerank"
	ModelTypeSpeech2Text   ModelType = "speech2text"
	ModelTypeModeration    ModelType = "moderation"
	ModelTypeTTS           ModelType = "tts"
)

// ProviderType identifies where a provider configuration comes from.
type ProviderType string

const (
	ProviderTypeSystem ProviderType = "system"
	ProviderTypeCustom ProviderType = "custom"
)

// Provider describes a model provider.
type Provider struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
}

// ModelSchema describes the model capability metadata used by the node.
type ModelSchema struct {
	ModelType ModelType `json:"model_type"`
	Provider  Provider  `json:"provider"`
	ModelName string    `json:"model_name,omitempty"`
	Features  []string  `json:"features,omitempty"`
}

// ProviderConfiguration describes the provider context for a model bundle.
type ProviderConfiguration struct {
	TenantID            string
	Provider            *Provider
	UsingProviderType   ProviderType
	CustomConfiguration *shared_model.CustomConfiguration
}

// ProviderModelBundle groups the provider configuration with the model type.
type ProviderModelBundle struct {
	Configuration     *ProviderConfiguration
	ModelType         ModelType
	ModelTypeInstance any
}

// ModelInstance describes a concrete model selection for runtime use.
type ModelInstance struct {
	ProviderModelBundle *ProviderModelBundle
	Model               string
	Provider            string
	Credentials         map[string]any
}
