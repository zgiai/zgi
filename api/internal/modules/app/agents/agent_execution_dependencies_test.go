package agents

import (
	"context"
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
	if result.Error != nil || result.Status != "success" || result.Result["status"] != "ready" || result.Result["fingerprint"] != "fingerprint-1" {
		t.Fatalf("dependency preparation result = %#v", result)
	}
	if fake.setupJobs != 1 || fake.command != "python3" || fake.networkPolicy != "offline" {
		t.Fatalf("dependency setup calls = %d, command = %q, policy = %q", fake.setupJobs, fake.command, fake.networkPolicy)
	}
}
