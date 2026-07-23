package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

type countingTimelineMessageRepo struct {
	repository.MessageRepository
	updates  int
	metadata map[string]interface{}
}

func (r *countingTimelineMessageRepo) UpdateMetadata(_ context.Context, _ uuid.UUID, metadata map[string]interface{}) error {
	r.updates++
	r.metadata = copyStringAnyMap(metadata)
	return nil
}

func TestProcessTimelineRecorderCheckpointsIntermediateAnswerWithoutWriteAmplification(t *testing.T) {
	messageRepo := &countingTimelineMessageRepo{}
	message := &runtimemodel.Message{ID: uuid.New(), Metadata: map[string]interface{}{}}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      message,
	}
	recorder := newProcessTimelineRecorder(
		context.Background(),
		context.Background(),
		&service{repos: &repository.Repositories{Message: messageRepo}},
		prepared,
		nil,
	)
	now := time.Unix(1_700_000_000, 0)
	recorder.now = func() time.Time { return now }

	for range 1490 {
		recorder.RecordEvent(streamEventIntermediateAnswer, map[string]interface{}{
			"answer_id": "draft-1",
			"content":   "ab",
			"delta":     true,
		})
	}
	recorder.RecordEvent(streamEventIntermediateAnswer, map[string]interface{}{
		"answer_id": "draft-1",
		"done":      true,
	})

	if messageRepo.updates != 2 {
		t.Fatalf("UpdateMetadata calls = %d, want first checkpoint plus final write", messageRepo.updates)
	}
	invocations := skillInvocationsFromMetadata(messageRepo.metadata["skill_invocations"])
	if len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want one intermediate answer", invocations)
	}
	if got := len(stringFromAny(invocations[0]["message"])); got != 2980 {
		t.Fatalf("persisted message length = %d, want 2980", got)
	}
	if got := stringFromAny(invocations[0]["status"]); got != "success" {
		t.Fatalf("status = %q, want success", got)
	}
	if partial, _ := invocations[0]["partial"].(bool); partial {
		t.Fatalf("partial = true, want false: %#v", invocations[0])
	}
}

func TestProcessTimelineRecorderPersistsIntermediateAnswerOnTimeOrSizeCheckpoint(t *testing.T) {
	messageRepo := &countingTimelineMessageRepo{}
	message := &runtimemodel.Message{ID: uuid.New(), Metadata: map[string]interface{}{}}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      message,
	}
	recorder := newProcessTimelineRecorder(
		context.Background(),
		context.Background(),
		&service{repos: &repository.Repositories{Message: messageRepo}},
		prepared,
		nil,
	)
	now := time.Unix(1_700_000_000, 0)
	recorder.now = func() time.Time { return now }

	recorder.RecordEvent(streamEventIntermediateAnswer, map[string]interface{}{"answer_id": "draft-1", "content": "a", "delta": true})
	now = now.Add(time.Second)
	recorder.RecordEvent(streamEventIntermediateAnswer, map[string]interface{}{"answer_id": "draft-1", "content": "b", "delta": true})
	if messageRepo.updates != 1 {
		t.Fatalf("UpdateMetadata calls before threshold = %d, want 1", messageRepo.updates)
	}
	now = now.Add(time.Second)
	recorder.RecordEvent(streamEventIntermediateAnswer, map[string]interface{}{"answer_id": "draft-1", "content": "c", "delta": true})
	if messageRepo.updates != 2 {
		t.Fatalf("UpdateMetadata calls after time threshold = %d, want 2", messageRepo.updates)
	}
	recorder.RecordEvent(streamEventIntermediateAnswer, map[string]interface{}{
		"answer_id": "draft-1",
		"content":   strings.Repeat("x", intermediateAnswerCheckpointBytes),
		"delta":     true,
	})
	if messageRepo.updates != 3 {
		t.Fatalf("UpdateMetadata calls after size threshold = %d, want 3", messageRepo.updates)
	}
}

