package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/zgiai/zgi/runner/internal/config"
	"github.com/zgiai/zgi/runner/internal/manager"
	"github.com/zgiai/zgi/runner/internal/plugin"
	"github.com/zgiai/zgi/runner/internal/registry"
	"github.com/zgiai/zgi/runner/internal/runtime"
	"github.com/zgiai/zgi/runner/internal/runtime/local"
	"github.com/zgiai/zgi/runner/internal/storage"
)

// TestE2E_UVEchoPlugin tests the complete lifecycle of the uv-echo plugin:
// 1. Register the plugin
// 2. Install the plugin (with dependency installation)
// 3. Launch a session
// 4. Invoke tools
// 5. Stop the session
func TestE2E_UVEchoPlugin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	// Check prerequisites
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}

	// Setup test server with real runtime
	srv, ts := setupE2ETestServer(t)
	_ = srv

	// Step 1: Register the plugin
	t.Log("Step 1: Registering uv-echo plugin...")
	createPayload := map[string]interface{}{
		"manifest": map[string]interface{}{
			"name":    "uv-echo",
			"version": "0.0.1",
			"author":  "vic",
			"runner": map[string]interface{}{
				"language":   "python",
				"entrypoint": "main_runner",
			},
		},
	}

	resp := doE2ERequest(t, ts, "POST", "/api/v1/plugins", createPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("register plugin failed: %d: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
	t.Log("  Plugin registered successfully")

	// Step 2: Create plugin package and install
	t.Log("Step 2: Installing uv-echo plugin...")
	pluginDir := findUVEchoPluginDir(t)
	pkgBytes := zipDirectory(t, pluginDir)
	pkgBase64 := base64.StdEncoding.EncodeToString(pkgBytes)

	installPayload := map[string]interface{}{
		"package_b64": pkgBase64,
	}

	resp = doE2ERequest(t, ts, "POST", "/api/v1/plugins/uv-echo:0.0.1/install", installPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("install plugin failed: %d: %s", resp.StatusCode, string(body))
	}

	var installation manager.Installation
	readE2EJSON(t, resp, &installation)

	if installation.Status != "installed" {
		t.Errorf("expected status 'installed', got %q", installation.Status)
	}
	if installation.Manifest.Name != "uv-echo" {
		t.Errorf("expected manifest name 'uv-echo', got %q", installation.Manifest.Name)
	}
	t.Logf("  Plugin installed at: %s", installation.Path)

	// Verify workspace was created
	if _, err := os.Stat(installation.Path); os.IsNotExist(err) {
		t.Fatalf("workspace not created at %s", installation.Path)
	}

	// Verify venv was created
	venvPath := filepath.Join(installation.Path, ".venv")
	if _, err := os.Stat(venvPath); os.IsNotExist(err) {
		t.Fatalf("venv not created at %s", venvPath)
	}
	t.Log("  Virtual environment created successfully")

	// Step 3: Verify in installed list
	t.Log("Step 3: Verifying plugin in installed list...")
	resp = doE2ERequest(t, ts, "GET", "/api/v1/plugins/installed", nil, "test-admin-key")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list installed failed: %d", resp.StatusCode)
	}

	var installedList []manager.Installation
	readE2EJSON(t, resp, &installedList)

	found := false
	for _, inst := range installedList {
		if inst.Manifest.Name == "uv-echo" && inst.Manifest.Version == "0.0.1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("installed plugin not found in list")
	}
	t.Log("  Plugin found in installed list")

	// Step 4: Launch a session
	t.Log("Step 4: Launching plugin session...")
	launchPayload := map[string]interface{}{
		"name":       "uv-echo",
		"version":    "0.0.1",
		"language":   "python",
		"entrypoint": "main_runner",
		"workingDir": installation.Path,
	}

	resp = doE2ERequest(t, ts, "POST", "/api/v1/sessions", launchPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("launch session failed: %d: %s", resp.StatusCode, string(body))
	}

	var snapshot runtime.Snapshot
	readE2EJSON(t, resp, &snapshot)

	sessionID := snapshot.ID
	t.Logf("  Session launched with ID: %s, PID: %d", sessionID, snapshot.PID)

	// Wait for the plugin to be ready
	t.Log("Step 5: Waiting for plugin ready...")
	ready := waitForPluginReady(t, ts, sessionID, 30*time.Second)
	if !ready {
		t.Fatalf("plugin did not become ready within timeout")
	}
	t.Log("  Plugin is ready!")

	// Step 6: Invoke tool - list_tools
	t.Log("Step 6: Invoking list_tools action...")
	invokePayload := map[string]interface{}{
		"session_id": sessionID,
		"action":     "list_tools",
		"timeout":    30,
	}

	resp = doE2ERequest(t, ts, "POST", "/api/v1/invoke", invokePayload, "test-admin-key")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("invoke list_tools failed: %d: %s", resp.StatusCode, string(body))
	}

	var listToolsResp InvokeResponse
	readE2EJSON(t, resp, &listToolsResp)

	if !listToolsResp.Success {
		t.Fatalf("list_tools failed: %s", listToolsResp.Error)
	}
	t.Logf("  list_tools response: %+v", listToolsResp.Data)

	// Step 7: Invoke tool - echo_http
	t.Log("Step 7: Invoking echo_http tool...")
	toolInvokePayload := map[string]interface{}{
		"session_id": sessionID,
		"provider":   "echo",
		"tool":       "echo_http",
		"parameters": map[string]interface{}{
			"url":     "https://httpbin.org/get",
			"message": "test-from-e2e",
		},
		"timeout": 30,
	}

	resp = doE2ERequest(t, ts, "POST", "/api/v1/invoke/tool", toolInvokePayload, "test-admin-key")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("invoke echo_http failed: %d: %s", resp.StatusCode, string(body))
	}

	var echoResp InvokeResponse
	readE2EJSON(t, resp, &echoResp)

	if !echoResp.Success {
		t.Fatalf("echo_http failed: %s", echoResp.Error)
	}
	t.Logf("  echo_http response: %+v", echoResp.Data)

	// Verify response data structure
	if data, ok := echoResp.Data.(map[string]interface{}); ok {
		if statusCode, exists := data["status_code"]; exists {
			t.Logf("  HTTP status code: %v", statusCode)
		}
		if message, exists := data["message"]; exists {
			if message != "test-from-e2e" {
				t.Errorf("expected message 'test-from-e2e', got %v", message)
			}
		}
	}

	// Step 8: Stop the session
	t.Log("Step 8: Stopping session...")
	resp = doE2ERequest(t, ts, "POST", fmt.Sprintf("/api/v1/sessions/%s/stop", sessionID), nil, "test-admin-key")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("stop session response: %d: %s (may be already stopped)", resp.StatusCode, string(body))
	} else {
		t.Log("  Session stopped successfully")
	}

	// Step 9: Verify session list
	t.Log("Step 9: Final verification...")
	resp = doE2ERequest(t, ts, "GET", "/api/v1/sessions", nil, "test-admin-key")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list sessions failed: %d", resp.StatusCode)
	}

	var sessions []runtime.Snapshot
	readE2EJSON(t, resp, &sessions)
	t.Logf("  Current sessions count: %d", len(sessions))

	t.Log("E2E test completed successfully!")
}

