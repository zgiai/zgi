package service

import (
	"context"
	"mime/multipart"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/agentmemoryruntime"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/tokenestimate"
	"github.com/zgiai/zgi/api/internal/modules/shared/titlegen"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
)

const (
	defaultConversationTitle = "New chat"
	systemPromptVersion      = "aichat.v1"
	maxContextMessages       = 20
	maxConversationTitleLen  = 50
	defaultSearchLimit       = 20
	maxSearchLimit           = 50
	searchSnippetRunes       = 120
	staleActiveMessageError  = "runtime_lease_expired"
	streamEventsExpiredError = "stream events expired"
	titleGenerationTimeout   = 15 * time.Second
	runtimeContextMaxRunes   = 8000
	runtimeLeaseHeartbeat    = 15 * time.Second
	runtimeLeaseFailureTTL   = 90 * time.Second
	runtimeLeaseLegacyTTL    = time.Hour
	defaultModelIdleTimeout  = 5 * time.Minute

	streamEventMessageStart         = "message_start"
	streamEventMessage              = "message"
	streamEventMessageRetract       = "message_retract"
	streamEventMessageEnd           = "message_end"
	streamEventError                = "error"
	streamEventAgentProgress        = "agent_progress"
	streamEventIntermediateAnswer   = "agent_intermediate_answer"
	streamEventUserInputRequested   = "user_input_requested"
	streamEventFileParseStart       = "file_parse_start"
	streamEventFileParseEnd         = "file_parse_end"
	streamEventFileParseError       = "file_parse_error"
	streamEventSkillCallStart       = "skill_call_start"
	streamEventSkillCallEnd         = "skill_call_end"
	streamEventSkillCallError       = "skill_call_error"
	streamEventClientActionRequired = "client_action_required"
	streamEventSkillLoadStart       = "skill_load_start"
	streamEventSkillLoadEnd         = "skill_load_end"
	streamEventSkillReferenceRead   = "skill_reference_read"
	streamEventSkillArtifactCreated = "skill_artifact_created"

	skillModeDisabled = "disabled"
	skillModeAuto     = "auto"
	skillModeRequired = "required"

	executionModeAgentLoop      = "agent_loop"
	executionModeLegacyToolChat = "legacy_tool_chat"
	executionModeDirectChat     = "direct_chat"

	userMemoryContextBudgetChars  = 4000
	agentMemoryContextBudgetChars = 4000
)

var defaultSystemSkillIDs = []string{
	skills.SkillTime,
	skills.SkillCalculator,
	skills.SkillConsoleNavigator,
	skills.SkillFileGenerator,
	skills.SkillFileManager,
	skills.SkillFileReader,
}

const (
	aiChatSurfaceWorkChat          = runtimedto.RuntimeSurfaceWorkChat
	aiChatSurfaceContextualSidebar = runtimedto.RuntimeSurfaceContextualSidebar
	aiChatSurfaceExternalPageChat  = runtimedto.RuntimeSurfaceExternalPageChat
)

type Scope struct {
	OrganizationID    uuid.UUID
	AccountID         uuid.UUID
	WorkspaceID       *uuid.UUID
	AgentMemoryUserID *uuid.UUID
	SkipAccessCheck   bool
}

type Caller struct {
	Type           string
	ID             *uuid.UUID
	Source         string
	SourceWebAppID *uuid.UUID
}

type RunConfig struct {
	SystemPrompt              string
	SystemPromptVersion       string
	ModelProvider             string
	Model                     string
	ModelParameters           map[string]interface{}
	EnabledSkillIDs           []string
	KnowledgeDatasetIDs       []string
	KnowledgeBoundByAccountID string
	KnowledgeBoundAtUnix      int64
	KnowledgeRetrievalConfig  map[string]interface{}
	DatabaseBindings          []AgentDatabaseBinding
	DatabaseBoundByAccountID  string
	DatabaseBoundAtUnix       int64
	WorkflowBindings          []AgentWorkflowBinding
	WorkflowBoundByAccountID  string
	WorkflowBoundAtUnix       int64
	BindingAuthorizations     []ResourceBindingAuthorization
	UseMemory                 bool
	AgentMemoryEnabled        bool
	AgentMemorySlots          []AgentMemorySlotConfig
	AgentMemoryUserScope      string
	BillingAppID              string
	BillingAppType            string
}

