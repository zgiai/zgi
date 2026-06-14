package service

import (
	"context"
	"mime/multipart"
	"time"

	"github.com/google/uuid"
	aichatdto "github.com/zgiai/zgi/api/internal/modules/aichat/dto"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	"github.com/zgiai/zgi/api/internal/modules/aichat/repository"
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
	staleActiveMessageTTL    = time.Hour
	staleActiveMessageError  = "stream interrupted before completion"
	streamEventsExpiredError = "stream events expired"
	titleGenerationTimeout   = 15 * time.Second
	runtimeContextMaxRunes   = 8000

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
	streamEventSkillLoadStart       = "skill_load_start"
	streamEventSkillLoadEnd         = "skill_load_end"
	streamEventSkillReferenceRead   = "skill_reference_read"
	streamEventSkillArtifactCreated = "skill_artifact_created"

	skillModeDisabled = "disabled"
	skillModeAuto     = "auto"
	skillModeRequired = "required"

	userMemoryContextBudgetChars = 4000
)

var defaultSystemSkillIDs = []string{
	skills.SkillTime,
	skills.SkillCalculator,
	skills.SkillFileGenerator,
}

type Scope struct {
	OrganizationID uuid.UUID
	AccountID      uuid.UUID
	WorkspaceID    *uuid.UUID
}

type Service interface {
	CreateConversation(ctx context.Context, scope Scope, title string) (*aichatmodel.Conversation, error)
	ListConversations(ctx context.Context, scope Scope, page, limit int) ([]*aichatmodel.Conversation, int64, error)
	GetConversation(ctx context.Context, scope Scope, id uuid.UUID) (*aichatmodel.Conversation, error)
	UpdateConversation(ctx context.Context, scope Scope, id uuid.UUID, req aichatdto.UpdateConversationRequest) (*aichatmodel.Conversation, error)
	DeleteConversation(ctx context.Context, scope Scope, id uuid.UUID) error
	ListMessages(ctx context.Context, scope Scope, conversationID uuid.UUID, page, limit int) ([]*aichatmodel.Message, int64, error)
	DeleteMessage(ctx context.Context, scope Scope, id uuid.UUID) error
	StopMessage(ctx context.Context, scope Scope, id uuid.UUID) (*aichatmodel.Message, error)
	StopConversation(ctx context.Context, scope Scope, id uuid.UUID) (*StopConversationResult, error)
	PrepareChat(ctx context.Context, scope Scope, req aichatdto.ChatRequest) (*PreparedChat, error)
	PrepareRootRegeneration(ctx context.Context, scope Scope, id uuid.UUID, req aichatdto.RegenerateMessageRequest) (*PreparedChat, error)
	RunPreparedStream(ctx context.Context, prepared *PreparedChat, onChunk func(string) error, onEvent ...func(StreamEvent) error) (*ChatResult, error)
	StreamConversationEvents(ctx context.Context, scope Scope, conversationID, messageID uuid.UUID, afterID string, onEvent func(StreamEvent) error) error
	ListSkills(ctx context.Context, scope Scope) ([]skills.SkillDiscoveryMetadata, error)
	GetSkill(ctx context.Context, scope Scope, skillID string) (*skills.SkillDiscoveryMetadata, error)
	GetSkillConfig(ctx context.Context, scope Scope) (*SkillConfig, error)
	UpdateSkillConfig(ctx context.Context, scope Scope, req aichatdto.UpdateSkillConfigRequest) (*SkillConfig, error)
	PreviewImportCustomSkill(ctx context.Context, scope Scope, fileHeader *multipart.FileHeader) (*SkillImportPreview, error)
	ConfirmCustomSkillImport(ctx context.Context, scope Scope, importID string, overwriteConfirmed bool) (*skills.SkillDiscoveryMetadata, error)
	CancelCustomSkillImportPreview(ctx context.Context, scope Scope, importID string) error
	DeleteSkill(ctx context.Context, scope Scope, skillID string) error
	CleanupStaleActiveMessages(ctx context.Context) (int64, error)
	CleanupExpiredCustomSkillImportPreviews(ctx context.Context) error
	MigrateWebAppConversation(ctx context.Context, scope Scope, sourceConversationID uuid.UUID) (*aichatmodel.Conversation, error)
}

type UserMemoryService interface {
	IsEnabled(ctx context.Context, accountID uuid.UUID) (bool, error)
	RenderContext(ctx context.Context, accountID uuid.UUID, budget int) (string, error)
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
	customSkillStorage customSkillStorage
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
) Service {
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
		customSkillStorage: newFilesystemCustomSkillStorage(customSkillStorageRoot),
	}
}

type PreparedChat struct {
	Conversation *aichatmodel.Conversation
	Message      *aichatmodel.Message
	LLMRequest   *adapter.ChatRequest
	ReplaceRoot  bool
	Scope        Scope
	ParentID     *uuid.UUID
	parts        *chatRequestParts
}

type ChatResult struct {
	Answer   string
	Metadata map[string]interface{}
	Usage    *adapter.Usage
}

type StopConversationResult struct {
	Conversation *aichatmodel.Conversation
	Message      *aichatmodel.Message
}

type SkillConfig struct {
	EnabledSkillIDs []string
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
	RuntimeContext               string
	ModelName                    string
	Provider                     string
	ProviderPtr                  *string
	Parameters                   map[string]interface{}
	ContextControl               map[string]interface{}
	Attachments                  *attachmentBundle
	ModelSupportsVision          bool
	FunctionCallingKnown         bool
	ModelSupportsFunctionCalling bool
	UseMemory                    bool
	SkillIDs                     []string
	ToolSkillIDs                 []string
	SkillMode                    string
}
