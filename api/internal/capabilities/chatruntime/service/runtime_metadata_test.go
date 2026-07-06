package service

import (
	"encoding/json"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestModelInvocationRequestPayloadRedactsInlineImageDataURLParts(t *testing.T) {
	imageBody := strings.Repeat("A", 128)
	dataURL := "data:image/jpeg;base64," + imageBody
	req := &adapter.ChatRequest{
		Model: "vision-model",
		Messages: []adapter.Message{{
			Role: "user",
			Content: []adapter.MessageContentPart{
				{
					Type: "image_url",
					ImageURL: &adapter.ImageURL{
						URL:    dataURL,
						Detail: "high",
					},
				},
				{Type: "text", Text: "describe this image"},
			},
		}},
	}

	payload := modelInvocationRequestPayload(req, false)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if strings.Contains(string(encoded), imageBody) {
		t.Fatalf("payload contains raw inline image data: %s", string(encoded))
	}

	messages := payload["messages"].([]interface{})
	message := messages[0].(map[string]interface{})
	content := message["content"].([]interface{})
	imagePart := content[0].(map[string]interface{})
	imageURL := imagePart["image_url"].(map[string]interface{})
	if got := stringFromAny(imageURL["url"]); got != "data:image/jpeg;base64,<redacted>" {
		t.Fatalf("image url = %q, want redacted data URL", got)
	}
	if imageURL["url_redacted"] != true {
		t.Fatalf("image url summary = %#v, want url_redacted", imageURL)
	}
	if imageURL["url_mime_type"] != "image/jpeg" {
		t.Fatalf("image url mime = %#v, want image/jpeg", imageURL["url_mime_type"])
	}
	if got := intValueFromAny(imageURL["url_base64_chars"]); got != len(imageBody) {
		t.Fatalf("image url base64 chars = %d, want %d", got, len(imageBody))
	}
}

func TestModelInvocationRequestPayloadRedactsEmbeddedImageDataURLText(t *testing.T) {
	imageBody := strings.Repeat("B", 96)
	req := &adapter.ChatRequest{
		Model: "vision-model",
		Messages: []adapter.Message{{
			Role:    "user",
			Content: "please inspect ![chart](data:image/png;base64," + imageBody + ")",
		}},
	}

	payload := modelInvocationRequestPayload(req, false)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if strings.Contains(string(encoded), imageBody) {
		t.Fatalf("payload contains raw embedded image data: %s", string(encoded))
	}

	messages := payload["messages"].([]interface{})
	message := messages[0].(map[string]interface{})
	content := stringFromAny(message["content"])
	if !strings.Contains(content, "data:image/png;base64,<redacted>") {
		t.Fatalf("content = %q, want embedded data URL redacted", content)
	}
	if message["content_redacted"] != true {
		t.Fatalf("message = %#v, want content_redacted marker for embedded image data", message)
	}
	if got := intValueFromAny(message["content_base64_chars"]); got != len(imageBody) {
		t.Fatalf("content base64 chars = %d, want %d", got, len(imageBody))
	}
}
