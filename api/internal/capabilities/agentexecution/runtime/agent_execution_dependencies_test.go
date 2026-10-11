package executionruntime

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestAgentExecutionDependencyPreparationProgramPreparesAndReusesPythonWheel(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		python, err = exec.LookPath("python")
	}
	if err != nil {
		t.Skip("python interpreter is not available on this host")
	}
	if !pythonVenvAvailable(t, python) {
		t.Skip("python venv/ensurepip is not available on this host")
	}
	workspace := t.TempDir()
	packageRoot := filepath.Join(workspace, "skill")
	home := filepath.Join(workspace, "home")
	wheels := filepath.Join(packageRoot, "vendor", "python")
	if err := os.MkdirAll(wheels, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "requirements.txt"), []byte("demo-dependency==1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeTestWheel(filepath.Join(wheels, "demo_dependency-1.0.0-py3-none-any.whl")); err != nil {
		t.Fatal(err)
	}
	digestBytes := sha256.Sum256([]byte("python-wheel-test"))
	digest := hex.EncodeToString(digestBytes[:])
	if err := os.WriteFile(filepath.Join(packageRoot, ".zgi-ready"), []byte(digest+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspaceLiteral, err := json.Marshal(workspace)
	if err != nil {
		t.Fatal(err)
	}
	program := strings.Replace(agentExecutionDependencyPreparationProgram, "pathlib.Path('/workspace').resolve()", "pathlib.Path("+string(workspaceLiteral)+").resolve()", 1)
	run := func() map[string]interface{} {
		cmd := exec.Command(python, "-c", program, packageRoot, "python-demo", digest)
		cmd.Env = append(os.Environ(), "HOME="+home)
		output, runErr := cmd.CombinedOutput()
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok && exitErr.ExitCode() == 9009 {
				t.Skip("python interpreter is not executable on this host")
			}
			t.Fatalf("python dependency preparation failed: %v\n%s", runErr, strings.TrimSpace(string(output)))
		}
		var result map[string]interface{}
		if err := json.Unmarshal(output, &result); err != nil {
			t.Fatalf("python dependency preparation output is not JSON: %v\n%s", err, output)
		}
		return result
	}
	first := run()
	if first["status"] != "ready" || first["cache_status"] != "created" || first["source_mode"] != "offline-bundled" || first["preparation_version"] != "v2" {
		t.Fatalf("first Python preparation result = %#v", first)
	}
	pythonPath, ok := first["python_path"].(string)
	if !ok || pythonPath == "" {
		t.Fatalf("Python preparation did not return python_path: %#v", first)
	}
	check := exec.Command(filepath.Join(workspace, filepath.FromSlash(pythonPath)), "-c", "import demo_dependency; print(demo_dependency.VALUE)")
	check.Env = append(os.Environ(), "HOME="+home)
	output, err := check.CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "ready" {
		t.Fatalf("prepared Python cannot import wheel: %v\n%s", err, output)
	}
	second := run()
	if second["status"] != "reused" || second["cache_status"] != "reused" {
		t.Fatalf("Python cache was not reused: %#v", second)
	}
}

