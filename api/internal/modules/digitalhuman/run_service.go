package digitalhuman

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runspec"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/dto"
	workflowapp "github.com/zgiai/zgi/api/internal/modules/app/workflow"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	maxManualQueryRunes        = 32_000
	defaultOutboxBatch         = 50
	defaultOutboxLease         = 2 * time.Minute
	completedRunRecoveryWindow = 30 * time.Second
	defaultStaleRunWindow      = 10 * time.Minute
	channelFormalHistoryTurns  = 4
)

// ExecutionResolver seals the configuration used by one invocation.
type ExecutionResolver interface {
	Resolve(context.Context, invocation.Event, *uuid.UUID) (*ResolvedExecution, error)
}

// BrainRuntimeAdapter maps a sealed snapshot to the shared ChatRuntime.
type BrainRuntimeAdapter interface {
	RunConfig(*ResolvedExecution) (chatruntime.RunConfig, error)
}

// ChatRuntime is the narrow shared execution boundary used by DigitalHuman.
type ChatRuntime interface {
	EnsureConversationForCaller(context.Context, chatruntime.Scope, chatruntime.Caller, uuid.UUID, string) (*runtimemodel.Conversation, error)
	PrepareConfiguredChat(context.Context, chatruntime.Scope, chatruntime.Caller, chatruntime.RunConfig, runtimedto.ChatRequest) (*chatruntime.PreparedChat, error)
	RunPreparedStream(context.Context, *chatruntime.PreparedChat, func(string) error, ...func(chatruntime.StreamEvent) error) (*chatruntime.ChatResult, error)
	GetMessageByCaller(context.Context, chatruntime.Scope, chatruntime.Caller, uuid.UUID) (*runtimemodel.Message, *runtimemodel.Conversation, error)
	StopMessage(context.Context, chatruntime.Scope, uuid.UUID) (*runtimemodel.Message, error)
}

type idempotentCompletedMessageWriter interface {
	CreateCompletedMessageIfAbsent(context.Context, chatruntime.Scope, chatruntime.CreateCompletedMessageRequest) (*runtimemodel.Message, error)
}

type configuredRootRegenerationRuntime interface {
	PrepareConfiguredRootRegeneration(context.Context, chatruntime.Scope, chatruntime.Caller, chatruntime.RunConfig, uuid.UUID, runtimedto.RegenerateMessageRequest) (*chatruntime.PreparedChat, error)
}

type digitalHumanTaskApprovalRuntime interface {
	RunDigitalHumanTaskApprovalContinuation(context.Context, chatruntime.Scope, chatruntime.Caller, chatruntime.RunConfig, uuid.UUID, uuid.UUID, chatruntime.DigitalHumanTaskApprovalRequest, chatruntime.DigitalHumanTaskApprovalHandler, func(chatruntime.StreamEvent) error) (*chatruntime.ChatResult, error)
}

// WorkflowContinuationRunner is the shared workflow engine boundary used by
// Agent and DigitalHuman approval continuations.
type WorkflowContinuationRunner interface {
	ResumeApprovalWorkflow(context.Context, *approvalruntime.Form) error
	ResumeQuestionAnswerWorkflow(context.Context, string, map[string]interface{}) error
}

type workflowContinuationStreamRunner interface {
	ResumeApprovalWorkflowStream(context.Context, *approvalruntime.Form, func(string, map[string]interface{}) error) error
	ResumeQuestionAnswerWorkflowStream(context.Context, string, map[string]interface{}, func(string, map[string]interface{}) error) error
}

// RunDispatcher is a best-effort delivery accelerator. RunStore remains the
// source of truth for accepted and canceled executions.
type RunDispatcher interface {
	EnqueueRun(context.Context, uuid.UUID) error
	CancelRun(context.Context, uuid.UUID) error
}

// RuntimeService owns durable manual invocations and execution lifecycle.
type RuntimeService struct {
	store          RunStore
	resolver       ExecutionResolver
	brain          BrainRuntimeAdapter
	chat           ChatRuntime
	dispatcher     RunDispatcher
	transient      ChannelTransientDispatcher
	memory         MemoryRuntimeProvider
	tasks          TaskRuntimeProvider
	taskService    TaskHandlerService
	workflowRunner WorkflowContinuationRunner
	workflowDB     *gorm.DB
	// progressReader overrides the durable workflow state reader in tests.
	progressReader workflowProgressReader
	channelContext ChannelContextRuntimeProvider
	manualFiles    ManualInvocationFileValidator
	linux          LinuxExecutionRuntimeProvider
	// trustAuthorizer classifies console runs at admission (ADR 0004).
	trustAuthorizer MemoryPromotionAuthorizer
	now             func() time.Time
}

