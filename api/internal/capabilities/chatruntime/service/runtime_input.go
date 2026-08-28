package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

const (
	maxRuntimeInputRunes                   = 16000
	runtimeInputClaimTTL                   = 10 * time.Minute
	runtimeInputRecoveryMax                = 100
	runtimeInputFollowUpBatchIDMetadataKey = "runtime_input_follow_up_batch_id"
	runtimeInputApplicationsMetadataKey    = "runtime_input_applications"
)

const runtimeInputCheckpointBeforeModelRound = "before_model_round"

type runtimeInputCheckpoint struct {
	Name       string
	ModelRound int
}

type claimedRuntimeInputBatch struct {
	id         uuid.UUID
	items      []*runtimemodel.RuntimeInput
	checkpoint runtimeInputCheckpoint
	onApplied  func(map[string]interface{}) error
}

const runtimeInputSystemInstruction = `The user submitted the following additional messages while this task was already running. Treat them as ordinary new user input, preserve their order, and decide from the full conversation how they affect the current task. Do not infer intent from keywords alone.`

func (s *service) SubmitRuntimeInput(ctx context.Context, scope Scope, caller Caller, config RunConfig, req SubmitRuntimeInputRequest) (*SubmitRuntimeInputResult, error) {
	if config.RuntimeInputPolicy != RuntimeInputPolicyCheckpointAndFollowUp {
		return nil, ErrRuntimeInputDisabled
	}
	if req.AdmissionMode != RuntimeInputAdmissionCurrentOrFollowUp && req.AdmissionMode != RuntimeInputAdmissionFollowUpOnly {
		return nil, fmt.Errorf("%w: invalid runtime input admission mode", ErrInvalidInput)
	}
	content := strings.TrimSpace(req.Content)
	if req.ClientInputID == uuid.Nil || req.ConversationID == uuid.Nil || req.TargetMessageID == uuid.Nil || content == "" || !utf8.ValidString(content) || utf8.RuneCountInString(content) > maxRuntimeInputRunes {
		return nil, fmt.Errorf("%w: invalid runtime input", ErrInvalidInput)
	}
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	conversation, err := s.getConversationByCallerScoped(ctx, scope, caller, req.ConversationID)
	if err != nil {
		return nil, err
	}
	message, err := s.repos.Message.GetScoped(ctx, req.TargetMessageID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	if message.ConversationID != conversation.ID {
		return nil, fmt.Errorf("%w: runtime input target belongs to another conversation", ErrInvalidInput)
	}

	input, created, err := s.repos.RuntimeInput.Submit(ctx, &runtimemodel.RuntimeInput{
		OrganizationID: scope.OrganizationID, WorkspaceID: cloneUUIDPointer(scope.WorkspaceID), AccountID: scope.AccountID,
		ConversationID: conversation.ID, TargetMessageID: message.ID, ClientInputID: req.ClientInputID, Content: content,
		ConsumptionMode: initialRuntimeInputConsumptionMode(req.AdmissionMode),
	})
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrRuntimeInputConversationIdle):
			return nil, ErrConversationIdle
		case errors.Is(err, repository.ErrRuntimeInputTargetMismatch):
			return nil, fmt.Errorf("%w: active message changed", ErrInvalidInput)
		default:
			return nil, mapRepoError(err)
		}
	}
	disposition := runtimeInputDisposition(input)
	if created {
		s.appendStreamEventBestEffort(ctx, message.ID, conversation.ID, streamEventRuntimeInputAccepted, runtimeInputEventPayload(input, disposition))
	}
	return &SubmitRuntimeInputResult{Input: input, Disposition: disposition, Created: created}, nil
}

func initialRuntimeInputConsumptionMode(mode RuntimeInputAdmissionMode) *string {
	if mode != RuntimeInputAdmissionFollowUpOnly {
		return nil
	}
	value := runtimemodel.RuntimeInputConsumptionFollowUp
	return &value
}

func runtimeInputDisposition(input *runtimemodel.RuntimeInput) RuntimeInputDisposition {
	if input != nil && input.ConsumptionMode != nil && *input.ConsumptionMode == runtimemodel.RuntimeInputConsumptionFollowUp {
		return RuntimeInputDispositionFollowUp
	}
	return RuntimeInputDispositionCurrentRun
}

func (s *service) ListRuntimeInputs(ctx context.Context, scope Scope, caller Caller, conversationID uuid.UUID) ([]*runtimemodel.RuntimeInput, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	if _, err := s.getConversationByCallerScoped(ctx, scope, caller, conversationID); err != nil {
		return nil, err
	}
	return s.repos.RuntimeInput.ListByConversation(ctx, conversationID, scope.OrganizationID, scope.AccountID)
}

