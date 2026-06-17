package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	streamEventToolGovernanceDecision = "tool_governance_decision"

	toolGovernanceActionApprove = "approve"
	toolGovernanceActionReject  = "reject"

	toolGovernanceApprovalStatusApproved = "approved"
	toolGovernanceApprovalStatusRejected = "rejected"
)

func (s *service) persistToolGovernanceApprovalPending(ctx context.Context, prepared *PreparedChat, payload map[string]interface{}, usage *adapter.Usage) map[string]interface{} {
	if prepared == nil || prepared.Message == nil || prepared.Conversation == nil {
		return map[string]interface{}{}
	}
	pendingPayload := copyStringAnyMap(payload)
	if pendingPayload == nil {
		pendingPayload = map[string]interface{}{}
	}
	pendingPayload["conversation_id"] = prepared.Conversation.ID.String()
	pendingPayload["message_id"] = prepared.Message.ID.String()

	metadata := mergeToolGovernanceDecisionMetadata(prepared.Message.Metadata, pendingPayload)
	metadata = preparedResultMetadata(metadata, usage)
	metadata["tool_governance_continuation"] = compactSkillInvocation(map[string]interface{}{
		"status":         "waiting_approval",
		"correlation_id": toolGovernanceCorrelationID(pendingPayload),
		"skill_id":       pendingPayload["skill_id"],
		"tool_name":      pendingPayload["tool_name"],
		"original_query": prepared.Message.Query,
		"resume_policy":  "same_message",
	})
	prepared.Message.Metadata = metadata

	if s == nil || s.repos == nil || s.repos.Message == nil || s.repos.Conversation == nil {
		return metadata
	}
	if err := s.repos.Message.UpdateWaitingApproval(ctx, prepared.Message.ID, metadata); err != nil {
		return metadata
	}
	_ = s.repos.Conversation.FinishWaitingApprovalMessage(ctx, prepared.Conversation.ID, prepared.Message.ID)
	return metadata
}

func (s *service) SubmitToolGovernanceDecision(
	ctx context.Context,
	scope Scope,
	conversationID uuid.UUID,
	messageID uuid.UUID,
	correlationID string,
	req runtimedto.ToolGovernanceDecisionRequest,
) (*runtimedto.ToolGovernanceDecisionResponse, error) {
	if s == nil || s.repos == nil || s.repos.Message == nil || s.repos.Conversation == nil {
		return nil, fmt.Errorf("aichat repository is not configured")
	}
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	action, approvalStatus, err := normalizeToolGovernanceApprovalAction(req.Action)
	if err != nil {
		return nil, err
	}
	correlationID = strings.TrimSpace(correlationID)
	if correlationID == "" {
		return nil, fmt.Errorf("%w: governance correlation_id is required", ErrInvalidInput)
	}

	conversation, err := s.getConversation(ctx, scope, conversationID)
	if err != nil {
		return nil, err
	}

	var response *runtimedto.ToolGovernanceDecisionResponse
	var emitEvent map[string]interface{}
	if s.repos.DB != nil {
		err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			message, err := toolGovernanceDecisionMessageForUpdate(ctx, tx, messageID, scope)
			if err != nil {
				return err
			}
			txRepos := repository.NewRepositories(tx)
			response, emitEvent, err = s.resolveToolGovernanceDecision(ctx, txRepos, scope, conversation, message, correlationID, req, action, approvalStatus)
			return err
		})
	} else {
		var message *runtimemodel.Message
		message, err = s.repos.Message.GetScoped(ctx, messageID, scope.OrganizationID, scope.AccountID)
		if err != nil {
			return nil, mapRepoError(err)
		}
		response, emitEvent, err = s.resolveToolGovernanceDecision(ctx, s.repos, scope, conversation, message, correlationID, req, action, approvalStatus)
	}
	if err != nil {
		return nil, mapRepoError(err)
	}
	if response != nil && len(emitEvent) > 0 {
		s.appendStreamEventBestEffort(ctx, messageID, conversation.ID, streamEventToolGovernanceDecision, emitEvent)
	}
	return response, nil
}

func toolGovernanceDecisionMessageForUpdate(ctx context.Context, tx *gorm.DB, messageID uuid.UUID, scope Scope) (*runtimemodel.Message, error) {
	var message runtimemodel.Message
	if err := tx.WithContext(ctx).
		Table("chat_runtime_messages AS m").
		Select("m.*").
		Joins("JOIN chat_runtime_conversations AS c ON c.id = m.conversation_id").
		Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "m"}}).
		Where("m.id = ? AND c.organization_id = ? AND c.account_id = ? AND m.deleted_at IS NULL AND c.deleted_at IS NULL", messageID, scope.OrganizationID, scope.AccountID).
		Take(&message).Error; err != nil {
		return nil, err
	}
	return &message, nil
}

