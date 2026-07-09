package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const operationPlanEvidenceDigestPrefix = "sha256:"

func operationPlanEvidenceLedgerTarget(invocation map[string]interface{}) map[string]interface{} {
	if len(invocation) == 0 {
		return nil
	}
	target := copyStringAnyMap(structuredTurnStateTargetFromInvocation(invocation))
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
	result := mapFromOperationContext(invocation["result"])
	resultSummary := operationPlanEvidenceLedgerResultSummary(invocation)
	args := mapFromOperationContext(invocation["arguments"])
	if isConsoleNavigatorNavigateTool(skillID, toolName) ||
		strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "client_action") {
		if target == nil {
			target = map[string]interface{}{}
		}
		target["asset_type"] = "route"
		operationPlanEvidenceSetString(target, "target_route", 240, args["target_page"], args["target_route"], args["href"], args["route"], result["target_page"], result["target_route"], resultSummary["target_page"])
		operationPlanEvidenceSetString(target, "loaded_href", 240, result["loaded_href"], result["href"], resultSummary["loaded_href"], resultSummary["href"])
		operationPlanEvidenceSetString(target, "observed_path", 240, result["observed_path"], resultSummary["observed_path"])
		return operationPlanEvidenceNonEmptyMap(target)
	}
	if strings.EqualFold(skillID, skills.SkillAgentManagement) {
		if target == nil {
			target = map[string]interface{}{}
		}
		target["asset_type"] = "agent"
		operationPlanEvidenceSetString(target, "agent_id", 120, resultSummary["agent_id"], result["agent_id"], args["agent_id"], args["id"])
		operationPlanEvidenceSetString(target, "agent_name", 180, resultSummary["agent_name"], result["agent_name"], result["name"], args["agent_name"], args["name"])
		operationPlanEvidenceSetString(target, "workspace_id", 120, resultSummary["workspace_id"], result["workspace_id"], args["workspace_id"])
		return operationPlanEvidenceNonEmptyMap(target)
	}
	if strings.EqualFold(skillID, skills.SkillFileGenerator) ||
		strings.EqualFold(skillID, skills.SkillFileManager) {
		if target == nil {
			target = map[string]interface{}{}
		}
		target["asset_type"] = "file"
		operationPlanEvidenceSetString(target, "file_id", 120, resultSummary["file_id"], result["file_id"], result["id"], args["file_id"], args["id"])
		operationPlanEvidenceSetString(target, "upload_file_id", 120, resultSummary["upload_file_id"], result["upload_file_id"], args["upload_file_id"])
		operationPlanEvidenceSetString(target, "name", 180, resultSummary["filename"], resultSummary["file_name"], result["filename"], result["file_name"], result["name"], args["filename"], args["file_name"], args["name"])
		operationPlanEvidenceSetString(target, "target", 120, resultSummary["target"], result["target"], args["target"])
		operationPlanEvidenceSetString(target, "file_format", 40, resultSummary["format"], result["format"], args["format"])
		operationPlanEvidenceSetString(target, "file_lifecycle", 80, resultSummary["lifecycle"], result["lifecycle"], args["lifecycle"])
		if ext := operationPlanEvidenceFileExtensionFromMaps(target, resultSummary, result, args); ext != "" {
			target["file_extension"] = ext
		}
		return operationPlanEvidenceNonEmptyMap(target)
	}
	return operationPlanEvidenceNonEmptyMap(target)
}

func operationPlanEvidenceLedgerResultSummary(invocation map[string]interface{}) map[string]interface{} {
	if len(invocation) == 0 {
		return nil
	}
	if summary := operationPlanResultSummary(invocation); len(summary) > 0 {
		return operationPlanEvidenceSafeSummary(summary)
	}
	return operationPlanEvidenceSafeSummary(mapFromOperationContext(invocation["result_summary"]))
}

func operationPlanEvidenceLedgerResultFacts(invocation map[string]interface{}) map[string]interface{} {
	if len(invocation) == 0 {
		return nil
	}
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
	result := mapFromOperationContext(invocation["result"])
	resultSummary := operationPlanEvidenceLedgerResultSummary(invocation)
	args := mapFromOperationContext(invocation["arguments"])
	facts := map[string]interface{}{}
	operationPlanEvidenceSetString(facts, "status", 80, resultSummary["status"], result["status"], invocation["status"])
	switch {
	case strings.EqualFold(skillID, skills.SkillAgentManagement):
		operationPlanAddAgentEvidenceFacts(facts, toolName, result, resultSummary, args)
	case strings.EqualFold(skillID, skills.SkillFileReader) && strings.EqualFold(toolName, "read_file"):
		operationPlanAddFileEvidenceFacts(facts, result, resultSummary)
	case strings.EqualFold(skillID, skills.SkillFileGenerator):
		operationPlanAddGeneratedFileEvidenceFacts(facts, result, resultSummary, args, invocation)
	case strings.EqualFold(skillID, skills.SkillFileManager) && strings.EqualFold(toolName, "save_file_to_management"):
		operationPlanAddManagedFileEvidenceFacts(facts, result, resultSummary, args)
	case isConsoleNavigatorNavigateTool(skillID, toolName):
		operationPlanAddNavigationEvidenceFacts(facts, result, resultSummary, args)
	default:
		operationPlanAddGenericEvidenceFacts(facts, result, resultSummary)
	}
	return operationPlanEvidenceNonEmptyMap(facts)
}