func (s *service) GetRuntimeInputFollowUpContext(ctx context.Context, batchID uuid.UUID) (*RuntimeInputFollowUpContext, error) {
	if batchID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	items, err := s.repos.RuntimeInput.ListByBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 || items[0] == nil {
		return nil, ErrNotFound
	}
	first := items[0]
	conversation, err := s.repos.Conversation.GetScoped(ctx, first.ConversationID, first.OrganizationID, first.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	if conversation.CallerID == nil || *conversation.CallerID == uuid.Nil ||
		(conversation.CallerType != runtimemodel.ConversationCallerAgent && conversation.CallerType != runtimemodel.ConversationCallerDigitalHuman) {
		return nil, fmt.Errorf("%w: runtime input follow-up has an unsupported owner", ErrInvalidInput)
	}
	clientInputIDs := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		if item != nil && item.ClientInputID != uuid.Nil {
			clientInputIDs = append(clientInputIDs, item.ClientInputID)
		}
	}
	return &RuntimeInputFollowUpContext{
		BatchID: batchID, OrganizationID: first.OrganizationID, WorkspaceID: cloneUUIDPointer(first.WorkspaceID),
		AccountID: first.AccountID, CallerType: conversation.CallerType, CallerID: *conversation.CallerID,
		ConversationID: first.ConversationID, TargetMessageID: first.TargetMessageID,
		ClientInputIDs: clientInputIDs, Contents: runtimeInputContents(items), Source: conversation.Source,
		SourceWebAppID: cloneUUIDPointer(conversation.SourceWebAppID),
	}, nil
}

func (s *service) claimRuntimeInputsForCurrentRun(
	ctx context.Context,
	prepared *PreparedChat,
	checkpoint runtimeInputCheckpoint,
	onApplied func(map[string]interface{}) error,
) ([]adapter.Message, error) {
	if prepared == nil || prepared.Message == nil || prepared.Conversation == nil || prepared.RunConfig.RuntimeInputPolicy != RuntimeInputPolicyCheckpointAndFollowUp {
		return nil, nil
	}
	now := time.Now().UTC()
	batchID := uuid.New()
	items, err := s.repos.RuntimeInput.ClaimPending(ctx, prepared.Message.ID, runtimemodel.RuntimeInputConsumptionCurrentRun, &batchID, now, now.Add(runtimeInputClaimTTL))
	if err != nil || len(items) == 0 {
		return nil, err
	}
	prepared.runtimeInputClaimedBatches = append(prepared.runtimeInputClaimedBatches, claimedRuntimeInputBatch{
		id: batchID, items: items, checkpoint: checkpoint, onApplied: onApplied,
	})
	return runtimeInputModelMessages(items), nil
}

func (s *service) settleClaimedRuntimeInputBatches(ctx context.Context, prepared *PreparedChat, accepted bool) error {
	if prepared == nil || prepared.Message == nil || len(prepared.runtimeInputClaimedBatches) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for len(prepared.runtimeInputClaimedBatches) > 0 {
		batch := prepared.runtimeInputClaimedBatches[0]
		var err error
		if accepted {
			err = s.repos.RuntimeInput.MarkBatchConsumed(ctx, batch.id, prepared.Message.ID, now)
		} else {
			err = s.repos.RuntimeInput.ReleaseCurrentRunBatch(ctx, batch.id, now)
		}
		if err != nil {
			return err
		}
		if accepted {
			s.emitAppliedRuntimeInputBatch(ctx, prepared, batch, now)
		}
		prepared.runtimeInputClaimedBatches = prepared.runtimeInputClaimedBatches[1:]
	}
	return nil
}

func (s *service) emitAppliedRuntimeInputBatch(ctx context.Context, prepared *PreparedChat, batch claimedRuntimeInputBatch, appliedAt time.Time) {
	for _, input := range batch.items {
		if input == nil {
			continue
		}
		input.Status = runtimemodel.RuntimeInputStatusConsumed
		input.ConsumedMessageID = cloneUUIDPointer(&prepared.Message.ID)
		input.ConsumedAt = timePointer(appliedAt)
		payload := runtimeInputAppliedEventPayload(input, batch.checkpoint, appliedAt)
		if batch.onApplied == nil {
			s.appendStreamEventBestEffort(ctx, prepared.Message.ID, prepared.Conversation.ID, streamEventRuntimeInputApplied, payload)
			continue
		}
		if err := batch.onApplied(payload); err != nil {
			logger.WarnContext(ctx, "failed to publish applied chat runtime input", "input_id", input.ID.String(), err)
		}
	}
}

