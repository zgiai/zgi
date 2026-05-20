package parameterextractor

import (
	"encoding/json"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
)

// TestBuildUserMessageWithVision tests that multi-part content is correctly built for vision
func TestBuildUserMessageWithVision(t *testing.T) {
	// Create a prompt generator with vision enabled
	nodeData := NodeData{
		Vision: VisionConfig{
			Enabled: true,
			Configs: VisionConfigOptions{
				Detail: "high",
			},
		},
	}

	pg := NewPromptGenerator(nodeData, nil)

	// Create test files
	testURL := "https://example.com/image.jpg"
	files := []*file.File{
		{
			Type:           file.FileTypeImage,
			TransferMethod: file.FileTransferMethodRemoteURL,
			RemoteURL:      &testURL,
		},
	}

	// Build message with vision
	message := pg.buildUserMessageWithVision("Describe this image", files)

	// Verify the message has multi-part content
	if message.Role != "user" {
		t.Errorf("Expected role 'user', got '%s'", message.Role)
	}

	// Content should be []map[string]any for multi-part
	contentParts, ok := message.Content.([]map[string]any)
	if !ok {
		t.Fatalf("Expected content to be []map[string]any, got %T", message.Content)
	}

	// Should have 2 parts: image and text
	if len(contentParts) != 2 {
		t.Fatalf("Expected 2 content parts, got %d", len(contentParts))
	}

	// First part should be image_url
	if contentParts[0]["type"] != "image_url" {
		t.Errorf("Expected first part type 'image_url', got '%v'", contentParts[0]["type"])
	}

	imageURL, ok := contentParts[0]["image_url"].(map[string]any)
	if !ok {
		t.Fatalf("Expected image_url to be map[string]any, got %T", contentParts[0]["image_url"])
	}

	if imageURL["url"] != testURL {
		t.Errorf("Expected URL '%s', got '%v'", testURL, imageURL["url"])
	}

	if imageURL["detail"] != "high" {
		t.Errorf("Expected detail 'high', got '%v'", imageURL["detail"])
	}

	// Second part should be text
	if contentParts[1]["type"] != "text" {
		t.Errorf("Expected second part type 'text', got '%v'", contentParts[1]["type"])
	}

	if contentParts[1]["text"] != "Describe this image" {
		t.Errorf("Expected text 'Describe this image', got '%v'", contentParts[1]["text"])
	}
}

// TestBuildUserMessageWithoutVision tests that simple text content is used when vision is disabled
func TestBuildUserMessageWithoutVision(t *testing.T) {
	// Create a prompt generator with vision disabled
	nodeData := NodeData{
		Vision: VisionConfig{
			Enabled: false,
		},
	}

	pg := NewPromptGenerator(nodeData, nil)

	// Create test files (should be ignored)
	testURL := "https://example.com/image.jpg"
	files := []*file.File{
		{
			Type:           file.FileTypeImage,
			TransferMethod: file.FileTransferMethodRemoteURL,
			RemoteURL:      &testURL,
		},
	}

	// Build message without vision
	message := pg.buildUserMessageWithVision("Simple text query", files)

	// Verify the message has simple text content
	if message.Role != "user" {
		t.Errorf("Expected role 'user', got '%s'", message.Role)
	}

	// Content should be string for simple text
	contentStr, ok := message.Content.(string)
	if !ok {
		t.Fatalf("Expected content to be string, got %T", message.Content)
	}

	if contentStr != "Simple text query" {
		t.Errorf("Expected content 'Simple text query', got '%s'", contentStr)
	}
}

// TestGatewayInvokerHandlesMultiPartContent tests that the gateway invoker correctly handles multi-part content
func TestGatewayInvokerHandlesMultiPartContent(t *testing.T) {
	// Create a message with multi-part content
	multiPartContent := []map[string]any{
		{
			"type": "image_url",
			"image_url": map[string]any{
				"url":    "https://example.com/image.jpg",
				"detail": "high",
			},
		},
		{
			"type": "text",
			"text": "Describe this image",
		},
	}

	promptMsg := PromptMessage{
		Role:    "user",
		Content: multiPartContent,
	}

	// Create an invoke request
	req := &InvokeRequest{
		ModelSlug: "gpt-4-vision-preview",
		Messages:  []PromptMessage{promptMsg},
	}

	// Create a gateway invoker (without actual service)
	invoker := &gatewayLLMInvoker{}

	// Build chat request
	chatReq := invoker.buildChatRequest(req)

	// Verify the message content is preserved
	if len(chatReq.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(chatReq.Messages))
	}

	// Content should be JSON string of the multi-part content
	contentValue := chatReq.Messages[0].Content

	contentStr, ok := contentValue.(string)
	if !ok {
		t.Fatalf("Expected content to be string, got %T", contentValue)
	}

	// Parse the JSON string back to verify structure
	var contentParts []map[string]any
	err := json.Unmarshal([]byte(contentStr), &contentParts)
	if err != nil {
		t.Fatalf("Expected content to be valid JSON, got error: %v", err)
	}

	if len(contentParts) != 2 {
		t.Fatalf("Expected 2 content parts, got %d", len(contentParts))
	}

	// Verify the content can be marshaled to JSON (for sending to API)
	jsonBytes, err := json.Marshal(contentParts)
	if err != nil {
		t.Fatalf("Failed to marshal content to JSON: %v", err)
	}

	// Verify it's valid JSON
	var unmarshaled []map[string]any
	if err := json.Unmarshal(jsonBytes, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if len(unmarshaled) != 2 {
		t.Fatalf("Expected 2 parts after unmarshal, got %d", len(unmarshaled))
	}
}