func NewRuntimeService(store RunStore, resolver ExecutionResolver, brain BrainRuntimeAdapter, chat ChatRuntime, dispatcher RunDispatcher, options ...RuntimeServiceOption) *RuntimeService {
	if store == nil || resolver == nil || brain == nil || chat == nil || dispatcher == nil {
		panic("DigitalHuman runtime requires store, resolver, brain adapter, ChatRuntime, and dispatcher")
	}
	service := &RuntimeService{
		store: store, resolver: resolver, brain: brain, chat: chat, dispatcher: dispatcher,
		now: func() time.Time { return time.Now().UTC() },
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

// RuntimeServiceOption configures optional subject-owned runtime capabilities.
type RuntimeServiceOption func(*RuntimeService)

// WithMemoryRuntime attaches DigitalHuman memory without enabling legacy memory.
func WithMemoryRuntime(provider MemoryRuntimeProvider) RuntimeServiceOption {
	return func(service *RuntimeService) { service.memory = provider }
}

// WithTaskRuntime exposes subject-owned task tools at the DigitalHuman edge.
func WithTaskRuntime(provider TaskRuntimeProvider) RuntimeServiceOption {
	return func(service *RuntimeService) {
		service.tasks = provider
		if typed, ok := provider.(interface{ TaskService() TaskHandlerService }); ok {
			service.taskService = typed.TaskService()
		}
	}
}

// WithChannelTransientDispatcher enables UX-only channel messages without
// adding them to the durable DigitalHuman delivery outbox.
func WithChannelTransientDispatcher(dispatcher ChannelTransientDispatcher) RuntimeServiceOption {
	return func(service *RuntimeService) { service.transient = dispatcher }
}

// WithChannelContextRuntime enables ephemeral provider-history preparation for
// channel invocations without affecting other runtime surfaces.
func WithChannelContextRuntime(provider ChannelContextRuntimeProvider) RuntimeServiceOption {
	return func(service *RuntimeService) { service.channelContext = provider }
}

// WithManualInvocationFileValidator validates account-owned console uploads
// before a durable run and its immutable invocation context are created.
func WithManualInvocationFileValidator(validator ManualInvocationFileValidator) RuntimeServiceOption {
	return func(service *RuntimeService) { service.manualFiles = validator }
}

// WithWorkflowContinuationRunner enables generic workflow approvals for the
// DigitalHuman surfaces while keeping task approvals on their compatibility
// path.
func WithWorkflowContinuationRunner(db *gorm.DB, runner WorkflowContinuationRunner) RuntimeServiceOption {
	return func(service *RuntimeService) {
		service.workflowDB = db
		service.workflowRunner = runner
	}
}

func (s *RuntimeService) SubmitManual(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request ManualInvocationRequest) (*Run, bool, error) {
	query := strings.TrimSpace(request.Query)
	if !validScope(scope) || digitalHumanID == uuid.Nil || request.RequestID == uuid.Nil || query == "" ||
		!utf8.ValidString(query) || utf8.RuneCountInString(query) > maxManualQueryRunes || request.MaxTaskTokens != 0 {
		return nil, false, ErrInvalidRequest
	}
	event := invocation.Event{
		ID: request.RequestID.String(), Type: invocation.EventTypeManual,
		Origin: invocation.Origin{Kind: invocation.OriginManual},
		Target: runtimeidentity.SubjectRef{
			Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID,
			OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		},
		Sender:     invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: scope.ActorID.String()},
		OccurredAt: s.now(), Context: map[string]any{"query": query, "surface": SurfaceConsole},
	}
	scope.PrincipalType = string(invocation.PrincipalAccount)
	scope.PrincipalID = scope.ActorID
	subjectID := scope.ActorID
	scope.UserSubjectKind = string(usersubject.Account)
	scope.UserSubjectID = &subjectID
	scope.Surface = SurfaceConsole
	if len(request.FileIDs) > 0 {
		if s.manualFiles == nil {
			return nil, false, ErrConsoleFileNotAccessible
		}
		fileIDs, err := s.manualFiles.Validate(ctx, scope, request.FileIDs)
		if err != nil {
			return nil, false, err
		}
		request.FileIDs = fileIDs
	}
	appendInvocationRequestContext(event.Context, request)
	if request.ConversationID != nil && *request.ConversationID != uuid.Nil {
		event.Context["conversation_id"] = request.ConversationID.String()
	}
	return s.SubmitInvocation(ctx, scope, event, query, request.ConversationID)
}

// SubmitAPI accepts an API-key principal while keeping the external user subject
// as the durable conversation and memory owner.
func (s *RuntimeService) SubmitAPI(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request ManualInvocationRequest) (*Run, bool, error) {
	return s.submitExternalSurface(ctx, scope, digitalHumanID, request, SurfaceAPI)
}

// SubmitWebApp accepts an authenticated or anonymous hosted-page invocation.
func (s *RuntimeService) SubmitWebApp(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request ManualInvocationRequest) (*Run, bool, error) {
	return s.submitExternalSurface(ctx, scope, digitalHumanID, request, SurfaceWebApp)
}

// SubmitRegeneration accepts a root-message replay request and persists it as
// a normal durable invocation. The worker performs the actual root replacement
// through ChatRuntime, so a queue retry or process restart cannot bypass the
// DigitalHuman execution snapshot and access boundary.
func (s *RuntimeService) SubmitRegeneration(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request RegenerateInvocationRequest) (*Run, bool, error) {
	if !validScope(scope) || digitalHumanID == uuid.Nil || request.MessageID == uuid.Nil {
		return nil, false, ErrInvalidRequest
	}
	surface := strings.TrimSpace(scope.Surface)
	if surface == "" {
		surface = SurfaceConsole
		scope.Surface = surface
	}
	if surface != SurfaceConsole && surface != SurfaceAPI && surface != SurfaceWebApp {
		return nil, false, ErrInvalidRequest
	}
	if surface == SurfaceConsole {
		scope.PrincipalType = string(invocation.PrincipalAccount)
		scope.PrincipalID = scope.ActorID
		subjectID := scope.ActorID
		scope.UserSubjectKind = string(usersubject.Account)
		scope.UserSubjectID = &subjectID
	}

	chatScope, caller := digitalHumanChatContext(scope, digitalHumanID)
	source, conversation, err := s.chat.GetMessageByCaller(ctx, chatScope, caller, request.MessageID)
	if err != nil {
		return nil, false, mapConversationRuntimeError(err)
	}
	if source == nil || conversation == nil {
		return nil, false, ErrConversationNotFound
	}
	if request.ConversationID != nil && *request.ConversationID != uuid.Nil && *request.ConversationID != conversation.ID {
		return nil, false, ErrConversationNotFound
	}
	query := strings.TrimSpace(source.Query)
	if request.Query != nil {
		query = strings.TrimSpace(*request.Query)
	}
	if query == "" || !utf8.ValidString(query) || utf8.RuneCountInString(query) > maxManualQueryRunes {
		return nil, false, ErrInvalidRequest
	}

	requestID := uuid.New()
	eventType := invocation.EventTypeAPI
	origin := invocation.Origin{Kind: invocation.OriginAPI, Provider: "zgi"}
	senderType := invocation.PrincipalType(scope.PrincipalType)
	if surface == SurfaceConsole {
		eventType = invocation.EventTypeManual
		origin = invocation.Origin{Kind: invocation.OriginManual}
		senderType = invocation.PrincipalAccount
		scope.PrincipalType = string(invocation.PrincipalAccount)
		scope.PrincipalID = scope.ActorID
	}
	contextValues := map[string]any{
		"query": query, "surface": surface, "request_id": requestID.String(),
		"regenerate_message_id": request.MessageID.String(),
	}
	if request.RuntimeContext != "" {
		contextValues["regenerate_runtime_context"] = request.RuntimeContext
	}
	if request.OperationContext != nil {
		contextValues["regenerate_operation_context"] = request.OperationContext
	}
	if request.Model != nil {
		contextValues["regenerate_model"] = *request.Model
	}
	if request.Provider != nil {
		contextValues["regenerate_provider"] = *request.Provider
	}
	if request.Parameters != nil {
		contextValues["regenerate_parameters"] = request.Parameters
	}
	if request.UseMemory != nil {
		contextValues["regenerate_use_memory"] = *request.UseMemory
	}
	if surface == SurfaceWebApp && scope.WebAppID != nil && *scope.WebAppID != uuid.Nil {
		contextValues["web_app_id"] = scope.WebAppID.String()
	}
	event := invocation.Event{
		ID: requestID.String(), Type: eventType, Origin: origin,
		Target:     runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID},
		Sender:     invocation.PrincipalRef{Type: senderType, ID: scope.PrincipalID.String()},
		OccurredAt: s.now(), Context: contextValues,
	}
	scope.Surface = surface
	return s.SubmitInvocation(ctx, scope, event, query, &conversation.ID)
}

func digitalHumanChatContext(scope Scope, digitalHumanID uuid.UUID) (chatruntime.Scope, chatruntime.Caller) {
	workspaceID := scope.WorkspaceID
	chatScope := chatruntime.Scope{OrganizationID: scope.OrganizationID, WorkspaceID: &workspaceID, AccountID: scope.ActorID, SkipAccessCheck: scope.Surface == SurfaceAPI || scope.Surface == SurfaceWebApp}
	caller := chatruntime.Caller{Type: runtimemodel.ConversationCallerDigitalHuman, ID: &digitalHumanID, Source: digitalHumanConversationSourceForSurface(scope.Surface)}
	if scope.Surface == SurfaceWebApp && scope.WebAppID != nil && *scope.WebAppID != uuid.Nil {
		caller.SourceWebAppID = cloneUUIDPointer(scope.WebAppID)
	}
	return chatScope, caller
}

func (s *RuntimeService) submitExternalSurface(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request ManualInvocationRequest, surface string) (*Run, bool, error) {
	query := strings.TrimSpace(request.Query)
	if !validScope(scope) || digitalHumanID == uuid.Nil || request.RequestID == uuid.Nil || query == "" ||
		!utf8.ValidString(query) || utf8.RuneCountInString(query) > maxManualQueryRunes ||
		scope.PrincipalID == uuid.Nil || (surface != SurfaceAPI && surface != SurfaceWebApp) ||
		request.MaxTaskTokens < 0 || request.MaxTaskTokens > 100000000 ||
		(request.MaxTaskTokens > 0 && surface != SurfaceAPI) {
		return nil, false, ErrInvalidRequest
	}
	eventID := request.RequestID.String()
	if scope.UserSubjectID != nil && *scope.UserSubjectID != uuid.Nil {
		eventID = scope.UserSubjectID.String() + ":" + eventID
	} else if surface == SurfaceWebApp {
		eventID = scope.PrincipalID.String() + ":" + eventID
	}
	event := invocation.Event{
		ID: eventID, Type: invocation.EventTypeAPI,
		Origin: invocation.Origin{Kind: invocation.OriginAPI, Provider: "zgi"},
		Target: runtimeidentity.SubjectRef{
			Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID,
			OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		},
		Sender:     invocation.PrincipalRef{Type: invocation.PrincipalType(scope.PrincipalType), ID: scope.PrincipalID.String()},
		OccurredAt: s.now(), Context: map[string]any{"query": query, "surface": surface, "request_id": request.RequestID.String()},
	}
	if surface == SurfaceWebApp && scope.WebAppID != nil && *scope.WebAppID != uuid.Nil {
		event.Context["web_app_id"] = scope.WebAppID.String()
	}
	appendInvocationRequestContext(event.Context, request)
	if request.ConversationID != nil && *request.ConversationID != uuid.Nil {
		event.Context["conversation_id"] = request.ConversationID.String()
	}
	scope.Surface = surface
	return s.SubmitInvocation(ctx, scope, event, query, request.ConversationID)
}

func appendInvocationRequestContext(contextValues map[string]any, request ManualInvocationRequest) {
	if request.ParentMessageID != nil && *request.ParentMessageID != uuid.Nil {
		contextValues["parent_message_id"] = request.ParentMessageID.String()
	}
	if len(request.FileIDs) > 0 {
		contextValues["file_ids"] = slices.Clone(request.FileIDs)
	}
	if request.MaxTaskTokens > 0 {
		contextValues["max_task_tokens"] = request.MaxTaskTokens
	}
}

