package parameterextractor

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
)

func TestGeneratePromptEngineeringChatPrompt_UsesTemplates(t *testing.T) {
	instruction := "Return exact values"
	nodeData := NodeData{
		Parameters: []ParameterConfig{
			{
				Name:        "destination",
				Type:        ParameterTypeString,
				Description: "Trip destination",
				Required:    true,
			},
		},
	}

	pg := NewPromptGenerator(nodeData, nil)
	messages, err := pg.GeneratePromptEngineeringChatPrompt("Book a flight to Paris", nil, instruction)
	if err != nil {
		t.Fatalf("GeneratePromptEngineeringChatPrompt returned error: %v", err)
	}

	if len(messages) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(messages))
	}

	expectedRoles := []string{"system", "user", "assistant", "user", "assistant", "user"}
	for i, role := range expectedRoles {
		if messages[i].Role != role {
			t.Fatalf("message %d role = %q, want %q", i, messages[i].Role, role)
		}
	}

	expectedContents := []string{
		"You should always follow the instructions and output a valid JSON object.\nThe structure of the JSON object you can find in the instructions.\nDo not include any explanations or additional text, only output the JSON object.",
		`Extract from: "I want to book a flight to San Francisco on January 15th"`,
		`{"destination": "San Francisco", "date": "2024-01-15"}`,
		`Extract from: "Set the temperature to 72 degrees and turn on the lights"`,
		`{"temperature": 72, "lights_on": true}`,
	}

	for i, expected := range expectedContents {
		content, ok := messages[i].Content.(string)
		if !ok {
			t.Fatalf("message %d content type = %T, want string", i, messages[i].Content)
		}
		if content != expected {
			t.Fatalf("message %d content = %q, want %q", i, content, expected)
		}
	}

	userContent, ok := messages[5].Content.(string)
	if !ok {
		t.Fatalf("message 5 content type = %T, want string", messages[5].Content)
	}

	expectedUserPrompt := "### Structure\n<structure>\n" + pg.generateParameterSchema() + "\n</structure>\n\n### Instructions\n" + instruction + "\n\n### Text to be converted to JSON\n<text>\nBook a flight to Paris\n</text>\n\nPlease extract the parameters according to the structure and output only the JSON object."
	if userContent != expectedUserPrompt {
		t.Fatalf("user prompt = %q, want %q", userContent, expectedUserPrompt)
	}
}

func TestGeneratePromptEngineeringChatPrompt_WithVisionPreservesMultipartContent(t *testing.T) {
	nodeData := NodeData{
		Vision: VisionConfig{
			Enabled: true,
			Configs: VisionConfigOptions{
				Detail: "high",
			},
		},
	}

	pg := NewPromptGenerator(nodeData, nil)
	testURL := "https://example.com/image.jpg"
	workflowFiles := []*file.File{
		{
			Type:           file.FileTypeImage,
			TransferMethod: file.FileTransferMethodRemoteURL,
			RemoteURL:      &testURL,
		},
	}

	messages, err := pg.GeneratePromptEngineeringChatPrompt("Describe this image", workflowFiles, "Focus on the main object")
	if err != nil {
		t.Fatalf("GeneratePromptEngineeringChatPrompt returned error: %v", err)
	}

	contentParts, ok := messages[len(messages)-1].Content.([]map[string]any)
	if !ok {
		t.Fatalf("last message content type = %T, want []map[string]any", messages[len(messages)-1].Content)
	}
	if len(contentParts) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(contentParts))
	}
	if contentParts[0]["type"] != "image_url" {
		t.Fatalf("first content part type = %v, want image_url", contentParts[0]["type"])
	}
	if contentParts[1]["type"] != "text" {
		t.Fatalf("second content part type = %v, want text", contentParts[1]["type"])
	}
}
