package workflow

import (
	"testing"

	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
)

func TestBuildQuestionAnswerTranscriptFromEventsRestoresMultipleNodes(t *testing.T) {
	events := []workflowpause.RunEventPayload{
		{
			Event: workflowpause.EventQuestionAnswerRequested,
			Data: map[string]interface{}{
				"node_id":  "question-1",
				"question": "Which city?",
				"round":    float64(1),
			},
		},
		{
			Event: workflowpause.EventQuestionAnswerSubmitted,
			Data: map[string]interface{}{
				"node_id": "question-1",
				"answer":  "Zhengzhou",
				"round":   float64(1),
			},
		},
		{
			Event: workflowpause.EventQuestionAnswerRequested,
			Data: map[string]interface{}{
				"node_id":  "question-2",
				"question": "How is the weather today?",
				"round":    float64(1),
			},
		},
		{
			Event: workflowpause.EventQuestionAnswerSubmitted,
			Data: map[string]interface{}{
				"node_id":      "question-2",
				"answer":       "A",
				"choice_label": "Sunny",
				"round":        float64(1),
			},
		},
	}

	transcript := buildQuestionAnswerTranscriptFromEvents(events)
	if len(transcript) != 2 {
		t.Fatalf("transcript length = %d, want 2", len(transcript))
	}
	if got := transcript[0]["question"]; got != "Which city?" {
		t.Fatalf("first question = %#v, want Which city?", got)
	}
	if got := transcript[0]["answer"]; got != "Zhengzhou" {
		t.Fatalf("first answer = %#v, want Zhengzhou", got)
	}
	if got := transcript[1]["question"]; got != "How is the weather today?" {
		t.Fatalf("second question = %#v, want weather question", got)
	}
	if got := transcript[1]["answer"]; got != "Sunny" {
		t.Fatalf("second answer = %#v, want Sunny", got)
	}
}

func TestBuildQuestionAnswerTranscriptFromEventsUsesWorkflowPausedReasonAsFallback(t *testing.T) {
	events := []workflowpause.RunEventPayload{
		{
			Event: workflowpause.EventWorkflowPaused,
			Data: map[string]interface{}{
				"reasons": []interface{}{
					map[string]interface{}{
						"type":     workflowpause.ReasonTypeQuestionAnswerRequired,
						"node_id":  "question-2",
						"question": "Second question?",
						"round":    float64(1),
					},
				},
			},
		},
	}

	transcript := buildQuestionAnswerTranscriptFromEvents(events)
	if len(transcript) != 1 {
		t.Fatalf("transcript length = %d, want 1", len(transcript))
	}
	if got := transcript[0]["question"]; got != "Second question?" {
		t.Fatalf("question = %#v, want fallback question", got)
	}
	if got := transcript[0]["nodeId"]; got != "question-2" {
		t.Fatalf("nodeId = %#v, want question-2", got)
	}
}

func TestBuildQuestionAnswerTranscriptFromEventsDedupesPausedReasonPrompt(t *testing.T) {
	events := []workflowpause.RunEventPayload{
		{
			Event: workflowpause.EventQuestionAnswerRequested,
			Data: map[string]interface{}{
				"node_id":  "question-1",
				"question": "Which city?",
				"round":    float64(1),
			},
		},
		{
			Event: workflowpause.EventWorkflowPaused,
			Data: map[string]interface{}{
				"reasons": []interface{}{
					map[string]interface{}{
						"type":     workflowpause.ReasonTypeQuestionAnswerRequired,
						"node_id":  "question-1",
						"question": "Which city?",
					},
				},
			},
		},
	}

	transcript := buildQuestionAnswerTranscriptFromEvents(events)
	if len(transcript) != 1 {
		t.Fatalf("transcript length = %d, want 1", len(transcript))
	}
	if got := transcript[0]["question"]; got != "Which city?" {
		t.Fatalf("question = %#v, want Which city?", got)
	}
}

func TestBuildQuestionAnswerPromptFromEventsUsesLatestPausedQuestion(t *testing.T) {
	events := []workflowpause.RunEventPayload{
		{
			Event: workflowpause.EventQuestionAnswerRequested,
			Data: map[string]interface{}{
				"node_id":  "question-1",
				"question": "First?",
				"round":    float64(1),
				"choices": []interface{}{
					map[string]interface{}{"id": "A", "label": "A"},
				},
			},
		},
		{
			Event: workflowpause.EventQuestionAnswerSubmitted,
			Data: map[string]interface{}{
				"node_id": "question-1",
				"answer":  "A",
				"round":   float64(1),
			},
		},
		{
			Event: workflowpause.EventWorkflowPaused,
			Data: map[string]interface{}{
				"reasons": []interface{}{
					map[string]interface{}{
						"type":     workflowpause.ReasonTypeQuestionAnswerRequired,
						"node_id":  "question-2",
						"question": "Second?",
						"round":    float64(1),
						"choices": []interface{}{
							map[string]interface{}{"id": "B", "label": "B"},
						},
					},
				},
			},
		},
	}

	prompt := buildQuestionAnswerPromptFromEvents(events)
	if got := prompt["question"]; got != "Second?" {
		t.Fatalf("prompt question = %#v, want Second?", got)
	}
	choices, ok := prompt["choices"].([]interface{})
	if !ok || len(choices) != 1 {
		t.Fatalf("choices = %#v, want one choice", prompt["choices"])
	}
}
