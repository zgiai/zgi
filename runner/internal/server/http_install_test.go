package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
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
	"github.com/zgiai/zgi/runner/internal/storage"
)

// mockStore implements storage.Store for testing.
type mockStore struct {
	workspaceRoot string
}

func newMockStore(t *testing.T) *mockStore {
	t.Helper()
	return &mockStore{workspaceRoot: t.TempDir()}
}

func (m *mockStore) SavePackage(ctx context.Context, manifest plugin.Manifest, pkg []byte) (string, error) {
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

func (m *mockStore) Remove(ctx context.Context, manifest plugin.Manifest) error {
	return os.RemoveAll(m.Workspace(manifest))
}

func (m *mockStore) Workspace(manifest plugin.Manifest) string {
	return filepath.Join(m.workspaceRoot, manifest.Name+"-"+manifest.Version)
}

var _ storage.Store = (*mockStore)(nil)

// mockRuntime implements runtime.Runtime for testing.
type mockRuntime struct{}

func (m *mockRuntime) Start(ctx context.Context, req runtime.StartRequest) (*runtime.Session, error) {
	session := runtime.NewSession(req.Manifest, req.WorkingDir)
	session.MarkRunning(12345)
	return session, nil
}

var _ runtime.Runtime = (*mockRuntime)(nil)

// testConfig returns a minimal config for API testing.
func testConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		HTTPPort:             0, // Will use httptest
		PluginHome:           t.TempDir(),
		WorkspacePath:        t.TempDir(),
		PackageCachePath:     t.TempDir(),
		PythonInterpreter:    "python3",
		NodeInterpreter:      "node",
		PipCommand:           "pip",
		NPMCommand:           "npm",
		PythonEnvInitTimeout: 10 * time.Minute,
		InstallTimeout:       2 * time.Minute,
		StdoutBufferSize:     4096,
		StdoutMaxBufferSize:  5 * 1024 * 1024,
		APIKey:               "",                  // No default API key
		AdminAPIKeys:         "test-admin-key",    // Admin access
		ReadonlyAPIKeys:      "test-readonly-key", // Read-only access
	}
}

// setupTestServer creates a test HTTP server with all dependencies.
func setupTestServer(t *testing.T) (*HTTPServer, *httptest.Server) {
	t.Helper()

	cfg := testConfig(t)
	store := newMockStore(t)
	log := zap.NewNop()
	reg := registry.New(nil, log) // in-memory registry
	mgr := manager.New(cfg, &mockRuntime{}, store, log, nil, nil, nil)

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

// createMinimalPluginZip creates a minimal valid plugin zip with requirements.txt.
func createMinimalPluginZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	mainPy, _ := w.Create("main.py")
	_, _ = mainPy.Write([]byte("# minimal plugin\n"))

	reqTxt, _ := w.Create("requirements.txt")
	_, _ = reqTxt.Write([]byte("# no dependencies\n"))

	_ = w.Close()
	return buf.Bytes()
}

// zipDirectory creates a zip archive from a directory.
func zipDirectory(t *testing.T, srcDir string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	_ = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(srcDir, path)
		if relPath == "." {
			return nil
		}
		// Skip hidden directories
		if info.IsDir() && len(info.Name()) > 0 && info.Name()[0] == '.' {
			return filepath.SkipDir
		}

		header, _ := zip.FileInfoHeader(info)
		header.Name = relPath
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, _ := w.CreateHeader(header)
		if !info.IsDir() {
			file, _ := os.Open(path)
			_, _ = io.Copy(writer, file)
			file.Close()
		}
		return nil
	})

	_ = w.Close()
	return buf.Bytes()
}

func doRequest(t *testing.T, ts *httptest.Server, method, path string, body interface{}, apiKey string) *http.Response {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, ts.URL+path, reqBody)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	if body != nil {
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

func readJSON(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("unmarshal response: %v, body: %s", err, string(body))
	}
}

