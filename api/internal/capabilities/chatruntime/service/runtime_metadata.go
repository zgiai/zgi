package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	maxModelInvocationMetadataRecords = 100
	maxTurnStateMetadataItems         = 32

	modelInvocationInlineImageDataRedactionReason = "inline_image_data_omitted_from_model_invocation_metadata"
	modelInvocationRedactedDataURLToken           = "<redacted>"
)

func (s *service) persistSkillTracesBestEffort(ctx context.Context, prepared *PreparedChat, traces []skills.SkillTrace) {
	if prepared == nil || prepared.Message == nil {
		return
	}
	metadata := mergeSkillTraceMetadata(prepared.Message.Metadata, traces)
	prepared.Message.Metadata = metadata
	if s == nil || s.repos == nil || s.repos.Message == nil {
		return
	}
	_ = s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata)
}

func (s *service) persistGeneratedArtifactBestEffort(ctx context.Context, prepared *PreparedChat, artifact map[string]interface{}) {
	if prepared == nil || prepared.Message == nil || len(artifact) == 0 {
		return
	}
	metadata := mergeGeneratedArtifactMetadata(prepared.Message.Metadata, artifact)
	prepared.Message.Metadata = metadata
	if err := s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata); err != nil {
		logger.WarnContext(ctx, "failed to persist aichat generated artifact metadata", "message_id", prepared.Message.ID.String(), err)
	}
}

func (s *service) persistModelInvocationBestEffort(ctx context.Context, prepared *PreparedChat, trace skillloop.ModelInvocationTrace) {
	if prepared == nil || prepared.Message == nil {
		return
	}
	invocation := modelInvocationFromTrace(trace, runtimeUserSystemPrompt(prepared), shouldRedactModelInvocationRequest(prepared))
	if len(invocation) == 0 {
		return
	}
	metadata := mergeModelInvocationMetadata(prepared.Message.Metadata, invocation)
	prepared.Message.Metadata = metadata
	if s == nil || s.repos == nil || s.repos.Message == nil {
		return
	}
	if err := s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata); err != nil {
		logger.WarnContext(ctx, "failed to persist aichat model invocation metadata", "message_id", prepared.Message.ID.String(), err)
	}
}

func mergeGeneratedArtifactMetadata(source map[string]interface{}, artifact map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata = mergeConversationArtifactMetadata(metadata, artifact)
	storedArtifact := persistentGeneratedArtifact(artifact)
	files := generatedFilesFromMetadata(metadata["generated_files"])
	fileID := stringFromAny(storedArtifact["file_id"])
	for idx, item := range files {
		if fileID != "" && stringFromAny(item["file_id"]) == fileID {
			files[idx] = storedArtifact
			metadata["generated_files"] = files
			metadata["generated_file_count"] = len(files)
			applyOperationPlanArtifactState(metadata, files)
			applyStructuredTurnStateMetadata(metadata, sanitizeSkillInvocationsForMetadata(skillInvocationsFromMetadata(metadata["skill_invocations"])))
			return metadata
		}
	}
	files = append(files, storedArtifact)
	metadata["generated_files"] = files
	metadata["generated_file_count"] = len(files)
	applyOperationPlanArtifactState(metadata, files)
	applyStructuredTurnStateMetadata(metadata, sanitizeSkillInvocationsForMetadata(skillInvocationsFromMetadata(metadata["skill_invocations"])))
	return metadata
}

func mergeSkillTraceMetadata(source map[string]interface{}, traces []skills.SkillTrace) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if len(traces) == 0 {
		return metadata
	}
	invocations := sanitizeSkillInvocationsForMetadata(skillInvocationsFromMetadata(metadata["skill_invocations"]))
	for index, trace := range traces {
		applyTurnStateTraceMetadata(metadata, trace)
		if !visibleSkillInvocationKind(trace.Kind) {
			continue
		}
		invocation := skillInvocationFromTrace(trace, index)
		if internalPlannerFeedbackInvocation(invocation) {
			continue
		}
		invocations = upsertSkillInvocation(invocations, invocation)
	}
	applySkillInvocationSummary(metadata, invocations)
	applyOperationPlanInvocationState(metadata, invocations)
	applyOperationPlanPlannerFeedbackState(metadata, traces)
	applyStructuredTurnStateMetadata(metadata, invocations)
	return metadata
}

func applyTurnStateTraceMetadata(metadata map[string]interface{}, trace skills.SkillTrace) {
	if metadata == nil {
		return
	}
	items := turnStateItemsFromTrace(trace)
	if len(items) == 0 {
		return
	}
	current := mapFromOperationContext(metadata["turn_state"])
	stored := mapSliceFromAny(current["items"])
	for _, item := range items {
		item = sanitizeTurnStateItemForMetadata(item)
		if len(item) == 0 {
			continue
		}
		stored = upsertTurnStateItem(stored, item)
	}
	if len(stored) > maxTurnStateMetadataItems {
		stored = stored[len(stored)-maxTurnStateMetadataItems:]
	}
	metadata["turn_state"] = map[string]interface{}{
		"items":      mapsToInterfaceSlice(stored),
		"updated_at": time.Now().Unix(),
	}
	metadata["turn_state_count"] = len(stored)
	metadata["has_trace"] = true
}

func applyStructuredTurnStateMetadata(metadata map[string]interface{}, invocations []map[string]interface{}) {
	if metadata == nil {
		return
	}
	current := mapFromOperationContext(metadata["turn_state"])
	state := copyStringAnyMap(current)
	if state == nil {
		state = map[string]interface{}{}
	}
	items := mapSliceFromAny(current["items"])
	if len(items) > 0 {
		state["items"] = mapsToInterfaceSlice(items)
	}
	steps := structuredTurnStateStepsFromInvocations(invocations, 16)
	toolResults := structuredTurnStateToolResultsFromInvocations(invocations, 12)
	assets := structuredTurnStateAssetsFromInvocations(invocations, 12)
	navigations := structuredTurnStateNavigationsFromMetadata(metadata, 8)
	artifacts := structuredTurnStateArtifactsFromMetadata(metadata, 8)
	openItems := structuredTurnStateOpenItemsFromInvocations(invocations, 6)

	setStructuredTurnStateSlice(state, "steps", steps)
	setStructuredTurnStateSlice(state, "tool_results", toolResults)
	setStructuredTurnStateSlice(state, "assets", assets)
	setStructuredTurnStateSlice(state, "navigations", navigations)
	setStructuredTurnStateSlice(state, "generated_artifacts", artifacts)
	setStructuredTurnStateSlice(state, "open_items", openItems)
	if len(items) == 0 && len(steps) == 0 && len(toolResults) == 0 && len(assets) == 0 &&
		len(navigations) == 0 && len(artifacts) == 0 && len(openItems) == 0 {
		return
	}
	state["updated_at"] = time.Now().Unix()
	metadata["turn_state"] = state
	metadata["turn_state_count"] = len(items)
	metadata["turn_state_step_count"] = len(steps)
	metadata["turn_state_tool_result_count"] = len(toolResults)
	metadata["has_trace"] = true
}

func setStructuredTurnStateSlice(state map[string]interface{}, key string, items []map[string]interface{}) {
	if state == nil || strings.TrimSpace(key) == "" {
		return
	}
	if len(items) == 0 {
		delete(state, key)
		return
	}
	state[key] = mapsToInterfaceSlice(items)
}

func structuredTurnStateStepsFromInvocations(invocations []map[string]interface{}, limit int) []map[string]interface{} {
	if limit <= 0 || len(invocations) == 0 {
		return nil
	}
	start := 0
	if len(invocations) > limit {
		start = len(invocations) - limit
	}
	out := make([]map[string]interface{}, 0, min(len(invocations)-start, limit))
	for _, invocation := range invocations[start:] {
		item := structuredTurnStateStepFromInvocation(invocation)
		if len(item) == 0 {
			continue
		}
		out = append(out, item)
	}
	return out
}

func structuredTurnStateStepFromInvocation(invocation map[string]interface{}) map[string]interface{} {
	if len(invocation) == 0 {
		return nil
	}
	kind := strings.TrimSpace(stringFromAny(invocation["kind"]))
	if kind == "" {
		return nil
	}
	switch strings.ToLower(kind) {
	case "tool_call", "client_action", "skill_load", "load_skill":
	default:
		return nil
	}
	item := map[string]interface{}{
		"kind":   kind,
		"status": strings.TrimSpace(firstNonEmptyString(invocation["status"], invocation["result_status"])),
	}
	for _, key := range []string{"skill_id", "tool_name", "action_type", "title", "runtime_id"} {
		if value := strings.TrimSpace(stringFromAny(invocation[key])); value != "" {
			item[key] = truncateRunes(value, 180)
		}
	}
	if target := structuredTurnStateTargetFromInvocation(invocation); len(target) > 0 {
		item["target"] = target
	}
	if createdAt := numericOrStringValue(invocation["created_at"]); createdAt != nil {
		item["created_at"] = createdAt
	}
	if createdAtMS := numericOrStringValue(invocation["created_at_ms"]); createdAtMS != nil {
		item["created_at_ms"] = createdAtMS
	}
	if errText := structuredTurnStateInvocationError(invocation); errText != "" {
		item["error"] = errText
	}
	if strings.TrimSpace(stringFromAny(item["status"])) == "" {
		item["status"] = "recorded"
	}
	return item
}

