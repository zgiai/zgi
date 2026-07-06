package skilltrace

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

// PayloadIDs identifies the conversation/message pair a skill event belongs to.
type PayloadIDs struct {
	ConversationID string
	MessageID      string
}

// SkillCallStartPayload builds the public skill_call_start event payload.
func SkillCallStartPayload(ids PayloadIDs, skillID string, toolName string, argumentsSummary map[string]interface{}) map[string]interface{} {
	return withPayloadTimestamp(map[string]interface{}{
		"conversation_id":   ids.ConversationID,
		"message_id":        ids.MessageID,
		"skill_id":          skillID,
		"tool_name":         toolName,
		"arguments":         argumentsSummary,
		"arguments_summary": argumentsSummary,
	})
}

// SkillCallEndPayload builds the public skill_call_end event payload.
func SkillCallEndPayload(ids PayloadIDs, trace skills.SkillTrace, includeKind bool) map[string]interface{} {
	payload := withPayloadTimestamp(map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"skill_id":        trace.SkillID,
		"tool_name":       trace.ToolName,
		"duration_ms":     trace.DurationMS,
		"status":          trace.Status,
	})
	if includeKind {
		payload["kind"] = trace.Kind
	}
	if trace.Message != "" {
		payload["message"] = trace.Message
	}
	if len(trace.Result) > 0 {
		payload["result"] = trace.Result
	}
	if trace.Governance != nil {
		payload["governance"] = trace.Governance
	}
	enrichSkillCallPayloadSemantics(payload, trace)
	return payload
}

func ToolGovernanceDecisionPayload(ids PayloadIDs, trace skills.SkillTrace) map[string]interface{} {
	payload := withPayloadTimestamp(map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"skill_id":        trace.SkillID,
		"tool_name":       trace.ToolName,
		"status":          trace.Status,
		"duration_ms":     trace.DurationMS,
	})
	if trace.Governance != nil {
		payload["governance"] = trace.Governance
		payload["correlation_id"] = trace.Governance.CorrelationID
		payload["decision"] = trace.Governance.Status
		payload["requires_approval"] = trace.Governance.RequiresApproval
		payload["reason"] = trace.Governance.Reason
		payload["risk_level"] = trace.Governance.Manifest.RiskLevel
		payload["effect"] = trace.Governance.Manifest.Effect
		payload["asset_type"] = trace.Governance.Manifest.AssetType
		if len(trace.Governance.AssetOperationAudit) > 0 {
			payload["asset_operation_audit"] = trace.Governance.AssetOperationAudit
		}
		if trace.Governance.ApprovalEvent != nil {
			payload["approval_event"] = trace.Governance.ApprovalEvent
		}
	}
	return payload
}

// SkillCallErrorPayload builds the public skill_call_error event payload.
func SkillCallErrorPayload(ids PayloadIDs, trace skills.SkillTrace, status string, includeKind bool) map[string]interface{} {
	resolvedStatus := strings.TrimSpace(status)
	if strings.TrimSpace(trace.Kind) == "guardrail" && strings.TrimSpace(trace.Status) != "" {
		resolvedStatus = strings.TrimSpace(trace.Status)
	}
	payload := withPayloadTimestamp(map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"skill_id":        trace.SkillID,
		"tool_name":       trace.ToolName,
		"duration_ms":     trace.DurationMS,
		"status":          resolvedStatus,
		"message":         trace.Error,
	})
	if includeKind {
		payload["kind"] = trace.Kind
	}
	if trace.Governance != nil {
		payload["governance"] = trace.Governance
	}
	enrichSkillCallPayloadSemantics(payload, trace)
	return payload
}

func enrichSkillCallPayloadSemantics(payload map[string]interface{}, trace skills.SkillTrace) {
	if payload == nil {
		return
	}
	result := trace.Result
	copyPayloadField := func(key string, value interface{}) {
		if _, exists := payload[key]; exists {
			return
		}
		if text, ok := value.(string); ok {
			if strings.TrimSpace(text) == "" {
				return
			}
			payload[key] = text
			return
		}
		if value != nil {
			payload[key] = value
		}
	}

	for _, key := range []string{"action_id", "action_type", "href", "effect", "asset_type", "correlation_id"} {
		copyPayloadField(key, result[key])
	}
	copyPayloadField("assets", result["assets"])
	if audit, ok := result["asset_operation_audit"].(map[string]interface{}); ok && len(audit) > 0 {
		copyPayloadField("asset_operation_audit", audit)
	}
	if trace.Governance == nil {
		return
	}
	copyPayloadField("correlation_id", trace.Governance.CorrelationID)
	copyPayloadField("effect", string(trace.Governance.Manifest.Effect))
	copyPayloadField("asset_type", trace.Governance.Manifest.AssetType)
	if len(trace.Governance.AssetOperationAudit) > 0 {
		copyPayloadField("asset_operation_audit", trace.Governance.AssetOperationAudit)
	}
}

// SkillLoadPayload builds the public skill_load_start event payload.
func SkillLoadPayload(ids PayloadIDs, skillID string) map[string]interface{} {
	return withPayloadTimestamp(map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"skill_id":        skillID,
	})
}

// SkillLoadEndPayload builds the public skill_load_end event payload.
func SkillLoadEndPayload(ids PayloadIDs, trace skills.SkillTrace) map[string]interface{} {
	return withPayloadTimestamp(map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"skill_id":        trace.SkillID,
		"duration_ms":     trace.DurationMS,
		"status":          trace.Status,
	})
}