// ResourceBindingAuthorization is the runtime authorization evidence for one
// concrete Agent resource binding.
type ResourceBindingAuthorization struct {
	BindingType      string `json:"binding_type"`
	ResourceID       string `json:"resource_id"`
	ParentResourceID string `json:"parent_resource_id,omitempty"`
	AccessMode       string `json:"access_mode"`
	BoundByAccountID string `json:"bound_by_account_id"`
	BoundAtUnix      int64  `json:"bound_at_unix"`
}

type AgentMemorySlotConfig = agentmemoryruntime.Slot
type AgentDatabaseBinding struct {
	DataSourceID     string   `json:"data_source_id"`
	TableIDs         []string `json:"table_ids"`
	WritableTableIDs []string `json:"writable_table_ids,omitempty"`
}
type AgentWorkflowBinding struct {
	BindingID       string                    `json:"binding_id"`
	Label           string                    `json:"label"`
	Description     string                    `json:"description,omitempty"`
	AgentID         string                    `json:"agent_id"`
	WorkflowID      string                    `json:"workflow_id"`
	AgentType       string                    `json:"agent_type,omitempty"`
	VersionStrategy string                    `json:"version_strategy"`
	VersionUUID     string                    `json:"version_uuid,omitempty"`
	TimeoutSeconds  int                       `json:"timeout_seconds,omitempty"`
	StartInputs     []AgentWorkflowStartInput `json:"start_inputs,omitempty"`
	RequiredInputs  []string                  `json:"required_inputs,omitempty"`
	DefaultInputKey string                    `json:"default_input_key,omitempty"`
}
type AgentWorkflowStartInput struct {
	Variable            string      `json:"variable"`
	Label               string      `json:"label,omitempty"`
	Type                string      `json:"type,omitempty"`
	Required            bool        `json:"required,omitempty"`
	Default             interface{} `json:"default,omitempty"`
	DefaultDateTimeMode string      `json:"default_datetime_mode,omitempty"`
}
type AgentMemoryRuntimeState = agentmemoryruntime.State
type AgentMemoryPlannerDecision = agentmemoryruntime.Decision
type AgentMemoryPlannerResult = agentmemoryruntime.PlannerResult
type AgentMemoryMutationResult = agentmemoryruntime.MutationResult

