package workflow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	graphentities "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

func TestRestoreQuestionAnswerResumeInputsKeepsAnswersScopedToEachNode(t *testing.T) {
	pool := graphentities.NewVariablePool()
	restoreQuestionAnswerResumeInputs(pool, nil, map[string]interface{}{
		"query":     "second",
		"sys.query": "second",
		"interaction_submissions": []interface{}{
			map[string]interface{}{
				"reason_type": workflowpause.ReasonTypeQuestionAnswerRequired,
				"node_id":     "question-1",
				"data": map[string]interface{}{
					"node_id": "question-1",
					"answer":  "first",
				},
			},
			map[string]interface{}{
				"reason_type": workflowpause.ReasonTypeQuestionAnswerRequired,
				"node_id":     "question-2",
				"data": map[string]interface{}{
					"node_id":   "question-2",
					"answer":    "second",
					"choice_id": "second-choice",
				},
			},
		},
	}, nil)

	first := pool.Get([]string{"question-1", "query"})
	if first == nil || first.Text() != "first" {
		t.Fatalf("question-1 query = %#v, want first", first)
	}
	second := pool.Get([]string{"question-2", "query"})
	if second == nil || second.Text() != "second" {
		t.Fatalf("question-2 query = %#v, want second", second)
	}
	secondChoice := pool.Get([]string{"question-2", "question_answer_option_id"})
	if secondChoice == nil || secondChoice.Text() != "second-choice" {
		t.Fatalf("question-2 choice = %#v, want second-choice", secondChoice)
	}
}

