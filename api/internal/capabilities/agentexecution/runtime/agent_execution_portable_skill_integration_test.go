//go:build agent_runtime_integration

package executionruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestAgentExecutionPortableSkillWorkerContract(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_URL"))
	apiKey := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_API_KEY"))
	profileID := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_PROFILE"))
	backend := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_BACKEND"))
	if baseURL == "" || apiKey == "" || profileID == "" || backend == "" {
		t.Fatal("ZGI_AGENT_RUNTIME_TEST_URL, _API_KEY, _PROFILE, and _BACKEND are required")
	}
	gateway := &agentExecutionHTTPGateway{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, client: &http.Client{Timeout: 90 * time.Second}}
	scope := agentExecutionScope{OrganizationID: uuid.NewString(), SubjectID: uuid.NewString()}
	agentID, conversationID := uuid.NewString(), uuid.NewString()
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Minute)
	defer cancel()
	binding, err := gateway.CreateBinding(ctx, scope, agentID, conversationID, profileID, backend)
	if err != nil || binding.Status != "active" {
		t.Fatalf("create binding = %#v, error = %v", binding, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		latest, err := gateway.CreateBinding(cleanupCtx, scope, agentID, conversationID, profileID, backend)
		if err != nil {
			t.Errorf("read current binding for cleanup: %v", err)
			return
		}
		if latest.Status == "deleted" {
			return
		}
		var deleted agentExecutionBinding
		requestPath := fmt.Sprintf("bindings/%s?generation=%d", latest.ID, latest.Generation)
		if err := gateway.request(cleanupCtx, scope, http.MethodDelete, requestPath, nil, &deleted); err != nil {
			t.Errorf("destroy test binding: %v", err)
		} else if deleted.Status != "deleted" {
			t.Errorf("destroy test binding status = %q, want deleted", deleted.Status)
		}
	})

	const input = "item,qty\napple,2\nbanana,3\n"
	fileID := uuid.NewString()
	reader := &workerCSVAttachmentReader{files: map[string][]byte{fileID: []byte(input)}}
	attachments := []runtimeservice.RuntimeAttachmentRef{{ID: fileID, Name: "data.csv", Size: int64(len(input))}}
	const expected = "total=5\n"
	// The Python case destroys and recreates the Binding after its export.
	cases := []struct {
		id, filename, source, command, output string
	}{
		{"portable-node", "process.js", `const fs = require('fs');
if (process.argv.length !== 4) { console.error('usage: process.js INPUT OUTPUT'); process.exit(2); }
const rows = fs.readFileSync(process.argv[2], 'utf8').trim().split('\n').slice(1);
const total = rows.reduce((sum, row) => sum + Number(row.split(',')[1]), 0);
const result = 'total=' + total + '\n';
fs.mkdirSync('outputs', { recursive: true });
fs.writeFileSync(process.argv[3], result);
process.stdout.write(result);
process.stderr.write('processed node input\n');
`, `node "$ZGI_SKILL_DIR/scripts/process.js" {{INPUT}} outputs/node.txt`, "outputs/node.txt"},
		{"portable-python", "process.py", `import csv, pathlib, sys
if len(sys.argv) != 3:
    raise SystemExit('usage: process.py INPUT OUTPUT')
with open(sys.argv[1], newline='') as source:
    total = sum(int(row['qty']) for row in csv.DictReader(source))
result = 'total=' + str(total) + '\n'
pathlib.Path(sys.argv[2]).parent.mkdir(exist_ok=True)
pathlib.Path(sys.argv[2]).write_text(result)
print(result, end='')
print('processed python input', file=sys.stderr)
`, `python3 "$ZGI_SKILL_DIR/scripts/process.py" {{INPUT}} outputs/python.txt`, "outputs/python.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "scripts"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: "+tc.id+"\ndescription: Portable contract Skill.\n---\nRun the bundled script.\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "scripts", tc.filename), []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			digest, err := skills.SkillPackageSHA256(root)
			if err != nil {
				t.Fatal(err)
			}
			resolver := func(context.Context, string) (agentSkillPackage, error) {
				return agentSkillPackage{ID: tc.id, Root: root, ExpectedDigest: digest}, nil
			}
			shell := newAgentExecutionShellTool(gateway, scope, agentID, conversationID, uuid.NewString(), profileID, backend, attachments, reader, resolver)
			invoke := func(command string) map[string]interface{} {
				args, _ := json.Marshal(map[string]any{"skill_id": tc.id, "package_sha256": digest, "command": command})
				result := shell.Handler(ctx, adapter.ToolCall{ID: uuid.NewString(), Function: adapter.FunctionCall{Arguments: string(args)}})
				if result.Error != nil || result.Status != "success" {
					t.Fatalf("shell result = %#v", result)
				}
				return result.ModelResult
			}
			commandWithInput := strings.ReplaceAll(tc.command, "{{INPUT}}", "inputs/"+fileID+".csv")
			first := invoke(commandWithInput)
			if first["stdout"] != expected || !strings.Contains(fmt.Sprint(first["stderr"]), "processed") {
				t.Fatalf("script output = %#v", first)
			}
			if code, ok := first["exit_code"].(*int); !ok || code == nil || *code != 0 {
				t.Fatalf("script exit code = %#v", first["exit_code"])
			}
			second := invoke(commandWithInput)
			if second["stdout"] != expected {
				t.Fatalf("repeat script output = %#v", second)
			}
			invalid := invoke(strings.Split(commandWithInput, " inputs/")[0])
			if code, ok := invalid["exit_code"].(*int); !ok || code == nil || *code == 0 || !strings.Contains(fmt.Sprint(invalid["stderr"]), "usage") {
				t.Fatalf("invalid arguments = %#v", invalid)
			}
			store := &workerCSVArtifactStore{}
			export := newAgentExecutionExportTool(gateway, store, scope, agentID, conversationID, uuid.NewString(), profileID, backend)
			arguments, _ := json.Marshal(map[string]string{"path": tc.output})
			exported := export.Handler(ctx, adapter.ToolCall{ID: uuid.NewString(), Function: adapter.FunctionCall{Arguments: string(arguments)}})
			if exported.Error != nil || string(store.data) != expected || len(exported.Artifacts) != 1 {
				t.Fatalf("export result = %#v, data = %q", exported, store.data)
			}
			if tc.id == "portable-python" {
				current, err := gateway.CreateBinding(ctx, scope, agentID, conversationID, profileID, backend)
				if err != nil {
					t.Fatal(err)
				}
				var deleted agentExecutionBinding
				requestPath := fmt.Sprintf("bindings/%s?generation=%d", current.ID, current.Generation)
				if err := gateway.request(ctx, scope, http.MethodDelete, requestPath, nil, &deleted); err != nil {
					t.Fatalf("destroy binding for recreation: %v", err)
				}
				stale := export.Handler(ctx, adapter.ToolCall{ID: uuid.NewString(), Function: adapter.FunctionCall{Arguments: string(arguments)}})
				if stale.Result["error_code"] != "workspace_recreated" {
					t.Fatalf("old workspace file remained exportable: %#v", stale)
				}
				recreatedShell := newAgentExecutionShellTool(gateway, scope, agentID, conversationID, uuid.NewString(), profileID, backend, attachments, reader, resolver)
				recreateArgs, _ := json.Marshal(map[string]any{"skill_id": tc.id, "package_sha256": digest, "command": commandWithInput})
				recreated := recreatedShell.Handler(ctx, adapter.ToolCall{ID: uuid.NewString(), Function: adapter.FunctionCall{Arguments: string(recreateArgs)}})
				if recreated.Error != nil || recreated.ModelResult["stdout"] != expected || recreated.ModelResult["workspace_recreated"] != true {
					t.Fatalf("recreated Skill workspace = %#v", recreated)
				}
				store.data = nil
				fresh := export.Handler(ctx, adapter.ToolCall{ID: uuid.NewString(), Function: adapter.FunctionCall{Arguments: string(arguments)}})
				if fresh.Error != nil || fresh.Status != "success" || string(store.data) != expected {
					t.Fatalf("export from recreated Skill workspace = %#v, data = %q", fresh, store.data)
				}
			}
		})
	}
}