type Service interface {
	CreateConversation(ctx context.Context, scope Scope, title string) (*runtimemodel.Conversation, error)
	CreateConversationForCaller(ctx context.Context, scope Scope, caller Caller, title string) (*runtimemodel.Conversation, error)
	ListConversations(ctx context.Context, scope Scope, page, limit int) ([]*runtimemodel.Conversation, int64, error)
	ListConversationsByCaller(ctx context.Context, scope Scope, caller Caller, page, limit int) ([]*runtimemodel.Conversation, int64, error)
	ListConversationsBySurface(ctx context.Context, scope Scope, surface string, page, limit int) ([]*runtimemodel.Conversation, int64, error)
	Search(ctx context.Context, scope Scope, query string, limit int) ([]*SearchResult, error)
	SearchBySurface(ctx context.Context, scope Scope, surface string, query string, limit int) ([]*SearchResult, error)
	SearchByCaller(ctx context.Context, scope Scope, caller Caller, query string, limit int) ([]*SearchResult, error)
	GetConversation(ctx context.Context, scope Scope, id uuid.UUID) (*runtimemodel.Conversation, error)
	GetConversationByCaller(ctx context.Context, scope Scope, caller Caller, id uuid.UUID) (*runtimemodel.Conversation, error)
	UpdateConversation(ctx context.Context, scope Scope, id uuid.UUID, req runtimedto.UpdateConversationRequest) (*runtimemodel.Conversation, error)
	UpdateConversationByCaller(ctx context.Context, scope Scope, caller Caller, id uuid.UUID, req runtimedto.UpdateConversationRequest) (*runtimemodel.Conversation, error)
	DeleteConversation(ctx context.Context, scope Scope, id uuid.UUID) error
	DeleteConversationByCaller(ctx context.Context, scope Scope, caller Caller, id uuid.UUID) error
	ListMessages(ctx context.Context, scope Scope, conversationID uuid.UUID, page, limit int) ([]*runtimemodel.Message, int64, error)
	ListConversationMessagesByCaller(ctx context.Context, scope Scope, caller Caller, conversationID uuid.UUID, page, limit int) ([]*runtimemodel.Message, int64, error)
	ListMessagesByCaller(ctx context.Context, scope Scope, caller Caller, page, limit int) ([]*runtimemodel.Message, int64, error)
	ListMessagesByCallerSource(ctx context.Context, scope Scope, caller Caller, source string, page, limit int) ([]*runtimemodel.Message, int64, error)
	ListMessagesByCallerLogFilters(ctx context.Context, scope Scope, caller Caller, source string, conversationID *uuid.UUID, queryText string, page, limit int) ([]*runtimemodel.Message, int64, error)
	ListMessagesByCallerRuntimeLogFilters(ctx context.Context, scope Scope, caller Caller, source string, conversationID *uuid.UUID, queryText string, page, limit int) ([]*runtimemodel.Message, int64, error)
	GetMessageByCaller(ctx context.Context, scope Scope, caller Caller, id uuid.UUID) (*runtimemodel.Message, *runtimemodel.Conversation, error)
	GetMessageByCallerRuntimeLog(ctx context.Context, scope Scope, caller Caller, id uuid.UUID, source string) (*runtimemodel.Message, *runtimemodel.Conversation, error)
	DeleteMessage(ctx context.Context, scope Scope, id uuid.UUID) error
	StopMessage(ctx context.Context, scope Scope, id uuid.UUID) (*runtimemodel.Message, error)
	StopConversation(ctx context.Context, scope Scope, id uuid.UUID) (*StopConversationResult, error)
	StopConversationByCaller(ctx context.Context, scope Scope, caller Caller, id uuid.UUID) (*StopConversationResult, error)
	PrepareChat(ctx context.Context, scope Scope, req runtimedto.ChatRequest) (*PreparedChat, error)
	PrepareConfiguredChat(ctx context.Context, scope Scope, caller Caller, config RunConfig, req runtimedto.ChatRequest) (*PreparedChat, error)
	PrepareRootRegeneration(ctx context.Context, scope Scope, id uuid.UUID, req runtimedto.RegenerateMessageRequest) (*PreparedChat, error)
	PrepareConfiguredRootRegeneration(ctx context.Context, scope Scope, caller Caller, config RunConfig, id uuid.UUID, req runtimedto.RegenerateMessageRequest) (*PreparedChat, error)
	RunPreparedStream(ctx context.Context, prepared *PreparedChat, onChunk func(string) error, onEvent ...func(StreamEvent) error) (*ChatResult, error)
	StreamConversationEvents(ctx context.Context, scope Scope, conversationID, messageID uuid.UUID, afterID string, onEvent func(StreamEvent) error) error
	StreamConversationEventsForCaller(ctx context.Context, scope Scope, caller Caller, conversationID, messageID uuid.UUID, afterID string, onEvent func(StreamEvent) error) error
	SubmitToolGovernanceDecision(ctx context.Context, scope Scope, conversationID, messageID uuid.UUID, correlationID string, req runtimedto.ToolGovernanceDecisionRequest) (*runtimedto.ToolGovernanceDecisionResponse, error)
	RunToolGovernanceDecisionStream(ctx context.Context, scope Scope, conversationID, messageID uuid.UUID, correlationID string, req runtimedto.ToolGovernanceDecisionRequest, onEvent func(StreamEvent) error) (*ChatResult, error)
	RunClientActionContinuationStream(ctx context.Context, scope Scope, conversationID, messageID uuid.UUID, actionID string, req runtimedto.ClientActionResultRequest, onEvent func(StreamEvent) error) (*ChatResult, error)
	RunUserInputContinuationStream(ctx context.Context, scope Scope, conversationID, messageID uuid.UUID, requestID string, req runtimedto.UserInputContinuationRequest, onEvent func(StreamEvent) error) (*ChatResult, error)
	RunConfiguredUserInputContinuationStream(ctx context.Context, scope Scope, caller Caller, config RunConfig, conversationID, messageID uuid.UUID, requestID string, req runtimedto.UserInputContinuationRequest, onEvent func(StreamEvent) error) (*ChatResult, error)
	BeginWorkflowApprovalContinuation(ctx context.Context, scope Scope, caller Caller, config RunConfig, conversationID, messageID uuid.UUID) (*WorkflowApprovalContinuation, error)
	RecordWorkflowApprovalContinuationEvent(ctx context.Context, continuation *WorkflowApprovalContinuation, eventType string, payload map[string]interface{}) (*StreamEvent, error)
	AppendWorkflowApprovalContinuationStreamEvent(ctx context.Context, continuation *WorkflowApprovalContinuation, eventType string, payload map[string]interface{}) (*StreamEvent, error)
	UpdateWorkflowApprovalContinuationStatus(ctx context.Context, continuation *WorkflowApprovalContinuation, status string) (map[string]interface{}, error)
	PauseWorkflowApprovalContinuation(ctx context.Context, continuation *WorkflowApprovalContinuation, status string) (map[string]interface{}, error)
	SummarizeWorkflowApprovalContinuation(ctx context.Context, scope Scope, continuation *WorkflowApprovalContinuation, req WorkflowContinuationSummaryRequest, onEvent func(StreamEvent) error) (*ChatResult, error)
	CompleteWorkflowApprovalContinuation(ctx context.Context, continuation *WorkflowApprovalContinuation, answer string, status string) (map[string]interface{}, error)
	FailWorkflowApprovalContinuation(ctx context.Context, continuation *WorkflowApprovalContinuation, message string) (map[string]interface{}, error)
	ListSkills(ctx context.Context, scope Scope) ([]skills.SkillDiscoveryMetadata, error)
	GetSkill(ctx context.Context, scope Scope, skillID string) (*skills.SkillDiscoveryMetadata, error)
	GetSkillConfig(ctx context.Context, scope Scope) (*SkillConfig, error)
	UpdateSkillConfig(ctx context.Context, scope Scope, req runtimedto.UpdateSkillConfigRequest) (*SkillConfig, error)
	GetAccountSkillPreference(ctx context.Context, scope Scope, callerType string) (*AccountSkillPreference, error)
	UpdateAccountSkillPreference(ctx context.Context, scope Scope, callerType string, req runtimedto.UpdateAccountSkillPreferenceRequest) (*AccountSkillPreference, error)
	PreviewImportCustomSkill(ctx context.Context, scope Scope, fileHeader *multipart.FileHeader) (*SkillImportPreview, error)
	ConfirmCustomSkillImport(ctx context.Context, scope Scope, importID string, overwriteConfirmed bool) (*skills.SkillDiscoveryMetadata, error)
	CancelCustomSkillImportPreview(ctx context.Context, scope Scope, importID string) error
	DeleteSkill(ctx context.Context, scope Scope, skillID, agentBindingAction, impactToken string) error
	CleanupStaleActiveMessages(ctx context.Context) (int64, error)
	CleanupExpiredCustomSkillImportPreviews(ctx context.Context) error
	MigrateWebAppConversation(ctx context.Context, scope Scope, sourceConversationID uuid.UUID) (*runtimemodel.Conversation, error)
}

