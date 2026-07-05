package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

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
