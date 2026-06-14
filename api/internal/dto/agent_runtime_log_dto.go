package dto

// AgentRuntimeRunsRequest represents the request parameters for agent runtime runs.
type AgentRuntimeRunsRequest struct {
	Page           int    `form:"page"`
	Limit          int    `form:"limit"`
	TriggeredFrom  string `form:"triggered_from" binding:"omitempty"`
	Source         string `form:"source" binding:"omitempty"`
	Query          string `form:"q" binding:"omitempty"`
	ConversationID string `form:"conversation_id" binding:"omitempty"`
}

// AgentRuntimeRunsResponse represents a page of agent runtime run summaries.
type AgentRuntimeRunsResponse struct {
	Page    int                   `json:"page"`
	Limit   int                   `json:"limit"`
	Total   int64                 `json:"total"`
	HasMore bool                  `json:"has_more"`
	Data    []AgentRuntimeRunItem `json:"data"`
}

// AgentRuntimeRunItem represents one agent runtime turn in a log list.
type AgentRuntimeRunItem struct {
	ID             string  `json:"id"`
	ConversationID string  `json:"conversation_id"`
	Status         string  `json:"status"`
	Query          string  `json:"query"`
	AnswerPreview  string  `json:"answer_preview,omitempty"`
	ModelName      string  `json:"model_name,omitempty"`
	ModelProvider  *string `json:"model_provider,omitempty"`
	ElapsedTime    float64 `json:"elapsed_time"`
	TotalTokens    int64   `json:"total_tokens"`
	TotalSteps     int     `json:"total_steps"`
	CreatedAt      int64   `json:"created_at"`
	FinishedAt     *int64  `json:"finished_at"`
	Error          string  `json:"error,omitempty"`
	Source         string  `json:"source"`
	SourceWebAppID *string `json:"source_web_app_id,omitempty"`
}

// AgentRuntimeRunDetail represents one agent runtime turn detail.
type AgentRuntimeRunDetail struct {
	ID              string                 `json:"id"`
	ConversationID  string                 `json:"conversation_id"`
	Status          string                 `json:"status"`
	Query           string                 `json:"query"`
	Answer          string                 `json:"answer"`
	ModelName       string                 `json:"model_name,omitempty"`
	ModelProvider   *string                `json:"model_provider,omitempty"`
	ModelParameters map[string]interface{} `json:"model_parameters,omitempty"`
	Usage           interface{}            `json:"usage,omitempty"`
	ElapsedTime     float64                `json:"elapsed_time"`
	TotalTokens     int64                  `json:"total_tokens"`
	TotalSteps      int                    `json:"total_steps"`
	CreatedAt       int64                  `json:"created_at"`
	FinishedAt      *int64                 `json:"finished_at"`
	Error           string                 `json:"error,omitempty"`
	Source          string                 `json:"source"`
	SourceWebAppID  *string                `json:"source_web_app_id,omitempty"`
}

// AgentRuntimeStepsResponse represents the ordered steps for one agent runtime run.
type AgentRuntimeStepsResponse struct {
	Data []AgentRuntimeStep `json:"data"`
}

// AgentRuntimeDebugTraceResponse represents one short-lived raw model debug trace.
type AgentRuntimeDebugTraceResponse struct {
	MessageID string                 `json:"message_id"`
	RuntimeID string                 `json:"runtime_id"`
	Trace     map[string]interface{} `json:"trace"`
	ExpiresAt *int64                 `json:"expires_at,omitempty"`
}

// AgentRuntimeStep represents a user input, skill/tool invocation, or model answer step.
type AgentRuntimeStep struct {
	ID          string                 `json:"id"`
	Index       int                    `json:"index"`
	Type        string                 `json:"type"`
	Title       string                 `json:"title"`
	Status      string                 `json:"status"`
	Input       interface{}            `json:"input,omitempty"`
	Output      interface{}            `json:"output,omitempty"`
	Process     map[string]interface{} `json:"process,omitempty"`
	ElapsedTime float64                `json:"elapsed_time"`
	CreatedAt   *int64                 `json:"created_at,omitempty"`
	FinishedAt  *int64                 `json:"finished_at,omitempty"`
	Error       string                 `json:"error,omitempty"`
}