// SkillReferenceReadPayload builds the public skill_reference_read event payload.
func SkillReferenceReadPayload(ids PayloadIDs, trace skills.SkillTrace, path string) map[string]interface{} {
	return withPayloadTimestamp(map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"skill_id":        trace.SkillID,
		"path":            path,
		"duration_ms":     trace.DurationMS,
		"status":          trace.Status,
	})
}

// IntermediateAnswerPayload builds the public agent_intermediate_answer event payload.
func IntermediateAnswerPayload(ids PayloadIDs, trace skills.SkillTrace, answerID string, content string, index int, done bool, status string) map[string]interface{} {
	return withPayloadTimestamp(map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"answer_id":       answerID,
		"title":           trace.Title,
		"content":         content,
		"delta":           true,
		"index":           index,
		"done":            done,
		"status":          status,
	})
}

func withPayloadTimestamp(payload map[string]interface{}) map[string]interface{} {
	now := time.Now()
	payload["created_at"] = now.Unix()
	payload["created_at_ms"] = now.UnixMilli()
	return payload
}

// SkillArtifactsFromToolMessages extracts public file artifacts from tool messages.
func SkillArtifactsFromToolMessages(ids PayloadIDs, trace skills.SkillTrace, messages []tools.ToolInvokeMessage) []map[string]interface{} {
	artifacts := make([]map[string]interface{}, 0)
	for _, message := range messages {
		if message.Type != tools.ToolInvokeMessageTypeFile || len(message.Meta) == 0 {
			continue
		}
		file, ok := message.Meta["file"].(map[string]interface{})
		if !ok || len(file) == 0 {
			continue
		}
		artifacts = append(artifacts, skillArtifactFromToolFile(ids, trace, message, file))
	}
	if artifact := skillArtifactFromManagedFileJSON(ids, trace, firstJSONToolPayload(messages)); len(artifact) > 0 && !containsSkillArtifact(artifacts, artifact) {
		artifacts = append(artifacts, artifact)
	}
	return artifacts
}

type resultSummaryBuilder func(toolName string, payload map[string]interface{}) map[string]interface{}

var resultSummaryBuilders = map[string]resultSummaryBuilder{
	skills.SkillUserMemory:        summarizeMemoryResult,
	skills.SkillConsoleNavigator:  summarizeConsoleNavigatorResult,
	skills.SkillAgentKnowledge:    summarizeAgentKnowledgeResult,
	skills.SkillInternalKnowledge: summarizeInternalKnowledgeResult,
	skills.SkillInternalDatabase:  summarizeDatabaseResult,
	skills.SkillAgentDatabase:     summarizeDatabaseResult,
	skills.SkillAgentWorkflow:     summarizeWorkflowResult,
	skills.SkillAgentManagement:   summarizeAgentManagementResult,
	skills.SkillFileManager:       summarizeFileReaderResult,
	skills.SkillFileReader:        summarizeFileReaderResult,
}

// SummarizeToolResult returns the compact trace-visible result for known skill tools.
func SummarizeToolResult(skillID string, toolName string, messages []tools.ToolInvokeMessage) map[string]interface{} {
	builder := resultSummaryBuilders[strings.TrimSpace(skillID)]
	if builder == nil {
		return nil
	}
	return builder(strings.TrimSpace(toolName), firstJSONToolPayload(messages))
}

func summarizeMemoryResult(toolName string, payload map[string]interface{}) map[string]interface{} {
	switch toolName {
	case "add_user_memory", "update_user_memory":
		return compactMemoryEntryResult(payload)
	case "delete_user_memory":
		return compactFields(payload, "result", "entry_id")
	case "read_user_memory", "list_temporary_memories":
		return map[string]interface{}{
			"entries_count": len(interfaceSlice(payload["entries"])),
		}
	default:
		return nil
	}
}

func summarizeAgentKnowledgeResult(_ string, payload map[string]interface{}) map[string]interface{} {
	return compactKnowledgeRetrieveResult(payload)
}

func summarizeInternalKnowledgeResult(toolName string, payload map[string]interface{}) map[string]interface{} {
	return compactInternalKnowledgeResult(toolName, payload)
}

func summarizeDatabaseResult(toolName string, payload map[string]interface{}) map[string]interface{} {
	return compactDatabaseResult(toolName, payload)
}

func summarizeWorkflowResult(toolName string, payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	switch strings.TrimSpace(toolName) {
	case "list_agent_workflows":
		result := compactFields(payload, "status")
		result["workflows_count"] = collectionLen(payload["workflows"])
		return result
	case "run_agent_workflow", "get_workflow_run_status":
		return compactFields(payload, "status", "workflow_run_id", "elapsed_time", "output_keys", "primary_output", "error")
	default:
		return nil
	}
}

func summarizeConsoleNavigatorResult(toolName string, payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 || strings.TrimSpace(toolName) != "navigate" {
		return nil
	}
	return compactFields(payload, "status", "event_type", "href", "label", "reason")
}

