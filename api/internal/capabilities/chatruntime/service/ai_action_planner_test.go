package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestAIChatActionPlannerParsesFileReadPlan(t *testing.T) {
	fakeLLM := &fakeAIChatActionPlannerLLM{
		response: aiChatActionPlannerResponse(`{"matched":true,"confidence":0.91,"capability_id":"file.read","intent":"read_selected_file","resource_refs":[{"type":"file","id":"file-1","source":"console.files","name":"notes.txt"}],"reason":"selected file read request"}`),
	}
	planner := newAIChatActionPlanner(fakeLLM)

	decision := planner.Plan(context.Background(), testAIChatActionPlanRequest("read the selected file"))

	if !decision.Matched {
		t.Fatalf("Matched = false, want true; decision=%#v", decision)
	}
	if decision.CapabilityID != consoleFilesActionCapabilityID {
		t.Fatalf("CapabilityID = %q, want %q", decision.CapabilityID, consoleFilesActionCapabilityID)
	}
	if decision.Confidence == nil || *decision.Confidence != 0.91 {
		t.Fatalf("Confidence = %#v, want 0.91", decision.Confidence)
	}
	if len(decision.ResourceRefs) != 1 || decision.ResourceRefs[0].ID != "file-1" {
		t.Fatalf("ResourceRefs = %#v, want file-1", decision.ResourceRefs)
	}
	if len(fakeLLM.requests) != 1 {
		t.Fatalf("planner requests = %d, want 1", len(fakeLLM.requests))
	}
	if fakeLLM.requests[0].ResponseFormat == nil || fakeLLM.requests[0].ResponseFormat.Type != "json_object" {
		t.Fatalf("ResponseFormat = %#v, want json_object", fakeLLM.requests[0].ResponseFormat)
	}
	systemPrompt, ok := fakeLLM.requests[0].Messages[0].Content.(string)
	if !ok || !strings.Contains(systemPrompt, "Return exactly one JSON object") {
		t.Fatalf("system prompt = %#v, want strict JSON instruction", fakeLLM.requests[0].Messages[0].Content)
	}
}

func TestAIChatActionPlannerReturnsNoMatchWhenLLMUnavailable(t *testing.T) {
	decision := newAIChatActionPlanner(nil).Plan(context.Background(), testAIChatActionPlanRequest("read notes.txt"))

	if decision.Matched {
		t.Fatalf("Matched = true, want false; decision=%#v", decision)
	}
}

func TestAIChatActionPlannerReturnsNoMatchForInvalidJSON(t *testing.T) {
	fakeLLM := &fakeAIChatActionPlannerLLM{response: aiChatActionPlannerResponse("I should read it.")}

	decision := newAIChatActionPlanner(fakeLLM).Plan(context.Background(), testAIChatActionPlanRequest("read notes.txt"))

	if decision.Matched {
		t.Fatalf("Matched = true, want false; decision=%#v", decision)
	}
	if !strings.Contains(decision.Reason, "planner decision invalid") {
		t.Fatalf("Reason = %q, want invalid planner decision", decision.Reason)
	}
}