// TestE2E_UVEchoPlugin_WithLocalURL tests echo_http with a local httptest server
func TestE2E_UVEchoPlugin_WithLocalURL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}

	// Start a local test HTTP server
	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"test": "hello-from-local-server"}`))
	}))
	defer localServer.Close()

	srv, ts := setupE2ETestServer(t)
	_ = srv

	// Register plugin
	createPayload := map[string]interface{}{
		"manifest": map[string]interface{}{
			"name":    "uv-echo",
			"version": "0.0.1",
			"author":  "vic",
			"runner": map[string]interface{}{
				"language":   "python",
				"entrypoint": "main_runner",
			},
		},
	}
	resp := doE2ERequest(t, ts, "POST", "/api/v1/plugins", createPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("register failed: %d: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	// Install plugin
	pluginDir := findUVEchoPluginDir(t)
	pkgBytes := zipDirectory(t, pluginDir)
	pkgBase64 := base64.StdEncoding.EncodeToString(pkgBytes)

	installPayload := map[string]interface{}{"package_b64": pkgBase64}
	resp = doE2ERequest(t, ts, "POST", "/api/v1/plugins/uv-echo:0.0.1/install", installPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("install failed: %d: %s", resp.StatusCode, string(body))
	}

	var installation manager.Installation
	readE2EJSON(t, resp, &installation)

	// Launch session
	launchPayload := map[string]interface{}{
		"name":       "uv-echo",
		"version":    "0.0.1",
		"language":   "python",
		"entrypoint": "main_runner",
		"workingDir": installation.Path,
	}
	resp = doE2ERequest(t, ts, "POST", "/api/v1/sessions", launchPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("launch failed: %d: %s", resp.StatusCode, string(body))
	}

	var snapshot runtime.Snapshot
	readE2EJSON(t, resp, &snapshot)
	sessionID := snapshot.ID

	// Wait for ready
	if !waitForPluginReady(t, ts, sessionID, 30*time.Second) {
		t.Fatalf("plugin did not become ready")
	}

	// Invoke with local server URL
	toolInvokePayload := map[string]interface{}{
		"session_id": sessionID,
		"provider":   "echo",
		"tool":       "echo_http",
		"parameters": map[string]interface{}{
			"url":     localServer.URL,
			"message": "local-test-message",
		},
		"timeout": 30,
	}

	resp = doE2ERequest(t, ts, "POST", "/api/v1/invoke/tool", toolInvokePayload, "test-admin-key")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("invoke failed: %d: %s", resp.StatusCode, string(body))
	}

	var echoResp InvokeResponse
	readE2EJSON(t, resp, &echoResp)

	if !echoResp.Success {
		t.Fatalf("echo_http with local URL failed: %s", echoResp.Error)
	}

	t.Logf("Local URL test response: %+v", echoResp.Data)

	if data, ok := echoResp.Data.(map[string]interface{}); ok {
		if statusCode, exists := data["status_code"]; exists {
			if statusCode != float64(200) {
				t.Errorf("expected status_code 200, got %v", statusCode)
			}
		}
		if message, exists := data["message"]; exists {
			if message != "local-test-message" {
				t.Errorf("expected message 'local-test-message', got %v", message)
			}
		}
	}

	// Test regex_extract tool
	t.Log("Testing regex_extract tool...")
	regexPayload := map[string]interface{}{
		"session_id": sessionID,
		"provider":   "echo",
		"tool":       "regex_extract",
		"parameters": map[string]interface{}{
			"content":    "Hello world! My email is test@example.com and another@domain.org",
			"expression": `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`,
		},
		"timeout": 30,
	}

	resp = doE2ERequest(t, ts, "POST", "/api/v1/invoke/tool", regexPayload, "test-admin-key")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("invoke regex_extract failed: %d: %s", resp.StatusCode, string(body))
	}

	var regexResp InvokeResponse
	readE2EJSON(t, resp, &regexResp)

	if !regexResp.Success {
		t.Fatalf("regex_extract failed: %s", regexResp.Error)
	}

	t.Logf("regex_extract response: %+v", regexResp.Data)

	if data, ok := regexResp.Data.(map[string]interface{}); ok {
		if count, exists := data["count"]; exists {
			if count != float64(2) {
				t.Errorf("expected count 2, got %v", count)
			}
		}
		if matches, exists := data["matches"]; exists {
			matchList, _ := matches.([]interface{})
			t.Logf("  Found %d matches: %v", len(matchList), matchList)
		}
	}
}

// Helper functions

func e2eTestConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		HTTPPort:             0,
		PluginHome:           t.TempDir(),
		WorkspacePath:        t.TempDir(),
		PackageCachePath:     t.TempDir(),
		PythonInterpreter:    "python3",
		NodeInterpreter:      "node",
		PipCommand:           "pip",
		NPMCommand:           "npm",
		PythonEnvInitTimeout: 10 * time.Minute,
		InstallTimeout:       5 * time.Minute,
		StdoutBufferSize:     4096,
		StdoutMaxBufferSize:  5 * 1024 * 1024,
		APIKey:               "",
		AdminAPIKeys:         "test-admin-key",
		ReadonlyAPIKeys:      "test-readonly-key",
		ShutdownTimeout:      30 * time.Second,
	}
}

// e2eStore implements storage.Store for e2e testing.
type e2eStore struct {
	workspaceRoot string
}

func newE2EStore(t *testing.T) *e2eStore {
	t.Helper()
	return &e2eStore{workspaceRoot: t.TempDir()}
}

func (m *e2eStore) SavePackage(ctx context.Context, manifest plugin.Manifest, pkg []byte) (string, error) {
	targetDir := m.Workspace(manifest)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", err
	}

	if len(pkg) == 0 {
		return targetDir, nil
	}

	readerAt := bytes.NewReader(pkg)
	archive, err := zip.NewReader(readerAt, int64(len(pkg)))
	if err != nil {
		return "", err
	}

	for _, file := range archive.File {
		path := filepath.Join(targetDir, file.Name)
		if file.FileInfo().IsDir() {
			_ = os.MkdirAll(path, file.Mode())
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		rc, err := file.Open()
		if err != nil {
			return "", err
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
		if err != nil {
			rc.Close()
			return "", err
		}
		_, _ = io.Copy(out, rc)
		rc.Close()
		out.Close()
	}

	return targetDir, nil
}

func (m *e2eStore) Remove(ctx context.Context, manifest plugin.Manifest) error {
	return os.RemoveAll(m.Workspace(manifest))
}

func (m *e2eStore) Workspace(manifest plugin.Manifest) string {
	return filepath.Join(m.workspaceRoot, manifest.Name+"-"+manifest.Version)
}

var _ storage.Store = (*e2eStore)(nil)

func setupE2ETestServer(t *testing.T) (*HTTPServer, *httptest.Server) {
	t.Helper()

	cfg := e2eTestConfig(t)
	store := newE2EStore(t)
	log := zap.NewExample() // Use Example logger for debugging
	reg := registry.New(nil, log)

	// Use real local runtime instead of mock
	rt := local.NewRuntime(cfg, log.Named("runtime"))
	mgr := manager.New(cfg, rt, store, log, nil, nil, nil)

	srv := New(cfg, mgr, reg, log)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ts := &httptest.Server{
		Listener: listener,
		Config: &http.Server{
			Handler: srv.engine,
		},
	}
	ts.Start()

	t.Cleanup(func() {
		ts.Close()
	})

	return srv, ts
}

func findUVEchoPluginDir(t *testing.T) string {
	t.Helper()
	// Try relative path from project root
	candidates := []string{
		"examples/test_plugin/uv_echo_0.0.1",
		"../../examples/test_plugin/uv_echo_0.0.1",
		"../../../examples/test_plugin/uv_echo_0.0.1",
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			absPath, _ := filepath.Abs(candidate)
			return absPath
		}
	}

	// Try using go mod to find workspace root
	wd, _ := os.Getwd()
	for d := wd; d != "/"; d = filepath.Dir(d) {
		pluginDir := filepath.Join(d, "examples/test_plugin/uv_echo_0.0.1")
		if info, err := os.Stat(pluginDir); err == nil && info.IsDir() {
			return pluginDir
		}
	}

	t.Fatalf("uv_echo plugin directory not found")
	return ""
}

func doE2ERequest(t *testing.T, ts *httptest.Server, method, path string, payload interface{}, apiKey string) *http.Response {
	t.Helper()

	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, ts.URL+path, body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}

	return resp
}

func readE2EJSON(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("unmarshal json: %v, body: %s", err, string(body))
	}
}

func waitForPluginReady(t *testing.T, ts *httptest.Server, sessionID string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp := doE2ERequest(t, ts, "GET", fmt.Sprintf("/api/v1/invoke/sessions/%s/ready", sessionID), nil, "test-admin-key")
		if resp.StatusCode == http.StatusOK {
			var result map[string]interface{}
			readE2EJSON(t, resp, &result)
			if ready, ok := result["ready"].(bool); ok && ready {
				return true
			}
		} else {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}

	return false
}