func structuredTurnStateToolResultsFromInvocations(invocations []map[string]interface{}, limit int) []map[string]interface{} {
	if limit <= 0 || len(invocations) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, limit)
	for idx := len(invocations) - 1; idx >= 0 && len(out) < limit; idx-- {
		invocation := invocations[idx]
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "tool_call") {
			continue
		}
		item := structuredTurnStateToolResultFromInvocation(invocation)
		if len(item) == 0 {
			continue
		}
		out = append([]map[string]interface{}{item}, out...)
	}
	return out
}

func structuredTurnStateToolResultFromInvocation(invocation map[string]interface{}) map[string]interface{} {
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
	if skillID == "" && toolName == "" {
		return nil
	}
	item := map[string]interface{}{
		"skill_id":  skillID,
		"tool_name": toolName,
		"status":    strings.TrimSpace(firstNonEmptyString(invocation["status"], invocation["result_status"])),
	}
	if target := structuredTurnStateTargetFromInvocation(invocation); len(target) > 0 {
		item["target"] = target
	}
	if result := structuredTurnStateResultFacts(invocation); len(result) > 0 {
		item["result_facts"] = result
	}
	if errText := structuredTurnStateInvocationError(invocation); errText != "" {
		item["error"] = errText
	}
	if runtimeID := strings.TrimSpace(stringFromAny(invocation["runtime_id"])); runtimeID != "" {
		item["runtime_id"] = runtimeID
	}
	return item
}

func structuredTurnStateAssetsFromInvocations(invocations []map[string]interface{}, limit int) []map[string]interface{} {
	if limit <= 0 || len(invocations) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, limit)
	seen := map[string]struct{}{}
	for idx := len(invocations) - 1; idx >= 0 && len(out) < limit; idx-- {
		invocation := invocations[idx]
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "tool_call") ||
			!toolGovernanceContinuationInvocationSucceeded(invocation) {
			continue
		}
		skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
		toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
		target := structuredTurnStateTargetFromInvocation(invocation)
		if len(target) == 0 {
			continue
		}
		item := map[string]interface{}{
			"skill_id":  skillID,
			"tool_name": toolName,
			"status":    "completed",
			"target":    target,
		}
		key := compactJSON(item)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append([]map[string]interface{}{item}, out...)
	}
	return out
}

func structuredTurnStateOpenItemsFromInvocations(invocations []map[string]interface{}, limit int) []map[string]interface{} {
	failed := currentTurnFailedOperations(invocations, limit)
	if len(failed) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(failed))
	for _, item := range failed {
		compact := copyStringAnyMap(item)
		compact["reason"] = "failed_tool_call_needs_model_decision"
		out = append(out, compact)
	}
	return out
}

func structuredTurnStateNavigationsFromMetadata(metadata map[string]interface{}, limit int) []map[string]interface{} {
	if limit <= 0 {
		return nil
	}
	actions := completedClientActionRecordsFromMetadata(metadata)
	if len(actions) == 0 {
		return nil
	}
	if len(actions) > limit {
		actions = actions[len(actions)-limit:]
	}
	out := make([]map[string]interface{}, 0, len(actions))
	for _, action := range actions {
		if strings.TrimSpace(stringFromAny(action["action_type"])) == "" {
			continue
		}
		out = append(out, action)
	}
	return out
}

func structuredTurnStateArtifactsFromMetadata(metadata map[string]interface{}, limit int) []map[string]interface{} {
	if limit <= 0 {
		return nil
	}
	artifacts := conversationArtifactsFromMetadata(metadataValue(metadata, "conversation_artifacts"))
	if len(artifacts) == 0 {
		artifacts = generatedFilesFromMetadata(metadataValue(metadata, "generated_files"))
	}
	if len(artifacts) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, min(len(artifacts), limit))
	seen := map[string]struct{}{}
	for idx := len(artifacts) - 1; idx >= 0 && len(out) < limit; idx-- {
		compact := compactGeneratedArtifactForPrompt(artifacts[idx])
		if len(compact) == 0 {
			continue
		}
		key := strings.TrimSpace(firstNonEmptyString(compact["artifact_id"], compact["tool_file_id"], compact["file_id"], compact["filename"]))
		if key != "" {
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
		}
		out = append([]map[string]interface{}{compact}, out...)
	}
	return out
}

func structuredTurnStateTargetFromInvocation(invocation map[string]interface{}) map[string]interface{} {
	if target := currentTurnAgentTargetFromInvocation(invocation); len(target) > 0 {
		target["asset_type"] = "agent"
		return target
	}
	result := mapFromOperationContext(invocation["result"])
	resultSummary := mapFromOperationContext(invocation["result_summary"])
	args := mapFromOperationContext(invocation["arguments"])
	target := map[string]interface{}{}
	if id := strings.TrimSpace(firstNonEmptyString(result["file_id"], result["id"], resultSummary["file_id"], resultSummary["id"], args["file_id"], args["id"])); id != "" {
		target["file_id"] = id
	}
	if name := strings.TrimSpace(firstNonEmptyString(result["file_name"], result["filename"], result["name"], resultSummary["file_name"], resultSummary["filename"], resultSummary["name"], args["file_name"], args["filename"], args["name"])); name != "" {
		target["name"] = truncateRunes(name, 180)
	}
	if assetType := strings.TrimSpace(firstNonEmptyString(result["asset_type"], resultSummary["asset_type"], args["asset_type"])); assetType != "" {
		target["asset_type"] = truncateRunes(assetType, 80)
	}
	if len(target) == 0 {
		return nil
	}
	if strings.TrimSpace(stringFromAny(target["asset_type"])) == "" {
		target["asset_type"] = structuredTurnStateAssetTypeFromInvocation(invocation)
	}
	return target
}

func structuredTurnStateAssetTypeFromInvocation(invocation map[string]interface{}) string {
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	switch skillID {
	case skills.SkillAgentManagement:
		return "agent"
	case skills.SkillFileReader, skills.SkillFileGenerator, skills.SkillFileManager:
		return "file"
	default:
		return "asset"
	}
}

