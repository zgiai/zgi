package dto

import "time"

// StatisticTimeRange represents time range for statistics
type StatisticTimeRange struct {
	Start *time.Time `json:"start"`
	End   *time.Time `json:"end"`
}

// DailyStatisticItem represents daily statistic base item
type DailyStatisticItem struct {
	Date string `json:"date"`
}

// DailyMessageStatisticItem represents daily message statistic item
type DailyMessageStatisticItem struct {
	DailyStatisticItem
	MessageCount int `json:"message_count"`
}

// DailyConversationStatisticItem represents daily conversation statistic item
type DailyConversationStatisticItem struct {
	DailyStatisticItem
	ConversationCount int `json:"conversation_count"`
}

// DailyTerminalStatisticItem represents daily terminal statistic item
type DailyTerminalStatisticItem struct {
	DailyStatisticItem
	TerminalCount int `json:"terminal_count"`
}

// DailyTokenCostStatisticItem represents daily token cost statistic item
type DailyTokenCostStatisticItem struct {
	DailyStatisticItem
	TokenCount int     `json:"token_count"`
	TotalPrice float64 `json:"total_price"`
}

// AverageSessionInteractionData represents average session interaction data
type AverageSessionInteractionData struct {
	AverageInteractions float64 `json:"average_interactions"`
}

// UserSatisfactionRateData represents user satisfaction rate data
type UserSatisfactionRateData struct {
	SatisfactionRate float64 `json:"satisfaction_rate"`
}

// AverageResponseTimeData represents average response time data
type AverageResponseTimeData struct {
	AverageResponseTime float64 `json:"average_response_time"`
}

// TokensPerSecondData represents tokens per second data
type TokensPerSecondData struct {
	TokensPerSecond float64 `json:"tokens_per_second"`
}

// Response types
type DailyMessageStatisticResponse struct {
	Data []DailyMessageStatisticItem `json:"data"`
}

type DailyConversationStatisticResponse struct {
	Data []DailyConversationStatisticItem `json:"data"`
}

type DailyTerminalStatisticResponse struct {
	Data []DailyTerminalStatisticItem `json:"data"`
}

type DailyTokenCostStatisticResponse struct {
	Data []DailyTokenCostStatisticItem `json:"data"`
}

type AverageSessionInteractionStatisticItem struct {
	DailyStatisticItem
	Interactions float64 `json:"interactions"`
}

type AverageSessionInteractionStatisticResponse struct {
	Data []AverageSessionInteractionStatisticItem `json:"data"`
}

type UserSatisfactionRateStatisticItem struct {
	DailyStatisticItem
	Rate float64 `json:"rate"`
}

type UserSatisfactionRateStatisticResponse struct {
	Data []UserSatisfactionRateStatisticItem `json:"data"`
}

type AverageResponseTimeStatisticItem struct {
	DailyStatisticItem
	LatencyMs float64 `json:"latency_ms"`
}

type AverageResponseTimeStatisticResponse struct {
	Data []AverageResponseTimeStatisticItem `json:"data"`
}

type TokensPerSecondStatisticItem struct {
	DailyStatisticItem
	TokensPerSecond float64 `json:"tokens_per_second"`
}

type TokensPerSecondStatisticResponse struct {
	Data []TokensPerSecondStatisticItem `json:"data"`
}

type StatisticRequest struct {
	TimeRange StatisticTimeRange `json:"time_range"`
	Timezone  string             `json:"timezone"`
	Start     *string            `json:"start"`
	End       *string            `json:"end"`
}