func summarizeAgentManagementResult(toolName string, payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	switch strings.TrimSpace(toolName) {
	case "list_agents":
		result := compactFields(payload, "status", "count", "limit", "workspace_id")
		result["agents_count"] = collectionLen(payload["agents"])
		return result
	case "list_available_models":
		result := compactFields(payload, "status", "use_case", "provider", "count", "total", "truncated")
		result["models_count"] = collectionLen(payload["models"])
		return result
	case "list_agent_skill_candidates":
		return compactAgentBindingCandidateListResult(payload, "data", "skills", "agent_skills", "candidates")
	case "list_agent_knowledge_candidates", "list_agent_knowledge_binding_candidates":
		return compactAgentBindingCandidateListResult(payload, "knowledge_bases", "datasets", "candidates")
	case "list_agent_database_candidates", "list_agent_database_binding_candidates":
		return compactAgentBindingCandidateListResult(payload, "database_bindings", "tables", "databases", "candidates")
	case "list_agent_database_tables":
		return compactAgentBindingCandidateListResult(payload, "tables", "database_tables", "candidates")
	case "list_agent_workflow_binding_candidates":
		return compactAgentBindingCandidateListResult(payload, "workflow_bindings", "workflows", "candidates")
	case "get_agent", "create_agent", "update_agent_identity", "delete_agent", "delete_agents":
		return compactAgentManagementOperationResult(payload)
	case "get_agent_config", "update_agent_config":
		result := compactAgentConfigOperationResult(payload)
		copyCompactField(result, payload, "draft_updated")
		return result
	case "replace_agent_memory_slots":
		return compactAgentConfigOperationResult(payload)
	case "replace_agent_skill_bindings":
		return compactAgentBindingOperationResult(payload, "agent_skill")
	case "replace_agent_knowledge_bindings":
		return compactAgentBindingOperationResult(payload, "knowledge_base")
	case "replace_agent_database_bindings":
		return compactAgentBindingOperationResult(payload, "database_table")
	case "replace_agent_workflow_bindings":
		return compactAgentBindingOperationResult(payload, "workflow")
	default:
		return nil
	}
}

func compactAgentBindingCandidateListResult(payload map[string]interface{}, collectionKeys ...string) map[string]interface{} {
	result := compactFields(payload, "status", "query", "count", "total", "limit", "truncated", "error")
	count := 0
	for _, key := range collectionKeys {
		if count = collectionLen(payload[key]); count > 0 {
			break
		}
	}
	result["candidates_count"] = count
	if samples := compactAgentCandidateSamples(payload, 3, collectionKeys...); len(samples) > 0 {
		result["candidate_samples"] = samples
	}
	return result
}