func structuredTurnStateResultFacts(invocation map[string]interface{}) map[string]interface{} {
	resultSummary := mapFromOperationContext(invocation["result_summary"])
	result := mapFromOperationContext(invocation["result"])
	out := map[string]interface{}{}
	for _, key := range []string{"status", "operation", "success_count", "failure_count", "count", "content_status", "content_value_preview", "content_chars", "content_truncated", "filename", "file_name", "name", "model", "provider"} {
		if safe, ok := modelInvocationSafeSummaryValue(resultSummary[key]); ok {
			out[key] = safe
			continue
		}
		if safe, ok := modelInvocationSafeSummaryValue(result[key]); ok {
			out[key] = safe
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func structuredTurnStateInvocationError(invocation map[string]interface{}) string {
	return truncateRunes(strings.TrimSpace(firstNonEmptyString(
		invocation["error"],
		mapFromOperationContext(invocation["result"])["error"],
		mapFromOperationContext(invocation["result_summary"])["error"],
	)), 240)
}

func numericOrStringValue(value interface{}) interface{} {
	if value == nil {
		return nil
	}
	if safe, ok := modelInvocationSafeSummaryValue(value); ok {
		return safe
	}
	return nil
}

func turnStateItemsFromTrace(trace skills.SkillTrace) []map[string]interface{} {
	switch strings.TrimSpace(trace.Kind) {
	case "turn_state":
		result := mapFromOperationContext(trace.Result)
		return mapSliceFromAny(result["items"])
	case "intermediate_answer":
		content := strings.TrimSpace(trace.Message)
		if content == "" {
			return nil
		}
		item := map[string]interface{}{
			"kind":       "user_deliverable",
			"visibility": "user_visible",
			"content":    content,
		}
		if title := strings.TrimSpace(trace.Title); title != "" {
			item["title"] = title
		}
		return []map[string]interface{}{item}
	default:
		return nil
	}
}

func sanitizeTurnStateItemForMetadata(item map[string]interface{}) map[string]interface{} {
	kind := normalizeTurnStateMetadataKind(stringFromAny(item["kind"]))
	if kind == "" {
		return nil
	}
	visibility := normalizeTurnStateMetadataVisibility(stringFromAny(item["visibility"]), kind)
	value := strings.TrimSpace(stringFromAny(item["value"]))
	content := strings.TrimSpace(stringFromAny(item["content"]))
	if kind == "user_deliverable" && content == "" {
		content = value
	}
	if kind != "user_deliverable" && value == "" {
		value = content
	}
	if value == "" && content == "" {
		return nil
	}
	out := map[string]interface{}{
		"kind":       kind,
		"visibility": visibility,
	}
	if key := normalizeTurnStateMetadataKey(stringFromAny(item["key"])); key != "" {
		out["key"] = key
	}
	if value != "" {
		out["value"] = truncateRunes(value, 4000)
	}
	if content != "" {
		out["content"] = truncateRunes(content, 16000)
	}
	if title := strings.TrimSpace(stringFromAny(item["title"])); title != "" {
		out["title"] = truncateRunes(title, 120)
	}
	if source := strings.TrimSpace(stringFromAny(item["source"])); source != "" {
		out["source"] = truncateRunes(source, 200)
	}
	if usedFor := sanitizeTurnStateMetadataStringSlice(item["used_for"], 8, 120); len(usedFor) > 0 {
		out["used_for"] = usedFor
	}
	if confidence, ok := floatValue(item["confidence"]); ok {
		if confidence < 0 {
			confidence = 0
		}
		if confidence > 1 {
			confidence = 1
		}
		out["confidence"] = confidence
	}
	if createdAt := strings.TrimSpace(stringFromAny(item["created_at"])); createdAt != "" {
		out["created_at"] = createdAt
	}
	return out
}

func upsertTurnStateItem(current []map[string]interface{}, incoming map[string]interface{}) []map[string]interface{} {
	if len(incoming) == 0 {
		return current
	}
	identity := turnStateItemIdentity(incoming)
	if identity != "" {
		for index, item := range current {
			if turnStateItemIdentity(item) == identity {
				current[index] = incoming
				return current
			}
		}
	}
	return append(current, incoming)
}

func turnStateItemIdentity(item map[string]interface{}) string {
	kind := strings.TrimSpace(stringFromAny(item["kind"]))
	key := strings.TrimSpace(stringFromAny(item["key"]))
	if kind != "" && key != "" {
		return kind + ":" + key
	}
	return ""
}

func normalizeTurnStateMetadataKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "working_fact", "fact", "working-fact":
		return "working_fact"
	case "user_deliverable", "deliverable", "answer", "intermediate_answer":
		return "user_deliverable"
	case "decision":
		return "decision"
	case "assumption":
		return "assumption"
	case "verification", "verify", "verification_result":
		return "verification"
	default:
		return ""
	}
}

func normalizeTurnStateMetadataVisibility(value string, kind string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "user_visible", "visible", "user":
		return "user_visible"
	case "audit":
		return "audit"
	case "model_only", "internal", "":
		if kind == "user_deliverable" {
			return "user_visible"
		}
		return "model_only"
	default:
		if kind == "user_deliverable" {
			return "user_visible"
		}
		return "model_only"
	}
}

func normalizeTurnStateMetadataKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	return truncateRunes(value, 120)
}

func sanitizeTurnStateMetadataStringSlice(value interface{}, maxItems int, maxRunes int) []interface{} {
	var raw []interface{}
	switch typed := value.(type) {
	case []interface{}:
		raw = typed
	case []string:
		raw = make([]interface{}, 0, len(typed))
		for _, item := range typed {
			raw = append(raw, item)
		}
	default:
		return nil
	}
	out := make([]interface{}, 0, len(raw))
	for _, item := range raw {
		text := strings.TrimSpace(stringFromAny(item))
		if text == "" {
			continue
		}
		out = append(out, truncateRunes(text, maxRunes))
		if len(out) >= maxItems {
			break
		}
	}
	return out
}

func mergeSkillInvocationMetadata(source map[string]interface{}, invocations []map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if len(invocations) == 0 {
		return metadata
	}
	stored := sanitizeSkillInvocationsForMetadata(skillInvocationsFromMetadata(metadata["skill_invocations"]))
	for _, invocation := range invocations {
		invocation = sanitizeSkillInvocationForMetadata(invocation)
		if !visibleSkillInvocationKind(stringFromAny(invocation["kind"])) {
			continue
		}
		if internalPlannerFeedbackInvocation(invocation) {
			continue
		}
		stored = upsertSkillInvocation(stored, invocation)
	}
	applySkillInvocationSummary(metadata, stored)
	applyOperationPlanInvocationState(metadata, stored)
	applyStructuredTurnStateMetadata(metadata, stored)
	return metadata
}

func mergeModelInvocationMetadata(source map[string]interface{}, invocation map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if len(invocation) == 0 {
		return metadata
	}
	stored := modelInvocationsFromMetadata(metadata["model_invocations"])
	runtimeID := strings.TrimSpace(stringFromAny(invocation["runtime_id"]))
	replaced := false
	if runtimeID != "" {
		for index, item := range stored {
			if strings.TrimSpace(stringFromAny(item["runtime_id"])) == runtimeID {
				stored[index] = mergeInvocation(item, invocation)
				replaced = true
				break
			}
		}
	}
	if !replaced {
		stored = append(stored, compactSkillInvocation(invocation))
	}
	if len(stored) > maxModelInvocationMetadataRecords {
		stored = stored[len(stored)-maxModelInvocationMetadataRecords:]
	}
	metadata["model_invocations"] = skillInvocationsToInterfaceSlice(stored)
	metadata["model_invocation_count"] = len(stored)
	return metadata
}

func modelInvocationFromTrace(trace skillloop.ModelInvocationTrace, userSystemPrompt string, redactRequest bool) map[string]interface{} {
	phase := strings.TrimSpace(trace.Phase)
	if phase == "" {
		phase = "model_call"
	}
	startedAt := trace.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	status := "success"
	if strings.TrimSpace(trace.Error) != "" {
		status = "error"
	}
	invocation := map[string]interface{}{
		"kind":        "model_call",
		"phase":       phase,
		"round":       trace.Round,
		"streaming":   trace.Streaming,
		"status":      status,
		"title":       modelInvocationTitle(phase, trace.Round),
		"created_at":  startedAt.Unix(),
		"duration_ms": trace.DurationMS,
		"runtime_id":  fmt.Sprintf("model_call:%s:%d:%d", phase, trace.Round, startedAt.UnixNano()),
		"usage":       usageMetadata(trace.Usage),
		"error":       strings.TrimSpace(trace.Error),
	}
	if modelInvocationDebugPayloadEnabled() {
		invocation["request"] = modelInvocationRequestPayload(trace.Request, redactRequest)
		invocation["response"] = modelInvocationResponsePayload(trace.Response, trace.Usage)
	} else {
		if summary := modelInvocationRequestSummaryPayload(trace.Request); len(summary) > 0 {
			invocation["request"] = summary
		}
		if summary := modelInvocationResponseSummaryPayload(trace.Response, trace.Usage); len(summary) > 0 {
			invocation["response"] = summary
		}
		invocation["payload_mode"] = "compact"
	}
	if trace.Request != nil {
		invocation["model"] = trace.Request.Model
		invocation["provider"] = trace.Request.Provider
	}
	if trace.Usage != nil {
		invocation["prompt_tokens"] = trace.Usage.PromptTokens
		invocation["completion_tokens"] = trace.Usage.CompletionTokens
		invocation["total_tokens"] = trace.Usage.TotalTokens
	}
	if strings.TrimSpace(userSystemPrompt) != "" {
		invocation["user_system_prompt"] = strings.TrimSpace(userSystemPrompt)
	}
	return compactSkillInvocation(invocation)
}

func modelInvocationDebugPayloadEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ZGI_AICHAT_MODEL_INVOCATION_DEBUG"))) {
	case "1", "true", "yes", "on", "full":
		return true
	default:
		return false
	}
}

func shouldRedactModelInvocationRequest(prepared *PreparedChat) bool {
	if prepared == nil {
		return true
	}
	return normalizeCallerType(prepared.Caller.Type) != runtimemodel.ConversationCallerAgent
}

func runtimeUserSystemPrompt(prepared *PreparedChat) string {
	if prepared == nil || prepared.parts == nil {
		return ""
	}
	return strings.TrimSpace(prepared.parts.SystemPrompt)
}

func modelInvocationTitle(phase string, round int) string {
	switch phase {
	case "final_answer":
		return "Model call: final answer"
	case "skill_planning":
		if round >= 0 {
			return fmt.Sprintf("Model call: skill planning #%d", round+1)
		}
		return "Model call: skill planning"
	default:
		return "Model call"
	}
}