// SubmitInvocation durably stores both the run and its outbox record before
// attempting queue delivery. A duplicate canonical event returns the original
// immutable run without resolving newer configuration.
func (s *RuntimeService) SubmitInvocation(ctx context.Context, scope Scope, event invocation.Event, query string, conversationID *uuid.UUID) (*Run, bool, error) {
	if !validScope(scope) || !utf8.ValidString(query) {
		return nil, false, ErrInvalidRequest
	}
	query = strings.TrimSpace(query)
	if query == "" || utf8.RuneCountInString(query) > maxManualQueryRunes {
		return nil, false, ErrInvalidRequest
	}
	validated, err := invocation.ValidateEvent(event)
	if err != nil || validated.Target.Type != runtimeidentity.SubjectTypeDigitalHuman ||
		validated.Target.OrganizationID != scope.OrganizationID || validated.Target.WorkspaceID != scope.WorkspaceID {
		return nil, false, ErrInvalidRequest
	}
	fingerprint, err := invocationRequestFingerprint(validated, query, conversationID, scope.UserSubjectID)
	if err != nil {
		return nil, false, err
	}
	if existing, loadErr := s.store.RunByIdempotency(ctx, scope.OrganizationID, validated.Target.ID, validated.IdempotencyKey); loadErr == nil {
		if existing.RequestFingerprint != "" && existing.RequestFingerprint != fingerprint {
			return nil, false, ErrRunConflict
		}
		return existing, false, nil
	} else if !errors.Is(loadErr, ErrRunNotFound) {
		return nil, false, loadErr
	}

	resolved, err := s.resolver.Resolve(ctx, validated, nil)
	if err != nil {
		return nil, false, err
	}
	// Only newly accepted, opted-in scheduler runs use the task execution
	// namespace. Historical runs keep their persisted source and conversation.
	if validated.Origin.Kind == invocation.OriginSchedule && resolved.Snapshot.ExecutionRuntime != nil && resolved.Snapshot.ExecutionRuntime.Enabled {
		validated.Context = maps.Clone(validated.Context)
		if validated.Context == nil {
			validated.Context = make(map[string]any)
		}
		validated.Context[backgroundExecutionEntryKey] = backgroundTaskEntry
		id := uuid.New()
		conversationID = &id
	}
	eventJSON, specJSON, snapshotJSON, err := marshalRunSnapshots(validated, resolved)
	if err != nil {
		return nil, false, err
	}
	now, runID := s.now(), uuid.New()
	principalType := normalizedPrincipalType(scope, validated)
	surface := normalizedInvocationSurface(scope, validated)
	actorID := persistedRunActorID(scope, principalType, surface)
	principalID := cloneUUIDPointer(&scope.PrincipalID)
	sourceTrust := s.classifySourceTrust(ctx, scope.OrganizationID, scope.WorkspaceID, surface, actorID, validated)
	run := &Run{
		ID: runID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		DigitalHumanID: validated.Target.ID, DigitalHumanVersionID: resolved.Snapshot.DigitalHumanVersion,
		ActorID: actorID, PrincipalType: principalType, PrincipalID: principalID,
		UserSubjectKind: strings.TrimSpace(scope.UserSubjectKind), UserSubjectID: cloneUUIDPointer(scope.UserSubjectID),
		Surface: surface, SourceTrust: sourceTrustPointer(sourceTrust), RequestFingerprint: fingerprint, IdempotencyKey: validated.IdempotencyKey,
		InvocationEvent: eventJSON, RunSpec: specJSON, RunSpecRevision: resolved.Spec.Revision,
		ExecutionSnapshot: snapshotJSON, ExecutionSnapshotRevision: resolved.Snapshot.Revision,
		ExecutionTrace: datatypes.JSON(`{}`),
		Query:          query, Status: RunStatusQueued, ConversationID: cloneUUIDPointer(conversationID),
		CreatedAt: now, UpdatedAt: now,
	}
	outbox := &RunOutbox{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		RunID: runID, Status: RunOutboxPending, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
	var createErr error
	instanceStore, admitsInstances := s.store.(endUserInstanceStore)
	if instance := endUserInstanceForRun(scope, run, now); instance != nil && admitsInstances {
		createErr = instanceStore.CreateRunWithOutboxForEndUser(ctx, run, outbox, instance)
	} else {
		createErr = s.store.CreateRunWithOutbox(ctx, run, outbox)
	}
	if err := createErr; err != nil {
		if errors.Is(err, ErrRunConflict) {
			existing, loadErr := s.store.RunByIdempotency(ctx, scope.OrganizationID, validated.Target.ID, validated.IdempotencyKey)
			if loadErr == nil {
				return existing, false, nil
			}
		}
		return nil, false, err
	}
	// Failure here is recoverable by Reconcile; acceptance must not be rolled back.
	_ = s.DispatchPending(ctx, defaultOutboxBatch)
	return run, true, nil
}

func invocationRequestFingerprint(event invocation.Event, query string, conversationID, subjectID *uuid.UUID) (string, error) {
	payload := struct {
		Query          string         `json:"query"`
		ConversationID *uuid.UUID     `json:"conversation_id,omitempty"`
		SubjectID      *uuid.UUID     `json:"subject_id,omitempty"`
		Context        map[string]any `json:"context,omitempty"`
	}{Query: query, ConversationID: cloneUUIDPointer(conversationID), SubjectID: cloneUUIDPointer(subjectID), Context: event.Context}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode DigitalHuman request fingerprint: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func normalizedPrincipalType(scope Scope, event invocation.Event) string {
	if value := strings.TrimSpace(scope.PrincipalType); value != "" {
		return value
	}
	return string(event.Sender.Type)
}

func normalizedInvocationSurface(scope Scope, event invocation.Event) string {
	if value := strings.TrimSpace(scope.Surface); value != "" {
		return value
	}
	switch event.Origin.Kind {
	case invocation.OriginChannel:
		return SurfaceChannel
	case invocation.OriginSchedule:
		return SurfaceTask
	case invocation.OriginAPI:
		return SurfaceAPI
	default:
		return SurfaceConsole
	}
}

func (s *RuntimeService) GetRun(ctx context.Context, scope Scope, digitalHumanID, runID uuid.UUID) (*Run, error) {
	return s.store.RunByID(ctx, scope, digitalHumanID, runID)
}

// DeliverTaskResult projects a completed scheduled task back into the
// originating ChatRuntime conversation. The scheduler owns the retry; the
// message writer owns idempotency via the task-run source ID.
func (s *RuntimeService) DeliverTaskResult(ctx context.Context, req TaskResultDeliveryRequest) error {
	if s == nil || s.chat == nil || req.OrganizationID == uuid.Nil || req.WorkspaceID == uuid.Nil ||
		req.DigitalHumanID == uuid.Nil || req.TaskID == uuid.Nil || req.TaskRunID == uuid.Nil ||
		req.ConversationID == uuid.Nil || (strings.TrimSpace(req.Answer) == "" && req.Report == nil) {
		return ErrInvalidRequest
	}
	writer, ok := s.chat.(idempotentCompletedMessageWriter)
	if !ok {
		return fmt.Errorf("chat runtime does not support idempotent task result delivery")
	}
	workspaceID := req.WorkspaceID
	scope := chatruntime.Scope{
		OrganizationID:  req.OrganizationID,
		WorkspaceID:     &workspaceID,
		AccountID:       req.AccountID,
		SkipAccessCheck: req.AccountID == uuid.Nil || req.UserSubjectKind == string(usersubject.APIEndUser),
	}
	metadata := map[string]interface{}{
		"digital_human_task_result": true,
		"digital_human_id":          req.DigitalHumanID.String(),
		"task_id":                   req.TaskID.String(),
		"task_run_id":               req.TaskRunID.String(),
		"source":                    strings.TrimSpace(req.Source),
	}
	query := "任务执行结果"
	if name := strings.TrimSpace(req.TaskName); name != "" {
		query = "任务执行结果：" + name
	}
	answer := strings.TrimSpace(req.Answer)
	if report := req.Report; report != nil {
		query = "任务汇报"
		if name := strings.TrimSpace(req.TaskName); name != "" {
			query = "任务汇报：" + name
		}
		answer = taskReportMessage(report)
		metadata["digital_human_task_report"] = map[string]interface{}{
			"report_id": report.ID.String(), "task_id": req.TaskID.String(), "task_name": strings.TrimSpace(req.TaskName),
			"outcome": string(report.Outcome), "summary": report.Summary,
		}
	}
	_, err := writer.CreateCompletedMessageIfAbsent(ctx, scope, chatruntime.CreateCompletedMessageRequest{
		ConversationID:  req.ConversationID,
		Query:           query,
		Answer:          answer,
		ModelProvider:   "digital-human-task",
		ModelName:       "task-result",
		Metadata:        metadata,
		SourceMessageID: &req.TaskRunID,
	})
	return err
}

// taskReportMessage renders a final report as the delivered message text. The
// report card in the client reads the structured metadata; this text keeps the
// conversation readable everywhere else.
func taskReportMessage(report *TaskReportDelivery) string {
	outcome := map[TaskReportOutcome]string{
		TaskReportDone: "已完成", TaskReportPartial: "部分完成", TaskReportNotDone: "未完成", TaskReportFailed: "执行失败",
	}[report.Outcome]
	parts := []string{"**结论：" + outcome + "**", strings.TrimSpace(report.Summary)}
	if body := strings.TrimSpace(report.Body); body != "" && body != strings.TrimSpace(report.Summary) {
		parts = append(parts, body)
	}
	if issues := strings.TrimSpace(report.Issues); issues != "" {
		parts = append(parts, "**问题：**\n"+issues)
	}
	return strings.Join(parts, "\n\n")
}

// GetRunRuntimeLog loads the ChatRuntime message linked to a durable run and
// projects its model/tool events through the same sanitizer used by Agent logs.
func (s *RuntimeService) GetRunRuntimeLog(ctx context.Context, scope Scope, digitalHumanID, runID uuid.UUID) (*RunRuntimeLog, error) {
	run, err := s.store.RunByID(ctx, scope, digitalHumanID, runID)
	if err != nil {
		return nil, err
	}
	if run.MessageID == nil || *run.MessageID == uuid.Nil {
		return &RunRuntimeLog{Steps: channelContextRuntimeSteps(run, nil)}, nil
	}
	callerID := run.DigitalHumanID
	message, conversation, err := s.chat.GetMessageByCaller(ctx, runtimeScope(run), chatruntime.Caller{
		Type:   runtimemodel.ConversationCallerDigitalHuman,
		ID:     &callerID,
		Source: digitalHumanConversationSourceForRun(run),
	}, *run.MessageID)
	if err != nil {
		return nil, mapConversationRuntimeError(err)
	}
	detail := workflowapp.BuildAgentRuntimeRunDetail(message, conversation)
	steps := workflowapp.BuildAgentRuntimeSteps(message)
	overlayRunRuntimeDetail(&detail, run)
	return &RunRuntimeLog{
		Detail:                       &detail,
		Steps:                        channelContextRuntimeSteps(run, steps),
		PendingConnectorConfirmation: pendingRunConnectorConfirmation(run, message, conversation),
		PendingWorkflowApproval:      pendingRunWorkflowApproval(run, message),
	}, nil
}

func overlayRunRuntimeDetail(detail *dto.AgentRuntimeRunDetail, run *Run) {
	if detail == nil || run == nil {
		return
	}
	detail.ID = run.ID.String()
	detail.CallerType = runtimemodel.ConversationCallerDigitalHuman
	detail.Status = string(run.Status)
	detail.Query = run.Query
	if run.ResultSummary != "" {
		detail.Answer = run.ResultSummary
	} else if run.Status == RunStatusFailed || run.Status == RunStatusCanceled || run.Status == RunStatusUnknown {
		detail.Answer = ""
	}
	if run.ErrorMessage != "" {
		detail.Error = run.ErrorMessage
	}
	if !run.CreatedAt.IsZero() {
		detail.CreatedAt = run.CreatedAt.Unix()
	}
	if run.CompletedAt != nil {
		finishedAt := run.CompletedAt.Unix()
		detail.FinishedAt = &finishedAt
	}
	if run.StartedAt != nil {
		end := run.UpdatedAt
		if run.CompletedAt != nil {
			end = *run.CompletedAt
		}
		if !end.Before(*run.StartedAt) {
			detail.ElapsedTime = float64(end.Sub(*run.StartedAt).Milliseconds())
		}
	}
}

func pendingRunWorkflowApproval(run *Run, message *runtimemodel.Message) *RunWorkflowApproval {
	if run == nil || message == nil || run.Status != RunStatusWaiting ||
		run.WaitingReason != runtimemodel.MessageStatusWaitingApproval ||
		message.Status != runtimemodel.MessageStatusWaitingApproval {
		return nil
	}
	runs, ok := interfaceSlice(message.Metadata["workflow_runs"])
	if !ok {
		return nil
	}
	for index := len(runs) - 1; index >= 0; index-- {
		workflowRun, ok := stringMap(runs[index])
		if !ok {
			continue
		}
		approval, ok := stringMap(workflowRun["approval"])
		if !ok || !approvalStatusPending(approval["status"]) {
			continue
		}
		allowed := true
		if value, exists := approval["ui_approval_allowed"].(bool); exists {
			allowed = value
		}
		if !allowed {
			return &RunWorkflowApproval{ApprovalDeliveryMode: "external", UIApprovalAllowed: false, ExternalApprovalRequired: true}
		}
		form, _ := stringMap(approval["approval_form"])
		token := firstString(approval["approval_token"])
		if token == "" {
			token = firstString(form["token"])
		}
		if token == "" || len(form) == 0 {
			return nil
		}
		return &RunWorkflowApproval{ApprovalToken: token, ApprovalForm: publicWorkflowApprovalForm(form, token), ApprovalDeliveryMode: "inline", UIApprovalAllowed: true}
	}
	return nil
}

func approvalStatusPending(value interface{}) bool {
	status := strings.ToLower(strings.TrimSpace(firstString(value)))
	return status == "" || status == "pending" || status == "pending_approval" || status == "waiting" || status == "waiting_approval"
}

func publicWorkflowApprovalForm(form map[string]interface{}, token string) map[string]interface{} {
	allowed := map[string]struct{}{
		"id": {}, "form_id": {}, "token": {}, "node_id": {}, "node_title": {}, "content": {},
		"fields": {}, "actions": {}, "submit_methods": {}, "resolved_default_values": {},
		"expiration_at": {}, "expires_at": {}, "task_approval": {},
	}
	result := make(map[string]interface{}, len(form)+1)
	for key, value := range form {
		if _, ok := allowed[key]; ok {
			result[key] = value
		}
	}
	result["token"] = token
	return result
}

func interfaceSlice(value interface{}) ([]interface{}, bool) {
	items, ok := value.([]interface{})
	return items, ok
}

func stringMap(value interface{}) (map[string]interface{}, bool) {
	items, ok := value.(map[string]interface{})
	return items, ok
}

func firstString(value interface{}) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

const (
	connectorConfirmationRequiredEventType = "connector_confirmation_required"
	connectorConfirmationResolvedEventType = "connector_confirmation_resolved"
	connectorConfirmationEventsMetadataKey = "connector_confirmation_events"
)

// pendingRunConnectorConfirmation projects the safe Connector confirmation that
// is holding a durable DigitalHuman run. It returns nil unless every server-
// owned coordinate agrees, so an ordinary absence never fails the endpoint. The
// projection is deliberately narrower than the stored event: it never returns
// the raw event, message metadata, provider output, operation input, or
// credentials, and Preview stays absent because the owner-scoped receipt status
// endpoint owns it.
func pendingRunConnectorConfirmation(
	run *Run,
	message *runtimemodel.Message,
	conversation *runtimemodel.Conversation,
) *connectorexecution.RuntimeConfirmation {
	if run == nil || message == nil || conversation == nil {
		return nil
	}
	if run.Status != RunStatusWaiting || run.WaitingReason != runtimemodel.MessageStatusWaitingApproval ||
		run.WaitingReceipt == nil || *run.WaitingReceipt == uuid.Nil {
		return nil
	}
	if run.DigitalHumanID == uuid.Nil || run.DigitalHumanVersionID == uuid.Nil ||
		run.ConversationID == nil || *run.ConversationID == uuid.Nil ||
		run.MessageID == nil || *run.MessageID == uuid.Nil {
		return nil
	}
	if message.ID == uuid.Nil || message.ConversationID == uuid.Nil || conversation.ID == uuid.Nil ||
		message.Status != runtimemodel.MessageStatusWaitingApproval {
		return nil
	}
	// The loaded message and conversation must be exactly the ones the run
	// linked, otherwise a stale or unrelated turn could borrow the projection.
	if message.ID != *run.MessageID || message.ConversationID != conversation.ID ||
		conversation.ID != *run.ConversationID {
		return nil
	}

	receipt := run.WaitingReceipt.String()
	events := connectorConfirmationEventRecords(message.Metadata)
	if len(events) == 0 || connectorConfirmationEventResolved(events, receipt) {
		return nil
	}
	var matched *connectorexecution.RuntimeConfirmation
	for _, event := range events {
		if strings.TrimSpace(metadataEventString(event["event_type"])) != connectorConfirmationRequiredEventType ||
			metadataEventString(event["route_receipt"]) != receipt {
			continue
		}
		if metadataEventString(event["conversation_id"]) != conversation.ID.String() ||
			metadataEventString(event["message_id"]) != message.ID.String() {
			return nil
		}
		// A second required event for the same receipt is ambiguous, so nothing
		// is trustworthy and the projection is withheld.
		if matched != nil {
			return nil
		}
		confirmation, ok := safeRunConnectorConfirmation(event, run, receipt)
		if !ok {
			return nil
		}
		matched = confirmation
	}
	return matched
}

// connectorConfirmationEventRecords extracts the stored confirmation events.
// Any element that is not a JSON object makes the whole list untrustworthy.
func connectorConfirmationEventRecords(metadata map[string]interface{}) []map[string]interface{} {
	if metadata == nil {
		return nil
	}
	raw, ok := metadata[connectorConfirmationEventsMetadataKey]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []interface{}:
		records := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			record, ok := item.(map[string]interface{})
			if !ok || record == nil {
				return nil
			}
			records = append(records, record)
		}
		return records
	case []map[string]interface{}:
		records := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if item == nil {
				return nil
			}
			records = append(records, item)
		}
		return records
	default:
		return nil
	}
}