// ============================================================
// API Integration Tests
// ============================================================

func TestAPI_HealthCheck(t *testing.T) {
	_, ts := setupTestServer(t)

	resp := doRequest(t, ts, "GET", "/healthz", nil, "test-admin-key")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	readJSON(t, resp, &result)
	if result["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", result["status"])
	}
}

func TestAPI_CreatePlugin(t *testing.T) {
	_, ts := setupTestServer(t)

	payload := map[string]interface{}{
		"manifest": map[string]interface{}{
			"name":    "test-plugin",
			"version": "1.0.0",
			"runner": map[string]interface{}{
				"language":   "python",
				"entrypoint": "main",
			},
		},
	}

	resp := doRequest(t, ts, "POST", "/api/v1/plugins", payload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}

	var result plugin.Manifest
	readJSON(t, resp, &result)

	if result.Name != "test-plugin" {
		t.Errorf("expected name 'test-plugin', got %q", result.Name)
	}
	if result.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", result.Version)
	}
}

func TestAPI_CreatePlugin_ValidationError(t *testing.T) {
	_, ts := setupTestServer(t)

	// Missing required fields
	payload := map[string]interface{}{
		"manifest": map[string]interface{}{
			"name": "test-plugin",
			// missing version and runner
		},
	}

	resp := doRequest(t, ts, "POST", "/api/v1/plugins", payload, "test-admin-key")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestAPI_InstallPlugin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}

	_, ts := setupTestServer(t)

	// Step 1: Create/register the plugin
	createPayload := map[string]interface{}{
		"manifest": map[string]interface{}{
			"name":    "test-plugin",
			"version": "1.0.0",
			"runner": map[string]interface{}{
				"language":   "python",
				"entrypoint": "main",
			},
		},
	}

	resp := doRequest(t, ts, "POST", "/api/v1/plugins", createPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create plugin failed: %d: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	// Step 2: Install the plugin with package
	pkg := createMinimalPluginZip(t)
	pkgBase64 := base64.StdEncoding.EncodeToString(pkg)

	installPayload := map[string]interface{}{
		"package_b64": pkgBase64,
	}

	resp = doRequest(t, ts, "POST", "/api/v1/plugins/test-plugin:1.0.0/install", installPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("install plugin failed: %d: %s", resp.StatusCode, string(body))
	}

	var installation manager.Installation
	readJSON(t, resp, &installation)

	if installation.Status != "installed" {
		t.Errorf("expected status 'installed', got %q", installation.Status)
	}
	if installation.Manifest.Name != "test-plugin" {
		t.Errorf("expected manifest name 'test-plugin', got %q", installation.Manifest.Name)
	}

	// Step 3: Verify in installed list
	resp = doRequest(t, ts, "GET", "/api/v1/plugins/installed", nil, "test-admin-key")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list installed failed: %d", resp.StatusCode)
	}

	var installedList []manager.Installation
	readJSON(t, resp, &installedList)

	found := false
	for _, inst := range installedList {
		if inst.Manifest.Name == "test-plugin" && inst.Manifest.Version == "1.0.0" {
			found = true
			break
		}
	}
	if !found {
		t.Error("installed plugin not found in list")
	}
}

func TestAPI_InstallPlugin_Multipart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}

	_, ts := setupTestServer(t)

	createPayload := map[string]interface{}{
		"manifest": map[string]interface{}{
			"name":    "multipart-plugin",
			"version": "1.0.0",
			"runner": map[string]interface{}{
				"language":   "python",
				"entrypoint": "main",
			},
		},
	}

	resp := doRequest(t, ts, "POST", "/api/v1/plugins", createPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create plugin failed: %d: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	pkg := createMinimalPluginZip(t)
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "plugin.zip")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(pkg); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req, err := http.NewRequest("POST", ts.URL+"/api/v1/plugins/multipart-plugin:1.0.0/install", &buf)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-API-Key", "test-admin-key")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("install plugin failed: %d: %s", resp.StatusCode, string(body))
	}

	var installation manager.Installation
	readJSON(t, resp, &installation)

	if installation.Manifest.Name != "multipart-plugin" {
		t.Errorf("expected manifest name 'multipart-plugin', got %q", installation.Manifest.Name)
	}
	if installation.Manifest.Version != "1.0.0" {
		t.Errorf("expected manifest version '1.0.0', got %q", installation.Manifest.Version)
	}
	if installation.Status != "installed" {
		t.Errorf("expected status 'installed', got %q", installation.Status)
	}
}