type UserMemoryService interface {
	IsEnabled(ctx context.Context, accountID uuid.UUID) (bool, error)
	RenderContext(ctx context.Context, accountID uuid.UUID, budget int) (string, error)
}

type AgentMemoryContextService interface {
	ReadUserMemory(ctx context.Context, workspaceID, agentID uuid.UUID, slots []agentmemory.RuntimeSlot, userScope string, userID uuid.UUID) ([]agentmemory.SlotValueResponse, error)
	UpdateValue(ctx context.Context, workspaceID, agentID uuid.UUID, slots []agentmemory.RuntimeSlot, userScope string, userID uuid.UUID, req agentmemory.UpdateValueRequest, meta agentmemory.MutationMetadata) (*agentmemory.SlotValueResponse, error)
	ClearValue(ctx context.Context, workspaceID, agentID uuid.UUID, slots []agentmemory.RuntimeSlot, userScope string, userID uuid.UUID, key string, meta agentmemory.MutationMetadata) (*agentmemory.SlotValueResponse, error)
}

type service struct {
	repos              *repository.Repositories
	llmClient          llmclient.LLMClient
	streams            *streamRegistry
	events             *streamEventStore
	titleGen           titlegen.Service
	modelSpecResolver  ModelSpecResolver
	tokenEstimator     *tokenestimate.Estimator
	fileService        FileLookupService
	contentExtractor   ContentExtractionService
	workspacePerms     WorkspacePermissionService
	skillRuntime       *skills.Runtime
	memoryService      UserMemoryService
	agentMemoryService AgentMemoryContextService
	customSkillStorage customSkillStorage
	modelIdleTimeout   time.Duration
}

