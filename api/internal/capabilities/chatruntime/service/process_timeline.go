package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	intermediateAnswerCheckpointInterval = 2 * time.Second
	intermediateAnswerCheckpointBytes    = 4 * 1024
)

type intermediateAnswerPersistenceState struct {
	lastPersistedAt   time.Time
	lastPersistedSize int
}

type processTimelineRecorder struct {
	service         *service
	ctx             context.Context
	persistCtx      context.Context
	prepared        *PreparedChat
	onEvent         func(StreamEvent) error
	openRuntimeIDs  map[string]string
	runtimeCounters map[string]int
	intermediate    map[string]intermediateAnswerPersistenceState
	presentation    presentationProjection
	activeSegmentID string
	now             func() time.Time
}

func newProcessTimelineRecorder(ctx context.Context, persistCtx context.Context, svc *service, prepared *PreparedChat, onEvent func(StreamEvent) error) *processTimelineRecorder {
	metadata := map[string]interface{}{}
	if prepared != nil && prepared.Message != nil {
		metadata = prepared.Message.Metadata
	}
	return &processTimelineRecorder{
		service:         svc,
		ctx:             ctx,
		persistCtx:      persistCtx,
		prepared:        prepared,
		onEvent:         onEvent,
		openRuntimeIDs:  map[string]string{},
		runtimeCounters: map[string]int{},
		intermediate:    map[string]intermediateAnswerPersistenceState{},
		presentation:    presentationProjectionFromMetadata(metadata),
		now:             time.Now,
	}
}

func (r *processTimelineRecorder) Emit(eventType string, payload map[string]interface{}) error {
	if r == nil || r.service == nil {
		return nil
	}
	return r.service.emitPreparedEvent(r.ctx, r.prepared, eventType, payload, r.onEvent)
}

func (r *processTimelineRecorder) RecordEvent(eventType string, payload map[string]interface{}) error {
	if r == nil || r.service == nil {
		return nil
	}
	if r.shouldSkipDuplicateSkillLoadEvent(eventType, payload) {
		return nil
	}
	if payload == nil {
		payload = map[string]interface{}{}
	}
	if eventType == streamEventMessage {
		r.recordPresentationTextChunk(payload)
		return r.Emit(eventType, payload)
	}
	if eventType == streamEventMessageRetract {
		segmentID := r.activeSegmentID
		disposition := strings.ToLower(strings.TrimSpace(payloadString(payload, "presentation_disposition")))
		if disposition == presentationDispositionDiscard {
			if item := r.presentation.itemByID(segmentID); len(item) > 0 {
				payload["segment_id"] = item["segment_id"]
			}
			payload["presentation_disposition"] = presentationDispositionDiscard
			r.presentation.removeByID(segmentID)
			r.activeSegmentID = ""
			r.applyPresentationMetadata()
			if err := r.persistPresentation(nil); err != nil {
				return err
			}
			return r.Emit(eventType, payload)
		}
		payload["presentation_disposition"] = presentationDispositionProcess
		r.transitionActivePresentationSegment(presentationPhaseProcess)
		if item := r.presentation.itemByID(segmentID); len(item) > 0 {
			annotatePresentationPayload(payload, item)
		}
		if err := r.persistPresentation(nil); err != nil {
			return err
		}
		return r.Emit(eventType, payload)
	}
	if eventType == streamEventAgentProgress {
		return r.Emit(eventType, payload)
	}
	if !r.activePresentationSegmentIsFinalOutput() {
		r.transitionActivePresentationSegment(presentationPhaseProcess)
		r.activeSegmentID = ""
	}
	if isWorkflowTimelineEvent(eventType) {
		r.service.persistWorkflowRunEventBestEffort(r.persistCtx, r.prepared, eventType, payload)
	}
	invocation := r.invocationFromEvent(eventType, payload)
	r.recordPresentationEvent(eventType, payload, invocation)
	if len(invocation) > 0 {
		if strings.TrimSpace(stringFromAny(invocation["kind"])) == "tool_governance" {
			r.persistGovernedToolCallSuspension(payload)
		}
		if strings.TrimSpace(stringFromAny(invocation["kind"])) == "intermediate_answer" {
			if err := r.recordIntermediateAnswerInvocation(invocation); err != nil {
				return err
			}
		} else {
			if err := r.persistInvocation(invocation); err != nil {
				return err
			}
		}
		copyInvocationRuntimeFields(payload, invocation)
	} else if err := r.persistPresentation(nil); err != nil {
		return err
	}
	return r.Emit(eventType, payload)
}

