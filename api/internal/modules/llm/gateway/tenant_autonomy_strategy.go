package gateway

import (
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

// TenantAutonomyStrategy implements model matching for PRIVATE routes
// These routes are fully controlled by the tenant with no fallback to system configuration
// This ensures complete tenant autonomy over their custom channels
type TenantAutonomyStrategy struct{}

// NewTenantAutonomyStrategy creates a new tenant autonomy strategy instance
func NewTenantAutonomyStrategy() *TenantAutonomyStrategy {
	return &TenantAutonomyStrategy{}
}

// GetStrategyName returns the name of this strategy for logging
func (s *TenantAutonomyStrategy) GetStrategyName() string {
	return "TenantAutonomy"
}

// GetModelList retrieves the model list for PRIVATE routes
// Returns ONLY the route's own model list with no fallback
// This enforces strict tenant control over model availability
func (s *TenantAutonomyStrategy) GetModelList(route *channelmodel.LLMRoute) []string {
	return route.GetEffectiveModels()
}

// GetProvider retrieves the channel provider for PRIVATE routes.
func (s *TenantAutonomyStrategy) GetProvider(route *channelmodel.LLMRoute) string {
	return route.ChannelProvider
}

// SupportsModel checks if a PRIVATE route supports the given model
// Implements strict validation:
// 1. Empty model list is treated as misconfiguration (not "all models")
// 2. Model must be explicitly listed or wildcard "*" present
//
// route.ChannelProvider identifies which adapter should be used, but the model list
// remains the capability boundary.
func (s *TenantAutonomyStrategy) SupportsModel(route *channelmodel.LLMRoute, modelName, modelProvider string) bool {
	if len(s.GetModelList(route)) == 0 {
		logger.Warn("private LLM route has empty model list",
			zap.String("strategy", s.GetStrategyName()),
			zap.String("route_id", route.ID.String()),
		)
		return false
	}

	supported := route.SupportsModel(modelName)
	if !supported {
		logger.Debug("private LLM route does not support model",
			zap.String("strategy", s.GetStrategyName()),
			zap.String("route_id", route.ID.String()),
			zap.String("model", modelName),
			zap.String("provider", modelProvider),
			zap.Int("route_model_count", len(s.GetModelList(route))),
		)
	}

	return supported
}