func connectorConfirmationEventResolved(events []map[string]interface{}, receipt string) bool {
	for _, event := range events {
		if strings.TrimSpace(metadataEventString(event["event_type"])) == connectorConfirmationResolvedEventType &&
			metadataEventString(event["route_receipt"]) == receipt {
			return true
		}
	}
	return false
}

func metadataEventString(value interface{}) string {
	text, _ := value.(string)
	return text
}

// safeRunConnectorConfirmation decodes only the public RuntimeConfirmation
// fields from the server-authored event and re-validates them against the run.
// It never trusts event framing, provider output, or inferred presentation.
func safeRunConnectorConfirmation(
	event map[string]interface{},
	run *Run,
	receipt string,
) (*connectorexecution.RuntimeConfirmation, bool) {
	raw, ok := event["confirmation"]
	if !ok || raw == nil {
		return nil, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var confirmation connectorexecution.RuntimeConfirmation
	if err := json.Unmarshal(encoded, &confirmation); err != nil {
		return nil, false
	}
	if !validRunConnectorConfirmation(confirmation, run, receipt) {
		return nil, false
	}
	// Preview and the frozen operation authority are never part of this
	// projection. The JSON tags already omit them; drop them explicitly so no
	// in-memory copy can travel with the response.
	confirmation.Preview = nil
	confirmation.AcceptedOperation = nil
	return &confirmation, true
}

func validRunConnectorConfirmation(
	confirmation connectorexecution.RuntimeConfirmation,
	run *Run,
	receipt string,
) bool {
	if run == nil || run.DigitalHumanID == uuid.Nil || receipt == "" {
		return false
	}
	// The receipt must be the run's own frozen receipt, byte for byte.
	if string(confirmation.RouteReceipt) != receipt {
		return false
	}
	if confirmation.Risk != connectorexecution.RiskHigh && confirmation.Risk != connectorexecution.RiskUnclassified {
		return false
	}
	// Complete server-authored presentation is required for informed consent.
	// It is never reconstructed from provider output or event framing.
	if !confirmation.ConnectorID.Valid() || !confirmation.OperationID.Valid() ||
		strings.TrimSpace(confirmation.ConnectorName) == "" || strings.TrimSpace(confirmation.ActionName) == "" ||
		strings.TrimSpace(confirmation.Consequence.ExternalEffect) == "" {
		return false
	}
	target := confirmation.Target
	if target.Type != connectorcore.TargetTypeDigitalHuman ||
		target.ID != run.DigitalHumanID.String() ||
		target.EffectiveScope() != connectorcore.ScopePublished ||
		target.VersionID != run.DigitalHumanVersionID.String() ||
		target.Validate() != nil {
		return false
	}
	return true
}

func (s *RuntimeService) ListRuns(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, query ListQuery) (*RunListResult, error) {
	if !validScope(scope) || digitalHumanID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	var err error
	query, err = normalizeListQuery(scope, query)
	if err != nil {
		return nil, err
	}
	items, total, err := s.store.ListRuns(ctx, scope, digitalHumanID, query)
	if err != nil {
		return nil, err
	}
	return &RunListResult{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, HasMore: int64(query.Page*query.PageSize) < total}, nil
}

func (s *RuntimeService) CancelRun(ctx context.Context, scope Scope, digitalHumanID, runID uuid.UUID) (*Run, error) {
	if !validScope(scope) || digitalHumanID == uuid.Nil || runID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	run, err := s.store.RequestRunCancel(ctx, scope, digitalHumanID, runID, s.now())
	if err != nil {
		return nil, err
	}
	_ = s.dispatcher.CancelRun(ctx, runID)
	if run.MessageID != nil && *run.MessageID != uuid.Nil {
		stopped, stopErr := s.chat.StopMessage(ctx, runtimeScope(run), *run.MessageID)
		// StopMessage is the authoritative user-visible terminal transition for
		// an in-flight turn. Persist the matching durable terminal state here so
		// a successful user cancellation cannot drift into stale/unknown during
		// reconciliation.
		if stopErr == nil && stopped != nil && stopped.Status == runtimemodel.MessageStatusStopped {
			now := s.now()
			if completeErr := s.store.CompleteRun(ctx, run.ID, stopped.Answer, executionTraceFromMetadata(stopped.Metadata), now); completeErr == nil {
				run.Status, run.CompletedAt, run.UpdatedAt, run.ResultSummary = RunStatusCanceled, &now, now, stopped.Answer
			}
		}
	}
	return run, nil
}

// Execute claims a queued run exactly once. A worker crash after claim is not
// blindly replayed; Reconcile marks it unknown because side effects may exist.
func (s *RuntimeService) Execute(ctx context.Context, runID uuid.UUID) error {
	if runID == uuid.Nil {
		return ErrInvalidRequest
	}
	run, claimed, err := s.store.ClaimRun(ctx, runID, s.now())
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	resolved, event, err := decodeAndVerifyRun(run)
	if err != nil {
		_ = s.store.FailRun(ctx, run.ID, AppCodeRunConflict.String(), "Stored execution snapshot failed integrity validation.", s.now())
		return err
	}
	absorbed, err := s.absorbChannelRunIntoActiveTurn(ctx, run, event)
	if err != nil {
		_ = s.store.FailRun(ctx, run.ID, "digital_human.run.runtime_input_absorb_failed", "The channel message could not join the active conversation.", s.now())
		return err
	}
	if absorbed {
		return nil
	}
	config, err := s.brain.RunConfig(resolved)
	if err != nil {
		_ = s.store.FailRun(ctx, run.ID, AppCodeRunConflict.String(), "Stored execution configuration is not executable.", s.now())
		return err
	}
	s.attachAttachmentReader(&config, run, resolved, event, nil)
	if s.linux != nil {
		factory, prepareErr := s.linux.Prepare(ctx, run, resolved, event)
		if prepareErr != nil {
			_ = s.store.FailRun(ctx, run.ID, AppCodeRunConflict.String(), "The DigitalHuman Linux execution environment is not approved or available.", s.now())
			return prepareErr
		}
		config.RuntimeToolFactory = factory
	}
	config.ConnectorRunID = run.ID.String()
	config.ConnectorInteractive = event.Origin.Kind == invocation.OriginManual
	applyScheduledConnectorPolicy(&config, run, event)
	var channelContextPreparation *ChannelContextPreparation
	if s.channelContext != nil {
		channelContextPreparation, err = s.channelContext.Prepare(ctx, run, resolved, event, config.ModelProvider, config.Model)
		if err != nil {
			channelContextPreparation = &ChannelContextPreparation{
				SystemPromptAddendum: strings.Join([]string{
					"Channel conversation context is unavailable for this turn.",
					"Do not guess missing source content.",
					channelContextMissingSourceGuidance,
				}, "\n"),
				Diagnostics: ChannelContextDiagnostics{Status: string(channelruntime.ContextStatusUnavailable), ErrorCode: "channel_context_unavailable"},
			}
		}
		if channelContextPreparation != nil {
			if addendum := strings.TrimSpace(channelContextPreparation.SystemPromptAddendum); addendum != "" {
				config.SystemPrompt = strings.TrimSpace(config.SystemPrompt) + "\n\n" + addendum
			}
			config.RuntimeTools = append(config.RuntimeTools, channelContextPreparation.Tools...)
		}
	}
	if s.memory != nil {
		memoryPreparation, memoryErr := s.memory.Prepare(ctx, run, resolved, event)
		if memoryErr != nil {
			_ = s.store.FailRun(ctx, run.ID, "digital_human.memory.prepare_failed", "The DigitalHuman could not prepare its long-term memory context.", s.now())
			return memoryErr
		}
		if memoryPreparation != nil {
			if addendum := strings.TrimSpace(memoryPreparation.SystemPromptAddendum); addendum != "" {
				config.SystemPrompt = strings.TrimSpace(config.SystemPrompt) + "\n\n" + addendum
			}
			config.RuntimeTools = append(config.RuntimeTools, memoryPreparation.Tools...)
		}
	}
	if s.tasks != nil {
		taskPreparation, taskErr := s.tasks.Prepare(ctx, run, event)
		if taskErr != nil {
			_ = s.store.FailRun(ctx, run.ID, "digital_human.tasks.prepare_failed", "The DigitalHuman could not prepare its task tools.", s.now())
			return taskErr
		}
		if taskPreparation != nil {
			if addendum := strings.TrimSpace(taskPreparation.SystemPromptAddendum); addendum != "" {
				config.SystemPrompt = strings.TrimSpace(config.SystemPrompt) + "\n\n" + addendum
			}
			config.RuntimeTools = append(config.RuntimeTools, taskPreparation.Tools...)
		}
	}
	chatRequest := runtimedto.ChatRequest{
		Query: run.Query, Model: config.Model, Provider: config.ModelProvider,
		Surface: runtimedto.RuntimeSurfaceExternalPageChat, UseMemory: false,
		SourceMessageID: &run.ID,
		OperationContext: map[string]any{
			"digital_human_run_id": run.ID.String(), "invocation_event_id": event.ID,
			"run_spec_revision": resolved.Spec.Revision, "execution_snapshot_revision": resolved.Snapshot.Revision,
		},
	}
	if parentID, ok := stringContextValue(event.Context, "parent_message_id"); ok {
		chatRequest.ParentID = parentID
	}
	chatRequest.FileIDs = stringSliceContextValue(event.Context, "file_ids")
	if surface, ok := stringContextValue(event.Context, "surface"); ok && surface == SurfaceAPI {
		chatRequest.MaxTaskTokens = intContextValue(event.Context, "max_task_tokens")
	}
	if event.Origin.Kind == invocation.OriginChannel {
		excludedTexts := []string{run.Query}
		if channelContextPreparation != nil {
			excludedTexts = append(excludedTexts, channelContextPreparation.ContextTexts...)
		}
		chatRequest.HistoryPolicy = &runtimedto.ConversationHistoryPolicy{
			Mode: runtimedto.ConversationHistoryModeSupplemental, MaxTurns: channelFormalHistoryTurns,
			ExcludedTexts: excludedTexts,
		}
	}
	if run.ConversationID != nil {
		chatRequest.ConversationID = run.ConversationID.String()
	}
	callerID := run.DigitalHumanID
	caller := chatruntime.Caller{
		Type: runtimemodel.ConversationCallerDigitalHuman, ID: &callerID, RunID: &run.ID, Source: digitalHumanConversationSourceForRun(run),
	}
	if webAppID, ok := stringContextValue(event.Context, "web_app_id"); ok {
		if parsed, parseErr := uuid.Parse(webAppID); parseErr == nil && parsed != uuid.Nil {
			caller.SourceWebAppID = &parsed
		}
	}
	if (event.Origin.Kind == invocation.OriginChannel || caller.Source == backgroundTaskEntry) && run.ConversationID != nil {
		title := "Channel conversation"
		if caller.Source == backgroundTaskEntry {
			title = "Scheduled task"
		}
		if _, err := s.chat.EnsureConversationForCaller(
			ctx, runtimeScope(run), caller, *run.ConversationID, title,
		); err != nil {
			_ = s.store.FailRun(ctx, run.ID, "digital_human.run.conversation_prepare_failed", "The channel conversation could not be prepared.", s.now())
			return err
		}
	}
	var prepared *chatruntime.PreparedChat
	if batchID, followUp := runtimeInputFollowUpBatchID(event); followUp {
		followUpRuntime, ok := s.chat.(runtimeInputChatRuntime)
		if !ok {
			err = fmt.Errorf("digital human runtime input follow-up is unavailable")
		} else {
			prepared, err = followUpRuntime.PrepareRuntimeInputFollowUp(ctx, config, batchID)
		}
	} else if regenerateID, ok := stringContextValue(event.Context, "regenerate_message_id"); ok {
		regenerateMessageID, parseErr := uuid.Parse(regenerateID)
		if parseErr != nil || regenerateMessageID == uuid.Nil {
			err = ErrInvalidRequest
		} else if rootRuntime, rootOK := s.chat.(configuredRootRegenerationRuntime); !rootOK {
			err = fmt.Errorf("digital human root regeneration is unavailable")
		} else {
			regenerateRequest := regenerateRequestFromContext(event.Context)
			prepared, err = rootRuntime.PrepareConfiguredRootRegeneration(ctx, runtimeScope(run), caller, config, regenerateMessageID, regenerateRequest)
		}
	} else {
		prepared, err = s.chat.PrepareConfiguredChat(ctx, runtimeScope(run), caller, config, chatRequest)
	}
	if err != nil {
		if batchID, followUp := runtimeInputFollowUpBatchID(event); followUp {
			if followUpRuntime, ok := s.chat.(runtimeInputChatRuntime); ok {
				_ = followUpRuntime.FailRuntimeInputFollowUp(context.WithoutCancel(ctx), batchID, err)
			}
		}
		_ = s.store.FailRun(ctx, run.ID, "digital_human.run.prepare_failed", "The DigitalHuman could not prepare this execution.", s.now())
		return err
	}
	if prepared == nil {
		_ = s.store.CompleteRun(ctx, run.ID, "", datatypes.JSON(`{}`), s.now())
		return nil
	}
	if err := s.store.LinkRunMessage(ctx, run.ID, prepared.Conversation.ID, prepared.Message.ID, s.now()); err != nil {
		_, _ = s.chat.StopMessage(ctx, runtimeScope(run), prepared.Message.ID)
		_ = s.store.FailRun(ctx, run.ID, AppCodeRunConflict.String(), "The execution was canceled before it started.", s.now())
		return err
	}
	current, err := s.store.RunByID(ctx, runScope(run), run.DigitalHumanID, run.ID)
	if err != nil || current.Status == RunStatusCanceling || current.CancelRequestedAt != nil {
		_, _ = s.chat.StopMessage(ctx, runtimeScope(run), prepared.Message.ID)
		_ = s.store.CompleteRun(ctx, run.ID, "", datatypes.JSON(`{}`), s.now())
		return err
	}
	var eventCallbacks []func(chatruntime.StreamEvent) error
	policy := defaultChannelResponsePolicy()
	if resolved.Snapshot.ChannelResponsePolicy != nil {
		policy = *resolved.Snapshot.ChannelResponsePolicy
	}
	if event.Origin.Kind == invocation.OriginChannel && policy.IntermediateMessagesEnabled && s.transient != nil {
		progress := newChannelProgressCollector(s.transient, run.ID)
		eventCallbacks = append(eventCallbacks, func(streamEvent chatruntime.StreamEvent) error {
			return progress.Handle(ctx, streamEvent)
		})
	}
	result, err := s.chat.RunPreparedStream(ctx, prepared, nil, eventCallbacks...)
	if err != nil {
		if batchID, followUp := runtimeInputFollowUpBatchID(event); followUp {
			if followUpRuntime, ok := s.chat.(runtimeInputChatRuntime); ok {
				_ = followUpRuntime.FailRuntimeInputFollowUp(context.WithoutCancel(ctx), batchID, err)
			}
		}
		_ = s.store.FailRun(ctx, run.ID, "digital_human.run.execution_failed", "The DigitalHuman execution did not complete.", s.now())
		return err
	}
	if result != nil && (result.Status == runtimemodel.MessageStatusWaitingApproval || result.Status == runtimemodel.MessageStatusWaitingQuestion || result.Status == runtimemodel.MessageStatusWaitingClientAction || result.Status == runtimemodel.MessageStatusWaitingBudget) {
		waiter, ok := s.store.(interface {
			MarkRunWaiting(context.Context, uuid.UUID, string, time.Time) error
		})
		if !ok {
			return ErrRunConflict
		}
		return waiter.MarkRunWaiting(ctx, run.ID, result.Status, s.now())
	}
	if channelContextPreparation != nil {
		if result.Metadata == nil {
			result.Metadata = make(map[string]interface{})
		}
		result.Metadata["channel_context"] = []interface{}{channelContextPreparation.Diagnostics}
		recordChannelContextMetrics(ctx, channelContextPreparation.Diagnostics)
	}
	if finalizer, ok := s.memory.(memoryRuntimeFinalizer); ok {
		if err := finalizer.Finalize(ctx, run, prepared, result); err != nil {
			_ = s.store.FailRun(ctx, run.ID, "digital_human.memory.schedule_failed", "The DigitalHuman could not schedule its memory extraction task.", s.now())
			return err
		}
	}
	return s.store.CompleteRunWithDelivery(ctx, run.ID, result.Answer, executionTraceFromMetadata(result.Metadata), s.now())
}

func executionTraceFromMetadata(metadata map[string]interface{}) datatypes.JSON {
	trace := map[string]interface{}{}
	truncated := false
	for _, section := range []struct {
		key     string
		allowed map[string]struct{}
	}{
		{key: "runtime_timeline", allowed: allowedTraceFields("runtime_id", "caller_type", "kind", "source", "source_id", "sequence", "created_at", "created_at_ms", "authorization_status", "approval_status", "error_code")},
		{key: "skill_invocations", allowed: allowedTraceFields("runtime_id", "caller_type", "invocation_id", "skill_id", "tool_name", "title", "kind", "status", "duration_ms", "created_at", "created_at_ms", "resource_type", "resource_id", "authorization_status", "error_code", "approval_status", "workflow_run_id")},
		{key: "workflow_runs", allowed: allowedTraceFields("workflow_run_id", "caller_type", "task_id", "id", "workflow_id", "agent_id", "version", "status", "elapsed_time", "created_at", "created_at_ms", "approval_status", "authorization_status", "error_code")},
		{key: "conversation_artifacts", allowed: allowedTraceFields("artifact_id", "artifact_type", "invocation_id", "skill_id", "tool_name", "file_id", "tool_file_id", "upload_file_id", "filename", "extension", "mime_type", "transfer_method", "target", "lifecycle", "expires_at", "availability", "created_at")},
		{key: "generated_files", allowed: allowedTraceFields("artifact_id", "artifact_type", "invocation_id", "skill_id", "tool_name", "file_id", "tool_file_id", "upload_file_id", "filename", "extension", "mime_type", "transfer_method", "target", "lifecycle", "expires_at", "availability", "created_at")},
		{key: "channel_context", allowed: allowedTraceFields("status", "message_count", "author_count", "estimated_tokens", "truncated", "truncated_reason", "duration_ms", "provider_request_ids", "error_code")},
	} {
		items := traceMapSlice(metadata[section.key], section.allowed, 100)
		if traceSliceLength(metadata[section.key]) > 100 {
			truncated = true
		}
		if len(items) > 0 {
			trace[section.key] = items
		}
	}
	if truncated {
		trace["truncated"] = true
	}
	encoded, err := json.Marshal(trace)
	if err != nil || len(encoded) > 256*1024 {
		return datatypes.JSON(`{"truncated":true}`)
	}
	return datatypes.JSON(encoded)
}

func traceSliceLength(value interface{}) int {
	if raw, ok := value.([]interface{}); ok {
		return len(raw)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	var raw []interface{}
	if json.Unmarshal(encoded, &raw) != nil {
		return 0
	}
	return len(raw)
}

func allowedTraceFields(fields ...string) map[string]struct{} {
	allowed := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		allowed[field] = struct{}{}
	}
	return allowed
}

func traceMapSlice(value interface{}, allowed map[string]struct{}, limit int) []map[string]interface{} {
	raw, ok := value.([]interface{})
	if !ok {
		encoded, err := json.Marshal(value)
		if err != nil || json.Unmarshal(encoded, &raw) != nil {
			return nil
		}
	}
	if len(raw) > limit {
		raw = raw[:limit]
	}
	items := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		encoded, err := json.Marshal(item)
		if err != nil {
			continue
		}
		var source map[string]interface{}
		if json.Unmarshal(encoded, &source) != nil {
			continue
		}
		projected := make(map[string]interface{})
		for key, value := range source {
			if _, keep := allowed[key]; keep {
				projected[key] = value
			}
		}
		if len(projected) > 0 {
			items = append(items, projected)
		}
	}
	return items
}

