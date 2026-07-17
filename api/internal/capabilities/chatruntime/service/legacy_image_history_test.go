package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	legacyconversation "github.com/zgiai/zgi/api/internal/modules/app/conversation"
)

func TestLegacyImageConversationToRuntimeMarksImageHistory(t *testing.T) {
	scope := Scope{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
	}
	legacy := &legacyconversation.AgentConversation{
		ID:            uuid.New(),
		AgentID:       legacyImageWorkflowAgentID(),
		Name:          "legacy image",
		DialogueCount: 2,
		CreatedAt:     time.Now().Add(-2 * time.Hour),
		UpdatedAt:     time.Now().Add(-time.Hour),
	}

	conversation := legacyImageConversationToRuntime(scope, legacy)

	if conversation.ID != legacy.ID || conversation.SourceConversationID == nil || *conversation.SourceConversationID != legacy.ID {
		t.Fatalf("conversation source = id:%s source:%v, want legacy id", conversation.ID, conversation.SourceConversationID)
	}
	if conversation.ConversationType != runtimemodel.ConversationTypeImage || conversation.Title != "legacy image" {
		t.Fatalf("conversation = %#v, want image legacy title", conversation)
	}
	if conversation.Metadata["legacy_workflow_image"] != true {
		t.Fatalf("metadata = %#v, want legacy marker", conversation.Metadata)
	}
}

func TestLegacyImageMessageToRuntimeBuildsImageGenerationMetadata(t *testing.T) {
	metadataBytes, err := json.Marshal(map[string]interface{}{
		"generated_files": []interface{}{map[string]interface{}{
			"file_id":         "tool-image",
			"tool_file_id":    "tool-image",
			"filename":        "image.png",
			"extension":       ".png",
			"mime_type":       "image/png",
			"transfer_method": "tool_file",
		}},
	})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	metadata := string(metadataBytes)
	provider := "qwen"
	modelName := "qwen-image"
	legacy := &legacyconversation.AgentMessage{
		ID:              uuid.New(),
		ConversationID:  uuid.New(),
		Query:           "draw a flower",
		Status:          legacyconversation.AgentMessageStatusCompleted,
		ModelProvider:   &provider,
		ModelVersionID:  &modelName,
		MessageMetadata: &metadata,
		CreatedAt:       time.Now().Add(-time.Hour),
		UpdatedAt:       time.Now().Add(-time.Hour),
	}

	message := legacyImageMessageToRuntime(legacy)

	imageGeneration, ok := message.Metadata["image_generation"].(map[string]interface{})
	if !ok {
		t.Fatalf("image_generation = %#v, want object", message.Metadata["image_generation"])
	}
	files := generatedFilesFromMetadata(imageGeneration["files"])
	if len(files) != 1 || files[0]["file_id"] != "tool-image" {
		t.Fatalf("image_generation.files = %#v, want tool-image", imageGeneration["files"])
	}
	if imageGeneration["provider"] != "qwen" || imageGeneration["model"] != "qwen-image" || imageGeneration["prompt"] != "draw a flower" {
		t.Fatalf("image_generation = %#v, want legacy model and prompt", imageGeneration)
	}
	if message.Status != runtimemodel.MessageStatusCompleted || message.ModelName != "qwen-image" {
		t.Fatalf("message status/model = %q/%q, want completed/qwen-image", message.Status, message.ModelName)
	}
}
