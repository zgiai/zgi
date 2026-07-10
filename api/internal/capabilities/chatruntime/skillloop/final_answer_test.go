package skillloop

import (
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestParseFinalAnswerSubmissionKeepsPlanWithAdvisoryEvidenceWarning(t *testing.T) {
	call := adapter.ToolCall{Function: adapter.FunctionCall{
		Name: skills.MetaToolFinalAnswer,
		Arguments: `{
			"answer":"已完成。",
			"plan":[{
				"id":"phase-1",
				"step":"更新智能体",
				"status":"completed",
				"evidence_refs":["tool:agent-management/update_agent_config"]
			}]
		}`,
	}}

	submission, err := parseFinalAnswerSubmission(call, map[string]interface{}{})
	if err != nil {
		t.Fatalf("parseFinalAnswerSubmission() error = %v", err)
	}
	if len(submission.plan) != 1 {
		t.Fatalf("submission.plan = %#v, want advisory snapshot preserved", submission.plan)
	}
	if !strings.Contains(submission.planWarning, "unresolved_evidence_ref") {
		t.Fatalf("submission.planWarning = %q, want unresolved ref audit warning", submission.planWarning)
	}
}