func channelContextRuntimeSteps(run *Run, steps []dto.AgentRuntimeStep) []dto.AgentRuntimeStep {
	if run == nil || len(run.ExecutionTrace) == 0 {
		return steps
	}
	var trace map[string]interface{}
	if json.Unmarshal(run.ExecutionTrace, &trace) != nil {
		return steps
	}
	items := traceMapSlice(trace["channel_context"], allowedTraceFields(
		"status", "message_count", "author_count", "estimated_tokens", "truncated",
		"truncated_reason", "duration_ms", "provider_request_ids", "error_code",
	), 1)
	if len(items) == 0 {
		return steps
	}
	status, _ := items[0]["status"].(string)
	errorCode, _ := items[0]["error_code"].(string)
	stepStatus := "success"
	if status == string(channelruntime.ContextStatusUnavailable) && errorCode == "channel_context_not_supported" {
		stepStatus = "skipped"
	} else if status == string(channelruntime.ContextStatusUnavailable) {
		stepStatus = "failed"
	}
	createdAt := run.CreatedAt.Unix()
	contextStep := dto.AgentRuntimeStep{
		ID: "channel-context-" + run.ID.String(), Index: 1, Type: "channel_context",
		Title: "Channel context preparation", Status: stepStatus, Output: items[0], CreatedAt: &createdAt,
	}
	if duration, ok := numericTraceValue(items[0]["duration_ms"]); ok {
		contextStep.ElapsedTime = duration
	}
	result := make([]dto.AgentRuntimeStep, 0, len(steps)+1)
	inserted := false
	for _, step := range steps {
		if !inserted && step.Type != "user_input" {
			result = append(result, contextStep)
			inserted = true
		}
		step.Index = len(result) + 1
		result = append(result, step)
	}
	if !inserted {
		result = append(result, contextStep)
	}
	for index := range result {
		result[index].Index = index + 1
	}
	return result
}

