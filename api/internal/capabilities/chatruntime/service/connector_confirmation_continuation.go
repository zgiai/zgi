package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"gorm.io/gorm"
)

// ConnectorConfirmationContinuation is a DigitalHuman turn claimed for the
// model pass that follows a decided connector confirmation (ADR 0024).
// ToolResult is the model-visible result of the decided call. It never enters
// message metadata or stream events.
type ConnectorConfirmationContinuation struct {
	OrganizationID uuid.UUID
	WorkspaceID    uuid.UUID
	ConversationID uuid.UUID
	MessageID      uuid.UUID
	DigitalHumanID uuid.UUID
	RunID          uuid.UUID
	RouteReceipt   string
	State          connectorconfirmation.State
	Title          string
	ToolResult     string
}

// ClaimConnectorConfirmationContinuation records a decided DigitalHuman
// confirmation and, in the same transaction, claims its paused message for
// streaming and moves its run back to running. The run's own continuation then
// resumes the turn; nothing here invokes a Connector or the model.
func (s *service) ClaimConnectorConfirmationContinuation(
	ctx context.Context,
	command connectorconfirmation.Command,
	result connectorconfirmation.Result,
) (*ConnectorConfirmationContinuation, error) {
	resolution, err := validateConnectorConfirmationResolution(command, result)
	if err != nil {
		return nil, err
	}
	if resolution.surface != connectorcore.TargetTypeDigitalHuman {
		return nil, connectorResolutionError(connectorcore.ErrorCodeAccessDenied)
	}
	if ctx == nil || s == nil || s.repos == nil || s.repos.DB == nil {
		return nil, connectorResolutionError(connectorcore.ErrorCodeInternal)
	}

	var workspaceID uuid.UUID
	created := false
	err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repos := repository.NewRepositories(tx)
		message, loadErr := repository.GetMessageScopedForUpdate(
			ctx, tx, resolution.messageID, command.OrganizationID, command.OwnerAccountID,
		)
		if loadErr != nil {
			return connectorResolutionLoadError(loadErr)
		}
		conversation, loadErr := repos.Conversation.GetScoped(
			ctx, resolution.conversationID, command.OrganizationID, command.OwnerAccountID,
		)
		if loadErr != nil {
			return connectorResolutionLoadError(loadErr)
		}
		if message.ConversationID != resolution.conversationID ||
			!connectorResolutionConversationMatches(conversation, command, resolution) ||
			conversation.WorkspaceID == nil || *conversation.WorkspaceID == uuid.Nil {
			return connectorResolutionError(connectorcore.ErrorCodeAccessDenied)
		}
		if message.Status != runtimemodel.MessageStatusWaitingApproval {
			return connectorResolutionError(connectorcore.ErrorCodeConflict)
		}
		metadata, added, applyErr := applyConnectorConfirmationResolution(message.Metadata, resolution)
		if applyErr != nil {
			return applyErr
		}
		if updateErr := repos.Message.UpdateMetadata(ctx, message.ID, metadata); updateErr != nil {
			return connectorResolutionError(connectorcore.ErrorCodeInternal)
		}
		claimed := tx.Model(&runtimemodel.Message{}).
			Where("id = ? AND deleted_at IS NULL AND status = ?", message.ID, runtimemodel.MessageStatusWaitingApproval).
			Updates(map[string]interface{}{"status": runtimemodel.MessageStatusStreaming, "error": nil})
		if claimed.Error != nil {
			return connectorResolutionError(connectorcore.ErrorCodeInternal)
		}
		if claimed.RowsAffected != 1 {
			return connectorResolutionError(connectorcore.ErrorCodeConflict)
		}
		if startErr := repos.Conversation.StartStreaming(ctx, conversation.ID, command.OrganizationID, command.OwnerAccountID, message.ID); startErr != nil {
			return connectorResolutionError(connectorcore.ErrorCodeConflict)
		}
		if updateErr := moveDigitalHumanRun(ctx, tx, command, resolution, "running", "", nil); updateErr != nil {
			return updateErr
		}
		workspaceID = *conversation.WorkspaceID
		created = added
		return nil
	})
	if err != nil {
		return nil, err
	}
	if created {
		s.appendStreamEventBestEffort(
			ctx,
			resolution.messageID,
			resolution.conversationID,
			streamEventConnectorConfirmationResolved,
			resolution.eventPayload(),
		)
	}
	title := ""
	if result.Presentation != nil {
		title = strings.TrimSpace(result.Presentation.Title)
	}
	return &ConnectorConfirmationContinuation{
		OrganizationID: command.OrganizationID,
		WorkspaceID:    workspaceID,
		ConversationID: resolution.conversationID,
		MessageID:      resolution.messageID,
		DigitalHumanID: resolution.digitalHumanID,
		RunID:          resolution.runID,
		RouteReceipt:   resolution.routeReceipt,
		State:          resolution.state,
		Title:          title,
		ToolResult:     connectorConfirmationModelToolResult(result),
	}, nil
}

