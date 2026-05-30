package dto

import (
	"time"
)

type CreateAgentRequest struct {
	Name        string `json:"name" binding:"required"`
	IconType    string `json:"icon_type,omitempty"`
	Icon        string `json:"icon,omitempty"`
	AgentType   string `json:"agent_type,omitempty"`
	Description string `json:"description,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Internal    *bool  `json:"internal,omitempty"`
}

// Agents represents the application basic information and capability switches.
type Agents struct {
	ID                  string     `json:"id" db:"id"`
	TenantID            string     `json:"tenant_id" db:"tenant_id"`
	WorkspaceID         string     `json:"workspace_id" db:"workspace_id"`
	Name                string     `json:"name" db:"name"`
	Description         string     `json:"description" db:"description"`
	AgentsType          string     `json:"agent_type" db:"agent_type"`
	IconType            *string    `json:"icon_type,omitempty" db:"icon_type"`
	Icon                *string    `json:"icon,omitempty" db:"icon"`
	AgentsModelConfigID *string    `json:"agents_model_config_id,omitempty" db:"agents_model_config_id"`
	WorkflowID          *string    `json:"workflow_id,omitempty" db:"workflow_id"`
	WorkflowConfig      *string    `json:"workflow_config,omitempty" db:"workflow_config"` // JSONB field for conversational workflow config
	EnableAPI           bool       `json:"enable_api" db:"enable_api"`
	IsPublic            bool       `json:"is_public" db:"is_public"`
	IsUniversal         bool       `json:"is_universal" db:"is_universal"`
	WebAppStatus        string     `json:"web_app_status" db:"web_app_status"`
	CreatedBy           *string    `json:"created_by,omitempty" db:"created_by"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	UpdatedBy           *string    `json:"updated_by,omitempty" db:"updated_by"`
	UpdatedAt           time.Time  `json:"updated_at" db:"updated_at"`
	DeletedBy           *string    `json:"deleted_by,omitempty" db:"deleted_by"`
	DeletedAt           *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

type WorkflowConfig struct {
	WorkflowID        string                 `json:"workflow_id"`
	VariableConfig    map[string]interface{} `json:"variable_config"`
	HistoryWindowSize int                    `json:"history_window_size"`
	ConversationID    string                 `json:"conversation_id"`
}

type GetAgentsListRequest struct {
	Page          int    `form:"page" json:"page"`
	PageSize      int    `form:"pageSize" json:"pageSize"`
	Limit         int    `form:"limit" json:"limit"`
	WorkspaceID   string `form:"workspace_id" json:"workspace_id"`
	Name          string `form:"name" json:"name"`
	Keyword       string `form:"keyword" json:"keyword"`
	IsCreatedByMe bool   `form:"is_created_by_me" json:"is_created_by_me"`
	AgentType     string `form:"agent_type" json:"agent_type"`
	Internal      *bool  `form:"internal" json:"internal"`
}

type GetRunnableWebAppsRequest struct {
	WorkspaceID string `form:"workspace_id" json:"workspace_id"`
}

type RunnableWebAppMetaData struct {
	Name      string  `json:"name"`
	Icon      *string `json:"icon"`
	IconType  *string `json:"icon_type"`
	IconUrl   string  `json:"icon_url"`
	Desc      string  `json:"desc"`
	AgentType string  `json:"agent_type"`
}

type RunnableWebAppItem struct {
	AgentID      string                 `json:"agent_id"`
	WorkspaceID  string                 `json:"workspace_id"`
	WebAppID     string                 `json:"web_app_id"`
	WebAppStatus string                 `json:"web_app_status"`
	MetaData     RunnableWebAppMetaData `json:"meta_data"`
}

type RunnableWebAppsResponse struct {
	Items []RunnableWebAppItem `json:"items"`
}

// AgentListItem represents a single agent in the list response
type AgentListItem struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	AgentType    string  `json:"agent_type"`
	TenantID     string  `json:"tenant_id"`
	WorkspaceID  string  `json:"workspace_id"`
	IconType     *string `json:"icon_type,omitempty"`
	Icon         *string `json:"icon,omitempty"`
	IconUrl      string  `json:"icon_url,omitempty"`
	IsPublic     bool    `json:"is_public"`
	IsPublished  bool    `json:"is_published"`
	WebAppStatus string  `json:"web_app_status"`
	CreatedBy    *string `json:"created_by,omitempty"`
	CreatedAt    int64   `json:"created_at"`
	UpdatedAt    int64   `json:"updated_at"`
	Internal     bool    `json:"internal"`
	CanEdit      bool    `json:"can_edit"` // Indicates if current user can edit this agent
}

// AgentsListResponse represents the paginated response for agent list queries
type AgentsListResponse struct {
	Page    int             `json:"page"`
	Limit   int             `json:"limit"`
	Total   int64           `json:"total"`
	HasMore bool            `json:"has_more"`
	Data    []AgentListItem `json:"data"`
}