// ZGI_PORTABLE_SKILL_TEST_ROOT points at an unpacked skill-creator Skill.
// The test packages its original files, stages them through the real Worker,
// runs the bundled helper in the offline session, and exports its result.
func TestAgentExecutionExternalPortableSkillWorkerSmoke(t *testing.T) {
	root := strings.TrimSpace(os.Getenv("ZGI_PORTABLE_SKILL_TEST_ROOT"))
	if root == "" {
		t.Skip("ZGI_PORTABLE_SKILL_TEST_ROOT is not set")
	}
	baseURL := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_URL"))
	apiKey := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_API_KEY"))
	profileID := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_PROFILE"))
	backend := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_BACKEND"))
	if baseURL == "" || apiKey == "" || profileID == "" || backend == "" {
		t.Fatal("ZGI_AGENT_RUNTIME_TEST_URL, _API_KEY, _PROFILE, and _BACKEND are required")
	}
	if _, err := os.Stat(filepath.Join(root, "scripts", "init_skill.py")); err != nil {
		t.Fatal(err)
	}
	digest, err := skills.SkillPackageSHA256(root)
	if err != nil {
		t.Fatal(err)
	}
	gateway := &agentExecutionHTTPGateway{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, client: &http.Client{Timeout: 90 * time.Second}}
	scope := agentExecutionScope{OrganizationID: uuid.NewString(), SubjectID: uuid.NewString()}
	agentID, conversationID := uuid.NewString(), uuid.NewString()
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	binding, err := gateway.CreateBinding(ctx, scope, agentID, conversationID, profileID, backend)
	if err != nil || binding.Status != "active" {
		t.Fatalf("create binding = %#v, error = %v", binding, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		latest, err := gateway.CreateBinding(cleanupCtx, scope, agentID, conversationID, profileID, backend)
		if err != nil {
			t.Errorf("read current binding for cleanup: %v", err)
			return
		}
		var deleted agentExecutionBinding
		requestPath := fmt.Sprintf("bindings/%s?generation=%d", binding.ID, latest.Generation)
		if err := gateway.request(cleanupCtx, scope, http.MethodDelete, requestPath, nil, &deleted); err != nil {
			t.Errorf("destroy smoke binding: %v", err)
		} else if deleted.Status != "deleted" {
			t.Errorf("destroy smoke binding status = %q, want deleted", deleted.Status)
		}
	})
	resolver := func(context.Context, string) (agentSkillPackage, error) {
		return agentSkillPackage{ID: "skill-creator", Root: root, ExpectedDigest: digest}, nil
	}
	shell := newAgentExecutionShellTool(gateway, scope, agentID, conversationID, uuid.NewString(), profileID, backend, nil, nil, resolver)
	const command = `python3 "$ZGI_SKILL_DIR/scripts/init_skill.py" smoke-generated --path outputs`
	arguments, _ := json.Marshal(map[string]any{"skill_id": "skill-creator", "package_sha256": digest, "command": command, "timeout_seconds": 30})
	result := shell.Handler(ctx, adapter.ToolCall{ID: "external-skill-run", Function: adapter.FunctionCall{Arguments: string(arguments)}})
	if result.Error != nil || result.Status != "success" || !strings.Contains(fmt.Sprint(result.ModelResult["stdout"]), "initialized successfully") {
		t.Fatalf("external Skill shell result = %#v, want generated Skill confirmation", result)
	}
	if code, ok := result.ModelResult["exit_code"].(*int); !ok || code == nil || *code != 0 {
		t.Fatalf("external Skill exit code = %#v, want zero", result.ModelResult["exit_code"])
	}
	store := &workerCSVArtifactStore{}
	export := newAgentExecutionExportTool(gateway, store, scope, agentID, conversationID, uuid.NewString(), profileID, backend)
	exported := export.Handler(ctx, adapter.ToolCall{ID: "external-skill-export", Function: adapter.FunctionCall{Arguments: `{"path":"outputs/smoke-generated/SKILL.md"}`}})
	if exported.Error != nil || exported.Status != "success" || !strings.Contains(string(store.data), "name: smoke-generated") || len(exported.Artifacts) != 1 {
		t.Fatalf("external Skill export = %#v, data length = %d, want generated SKILL.md", exported, len(store.data))
	}
	repeatArguments, _ := json.Marshal(map[string]any{"skill_id": "skill-creator", "package_sha256": digest, "command": `python3 "$ZGI_SKILL_DIR/scripts/init_skill.py" smoke-generated-again --path outputs`})
	repeated := shell.Handler(ctx, adapter.ToolCall{ID: "external-skill-repeat", Function: adapter.FunctionCall{Arguments: string(repeatArguments)}})
	if repeated.Error != nil || !strings.Contains(fmt.Sprint(repeated.ModelResult["stdout"]), "initialized successfully") || repeated.ModelResult["skill_workspace_path"] != result.ModelResult["skill_workspace_path"] {
		t.Fatalf("repeated external Skill run = %#v", repeated)
	}
	if code, ok := repeated.ModelResult["exit_code"].(*int); !ok || code == nil || *code != 0 {
		t.Fatalf("repeated external Skill exit code = %#v, want zero", repeated.ModelResult["exit_code"])
	}
	invalidArguments, _ := json.Marshal(map[string]any{"skill_id": "skill-creator", "package_sha256": digest, "command": `python3 "$ZGI_SKILL_DIR/scripts/init_skill.py"`})
	invalid := shell.Handler(ctx, adapter.ToolCall{ID: "external-skill-invalid", Function: adapter.FunctionCall{Arguments: string(invalidArguments)}})
	if invalid.Error != nil || !strings.Contains(fmt.Sprint(invalid.ModelResult["stdout"]), "Usage:") {
		t.Fatalf("external Skill invalid-input diagnostics = %#v", invalid)
	}
	if code, ok := invalid.ModelResult["exit_code"].(*int); !ok || code == nil || *code == 0 {
		t.Fatalf("external Skill invalid-input exit code = %#v, want nonzero", invalid.ModelResult["exit_code"])
	}
	t.Logf("staged %s at %s; exported %d bytes; invalid arguments returned %d", "skill-creator", result.ModelResult["skill_workspace_path"], len(store.data), *invalid.ModelResult["exit_code"].(*int))
}