// RunConfiguredConnectorConfirmationContinuation resumes a claimed turn: the
// paused tool result of the decided call is replaced by its outcome, and the
// model continues the same assistant message.
func (s *service) RunConfiguredConnectorConfirmationContinuation(
	ctx context.Context,
	scope Scope,
	caller Caller,
	config RunConfig,
	claim *ConnectorConfirmationContinuation,
	onEvent func(StreamEvent) error,
) (*ChatResult, error) {
	if claim == nil || claim.ConversationID == uuid.Nil || claim.MessageID == uuid.Nil || strings.TrimSpace(claim.RouteReceipt) == "" {
		return nil, fmt.Errorf("%w: connector confirmation continuation is required", ErrInvalidInput)
	}
	if onEvent == nil {
		onEvent = func(StreamEvent) error { return nil }
	}
	conversation, message, err := s.reloadToolGovernanceContinuationMessageForCaller(ctx, scope, caller, claim.ConversationID, claim.MessageID)
	if err != nil {
		return nil, err
	}
	continuation := &ToolGovernanceContinuation{Conversation: conversation, Message: message}
	if message.Status != runtimemodel.MessageStatusStreaming {
		return nil, fmt.Errorf("%w: message is not claimed for a connector continuation", ErrInvalidInput)
	}
	prepared, err := s.prepareConfiguredToolGovernanceContinuationChat(ctx, scope, caller, config, continuation)
	if err != nil {
		s.failToolGovernanceContinuation(context.WithoutCancel(ctx), continuation, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	execution, err := s.beginRuntimeExecution(ctx, message.ID)
	if err != nil {
		s.failToolGovernanceContinuation(context.WithoutCancel(ctx), continuation, err, onEvent)
		return nil, newFinalizedStreamError(err)
	}
	defer execution.Finish()
	runCtx := execution.Context
	persistCtx := execution.PersistContext
	prepared.ContinuationType = "connector_confirmation"

	s.emitPreparedEvent(persistCtx, prepared, streamEventMessageStart, messageStartPayload(conversation, message, false), onEvent)
	replacePendingConnectorToolResult(prepared.LLMRequest, claim.RouteReceipt, claim.ToolResult)
	prepared.resumeWith(continuationMessageForExecutionMode(connectorConfirmationContinuationMessage(message, claim), prepared.parts.ExecutionMode))
	answer, usage, err := s.runPreparedToolLoop(runCtx, persistCtx, prepared, nil, onEvent)
	if err != nil {
		return s.finishContinuationPendingOrError(persistCtx, prepared, answer, usage, err, onEvent)
	}
	if budgetErr := prepared.taskTokenBudget.exhausted(); budgetErr != nil {
		return s.finishContinuationPendingOrError(persistCtx, prepared, answer, usage, budgetErr, onEvent)
	}
	if s.streams.IsStopped(message.ID, execution.runID) {
		_ = s.persistStoppedAnswer(persistCtx, prepared, answer, usage)
		return nil, ErrMessageStopped
	}
	metadata := preparedResultMetadataForPrepared(prepared, prepared.Message.Metadata, usage)
	prepared.Message.Metadata = metadata
	if err := s.completePreparedChat(persistCtx, prepared, answer, metadata); err != nil {
		return nil, finalizedRuntimePersistenceError(err)
	}
	s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayload(prepared, metadata), onEvent)
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusCompleted}, nil
}

