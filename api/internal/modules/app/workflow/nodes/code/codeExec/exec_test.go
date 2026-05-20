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

func TestExecuteWorkflowCodeTemplatePython(t *testing.T) {
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
				Stdout: ptr("<<RESULT>>{\"result\":\"ok tester\"}<<RESULT>>"),
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

	executor := NewExecutor(NewPythonTransformer())

	code := `
def main(name="world"):
    return {"result": f"ok {name}"}
`
	result, err := executor.ExecuteWorkflowCodeTemplate(
		context.Background(),
		LanguagePython3,
		code,
		map[string]any{"name": "tester"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedPayload.Language != "python3" {
		t.Fatalf("unexpected language sent to sandbox: %s", receivedPayload.Language)
	}
	if receivedPayload.Preload != "" {
		t.Fatalf("expected empty preload but got %q", receivedPayload.Preload)
	}
	if !receivedPayload.EnableNetwork {
		t.Fatalf("expected network to be enabled")
	}
	if !strings.Contains(receivedPayload.Code, "def main") {
		t.Fatalf("runner script should contain original code")
	}

	if got, ok := result["result"]; !ok || got != "ok tester" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestExecuteWorkflowCodeTemplatePython2(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := executionResponse{
			Code:    0,
			Message: "ok",
			Data: executionResponseData{
				Stdout: ptr("<<RESULT>>{\"result\":\"ok tester\"}<<RESULT>>"),
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
			APIKey:   "test-sandbox",
		},
	}
	t.Cleanup(func() {
		config.GlobalConfig = prevConfig
	})

	executor := NewExecutor(NewPythonTransformer())

	code := `
def main(name="world"):
    return {"result": f"ok {name}"}
`
	result, err := executor.ExecuteWorkflowCodeTemplate(
		context.Background(),
		LanguagePython3,
		code,
		map[string]any{"name": "tester"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, ok := result["result"]; !ok || got != "ok tester" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestExecuteWorkflowCodeTemplateSandboxError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := executionResponse{
			Code:    123,
			Message: "sandbox error",
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

	executor := NewExecutor(NewPythonTransformer())
	_, err := executor.ExecuteWorkflowCodeTemplate(
		context.Background(),
		LanguagePython3,
		"def main(): return {}",
		map[string]any{},
	)
	if err == nil {
		t.Fatalf("expected error from sandbox, got nil")
	}
}

func ptr[T any](value T) *T {
	return &value
}