func operationPlanAnnotateEvidenceLedger(ledger []map[string]interface{}) []map[string]interface{} {
	var latestUpdate map[string]interface{}
	for _, entry := range ledger {
		if len(entry) == 0 || !operationPlanEvidenceEntrySucceeded(entry) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(entry["skill_id"])), skills.SkillAgentManagement) {
			continue
		}
		toolName := strings.TrimSpace(stringFromAny(entry["tool_name"]))
		switch toolName {
		case "update_agent_config", "replace_agent_skill_bindings", "replace_agent_knowledge_bindings", "replace_agent_database_bindings", "replace_agent_workflow_bindings", "replace_agent_memory_slots":
			latestUpdate = operationPlanEvidenceFactsForComparison(entry)
		case "get_agent_config":
			if len(latestUpdate) == 0 {
				continue
			}
			operationPlanAnnotateAgentConfigReadVerification(entry, latestUpdate)
		}
	}
	return ledger
}

func operationPlanCompactEvidenceTarget(target map[string]interface{}) map[string]interface{} {
	return operationPlanEvidenceCompactMap(target, []string{
		"asset_type",
		"agent_id",
		"agent_name",
		"workspace_id",
		"file_id",
		"upload_file_id",
		"name",
		"target",
		"file_format",
		"file_lifecycle",
		"file_extension",
		"target_route",
		"loaded_href",
		"observed_path",
	}, 240)
}

func operationPlanCompactEvidenceResultSummary(summary map[string]interface{}) map[string]interface{} {
	return operationPlanEvidenceCompactMap(summary, []string{
		"status",
		"effect",
		"agent_id",
		"agent_name",
		"workspace_id",
		"href",
		"route_after_delete",
		"requested_fields",
		"satisfied_fields",
		"updated_fields",
		"model_provider",
		"model",
		"agent_memory_enabled",
		"file_upload",
		"file_upload_enabled",
		"home_title",
		"input_placeholder",
		"theme_color",
		"suggested_questions",
		"enabled_skill_refs",
		"enabled_skill_count",
		"knowledge_dataset_refs",
		"knowledge_dataset_count",
		"database_binding_refs",
		"database_binding_count",
		"workflow_binding_refs",
		"workflow_binding_count",
		"suggested_question_count",
		"model_parameter_count",
		"memory_slot_config_count",
		"knowledge_retrieval_count",
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
		"draft_updated",
		"reversible",
		"operation_type",
		"operation_group_id",
		"target_count",
		"deleted_count",
		"success_count",
		"failed_count",
		"requires_refresh",
		"refresh_target",
		"target",
		"file_id",
		"upload_file_id",
		"managed_file_id",
		"tool_file_id",
		"source_file_id",
		"source_tool_file_id",
		"filename",
		"file_name",
		"managed_filename",
		"source_type",
		"mime_type",
		"format",
		"lifecycle",
		"file_format",
		"file_lifecycle",
		"file_extension",
		"transfer_method",
		"size",
		"content_length",
		"content_status",
		"content_chars",
		"content_returned_chars",
		"content_truncated",
		"content_value_preview",
		"content_value_source",
		"content_error",
		"deleted_count",
		"loaded_href",
		"observed_path",
		"target_route",
		"error",
	}, 240)
}

func operationPlanCompactEvidenceResultFacts(facts map[string]interface{}) map[string]interface{} {
	return operationPlanEvidenceCompactMap(facts, []string{
		"status",
		"effect",
		"agent_id",
		"agent_name",
		"workspace_id",
		"model_provider",
		"model",
		"agent.config.model_provider",
		"agent.config.model",
		"agent_memory_enabled",
		"agent.config.agent_memory_enabled",
		"file_upload",
		"file_upload_enabled",
		"agent.config.file_upload_enabled",
		"enabled_skill_refs",
		"agent.config.enabled_skill_refs",
		"enabled_skill_count",
		"agent.config.enabled_skill_count",
		"knowledge_dataset_refs",
		"knowledge_dataset_count",
		"database_binding_refs",
		"database_binding_count",
		"workflow_binding_refs",
		"workflow_binding_count",
		"resource_names",
		"final_resource_names",
		"binding_final_states",
		"updated_fields",
		"satisfied_fields",
		"requested_fields",
		"verified_fields",
		"field_status",
		"system_prompt_present",
		"system_prompt_chars",
		"system_prompt_digest",
		"agent.config.system_prompt_present",
		"agent.config.system_prompt_chars",
		"agent.config.system_prompt_digest",
		"file_id",
		"upload_file_id",
		"managed_file_id",
		"tool_file_id",
		"source_file_id",
		"source_tool_file_id",
		"filename",
		"file_name",
		"name",
		"target",
		"source_type",
		"transfer_method",
		"format",
		"lifecycle",
		"file_format",
		"file_lifecycle",
		"file_extension",
		"file_mime_type",
		"mime_type",
		"size",
		"content_length",
		"content_status",
		"content_chars",
		"content_returned_chars",
		"content_truncated",
		"content_value_preview",
		"content_value_source",
		"target_route",
		"loaded_href",
		"observed_path",
		"target_route_already_available",
		"route.target",
		"route.loaded_href",
		"route.observed_path",
	}, 240)
}

