package dto

// AgentLogRequest represents the request for getting agent logs
type AgentLogRequest struct {
	ConversationID string `form:"conversation_id" json:"conversation_id" binding:"required"`
	MessageID      string `form:"message_id" json:"message_id" binding:"required"`
}

// AgentLogResponse represents the response for agent logs
type AgentLogResponse struct {
	Meta       AgentLogMeta     `json:"meta"`
	Iterations []AgentIteration `json:"iterations"`
	Files      []interface{}    `json:"files"`
}

// AgentLogMeta represents metadata for agent logs
type AgentLogMeta struct {
	Status      string  `json:"status"`
	Executor    string  `json:"executor"`
	StartTime   string  `json:"start_time"`
	ElapsedTime float64 `json:"elapsed_time"`
	TotalTokens int     `json:"total_tokens"`
	AgentMode   string  `json:"agent_mode"`
	Iterations  int     `json:"iterations"`
	Error       *string `json:"error,omitempty"`
}

// AgentIteration represents a single iteration in agent thinking
type AgentIteration struct {
	Tokens    int        `json:"tokens"`
	ToolCalls []ToolCall `json:"tool_calls"`
	ToolRaw   ToolRaw    `json:"tool_raw"`
	Thought   string     `json:"thought"`
	CreatedAt string     `json:"created_at"`
	Files     []string   `json:"files"`
}

// ToolCall represents a tool call in agent iteration
type ToolCall struct {
	Status         string                 `json:"status"`
	Error          string                 `json:"error"`
	TimeCost       float64                `json:"time_cost"`
	ToolName       string                 `json:"tool_name"`
	ToolLabel      string                 `json:"tool_label"`
	ToolInput      map[string]interface{} `json:"tool_input"`
	ToolOutput     map[string]interface{} `json:"tool_output"`
	ToolParameters map[string]interface{} `json:"tool_parameters"`
	ToolIcon       string                 `json:"tool_icon"`
}

// ToolRaw represents raw tool input/output
type ToolRaw struct {
	Inputs  string `json:"inputs"`
	Outputs string `json:"outputs"`
}
