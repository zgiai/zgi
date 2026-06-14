package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"gorm.io/gorm"
)

const (
	streamEventToolGovernanceDecision = "tool_governance_decision"

	toolGovernanceActionApprove = "approve"
	toolGovernanceActionReject  = "reject"

	toolGovernanceApprovalStatusApproved = "approved"
	toolGovernanceApprovalStatusRejected = "rejected"
)

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
	message, err := s.repos.Message.GetScoped(ctx, messageID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	if message.ConversationID != conversation.ID {
		return nil, fmt.Errorf("%w: message belongs to another conversation", ErrInvalidInput)
	}

	now := time.Now().UTC()
	event, ok := toolGovernanceDecisionEventFromMetadata(message.Metadata, correlationID)
	if !ok {
		return nil, fmt.Errorf("%w: tool governance approval event not found", ErrNotFound)
	}
	if previous := strings.TrimSpace(stringFromAny(event["approval_status"])); previous != "" {
		if previous == approvalStatus {
			return toolGovernanceDecisionResponse(conversation.ID, message.ID, correlationID, action, approvalStatus, req.RememberForSession, nil, event), nil
		}
		return nil, fmt.Errorf("%w: tool governance approval already resolved", ErrInvalidInput)
	}

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
	if action == toolGovernanceActionApprove && req.RememberForSession {
		sessionGrant = toolGovernanceSessionGrantFromEvent(event, conversation.ID.String(), now)
		resolution["session_grant"] = sessionGrant
	}
	updatedEvent := resolvedToolGovernanceDecisionEvent(event, resolution)
	messageMetadata := mergeToolGovernanceDecisionMetadata(message.Metadata, updatedEvent)
	conversationMetadata := copyStringAnyMap(conversation.Metadata)
	if sessionGrant != nil {
		conversationMetadata = appendToolGovernanceSessionGrant(conversationMetadata, sessionGrant)
	}

	updateErr := s.updateToolGovernanceDecisionMetadata(ctx, message.ID, conversation.ID, messageMetadata, conversationMetadata, sessionGrant != nil)
	if updateErr != nil {
		return nil, mapRepoError(updateErr)
	}

	s.appendStreamEventBestEffort(ctx, message.ID, conversation.ID, streamEventToolGovernanceDecision, updatedEvent)
	return toolGovernanceDecisionResponse(conversation.ID, message.ID, correlationID, action, approvalStatus, req.RememberForSession, sessionGrant, updatedEvent), nil
}

func (s *service) updateToolGovernanceDecisionMetadata(
	ctx context.Context,
	messageID uuid.UUID,
	conversationID uuid.UUID,
	messageMetadata map[string]interface{},
	conversationMetadata map[string]interface{},
	updateConversation bool,
) error {
	if s.repos.DB == nil {
		if err := s.repos.Message.UpdateMetadataAnyStatus(ctx, messageID, messageMetadata); err != nil {
			return err
		}
		if updateConversation {
			return s.repos.Conversation.UpdateMetadata(ctx, conversationID, conversationMetadata)
		}
		return nil
	}
	return s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		if err := txRepos.Message.UpdateMetadataAnyStatus(ctx, messageID, messageMetadata); err != nil {
			return err
		}
		if updateConversation {
			if err := txRepos.Conversation.UpdateMetadata(ctx, conversationID, conversationMetadata); err != nil {
				return err
			}
		}
		return nil
	})
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
	governance["approval_result"] = copyStringAnyMap(resolution)
	updated["governance"] = governance
	return compactSkillInvocation(updated)
}

func mergeToolGovernanceDecisionMetadata(source map[string]interface{}, event map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	correlationID := toolGovernanceCorrelationID(event)
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
		if strings.TrimSpace(stringFromAny(invocation["kind"])) != "tool_governance" {
			continue
		}
		if toolGovernanceCorrelationID(toolGovernanceDecisionEventFromInvocation(invocation)) != correlationID {
			continue
		}
		invocations[index] = mergeInvocation(invocation, map[string]interface{}{
			"approval_status": event["approval_status"],
			"governance":      event["governance"],
		})
	}
	applySkillInvocationSummary(metadata, invocations)
	return metadata
}

func toolGovernanceSessionGrantFromEvent(event map[string]interface{}, conversationID string, now time.Time) map[string]interface{} {
	approvalEvent := governanceMapFromAny(event["approval_event"])
	if len(approvalEvent) == 0 {
		if governance := governanceMapFromAny(event["governance"]); len(governance) > 0 {
			approvalEvent = governanceMapFromAny(governance["approval_event"])
		}
	}
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
	grant["granted_at"] = now.Format(time.RFC3339)
	return compactSkillInvocation(grant)
}

func appendToolGovernanceSessionGrant(metadata map[string]interface{}, grant map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if len(grant) == 0 {
		return metadata
	}
	grants := mapSliceFromAny(metadata["tool_governance_session_grants"])
	key := toolGovernanceSessionGrantKey(grant)
	replaced := false
	for index, existing := range grants {
		if toolGovernanceSessionGrantKey(existing) == key {
			grants[index] = mergeInvocation(existing, grant)
			replaced = true
			break
		}
	}
	if !replaced {
		grants = append(grants, copyStringAnyMap(grant))
	}
	metadata["tool_governance_session_grants"] = mapsToInterfaceSlice(grants)
	return metadata
}

func toolGovernanceSessionGrantKey(grant map[string]interface{}) string {
	return strings.Join([]string{
		strings.TrimSpace(stringFromAny(grant["conversation_id"])),
		strings.TrimSpace(stringFromAny(grant["tool_id"])),
		strings.TrimSpace(stringFromAny(grant["effect"])),
		strings.TrimSpace(stringFromAny(grant["asset_type"])),
		strings.TrimSpace(stringFromAny(grant["risk_level"])),
	}, "|")
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
