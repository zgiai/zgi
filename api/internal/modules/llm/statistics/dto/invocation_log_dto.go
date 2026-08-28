package dto

type InvocationLogRequest struct {
	StartTime        int64   `form:"start_time" binding:"required"`
	EndTime          int64   `form:"end_time" binding:"required"`
	InvocationSource *string `form:"invocation_source" binding:"omitempty,oneof=api product unknown"`
	AppType          *string `form:"app_type" binding:"omitempty,max=50"`
	ModelName        *string `form:"model_name" binding:"omitempty,max=100"`
	CursorTime       *string `form:"cursor_time"`
	CursorID         *string `form:"cursor_id" binding:"omitempty,max=100"`
	Limit            int     `form:"limit" binding:"omitempty,min=1,max=100"`
	IncludeSummary   *bool   `form:"include_summary"`
}

type InvocationLogSummary struct {
	InvocationCount int64 `json:"invocation_count"`
	APICount        int64 `json:"api_count"`
	ProductCount    int64 `json:"product_count"`
	UnknownCount    int64 `json:"unknown_count"`
	TotalTokens     int64 `json:"total_tokens"`
	TotalPoints     int64 `json:"total_points"`
}

type InvocationLogItem struct {
	InvocationID     string  `json:"invocation_id"`
	InvocationSource string  `json:"invocation_source"`
	AppID            *string `json:"app_id,omitempty"`
	AppType          string  `json:"app_type"`
	ModelName        string  `json:"model_name"`
	ProviderName     string  `json:"provider_name"`
	Status           string  `json:"status"`
	AttemptCount     int64   `json:"attempt_count"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	TotalPoints      int64   `json:"total_points"`
	DurationMS       int64   `json:"duration_ms"`
	StartedAt        int64   `json:"started_at"`
	SettledAt        int64   `json:"settled_at"`
	ErrorCode        *string `json:"error_code,omitempty"`
	ContentAvailable bool    `json:"content_available"`
	ContentExpiresAt *int64  `json:"content_expires_at,omitempty"`
}

type InvocationLogCursor struct {
	Time string `json:"time"`
	ID   string `json:"id"`
}

type InvocationLogResponse struct {
	Summary    InvocationLogSummary `json:"summary"`
	Items      []InvocationLogItem  `json:"items"`
	NextCursor *InvocationLogCursor `json:"next_cursor,omitempty"`
}
