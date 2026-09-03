package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	"gorm.io/gorm"
)

// connectorConfirmationResolution contains the complete set of values that
// may enter Chat metadata. It cannot carry the provider value or frozen input.
type connectorConfirmationResolution struct {
	routeReceipt   string
	state          connectorconfirmation.State
	decisionID     string
	errorCode      string
	surface        connectorcore.TargetType
	conversationID uuid.UUID
	messageID      uuid.UUID
	agentID        uuid.UUID
	operation      *connectortools.OperationRef
	outcomeID      uuid.UUID
}

// RecordConnectorConfirmationResolution projects a resolved Connector call
// into its owner-scoped Chat or Agent message. It never invokes a Connector or
// any legacy continuation path.
func (s *service) RecordConnectorConfirmationResolution(
	ctx context.Context,
	command connectorconfirmation.Command,
	result connectorconfirmation.Result,
) error {
	resolution, err := validateConnectorConfirmationResolution(command, result)
	if err != nil {
		return err
	}
	if ctx == nil || s == nil || s.repos == nil || s.repos.DB == nil {
		return connectorResolutionError(connectorcore.ErrorCodeInternal)
	}

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
			!connectorResolutionConversationMatches(conversation, command, resolution) {
			return connectorResolutionError(connectorcore.ErrorCodeAccessDenied)
		}

		metadata, added, applyErr := applyConnectorConfirmationResolution(message.Metadata, resolution)
		if applyErr != nil {
			return applyErr
		}
		switch message.Status {
		case runtimemodel.MessageStatusWaitingApproval:
			if updateErr := repos.Message.UpdateCompleted(ctx, message.ID, message.Answer, metadata); updateErr != nil {
				if errors.Is(updateErr, gorm.ErrRecordNotFound) {
					return connectorResolutionError(connectorcore.ErrorCodeConflict)
				}
				return connectorResolutionError(connectorcore.ErrorCodeInternal)
			}
			created = added
			return nil
		case runtimemodel.MessageStatusCompleted:
			if !added {
				return nil
			}
		}
		return connectorResolutionError(connectorcore.ErrorCodeConflict)
	})
	if err != nil {
		return err
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
	return nil
}

func validateConnectorConfirmationResolution(
	command connectorconfirmation.Command,
	result connectorconfirmation.Result,
) (connectorConfirmationResolution, error) {
	if command.OrganizationID == uuid.Nil || command.OwnerAccountID == uuid.Nil ||
		(command.Decision != connectorexecution.ConfirmationDecisionConfirmed &&
			command.Decision != connectorexecution.ConfirmationDecisionCancelled) {
		return connectorConfirmationResolution{}, connectorResolutionError(connectorcore.ErrorCodeInvalidInput)
	}
	receipt, ok := canonicalConnectorResolutionUUID(string(command.RouteReceipt))
	if !ok {
		return connectorConfirmationResolution{}, connectorResolutionError(connectorcore.ErrorCodeInvalidInput)
	}
	if err := validateConnectorResolutionTerminal(command.Decision, result); err != nil {
		return connectorConfirmationResolution{}, err
	}
	conversationID, conversationOK := canonicalConnectorResolutionUUID(result.Origin.ConversationID)
	messageID, messageOK := canonicalConnectorResolutionUUID(result.Origin.MessageID)
	if !conversationOK || !messageOK {
		return connectorConfirmationResolution{}, connectorResolutionError(connectorcore.ErrorCodeAccessDenied)
	}
	resolution := connectorConfirmationResolution{
		routeReceipt:   receipt.String(),
		state:          result.State,
		surface:        result.Origin.Surface,
		conversationID: conversationID,
		messageID:      messageID,
		outcomeID:      result.OutcomeID,
	}
	identity := result.RequestIdentity
	if identity.LogicalOperationID != "" {
		operation := connectortools.OperationRef{ConnectorID: identity.ConnectorID, OperationID: identity.OperationID, BindingID: identity.BindingID, ConnectionID: identity.ConnectionID, Target: identity.Target, LogicalOperationID: identity.LogicalOperationID}
		if !operation.Valid() || identity.OrganizationID != command.OrganizationID || identity.OwnerAccountID != command.OwnerAccountID || identity.ActingPrincipalID != command.OwnerAccountID || identity.CorrelationID != "connector-correlation-"+messageID.String() {
			return connectorConfirmationResolution{}, connectorResolutionError(connectorcore.ErrorCodeAccessDenied)
		}
		resolution.operation = &operation
	}
	if result.DecisionID != uuid.Nil {
		resolution.decisionID = result.DecisionID.String()
	}
	if result.ErrorCode != "" {
		resolution.errorCode = string(result.ErrorCode)
	}
	switch result.Origin.Surface {
	case connectorcore.TargetTypeChat:
		if result.Origin.AgentID != "" || result.Origin.WorkflowID != "" ||
			result.Origin.RunID != "" || result.Origin.NodeID != "" {
			return connectorConfirmationResolution{}, connectorResolutionError(connectorcore.ErrorCodeAccessDenied)
		}
	case connectorcore.TargetTypeAgent:
		agentID, agentOK := canonicalConnectorResolutionUUID(result.Origin.AgentID)
		if !agentOK || result.Origin.WorkflowID != "" || result.Origin.RunID != "" || result.Origin.NodeID != "" {
			return connectorConfirmationResolution{}, connectorResolutionError(connectorcore.ErrorCodeAccessDenied)
		}
		resolution.agentID = agentID
	default:
		return connectorConfirmationResolution{}, connectorResolutionError(connectorcore.ErrorCodeAccessDenied)
	}
	return resolution, nil
}

