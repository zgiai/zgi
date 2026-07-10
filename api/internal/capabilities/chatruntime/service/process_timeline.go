package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

type processTimelineRecorder struct {
	service         *service
	ctx             context.Context
	persistCtx      context.Context
	prepared        *PreparedChat
	onEvent         func(StreamEvent) error
	openRuntimeIDs  map[string]string
	runtimeCounters map[string]int
}

func newProcessTimelineRecorder(ctx context.Context, persistCtx context.Context, svc *service, prepared *PreparedChat, onEvent func(StreamEvent) error) *processTimelineRecorder {
	return &processTimelineRecorder{
		service:         svc,
		ctx:             ctx,
		persistCtx:      persistCtx,
		prepared:        prepared,
		onEvent:         onEvent,
		openRuntimeIDs:  map[string]string{},
		runtimeCounters: map[string]int{},
	}
}

func (r *processTimelineRecorder) Emit(eventType string, payload map[string]interface{}) {
	if r == nil || r.service == nil {
		return
	}
	r.service.emitPreparedEvent(r.ctx, r.prepared, eventType, payload, r.onEvent)
}

func (r *processTimelineRecorder) RecordEvent(eventType string, payload map[string]interface{}) {
	if r == nil || r.service == nil {
		return
	}
	if r.shouldSkipDuplicateSkillLoadEvent(eventType, payload) {
		return
	}
	if isWorkflowTimelineEvent(eventType) {
		r.service.persistWorkflowRunEventBestEffort(r.persistCtx, r.prepared, eventType, payload)
	}
	invocation := r.invocationFromEvent(eventType, payload)
	if len(invocation) > 0 {
		if strings.TrimSpace(stringFromAny(invocation["kind"])) == "tool_governance" {
			r.persistGovernedToolCallSuspension(payload)
		}
		r.persistInvocation(invocation)
		copyInvocationRuntimeFields(payload, invocation)
	}
	r.Emit(eventType, payload)
}

func (r *processTimelineRecorder) shouldSkipDuplicateSkillLoadEvent(eventType string, payload map[string]interface{}) bool {
	switch strings.TrimSpace(eventType) {
	case streamEventSkillLoadStart, streamEventSkillLoadEnd:
	default:
		return false
	}
	if r == nil || r.prepared == nil || r.prepared.Message == nil {
		return false
	}
	skillID := payloadString(payload, "skill_id")
	if skillID == "" {
		return false
	}
	for _, invocation := range skillInvocationsFromMetadata(r.prepared.Message.Metadata["skill_invocations"]) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "skill_load") ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skillID) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["status"])), "success") {
			return true
		}
	}
	return false
}

func isWorkflowTimelineEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "workflow_started", "node_started", "node_finished", "workflow_paused", "approval_requested", "workflow_finished", "workflow_failed",
		"iteration_started", "iteration_next", "iteration_completed", "iteration_succeeded", "iteration_failed",
		"loop_started", "loop_next", "loop_completed", "loop_succeeded", "loop_failed":
		return true
	default:
		return false
	}
}

func (r *processTimelineRecorder) RecordTrace(traces []skills.SkillTrace, trace skills.SkillTrace) {
	if r == nil || r.service == nil {
		return
	}
	if nonVisibleTraceCarriesMetadata(trace) {
		r.service.persistSkillTracesBestEffort(r.persistCtx, r.prepared, []skills.SkillTrace{trace})
	}
	if streamBackedTrace(trace) {
		r.service.logSkillTrace(r.ctx, r.prepared, trace)
		return
	}
	if !visibleSkillInvocationKind(trace.Kind) {
		r.service.logSkillTrace(r.ctx, r.prepared, trace)
		return
	}
	index := len(traces) - 1
	if index < 0 {
		index = 0
	}
	r.persistInvocation(skillInvocationFromTrace(trace, index))
	r.service.logSkillTrace(r.ctx, r.prepared, trace)
}

func nonVisibleTraceCarriesMetadata(trace skills.SkillTrace) bool {
	switch strings.TrimSpace(trace.Kind) {
	case "turn_state", "plan_update":
		return true
	default:
		return false
	}
}