func modelInvocationRequestPayload(req *adapter.ChatRequest, redactToolContent bool) map[string]interface{} {
	if req == nil {
		return map[string]interface{}{}
	}
	payload := jsonObjectPayload(req)
	if strings.TrimSpace(req.Provider) != "" {
		payload["provider"] = req.Provider
	}
	if len(req.AdditionalParameters) > 0 {
		payload["additional_parameters"] = copyStringAnyMap(req.AdditionalParameters)
	}
	return sanitizeModelInvocationRequestPayload(payload, redactToolContent)
}

func modelInvocationRequestSummaryPayload(req *adapter.ChatRequest) map[string]interface{} {
	if req == nil {
		return nil
	}
	payload := map[string]interface{}{}
	if strings.TrimSpace(req.Provider) != "" {
		payload["provider"] = req.Provider
	}
	if strings.TrimSpace(req.Model) != "" {
		payload["model"] = req.Model
	}
	payload["message_count"] = len(req.Messages)
	payload["tool_count"] = len(req.Tools)
	payload["stream"] = req.Stream
	if req.ToolChoice != nil {
		payload["tool_choice"] = fmt.Sprintf("%v", req.ToolChoice)
	}
	if req.ResponseFormat != nil {
		payload["response_format"] = jsonObjectPayload(req.ResponseFormat)
	}
	if req.MaxTokens != nil {
		payload["max_tokens"] = *req.MaxTokens
	}
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	roles := make([]string, 0, len(req.Messages))
	for _, message := range req.Messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "unknown"
		}
		roles = append(roles, role)
	}
	if len(roles) > 0 {
		payload["message_roles"] = roles
	}
	return operationPlanEvidenceSafeMapForModelInvocation(payload)
}

func sanitizeModelInvocationRequestPayload(payload map[string]interface{}, redactToolContent bool) map[string]interface{} {
	if len(payload) == 0 {
		return payload
	}
	messages, ok := payload["messages"].([]interface{})
	if !ok {
		sanitizeModelInvocationInlineImageDataURLs(payload)
		return payload
	}
	for _, raw := range messages {
		message, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(stringFromAny(message["role"])))
		switch role {
		case "tool", "function":
			if !redactToolContent {
				continue
			}
			content, exists := message["content"]
			if !exists {
				continue
			}
			message["content"] = modelInvocationToolContentSummary(content)
			message["content_redacted"] = true
		case "user":
			if !redactToolContent {
				continue
			}
			content, exists := message["content"]
			if !exists {
				continue
			}
			if summary, ok := modelInvocationUserContentSummary(content); ok {
				message["content"] = summary
				message["content_redacted"] = true
			}
		}
	}
	sanitizeModelInvocationInlineImageDataURLs(payload)
	return payload
}

func sanitizeModelInvocationInlineImageDataURLs(value interface{}) bool {
	switch typed := value.(type) {
	case map[string]interface{}:
		return sanitizeModelInvocationInlineImageDataURLMap(typed)
	case []interface{}:
		changed := false
		for index, item := range typed {
			switch itemValue := item.(type) {
			case string:
				if redacted, _, ok := redactEmbeddedModelInvocationImageDataURLs(itemValue); ok {
					typed[index] = redacted
					changed = true
				}
			default:
				if sanitizeModelInvocationInlineImageDataURLs(itemValue) {
					changed = true
				}
			}
		}
		return changed
	default:
		return false
	}
}

func sanitizeModelInvocationInlineImageDataURLMap(object map[string]interface{}) bool {
	if len(object) == 0 {
		return false
	}
	changed := false
	for key, value := range object {
		switch typed := value.(type) {
		case string:
			redacted, summaries, ok := redactEmbeddedModelInvocationImageDataURLs(typed)
			if !ok {
				continue
			}
			object[key] = redacted
			applyInlineImageDataURLFieldSummary(object, key, typed, summaries)
			changed = true
		default:
			if sanitizeModelInvocationInlineImageDataURLs(typed) {
				changed = true
			}
		}
	}
	return changed
}

func applyInlineImageDataURLFieldSummary(object map[string]interface{}, key string, original string, summaries []map[string]interface{}) {
	if len(object) == 0 || strings.TrimSpace(key) == "" || len(summaries) == 0 {
		return
	}
	prefix := strings.TrimSpace(key)
	object[prefix+"_redacted"] = true
	object[prefix+"_redaction_reason"] = modelInvocationInlineImageDataRedactionReason
	object[prefix+"_chars"] = len([]rune(original))
	object[prefix+"_inline_image_data_count"] = len(summaries)
	if len(summaries) != 1 {
		object[prefix+"_inline_image_data"] = summaries
		return
	}
	for _, summaryKey := range []string{"mime_type", "data_url_chars", "base64_chars", "estimated_bytes"} {
		if value, ok := summaries[0][summaryKey]; ok {
			object[prefix+"_"+summaryKey] = value
		}
	}
}

func redactEmbeddedModelInvocationImageDataURLs(text string) (string, []map[string]interface{}, bool) {
	lower := strings.ToLower(text)
	if !strings.Contains(lower, "data:image/") {
		return text, nil, false
	}
	var builder strings.Builder
	summaries := make([]map[string]interface{}, 0, 1)
	cursor := 0
	searchFrom := 0
	for {
		relativeStart := strings.Index(lower[searchFrom:], "data:image/")
		if relativeStart < 0 {
			break
		}
		start := searchFrom + relativeStart
		markerRelative := strings.Index(lower[start:], ";base64,")
		if markerRelative < 0 {
			searchFrom = start + len("data:image/")
			continue
		}
		base64Start := start + markerRelative + len(";base64,")
		end := base64Start
		for end < len(text) && isModelInvocationBase64DataURLByte(text[end]) {
			end++
		}
		if end == base64Start {
			searchFrom = base64Start
			continue
		}
		raw := text[start:end]
		summary, ok := modelInvocationInlineImageDataURLSummary(raw)
		if !ok {
			searchFrom = end
			continue
		}
		builder.WriteString(text[cursor:start])
		builder.WriteString(stringFromAny(summary["redacted_url"]))
		summaries = append(summaries, summary)
		cursor = end
		searchFrom = end
	}
	if len(summaries) == 0 {
		return text, nil, false
	}
	builder.WriteString(text[cursor:])
	return builder.String(), summaries, true
}

func modelInvocationInlineImageDataURLSummary(raw string) (map[string]interface{}, bool) {
	raw = strings.TrimSpace(raw)
	lower := strings.ToLower(raw)
	if !strings.HasPrefix(lower, "data:image/") {
		return nil, false
	}
	markerIndex := strings.Index(lower, ";base64,")
	if markerIndex <= len("data:") {
		return nil, false
	}
	base64Start := markerIndex + len(";base64,")
	if base64Start >= len(raw) {
		return nil, false
	}
	mimeType := strings.ToLower(strings.TrimSpace(raw[len("data:"):markerIndex]))
	base64Data := strings.TrimSpace(raw[base64Start:])
	if base64Data == "" {
		return nil, false
	}
	return map[string]interface{}{
		"redacted":        true,
		"reason":          modelInvocationInlineImageDataRedactionReason,
		"mime_type":       mimeType,
		"data_url_chars":  len([]rune(raw)),
		"base64_chars":    len(base64Data),
		"estimated_bytes": estimatedBase64DecodedBytes(base64Data),
		"redacted_url":    raw[:base64Start] + modelInvocationRedactedDataURLToken,
	}, true
}

func estimatedBase64DecodedBytes(encoded string) int {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return 0
	}
	padding := 0
	if strings.HasSuffix(encoded, "==") {
		padding = 2
	} else if strings.HasSuffix(encoded, "=") {
		padding = 1
	}
	decoded := len(encoded)*3/4 - padding
	if decoded < 0 {
		return 0
	}
	return decoded
}

func isModelInvocationBase64DataURLByte(value byte) bool {
	return (value >= 'A' && value <= 'Z') ||
		(value >= 'a' && value <= 'z') ||
		(value >= '0' && value <= '9') ||
		value == '+' ||
		value == '/' ||
		value == '=' ||
		value == '-' ||
		value == '_'
}

func modelInvocationUserContentSummary(content interface{}) (map[string]interface{}, bool) {
	summary := map[string]interface{}{
		"redacted": true,
		"reason":   "user_payload_file_content_omitted_from_model_invocation_metadata",
	}
	if content == nil {
		return nil, false
	}
	switch typed := content.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil, false
		}
		var parsed interface{}
		if err := json.Unmarshal([]byte(text), &parsed); err != nil {
			return nil, false
		}
		jsonSummary, ok := modelInvocationUserJSONSummary(parsed)
		if !ok {
			return nil, false
		}
		summary["original_type"] = "string"
		summary["content_chars"] = len([]rune(typed))
		summary["json"] = jsonSummary
	case map[string]interface{}:
		jsonSummary, ok := modelInvocationUserJSONSummary(typed)
		if !ok {
			return nil, false
		}
		summary["original_type"] = "object"
		summary["json"] = jsonSummary
	default:
		return nil, false
	}
	return summary, true
}

