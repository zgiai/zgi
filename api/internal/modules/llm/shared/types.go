package shared

import "context"

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
	ContextKeyModelCategory  = "llm_model_category" // "chat", "image", "embedding", etc.
	ContextKeyModelOperation = "llm_model_operation"
	ContextKeyModelUseCase   = "llm_model_use_case" // "agent", "text-chat", etc.
	ContextKeyRequestID      = "llm_request_id"
	// ContextKeyMeteredUsage carries an optional *MeteredUsageFact so a caller
	// can declare a non-token billing identity before the pre-deduct runs.
	ContextKeyMeteredUsage = "llm_metered_usage"
)

// MeteredUsageFact declares the metered billing identity of one request. It
// lets the platform billing authority resolve the price without inferring it.
// Quantity is a raw base-unit count, never a point or token amount.
type MeteredUsageFact struct {
	Operation  string
	Meter      string
	BaseUnit   string
	Quantity   int64
	Dimensions map[string]string
}

// MeteredUsageFromContext returns the declared metered identity when a caller
// attached one. The second result is false for a missing or malformed value.
func MeteredUsageFromContext(ctx context.Context) (MeteredUsageFact, bool) {
	if ctx == nil {
		return MeteredUsageFact{}, false
	}
	fact, ok := ctx.Value(ContextKeyMeteredUsage).(*MeteredUsageFact)
	if !ok || fact == nil {
		return MeteredUsageFact{}, false
	}
	return *fact, true
}
