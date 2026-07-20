package approval

import (
	"context"
	"testing"

	"github.com/hibiken/asynq"
)

func TestQuestionResumeInputsRestoresWorkflowQueryAndChoice(t *testing.T) {
	inputs := questionResumeInputs(map[string]interface{}{
		"answer":    "continue",
		"choice_id": "approve",
	})
	if inputs["query"] != "continue" || inputs["sys.query"] != "continue" || inputs["question_answer_option_id"] != "approve" {
		t.Fatalf("resume inputs = %#v", inputs)
	}
}

func TestQuestionResumeTaskHandlerDispatchesPersistedInputs(t *testing.T) {
	task, err := NewQuestionResumeTask(QuestionResumeTaskPayload{
		WorkflowRunID: "run-1",
		Inputs:        map[string]interface{}{"query": "continue"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	handler := NewQuestionResumeTaskHandler(func(_ context.Context, workflowRunID string, inputs map[string]interface{}) error {
		called = true
		if workflowRunID != "run-1" || inputs["query"] != "continue" {
			t.Fatalf("callback payload run=%q inputs=%#v", workflowRunID, inputs)
		}
		return nil
	})
	if err := handler(context.Background(), asynq.NewTask(task.Type(), task.Payload())); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("question resume callback was not called")
	}
}
