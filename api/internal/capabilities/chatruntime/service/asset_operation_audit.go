package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

const assetOperationAuditMessageBatchSize = 200

// AssetOperationAuditRecord is a read-only projection of governed asset tool activity.
// It is derived from message metadata and is not an executable replay contract.
type AssetOperationAuditRecord struct {
	ID                 string                   `json:"id"`
	ConversationID     string                   `json:"conversation_id"`
	MessageID          string                   `json:"message_id"`
	RuntimeID          string                   `json:"runtime_id,omitempty"`
	CorrelationID      string                   `json:"correlation_id"`
	Source             string                   `json:"source"`
	SchemaVersion      string                   `json:"schema_version,omitempty"`
	Status             string                   `json:"status,omitempty"`
	SkillID            string                   `json:"skill_id,omitempty"`
	ToolName           string                   `json:"tool_name,omitempty"`
	ToolID             string                   `json:"tool_id,omitempty"`
	Effect             string                   `json:"effect,omitempty"`
	AssetType          string                   `json:"asset_type,omitempty"`
	RiskLevel          string                   `json:"risk_level,omitempty"`
	ApprovalStatus     string                   `json:"approval_status,omitempty"`
	GovernanceStatus   string                   `json:"governance_status,omitempty"`
	Action             string                   `json:"action,omitempty"`
	Reason             string                   `json:"reason,omitempty"`
	ResolvedAt         string                   `json:"resolved_at,omitempty"`
	ResolvedBy         string                   `json:"resolved_by,omitempty"`
	RequiresApproval   bool                     `json:"requires_approval"`
	RememberForSession bool                     `json:"remember_for_session,omitempty"`
	AssetCount         int                      `json:"asset_count"`
	WorkspaceID        string                   `json:"workspace_id,omitempty"`
	Assets             []map[string]interface{} `json:"assets,omitempty"`
	CreatedAt          int64                    `json:"created_at"`
	MessageCreatedAt   int64                    `json:"message_created_at"`
}

func (s *service) ListAssetOperationAudits(ctx context.Context, scope Scope, conversationID uuid.UUID, page, limit int) ([]AssetOperationAuditRecord, int64, error) {
	if s == nil || s.repos == nil || s.repos.Message == nil {
		return nil, 0, ErrInvalidInput
	}
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, 0, err
	}
	if _, err := s.GetConversation(ctx, scope, conversationID); err != nil {
		return nil, 0, err
	}
	limit = clampLimit(limit, 50, 200)
	var records []AssetOperationAuditRecord
	var messageOffset int
	var messageTotal int64
	for {
		messages, total, err := s.repos.Message.ListByConversationScoped(ctx, conversationID, scope.OrganizationID, scope.AccountID, assetOperationAuditMessageBatchSize, messageOffset)
		if err != nil {
			return nil, 0, mapRepoError(err)
		}
		messageTotal = total
		records = append(records, assetOperationAuditRecordsFromMessages(messages)...)
		messageOffset += len(messages)
		if len(messages) == 0 || int64(messageOffset) >= messageTotal {
			break
		}
	}
	total := int64(len(records))
	start := pageOffset(page, limit)
	if start >= len(records) {
		return []AssetOperationAuditRecord{}, total, nil
	}
	end := start + limit
	if end > len(records) {
		end = len(records)
	}
	return append([]AssetOperationAuditRecord(nil), records[start:end]...), total, nil
}

func assetOperationAuditRecordsFromMessages(messages []*runtimemodel.Message) []AssetOperationAuditRecord {
	var records []AssetOperationAuditRecord
	for _, message := range messages {
		records = append(records, assetOperationAuditRecordsFromMessage(message)...)
	}
	return records
}

func assetOperationAuditRecordsFromMessage(message *runtimemodel.Message) []AssetOperationAuditRecord {
	if message == nil || len(message.Metadata) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var records []AssetOperationAuditRecord
	for _, event := range mapSliceFromAny(metadataValue(message.Metadata, "tool_governance_decisions")) {
		record, ok := assetOperationAuditRecordFromEvent(message, event, "tool_governance_decision")
		if !ok {
			continue
		}
		seen[record.CorrelationID] = struct{}{}
		records = append(records, record)
	}
	for _, invocation := range skillInvocationsFromMetadata(metadataValue(message.Metadata, "skill_invocations")) {
		if strings.TrimSpace(stringFromAny(invocation["kind"])) != "tool_governance" {
			continue
		}
		event := toolGovernanceDecisionEventFromInvocation(invocation)
		correlationID := toolGovernanceCorrelationID(event)
		if correlationID == "" {
			continue
		}
		if _, ok := seen[correlationID]; ok {
			continue
		}
		record, ok := assetOperationAuditRecordFromEvent(message, event, "skill_invocation")
		if !ok {
			continue
		}
		seen[record.CorrelationID] = struct{}{}
		records = append(records, record)
	}
	return records
}

