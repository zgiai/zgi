package handler

import (
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestMessageResponseRedactsContextCompactionSnapshot(t *testing.T) {
	message := &runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: uuid.New(),
		Query:          "q",
		Answer:         "a",
		Status:         runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"context_control": map[string]interface{}{
				"prompt_budget": 8000,
				"compaction": map[string]interface{}{
					"status":                "succeeded",
					"snapshot":              map[string]interface{}{"summary": "private-marker"},
					"snapshot_ref":          map[string]interface{}{"owner_message_id": uuid.NewString()},
					"history_tokens_before": 7000,
				},
			},
		},
	}

	resp := messageResponse(message)
	control, _ := resp.Metadata["context_control"].(map[string]interface{})
	compaction, _ := control["compaction"].(map[string]interface{})
	if len(compaction) != 1 || compaction["status"] != "succeeded" {
		t.Fatalf("client compaction metadata = %#v, want status only", compaction)
	}
}
