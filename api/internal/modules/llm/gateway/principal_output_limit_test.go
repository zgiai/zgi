package gateway

import (
	"encoding/json"
	"testing"

	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestPrincipalChatOutputLimitIsAppliedWithoutMutatingCallerRequest(t *testing.T) {
	svc := &llmGatewayServiceImpl{tokenEstimator: NewTokenEstimator()}
	req := &adapter.ChatRequest{Model: "gpt-4o", Messages: []adapter.Message{{Role: "user", Content: "hello"}}}

	limited := svc.withPrincipalChatOutputLimit(principalBoundTestAPIKey(), req)
	if limited == req || limited.MaxTokens == nil || *limited.MaxTokens != 1000 {
		t.Fatalf("limited request = %#v, want cloned request with max_tokens=1000", limited)
	}
	if req.MaxTokens != nil {
		t.Fatal("caller request was mutated")
	}

	explicit := 7
	req.MaxTokens = &explicit
	if got := svc.withPrincipalChatOutputLimit(principalBoundTestAPIKey(), req); got != req || got.MaxTokens != &explicit {
		t.Fatalf("explicit max_tokens was not preserved: %#v", got)
	}
	if got := svc.withPrincipalChatOutputLimit(&apikeymodel.TenantAPIKey{}, &adapter.ChatRequest{Model: "gpt-4o"}); got.MaxTokens != nil {
		t.Fatalf("organization key request unexpectedly received a limit: %#v", got)
	}
}

func TestPrincipalNativeOutputLimitsCoverSupportedProtocols(t *testing.T) {
	svc := &llmGatewayServiceImpl{tokenEstimator: NewTokenEstimator()}
	tests := []struct {
		name     string
		protocol string
		body     string
		assert   func(*testing.T, map[string]interface{})
	}{
		{
			name:     "OpenAI Responses",
			protocol: protocolOpenAIResponses,
			body:     `{"input":"hello"}`,
			assert: func(t *testing.T, payload map[string]interface{}) {
				if payload["max_output_tokens"] != float64(1000) {
					t.Fatalf("max_output_tokens = %#v, want 1000", payload["max_output_tokens"])
				}
			},
		},
		{
			name:     "Anthropic Messages",
			protocol: protocolAnthropicMessages,
			body:     `{"messages":[{"role":"user","content":"hello"}]}`,
			assert: func(t *testing.T, payload map[string]interface{}) {
				if payload["max_tokens"] != float64(500) {
					t.Fatalf("max_tokens = %#v, want 500", payload["max_tokens"])
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := svc.withPrincipalNativeOutputLimit(principalBoundTestAPIKey(), json.RawMessage(test.body), protocolModel(test.protocol), test.protocol)
			if err != nil {
				t.Fatalf("withPrincipalNativeOutputLimit() error = %v", err)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(got, &payload); err != nil {
				t.Fatalf("unmarshal output: %v", err)
			}
			test.assert(t, payload)
		})
	}
}

func TestPrincipalNativeOutputLimitPreservesExplicitLimitAndRejectsInvalidLimit(t *testing.T) {
	svc := &llmGatewayServiceImpl{tokenEstimator: NewTokenEstimator()}
	explicit := json.RawMessage(`{"input":"hello","max_output_tokens":7}`)
	got, err := svc.withPrincipalNativeOutputLimit(principalBoundTestAPIKey(), explicit, "gpt-4o", protocolOpenAIResponses)
	if err != nil || string(got) != string(explicit) {
		t.Fatalf("explicit limit result = %s, %v", got, err)
	}

	_, err = svc.withPrincipalNativeOutputLimit(principalBoundTestAPIKey(), json.RawMessage(`{"input":"hello","max_output_tokens":0}`), "gpt-4o", protocolOpenAIResponses)
	if err == nil {
		t.Fatal("invalid explicit limit error = nil")
	}
}

func principalBoundTestAPIKey() *apikeymodel.TenantAPIKey {
	grantID := "00000000-0000-0000-0000-000000000001"
	principalType := "user"
	principalID := "00000000-0000-0000-0000-000000000002"
	workspaceID := "00000000-0000-0000-0000-000000000003"
	return &apikeymodel.TenantAPIKey{
		AccessGrantID: &grantID,
		PrincipalType: &principalType,
		PrincipalID:   &principalID,
		WorkspaceID:   &workspaceID,
	}
}

func protocolModel(protocol string) string {
	switch protocol {
	case protocolOpenAIResponses:
		return "gpt-4o"
	case protocolAnthropicMessages:
		return "claude-sonnet"
	}
	return ""
}
