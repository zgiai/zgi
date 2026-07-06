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