func (s *service) resolveToolGovernanceDecision(
	ctx context.Context,
	repos *repository.Repositories,
	scope Scope,
	conversation *runtimemodel.Conversation,
	message *runtimemodel.Message,
	correlationID string,
	req runtimedto.ToolGovernanceDecisionRequest,
	action string,
	approvalStatus string,
) (*runtimedto.ToolGovernanceDecisionResponse, map[string]interface{}, error) {
	if repos == nil || repos.Message == nil || repos.Conversation == nil {
		return nil, nil, fmt.Errorf("aichat repository is not configured")
	}
	if conversation == nil || message == nil || message.ConversationID != conversation.ID {
		return nil, nil, fmt.Errorf("%w: message belongs to another conversation", ErrInvalidInput)
	}

	event, ok := toolGovernanceDecisionEventFromMetadata(message.Metadata, correlationID)
	if !ok {
		return nil, nil, fmt.Errorf("%w: tool governance approval event not found", ErrNotFound)
	}
	if previous := strings.TrimSpace(stringFromAny(event["approval_status"])); previous != "" {
		if previous == approvalStatus {
			return toolGovernanceDecisionResponse(conversation.ID, message.ID, correlationID, action, approvalStatus, req.RememberForSession, nil, event), nil, nil
		}
		return nil, nil, fmt.Errorf("%w: tool governance approval already resolved", ErrInvalidInput)
	}
	if err := ensureApprovableToolGovernanceDecisionEvent(event, correlationID); err != nil {
		return nil, nil, err
	}
	if err := ensurePendingToolGovernanceDecisionMessage(message); err != nil {
		return nil, nil, err
	}

	now := time.Now().UTC()
	resolution := map[string]interface{}{
		"action":               action,
		"approval_status":      approvalStatus,
		"reason":               strings.TrimSpace(req.Reason),
		"resolved_at":          now.Format(time.RFC3339),
		"resolved_by":          scope.AccountID.String(),
		"remember_for_session": req.RememberForSession,
	}
	if action == toolGovernanceActionReject {
		resolution["model_feedback"] = map[string]interface{}{
			"status":         "user_rejected",
			"correlation_id": correlationID,
			"reason":         strings.TrimSpace(req.Reason),
			"instruction":    "The user rejected this tool call. Do not execute it; continue with a safe alternative or explain what can be done without the action.",
		}
	}

	var sessionGrant map[string]interface{}
	if action == toolGovernanceActionApprove {
		approvedGrant := toolGovernanceSessionGrantFromEvent(event, conversation.ID.String(), scope, now)
		resolution["approved_grant"] = approvedGrant
		if req.RememberForSession {
			sessionGrant = approvedGrant
			resolution["session_grant"] = sessionGrant
		}
	}
	updatedEvent := resolvedToolGovernanceDecisionEvent(event, resolution)
	messageMetadata := mergeToolGovernanceDecisionMetadata(message.Metadata, updatedEvent)
	messageMetadata = resolveToolGovernanceContinuationMetadata(messageMetadata, correlationID, resolution)
	if approvedGrant := governanceMapFromAny(resolution["approved_grant"]); len(approvedGrant) > 0 {
		messageMetadata = appendToolGovernanceOneShotGrant(messageMetadata, approvedGrant)
	}
	conversationMetadata := copyStringAnyMap(conversation.Metadata)
	if sessionGrant != nil {
		conversationMetadata = appendToolGovernanceSessionGrant(conversationMetadata, sessionGrant)
	}

	if err := repos.Message.UpdateMetadataAnyStatus(ctx, message.ID, messageMetadata); err != nil {
		return nil, nil, err
	}
	message.Metadata = messageMetadata
	if sessionGrant != nil {
		if err := repos.Conversation.UpdateMetadata(ctx, conversation.ID, conversationMetadata); err != nil {
			return nil, nil, err
		}
		conversation.Metadata = conversationMetadata
	}

	response := toolGovernanceDecisionResponse(conversation.ID, message.ID, correlationID, action, approvalStatus, req.RememberForSession, sessionGrant, updatedEvent)
	return response, updatedEvent, nil
}

func normalizeToolGovernanceApprovalAction(action string) (string, string, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case toolGovernanceActionApprove, "approved", "allow", "allowed":
		return toolGovernanceActionApprove, toolGovernanceApprovalStatusApproved, nil
	case toolGovernanceActionReject, "rejected", "deny", "denied":
		return toolGovernanceActionReject, toolGovernanceApprovalStatusRejected, nil
	default:
		return "", "", fmt.Errorf("%w: action must be approve or reject", ErrInvalidInput)
	}
}

