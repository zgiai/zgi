package manager

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/zgiai/zgi/runner/internal/config"
	"github.com/zgiai/zgi/runner/internal/plugin"
	"github.com/zgiai/zgi/runner/internal/runtime"
	"github.com/zgiai/zgi/runner/internal/storage"
)

// mockStore implements storage.Store for testing.
type mockStore struct {
	workspaceRoot string
	savedPackages map[string][]byte
	saveErr       error
}

func newMockStore(t *testing.T) *mockStore {
	t.Helper()
	return &mockStore{
		workspaceRoot: t.TempDir(),
		savedPackages: make(map[string][]byte),
	}
}

func (m *mockStore) SavePackage(ctx context.Context, manifest plugin.Manifest, pkg []byte) (string, error) {
	if m.saveErr != nil {
		return "", m.saveErr
	}
	key := manifest.Name + ":" + manifest.Version
	m.savedPackages[key] = pkg

	// Extract to workspace directory
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
	key := manifest.Name + ":" + manifest.Version
	delete(m.savedPackages, key)
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

// testConfig returns a minimal config for testing.
func testConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		PluginHome:           t.TempDir(),
		WorkspacePath:        t.TempDir(),
		PackageCachePath:     t.TempDir(),
		PythonInterpreter:    "python3",
		NodeInterpreter:      "node",
		PipCommand:           "pip",
		NPMCommand:           "npm",
		PythonEnvInitTimeout: 10 * time.Minute,
		InstallTimeout:       2 * time.Minute,
		HTTPPort:             14000,
		StdoutBufferSize:     4096,
		StdoutMaxBufferSize:  5 * 1024 * 1024,
	}
}

// regexManifest returns a manifest matching the regex example plugin.
func regexManifest() plugin.Manifest {
	return plugin.Manifest{
		Name:    "regex",
		Version: "0.0.3",
		Author:  "zgi",
		Runner: plugin.Runner{
			Language:   plugin.LanguagePython,
			Entrypoint: "main",
		},
		Tags: []string{"utilities", "productivity"},
	}
}