func TestProcessTimelineRecorderFlushesPartialIntermediateAnswerOnError(t *testing.T) {
	messageRepo := &countingTimelineMessageRepo{}
	message := &runtimemodel.Message{ID: uuid.New(), Metadata: map[string]interface{}{}}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      message,
	}
	recorder := newProcessTimelineRecorder(
		context.Background(),
		context.Background(),
		&service{repos: &repository.Repositories{Message: messageRepo}},
		prepared,
		nil,
	)
	recorder.RecordEvent(streamEventIntermediateAnswer, map[string]interface{}{"answer_id": "draft-1", "content": "first ", "delta": true})
	recorder.RecordEvent(streamEventIntermediateAnswer, map[string]interface{}{"answer_id": "draft-1", "content": "second", "delta": true})
	recorder.FlushPendingIntermediateAnswers(errors.New("model stream interrupted"))

	if messageRepo.updates != 2 {
		t.Fatalf("UpdateMetadata calls = %d, want checkpoint plus error flush", messageRepo.updates)
	}
	invocations := skillInvocationsFromMetadata(messageRepo.metadata["skill_invocations"])
	if len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want one intermediate answer", invocations)
	}
	if got := stringFromAny(invocations[0]["message"]); got != "first second" {
		t.Fatalf("message = %q, want full partial answer", got)
	}
	if got := stringFromAny(invocations[0]["status"]); got != "error" {
		t.Fatalf("status = %q, want error", got)
	}
	if partial, _ := invocations[0]["partial"].(bool); !partial {
		t.Fatalf("partial = false, want true: %#v", invocations[0])
	}
	if got := stringFromAny(invocations[0]["error"]); got != "model stream interrupted" {
		t.Fatalf("error = %q, want model stream interrupted", got)
	}
}

func TestProcessTimelineRecorderReusesPendingGovernedToolCallRuntimeID(t *testing.T) {
	const runtimeID = "tool_call:agent-management:delete_agent::#1"
	message := &runtimemodel.Message{
		ID: uuid.New(),
		Metadata: map[string]interface{}{
			"tool_governance_continuation": map[string]interface{}{
				"correlation_id": "approval-corr-1",
				"status":         "approved",
			},
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":                  "tool_call",
					"skill_id":              skills.SkillAgentManagement,
					"tool_name":             "delete_agent",
					"status":                "waiting_approval",
					"runtime_id":            runtimeID,
					"correlation_id":        "approval-corr-1",
					"governance_runtime_id": "tool_governance:approval-corr-1",
					"governance": map[string]interface{}{
						"status": "needs_approval",
					},
				},
			},
		},
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      message,
	}
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)

	recorder.RecordInvocationStart(skills.SkillAgentManagement, "delete_agent", map[string]interface{}{"agent_id": "agent-1"})
	recorder.RecordInvocationError(skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agent",
		Status:   "error",
		Error:    "agent not found",
	})

	invocations := skillInvocationsFromMetadata(message.Metadata["skill_invocations"])
	if len(invocations) != 1 {
		t.Fatalf("skill_invocations len = %d, want 1: %#v", len(invocations), invocations)
	}
	invocation := invocations[0]
	if got := stringFromAny(invocation["runtime_id"]); got != runtimeID {
		t.Fatalf("runtime_id = %q, want %q; invocation=%#v", got, runtimeID, invocation)
	}
	if got := stringFromAny(invocation["status"]); got != "error" {
		t.Fatalf("status = %q, want error; invocation=%#v", got, invocation)
	}
	if got := stringFromAny(invocation["error"]); got != "agent not found" {
		t.Fatalf("error = %q, want agent not found; invocation=%#v", got, invocation)
	}
	if governance := governanceMapFromAny(invocation["governance"]); len(governance) == 0 {
		t.Fatalf("governance metadata was dropped: %#v", invocation)
	}
}