// connectorConfirmationModelToolResult is what an unconfirmed call of the same
// operation would have returned to the model. Without an execution result,
// only the terminal state and error code are known.
func connectorConfirmationModelToolResult(result connectorconfirmation.Result) string {
	var messages []tools.ToolInvokeMessage
	if result.ExecutionResult != nil {
		identity := result.RequestIdentity
		messages = connectortools.MessagesForInvocation(*result.ExecutionResult, dispatch.Identity{
			OrganizationID:     identity.OrganizationID,
			OwnerAccountID:     identity.OwnerAccountID,
			ActingPrincipalID:  identity.ActingPrincipalID,
			BindingID:          identity.BindingID,
			ConnectionID:       identity.ConnectionID,
			ConnectorID:        identity.ConnectorID,
			Target:             identity.Target,
			OperationID:        identity.OperationID,
			LogicalOperationID: identity.LogicalOperationID,
			CorrelationID:      identity.CorrelationID,
			Interactive:        identity.Interactive,
		})
	} else {
		data := map[string]interface{}{"state": string(result.State)}
		if result.ErrorCode != "" {
			data["error_code"] = string(result.ErrorCode)
		}
		messages = []tools.ToolInvokeMessage{{Type: tools.ToolInvokeMessageTypeJSON, Data: data}}
	}
	for index := range messages {
		if messages[index].Data != nil {
			messages[index].Data["confirmation_decision"] = string(result.State)
		}
	}
	payload, err := json.Marshal(messages)
	if err != nil {
		return `[{"type":"json","data":{"state":"unknown"}}]`
	}
	return string(payload)
}

// replacePendingConnectorToolResult swaps the tool result that said the call
// waits for confirmation. A model trusts that result over a later note, so the
// transcript must show the outcome where the call returned.
func replacePendingConnectorToolResult(request *adapter.ChatRequest, routeReceipt, content string) bool {
	routeReceipt = strings.TrimSpace(routeReceipt)
	if request == nil || routeReceipt == "" || strings.TrimSpace(content) == "" {
		return false
	}
	for index := len(request.Messages) - 1; index >= 0; index-- {
		message := request.Messages[index]
		if message.Role != "tool" {
			continue
		}
		current, ok := message.Content.(string)
		if !ok || !strings.Contains(current, routeReceipt) {
			continue
		}
		request.Messages[index].Content = content
		return true
	}
	return false
}

func connectorConfirmationContinuationMessage(message *runtimemodel.Message, claim *ConnectorConfirmationContinuation) adapter.Message {
	userQuery := ""
	if message != nil {
		userQuery = strings.TrimSpace(message.Query)
	}
	operation := strings.TrimSpace(claim.Title)
	if operation == "" {
		operation = "the connector operation"
	}
	var outcome string
	switch claim.State {
	case connectorconfirmation.StateCompleted:
		outcome = "The user confirmed " + operation + ", and the runtime has executed it exactly once. Its result is the tool result of that call."
	case connectorconfirmation.StateCancelled:
		outcome = "The user cancelled " + operation + ". It was not executed."
	case connectorconfirmation.StateFailed:
		outcome = "The user confirmed " + operation + ", and the runtime attempted it exactly once, but it failed. The tool result of that call holds the failure."
	default:
		outcome = "The user confirmed " + operation + ", but its outcome is unknown. Do not assume it succeeded or failed."
	}
	instructions := strings.Join([]string{
		"You are continuing the same assistant turn after the user decided a connector confirmation.",
		"Do not repeat the same connector call with the same arguments in this turn.",
		"Treat the tool result of that call as authoritative.",
		"Report what the result shows and continue any remaining steps of the original request; another operation that needs confirmation will ask the user again.",
		"If the operation was cancelled, failed, or has an unknown outcome, say so plainly and do not claim it succeeded.",
		"Answer in the user's language.",
		"Do not expose internal IDs, UUIDs, receipts, correlation values, or raw JSON field names.",
	}, " ")
	// The note is stored with the turn while the replaced tool result is not,
	// so it repeats the result for every later pass and turn.
	return adapter.Message{
		Role: "system",
		Content: instructions + "\n\nOriginal user request:\n" + userQuery + "\n\nOutcome:\n" + outcome +
			"\n\nResult of that call:\n" + boundedConnectorToolResult(claim.ToolResult),
	}
}

// maxConnectorContinuationResultRunes bounds the repeated result; connector
// providers already bound their own output.
const maxConnectorContinuationResultRunes = 12000

func boundedConnectorToolResult(content string) string {
	runes := []rune(strings.TrimSpace(content))
	if len(runes) <= maxConnectorContinuationResultRunes {
		return string(runes)
	}
	return string(runes[:maxConnectorContinuationResultRunes]) + "…(truncated)"
}
