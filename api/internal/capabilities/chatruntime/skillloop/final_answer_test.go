package skillloop

import (
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestParseFinalAnswerSubmissionKeepsPlanWithoutEvidenceCoverageValidation(t *testing.T) {
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
	if submission.planWarning != "" {
		t.Fatalf("submission.planWarning = %q, want no evidence coverage warning", submission.planWarning)
	}
}

func TestParseFinalAnswerSubmissionAllowsNoPlanForOpenCurrentPlan(t *testing.T) {
	call := adapter.ToolCall{Function: adapter.FunctionCall{
		Name:      skills.MetaToolFinalAnswer,
		Arguments: `{"answer":"done"}`,
	}}
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{map[string]interface{}{
				"id":     "phase-update",
				"step":   "Update the Agent",
				"status": "pending",
			}},
		},
	}

	submission, err := parseFinalAnswerSubmission(call, evidence)
	if err != nil {
		t.Fatalf("parseFinalAnswerSubmission() error = %v", err)
	}
	if submission.answer != "done" || len(submission.plan) != 0 {
		t.Fatalf("submission = %#v, want answer accepted without final plan", submission)
	}
	if !strings.Contains(submission.planWarning, "missing_or_invalid_final_plan_snapshot") {
		t.Fatalf("submission.planWarning = %q, want missing final plan audit warning", submission.planWarning)
	}
}

func TestParseFinalAnswerSubmissionKeepsOpenPlanSnapshotAsAdvisoryMetadata(t *testing.T) {
	call := adapter.ToolCall{Function: adapter.FunctionCall{
		Name: skills.MetaToolFinalAnswer,
		Arguments: `{
			"answer":"done",
			"plan":[{"id":"phase-update","step":"Update the Agent","status":"in_progress"}]
		}`,
	}}

	submission, err := parseFinalAnswerSubmission(call, nil)
	if err != nil {
		t.Fatalf("parseFinalAnswerSubmission() error = %v", err)
	}
	if len(submission.plan) != 1 || submission.plan[0]["status"] != "in_progress" {
		t.Fatalf("submission.plan = %#v, want advisory in_progress snapshot", submission.plan)
	}
}

func TestParseFinalAnswerSubmissionIgnoresInvalidOptionalPlan(t *testing.T) {
	call := adapter.ToolCall{Function: adapter.FunctionCall{
		Name: skills.MetaToolFinalAnswer,
		Arguments: `{
			"answer":"done",
			"plan":[{"id":"phase-update","step":"Update the Agent","status":"unknown"}]
		}`,
	}}

	submission, err := parseFinalAnswerSubmission(call, nil)
	if err != nil {
		t.Fatalf("parseFinalAnswerSubmission() error = %v", err)
	}
	if submission.answer != "done" || len(submission.plan) != 0 {
		t.Fatalf("submission = %#v, want answer with ignored invalid plan", submission)
	}
	if !strings.Contains(submission.planWarning, "optional plan was ignored") {
		t.Fatalf("submission.planWarning = %q, want advisory parse warning", submission.planWarning)
	}
}

func TestParseFinalAnswerSubmissionRecoversCompleteAnswerFromMalformedOptionalMetadata(t *testing.T) {
	call := adapter.ToolCall{Function: adapter.FunctionCall{
		Name:      skills.MetaToolFinalAnswer,
		Arguments: `{"answer":"done","plan":[`,
	}}

	submission, err := parseFinalAnswerSubmission(call, nil)
	if err != nil {
		t.Fatalf("parseFinalAnswerSubmission() error = %v", err)
	}
	if submission.answer != "done" {
		t.Fatalf("submission.answer = %q, want recovered complete answer", submission.answer)
	}
	if !strings.Contains(submission.planWarning, "optional metadata was invalid") {
		t.Fatalf("submission.planWarning = %q, want malformed metadata warning", submission.planWarning)
	}
}

func TestParseFinalAnswerSubmissionRecoversAnswerAndWarnsWhenRequiredPlanIsMalformed(t *testing.T) {
	call := adapter.ToolCall{Function: adapter.FunctionCall{
		Name:      skills.MetaToolFinalAnswer,
		Arguments: `{"answer":"done","plan":[`,
	}}
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{map[string]interface{}{
				"id":     "phase-1",
				"step":   "Complete the task",
				"status": "in_progress",
			}},
		},
	}

	submission, err := parseFinalAnswerSubmission(call, evidence)
	if err != nil {
		t.Fatalf("parseFinalAnswerSubmission() error = %v", err)
	}
	if submission.answer != "done" {
		t.Fatalf("submission.answer = %q, want recovered complete answer", submission.answer)
	}
	if !strings.Contains(submission.planWarning, "missing_or_invalid_final_plan_snapshot") {
		t.Fatalf("submission.planWarning = %q, want required plan audit warning", submission.planWarning)
	}
}

func TestParseFinalAnswerSubmissionRejectsIncompleteAnswer(t *testing.T) {
	call := adapter.ToolCall{Function: adapter.FunctionCall{
		Name:      skills.MetaToolFinalAnswer,
		Arguments: `{"answer":"part`,
	}}

	_, err := parseFinalAnswerSubmission(call, nil)
	if err == nil {
		t.Fatal("parseFinalAnswerSubmission() error = nil, want incomplete answer error")
	}
}
