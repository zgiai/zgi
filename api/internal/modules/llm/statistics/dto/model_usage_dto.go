package dto

import "github.com/google/uuid"

type ModelUsageRequest struct {
	StartTime         int64      `form:"start_time" binding:"required"`
	EndTime           int64      `form:"end_time" binding:"required"`
	AppType           *string    `form:"app_type" binding:"omitempty,oneof=workflow dataset agent aichat unknown"`
	AppID             *uuid.UUID `form:"app_id"`
	ModelName         *string    `form:"model_name"`
	BillingLane       *string    `form:"billing_lane" binding:"omitempty,oneof=platform private"`
	UseSystemProvider *bool      `form:"use_system_provider"`
}

type ModelUsagePeriod struct {
	StartTime int64 `json:"start_time"`
	EndTime   int64 `json:"end_time"`
}

type ModelUsageSummary struct {
	AttemptCount     int64 `json:"attempt_count"`
	SuccessCount     int64 `json:"success_count"`
	FailedCount      int64 `json:"failed_count"`
	PartialCount     int64 `json:"partial_count"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	OfficialPoints   int64 `json:"official_points"`
	PrivatePoints    int64 `json:"private_points"`
	TotalPoints      int64 `json:"total_points"`
}

type ModelUsageByModelItem struct {
	ModelID          uuid.UUID `json:"model_id"`
	ModelName        string    `json:"model_name"`
	ProviderID       uuid.UUID `json:"provider_id"`
	ProviderName     string    `json:"provider_name"`
	AttemptCount     int64     `json:"attempt_count"`
	SuccessCount     int64     `json:"success_count"`
	FailedCount      int64     `json:"failed_count"`
	PartialCount     int64     `json:"partial_count"`
	PromptTokens     int64     `json:"prompt_tokens"`
	CompletionTokens int64     `json:"completion_tokens"`
	TotalTokens      int64     `json:"total_tokens"`
	OfficialPoints   int64     `json:"official_points"`
	PrivatePoints    int64     `json:"private_points"`
	TotalPoints      int64     `json:"total_points"`
	PointsShare      float64   `json:"points_share"`
}

type ModelUsageByAppTypeItem struct {
	AppType          string  `json:"app_type"`
	AttemptCount     int64   `json:"attempt_count"`
	SuccessCount     int64   `json:"success_count"`
	FailedCount      int64   `json:"failed_count"`
	PartialCount     int64   `json:"partial_count"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	OfficialPoints   int64   `json:"official_points"`
	PrivatePoints    int64   `json:"private_points"`
	TotalPoints      int64   `json:"total_points"`
	PointsShare      float64 `json:"points_share"`
}

type ModelUsageDailyItem struct {
	Date             string `json:"date"`
	AttemptCount     int64  `json:"attempt_count"`
	SuccessCount     int64  `json:"success_count"`
	FailedCount      int64  `json:"failed_count"`
	PartialCount     int64  `json:"partial_count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	OfficialPoints   int64  `json:"official_points"`
	PrivatePoints    int64  `json:"private_points"`
	TotalPoints      int64  `json:"total_points"`
}

type ModelUsageResponse struct {
	Period     ModelUsagePeriod          `json:"period"`
	Summary    ModelUsageSummary         `json:"summary"`
	ByModel    []ModelUsageByModelItem   `json:"by_model"`
	ByAppType  []ModelUsageByAppTypeItem `json:"by_app_type"`
	DailyTrend []ModelUsageDailyItem     `json:"daily_trend"`
}
