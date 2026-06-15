package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	actionmodel "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/model"
	actionservice "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/service"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestConsoleFilesPostprocessRequestIncludesReadOutputAndTranslateInstruction(t *testing.T) {
	run := &actionservice.ActionRunView{
		Run: &actionmodel.ActionRun{ID: uuid.New()},
		Steps: []*actionmodel.ActionStep{{
			ID:        uuid.New(),
			RunID:     uuid.New(),
			StepKey:   "execute",
			Status:    actionmodel.ActionStepStatusDone,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Output: map[string]interface{}{"files": []map[string]interface{}{
				{
					"id":              "file-4",
					"name":            "2501.00011v1.pdf",
					"content_preview": "The invoice total is 123 yuan.",
					"content_status":  "ready",
				},
			}},
		}},
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New()},
		parts: &chatRequestParts{
			Query:     "帮我读一下第四个文件的内容，翻译一下",
			Provider:  "test-provider",
			ModelName: "test-model",
		},
	}
	decision := AIChatActionDecision{
		Postprocess: []AIChatActionPostprocess{{Type: "translate", TargetLanguage: "zh-CN"}},
	}

	req, err := newConsoleFilesPostprocessLLMRequest(prepared, run, decision, "fallback")
	if err != nil {
		t.Fatalf("newConsoleFilesPostprocessLLMRequest: %v", err)
	}
	if req.Stream {
		t.Fatalf("Stream = true, want false")
	}
	if req.Model != "test-model" || req.Provider != "test-provider" {
		t.Fatalf("model/provider = %q/%q, want test-model/test-provider", req.Model, req.Provider)
	}
	if req.MaxTokens == nil || *req.MaxTokens != consoleFilesPostprocessMaxTokens {
		t.Fatalf("MaxTokens = %#v, want %d", req.MaxTokens, consoleFilesPostprocessMaxTokens)
	}
	payload := strings.TrimSpace(messageContentText(req.Messages[1].Content))
	for _, want := range []string{"translate", "zh-CN", "2501.00011v1.pdf", "The invoice total is 123 yuan."} {
		if !strings.Contains(payload, want) {
			t.Fatalf("payload missing %q: %s", want, payload)
		}
	}
}

func TestConsoleFilesPostprocessRequestMetadataRedactsReadOutput(t *testing.T) {
	const rawFileContent = "POSTPROCESS_SECRET_SHOULD_NOT_PERSIST"
	run := &actionservice.ActionRunView{
		Run: &actionmodel.ActionRun{ID: uuid.New()},
		Steps: []*actionmodel.ActionStep{{
			ID:        uuid.New(),
			RunID:     uuid.New(),
			StepKey:   "execute",
			Status:    actionmodel.ActionStepStatusDone,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Output: map[string]interface{}{"files": []map[string]interface{}{
				{
					"id":                "file-4",
					"name":              "2501.00011v1.pdf",
					"content_preview":   rawFileContent,
					"content_status":    "extracted",
					"content_truncated": false,
				},
			}},
		}},
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New()},
		parts: &chatRequestParts{
			Query:     "summarize the fourth file",
			Provider:  "test-provider",
			ModelName: "test-model",
		},
	}
	decision := AIChatActionDecision{
		Postprocess: []AIChatActionPostprocess{{Type: "summarize"}},
	}

	req, err := newConsoleFilesPostprocessLLMRequest(prepared, run, decision, "I read 2501.00011v1.pdf:\n"+rawFileContent)
	if err != nil {
		t.Fatalf("newConsoleFilesPostprocessLLMRequest: %v", err)
	}
	requestPayload := modelInvocationRequestPayload(req, true)
	encoded, err := json.Marshal(requestPayload)
	if err != nil {
		t.Fatalf("marshal request payload: %v", err)
	}
	if strings.Contains(string(encoded), rawFileContent) {
		t.Fatalf("metadata request payload leaked raw file content: %s", string(encoded))
	}

	messages, ok := requestPayload["messages"].([]interface{})
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v, want two messages", requestPayload["messages"])
	}
	userMessage, _ := messages[1].(map[string]interface{})
	if userMessage["content_redacted"] != true {
		t.Fatalf("content_redacted = %#v, want true", userMessage["content_redacted"])
	}
	contentSummary, _ := userMessage["content"].(map[string]interface{})
	jsonSummary, _ := contentSummary["json"].(map[string]interface{})
	fields, _ := jsonSummary["fields"].(map[string]interface{})
	if fields["fallback_answer_redacted"] != true {
		t.Fatalf("fallback_answer_redacted = %#v, want true", fields["fallback_answer_redacted"])
	}
	files, ok := fields["files"].([]map[string]interface{})
	if !ok || len(files) != 1 {
		t.Fatalf("files = %#v, want one summarized file", fields["files"])
	}
	if files[0]["id"] != "file-4" || files[0]["content_preview_redacted"] != true {
		t.Fatalf("file summary = %#v, want safe metadata with redacted preview", files[0])
	}
}