func TestProcessTimelineRecorderSkipsDuplicateSuccessfulSkillLoadEvents(t *testing.T) {
	const runtimeID = "skill_load:agent-management:::#1"
	now := time.Now()
	message := &runtimemodel.Message{
		ID: uuid.New(),
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":          "skill_load",
					"skill_id":      skills.SkillAgentManagement,
					"status":        "success",
					"runtime_id":    runtimeID,
					"created_at_ms": now.UnixMilli(),
				},
			},
		},
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      message,
	}
	emitted := []StreamEvent{}
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, func(event StreamEvent) error {
		emitted = append(emitted, event)
		return nil
	})

	recorder.RecordEvent(streamEventSkillLoadStart, map[string]interface{}{
		"skill_id":      skills.SkillAgentManagement,
		"created_at_ms": now.Add(time.Second).UnixMilli(),
	})
	recorder.RecordEvent(streamEventSkillLoadEnd, map[string]interface{}{
		"skill_id":      skills.SkillAgentManagement,
		"status":        "success",
		"created_at_ms": now.Add(2 * time.Second).UnixMilli(),
	})

	if len(emitted) != 0 {
		t.Fatalf("emitted events = %#v, want duplicate skill_load events skipped", emitted)
	}
	invocations := skillInvocationsFromMetadata(message.Metadata["skill_invocations"])
	if len(invocations) != 1 {
		t.Fatalf("skill_invocations len = %d, want 1: %#v", len(invocations), invocations)
	}
	if got := stringFromAny(invocations[0]["runtime_id"]); got != runtimeID {
		t.Fatalf("runtime_id = %q, want original %q; invocations=%#v", got, runtimeID, invocations)
	}
}

func TestProcessTimelineRecorderPersistsTurnStateTraceWithoutVisibleInvocation(t *testing.T) {
	message := &runtimemodel.Message{
		ID:       uuid.New(),
		Metadata: map[string]interface{}{},
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      message,
	}
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)

	recorder.RecordTrace([]skills.SkillTrace{{
		Kind:   "turn_state",
		Status: "success",
		Result: map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{
					"kind":       "working_fact",
					"visibility": "model_only",
					"key":        "source_file_theme",
					"value":      "snow character",
					"source":     "file-reader/read_file",
				},
			},
		},
	}}, skills.SkillTrace{
		Kind:   "turn_state",
		Status: "success",
		Result: map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{
					"kind":       "working_fact",
					"visibility": "model_only",
					"key":        "source_file_theme",
					"value":      "snow character",
					"source":     "file-reader/read_file",
				},
			},
		},
	})

	state := mapFromOperationContext(message.Metadata["turn_state"])
	items := mapSliceFromAny(state["items"])
	if len(items) != 1 {
		t.Fatalf("turn_state items = %#v, want one item", items)
	}
	if got := stringFromAny(items[0]["key"]); got != "source_file_theme" {
		t.Fatalf("turn_state key = %q, want source_file_theme; items=%#v", got, items)
	}
	if got := stringFromAny(items[0]["value"]); got != "snow character" {
		t.Fatalf("turn_state value = %q, want snow character; items=%#v", got, items)
	}
	if invocations := skillInvocationsFromMetadata(message.Metadata["skill_invocations"]); len(invocations) != 0 {
		t.Fatalf("skill_invocations = %#v, want turn_state hidden from visible timeline", invocations)
	}
}

func TestProcessTimelineRecorderPersistsFinalAnswerPlanWithoutVisibleInvocation(t *testing.T) {
	message := &runtimemodel.Message{
		ID: uuid.New(),
		Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"phases": []interface{}{map[string]interface{}{
					"id":     "phase-1",
					"status": "in_progress",
				}},
			},
		},
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      message,
	}
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)

	recorder.RecordTrace([]skills.SkillTrace{{
		Kind:   "final_answer",
		Status: "success",
		Result: map[string]interface{}{
			"plan": []interface{}{map[string]interface{}{
				"id":            "phase-1",
				"step":          "Complete the requested operation",
				"status":        "completed",
				"evidence_refs": []interface{}{"runtime_id:tool-1"},
			}},
		},
	}}, skills.SkillTrace{
		Kind:   "final_answer",
		Status: "success",
		Result: map[string]interface{}{
			"plan": []interface{}{map[string]interface{}{
				"id":            "phase-1",
				"step":          "Complete the requested operation",
				"status":        "completed",
				"evidence_refs": []interface{}{"runtime_id:tool-1"},
			}},
		},
	})

	phases := mapSliceFromAny(mapFromOperationContext(message.Metadata["operation_plan"])["phases"])
	if len(phases) != 1 || stringFromAny(phases[0]["status"]) != "completed" {
		t.Fatalf("operation_plan.phases = %#v, want completed final answer plan snapshot", phases)
	}
	plan := mapFromOperationContext(message.Metadata["operation_plan"])
	if got := stringFromAny(plan["plan_sync_status"]); got != "current" {
		t.Fatalf("plan_sync_status = %q, want current", got)
	}
	if invocations := skillInvocationsFromMetadata(message.Metadata["skill_invocations"]); len(invocations) != 0 {
		t.Fatalf("skill_invocations = %#v, want final_answer hidden from visible timeline", invocations)
	}
}