func numericTraceValue(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int64:
		return float64(typed), true
	case int:
		return float64(typed), true
	default:
		return 0, false
	}
}

// DispatchPending leases durable outbox rows and sends deterministic queue
// deliveries. Delivery errors leave rows retryable without exposing secrets.
func (s *RuntimeService) DispatchPending(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = defaultOutboxBatch
	}
	items, err := s.store.ClaimRunOutboxBatch(ctx, limit, s.now())
	if err != nil {
		return err
	}
	var combined error
	for _, item := range items {
		if err := s.dispatcher.EnqueueRun(ctx, item.RunID); err != nil {
			retryAt := s.now().Add(outboxBackoff(item.AttemptCount))
			if retryErr := s.store.RetryRunOutbox(ctx, item.ID, retryAt, "queue delivery failed"); retryErr != nil {
				combined = errors.Join(combined, retryErr)
			}
			combined = errors.Join(combined, err)
			continue
		}
		if err := s.store.ConfirmRunOutbox(ctx, item.ID, s.now()); err != nil {
			combined = errors.Join(combined, err)
		}
	}
	return combined
}

func (s *RuntimeService) Reconcile(ctx context.Context) error {
	now := s.now()
	return errors.Join(
		s.store.RequeueStaleRunOutbox(ctx, now.Add(-defaultOutboxLease), now),
		s.DispatchPending(ctx, defaultOutboxBatch),
		s.recoverCompletedRuns(ctx, now.Add(-completedRunRecoveryWindow), defaultOutboxBatch),
		s.store.MarkStaleRunsUnknown(ctx, now.Add(-defaultStaleRunWindow), now),
	)
}

