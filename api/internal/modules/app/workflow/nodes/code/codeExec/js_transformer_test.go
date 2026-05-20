package codeexec

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zgiai/ginext/config"
)

func TestJavaScriptTransformerLanguage(t *testing.T) {
	transformer := NewJavaScriptTransformer()
	if transformer.Language() != LanguageJavascript {
		t.Fatalf("expected language %q, got %q", LanguageJavascript, transformer.Language())
	}
}

func TestJavaScriptTransformerTransformCaller(t *testing.T) {
	transformer := NewJavaScriptTransformer()

	tests := []struct {
		name        string
		code        string
		inputs      map[string]any
		expectError bool
	}{
		{
			name:        "empty code",
			code:        "",
			inputs:      map[string]any{},
			expectError: true,
		},
		{
			name:        "whitespace only code",
			code:        "   ",
			inputs:      map[string]any{},
			expectError: true,
		},
		{
			name: "valid code with inputs",
			code: `function main(args) {
    return { result: args.name };
}`,
			inputs:      map[string]any{"name": "test"},
			expectError: false,
		},
		{
			name: "valid async code",
			code: `async function main(args) {
    return { result: args.name };
}`,
			inputs:      map[string]any{"name": "async-test"},
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner, preload, err := transformer.TransformCaller(tc.code, tc.inputs)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if preload != "" {
				t.Fatalf("expected empty preload, got %q", preload)
			}

			// Verify runner contains the original code
			if !strings.Contains(runner, tc.code) {
				t.Fatalf("runner should contain original code")
			}

			// Verify runner contains the base64 encoded inputs
			if !strings.Contains(runner, "inputsBase64") {
				t.Fatalf("runner should contain base64 input handling")
			}

			// Verify runner contains the result tag pattern
			if !strings.Contains(runner, "<<RESULT>>") {
				t.Fatalf("runner should contain result tags")
			}
		})
	}
}

func TestJavaScriptTransformerTransformResponse(t *testing.T) {
	transformer := NewJavaScriptTransformer()

	tests := []struct {
		name        string
		raw         string
		expectError bool
		expected    map[string]any
	}{
		{
			name:        "missing start tag",
			raw:         `{"result": "test"}<<RESULT>>`,
			expectError: true,
		},
		{
			name:        "missing end tag",
			raw:         `<<RESULT>>{"result": "test"}`,
			expectError: true,
		},
		{
			name:        "invalid json between tags",
			raw:         `<<RESULT>>not json<<RESULT>>`,
			expectError: true,
		},
		{
			name:        "valid response",
			raw:         `some log output<<RESULT>>{"result": "ok", "count": 42}<<RESULT>>more output`,
			expectError: false,
			expected:    map[string]any{"result": "ok", "count": float64(42)},
		},
		{
			name:        "nested object response",
			raw:         `<<RESULT>>{"data": {"name": "test", "items": [1, 2, 3]}}<<RESULT>>`,
			expectError: false,
			expected: map[string]any{
				"data": map[string]any{
					"name":  "test",
					"items": []any{float64(1), float64(2), float64(3)},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := transformer.TransformResponse(tc.raw)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Compare results
			expectedJSON, _ := json.Marshal(tc.expected)
			resultJSON, _ := json.Marshal(result)
			if string(expectedJSON) != string(resultJSON) {
				t.Fatalf("expected %s, got %s", expectedJSON, resultJSON)
			}
		})
	}
}

func TestExecuteWorkflowCodeTemplateJavaScript(t *testing.T) {
	type executionPayload struct {
		Language      string `json:"language"`
		Code          string `json:"code"`
		Preload       string `json:"preload"`
		EnableNetwork bool   `json:"enable_network"`
	}

	var receivedPayload executionPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}

		if !strings.HasSuffix(r.URL.Path, "/v1/sandbox/run") {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}

		if got := r.Header.Get("X-Api-Key"); got != "test-key" {
			t.Fatalf("expected X-Api-Key header %q, got %q", "test-key", got)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		defer r.Body.Close()

		if err := json.Unmarshal(body, &receivedPayload); err != nil {
			t.Fatalf("failed to unmarshal request payload: %v", err)
		}

		response := executionResponse{
			Code:    0,
			Message: "ok",
			Data: executionResponseData{
				Stdout: ptr("<<RESULT>>{\"result\":\"hello tester\"}<<RESULT>>"),
			},
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	prevConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		CodeExec: config.CodeExecConfig{
			Endpoint: server.URL,
			APIKey:   "test-key",
		},
	}
	t.Cleanup(func() {
		config.GlobalConfig = prevConfig
	})

	executor := NewExecutor(NewJavaScriptTransformer())

	code := `
function main(args) {
    return { result: "hello " + args.name };
}
`
	result, err := executor.ExecuteWorkflowCodeTemplate(
		context.Background(),
		LanguageJavascript,
		code,
		map[string]any{"name": "tester"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedPayload.Language != "nodejs" {
		t.Fatalf("unexpected language sent to sandbox: %s", receivedPayload.Language)
	}
	if receivedPayload.Preload != "" {
		t.Fatalf("expected empty preload but got %q", receivedPayload.Preload)
	}
	if !receivedPayload.EnableNetwork {
		t.Fatalf("expected network to be enabled")
	}
	if !strings.Contains(receivedPayload.Code, "function main") {
		t.Fatalf("runner script should contain original code")
	}

	if got, ok := result["result"]; !ok || got != "hello tester" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestExecuteWorkflowCodeTemplateJavaScriptAsync(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := executionResponse{
			Code:    0,
			Message: "ok",
			Data: executionResponseData{
				Stdout: ptr("<<RESULT>>{\"data\":[1,2,3]}<<RESULT>>"),
			},
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	prevConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		CodeExec: config.CodeExecConfig{
			Endpoint: server.URL,
			APIKey:   "test-key",
		},
	}
	t.Cleanup(func() {
		config.GlobalConfig = prevConfig
	})

	executor := NewExecutor(NewJavaScriptTransformer())

	code := `
async function main(args) {
    // Simulate async operation
    return { data: args.items };
}
`
	result, err := executor.ExecuteWorkflowCodeTemplate(
		context.Background(),
		LanguageJavascript,
		code,
		map[string]any{"items": []int{1, 2, 3}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, ok := result["data"].([]any)
	if !ok {
		t.Fatalf("expected data to be array, got %T", result["data"])
	}
	if len(data) != 3 {
		t.Fatalf("expected 3 items, got %d", len(data))
	}
}

func TestMultipleTransformersRegistered(t *testing.T) {
	executor := NewExecutor(
		NewPythonTransformer(),
		NewJavaScriptTransformer(),
	)

	// Verify both transformers are registered
	if _, exists := executor.transformers[LanguagePython3]; !exists {
		t.Fatalf("python transformer not registered")
	}
	if _, exists := executor.transformers[LanguageJavascript]; !exists {
		t.Fatalf("javascript transformer not registered")
	}
}
