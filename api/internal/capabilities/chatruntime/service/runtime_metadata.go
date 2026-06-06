package service

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
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

func (s *service) persistWorkflowRunEventBestEffort(ctx context.Context, prepared *PreparedChat, eventType string, payload map[string]interface{}) {
	if prepared == nil || prepared.Message == nil || len(payload) == 0 {
		return
	}
	metadata := mergeWorkflowRunMetadata(prepared.Message.Metadata, eventType, payload)
	prepared.Message.Metadata = metadata
	if s == nil || s.repos == nil || s.repos.Message == nil {
		return
	}
	if err := s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata); err != nil {
		logger.WarnContext(ctx, "failed to persist aichat workflow run metadata", "message_id", prepared.Message.ID.String(), err)
	}
}

func (s *service) persistWorkflowApprovalPending(ctx context.Context, prepared *PreparedChat, payload map[string]interface{}, usage *adapter.Usage) map[string]interface{} {
	if prepared == nil || prepared.Message == nil || prepared.Conversation == nil {
		return map[string]interface{}{}
	}
	pendingPayload := copyStringAnyMap(payload)
	if pendingPayload == nil {
		pendingPayload = map[string]interface{}{}
	}
	pendingPayload["conversation_id"] = prepared.Conversation.ID.String()
	pendingPayload["message_id"] = prepared.Message.ID.String()
	metadata := mergeWorkflowRunMetadata(prepared.Message.Metadata, "approval_requested", pendingPayload)
	metadata = preparedResultMetadata(metadata, usage)
	metadata["agent_workflow_continuation"] = compactWorkflowRun(map[string]interface{}{
		"status":          "waiting_approval",
		"workflow_run_id": firstNonEmptyString(pendingPayload["workflow_run_id"]),
		"workflow_id":     firstNonEmptyString(pendingPayload["workflow_id"]),
		"agent_id":        firstNonEmptyString(pendingPayload["agent_id"]),
		"agent_type":      firstNonEmptyString(pendingPayload["agent_type"]),
		"binding_id":      firstNonEmptyString(pendingPayload["binding_id"]),
		"original_query":  prepared.Message.Query,
		"approval_token":  firstNonEmptyString(pendingPayload["approval_token"]),
		"approval_url":    firstNonEmptyString(pendingPayload["approval_url"]),
		"resume_policy":   "same_message",
	})
	prepared.Message.Metadata = metadata
	if s == nil || s.repos == nil || s.repos.Message == nil || s.repos.Conversation == nil {
		return metadata
	}
	if err := s.repos.Message.UpdateWaitingApproval(ctx, prepared.Message.ID, metadata); err != nil {
		logger.WarnContext(ctx, "failed to mark aichat workflow approval pending", "message_id", prepared.Message.ID.String(), err)
	}
	if err := s.repos.Conversation.FinishWaitingApprovalMessage(ctx, prepared.Conversation.ID, prepared.Message.ID); err != nil {
		logger.WarnContext(ctx, "failed to finish aichat workflow approval pending message", "conversation_id", prepared.Conversation.ID.String(), err)
	}
	return metadata
}

func mergeGeneratedArtifactMetadata(source map[string]interface{}, artifact map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	storedArtifact := persistentGeneratedArtifact(artifact)
	files := generatedFilesFromMetadata(metadata["generated_files"])
	fileID := stringFromAny(storedArtifact["file_id"])
	for idx, item := range files {
		if fileID != "" && stringFromAny(item["file_id"]) == fileID {
			files[idx] = storedArtifact
			metadata["generated_files"] = files
			metadata["generated_file_count"] = len(files)
			return metadata
		}
	}
	files = append(files, storedArtifact)
	metadata["generated_files"] = files
	metadata["generated_file_count"] = len(files)
	return metadata
}

func mergeWorkflowRunMetadata(source map[string]interface{}, eventType string, payload map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	run := workflowRunFromEvent(eventType, payload)
	if len(run) == 0 {
		return metadata
	}
	runs := workflowRunsFromMetadata(metadata["workflow_runs"])
	runs = upsertWorkflowRun(runs, run)
	metadata["has_trace"] = true
	metadata["workflow_runs"] = workflowRunsToInterfaceSlice(runs)
	metadata["workflow_run_count"] = len(runs)
	return metadata
}