func (r *processTimelineRecorder) RecordInvocationStart(skillID string, toolName string, arguments map[string]interface{}) {
	if r == nil || r.service == nil || r.prepared == nil || r.prepared.Message == nil {
		return
	}
	invocation := newSkillInvocation("tool_call", skillID, toolName, "running", map[string]interface{}{
		"arguments": arguments,
	})
	invocation["runtime_id"] = r.runtimeIDForStart(invocation)
	r.persistInvocation(invocation)
	payload := skillCallStartPayload(r.prepared, skillID, toolName, arguments)
	copyInvocationRuntimeFields(payload, invocation)
	r.Emit(streamEventSkillCallStart, payload)
}

func (r *processTimelineRecorder) RecordInvocationEnd(trace skills.SkillTrace) {
	if r == nil || r.service == nil || r.prepared == nil || r.prepared.Message == nil {
		return
	}
	if strings.TrimSpace(trace.Kind) == "" {
		trace.Kind = "tool_call"
	}
	if strings.TrimSpace(trace.Status) == "" {
		trace.Status = "success"
	}
	invocation := skillInvocationFromTrace(trace, 0)
	invocation["runtime_id"] = r.runtimeIDForEnd(invocation)
	payload := skillCallEndPayload(r.prepared, trace)
	fillInvocationTimelineFromPayload(invocation, payload)
	r.persistInvocation(invocation)
	copyInvocationRuntimeFields(payload, invocation)
	r.Emit(streamEventSkillCallEnd, payload)
	r.service.logSkillTrace(r.ctx, r.prepared, trace)
}

func (r *processTimelineRecorder) RecordInvocationError(trace skills.SkillTrace) {
	if r == nil || r.service == nil || r.prepared == nil || r.prepared.Message == nil {
		return
	}
	if strings.TrimSpace(trace.Kind) == "" {
		trace.Kind = "tool_call"
	}
	if strings.TrimSpace(trace.Status) == "" {
		trace.Status = "error"
	}
	invocation := skillInvocationFromTrace(trace, 0)
	invocation["runtime_id"] = r.runtimeIDForEnd(invocation)
	payload := skillCallErrorPayload(r.prepared, trace)
	fillInvocationTimelineFromPayload(invocation, payload)
	r.persistInvocation(invocation)
	copyInvocationRuntimeFields(payload, invocation)
	r.Emit(streamEventSkillCallError, payload)
	r.service.logSkillTrace(r.ctx, r.prepared, trace)
}

func (r *processTimelineRecorder) RecordIntermediateAnswer(trace skills.SkillTrace) {
	if r == nil || r.service == nil || r.prepared == nil || r.prepared.Message == nil {
		return
	}
	if strings.TrimSpace(trace.Kind) == "" {
		trace.Kind = "intermediate_answer"
	}
	r.persistInvocation(skillInvocationFromTrace(trace, 0))
	r.service.logSkillTrace(r.ctx, r.prepared, trace)
}