// recoverCompletedRuns closes the crash window after ChatRuntime has persisted
// a completed answer but before the DigitalHuman run and channel delivery are
// committed. It reuses that stored answer and never invokes the model again.
func (s *RuntimeService) recoverCompletedRuns(ctx context.Context, updatedBefore time.Time, limit int) error {
	runs, err := s.store.ListRecoverableRuns(ctx, updatedBefore, limit)
	if err != nil {
		return err
	}
	var combined error
	for _, run := range runs {
		if run == nil || run.MessageID == nil || *run.MessageID == uuid.Nil || run.ActorID == nil || *run.ActorID == uuid.Nil {
			continue
		}
		callerID := run.DigitalHumanID
		message, conversation, loadErr := s.chat.GetMessageByCaller(ctx, runtimeScope(run), chatruntime.Caller{
			Type: runtimemodel.ConversationCallerDigitalHuman, ID: &callerID, Source: digitalHumanConversationSourceForRun(run),
		}, *run.MessageID)
		if loadErr != nil {
			combined = errors.Join(combined, loadErr)
			continue
		}
		if message == nil || message.Status != runtimemodel.MessageStatusCompleted {
			continue
		}
		if workflowContinuationEndedFailed(message.Metadata) {
			// A failed workflow's summary completes the message, but the run
			// settles with the workflow, as the continuation does.
			if failErr := s.store.FailRun(ctx, run.ID, workflowContinuationFailedCode, workflowContinuationFailedMessage, s.now()); failErr != nil && !errors.Is(failErr, ErrRunConflict) {
				combined = errors.Join(combined, failErr)
			}
			continue
		}
		if finalizer, ok := s.memory.(memoryRuntimeFinalizer); ok {
			prepared := &chatruntime.PreparedChat{Conversation: conversation, Message: message}
			result := &chatruntime.ChatResult{Answer: message.Answer, Metadata: message.Metadata, Status: message.Status}
			if finalizeErr := finalizer.Finalize(ctx, run, prepared, result); finalizeErr != nil {
				combined = errors.Join(combined, finalizeErr)
				continue
			}
		}
		if completeErr := s.store.CompleteRunWithDelivery(
			ctx, run.ID, message.Answer, executionTraceFromMetadata(message.Metadata), s.now(),
		); completeErr != nil && !errors.Is(completeErr, ErrRunConflict) {
			combined = errors.Join(combined, completeErr)
		}
	}
	return combined
}