// AgentDetailResponse represents the detailed response for a single agent
type AgentDetailResponse struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	AgentType    string                 `json:"agent_type"`
	IconType     *string                `json:"icon_type,omitempty"`
	Icon         *string                `json:"icon,omitempty"`
	EnableAPI    bool                   `json:"enable_api"`
	CreatedBy    *string                `json:"created_by,omitempty"`
	UpdatedBy    *string                `json:"updated_by,omitempty"`
	CreatedAt    int64                  `json:"created_at"`
	UpdatedAt    int64                  `json:"updated_at"`
	IsPublished  bool                   `json:"is_published"`
	WebAppStatus string                 `json:"web_app_status"`
	Permission   *string                `json:"permission,omitempty"`
	Internal     bool                   `json:"internal"`
	IsEditor     bool                   `json:"is_editor"`
	CanEdit      bool                   `json:"can_edit"` // Indicates if current user can edit this agent
	Workflow     interface{}            `json:"workflow,omitempty"`
	AgentConfig  interface{}            `json:"agent_config,omitempty"`
	Tenant       map[string]interface{} `json:"tenant,omitempty"`
	OwnerAccount map[string]interface{} `json:"owner_account,omitempty"`
}

type UpdateWebAppStatusRequest struct {
	Status string `json:"status" binding:"required"`
	Reason string `json:"reason,omitempty"`
}

type WebAppStatusResponse struct {
	AgentID      string `json:"agent_id"`
	WebAppID     string `json:"web_app_id"`
	WebAppStatus string `json:"web_app_status"`
	UpdatedAt    int64  `json:"updated_at"`
}

type AgentRuntimeModeConfig struct {
	EnabledSkillIDs          []string                `json:"enabled_skill_ids"`
	UseMemory                bool                    `json:"use_memory"`
	AgentMemoryEnabled       bool                    `json:"agent_memory_enabled"`
	AgentMemorySlots         []AgentMemorySlotConfig `json:"agent_memory_slots,omitempty"`
	FileUploadEnabled        bool                    `json:"file_upload_enabled"`
	HomeTitle                string                  `json:"home_title"`
	InputPlaceholder         string                  `json:"input_placeholder"`
	ThemeColor               string                  `json:"theme_color"`
	SuggestedQuestions       []string                `json:"suggested_questions"`
	KnowledgeDatasetIDs      []string                `json:"knowledge_dataset_ids"`
	KnowledgeRetrievalConfig map[string]interface{}  `json:"knowledge_retrieval_config"`
}

type AgentMemorySlotConfig struct {
	ID               string `json:"id,omitempty"`
	Key              string `json:"key"`
	Description      string `json:"description"`
	MaxChars         int    `json:"max_chars"`
	Enabled          bool   `json:"enabled"`
	SortOrder        int    `json:"sort_order"`
	CreatedAt        int64  `json:"created_at,omitempty"`
	UpdatedAt        int64  `json:"updated_at,omitempty"`
	CreatedAtUnix    int64  `json:"created_at_unix,omitempty"`
	UpdatedAtUnix    int64  `json:"updated_at_unix,omitempty"`
	CreatedAtISO     string `json:"created_at_iso,omitempty"`
	UpdatedAtISO     string `json:"updated_at_iso,omitempty"`
	CreatedAtDisplay string `json:"created_at_display,omitempty"`
	UpdatedAtDisplay string `json:"updated_at_display,omitempty"`
}

type AgentMemoryValueResponse struct {
	AgentMemorySlotConfig
	Content string `json:"content"`
}

type AgentMemoryValuesResponse struct {
	UserScope string                     `json:"user_scope"`
	UserID    string                     `json:"user_id"`
	Values    []AgentMemoryValueResponse `json:"values"`
}