func toolGovernanceDecisionResponse(
	conversationID uuid.UUID,
	messageID uuid.UUID,
	correlationID string,
	action string,
	approvalStatus string,
	rememberForSession bool,
	sessionGrant map[string]interface{},
	event map[string]interface{},
) *runtimedto.ToolGovernanceDecisionResponse {
	return &runtimedto.ToolGovernanceDecisionResponse{
		ConversationID:     conversationID.String(),
		MessageID:          messageID.String(),
		CorrelationID:      correlationID,
		Action:             action,
		ApprovalStatus:     approvalStatus,
		RememberForSession: rememberForSession,
		SessionGrant:       sessionGrant,
		Event:              copyStringAnyMap(event),
	}
}

func ensurePendingToolGovernanceDecisionMessage(message *runtimemodel.Message) error {
	if message == nil {
		return fmt.Errorf("%w: message is required", ErrInvalidInput)
	}
	if message.Status != runtimemodel.MessageStatusWaitingApproval {
		return fmt.Errorf("%w: message is not waiting for tool governance approval", ErrInvalidInput)
	}
	return nil
}

func ensureApprovableToolGovernanceDecisionEvent(event map[string]interface{}, correlationID string) error {
	if len(event) == 0 {
		return fmt.Errorf("%w: tool governance approval event not found", ErrNotFound)
	}
	governance := governanceMapFromAny(event["governance"])
	status := strings.TrimSpace(firstNonEmptyString(
		valueFromMap(governance, "status"),
		event["decision"],
		event["status"],
	))
	if status != "needs_approval" {
		return fmt.Errorf("%w: tool governance event is not awaiting approval", ErrInvalidInput)
	}
	if !boolMetadataValue(event["requires_approval"]) && !boolMetadataValue(governance["requires_approval"]) {
		return fmt.Errorf("%w: tool governance event does not require approval", ErrInvalidInput)
	}
	approvalEvent := governanceMapFromAny(event["approval_event"])
	if len(approvalEvent) == 0 {
		approvalEvent = governanceMapFromAny(governance["approval_event"])
	}
	if len(approvalEvent) == 0 {
		return fmt.Errorf("%w: tool governance approval event is missing approval metadata", ErrInvalidInput)
	}
	if eventCorrelationID := strings.TrimSpace(stringFromAny(approvalEvent["correlation_id"])); eventCorrelationID != "" && eventCorrelationID != correlationID {
		return fmt.Errorf("%w: tool governance approval event correlation_id mismatch", ErrInvalidInput)
	}
	if grant := governanceMapFromAny(approvalEvent["grant"]); len(grant) == 0 {
		return fmt.Errorf("%w: tool governance approval event is missing a grant scope", ErrInvalidInput)
	}
	return nil
}

func toolGovernanceDecisionEventFromMetadata(metadata map[string]interface{}, correlationID string) (map[string]interface{}, bool) {
	correlationID = strings.TrimSpace(correlationID)
	if correlationID == "" {
		return nil, false
	}
	for _, event := range mapSliceFromAny(metadataValue(metadata, "tool_governance_decisions")) {
		if toolGovernanceCorrelationID(event) == correlationID {
			return event, true
		}
	}
	for _, invocation := range skillInvocationsFromMetadata(metadataValue(metadata, "skill_invocations")) {
		if strings.TrimSpace(stringFromAny(invocation["kind"])) != "tool_governance" {
			continue
		}
		event := toolGovernanceDecisionEventFromInvocation(invocation)
		if toolGovernanceCorrelationID(event) == correlationID {
			return event, true
		}
	}
	return nil, false
}

func toolGovernanceDecisionEventFromInvocation(invocation map[string]interface{}) map[string]interface{} {
	event := map[string]interface{}{
		"conversation_id": invocation["conversation_id"],
		"message_id":      invocation["message_id"],
		"skill_id":        invocation["skill_id"],
		"tool_name":       invocation["tool_name"],
		"status":          invocation["status"],
		"duration_ms":     invocation["duration_ms"],
		"created_at":      invocation["created_at"],
		"governance":      governanceMapFromAny(invocation["governance"]),
		"runtime_id":      invocation["runtime_id"],
		"approval_status": invocation["approval_status"],
	}
	if audit := governanceMapFromAny(invocation["asset_operation_audit"]); len(audit) > 0 {
		event["asset_operation_audit"] = audit
	}
	if result := governanceMapFromAny(invocation["result"]); len(result) > 0 {
		if approvalEvent := governanceMapFromAny(result["approval_event"]); len(approvalEvent) > 0 {
			event["approval_event"] = approvalEvent
		}
	}
	if governance := governanceMapFromAny(event["governance"]); len(governance) > 0 {
		event["correlation_id"] = governance["correlation_id"]
		event["decision"] = governance["status"]
		event["requires_approval"] = governance["requires_approval"]
		event["reason"] = governance["reason"]
		if manifest := governanceMapFromAny(governance["manifest"]); len(manifest) > 0 {
			event["risk_level"] = manifest["risk_level"]
			event["effect"] = manifest["effect"]
			event["asset_type"] = manifest["asset_type"]
		}
		if audit := governanceMapFromAny(governance["asset_operation_audit"]); len(audit) > 0 {
			event["asset_operation_audit"] = audit
		}
		if approvalEvent := governanceMapFromAny(governance["approval_event"]); len(approvalEvent) > 0 {
			event["approval_event"] = approvalEvent
		}
	}
	return compactSkillInvocation(event)
}