func workflowRunFromEvent(eventType string, payload map[string]interface{}) map[string]interface{} {
	runID := firstNonEmptyString(payload["workflow_run_id"], payload["task_id"], payload["id"])
	if runID == "" {
		return nil
	}
	run := map[string]interface{}{
		"workflow_run_id": runID,
		"status":          workflowRunStatusFromEvent(eventType, payload),
	}
	copyWorkflowFields(run, payload, "workflow_id", "agent_id", "version", "inputs", "outputs", "elapsed_time", "error", "created_at")
	if createdAt := workflowCreatedAt(payload); createdAt != nil {
		run["created_at"] = createdAt
	}
	if approval := workflowApprovalFromEvent(payload); len(approval) > 0 {
		run["approval"] = approval
	}
	switch strings.TrimSpace(eventType) {
	case "node_started":
		run["nodes"] = []interface{}{workflowNodeFromEvent(payload, false)}
	case "node_finished":
		run["nodes"] = []interface{}{workflowNodeFromEvent(payload, true)}
	case "workflow_paused":
		if node := workflowNodeFromEvent(payload, true); len(node) > 0 {
			node["status"] = "paused"
			run["nodes"] = []interface{}{node}
		}
	}
	return compactWorkflowRun(run)
}

func workflowRunStatusFromEvent(eventType string, payload map[string]interface{}) string {
	switch strings.TrimSpace(eventType) {
	case "workflow_started":
		return "running"
	case "workflow_paused", "approval_requested":
		return "pending_approval"
	case "workflow_finished":
		if status := firstNonEmptyString(payload["status"]); status != "" {
			return status
		}
		return "completed"
	case "workflow_failed":
		return "error"
	default:
		if status := firstNonEmptyString(payload["status"]); status != "" {
			return status
		}
		return "running"
	}
}

func workflowNodeFromEvent(payload map[string]interface{}, finished bool) map[string]interface{} {
	nodeID := firstNonEmptyString(payload["node_id"], payload["execution_id"])
	nodeType := firstNonEmptyString(payload["node_type"], payload["type"])
	title := firstNonEmptyString(payload["title"], payload["node_title"], payload["name"], payload["label"])
	if nodeID == "" && nodeType == "" && title == "" {
		return nil
	}
	status := firstNonEmptyString(payload["status"])
	if status == "" {
		if finished {
			status = "succeeded"
		} else {
			status = "running"
		}
	}
	node := map[string]interface{}{
		"node_id":   nodeID,
		"node_type": nodeType,
		"title":     title,
		"status":    status,
	}
	copyWorkflowFields(node, payload, "inputs", "outputs", "elapsed_time", "error", "created_at")
	if createdAt := workflowCreatedAt(payload); createdAt != nil {
		node["created_at"] = createdAt
	}
	return compactWorkflowRun(node)
}

func workflowApprovalFromEvent(payload map[string]interface{}) map[string]interface{} {
	approval := map[string]interface{}{}
	copyWorkflowFields(approval, payload, "approval_form_id", "approval_token", "approval_url", "approval_form")
	return compactWorkflowRun(approval)
}

func copyWorkflowFields(target map[string]interface{}, source map[string]interface{}, keys ...string) {
	for _, key := range keys {
		value, ok := source[key]
		if !ok || value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		target[key] = value
	}
}

func workflowCreatedAt(payload map[string]interface{}) interface{} {
	if createdAt := numericValueFromMap(payload, "created_at"); createdAt != nil {
		return createdAt
	}
	return time.Now().Unix()
}

func workflowRunsFromMetadata(value interface{}) []map[string]interface{} {
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
			if run, ok := item.(map[string]interface{}); ok {
				out = append(out, copyStringAnyMap(run))
			}
		}
		return out
	default:
		return []map[string]interface{}{}
	}
}

func workflowRunsToInterfaceSlice(runs []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(runs))
	for _, run := range runs {
		out = append(out, run)
	}
	return out
}

func upsertWorkflowRun(current []map[string]interface{}, incoming map[string]interface{}) []map[string]interface{} {
	runID := strings.TrimSpace(stringFromAny(incoming["workflow_run_id"]))
	if runID == "" {
		return current
	}
	for index, run := range current {
		if strings.TrimSpace(stringFromAny(run["workflow_run_id"])) == runID {
			current[index] = mergeWorkflowRun(run, incoming)
			return current
		}
	}
	return append(current, incoming)
}

func mergeWorkflowRun(existing map[string]interface{}, incoming map[string]interface{}) map[string]interface{} {
	merged := copyStringAnyMap(existing)
	if merged == nil {
		merged = map[string]interface{}{}
	}
	for key, value := range incoming {
		if value == nil {
			continue
		}
		switch key {
		case "nodes":
			merged["nodes"] = mergeWorkflowNodes(workflowNodesFromMetadata(merged["nodes"]), workflowNodesFromMetadata(value))
		case "approval":
			merged["approval"] = mergeWorkflowMap(workflowRecordFromAny(merged["approval"]), workflowRecordFromAny(value))
		default:
			merged[key] = value
		}
	}
	return compactWorkflowRun(merged)
}

