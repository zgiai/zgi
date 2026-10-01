package connectorfinalize

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type hostChange struct {
	Table, ID string
	Values    row
	Remove    bool
}

func object(value any) row {
	switch value := value.(type) {
	case map[string]any:
		return row(value)
	case row:
		return value
	}
	return nil
}

func cloneJSON(value any) any {
	encoded, _ := json.Marshal(value)
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var result any
	_ = decoder.Decode(&result)
	return result
}

// Only known machine fields are rewritten. Human text, tool business input,
// provider payloads, prompts and custom Skill source are never traversed.
func renameSkillFields(value any, depth int) {
	if depth > 48 {
		return
	}
	if list, ok := value.([]any); ok {
		for _, item := range list {
			renameSkillFields(item, depth+1)
		}
		return
	}
	data := object(value)
	for key, child := range data {
		switch key {
		case "skill_id":
			if child == "external-apps" {
				data[key] = "connector-runtime"
			}
		case "skill_ids", "skill_names", "selected_skill_ids", "configured_skill_ids", "loaded_skill_ids", "loaded_skill_ids_this_run", "active_skill_ids", "candidate_skill_ids", "activated_skill_ids", "exposed_candidate_skill_ids", "initial_active_skill_ids", "unused_active_skill_ids":
			if values, ok := child.([]any); ok {
				for i, value := range values {
					if value == "external-apps" {
						values[i] = "connector-runtime"
					}
				}
			}
		case "arguments", "input", "output", "result", "tool_result", "latest_tool_result", "content", "text", "prompt", "instructions", "description", "summary", "manifest", "log_context", "agent_transcript", "frozen_invocation", "frozen_payload":
			continue
		default:
			renameSkillFields(child, depth+1)
		}
	}
}

func legacyInvocation(invocation row) bool {
	// A host-marked native invocation can still carry the former hidden Skill
	// ID. Renaming that ID must not turn a live confirmation into history.
	if text(invocation, "runtime_source") == "connector" || text(invocation, "runtime_source") == "connector_history" {
		return false
	}
	return text(invocation, "skill_id") == "external-apps" || text(invocation, "tool_name") == "execute_action" || text(invocation, "tool_name") == "get_action_guide"
}

func historyStatus(invocation row, data snapshot, messageID string) string {
	// Only complete durable native coordinates can promote history to success.
	result := object(invocation["result"])
	op := object(result["operation"])
	for _, claim := range data["connector_invocation_claims"] {
		if text(claim, "correlation_id") != "connector-correlation-"+messageID {
			continue
		}
		if text(op, "logical_operation_id") == "" || text(op, "logical_operation_id") != text(claim, "logical_operation_id") || text(op, "binding_id") != text(claim, "binding_id") || text(op, "connection_id") != text(claim, "connection_id") || text(op, "connector_id") != text(claim, "connector_id") || text(op, "operation_id") != text(claim, "operation_id") {
			continue
		}
		target := object(op["target"])
		if text(target, "id") != text(claim, "target_id") || text(target, "type") != text(claim, "target_type") || text(target, "scope") != text(claim, "target_scope") || text(target, "version_id") != text(claim, "target_version_id") || text(target, "binding_instance_id") != text(claim, "binding_instance_id") {
			continue
		}
		for _, outcome := range data["connector_operation_outcomes"] {
			if text(outcome, "id") == text(result, "outcome_id") && sameClaimCoordinates(claim, outcome, historicalClaim(data, claim)) && text(outcome, "status") == text(claim, "state") {
				return text(outcome, "status")
			}
		}
	}
	return "unknown"
}

func archiveInvocation(invocation row, data snapshot, messageID, stableID string) {
	status := historyStatus(invocation, data, messageID)
	id := text(invocation, "invocation_id")
	if id == "" {
		id = text(invocation, "runtime_id")
	}
	if id == "" {
		id = "connector-history-" + digest(stableID)[:32]
	}
	// Copy only host display fields. In particular, no receipt, confirmation
	// command, input or old executable wrapper is retained in public history.
	kept := row{"kind": "tool_call", "skill_id": "connector-runtime", "runtime_source": "connector_history", "invocation_id": id, "status": "completed"}
	for _, key := range []string{"runtime_id", "call_id", "started_at", "finished_at", "duration_ms", "sequence", "tool_name"} {
		if v, ok := invocation[key]; ok {
			kept[key] = v
		}
	}
	if text(kept, "tool_name") == "execute_action" || text(kept, "tool_name") == "get_action_guide" {
		kept["tool_name"] = "connector_history"
	}
	kept["result"] = row{"connector_presentation": row{"schema_version": 1, "source_kind": "history", "invocation_id": id, "display_status": status, "redacted": true, "truncated": false}}
	clear(invocation)
	for key, value := range kept {
		invocation[key] = value
	}
}