type UpdateAgentMemoryValueRequest struct {
	UserScope string `json:"user_scope,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	Key       string `json:"key" binding:"required"`
	Content   string `json:"content"`
}

type AgentConfigRequest struct {
	SystemPrompt             string                 `json:"system_prompt"`
	ModelProvider            string                 `json:"model_provider"`
	Model                    string                 `json:"model"`
	ModelParameters          map[string]interface{} `json:"model_parameters"`
	EnabledSkillIDs          []string               `json:"enabled_skill_ids"`
	UseMemory                bool                   `json:"use_memory"`
	AgentMemoryEnabled       bool                   `json:"agent_memory_enabled"`
	FileUpload               bool                   `json:"file_upload_enabled"`
	HomeTitle                string                 `json:"home_title"`
	InputPlaceholder         string                 `json:"input_placeholder"`
	ThemeColor               string                 `json:"theme_color"`
	SuggestedQuestions       []string               `json:"suggested_questions"`
	KnowledgeDatasetIDs      []string               `json:"knowledge_dataset_ids"`
	KnowledgeRetrievalConfig map[string]interface{} `json:"knowledge_retrieval_config"`
}

type AgentConfigResponse struct {
	AgentID                  string                  `json:"agent_id"`
	SystemPrompt             string                  `json:"system_prompt"`
	ModelProvider            string                  `json:"model_provider"`
	Model                    string                  `json:"model"`
	ModelParameters          map[string]interface{}  `json:"model_parameters"`
	EnabledSkillIDs          []string                `json:"enabled_skill_ids"`
	UseMemory                bool                    `json:"use_memory"`
	AgentMemoryEnabled       bool                    `json:"agent_memory_enabled"`
	AgentMemorySlots         []AgentMemorySlotConfig `json:"agent_memory_slots"`
	FileUpload               bool                    `json:"file_upload_enabled"`
	HomeTitle                string                  `json:"home_title"`
	InputPlaceholder         string                  `json:"input_placeholder"`
	ThemeColor               string                  `json:"theme_color"`
	SuggestedQuestions       []string                `json:"suggested_questions"`
	UpdatedAt                int64                   `json:"updated_at"`
	KnowledgeDatasetIDs      []string                `json:"knowledge_dataset_ids"`
	KnowledgeRetrievalConfig map[string]interface{}  `json:"knowledge_retrieval_config"`
}

type AgentDraftRuntimeConfigResponse struct {
	AgentID     string              `json:"agent_id"`
	WorkspaceID string              `json:"workspace_id"`
	Config      AgentConfigResponse `json:"config"`
}

type AgentSuggestedQuestionSkillContext struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type GenerateAgentSuggestedQuestionsRequest struct {
	Locale            string                               `json:"locale,omitempty"`
	Count             int                                  `json:"count,omitempty" validate:"omitempty,min=1,max=6"`
	Provider          string                               `json:"provider,omitempty"`
	Model             string                               `json:"model,omitempty"`
	SystemPrompt      string                               `json:"system_prompt,omitempty"`
	HomeTitle         string                               `json:"home_title,omitempty"`
	ExistingQuestions []string                             `json:"existing_questions,omitempty"`
	Skills            []AgentSuggestedQuestionSkillContext `json:"skills,omitempty"`
	KnowledgeRefs     []string                             `json:"knowledge_refs,omitempty"`
}

type PublishAgentRequest struct {
	Description string `json:"description"`
}

type PublishAgentResponse struct {
	AgentID     string `json:"agent_id"`
	VersionUUID string `json:"version_uuid"`
	Version     string `json:"version"`
	WebAppID    string `json:"web_app_id"`
	PublishedAt int64  `json:"published_at"`
}

type AgentPublishedVersionResponse struct {
	ID             string              `json:"id"`
	AgentID        string              `json:"agent_id"`
	VersionUUID    string              `json:"version_uuid"`
	Version        string              `json:"version"`
	Description    string              `json:"description"`
	ConfigSnapshot AgentConfigResponse `json:"config_snapshot"`
	IsCurrent      bool                `json:"is_current"`
	CreatedAt      int64               `json:"created_at"`
}

type AgentPublishedVersionsResponse struct {
	Data    []AgentPublishedVersionResponse `json:"data"`
	Page    int                             `json:"page"`
	Limit   int                             `json:"limit"`
	Total   int64                           `json:"total"`
	HasMore bool                            `json:"has_more"`
}

type RollbackAgentPublishedVersionRequest struct {
	VersionID string `json:"version_id" binding:"required"`
}

type AgentWebAppRuntimeConfigResponse struct {
	AgentID        string              `json:"agent_id"`
	WebAppID       string              `json:"web_app_id"`
	WorkspaceID    string              `json:"workspace_id"`
	OrganizationID string              `json:"organization_id"`
	AgentType      string              `json:"agent_type"`
	Name           string              `json:"name"`
	Description    string              `json:"description"`
	Icon           string              `json:"icon"`
	IconType       string              `json:"icon_type"`
	IconURL        string              `json:"icon_url"`
	Version        string              `json:"version"`
	VersionUUID    string              `json:"version_uuid"`
	Config         AgentConfigResponse `json:"config"`
}

type AgentPublicWebAppConfigResponse struct {
	AgentID            string   `json:"agent_id"`
	WebAppID           string   `json:"web_app_id"`
	AgentType          string   `json:"agent_type"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Icon               string   `json:"icon"`
	IconType           string   `json:"icon_type"`
	IconURL            string   `json:"icon_url"`
	HomeTitle          string   `json:"home_title"`
	InputPlaceholder   string   `json:"input_placeholder"`
	SuggestedQuestions []string `json:"suggested_questions"`
	FileUpload         bool     `json:"file_upload_enabled"`
	AgentMemoryEnabled bool     `json:"agent_memory_enabled"`
	Version            string   `json:"version"`
	VersionUUID        string   `json:"version_uuid"`
}