func (r *processTimelineRecorder) recordPresentationTextChunk(payload map[string]interface{}) {
	if r == nil || r.prepared == nil || r.prepared.Message == nil {
		return
	}
	content := payloadText(payload, "answer")
	if r.activeSegmentID == "" {
		sequence := r.presentation.nextSequence()
		item := newPresentationTextItem(r.prepared.Message.ID.String(), sequence, content, r.currentTime())
		if role := strings.TrimSpace(payloadString(payload, "presentation_role")); role != "" {
			item["presentation_role"] = role
		}
		r.activeSegmentID = stringFromAny(item["presentation_id"])
		r.presentation.upsert(item)
	} else if item := r.presentation.itemByID(r.activeSegmentID); len(item) > 0 {
		item["content"] = stringFromAny(item["content"]) + content
		item["content_phase"] = presentationPhaseProvisional
		if role := strings.TrimSpace(payloadString(payload, "presentation_role")); role != "" {
			item["presentation_role"] = role
		}
		r.presentation.upsert(item)
	}
	annotatePresentationPayload(payload, r.presentation.itemByID(r.activeSegmentID))
	payload["segment_content"] = stringFromAny(r.presentation.itemByID(r.activeSegmentID)["content"])
	r.applyPresentationMetadata()
}

func (r *processTimelineRecorder) activePresentationSegmentIsFinalOutput() bool {
	if r == nil || r.activeSegmentID == "" {
		return false
	}
	item := r.presentation.itemByID(r.activeSegmentID)
	return strings.EqualFold(strings.TrimSpace(stringFromAny(item["presentation_role"])), presentationRoleFinalOutput)
}

func (r *processTimelineRecorder) transitionActivePresentationSegment(phase string) {
	if r == nil || r.activeSegmentID == "" {
		return
	}
	item := r.presentation.itemByID(r.activeSegmentID)
	if len(item) == 0 {
		return
	}
	item["content_phase"] = phase
	r.presentation.upsert(item)
	r.applyPresentationMetadata()
	if phase == presentationPhaseProcess {
		r.activeSegmentID = ""
	}
}

func (r *processTimelineRecorder) recordPresentationEvent(eventType string, payload map[string]interface{}, invocation map[string]interface{}) {
	if r == nil || r.prepared == nil || r.prepared.Message == nil {
		return
	}
	reference := presentationEventReference(payload, invocation)
	sequence := int64(0)
	id := ""
	createdAtMS := r.currentTime().UnixMilli()
	if reference != "" {
		id = presentationEventID(r.prepared.Message.ID.String(), eventType, reference, 0)
		if existing := r.presentation.itemByID(id); len(existing) > 0 {
			sequence, _ = int64FromPresentationValue(existing["presentation_sequence"])
			if existingCreatedAtMS, ok := int64FromPresentationValue(existing["created_at_ms"]); ok {
				createdAtMS = existingCreatedAtMS
			}
		}
	}
	if sequence == 0 {
		sequence = r.presentation.nextSequence()
		if id == "" {
			id = presentationEventID(r.prepared.Message.ID.String(), eventType, reference, sequence)
		}
	}
	item := map[string]interface{}{
		"presentation_id":       id,
		"presentation_sequence": sequence,
		"kind":                  presentationKindEvent,
		"event_type":            eventType,
		"event_ref":             reference,
		"created_at_ms":         createdAtMS,
	}
	r.presentation.upsert(item)
	annotatePresentationPayload(payload, item)
	r.applyPresentationMetadata()
}

func (r *processTimelineRecorder) applyPresentationMetadata() {
	if r == nil || r.prepared == nil || r.prepared.Message == nil {
		return
	}
	if r.prepared.Message.Metadata == nil {
		r.prepared.Message.Metadata = map[string]interface{}{}
	}
	r.prepared.Message.Metadata["presentation_version"] = presentationVersionV2
	r.prepared.Message.Metadata["presentation"] = r.presentation.metadataValue()
}

func (r *processTimelineRecorder) persistPresentation(invocation map[string]interface{}) error {
	if r == nil || r.prepared == nil || r.prepared.Message == nil {
		return nil
	}
	r.applyPresentationMetadata()
	return r.persistMetadata(r.prepared.Message.Metadata, invocation)
}

func (r *processTimelineRecorder) FinalizePresentation(terminationErr error) error {
	if r == nil || r.activeSegmentID == "" {
		return nil
	}
	phase := presentationPhaseFinal
	if terminationErr != nil {
		phase = presentationPhaseProcess
	}
	r.transitionActivePresentationSegment(phase)
	r.activeSegmentID = ""
	return r.persistPresentation(nil)
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
	invocation := skillInvocationFromTrace(trace, index)
	ensureInvocationTimelineTime(invocation, r.now())
	r.persistInvocation(invocation)
	r.service.logSkillTrace(r.ctx, r.prepared, trace)
}