func convertMessageMetadata(value any, data snapshot, messageID string) (any, error) {
	metadata := object(cloneJSON(value))
	if metadata == nil {
		return value, nil
	}
	hadLegacy := retiredTranscriptCalls(metadata["agent_transcript"])
	for _, receipt := range data["integration_operation_receipts"] {
		if text(receipt, "message_id") == messageID {
			hadLegacy = true
		}
	}
	if invocations, ok := metadata["skill_invocations"].([]any); ok {
		for i, raw := range invocations {
			invocation := object(raw)
			if legacyInvocation(invocation) {
				hadLegacy = true
				archiveInvocation(invocation, data, messageID, fmt.Sprintf("%s:%d", messageID, i))
			}
		}
	}
	// Old candidate pools and approval wrappers are archival evidence, not a
	// resumable execution plan. The independent native continuation is untouched.
	for _, key := range []string{"native_external_action_candidates", "native_external_action_intents", "native_external_preparation_hints", "native_external_action_optional_targets", "_native_external_action_candidates", "_native_external_action_projections", "_native_external_action_intent_matched", "_native_external_action_intent_keys", "_native_external_action_intents", "_native_external_preparation_hints", "_native_external_action_optional_targets"} {
		if _, ok := metadata[key]; ok {
			hadLegacy = true
			delete(metadata, key)
		}
	}
	for _, key := range []string{"selected_skill_ids", "configured_skill_ids", "skill_names", "native_skill_diagnostics", "operation_plan", "operation_result_summary", "turn_state", "model_invocations", "skill_invocations"} {
		renameSkillFields(row{key: metadata[key]}, 0)
	}
	if hadLegacy {
		if _, exists := metadata["connector_action_instances"]; !exists {
			metadata["connector_action_instances"] = row{"version": 1, "blocked": true, "instances": row{}}
		}
		metadata["connector_history_version"] = 1
		// Keep the model transcript as archival text, but remove its executable
		// transcript version. Recovery will rebuild context from safe history.
		delete(metadata, "agent_transcript_version")
	}
	return metadata, nil
}

// Inspect only model protocol call envelopes, never human text or business
// arguments. Historical call_skill_tool JSON is bounded by the host snapshot.
func retiredTranscriptCalls(value any) bool {
	messages, _ := value.([]any)
	for _, raw := range messages {
		message := object(raw)
		calls, _ := message["tool_calls"].([]any)
		for _, call := range calls {
			function := object(object(call)["function"])
			name := text(function, "name")
			if name == "execute_action" || name == "get_action_guide" {
				return true
			}
			if name != "call_skill_tool" {
				continue
			}
			var arguments row
			if json.Unmarshal([]byte(text(function, "arguments")), &arguments) == nil && legacyInvocation(arguments) {
				return true
			}
		}
	}
	return false
}

