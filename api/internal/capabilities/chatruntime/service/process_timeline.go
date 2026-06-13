package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
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
	if isWorkflowTimelineEvent(eventType) {
		r.service.persistWorkflowRunEventBestEffort(r.persistCtx, r.prepared, eventType, payload)
	}
	invocation := r.invocationFromEvent(eventType, payload)
	if len(invocation) > 0 {
		r.persistInvocation(invocation)
		copyInvocationRuntimeFields(payload, invocation)
	}
	r.Emit(eventType, payload)
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
	r.persistInvocation(invocation)
	payload := skillCallEndPayload(r.prepared, trace)
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
	r.persistInvocation(invocation)
	payload := skillCallErrorPayload(r.prepared, trace)
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
		invocation := newSkillInvocation("skill_load", payloadString(payload, "skill_id"), "", "loading", map[string]interface{}{
			"created_at": payload["created_at"],
		})
		invocation["runtime_id"] = r.runtimeIDForStart(invocation)
		return invocation
	case streamEventSkillLoadEnd:
		invocation := newSkillInvocation("skill_load", payloadString(payload, "skill_id"), "", payloadStatus(payload, "success"), map[string]interface{}{
			"duration_ms": payload["duration_ms"],
			"created_at":  payload["created_at"],
		})
		invocation["runtime_id"] = r.runtimeIDForEnd(invocation)
		return invocation
	case streamEventSkillReferenceRead:
		invocation := newSkillInvocation("reference_read", payloadString(payload, "skill_id"), "", payloadStatus(payload, "success"), map[string]interface{}{
			"path":        payloadString(payload, "path"),
			"duration_ms": payload["duration_ms"],
			"created_at":  payload["created_at"],
		})
		invocation["runtime_id"] = r.runtimeIDForStandalone(invocation)
		return invocation
	case streamEventSkillCallStart:
		invocation := newSkillInvocation("tool_call", payloadString(payload, "skill_id"), payloadString(payload, "tool_name"), "running", map[string]interface{}{
			"arguments":  payloadMap(payload, "arguments_summary", "arguments"),
			"created_at": payload["created_at"],
		})
		invocation["runtime_id"] = r.runtimeIDForStart(invocation)
		return invocation
	case streamEventSkillCallEnd:
		kind := payloadString(payload, "kind")
		if kind == "" {
			kind = "tool_call"
		}
		invocation := newSkillInvocation(kind, payloadString(payload, "skill_id"), payloadString(payload, "tool_name"), payloadStatus(payload, "success"), map[string]interface{}{
			"duration_ms": payload["duration_ms"],
			"message":     payloadString(payload, "message"),
			"result":      payloadMap(payload, "result"),
			"created_at":  payload["created_at"],
		})
		invocation["runtime_id"] = r.runtimeIDForEnd(invocation)
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
		invocation := newSkillInvocation(kind, payloadString(payload, "skill_id"), payloadString(payload, "tool_name"), "error", map[string]interface{}{
			"duration_ms": payload["duration_ms"],
			"message":     payloadString(payload, "message"),
			"error":       payloadString(payload, "message"),
			"created_at":  payload["created_at"],
		})
		invocation["runtime_id"] = r.runtimeIDForEnd(invocation)
		return invocation
	case streamEventIntermediateAnswer:
		answerID := payloadString(payload, "answer_id")
		invocation := newSkillInvocation("intermediate_answer", "", "", intermediateAnswerStatus(payload), map[string]interface{}{
			"answer_id":  answerID,
			"title":      payloadString(payload, "title"),
			"message":    r.intermediateAnswerMessage(answerID, payloadText(payload, "content"), payloadBool(payload, "delta")),
			"created_at": payload["created_at"],
		})
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
	r.runtimeCounters[base]++
	return fmt.Sprintf("%s#%d", base, r.runtimeCounters[base])
}

func (r *processTimelineRecorder) persistInvocation(invocation map[string]interface{}) {
	if r == nil || r.service == nil || r.prepared == nil || r.prepared.Message == nil || len(invocation) == 0 {
		return
	}
	metadata := mergeSkillInvocationMetadata(r.prepared.Message.Metadata, []map[string]interface{}{invocation})
	r.prepared.Message.Metadata = metadata
	if r.service.repos == nil || r.service.repos.Message == nil {
		return
	}
	_ = r.service.repos.Message.UpdateMetadata(r.persistCtx, r.prepared.Message.ID, metadata)
}

func streamBackedTrace(trace skills.SkillTrace) bool {
	switch strings.TrimSpace(trace.Kind) {
	case "skill_load", "reference_read", "tool_call", "intermediate_answer":
		return true
	default:
		return false
	}
}

func payloadString(payload map[string]interface{}, key string) string {
	return strings.TrimSpace(stringFromAny(payload[key]))
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
	for _, key := range []string{"kind", "runtime_id", "path", "answer_id"} {
		if value, ok := invocation[key]; ok && value != nil {
			payload[key] = value
		}
	}
}
