package workflow

import (
	"testing"

	graphentities "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
)

func TestRestoreQuestionAnswerResumeInputsRestoresPausedOutputs(t *testing.T) {
	pool := graphentities.NewVariablePool()
	state := &workflowpause.State{
		ExecutorState: workflowpause.ExecutorState{
			PausedNodeID:  "qa-node",
			PausedNodeIDs: []string{"qa-node"},
			ExecutionOutputs: map[string]map[string]interface{}{
				"qa-node": {
					"question": "Choose a route",
					"answers":  []interface{}{},
				},
			},
		},
	}

	restoreQuestionAnswerResumeInputs(pool, nil, map[string]interface{}{
		"question_answer_option_id": "reject",
	}, state)

	question := pool.Get([]string{"qa-node", "question"})
	if question == nil || question.Text() != "Choose a route" {
		t.Fatalf("question = %#v, want restored paused question", question)
	}
	if got, _ := pool.UserInputs["question_answer_option_id"].(string); got != "reject" {
		t.Fatalf("question_answer_option_id = %q, want reject", got)
	}
}

func TestRestoreQuestionAnswerPausedOutputsDoesNotOverwriteExistingVariables(t *testing.T) {
	pool := graphentities.NewVariablePool()
	pool.Add([]string{"qa-node", "question"}, "Existing question")
	state := &workflowpause.State{
		ExecutorState: workflowpause.ExecutorState{
			PausedNodeID: "qa-node",
			ExecutionOutputs: map[string]map[string]interface{}{
				"qa-node": {
					"question": "Paused question",
				},
			},
		},
	}

	restoreQuestionAnswerPausedOutputs(pool, state)

	question := pool.Get([]string{"qa-node", "question"})
	if question == nil || question.Text() != "Existing question" {
		t.Fatalf("question = %#v, want existing question", question)
	}
}
