package service

import (
	"testing"
	"time"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestModelInvocationMetadataRecordsStreamTermination(t *testing.T) {
	invocation := modelInvocationFromTrace(skillloop.ModelInvocationTrace{
		Phase:              "skill_planning",
		Round:              2,
		Streaming:          true,
		StartedAt:          time.Unix(10, 0),
		FinishReason:       "tool_calls",
		StreamDoneReceived: true,
		TerminatedBy:       "done",
	}, "", true)

	if got := stringFromAny(invocation["finish_reason"]); got != "tool_calls" {
		t.Fatalf("finish_reason = %q, want tool_calls", got)
	}
	if got := stringFromAny(invocation["terminated_by"]); got != "done" {
		t.Fatalf("terminated_by = %q, want done", got)
	}
	if got, ok := invocation["stream_done_received"].(bool); !ok || !got {
		t.Fatalf("stream_done_received = %#v, want true", invocation["stream_done_received"])
	}
}

func TestFinalAnswerTraceAppliesFinalPlanSnapshot(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{map[string]interface{}{
				"id":     "phase-1",
				"status": "in_progress",
			}},
		},
	}
	applyPlanUpdateTraceMetadata(metadata, skills.SkillTrace{
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

	phases := mapSliceFromAny(mapFromOperationContext(metadata["operation_plan"])["phases"])
	if len(phases) != 1 || stringFromAny(phases[0]["status"]) != "completed" {
		t.Fatalf("operation plan phases = %#v, want completed final snapshot", phases)
	}
}

func TestPlanUpdateTraceMarksPlanCurrentAtLatestEvidence(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"plan_sync_status": "stale",
			operationPlanEvidenceLedgerKey: []interface{}{map[string]interface{}{
				"status": "completed", "sequence": 4,
			}},
		},
	}
	applyPlanUpdateTraceMetadata(metadata, skills.SkillTrace{
		Kind:      "plan_update",
		Status:    "success",
		Arguments: map[string]interface{}{"round": 3},
		Result: map[string]interface{}{
			"plan": []interface{}{map[string]interface{}{
				"id": "phase-1", "step": "Continue the task", "status": "in_progress",
			}},
		},
	})

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["plan_sync_status"]); got != "current" {
		t.Fatalf("plan_sync_status = %q, want current", got)
	}
	if got := intValueFromAny(plan["last_plan_update_round"]); got != 3 {
		t.Fatalf("last_plan_update_round = %d, want 3", got)
	}
	if got := intValueFromAny(plan["evidence_sequence_at_plan_update"]); got != 4 {
		t.Fatalf("evidence_sequence_at_plan_update = %d, want 4", got)
	}
	if got := intValueFromAny(plan["evidence_after_last_plan_update"]); got != 0 {
		t.Fatalf("evidence_after_last_plan_update = %d, want 0", got)
	}
}

func TestSkillLoopPrefersExplicitFinalAnswerOnlyForPlannedAIChatTurn(t *testing.T) {
	tests := []struct {
		name       string
		callerType string
		metadata   map[string]interface{}
		want       bool
	}{
		{
			name:       "webapp agent without plan",
			callerType: runtimemodel.ConversationCallerAgent,
			metadata:   map[string]interface{}{},
		},
		{
			name:       "webapp agent with plan remains ordinary agent chat",
			callerType: runtimemodel.ConversationCallerAgent,
			metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{"status": "running"},
			},
		},
		{
			name:       "aichat without plan",
			callerType: runtimemodel.ConversationCallerAIChat,
			metadata:   map[string]interface{}{},
		},
		{
			name:       "planned aichat operation",
			callerType: runtimemodel.ConversationCallerAIChat,
			metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{"status": "running"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepared := &PreparedChat{
				Caller:  Caller{Type: tt.callerType},
				Message: &runtimemodel.Message{Metadata: tt.metadata},
			}
			if got := skillLoopPrefersExplicitFinalAnswer(prepared); got != tt.want {
				t.Fatalf("skillLoopPrefersExplicitFinalAnswer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContextualTurnIntentClassifierPayloadIncludesRecentTaskContext(t *testing.T) {
	parts := &chatRequestParts{
		Query:   "do not delete it; update it directly",
		Surface: aiChatSurfaceContextualSidebar,
		RecentOperationPlans: []map[string]interface{}{{
			"status":              "running",
			"original_user_goal":  "generate a chapter, then update the current Agent with it",
			"pending_next_action": "continue_from_phase_success_criteria",
			"phases": []interface{}{
				map[string]interface{}{
					"id":     "phase-delete",
					"step":   "delete the Agent",
					"status": "pending",
				},
				map[string]interface{}{
					"id":     "phase-update",
					"step":   "update the Agent system prompt with the generated chapter",
					"status": "pending",
				},
			},
		}},
	}
	payload := contextualTurnIntentClassifierPayload(parts)
	recent := mapSliceFromAny(payload["recent_task_context"])
	if len(recent) != 1 {
		t.Fatalf("recent_task_context = %#v, want one plan", payload["recent_task_context"])
	}
	if got := stringFromAny(recent[0]["original_user_goal"]); got != "generate a chapter, then update the current Agent with it" {
		t.Fatalf("original_user_goal = %q", got)
	}
	phases := mapSliceFromAny(recent[0]["phases"])
	if len(phases) != 2 || stringFromAny(phases[1]["id"]) != "phase-update" {
		t.Fatalf("phases = %#v, want pending update context", phases)
	}
}
