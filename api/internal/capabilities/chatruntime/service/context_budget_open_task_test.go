package service

import (
	"strings"
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

// The open task state follows the previous turn's state, whatever the latest
// request says; the model decides whether the request continues it (ADR 0047).
func TestContinuationTaskStateFollowsThePreviousTurnState(t *testing.T) {
	openPlan := map[string]interface{}{
		"original_user_goal": "compute three steps",
		"phases": []interface{}{
			map[string]interface{}{"id": "phase-1", "status": "completed"},
			map[string]interface{}{"id": "phase-2", "status": "pending"},
		},
	}
	for _, query := range []string{"继续", "finish the rest", "what day is it?"} {
		parts := &chatRequestParts{Query: query}
		branch := []*runtimemodel.Message{{Query: "compute three steps", Status: runtimemodel.MessageStatusCompleted, Metadata: map[string]interface{}{"operation_plan": openPlan}}}
		message := buildContinuationTaskStateMessage(parts, branch)
		if message == nil || !strings.Contains(messageContentText(message.Content), "Open task goal: compute three steps") {
			t.Fatalf("buildContinuationTaskStateMessage(%q) = %#v, want the open task", query, message)
		}
		if currentTurnBoundaryMessage(parts) == nil {
			t.Fatalf("currentTurnBoundaryMessage(%q) = nil, want the boundary on every turn", query)
		}
		if !recentExecutionContextAppliesToCurrentRequest(parts) {
			t.Fatalf("recentExecutionContextAppliesToCurrentRequest(%q) = false", query)
		}
	}

	stopped := []*runtimemodel.Message{{Query: "write the report", Status: runtimemodel.MessageStatusStopped}}
	if message := buildContinuationTaskStateMessage(&chatRequestParts{Query: "继续"}, stopped); message == nil || !strings.Contains(messageContentText(message.Content), "write the report") {
		t.Fatalf("a stopped turn gave no open task state: %#v", message)
	}

	finished := []*runtimemodel.Message{
		{Query: "older open task", Status: runtimemodel.MessageStatusStopped},
		{Query: "a finished task", Status: runtimemodel.MessageStatusCompleted},
	}
	if message := buildContinuationTaskStateMessage(&chatRequestParts{Query: "继续"}, finished); message != nil {
		t.Fatalf("a finished previous turn gave open task state: %#v", message)
	}
}
