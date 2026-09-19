package dto

import "github.com/google/uuid"

const (
	RuntimeSurfaceWorkChat          = "work_chat"
	RuntimeSurfaceContextualSidebar = "contextual_sidebar"
	RuntimeSurfaceExternalPageChat  = "external_page_chat"
)

type CreateConversationRequest struct {
	Title            string `json:"title"`
	ConversationType string `json:"conversation_type,omitempty"`
}

type UpdateConversationRequest struct {
	Title                *string `json:"title,omitempty"`
	Status               *string `json:"status,omitempty"`
	CurrentLeafMessageID *string `json:"current_leaf_message_id,omitempty"`
}

type ChatRequest struct {
	InitialConnectorBindings []InitialConnectorBinding `json:"initial_connector_bindings,omitempty"`
	// InitialConnectorDefaultsRevision validates the reviewed defaults snapshot;
	// it never asks the server to infer or populate connection selections.
	InitialConnectorDefaultsRevision *int64                 `json:"initial_connector_defaults_revision,omitempty"`
	ConversationID                   string                 `json:"conversation_id,omitempty"`
	ParentID                         string                 `json:"parent_id,omitempty"`
	Query                            string                 `json:"query" binding:"required"`
	Surface                          string                 `json:"surface,omitempty"`
	RuntimeContext                   string                 `json:"runtime_context,omitempty"`
	OperationContext                 map[string]interface{} `json:"operation_context,omitempty"`
	FileIDs                          []string               `json:"file_ids,omitempty"`
	Model                            string                 `json:"model,omitempty"`
	Provider                         string                 `json:"provider,omitempty"`
	ResponseMode                     string                 `json:"response_mode,omitempty"`
	Parameters                       map[string]interface{} `json:"parameters,omitempty"`
	UseMemory                        bool                   `json:"use_memory,omitempty"`
	// MaxTaskTokens caps the total provider-reported model tokens for this
	// API-originated task. Zero keeps the legacy unlimited behaviour and is
	// never populated by Workflow callers.
	MaxTaskTokens int `json:"max_task_tokens,omitempty"`
	// SourceMessageID is a server-owned idempotency link. HTTP decoders never populate it.
	SourceMessageID *uuid.UUID `json:"-"`
	// HistoryPolicy is supplied only by trusted subject adapters. It cannot be
	// selected through the public Chat API and is not persisted in message
	// metadata. It lets channel callers keep durable formal turns as bounded
	// supplemental context while the provider conversation remains primary.
	HistoryPolicy *ConversationHistoryPolicy `json:"-"`
}

const (
	ConversationHistoryModeSupplemental = "supplemental"
	ConversationHistoryModeIsolated     = "isolated"
)

// ConversationHistoryPolicy controls only the durable ChatRuntime branch used
// to prepare one model turn. ExcludedTexts is transient and should contain
// provider context already supplied elsewhere in the same request.
type ConversationHistoryPolicy struct {
	Mode          string
	MaxTurns      int
	ExcludedTexts []string
}

// InitialConnectorBinding is a first-turn selection, never a credential or an
// instruction to replace a binding on an existing conversation.
type InitialConnectorBinding struct {
	ConnectorID  string `json:"connector_id"`
	ConnectionID string `json:"connection_id"`
}

type RegenerateMessageRequest struct {
	Query            *string                `json:"query,omitempty"`
	Surface          string                 `json:"surface,omitempty"`
	RuntimeContext   string                 `json:"runtime_context,omitempty"`
	OperationContext map[string]interface{} `json:"operation_context,omitempty"`
	Model            *string                `json:"model,omitempty"`
	Provider         *string                `json:"provider,omitempty"`
	Parameters       map[string]interface{} `json:"parameters,omitempty"`
	UseMemory        *bool                  `json:"use_memory,omitempty"`
}

type ToolGovernanceDecisionRequest struct {
	Action             string `json:"action" binding:"required"`
	Reason             string `json:"reason,omitempty"`
	RememberForSession bool   `json:"remember_for_session,omitempty"`
}

type ToolGovernanceDecisionResponse struct {
	ConversationID     string                 `json:"conversation_id"`
	MessageID          string                 `json:"message_id"`
	CorrelationID      string                 `json:"correlation_id"`
	Action             string                 `json:"action"`
	ApprovalStatus     string                 `json:"approval_status"`
	RememberForSession bool                   `json:"remember_for_session,omitempty"`
	SessionGrant       map[string]interface{} `json:"session_grant,omitempty"`
	Event              map[string]interface{} `json:"event"`
}

type ClientActionResultRequest struct {
	Status           string                 `json:"status" binding:"required"`
	Surface          string                 `json:"surface,omitempty"`
	RuntimeContext   string                 `json:"runtime_context,omitempty"`
	OperationContext map[string]interface{} `json:"operation_context,omitempty"`
	Result           map[string]interface{} `json:"result,omitempty"`
	Error            string                 `json:"error,omitempty"`
}

type UserInputContinuationRequest struct {
	Answers map[string]string `json:"answers,omitempty"`
	// Action and AdditionalTokens are used only when the pending request is a
	// task-token-budget pause. Ordinary model questions continue to use Answers.
	Action           string                 `json:"action,omitempty"`
	AdditionalTokens int                    `json:"additional_tokens,omitempty"`
	Surface          string                 `json:"surface,omitempty"`
	RuntimeContext   string                 `json:"runtime_context,omitempty"`
	OperationContext map[string]interface{} `json:"operation_context,omitempty"`
}

type StopConversationResponse struct {
	ConversationID string  `json:"conversation_id"`
	MessageID      *string `json:"message_id,omitempty"`
	RuntimeStatus  string  `json:"runtime_status"`
	Status         string  `json:"status"`
}