func TestProcessTimelineRecorderPersistsMissingFinalPlanWarningWithoutClosingStalePlan(t *testing.T) {
	message := &runtimemodel.Message{
		ID: uuid.New(),
		Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"plan_sync_status": "stale",
				"phases": []interface{}{map[string]interface{}{
					"id":     "phase-1",
					"status": "in_progress",
				}},
			},
		},
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      message,
	}
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)
	warning := "missing_or_invalid_final_plan_snapshot: plan is required when operation_plan.phases is non-empty"

	recorder.RecordTrace([]skills.SkillTrace{{
		Kind:   "final_answer",
		Status: "success",
		Result: map[string]interface{}{"plan_warning": warning},
	}}, skills.SkillTrace{
		Kind:   "final_answer",
		Status: "success",
		Result: map[string]interface{}{"plan_warning": warning},
	})

	plan := mapFromOperationContext(message.Metadata["operation_plan"])
	if got := stringFromAny(plan["plan_sync_status"]); got != "stale" {
		t.Fatalf("plan_sync_status = %q, want stale", got)
	}
	warnings := stringSliceFromAny(plan["final_plan_warnings"])
	if len(warnings) != 1 || warnings[0] != warning {
		t.Fatalf("final_plan_warnings = %#v, want missing plan warning", warnings)
	}
}

func TestUpsertSkillInvocationKeepsFirstSuccessfulSkillLoad(t *testing.T) {
	first := map[string]interface{}{
		"kind":          "skill_load",
		"skill_id":      skills.SkillAgentManagement,
		"status":        "success",
		"runtime_id":    "skill_load:agent-management:::#1",
		"created_at_ms": int64(1000),
	}
	second := map[string]interface{}{
		"kind":          "skill_load",
		"skill_id":      skills.SkillAgentManagement,
		"status":        "success",
		"runtime_id":    "skill_load:agent-management:::#2",
		"created_at_ms": int64(2000),
	}

	invocations := upsertSkillInvocation([]map[string]interface{}{first}, second)
	if len(invocations) != 1 {
		t.Fatalf("skill_invocations len = %d, want 1: %#v", len(invocations), invocations)
	}
	if got := stringFromAny(invocations[0]["runtime_id"]); got != stringFromAny(first["runtime_id"]) {
		t.Fatalf("runtime_id = %q, want original %q; invocations=%#v", got, first["runtime_id"], invocations)
	}
	if got, _ := unixMillisecondsFromAny(invocations[0]["created_at_ms"]); got != 1000 {
		t.Fatalf("created_at_ms = %d, want original 1000; invocations=%#v", got, invocations)
	}
}