func writeTestWheel(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(file)
	files := map[string]string{
		"demo_dependency/__init__.py":              "VALUE = 'ready'\n",
		"demo_dependency-1.0.0.dist-info/METADATA": "Metadata-Version: 2.1\nName: demo-dependency\nVersion: 1.0.0\n\n",
		"demo_dependency-1.0.0.dist-info/WHEEL":    "Wheel-Version: 1.0\nGenerator: zgi-test\nRoot-Is-Purelib: true\nTag: py3-none-any\n\n",
		"demo_dependency-1.0.0.dist-info/RECORD":   "",
	}
	for name, content := range files {
		entry, err := archive.Create(name)
		if err != nil {
			_ = archive.Close()
			_ = file.Close()
			return err
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			_ = archive.Close()
			_ = file.Close()
			return err
		}
	}
	if err := archive.Close(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func pythonVenvAvailable(t *testing.T, python string) bool {
	t.Helper()
	probe := exec.Command(python, "-m", "venv", filepath.Join(t.TempDir(), "probe"))
	return probe.Run() == nil
}

func TestAgentExecutionDependencyPreparationProgramReportsMissingPythonSource(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		python, err = exec.LookPath("python")
	}
	if err != nil {
		t.Skip("python interpreter is not available on this host")
	}
	if !pythonVenvAvailable(t, python) {
		t.Skip("python venv/ensurepip is not available on this host")
	}
	workspace := t.TempDir()
	packageRoot := filepath.Join(workspace, "skill")
	home := filepath.Join(workspace, "home")
	if err := os.MkdirAll(packageRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "requirements.txt"), []byte("demo-dependency==1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	digestBytes := sha256.Sum256([]byte("missing-python-source"))
	digest := hex.EncodeToString(digestBytes[:])
	if err := os.WriteFile(filepath.Join(packageRoot, ".zgi-ready"), []byte(digest+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspaceLiteral, err := json.Marshal(workspace)
	if err != nil {
		t.Fatal(err)
	}
	program := strings.Replace(agentExecutionDependencyPreparationProgram, "pathlib.Path('/workspace').resolve()", "pathlib.Path("+string(workspaceLiteral)+").resolve()", 1)
	cmd := exec.Command(python, "-c", program, packageRoot, "missing-demo", digest)
	cmd.Env = append(os.Environ(), "HOME="+home)
	output, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Fatalf("missing Python source unexpectedly succeeded: %s", output)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("missing Python source output is not JSON: %v\n%s", err, output)
	}
	if result["status"] != "blocked" || result["reason"] != "offline_dependency_source_missing" || result["language"] != "python3" {
		t.Fatalf("missing Python source result = %#v", result)
	}
}

func TestAgentExecutionDependencyPreparationProgramReportsMissingNodeSource(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		python, err = exec.LookPath("python")
	}
	if err != nil {
		t.Skip("python interpreter is not available on this host")
	}
	workspace := t.TempDir()
	packageRoot := filepath.Join(workspace, "skill")
	home := filepath.Join(workspace, "home")
	if err := os.MkdirAll(packageRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "package.json"), []byte(`{"dependencies":{"demo-package":"1.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	digestBytes := sha256.Sum256([]byte("missing-node-source"))
	digest := hex.EncodeToString(digestBytes[:])
	if err := os.WriteFile(filepath.Join(packageRoot, ".zgi-ready"), []byte(digest+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspaceLiteral, err := json.Marshal(workspace)
	if err != nil {
		t.Fatal(err)
	}
	program := strings.Replace(agentExecutionDependencyPreparationProgram, "pathlib.Path('/workspace').resolve()", "pathlib.Path("+string(workspaceLiteral)+").resolve()", 1)
	cmd := exec.Command(python, "-c", program, packageRoot, "missing-node-demo", digest)
	cmd.Env = append(os.Environ(), "HOME="+home)
	output, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Fatalf("missing Node source unexpectedly succeeded: %s", output)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("missing Node source output is not JSON: %v\n%s", err, output)
	}
	if result["status"] != "blocked" || result["reason"] != "offline_dependency_source_missing" {
		t.Fatalf("missing Node source result = %#v", result)
	}
}

func TestAgentExecutionDependencyPreparationProgramHasValidPythonSyntax(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		python, err = exec.LookPath("python")
	}
	if err != nil {
		t.Skip("python interpreter is not available on this host")
	}
	cmd := exec.Command(python, "-c", "import sys; compile(sys.argv[1], '<dependency-preparation>', 'exec')", agentExecutionDependencyPreparationProgram)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 9009 {
			t.Skip("python interpreter is not executable on this host")
		}
		t.Fatalf("dependency preparation program has invalid Python syntax: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestAgentExecutionDependencyPreparationProgramRebuildsInvalidCache(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		python, err = exec.LookPath("python")
	}
	if err != nil {
		t.Skip("python interpreter is not available on this host")
	}
	workspace := t.TempDir()
	packageRoot := filepath.Join(workspace, "skill")
	home := filepath.Join(workspace, "home")
	if err := os.MkdirAll(filepath.Join(packageRoot, "vendor", "node_modules", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "SKILL.md"), []byte("Skill package\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, ".zgi-ready"), []byte(strings.Repeat("a", 64)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "package.json"), []byte(`{"dependencies":{"demo":"1.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "vendor", "node_modules", "demo", "index.js"), []byte("module.exports = true;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspaceLiteral, err := json.Marshal(workspace)
	if err != nil {
		t.Fatal(err)
	}
	program := strings.Replace(agentExecutionDependencyPreparationProgram, "pathlib.Path('/workspace').resolve()", "pathlib.Path("+string(workspaceLiteral)+").resolve()", 1)
	digestBytes := sha256.Sum256([]byte("dependency-cache-test"))
	digest := hex.EncodeToString(digestBytes[:])
	if err := os.WriteFile(filepath.Join(packageRoot, ".zgi-ready"), []byte(digest+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run := func() map[string]interface{} {
		cmd := exec.Command(python, "-c", program, packageRoot, "cache-demo", digest)
		cmd.Env = append(os.Environ(), "HOME="+home)
		output, runErr := cmd.CombinedOutput()
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok && exitErr.ExitCode() == 9009 {
				t.Skip("python interpreter is not executable on this host")
			}
			t.Fatalf("dependency preparation program failed: %v\n%s", runErr, strings.TrimSpace(string(output)))
		}
		var result map[string]interface{}
		if err := json.Unmarshal(output, &result); err != nil {
			t.Fatalf("dependency preparation output is not JSON: %v\n%s", err, output)
		}
		return result
	}

	first := run()
	if first["status"] != "ready" || first["cache_status"] != "created" || first["source_mode"] != "offline-bundled" {
		t.Fatalf("first preparation result = %#v", first)
	}
	nodePath, ok := first["node_path"].(string)
	if !ok || nodePath == "" {
		t.Fatalf("first preparation did not return node_path: %#v", first)
	}
	if data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(nodePath), "demo", "index.js")); err != nil || string(data) != "module.exports = true;\n" {
		t.Fatalf("prepared Node module = %q, error=%v", data, err)
	}
	if err := os.RemoveAll(filepath.Join(workspace, nodePath)); err != nil {
		t.Fatal(err)
	}
	second := run()
	if second["status"] != "ready" || second["cache_status"] != "created" {
		t.Fatalf("invalid cache was reused: %#v", second)
	}
	third := run()
	if third["status"] != "reused" || third["cache_status"] != "reused" {
		t.Fatalf("valid cache was not reused: %#v", third)
	}
	dependencyRoot, ok := third["dependency_root"].(string)
	if !ok || dependencyRoot == "" {
		t.Fatalf("reused result did not return dependency_root: %#v", third)
	}
	markerPath := filepath.Join(workspace, filepath.FromSlash(dependencyRoot), "prepare.json")
	markerData, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	var tampered map[string]interface{}
	if err := json.Unmarshal(markerData, &tampered); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "rogue", "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "rogue", "node_modules", "index.js"), []byte("rogue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tampered["node_path"] = filepath.ToSlash(filepath.Join("home", "rogue", "node_modules"))
	tamperedData, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, append(tamperedData, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	fourth := run()
	if fourth["status"] != "ready" || fourth["cache_status"] != "created" {
		t.Fatalf("tampered cache was reused: %#v", fourth)
	}
}

func TestAgentExecutionDependencyPreparationProgramRejectsHomeCacheSymlink(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		python, err = exec.LookPath("python")
	}
	if err != nil {
		t.Skip("python interpreter is not available on this host")
	}
	workspace := t.TempDir()
	packageRoot := filepath.Join(workspace, "skill")
	home := filepath.Join(workspace, "home")
	if err := os.MkdirAll(packageRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	digestBytes := sha256.Sum256([]byte("cache-symlink-test"))
	digest := hex.EncodeToString(digestBytes[:])
	if err := os.WriteFile(filepath.Join(packageRoot, ".zgi-ready"), []byte(digest+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cacheParent := filepath.Join(home, ".zgi-dependencies", "symlink-demo")
	if err := os.MkdirAll(cacheParent, 0o755); err != nil {
		t.Fatal(err)
	}
	cacheTarget := filepath.Join(home, "other-cache")
	if err := os.MkdirAll(cacheTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(cacheParent, digest)
	if err := os.Symlink(cacheTarget, cachePath); err != nil {
		t.Skipf("symbolic links are not available on this host: %v", err)
	}
	workspaceLiteral, err := json.Marshal(workspace)
	if err != nil {
		t.Fatal(err)
	}
	program := strings.Replace(agentExecutionDependencyPreparationProgram, "pathlib.Path('/workspace').resolve()", "pathlib.Path("+string(workspaceLiteral)+").resolve()", 1)
	cmd := exec.Command(python, "-c", program, packageRoot, "symlink-demo", digest)
	cmd.Env = append(os.Environ(), "HOME="+home)
	output, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Fatalf("cache symlink unexpectedly succeeded: %s", output)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("cache symlink output is not JSON: %v\n%s", err, output)
	}
	if result["status"] != "blocked" || result["reason"] != "dependency_cache_invalid" {
		t.Fatalf("cache symlink result = %#v", result)
	}
}

func TestAgentExecutionDependencyPreparationProgramReportsUndeclaredImports(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		python, err = exec.LookPath("python")
	}
	if err != nil {
		t.Skip("python interpreter is not available on this host")
	}
	workspace := t.TempDir()
	packageRoot := filepath.Join(workspace, "skill")
	home := filepath.Join(workspace, "home")
	if err := os.MkdirAll(filepath.Join(packageRoot, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "SKILL.md"), []byte("Skill package\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "scripts", "run.py"), []byte("import requests\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "scripts", "run.js"), []byte("const axios = require('axios');\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	digestBytes := sha256.Sum256([]byte("manifest-required-test"))
	digest := hex.EncodeToString(digestBytes[:])
	if err := os.WriteFile(filepath.Join(packageRoot, ".zgi-ready"), []byte(digest+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspaceLiteral, err := json.Marshal(workspace)
	if err != nil {
		t.Fatal(err)
	}
	program := strings.Replace(agentExecutionDependencyPreparationProgram, "pathlib.Path('/workspace').resolve()", "pathlib.Path("+string(workspaceLiteral)+").resolve()", 1)
	cmd := exec.Command(python, "-c", program, packageRoot, "manifest-demo", digest)
	cmd.Env = append(os.Environ(), "HOME="+home)
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok && exitErr.ExitCode() == 9009 {
			t.Skip("python interpreter is not executable on this host")
		}
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != 78 {
			t.Fatalf("unexpected dependency preparation error: %v\n%s", runErr, strings.TrimSpace(string(output)))
		}
	} else {
		t.Fatalf("dependency preparation unexpectedly succeeded: %s", strings.TrimSpace(string(output)))
	}
	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("dependency preparation output is not JSON: %v\n%s", err, output)
	}
	if result["status"] != "blocked" || result["reason"] != "dependency_manifest_required" {
		t.Fatalf("unexpected manifest diagnostic: %#v", result)
	}
	if !strings.Contains(string(output), "requests") || !strings.Contains(string(output), "axios") {
		t.Fatalf("diagnostic did not include detected imports: %s", output)
	}
}

func TestAgentExecutionDependencyPreparationRunsOfflineJob(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: dependency-demo\ndescription: test\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "run.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := skills.SkillPackageSHA256(root)
	if err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	fake := &fakeAgentExecutionGateway{
		binding: agentExecutionBinding{ID: "binding-1", Status: "active"},
		job:     agentExecutionJob{ID: "job-1", Status: "succeeded", ExitCode: &exitCode},
		output:  []agentExecutionOutputChunk{{Sequence: 0, Stream: "stdout", Data: []byte(`{"status":"ready","fingerprint":"fingerprint-1","node_path":".zgi-runtime-home/.zgi-dependencies/dependency-demo/node/node_modules"}`)}},
		capabilities: agentExecutionCapabilities{
			Shell: true, FileTransfer: true, DependencyPreparation: true, NetworkMode: "disabled", MaxUploadBytes: 1024 * 1024,
		},
	}
	tool := newAgentExecutionDependencyPreparationTool(fake, agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "agent-1", "conversation-1", "message-1", "standard", "local-gvisor", "offline", func(_ context.Context, skillID string) (agentSkillPackage, error) {
		if skillID != "dependency-demo" {
			t.Fatalf("resolved Skill ID = %q", skillID)
		}
		return agentSkillPackage{ID: skillID, Root: root, ExpectedDigest: digest}, nil
	})
	result := tool.Handler(t.Context(), adapter.ToolCall{ID: "call-1", Function: adapter.FunctionCall{Arguments: `{"skill_id":"dependency-demo","package_sha256":"` + digest + `"}`}})
	if result.Error != nil || result.Status != "success" || result.Result["status"] != "ready" || result.Result["fingerprint"] != "fingerprint-1" || result.Result["source_mode"] != "offline-bundled" || result.Result["preparation_version"] != "v2" {
		t.Fatalf("dependency preparation result = %#v", result)
	}
	if fake.setupJobs != 1 || fake.command != "python3" || fake.networkPolicy != "offline" {
		t.Fatalf("dependency setup calls = %d, command = %q, policy = %q", fake.setupJobs, fake.command, fake.networkPolicy)
	}
}

func TestAgentExecutionDependencyPreparationRunsControlledHTTPJob(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: controlled-demo\ndescription: test\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "run.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := skills.SkillPackageSHA256(root)
	if err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	fake := &fakeAgentExecutionGateway{
		binding: agentExecutionBinding{ID: "binding-1", Status: "active"},
		job:     agentExecutionJob{ID: "job-controlled-1", Status: "succeeded", ExitCode: &exitCode},
		output:  []agentExecutionOutputChunk{{Sequence: 0, Stream: "stdout", Data: []byte(`{"status":"ready","source_mode":"controlled-http","preparation_version":"v3"}`)}},
		capabilities: agentExecutionCapabilities{
			Shell: true, FileTransfer: true, DependencyPreparation: true, DependencyPreparationMode: "controlled-http", NetworkMode: "controlled-http", EgressProxyAddress: "127.0.0.1:3128", MaxUploadBytes: 1024 * 1024,
		},
	}
	tool := newAgentExecutionDependencyPreparationTool(fake, agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "agent-1", "conversation-1", "message-1", "standard", "local-gvisor", "controlled-http", func(_ context.Context, skillID string) (agentSkillPackage, error) {
		if skillID != "controlled-demo" {
			t.Fatalf("resolved Skill ID = %q", skillID)
		}
		return agentSkillPackage{ID: skillID, Root: root, ExpectedDigest: digest}, nil
	})
	result := tool.Handler(t.Context(), adapter.ToolCall{ID: "call-controlled-1", Function: adapter.FunctionCall{Arguments: `{"skill_id":"controlled-demo","package_sha256":"` + digest + `"}`}})
	if result.Error != nil || result.Status != "success" || result.Result["source_mode"] != "controlled-http" || result.Result["preparation_version"] != "v3" || result.Result["offline_only"] != false {
		t.Fatalf("controlled dependency preparation result = %#v", result)
	}
	if len(fake.args) != 6 || fake.args[5] != "controlled-http" || fake.networkPolicy != "controlled-http" {
		t.Fatalf("controlled setup args = %#v, policy = %q", fake.args, fake.networkPolicy)
	}
}
