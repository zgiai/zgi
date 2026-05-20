package workflow

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
)

func TestBuildChatMessagesResponseIncludesStatus(t *testing.T) {
	message := &conversation.AgentMessage{
		ID:             uuid.New(),
		ConversationID: uuid.New(),
		Query:          "hello",
		Answer:         "done",
		Status:         conversation.AgentMessageStatusCompleted,
		CreatedAt:      time.Unix(100, 0),
	}

	response := buildChatMessagesResponse([]*conversation.AgentMessage{message}, 1, 1, 20)
	items, ok := response["data"].([]map[string]interface{})
	if !ok {
		t.Fatalf("response data type = %T, want []map[string]interface{}", response["data"])
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if got := items[0]["status"]; got != conversation.AgentMessageStatusCompleted {
		t.Fatalf("status = %#v, want %s", got, conversation.AgentMessageStatusCompleted)
	}
}

func TestBuildChatMessagesResponseIncludesMessageMetadata(t *testing.T) {
	metadata := `{"foo":"bar"}`
	message := &conversation.AgentMessage{
		ID:              uuid.New(),
		ConversationID:  uuid.New(),
		Query:           "hello",
		Answer:          "done",
		Status:          conversation.AgentMessageStatusCompleted,
		MessageMetadata: &metadata,
		CreatedAt:       time.Unix(100, 0),
	}

	response := buildChatMessagesResponse([]*conversation.AgentMessage{message}, 1, 1, 20)
	items, ok := response["data"].([]map[string]interface{})
	if !ok {
		t.Fatalf("response data type = %T, want []map[string]interface{}", response["data"])
	}
	got, ok := items[0]["message_metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("message_metadata type = %T, want map", items[0]["message_metadata"])
	}
	if got["foo"] != "bar" {
		t.Fatalf("metadata foo = %#v, want bar", got["foo"])
	}
}
