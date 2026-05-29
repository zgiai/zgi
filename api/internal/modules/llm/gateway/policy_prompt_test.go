package gateway

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	appconfig "github.com/zgiai/zgi/api/config"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const testPolicyPrompt = "policy prompt"

func TestLLMPolicyPromptInjector_DisabledLeavesChatRequestUnchanged(t *testing.T) {
	injector := newLLMPolicyPromptInjector(appconfig.LLMPolicyPromptConfig{})
	req := &adapter.ChatRequest{
		Model: "gpt-test",
		Messages: []adapter.Message{{
			Role:    "user",
			Content: "hello",
		}},
	}

	got := injector.injectChatRequest(req)
	if got != req {
		t.Fatal("injectChatRequest returned a cloned request when disabled")
	}
	if !reflect.DeepEqual(got, req) {
		t.Fatalf("disabled injection changed request: got %#v want %#v", got, req)
	}
}

func TestLLMPolicyPromptInjector_PrependsChatSystemPrompt(t *testing.T) {
	injector := newLLMPolicyPromptInjector(appconfig.LLMPolicyPromptConfig{
		Enabled: true,
		Prompt:  testPolicyPrompt,
	})
	req := &adapter.ChatRequest{
		Model: "gpt-test",
		Messages: []adapter.Message{
			{Role: "system", Content: "app system"},
			{Role: "user", Content: "hello"},
		},
	}

	got := injector.injectChatRequest(req)
	if got == req {
		t.Fatal("injectChatRequest returned original request when enabled")
	}
	if len(got.Messages) != 3 {
		t.Fatalf("message count = %d, want 3", len(got.Messages))
	}
	if got.Messages[0].Role != "system" || got.Messages[0].Content != testPolicyPrompt {
		t.Fatalf("first message = %#v, want policy system prompt", got.Messages[0])
	}
	if got.Messages[1].Content != "app system" {
		t.Fatalf("existing system prompt = %#v, want preserved after policy", got.Messages[1])
	}
	if len(req.Messages) != 2 {
		t.Fatalf("original request message count = %d, want 2", len(req.Messages))
	}
}

func TestLLMPolicyPromptInjector_InjectsCreateResponseInstructions(t *testing.T) {
	injector := newLLMPolicyPromptInjector(appconfig.LLMPolicyPromptConfig{
		Enabled: true,
		Prompt:  testPolicyPrompt,
	})
	req := &adapter.CreateResponseRequest{
		Model:        "gpt-test",
		Instructions: "app instructions",
	}

	got := injector.injectCreateResponseRequest(req)
	if got == req {
		t.Fatal("injectCreateResponseRequest returned original request when enabled")
	}
	if got.Instructions != "policy prompt\n\napp instructions" {
		t.Fatalf("Instructions = %q, want combined policy and original instructions", got.Instructions)
	}
	if req.Instructions != "app instructions" {
		t.Fatalf("original Instructions = %q, want unchanged", req.Instructions)
	}
}

func TestLLMPolicyPromptInjector_InjectsRawOpenAIResponsesInstructions(t *testing.T) {
	injector := newLLMPolicyPromptInjector(appconfig.LLMPolicyPromptConfig{
		Enabled: true,
		Prompt:  testPolicyPrompt,
	})
	body := json.RawMessage(`{"model":"gpt-test","instructions":"app instructions","input":"hello"}`)

	got, err := injector.injectOpenAIResponseBody(body)
	if err != nil {
		t.Fatalf("injectOpenAIResponseBody() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("unmarshal injected body: %v", err)
	}
	if payload["instructions"] != "policy prompt\n\napp instructions" {
		t.Fatalf("instructions = %#v, want combined policy and original instructions", payload["instructions"])
	}
}

func TestLLMPolicyPromptInjector_InjectsAnthropicSystemPrompt(t *testing.T) {
	injector := newLLMPolicyPromptInjector(appconfig.LLMPolicyPromptConfig{
		Enabled: true,
		Prompt:  testPolicyPrompt,
	})
	body := json.RawMessage(`{"model":"claude-test","system":"app system","messages":[{"role":"user","content":"hello"}]}`)

	got, err := injector.injectAnthropicMessageBody(body)
	if err != nil {
		t.Fatalf("injectAnthropicMessageBody() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("unmarshal injected body: %v", err)
	}
	if payload["system"] != "policy prompt\n\napp system" {
		t.Fatalf("system = %#v, want combined policy and original system", payload["system"])
	}
}

func TestLLMPolicyPromptInjector_WrapsImagePrompt(t *testing.T) {
	injector := newLLMPolicyPromptInjector(appconfig.LLMPolicyPromptConfig{
		Enabled: true,
		Prompt:  testPolicyPrompt,
	})
	req := &adapter.ImageRequest{
		Model:  "image-test",
		Prompt: "draw a cat",
	}

	got := injector.injectImageRequest(req)
	if got == req {
		t.Fatal("injectImageRequest returned original request when enabled")
	}
	if !strings.Contains(got.Prompt, testPolicyPrompt) || !strings.Contains(got.Prompt, "draw a cat") {
		t.Fatalf("wrapped prompt = %q, want policy and original prompt", got.Prompt)
	}
	if req.Prompt != "draw a cat" {
		t.Fatalf("original prompt = %q, want unchanged", req.Prompt)
	}
}