func resolvedToolGovernanceDecisionEvent(event map[string]interface{}, resolution map[string]interface{}) map[string]interface{} {
	updated := copyStringAnyMap(event)
	if updated == nil {
		updated = map[string]interface{}{}
	}
	for key, value := range resolution {
		if value != nil {
			updated[key] = value
		}
	}
	approvalStatus := strings.TrimSpace(stringFromAny(resolution["approval_status"]))
	governance := governanceMapFromAny(updated["governance"])
	if governance == nil {
		governance = map[string]interface{}{}
	}
	if approvalStatus != "" {
		governance["approval_status"] = approvalStatus
		governance["requires_approval"] = false
		updated["requires_approval"] = false
	}
	approvalResult := toolGovernanceApprovalResultPayload(updated, governance, resolution)
	governance["approval_result"] = approvalResult
	if audit := resolvedToolGovernanceAssetOperationAudit(updated, governance, approvalResult); len(audit) > 0 {
		updated["asset_operation_audit"] = audit
		governance["asset_operation_audit"] = audit
	}
	updated["governance"] = governance
	return compactSkillInvocation(updated)
}

func toolGovernanceApprovalResultPayload(event map[string]interface{}, governance map[string]interface{}, resolution map[string]interface{}) map[string]interface{} {
	result := copyStringAnyMap(resolution)
	if result == nil {
		result = map[string]interface{}{}
	}
	approvalEvent := toolGovernanceApprovalEventFromEvent(event)
	approvedGrant := governanceMapFromAny(result["approved_grant"])
	sessionGrant := governanceMapFromAny(result["session_grant"])
	manifest := governanceMapFromAny(governance["manifest"])

	setStringIfEmpty(result, "correlation_id", toolGovernanceCorrelationID(event))
	setStringIfEmpty(result, "tool_id", approvalEvent["tool_id"], approvedGrant["tool_id"], sessionGrant["tool_id"], event["tool_id"])
	setStringIfEmpty(result, "effect", approvalEvent["effect"], approvedGrant["effect"], sessionGrant["effect"], event["effect"], manifest["effect"])
	setStringIfEmpty(result, "asset_type", approvalEvent["asset_type"], approvedGrant["asset_type"], sessionGrant["asset_type"], event["asset_type"], manifest["asset_type"])
	setStringIfEmpty(result, "risk_level", approvalEvent["risk_level"], approvedGrant["risk_level"], sessionGrant["risk_level"], event["risk_level"], manifest["risk_level"])

	assets := compactToolGovernanceGrantAssets(firstMapSlice(
		result["assets"],
		sessionGrant["assets"],
		approvedGrant["assets"],
	))
	if len(assets) == 0 {
		assets = compactToolGovernanceGrantAssets(toolGovernanceAssetsFromEvent(event, approvalEvent))
	}
	if len(assets) > 0 {
		result["assets"] = mapsToInterfaceSlice(assets)
		if int64ValueFromAny(result["asset_count"]) == 0 {
			result["asset_count"] = len(assets)
		}
	}
	return result
}