func validateConnectorResolutionTerminal(
	decision connectorexecution.ConfirmationDecision,
	result connectorconfirmation.Result,
) error {
	valid := false
	switch result.State {
	case connectorconfirmation.StateCancelled:
		valid = decision == connectorexecution.ConfirmationDecisionCancelled &&
			result.DecisionID != uuid.Nil && result.ErrorCode == ""
	case connectorconfirmation.StateCompleted:
		valid = decision == connectorexecution.ConfirmationDecisionConfirmed &&
			result.DecisionID != uuid.Nil && result.ErrorCode == ""
	case connectorconfirmation.StateFailed:
		valid = decision == connectorexecution.ConfirmationDecisionConfirmed && result.ErrorCode.Valid()
	case connectorconfirmation.StateUnknown:
		valid = decision == connectorexecution.ConfirmationDecisionConfirmed &&
			result.ErrorCode == connectorcore.ErrorCodeOutcomeUnknown
	}
	if !valid {
		return connectorResolutionError(connectorcore.ErrorCodeInvalidInput)
	}
	return nil
}

func connectorResolutionConversationMatches(
	conversation *runtimemodel.Conversation,
	command connectorconfirmation.Command,
	resolution connectorConfirmationResolution,
) bool {
	if conversation == nil || conversation.ID != resolution.conversationID ||
		conversation.OrganizationID != command.OrganizationID ||
		conversation.AccountID != command.OwnerAccountID ||
		strings.TrimSpace(conversation.ConversationType) != runtimemodel.ConversationTypeChat {
		return false
	}
	if resolution.surface == connectorcore.TargetTypeChat {
		return strings.TrimSpace(conversation.CallerType) == runtimemodel.ConversationCallerAIChat &&
			normalizeCallerID(conversation.CallerID) == nil
	}
	callerID := normalizeCallerID(conversation.CallerID)
	return resolution.surface == connectorcore.TargetTypeAgent &&
		strings.TrimSpace(conversation.CallerType) == runtimemodel.ConversationCallerAgent &&
		callerID != nil && *callerID == resolution.agentID
}