func compactAgentCandidateSamples(payload map[string]interface{}, limit int, collectionKeys ...string) []map[string]interface{} {
	if len(payload) == 0 || limit <= 0 {
		return nil
	}
	if len(recordsFromAny(payload["binding_candidates"])) > 0 {
		collectionKeys = append([]string{"binding_candidates"}, collectionKeys...)
	}
	for _, key := range collectionKeys {
		records := recordsFromAny(payload[key])
		if len(records) == 0 {
			continue
		}
		out := make([]map[string]interface{}, 0, min(len(records), limit))
		for _, record := range records {
			item := compactAgentCandidateSample(record)
			if len(item) == 0 {
				continue
			}
			out = append(out, item)
			if len(out) >= limit {
				break
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func compactAgentCandidateSample(record map[string]interface{}) map[string]interface{} {
	if len(record) == 0 {
		return nil
	}
	item := map[string]interface{}{}
	if id := firstNonEmptyString(record["id"], record["skill_id"], record["dataset_id"], record["knowledge_base_id"], record["data_source_id"], record["table_id"], record["workflow_id"], record["binding_id"]); id != "" {
		item["id"] = id
	}
	if name := firstNonEmptyString(record["name"], record["title"], record["label"], record["display_name"], record["dataset_name"], record["database_name"], record["table_name"], record["workflow_name"]); name != "" {
		item["name"] = name
	}
	for _, key := range []string{"selected", "writable"} {
		if value, ok := record[key]; ok {
			item[key] = value
		}
	}
	if binding := compactAgentCandidateBinding(record["binding"]); len(binding) > 0 {
		item["binding"] = binding
	}
	return item
}

func compactAgentCandidateBinding(value interface{}) map[string]interface{} {
	binding := recordFromAny(value)
	if len(binding) == 0 {
		return nil
	}
	item := map[string]interface{}{}
	for _, key := range []string{"data_source_id", "table_ids", "writable_table_ids", "agent_id", "workflow_id", "binding_id", "version_strategy", "version_uuid", "timeout_seconds"} {
		if value, ok := binding[key]; ok && value != nil && value != "" {
			item[key] = value
		}
	}
	return item
}

func compactAgentBindingOperationResult(payload map[string]interface{}, bindingKind string) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	result := compactFields(payload,
		"status",
		"effect",
		"agent_name",
		"draft_updated",
		"reversible",
		"error",
		"binding_kind",
		"change_action",
		"resource_count",
		"resource_names",
		"added_resource_count",
		"added_resource_names",
		"removed_resource_count",
		"removed_resource_names",
		"final_resource_count",
		"final_resource_names",
		"config_changes",
		"binding_changes",
	)
	if result == nil {
		result = map[string]interface{}{}
	}
	if agent := recordFromAny(payload["agent"]); len(agent) > 0 {
		if name := firstNonEmptyString(agent["name"], agent["agent_name"]); name != "" {
			result["agent_name"] = name
		}
	}
	if _, ok := result["binding_kind"]; !ok {
		result["binding_kind"] = bindingKind
	}
	if _, ok := result["resource_names"]; !ok {
		names := bindingResourceNames(bindingKind, payload)
		if len(names) > 0 {
			result["resource_names"] = names
		}
	}
	if _, ok := result["resource_count"]; !ok {
		names := stringSliceFromAny(result["resource_names"])
		result["resource_count"] = bindingResourceCount(bindingKind, payload, names)
	}
	return result
}

func bindingResourceNames(bindingKind string, payload map[string]interface{}) []string {
	seen := map[string]struct{}{}
	names := make([]string, 0)
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		names = append(names, name)
	}
	for _, name := range stringSliceFromAny(payload["resource_names"]) {
		add(name)
	}
	for _, record := range bindingResourceRecords(bindingKind, payload) {
		add(bindingResourceNameFromRecord(bindingKind, record))
	}
	return names
}

func bindingResourceRecords(bindingKind string, payload map[string]interface{}) []map[string]interface{} {
	config := recordFromAny(payload["config"])
	switch strings.TrimSpace(bindingKind) {
	case "agent_skill":
		if records := firstNonEmptyRecords(payload, "skills", "agent_skills", "resources", "assets"); len(records) > 0 {
			return records
		}
		return firstNonEmptyRecords(config, "skills", "agent_skills")
	case "knowledge_base":
		if records := firstNonEmptyRecords(payload, "knowledge_bases", "knowledge_bindings", "datasets", "resources", "assets"); len(records) > 0 {
			return records
		}
		return firstNonEmptyRecords(config, "knowledge_bases", "knowledge_bindings", "datasets")
	case "database_table":
		records := firstNonEmptyRecords(payload, "database_resources", "tables", "resources", "assets")
		if len(records) > 0 {
			return records
		}
		return databaseBindingTableRecords(firstNonEmptyValue(payload["database_bindings"], payload["bindings"], config["database_bindings"], config["bindings"]))
	case "workflow":
		if records := firstNonEmptyRecords(payload, "workflow_bindings", "bindings", "workflows", "resources", "assets"); len(records) > 0 {
			return records
		}
		return firstNonEmptyRecords(config, "workflow_bindings", "bindings", "workflows")
	default:
		return nil
	}
}

func bindingResourceNameFromRecord(bindingKind string, record map[string]interface{}) string {
	switch strings.TrimSpace(bindingKind) {
	case "agent_skill":
		return firstNonEmptyString(record["skill_name"], record["display_name"], record["name"], record["title"], record["label"])
	case "knowledge_base":
		return firstNonEmptyString(record["dataset_name"], record["knowledge_base_name"], record["name"], record["title"], record["label"])
	case "database_table":
		tableName := firstNonEmptyString(record["table_name"], record["database_table_name"], record["name"], record["title"], record["label"])
		databaseName := firstNonEmptyString(record["database_name"], record["data_source_name"], record["schema_name"])
		if tableName != "" && databaseName != "" {
			return databaseName + "." + tableName
		}
		return tableName
	case "workflow":
		return firstNonEmptyString(record["label"], record["binding_name"], record["workflow_name"], record["name"], record["title"])
	default:
		return firstNonEmptyString(record["name"], record["title"], record["label"])
	}
}

func bindingResourceCount(bindingKind string, payload map[string]interface{}, names []string) int {
	if len(names) > 0 {
		return len(names)
	}
	config := recordFromAny(payload["config"])
	switch strings.TrimSpace(bindingKind) {
	case "agent_skill":
		if count := firstNonZeroCollectionLen(payload, "enabled_skill_ids", "skill_ids", "skills", "agent_skills"); count > 0 {
			return count
		}
		return firstNonZeroCollectionLen(config, "enabled_skill_ids", "skill_ids", "skills", "agent_skills")
	case "knowledge_base":
		if count := firstNonZeroCollectionLen(payload, "knowledge_dataset_ids", "dataset_ids", "knowledge_base_ids", "knowledge_bases", "knowledge_bindings", "datasets"); count > 0 {
			return count
		}
		return firstNonZeroCollectionLen(config, "knowledge_dataset_ids", "dataset_ids", "knowledge_base_ids", "knowledge_bases", "knowledge_bindings", "datasets")
	case "database_table":
		if count := firstNonZeroCollectionLen(payload, "table_ids", "database_table_ids", "tables", "database_resources"); count > 0 {
			return count
		}
		return databaseBindingTableCount(firstNonEmptyValue(payload["database_bindings"], payload["bindings"], config["database_bindings"], config["bindings"]))
	case "workflow":
		if count := firstNonZeroCollectionLen(payload, "workflow_bindings", "bindings", "binding_ids", "workflow_binding_ids", "workflows"); count > 0 {
			return count
		}
		return firstNonZeroCollectionLen(config, "workflow_bindings", "bindings", "binding_ids", "workflow_binding_ids", "workflows")
	default:
		return 0
	}
}

func firstNonZeroCollectionLen(payload map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if count := collectionLen(payload[key]); count > 0 {
			return count
		}
	}
	return 0
}

func firstNonEmptyRecords(payload map[string]interface{}, keys ...string) []map[string]interface{} {
	for _, key := range keys {
		if records := recordsFromAny(payload[key]); len(records) > 0 {
			return records
		}
	}
	return nil
}

func databaseBindingTableRecords(value interface{}) []map[string]interface{} {
	bindings := recordsFromAny(value)
	if len(bindings) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0)
	for _, binding := range bindings {
		databaseName := firstNonEmptyString(binding["database_name"], binding["data_source_name"], binding["name"])
		for _, table := range recordsFromAny(firstNonEmptyValue(binding["tables"], binding["database_tables"], binding["table_bindings"])) {
			item := map[string]interface{}{}
			for key, value := range recordFromAny(table) {
				item[key] = value
			}
			if databaseName != "" {
				item["database_name"] = databaseName
			}
			out = append(out, item)
		}
	}
	return out
}

func databaseBindingTableCount(value interface{}) int {
	bindings := recordsFromAny(value)
	if len(bindings) == 0 {
		return 0
	}
	count := 0
	for _, binding := range bindings {
		if tables := recordsFromAny(firstNonEmptyValue(binding["tables"], binding["database_tables"], binding["table_bindings"])); len(tables) > 0 {
			count += len(tables)
			continue
		}
		if tableCount := collectionLen(firstNonEmptyValue(binding["table_ids"], binding["tableIds"], binding["database_table_ids"], binding["databaseTableIds"])); tableCount > 0 {
			count += tableCount
		}
	}
	return count
}