type SkillResponse struct {
	SkillID               string                              `json:"skill_id"`
	Source                string                              `json:"source"`
	Name                  string                              `json:"name"`
	Description           string                              `json:"description"`
	WhenToUse             string                              `json:"when_to_use"`
	Display               SkillDisplayResponse                `json:"display"`
	RuntimeType           string                              `json:"runtime_type"`
	Enabled               bool                                `json:"enabled"`
	HasTools              bool                                `json:"has_tools"`
	HasReferences         bool                                `json:"has_references"`
	HasScripts            bool                                `json:"has_scripts"`
	ScriptsSupported      bool                                `json:"scripts_supported"`
	MaxCallsPerTurn       int                                 `json:"max_calls_per_turn"`
	TimeoutSeconds        int                                 `json:"timeout_seconds"`
	Status                string                              `json:"status"`
	ValidationError       string                              `json:"validation_error,omitempty"`
	SupportedCallers      []string                            `json:"supported_callers,omitempty"`
	RequiredConfig        []string                            `json:"required_config,omitempty"`
	DependencyType        string                              `json:"dependency_type"`
	ConnectorRequirements []SkillConnectorRequirementResponse `json:"connector_requirements,omitempty"`
	Exposure              SkillExposureResponse               `json:"exposure"`
}

type SkillConnectorRequirementResponse struct {
	ConnectorID  string   `json:"connector_id"`
	OperationIDs []string `json:"operation_ids,omitempty"`
	Required     bool     `json:"required"`
}

type SkillExposureResponse struct {
	Category            string `json:"category"`
	UserSelectable      bool   `json:"user_selectable"`
	RuntimeManaged      bool   `json:"runtime_managed"`
	SystemAsset         bool   `json:"system_asset"`
	PageContextRequired bool   `json:"page_context_required"`
	GovernanceRisk      string `json:"governance_risk"`
}

type SkillConfigResponse struct {
	EnabledSkillIDs []string `json:"enabled_skill_ids"`
}

type UpdateSkillConfigRequest struct {
	EnabledSkillIDs    []string `json:"enabled_skill_ids"`
	AgentBindingAction string   `json:"agent_binding_action,omitempty"`
	ImpactToken        string   `json:"impact_token,omitempty"`
}

type AccountSkillPreferenceResponse struct {
	EnabledSkillIDs []string `json:"enabled_skill_ids"`
	Defaulted       bool     `json:"defaulted"`
}

type UpdateAccountSkillPreferenceRequest struct {
	EnabledSkillIDs []string `json:"enabled_skill_ids"`
}

type ImportSkillPreviewFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type ImportSkillPreviewResponse struct {
	ImportID         string                   `json:"import_id,omitempty"`
	ExpiresAt        int64                    `json:"expires_at,omitempty"`
	Skill            *SkillResponse           `json:"skill,omitempty"`
	WillOverwrite    bool                     `json:"will_overwrite"`
	ExistingSkill    *ExistingSkillResponse   `json:"existing_skill,omitempty"`
	FileCount        int                      `json:"file_count"`
	TotalSize        int64                    `json:"total_size"`
	Files            []ImportSkillPreviewFile `json:"files"`
	References       []string                 `json:"references"`
	HasScripts       bool                     `json:"has_scripts"`
	ScriptsSupported bool                     `json:"scripts_supported"`
	Warnings         []string                 `json:"warnings"`
	ValidationErrors []string                 `json:"validation_errors"`
	CanImport        bool                     `json:"can_import"`
}

type ConfirmImportSkillRequest struct {
	ImportID           string `json:"import_id" binding:"required"`
	OverwriteConfirmed bool   `json:"overwrite_confirmed,omitempty"`
}

type ExistingSkillResponse struct {
	SkillID   string `json:"skill_id"`
	Name      string `json:"name"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
}

type SkillDisplayResponse struct {
	Icon        string              `json:"icon"`
	Category    string              `json:"category"`
	Scenarios   []string            `json:"scenarios,omitempty"`
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
	ConversationType     string                 `json:"conversation_type"`
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
	RuntimeInputs       []RuntimeInputResponse `json:"runtime_inputs"`
	CreatedAt           int64                  `json:"created_at"`
	UpdatedAt           int64                  `json:"updated_at"`
}

type RuntimeInputResponse struct {
	ID                string  `json:"id"`
	ClientInputID     string  `json:"client_input_id"`
	Content           string  `json:"content"`
	Status            string  `json:"status"`
	TargetMessageID   string  `json:"target_message_id"`
	ConsumedMessageID *string `json:"consumed_message_id,omitempty"`
	Sequence          int64   `json:"sequence"`
	CreatedAt         int64   `json:"created_at"`
}

type SubmitRuntimeInputRequest struct {
	ClientInputID string `json:"client_input_id" binding:"required"`
	Content       string `json:"content" binding:"required"`
}

type SubmitRuntimeInputResponse struct {
	Input       RuntimeInputResponse `json:"input"`
	Disposition string               `json:"disposition"`
	Created     bool                 `json:"created"`
}

type SearchResultResponse struct {
	Type              string  `json:"type"`
	ConversationID    string  `json:"conversation_id"`
	ConversationTitle string  `json:"conversation_title"`
	MessageID         *string `json:"message_id,omitempty"`
	Snippet           string  `json:"snippet"`
	UpdatedAt         int64   `json:"updated_at"`
}

type ListResponse[T any] struct {
	Data    []T   `json:"data"`
	Page    int   `json:"page"`
	Limit   int   `json:"limit"`
	Total   int64 `json:"total"`
	HasMore bool  `json:"has_more"`
}