func operationPlanAddAgentEvidenceFacts(facts map[string]interface{}, toolName string, result map[string]interface{}, summary map[string]interface{}, args map[string]interface{}) {
	config := operationPlanEvidenceAgentConfigMap(result)
	agent := mapFromOperationContext(result["agent"])
	agentConfig := operationPlanEvidenceAgentConfigMap(agent)
	sources := []map[string]interface{}{summary, result, config, agent, agentConfig, args}
	operationPlanEvidenceSetStringFromSources(facts, "agent_id", 120, sources, "agent_id", "id")
	operationPlanEvidenceSetStringFromSources(facts, "agent_name", 180, sources, "agent_name", "name")
	operationPlanEvidenceSetStringFromSources(facts, "workspace_id", 120, sources, "workspace_id")
	operationPlanEvidenceSetStringFromSources(facts, "model_provider", 120, sources, "model_provider", "provider")
	operationPlanEvidenceCopyAlias(facts, "agent.config.model_provider", "model_provider")
	operationPlanEvidenceSetStringFromSources(facts, "model", 160, sources, "model", "model_name")
	operationPlanEvidenceCopyAlias(facts, "agent.config.model", "model")
	operationPlanEvidenceSetBoolFromSources(facts, "file_upload_enabled", sources, "file_upload_enabled", "file_upload")
	operationPlanEvidenceCopyAlias(facts, "agent.config.file_upload_enabled", "file_upload_enabled")
	operationPlanEvidenceSetBoolFromSources(facts, "agent_memory_enabled", sources, "agent_memory_enabled", "use_memory")
	operationPlanEvidenceCopyAlias(facts, "agent.config.agent_memory_enabled", "agent_memory_enabled")
	operationPlanEvidenceSetStringListFromSources(facts, "enabled_skill_refs", sources, "enabled_skill_refs", "enabled_skill_ids", "skill_ids", "agent_skill_ids")
	operationPlanEvidenceCopyAlias(facts, "agent.config.enabled_skill_refs", "enabled_skill_refs")
	operationPlanEvidenceSetIntFromSources(facts, "enabled_skill_count", sources, "enabled_skill_count")
	operationPlanEvidenceCopyAlias(facts, "agent.config.enabled_skill_count", "enabled_skill_count")
	for _, pair := range []struct {
		refs  string
		count string
		keys  []string
	}{
		{refs: "knowledge_dataset_refs", count: "knowledge_dataset_count", keys: []string{"knowledge_dataset_refs", "knowledge_dataset_ids", "dataset_ids"}},
		{refs: "database_binding_refs", count: "database_binding_count", keys: []string{"database_binding_refs", "database_bindings", "database_table_ids", "table_ids"}},
		{refs: "workflow_binding_refs", count: "workflow_binding_count", keys: []string{"workflow_binding_refs", "workflow_bindings", "workflow_ids"}},
	} {
		operationPlanEvidenceSetStringListFromSources(facts, pair.refs, sources, pair.keys...)
		operationPlanEvidenceSetIntFromSources(facts, pair.count, sources, pair.count)
	}
	operationPlanEvidenceSetStringListFromSources(facts, "resource_names", sources, "resource_names", "added_resource_names", "final_resource_names")
	operationPlanEvidenceSetStringListFromSources(facts, "final_resource_names", sources, "final_resource_names")
	operationPlanEvidenceSetStringListFromSources(facts, "updated_fields", sources, "updated_fields")
	operationPlanEvidenceSetStringListFromSources(facts, "satisfied_fields", sources, "satisfied_fields")
	operationPlanEvidenceSetStringListFromSources(facts, "requested_fields", sources, "requested_fields", "expected_updated_fields")
	if effect := operationPlanEvidenceAgentEffect(toolName, facts, sources); effect != "" {
		facts["effect"] = effect
	}
	if states := firstMapFromSources(sources, "binding_final_states"); len(states) > 0 {
		facts["binding_final_states"] = states
	}
	operationPlanAddSystemPromptEvidenceFacts(facts, sources)
	fieldStatus := operationPlanEvidenceFieldStatusFromSources(sources)
	if strings.EqualFold(toolName, "get_agent_config") {
		for _, field := range []string{"model", "file_upload_enabled", "agent_memory_enabled", "enabled_skill_ids"} {
			if operationPlanEvidenceAgentFieldPresent(facts, field) {
				fieldStatus[field] = "observed"
			}
		}
		if operationPlanEvidenceSystemPromptDigest(facts) != "" {
			fieldStatus["system_prompt"] = "observed"
		}
	}
	if strings.EqualFold(toolName, "update_agent_config") {
		for _, field := range stringSliceFromAny(facts["updated_fields"]) {
			if canonical := operationPlanEvidenceCanonicalAgentConfigField(field); canonical != "" {
				fieldStatus[canonical] = "updated"
			}
		}
		for _, field := range stringSliceFromAny(facts["satisfied_fields"]) {
			if canonical := operationPlanEvidenceCanonicalAgentConfigField(field); canonical != "" {
				if _, exists := fieldStatus[canonical]; !exists {
					fieldStatus[canonical] = "satisfied"
				}
			}
		}
	}
	if len(fieldStatus) > 0 {
		facts["field_status"] = fieldStatus
	}
}

