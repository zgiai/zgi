package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/zgiai/zgi/api/internal/dto"
	workflowapp "github.com/zgiai/zgi/api/internal/modules/app/workflow"
	"gorm.io/datatypes"
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
	channelContext ChannelContextRuntimeProvider
	now            func() time.Time
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

func (s *RuntimeService) SubmitManual(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request ManualInvocationRequest) (*Run, bool, error) {
	query := strings.TrimSpace(request.Query)
	if !validScope(scope) || digitalHumanID == uuid.Nil || request.RequestID == uuid.Nil || query == "" ||
		!utf8.ValidString(query) || utf8.RuneCountInString(query) > maxManualQueryRunes {
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
		OccurredAt: s.now(), Context: map[string]any{"query": query},
	}
	if request.ConversationID != nil && *request.ConversationID != uuid.Nil {
		event.Context["conversation_id"] = request.ConversationID.String()
	}
	return s.SubmitInvocation(ctx, scope, event, query, request.ConversationID)
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
	if existing, loadErr := s.store.RunByIdempotency(ctx, scope.OrganizationID, validated.Target.ID, validated.IdempotencyKey); loadErr == nil {
		return existing, false, nil
	} else if !errors.Is(loadErr, ErrRunNotFound) {
		return nil, false, loadErr
	}

	resolved, err := s.resolver.Resolve(ctx, validated, nil)
	if err != nil {
		return nil, false, err
	}
	eventJSON, specJSON, snapshotJSON, err := marshalRunSnapshots(validated, resolved)
	if err != nil {
		return nil, false, err
	}
	now, runID := s.now(), uuid.New()
	actorID := scope.ActorID
	run := &Run{
		ID: runID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		DigitalHumanID: validated.Target.ID, DigitalHumanVersionID: resolved.Snapshot.DigitalHumanVersion,
		ActorID: &actorID, IdempotencyKey: validated.IdempotencyKey,
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
	if err := s.store.CreateRunWithOutbox(ctx, run, outbox); err != nil {
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

func (s *RuntimeService) GetRun(ctx context.Context, scope Scope, digitalHumanID, runID uuid.UUID) (*Run, error) {
	return s.store.RunByID(ctx, scope, digitalHumanID, runID)
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
	return &RunRuntimeLog{
		Detail: &detail,
		Steps:  channelContextRuntimeSteps(run, steps),
	}, nil
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
		_, _ = s.chat.StopMessage(ctx, runtimeScope(run), *run.MessageID)
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
	config, err := s.brain.RunConfig(resolved)
	if err != nil {
		_ = s.store.FailRun(ctx, run.ID, AppCodeRunConflict.String(), "Stored execution configuration is not executable.", s.now())
		return err
	}
	var channelContextPreparation *ChannelContextPreparation
	if s.channelContext != nil {
		channelContextPreparation, err = s.channelContext.Prepare(ctx, run, resolved, event, config.ModelProvider, config.Model)
		if err != nil {
			channelContextPreparation = &ChannelContextPreparation{
				SystemPromptAddendum: strings.Join([]string{
					"Channel conversation context is unavailable for this turn.",
					"Do not guess missing source content. State that the referenced channel text could not be read and ask the user to quote or reply directly to it.",
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
	chatRequest := runtimedto.ChatRequest{
		Query: run.Query, Model: config.Model, Provider: config.ModelProvider,
		Surface: runtimedto.RuntimeSurfaceExternalPageChat, UseMemory: false,
		SourceMessageID: &run.ID,
		OperationContext: map[string]any{
			"digital_human_run_id": run.ID.String(), "invocation_event_id": event.ID,
			"run_spec_revision": resolved.Spec.Revision, "execution_snapshot_revision": resolved.Snapshot.Revision,
		},
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
		Type: runtimemodel.ConversationCallerDigitalHuman, ID: &callerID, Source: digitalHumanConversationSourceForEvent(event),
	}
	if event.Origin.Kind == invocation.OriginChannel && run.ConversationID != nil {
		if _, err := s.chat.EnsureConversationForCaller(
			ctx, runtimeScope(run), caller, *run.ConversationID, "Channel conversation",
		); err != nil {
			_ = s.store.FailRun(ctx, run.ID, "digital_human.run.conversation_prepare_failed", "The channel conversation could not be prepared.", s.now())
			return err
		}
	}
	prepared, err := s.chat.PrepareConfiguredChat(ctx, runtimeScope(run), caller, config, chatRequest)
	if err != nil {
		_ = s.store.FailRun(ctx, run.ID, "digital_human.run.prepare_failed", "The DigitalHuman could not prepare this execution.", s.now())
		return err
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
		_ = s.store.FailRun(ctx, run.ID, "digital_human.run.execution_failed", "The DigitalHuman execution did not complete.", s.now())
		return err
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
	for _, section := range []struct {
		key     string
		allowed map[string]struct{}
	}{
		{key: "runtime_timeline", allowed: allowedTraceFields("runtime_id", "kind", "source", "source_id", "sequence", "created_at", "created_at_ms")},
		{key: "skill_invocations", allowed: allowedTraceFields("runtime_id", "invocation_id", "skill_id", "tool_name", "title", "kind", "status", "duration_ms", "created_at", "created_at_ms", "error_code", "approval_status", "workflow_run_id")},
		{key: "workflow_runs", allowed: allowedTraceFields("workflow_run_id", "task_id", "id", "workflow_id", "agent_id", "version", "status", "elapsed_time", "created_at", "created_at_ms")},
		{key: "conversation_artifacts", allowed: allowedTraceFields("artifact_id", "artifact_type", "invocation_id", "skill_id", "tool_name", "file_id", "tool_file_id", "upload_file_id", "filename", "extension", "mime_type", "transfer_method", "target", "lifecycle", "expires_at", "availability", "created_at")},
		{key: "generated_files", allowed: allowedTraceFields("artifact_id", "artifact_type", "invocation_id", "skill_id", "tool_name", "file_id", "tool_file_id", "upload_file_id", "filename", "extension", "mime_type", "transfer_method", "target", "lifecycle", "expires_at", "availability", "created_at")},
		{key: "channel_context", allowed: allowedTraceFields("status", "message_count", "author_count", "estimated_tokens", "truncated", "truncated_reason", "duration_ms", "provider_request_ids", "error_code")},
	} {
		items := traceMapSlice(metadata[section.key], section.allowed, 100)
		if len(items) > 0 {
			trace[section.key] = items
		}
	}
	encoded, err := json.Marshal(trace)
	if err != nil || len(encoded) > 256*1024 {
		return datatypes.JSON(`{}`)
	}
	return datatypes.JSON(encoded)
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
		message, _, loadErr := s.chat.GetMessageByCaller(ctx, runtimeScope(run), chatruntime.Caller{
			Type: runtimemodel.ConversationCallerDigitalHuman, ID: &callerID, Source: digitalHumanConversationSourceForRun(run),
		}, *run.MessageID)
		if loadErr != nil {
			combined = errors.Join(combined, loadErr)
			continue
		}
		if message == nil || message.Status != runtimemodel.MessageStatusCompleted {
			continue
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
	if event.Origin.Kind == invocation.OriginChannel {
		return digitalHumanChannelConversationSource
	}
	return digitalHumanConversationSource
}

func digitalHumanConversationSourceForRun(run *Run) string {
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
		snapshot.Subject != event.Target || snapshot.DigitalHumanVersion != run.DigitalHumanVersionID || snapshot.UseUserMemory || snapshot.UseAgentMemory {
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
	return chatruntime.Scope{OrganizationID: run.OrganizationID, WorkspaceID: &workspaceID, AccountID: actorID}
}

func runScope(run *Run) Scope {
	actorID := uuid.Nil
	if run.ActorID != nil {
		actorID = *run.ActorID
	}
	return Scope{OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, ActorID: actorID}
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
