package service

import (
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