func assetOperationAuditRecordFromEvent(message *runtimemodel.Message, event map[string]interface{}, source string) (AssetOperationAuditRecord, bool) {
	correlationID := toolGovernanceCorrelationID(event)
	if correlationID == "" {
		return AssetOperationAuditRecord{}, false
	}
	governance := governanceMapFromAny(event["governance"])
	audit := governanceMapFromAny(event["asset_operation_audit"])
	approvalEvent := governanceMapFromAny(event["approval_event"])
	if len(approvalEvent) == 0 {
		approvalEvent = governanceMapFromAny(governance["approval_event"])
	}
	manifest := governanceMapFromAny(governance["manifest"])
	assets := compactToolGovernanceGrantAssets(firstMapSlice(
		audit["assets"],
		approvalEvent["assets"],
		event["assets"],
		governance["assets"],
	))
	workspaceID := firstAssetWorkspaceID(assets)
	approvalStatus := firstNonEmptyString(audit["approval_status"], event["approval_status"], governance["approval_status"])
	governanceStatus := firstNonEmptyString(audit["governance_status"], governance["status"], event["decision"], event["status"])
	action := firstNonEmptyString(audit["action"], event["action"])
	record := AssetOperationAuditRecord{
		ID:                 auditRecordID(message, correlationID),
		ConversationID:     firstNonEmptyString(event["conversation_id"], message.ConversationID.String()),
		MessageID:          firstNonEmptyString(event["message_id"], message.ID.String()),
		RuntimeID:          strings.TrimSpace(stringFromAny(event["runtime_id"])),
		CorrelationID:      correlationID,
		Source:             source,
		SchemaVersion:      strings.TrimSpace(stringFromAny(audit["schema_version"])),
		Status:             firstNonEmptyString(approvalStatus, governanceStatus, action),
		SkillID:            firstNonEmptyString(event["skill_id"], approvalEvent["skill_id"], audit["skill_id"]),
		ToolName:           firstNonEmptyString(event["tool_name"], audit["tool_name"]),
		ToolID:             firstNonEmptyString(audit["tool_id"], approvalEvent["tool_id"], event["tool_id"]),
		Effect:             firstNonEmptyString(audit["effect"], approvalEvent["effect"], event["effect"], manifest["effect"]),
		AssetType:          firstNonEmptyString(audit["asset_type"], approvalEvent["asset_type"], event["asset_type"], manifest["asset_type"]),
		RiskLevel:          firstNonEmptyString(audit["risk_level"], approvalEvent["risk_level"], event["risk_level"], manifest["risk_level"]),
		ApprovalStatus:     approvalStatus,
		GovernanceStatus:   governanceStatus,
		Action:             action,
		Reason:             firstNonEmptyString(audit["reason"], event["reason"], governance["reason"]),
		ResolvedAt:         firstNonEmptyString(audit["resolved_at"], event["resolved_at"]),
		ResolvedBy:         firstNonEmptyString(audit["resolved_by"], event["resolved_by"]),
		RequiresApproval:   boolMetadataValue(firstNonNil(event["requires_approval"], governance["requires_approval"])),
		RememberForSession: boolMetadataValue(firstNonNil(audit["remember_for_session"], event["remember_for_session"])),
		AssetCount:         len(assets),
		WorkspaceID:        workspaceID,
		Assets:             mapsToInterfaceMapSlice(assets),
		CreatedAt:          auditRecordCreatedAt(event, audit, message),
		MessageCreatedAt:   message.CreatedAt.Unix(),
	}
	return record, true
}

func auditRecordID(message *runtimemodel.Message, correlationID string) string {
	messageID := ""
	if message != nil {
		messageID = message.ID.String()
	}
	if messageID == "" {
		return correlationID
	}
	return messageID + ":" + correlationID
}

func firstMapSlice(values ...interface{}) []map[string]interface{} {
	for _, value := range values {
		if items := mapSliceFromAny(value); len(items) > 0 {
			return items
		}
	}
	return nil
}

func mapsToInterfaceMapSlice(values []map[string]interface{}) []map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(values))
	for _, value := range values {
		out = append(out, copyStringAnyMap(value))
	}
	return out
}

func firstAssetWorkspaceID(assets []map[string]interface{}) string {
	for _, asset := range assets {
		if workspaceID := strings.TrimSpace(stringFromAny(asset["workspace_id"])); workspaceID != "" {
			return workspaceID
		}
	}
	return ""
}

func firstNonNil(values ...interface{}) interface{} {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func auditRecordCreatedAt(event map[string]interface{}, audit map[string]interface{}, message *runtimemodel.Message) int64 {
	if createdAt := int64ValueFromAny(audit["created_at"]); createdAt > 0 {
		return createdAt
	}
	if createdAt := int64ValueFromAny(event["created_at"]); createdAt > 0 {
		return createdAt
	}
	if message == nil {
		return 0
	}
	return message.CreatedAt.Unix()
}

func int64ValueFromAny(value interface{}) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint64:
		return int64(typed)
	case float32:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return parsed
		}
		return 0
	default:
		return 0
	}
}