func TestAIChatActionPlannerReturnsNoMatchWithoutExecutableConfidence(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "missing confidence",
			content: `{"matched":true,"capability_id":"file.read","intent":"read_selected_file","resource_refs":[{"type":"file","id":"file-1"}],"reason":"missing confidence"}`,
		},
		{
			name:    "low confidence",
			content: `{"matched":true,"confidence":0.2,"capability_id":"file.read","intent":"read_selected_file","resource_refs":[{"type":"file","id":"file-1"}],"reason":"uncertain"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeLLM := &fakeAIChatActionPlannerLLM{response: aiChatActionPlannerResponse(tt.content)}

			decision := newAIChatActionPlanner(fakeLLM).Plan(context.Background(), testAIChatActionPlanRequest("read notes.txt"))

			if decision.Matched {
				t.Fatalf("Matched = true, want false; decision=%#v", decision)
			}
		})
	}
}

func TestAIChatActionPlannerReturnsNoMatchForUnknownCapability(t *testing.T) {
	fakeLLM := &fakeAIChatActionPlannerLLM{
		response: aiChatActionPlannerResponse(`{"matched":true,"confidence":0.92,"capability_id":"file.delete","intent":"delete_file","resource_refs":[{"type":"file","id":"file-1"}],"reason":"not available"}`),
	}

	decision := newAIChatActionPlanner(fakeLLM).Plan(context.Background(), testAIChatActionPlanRequest("delete notes.txt"))

	if decision.Matched {
		t.Fatalf("Matched = true, want false; decision=%#v", decision)
	}
	if !strings.Contains(decision.Reason, "capability unavailable") {
		t.Fatalf("Reason = %q, want capability unavailable", decision.Reason)
	}
}

func TestAIChatActionPlannerPreservesTranslatePostprocess(t *testing.T) {
	fakeLLM := &fakeAIChatActionPlannerLLM{
		response: aiChatActionPlannerResponse(`{"matched":true,"confidence":0.88,"capability_id":"file.read","intent":"read_then_translate","resource_refs":[{"type":"file","id":"file-1"}],"postprocess":[{"type":"translate","target_language":"zh-CN"}],"reason":"translate file content after reading"}`),
	}

	decision := newAIChatActionPlanner(fakeLLM).Plan(context.Background(), testAIChatActionPlanRequest("translate notes.txt to Chinese"))

	if !decision.Matched {
		t.Fatalf("Matched = false, want true; decision=%#v", decision)
	}
	if len(decision.Postprocess) != 1 {
		t.Fatalf("Postprocess = %#v, want one translate item", decision.Postprocess)
	}
	if decision.Postprocess[0].Type != "translate" || decision.Postprocess[0].TargetLanguage != "zh-CN" {
		t.Fatalf("Postprocess[0] = %#v, want translate zh-CN", decision.Postprocess[0])
	}
}

func TestAIChatActionPlannerLLMErrorReturnsNoMatch(t *testing.T) {
	fakeLLM := &fakeAIChatActionPlannerLLM{err: errors.New("planner model unavailable")}

	decision := newAIChatActionPlanner(fakeLLM).Plan(context.Background(), testAIChatActionPlanRequest("read notes.txt"))

	if decision.Matched {
		t.Fatalf("Matched = true, want false; decision=%#v", decision)
	}
	if !strings.Contains(decision.Reason, "planner llm failed") {
		t.Fatalf("Reason = %q, want planner llm failed", decision.Reason)
	}
}

func TestAIChatActionCapabilitiesFromPartsFindsFileRead(t *testing.T) {
	parts := &chatRequestParts{
		RawOperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"resource_type":  "page",
					"resource_id":    "console.files",
					"capability_ids": []interface{}{"file.read"},
				},
			},
		},
	}

	capabilities := aiChatActionCapabilitiesFromParts(parts)

	if len(capabilities) != 1 {
		t.Fatalf("capabilities = %#v, want one capability", capabilities)
	}
	if capabilities[0].ID != consoleFilesActionCapabilityID || capabilities[0].Description == "" {
		t.Fatalf("capability = %#v, want file.read with defaults", capabilities[0])
	}
}

type fakeAIChatActionPlannerLLM struct {
	response *adapter.ChatResponse
	err      error
	requests []*adapter.ChatRequest
}

func (f *fakeAIChatActionPlannerLLM) AppChat(_ context.Context, _ *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	f.requests = append(f.requests, cloneChatRequest(req))
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func testAIChatActionPlanRequest(query string) AIChatActionPlanRequest {
	return AIChatActionPlanRequest{
		Query:          query,
		RuntimeContext: "route=/console/files",
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "notes.txt", "selected": true},
			},
		},
		Capabilities: []AIChatActionCapability{{
			ID:          consoleFilesActionCapabilityID,
			Name:        "Read file",
			Description: "Read selected file content.",
		}},
		ModelName: "planner-test-model",
		Provider:  "planner-test-provider",
	}
}

func aiChatActionPlannerResponse(content string) *adapter.ChatResponse {
	return &adapter.ChatResponse{
		Choices: []adapter.Choice{{
			Message: adapter.Message{Role: "assistant", Content: content},
		}},
	}
}
