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
	InvocationCount int64   `json:"invocation_count"`
	APICount        int64   `json:"api_count"`
	ProductCount    int64   `json:"product_count"`
	UnknownCount    int64   `json:"unknown_count"`
	TotalTokens     int64   `json:"total_tokens"`
	TotalPoints     int64   `json:"total_points"`
	TotalCostUSD    *string `json:"total_cost_usd,omitempty"`
	TotalCostCNY    *string `json:"total_cost_cny,omitempty"`
}

type InvocationLogItem struct {
	InvocationID     string                    `json:"invocation_id"`
	InvocationSource string                    `json:"invocation_source"`
	AppID            *string                   `json:"app_id,omitempty"`
	AppType          string                    `json:"app_type"`
	ModelName        string                    `json:"model_name"`
	ProviderName     string                    `json:"provider_name"`
	ChannelName      string                    `json:"channel_name"`
	Status           string                    `json:"status"`
	AttemptCount     int64                     `json:"attempt_count"`
	PromptTokens     int64                     `json:"prompt_tokens"`
	CacheReadTokens  int64                     `json:"cache_read_tokens"`
	CacheWriteTokens int64                     `json:"cache_write_tokens"`
	CompletionTokens int64                     `json:"completion_tokens"`
	TotalTokens      int64                     `json:"total_tokens"`
	TotalPoints      int64                     `json:"total_points"`
	TotalCostUSD     *string                   `json:"total_cost_usd,omitempty"`
	TotalCostCNY     *string                   `json:"total_cost_cny,omitempty"`
	PricingDetails   *InvocationPricingDetails `json:"pricing_details,omitempty"`
	DurationMS       int64                     `json:"duration_ms"`
	StartedAt        int64                     `json:"started_at"`
	SettledAt        int64                     `json:"settled_at"`
	ErrorCode        *string                   `json:"error_code,omitempty"`
	ContentAvailable bool                      `json:"content_available"`
	ContentExpiresAt *int64                    `json:"content_expires_at,omitempty"`
}

type InvocationPricingDetails struct {
	BillingLane                   string  `json:"billing_lane"`
	PricingSource                 string  `json:"pricing_source,omitempty"`
	UsageSource                   string  `json:"usage_source,omitempty"`
	InputPriceUSDPer1MTokens      *string `json:"input_price_usd_per_1m_tokens,omitempty"`
	CacheReadPriceUSDPer1MTokens  *string `json:"cache_read_price_usd_per_1m_tokens,omitempty"`
	CacheWritePriceUSDPer1MTokens *string `json:"cache_write_price_usd_per_1m_tokens,omitempty"`
	OutputPriceUSDPer1MTokens     *string `json:"output_price_usd_per_1m_tokens,omitempty"`
	InputCostUSD                  *string `json:"input_cost_usd,omitempty"`
	CacheReadCostUSD              *string `json:"cache_read_cost_usd,omitempty"`
	CacheWriteCostUSD             *string `json:"cache_write_cost_usd,omitempty"`
	OutputCostUSD                 *string `json:"output_cost_usd,omitempty"`
	CNYPerUSD                     *string `json:"cny_per_usd,omitempty"`
	BillingDisplayCurrency        string  `json:"billing_display_currency,omitempty"`
	InputPriceSource              string  `json:"input_price_source,omitempty"`
	CacheReadPriceSource          string  `json:"cache_read_price_source,omitempty"`
	CacheWritePriceSource         string  `json:"cache_write_price_source,omitempty"`
	OutputPriceSource             string  `json:"output_price_source,omitempty"`
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
