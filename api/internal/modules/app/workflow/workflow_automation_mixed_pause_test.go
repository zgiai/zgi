package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	workflowshared "github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type automationPauseRunLog struct {
	ID                      string `gorm:"type:uuid;primaryKey"`
	TenantID                string
	AgentID                 string
	WorkflowID              string
	RuntimeProtocolVersion  int
	NextEventSequence       int64
	ExecutionGeneration     int64
	ActiveExecutionID       *string
	ExecutionLeaseExpiresAt *time.Time
	StateRevision           int64
	Status                  string
	Error                   *string
	Outputs                 *string
	ExceptionsCount         int
	ElapsedTime             float64
	TotalSteps              int
	TotalTokens             int
	FinishedAt              *time.Time
	ConversationID          *string
	DeletedAt               *time.Time
}

func (automationPauseRunLog) TableName() string { return "workflow_run_logs" }

// mixedAutomationPause is an approval node and a question node that paused
// together; the approval started first, so it is the pause's first node.
func mixedAutomationPause() *WorkflowExecutionResult {
	start := time.Unix(100, 0)
	return &WorkflowExecutionResult{NodeExecutions: []graph_engine.NodeExecutionSnapshot{
		{
			NodeID: "approval_node", NodeType: workflowshared.Approval, Status: workflowshared.PAUSED, StartTime: start,
			Outputs: map[string]interface{}{
				"__approval_form_id": "form-1",
				"__approval_token":   "token-1",
				"__approval_form":    map[string]interface{}{"id": "form-1", "content": "Approve the purchase"},
			},
		},
		{
			NodeID: "qa_node", NodeType: workflowshared.QuestionAnswer, Status: workflowshared.PAUSED, StartTime: start.Add(time.Second),
			Outputs: map[string]interface{}{"question": "Which department owns this request?"},
		},
	}}
}

func TestAutomationMixedPauseRequestsEveryPausedNode(t *testing.T) {
	result := mixedAutomationPause()
	outputs := map[string]interface{}{}
	mergeAutomationApprovalOutputs(outputs, result.NodeExecutions)
	events := collectAutomationWorkflowPausedEvents(automationaction.WorkflowRunRequest{WorkspaceID: "tenant-1"}, nil, "run-1", result, outputs, 1, nil)

	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	if strings.Join(types, ",") != "workflow_paused,approval_requested,question_answer_requested" {
		t.Fatalf("events = %v, want the pause then a request for each paused node, the question last", types)
	}
	paused, approval, question := events[0].Payload, events[1].Payload, events[2].Payload
	if paused["status"] != "pending_question" {
		t.Fatalf("workflow_paused status = %v, want pending_question like the tool result", paused["status"])
	}
	if reasons, _ := paused["reasons"].([]interface{}); len(reasons) != 2 {
		t.Fatalf("workflow_paused reasons = %#v, want both paused nodes", paused["reasons"])
	}
	if approval["node_id"] != "approval_node" || approval["form_id"] != "form-1" || approval["approval_token"] != "token-1" {
		t.Fatalf("approval_requested = %#v, want the approval node with its form", approval)
	}
	if question["node_id"] != "qa_node" || question["question"] == nil {
		t.Fatalf("question_answer_requested = %#v, want the question node", question)
	}
}

// The saved pause must let an answered question project the approval that is
// still pending, which needs each reason matched to its own request event.
func TestAutomationMixedPauseProjectsTheRemainingApprovalAfterTheAnswer(t *testing.T) {
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skipf("sqlite driver unavailable without cgo: %v", err)
		}
		t.Fatalf("open sqlite: %v", err)
	}
	if err := autoMigrateWorkflowTest(db, &automationPauseRunLog{}, &workflowpause.RunPause{}, &workflowpause.RunPauseReason{},
		&workflowpause.RunEvent{}, &workflowpause.RuntimeOutbox{}); err != nil {
		t.Fatalf("migrate automation pause tables: %v", err)
	}
	executionID := "00000000-0000-0000-0000-000000000021"
	if err := db.Create(&automationPauseRunLog{ID: "run-1", RuntimeProtocolVersion: 2, ExecutionGeneration: 1, ActiveExecutionID: &executionID, Status: "running"}).Error; err != nil {
		t.Fatal(err)
	}

	result := mixedAutomationPause()
	outputs := map[string]interface{}{}
	mergeAutomationApprovalOutputs(outputs, result.NodeExecutions)
	req := automationaction.WorkflowRunRequest{WorkspaceID: "tenant-1", WorkflowRef: automationaction.WorkflowRef{AgentID: "agent-1"}}
	events := collectAutomationWorkflowPausedEvents(req, nil, "run-1", result, outputs, 1, nil)
	params, ok := automationWorkflowPauseSaveParams(req, nil, "run-1", result, map[string]interface{}{}, outputs, 3, 1, events,
		workflowExecutionOwner{WorkflowRunID: "run-1", ExecutionID: executionID, Generation: 1})
	if !ok {
		t.Fatal("no pause to save")
	}
	for _, reason := range params.Reasons {
		if reason.Type == workflowpause.ReasonTypeQuestionAnswerRequired && reason.FormID != "" {
			t.Fatalf("question reason form = %q, want none", reason.FormID)
		}
		if reason.Type == workflowpause.ReasonTypeApprovalRequired && reason.FormID != "form-1" {
			t.Fatalf("approval reason form = %q, want form-1", reason.FormID)
		}
	}
	service := workflowpause.NewService(db)
	pause, err := service.Save(context.Background(), params)
	if err != nil {
		t.Fatalf("save automation pause: %v", err)
	}

	answer, err := service.SubmitInteraction(context.Background(), "run-1", pause.ID, "question-answer", workflowpause.EventQuestionAnswerSubmitted,
		map[string]interface{}{"node_id": "qa_node", "answer": "Finance"}, "question-answer:run-1")
	if err != nil {
		t.Fatalf("answer the question of the mixed pause: %v", err)
	}
	if answer.ResumeReady {
		t.Fatal("the pause resumed while its approval was still pending")
	}
	projected := false
	for _, event := range answer.PendingEvents {
		if event.Event == workflowpause.EventApprovalRequested && event.Data["form_id"] == "form-1" {
			projected = true
		}
	}
	if !projected {
		t.Fatalf("pending events = %#v, want the pending approval projected", answer.PendingEvents)
	}
}

func TestQuestionAnswerWithoutANodeAnswersTheOnlyOpenQuestion(t *testing.T) {
	reasons := []workflowpause.RunPauseReason{
		{Type: workflowpause.ReasonTypeApprovalRequired, NodeID: "approval_node", FormID: "form-1", Status: workflowpause.RunPauseReasonStatusPending},
		{Type: workflowpause.ReasonTypeQuestionAnswerRequired, NodeID: "qa_node", Status: workflowpause.RunPauseReasonStatusPending},
	}
	if got := pendingQuestionReasonNodeID(reasons); got != "qa_node" {
		t.Fatalf("pending question node = %q, want qa_node", got)
	}
	reasons = append(reasons, workflowpause.RunPauseReason{Type: workflowpause.ReasonTypeQuestionAnswerRequired, NodeID: "qa_other", Status: workflowpause.RunPauseReasonStatusPending})
	if got := pendingQuestionReasonNodeID(reasons); got != "" {
		t.Fatalf("pending question node = %q, want none when several questions are open", got)
	}
}