func hostChanges(data snapshot) ([]hostChange, error) {
	changes := []hostChange{}
	for _, table := range hostTables() {
		for _, source := range data[table] {
			values := row{}
			id := text(source, "id")
			switch table {
			case "chat_runtime_messages":
				converted, err := convertMessageMetadata(source["metadata"], data, id)
				if err != nil {
					return nil, err
				}
				if digest(converted) != digest(source["metadata"]) {
					values["metadata"] = converted
				}
			case "agents_configs", "agent_published_versions", "workflows", "workflow_runs", "workflow_run_logs", "workflow_node_runtime_logs", "chat_runtime_conversations":
				for _, column := range []string{"agent_mode", "config_snapshot", "graph", "features", "execution_metadata", "metadata", "selected_skill_ids"} {
					before, exists := source[column]
					if !exists || before == nil {
						continue
					}
					after := cloneJSON(before)
					encodedText := false
					if raw, ok := before.(string); ok && column == "agent_mode" {
						decoder := json.NewDecoder(strings.NewReader(raw))
						decoder.UseNumber()
						if decoder.Decode(&after) != nil {
							if strings.Contains(raw, "external-apps") {
								return nil, fmt.Errorf("machine configuration is not valid JSON")
							}
							continue
						}
						encodedText = true
					}
					if hasRetiredExecutableReference(after, 0) {
						return nil, fmt.Errorf("host %s requires historical schema1 conversion", table)
					}
					renameSkillFields(row{column: after}, 0)
					if encodedText {
						encoded, _ := json.Marshal(after)
						after = string(encoded)
					}
					if digest(after) != digest(before) {
						values[column] = after
					}
				}
			case "chat_runtime_organization_skill_configs", "chat_runtime_account_skill_preferences":
				if text(source, "skill_id") == "external-apps" {
					for _, other := range data[table] {
						if text(other, "skill_id") == "connector-runtime" && text(other, "organization_id") == text(source, "organization_id") && text(other, "account_id") == text(source, "account_id") {
							return nil, fmt.Errorf("conflicting hidden Skill configuration")
						}
					}
					values["skill_id"] = "connector-runtime"
				}
			case "chat_runtime_custom_skills":
				if hasRetiredExecutableReference(source["manifest"], 0) && text(source, "status") != "invalid" {
					values["status"] = "invalid"
					values["validation_error"] = "This Skill references a retired Connector protocol. Reimport it using native Connector operations."
				}
			case "workflow_run_pauses":
				if hasRetiredExecutableReference(source, 0) {
					return nil, fmt.Errorf("legacy workflow pause requires historical conversion")
				}
			case "agent_resource_bindings":
				if text(source, "binding_type") == "integration_connection" {
					if !legacyBindingCovered(data, source) {
						return nil, fmt.Errorf("shared binding requires historical conversion")
					}
					changes = append(changes, hostChange{Table: table, ID: id, Remove: true})
				}
			}
			if len(values) > 0 {
				if _, err := uuid.Parse(id); err != nil {
					return nil, fmt.Errorf("host update requires a UUID identity")
				}
				changes = append(changes, hostChange{Table: table, ID: id, Values: values})
			}
		}
	}
	return changes, nil
}

func hasRetiredExecutableReference(value any, depth int) bool {
	if depth > 48 {
		return true
	}
	if list, ok := value.([]any); ok {
		for _, child := range list {
			if hasRetiredExecutableReference(child, depth+1) {
				return true
			}
		}
		return false
	}
	for key, child := range object(value) {
		switch key {
		case "integration_bindings":
			if list, ok := child.([]any); !ok || len(list) > 0 {
				return true
			}
		case "provider_type", "runtime_type":
			if child == "external_integration" || child == "integration" {
				return true
			}
		case "tool_name":
			if child == "execute_action" || child == "get_action_guide" {
				return true
			}
		case "text", "content", "prompt", "instructions", "description", "arguments", "input", "output":
			continue
		default:
			if hasRetiredExecutableReference(child, depth+1) {
				return true
			}
		}
	}
	return false
}

func applyHostChanges(tx *gorm.DB, changes []hostChange) error {
	for _, change := range changes {
		if change.Remove {
			if change.Table != "agent_resource_bindings" || !canonicalUUID(change.ID) {
				return fmt.Errorf("shared binding retirement outside fixed capability")
			}
			result := tx.Exec("DELETE FROM public.agent_resource_bindings WHERE id=? AND binding_type='integration_connection'", change.ID)
			if result.Error != nil || result.RowsAffected != 1 {
				return fmt.Errorf("retire proven shared binding projection")
			}
			continue
		}
		values := map[string]any{}
		for key, value := range change.Values {
			if key == "skill_id" || key == "status" || key == "agent_mode" || key == "validation_error" {
				values[key] = value
				continue
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				return err
			}
			values[key] = gorm.Expr("?::jsonb", string(encoded))
		}
		result := tx.Table(change.Table).Where("id=?", change.ID).Updates(values)
		if result.Error != nil {
			return fmt.Errorf("convert %s host metadata: %w", change.Table, result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("host conversion identity changed")
		}
	}
	return nil
}