func TestDurableWorkflowResumeInputsReachEveryQuestionInDirectResume(t *testing.T) {
	for _, interactionType := range []string{
		workflowpause.ReasonTypeQuestionAnswerRequired,
		workflowpause.ReasonTypeApprovalRequired,
	} {
		t.Run(interactionType, func(t *testing.T) {
			dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
			db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
			if err != nil {
				if strings.Contains(err.Error(), "requires cgo") {
					t.Skipf("sqlite driver unavailable without cgo: %v", err)
				}
				t.Fatalf("open sqlite: %v", err)
			}
			if err := db.AutoMigrate(&workflowpause.RuntimeOutbox{}); err != nil {
				t.Fatalf("migrate workflow outbox: %v", err)
			}

			pauseID := "pause-direct-resume"
			payload := workflowpause.RuntimeOutboxPayload{
				WorkflowRunID:   "run-direct-resume",
				PauseID:         pauseID,
				Generation:      2,
				InteractionType: interactionType,
				ResumeInputs: map[string]interface{}{
					"query": "second",
					"interaction_submissions": []interface{}{
						map[string]interface{}{
							"reason_type": workflowpause.ReasonTypeQuestionAnswerRequired,
							"node_id":     "question-1",
							"data": map[string]interface{}{
								"node_id": "question-1",
								"answer":  "first",
							},
						},
						map[string]interface{}{
							"reason_type": workflowpause.ReasonTypeQuestionAnswerRequired,
							"node_id":     "question-2",
							"data": map[string]interface{}{
								"node_id": "question-2",
								"answer":  "second",
							},
						},
					},
				},
			}
			payloadJSON, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal workflow resume payload: %v", err)
			}
			now := time.Now()
			outbox := workflowpause.RuntimeOutbox{
				ID:             "outbox-direct-resume",
				TenantID:       "tenant-direct-resume",
				WorkflowRunID:  payload.WorkflowRunID,
				PauseID:        &pauseID,
				Kind:           workflowpause.RuntimeOutboxKindResume,
				IdempotencyKey: "workflow-resume:" + pauseID + ":2",
				PayloadJSON:    string(payloadJSON),
				Status:         workflowpause.RuntimeOutboxPending,
				NextAttemptAt:  now,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := db.Create(&outbox).Error; err != nil {
				t.Fatalf("create workflow resume outbox: %v", err)
			}

			inputs, err := loadDurableWorkflowResumeInputs(
				context.Background(),
				workflowpause.NewService(db),
				payload.WorkflowRunID,
				&workflowpause.RunPause{ID: pauseID, Generation: payload.Generation},
				map[string]interface{}{"query": "current-request-only"},
			)
			if err != nil {
				t.Fatalf("load durable workflow resume inputs: %v", err)
			}

			pool := graphentities.NewVariablePool()
			restoreQuestionAnswerResumeInputs(pool, nil, inputs, nil)
			first := pool.Get([]string{"question-1", "query"})
			if first == nil || first.Text() != "first" {
				t.Fatalf("question-1 query = %#v, want first", first)
			}
			second := pool.Get([]string{"question-2", "query"})
			if second == nil || second.Text() != "second" {
				t.Fatalf("question-2 query = %#v, want second", second)
			}
		})
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

func TestQuestionAnswerStateConversationIDIgnoresBusinessConversationID(t *testing.T) {
	state := workflowpause.State{
		Request: workflowpause.RequestState{
			Inputs: map[string]interface{}{
				"conversation_id": "business-conversation-value",
			},
		},
	}

	got := questionAnswerStateConversationID(state)
	if got != "" {
		t.Fatalf("question answer state conversation id = %q, want empty for business input", got)
	}
}

func TestQuestionAnswerStateConversationIDUsesSystemConversationID(t *testing.T) {
	state := workflowpause.State{
		Request: workflowpause.RequestState{
			Inputs: map[string]interface{}{
				"conversation_id":     "business-conversation-value",
				"sys.conversation_id": "system-conversation-id",
			},
		},
	}

	got := questionAnswerStateConversationID(state)
	if got != "system-conversation-id" {
		t.Fatalf("question answer state conversation id = %q, want %q", got, "system-conversation-id")
	}
}

func TestQuestionAnswerStateConversationIDPrefersVariablePoolSystemConversationID(t *testing.T) {
	state := workflowpause.State{
		VariablePool: workflowpause.VariablePoolSnapshot{
			SystemVariables: &graphentities.SystemVariable{
				ConversationID: "pool-conversation-id",
			},
		},
		Request: workflowpause.RequestState{
			Inputs: map[string]interface{}{
				"sys.conversation_id": "request-system-conversation-id",
			},
		},
	}

	got := questionAnswerStateConversationID(state)
	if got != "pool-conversation-id" {
		t.Fatalf("question answer state conversation id = %q, want %q", got, "pool-conversation-id")
	}
}

func TestQuestionAnswerSubmissionIdempotencyKeyDistinguishesQuestionsAndRounds(t *testing.T) {
	state := &workflowpause.State{
		ExecutorState: workflowpause.ExecutorState{
			PausedNodeID: "question-1",
			ExecutionOutputs: map[string]map[string]interface{}{
				"question-1": {"answers": []interface{}{}},
				"question-2": {"answers": []interface{}{"first"}},
			},
		},
	}

	first := questionAnswerSubmissionIdempotencyKey("pause-1", 2, state, map[string]interface{}{
		"question_answer_node_id": "question-1",
	})
	replay := questionAnswerSubmissionIdempotencyKey("pause-1", 2, state, map[string]interface{}{
		"question_answer_node_id": "question-1",
	})
	secondQuestion := questionAnswerSubmissionIdempotencyKey("pause-1", 2, state, map[string]interface{}{
		"question_answer_node_id": "question-2",
	})
	secondRound := questionAnswerSubmissionIdempotencyKey("pause-1", 2, state, map[string]interface{}{
		"question_answer_node_id": "question-1",
		"question_answer_round":   2,
	})
	nextGeneration := questionAnswerSubmissionIdempotencyKey("pause-1", 3, state, map[string]interface{}{
		"question_answer_node_id": "question-1",
	})

	if first != replay {
		t.Fatalf("same question submission keys differ: %q != %q", first, replay)
	}
	for name, key := range map[string]string{
		"second question": secondQuestion,
		"second round":    secondRound,
		"next generation": nextGeneration,
	} {
		if key == first {
			t.Fatalf("%s reused first question idempotency key %q", name, key)
		}
	}
}
