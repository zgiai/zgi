package gateway

import (
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
)

// StrategyFactory creates the appropriate matching strategy for a route
// Note: SystemChannelFallbackStrategy has been removed - all routes now use TenantAutonomyStrategy
type StrategyFactory struct {
	tenantStrategy *TenantAutonomyStrategy
}

// NewStrategyFactory creates a new strategy factory
func NewStrategyFactory() *StrategyFactory {
	return &StrategyFactory{
		tenantStrategy: NewTenantAutonomyStrategy(),
	}
}

// GetStrategy returns the appropriate strategy for the given route
// All routes (PRIVATE, ZGI_CLOUD, OFFICIAL) now use TenantAutonomyStrategy
// Each route maintains its own model list
func (f *StrategyFactory) GetStrategy(route *channelmodel.LLMRoute) ModelMatchingStrategy {
	// All routes use tenant autonomy strategy
	return f.tenantStrategy
}