func stringSliceFromAny(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(stringFromAny(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		if text := strings.TrimSpace(stringFromAny(value)); text != "" {
			return []string{text}
		}
		return nil
	}
}

func compactAgentManagementOperationResult(payload map[string]interface{}) map[string]interface{} {
	result := compactFields(payload, "status", "effect", "agent_id", "agent_name", "href", "route_after_delete", "workspace_id", "requested_fields", "satisfied_fields", "updated_fields", "reversible", "operation_type", "operation_group_id", "target_count", "deleted_count", "failed_count", "requires_refresh", "refresh_target", "error")
	if result == nil {
		result = map[string]interface{}{}
	}
	if group := recordFromAny(payload["operation_group"]); len(group) > 0 {
		result["operation_group"] = group
	}
	agent := recordFromAny(payload["agent"])
	for _, field := range []string{"id", "name", "description", "icon_type", "icon", "workspace_id", "href"} {
		if value, ok := agent[field]; ok && value != nil && value != "" {
			result["agent_"+field] = value
		}
	}
	if _, ok := result["agent_id"]; !ok {
		if value, ok := agent["id"]; ok && value != nil && value != "" {
			result["agent_id"] = value
		}
	}
	if _, ok := result["agent_name"]; !ok {
		if value, ok := agent["name"]; ok && value != nil && value != "" {
			result["agent_name"] = value
		}
	}
	if _, ok := result["href"]; !ok {
		if value, ok := agent["href"]; ok && value != nil && value != "" {
			result["href"] = value
		}
	}
	return result
}

func compactAgentConfigOperationResult(payload map[string]interface{}) map[string]interface{} {
	result := compactAgentManagementOperationResult(payload)
	if result == nil {
		result = map[string]interface{}{}
	}
	for _, field := range []string{
		"binding_kind",
		"change_action",
		"resource_count",
		"resource_names",
		"added_resource_count",
		"added_resource_names",
		"removed_resource_count",
		"removed_resource_names",
		"final_resource_count",
		"final_resource_names",
		"binding_final_states",
		"config_changes",
		"binding_changes",
	} {
		copyCompactField(result, payload, field)
	}
	config := recordFromAny(payload["config"])
	if len(config) == 0 {
		return result
	}
	for _, field := range []string{
		"model_provider",
		"model",
		"system_prompt",
		"home_title",
		"input_placeholder",
		"theme_color",
	} {
		if field == "system_prompt" {
			if value := compactStringValue(config[field], 2000); value != "" {
				result[field] = value
			}
			continue
		}
		copyCompactField(result, config, field)
	}
	for _, field := range []string{
		"agent_memory_enabled",
		"file_upload",
		"file_upload_enabled",
	} {
		if value, ok := config[field]; ok && value != nil {
			result[field] = value
		}
	}
	counts := map[string]int{
		"enabled_skill_count":       collectionLen(config["enabled_skill_ids"]),
		"knowledge_dataset_count":   collectionLen(config["knowledge_dataset_ids"]),
		"database_binding_count":    collectionLen(config["database_bindings"]),
		"database_table_count":      agentConfigDatabaseTableCount(config["database_bindings"]),
		"workflow_binding_count":    collectionLen(config["workflow_bindings"]),
		"suggested_question_count":  collectionLen(config["suggested_questions"]),
		"model_parameter_count":     len(recordFromAny(config["model_parameters"])),
		"memory_slot_config_count":  collectionLen(firstPresent(config, "memory_slots", "agent_memory_slots")),
		"knowledge_retrieval_count": len(recordFromAny(config["knowledge_retrieval_config"])),
	}
	for key, count := range counts {
		if count > 0 {
			result[key] = count
		}
	}
	if questions := compactStringList(config["suggested_questions"], 6, 120); len(questions) > 0 {
		result["suggested_questions"] = questions
	}
	if skillIDs := compactStringList(config["enabled_skill_ids"], 24, 160); len(skillIDs) > 0 {
		result["enabled_skill_ids"] = skillIDs
	}
	if datasetIDs := compactStringList(config["knowledge_dataset_ids"], 24, 160); len(datasetIDs) > 0 {
		result["knowledge_dataset_ids"] = datasetIDs
	}
	return result
}

func agentConfigDatabaseTableCount(value interface{}) int {
	count := 0
	for _, binding := range recordsFromAny(value) {
		tableIDs := compactStringList(firstPresent(binding, "table_ids", "tableIds"), 100, 120)
		if len(tableIDs) > 0 {
			count += len(tableIDs)
			continue
		}
		count++
	}
	return count
}

func summarizeFileReaderResult(toolName string, payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	switch strings.TrimSpace(toolName) {
	case "list_visible_files":
		result := compactFields(payload, "status", "count", "selected_count")
		if files := compactFileReaderFiles(payload["files"]); len(files) > 0 {
			result["files"] = files
		}
		return result
	case "read_file":
		result := compactFields(payload,
			"status",
			"max_chars",
			"content_status",
			"content_chars",
			"content_truncated",
			"from_cache",
			"content_lifetime",
			"content_redacted_in_history",
			"handoff_recommended",
			"recommended_next_tool",
			"handoff_required_when",
			"handoff_instruction",
		)
		if file := recordFromAny(payload["file"]); len(file) > 0 {
			for _, field := range []string{"id", "name", "workspace_id", "extension", "mime_type", "size"} {
				if value, ok := file[field]; ok {
					result["file_"+field] = value
				}
			}
		}
		if content := strings.TrimSpace(stringFromAny(payload["content"])); content != "" {
			result["content_returned_chars"] = len([]rune(content))
			if preview := safeFileReadContentValuePreview(payload, content); preview != "" {
				result["content_value_preview"] = preview
				result["content_value_source"] = "read_file.content"
			}
			result["content_redacted"] = true
		} else if _, exists := payload["content"]; exists {
			result["content_redacted"] = true
		}
		if contentError := strings.TrimSpace(stringFromAny(payload["content_error"])); contentError != "" {
			result["content_error_chars"] = len([]rune(contentError))
			result["content_error_present"] = true
		}
		return result
	case "delete_file":
		result := compactFields(payload, "status", "deleted_count", "reversible", "error")
		if file := recordFromAny(payload["file"]); len(file) > 0 {
			for _, field := range []string{"id", "name", "workspace_id", "extension", "mime_type", "size"} {
				if value, ok := file[field]; ok {
					result["file_"+field] = value
				}
			}
		}
		return result
	case "save_file_to_management":
		result := compactFields(payload, "status", "target", "transfer_method", "source_type", "error")
		for _, field := range []string{"file_id", "upload_file_id", "id", "filename", "name", "file_name"} {
			copyCompactField(result, payload, field)
		}
		if file := recordFromAny(payload["file"]); len(file) > 0 {
			for _, field := range []string{"id", "name", "workspace_id", "extension", "mime_type", "size"} {
				if value, ok := file[field]; ok {
					result["file_"+field] = value
				}
			}
		}
		return result
	default:
		return nil
	}
}

const fileReadContentValuePreviewMaxRunes = 120

func safeFileReadContentValuePreview(payload map[string]interface{}, content string) string {
	if len(payload) == 0 {
		return ""
	}
	text := strings.TrimSpace(content)
	if text == "" || len([]rune(text)) > fileReadContentValuePreviewMaxRunes {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(payload["content_status"])), "extracted") {
		return ""
	}
	if boolFromAny(payload["content_truncated"]) {
		return ""
	}
	mimeType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(payload["file_mime_type"], payload["mime_type"])))
	extension := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(firstNonEmptyString(payload["file_extension"], payload["extension"])), "."))
	if file := recordFromAny(payload["file"]); len(file) > 0 {
		if mimeType == "" {
			mimeType = strings.ToLower(strings.TrimSpace(firstNonEmptyString(file["mime_type"], file["file_mime_type"])))
		}
		if extension == "" {
			extension = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(firstNonEmptyString(file["extension"], file["file_extension"])), "."))
		}
	}
	if mimeType != "" && safeFileReadContentMimeType(mimeType) {
		return text
	}
	if safeFileReadContentExtension(extension) {
		return text
	}
	return ""
}

