package shared

// Common types and constants for LLM module

// ProviderType defines the type of provider
type ProviderType string

const (
	ProviderTypeGlobal ProviderType = "global" // Global provider (system)
	ProviderTypeCustom ProviderType = "custom" // Custom provider (tenant)
)

// ModelType defines the type of model
type ModelType string

const (
	ModelTypeLLM       ModelType = "llm"
	ModelTypeEmbedding ModelType = "text-embedding"
	ModelTypeImage     ModelType = "image"
	ModelTypeAudio     ModelType = "audio"
	ModelTypeVideo     ModelType = "video"
	ModelTypeRerank    ModelType = "rerank"
)

// AccessScope defines who can access a resource
type AccessScope string

const (
	AccessScopeAll   AccessScope = "all"
	AccessScopeGroup AccessScope = "group"
	AccessScopeUser  AccessScope = "user"
)

// RouteType defines the type of route
type RouteType string

const (
	RouteTypeZGICloud RouteType = "ZGI_CLOUD" // ZGI official cloud service channel
	RouteTypePrivate  RouteType = "PRIVATE"   // User's private channel
)

// Context keys
const (
	ContextKeyModelCategory = "llm_model_category" // "chat", "image", "embedding", etc.
)
