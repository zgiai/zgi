package profilebuilder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildRepositoryStdlibSmoke(t *testing.T) {
	result, err := Build(Options{
		ProfileName: "stdlib",
		SourceDir:   "../../profiles",
		OutputDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("build stdlib profile: %v", err)
	}
	if !result.VerificationPassed {
		t.Fatalf("expected verification to pass, got %+v", result)
	}
	if result.Checksum == "" || result.SizeBytes <= 0 {
		t.Fatalf("expected checksum and size, got %+v", result)
	}

	raw, err := os.ReadFile(filepath.Join(result.OutputDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read built manifest: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse built manifest: %v", err)
	}
	build, ok := manifest["build"].(map[string]any)
	if !ok || build["verification_passed"] != true {
		t.Fatalf("expected built manifest verification metadata, got %s", string(raw))
	}
}

func TestDryRunDoesNotCreateOutput(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "out")
	result, err := Build(Options{
		ProfileName: "stdlib",
		SourceDir:   "../../profiles",
		OutputDir:   outputDir,
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("dry-run stdlib profile: %v", err)
	}
	if !result.DryRun || result.OutputDir != "" {
		t.Fatalf("expected dry-run without output, got %+v", result)
	}
	if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
		t.Fatalf("expected dry-run to leave output absent, stat error=%v", err)
	}
}

func TestBuildSupportsRelativeSourceAndOutputDirs(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	profileDir := filepath.Join(root, "profiles", "minimal")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	manifest := `{
		"name":"minimal",
		"version":"2026.05.31",
		"status":"ready",
		"enabled":true,
		"owner_scope":"global",
		"languages":["python3"],
		"base_runtime":"preview-process",
		"checksum":"profile-source:minimal:2026.05.31",
		"estimated_size_bytes":1,
		"required_files":["manifest.json","verify.py"]
	}`
	if err := os.WriteFile(filepath.Join(profileDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "verify.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("write verify script: %v", err)
	}

	result, err := Build(Options{
		ProfileName: "minimal",
		SourceDir:   "profiles",
		OutputDir:   "out",
	})
	if err != nil {
		t.Fatalf("build relative profile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "out", "minimal", "manifest.json")); err != nil {
		t.Fatalf("expected relative output manifest, result=%+v err=%v", result, err)
	}
}

func TestBuildEmitsReadyManifestFromDisabledSource(t *testing.T) {
	sourceDir := t.TempDir()
	profileDir := filepath.Join(sourceDir, "disabled-profile")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	manifest := `{
		"name":"disabled-profile",
		"version":"2026.05.31",
		"status":"disabled",
		"enabled":false,
		"owner_scope":"global",
		"languages":["python3"],
		"base_runtime":"preview-process",
		"checksum":"profile-source:disabled-profile:2026.05.31",
		"estimated_size_bytes":1,
		"required_files":["manifest.json"]
	}`
	if err := os.WriteFile(filepath.Join(profileDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := Build(Options{
		ProfileName: "disabled-profile",
		SourceDir:   sourceDir,
		OutputDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("build disabled source profile: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(result.OutputDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read built manifest: %v", err)
	}
	var built struct {
		Status  string `json:"status"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(raw, &built); err != nil {
		t.Fatalf("parse built manifest: %v", err)
	}
	if built.Status != "ready" || !built.Enabled {
		t.Fatalf("expected built profile artifact to be ready and enabled, got %s", string(raw))
	}
}

func TestManagedOfficePackagesUsesAllowedBundleNames(t *testing.T) {
	packages := managedOfficePackages([]string{"python3", "nodejs"})
	raw, err := json.Marshal(packages)
	if err != nil {
		t.Fatalf("marshal packages: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"ecosystem":"python3"`) || !strings.Contains(text, `"ecosystem":"nodejs"`) {
		t.Fatalf("expected python and node managed packages, got %s", text)
	}
	if !strings.Contains(text, `"name":"office-tools"`) || !strings.Contains(text, `"version":"managed"`) {
		t.Fatalf("expected managed office-tools packages, got %s", text)
	}
	if strings.Contains(text, "libreoffice") || strings.Contains(text, "openpyxl") || strings.Contains(text, "pptxgenjs") {
		t.Fatalf("expected implementation packages to be hidden from policy manifest, got %s", text)
	}
}

func TestBuildRejectsProfileSourceSymlink(t *testing.T) {
	sourceDir := t.TempDir()
	profileDir := filepath.Join(sourceDir, "bad-profile")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	manifest := `{
		"name":"bad-profile",
		"version":"2026.05.31",
		"status":"disabled",
		"enabled":false,
		"owner_scope":"global",
		"languages":["python3"],
		"base_runtime":"preview-process",
		"checksum":"profile-source:bad-profile:2026.05.31",
		"estimated_size_bytes":1,
		"required_files":["manifest.json","verify.py"]
	}`
	if err := os.WriteFile(filepath.Join(profileDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(profileDir, "verify.py")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, err := Build(Options{
		ProfileName: "bad-profile",
		SourceDir:   sourceDir,
		OutputDir:   t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "contains symlink") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestMaterializeSymlinksCopiesInternalTargets(t *testing.T) {
	if os.Getenv("OS") == "Windows_NT" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "venv", "lib"), 0o755); err != nil {
		t.Fatalf("create target dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "venv", "lib", "module.py"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}
	if err := os.Symlink("lib", filepath.Join(root, "venv", "lib64")); err != nil {
		t.Fatalf("create internal symlink: %v", err)
	}

	if err := materializeSymlinks(root); err != nil {
		t.Fatalf("materialize internal symlink: %v", err)
	}
	info, err := os.Lstat(filepath.Join(root, "venv", "lib64"))
	if err != nil {
		t.Fatalf("stat materialized path: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		t.Fatalf("expected materialized directory, got mode=%s", info.Mode())
	}
	if raw, err := os.ReadFile(filepath.Join(root, "venv", "lib64", "module.py")); err != nil || string(raw) != "ok" {
		t.Fatalf("expected copied target content, raw=%q err=%v", string(raw), err)
	}
}

func TestMaterializeSymlinksRejectsEscapingTargets(t *testing.T) {
	if os.Getenv("OS") == "Windows_NT" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside target: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "leak")); err != nil {
		t.Fatalf("create escaping symlink: %v", err)
	}

	err := materializeSymlinks(root)
	if err == nil || !strings.Contains(err.Error(), "escapes output directory") {
		t.Fatalf("expected escaping symlink rejection, got %v", err)
	}
}

func TestMergeEnvOverridesExistingKeys(t *testing.T) {
	env := mergeEnv([]string{"PATH=/bin", "HOME=/tmp"}, []string{"PATH=/custom/bin", "NODE_PATH=/profile/node_modules"})
	expected := []string{"PATH=/custom/bin", "HOME=/tmp", "NODE_PATH=/profile/node_modules"}
	if strings.Join(env, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("unexpected merged env: %#v", env)
	}
}

func TestNodeInstallArgsProduceMaterializedLayout(t *testing.T) {
	args := strings.Join(nodeInstallArgs(), "\n")
	for _, expected := range []string{
		"--frozen-lockfile",
		"--config.node-linker=hoisted",
		"--config.prefer-symlinked-executables=false",
	} {
		if !strings.Contains(args, expected) {
			t.Fatalf("expected node install args to include %s, got %v", expected, nodeInstallArgs())
		}
	}
}
