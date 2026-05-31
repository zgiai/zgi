package app

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
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

func TestReadyEndpoint(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"ready"`) {
		t.Fatalf("expected ready status, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"ready":true`) {
		t.Fatalf("expected ready flag, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"postgres":"ok"`) {
		t.Fatalf("expected postgres check, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"runtime":"ok"`) {
		t.Fatalf("expected runtime check, got %s", rr.Body.String())
	}
}

func TestReadyEndpointReportsStoreFailure(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}
	if err := server.store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"not_ready"`) {
		t.Fatalf("expected not ready status, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"ready":false`) {
		t.Fatalf("expected not ready flag, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"postgres":"error"`) {
		t.Fatalf("expected postgres error check, got %s", rr.Body.String())
	}
}

func TestReadyEndpointRejectsNonGet(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/ready", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", rr.Code, rr.Body.String())
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

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60,"organization_id":"organization-api","workspace_id":"workspace-api","app_id":"app-api","workflow_run_id":"run-api","user_id":"user-api"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)

	if createRes.Code != http.StatusOK {
		t.Fatalf("expected sandbox create to return 200, got %d", createRes.Code)
	}
	if !strings.Contains(createRes.Body.String(), `"effective_limits"`) {
		t.Fatalf("expected sandbox create response to include effective limits, got %s", createRes.Body.String())
	}
	for _, expected := range []string{
		`"organization_id":"organization-api"`,
		`"workspace_id":"workspace-api"`,
		`"app_id":"app-api"`,
		`"workflow_run_id":"run-api"`,
		`"user_id":"user-api"`,
	} {
		if !strings.Contains(createRes.Body.String(), expected) {
			t.Fatalf("expected create response to include %s, got %s", expected, createRes.Body.String())
		}
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/sandboxes", nil)
	listRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRes, listReq)

	if listRes.Code != http.StatusOK {
		t.Fatalf("expected sandbox list to return 200, got %d", listRes.Code)
	}
	if !strings.Contains(listRes.Body.String(), `"organization_id":"organization-api"`) {
		t.Fatalf("expected sandbox list to include ownership fields, got %s", listRes.Body.String())
	}
}

func TestSandboxCreateRejectsInvalidOwnershipField(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","organization_id":"organization api"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)

	if createRes.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid ownership field to return 400, got %d body=%s", createRes.Code, createRes.Body.String())
	}
	if !strings.Contains(createRes.Body.String(), "organization_id contains invalid characters") {
		t.Fatalf("expected ownership validation error, got %s", createRes.Body.String())
	}
}

func TestTemplateEndpointRendersAndRejectsUnsafeHelpers(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/exec/template", strings.NewReader(`{"template":"Hello {{ upper .name }}","variables":{"name":"zgi"}}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected template render to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"content":"Hello ZGI"`) {
		t.Fatalf("expected rendered content, got %s", rr.Body.String())
	}

	rejectReq := httptest.NewRequest(http.MethodPost, "/v1/exec/template", strings.NewReader(`{"template":"{{ env \"HOME\" }}","variables":{}}`))
	rejectReq.Header.Set("Content-Type", "application/json")
	rejectRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(rejectRes, rejectReq)
	if rejectRes.Code != http.StatusBadRequest {
		t.Fatalf("expected unsafe helper to return 400, got %d body=%s", rejectRes.Code, rejectRes.Body.String())
	}
}

