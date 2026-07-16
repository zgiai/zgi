package skillloop

import (
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestApplyPageContextInvalidationAdvisoryForMutation(t *testing.T) {
	invocation := &skills.ToolInvocationResult{
		Trace:       skills.SkillTrace{SkillID: skills.SkillAgentManagement, ToolName: "delete_agent", Status: "success"},
		ToolMessage: skills.ToolResultMessage("call-1", map[string]interface{}{"status": "completed"}),
	}
	applyPageContextInvalidationAdvisory(invocation)
	payload := toolMessageJSONPayload(invocation.ToolMessage)
	if _, ok := payload["page_context_invalidation"].(map[string]interface{}); !ok {
		t.Fatalf("tool payload = %#v, want page_context_invalidation", payload)
	}

	readOnly := &skills.ToolInvocationResult{
		Trace:       skills.SkillTrace{SkillID: skills.SkillAgentManagement, ToolName: "get_agent_config", Status: "success"},
		ToolMessage: adapter.Message{Role: "tool", ToolCallID: "call-2", Content: `{"status":"completed"}`},
	}
	applyPageContextInvalidationAdvisory(readOnly)
	if _, ok := toolMessageJSONPayload(readOnly.ToolMessage)["page_context_invalidation"]; ok {
		t.Fatal("read-only tool unexpectedly invalidated page context")
	}
}