// zipDirectory creates a zip archive from a directory.
func zipDirectory(t *testing.T, srcDir string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		// Skip hidden directories like .venv, .git
		if info.IsDir() && len(info.Name()) > 0 && info.Name()[0] == '.' {
			return filepath.SkipDir
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = relPath
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := w.CreateHeader(header)
		if err != nil {
			return err
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(writer, file)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("zip directory: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	return buf.Bytes()
}

// createMinimalPluginZip creates a minimal valid plugin zip with requirements.txt.
func createMinimalPluginZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	// Add main.py
	mainPy, err := w.Create("main.py")
	if err != nil {
		t.Fatalf("create main.py: %v", err)
	}
	_, _ = mainPy.Write([]byte("# minimal plugin\n"))

	// Add requirements.txt (empty is fine for venv creation)
	reqTxt, err := w.Create("requirements.txt")
	if err != nil {
		t.Fatalf("create requirements.txt: %v", err)
	}
	_, _ = reqTxt.Write([]byte("# no dependencies\n"))

	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	return buf.Bytes()
}

// createEmptyZip creates a valid empty zip file.
func createEmptyZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func TestInstall_ManifestValidation(t *testing.T) {
	store := newMockStore(t)
	cfg := testConfig(t)
	log := zap.NewNop()
	m := New(cfg, &mockRuntime{}, store, log, nil, nil, nil)

	// Use a valid plugin zip for the successful case
	validPkg := createMinimalPluginZip(t)

	tests := []struct {
		name        string
		manifest    plugin.Manifest
		pkg         []byte
		wantErr     bool
		errContains string
	}{
		{
			name: "missing name",
			manifest: plugin.Manifest{
				Version: "1.0.0",
				Runner:  plugin.Runner{Language: plugin.LanguagePython, Entrypoint: "main"},
			},
			pkg:         validPkg,
			wantErr:     true,
			errContains: "name is required",
		},
		{
			name: "missing version",
			manifest: plugin.Manifest{
				Name:   "test-plugin",
				Runner: plugin.Runner{Language: plugin.LanguagePython, Entrypoint: "main"},
			},
			pkg:         validPkg,
			wantErr:     true,
			errContains: "version is required",
		},
		{
			name: "missing entrypoint",
			manifest: plugin.Manifest{
				Name:    "test-plugin",
				Version: "1.0.0",
				Runner:  plugin.Runner{Language: plugin.LanguagePython},
			},
			pkg:         validPkg,
			wantErr:     true,
			errContains: "entrypoint is required",
		},
		{
			name: "unsupported language",
			manifest: plugin.Manifest{
				Name:    "test-plugin",
				Version: "1.0.0",
				Runner:  plugin.Runner{Language: "rust", Entrypoint: "main"},
			},
			pkg:         validPkg,
			wantErr:     true,
			errContains: "not supported",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := InstallRequest{
				Manifest: tc.manifest,
				Package:  tc.pkg,
			}
			_, err := m.Install(context.Background(), req)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tc.errContains)
				} else if tc.errContains != "" && !contains(err.Error(), tc.errContains) {
					t.Errorf("expected error containing %q, got %q", tc.errContains, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestInstall_PackageSizeLimit(t *testing.T) {
	store := newMockStore(t)
	cfg := testConfig(t)
	cfg.MaxPackageSize = 100 // 100 bytes limit
	log := zap.NewNop()
	m := New(cfg, &mockRuntime{}, store, log, nil, nil, nil)

	manifest := plugin.Manifest{
		Name:    "test-plugin",
		Version: "1.0.0",
		Runner:  plugin.Runner{Language: plugin.LanguagePython, Entrypoint: "main"},
	}

	req := InstallRequest{
		Manifest: manifest,
		Package:  make([]byte, 200), // Exceeds limit
	}

	_, err := m.Install(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for oversized package")
	}
	if !contains(err.Error(), "exceeds max size") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInstall_ChecksumMismatch(t *testing.T) {
	store := newMockStore(t)
	cfg := testConfig(t)
	log := zap.NewNop()
	m := New(cfg, &mockRuntime{}, store, log, nil, nil, nil)

	manifest := plugin.Manifest{
		Name:    "test-plugin",
		Version: "1.0.0",
		Runner:  plugin.Runner{Language: plugin.LanguagePython, Entrypoint: "main"},
	}

	// Use a valid zip file
	pkg := createMinimalPluginZip(t)

	req := InstallRequest{
		Manifest: manifest,
		Package:  pkg,
		Checksum: "invalid-checksum",
	}

	_, err := m.Install(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for checksum mismatch")
	}
	if !contains(err.Error(), "checksum mismatch") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInstall_SignatureRequired(t *testing.T) {
	store := newMockStore(t)
	cfg := testConfig(t)
	cfg.RequireManifestSignature = true
	cfg.SignaturePublicKeyPath = "/tmp/key.pem" // dummy path for validation
	log := zap.NewNop()
	m := New(cfg, &mockRuntime{}, store, log, nil, nil, nil)

	manifest := plugin.Manifest{
		Name:    "test-plugin",
		Version: "1.0.0",
		Runner:  plugin.Runner{Language: plugin.LanguagePython, Entrypoint: "main"},
		// No signature
	}

	req := InstallRequest{
		Manifest: manifest,
		Package:  []byte{},
	}

	_, err := m.Install(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for missing signature")
	}
	if !contains(err.Error(), "signature required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Integration test using the real regex plugin from examples directory.
// This test requires Python3 to be available.
func TestInstall_RegexPlugin_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check if python3 is available
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available, skipping integration test")
	}

	// Locate the regex plugin directory
	regexPluginDir := filepath.Join("..", "..", "examples", "regex", "zgi-regex_0.0.3")
	if _, err := os.Stat(regexPluginDir); os.IsNotExist(err) {
		t.Skipf("regex plugin not found at %s", regexPluginDir)
	}

	// Create zip from the regex plugin directory
	pkg := zipDirectory(t, regexPluginDir)

	store := newMockStore(t)
	cfg := testConfig(t)
	log := zap.NewNop()
	m := New(cfg, &mockRuntime{}, store, log, nil, nil, nil)

	manifest := regexManifest()

	req := InstallRequest{
		Manifest: manifest,
		Package:  pkg,
		Operator: "test-user",
		Source:   "test",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ins, err := m.Install(ctx, req)
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	// Verify installation result
	if ins.Status != "installed" {
		t.Errorf("expected status 'installed', got %q", ins.Status)
	}
	if ins.Manifest.Name != "regex" {
		t.Errorf("expected manifest name 'regex', got %q", ins.Manifest.Name)
	}
	if ins.Manifest.Version != "0.0.3" {
		t.Errorf("expected manifest version '0.0.3', got %q", ins.Manifest.Version)
	}

	// Verify the workspace was created
	if _, err := os.Stat(ins.Path); os.IsNotExist(err) {
		t.Errorf("workspace not created at %s", ins.Path)
	}

	// Verify venv was created
	venvPath := filepath.Join(ins.Path, ".venv")
	if _, err := os.Stat(venvPath); os.IsNotExist(err) {
		t.Errorf("venv not created at %s", venvPath)
	}

	// Verify main.py exists
	mainPyPath := filepath.Join(ins.Path, "main.py")
	if _, err := os.Stat(mainPyPath); os.IsNotExist(err) {
		t.Errorf("main.py not found at %s", mainPyPath)
	}

	// Verify the plugin is in the installed list
	installed := m.ListInstalled()
	found := false
	for _, i := range installed {
		if i.Manifest.Name == "regex" && i.Manifest.Version == "0.0.3" {
			found = true
			break
		}
	}
	if !found {
		t.Error("regex plugin not found in installed list")
	}
}

// TestInstall_Success tests successful installation with minimal plugin.
func TestInstall_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test that requires Python in short mode")
	}

	// Check if python3 is available
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}

	store := newMockStore(t)
	cfg := testConfig(t)
	log := zap.NewNop()
	m := New(cfg, &mockRuntime{}, store, log, nil, nil, nil)

	manifest := plugin.Manifest{
		Name:    "test-plugin",
		Version: "1.0.0",
		Runner:  plugin.Runner{Language: plugin.LanguagePython, Entrypoint: "main"},
	}

	pkg := createMinimalPluginZip(t)

	req := InstallRequest{
		Manifest: manifest,
		Package:  pkg,
		Operator: "test-user",
		Source:   "test",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ins, err := m.Install(ctx, req)
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	if ins.Status != "installed" {
		t.Errorf("expected status 'installed', got %q", ins.Status)
	}
	if ins.Manifest.Name != "test-plugin" {
		t.Errorf("expected name 'test-plugin', got %q", ins.Manifest.Name)
	}
}

// TestInstall_Uninstall tests the uninstall flow.
func TestInstall_Uninstall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test that requires Python in short mode")
	}

	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}

	store := newMockStore(t)
	cfg := testConfig(t)
	log := zap.NewNop()
	m := New(cfg, &mockRuntime{}, store, log, nil, nil, nil)

	manifest := plugin.Manifest{
		Name:    "test-plugin",
		Version: "1.0.0",
		Runner:  plugin.Runner{Language: plugin.LanguagePython, Entrypoint: "main"},
	}

	pkg := createMinimalPluginZip(t)

	// Install first
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ins, err := m.Install(ctx, InstallRequest{
		Manifest: manifest,
		Package:  pkg,
	})
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	workspacePath := ins.Path

	// Verify workspace exists
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		t.Fatalf("workspace should exist after install")
	}

	// Uninstall
	if err := m.Uninstall(ctx, "test-plugin", "1.0.0"); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	// Verify workspace is removed
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Errorf("workspace should be removed after uninstall")
	}

	// Verify plugin is not in installed list
	installed := m.ListInstalled()
	for _, i := range installed {
		if i.Manifest.Name == "test-plugin" {
			t.Error("plugin should not be in installed list after uninstall")
		}
	}
}