func TestAPI_InstallRegexPlugin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}

	// Locate the regex plugin directory
	regexPluginDir := filepath.Join("..", "..", "examples", "regex", "zgi-regex_0.0.3")
	if _, err := os.Stat(regexPluginDir); os.IsNotExist(err) {
		t.Skipf("regex plugin not found at %s", regexPluginDir)
	}

	_, ts := setupTestServer(t)

	// Step 1: Create/register the regex plugin
	createPayload := map[string]interface{}{
		"manifest": map[string]interface{}{
			"name":    "regex",
			"version": "0.0.3",
			"author":  "zgi",
			"runner": map[string]interface{}{
				"language":   "python",
				"entrypoint": "main",
			},
			"tags": []string{"utilities", "productivity"},
		},
	}

	resp := doRequest(t, ts, "POST", "/api/v1/plugins", createPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create plugin failed: %d: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	// Step 2: Install with the actual regex plugin package
	pkg := zipDirectory(t, regexPluginDir)
	pkgBase64 := base64.StdEncoding.EncodeToString(pkg)

	installPayload := map[string]interface{}{
		"package_b64": pkgBase64,
	}

	resp = doRequest(t, ts, "POST", "/api/v1/plugins/regex:0.0.3/install", installPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("install plugin failed: %d: %s", resp.StatusCode, string(body))
	}

	var installation manager.Installation
	readJSON(t, resp, &installation)

	if installation.Status != "installed" {
		t.Errorf("expected status 'installed', got %q", installation.Status)
	}
	if installation.Manifest.Name != "regex" {
		t.Errorf("expected manifest name 'regex', got %q", installation.Manifest.Name)
	}
	if installation.Manifest.Version != "0.0.3" {
		t.Errorf("expected manifest version '0.0.3', got %q", installation.Manifest.Version)
	}

	// Verify workspace was created
	if _, err := os.Stat(installation.Path); os.IsNotExist(err) {
		t.Errorf("workspace not created at %s", installation.Path)
	}

	// Verify venv was created
	venvPath := filepath.Join(installation.Path, ".venv")
	if _, err := os.Stat(venvPath); os.IsNotExist(err) {
		t.Errorf("venv not created at %s", venvPath)
	}
}