func operationPlanEvidenceAgentEffect(toolName string, facts map[string]interface{}, sources []map[string]interface{}) string {
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	switch toolName {
	case "create_agent":
		return "agent.create"
	case "delete_agent", "delete_agents":
		return "agent.delete"
	case "get_agent", "get_agent_config":
		return "agent.config_read"
	case "replace_agent_knowledge_bindings":
		return "agent.knowledge_binding"
	case "replace_agent_skill_bindings":
		return "agent.skill_binding"
	case "replace_agent_database_bindings":
		return "agent.database_binding"
	case "replace_agent_workflow_bindings":
		return "agent.workflow_binding"
	case "update_agent_config":
		fields := append(stringSliceFromAny(facts["updated_fields"]), stringSliceFromAny(facts["satisfied_fields"])...)
		if len(fields) == 0 {
			for _, source := range sources {
				fields = append(fields, stringSliceFromAny(source["updated_fields"])...)
				fields = append(fields, stringSliceFromAny(source["satisfied_fields"])...)
			}
		}
		for _, field := range fields {
			switch operationPlanEvidenceCanonicalAgentConfigField(field) {
			case "system_prompt":
				return "agent.system_prompt_update"
			case "knowledge_dataset_ids":
				return "agent.knowledge_binding"
			case "enabled_skill_ids":
				return "agent.skill_binding"
			case "database_bindings":
				return "agent.database_binding"
			case "workflow_bindings":
				return "agent.workflow_binding"
			}
		}
		if operationPlanEvidenceSystemPromptDigest(facts) != "" {
			return "agent.system_prompt_update"
		}
		return "agent.config_update"
	default:
		return ""
	}
}

func operationPlanAddFileEvidenceFacts(facts map[string]interface{}, result map[string]interface{}, summary map[string]interface{}) {
	sources := []map[string]interface{}{summary, result}
	for _, key := range []string{"file_id", "upload_file_id", "file_name", "name", "file_extension", "file_mime_type", "content_status", "content_value_source"} {
		operationPlanEvidenceSetStringFromSources(facts, key, 240, sources, key)
	}
	operationPlanEvidenceSetIntFromSources(facts, "content_chars", sources, "content_chars")
	operationPlanEvidenceSetIntFromSources(facts, "content_returned_chars", sources, "content_returned_chars")
	operationPlanEvidenceSetBoolFromSources(facts, "content_truncated", sources, "content_truncated")
	operationPlanEvidenceSetStringFromSources(facts, "content_value_preview", 800, sources, "content_value_preview")
}

func operationPlanAddGeneratedFileEvidenceFacts(facts map[string]interface{}, result map[string]interface{}, summary map[string]interface{}, args map[string]interface{}, invocation map[string]interface{}) {
	asset := operationPlanEvidenceFirstAuditAsset(invocation)
	assetMetadata := mapFromOperationContext(asset["metadata"])
	sources := []map[string]interface{}{summary, result, args, asset, assetMetadata}
	for _, key := range []string{
		"file_id",
		"upload_file_id",
		"managed_file_id",
		"tool_file_id",
		"source_file_id",
		"source_tool_file_id",
		"filename",
		"file_name",
		"name",
		"target",
		"source_type",
		"mime_type",
		"file_mime_type",
		"format",
		"lifecycle",
	} {
		operationPlanEvidenceSetStringFromSources(facts, key, 240, sources, key)
	}
	operationPlanEvidenceSetIntFromSources(facts, "size", sources, "size")
	operationPlanEvidenceSetIntFromSources(facts, "content_length", sources, "content_length")
	if _, ok := facts["file_name"]; !ok {
		operationPlanEvidenceCopyAlias(facts, "file_name", "filename")
	}
	if _, ok := facts["filename"]; !ok {
		operationPlanEvidenceCopyAlias(facts, "filename", "file_name")
	}
	if format := strings.TrimSpace(stringFromAny(facts["format"])); format != "" {
		facts["file_format"] = format
	}
	if lifecycle := strings.TrimSpace(stringFromAny(facts["lifecycle"])); lifecycle != "" {
		facts["file_lifecycle"] = lifecycle
		if _, ok := facts["target"]; !ok && strings.EqualFold(lifecycle, "temporary") {
			facts["target"] = "temporary_artifact"
		}
	}
	if ext := operationPlanEvidenceFileExtensionFromMaps(facts, summary, result, args, assetMetadata); ext != "" {
		facts["file_extension"] = ext
	}
	if mimeType := operationPlanEvidenceMimeTypeFromMaps(facts, summary, result, args, assetMetadata); mimeType != "" {
		facts["mime_type"] = mimeType
		facts["file_mime_type"] = mimeType
	}
}