func digitalHumanConversationSourceForEvent(event invocation.Event) string {
	if surface, ok := stringContextValue(event.Context, "surface"); ok {
		return digitalHumanConversationSourceForSurface(surface)
	}
	if event.Origin.Kind == invocation.OriginChannel {
		return digitalHumanChannelConversationSource
	}
	return digitalHumanConversationSource
}

func digitalHumanConversationSourceForRun(run *Run) string {
	if run != nil && run.Surface == SurfaceTask {
		var event invocation.Event
		if json.Unmarshal(run.InvocationEvent, &event) == nil {
			if entry, _ := stringContextValue(event.Context, backgroundExecutionEntryKey); entry == backgroundTaskEntry {
				return backgroundTaskEntry
			}
		}
	}
	if run != nil && strings.TrimSpace(run.Surface) != "" {
		return digitalHumanConversationSourceForSurface(run.Surface)
	}
	if run == nil || len(run.InvocationEvent) == 0 {
		return digitalHumanConversationSource
	}
	var event invocation.Event
	if err := json.Unmarshal(run.InvocationEvent, &event); err != nil {
		return digitalHumanConversationSource
	}
	return digitalHumanConversationSourceForEvent(event)
}

func marshalRunSnapshots(event invocation.Event, resolved *ResolvedExecution) (datatypes.JSON, datatypes.JSON, datatypes.JSON, error) {
	if resolved == nil {
		return nil, nil, nil, ErrInvalidRequest
	}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode DigitalHuman invocation: %w", err)
	}
	specJSON, err := json.Marshal(resolved.Spec)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode DigitalHuman RunSpec: %w", err)
	}
	snapshotJSON, err := json.Marshal(resolved.Snapshot)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode DigitalHuman execution snapshot: %w", err)
	}
	return eventJSON, specJSON, snapshotJSON, nil
}

func decodeAndVerifyRun(run *Run) (*ResolvedExecution, invocation.Event, error) {
	if run == nil || run.ID == uuid.Nil {
		return nil, invocation.Event{}, ErrRunNotFound
	}
	var event invocation.Event
	if err := json.Unmarshal(run.InvocationEvent, &event); err != nil {
		return nil, invocation.Event{}, fmt.Errorf("decode invocation event: %w", err)
	}
	event, err := invocation.ValidateEvent(event)
	if err != nil {
		return nil, invocation.Event{}, err
	}
	var spec runspec.Spec
	if err := json.Unmarshal(run.RunSpec, &spec); err != nil {
		return nil, invocation.Event{}, fmt.Errorf("decode RunSpec: %w", err)
	}
	spec, err = runspec.Seal(spec)
	if err != nil {
		return nil, invocation.Event{}, err
	}
	var snapshot ExecutionSnapshot
	if err := json.Unmarshal(run.ExecutionSnapshot, &snapshot); err != nil {
		return nil, invocation.Event{}, fmt.Errorf("decode execution snapshot: %w", err)
	}
	snapshotRevision, err := executionSnapshotRevision(snapshot)
	if err != nil {
		return nil, invocation.Event{}, err
	}
	if event.Target.ID != run.DigitalHumanID || event.Target.OrganizationID != run.OrganizationID || event.Target.WorkspaceID != run.WorkspaceID ||
		event.IdempotencyKey != run.IdempotencyKey || spec.InvocationID != event.ID || spec.Subject != event.Target ||
		spec.Revision != run.RunSpecRevision || snapshotRevision != snapshot.Revision || snapshot.Revision != run.ExecutionSnapshotRevision ||
		snapshot.Subject != event.Target || snapshot.DigitalHumanVersion != run.DigitalHumanVersionID || snapshot.UseAgentMemory {
		return nil, invocation.Event{}, ErrRunConflict
	}
	return &ResolvedExecution{Spec: spec, Snapshot: snapshot}, event, nil
}

func runtimeScope(run *Run) chatruntime.Scope {
	workspaceID := run.WorkspaceID
	actorID := uuid.Nil
	if run.ActorID != nil {
		actorID = *run.ActorID
	}
	if run.UserSubjectID != nil && *run.UserSubjectID != uuid.Nil {
		actorID = *run.UserSubjectID
	} else if actorID == uuid.Nil && run.PrincipalID != nil {
		actorID = *run.PrincipalID
	}
	return chatruntime.Scope{
		OrganizationID:  run.OrganizationID,
		WorkspaceID:     &workspaceID,
		AccountID:       actorID,
		SkipAccessCheck: run.Surface == SurfaceAPI || run.Surface == SurfaceWebApp || run.Surface == SurfaceTask,
	}
}

func stringContextValue(values map[string]any, key string) (string, bool) {
	value, ok := values[key].(string)
	value = strings.TrimSpace(value)
	return value, ok && value != ""
}

func intContextValue(values map[string]any, key string) int {
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(value))
		return parsed
	default:
		return 0
	}
}

func stringSliceContextValue(values map[string]any, key string) []string {
	raw, ok := values[key].([]any)
	if !ok {
		if typed, typedOK := values[key].([]string); typedOK {
			return slices.Clone(typed)
		}
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if value, ok := item.(string); ok && strings.TrimSpace(value) != "" {
			result = append(result, strings.TrimSpace(value))
		}
	}
	return result
}

func regenerateRequestFromContext(values map[string]any) runtimedto.RegenerateMessageRequest {
	request := runtimedto.RegenerateMessageRequest{}
	if query, ok := stringContextValue(values, "query"); ok {
		request.Query = &query
	}
	if runtimeContext, ok := stringContextValue(values, "regenerate_runtime_context"); ok {
		request.RuntimeContext = runtimeContext
	}
	if operationContext, ok := values["regenerate_operation_context"].(map[string]any); ok {
		request.OperationContext = operationContext
	}
	if model, ok := stringContextValue(values, "regenerate_model"); ok {
		request.Model = &model
	}
	if provider, ok := stringContextValue(values, "regenerate_provider"); ok {
		request.Provider = &provider
	}
	if parameters, ok := values["regenerate_parameters"].(map[string]any); ok {
		request.Parameters = parameters
	}
	if useMemory, ok := values["regenerate_use_memory"].(bool); ok {
		request.UseMemory = &useMemory
	}
	request.Surface, _ = values["surface"].(string)
	return request
}

func runScope(run *Run) Scope {
	actorID := uuid.Nil
	if run.ActorID != nil {
		actorID = *run.ActorID
	} else if run.PrincipalID != nil {
		actorID = *run.PrincipalID
	}
	return Scope{
		OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, ActorID: actorID,
		PrincipalType: run.PrincipalType, PrincipalID: valueOrNil(run.PrincipalID),
		UserSubjectKind: run.UserSubjectKind, UserSubjectID: cloneUUIDPointer(run.UserSubjectID), Surface: run.Surface,
	}
}

func applyScheduledConnectorPolicy(config *chatruntime.RunConfig, run *Run, event invocation.Event) {
	if config != nil && run != nil && run.Surface == SurfaceTask && event.Origin.Kind == invocation.OriginSchedule && run.UserSubjectKind == string(usersubject.APIEndUser) {
		config.ConnectorToolsDisabled = true
	}
}

func persistedRunActorID(scope Scope, principalType, surface string) *uuid.UUID {
	if scope.ActorID == uuid.Nil || surface == SurfaceAPI ||
		(surface == SurfaceWebApp && principalType != string(invocation.PrincipalAccount)) ||
		(surface == SurfaceTask && principalType == string(invocation.PrincipalSystem) &&
			(scope.UserSubjectKind != string(usersubject.Account) || scope.UserSubjectID == nil || *scope.UserSubjectID != scope.ActorID)) {
		return nil
	}
	return cloneUUIDPointer(&scope.ActorID)
}

func valueOrNil(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.Nil
	}
	return *value
}

func cloneUUIDPointer(value *uuid.UUID) *uuid.UUID {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func outboxBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<(attempt-1)) * 5 * time.Second
}