func runtimeInputModelMessages(items []*runtimemodel.RuntimeInput) []adapter.Message {
	if len(items) == 0 {
		return nil
	}
	messages := make([]adapter.Message, 0, len(items)+1)
	messages = append(messages, adapter.Message{Role: "system", Content: runtimeInputSystemInstruction})
	for _, item := range items {
		if item != nil && strings.TrimSpace(item.Content) != "" {
			messages = append(messages, adapter.Message{Role: "user", Content: item.Content})
		}
	}
	return messages
}

func runtimeInputFollowUpBatchID(messageID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("zgi.chat-runtime.follow-up:"+messageID.String()))
}

func (s *service) claimRuntimeInputsForFollowUp(ctx context.Context, repos *repository.Repositories, prepared *PreparedChat) (*uuid.UUID, error) {
	if prepared == nil || prepared.Message == nil || prepared.RunConfig.RuntimeInputPolicy != RuntimeInputPolicyCheckpointAndFollowUp {
		return nil, nil
	}
	now := time.Now().UTC()
	batchID := runtimeInputFollowUpBatchID(prepared.Message.ID)
	items, err := repos.RuntimeInput.ClaimPending(ctx, prepared.Message.ID, runtimemodel.RuntimeInputConsumptionFollowUp, &batchID, now, now.Add(runtimeInputClaimTTL))
	if err != nil || len(items) == 0 {
		return nil, err
	}
	return &batchID, nil
}

func (s *service) dispatchPreparedRuntimeInputFollowUp(ctx context.Context, prepared *PreparedChat) {
	if prepared == nil || prepared.RuntimeInputFollowUpBatchID == nil || s.runtimeInputDispatcher == nil ||
		!s.runtimeInputDispatcher.OwnsCallerType(prepared.Caller.Type) {
		return
	}
	batchID := *prepared.RuntimeInputFollowUpBatchID
	if err := s.runtimeInputDispatcher.EnqueueRuntimeInputFollowUp(context.WithoutCancel(ctx), batchID); err != nil {
		logger.WarnContext(ctx, "failed to enqueue chat runtime input follow-up", "batch_id", batchID.String(), err)
	}
}

