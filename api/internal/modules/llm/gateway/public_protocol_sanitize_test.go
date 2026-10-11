package gateway

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizePublicNativeBodyRestoresAnthropicModelAndRemovesInternalUsage(t *testing.T) {
	body := json.RawMessage(`{"type":"message_start","message":{"type":"message","model":"upstream-model","usage":{"input_tokens":10,"billing_usage":{"source":"oai_chat","semantic":"openai","openai_usage":{"prompt_tokens":10}}}}}`)
	got := string(sanitizePublicNativeBody(body, "logical-model"))
	if !strings.Contains(got, `"model":"logical-model"`) || strings.Contains(got, "upstream-model") || strings.Contains(got, "billing_usage") || strings.Contains(got, `"semantic"`) {
		t.Fatalf("sanitized body = %s", got)
	}
}

func TestSanitizePublicNativeBodyLeavesFunctionArgumentsUnchanged(t *testing.T) {
	body := json.RawMessage(`{"type":"response.completed","response":{"model":"upstream-model","output":[{"type":"function_call","arguments":"{\"model\":\"user-value\"}"}]}}`)
	got := string(sanitizePublicNativeBody(body, "logical-model"))
	if !strings.Contains(got, `"model":"logical-model"`) || !strings.Contains(got, `\"model\":\"user-value\"`) {
		t.Fatalf("sanitized body = %s", got)
	}
}

func TestSanitizePublicNativeErrorBodyRemovesRouteAndRestoresLogicalModel(t *testing.T) {
	body := json.RawMessage(`{"error":{"message":"provider dashscope rejected model qwen-upstream-2026 on channel internal-a","model":"qwen-upstream-2026","upstream_model":"qwen-upstream-2026","provider":"dashscope","channel_name":"internal-a"}}`)
	got := string(SanitizePublicNativeErrorBody(body, "qwen-logical"))
	for _, forbidden := range []string{"dashscope", "qwen-upstream-2026", "internal-a", "upstream_model", "channel_name", `"provider"`} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized error leaked %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, `"model":"qwen-logical"`) || !strings.Contains(got, "model qwen-logical") {
		t.Fatalf("sanitized error did not restore logical model: %s", got)
	}
}

func TestSanitizePublicNativeErrorBodyDropsNonJSON(t *testing.T) {
	if got := SanitizePublicNativeErrorBody(json.RawMessage("provider secret failure"), "logical"); got != nil {
		t.Fatalf("non-JSON error = %q, want nil", string(got))
	}
}