func operationPlanAddManagedFileEvidenceFacts(facts map[string]interface{}, result map[string]interface{}, summary map[string]interface{}, args map[string]interface{}) {
	sources := []map[string]interface{}{summary, result, args}
	for _, key := range []string{
		"file_id",
		"upload_file_id",
		"managed_file_id",
		"tool_file_id",
		"source_file_id",
		"source_tool_file_id",
		"filename",
		"file_name",
		"managed_filename",
		"name",
		"target",
		"source_type",
		"transfer_method",
		"mime_type",
		"file_mime_type",
		"format",
		"lifecycle",
	} {
		operationPlanEvidenceSetStringFromSources(facts, key, 240, sources, key)
	}
	operationPlanEvidenceSetIntFromSources(facts, "size", sources, "size")
	if _, ok := facts["file_name"]; !ok {
		operationPlanEvidenceCopyAlias(facts, "file_name", "filename")
	}
	if _, ok := facts["filename"]; !ok {
		operationPlanEvidenceCopyAlias(facts, "filename", "file_name")
	}
	if _, ok := facts["target"]; !ok {
		facts["target"] = "managed_file"
	}
	if ext := operationPlanEvidenceFileExtensionFromMaps(facts, summary, result, args); ext != "" {
		facts["file_extension"] = ext
	}
	if mimeType := operationPlanEvidenceMimeTypeFromMaps(facts, summary, result, args); mimeType != "" {
		facts["mime_type"] = mimeType
		facts["file_mime_type"] = mimeType
	}
}

func operationPlanAddNavigationEvidenceFacts(facts map[string]interface{}, result map[string]interface{}, summary map[string]interface{}, args map[string]interface{}) {
	sources := []map[string]interface{}{summary, result, args}
	operationPlanEvidenceSetStringFromSources(facts, "target_route", 240, sources, "target_route", "target_page", "href", "route")
	operationPlanEvidenceCopyAlias(facts, "route.target", "target_route")
	operationPlanEvidenceSetStringFromSources(facts, "loaded_href", 240, sources, "loaded_href", "href")
	operationPlanEvidenceCopyAlias(facts, "route.loaded_href", "loaded_href")
	operationPlanEvidenceSetStringFromSources(facts, "observed_path", 240, sources, "observed_path")
	operationPlanEvidenceCopyAlias(facts, "route.observed_path", "observed_path")
	operationPlanEvidenceSetBoolFromSources(facts, "target_route_already_available", sources, "target_route_already_available")
}

func operationPlanAddGenericEvidenceFacts(facts map[string]interface{}, result map[string]interface{}, summary map[string]interface{}) {
	sources := []map[string]interface{}{summary, result}
	for _, key := range []string{"effect", "target", "file_id", "upload_file_id", "managed_file_id", "tool_file_id", "filename", "file_name", "managed_filename", "deleted_count", "error"} {
		operationPlanEvidenceSetStringFromSources(facts, key, 240, sources, key)
	}
}

func operationPlanEvidenceFirstAuditAsset(invocation map[string]interface{}) map[string]interface{} {
	if len(invocation) == 0 {
		return nil
	}
	sources := []map[string]interface{}{
		invocation,
		mapFromOperationContext(invocation["governance"]),
		mapFromOperationContext(mapFromOperationContext(invocation["governance"])["model_feedback"]),
	}
	for _, source := range sources {
		if len(source) == 0 {
			continue
		}
		if assets := mapSliceFromAny(source["assets"]); len(assets) > 0 {
			return assets[0]
		}
		audit := mapFromOperationContext(source["asset_operation_audit"])
		if assets := mapSliceFromAny(audit["assets"]); len(assets) > 0 {
			return assets[0]
		}
	}
	return nil
}

