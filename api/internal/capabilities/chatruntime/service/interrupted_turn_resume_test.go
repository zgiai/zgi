package service

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func interruptedTestMessage(invocations ...map[string]interface{}) *runtimemodel.Message {
	cause := staleActiveMessageError
	items := make([]interface{}, 0, len(invocations))
	for _, invocation := range invocations {
		items = append(items, invocation)
	}
	return &runtimemodel.Message{
		Status:   runtimemodel.MessageStatusError,
		Error:    &cause,
		Metadata: map[string]interface{}{"skill_invocations": items},
	}
}

func TestInterruptedTurnResumesOnlyWithoutAnUnfinishedChange(t *testing.T) {
	finished := map[string]interface{}{"kind": "tool_call", "status": "success", "skill_id": "file-generator", "tool_name": "generate_file"}
	unfinishedRead := map[string]interface{}{"kind": "tool_call", "status": "running", "skill_id": "file-reader", "tool_name": "read_file"}
	unfinishedWrite := map[string]interface{}{"kind": "tool_call", "status": "running", "skill_id": "file-generator", "tool_name": "generate_file"}
	unfinishedUndeclared := map[string]interface{}{"kind": "tool_call", "status": "running", "skill_id": "member-report", "tool_name": "write_report"}
	unfinishedPure := map[string]interface{}{"kind": "tool_call", "status": "running", "skill_id": "calculator", "tool_name": "evaluate_expression"}

	if err := interruptedTurnResumable(interruptedTestMessage(finished)); err != nil {
		t.Fatalf("stopped between calls: %v, want resumable", err)
	}
	if err := interruptedTurnResumable(interruptedTestMessage(finished, unfinishedPure)); err != nil {
		t.Fatalf("stopped during a calculation: %v, want resumable (the calculator declares no side effects)", err)
	}
	if err := interruptedTurnResumable(interruptedTestMessage(finished, unfinishedRead)); err != nil {
		t.Fatalf("stopped during a read: %v, want resumable", err)
	}
	runtimeRead := map[string]interface{}{"kind": "tool_call", "status": "running", "skill_id": "digital-human-memory", "tool_name": "memory_view", skillCallReadOnlyKey: true}
	if err := interruptedTurnResumable(interruptedTestMessage(finished, runtimeRead)); err != nil {
		t.Fatalf("stopped during a read-only runtime tool: %v, want resumable", err)
	}
	runtimeWrite := map[string]interface{}{"kind": "tool_call", "status": "running", "skill_id": "digital-human-memory", "tool_name": "memory_edit"}
	if err := interruptedTurnResumable(interruptedTestMessage(finished, runtimeWrite)); !errors.Is(err, ErrInterruptedTurnNotResumable) {
		t.Fatalf("stopped during a runtime tool not marked read-only: %v, want not resumable", err)
	}
	for name, message := range map[string]*runtimemodel.Message{
		"stopped during a change":                       interruptedTestMessage(unfinishedWrite),
		"stopped during a call with no declared effect": interruptedTestMessage(unfinishedUndeclared),
	} {
		if err := interruptedTurnResumable(message); !errors.Is(err, ErrInterruptedTurnNotResumable) {
			t.Fatalf("%s: %v, want not resumable", name, err)
		}
	}

	resumed := interruptedTestMessage(finished)
	resumed.Metadata[interruptedTurnResumeKey] = map[string]interface{}{"status": "completed"}
	failed := interruptedTestMessage(finished)
	other := "model_invocation_failed"
	failed.Error = &other
	completed := interruptedTestMessage(finished)
	completed.Status = runtimemodel.MessageStatusCompleted
	for name, message := range map[string]*runtimemodel.Message{"resumed before": resumed, "failed for another reason": failed, "completed": completed} {
		if err := interruptedTurnResumable(message); !errors.Is(err, ErrInterruptedTurnNotResumable) {
			t.Fatalf("%s: %v, want not resumable", name, err)
		}
	}
}

func TestInterruptedTurnIsLatestOnlyWhenNothingFollowedIt(t *testing.T) {
	parent, stopped, later := uuid.New(), uuid.New(), uuid.New()
	message := &runtimemodel.Message{ID: stopped, ParentID: &parent}
	for name, tt := range map[string]struct {
		leaf, active *uuid.UUID
		want         bool
	}{
		"first turn, nothing completed yet": {nil, nil, true},
		"leaf is the turn's parent":         {&parent, nil, true},
		"leaf is the turn itself":           {&stopped, &stopped, true},
		"a later turn completed":            {&later, nil, false},
		"a later turn is running":           {&parent, &later, false},
	} {
		conversation := &runtimemodel.Conversation{CurrentLeafMessageID: tt.leaf, ActiveMessageID: tt.active}
		if got := interruptedTurnIsLatest(conversation, message); got != tt.want {
			t.Fatalf("%s: interruptedTurnIsLatest() = %v, want %v", name, got, tt.want)
		}
	}
}
