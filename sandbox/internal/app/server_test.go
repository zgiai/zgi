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

	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/testutil"
)

func TestHealthEndpoint(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
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

	listReq := httptest.NewRequest(http.MethodGet, "/v1/sandboxes", nil)
	listRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRes, listReq)

	if listRes.Code != http.StatusOK {
		t.Fatalf("expected sandbox list to return 200, got %d", listRes.Code)
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