func operationPlanEvidenceFileExtensionFromMaps(sources ...map[string]interface{}) string {
	for _, source := range sources {
		if len(source) == 0 {
			continue
		}
		for _, key := range []string{"file_extension", "extension", "format", "file_format"} {
			if ext := operationPlanEvidenceNormalizeFileExtension(stringFromAny(source[key])); ext != "" {
				return ext
			}
		}
		for _, key := range []string{"filename", "file_name", "name", "managed_filename"} {
			if ext := operationPlanEvidenceExtensionFromFilename(stringFromAny(source[key])); ext != "" {
				return ext
			}
		}
		if ext := operationPlanEvidenceExtensionFromMimeType(stringFromAny(firstNonEmptyValue(source["mime_type"], source["file_mime_type"]))); ext != "" {
			return ext
		}
	}
	return ""
}

func operationPlanEvidenceMimeTypeFromMaps(sources ...map[string]interface{}) string {
	for _, source := range sources {
		if len(source) == 0 {
			continue
		}
		if mimeType := strings.TrimSpace(firstNonEmptyString(source["mime_type"], source["file_mime_type"])); mimeType != "" {
			return truncateRunes(mimeType, 120)
		}
		if ext := operationPlanEvidenceFileExtensionFromMaps(source); ext != "" {
			switch ext {
			case "md", "markdown":
				return "text/markdown"
			case "pdf":
				return "application/pdf"
			case "txt":
				return "text/plain"
			case "html":
				return "text/html"
			case "json":
				return "application/json"
			case "csv":
				return "text/csv"
			case "docx":
				return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
			case "pptx":
				return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
			case "xlsx":
				return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
			}
		}
	}
	return ""
}

func operationPlanEvidenceNormalizeFileExtension(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, ".")
	switch value {
	case "markdown":
		return "md"
	case "md", "pdf", "txt", "html", "json", "csv", "docx", "pptx", "xlsx", "svg", "png", "jpg", "jpeg":
		return value
	default:
		return ""
	}
}

func operationPlanEvidenceExtensionFromFilename(filename string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return ""
	}
	idx := strings.LastIndex(filename, ".")
	if idx < 0 || idx == len(filename)-1 {
		return ""
	}
	return operationPlanEvidenceNormalizeFileExtension(filename[idx+1:])
}

func operationPlanEvidenceExtensionFromMimeType(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "text/markdown":
		return "md"
	case "application/pdf":
		return "pdf"
	case "text/plain":
		return "txt"
	case "text/html":
		return "html"
	case "application/json":
		return "json"
	case "text/csv":
		return "csv"
	default:
		return ""
	}
}

func operationPlanAddSystemPromptEvidenceFacts(facts map[string]interface{}, sources []map[string]interface{}) {
	prompt := firstStringFromSources(sources, "system_prompt")
	if prompt == "" {
		return
	}
	digest := operationPlanEvidenceTextDigest(prompt)
	facts["system_prompt_present"] = true
	facts["system_prompt_chars"] = len([]rune(prompt))
	facts["system_prompt_digest"] = digest
	facts["agent.config.system_prompt_present"] = true
	facts["agent.config.system_prompt_chars"] = len([]rune(prompt))
	facts["agent.config.system_prompt_digest"] = digest
}

func operationPlanAnnotateAgentConfigReadVerification(entry map[string]interface{}, expected map[string]interface{}) {
	actual := operationPlanEvidenceFactsForComparison(entry)
	if len(actual) == 0 {
		return
	}
	facts := mapFromOperationContext(entry["result_facts"])
	if facts == nil {
		facts = map[string]interface{}{}
	}
	fieldStatus := mapFromOperationContext(facts["field_status"])
	if fieldStatus == nil {
		fieldStatus = map[string]interface{}{}
	}
	verified := stringSliceFromAny(facts["verified_fields"])
	verify := func(field string, ok bool, comparable bool) {
		if !comparable {
			return
		}
		if ok {
			fieldStatus[field] = "verified"
			verified = operationPlanEvidenceAppendUniqueStrings(verified, field)
			return
		}
		if strings.TrimSpace(stringFromAny(fieldStatus[field])) == "" {
			fieldStatus[field] = "mismatch"
		}
	}
	verify("system_prompt", operationPlanEvidenceSystemPromptDigest(expected) != "" &&
		operationPlanEvidenceSystemPromptDigest(expected) == operationPlanEvidenceSystemPromptDigest(actual),
		operationPlanEvidenceSystemPromptDigest(expected) != "")
	verify("model", operationPlanEvidenceComparableString(expected, "model") != "" &&
		strings.EqualFold(operationPlanEvidenceComparableString(expected, "model"), operationPlanEvidenceComparableString(actual, "model")),
		operationPlanEvidenceComparableString(expected, "model") != "")
	verify("file_upload_enabled", operationPlanEvidenceComparableBool(expected, "file_upload_enabled") == operationPlanEvidenceComparableBool(actual, "file_upload_enabled"),
		operationPlanEvidenceHasComparableBool(expected, "file_upload_enabled") && operationPlanEvidenceHasComparableBool(actual, "file_upload_enabled"))
	verify("agent_memory_enabled", operationPlanEvidenceComparableBool(expected, "agent_memory_enabled") == operationPlanEvidenceComparableBool(actual, "agent_memory_enabled"),
		operationPlanEvidenceHasComparableBool(expected, "agent_memory_enabled") && operationPlanEvidenceHasComparableBool(actual, "agent_memory_enabled"))
	if len(fieldStatus) > 0 {
		facts["field_status"] = fieldStatus
	}
	if verified = operationPlanEvidenceDedupeStrings(verified); len(verified) > 0 {
		facts["verified_fields"] = verified
	}
	entry["result_facts"] = facts
}