func (r *processTimelineRecorder) invocationFromEvent(eventType string, payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	switch eventType {
	case streamEventSkillLoadStart:
		invocation := newSkillInvocation("skill_load", payloadString(payload, "skill_id"), "", "loading", invocationTimelineFields(payload, nil))
		invocation["runtime_id"] = r.runtimeIDForStart(invocation)
		return invocation
	case streamEventSkillLoadEnd:
		invocation := newSkillInvocation("skill_load", payloadString(payload, "skill_id"), "", payloadStatus(payload, "success"), invocationTimelineFields(payload, map[string]interface{}{
			"duration_ms": payload["duration_ms"],
		}))
		invocation["runtime_id"] = r.runtimeIDForEnd(invocation)
		return invocation
	case streamEventSkillReferenceRead:
		invocation := newSkillInvocation("reference_read", payloadString(payload, "skill_id"), "", payloadStatus(payload, "success"), invocationTimelineFields(payload, map[string]interface{}{
			"path":        payloadString(payload, "path"),
			"duration_ms": payload["duration_ms"],
		}))
		invocation["runtime_id"] = r.runtimeIDForStandalone(invocation)
		return invocation
	case streamEventToolGovernanceDecision:
		invocation := newSkillInvocation("tool_governance", payloadString(payload, "skill_id"), payloadString(payload, "tool_name"), payloadStatus(payload, "needs_approval"), invocationTimelineFields(payload, map[string]interface{}{
			"conversation_id":       payload["conversation_id"],
			"message_id":            payload["message_id"],
			"duration_ms":           payload["duration_ms"],
			"governance":            governanceMapFromAny(payload["governance"]),
			"asset_operation_audit": governanceMapFromAny(payload["asset_operation_audit"]),
			"approval_status":       payload["approval_status"],
			"result": map[string]interface{}{
				"approval_event": governanceMapFromAny(payload["approval_event"]),
			},
		}))
		if runtimeID := toolGovernanceRuntimeIDFromEvent(payload); runtimeID != "" {
			invocation["runtime_id"] = runtimeID
		} else {
			invocation["runtime_id"] = r.runtimeIDForStandalone(invocation)
		}
		return invocation
	case streamEventSkillCallStart:
		invocation := newSkillInvocation("tool_call", payloadString(payload, "skill_id"), payloadString(payload, "tool_name"), "running", invocationTimelineFields(payload, map[string]interface{}{
			"arguments": payloadMap(payload, "arguments_summary", "arguments"),
		}))
		invocation["runtime_id"] = r.runtimeIDForStart(invocation)
		return invocation
	case streamEventSkillCallEnd:
		kind := payloadString(payload, "kind")
		if kind == "" {
			kind = "tool_call"
		}
		invocation := newSkillInvocation(kind, payloadString(payload, "skill_id"), payloadString(payload, "tool_name"), payloadStatus(payload, "success"), invocationTimelineFields(payload, map[string]interface{}{
			"duration_ms":     payload["duration_ms"],
			"message":         payloadString(payload, "message"),
			"result":          payloadMap(payload, "result"),
			"governance":      governanceMapFromAny(payload["governance"]),
			"conversation_id": payload["conversation_id"],
			"message_id":      payload["message_id"],
		}))
		if kind == "tool_governance" {
			if runtimeID := toolGovernanceRuntimeIDFromEvent(payload); runtimeID != "" {
				invocation["runtime_id"] = runtimeID
			} else {
				invocation["runtime_id"] = r.runtimeIDForStandalone(invocation)
			}
		} else {
			invocation["runtime_id"] = r.runtimeIDForEnd(invocation)
		}
		return invocation
	case streamEventClientActionRequired:
		invocation := newSkillInvocation("client_action", payloadString(payload, "skill_id"), payloadString(payload, "tool_name"), payloadStatus(payload, "waiting_client_action"), invocationTimelineFields(payload, map[string]interface{}{
			"action_id":             payloadString(payload, "action_id"),
			"action_type":           payloadString(payload, "action_type"),
			"href":                  payloadString(payload, "href"),
			"label":                 payloadString(payload, "label"),
			"reason":                payloadString(payload, "reason"),
			"continuation_policy":   payloadString(payload, "continuation_policy"),
			"blocking":              payload["blocking"],
			"effect":                payloadString(payload, "effect"),
			"asset_type":            payloadString(payload, "asset_type"),
			"assets":                payload["assets"],
			"correlation_id":        payloadString(payload, "correlation_id"),
			"result":                payloadMap(payload, "result"),
			"asset_operation_audit": governanceMapFromAny(payload["asset_operation_audit"]),
			"conversation_id":       payload["conversation_id"],
			"message_id":            payload["message_id"],
		}))
		invocation["runtime_id"] = "client_action:" + payloadString(payload, "action_id")
		return invocation
	case streamEventSkillCallError:
		kind := payloadString(payload, "kind")
		if kind == "" {
			if payloadString(payload, "tool_name") == "" {
				kind = "skill_load"
			} else {
				kind = "tool_call"
			}
		}
		invocation := newSkillInvocation(kind, payloadString(payload, "skill_id"), payloadString(payload, "tool_name"), payloadStatus(payload, "error"), invocationTimelineFields(payload, map[string]interface{}{
			"duration_ms": payload["duration_ms"],
			"message":     payloadString(payload, "message"),
			"error":       payloadString(payload, "message"),
		}))
		invocation["runtime_id"] = r.runtimeIDForEnd(invocation)
		return invocation
	case streamEventIntermediateAnswer:
		answerID := payloadString(payload, "answer_id")
		invocation := newSkillInvocation("intermediate_answer", "", "", intermediateAnswerStatus(payload), invocationTimelineFields(payload, map[string]interface{}{
			"answer_id": answerID,
			"title":     payloadString(payload, "title"),
			"message":   r.intermediateAnswerMessage(answerID, payloadText(payload, "content"), payloadBool(payload, "delta")),
		}))
		if answerID != "" {
			invocation["runtime_id"] = invocationRuntimeIdentity(invocation)
		} else {
			invocation["runtime_id"] = r.runtimeIDForStandalone(invocation)
		}
		return invocation
	default:
		return nil
	}
}