func resolvedToolGovernanceAssetOperationAudit(event map[string]interface{}, governance map[string]interface{}, resolution map[string]interface{}) map[string]interface{} {
	audit := governanceMapFromAny(event["asset_operation_audit"])
	if len(audit) == 0 {
		audit = governanceMapFromAny(governance["asset_operation_audit"])
	}
	if len(audit) == 0 {
		audit = map[string]interface{}{
			"schema_version": "tool_governance.asset_operation.v1",
			"event_type":     "asset_operation",
		}
	}
	approvalEvent := toolGovernanceApprovalEventFromEvent(event)
	approvedGrant := governanceMapFromAny(resolution["approved_grant"])
	sessionGrant := governanceMapFromAny(resolution["session_grant"])
	manifest := governanceMapFromAny(governance["manifest"])

	setStringIfEmpty(audit, "correlation_id", resolution["correlation_id"], toolGovernanceCorrelationID(event))
	setStringIfEmpty(audit, "conversation_id", event["conversation_id"], approvalEvent["conversation_id"], approvedGrant["conversation_id"], sessionGrant["conversation_id"])
	setStringIfEmpty(audit, "governance_status", governance["status"], event["decision"], event["status"])
	setStringIfEmpty(audit, "decision_reason", governance["reason"], event["reason"])
	setStringIfEmpty(audit, "tool_id", resolution["tool_id"], approvalEvent["tool_id"], approvedGrant["tool_id"], sessionGrant["tool_id"], event["tool_id"])
	setStringIfEmpty(audit, "skill_id", approvalEvent["skill_id"], event["skill_id"], manifest["skill_id"])
	setStringIfEmpty(audit, "domain", approvalEvent["domain"], manifest["domain"])
	setStringIfEmpty(audit, "effect", resolution["effect"], approvalEvent["effect"], approvedGrant["effect"], sessionGrant["effect"], event["effect"], manifest["effect"])
	setStringIfEmpty(audit, "asset_type", resolution["asset_type"], approvalEvent["asset_type"], approvedGrant["asset_type"], sessionGrant["asset_type"], event["asset_type"], manifest["asset_type"])
	setStringIfEmpty(audit, "risk_level", resolution["risk_level"], approvalEvent["risk_level"], approvedGrant["risk_level"], sessionGrant["risk_level"], event["risk_level"], manifest["risk_level"])
	setStringIfEmpty(audit, "permission_tier", approvalEvent["permission_tier"], manifest["permission_tier"])
	if _, ok := audit["requires_approval"]; !ok {
		if value := firstNonNil(event["requires_approval"], governance["requires_approval"]); value != nil {
			audit["requires_approval"] = boolMetadataValue(value)
		}
	}
	approvalStatus := strings.TrimSpace(stringFromAny(resolution["approval_status"]))
	if approvalStatus != "" {
		audit["approval_status"] = approvalStatus
	}
	for _, key := range []string{"action", "reason", "resolved_at", "resolved_by", "remember_for_session"} {
		if value, ok := resolution[key]; ok && value != nil {
			audit[key] = value
		}
	}
	if grant := governanceMapFromAny(resolution["approved_grant"]); len(grant) > 0 {
		audit["approved_grant"] = grant
		if correlationID := strings.TrimSpace(stringFromAny(grant["approval_correlation_id"])); correlationID != "" {
			audit["approved_by_correlation_id"] = correlationID
		}
	}
	if grant := governanceMapFromAny(resolution["session_grant"]); len(grant) > 0 {
		audit["session_grant"] = grant
	}
	assets := compactToolGovernanceGrantAssets(firstMapSlice(
		audit["assets"],
		resolution["assets"],
		sessionGrant["assets"],
		approvedGrant["assets"],
	))
	if len(assets) == 0 {
		assets = compactToolGovernanceGrantAssets(toolGovernanceAssetsFromEvent(event, approvalEvent))
	}
	if len(assets) > 0 {
		audit["assets"] = mapsToInterfaceSlice(assets)
		if int64ValueFromAny(audit["asset_count"]) == 0 {
			audit["asset_count"] = len(assets)
		}
	}
	return compactSkillInvocation(audit)
}

func setStringIfEmpty(target map[string]interface{}, key string, values ...interface{}) {
	if strings.TrimSpace(stringFromAny(target[key])) != "" {
		return
	}
	if value := firstNonEmptyString(values...); value != "" {
		target[key] = value
	}
}

func mergeToolGovernanceDecisionMetadata(source map[string]interface{}, event map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	correlationID := toolGovernanceCorrelationID(event)
	if correlationID == "" {
		return metadata
	}
	records := mapSliceFromAny(metadata["tool_governance_decisions"])
	replaced := false
	for index, existing := range records {
		if toolGovernanceCorrelationID(existing) == correlationID {
			records[index] = mergeInvocation(existing, event)
			replaced = true
			break
		}
	}
	if !replaced {
		records = append(records, copyStringAnyMap(event))
	}
	metadata["tool_governance_decisions"] = mapsToInterfaceSlice(records)

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	for index, invocation := range invocations {
		kind := strings.TrimSpace(stringFromAny(invocation["kind"]))
		if kind == "tool_governance" {
			if toolGovernanceCorrelationID(toolGovernanceDecisionEventFromInvocation(invocation)) != correlationID {
				continue
			}
			invocations[index] = mergeInvocation(invocation, map[string]interface{}{
				"approval_status":       event["approval_status"],
				"governance":            event["governance"],
				"asset_operation_audit": event["asset_operation_audit"],
			})
			continue
		}
		if kind == "tool_call" && governedToolCallMatchesDecision(invocation, event, correlationID) {
			update := map[string]interface{}{
				"approval_status":       event["approval_status"],
				"governance":            event["governance"],
				"asset_operation_audit": event["asset_operation_audit"],
			}
			if status := resolvedToolCallStatusFromApproval(event["approval_status"]); status != "" && canUpdateGovernedToolCallStatus(invocation) {
				update["status"] = status
			}
			invocations[index] = mergeInvocation(invocation, update)
		}
	}
	applySkillInvocationSummary(metadata, invocations)
	return metadata
}

