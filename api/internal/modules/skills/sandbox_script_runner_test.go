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
			writeSandboxEnvelope(t, w, map[string]interface{}{
				"stdout":      "{\"result\":\"ok\"}\n",
				"error":       "",
				"exit_code":   0,
				"duration_ms": 12,
				"truncated":   false,
				"command":     "python3",
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
	result, err := runtime.CallSkillTool(context.Background(), &ResolvedSkills{Skills: []SkillDocument{doc}}, "script-skill", SkillScriptToolRun, map[string]interface{}{"input": "hello"}, ExecutionContext{}, "call_1")
	if err != nil {
		t.Fatalf("run skill script: %v", err)
	}
	if result == nil || result.Trace.Status != "success" || len(result.Messages) == 0 {
		t.Fatalf("unexpected invocation result: %+v", result)
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
	runtime := NewRuntimeWithCatalog(nil, nil, "").WithScriptRunner(NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: endpoint}))
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
	if err := os.WriteFile(filepath.Join(root, "scripts", "run.py"), []byte("import json, sys\nargs = json.loads(sys.stdin.read() or '{}')\nprint(json.dumps({'echo': args.get('input', '')}))\n"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return root
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
