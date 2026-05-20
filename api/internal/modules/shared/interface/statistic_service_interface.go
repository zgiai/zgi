package interfaces

import "context"

// StatisticService defines the interface for statistic-related operations
type StatisticService interface {
	GetDailyMessageStats(ctx context.Context, appID string, start, end interface{}, timezone string) (interface{}, error)
	GetDailyConversationStats(ctx context.Context, appID string, start, end interface{}, timezone string) (interface{}, error)
	GetDailyTerminalStats(ctx context.Context, appID string, start, end interface{}, timezone string) (interface{}, error)
	GetDailyTokenCostStats(ctx context.Context, appID string, start, end interface{}, timezone string) (interface{}, error)
	GetAverageSessionInteractionStats(ctx context.Context, appID string, start, end interface{}, timezone string) (interface{}, error)
	GetUserSatisfactionRateStats(ctx context.Context, appID string, start, end interface{}, timezone string) (interface{}, error)
	GetAverageResponseTimeStats(ctx context.Context, appID string, start, end interface{}, timezone string) (interface{}, error)
	GetTokensPerSecondStats(ctx context.Context, appID string, start, end interface{}, timezone string) (interface{}, error)
}
