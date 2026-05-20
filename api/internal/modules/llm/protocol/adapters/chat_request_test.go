package adapter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestChatRequestJSONDoesNotExposeProviderHint(t *testing.T) {
	request := ChatRequest{
		Provider: "deepseek",
		Model:    "deepseek-chat",
		Messages: []Message{
			{Role: "user", Content: "hello"},
		},
	}

	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	body := string(payload)
	if strings.Contains(body, `"provider"`) {
		t.Fatalf("json payload = %s, want provider hint omitted", body)
	}
	if !strings.Contains(body, `"model":"deepseek-chat"`) {
		t.Fatalf("json payload = %s, want model preserved", body)
	}
	if !strings.Contains(body, `"messages"`) {
		t.Fatalf("json payload = %s, want messages preserved", body)
	}
}
