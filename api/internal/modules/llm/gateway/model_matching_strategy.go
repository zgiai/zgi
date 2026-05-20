package gateway

import (
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
)

// ModelMatchingStrategy defines how to determine if a route supports a model
// This abstraction allows different route types to have different model resolution logic
// following the Strategy Pattern for extensibility and testability
type ModelMatchingStrategy interface {
	// SupportsModel checks if the route supports the given model and provider
	// Returns true if the route can handle requests for this model
	SupportsModel(route *channelmodel.LLMRoute, modelName, modelProvider string) bool

	// GetModelList retrieves the effective model list for the route
	// Returns the models that this route is authorized to handle
	// The implementation varies by route type (e.g., tenant snapshot vs system channel)
	GetModelList(route *channelmodel.LLMRoute) []string

	// GetProvider retrieves the provider for this route
	// Returns the provider name (e.g., "openai", "anthropic")
	GetProvider(route *channelmodel.LLMRoute) string

	// GetStrategyName returns the name of this strategy for logging and debugging
	// Used for observability and troubleshooting
	GetStrategyName() string
}