func governedToolCallMatchesDecision(invocation map[string]interface{}, event map[string]interface{}, correlationID string) bool {
	if toolGovernanceCorrelationID(toolGovernanceDecisionEventFromInvocation(invocation)) == correlationID {
		return true
	}
	if strings.TrimSpace(stringFromAny(invocation["correlation_id"])) == correlationID {
		return true
	}
	return strings.TrimSpace(stringFromAny(invocation["skill_id"])) == strings.TrimSpace(stringFromAny(event["skill_id"])) &&
		strings.TrimSpace(stringFromAny(invocation["tool_name"])) == strings.TrimSpace(stringFromAny(event["tool_name"])) &&
		canUpdateGovernedToolCallStatus(invocation)
}

func resolvedToolCallStatusFromApproval(value interface{}) string {
	switch strings.TrimSpace(stringFromAny(value)) {
	case "rejected":
		return "rejected"
	case "approved":
		return "approved"
	default:
		return ""
	}
}

func canUpdateGovernedToolCallStatus(invocation map[string]interface{}) bool {
	switch strings.TrimSpace(stringFromAny(invocation["status"])) {
	case "", "running", "pending", "waiting_approval", "needs_approval", "needs_resolution", "approved":
		return true
	default:
		return false
	}
}

func resolveToolGovernanceContinuationMetadata(metadata map[string]interface{}, correlationID string, resolution map[string]interface{}) map[string]interface{} {
	if len(metadata) == 0 {
		return metadata
	}
	continuation := governanceMapFromAny(metadata["tool_governance_continuation"])
	if len(continuation) == 0 || toolGovernanceCorrelationID(continuation) != strings.TrimSpace(correlationID) {
		return metadata
	}
	approvalStatus := strings.TrimSpace(stringFromAny(resolution["approval_status"]))
	if approvalStatus == "" {
		approvalStatus = "resolved"
	}
	continuation["status"] = approvalStatus
	continuation["approval_status"] = approvalStatus
	for _, key := range []string{"action", "reason", "resolved_at", "resolved_by", "remember_for_session"} {
		if value, ok := resolution[key]; ok && value != nil {
			continuation[key] = value
		}
	}
	if feedback := governanceMapFromAny(resolution["model_feedback"]); len(feedback) > 0 {
		continuation["model_feedback"] = feedback
	}
	metadata["tool_governance_continuation"] = compactSkillInvocation(continuation)
	return metadata
}

func toolGovernanceSessionGrantFromEvent(event map[string]interface{}, conversationID string, scope Scope, now time.Time) map[string]interface{} {
	approvalEvent := governanceMapFromAny(event["approval_event"])
	if len(approvalEvent) == 0 {
		if governance := governanceMapFromAny(event["governance"]); len(governance) > 0 {
			approvalEvent = governanceMapFromAny(governance["approval_event"])
		}
	}
	audit := toolGovernanceAssetOperationAuditFromEvent(event)
	grant := governanceMapFromAny(approvalEvent["grant"])
	if grant == nil {
		grant = map[string]interface{}{}
	}
	if strings.TrimSpace(stringFromAny(grant["conversation_id"])) == "" {
		grant["conversation_id"] = strings.TrimSpace(conversationID)
	}
	if strings.TrimSpace(stringFromAny(grant["tool_id"])) == "" {
		grant["tool_id"] = firstNonEmptyString(stringFromAny(approvalEvent["tool_id"]), stringFromAny(event["tool_id"]))
	}
	if strings.TrimSpace(stringFromAny(grant["effect"])) == "" {
		grant["effect"] = firstNonEmptyString(stringFromAny(approvalEvent["effect"]), stringFromAny(event["effect"]))
	}
	if strings.TrimSpace(stringFromAny(grant["asset_type"])) == "" {
		grant["asset_type"] = firstNonEmptyString(stringFromAny(approvalEvent["asset_type"]), stringFromAny(event["asset_type"]))
	}
	if strings.TrimSpace(stringFromAny(grant["risk_level"])) == "" {
		grant["risk_level"] = firstNonEmptyString(stringFromAny(approvalEvent["risk_level"]), stringFromAny(event["risk_level"]))
	}
	if strings.TrimSpace(stringFromAny(grant["organization_id"])) == "" {
		grant["organization_id"] = firstNonEmptyString(
			approvalEvent["organization_id"],
			approvalEvent["organizationId"],
			audit["organization_id"],
			audit["organizationId"],
			event["organization_id"],
			event["organizationId"],
			scopeUUIDString(scope.OrganizationID),
		)
	}
	if strings.TrimSpace(firstNonEmptyString(grant["user_id"], grant["account_id"])) == "" {
		userID := firstNonEmptyString(
			approvalEvent["user_id"],
			approvalEvent["userId"],
			approvalEvent["account_id"],
			approvalEvent["accountId"],
			audit["user_id"],
			audit["userId"],
			audit["account_id"],
			audit["accountId"],
			event["user_id"],
			event["userId"],
			event["account_id"],
			event["accountId"],
			scopeUUIDString(scope.AccountID),
		)
		if userID != "" {
			grant["user_id"] = userID
		}
	}
	if strings.TrimSpace(stringFromAny(grant["skill_id"])) == "" {
		grant["skill_id"] = firstNonEmptyString(approvalEvent["skill_id"], approvalEvent["skillId"], event["skill_id"], event["skillId"], audit["skill_id"], audit["skillId"])
	}
	if strings.TrimSpace(stringFromAny(grant["provider_type"])) == "" {
		grant["provider_type"] = firstNonEmptyString(approvalEvent["provider_type"], approvalEvent["providerType"], event["provider_type"], event["providerType"], audit["provider_type"], audit["providerType"])
	}
	if strings.TrimSpace(stringFromAny(grant["provider_id"])) == "" {
		grant["provider_id"] = firstNonEmptyString(approvalEvent["provider_id"], approvalEvent["providerId"], event["provider_id"], event["providerId"], audit["provider_id"], audit["providerId"])
	}
	assets := mapSliceFromAny(grant["assets"])
	if len(assets) == 0 {
		assets = toolGovernanceAssetsFromEvent(event, approvalEvent)
	}
	if assets = compactToolGovernanceGrantAssets(assets); len(assets) > 0 {
		grant["assets"] = mapsToInterfaceSlice(assets)
	}
	if strings.TrimSpace(stringFromAny(grant["approval_correlation_id"])) == "" {
		grant["approval_correlation_id"] = firstNonEmptyString(
			stringFromAny(approvalEvent["correlation_id"]),
			stringFromAny(event["correlation_id"]),
			toolGovernanceCorrelationID(event),
		)
	}
	grant["granted_at"] = now.Format(time.RFC3339)
	if strings.TrimSpace(stringFromAny(grant["expires_at"])) == "" {
		grant["expires_at"] = now.Add(toolgovernance.DefaultSessionGrantTTL).Format(time.RFC3339)
	}
	return compactSkillInvocation(grant)
}