func NewService(repos *repository.Repositories, llmClient llmclient.LLMClient) Service {
	return NewServiceWithTitleGenerator(repos, llmClient, nil)
}

func NewServiceWithTitleGenerator(repos *repository.Repositories, llmClient llmclient.LLMClient, titleGen titlegen.Service) Service {
	return NewServiceWithTitleGeneratorAndModelSpecResolver(repos, llmClient, titleGen, nil)
}

func NewServiceWithTitleGeneratorAndModelSpecResolver(
	repos *repository.Repositories,
	llmClient llmclient.LLMClient,
	titleGen titlegen.Service,
	modelSpecResolver ModelSpecResolver,
) Service {
	return NewServiceWithDependencies(repos, llmClient, titleGen, modelSpecResolver, nil, nil, nil)
}

func NewServiceWithDependencies(
	repos *repository.Repositories,
	llmClient llmclient.LLMClient,
	titleGen titlegen.Service,
	modelSpecResolver ModelSpecResolver,
	fileService FileLookupService,
	contentExtractor ContentExtractionService,
	workspacePerms WorkspacePermissionService,
) Service {
	return NewServiceWithSkillRuntime(repos, llmClient, titleGen, modelSpecResolver, fileService, contentExtractor, workspacePerms, nil, nil)
}

func NewServiceWithSkillRuntime(
	repos *repository.Repositories,
	llmClient llmclient.LLMClient,
	titleGen titlegen.Service,
	modelSpecResolver ModelSpecResolver,
	fileService FileLookupService,
	contentExtractor ContentExtractionService,
	workspacePerms WorkspacePermissionService,
	skillRuntime *skills.Runtime,
	memoryService UserMemoryService,
	optionalServices ...interface{},
) Service {
	var agentMemoryService AgentMemoryContextService
	for _, item := range optionalServices {
		switch typed := item.(type) {
		case AgentMemoryContextService:
			if agentMemoryService == nil {
				agentMemoryService = typed
			}
		}
	}
	return &service{
		repos:              repos,
		llmClient:          llmClient,
		streams:            newStreamRegistry(),
		events:             newStreamEventStore(redisutil.GetClient()),
		titleGen:           titleGen,
		modelSpecResolver:  modelSpecResolver,
		tokenEstimator:     newTokenEstimator(),
		fileService:        fileService,
		contentExtractor:   contentExtractor,
		workspacePerms:     workspacePerms,
		skillRuntime:       skillRuntime,
		memoryService:      memoryService,
		agentMemoryService: agentMemoryService,
		customSkillStorage: newFilesystemCustomSkillStorage(customSkillStorageRoot),
		modelIdleTimeout:   configuredModelIdleTimeout(),
	}
}

