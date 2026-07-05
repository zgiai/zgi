package skillloop

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestApplyStateHandoffAdvisoryToInformationToolMessage(t *testing.T) {
	invocation := &skills.ToolInvocationResult{
		Trace: skills.SkillTrace{
			Kind:     "tool_call",
			SkillID:  skills.SkillFileReader,
			ToolName: "read_file",
			Status:   "success",
		},
		ToolMessage: skills.ToolResultMessage("call-read", map[string]interface{}{
			"status":  "completed",
			"content": "short reusable text",
		}),
	}

	applyStateHandoffAdvisoryToToolMessage(invocation)

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(fmt.Sprint(invocation.ToolMessage.Content)), &payload); err != nil {
		t.Fatalf("unmarshal tool message: %v", err)
	}
	advisory, ok := payload["state_handoff"].(map[string]interface{})
	if !ok {
		t.Fatalf("state_handoff = %#v, want advisory payload", payload["state_handoff"])
	}
	if advisory["recommended"] != true {
		t.Fatalf("state_handoff.recommended = %#v, want true", advisory["recommended"])
	}
	if !strings.Contains(fmt.Sprint(advisory["reason"]), "implicit working memory") {
		t.Fatalf("state_handoff.reason = %#v, want memory-loss explanation", advisory["reason"])
	}
	if !strings.Contains(fmt.Sprint(advisory["example_call"]), skills.MetaToolTurnState) {
		t.Fatalf("state_handoff.example_call = %#v, want submit_turn_state example", advisory["example_call"])
	}
}

func TestApplyStateHandoffAdvisorySkipsMutationToolMessage(t *testing.T) {
	invocation := &skills.ToolInvocationResult{
		Trace: skills.SkillTrace{
			Kind:     "tool_call",
			SkillID:  skills.SkillAgentManagement,
			ToolName: "update_agent_config",
			Status:   "success",
		},
		ToolMessage: skills.ToolResultMessage("call-update", map[string]interface{}{
			"status": "completed",
		}),
	}

	applyStateHandoffAdvisoryToToolMessage(invocation)

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(fmt.Sprint(invocation.ToolMessage.Content)), &payload); err != nil {
		t.Fatalf("unmarshal tool message: %v", err)
	}
	if _, exists := payload["state_handoff"]; exists {
		t.Fatalf("state_handoff exists for mutation tool payload: %#v", payload)
	}
}
