package dto

// AppNode represents basic app information
type AppNode struct {
	ID   string `json:"id"`
	Mode string `json:"mode"`
	Name string `json:"name"`
}

// MessageDetailResponse represents detailed message information
type MessageDetailResponse struct {
	ID                      string                     `json:"id"`
	ConversationID          string                     `json:"conversation_id"`
	Inputs                  map[string]interface{}     `json:"inputs"`
	Query                   string                     `json:"query"`
	Message                 []interface{}              `json:"message"`
	MessageTokens           int                        `json:"message_tokens"`
	Answer                  string                     `json:"answer"`
	AnswerTokens            int                        `json:"answer_tokens"`
	ProviderResponseLatency float64                    `json:"provider_response_latency"`
	FromSource              string                     `json:"from_source"`
	FromEndUserID           *string                    `json:"from_end_user_id"`
	FromAccountID           *string                    `json:"from_account_id"`
	CreatedAt               int64                      `json:"created_at"`
	Status                  string                     `json:"status"`
	Error                   *string                    `json:"error"`
	ParentMessageID         *string                    `json:"parent_message_id"`
	Feedbacks               []MessageFeedbackResponse  `json:"feedbacks"`
	WorkflowRunID           *string                    `json:"workflow_run_id"`
	Annotation              *MessageAnnotationResponse `json:"annotation"`
	AgentThoughts           []AgentThoughtResponse     `json:"agent_thoughts"`
	MessageFiles            []MessageFileResponse      `json:"message_files"`
	Metadata                map[string]interface{}     `json:"metadata"`
}

// CreateMessageRequest represents request for creating a message
type CreateMessageRequest struct {
	ConversationID  string                 `json:"conversation_id" binding:"required"`
	Query           string                 `json:"query" binding:"required"`
	Inputs          map[string]interface{} `json:"inputs"`
	ModelProvider   string                 `json:"model_provider"`
	ModelID         string                 `json:"model_id"`
	ResponseMode    string                 `json:"response_mode" binding:"omitempty,oneof=blocking streaming"`
	ParentMessageID *string                `json:"parent_message_id"`
}

// UpdateMessageRequest represents request for updating a message
type UpdateMessageRequest struct {
	Answer                  *string                `json:"answer"`
	AnswerTokens            *int                   `json:"answer_tokens"`
	ProviderResponseLatency *float64               `json:"provider_response_latency"`
	Status                  *string                `json:"status"`
	Error                   *string                `json:"error"`
	Metadata                map[string]interface{} `json:"metadata"`
}

// MessageFeedbackResponse represents message feedback information
type MessageFeedbackResponse struct {
	ID      string  `json:"id"`
	Rating  *string `json:"rating"`
	Content *string `json:"content"`
}

// MessageAnnotationResponse represents message annotation information
type MessageAnnotationResponse struct {
	ID       string  `json:"id"`
	Question *string `json:"question"`
	Content  string  `json:"content"`
}

// AgentThoughtResponse represents agent thought information
type AgentThoughtResponse struct {
	ID          string                 `json:"id"`
	ChainID     *string                `json:"chain_id"`
	Position    int                    `json:"position"`
	Thought     *string                `json:"thought"`
	Tool        *string                `json:"tool"`
	ToolLabels  map[string]interface{} `json:"tool_labels"`
	ToolInput   *string                `json:"tool_input"`
	Observation *string                `json:"observation"`
	CreatedAt   int64                  `json:"created_at"`
}

// MessageFileResponse represents message file information
type MessageFileResponse struct {
	ID             string  `json:"id"`
	Type           string  `json:"type"`
	TransferMethod string  `json:"transfer_method"`
	URL            *string `json:"url"`
	BelongsTo      *string `json:"belongs_to"`
}
