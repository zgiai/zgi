package codeexec

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/config"
)

func TestExecuteWorkflowCodeTemplatePython(t *testing.T) {
	type executionPayload struct {
		Language          string `json:"language"`
		Code              string `json:"code"`
		Preload           string `json:"preload"`
		EnableNetwork     bool   `json:"enable_network"`
		DependencyProfile string `json:"dependency_profile"`
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
	if receivedPayload.EnableNetwork {
		t.Fatal("expected network to be disabled by default")
	}
	if receivedPayload.DependencyProfile != "" {
		t.Fatalf("expected empty dependency profile, got %q", receivedPayload.DependencyProfile)
	}
	if !strings.Contains(receivedPayload.Code, "def main") {
		t.Fatalf("runner script should contain original code")
	}

	if got, ok := result["result"]; !ok || got != "ok tester" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestExecuteWorkflowCodeTemplateWithManagedProfile(t *testing.T) {
	var dependencyProfile string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload executionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		dependencyProfile = payload.DependencyProfile
		response := executionResponse{
			Code:    0,
			Message: "ok",
			Data: executionResponseData{
				Stdout: ptr("<<RESULT>>{\"result\":\"scientific\"}<<RESULT>>"),
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	prevConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{CodeExec: config.CodeExecConfig{Endpoint: server.URL}}
	t.Cleanup(func() { config.GlobalConfig = prevConfig })

	executor := NewExecutor(NewPythonTransformer())
	result, err := executor.ExecuteWorkflowCodeTemplateWithProfile(
		context.Background(),
		LanguagePython3,
		"def main(): return {'result': 'scientific'}",
		map[string]any{},
		"scientific-data",
	)
	if err != nil {
		t.Fatalf("execute with profile: %v", err)
	}
	if dependencyProfile != "scientific-data" {
		t.Fatalf("dependency_profile = %q, want scientific-data", dependencyProfile)
	}
	if result["result"] != "scientific" {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteWorkflowCodeTemplateWithScientificProfileIntegration(t *testing.T) {
	if os.Getenv("ZGI_SCIENTIFIC_INTEGRATION") != "1" {
		t.Skip("set ZGI_SCIENTIFIC_INTEGRATION=1 to run the Sandbox integration test")
	}
	endpoint := os.Getenv("ZGI_SANDBOX_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:8194"
	}
	prevConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{CodeExec: config.CodeExecConfig{Endpoint: endpoint}}
	t.Cleanup(func() { config.GlobalConfig = prevConfig })

	executor := NewExecutor(NewPythonTransformer())
	result, err := executor.ExecuteWorkflowCodeTemplateWithProfile(
		context.Background(),
		LanguagePython3,
		`def main(values):
    import pandas as pd
    from scipy import stats
    series = pd.Series(values, dtype=float)
    return {"count": int(series.count()), "mean": float(series.mean()), "p_value": float(stats.ttest_1samp(series, popmean=0).pvalue)}`,
		map[string]any{"values": []float64{1, 2, 3, 4, 5}},
		"scientific-data",
	)
	if err != nil {
		t.Fatalf("execute scientific workflow code: %v", err)
	}
	if result["count"] != float64(5) || result["mean"] != float64(3) {
		t.Fatalf("unexpected scientific result: %#v", result)
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
