package dto

import (
	"time"
)

// AppDetailKernel represents basic app information - corresponds to app_detail_kernel_fields
type AppDetailKernel struct {
	ID             string `json:"id" db:"id"`
	Name           string `json:"name" db:"name"`
	Description    string `json:"description" db:"description"`
	Mode           string `json:"mode" db:"mode"`
	IconType       string `json:"icon_type" db:"icon_type"`
	Icon           string `json:"icon" db:"icon"`
	IconBackground string `json:"icon_background" db:"icon_background"`
	IconURL        string `json:"icon_url"`
}

// ModelConfig represents app model configuration - corresponds to model_config_fields
type ModelConfig struct {
	OpeningStatement              interface{} `json:"opening_statement"`
	SuggestedQuestions            interface{} `json:"suggested_questions"`
	SuggestedQuestionsAfterAnswer interface{} `json:"suggested_questions_after_answer"`
	SpeechToText                  interface{} `json:"speech_to_text"`
	TextToSpeech                  interface{} `json:"text_to_speech"`
	RetrieverResource             interface{} `json:"retriever_resource"`
	AnnotationReply               interface{} `json:"annotation_reply"`
	MoreLikeThis                  interface{} `json:"more_like_this"`
	SensitiveWordAvoidance        interface{} `json:"sensitive_word_avoidance"`
	ExternalDataTools             interface{} `json:"external_data_tools"`
	Model                         interface{} `json:"model"`
	UserInputForm                 interface{} `json:"user_input_form"`
	DatasetQueryVariable          string      `json:"dataset_query_variable"`
	PrePrompt                     string      `json:"pre_prompt"`
	AgentMode                     interface{} `json:"agent_mode"`
	PromptType                    string      `json:"prompt_type"`
	ChatPromptConfig              interface{} `json:"chat_prompt_config"`
	CompletionPromptConfig        interface{} `json:"completion_prompt_config"`
	DatasetConfigs                interface{} `json:"dataset_configs"`
	FileUpload                    interface{} `json:"file_upload"`
	CreatedBy                     string      `json:"created_by"`
	CreatedAt                     time.Time   `json:"created_at"`
	UpdatedBy                     string      `json:"updated_by"`
	UpdatedAt                     time.Time   `json:"updated_at"`
}

