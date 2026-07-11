package handler

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestRegisterRoutesDoesNotConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/console/api")

	NewHandler(nil).RegisterRoutes(group)
}

func TestMessageResponseRedactsModelInvocationMetadata(t *testing.T) {
	message := &runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: uuid.New(),
		Query:          "q",
		Answer:         "a",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "vision-model",
		Metadata: map[string]interface{}{
			"model_invocations": []interface{}{
				map[string]interface{}{
					"request": map[string]interface{}{
						"messages": []interface{}{
							"data:image/jpeg;base64,raw-body",
						},
					},
				},
			},
			"generated_files": []interface{}{
				map[string]interface{}{"file_id": "file-1"},
			},
		},
	}

	resp := messageResponse(message)
	if _, ok := resp.Metadata["model_invocations"]; ok {
		t.Fatalf("metadata = %#v, should not expose model_invocations", resp.Metadata)
	}
	if resp.Metadata["model_invocations_redacted"] != true {
		t.Fatalf("metadata = %#v, want model_invocations_redacted marker", resp.Metadata)
	}
	if resp.Metadata["model_invocation_count"] != 1 {
		t.Fatalf("metadata = %#v, want model_invocation_count=1", resp.Metadata)
	}
	if _, ok := resp.Metadata["generated_files"]; !ok {
		t.Fatalf("metadata = %#v, should preserve lightweight message metadata", resp.Metadata)
	}
}

func TestMessageResponseFiltersFinalAnswerInvocationMetadata(t *testing.T) {
	message := &runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: uuid.New(),
		Query:          "q",
		Answer:         "a",
		Status:         runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{
				map[string]interface{}{"kind": "final_answer", "tool_name": "submit_final_answer"},
				map[string]interface{}{"kind": "tool_call", "skill_id": "file-reader", "tool_name": "read_file"},
			},
		},
	}

	resp := messageResponse(message)
	invocations, _ := resp.Metadata["skill_invocations"].([]interface{})
	if len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want only one user-visible invocation", resp.Metadata["skill_invocations"])
	}
	invocation, _ := invocations[0].(map[string]interface{})
	if invocation["kind"] != "tool_call" {
		t.Fatalf("skill_invocations = %#v, want final_answer filtered", invocations)
	}
}