func modelInvocationUserJSONSummary(value interface{}) (map[string]interface{}, bool) {
	object, ok := value.(map[string]interface{})
	if !ok {
		return nil, false
	}
	files := modelInvocationFileItemsFromAny(object["files"])
	if len(files) == 0 {
		return nil, false
	}
	fileSummaries, redacted := modelInvocationPayloadFilesSummary(files)
	if !redacted {
		return nil, false
	}
	fields := map[string]interface{}{
		"files": fileSummaries,
	}
	if userQuery := strings.TrimSpace(stringFromAny(object["user_query"])); userQuery != "" {
		fields["user_query"] = userQuery
	}
	if postprocess, ok := modelInvocationPayloadSafeValue(object["postprocess"]); ok {
		fields["postprocess"] = postprocess
	}
	if fallback := strings.TrimSpace(stringFromAny(object["fallback_answer"])); fallback != "" {
		fields["fallback_answer_chars"] = len([]rune(fallback))
		fields["fallback_answer_redacted"] = true
	}
	return map[string]interface{}{
		"type":      "object",
		"json_keys": sortedStringKeys(object),
		"fields":    fields,
	}, true
}

func modelInvocationPayloadSafeValue(value interface{}) (interface{}, bool) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil, false
		}
		return text, true
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed, true
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			if safe, ok := modelInvocationPayloadSafeValue(item); ok {
				out = append(out, safe)
			}
		}
		return out, len(out) > 0
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for _, key := range sortedStringKeys(typed) {
			if safe, ok := modelInvocationPayloadSafeValue(typed[key]); ok {
				out[key] = safe
			}
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

func modelInvocationToolContentSummary(content interface{}) map[string]interface{} {
	summary := map[string]interface{}{
		"redacted": true,
		"reason":   "tool_result_content_omitted_from_model_invocation_metadata",
	}
	if content == nil {
		return summary
	}
	switch typed := content.(type) {
	case string:
		summary["original_type"] = "string"
		summary["content_chars"] = len([]rune(typed))
		if strings.TrimSpace(typed) != "" {
			var parsed interface{}
			if err := json.Unmarshal([]byte(typed), &parsed); err == nil {
				summary["json"] = modelInvocationToolJSONSummary(parsed)
			}
		}
	case map[string]interface{}:
		summary["original_type"] = "object"
		summary["json"] = modelInvocationToolJSONSummary(typed)
	case []interface{}:
		summary["original_type"] = "array"
		summary["item_count"] = len(typed)
		summary["json"] = modelInvocationToolJSONSummary(typed)
	default:
		summary["original_type"] = fmt.Sprintf("%T", content)
	}
	return summary
}

func modelInvocationToolJSONSummary(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return modelInvocationToolObjectSummary(typed)
	case []interface{}:
		return map[string]interface{}{
			"type":       "array",
			"item_count": len(typed),
		}
	default:
		return map[string]interface{}{"type": fmt.Sprintf("%T", value)}
	}
}

func modelInvocationToolObjectSummary(value map[string]interface{}) map[string]interface{} {
	summary := map[string]interface{}{
		"type":      "object",
		"json_keys": sortedStringKeys(value),
	}
	fields := map[string]interface{}{}
	for _, key := range []string{
		"status",
		"content_status",
		"content_chars",
		"content_truncated",
		"from_cache",
		"file_id",
		"file_ids",
		"name",
		"extension",
		"mime_type",
		"workspace_id",
		"count",
		"selected_count",
	} {
		if sanitized, ok := modelInvocationSafeSummaryValue(value[key]); ok {
			fields[key] = sanitized
		}
	}
	if _, exists := value["content"]; exists {
		fields["content_redacted"] = true
	}
	if errorText := strings.TrimSpace(stringFromAny(value["content_error"])); errorText != "" {
		fields["content_error_chars"] = len([]rune(errorText))
	}
	if files := modelInvocationFileItemsFromAny(value["files"]); len(files) > 0 {
		fields["files"] = modelInvocationToolFilesSummary(files)
	}
	if len(fields) > 0 {
		summary["fields"] = fields
	}
	return summary
}

func modelInvocationSafeSummaryValue(value interface{}) (interface{}, bool) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil, false
		}
		return text, true
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed, true
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			if safe, ok := modelInvocationSafeSummaryValue(item); ok {
				out = append(out, safe)
			}
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

func modelInvocationToolFilesSummary(files []interface{}) []map[string]interface{} {
	summaries, _ := modelInvocationPayloadFilesSummary(files)
	return summaries
}

func modelInvocationFileItemsFromAny(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		return typed
	case []map[string]interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func modelInvocationPayloadFilesSummary(files []interface{}) ([]map[string]interface{}, bool) {
	out := make([]map[string]interface{}, 0, min(len(files), 20))
	redacted := false
	for idx, raw := range files {
		if idx >= 20 {
			break
		}
		file, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		item := map[string]interface{}{}
		for _, key := range []string{"visible_index", "id", "file_id", "name", "extension", "mime_type", "file_type", "workspace_id", "size", "content_status", "content_chars", "filtered_reason", "content_truncated", "from_cache", "selected"} {
			if safe, ok := modelInvocationSafeSummaryValue(file[key]); ok {
				item[key] = safe
			}
		}
		if preview := strings.TrimSpace(stringFromAny(file["content_preview"])); preview != "" {
			item["content_preview_chars"] = len([]rune(preview))
			item["content_preview_redacted"] = true
			redacted = true
		}
		if content := strings.TrimSpace(stringFromAny(file["content"])); content != "" {
			setRedactedTextSummary(item, "content", content)
			redacted = true
		}
		if contentError := strings.TrimSpace(stringFromAny(file["content_error"])); contentError != "" {
			item["content_error_chars"] = len([]rune(contentError))
		}
		if len(item) > 0 {
			out = append(out, item)
		}
	}
	return out, redacted
}

func sortedStringKeys(value map[string]interface{}) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func modelInvocationResponsePayload(message *adapter.Message, usage *adapter.Usage) map[string]interface{} {
	payload := map[string]interface{}{}
	if message != nil {
		payload["message"] = jsonObjectPayload(message)
	}
	if usageMap := usageMetadata(usage); len(usageMap) > 0 {
		payload["usage"] = usageMap
	}
	sanitizeModelInvocationInlineImageDataURLs(payload)
	return payload
}

func modelInvocationResponseSummaryPayload(message *adapter.Message, usage *adapter.Usage) map[string]interface{} {
	payload := map[string]interface{}{}
	if message != nil {
		if strings.TrimSpace(message.Role) != "" {
			payload["role"] = message.Role
		}
		if content := strings.TrimSpace(messageContentText(message.Content)); content != "" {
			payload["content_chars"] = len([]rune(content))
			payload["content_preview"] = truncateRunes(content, 240)
		}
		if reasoning := strings.TrimSpace(message.ReasoningContent); reasoning != "" {
			payload["reasoning_chars"] = len([]rune(reasoning))
		}
		if len(message.ToolCalls) > 0 {
			payload["tool_call_count"] = len(message.ToolCalls)
			names := make([]string, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				if name := strings.TrimSpace(call.Function.Name); name != "" {
					names = append(names, truncateRunes(name, 120))
				}
			}
			if len(names) > 0 {
				payload["tool_call_names"] = names
			}
		}
	}
	if usageMap := usageMetadata(usage); len(usageMap) > 0 {
		payload["usage"] = usageMap
	}
	return operationPlanEvidenceSafeMapForModelInvocation(payload)
}

func operationPlanEvidenceSafeMapForModelInvocation(source map[string]interface{}) map[string]interface{} {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(source))
	for key, value := range source {
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok {
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			out[key] = truncateRunes(text, 500)
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func jsonObjectPayload(value interface{}) map[string]interface{} {
	if value == nil {
		return map[string]interface{}{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return map[string]interface{}{"value": fmt.Sprintf("%v", value)}
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err == nil && payload != nil {
		return payload
	}
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return map[string]interface{}{"value": string(data)}
	}
	return map[string]interface{}{"value": raw}
}

func applySkillInvocationSummary(metadata map[string]interface{}, invocations []map[string]interface{}) {
	invocations = sortSkillInvocationsForMetadata(invocations)
	selected := make([]interface{}, 0)
	loaded := make([]interface{}, 0)
	toolsUsed := make([]interface{}, 0)
	selectedSeen := map[string]struct{}{}
	loadedSeen := map[string]struct{}{}
	toolSeen := map[string]struct{}{}
	toolCallCount := 0
	guardrailCount := 0
	addConfiguredSkillIDs(metadata, selectedSeen, &selected)

	for _, invocation := range invocations {
		skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
		kind := strings.TrimSpace(stringFromAny(invocation["kind"]))
		status := strings.TrimSpace(stringFromAny(invocation["status"]))
		toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
		if !visibleSkillInvocationKind(kind) {
			continue
		}
		if skillID != "" {
			if _, exists := selectedSeen[skillID]; !exists {
				selectedSeen[skillID] = struct{}{}
				selected = append(selected, skillID)
			}
		}
		if kind == "skill_load" && status == "success" {
			if _, exists := loadedSeen[skillID]; skillID != "" && !exists {
				loadedSeen[skillID] = struct{}{}
				loaded = append(loaded, skillID)
			}
		}
		if kind == "tool_call" {
			toolCallCount++
			if _, exists := toolSeen[toolName]; toolName != "" && !exists {
				toolSeen[toolName] = struct{}{}
				toolsUsed = append(toolsUsed, toolName)
			}
		}
		if kind == "guardrail" {
			guardrailCount++
		}
	}
	metadata["has_trace"] = true
	metadata["selected_skill_ids"] = selected
	metadata["loaded_skill_ids"] = loaded
	actionTraceCount := countSkillActionInvocations(invocations)
	metadata["skill_step_count"] = actionTraceCount
	metadata["skill_call_count"] = actionTraceCount
	metadata["tool_call_count"] = toolCallCount
	metadata["guardrail_count"] = guardrailCount
	metadata["skill_names"] = selected
	metadata["tool_names"] = toolsUsed
	metadata["skill_invocations"] = skillInvocationsToInterfaceSlice(invocations)
}

func sortSkillInvocationsForMetadata(invocations []map[string]interface{}) []map[string]interface{} {
	if len(invocations) <= 1 {
		return invocations
	}
	sort.SliceStable(invocations, func(i, j int) bool {
		leftAt, leftOK := skillInvocationTimelineMilliseconds(invocations[i])
		rightAt, rightOK := skillInvocationTimelineMilliseconds(invocations[j])
		if leftOK != rightOK {
			return leftOK
		}
		if !leftOK {
			return false
		}
		return leftAt < rightAt
	})
	return invocations
}

func skillInvocationTimelineMilliseconds(invocation map[string]interface{}) (int64, bool) {
	if at, ok := unixMillisecondsFromAny(invocation["created_at_ms"]); ok {
		return at, true
	}
	at, ok := skillInvocationTimelineUnix(invocation)
	if !ok {
		return 0, false
	}
	return at * 1000, true
}

func skillInvocationTimelineUnix(invocation map[string]interface{}) (int64, bool) {
	for _, key := range []string{"created_at", "resolved_at", "updated_at"} {
		if at, ok := unixSecondsFromAny(invocation[key]); ok {
			return at, true
		}
	}
	return 0, false
}

func unixSecondsFromAny(value interface{}) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		if typed > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(typed), true
	case float32:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return parsed, true
		}
		return 0, false
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return 0, false
		}
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err == nil {
			return parsed, true
		}
		if timestamp, err := time.Parse(time.RFC3339, text); err == nil {
			return timestamp.Unix(), true
		}
		return 0, false
	default:
		return 0, false
	}
}

