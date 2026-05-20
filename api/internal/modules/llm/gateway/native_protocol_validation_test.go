package gateway

import (
	"encoding/json"
	"errors"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestValidateRawResponseRequest_NormalizesCursorModelPrefix(t *testing.T) {
	req := &adapter.RawResponseRequest{
		Model: " cursor-gpt-5 ",
		Body:  json.RawMessage(`{"input":"hello"}`),
	}

	if err := validateRawResponseRequest(req); err != nil {
		t.Fatalf("validateRawResponseRequest() error = %v", err)
	}
	if req.Model != "gpt-5" {
		t.Fatalf("req.Model = %q, want %q", req.Model, "gpt-5")
	}
}

func TestValidateRawResponseRequest_CursorPrefixOnlyFailsFast(t *testing.T) {
	req := &adapter.RawResponseRequest{
		Model: "cursor-",
		Body:  json.RawMessage(`{"input":"hello"}`),
	}

	err := validateRawResponseRequest(req)
	if !errors.Is(err, ErrMissingModel) {
		t.Fatalf("validateRawResponseRequest() error = %v, want ErrMissingModel", err)
	}
}

func TestValidateAnthropicMessageRequest_NormalizesCursorModelPrefix(t *testing.T) {
	req := &adapter.AnthropicMessageRequest{
		Model: " cursor-claude-sonnet-4-0 ",
		Body:  json.RawMessage(`{"messages":[{"role":"user","content":"hello"}]}`),
	}

	if err := validateAnthropicMessageRequest(req); err != nil {
		t.Fatalf("validateAnthropicMessageRequest() error = %v", err)
	}
	if req.Model != "claude-sonnet-4-0" {
		t.Fatalf("req.Model = %q, want %q", req.Model, "claude-sonnet-4-0")
	}
}

func TestValidateAnthropicMessageRequest_CursorPrefixOnlyFailsFast(t *testing.T) {
	req := &adapter.AnthropicMessageRequest{
		Model: "cursor-",
		Body:  json.RawMessage(`{"messages":[{"role":"user","content":"hello"}]}`),
	}

	err := validateAnthropicMessageRequest(req)
	if !errors.Is(err, ErrMissingModel) {
		t.Fatalf("validateAnthropicMessageRequest() error = %v, want ErrMissingModel", err)
	}
}
