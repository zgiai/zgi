// Package types contains shared types and interfaces for the gateway package.
// This package breaks circular dependencies by providing common types that
// can be imported by all gateway subpackages.
package types

import (
	"context"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
)

// =============================================================================
// Core Service Interface
// =============================================================================

// GatewayService defines the interface for LLM gateway operations.
// This interface is used by handlers and other components to interact with the gateway.
type GatewayService interface {
	// ChatCompletion handles chat completion requests
	ChatCompletion(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.ChatRequest) (*adapter.ChatResponse, error)

	// ChatCompletionStream handles streaming chat completion requests
	ChatCompletionStream(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error)

	// CreateResponse handles response creation requests
	CreateResponse(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error)

	// CreateResponseRaw handles native OpenAI Responses requests
	CreateResponseRaw(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.RawResponseRequest) (*adapter.RawResponse, error)

	// CreateResponseStream handles native OpenAI Responses stream requests
	CreateResponseStream(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.RawResponseRequest) (<-chan adapter.RawStreamEvent, error)

	// CreateAnthropicMessage handles native Anthropic Messages requests
	CreateAnthropicMessage(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.AnthropicMessageRequest) (*adapter.RawResponse, error)

	// CreateAnthropicMessageStream handles native Anthropic Messages stream requests
	CreateAnthropicMessageStream(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.AnthropicMessageRequest) (<-chan adapter.RawStreamEvent, error)

	// CreateEmbeddings handles embeddings creation requests
	CreateEmbeddings(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error)

	// CreateImage handles image generation requests
	CreateImage(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.ImageRequest) (*adapter.ImageResponse, error)

	// ListAvailableModels lists available models for the API key
	ListAvailableModels(ctx context.Context, apiKey *apikeymodel.TenantAPIKey) ([]adapter.Model, error)
}

// ErrorCode represents HTTP error codes for LLM gateway
type ErrorCode struct {
	Code    int
	Message string
}

// Common error codes
var (
	ErrCodeInvalidAPIKey     = ErrorCode{Code: 40101, Message: "Invalid API key"}
	ErrCodeAPIKeyInactive    = ErrorCode{Code: 40103, Message: "API key is inactive"}
	ErrCodeInsufficientQuota = ErrorCode{Code: 114009, Message: "Insufficient API key quota"}
	ErrCodeInvalidRequest    = ErrorCode{Code: 40001, Message: "Invalid request"}
)

// =============================================================================
// Config Cache Interface
// =============================================================================

// ShadowTenantInfo contains shadow tenant information
type ShadowTenantInfo struct {
	ShadowOrganizationID uuid.UUID `json:"shadow_tenant_id"`
	OwnerID              uuid.UUID `json:"owner_id"`
}

// ConfigCache defines the interface for configuration caching
type ConfigCache interface {
	// GetModelByName retrieves model by name, using cache first
	GetModelByName(ctx context.Context, name string) (*llmmodel.LLMModel, error)
	// GetModelByID retrieves model by ID, using cache first
	GetModelByID(ctx context.Context, id uuid.UUID) (*llmmodel.LLMModel, error)
	// GetProviderByName retrieves provider by name, using cache first
	GetProviderByName(ctx context.Context, name string) (*providermodel.LLMProvider, error)
	// GetShadowTenantInfo retrieves shadow tenant info, using cache first
	GetShadowTenantInfo(ctx context.Context, organizationID uuid.UUID) (*ShadowTenantInfo, error)
	// InvalidateModel invalidates model cache
	InvalidateModel(ctx context.Context, id uuid.UUID, name string)
	// InvalidateModelCache invalidates all cached model records
	InvalidateModelCache(ctx context.Context)
	// InvalidateProvider invalidates provider cache
	InvalidateProvider(ctx context.Context, name string)
}

// =============================================================================
// Health Tracker Interface
// =============================================================================

// HealthTracker defines the interface for channel health tracking
type HealthTracker interface {
	// RecordSuccess records a successful request for a channel
	RecordSuccess(channelID uuid.UUID)
	// RecordFailure records a failed request for a channel
	RecordFailure(ctx context.Context, channelID uuid.UUID, autoBanEnabled bool) error
	// StartCleanupRoutine starts the background cleanup routine
	StartCleanupRoutine(ctx context.Context)
}