func (r *processTimelineRecorder) intermediateAnswerMessage(answerID string, content string, delta bool) string {
	if answerID == "" || !delta {
		return content
	}
	runtimeID := invocationRuntimeIdentity(map[string]interface{}{
		"kind":      "intermediate_answer",
		"answer_id": answerID,
	})
	existing := r.existingInvocation(runtimeID)
	if existing == nil {
		return content
	}
	previous := stringFromAny(existing["message"])
	if content == "" {
		return previous
	}
	return previous + content
}

func (r *processTimelineRecorder) existingInvocation(runtimeID string) map[string]interface{} {
	if r == nil || r.prepared == nil || r.prepared.Message == nil || strings.TrimSpace(runtimeID) == "" {
		return nil
	}
	for _, invocation := range skillInvocationsFromMetadata(r.prepared.Message.Metadata["skill_invocations"]) {
		if strings.TrimSpace(stringFromAny(invocation["runtime_id"])) == runtimeID {
			return invocation
		}
	}
	return nil
}

func (r *processTimelineRecorder) runtimeIDForStart(invocation map[string]interface{}) string {
	base := invocationRuntimeIdentity(invocation)
	if runtimeID := r.reusableGovernedToolCallRuntimeID(invocation); runtimeID != "" {
		r.openRuntimeIDs[base] = runtimeID
		return runtimeID
	}
	runtimeID := r.nextRuntimeID(base)
	r.openRuntimeIDs[base] = runtimeID
	return runtimeID
}

func (r *processTimelineRecorder) runtimeIDForEnd(invocation map[string]interface{}) string {
	base := invocationRuntimeIdentity(invocation)
	if runtimeID := strings.TrimSpace(r.openRuntimeIDs[base]); runtimeID != "" {
		delete(r.openRuntimeIDs, base)
		return runtimeID
	}
	return r.nextRuntimeID(base)
}

func (r *processTimelineRecorder) runtimeIDForStandalone(invocation map[string]interface{}) string {
	return r.nextRuntimeID(invocationRuntimeIdentity(invocation))
}

func (r *processTimelineRecorder) nextRuntimeID(base string) string {
	if strings.TrimSpace(base) == "" {
		base = "event"
	}
	for {
		r.runtimeCounters[base]++
		runtimeID := fmt.Sprintf("%s#%d", base, r.runtimeCounters[base])
		if !r.runtimeIDExists(runtimeID) {
			return runtimeID
		}
	}
}

func (r *processTimelineRecorder) runtimeIDExists(runtimeID string) bool {
	runtimeID = strings.TrimSpace(runtimeID)
	if r == nil || r.prepared == nil || r.prepared.Message == nil || runtimeID == "" {
		return false
	}
	for _, invocation := range skillInvocationsFromMetadata(r.prepared.Message.Metadata["skill_invocations"]) {
		if strings.TrimSpace(stringFromAny(invocation["runtime_id"])) == runtimeID {
			return true
		}
	}
	return false
}

func (r *processTimelineRecorder) reusableGovernedToolCallRuntimeID(invocation map[string]interface{}) string {
	if r == nil || r.prepared == nil || r.prepared.Message == nil {
		return ""
	}
	if strings.TrimSpace(stringFromAny(invocation["kind"])) != "tool_call" {
		return ""
	}
	base := invocationRuntimeIdentity(invocation)
	continuationCorrelationID := toolGovernanceCorrelationID(
		governanceMapFromAny(r.prepared.Message.Metadata["tool_governance_continuation"]),
	)
	invocations := skillInvocationsFromMetadata(r.prepared.Message.Metadata["skill_invocations"])
	for idx := len(invocations) - 1; idx >= 0; idx-- {
		existing := invocations[idx]
		if invocationRuntimeIdentity(existing) != base {
			continue
		}
		if continuationCorrelationID != "" && toolGovernanceCorrelationID(existing) != continuationCorrelationID {
			continue
		}
		if !isReusableGovernedToolCall(existing) {
			continue
		}
		if runtimeID := strings.TrimSpace(stringFromAny(existing["runtime_id"])); runtimeID != "" {
			return runtimeID
		}
	}
	return ""
}