func nonVisibleTraceCarriesMetadata(trace skills.SkillTrace) bool {
	switch strings.TrimSpace(trace.Kind) {
	case "turn_state", "plan_update", "final_answer":
		return true
	case "planner_feedback":
		return strings.EqualFold(strings.TrimSpace(stringFromAny(trace.Arguments["code"])), "operation_plan_phase_mismatch")
	default:
		return false
	}
}

func (r *processTimelineRecorder) RecordInvocationStart(invocationID string, skillID string, toolName string, arguments map[string]interface{}) error {
	if r == nil || r.service == nil || r.prepared == nil || r.prepared.Message == nil {
		return nil
	}
	invocation := newSkillInvocation("tool_call", skillID, toolName, "running", map[string]interface{}{
		"invocation_id": strings.TrimSpace(invocationID),
		"arguments":     arguments,
	})
	invocation["runtime_id"] = r.runtimeIDForStart(invocation)
	payload := skillCallStartPayload(r.prepared, invocationID, skillID, toolName, arguments)
	copyInvocationRuntimeFields(payload, invocation)
	return r.recordInvocationEvent(streamEventSkillCallStart, payload, invocation)
}

func (r *processTimelineRecorder) RecordInvocationEnd(trace skills.SkillTrace) error {
	if r == nil || r.service == nil || r.prepared == nil || r.prepared.Message == nil {
		return nil
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
	copyInvocationRuntimeFields(payload, invocation)
	if err := r.recordInvocationEvent(streamEventSkillCallEnd, payload, invocation); err != nil {
		return err
	}
	r.service.logSkillTrace(r.ctx, r.prepared, trace)
	return nil
}

func (r *processTimelineRecorder) RecordInvocationError(trace skills.SkillTrace) error {
	if r == nil || r.service == nil || r.prepared == nil || r.prepared.Message == nil {
		return nil
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
	copyInvocationRuntimeFields(payload, invocation)
	if err := r.recordInvocationEvent(streamEventSkillCallError, payload, invocation); err != nil {
		return err
	}
	r.service.logSkillTrace(r.ctx, r.prepared, trace)
	return nil
}

func (r *processTimelineRecorder) recordInvocationEvent(eventType string, payload map[string]interface{}, invocation map[string]interface{}) error {
	r.transitionActivePresentationSegment(presentationPhaseProcess)
	r.recordPresentationEvent(eventType, payload, invocation)
	if err := r.persistInvocation(invocation); err != nil {
		return err
	}
	return r.Emit(eventType, payload)
}

func (r *processTimelineRecorder) RecordIntermediateAnswer(trace skills.SkillTrace) {
	if r == nil || r.service == nil || r.prepared == nil || r.prepared.Message == nil {
		return
	}
	if strings.TrimSpace(trace.Kind) == "" {
		trace.Kind = "intermediate_answer"
	}
	_ = r.recordIntermediateAnswerInvocation(skillInvocationFromTrace(trace, 0))
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
			"result": map[string]interface{}{
				"instruction_digest": payload["instruction_digest"],
				"instruction_chars":  payload["instruction_chars"],
				"effective_version":  payload["effective_version"],
				"policy_state":       payload["policy_state"],
				"access_status":      payload["access_status"],
			},
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
			"error_code":      payloadString(payload, "error_code"),
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
			"error_code":  payloadString(payload, "error_code"),
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
	case streamEventMemoryCreate, streamEventMemoryUpdate, streamEventMemoryDelete, streamEventMemoryClear:
		action := payloadString(payload, "action")
		if action == "" {
			switch eventType {
			case streamEventMemoryDelete, streamEventMemoryClear:
				action = "clear"
			default:
				action = "update"
			}
		}
		mutationStatus := payloadString(payload, "mutation_status")
		if mutationStatus == "" {
			if action == "clear" {
				mutationStatus = "cleared"
			} else {
				mutationStatus = "updated"
			}
		}
		invocation := newSkillInvocation("memory_mutation", skills.SkillAgentMemory, agentMemoryToolMutate, payloadStatus(payload, "success"), invocationTimelineFields(payload, map[string]interface{}{
			"memory_scope":    payloadString(payload, "memory_scope"),
			"action":          action,
			"key":             payloadString(payload, "key"),
			"display_name":    payloadString(payload, "display_name"),
			"mutation_status": mutationStatus,
			"source_kind":     payloadString(payload, "source_kind"),
			"operation_id":    payloadString(payload, "operation_id"),
			"revision":        payload["revision"],
			"undoable_until":  payload["undoable_until"],
			"conversation_id": payload["conversation_id"],
			"message_id":      payload["message_id"],
		}))
		if operationID := payloadString(payload, "operation_id"); operationID != "" {
			invocation["runtime_id"] = "memory_mutation:" + operationID
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

func (r *processTimelineRecorder) persistInvocation(invocation map[string]interface{}) error {
	if r == nil || r.service == nil || r.prepared == nil || r.prepared.Message == nil || len(invocation) == 0 {
		return nil
	}
	metadata := r.mergeInvocation(invocation)
	return r.persistMetadata(metadata, invocation)
}

func (r *processTimelineRecorder) mergeInvocation(invocation map[string]interface{}) map[string]interface{} {
	metadata := mergeSkillInvocationMetadata(r.prepared.Message.Metadata, []map[string]interface{}{invocation})
	applyManagedFileArtifactLinks(metadata, []map[string]interface{}{invocation})
	if strings.TrimSpace(stringFromAny(invocation["kind"])) == "tool_governance" {
		if event := toolGovernanceDecisionEventFromInvocation(invocation); toolGovernanceCorrelationID(event) != "" {
			metadata = mergeToolGovernanceDecisionMetadata(metadata, event)
		}
	}
	r.prepared.Message.Metadata = metadata
	return metadata
}

func (r *processTimelineRecorder) persistMetadata(metadata map[string]interface{}, invocation map[string]interface{}) error {
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
		return nil
	}
	if err := r.service.repos.Message.UpdateMetadata(r.persistCtx, r.prepared.Message.ID, metadata); err != nil {
		return fmt.Errorf("persist agent presentation metadata: %w", err)
	}
	return nil
}

func (r *processTimelineRecorder) recordIntermediateAnswerInvocation(invocation map[string]interface{}) error {
	if r == nil || r.prepared == nil || r.prepared.Message == nil || len(invocation) == 0 {
		return nil
	}
	runtimeID := strings.TrimSpace(stringFromAny(invocation["runtime_id"]))
	if runtimeID == "" {
		return r.persistInvocation(invocation)
	}
	status := strings.TrimSpace(stringFromAny(invocation["status"]))
	if status == "success" {
		invocation["partial"] = false
	} else {
		invocation["partial"] = true
	}
	metadata := r.mergeInvocation(invocation)
	now := r.currentTime()
	state := r.intermediate[runtimeID]
	messageSize := len([]byte(stringFromAny(invocation["message"])))
	shouldPersist := status == "success" || state.lastPersistedAt.IsZero() ||
		now.Sub(state.lastPersistedAt) >= intermediateAnswerCheckpointInterval ||
		messageSize-state.lastPersistedSize >= intermediateAnswerCheckpointBytes
	if !shouldPersist {
		return nil
	}
	invocation["checkpointed_at"] = now.Unix()
	metadata = r.mergeInvocation(invocation)
	if err := r.persistMetadata(metadata, invocation); err != nil {
		return err
	}
	if status == "success" {
		delete(r.intermediate, runtimeID)
		return nil
	}
	r.intermediate[runtimeID] = intermediateAnswerPersistenceState{
		lastPersistedAt:   now,
		lastPersistedSize: messageSize,
	}
	return nil
}

func (r *processTimelineRecorder) FlushPendingIntermediateAnswers(terminationErr error) {
	if r == nil || len(r.intermediate) == 0 {
		return
	}
	for runtimeID := range r.intermediate {
		invocation := r.existingInvocation(runtimeID)
		if len(invocation) == 0 {
			delete(r.intermediate, runtimeID)
			continue
		}
		invocation = copyStringAnyMap(invocation)
		invocation["status"] = "error"
		invocation["partial"] = true
		invocation["checkpointed_at"] = r.currentTime().Unix()
		if terminationErr != nil {
			invocation["error"] = trimRunes(terminationErr.Error(), 500)
		} else {
			invocation["error"] = "intermediate answer stream ended before completion"
		}
		metadata := r.mergeInvocation(invocation)
		r.persistMetadata(metadata, invocation)
		delete(r.intermediate, runtimeID)
	}
}

func (r *processTimelineRecorder) currentTime() time.Time {
	if r != nil && r.now != nil {
		return r.now()
	}
	return time.Now()
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
	if _, exists := values["invocation_id"]; !exists {
		values["invocation_id"] = payload["invocation_id"]
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
	if _, ok := invocation["invocation_id"]; !ok {
		invocation["invocation_id"] = payload["invocation_id"]
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
	for _, key := range []string{"kind", "invocation_id", "runtime_id", "path", "answer_id", "created_at", "created_at_ms"} {
		if value, ok := invocation[key]; ok && value != nil {
			payload[key] = value
		}
	}
}
