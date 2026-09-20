package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

type digitalHumanTaskTokenBudgetContinuationRuntime interface {
	RunConfiguredTaskTokenBudgetContinuationStream(
		context.Context,
		runtimeservice.Scope,
		runtimeservice.Caller,
		runtimeservice.RunConfig,
		uuid.UUID,
		uuid.UUID,
		string,
		runtimedto.TaskTokenBudgetContinuationRequest,
		func(runtimeservice.StreamEvent) error,
	) (*runtimeservice.ChatResult, error)
}

type digitalHumanTaskTokenBudgetContinuationValidator interface {
	ValidateTaskTokenBudgetContinuation(
		context.Context,
		runtimeservice.Scope,
		runtimeservice.Caller,
		uuid.UUID,
		uuid.UUID,
		string,
	) error
}

// ContinueTaskTokenBudget resumes a DigitalHuman API run paused by its task
// token budget, restoring capabilities from the immutable run snapshot.
func (s *RuntimeService) ContinueTaskTokenBudget(
	ctx context.Context,
	scope Scope,
	digitalHumanID, conversationID, messageID uuid.UUID,
	requestID string,
	request runtimedto.TaskTokenBudgetContinuationRequest,
	onEvent func(runtimeservice.StreamEvent) error,
) (*runtimeservice.ChatResult, error) {
	if s == nil || digitalHumanID == uuid.Nil || conversationID == uuid.Nil || messageID == uuid.Nil || strings.TrimSpace(requestID) == "" || onEvent == nil {
		return nil, ErrInvalidRequest
	}
	continuationRuntime, ok := s.chat.(digitalHumanTaskTokenBudgetContinuationRuntime)
	if !ok {
		return nil, fmt.Errorf("digital human token budget continuation is unavailable")
	}
	lookup, ok := s.store.(digitalHumanContinuationRunLookup)
	if !ok {
		return nil, fmt.Errorf("digital human continuation run lookup is unavailable")
	}
	run, err := lookup.RunByMessageID(ctx, scope.OrganizationID, scope.WorkspaceID, digitalHumanID, messageID)
	if err != nil {
		return nil, err
	}
	if !continuationRunOwnedByScope(run, scope, digitalHumanID) || run.ConversationID == nil || *run.ConversationID != conversationID || run.MessageID == nil || *run.MessageID != messageID {
		return nil, ErrConversationNotFound
	}
	if run.Status != RunStatusWaiting || run.WaitingReason != runtimemodel.MessageStatusWaitingBudget {
		return nil, ErrRunConflict
	}
	action := strings.ToLower(strings.TrimSpace(request.Action))
	if action != "add" && action != "stop" {
		return nil, ErrInvalidRequest
	}
	if action == "add" && (request.AdditionalTokens < 1 || request.AdditionalTokens > 100000000) {
		return nil, ErrInvalidRequest
	}
	resolved, event, err := decodeAndVerifyRun(run)
	if err != nil {
		return nil, err
	}
	config, cleanup, err := s.userInputContinuationConfig(ctx, run, resolved, event)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	_, runtimeScope, caller, err := s.conversationContext(scope, digitalHumanID)
	if err != nil {
		return nil, err
	}
	if validator, ok := continuationRuntime.(digitalHumanTaskTokenBudgetContinuationValidator); ok {
		if err := validator.ValidateTaskTokenBudgetContinuation(ctx, runtimeScope, caller, conversationID, messageID, strings.TrimSpace(requestID)); err != nil {
			return nil, err
		}
	}
	if action == "add" {
		resumer, ok := s.store.(runWaitingStore)
		if !ok {
			return nil, fmt.Errorf("digital human waiting run resume is unavailable")
		}
		if err := resumer.ResumeWaitingRun(ctx, run.ID, s.now()); err != nil {
			return nil, err
		}
	}
	result, err := continuationRuntime.RunConfiguredTaskTokenBudgetContinuationStream(ctx, runtimeScope, caller, config, conversationID, messageID, strings.TrimSpace(requestID), request, onEvent)
	if err != nil {
		if action == "add" {
			_ = s.store.FailRun(context.WithoutCancel(ctx), run.ID, "digital_human.run.execution_failed", "The DigitalHuman continuation failed.", s.now())
		}
		return nil, err
	}
	if result == nil {
		return nil, ErrRunConflict
	}
	if result.Status == runtimemodel.MessageStatusWaitingBudget {
		if waiter, ok := s.store.(runWaitingStore); ok {
			if err := waiter.MarkRunWaiting(ctx, run.ID, runtimemodel.MessageStatusWaitingBudget, s.now()); err != nil {
				return nil, err
			}
		}
		return result, nil
	}
	if result.Status == runtimemodel.MessageStatusStopped {
		if waiter, ok := s.store.(runWaitingStore); ok {
			if err := waiter.CancelWaitingRun(ctx, run.ID, s.now()); err != nil {
				return nil, err
			}
		}
		return result, nil
	}
	if err := s.store.CompleteRunWithDelivery(ctx, run.ID, result.Answer, executionTraceFromMetadata(result.Metadata), s.now()); err != nil {
		return nil, err
	}
	return result, nil
}

func (h *ExternalHandler) ContinueAPITaskTokenBudget(c *gin.Context) {
	key, access, ok := h.apiAccess(c)
	if !ok {
		return
	}
	var raw struct {
		User             string                 `json:"user"`
		Action           string                 `json:"action"`
		AdditionalTokens int                    `json:"additional_tokens,omitempty"`
		RuntimeContext   string                 `json:"runtime_context,omitempty"`
		OperationContext map[string]interface{} `json:"operation_context,omitempty"`
	}
	if !h.base.decode(c, &raw) {
		return
	}
	scope, ok := h.apiUserScope(c, key, access, raw.User)
	if !ok {
		return
	}
	conversationID, ok := h.conversationID(c)
	if !ok {
		return
	}
	messageID, ok := parseUserInputMessageID(c, h.base, "digital_human.external.token_budget.message")
	if !ok {
		return
	}
	requestID := strings.TrimSpace(c.Param("request_id"))
	if requestID == "" {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.external.token_budget.request")
		return
	}
	request := runtimedto.TaskTokenBudgetContinuationRequest{Action: raw.Action, AdditionalTokens: raw.AdditionalTokens, Surface: SurfaceAPI, RuntimeContext: raw.RuntimeContext, OperationContext: raw.OperationContext}
	writer := newDigitalHumanSSEWriter(c)
	writer.start(c.Request.Context())
	_, err := h.runtime.ContinueTaskTokenBudget(c.Request.Context(), scope, access.Definition.ID, conversationID, messageID, requestID, request, func(event runtimeservice.StreamEvent) error {
		return writer.write(event.ID, event.EventType, redactConversationValue(event.Payload))
	})
	if err == nil || errors.Is(err, runtimeservice.ErrMessageStopped) || runtimeservice.IsFinalizedStreamError(err) {
		return
	}
	_ = h.writeStreamError(writer, uuid.Nil, err)
}
