package dto

type CreateConversationRequest struct {
	Title string `json:"title"`
}

type UpdateConversationRequest struct {
	Title  *string `json:"title,omitempty"`
	Status *string `json:"status,omitempty"`
}

type ChatRequest struct {
	ConversationID string                 `json:"conversation_id,omitempty"`
	ParentID       string                 `json:"parent_id,omitempty"`
	Query          string                 `json:"query" binding:"required"`
	FileIDs        []string               `json:"file_ids,omitempty"`
	Model          string                 `json:"model" binding:"required"`
	Provider       string                 `json:"provider,omitempty"`
	ResponseMode   string                 `json:"response_mode,omitempty"`
	Parameters     map[string]interface{} `json:"parameters,omitempty"`
}

type RegenerateMessageRequest struct {
	Query      *string                `json:"query,omitempty"`
	Model      *string                `json:"model,omitempty"`
	Provider   *string                `json:"provider,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

type StopConversationResponse struct {
	ConversationID string  `json:"conversation_id"`
	MessageID      *string `json:"message_id,omitempty"`
	RuntimeStatus  string  `json:"runtime_status"`
	Status         string  `json:"status"`
}

type SkillResponse struct {
	SkillID          string               `json:"skill_id"`
	Source           string               `json:"source"`
	Name             string               `json:"name"`
	Description      string               `json:"description"`
	WhenToUse        string               `json:"when_to_use"`
	Display          SkillDisplayResponse `json:"display"`
	RuntimeType      string               `json:"runtime_type"`
	Enabled          bool                 `json:"enabled"`
	HasTools         bool                 `json:"has_tools"`
	HasReferences    bool                 `json:"has_references"`
	HasScripts       bool                 `json:"has_scripts"`
	ScriptsSupported bool                 `json:"scripts_supported"`
	MaxCallsPerTurn  int                  `json:"max_calls_per_turn"`
	TimeoutSeconds   int                  `json:"timeout_seconds"`
}

type SkillConfigResponse struct {
	EnabledSkillIDs []string `json:"enabled_skill_ids"`
}

type UpdateSkillConfigRequest struct {
	EnabledSkillIDs []string `json:"enabled_skill_ids"`
}

type SkillDisplayResponse struct {
	Icon        string              `json:"icon"`
	Category    string              `json:"category"`
	Label       map[string]string   `json:"label"`
	Description map[string]string   `json:"description"`
	WhenToUse   map[string]string   `json:"when_to_use"`
	Tags        map[string][]string `json:"tags,omitempty"`
}

type ConversationResponse struct {
	ID                   string                 `json:"id"`
	OrganizationID       string                 `json:"organization_id"`
	WorkspaceID          *string                `json:"workspace_id,omitempty"`
	AccountID            string                 `json:"account_id"`
	Title                string                 `json:"title"`
	Status               string                 `json:"status"`
	RuntimeStatus        string                 `json:"runtime_status"`
	CurrentLeafMessageID *string                `json:"current_leaf_message_id,omitempty"`
	ActiveMessageID      *string                `json:"active_message_id,omitempty"`
	DialogueCount        int                    `json:"dialogue_count"`
	Source               string                 `json:"source"`
	SourceConversationID *string                `json:"source_conversation_id,omitempty"`
	SourceWebAppID       *string                `json:"source_web_app_id,omitempty"`
	Metadata             map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt            int64                  `json:"created_at"`
	UpdatedAt            int64                  `json:"updated_at"`
}

type MessageResponse struct {
	ID                  string                 `json:"id"`
	ConversationID      string                 `json:"conversation_id"`
	ParentID            *string                `json:"parent_id,omitempty"`
	Query               string                 `json:"query"`
	Answer              string                 `json:"answer"`
	Status              string                 `json:"status"`
	Error               *string                `json:"error,omitempty"`
	ModelProvider       *string                `json:"model_provider,omitempty"`
	ModelName           string                 `json:"model_name"`
	BillingReasonSource *string                `json:"billing_reason_source,omitempty"`
	ModelParameters     map[string]interface{} `json:"model_parameters,omitempty"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
	SourceMessageID     *string                `json:"source_message_id,omitempty"`
	CreatedAt           int64                  `json:"created_at"`
	UpdatedAt           int64                  `json:"updated_at"`
}

type ListResponse[T any] struct {
	Data    []T   `json:"data"`
	Page    int   `json:"page"`
	Limit   int   `json:"limit"`
	Total   int64 `json:"total"`
	HasMore bool  `json:"has_more"`
}