func safeFileReadContentMimeType(mimeType string) bool {
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}
	return mimeType == "application/json" ||
		mimeType == "application/xml" ||
		mimeType == "application/yaml" ||
		mimeType == "application/x-yaml" ||
		mimeType == "text/yaml" ||
		mimeType == "text/csv" ||
		strings.HasSuffix(mimeType, "+json") ||
		strings.HasSuffix(mimeType, "+xml") ||
		strings.HasSuffix(mimeType, "+yaml")
}

func safeFileReadContentExtension(extension string) bool {
	switch extension {
	case "txt", "md", "markdown", "csv", "json", "jsonl", "xml", "yaml", "yml", "svg", "html", "htm", "log":
		return true
	default:
		return false
	}
}

func boolFromAny(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func compactFileReaderFiles(value interface{}) []map[string]interface{} {
	files := recordsFromAny(value)
	if len(files) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, min(len(files), 20))
	for index, file := range files {
		if index >= 20 {
			break
		}
		item := map[string]interface{}{}
		for _, field := range []string{"visible_index", "id", "file_id", "name", "workspace_id", "extension", "mime_type", "file_type", "size", "selected", "content_status", "content_chars", "content_truncated", "from_cache"} {
			if value, ok := file[field]; ok && value != nil && value != "" {
				item[field] = value
			}
		}
		if content := strings.TrimSpace(stringFromAny(file["content"])); content != "" {
			item["content_returned_chars"] = len([]rune(content))
			item["content_redacted"] = true
		} else if _, exists := file["content"]; exists {
			item["content_redacted"] = true
		}
		if preview := strings.TrimSpace(stringFromAny(file["content_preview"])); preview != "" {
			item["content_preview_chars"] = len([]rune(preview))
			item["content_preview_redacted"] = true
		}
		if len(item) > 0 {
			out = append(out, item)
		}
	}
	return out
}

func compactKnowledgeRetrieveResult(payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	return compactFields(payload, "query", "status", "result_count", "top_score", "source_summary", "warnings")
}

func compactInternalKnowledgeResult(toolName string, payload map[string]interface{}) map[string]interface{} {
	switch strings.TrimSpace(toolName) {
	case "list_accessible_knowledge_bases":
		return compactFields(payload, "query", "status", "result_count", "fallback_used", "limit", "warnings")
	case "retrieve_knowledge":
		return compactKnowledgeRetrieveResult(payload)
	default:
		return nil
	}
}

func compactDatabaseResult(toolName string, payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	switch strings.TrimSpace(toolName) {
	case "list_accessible_databases":
		result := compactFields(payload, "query", "limit")
		result["databases_count"] = collectionLen(payload["databases"])
		return result
	case "list_database_tables":
		result := compactDatabaseResourceResult(payload)
		result["tables_count"] = collectionLen(payload["tables"])
		return result
	case "describe_database_table":
		result := compactDatabaseResourceResult(payload)
		result["columns_count"] = collectionLen(payload["columns"])
		return result
	case "query_table_records":
		result := compactDatabaseResourceResult(payload)
		copyCompactField(result, payload, "has_more")
		copyCompactField(result, payload, "total_num")
		result["records_count"] = collectionLen(payload["records"])
		return result
	case "insert_table_records", "update_table_records", "delete_table_records":
		result := compactDatabaseResourceResult(payload)
		copyCompactField(result, payload, "affected_rows")
		return result
	default:
		return nil
	}
}