func applyConnectorConfirmationResolution(
	metadata map[string]interface{},
	resolution connectorConfirmationResolution,
) (map[string]interface{}, bool, error) {
	updated := copyStringAnyMap(metadata)
	if updated == nil {
		updated = map[string]interface{}{}
	}
	matches := 0
	for _, event := range connectorConfirmationEventsFromAny(updated[connectorConfirmationEventsMetadataKey]) {
		if strings.TrimSpace(stringFromAny(event["event_type"])) != streamEventConnectorConfirmationResolved ||
			strings.TrimSpace(stringFromAny(event["route_receipt"])) != resolution.routeReceipt {
			continue
		}
		matches++
		if !resolution.matchesEvent(event) {
			return nil, false, connectorResolutionError(connectorcore.ErrorCodeConflict)
		}
	}
	if matches > 1 {
		return nil, false, connectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	if matches == 1 {
		return updated, false, nil
	}

	invocations := skillInvocationsFromMetadata(updated["skill_invocations"])
	match := -1
	for index, invocation := range invocations {
		if connectorResolutionInvocationReceipt(invocation) == resolution.routeReceipt {
			if match >= 0 {
				return nil, false, connectorResolutionError(connectorcore.ErrorCodeConflict)
			}
			match = index
		}
	}
	if match < 0 {
		return nil, false, connectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	if resolution.operation != nil {
		previous := mapFromOperationContext(invocations[match]["result"])
		encoded, err := json.Marshal(previous["operation"])
		var operation connectortools.OperationRef
		if err != nil || json.Unmarshal(encoded, &operation) != nil {
			return nil, false, connectorResolutionError(connectorcore.ErrorCodeConflict)
		}
		if operation.Valid() {
			prior := connectortools.InvocationEvidence{Operation: operation}
			next := connectortools.InvocationEvidence{Operation: *resolution.operation}
			if prior.Key() != next.Key() {
				return nil, false, connectorResolutionError(connectorcore.ErrorCodeConflict)
			}
			resolution.operation.ActionStepID, resolution.operation.PlanPhaseID = operation.ActionStepID, operation.PlanPhaseID
		}
	}
	invocations[match]["result"] = resolution.terminalPayload()
	invocations[match]["status"] = string(resolution.state)
	updated["skill_invocations"] = skillInvocationsToInterfaceSlice(invocations)
	if resolution.operation != nil {
		applyOperationPlanInvocationState(updated, []map[string]interface{}{invocations[match]})
	}
	events := connectorConfirmationEventsFromAny(updated[connectorConfirmationEventsMetadataKey])
	events = append(events, resolution.eventPayload())
	updated[connectorConfirmationEventsMetadataKey] = connectorConfirmationEventsToAny(events)
	return updated, true, nil
}

func connectorResolutionInvocationReceipt(invocation map[string]interface{}) string {
	result := governanceMapFromAny(invocation["result"])
	state := strings.TrimSpace(stringFromAny(result["state"]))
	if state == "" {
		state = strings.TrimSpace(stringFromAny(invocation["state"]))
	}
	if state != string(connectorexecution.ExecutionStateRequiresConfirmation) {
		return ""
	}
	candidates := []string{
		strings.TrimSpace(stringFromAny(invocation["route_receipt"])),
		strings.TrimSpace(stringFromAny(result["route_receipt"])),
		strings.TrimSpace(stringFromAny(governanceMapFromAny(invocation["confirmation"])["route_receipt"])),
		strings.TrimSpace(stringFromAny(governanceMapFromAny(result["confirmation"])["route_receipt"])),
	}
	receipt := ""
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if receipt != "" && receipt != candidate {
			return ""
		}
		receipt = candidate
	}
	return receipt
}

func (resolution connectorConfirmationResolution) terminalPayload() map[string]interface{} {
	payload := map[string]interface{}{"state": string(resolution.state)}
	if resolution.operation != nil {
		payload["operation"] = *resolution.operation
		status := "unknown"
		if resolution.state == connectorconfirmation.StateCancelled {
			status = "cancelled"
		}
		if resolution.outcomeID != uuid.Nil {
			payload["outcome_id"] = resolution.outcomeID.String()
			switch resolution.state {
			case connectorconfirmation.StateCompleted:
				status = "successful"
			case connectorconfirmation.StateFailed:
				status = "failed"
			}
			payload["state"], payload["outcome_status"] = "completed", status
		}
		payload["connector_presentation"] = connectortools.OperationPresentation{SchemaVersion: 1, SourceKind: "native", InvocationID: resolution.operation.LogicalOperationID, ConnectorID: resolution.operation.ConnectorID, OperationID: resolution.operation.OperationID, DisplayStatus: status, Redacted: true}
	}
	if resolution.decisionID != "" {
		payload["decision_id"] = resolution.decisionID
	}
	if resolution.errorCode != "" {
		payload["error_code"] = resolution.errorCode
	}
	return payload
}

func (resolution connectorConfirmationResolution) eventPayload() map[string]interface{} {
	payload := resolution.terminalPayload()
	payload["state"] = string(resolution.state)
	payload["event_type"] = streamEventConnectorConfirmationResolved
	payload["route_receipt"] = resolution.routeReceipt
	payload["surface"] = string(resolution.surface)
	payload["conversation_id"] = resolution.conversationID.String()
	payload["message_id"] = resolution.messageID.String()
	if resolution.agentID != uuid.Nil {
		payload["agent_id"] = resolution.agentID.String()
	}
	return payload
}

func (resolution connectorConfirmationResolution) matchesEvent(event map[string]interface{}) bool {
	agentID := ""
	if resolution.agentID != uuid.Nil {
		agentID = resolution.agentID.String()
	}
	return strings.TrimSpace(stringFromAny(event["event_type"])) == streamEventConnectorConfirmationResolved &&
		strings.TrimSpace(stringFromAny(event["route_receipt"])) == resolution.routeReceipt &&
		strings.TrimSpace(stringFromAny(event["surface"])) == string(resolution.surface) &&
		strings.TrimSpace(stringFromAny(event["conversation_id"])) == resolution.conversationID.String() &&
		strings.TrimSpace(stringFromAny(event["message_id"])) == resolution.messageID.String() &&
		strings.TrimSpace(stringFromAny(event["agent_id"])) == agentID &&
		strings.TrimSpace(stringFromAny(event["state"])) == string(resolution.state) &&
		strings.TrimSpace(stringFromAny(event["decision_id"])) == resolution.decisionID &&
		strings.TrimSpace(stringFromAny(event["error_code"])) == resolution.errorCode
}

func canonicalConnectorResolutionUUID(value string) (uuid.UUID, bool) {
	if value == "" || value != strings.TrimSpace(value) {
		return uuid.Nil, false
	}
	parsed, err := uuid.Parse(value)
	return parsed, err == nil && parsed != uuid.Nil && parsed.String() == value
}

func connectorResolutionLoadError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return connectorResolutionError(connectorcore.ErrorCodeAccessDenied)
	}
	return connectorResolutionError(connectorcore.ErrorCodeInternal)
}

func connectorResolutionError(code connectorcore.ErrorCode) error {
	return connectorcore.NewError(code, false, nil)
}
