package service

import (
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestShouldSuppressDigitalHumanTaskGovernanceEvent(t *testing.T) {
	tests := []struct {
		name    string
		caller  Caller
		event   string
		payload map[string]interface{}
		want    bool
	}{
		{
			name:   "digital human task",
			caller: Caller{Type: runtimemodel.ConversationCallerDigitalHuman},
			event:  streamEventToolGovernanceDecision,
			payload: map[string]interface{}{
				"kind":            "digital_human_task",
				"task_id":         "task-1",
				"task_action":     "pause",
				"workflow_run_id": "digital-human-task:task-1:run-1",
			},
			want: true,
		},
		{
			name:   "digital human frozen task",
			caller: Caller{Type: runtimemodel.ConversationCallerDigitalHuman},
			event:  streamEventToolGovernanceDecision,
			payload: map[string]interface{}{
				"governance": map[string]interface{}{
					"frozen_invocation": map[string]interface{}{
						"skill_id":  "digital-human-tasks",
						"tool_name": "manage_digital_human_task",
						"arguments": map[string]interface{}{"task_id": "task-1"},
					},
				},
			},
			want: true,
		},
		{
			name:   "ordinary tool remains public",
			caller: Caller{Type: runtimemodel.ConversationCallerDigitalHuman},
			event:  streamEventToolGovernanceDecision,
			payload: map[string]interface{}{
				"skill_id":  "file-manager",
				"tool_name": "delete_file",
			},
			want: false,
		},
		{
			name:   "agent task-shaped event remains public",
			caller: Caller{Type: runtimemodel.ConversationCallerAgent},
			event:  streamEventToolGovernanceDecision,
			payload: map[string]interface{}{
				"kind":    "digital_human_task",
				"task_id": "task-1",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSuppressDigitalHumanTaskGovernanceEvent(tt.caller, tt.event, tt.payload); got != tt.want {
				t.Fatalf("shouldSuppressDigitalHumanTaskGovernanceEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}
