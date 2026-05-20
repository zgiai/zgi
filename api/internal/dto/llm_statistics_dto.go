package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ============ Usage Detail DTOs ============

// UsageDetailRequest represents usage detail query request
type UsageDetailRequest struct {
	Page      int        `form:"page" binding:"omitempty,min=1"`
	Limit     int        `form:"limit" binding:"omitempty,min=1,max=100"`
	StartDate *time.Time `form:"start_date" time_format:"2006-01-02T15:04:05Z07:00"`
	EndDate   *time.Time `form:"end_date" time_format:"2006-01-02T15:04:05Z07:00"`
	ModelName *string    `form:"model_name"`
	AppID     *uuid.UUID `form:"app_id"`
	GroupID   *uuid.UUID `form:"group_id"` // Department ID (shadow tenant)
	AccountID *uuid.UUID `form:"account_id"`
}

// UsageDetailResponse represents a single usage log entry
type UsageDetailResponse struct {
	ID               uuid.UUID       `json:"id"`
	CreatedAt        time.Time       `json:"created_at"`
	AccountName      string          `json:"account_name"`
	GroupName        string          `json:"group_name"` // Department name
	AppName          string          `json:"app_name"`   // Agent or dataset name
	AppType          string          `json:"app_type"`   // 'agent' or 'dataset'
	ModelName        string          `json:"model_name"`
	PromptTokens     int             `json:"prompt_tokens"`
	CompletionTokens int             `json:"completion_tokens"`
	TotalCost        decimal.Decimal `json:"total_cost"`
	Status           string          `json:"status"`
}

// UsageDetailListResponse represents paginated usage detail list
type UsageDetailListResponse struct {
	Items      []UsageDetailResponse `json:"items"`
	Total      int64                 `json:"total"`
	Page       int                   `json:"page"`
	Limit      int                   `json:"limit"`
	TotalPages int                   `json:"total_pages"`
}

// ============ Group Statistics DTOs ============

// GroupStatisticsRequest represents group statistics query request
type GroupStatisticsRequest struct {
	StartDate *time.Time `form:"start_date" time_format:"2006-01-02T15:04:05Z07:00"`
	EndDate   *time.Time `form:"end_date" time_format:"2006-01-02T15:04:05Z07:00"`
	GroupID   *uuid.UUID `form:"group_id"`                                         // Department ID filter
	AppID     *uuid.UUID `form:"app_id"`                                           // App ID filter
	GroupBy   string     `form:"group_by" binding:"required,oneof=department app"` // 'department' or 'app'
}

// DepartmentStatisticsItem represents statistics grouped by department
type DepartmentStatisticsItem struct {
	GroupID          uuid.UUID       `json:"group_id"`
	GroupName        string          `json:"group_name"`
	TotalCost        decimal.Decimal `json:"total_cost"`
	PromptTokens     int64           `json:"prompt_tokens"`
	CompletionTokens int64           `json:"completion_tokens"`
	ActiveMembers    int             `json:"active_members"` // Number of users who called models
	TotalMembers     int             `json:"total_members"`  // Total users in department
	CallCount        int64           `json:"call_count"`     // API call count
}

// AppStatisticsItem represents statistics grouped by app
type AppStatisticsItem struct {
	AppID            uuid.UUID       `json:"app_id"`
	AppName          string          `json:"app_name"`
	AppType          string          `json:"app_type"` // 'agent' or 'dataset'
	CallCount        int64           `json:"call_count"`
	PromptTokens     int64           `json:"prompt_tokens"`
	CompletionTokens int64           `json:"completion_tokens"`
	TotalCost        decimal.Decimal `json:"total_cost"`
	GroupName        string          `json:"group_name"`       // Department name
	GroupOwnerName   string          `json:"group_owner_name"` // Department creator name
}

// GroupStatisticsResponse represents group statistics response
type GroupStatisticsResponse struct {
	Departments []DepartmentStatisticsItem `json:"departments,omitempty"`
	Apps        []AppStatisticsItem        `json:"apps,omitempty"`
}

// ============ Model Consumption DTOs ============

// ModelConsumptionRequest represents model consumption query request
type ModelConsumptionRequest struct {
	StartDate *time.Time `form:"start_date" time_format:"2006-01-02T15:04:05Z07:00"`
	EndDate   *time.Time `form:"end_date" time_format:"2006-01-02T15:04:05Z07:00"`
}

// ModelConsumptionItem represents consumption data for a single model
type ModelConsumptionItem struct {
	ModelName        string          `json:"model_name"`
	RequestCount     int64           `json:"request_count"`
	PromptTokens     int64           `json:"prompt_tokens"`
	CompletionTokens int64           `json:"completion_tokens"`
	TotalCost        decimal.Decimal `json:"total_cost"`
	UsagePercentage  float64         `json:"usage_percentage"` // Percentage of total usage
}

// DailyConsumptionItem represents daily consumption data
type DailyConsumptionItem struct {
	Date      string          `json:"date"`
	TotalCost decimal.Decimal `json:"total_cost"`
	CallCount int64           `json:"call_count"`
}

// ModelConsumptionResponse represents model consumption statistics
type ModelConsumptionResponse struct {
	TotalCost        decimal.Decimal        `json:"total_cost"`
	PromptTokens     int64                  `json:"prompt_tokens"`
	CompletionTokens int64                  `json:"completion_tokens"`
	TotalRequests    int64                  `json:"total_requests"`
	ModelCount       int                    `json:"model_count"`
	Models           []ModelConsumptionItem `json:"models"`
	DailyConsumption []DailyConsumptionItem `json:"daily_consumption"`
}