func toolGovernanceAssetOperationAuditFromEvent(event map[string]interface{}) map[string]interface{} {
	if audit := governanceMapFromAny(event["asset_operation_audit"]); len(audit) > 0 {
		return audit
	}
	if governance := governanceMapFromAny(event["governance"]); len(governance) > 0 {
		return governanceMapFromAny(governance["asset_operation_audit"])
	}
	return nil
}

func scopeUUIDString(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

func toolGovernanceAssetsFromEvent(event map[string]interface{}, approvalEvent map[string]interface{}) []map[string]interface{} {
	if assets := mapSliceFromAny(approvalEvent["assets"]); len(assets) > 0 {
		return assets
	}
	if assets := mapSliceFromAny(event["assets"]); len(assets) > 0 {
		return assets
	}
	if governance := governanceMapFromAny(event["governance"]); len(governance) > 0 {
		if assets := mapSliceFromAny(governance["assets"]); len(assets) > 0 {
			return assets
		}
		if nestedApprovalEvent := governanceMapFromAny(governance["approval_event"]); len(nestedApprovalEvent) > 0 {
			return mapSliceFromAny(nestedApprovalEvent["assets"])
		}
	}
	return nil
}

func compactToolGovernanceGrantAssets(assets []map[string]interface{}) []map[string]interface{} {
	if len(assets) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(assets))
	for _, asset := range assets {
		compact := map[string]interface{}{}
		if id := strings.TrimSpace(firstNonEmptyString(asset["id"], asset["asset_id"], asset["file_id"])); id != "" {
			compact["id"] = id
		}
		if assetType := strings.TrimSpace(firstNonEmptyString(asset["type"], asset["asset_type"])); assetType != "" {
			compact["type"] = assetType
		}
		if name := strings.TrimSpace(firstNonEmptyString(asset["name"], asset["asset_name"], asset["file_name"], asset["filename"], asset["title"])); name != "" {
			compact["name"] = name
		}
		if workspaceID := strings.TrimSpace(firstNonEmptyString(asset["workspace_id"], asset["workspaceId"])); workspaceID != "" {
			compact["workspace_id"] = workspaceID
		}
		if source := strings.TrimSpace(stringFromAny(asset["source"])); source != "" {
			compact["source"] = source
		}
		if len(compact) > 0 {
			out = append(out, compact)
		}
	}
	return out
}

func appendToolGovernanceSessionGrant(metadata map[string]interface{}, grant map[string]interface{}) map[string]interface{} {
	return appendToolGovernanceGrant(metadata, "tool_governance_session_grants", grant)
}

func appendToolGovernanceOneShotGrant(metadata map[string]interface{}, grant map[string]interface{}) map[string]interface{} {
	return appendToolGovernanceGrant(metadata, "tool_governance_one_shot_grants", grant)
}