func operationPlanEvidenceFactsForComparison(entry map[string]interface{}) map[string]interface{} {
	if len(entry) == 0 {
		return nil
	}
	out := copyStringAnyMap(mapFromOperationContext(entry["result_facts"]))
	if out == nil {
		out = map[string]interface{}{}
	}
	for key, value := range mapFromOperationContext(entry["result_summary"]) {
		if _, exists := out[key]; !exists {
			out[key] = value
		}
	}
	return operationPlanEvidenceNonEmptyMap(out)
}

func operationPlanEvidenceEntrySucceeded(entry map[string]interface{}) bool {
	status := strings.ToLower(strings.TrimSpace(stringFromAny(entry["status"])))
	switch status {
	case "completed", "success", "succeeded", "approved":
		return true
	default:
		return false
	}
}

func operationPlanEvidenceSafeSummary(summary map[string]interface{}) map[string]interface{} {
	if len(summary) == 0 {
		return nil
	}
	out := copyStringAnyMap(summary)
	delete(out, "system_prompt")
	delete(out, "pre_prompt")
	delete(out, "chat_prompt_config")
	delete(out, "completion_prompt_config")
	return operationPlanCompactEvidenceResultSummary(out)
}

func operationPlanEvidenceCompactMap(source map[string]interface{}, keys []string, maxStringRunes int) map[string]interface{} {
	if len(source) == 0 {
		return nil
	}
	out := map[string]interface{}{}
	seen := map[string]struct{}{}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		value, ok := source[key]
		if !ok || value == nil {
			continue
		}
		if safe, ok := operationPlanEvidenceSafeValue(value, maxStringRunes); ok {
			out[key] = safe
		}
	}
	return operationPlanEvidenceNonEmptyMap(out)
}

func operationPlanEvidenceSafeValue(value interface{}, maxStringRunes int) (interface{}, bool) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil, false
		}
		return truncateRunes(text, maxStringRunes), true
	case bool:
		return typed, true
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed, true
	case []string:
		cleaned := operationPlanEvidenceCleanStringSlice(typed)
		if len(cleaned) == 0 {
			return nil, false
		}
		if len(cleaned) > 12 {
			cleaned = cleaned[:12]
		}
		out := make([]string, 0, len(cleaned))
		for _, item := range cleaned {
			out = append(out, truncateRunes(item, maxStringRunes))
		}
		return out, true
	case []interface{}:
		values := sanitizedStringListArgumentValue(typed)
		if len(values) == 0 {
			return nil, false
		}
		if len(values) > 12 {
			values = values[:12]
		}
		out := make([]string, 0, len(values))
		for _, item := range values {
			out = append(out, truncateRunes(item, maxStringRunes))
		}
		return out, true
	case map[string]interface{}:
		compact := map[string]interface{}{}
		for key, item := range typed {
			if safe, ok := operationPlanEvidenceSafeValue(item, maxStringRunes); ok {
				compact[key] = safe
			}
		}
		return operationPlanEvidenceNonEmptyMap(compact), len(compact) > 0
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			return nil, false
		}
		return truncateRunes(text, maxStringRunes), true
	}
}

func operationPlanEvidenceSetString(target map[string]interface{}, key string, maxRunes int, values ...interface{}) {
	for _, value := range values {
		text := strings.TrimSpace(stringFromAny(value))
		if text == "" {
			continue
		}
		target[key] = truncateRunes(text, maxRunes)
		return
	}
}

func operationPlanEvidenceSetStringFromSources(target map[string]interface{}, key string, maxRunes int, sources []map[string]interface{}, names ...string) {
	if text := firstStringFromSources(sources, names...); text != "" {
		target[key] = truncateRunes(text, maxRunes)
	}
}

func operationPlanEvidenceSetStringListFromSources(target map[string]interface{}, key string, sources []map[string]interface{}, names ...string) {
	for _, source := range sources {
		for _, name := range names {
			values := sanitizedStringListArgumentValue(source[name])
			if len(values) == 0 {
				continue
			}
			if len(values) > 12 {
				values = values[:12]
			}
			target[key] = values
			return
		}
	}
}

func operationPlanEvidenceSetBoolFromSources(target map[string]interface{}, key string, sources []map[string]interface{}, names ...string) {
	for _, source := range sources {
		for _, name := range names {
			if value, ok := operationPlanEvidenceBoolFromAny(source[name]); ok {
				target[key] = value
				return
			}
		}
	}
}