func TestSkillEndpointRunsManifestPackage(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60}`))
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected sandbox create to return 200, got %d body=%s", createRes.Code, createRes.Body.String())
	}
	var createBody struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &createBody); err != nil {
		t.Fatalf("decode sandbox create: %v", err)
	}

	archive := testZipBase64(t, map[string]string{
		"SKILL.md":       "skill",
		"scripts/run.py": "import json, os, sys\npayload = json.loads(sys.stdin.read() or '{}')\nos.makedirs('artifacts', exist_ok=True)\nopen('artifacts/report.txt', 'w').write('api artifact\\n')\nprint(json.dumps({'ok': True, 'input': payload.get('input')}))\n",
		"skill.manifest.json": `{
  "entrypoint": "scripts/run.py",
  "language": "python3",
  "timeout_ms": 30000,
  "allowed_artifact_paths": ["artifacts"],
  "max_artifact_count": 10,
  "max_artifact_bytes": 32768,
  "result_mode": "mixed"
}`,
	})
	uploadBody := fmt.Sprintf(`{"sandbox_id":%q,"path":"skills/api","archive_base64":%q,"format":"zip","validate_skill_manifest":true}`, createBody.Data.ID, archive)
	uploadReq := httptest.NewRequest(http.MethodPost, "/v1/files/upload-archive", strings.NewReader(uploadBody))
	uploadReq.Header.Set("Content-Type", "application/json")
	uploadRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(uploadRes, uploadReq)
	if uploadRes.Code != http.StatusOK {
		t.Fatalf("expected archive upload to return 200, got %d body=%s", uploadRes.Code, uploadRes.Body.String())
	}

	runReq := httptest.NewRequest(http.MethodPost, "/v1/exec/skill", strings.NewReader(fmt.Sprintf(`{"sandbox_id":%q,"path":"skills/api","input_json":{"input":"api"}}`, createBody.Data.ID)))
	runReq.Header.Set("Content-Type", "application/json")
	runRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(runRes, runReq)
	if runRes.Code != http.StatusOK {
		t.Fatalf("expected skill execution to return 200, got %d body=%s", runRes.Code, runRes.Body.String())
	}
	for _, expected := range []string{`"entrypoint":"scripts/run.py"`, `"artifact_manifests"`, `"result_json":{"input":"api","ok":true}`} {
		if !strings.Contains(runRes.Body.String(), expected) {
			t.Fatalf("expected skill response to include %s, got %s", expected, runRes.Body.String())
		}
	}
}

func TestFileManifestEndpointEnforcesArtifactLimits(t *testing.T) {
	server, err := NewServer(testConfig(t))
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

	uploadReq := httptest.NewRequest(http.MethodPost, "/v1/files/upload", strings.NewReader(fmt.Sprintf(`{"sandbox_id":%q,"path":"artifacts/report.txt","content":"hello manifest"}`, createPayload.Data.ID)))
	uploadReq.Header.Set("Content-Type", "application/json")
	uploadRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(uploadRes, uploadReq)
	if uploadRes.Code != http.StatusOK {
		t.Fatalf("expected upload to return 200, got %d body=%s", uploadRes.Code, uploadRes.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/files/manifest?sandbox_id="+url.QueryEscape(createPayload.Data.ID)+"&path=artifacts&max_total_bytes=4", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected artifact byte limit to return 429, got %d body=%s", rr.Code, rr.Body.String())
	}
	for _, expected := range []string{
		`"error_type":"limit_exceeded"`,
		`"code":"artifact_manifest_total_bytes_exceeded"`,
		`"limit":"max_artifact_manifest_total_bytes"`,
	} {
		if !strings.Contains(rr.Body.String(), expected) {
			t.Fatalf("expected response to include %s, got %s", expected, rr.Body.String())
		}
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

func TestSandboxCreateReturnsStructuredOrganizationLimitError(t *testing.T) {
	cfg := testConfig(t)
	cfg.MaxActive = 10
	cfg.MaxActivePerOrganization = 1
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60,"organization_id":"organization-api"}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(firstRes, firstReq)
	if firstRes.Code != http.StatusOK {
		t.Fatalf("expected first organization sandbox create to return 200, got %d body=%s", firstRes.Code, firstRes.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60,"organization_id":"organization-api"}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(secondRes, secondReq)
	if secondRes.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second organization sandbox create to return 429, got %d body=%s", secondRes.Code, secondRes.Body.String())
	}
	for _, expected := range []string{
		`"error_type":"limit_exceeded"`,
		`"code":"organization_active_sandbox_limit_exceeded"`,
		`"limit":"max_active_sandboxes_per_organization"`,
		`"organization_id":"organization-api"`,
	} {
		if !strings.Contains(secondRes.Body.String(), expected) {
			t.Fatalf("expected organization limit response to include %s, got %s", expected, secondRes.Body.String())
		}
	}

	thirdReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60,"organization_id":"organization-other"}`))
	thirdReq.Header.Set("Content-Type", "application/json")
	thirdRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(thirdRes, thirdReq)
	if thirdRes.Code != http.StatusOK {
		t.Fatalf("expected other organization sandbox create to return 200, got %d body=%s", thirdRes.Code, thirdRes.Body.String())
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

func TestObserverEventsEndpointFiltersByOwnershipScope(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	server.observer.Record("exec.command", "sbx_scope_one", "match", map[string]any{
		"organization_id": "organization-one",
		"workspace_id":    "workspace-one",
		"app_id":          "app-one",
		"workflow_run_id": "run-one",
		"user_id":         "user-one",
	})
	server.observer.Record("exec.command", "sbx_scope_two", "miss", map[string]any{
		"organization_id": "organization-two",
		"workspace_id":    "workspace-two",
		"app_id":          "app-two",
		"workflow_run_id": "run-two",
		"user_id":         "user-two",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/observer/events?organization_id=organization-one&workspace_id=workspace-one&app_id=app-one&workflow_run_id=run-one&user_id=user-one&limit=10", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected scope filter to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Data struct {
			Events []struct {
				Message string `json:"message"`
			} `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected observer payload, got %v", err)
	}
	if len(payload.Data.Events) != 1 {
		t.Fatalf("expected one scoped event, got %d", len(payload.Data.Events))
	}
	if payload.Data.Events[0].Message != "match" {
		t.Fatalf("expected matching event, got %q", payload.Data.Events[0].Message)
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

func testZipBase64(t *testing.T, files map[string]string) string {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for path, content := range files {
		fileWriter, err := writer.Create(path)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := fileWriter.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes())
}