func compactDatabaseResourceResult(payload map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	dataSource := recordFromAny(payload["data_source"])
	table := recordFromAny(payload["table"])
	if name := firstNonEmptyString(dataSource["name"]); name != "" {
		result["database_name"] = name
	}
	if schemaName := firstNonEmptyString(dataSource["schema_name"]); schemaName != "" {
		result["schema_name"] = schemaName
	}
	if tableName := firstNonEmptyString(table["name"]); tableName != "" {
		result["table_name"] = tableName
	}
	return result
}

func firstJSONToolPayload(messages []tools.ToolInvokeMessage) map[string]interface{} {
	for _, message := range messages {
		if len(message.Data) > 0 {
			return message.Data
		}
		if strings.TrimSpace(message.Text) == "" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(message.Text), &payload); err == nil && len(payload) > 0 {
			return payload
		}
	}
	return map[string]interface{}{}
}

func compactMemoryEntryResult(payload map[string]interface{}) map[string]interface{} {
	return compactFields(payload, "id", "entry_id", "content", "category", "memory_type", "expires_at", "status", "enabled")
}

func compactFields(payload map[string]interface{}, keys ...string) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	result := make(map[string]interface{}, len(keys))
	for _, key := range keys {
		copyCompactField(result, payload, key)
	}
	return result
}

func copyCompactField(result map[string]interface{}, payload map[string]interface{}, key string) {
	if value, ok := payload[key]; ok && value != nil && value != "" {
		result[key] = value
	}
}

func recordFromAny(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed
	default:
		if value == nil {
			return map[string]interface{}{}
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return map[string]interface{}{}
		}
		var record map[string]interface{}
		if err := json.Unmarshal(raw, &record); err != nil {
			return map[string]interface{}{}
		}
		return record
	}
}

func firstPresent(record map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := record[key]; ok {
			return value
		}
	}
	return nil
}

func recordsFromAny(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return typed
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if record, ok := item.(map[string]interface{}); ok {
				out = append(out, record)
			}
		}
		return out
	default:
		if value == nil {
			return nil
		}
		reflected := reflect.ValueOf(value)
		if reflected.Kind() != reflect.Array && reflected.Kind() != reflect.Slice {
			return nil
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		var out []map[string]interface{}
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil
		}
		return out
	}
}

func interfaceSlice(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		return typed
	default:
		return nil
	}
}

func compactStringList(value interface{}, limit int, maxRunes int) []string {
	if value == nil || limit <= 0 || maxRunes <= 0 {
		return nil
	}
	var raw []string
	switch typed := value.(type) {
	case []string:
		raw = typed
	case []interface{}:
		raw = make([]string, 0, len(typed))
		for _, item := range typed {
			raw = append(raw, stringFromAny(item))
		}
	default:
		reflected := reflect.ValueOf(value)
		if reflected.Kind() != reflect.Array && reflected.Kind() != reflect.Slice {
			return nil
		}
		bytes, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		if err := json.Unmarshal(bytes, &raw); err != nil {
			return nil
		}
	}
	out := make([]string, 0, min(len(raw), limit))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if runes := []rune(item); len(runes) > maxRunes {
			item = string(runes[:maxRunes]) + "..."
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func compactStringValue(value interface{}, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	text := strings.TrimSpace(stringFromAny(value))
	if text == "" {
		return ""
	}
	if runes := []rune(text); len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "..."
	}
	return text
}

func collectionLen(value interface{}) int {
	if value == nil {
		return 0
	}
	typed := reflect.ValueOf(value)
	switch typed.Kind() {
	case reflect.Array, reflect.Slice:
		return typed.Len()
	default:
		return 0
	}
}

func skillArtifactFromToolFile(ids PayloadIDs, trace skills.SkillTrace, message tools.ToolInvokeMessage, file map[string]interface{}) map[string]interface{} {
	url := firstNonEmptyString(file["url"], message.Text)
	downloadURL := firstNonEmptyString(file["download_url"])
	if downloadURL == "" {
		downloadURL = appendDownloadQuery(url)
	}
	artifact := withPayloadTimestamp(map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"artifact_type":   "file",
		"skill_id":        trace.SkillID,
		"tool_name":       trace.ToolName,
		"file_id":         firstNonEmptyString(file["id"], file["related_id"]),
		"filename":        stringFromAny(file["filename"]),
		"extension":       stringFromAny(file["extension"]),
		"mime_type":       stringFromAny(file["mime_type"]),
		"size":            file["size"],
		"url":             url,
		"download_url":    downloadURL,
		"transfer_method": stringFromAny(file["transfer_method"]),
	})
	if fileType := stringFromAny(file["type"]); fileType != "" {
		artifact["file_type"] = fileType
	}
	for _, field := range []string{"target", "workspace_id", "folder_id", "upload_file_id"} {
		if value := firstNonEmptyString(file[field]); value != "" {
			artifact[field] = value
		}
	}
	if trace.Governance != nil {
		if correlationID := strings.TrimSpace(trace.Governance.CorrelationID); correlationID != "" {
			artifact["correlation_id"] = correlationID
			artifact["operation_id"] = "tool_governance:" + correlationID
		}
		if len(trace.Governance.AssetOperationAudit) > 0 {
			artifact["asset_operation_audit"] = trace.Governance.AssetOperationAudit
		}
	}
	return artifact
}