func unixMillisecondsFromAny(value interface{}) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		if typed > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(typed), true
	case float32:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return parsed, true
		}
		return 0, false
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return 0, false
		}
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err == nil {
			return parsed, true
		}
		if timestamp, err := time.Parse(time.RFC3339Nano, text); err == nil {
			return timestamp.UnixMilli(), true
		}
		return 0, false
	default:
		return 0, false
	}
}

func skillInvocationsFromMetadata(value interface{}) []map[string]interface{} {
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
			if invocation, ok := item.(map[string]interface{}); ok {
				out = append(out, copyStringAnyMap(invocation))
			}
		}
		return out
	default:
		return []map[string]interface{}{}
	}
}

func modelInvocationsFromMetadata(value interface{}) []map[string]interface{} {
	return skillInvocationsFromMetadata(value)
}

func skillInvocationsToInterfaceSlice(invocations []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(invocations))
	for _, invocation := range invocations {
		out = append(out, invocation)
	}
	return out
}

func skillInvocationFromTrace(trace skills.SkillTrace, index int) map[string]interface{} {
	invocation := map[string]interface{}{
		"kind":        trace.Kind,
		"skill_id":    trace.SkillID,
		"tool_name":   trace.ToolName,
		"title":       trace.Title,
		"status":      trace.Status,
		"duration_ms": trace.DurationMS,
		"arguments":   trace.Arguments,
		"result":      trace.Result,
		"message":     trace.Message,
		"error":       trace.Error,
		"runtime_id":  traceRuntimeID(trace, index),
	}
	if trace.Governance != nil {
		invocation["governance"] = trace.Governance
	}
	if path := firstNonEmptyString(valueFromMap(trace.Arguments, "path"), valueFromMap(trace.Result, "path")); path != "" {
		invocation["path"] = path
	}
	if answerID := firstNonEmptyString(valueFromMap(trace.Arguments, "answer_id"), valueFromMap(trace.Result, "answer_id")); answerID != "" {
		invocation["answer_id"] = answerID
	}
	if createdAt := numericValueFromMap(trace.Arguments, "created_at"); createdAt != nil {
		invocation["created_at"] = createdAt
	} else if createdAt := numericValueFromMap(trace.Result, "created_at"); createdAt != nil {
		invocation["created_at"] = createdAt
	}
	if createdAtMS := numericValueFromMap(trace.Arguments, "created_at_ms"); createdAtMS != nil {
		invocation["created_at_ms"] = createdAtMS
	} else if createdAtMS := numericValueFromMap(trace.Result, "created_at_ms"); createdAtMS != nil {
		invocation["created_at_ms"] = createdAtMS
	}
	normalizeSkillInvocationTimelineFields(invocation)
	return sanitizeSkillInvocationForMetadata(compactSkillInvocation(invocation))
}

func sanitizeSkillInvocationsForMetadata(invocations []map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(invocations))
	for _, invocation := range invocations {
		if internalPlannerFeedbackInvocation(invocation) {
			continue
		}
		out = append(out, sanitizeSkillInvocationForMetadata(invocation))
	}
	return out
}

func sanitizeSkillInvocationForMetadata(invocation map[string]interface{}) map[string]interface{} {
	if len(invocation) == 0 {
		return invocation
	}
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	if skillID != skills.SkillFileReader && skillID != skills.SkillFileManager {
		return invocation
	}
	result, ok := invocation["result"].(map[string]interface{})
	if !ok || len(result) == 0 {
		return invocation
	}
	sanitizedResult, changed := sanitizeFileReaderResultForMetadata(result)
	if !changed {
		return invocation
	}
	out := copyStringAnyMap(invocation)
	out["result"] = sanitizedResult
	return out
}

func sanitizeFileReaderResultForMetadata(result map[string]interface{}) (map[string]interface{}, bool) {
	out := copyStringAnyMap(result)
	changed := false
	if preview := safeFileReaderContentValuePreview(out, stringFromAny(out["content"])); preview != "" {
		out["content_value_preview"] = preview
		out["content_value_source"] = "read_file.content"
		changed = true
	}
	if redactTextFieldForMetadata(out, "content", "content") {
		changed = true
	}
	if redactTextFieldForMetadata(out, "content_preview", "content_preview") {
		changed = true
	}
	if redactTextFieldForMetadata(out, "content_error", "content_error") {
		changed = true
	}
	if file, ok := out["file"].(map[string]interface{}); ok && len(file) > 0 {
		out["file"] = fileReaderMetadataFileSummary(file)
		changed = true
	}
	if files := modelInvocationFileItemsFromAny(out["files"]); len(files) > 0 {
		fileSummaries, redacted := modelInvocationPayloadFilesSummary(files)
		if len(fileSummaries) > 0 {
			out["files"] = fileSummaries
		}
		if redacted {
			out["files_content_redacted"] = true
		}
		changed = changed || redacted
	}
	return out, changed
}

const fileReaderContentValuePreviewMaxRunes = 120

func safeFileReaderContentValuePreview(payload map[string]interface{}, content string) string {
	if payload == nil {
		return ""
	}
	text := strings.TrimSpace(content)
	if text == "" || len([]rune(text)) > fileReaderContentValuePreviewMaxRunes {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(payload["content_status"])), "extracted") {
		return ""
	}
	if operationPlanBoolValue(payload["content_truncated"]) {
		return ""
	}
	mimeType := strings.ToLower(strings.TrimSpace(stringFromAny(payload["file_mime_type"])))
	extension := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(stringFromAny(payload["file_extension"])), "."))
	if mimeType != "" && safeFileReaderContentMimeType(mimeType) {
		return text
	}
	if safeFileReaderContentExtension(extension) {
		return text
	}
	return ""
}