func (s *service) PrepareRuntimeInputFollowUp(ctx context.Context, config RunConfig, batchID uuid.UUID) (*PreparedChat, error) {
	if config.RuntimeInputPolicy != RuntimeInputPolicyCheckpointAndFollowUp || batchID == uuid.Nil {
		return nil, ErrRuntimeInputDisabled
	}
	config.runtimeInputFollowUp = true
	items, err := s.repos.RuntimeInput.ListByBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	first := items[0]
	if first.Status == runtimemodel.RuntimeInputStatusCanceled || first.Status == runtimemodel.RuntimeInputStatusFailed {
		return nil, nil
	}
	conversation, err := s.repos.Conversation.GetScoped(ctx, first.ConversationID, first.OrganizationID, first.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	caller := callerFromConversation(conversation)
	scope := Scope{OrganizationID: first.OrganizationID, AccountID: first.AccountID, WorkspaceID: cloneUUIDPointer(first.WorkspaceID), SkipAccessCheck: conversation.Source == runtimemodel.ConversationSourceWebApp}
	if existing, loadErr := s.repos.Message.GetBySourceMessage(ctx, batchID); loadErr == nil && existing != nil {
		if existing.ConversationID != conversation.ID {
			return nil, fmt.Errorf("%w: runtime input follow-up message belongs to another conversation", ErrInvalidInput)
		}
		if existing.Status != runtimemodel.MessageStatusPending && existing.Status != runtimemodel.MessageStatusStreaming {
			return nil, nil
		}
		if conversation.RuntimeStatus != runtimemodel.ConversationRuntimeStatusStreaming || conversation.ActiveMessageID == nil || *conversation.ActiveMessageID != existing.ID {
			return nil, nil
		}
		prepared, prepareErr := s.prepareExistingRuntimeInputFollowUp(ctx, scope, caller, config, conversation, existing, items, batchID)
		if prepareErr != nil {
			return nil, prepareErr
		}
		if err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			txRepos := repository.NewRepositories(tx)
			if err := txRepos.Message.UpdateMetadataAnyStatus(ctx, existing.ID, prepared.Message.Metadata); err != nil {
				return err
			}
			return txRepos.RuntimeInput.MarkBatchConsumed(ctx, batchID, existing.ID, time.Now().UTC())
		}); err != nil {
			return nil, err
		}
		s.appendStreamEventBestEffort(ctx, prepared.Message.ID, prepared.Conversation.ID, streamEventRuntimeInputFollowUpStarted, map[string]interface{}{
			"conversation_id": prepared.Conversation.ID.String(), "message_id": prepared.Message.ID.String(), "batch_id": batchID.String(),
		})
		return prepared, nil
	} else if loadErr != nil && !errors.Is(loadErr, gorm.ErrRecordNotFound) {
		return nil, loadErr
	}
	for _, item := range items {
		if item == nil || item.ConversationID != first.ConversationID || item.Status != runtimemodel.RuntimeInputStatusClaimed || item.ConsumptionMode == nil || *item.ConsumptionMode != runtimemodel.RuntimeInputConsumptionFollowUp {
			return nil, fmt.Errorf("%w: invalid runtime input follow-up batch", ErrInvalidInput)
		}
	}
	contents := runtimeInputContents(items)
	prepared, err := s.PrepareConfiguredChat(ctx, scope, caller, config, runtimedto.ChatRequest{
		ConversationID: conversation.ID.String(), Query: strings.Join(contents, "\n\n"),
		SourceMessageID: &batchID,
	})
	if err != nil {
		return nil, err
	}
	metadata := copyStringAnyMap(prepared.Message.Metadata)
	metadata[runtimeInputFollowUpBatchIDMetadataKey] = batchID.String()
	prepared.Message.Metadata = metadata
	if err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		if err := txRepos.Message.UpdateMetadataAnyStatus(ctx, prepared.Message.ID, metadata); err != nil {
			return err
		}
		return txRepos.RuntimeInput.MarkBatchConsumed(ctx, batchID, prepared.Message.ID, time.Now().UTC())
	}); err != nil {
		return nil, err
	}
	s.appendStreamEventBestEffort(ctx, prepared.Message.ID, prepared.Conversation.ID, streamEventRuntimeInputFollowUpStarted, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(), "message_id": prepared.Message.ID.String(), "batch_id": batchID.String(),
	})
	return prepared, nil
}

func (s *service) FailRuntimeInputFollowUp(ctx context.Context, batchID uuid.UUID, cause error) error {
	if batchID == uuid.Nil {
		return ErrInvalidInput
	}
	failure := "runtime input follow-up failed"
	if cause != nil && strings.TrimSpace(cause.Error()) != "" {
		failure = cause.Error()
	}
	items, err := s.repos.RuntimeInput.MarkBatchFailed(ctx, batchID, failure, time.Now().UTC())
	if err != nil {
		return err
	}
	for _, input := range items {
		if input == nil {
			continue
		}
		messageID := input.TargetMessageID
		if input.ConsumedMessageID != nil {
			messageID = *input.ConsumedMessageID
		}
		s.appendStreamEventBestEffort(ctx, messageID, input.ConversationID, streamEventRuntimeInputFailed, runtimeInputEventPayload(input, RuntimeInputDispositionFollowUp))
	}
	return nil
}

func (s *service) prepareExistingRuntimeInputFollowUp(ctx context.Context, scope Scope, caller Caller, config RunConfig, conversation *runtimemodel.Conversation, message *runtimemodel.Message, items []*runtimemodel.RuntimeInput, batchID uuid.UUID) (*PreparedChat, error) {
	if conversation == nil || message == nil {
		return nil, ErrInvalidInput
	}
	req := runtimedto.ChatRequest{
		ConversationID:  conversation.ID.String(),
		Query:           strings.Join(runtimeInputContents(items), "\n\n"),
		SourceMessageID: &batchID,
	}
	if message.ParentID != nil {
		req.ParentID = message.ParentID.String()
	}
	resolvedConfig, parts, resolvedConversation, parentID, err := s.prepareConfiguredChatDefinition(ctx, scope, caller, config, req)
	if err != nil {
		return nil, err
	}
	metadata := copyStringAnyMap(message.Metadata)
	metadata[runtimeInputFollowUpBatchIDMetadataKey] = batchID.String()
	message.Metadata = metadata
	prepared := &PreparedChat{
		Conversation: resolvedConversation,
		Message:      message,
		Scope:        scope,
		Caller:       caller,
		RunConfig:    resolvedConfig,
		ParentID:     parentID,
		parts:        parts,
	}
	s.refreshInitialPageContext(ctx, prepared)
	return prepared, nil
}

