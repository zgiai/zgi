package agentexecution

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPAPIRequiresDedicatedCredentialAndInjectedScope(t *testing.T) {
	api := newTestHTTPAPI(t)

	request := httptest.NewRequest(http.MethodPost, agentRuntimePathPrefix+"bindings", strings.NewReader(`{"agent_id":"agent-1"}`))
	request.Header.Set("X-API-Key", "ordinary-sandbox-key")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("ordinary API key status = %d, want 401", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, agentRuntimePathPrefix+"bindings", strings.NewReader(`{"agent_id":"agent-1"}`))
	request.Header.Set("Authorization", "Bearer runtime-secret")
	response = httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_scope") {
		t.Fatalf("missing scope response = %d %s", response.Code, response.Body.String())
	}
}

func TestHTTPAPIBindingJobAndFileFlow(t *testing.T) {
	api := newTestHTTPAPI(t)
	createBody := `{
		"agent_id":"agent-1",
		"conversation_id":"conversation-1",
		"profile_id":"profile-1",
		"backend":"fake",
		"idempotency_key":"binding-request-1"
	}`
	response := performAgentRuntimeRequest(t, api, http.MethodPost, agentRuntimePathPrefix+"bindings", strings.NewReader(createBody), "org-1", "user-1")
	if response.Code != http.StatusCreated {
		t.Fatalf("create binding response = %d %s", response.Code, response.Body.String())
	}
	bindingID := responseDataString(t, response.Body.Bytes(), "id")

	jobBody := `{"idempotency_key":"job-request-1","command":"python3","args":["-c","print('ok')"],"work_dir":"workspace","timeout_ms":30000}`
	response = performAgentRuntimeRequest(t, api, http.MethodPost, agentRuntimePathPrefix+"bindings/"+bindingID+"/jobs", strings.NewReader(jobBody), "org-1", "user-1")
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit job response = %d %s", response.Code, response.Body.String())
	}

	fileValue := []byte("artifact-data")
	response = performAgentRuntimeRequest(t, api, http.MethodPut, agentRuntimePathPrefix+"bindings/"+bindingID+"/files?path=results/out.txt", bytes.NewReader(fileValue), "org-1", "user-1")
	if response.Code != http.StatusCreated {
		t.Fatalf("upload response = %d %s", response.Code, response.Body.String())
	}
	response = performAgentRuntimeRequest(t, api, http.MethodGet, agentRuntimePathPrefix+"bindings/"+bindingID+"/files?path=results/out.txt", nil, "org-1", "user-1")
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), fileValue) {
		t.Fatalf("download response = %d %q", response.Code, response.Body.Bytes())
	}

	response = performAgentRuntimeRequest(t, api, http.MethodGet, agentRuntimePathPrefix+"bindings/"+bindingID, nil, "org-1", "user-1")
	generation := responseDataInt64(t, response.Body.Bytes(), "generation")
	response = performAgentRuntimeRequest(t, api, http.MethodPost, agentRuntimePathPrefix+"bindings/"+bindingID+"/suspend", strings.NewReader(fmt.Sprintf(`{"generation":%d}`, generation)), "org-1", "user-1")
	if response.Code != http.StatusOK || responseDataString(t, response.Body.Bytes(), "status") != string(BindingStatusSuspended) {
		t.Fatalf("suspend response = %d %s", response.Code, response.Body.String())
	}
	generation = responseDataInt64(t, response.Body.Bytes(), "generation")
	response = performAgentRuntimeRequest(t, api, http.MethodPost, agentRuntimePathPrefix+"bindings/"+bindingID+"/resume", strings.NewReader(fmt.Sprintf(`{"generation":%d}`, generation)), "org-1", "user-1")
	if response.Code != http.StatusOK || responseDataString(t, response.Body.Bytes(), "status") != string(BindingStatusActive) {
		t.Fatalf("resume response = %d %s", response.Code, response.Body.String())
	}

	response = performAgentRuntimeRequest(t, api, http.MethodGet, agentRuntimePathPrefix+"bindings/"+bindingID, nil, "org-other", "user-1")
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant binding response = %d %s", response.Code, response.Body.String())
	}
}

func TestHTTPAPIRejectsUnknownFieldsAndPathEscape(t *testing.T) {
	api := newTestHTTPAPI(t)
	response := performAgentRuntimeRequest(t, api, http.MethodPost, agentRuntimePathPrefix+"bindings", strings.NewReader(`{"unknown":true}`), "org-1", "user-1")
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_json") {
		t.Fatalf("unknown field response = %d %s", response.Code, response.Body.String())
	}

	createBody := `{"agent_id":"agent-1","conversation_id":"conversation-1","profile_id":"profile-1","backend":"fake","idempotency_key":"binding-request-1"}`
	response = performAgentRuntimeRequest(t, api, http.MethodPost, agentRuntimePathPrefix+"bindings", strings.NewReader(createBody), "org-1", "user-1")
	bindingID := responseDataString(t, response.Body.Bytes(), "id")
	response = performAgentRuntimeRequest(t, api, http.MethodPut, agentRuntimePathPrefix+"bindings/"+bindingID+"/files?path=../escape", bytes.NewReader(nil), "org-1", "user-1")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("path escape response = %d %s", response.Code, response.Body.String())
	}
}

func newTestHTTPAPI(t *testing.T) *HTTPAPI {
	t.Helper()
	repository := newMemoryRepository()
	sequence := 0
	now := func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }
	newID := func(prefix string) (string, error) {
		sequence++
		return fmt.Sprintf("%s-%d", prefix, sequence), nil
	}
	control, err := NewService(repository, map[string]Backend{"fake": &fakeBackend{}}, ServiceOptions{Now: now, NewID: newID})
	if err != nil {
		t.Fatalf("new control plane: %v", err)
	}
	data, err := NewDataPlaneService(repository, newMemoryJobRepository(), map[string]ExecutionBackend{"fake": newFakeExecutionBackend()}, DataPlaneOptions{Now: now, NewID: newID})
	if err != nil {
		t.Fatalf("new data plane: %v", err)
	}
	api, err := NewHTTPAPI(control, data, HTTPAPIOptions{APIKey: "runtime-secret"})
	if err != nil {
		t.Fatalf("new HTTP API: %v", err)
	}
	return api
}

func performAgentRuntimeRequest(t *testing.T, handler http.Handler, method string, target string, body io.Reader, organizationID string, subjectID string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, body)
	request.Header.Set("Authorization", "Bearer runtime-secret")
	request.Header.Set("X-ZGI-Organization-ID", organizationID)
	request.Header.Set("X-ZGI-Subject-ID", subjectID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func responseDataString(t *testing.T, value []byte, field string) string {
	t.Helper()
	var response struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(value, &response); err != nil {
		t.Fatalf("decode response: %v (%s)", err, value)
	}
	result, ok := response.Data[field].(string)
	if !ok || result == "" {
		t.Fatalf("response field %q = %#v", field, response.Data[field])
	}
	return result
}

func responseDataInt64(t *testing.T, value []byte, field string) int64 {
	t.Helper()
	var response struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(value, &response); err != nil {
		t.Fatalf("decode response: %v (%s)", err, value)
	}
	result, ok := response.Data[field].(float64)
	if !ok {
		t.Fatalf("response field %q = %#v", field, response.Data[field])
	}
	return int64(result)
}