func safeFileReaderContentMimeType(mimeType string) bool {
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

func safeFileReaderContentExtension(extension string) bool {
	switch extension {
	case "txt", "md", "markdown", "csv", "json", "jsonl", "xml", "yaml", "yml", "svg", "html", "htm", "log":
		return true
	default:
		return false
	}
}

func redactTextFieldForMetadata(payload map[string]interface{}, key string, prefix string) bool {
	if payload == nil {
		return false
	}
	value, exists := payload[key]
	if !exists {
		return false
	}
	delete(payload, key)
	setRedactedTextSummary(payload, prefix, strings.TrimSpace(stringFromAny(value)))
	return true
}

func setRedactedTextSummary(payload map[string]interface{}, prefix string, text string) {
	if payload == nil {
		return
	}
	payload[prefix+"_redacted"] = true
	if text == "" {
		return
	}
	charsKey := prefix + "_chars"
	if prefix == "content" {
		if _, exists := payload["content_chars"]; exists {
			charsKey = "content_returned_chars"
		}
	}
	payload[charsKey] = len([]rune(text))
}

func fileReaderMetadataFileSummary(file map[string]interface{}) map[string]interface{} {
	item := map[string]interface{}{}
	for _, key := range []string{"visible_index", "id", "file_id", "name", "extension", "mime_type", "file_type", "workspace_id", "size", "selected", "content_status", "content_chars", "content_truncated", "from_cache"} {
		if safe, ok := modelInvocationSafeSummaryValue(file[key]); ok {
			item[key] = safe
		}
	}
	if preview := strings.TrimSpace(stringFromAny(file["content_preview"])); preview != "" {
		item["content_preview_chars"] = len([]rune(preview))
		item["content_preview_redacted"] = true
	}
	if content := strings.TrimSpace(stringFromAny(file["content"])); content != "" {
		setRedactedTextSummary(item, "content", content)
	}
	if contentError := strings.TrimSpace(stringFromAny(file["content_error"])); contentError != "" {
		item["content_error_chars"] = len([]rune(contentError))
		item["content_error_redacted"] = true
	}
	return item
}

func newSkillInvocation(kind, skillID, toolName, status string, values map[string]interface{}) map[string]interface{} {
	if values == nil {
		values = map[string]interface{}{}
	}
	now := time.Now()
	invocation := map[string]interface{}{
		"kind":          strings.TrimSpace(kind),
		"skill_id":      strings.TrimSpace(skillID),
		"tool_name":     strings.TrimSpace(toolName),
		"status":        strings.TrimSpace(status),
		"created_at":    now.Unix(),
		"created_at_ms": now.UnixMilli(),
	}
	_, hasProvidedCreatedAt := values["created_at"]
	_, hasProvidedCreatedAtMS := values["created_at_ms"]
	for key, value := range values {
		invocation[key] = value
	}
	if hasProvidedCreatedAt && !hasProvidedCreatedAtMS {
		delete(invocation, "created_at_ms")
	}
	normalizeSkillInvocationTimelineFields(invocation)
	if strings.TrimSpace(stringFromAny(invocation["runtime_id"])) == "" {
		invocation["runtime_id"] = invocationRuntimeIdentity(invocation)
	}
	return compactSkillInvocation(invocation)
}

func normalizeSkillInvocationTimelineFields(invocation map[string]interface{}) {
	if len(invocation) == 0 {
		return
	}
	if createdAtMS, ok := unixMillisecondsFromAny(invocation["created_at_ms"]); ok && createdAtMS > 0 {
		invocation["created_at_ms"] = createdAtMS
		if _, ok := unixSecondsFromAny(invocation["created_at"]); !ok {
			invocation["created_at"] = createdAtMS / 1000
		}
		return
	}
	if createdAt, ok := unixSecondsFromAny(invocation["created_at"]); ok && createdAt > 0 {
		invocation["created_at"] = createdAt
		invocation["created_at_ms"] = createdAt * 1000
	}
}

func compactSkillInvocation(invocation map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(invocation))
	for key, value := range invocation {
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		if number, ok := value.(int64); ok && number == 0 && key == "duration_ms" {
			continue
		}
		out[key] = value
	}
	return out
}

func upsertSkillInvocation(current []map[string]interface{}, incoming map[string]interface{}) []map[string]interface{} {
	if len(incoming) == 0 {
		return current
	}
	if runtimeID := strings.TrimSpace(stringFromAny(incoming["runtime_id"])); runtimeID != "" {
		for index, invocation := range current {
			if strings.TrimSpace(stringFromAny(invocation["runtime_id"])) == runtimeID {
				if shouldKeepExistingInvocation(invocation, incoming) {
					return current
				}
				current[index] = mergeInvocation(invocation, incoming)
				return current
			}
		}
	}
	if semanticID := skillInvocationSemanticIdentity(incoming); semanticID != "" {
		for index, invocation := range current {
			if skillInvocationSemanticIdentity(invocation) != semanticID {
				continue
			}
			if shouldKeepExistingInvocation(invocation, incoming) {
				return current
			}
			current[index] = mergeInvocation(invocation, incoming)
			return current
		}
	}
	for index, invocation := range current {
		if sameInvocationIdentity(invocation, incoming) && isOpenInvocation(invocation) {
			if shouldKeepExistingInvocation(invocation, incoming) {
				return current
			}
			current[index] = mergeInvocation(invocation, incoming)
			return current
		}
	}
	for index, invocation := range current {
		if sameInvocationIdentity(invocation, incoming) && shouldMergeClosedInvocation(invocation, incoming) {
			if shouldKeepExistingInvocation(invocation, incoming) {
				return current
			}
			current[index] = mergeInvocation(invocation, incoming)
			return current
		}
	}
	for _, invocation := range current {
		if reflect.DeepEqual(invocation, incoming) {
			return current
		}
	}
	return append(current, incoming)
}

func skillInvocationSemanticIdentity(invocation map[string]interface{}) string {
	if len(invocation) == 0 {
		return ""
	}
	kind := strings.TrimSpace(stringFromAny(invocation["kind"]))
	switch kind {
	case "intermediate_answer":
		if answerID := strings.TrimSpace(stringFromAny(invocation["answer_id"])); answerID != "" {
			return "intermediate_answer:" + answerID
		}
	case "tool_governance":
		if correlationID := toolGovernanceCorrelationID(invocation); correlationID != "" {
			return strings.Join([]string{
				"tool_governance",
				strings.TrimSpace(stringFromAny(invocation["skill_id"])),
				strings.TrimSpace(stringFromAny(invocation["tool_name"])),
				correlationID,
			}, ":")
		}
	case "tool_call":
		if correlationID := toolGovernanceCorrelationID(invocation); correlationID != "" {
			return strings.Join([]string{
				"tool_call_governed",
				strings.TrimSpace(stringFromAny(invocation["skill_id"])),
				strings.TrimSpace(stringFromAny(invocation["tool_name"])),
				correlationID,
			}, ":")
		}
		return assetOperationSemanticIdentity(invocation)
	}
	return ""
}

func assetOperationSemanticIdentity(invocation map[string]interface{}) string {
	if len(invocation) == 0 {
		return ""
	}
	result := mapFromOperationContext(invocation["result"])
	args := mapFromOperationContext(invocation["arguments"])
	audit := governanceMapFromAny(firstNonNil(invocation["asset_operation_audit"], result["asset_operation_audit"], args["asset_operation_audit"]))
	operationGroup := mapFromOperationContext(result["operation_group"])
	operationType := firstNonEmptyString(result["operation_type"], args["operation_type"], operationGroup["operation"])
	assetType := strings.ToLower(firstNonEmptyString(
		invocation["asset_type"],
		audit["asset_type"],
		result["asset_type"],
		args["asset_type"],
		operationGroup["asset_type"],
		assetTypeFromOperationType(operationType),
		assetTypeFromToolResult(invocation, result, args),
	))
	effect := strings.ToLower(firstNonEmptyString(
		invocation["effect"],
		audit["effect"],
		result["effect"],
		args["effect"],
		operationGroup["effect"],
		effectFromOperationType(operationType),
		effectFromToolName(stringFromAny(invocation["tool_name"])),
	))
	if assetType == "" || effect == "" {
		return ""
	}
	if correlationID := firstNonEmptyString(invocation["correlation_id"], audit["correlation_id"], result["correlation_id"], operationGroup["correlation_id"]); correlationID != "" {
		return "asset_operation:" + correlationID
	}
	if actionID := normalizeAssetOperationActionID(firstNonEmptyString(invocation["action_id"], result["action_id"], args["action_id"])); actionID != "" {
		return "asset_operation:" + actionID
	}
	return strings.Join([]string{
		"asset_operation",
		assetType,
		effect,
		stableInvocationIdentityValue(assetOperationIdentityTarget(invocation, result, args, audit, operationGroup)),
	}, ":")
}