// WorkflowPartial represents partial workflow information - corresponds to workflow_partial_fields
type WorkflowPartial struct {
	ID        string    `json:"id"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedBy string    `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AppDetail represents detailed app information - corresponds to app_detail_fields
type AppDetail struct {
	ID                  string           `json:"id" db:"id"`
	Name                string           `json:"name" db:"name"`
	Description         string           `json:"description" db:"description"`
	Mode                string           `json:"mode" db:"mode"`
	Icon                string           `json:"icon" db:"icon"`
	IconBackground      string           `json:"icon_background" db:"icon_background"`
	EnableSite          bool             `json:"enable_site" db:"enable_site"`
	EnableAPI           bool             `json:"enable_api" db:"enable_api"`
	ModelConfig         *ModelConfig     `json:"model_config,omitempty"`
	Workflow            *WorkflowPartial `json:"workflow,omitempty"`
	Tracing             interface{}      `json:"tracing"`
	UseIconAsAnswerIcon bool             `json:"use_icon_as_answer_icon" db:"use_icon_as_answer_icon"`
	CreatedBy           string           `json:"created_by" db:"created_by"`
	CreatedAt           time.Time        `json:"created_at" db:"created_at"`
	UpdatedBy           string           `json:"updated_by" db:"updated_by"`
	UpdatedAt           time.Time        `json:"updated_at" db:"updated_at"`
	AccessMode          string           `json:"access_mode"`
}

// SiteInfo represents site configuration - corresponds to site_fields
type SiteInfo struct {
	AccessToken            string    `json:"access_token"`
	Code                   string    `json:"code"`
	Title                  string    `json:"title"`
	IconType               string    `json:"icon_type"`
	Icon                   string    `json:"icon"`
	IconBackground         string    `json:"icon_background"`
	IconURL                string    `json:"icon_url"`
	Description            string    `json:"description"`
	DefaultLanguage        string    `json:"default_language"`
	ChatColorTheme         string    `json:"chat_color_theme"`
	ChatColorThemeInverted bool      `json:"chat_color_theme_inverted"`
	CustomizeDomain        string    `json:"customize_domain"`
	Copyright              string    `json:"copyright"`
	PrivacyPolicy          string    `json:"privacy_policy"`
	CustomDisclaimer       string    `json:"custom_disclaimer"`
	CustomizeTokenStrategy string    `json:"customize_token_strategy"`
	PromptPublic           bool      `json:"prompt_public"`
	AppBaseURL             string    `json:"app_base_url"`
	ShowWorkflowSteps      bool      `json:"show_workflow_steps"`
	UseIconAsAnswerIcon    bool      `json:"use_icon_as_answer_icon"`
	CreatedBy              string    `json:"created_by"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedBy              string    `json:"updated_by"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// DeletedTool represents deleted tool information - corresponds to deleted_tool_fields
type DeletedTool struct {
	Type       string `json:"type"`
	ToolName   string `json:"tool_name"`
	ProviderID string `json:"provider_id"`
}

// AppDetailWithSite represents app detail with site info - corresponds to app_detail_fields_with_site
type AppDetailWithSite struct {
	AppDetail
	IconType     string        `json:"icon_type" db:"icon_type"`
	IconURL      string        `json:"icon_url"`
	Site         *SiteInfo     `json:"site,omitempty"`
	APIBaseURL   string        `json:"api_base_url"`
	DeletedTools []DeletedTool `json:"deleted_tools"`
}

// Tag represents tag information - corresponds to tag_fields
type Tag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// AppPartial represents partial app information for lists - corresponds to app_partial_fields
type AppPartial struct {
	ID                  string           `json:"id" db:"id"`
	Name                string           `json:"name" db:"name"`
	MaxActiveRequests   interface{}      `json:"max_active_requests"`
	Description         string           `json:"description" db:"description"`
	Mode                string           `json:"mode" db:"mode"`
	IconType            string           `json:"icon_type" db:"icon_type"`
	Icon                string           `json:"icon" db:"icon"`
	IconBackground      string           `json:"icon_background" db:"icon_background"`
	IconURL             string           `json:"icon_url"`
	ModelConfig         *ModelConfig     `json:"model_config,omitempty"`
	Workflow            *WorkflowPartial `json:"workflow,omitempty"`
	UseIconAsAnswerIcon bool             `json:"use_icon_as_answer_icon" db:"use_icon_as_answer_icon"`
	CreatedBy           string           `json:"created_by" db:"created_by"`
	CreatedAt           time.Time        `json:"created_at" db:"created_at"`
	UpdatedBy           string           `json:"updated_by" db:"updated_by"`
	UpdatedAt           time.Time        `json:"updated_at" db:"updated_at"`
	Tags                []Tag            `json:"tags"`
	AccessMode          string           `json:"access_mode"`
	CreateUserName      string           `json:"create_user_name"`
	AuthorName          string           `json:"author_name"`
}

// AppPagination represents paginated app list - corresponds to app_pagination_fields
type AppPagination struct {
	Page    int          `json:"page"`
	Limit   int          `json:"limit"`
	Total   int64        `json:"total"`
	HasMore bool         `json:"has_more"`
	Data    []AppPartial `json:"data"`
}

// Request DTOs
// ListAppsRequest represents request for listing apps - corresponds to AppListApi GET parameters
type ListAppsRequest struct {
	Page          int      `form:"page" binding:"min=1" json:"page"`
	Limit         int      `form:"limit" binding:"min=1,max=100" json:"limit"`
	Mode          string   `form:"mode" binding:"oneof=completion chat advanced-chat workflow agent-chat channel all" json:"mode"`
	Name          string   `form:"name" json:"name"`
	TagIDs        []string `form:"tag_ids" json:"tag_ids"`
	IsCreatedByMe *bool    `form:"is_created_by_me" json:"is_created_by_me"`
}

// CreateAppRequest represents request for creating app - corresponds to AppListApi POST
type CreateAppRequest struct {
	Name           string `json:"name" binding:"required"`
	Description    string `json:"description"`
	Mode           string `json:"mode" binding:"required,oneof=completion chat advanced-chat workflow agent-chat"`
	IconType       string `json:"icon_type"`
	Icon           string `json:"icon"`
	IconBackground string `json:"icon_background"`
}

// UpdateAppRequest represents request for updating app - corresponds to AppApi PUT
type UpdateAppRequest struct {
	Name                string `json:"name" binding:"required"`
	Description         string `json:"description"`
	IconType            string `json:"icon_type"`
	Icon                string `json:"icon"`
	IconBackground      string `json:"icon_background"`
	UseIconAsAnswerIcon *bool  `json:"use_icon_as_answer_icon"`
}

// CopyAppRequest represents request for copying app - corresponds to AppCopyApi POST
type CopyAppRequest struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	IconType       string `json:"icon_type"`
	Icon           string `json:"icon"`
	IconBackground string `json:"icon_background"`
}

// ExportAppRequest represents request for exporting app - corresponds to AppExportApi GET
type ExportAppRequest struct {
	IncludeSecret bool `form:"include_secret" json:"include_secret"`
}

// UpdateAppNameRequest represents request for updating app name - corresponds to AppNameApi POST
type UpdateAppNameRequest struct {
	Name string `json:"name" binding:"required"`
}

// UpdateAppIconRequest represents request for updating app icon - corresponds to AppIconApi POST
type UpdateAppIconRequest struct {
	Icon           string `json:"icon"`
	IconBackground string `json:"icon_background"`
}

// UpdateAppSiteStatusRequest represents request for updating app site status - corresponds to AppSiteStatus POST
type UpdateAppSiteStatusRequest struct {
	EnableSite bool `json:"enable_site" binding:"required"`
}

// UpdateAppAPIStatusRequest represents request for updating app API status - corresponds to AppApiStatus POST
type UpdateAppAPIStatusRequest struct {
	EnableAPI bool `json:"enable_api" binding:"required"`
}

// AppTraceConfig represents app tracing configuration - corresponds to AppTraceApi
type AppTraceConfig struct {
	Enabled         bool   `json:"enabled"`
	TracingProvider string `json:"tracing_provider"`
}

// UpdateAppTraceRequest represents request for updating app trace config - corresponds to AppTraceApi POST
type UpdateAppTraceRequest struct {
	Enabled         bool   `json:"enabled" binding:"required"`
	TracingProvider string `json:"tracing_provider" binding:"required"`
}

// Response DTOs
// AppExportResponse represents app export response - corresponds to AppExportApi response
type AppExportResponse struct {
	Data string `json:"data"`
}

// StandardResponse represents standard success response
type StandardResponse struct {
	Result string `json:"result"`
}

// AppStatus application status enum
type AppStatus string

const (
	AppStatusNormal AppStatus = "normal"
	AppStatusSystem AppStatus = "system"
)

// App represents the app model
type App struct {
	ID                  string    `json:"id" db:"id"`
	TenantID            string    `json:"tenant_id" db:"tenant_id"`
	WorkspaceID         string    `json:"workspace_id" db:"workspace_id"`
	Name                string    `json:"name" db:"name"`
	Description         string    `json:"description" db:"description"`
	Mode                string    `json:"mode" db:"mode"`
	IconType            string    `json:"icon_type" db:"icon_type"`
	Icon                string    `json:"icon" db:"icon"`
	IconBackground      string    `json:"icon_background" db:"icon_background"`
	Status              string    `json:"status" db:"status"`
	EnableSite          bool      `json:"enable_site" db:"enable_site"`
	EnableAPI           bool      `json:"enable_api" db:"enable_api"`
	IsUniversal         bool      `json:"is_universal" db:"is_universal"`
	UseIconAsAnswerIcon bool      `json:"use_icon_as_answer_icon" db:"use_icon_as_answer_icon"`
	WorkflowID          *string   `json:"workflow_id" db:"workflow_id"`
	CreatedBy           string    `json:"created_by" db:"created_by"`
	CreatedAt           time.Time `json:"created_at" db:"created_at"`
	UpdatedBy           string    `json:"updated_by" db:"updated_by"`
	UpdatedAt           time.Time `json:"updated_at" db:"updated_at"`
}

// Conversation represents a conversation
type Conversation struct {
	ID                      string     `json:"id" db:"id"`
	AppID                   string     `json:"app_id" db:"app_id"`
	ModelProvider           *string    `json:"model_provider" db:"model_provider"`
	ModelID                 *string    `json:"model_id" db:"model_id"`
	ModelConfig             JSONMap    `json:"model_config" db:"model_config"`
	OverrideModelConfigs    *string    `json:"override_model_configs" db:"override_model_configs"`
	Mode                    string     `json:"mode" db:"mode"`
	Name                    string     `json:"name" db:"name"`
	Summary                 *string    `json:"summary" db:"summary"`
	Inputs                  JSONMap    `json:"inputs" db:"inputs"`
	Introduction            *string    `json:"introduction" db:"introduction"`
	SystemInstruction       *string    `json:"system_instruction" db:"system_instruction"`
	SystemInstructionTokens int        `json:"system_instruction_tokens" db:"system_instruction_tokens"`
	Status                  string     `json:"status" db:"status"`
	FromSource              string     `json:"from_source" db:"from_source"`
	FromEndUserID           *string    `json:"from_end_user_id" db:"from_end_user_id"`
	FromEndUserSessionID    *string    `json:"from_end_user_session_id" db:"from_end_user_session_id"`
	FromAccountID           *string    `json:"from_account_id" db:"from_account_id"`
	FromAccountName         *string    `json:"from_account_name" db:"from_account_name"`
	ReadAt                  *time.Time `json:"read_at" db:"read_at"`
	ReadAccountID           *string    `json:"read_account_id" db:"read_account_id"`
	CreatedAt               time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at" db:"updated_at"`
	IsDeleted               bool       `json:"is_deleted" db:"is_deleted"`
	InvokeFrom              *string    `json:"invoke_from" db:"invoke_from"`
}

// type JSONMap map[string]interface{}

// GetSummaryOrQuery returns summary or query
func (c *Conversation) GetSummaryOrQuery() string {
	if c.Summary != nil && *c.Summary != "" {
		return *c.Summary
	}
	return ""
}

// Message represents a chat message
type Message struct {
	ID                      string    `json:"id" db:"id"`
	AppID                   string    `json:"app_id" db:"app_id"`
	ConversationID          string    `json:"conversation_id" db:"conversation_id"`
	Query                   string    `json:"query" db:"query"`
	Answer                  string    `json:"answer" db:"answer"`
	AnswerTokens            int       `json:"answer_tokens" db:"answer_tokens"`
	MessageTokens           int       `json:"message_tokens" db:"message_tokens"`
	ProviderResponseLatency float64   `json:"provider_response_latency" db:"provider_response_latency"`
	CreatedAt               time.Time `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time `json:"updated_at" db:"updated_at"`
}

// AgentThought represents agent thinking process
type AgentThought struct {
	ID          string    `json:"id" db:"id"`
	MessageID   string    `json:"message_id" db:"message_id"`
	Position    int       `json:"position" db:"position"`
	Thought     *string   `json:"thought" db:"thought"`
	Tool        *string   `json:"tool" db:"tool"`
	ToolInput   *string   `json:"tool_input" db:"tool_input"`
	Observation *string   `json:"observation" db:"observation"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// MessageFile represents message file attachment
type MessageFile struct {
	ID             string `json:"id" db:"id"`
	MessageID      string `json:"message_id" db:"message_id"`
	Type           string `json:"type" db:"type"`
	TransferMethod string `json:"transfer_method" db:"transfer_method"`
	URL            string `json:"url" db:"url"`
	BelongsTo      string `json:"belongs_to" db:"belongs_to"`
}

// AppModelConfig represents app model configuration
type AppModelConfig struct {
	ID        string  `json:"id" db:"id"`
	AgentMode *string `json:"agent_mode" db:"agent_mode"`
}

// SharedAccountInfo represents comprehensive account information for sharing
type SharedAccountInfo struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Email              string    `json:"email"`
	Password           string    `json:"password"`
	PasswordSalt       string    `json:"password_salt"`
	Avatar             string    `json:"avatar"`
	InterfaceLanguage  string    `json:"interface_language"`
	InterfaceTheme     string    `json:"interface_theme"`
	Timezone           string    `json:"timezone"`
	LastLoginAt        time.Time `json:"last_login_at"`
	LastActiveAt       time.Time `json:"last_active_at"`
	Status             string    `json:"status"`
	InitializedAt      time.Time `json:"initialized_at"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	LastLoginIp        string    `json:"last_login_ip"`
	CurrentTenantID    string    `json:"current_tenant_id"`
	CurrentWorkspaceID string    `json:"current_workspace_id"`
	GroupRole          string    `json:"group_role"`
}

// Workflow represents a workflow
type Workflow struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