func appendToolGovernanceGrant(metadata map[string]interface{}, key string, grant map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if len(grant) == 0 {
		return metadata
	}
	grants := mapSliceFromAny(metadata[key])
	grantKey := toolGovernanceSessionGrantKey(grant)
	replaced := false
	for index, existing := range grants {
		if toolGovernanceSessionGrantKey(existing) == grantKey {
			grants[index] = mergeInvocation(existing, grant)
			replaced = true
			break
		}
	}
	if !replaced {
		grants = append(grants, copyStringAnyMap(grant))
	}
	metadata[key] = mapsToInterfaceSlice(grants)
	return metadata
}

func toolGovernanceSessionGrantKey(grant map[string]interface{}) string {
	return strings.Join([]string{
		strings.TrimSpace(stringFromAny(grant["conversation_id"])),
		strings.TrimSpace(firstNonEmptyString(grant["organization_id"], grant["organizationId"])),
		strings.TrimSpace(firstNonEmptyString(grant["user_id"], grant["userId"], grant["account_id"], grant["accountId"])),
		strings.TrimSpace(firstNonEmptyString(grant["skill_id"], grant["skillId"])),
		strings.TrimSpace(firstNonEmptyString(grant["provider_type"], grant["providerType"])),
		strings.TrimSpace(firstNonEmptyString(grant["provider_id"], grant["providerId"])),
		strings.TrimSpace(stringFromAny(grant["tool_id"])),
		strings.TrimSpace(stringFromAny(grant["effect"])),
		strings.TrimSpace(stringFromAny(grant["asset_type"])),
		strings.TrimSpace(stringFromAny(grant["risk_level"])),
		toolGovernanceGrantAssetsKey(grant),
	}, "|")
}

func toolGovernanceGrantAssetsKey(grant map[string]interface{}) string {
	assets := mapSliceFromAny(grant["assets"])
	if len(assets) == 0 {
		return ""
	}
	parts := make([]string, 0, len(assets))
	for _, asset := range assets {
		parts = append(parts, toolGovernanceGrantAssetKey(asset))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func toolGovernanceGrantAssetKey(asset map[string]interface{}) string {
	assetType := strings.ToLower(strings.TrimSpace(stringFromAny(asset["type"])))
	workspaceID := strings.TrimSpace(firstNonEmptyString(asset["workspace_id"], asset["workspaceId"]))
	id := strings.TrimSpace(firstNonEmptyString(asset["id"], asset["asset_id"], asset["file_id"]))
	if id != "" {
		return strings.Join([]string{assetType, workspaceID, "id:" + id}, "/")
	}
	name := strings.ToLower(strings.TrimSpace(firstNonEmptyString(asset["name"], asset["asset_name"], asset["file_name"], asset["filename"], asset["title"])))
	if name != "" {
		return strings.Join([]string{assetType, workspaceID, "name:" + name}, "/")
	}
	return strings.Join([]string{assetType, workspaceID}, "/")
}

func toolGovernanceCorrelationID(event map[string]interface{}) string {
	if len(event) == 0 {
		return ""
	}
	if id := strings.TrimSpace(stringFromAny(event["correlation_id"])); id != "" {
		return id
	}
	if governance := governanceMapFromAny(event["governance"]); len(governance) > 0 {
		if id := strings.TrimSpace(stringFromAny(governance["correlation_id"])); id != "" {
			return id
		}
		if approvalEvent := governanceMapFromAny(governance["approval_event"]); len(approvalEvent) > 0 {
			if id := strings.TrimSpace(stringFromAny(approvalEvent["correlation_id"])); id != "" {
				return id
			}
		}
	}
	if approvalEvent := governanceMapFromAny(event["approval_event"]); len(approvalEvent) > 0 {
		return strings.TrimSpace(stringFromAny(approvalEvent["correlation_id"]))
	}
	return ""
}

func toolGovernanceRuntimeIDFromEvent(event map[string]interface{}) string {
	correlationID := toolGovernanceCorrelationID(event)
	if correlationID == "" {
		return ""
	}
	return "tool_governance:" + correlationID
}

func mapSliceFromAny(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, copyStringAnyMap(item))
		}
		return out
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if mapped := governanceMapFromAny(item); len(mapped) > 0 {
				out = append(out, mapped)
			}
		}
		return out
	default:
		if mapped := governanceMapFromAny(value); len(mapped) > 0 {
			return []map[string]interface{}{mapped}
		}
		return nil
	}
}

func mapsToInterfaceSlice(items []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

func governanceMapFromAny(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case nil:
		return nil
	case map[string]interface{}:
		return copyStringAnyMap(typed)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return nil
		}
		var mapped map[string]interface{}
		if err := json.Unmarshal(data, &mapped); err != nil {
			return nil
		}
		return mapped
	}
}

func metadataValue(metadata map[string]interface{}, key string) interface{} {
	if metadata == nil {
		return nil
	}
	return metadata[key]
}