func configuredModelIdleTimeout() time.Duration {
	if config.GlobalConfig == nil || config.GlobalConfig.ChatRuntime.ModelIdleTimeoutSeconds <= 0 {
		return defaultModelIdleTimeout
	}
	return time.Duration(config.GlobalConfig.ChatRuntime.ModelIdleTimeoutSeconds) * time.Second
}

type PreparedChat struct {
	Conversation                   *runtimemodel.Conversation
	Message                        *runtimemodel.Message
	LLMRequest                     *adapter.ChatRequest
	ReplaceRoot                    bool
	Continuation                   bool
	SuppressInitialNaturalProgress bool
	PreferredRestoredSkillID       string
	ContinuationType               string
	TerminalOnly                   bool
	Scope                          Scope
	Caller                         Caller
	RunConfig                      RunConfig
	ParentID                       *uuid.UUID
	parts                          *chatRequestParts

	UserMemoryPreflightDone  bool
	UserMemoryPreflightUsage *adapter.Usage

	usageExecutionStarted  bool
	usageExecutionBaseline *adapter.Usage
	usageContinuation      bool
}

type ChatResult struct {
	Answer            string
	Metadata          map[string]interface{}
	Usage             *adapter.Usage
	Status            string
	MessageEndEventID string
}

type StopConversationResult struct {
	Conversation *runtimemodel.Conversation
	Message      *runtimemodel.Message
}

type SearchResult struct {
	Type              string
	ConversationID    uuid.UUID
	ConversationTitle string
	MessageID         *uuid.UUID
	Snippet           string
	UpdatedAt         time.Time
}

type SkillConfig struct {
	EnabledSkillIDs []string
}

type AccountSkillPreference struct {
	EnabledSkillIDs []string
	Defaulted       bool
}

type SkillImportPreviewFile struct {
	Path string
	Size int64
}

type SkillImportPreview struct {
	ImportID         string
	ExpiresAt        time.Time
	Skill            *skills.SkillDiscoveryMetadata
	WillOverwrite    bool
	ExistingSkill    *ExistingSkill
	FileCount        int
	TotalSize        int64
	Files            []SkillImportPreviewFile
	References       []string
	HasScripts       bool
	ScriptsSupported bool
	Warnings         []string
	ValidationErrors []string
	CanImport        bool
}

type ExistingSkill struct {
	SkillID   string
	Name      string
	UpdatedAt time.Time
}

type chatRequestParts struct {
	Query                        string
	Surface                      string
	RuntimeContext               string
	RawOperationContext          map[string]interface{}
	OperationContext             map[string]interface{}
	OperationLedger              map[string]interface{}
	ModelName                    string
	Provider                     string
	ProviderPtr                  *string
	Parameters                   map[string]interface{}
	ContextControl               map[string]interface{}
	Attachments                  *attachmentBundle
	RecentAssetCandidates        []ResourceCandidate
	RecentGeneratedArtifacts     []map[string]interface{}
	RecentOperationPlans         []map[string]interface{}
	ModelSupportsVision          bool
	ModelSupportsAgent           bool
	ExecutionMode                string
	FunctionCallingKnown         bool
	ModelSupportsFunctionCalling bool
	FunctionCallingAssumed       bool
	ModelCapabilityStatus        string
	ModelCapabilityError         string
	ProtocolToolsEnabled         bool
	UseMemory                    bool
	SkillIDs                     []string
	ToolSkillIDs                 []string
	SkillMode                    string
	SystemPrompt                 string
	SystemPromptVersion          string
	ConfiguredSkillIDs           []string
	KnowledgeDatasetIDs          []string
	KnowledgeRetrievalConfig     map[string]interface{}
	AgentMemoryEnabled           bool
	AgentMemorySlots             []AgentMemorySlotConfig
	AgentMemoryUserScope         string
	AgentMemoryAgentID           string
	AgentMemoryRuntimeState      *AgentMemoryRuntimeState
	BillingSource                string
	ModelTurnIntent              *AIChatModelTurnIntent
	ModelTurnIntentError         string
}
