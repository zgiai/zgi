package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestOpenAICompatibleAgentAdaptersPreserveToolLoop(t *testing.T) {
	tests := []struct {
		name string
		new  func(string) (adapter.ChatCapable, error)
	}{
		{name: "openai", new: func(baseURL string) (adapter.ChatCapable, error) {
			return NewOpenAIAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL})
		}},
		{name: "openai-compatible", new: func(baseURL string) (adapter.ChatCapable, error) {
			return NewOpenAIAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL})
		}},
		{name: "deepseek", new: func(baseURL string) (adapter.ChatCapable, error) {
			return NewDeepSeekAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL})
		}},
		{name: "siliconflow", new: func(baseURL string) (adapter.ChatCapable, error) {
			return NewSiliconFlowAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL})
		}},
		{name: "glm", new: func(baseURL string) (adapter.ChatCapable, error) {
			return NewGLMAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL})
		}},
		{name: "moonshot", new: func(baseURL string) (adapter.ChatCapable, error) {
			return NewMoonshotAIAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL})
		}},
		{name: "zgi-cloud", new: func(baseURL string) (adapter.ChatCapable, error) {
			return NewZGICloudAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: baseURL})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				defer r.Body.Close()
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				messages, _ := payload["messages"].([]any)
				if calls == 1 {
					assertInitialAgentPayload(t, payload, messages)
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"","reasoning_content":"checked tool policy","tool_calls":[{"id":"call_weather","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Hangzhou\"}"}}]}}]}`)
					return
				}
				assertToolResultPayload(t, messages)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"sunny"}}]}`)
			}))
			defer server.Close()

			chatAdapter, err := tt.new(server.URL + "/v1")
			if err != nil {
				t.Fatalf("create adapter: %v", err)
			}
			request := agentContractRequest()
			response, err := chatAdapter.ChatCompletion(context.Background(), request)
			if err != nil {
				t.Fatalf("first ChatCompletion: %v", err)
			}
			if len(response.Choices) != 1 || len(response.Choices[0].Message.ToolCalls) != 1 {
				t.Fatalf("tool call response = %#v, want one tool call", response)
			}
			request.Messages = append(request.Messages,
				response.Choices[0].Message,
				adapter.Message{Role: "tool", ToolCallID: "call_weather", Content: "sunny"},
			)
			finalResponse, err := chatAdapter.ChatCompletion(context.Background(), request)
			if err != nil {
				t.Fatalf("second ChatCompletion: %v", err)
			}
			if len(finalResponse.Choices) != 1 || finalResponse.Choices[0].Message.Content != "sunny" {
				t.Fatalf("final response = %#v, want sunny", finalResponse)
			}
		})
	}
}

func agentContractRequest() *adapter.ChatRequest {
	return &adapter.ChatRequest{
		Model: "agent-model",
		Messages: []adapter.Message{
			{Role: "system", Content: "Use tools when needed."},
			{Role: "user", Content: "Weather in Hangzhou?"},
		},
		Tools: []adapter.Tool{{
			Type: "function",
			Function: adapter.Function{
				Name: "get_weather",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{"city": map[string]any{"type": "string"}},
				},
			},
		}},
		ToolChoice: "auto",
	}
}

func assertInitialAgentPayload(t *testing.T, payload map[string]any, messages []any) {
	t.Helper()
	if payload["tool_choice"] != "auto" {
		t.Errorf("tool_choice = %#v, want auto", payload["tool_choice"])
	}
	tools, _ := payload["tools"].([]any)
	if len(tools) != 1 {
		t.Errorf("tools = %#v, want one tool", payload["tools"])
	}
	if len(messages) != 2 || messageRole(messages[0]) != "system" || messageRole(messages[1]) != "user" {
		t.Errorf("initial messages = %#v, want system then user", messages)
	}
}

func assertToolResultPayload(t *testing.T, messages []any) {
	t.Helper()
	if len(messages) != 4 {
		t.Errorf("tool result messages = %#v, want four messages", messages)
		return
	}
	assistant, _ := messages[2].(map[string]any)
	tool, _ := messages[3].(map[string]any)
	toolCalls, _ := assistant["tool_calls"].([]any)
	if messageRole(messages[2]) != "assistant" || len(toolCalls) != 1 {
		t.Errorf("assistant message = %#v, want preserved tool_calls", assistant)
	}
	if assistant["reasoning_content"] != "checked tool policy" {
		t.Errorf("assistant message = %#v, want preserved reasoning_content", assistant)
	}
	if messageRole(messages[3]) != "tool" || tool["tool_call_id"] != "call_weather" {
		t.Errorf("tool message = %#v, want matching tool_call_id", tool)
	}
}

func messageRole(raw any) string {
	message, _ := raw.(map[string]any)
	role, _ := message["role"].(string)
	return role
}
