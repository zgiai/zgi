package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

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
	tampered["dependency_root"] = "home/other-cache"
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
