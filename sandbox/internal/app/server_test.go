package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/runner"
	"github.com/zgiai/zgi-sandbox/internal/testutil"
)

func TestHealthEndpoint(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", "req_health")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("X-Request-ID") != "req_health" {
		t.Fatalf("expected request ID response header, got %q", rr.Header().Get("X-Request-ID"))
	}
	if !strings.Contains(rr.Body.String(), `"runtime_backend":"preview-process"`) {
		t.Fatalf("expected normalized runtime backend in health, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"environment":"local"`) {
		t.Fatalf("expected environment in health, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"network_policy_enforced":false`) {
		t.Fatalf("expected network enforcement flag in health, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"shutdown_timeout_secs":10`) {
		t.Fatalf("expected shutdown timeout in health, got %s", rr.Body.String())
	}
}

func TestRequestIDMiddlewareGeneratesRequestID(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	requestID := rr.Header().Get("X-Request-ID")
	if !strings.HasPrefix(requestID, "req_") {
		t.Fatalf("expected generated request ID header, got %q", requestID)
	}
}

func TestRequestIDMiddlewareSanitizesRequestID(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", " req_bad\nvalue\t ")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Header().Get("X-Request-ID") != "req_badvalue" {
		t.Fatalf("expected sanitized request ID, got %q", rr.Header().Get("X-Request-ID"))
	}
}

func TestRunEndpoint(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sandbox/run", strings.NewReader(`{"language":"python3","code":"print('ok')"}`))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid json, got %v", err)
	}

	if payload["message"] != "success" {
		t.Fatalf("expected success message, got %#v", payload["message"])
	}
}

func TestRunEndpointRejectsPreviewNetworkRequest(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sandbox/run", strings.NewReader(`{"language":"python3","code":"print('blocked')","enable_network":true}`))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "does not enforce network policy") {
		t.Fatalf("expected network enforcement error, got %s", rr.Body.String())
	}
}

func TestServerRejectsProductionPreviewBackend(t *testing.T) {
	cfg := testConfig(t)
	cfg.Environment = "production"
	cfg.RuntimeBackend = "preview"

	if _, err := NewServer(cfg); err == nil {
		t.Fatal("expected production preview backend to be rejected")
	}
}

func TestSandboxListEndpoint(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)

	if createRes.Code != http.StatusOK {
		t.Fatalf("expected sandbox create to return 200, got %d", createRes.Code)
	}
	if !strings.Contains(createRes.Body.String(), `"effective_limits"`) {
		t.Fatalf("expected sandbox create response to include effective limits, got %s", createRes.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/sandboxes", nil)
	listRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRes, listReq)

	if listRes.Code != http.StatusOK {
		t.Fatalf("expected sandbox list to return 200, got %d", listRes.Code)
	}
}