func isReusableGovernedToolCall(invocation map[string]interface{}) bool {
	if strings.TrimSpace(stringFromAny(invocation["kind"])) != "tool_call" {
		return false
	}
	switch strings.TrimSpace(stringFromAny(invocation["status"])) {
	case "waiting_approval", "approved", "allowed", "needs_resolution":
	default:
		return false
	}
	if strings.TrimSpace(stringFromAny(invocation["error"])) != "" ||
		strings.TrimSpace(stringFromAny(invocation["message"])) != "" {
		return false
	}
	if result := mapFromOperationContext(invocation["result"]); len(result) > 0 {
		return false
	}
	if strings.TrimSpace(stringFromAny(invocation["correlation_id"])) != "" ||
		strings.TrimSpace(stringFromAny(invocation["governance_runtime_id"])) != "" {
		return true
	}
	return len(governanceMapFromAny(invocation["governance"])) > 0 ||
		len(governanceMapFromAny(invocation["asset_operation_audit"])) > 0
}

func (r *processTimelineRecorder) persistInvocation(invocation map[string]interface{}) {
	if r == nil || r.service == nil || r.prepared == nil || r.prepared.Message == nil || len(invocation) == 0 {
		return
	}
	metadata := mergeSkillInvocationMetadata(r.prepared.Message.Metadata, []map[string]interface{}{invocation})
	if strings.TrimSpace(stringFromAny(invocation["kind"])) == "tool_governance" {
		if event := toolGovernanceDecisionEventFromInvocation(invocation); toolGovernanceCorrelationID(event) != "" {
			metadata = mergeToolGovernanceDecisionMetadata(metadata, event)
		}
	}
	r.prepared.Message.Metadata = metadata
	if aichatTimelineDebugEnabled() {
		invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
		logger.DebugContext(r.ctx, "aichat timeline metadata persisted",
			"message_id", r.prepared.Message.ID.String(),
			"conversation_id", r.prepared.Conversation.ID.String(),
			"invocation_count", len(invocations),
			"kind", timelineDebugString(invocation["kind"]),
			"runtime_id", timelineDebugString(invocation["runtime_id"]),
			"event_id", timelineDebugString(invocation["event_id"]),
			"created_at", timelineDebugString(invocation["created_at"]),
			"created_at_ms", timelineDebugString(invocation["created_at_ms"]),
			"skill_id", timelineDebugString(invocation["skill_id"]),
			"tool_name", timelineDebugString(invocation["tool_name"]),
			"status", timelineDebugString(invocation["status"]),
		)
	}
	if r.service.repos == nil || r.service.repos.Message == nil {
		return
	}
	_ = r.service.repos.Message.UpdateMetadata(r.persistCtx, r.prepared.Message.ID, metadata)
}

func (r *processTimelineRecorder) persistGovernedToolCallSuspension(payload map[string]interface{}) {
	if r == nil || r.prepared == nil || r.prepared.Message == nil || len(payload) == 0 {
		return
	}
	status := firstNonEmptyString(payloadString(payload, "status"), payloadString(payload, "decision"))
	if status == "" {
		status = strings.TrimSpace(stringFromAny(governanceMapFromAny(payload["governance"])["status"]))
	}
	toolCallStatus := governedToolCallPendingStatus(status)
	if toolCallStatus == "" {
		return
	}
	skillID := payloadString(payload, "skill_id")
	toolName := payloadString(payload, "tool_name")
	if skillID == "" || toolName == "" {
		return
	}
	base := invocationRuntimeIdentity(map[string]interface{}{
		"kind":      "tool_call",
		"skill_id":  skillID,
		"tool_name": toolName,
	})
	runtimeID := strings.TrimSpace(r.openRuntimeIDs[base])
	if runtimeID == "" {
		runtimeID = r.openGovernedToolCallRuntimeID(skillID, toolName)
	}
	if runtimeID == "" {
		return
	}
	delete(r.openRuntimeIDs, base)
	invocation := newSkillInvocation("tool_call", skillID, toolName, toolCallStatus, map[string]interface{}{
		"runtime_id":             runtimeID,
		"created_at":             payload["created_at"],
		"created_at_ms":          payload["created_at_ms"],
		"governance":             governanceMapFromAny(payload["governance"]),
		"asset_operation_audit":  governanceMapFromAny(payload["asset_operation_audit"]),
		"approval_status":        payload["approval_status"],
		"correlation_id":         payload["correlation_id"],
		"governance_runtime_id":  toolGovernanceRuntimeIDFromEvent(payload),
		"governance_status":      status,
		"requires_user_approval": toolCallStatus == "waiting_approval",
	})
	r.persistInvocation(invocation)
}