func TestAPI_InstallPlugin_Unauthorized(t *testing.T) {
	_, ts := setupTestServer(t)

	// Try to install without proper API key
	installPayload := map[string]interface{}{
		"package_b64": "",
	}

	resp := doRequest(t, ts, "POST", "/api/v1/plugins/test:1.0.0/install", installPayload, "invalid-key")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAPI_InstallPlugin_NotAdmin(t *testing.T) {
	_, ts := setupTestServer(t)

	// First create the plugin with admin key
	createPayload := map[string]interface{}{
		"manifest": map[string]interface{}{
			"name":    "auth-test-plugin",
			"version": "1.0.0",
			"runner": map[string]interface{}{
				"language":   "python",
				"entrypoint": "main",
			},
		},
	}

	resp := doRequest(t, ts, "POST", "/api/v1/plugins", createPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create plugin failed: %d: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	// Try to install with readonly key (not admin)
	installPayload := map[string]interface{}{
		"package_b64": "",
	}

	// Readonly key should be rejected for install (requires admin)
	resp = doRequest(t, ts, "POST", "/api/v1/plugins/auth-test-plugin:1.0.0/install", installPayload, "test-readonly-key")
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

func TestAPI_InstallPlugin_NotFound(t *testing.T) {
	_, ts := setupTestServer(t)

	installPayload := map[string]interface{}{
		"package_b64": "",
	}

	// Plugin not registered
	resp := doRequest(t, ts, "POST", "/api/v1/plugins/nonexistent:1.0.0/install", installPayload, "test-admin-key")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestAPI_ListPlugins(t *testing.T) {
	_, ts := setupTestServer(t)

	// Create a plugin first
	createPayload := map[string]interface{}{
		"manifest": map[string]interface{}{
			"name":    "list-test-plugin",
			"version": "1.0.0",
			"runner": map[string]interface{}{
				"language":   "python",
				"entrypoint": "main",
			},
		},
	}

	resp := doRequest(t, ts, "POST", "/api/v1/plugins", createPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create failed: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// List plugins (readonly access is enough for GET)
	resp = doRequest(t, ts, "GET", "/api/v1/plugins", nil, "test-readonly-key")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list failed: %d", resp.StatusCode)
	}

	var plugins []plugin.Manifest
	readJSON(t, resp, &plugins)

	if len(plugins) == 0 {
		t.Error("expected at least one plugin")
	}

	found := false
	for _, p := range plugins {
		if p.Name == "list-test-plugin" {
			found = true
			break
		}
	}
	if !found {
		t.Error("created plugin not found in list")
	}
}

func TestAPI_GetPlugin(t *testing.T) {
	_, ts := setupTestServer(t)

	// Create a plugin first
	createPayload := map[string]interface{}{
		"manifest": map[string]interface{}{
			"name":    "get-test-plugin",
			"version": "2.0.0",
			"runner": map[string]interface{}{
				"language":   "python",
				"entrypoint": "main",
			},
		},
	}

	resp := doRequest(t, ts, "POST", "/api/v1/plugins", createPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create failed: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Get the plugin by ID (readonly access is enough for GET)
	resp = doRequest(t, ts, "GET", "/api/v1/plugins/get-test-plugin:2.0.0", nil, "test-readonly-key")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("get failed: %d: %s", resp.StatusCode, string(body))
	}

	var result plugin.Manifest
	readJSON(t, resp, &result)

	if result.Name != "get-test-plugin" {
		t.Errorf("expected name 'get-test-plugin', got %q", result.Name)
	}
	if result.Version != "2.0.0" {
		t.Errorf("expected version '2.0.0', got %q", result.Version)
	}
}

// TestAPI_InstallPlugin_MultipartMissingFile tests multipart upload without file field.
func TestAPI_InstallPlugin_MultipartMissingFile(t *testing.T) {
	_, ts := setupTestServer(t)

	// Create the plugin first
	createPayload := map[string]interface{}{
		"manifest": map[string]interface{}{
			"name":    "test-multipart-error",
			"version": "1.0.0",
			"runner": map[string]interface{}{
				"language":   "python",
				"entrypoint": "main",
			},
		},
	}

	resp := doRequest(t, ts, "POST", "/api/v1/plugins", createPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create plugin failed: %d: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	// Send multipart request but without file field
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	// Intentionally not adding any file field
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req, err := http.NewRequest("POST", ts.URL+"/api/v1/plugins/test-multipart-error:1.0.0/install", &buf)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-API-Key", "test-admin-key")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 400, got %d: %s", resp.StatusCode, string(body))
	}

	var errorResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}

	if errorMsg, ok := errorResp["error"].(string); ok {
		if errorMsg != "multipart request requires 'file' or 'package' field" {
			t.Errorf("unexpected error message: %q", errorMsg)
		}
	} else {
		t.Error("error field not found in response")
	}
}

// TestAPI_InstallPlugin_JSONMissingBase64 tests JSON upload without package_b64 field.
func TestAPI_InstallPlugin_JSONMissingBase64(t *testing.T) {
	_, ts := setupTestServer(t)

	// Create the plugin first
	createPayload := map[string]interface{}{
		"manifest": map[string]interface{}{
			"name":    "test-json-error",
			"version": "1.0.0",
			"runner": map[string]interface{}{
				"language":   "python",
				"entrypoint": "main",
			},
		},
	}

	resp := doRequest(t, ts, "POST", "/api/v1/plugins", createPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create plugin failed: %d: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	// Send JSON request without package_b64 field
	installPayload := map[string]interface{}{}

	resp = doRequest(t, ts, "POST", "/api/v1/plugins/test-json-error:1.0.0/install", installPayload, "test-admin-key")
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 400, got %d: %s", resp.StatusCode, string(body))
		return
	}

	var errorResp map[string]interface{}
	readJSON(t, resp, &errorResp)

	if errorMsg, ok := errorResp["error"].(string); ok {
		if errorMsg != "package_b64 is required in JSON mode" {
			t.Errorf("unexpected error message: %q", errorMsg)
		}
	} else {
		t.Error("error field not found in response")
	}
}

// TestAPI_InstallPlugin_InvalidBase64 tests JSON upload with invalid base64 encoding.
func TestAPI_InstallPlugin_InvalidBase64(t *testing.T) {
	_, ts := setupTestServer(t)

	// Create the plugin first
	createPayload := map[string]interface{}{
		"manifest": map[string]interface{}{
			"name":    "test-invalid-base64",
			"version": "1.0.0",
			"runner": map[string]interface{}{
				"language":   "python",
				"entrypoint": "main",
			},
		},
	}

	resp := doRequest(t, ts, "POST", "/api/v1/plugins", createPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create plugin failed: %d: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	// Send JSON request with invalid base64
	installPayload := map[string]interface{}{
		"package_b64": "not-valid-base64!@#$",
	}

	resp = doRequest(t, ts, "POST", "/api/v1/plugins/test-invalid-base64:1.0.0/install", installPayload, "test-admin-key")
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 400, got %d: %s", resp.StatusCode, string(body))
		return
	}

	var errorResp map[string]interface{}
	readJSON(t, resp, &errorResp)

	if errorMsg, ok := errorResp["error"].(string); ok {
		if !bytes.Contains([]byte(errorMsg), []byte("invalid base64 encoding")) {
			t.Errorf("unexpected error message: %q", errorMsg)
		}
	} else {
		t.Error("error field not found in response")
	}
}

// TestAPI_InstallPlugin_MultipartPackageField tests multipart upload with 'package' field instead of 'file'.
func TestAPI_InstallPlugin_MultipartPackageField(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}

	_, ts := setupTestServer(t)

	createPayload := map[string]interface{}{
		"manifest": map[string]interface{}{
			"name":    "package-field-plugin",
			"version": "1.0.0",
			"runner": map[string]interface{}{
				"language":   "python",
				"entrypoint": "main",
			},
		},
	}

	resp := doRequest(t, ts, "POST", "/api/v1/plugins", createPayload, "test-admin-key")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create plugin failed: %d: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	pkg := createMinimalPluginZip(t)
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	// Use 'package' field instead of 'file'
	part, err := writer.CreateFormFile("package", "plugin.zip")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(pkg); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req, err := http.NewRequest("POST", ts.URL+"/api/v1/plugins/package-field-plugin:1.0.0/install", &buf)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-API-Key", "test-admin-key")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("install plugin failed: %d: %s", resp.StatusCode, string(body))
	}

	var installation manager.Installation
	readJSON(t, resp, &installation)

	if installation.Manifest.Name != "package-field-plugin" {
		t.Errorf("expected manifest name 'package-field-plugin', got %q", installation.Manifest.Name)
	}
	if installation.Status != "installed" {
		t.Errorf("expected status 'installed', got %q", installation.Status)
	}
}