func TestSandboxCreateReturnsStructuredLimitError(t *testing.T) {
	cfg := testConfig(t)
	cfg.MaxActive = 1
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(firstRes, firstReq)
	if firstRes.Code != http.StatusOK {
		t.Fatalf("expected first sandbox create to return 200, got %d body=%s", firstRes.Code, firstRes.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(secondRes, secondReq)
	if secondRes.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second sandbox create to return 429, got %d body=%s", secondRes.Code, secondRes.Body.String())
	}
	if !strings.Contains(secondRes.Body.String(), `"error_type":"limit_exceeded"`) {
		t.Fatalf("expected structured limit error, got %s", secondRes.Body.String())
	}
	if !strings.Contains(secondRes.Body.String(), `"limit":"max_active_sandboxes"`) {
		t.Fatalf("expected max active limit details, got %s", secondRes.Body.String())
	}
}

func TestWriteKnownErrorMapsQueueTimeout(t *testing.T) {
	rr := httptest.NewRecorder()

	writeKnownError(rr, &runner.QueueTimeoutError{TimeoutMS: 50})

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"code":"execution_queue_timeout"`) {
		t.Fatalf("expected queue timeout details, got %s", rr.Body.String())
	}
}

func TestWriteKnownErrorMapsCancellation(t *testing.T) {
	rr := httptest.NewRecorder()

	writeKnownError(rr, &runner.CancellationError{Phase: "execution"})

	if rr.Code != 499 {
		t.Fatalf("expected 499, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"code":-499`) {
		t.Fatalf("expected cancellation code, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"error_type":"execution_canceled"`) {
		t.Fatalf("expected cancellation details, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"phase":"execution"`) {
		t.Fatalf("expected cancellation phase, got %s", rr.Body.String())
	}
}

func TestUploadFileRejectsOversizedRequestBody(t *testing.T) {
	cfg := testConfig(t)
	cfg.MaxFileSizeKB = 1
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	body := `{"sandbox_id":"sbx_missing","path":"too-large.txt","content":"` + strings.Repeat("x", 80*1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/files/upload", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestDependencyEndpoint(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sandbox/dependencies?language=python3", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected dependency list to return 200, got %d", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), "workflow-safe") {
		t.Fatalf("expected managed dependency profile in response, got %s", rr.Body.String())
	}
}

func TestMetricsEndpointReportsSandboxRunnerAndObserverCounters(t *testing.T) {
	cfg := testConfig(t)
	cfg.MaxWorkers = 2
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected sandbox create to return 200, got %d body=%s", createRes.Code, createRes.Body.String())
	}

	var createPayload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("expected create payload, got %v", err)
	}

	execReq := httptest.NewRequest(http.MethodPost, "/v1/exec/code", strings.NewReader(fmt.Sprintf(`{"sandbox_id":%q,"language":"python3","code":"print('metrics-ok')"}`, createPayload.Data.ID)))
	execReq.Header.Set("Content-Type", "application/json")
	execRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(execRes, execReq)
	if execRes.Code != http.StatusOK {
		t.Fatalf("expected code execution to return 200, got %d body=%s", execRes.Code, execRes.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/metrics", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected metrics to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Data struct {
			WorkerID        string `json:"worker_id"`
			ActiveSandboxes int    `json:"active_sandboxes"`
			Runner          struct {
				MaxWorkers       int   `json:"max_workers"`
				ActiveWorkers    int   `json:"active_workers"`
				QueuedExecutions int64 `json:"queued_executions"`
			} `json:"runner"`
			Observer struct {
				ExecutionSuccessCount      int     `json:"execution_success_count"`
				ExecutionFailureCount      int     `json:"execution_failure_count"`
				ExecutionDurationCount     int     `json:"execution_duration_count"`
				ExecutionDurationAverageMS float64 `json:"execution_duration_average_ms"`
			} `json:"observer"`
			ObserverRetention struct {
				RetentionDays int `json:"retention_days"`
				MaxEvents     int `json:"max_events"`
			} `json:"observer_retention"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected metrics payload, got %v", err)
	}
	if payload.Data.WorkerID != "test-worker" {
		t.Fatalf("expected worker id test-worker, got %q", payload.Data.WorkerID)
	}
	if payload.Data.ActiveSandboxes != 1 {
		t.Fatalf("expected one active sandbox, got %d", payload.Data.ActiveSandboxes)
	}
	if payload.Data.Runner.MaxWorkers != 2 {
		t.Fatalf("expected two max workers, got %d", payload.Data.Runner.MaxWorkers)
	}
	if payload.Data.Runner.ActiveWorkers != 0 {
		t.Fatalf("expected no active workers after request, got %d", payload.Data.Runner.ActiveWorkers)
	}
	if payload.Data.Runner.QueuedExecutions != 0 {
		t.Fatalf("expected no queued executions after request, got %d", payload.Data.Runner.QueuedExecutions)
	}
	if payload.Data.Observer.ExecutionSuccessCount != 1 {
		t.Fatalf("expected one execution success, got %d", payload.Data.Observer.ExecutionSuccessCount)
	}
	if payload.Data.Observer.ExecutionFailureCount != 0 {
		t.Fatalf("expected no execution failures, got %d", payload.Data.Observer.ExecutionFailureCount)
	}
	if payload.Data.Observer.ExecutionDurationCount != 1 {
		t.Fatalf("expected one duration sample, got %d", payload.Data.Observer.ExecutionDurationCount)
	}
	if payload.Data.ObserverRetention.RetentionDays != 7 {
		t.Fatalf("expected default observer retention 7 days, got %d", payload.Data.ObserverRetention.RetentionDays)
	}
	if payload.Data.ObserverRetention.MaxEvents != 10000 {
		t.Fatalf("expected default observer max events 10000, got %d", payload.Data.ObserverRetention.MaxEvents)
	}
}

func TestObserverEventsEndpointPaginatesWithCursor(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	server.observer.Record("sandbox.test", "sbx_page", "first", nil)
	time.Sleep(2 * time.Millisecond)
	server.observer.Record("sandbox.test", "sbx_page", "second", nil)
	time.Sleep(2 * time.Millisecond)
	server.observer.Record("sandbox.test", "sbx_page", "third", nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/observer/events?sandbox_id=sbx_page&limit=2", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected first page to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var firstPage struct {
		Data struct {
			Events []struct {
				Message string `json:"message"`
			} `json:"events"`
			Limit      int    `json:"limit"`
			HasMore    bool   `json:"has_more"`
			NextCursor string `json:"next_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &firstPage); err != nil {
		t.Fatalf("expected first page payload, got %v", err)
	}
	if firstPage.Data.Limit != 2 {
		t.Fatalf("expected limit 2, got %d", firstPage.Data.Limit)
	}
	if len(firstPage.Data.Events) != 2 {
		t.Fatalf("expected two first page events, got %d", len(firstPage.Data.Events))
	}
	if !firstPage.Data.HasMore {
		t.Fatal("expected first page to have more events")
	}
	if firstPage.Data.NextCursor == "" {
		t.Fatal("expected next cursor")
	}
	if firstPage.Data.Events[0].Message != "third" {
		t.Fatalf("expected newest event first, got %q", firstPage.Data.Events[0].Message)
	}

	nextReq := httptest.NewRequest(http.MethodGet, "/v1/observer/events?sandbox_id=sbx_page&limit=2&before="+url.QueryEscape(firstPage.Data.NextCursor), nil)
	nextRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(nextRes, nextReq)
	if nextRes.Code != http.StatusOK {
		t.Fatalf("expected second page to return 200, got %d body=%s", nextRes.Code, nextRes.Body.String())
	}

	var secondPage struct {
		Data struct {
			Events []struct {
				Message string `json:"message"`
			} `json:"events"`
			HasMore bool `json:"has_more"`
		} `json:"data"`
	}
	if err := json.Unmarshal(nextRes.Body.Bytes(), &secondPage); err != nil {
		t.Fatalf("expected second page payload, got %v", err)
	}
	if len(secondPage.Data.Events) != 1 {
		t.Fatalf("expected one second page event, got %d", len(secondPage.Data.Events))
	}
	if secondPage.Data.HasMore {
		t.Fatal("expected second page to be terminal")
	}
	if secondPage.Data.Events[0].Message != "first" {
		t.Fatalf("expected oldest remaining event, got %q", secondPage.Data.Events[0].Message)
	}
}

func TestObserverEventsEndpointRejectsNonGet(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/observer/events", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected method not allowed, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSandboxPersistenceAcrossServerInstances(t *testing.T) {
	cfg := testConfig(t)

	serverA, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected first server, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	serverA.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected create to return 200, got %d", createRes.Code)
	}

	serverB, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected second server, got %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/sandboxes", nil)
	listRes := httptest.NewRecorder()
	serverB.Handler().ServeHTTP(listRes, listReq)
	if !strings.Contains(listRes.Body.String(), `"runtime_profile":"session"`) {
		t.Fatalf("expected persisted sandbox, got %s", listRes.Body.String())
	}
}