func assetTypeFromOperationType(operationType string) string {
	operationType = strings.TrimSpace(operationType)
	if operationType == "" || !strings.Contains(operationType, ".") {
		return ""
	}
	return strings.TrimSpace(strings.Split(operationType, ".")[0])
}

func assetTypeFromToolResult(invocation map[string]interface{}, result map[string]interface{}, args map[string]interface{}) string {
	if firstNonEmptyString(result["agent_id"], args["agent_id"], result["agent"], args["agent"]) != "" ||
		strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skills.SkillAgentManagement) {
		return "agent"
	}
	return ""
}

func effectFromOperationType(operationType string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(operationType)), ".")
	if len(parts) < 2 {
		return ""
	}
	for idx := len(parts) - 1; idx >= 0; idx-- {
		if effect := normalizedOperationEffect(parts[idx]); effect != "" {
			return effect
		}
	}
	return ""
}

func effectFromToolName(toolName string) string {
	for _, token := range strings.FieldsFunc(strings.ToLower(strings.TrimSpace(toolName)), func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == '/'
	}) {
		if effect := normalizedOperationEffect(token); effect != "" {
			return effect
		}
	}
	return ""
}

func normalizedOperationEffect(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "create", "created", "add", "added", "new":
		return "created"
	case "update", "updated", "modify", "modified", "edit", "edited", "set", "replace", "replaced", "bind", "bound", "unbind", "unbound", "remove", "removed":
		return "updated"
	case "delete", "deleted", "destroy", "destroyed":
		return "deleted"
	case "save", "saved", "upload", "uploaded":
		return "saved"
	default:
		return ""
	}
}

func normalizeAssetOperationActionID(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "asset_observation:") {
		return strings.TrimPrefix(value, "asset_observation:")
	}
	return value
}

func assetOperationIdentityTarget(invocation map[string]interface{}, result map[string]interface{}, args map[string]interface{}, audit map[string]interface{}, operationGroup map[string]interface{}) interface{} {
	for _, value := range []interface{}{
		invocation["assets"],
		audit["assets"],
		result["assets"],
		args["assets"],
		operationGroup["targets"],
		result["item_results"],
		operationGroup["item_results"],
	} {
		if hasStableIdentityValue(value) {
			return value
		}
	}
	target := map[string]interface{}{}
	for _, key := range []string{
		"agent_id",
		"agent_name",
		"name",
		"updated_fields",
		"requested_fields",
		"target_count",
		"deleted_count",
		"created_count",
		"updated_count",
	} {
		if value := firstNonNil(result[key], args[key], invocation[key]); hasStableIdentityValue(value) {
			target[key] = value
		}
	}
	if len(target) > 0 {
		return target
	}
	return map[string]interface{}{
		"skill_id":  strings.TrimSpace(stringFromAny(invocation["skill_id"])),
		"tool_name": strings.TrimSpace(stringFromAny(invocation["tool_name"])),
	}
}

func hasStableIdentityValue(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case []interface{}:
		return len(typed) > 0
	case []map[string]interface{}:
		return len(typed) > 0
	case map[string]interface{}:
		return len(typed) > 0
	default:
		return true
	}
}

func stableInvocationIdentityValue(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
	return string(data)
}

func shouldKeepExistingInvocation(existing map[string]interface{}, incoming map[string]interface{}) bool {
	if strings.EqualFold(strings.TrimSpace(stringFromAny(existing["kind"])), "skill_load") &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(incoming["kind"])), "skill_load") &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(existing["status"])), "success") {
		return true
	}
	return false
}

func shouldMergeClosedInvocation(existing map[string]interface{}, incoming map[string]interface{}) bool {
	if strings.EqualFold(strings.TrimSpace(stringFromAny(existing["kind"])), "skill_load") &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(incoming["kind"])), "skill_load") {
		return true
	}
	if strings.TrimSpace(stringFromAny(existing["kind"])) != "guardrail" ||
		strings.TrimSpace(stringFromAny(incoming["kind"])) != "guardrail" {
		return false
	}
	return strings.TrimSpace(stringFromAny(existing["message"])) == strings.TrimSpace(stringFromAny(incoming["message"])) &&
		strings.TrimSpace(stringFromAny(existing["error"])) == strings.TrimSpace(stringFromAny(incoming["error"]))
}

func mergeInvocation(existing map[string]interface{}, incoming map[string]interface{}) map[string]interface{} {
	merged := copyStringAnyMap(existing)
	if merged == nil {
		merged = map[string]interface{}{}
	}
	for key, value := range incoming {
		if value == nil {
			continue
		}
		merged[key] = value
	}
	return compactSkillInvocation(merged)
}

func sameInvocationIdentity(left map[string]interface{}, right map[string]interface{}) bool {
	return invocationRuntimeIdentity(left) == invocationRuntimeIdentity(right)
}

func invocationRuntimeIdentity(invocation map[string]interface{}) string {
	parts := []string{
		strings.TrimSpace(stringFromAny(invocation["kind"])),
		strings.TrimSpace(stringFromAny(invocation["skill_id"])),
		strings.TrimSpace(stringFromAny(invocation["tool_name"])),
		strings.TrimSpace(stringFromAny(invocation["path"])),
		strings.TrimSpace(stringFromAny(invocation["answer_id"])),
	}
	return strings.Join(parts, ":")
}

func traceRuntimeID(trace skills.SkillTrace, index int) string {
	if runtimeID := firstNonEmptyString(valueFromMap(trace.Arguments, "runtime_id"), valueFromMap(trace.Result, "runtime_id")); runtimeID != "" {
		return runtimeID
	}
	return fmt.Sprintf("trace:%06d:%s", index, invocationRuntimeIdentity(map[string]interface{}{
		"kind":      trace.Kind,
		"skill_id":  trace.SkillID,
		"tool_name": trace.ToolName,
		"path":      firstNonEmptyString(valueFromMap(trace.Arguments, "path"), valueFromMap(trace.Result, "path")),
		"answer_id": firstNonEmptyString(valueFromMap(trace.Arguments, "answer_id"), valueFromMap(trace.Result, "answer_id")),
	}))
}

func isOpenInvocation(invocation map[string]interface{}) bool {
	switch strings.TrimSpace(stringFromAny(invocation["status"])) {
	case "loading", "running":
		return true
	default:
		return false
	}
}

func valueFromMap(values map[string]interface{}, key string) interface{} {
	if len(values) == 0 {
		return nil
	}
	return values[key]
}

func numericValueFromMap(values map[string]interface{}, key string) interface{} {
	return numericValueFromAny(valueFromMap(values, key))
}

func numericValueFromAny(value interface{}) interface{} {
	switch value.(type) {
	case int, int64, int32, float64, float32, uint, uint64, uint32:
		return value
	default:
		return nil
	}
}

func intValueFromAny(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint:
		return int(typed)
	case uint32:
		return int(typed)
	case uint64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
		return 0
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
		return 0
	default:
		return 0
	}
}

func countSkillActionInvocations(invocations []map[string]interface{}) int {
	count := 0
	for _, invocation := range invocations {
		if visibleSkillInvocationKind(stringFromAny(invocation["kind"])) {
			count++
		}
	}
	return count
}

func visibleSkillInvocationKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "skill_load", "reference_read", "tool_call", "tool_governance", "client_action", "intermediate_answer", "user_input_request", "guardrail":
		return true
	default:
		return false
	}
}

func internalPlannerFeedbackInvocation(invocation map[string]interface{}) bool {
	if strings.TrimSpace(stringFromAny(invocation["kind"])) != "guardrail" {
		return false
	}
	return operationPlanGuardrailIsPlanningFeedback(invocation)
}

func addConfiguredSkillIDs(metadata map[string]interface{}, seen map[string]struct{}, out *[]interface{}) {
	value, ok := metadata["configured_skill_ids"]
	if !ok {
		return
	}
	add := func(raw string) {
		id := strings.TrimSpace(raw)
		if id == "" {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		*out = append(*out, id)
	}
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			add(item)
		}
	case []interface{}:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				add(text)
			}
		}
	}
}

func generatedFilesFromMetadata(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return append([]map[string]interface{}{}, typed...)
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if file, ok := item.(map[string]interface{}); ok {
				out = append(out, file)
			}
		}
		return out
	default:
		return []map[string]interface{}{}
	}
}

func firstNonEmptyString(values ...interface{}) string {
	for _, value := range values {
		text := strings.TrimSpace(stringFromAny(value))
		if text != "" {
			return text
		}
	}
	return ""
}

func appendDownloadQuery(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if strings.Contains(rawURL, "download=") {
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
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}