func workflowRecordFromAny(value interface{}) map[string]interface{} {
	if record, ok := value.(map[string]interface{}); ok {
		return record
	}
	return map[string]interface{}{}
}

func workflowNodesFromMetadata(value interface{}) []map[string]interface{} {
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
			if node, ok := item.(map[string]interface{}); ok {
				out = append(out, copyStringAnyMap(node))
			}
		}
		return out
	default:
		return []map[string]interface{}{}
	}
}

func mergeWorkflowNodes(current []map[string]interface{}, incoming []map[string]interface{}) []interface{} {
	for _, node := range incoming {
		if len(node) == 0 {
			continue
		}
		key := workflowNodeIdentity(node)
		matched := false
		for index, existing := range current {
			if workflowNodeIdentity(existing) == key {
				current[index] = mergeWorkflowMap(existing, node)
				matched = true
				break
			}
		}
		if !matched {
			current = append(current, node)
		}
	}
	out := make([]interface{}, 0, len(current))
	for _, node := range current {
		out = append(out, compactWorkflowRun(node))
	}
	return out
}

func workflowNodeIdentity(node map[string]interface{}) string {
	if nodeID := strings.TrimSpace(stringFromAny(node["node_id"])); nodeID != "" {
		return "node:" + nodeID
	}
	return strings.Join([]string{
		"type:" + strings.TrimSpace(stringFromAny(node["node_type"])),
		"title:" + strings.TrimSpace(stringFromAny(node["title"])),
	}, ":")
}

func mergeWorkflowMap(existing map[string]interface{}, incoming map[string]interface{}) map[string]interface{} {
	merged := copyStringAnyMap(existing)
	if merged == nil {
		merged = map[string]interface{}{}
	}
	for key, value := range incoming {
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		merged[key] = value
	}
	return compactWorkflowRun(merged)
}

func compactWorkflowRun(values map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(values))
	for key, value := range values {
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func mergeSkillTraceMetadata(source map[string]interface{}, traces []skills.SkillTrace) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if len(traces) == 0 {
		return metadata
	}
	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	for index, trace := range traces {
		if !visibleSkillInvocationKind(trace.Kind) {
			continue
		}
		invocations = upsertSkillInvocation(invocations, skillInvocationFromTrace(trace, index))
	}
	applySkillInvocationSummary(metadata, invocations)
	return metadata
}

func mergeSkillInvocationMetadata(source map[string]interface{}, invocations []map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if len(invocations) == 0 {
		return metadata
	}
	stored := skillInvocationsFromMetadata(metadata["skill_invocations"])
	for _, invocation := range invocations {
		if !visibleSkillInvocationKind(stringFromAny(invocation["kind"])) {
			continue
		}
		stored = upsertSkillInvocation(stored, invocation)
	}
	applySkillInvocationSummary(metadata, stored)
	return metadata
}

func applySkillInvocationSummary(metadata map[string]interface{}, invocations []map[string]interface{}) {
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
	return compactSkillInvocation(invocation)
}

func newSkillInvocation(kind, skillID, toolName, status string, values map[string]interface{}) map[string]interface{} {
	invocation := map[string]interface{}{
		"kind":       strings.TrimSpace(kind),
		"skill_id":   strings.TrimSpace(skillID),
		"tool_name":  strings.TrimSpace(toolName),
		"status":     strings.TrimSpace(status),
		"created_at": time.Now().Unix(),
	}
	for key, value := range values {
		invocation[key] = value
	}
	if strings.TrimSpace(stringFromAny(invocation["runtime_id"])) == "" {
		invocation["runtime_id"] = invocationRuntimeIdentity(invocation)
	}
	return compactSkillInvocation(invocation)
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
				current[index] = mergeInvocation(invocation, incoming)
				return current
			}
		}
	}
	for index, invocation := range current {
		if sameInvocationIdentity(invocation, incoming) && isOpenInvocation(invocation) {
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
	value := valueFromMap(values, key)
	switch value.(type) {
	case int, int64, int32, float64, float32, uint, uint64, uint32:
		return value
	default:
		return nil
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
	case "skill_load", "reference_read", "tool_call", "intermediate_answer", "user_input_request", "guardrail":
		return true
	default:
		return false
	}
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