func TestInteractiveProxyRoutesToRegisteredEndpoint(t *testing.T) {
	cfg := testConfig(t)
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "proxy:%s", r.URL.Path)
	}))
	defer targetServer.Close()

	targetURL, err := url.Parse(targetServer.URL)
	if err != nil {
		t.Fatalf("expected target url, got %v", err)
	}
	targetPort, err := strconv.Atoi(targetURL.Port())
	if err != nil {
		t.Fatalf("expected numeric port, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"interactive","ttl_seconds":60}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected interactive sandbox create to return 200, got %d", createRes.Code)
	}

	var createPayload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("expected create payload, got %v", err)
	}

	registerReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes/"+createPayload.Data.ID+"/endpoints/3010", strings.NewReader(fmt.Sprintf(`{"target_host":"127.0.0.1","target_port":%d,"scheme":"http"}`, targetPort)))
	registerReq.Header.Set("Content-Type", "application/json")
	registerRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(registerRes, registerReq)
	if registerRes.Code != http.StatusOK {
		t.Fatalf("expected endpoint register to return 200, got %d", registerRes.Code)
	}

	proxyReq := httptest.NewRequest(http.MethodGet, "/_zgi/ports/"+createPayload.Data.ID+"/3010/hello", nil)
	proxyRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(proxyRes, proxyReq)
	if proxyRes.Code != http.StatusOK {
		t.Fatalf("expected interactive proxy to return 200, got %d", proxyRes.Code)
	}
	if !strings.Contains(proxyRes.Body.String(), "proxy:/hello") {
		t.Fatalf("expected proxied response body, got %s", proxyRes.Body.String())
	}
}

func testConfig(t *testing.T) config.Config {
	t.Helper()

	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.DatabaseURL = testutil.CreateTestPostgresDSN(t)
	cfg.WorkerID = "test-worker"
	cfg.AdvertiseURL = "http://127.0.0.1:2660"
	cfg.PublicBaseURL = cfg.AdvertiseURL
	cfg.Environment = "local"
	cfg.RedisAddr = ""
	cfg.RuntimeBackend = "preview"
	return cfg
}
