package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestRuntimeLoadsCustomScriptSkillWhenRunnerConfigured(t *testing.T) {
	root := writeTestScriptSkill(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	runtime := NewRuntimeWithCatalog(nil, nil, "").WithScriptRunner(NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: server.URL}))
	doc, err := runtime.LoadCustomSkillDocument(root)
	if err != nil {
		t.Fatalf("load custom skill: %v", err)
	}
	if !doc.Metadata.HasScripts || !doc.Metadata.ScriptsSupported {
		t.Fatalf("expected scripts to be supported, metadata=%+v", doc.Metadata)
	}
	if _, ok := findSkillTool(doc, SkillScriptToolRun); !ok {
		t.Fatal("expected run_script tool")
	}
}

func TestRuntimeDoesNotExposeScriptToolWithoutRunner(t *testing.T) {
	root := writeTestScriptSkill(t)

	doc, err := LoadCustomSkillDocument(root)
	if err != nil {
		t.Fatalf("load custom skill: %v", err)
	}
	if !doc.Metadata.HasScripts || doc.Metadata.ScriptsSupported {
		t.Fatalf("expected scripts to be present but unsupported, metadata=%+v", doc.Metadata)
	}
	if _, ok := findSkillTool(doc, SkillScriptToolRun); ok {
		t.Fatal("did not expect run_script tool without configured runner")
	}
}

func TestSandboxScriptRunnerRunsSkillPackage(t *testing.T) {
	root := writeTestScriptSkill(t)
	deleted := false
	uploadedArchive := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			if req["organization_id"] != "organization-script" {
				t.Fatalf("unexpected sandbox create request: %#v", req)
			}
			writeSandboxEnvelope(t, w, map[string]interface{}{"id": "sbx_test"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files/upload-archive":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			uploadedArchive, _ = req["archive_base64"].(string)
			writeSandboxEnvelope(t, w, map[string]interface{}{"file_count": 2})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/exec/command":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			if req["sandbox_id"] != "sbx_test" || req["command"] != "python3" || req["profile"] != "skill-python" {
				t.Fatalf("unexpected command request: %#v", req)
			}
			if !strings.Contains(req["stdin"].(string), "hello") {
				t.Fatalf("expected stdin arguments, got %#v", req["stdin"])
			}
			env, ok := req["env"].(map[string]interface{})
			if !ok || env["ZGI_ORGANIZATION_ID"] != "organization-script" {
				t.Fatalf("expected organization env, got %#v", req["env"])
			}
			if len(env) != 1 {
				t.Fatalf("expected only organization env, got %#v", env)
			}
			writeSandboxEnvelope(t, w, map[string]interface{}{
				"stdout":      "{\"result\":\"ok\"}\n",
				"error":       "",
				"exit_code":   0,
				"duration_ms": 12,
				"truncated":   false,
				"command":     "python3",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/tree":
			writeSandboxEnvelope(t, w, map[string]interface{}{
				"items": []map[string]interface{}{
					{"path": "artifacts/report.txt", "size": 8, "is_directory": false},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/download":
			if r.URL.Query().Get("path") != "artifacts/report.txt" || r.URL.Query().Get("encoding") != "base64" {
				t.Fatalf("unexpected download query: %s", r.URL.RawQuery)
			}
			writeSandboxEnvelope(t, w, map[string]interface{}{
				"path":     "artifacts/report.txt",
				"content":  "cmVwb3J0Cg==",
				"encoding": "base64",
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sandboxes/sbx_test":
			deleted = true
			writeSandboxEnvelope(t, w, map[string]interface{}{"deleted": true})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	runtime := NewRuntimeWithCatalog(nil, nil, "").WithScriptRunner(NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: server.URL}))
	doc, err := runtime.LoadCustomSkillDocument(root)
	if err != nil {
		t.Fatalf("load custom skill: %v", err)
	}
	result, err := runtime.CallSkillTool(context.Background(), &ResolvedSkills{Skills: []SkillDocument{doc}}, "script-skill", SkillScriptToolRun, map[string]interface{}{"input": "hello"}, ExecutionContext{OrganizationID: "organization-script"}, "call_1")
	if err != nil {
		t.Fatalf("run skill script: %v", err)
	}
	if result == nil || result.Trace.Status != "success" || len(result.Messages) == 0 {
		t.Fatalf("unexpected invocation result: %+v", result)
	}
	if !messagesContainArtifacts(result.Messages) {
		t.Fatalf("expected artifact message, got %+v", result.Messages)
	}
	if !deleted {
		t.Fatal("expected sandbox to be deleted")
	}
	assertArchiveContains(t, uploadedArchive, "scripts/run.py")
}

func TestSandboxScriptRunnerRealSandboxE2E(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("ZGI_SANDBOX_E2E_ENDPOINT"))
	if endpoint == "" {
		t.Skip("set ZGI_SANDBOX_E2E_ENDPOINT to run real sandbox E2E")
	}
	root := writeTestScriptSkill(t)
	runtime := NewRuntimeWithCatalog(nil, nil, "").WithScriptRunner(NewSandboxScriptRunner(SandboxScriptRunnerConfig{
		Endpoint: endpoint,
		APIKey:   strings.TrimSpace(os.Getenv("ZGI_SANDBOX_E2E_API_KEY")),
	}))
	doc, err := runtime.LoadCustomSkillDocument(root)
	if err != nil {
		t.Fatalf("load custom skill: %v", err)
	}
	result, err := runtime.CallSkillTool(context.Background(), &ResolvedSkills{Skills: []SkillDocument{doc}}, "script-skill", SkillScriptToolRun, map[string]interface{}{"input": "hello"}, ExecutionContext{}, "call_1")
	if err != nil {
		t.Fatalf("run skill script against real sandbox: %v", err)
	}
	if len(result.Messages) == 0 || result.Messages[0].Data["echo"] != "hello" {
		t.Fatalf("unexpected real sandbox result: %+v", result.Messages)
	}
	if !messagesContainArtifacts(result.Messages) {
		t.Fatalf("expected real sandbox artifact message, got %+v", result.Messages)
	}
}

func writeTestScriptSkill(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(`---
name: script-skill
description: Script skill
runtime_type: prompt
---
Use the script to process structured input.
`), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "run.py"), []byte("import json, os, sys\nargs = json.loads(sys.stdin.read() or '{}')\nos.makedirs('artifacts', exist_ok=True)\nopen('artifacts/report.txt', 'w').write('report\\n')\nprint(json.dumps({'echo': args.get('input', '')}))\n"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return root
}

func messagesContainArtifacts(messages []tools.ToolInvokeMessage) bool {
	for _, message := range messages {
		if artifacts, ok := message.Data["artifacts"].([]map[string]interface{}); ok && len(artifacts) > 0 {
			return true
		}
		if raw, ok := message.Data["artifacts"].([]interface{}); ok && len(raw) > 0 {
			return true
		}
	}
	return false
}

func decodeJSON(t *testing.T, r *http.Request, out interface{}) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		t.Fatalf("decode request: %v", err)
	}
}

func writeSandboxEnvelope(t *testing.T, w http.ResponseWriter, data interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "message": "success", "data": data}); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func assertArchiveContains(t *testing.T, archiveBase64 string, path string) {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(archiveBase64)
	if err != nil {
		t.Fatalf("decode archive: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	for _, file := range reader.File {
		if file.Name == path {
			return
		}
	}
	t.Fatalf("expected archive to contain %s", path)
}
