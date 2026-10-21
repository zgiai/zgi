package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
)

const runtimeInputFollowUpBatchContextKey = "runtime_input_follow_up_batch_id"

type runtimeInputChatRuntime interface {
	GetRuntimeInputFollowUpContext(context.Context, uuid.UUID) (*chatruntime.RuntimeInputFollowUpContext, error)
	PrepareRuntimeInputFollowUp(context.Context, chatruntime.RunConfig, uuid.UUID) (*chatruntime.PreparedChat, error)
	FailRuntimeInputFollowUp(context.Context, uuid.UUID, error) error
}

type channelRuntimeInputChat interface {
	GetConversationByCaller(context.Context, chatruntime.Scope, chatruntime.Caller, uuid.UUID) (*runtimemodel.Conversation, error)
	SubmitRuntimeInput(context.Context, chatruntime.Scope, chatruntime.Caller, chatruntime.RunConfig, chatruntime.SubmitRuntimeInputRequest) (*chatruntime.SubmitRuntimeInputResult, error)
}

// SubmitChannelRuntimeInput serializes a new callback behind the active turn
// for the same external conversation. Only a group message from the same
// sender may be claimed by a safe checkpoint in that active turn; private
// messages and other group members always become a deterministic follow-up.
func (s *RuntimeService) SubmitChannelRuntimeInput(ctx context.Context, scope Scope, digitalHumanID, conversationID uuid.UUID, request ChannelRuntimeInputRequest) (bool, error) {
	if !validScope(scope) || digitalHumanID == uuid.Nil || conversationID == uuid.Nil || request.ClientInputID == uuid.Nil || strings.TrimSpace(request.Content) == "" {
		return false, ErrInvalidRequest
	}
	chat, ok := s.chat.(channelRuntimeInputChat)
	if !ok {
		return false, nil
	}
	workspaceID := scope.WorkspaceID
	runtimeScope := chatruntime.Scope{OrganizationID: scope.OrganizationID, WorkspaceID: &workspaceID, AccountID: scope.ActorID}
	callerID := digitalHumanID
	caller := chatruntime.Caller{Type: runtimemodel.ConversationCallerDigitalHuman, ID: &callerID, Source: digitalHumanChannelConversationSource}
	conversation, err := chat.GetConversationByCaller(ctx, runtimeScope, caller, conversationID)
	if errors.Is(err, chatruntime.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if conversation == nil || conversation.ActiveMessageID == nil || *conversation.ActiveMessageID == uuid.Nil || conversation.RuntimeStatus != runtimemodel.ConversationRuntimeStatusStreaming {
		return false, nil
	}
	admissionMode := chatruntime.RuntimeInputAdmissionFollowUpOnly
	canJoin, err := s.channelRuntimeInputCanJoinCurrentTurn(ctx, scope, digitalHumanID, *conversation.ActiveMessageID, request)
	if err != nil {
		return false, err
	}
	if canJoin {
		admissionMode = chatruntime.RuntimeInputAdmissionCurrentOrFollowUp
	}
	_, err = chat.SubmitRuntimeInput(ctx, runtimeScope, caller, chatruntime.RunConfig{
		RuntimeInputPolicy: chatruntime.RuntimeInputPolicyCheckpointAndFollowUp,
	}, chatruntime.SubmitRuntimeInputRequest{
		ConversationID: conversationID, TargetMessageID: *conversation.ActiveMessageID,
		ClientInputID: request.ClientInputID, Content: request.Content, AdmissionMode: admissionMode,
	})
	if errors.Is(err, chatruntime.ErrConversationIdle) || errors.Is(err, chatruntime.ErrInvalidInput) {
		return false, nil
	}
	return err == nil, err
}

func (s *RuntimeService) channelRuntimeInputCanJoinCurrentTurn(
	ctx context.Context,
	scope Scope,
	digitalHumanID, activeMessageID uuid.UUID,
	request ChannelRuntimeInputRequest,
) (bool, error) {
	if request.ConversationType != channelruntime.ConversationTypeGroup || strings.TrimSpace(request.SenderExternalID) == "" {
		return false, nil
	}
	store, ok := s.store.(runtimeInputFollowUpStore)
	if !ok {
		return false, nil
	}
	activeRun, err := store.RunByMessageID(ctx, scope.OrganizationID, scope.WorkspaceID, digitalHumanID, activeMessageID)
	if errors.Is(err, ErrRunNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, activeEvent, err := decodeAndVerifyRun(activeRun)
	if err != nil {
		return false, err
	}
	return activeEvent.Origin.Kind == invocation.OriginChannel &&
		strings.EqualFold(contextString(activeEvent.Context, "conversation_type"), string(channelruntime.ConversationTypeGroup)) &&
		strings.TrimSpace(activeEvent.Sender.ID) == strings.TrimSpace(request.SenderExternalID), nil
}

// absorbChannelRunIntoActiveTurn closes the small queue-to-ChatRuntime window:
// a callback can create a durable run after the previous callback was accepted
// but before that previous run linked its active message. Once this run reaches
// a worker, it joins the now-visible active turn instead of competing with it.
func (s *RuntimeService) absorbChannelRunIntoActiveTurn(ctx context.Context, run *Run, event invocation.Event) (bool, error) {
	if run == nil || run.ConversationID == nil || *run.ConversationID == uuid.Nil || run.ActorID == nil || *run.ActorID == uuid.Nil ||
		event.Origin.Kind != invocation.OriginChannel {
		return false, nil
	}
	if _, followUp := runtimeInputFollowUpBatchID(event); followUp {
		return false, nil
	}
	inboundID, ok := contextUUID(event.Context, "channel_inbound_event_id")
	if !ok {
		return false, nil
	}
	content := strings.TrimSpace(run.Query)
	if strings.EqualFold(contextString(event.Context, "conversation_type"), string(channelruntime.ConversationTypeGroup)) {
		author := strings.TrimSpace(contextString(event.Context, "sender_display_name"))
		if author == "" {
			installationID, _ := uuid.Parse(strings.TrimSpace(event.Origin.InstallationID))
			author = stableChannelAuthorAlias(installationID, contextString(event.Context, "external_conversation_id"), event.Sender.ID)
		}
		content = "Channel message from " + author + ":\n" + content
	}
	accepted, err := s.SubmitChannelRuntimeInput(
		ctx,
		runScope(run),
		run.DigitalHumanID,
		*run.ConversationID,
		ChannelRuntimeInputRequest{
			ClientInputID: inboundID, Content: content,
			ConversationType: channelruntime.ConversationType(contextString(event.Context, "conversation_type")),
			SenderExternalID: event.Sender.ID,
		},
	)
	if err != nil || !accepted {
		return accepted, err
	}
	trace, _ := json.Marshal(map[string]any{
		"runtime_input": []map[string]any{{
			"status":          "absorbed_into_active_turn",
			"conversation_id": run.ConversationID.String(),
		}},
	})
	if err := s.store.CompleteRun(ctx, run.ID, "", trace, s.now()); err != nil {
		return true, err
	}
	return true, nil
}

// SubmitRuntimeInputFollowUp promotes a claimed ChatRuntime input batch into a
// normal durable DigitalHuman run. This deliberately preserves published
// version resolution, run logs, resource snapshots, and channel delivery.
func (s *RuntimeService) SubmitRuntimeInputFollowUp(ctx context.Context, batchID uuid.UUID) (*Run, bool, error) {
	if batchID == uuid.Nil {
		return nil, false, ErrInvalidRequest
	}
	chat, ok := s.chat.(runtimeInputChatRuntime)
	if !ok {
		return nil, false, fmt.Errorf("digital human runtime input follow-up is unavailable")
	}
	info, err := chat.GetRuntimeInputFollowUpContext(ctx, batchID)
	if err != nil {
		if errors.Is(err, chatruntime.ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if info == nil || info.CallerType != runtimemodel.ConversationCallerDigitalHuman || info.CallerID == uuid.Nil || info.WorkspaceID == nil || len(info.Contents) == 0 {
		return nil, false, ErrInvalidRequest
	}
	store, ok := s.store.(runtimeInputFollowUpStore)
	if !ok {
		return nil, false, fmt.Errorf("digital human runtime input follow-up store is unavailable")
	}
	original, err := store.RunByMessageID(ctx, info.OrganizationID, *info.WorkspaceID, info.CallerID, info.TargetMessageID)
	if err != nil {
		return nil, false, err
	}
	_, originalEvent, err := decodeAndVerifyRun(original)
	if err != nil {
		return nil, false, err
	}
	event := originalEvent
	event.ID = batchID.String()
	event.IdempotencyKey = ""
	event.OccurredAt = s.now()
	event.Context = cloneInvocationContext(originalEvent.Context)
	event.Context[runtimeInputFollowUpBatchContextKey] = batchID.String()
	query := strings.Join(info.Contents, "\n\n")
	event.Context["query"] = query

	if event.Origin.Kind == invocation.OriginChannel {
		if err := hydrateRuntimeInputChannelEvent(ctx, store, original, &event, info.ClientInputIDs); err != nil {
			return nil, false, err
		}
	}
	scope := Scope{OrganizationID: info.OrganizationID, WorkspaceID: *info.WorkspaceID, ActorID: info.AccountID}
	conversationID := info.ConversationID
	return s.SubmitInvocation(ctx, scope, event, query, &conversationID)
}

func hydrateRuntimeInputChannelEvent(ctx context.Context, store runtimeInputFollowUpStore, original *Run, event *invocation.Event, clientInputIDs []uuid.UUID) error {
	if original == nil || event == nil || event.ReplyTo == nil || len(clientInputIDs) == 0 {
		return ErrInvalidRequest
	}
	installationID, err := uuid.Parse(strings.TrimSpace(event.Origin.InstallationID))
	if err != nil || installationID == uuid.Nil {
		return ErrInvalidRequest
	}
	inboundID := clientInputIDs[len(clientInputIDs)-1]
	inbound, err := store.ChannelInboundEventByID(ctx, original.OrganizationID, original.WorkspaceID, installationID, inboundID)
	if err != nil {
		return err
	}
	event.Sender = invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: inbound.SenderExternalID}
	event.OccurredAt = inbound.ReceivedAt.UTC()
	event.Context["provider_message_id"] = inbound.ProviderMessageID
	event.Context["external_conversation_id"] = inbound.ExternalConversationID
	event.Context["conversation_type"] = string(inbound.ConversationType)
	event.Context["sender_display_name"] = stableChannelAuthorAlias(installationID, inbound.ExternalConversationID, inbound.SenderExternalID)
	event.Context["mention_only"] = inbound.MentionOnly
	event.Context["channel_inbound_event_id"] = inbound.ID.String()
	delete(event.Context, "root_message_id")
	delete(event.Context, "parent_message_id")
	delete(event.Context, "thread_id")
	event.ReplyTo.ConversationID = inbound.ExternalConversationID
	if inbound.ConversationType == channelruntime.ConversationTypeGroup {
		event.ReplyTo.RecipientID = strings.TrimSpace(inbound.SenderExternalID)
	} else {
		event.ReplyTo.RecipientID = ""
	}
	return nil
}

func cloneInvocationContext(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(value))
	if json.Unmarshal(encoded, &cloned) != nil {
		return map[string]any{}
	}
	return cloned
}

func runtimeInputFollowUpBatchID(event invocation.Event) (uuid.UUID, bool) {
	raw := strings.TrimSpace(contextString(event.Context, runtimeInputFollowUpBatchContextKey))
	batchID, err := uuid.Parse(raw)
	return batchID, err == nil && batchID != uuid.Nil
}