func skillArtifactFromManagedFileJSON(ids PayloadIDs, trace skills.SkillTrace, payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	if !strings.EqualFold(firstNonEmptyString(payload["target"]), "managed_file") {
		return nil
	}
	file := recordFromAny(payload["file"])
	fileID := firstNonEmptyString(payload["upload_file_id"], payload["file_id"], file["id"], file["file_id"])
	if fileID == "" {
		return nil
	}
	artifact := withPayloadTimestamp(map[string]interface{}{
		"conversation_id": ids.ConversationID,
		"message_id":      ids.MessageID,
		"artifact_type":   "file",
		"skill_id":        trace.SkillID,
		"tool_name":       trace.ToolName,
		"file_id":         fileID,
		"upload_file_id":  fileID,
		"filename":        firstNonEmptyString(payload["filename"], file["name"], file["filename"]),
		"extension":       firstNonEmptyString(payload["extension"], file["extension"]),
		"mime_type":       firstNonEmptyString(payload["mime_type"], file["mime_type"]),
		"size":            firstNonEmptyValue(payload["size"], file["size"]),
		"target":          "managed_file",
		"workspace_id":    firstNonEmptyString(payload["workspace_id"], file["workspace_id"]),
		"transfer_method": firstNonEmptyString(payload["transfer_method"]),
	})
	for _, field := range []string{"folder_id", "source_type", "source_file_id", "source_url", "url", "download_url"} {
		if value := firstNonEmptyString(payload[field], file[field]); value != "" {
			artifact[field] = value
		}
	}
	if trace.Governance != nil {
		if correlationID := strings.TrimSpace(trace.Governance.CorrelationID); correlationID != "" {
			artifact["correlation_id"] = correlationID
			artifact["operation_id"] = "tool_governance:" + correlationID
		}
		if len(trace.Governance.AssetOperationAudit) > 0 {
			artifact["asset_operation_audit"] = trace.Governance.AssetOperationAudit
		}
	}
	return artifact
}

func containsSkillArtifact(artifacts []map[string]interface{}, candidate map[string]interface{}) bool {
	candidateKey := skillArtifactKey(candidate)
	if candidateKey == "" {
		return false
	}
	for _, artifact := range artifacts {
		if skillArtifactKey(artifact) == candidateKey {
			return true
		}
	}
	return false
}

func skillArtifactKey(artifact map[string]interface{}) string {
	if len(artifact) == 0 {
		return ""
	}
	target := strings.ToLower(strings.TrimSpace(firstNonEmptyString(artifact["target"])))
	id := strings.TrimSpace(firstNonEmptyString(artifact["upload_file_id"], artifact["file_id"]))
	if id == "" {
		return ""
	}
	return target + ":" + id
}

// TraceLooksLikeTemporaryFileArtifact reports whether a completed tool trace
// produced a chat-local file artifact. Prefer the standardized artifact shape
// over skill-specific tool names so new generators work without runtime edits.
func TraceLooksLikeTemporaryFileArtifact(trace skills.SkillTrace) bool {
	if ResultLooksLikeTemporaryFileArtifact(trace.Result) {
		return true
	}
	skillID := strings.TrimSpace(trace.SkillID)
	toolName := strings.ToLower(strings.TrimSpace(trace.ToolName))
	if strings.EqualFold(skillID, skills.SkillChartGenerator) {
		return toolName == "generate_chart"
	}
	if !strings.EqualFold(skillID, skills.SkillFileGenerator) {
		return false
	}
	switch toolName {
	case "generate_file", "generate_docx", "generate_pdf", "generate_pptx":
		return true
	default:
		return false
	}
}

func ResultLooksLikeTemporaryFileArtifact(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	target := strings.ToLower(strings.TrimSpace(firstNonEmptyString(result["target"])))
	lifecycle := strings.ToLower(strings.TrimSpace(firstNonEmptyString(result["lifecycle"])))
	if target == "managed_file" || target == "file_management" ||
		lifecycle == "managed" || lifecycle == "saved_to_file_management" ||
		strings.TrimSpace(firstNonEmptyString(result["upload_file_id"], result["managed_file_id"])) != "" {
		return false
	}
	fileRef := strings.TrimSpace(firstNonEmptyString(
		result["tool_file_id"],
		result["source_tool_file_id"],
		result["source_file_id"],
		result["file_id"],
		result["id"],
		result["download_url"],
		result["url"],
	))
	if fileRef == "" {
		return false
	}
	filename := strings.TrimSpace(firstNonEmptyString(
		result["filename"],
		result["name"],
		result["file_name"],
		result["title"],
	))
	if filename == "" {
		return false
	}
	artifactType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(result["artifact_type"])))
	transferMethod := strings.ToLower(strings.TrimSpace(firstNonEmptyString(result["transfer_method"])))
	return artifactType == "file" ||
		transferMethod == "tool_file" ||
		target == "temporary_artifact" ||
		target == "chat_artifact" ||
		target == "temporary" ||
		lifecycle == "temporary"
}

func firstNonEmptyValue(values ...interface{}) interface{} {
	for _, value := range values {
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		return value
	}
	return nil
}

func firstNonEmptyString(values ...interface{}) string {
	for _, value := range values {
		if text := strings.TrimSpace(stringFromAny(value)); text != "" {
			return text
		}
	}
	return ""
}

func appendDownloadQuery(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || strings.Contains(rawURL, "download=1") {
		return rawURL
	}
	if strings.Contains(rawURL, "?") {
		return rawURL + "&download=1"
	}
	return rawURL + "?download=1"
}

func stringFromAny(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case *string:
		if typed == nil {
			return ""
		}
		return *typed
	default:
		return ""
	}
}
