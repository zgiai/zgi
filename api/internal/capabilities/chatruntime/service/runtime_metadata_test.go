package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestModelInvocationMetadataStoresSummaryOnlyByDefault(t *testing.T) {
	t.Setenv(modelInvocationRawDebugEnv, "")
	secretContext := "Transient ZGI page context\nSECRET_RUNTIME_CONTEXT"
	trace := skillloop.ModelInvocationTrace{
		Phase:     "final_answer",
		Round:     0,
		StartedAt: time.Unix(1700000000, 0),
		Request: &adapter.ChatRequest{
			Provider: "test-provider",
			Model:    "test-model",
			Messages: []adapter.Message{
				{Role: "system", Content: secretContext},
				{Role: "user", Content: "hello"},
			},
		},
		Response: &adapter.Message{Role: "assistant", Content: "answer"},
		Usage:    &adapter.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5},
	}

	metadata := mergeModelInvocationMetadata(nil, modelInvocationFromTrace(trace, "VISIBLE_USER_PROMPT"))
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	metadataJSON := string(metadataBytes)
	if strings.Contains(metadataJSON, secretContext) || strings.Contains(metadataJSON, "SECRET_RUNTIME_CONTEXT") {
		t.Fatalf("model invocation metadata leaked raw context: %s", metadataJSON)
	}
	if strings.Contains(metadataJSON, "VISIBLE_USER_PROMPT") {
		t.Fatalf("model invocation metadata leaked raw user system prompt: %s", metadataJSON)
	}

	invocations := modelInvocationsFromMetadata(metadata["model_invocations"])
	if len(invocations) != 1 {
		t.Fatalf("model_invocations = %#v, want one invocation", metadata["model_invocations"])
	}
	invocation := invocations[0]
	if invocation["schema"] != modelInvocationSchema || invocation["trace_level"] != modelInvocationTraceLevelSummary {
		t.Fatalf("invocation schema/trace = %#v/%#v, want summary v2", invocation["schema"], invocation["trace_level"])
	}
	request := mapFromAny(invocation["request"])
	if request["schema"] != modelInvocationRequestSummarySchema || request["message_count"] != 2 {
		t.Fatalf("request summary = %#v, want summary schema and message count", request)
	}
	if request["has_runtime_context"] != true || request["runtime_context_char_count"] == nil {
		t.Fatalf("request runtime context summary = %#v, want runtime context flags", request)
	}
	if _, exists := request["content"]; exists {
		t.Fatalf("request summary unexpectedly contains content: %#v", request)
	}
	if _, exists := metadata[debugModelInvocationsMetadataKey]; exists {
		t.Fatalf("debug traces should be absent by default: %#v", metadata[debugModelInvocationsMetadataKey])
	}
}

func TestDebugModelInvocationRawTraceIsPubliclySummarized(t *testing.T) {
	t.Setenv(modelInvocationRawDebugEnv, "1")
	secretContext := "Current ZGI page context\nDEBUG_ONLY_RAW_CONTEXT"
	trace := skillloop.ModelInvocationTrace{
		Phase:     "skill_planning",
		Round:     1,
		StartedAt: time.Unix(1700000000, 0),
		Request: &adapter.ChatRequest{
			Provider: "test-provider",
			Model:    "test-model",
			Messages: []adapter.Message{
				{Role: "system", Content: secretContext},
				{Role: "user", Content: "hello"},
			},
		},
		Response: &adapter.Message{Role: "assistant", Content: "debug response"},
		Usage:    &adapter.Usage{PromptTokens: 4, CompletionTokens: 2, TotalTokens: 6},
	}

	metadata := mergeModelInvocationMetadata(nil, modelInvocationFromTrace(trace, "VISIBLE_USER_PROMPT"))
	metadata = mergeDebugModelInvocationMetadata(metadata, debugModelInvocationFromTrace(trace, "VISIBLE_USER_PROMPT"), trace.StartedAt)
	rawBytes, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal raw metadata: %v", err)
	}
	if !strings.Contains(string(rawBytes), "DEBUG_ONLY_RAW_CONTEXT") {
		t.Fatalf("debug metadata should retain raw trace when enabled: %s", string(rawBytes))
	}

	public := PublicMessageMetadata(metadata)
	publicBytes, err := json.Marshal(public)
	if err != nil {
		t.Fatalf("marshal public metadata: %v", err)
	}
	publicJSON := string(publicBytes)
	if strings.Contains(publicJSON, "DEBUG_ONLY_RAW_CONTEXT") || strings.Contains(publicJSON, "VISIBLE_USER_PROMPT") {
		t.Fatalf("public metadata leaked debug raw trace: %s", publicJSON)
	}
	if _, exists := public[debugModelInvocationsMetadataKey]; exists {
		t.Fatalf("public metadata exposed debug traces: %#v", public[debugModelInvocationsMetadataKey])
	}
}