func operationPlanEvidenceSetIntFromSources(target map[string]interface{}, key string, sources []map[string]interface{}, names ...string) {
	for _, source := range sources {
		for _, name := range names {
			if value := intValueFromAny(source[name]); value > 0 {
				target[key] = value
				return
			}
		}
	}
}

func operationPlanEvidenceCopyAlias(target map[string]interface{}, alias string, source string) {
	if value, ok := target[source]; ok && value != nil {
		target[alias] = value
	}
}

func operationPlanEvidenceAgentConfigMap(source map[string]interface{}) map[string]interface{} {
	if len(source) == 0 {
		return nil
	}
	if config := mapFromOperationContext(source["config"]); len(config) > 0 {
		return config
	}
	return nil
}

func operationPlanEvidenceFieldStatusFromSources(sources []map[string]interface{}) map[string]interface{} {
	for _, source := range sources {
		if status := mapFromOperationContext(source["field_status"]); len(status) > 0 {
			return copyStringAnyMap(status)
		}
	}
	return map[string]interface{}{}
}

func operationPlanEvidenceAgentFieldPresent(facts map[string]interface{}, field string) bool {
	switch operationPlanEvidenceCanonicalAgentConfigField(field) {
	case "model":
		return strings.TrimSpace(firstNonEmptyString(facts["model"], facts["agent.config.model"], facts["model_provider"], facts["agent.config.model_provider"])) != ""
	case "file_upload_enabled":
		_, ok := operationPlanEvidenceBoolFromAny(firstNonEmptyValue(facts["file_upload_enabled"], facts["agent.config.file_upload_enabled"]))
		return ok
	case "agent_memory_enabled":
		_, ok := operationPlanEvidenceBoolFromAny(firstNonEmptyValue(facts["agent_memory_enabled"], facts["agent.config.agent_memory_enabled"]))
		return ok
	case "enabled_skill_ids":
		return len(stringSliceFromAny(firstNonEmptyValue(facts["enabled_skill_refs"], facts["agent.config.enabled_skill_refs"]))) > 0
	default:
		return false
	}
}

func operationPlanEvidenceCanonicalAgentConfigField(field string) string {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "model", "model_provider", "provider":
		return "model"
	case "system_prompt":
		return "system_prompt"
	case "agent_memory_enabled", "use_memory":
		return "agent_memory_enabled"
	case "file_upload_enabled", "file_upload":
		return "file_upload_enabled"
	case "enabled_skill_ids", "add_enabled_skill_ids", "remove_enabled_skill_ids", "agent_skill", "skill", "skills":
		return "enabled_skill_ids"
	default:
		return ""
	}
}

func operationPlanEvidenceSystemPromptDigest(facts map[string]interface{}) string {
	return strings.TrimSpace(firstNonEmptyString(facts["system_prompt_digest"], facts["agent.config.system_prompt_digest"]))
}

func operationPlanEvidenceComparableString(facts map[string]interface{}, field string) string {
	switch operationPlanEvidenceCanonicalAgentConfigField(field) {
	case "model":
		return strings.TrimSpace(firstNonEmptyString(facts["model"], facts["agent.config.model"]))
	default:
		return strings.TrimSpace(firstNonEmptyString(facts[field], facts["agent.config."+field]))
	}
}

func operationPlanEvidenceComparableBool(facts map[string]interface{}, field string) bool {
	value, _ := operationPlanEvidenceBoolFromAny(firstNonEmptyValue(facts[field], facts["agent.config."+field]))
	return value
}

func operationPlanEvidenceHasComparableBool(facts map[string]interface{}, field string) bool {
	_, ok := operationPlanEvidenceBoolFromAny(firstNonEmptyValue(facts[field], facts["agent.config."+field]))
	return ok
}

func firstStringFromSources(sources []map[string]interface{}, names ...string) string {
	for _, source := range sources {
		for _, name := range names {
			if text := strings.TrimSpace(stringFromAny(source[name])); text != "" {
				return text
			}
		}
	}
	return ""
}

func firstMapFromSources(sources []map[string]interface{}, names ...string) map[string]interface{} {
	for _, source := range sources {
		for _, name := range names {
			if value := mapFromOperationContext(source[name]); len(value) > 0 {
				return value
			}
		}
	}
	return nil
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

func operationPlanEvidenceBoolFromAny(value interface{}) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "enabled", "yes", "1":
			return true, true
		case "false", "disabled", "no", "0":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}

func operationPlanEvidenceTextDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return operationPlanEvidenceDigestPrefix + hex.EncodeToString(sum[:])
}

func operationPlanEvidenceNonEmptyMap(input map[string]interface{}) map[string]interface{} {
	if len(input) == 0 {
		return nil
	}
	return input
}

func operationPlanEvidenceCleanStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return operationPlanEvidenceDedupeStrings(out)
}

func operationPlanEvidenceAppendUniqueStrings(values []string, additions ...string) []string {
	return operationPlanEvidenceDedupeStrings(append(values, additions...))
}

func operationPlanEvidenceDedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