func (r *processTimelineRecorder) openGovernedToolCallRuntimeID(skillID string, toolName string) string {
	if r == nil || r.prepared == nil || r.prepared.Message == nil {
		return ""
	}
	var runtimeID string
	for _, invocation := range skillInvocationsFromMetadata(r.prepared.Message.Metadata["skill_invocations"]) {
		if strings.TrimSpace(stringFromAny(invocation["kind"])) != "tool_call" {
			continue
		}
		if strings.TrimSpace(stringFromAny(invocation["skill_id"])) != skillID ||
			strings.TrimSpace(stringFromAny(invocation["tool_name"])) != toolName {
			continue
		}
		if !isOpenInvocation(invocation) {
			continue
		}
		if candidate := strings.TrimSpace(stringFromAny(invocation["runtime_id"])); candidate != "" {
			runtimeID = candidate
		}
	}
	return runtimeID
}

func governedToolCallPendingStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "needs_approval":
		return "waiting_approval"
	case "needs_resolution":
		return "needs_resolution"
	case "denied":
		return "denied"
	case "blocked":
		return "blocked"
	default:
		return ""
	}
}

func streamBackedTrace(trace skills.SkillTrace) bool {
	switch strings.TrimSpace(trace.Kind) {
	case "skill_load", "reference_read", "tool_call", "tool_governance", "client_action", "intermediate_answer":
		return true
	default:
		return false
	}
}

func payloadString(payload map[string]interface{}, key string) string {
	return strings.TrimSpace(stringFromAny(payload[key]))
}

func invocationTimelineFields(payload map[string]interface{}, values map[string]interface{}) map[string]interface{} {
	if values == nil {
		values = map[string]interface{}{}
	}
	if len(payload) == 0 {
		return values
	}
	if _, exists := values["created_at"]; !exists {
		values["created_at"] = payload["created_at"]
	}
	if _, exists := values["created_at_ms"]; !exists {
		values["created_at_ms"] = payload["created_at_ms"]
	}
	return values
}

func fillInvocationTimelineFromPayload(invocation map[string]interface{}, payload map[string]interface{}) {
	if len(invocation) == 0 || len(payload) == 0 {
		return
	}
	if _, ok := invocation["created_at"]; !ok {
		invocation["created_at"] = payload["created_at"]
	}
	if _, ok := invocation["created_at_ms"]; !ok {
		invocation["created_at_ms"] = payload["created_at_ms"]
	}
	normalizeSkillInvocationTimelineFields(invocation)
}

func payloadText(payload map[string]interface{}, key string) string {
	return stringFromAny(payload[key])
}

func payloadStatus(payload map[string]interface{}, fallback string) string {
	if status := payloadString(payload, "status"); status != "" {
		return status
	}
	return fallback
}

func payloadMap(payload map[string]interface{}, keys ...string) map[string]interface{} {
	for _, key := range keys {
		if value, ok := payload[key].(map[string]interface{}); ok {
			return value
		}
	}
	return nil
}

func payloadBool(payload map[string]interface{}, key string) bool {
	value, _ := payload[key].(bool)
	return value
}

func intermediateAnswerStatus(payload map[string]interface{}) string {
	if done, ok := payload["done"].(bool); ok && done {
		return "success"
	}
	return "running"
}

func copyInvocationRuntimeFields(payload map[string]interface{}, invocation map[string]interface{}) {
	if len(payload) == 0 || len(invocation) == 0 {
		return
	}
	for _, key := range []string{"kind", "runtime_id", "path", "answer_id", "created_at", "created_at_ms"} {
		if value, ok := invocation[key]; ok && value != nil {
			payload[key] = value
		}
	}
}