func TestProcessTimelineRecorderReusesMatchingGovernedToolCallRuntimeID(t *testing.T) {
	const targetRuntimeID = "tool_call:agent-management:delete_agent::#1"
	const otherRuntimeID = "tool_call:agent-management:delete_agent::#2"
	message := &runtimemodel.Message{
		ID: uuid.New(),
		Metadata: map[string]interface{}{
			"tool_governance_continuation": map[string]interface{}{
				"correlation_id": "approval-corr-target",
				"status":         "approved",
			},
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":           "tool_call",
					"skill_id":       skills.SkillAgentManagement,
					"tool_name":      "delete_agent",
					"status":         "waiting_approval",
					"runtime_id":     targetRuntimeID,
					"correlation_id": "approval-corr-target",
					"governance": map[string]interface{}{
						"status":         "needs_approval",
						"correlation_id": "approval-corr-target",
					},
				},
				map[string]interface{}{
					"kind":           "tool_call",
					"skill_id":       skills.SkillAgentManagement,
					"tool_name":      "delete_agent",
					"status":         "waiting_approval",
					"runtime_id":     otherRuntimeID,
					"correlation_id": "approval-corr-other",
					"governance": map[string]interface{}{
						"status":         "needs_approval",
						"correlation_id": "approval-corr-other",
					},
				},
			},
		},
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      message,
	}
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)

	recorder.RecordInvocationStart(skills.SkillAgentManagement, "delete_agent", map[string]interface{}{"agent_id": "agent-target"})
	recorder.RecordInvocationEnd(skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agent",
		Status:   "success",
		Result:   map[string]interface{}{"deleted_count": 1},
	})

	invocations := skillInvocationsFromMetadata(message.Metadata["skill_invocations"])
	if len(invocations) != 2 {
		t.Fatalf("skill_invocations len = %d, want 2: %#v", len(invocations), invocations)
	}
	if got := stringFromAny(invocations[0]["runtime_id"]); got != targetRuntimeID {
		t.Fatalf("target runtime_id = %q, want %q; invocations=%#v", got, targetRuntimeID, invocations)
	}
	if got := stringFromAny(invocations[0]["status"]); got != "success" {
		t.Fatalf("target status = %q, want success; invocations=%#v", got, invocations)
	}
	if got := stringFromAny(invocations[1]["runtime_id"]); got != otherRuntimeID {
		t.Fatalf("other runtime_id = %q, want %q; invocations=%#v", got, otherRuntimeID, invocations)
	}
	if got := stringFromAny(invocations[1]["status"]); got != "waiting_approval" {
		t.Fatalf("other status = %q, want waiting_approval; invocations=%#v", got, invocations)
	}
}

func TestProcessTimelineRecorderPersistsManagedFileLinkAfterApprovedInvocation(t *testing.T) {
	messageRepo := &countingTimelineMessageRepo{}
	message := &runtimemodel.Message{
		ID: uuid.New(),
		Metadata: mergeGeneratedArtifactMetadata(map[string]interface{}{}, map[string]interface{}{
			"file_id":         "tool-file-1",
			"tool_file_id":    "tool-file-1",
			"filename":        "chapter.md",
			"mime_type":       "text/markdown",
			"target":          "temporary_artifact",
			"content_chars":   4096,
			"content_sha256":  "sha256:chapter",
			"content_summary": "Chapter summary",
		}),
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      message,
	}
	recorder := newProcessTimelineRecorder(
		t.Context(),
		t.Context(),
		&service{repos: &repository.Repositories{Message: messageRepo}},
		prepared,
		nil,
	)

	recorder.RecordInvocationEnd(skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillFileManager,
		ToolName: "save_file_to_management",
		Status:   "success",
		Arguments: map[string]interface{}{
			"source_type":  "tool_file",
			"tool_file_id": "tool-file-1",
			"filename":     "chapter.md",
		},
		Result: map[string]interface{}{
			"file_id":        "managed-file-1",
			"upload_file_id": "managed-file-1",
			"filename":       "chapter.md",
			"target":         "managed_file",
		},
	})

	artifacts := conversationArtifactsFromMetadata(messageRepo.metadata["conversation_artifacts"])
	if len(artifacts) != 2 {
		t.Fatalf("conversation_artifacts = %#v, want temporary and managed artifacts", artifacts)
	}
	managed := artifacts[1]
	if got := stringFromAny(managed["artifact_id"]); got != "managed_file:managed-file-1" {
		t.Fatalf("managed artifact_id = %q, want managed_file:managed-file-1", got)
	}
	if got := stringFromAny(managed["source_tool_file_id"]); got != "tool-file-1" {
		t.Fatalf("managed source_tool_file_id = %q, want tool-file-1", got)
	}
	if got := stringFromAny(managed["content_sha256"]); got != "sha256:chapter" {
		t.Fatalf("managed content_sha256 = %q, want inherited digest", got)
	}
	if got := stringFromAny(managed["content_summary"]); got != "Chapter summary" {
		t.Fatalf("managed content_summary = %q, want inherited summary", got)
	}
	if got := intValueFromAny(managed["content_chars"]); got != 4096 {
		t.Fatalf("managed content_chars = %d, want 4096", got)
	}
}