func runtimeInputContents(items []*runtimemodel.RuntimeInput) []string {
	contents := make([]string, 0, len(items))
	for _, item := range items {
		if item != nil && strings.TrimSpace(item.Content) != "" {
			contents = append(contents, item.Content)
		}
	}
	return contents
}

func preserveRuntimeInputFollowUpMetadata(generated, existing map[string]interface{}) map[string]interface{} {
	if generated == nil {
		generated = map[string]interface{}{}
	}
	if value, ok := existing[runtimeInputFollowUpBatchIDMetadataKey]; ok {
		generated[runtimeInputFollowUpBatchIDMetadataKey] = value
	}
	return generated
}

func callerFromConversation(conversation *runtimemodel.Conversation) Caller {
	if conversation == nil {
		return Caller{}
	}
	return Caller{Type: conversation.CallerType, ID: cloneUUIDPointer(conversation.CallerID), ConversationType: conversation.ConversationType, Source: conversation.Source, SourceWebAppID: cloneUUIDPointer(conversation.SourceWebAppID)}
}

func (s *service) RecoverStaleRuntimeInputs(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	recovered, err := s.repos.RuntimeInput.RecoverStaleClaims(ctx, now)
	if err != nil {
		return 0, err
	}
	targets, err := s.repos.RuntimeInput.ListPendingInactiveTargets(ctx, runtimeInputRecoveryMax)
	if err != nil {
		return recovered, err
	}
	for _, targetMessageID := range targets {
		batchID := runtimeInputFollowUpBatchID(targetMessageID)
		items, claimErr := s.repos.RuntimeInput.ClaimPending(ctx, targetMessageID, runtimemodel.RuntimeInputConsumptionFollowUp, &batchID, now, now.Add(runtimeInputClaimTTL))
		if claimErr != nil {
			return recovered, claimErr
		}
		recovered += int64(len(items))
	}
	return recovered, nil
}

func (s *service) EnqueuePendingRuntimeInputFollowUps(ctx context.Context) (int, error) {
	if s.runtimeInputDispatcher == nil {
		return 0, nil
	}
	batches, err := s.repos.RuntimeInput.ListPendingFollowUpBatches(ctx, runtimeInputRecoveryMax)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, batchID := range batches {
		info, contextErr := s.GetRuntimeInputFollowUpContext(ctx, batchID)
		if contextErr != nil {
			logger.WarnContext(ctx, "failed to resolve chat runtime input follow-up owner", "batch_id", batchID.String(), contextErr)
			continue
		}
		if info == nil || !s.runtimeInputDispatcher.OwnsCallerType(info.CallerType) {
			continue
		}
		if err := s.runtimeInputDispatcher.EnqueueRuntimeInputFollowUp(ctx, batchID); err != nil {
			logger.WarnContext(ctx, "failed to recover chat runtime input follow-up", "batch_id", batchID.String(), err)
			continue
		}
		count++
	}
	return count, nil
}

func runtimeInputEventPayload(input *runtimemodel.RuntimeInput, disposition RuntimeInputDisposition) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	payload := map[string]interface{}{
		"input_id": input.ID.String(), "client_input_id": input.ClientInputID.String(),
		"conversation_id": input.ConversationID.String(), "target_message_id": input.TargetMessageID.String(),
		"sequence": input.Sequence, "status": input.Status, "disposition": disposition,
		"created_at": input.CreatedAt.Unix(),
	}
	if input.ConsumedMessageID != nil {
		payload["consumed_message_id"] = input.ConsumedMessageID.String()
	}
	return payload
}

func runtimeInputAppliedEventPayload(input *runtimemodel.RuntimeInput, checkpoint runtimeInputCheckpoint, appliedAt time.Time) map[string]interface{} {
	payload := runtimeInputEventPayload(input, RuntimeInputDispositionCurrentRun)
	if input == nil {
		return payload
	}
	payload["status"] = "applied_current_run"
	payload["content"] = input.Content
	messageID := input.TargetMessageID
	if input.ConsumedMessageID != nil && *input.ConsumedMessageID != uuid.Nil {
		messageID = *input.ConsumedMessageID
	}
	payload["message_id"] = messageID.String()
	payload["checkpoint"] = strings.TrimSpace(checkpoint.Name)
	if checkpoint.ModelRound > 0 {
		payload["model_round"] = checkpoint.ModelRound
	}
	payload["applied_at"] = appliedAt.Unix()
	return payload
}

func timePointer(value time.Time) *time.Time { return &value }
