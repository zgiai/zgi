package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
			if req["dependency_profile"] != defaultSkillDependencyProfile {
				t.Fatalf("expected default dependency profile, got %#v", req)
			}
			writeSandboxEnvelope(t, w, map[string]interface{}{"id": "sbx_test"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files/upload-archive":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			if req["organization_id"] != "organization-script" {
				t.Fatalf("expected upload organization scope, got %#v", req)
			}
			if req["validate_skill_manifest"] != true {
				t.Fatalf("expected skill manifest validation, got %#v", req)
			}
			uploadedArchive, _ = req["archive_base64"].(string)
			writeSandboxEnvelope(t, w, map[string]interface{}{"file_count": 2})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/exec/command":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			if req["sandbox_id"] != "sbx_test" || req["command"] != "python3" || req["profile"] != "skill-python" {
				t.Fatalf("unexpected command request: %#v", req)
			}
			if req["organization_id"] != "organization-script" {
				t.Fatalf("expected command organization scope, got %#v", req)
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
			if r.URL.Query().Get("organization_id") != "organization-script" {
				t.Fatalf("expected tree organization scope, got query %s", r.URL.RawQuery)
			}
			writeSandboxEnvelope(t, w, map[string]interface{}{
				"items": []map[string]interface{}{
					{"path": "artifacts/report.txt", "size": 8, "is_directory": false},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/download":
			if r.URL.Query().Get("path") != "artifacts/report.txt" || r.URL.Query().Get("encoding") != "base64" {
				t.Fatalf("unexpected download query: %s", r.URL.RawQuery)
			}
			if r.URL.Query().Get("organization_id") != "organization-script" {
				t.Fatalf("expected download organization scope, got query %s", r.URL.RawQuery)
			}
			writeSandboxEnvelope(t, w, map[string]interface{}{
				"path":     "artifacts/report.txt",
				"content":  "cmVwb3J0Cg==",
				"encoding": "base64",
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sandboxes/sbx_test":
			if r.URL.Query().Get("organization_id") != "organization-script" {
				t.Fatalf("expected delete organization scope, got query %s", r.URL.RawQuery)
			}
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
	manifestContent := archiveFileContent(t, uploadedArchive, "skill.manifest.json")
	if !strings.Contains(manifestContent, `"entrypoint":"scripts/run.py"`) || !strings.Contains(manifestContent, `"dependency_profile":"stdlib"`) {
		t.Fatalf("expected generated skill manifest, got %s", manifestContent)
	}
}

func TestSandboxScriptRunnerUsesManifestDependencyProfile(t *testing.T) {
	root := writeTestScriptSkill(t)
	if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(`{"dependency_profile":"workflow-safe"}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			if req["dependency_profile"] != "workflow-safe" {
				t.Fatalf("expected manifest dependency profile, got %#v", req)
			}
			writeSandboxEnvelope(t, w, map[string]interface{}{"id": "sbx_test"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files/upload-archive":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			archive, _ := req["archive_base64"].(string)
			manifestContent := archiveFileContent(t, archive, "skill.manifest.json")
			if !strings.Contains(manifestContent, `"dependency_profile":"workflow-safe"`) || !strings.Contains(manifestContent, `"entrypoint":"scripts/run.py"`) {
				t.Fatalf("expected normalized manifest dependency profile, got %s", manifestContent)
			}
			writeSandboxEnvelope(t, w, map[string]interface{}{"file_count": 3})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/exec/command":
			writeSandboxEnvelope(t, w, map[string]interface{}{
				"stdout":      "{\"result\":\"ok\"}\n",
				"error":       "",
				"exit_code":   0,
				"duration_ms": 12,
				"truncated":   false,
				"command":     "python3",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/tree":
			writeSandboxEnvelope(t, w, map[string]interface{}{"items": []map[string]interface{}{}})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sandboxes/sbx_test":
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
	if _, err := runtime.CallSkillTool(context.Background(), &ResolvedSkills{Skills: []SkillDocument{doc}}, "script-skill", SkillScriptToolRun, map[string]interface{}{"input": "hello"}, ExecutionContext{}, "call_1"); err != nil {
		t.Fatalf("run skill script: %v", err)
	}
}

func TestSandboxScriptRunnerAppliesManifestRuntimePolicies(t *testing.T) {
	root := writeTestScriptSkill(t)
	manifest := `{
  "entrypoint": "scripts/run.py",
  "language": "python3",
  "dependency_profile": "workflow-safe",
  "timeout_ms": 2500,
  "allowed_artifact_paths": ["artifacts/public"],
  "max_artifact_count": 1,
  "max_artifact_bytes": 4,
  "result_mode": "mixed"
}`
	if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	downloaded := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			if req["dependency_profile"] != "workflow-safe" {
				t.Fatalf("expected manifest dependency profile, got %#v", req)
			}
			writeSandboxEnvelope(t, w, map[string]interface{}{"id": "sbx_test"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files/upload-archive":
			writeSandboxEnvelope(t, w, map[string]interface{}{"file_count": 3})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/exec/command":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			if req["timeout_seconds"] != float64(3) {
				t.Fatalf("expected rounded manifest timeout, got %#v", req)
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
					{"path": "artifacts/private/skip.txt", "size": 4, "is_directory": false},
					{"path": "artifacts/public/ok.txt", "size": 4, "is_directory": false},
					{"path": "artifacts/public/ignored-by-count.txt", "size": 4, "is_directory": false},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/download":
			path := r.URL.Query().Get("path")
			downloaded = append(downloaded, path)
			writeSandboxEnvelope(t, w, map[string]interface{}{
				"path":     path,
				"content":  "b2s=",
				"encoding": "base64",
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sandboxes/sbx_test":
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
	if !messagesContainArtifacts(result.Messages) {
		t.Fatalf("expected allowed artifact message, got %+v", result.Messages)
	}
	if len(downloaded) != 1 || downloaded[0] != "artifacts/public/ok.txt" {
		t.Fatalf("expected only allowed artifact within count limit to be downloaded, got %#v", downloaded)
	}
}

func TestSandboxScriptRunnerRejectsManifestEntrypointMismatch(t *testing.T) {
	root := writeTestScriptSkill(t)
	if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(`{"entrypoint":"scripts/other.py","language":"python3"}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("sandbox should not be called for invalid local manifest")
	}))
	defer server.Close()

	runtime := NewRuntimeWithCatalog(nil, nil, "").WithScriptRunner(NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: server.URL}))
	doc, err := runtime.LoadCustomSkillDocument(root)
	if err != nil {
		t.Fatalf("load custom skill: %v", err)
	}
	_, err = runtime.CallSkillTool(context.Background(), &ResolvedSkills{Skills: []SkillDocument{doc}}, "script-skill", SkillScriptToolRun, map[string]interface{}{"input": "hello"}, ExecutionContext{}, "call_1")
	if err == nil || !strings.Contains(err.Error(), "entrypoint must be scripts/run.py") {
		t.Fatalf("expected manifest entrypoint rejection, got %v", err)
	}
}

func TestSandboxScriptRunnerUsesOperationTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"id":"sbx_test"}}`))
	}))
	defer server.Close()

	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{
		Endpoint:      server.URL,
		CreateTimeout: time.Millisecond,
	})

	_, err := runner.createSandbox(context.Background(), ExecutionContext{}, defaultSkillDependencyProfile)
	if err == nil {
		t.Fatal("expected create sandbox timeout")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "Client.Timeout exceeded") {
		t.Fatalf("expected deadline error, got %v", err)
	}
}

func TestSandboxScriptRunnerCommandRequestTimeoutIncludesPadding(t *testing.T) {
	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{
		Endpoint:              "http://sandbox.example",
		CommandTimeoutPadding: 2 * time.Second,
	})

	if got := runner.commandRequestTimeout(3); got != 5*time.Second {
		t.Fatalf("command request timeout = %s, want 5s", got)
	}
}

func TestSandboxScriptRunnerReturnsStructuredSandboxError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    -429,
			"message": "workspace byte limit exceeded",
			"data": map[string]interface{}{
				"error_type": "limit_exceeded",
				"limit":      "max_workspace_bytes",
				"maximum":    1024,
			},
		}); err != nil {
			t.Fatalf("write sandbox error: %v", err)
		}
	}))
	defer server.Close()

	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: server.URL})
	err := runner.uploadArchive(context.Background(), "sbx_test", "archive", ExecutionContext{})
	if err == nil {
		t.Fatal("expected sandbox request error")
	}
	var sandboxErr *SandboxRequestError
	if !errors.As(err, &sandboxErr) {
		t.Fatalf("expected SandboxRequestError, got %T %v", err, err)
	}
	if sandboxErr.StatusCode != http.StatusTooManyRequests || sandboxErr.Code != -429 {
		t.Fatalf("unexpected sandbox status/code: %+v", sandboxErr)
	}
	if sandboxErr.Data["error_type"] != "limit_exceeded" || sandboxErr.Data["limit"] != "max_workspace_bytes" {
		t.Fatalf("unexpected sandbox error data: %+v", sandboxErr.Data)
	}
}

func TestSandboxScriptRunnerRecordsStructuredSandboxErrorInTrace(t *testing.T) {
	root := writeTestScriptSkill(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			writeSandboxEnvelope(t, w, map[string]interface{}{"id": "sbx_test"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files/upload-archive":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"code":    -403,
				"message": "sandbox does not belong to organization",
				"data": map[string]interface{}{
					"error_type": "access_denied",
					"code":       "cross_organization_sandbox_access_denied",
				},
			}); err != nil {
				t.Fatalf("write sandbox error: %v", err)
			}
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sandboxes/sbx_test":
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
	if err == nil {
		t.Fatal("expected skill script error")
	}
	var sandboxErr *SandboxRequestError
	if !errors.As(err, &sandboxErr) {
		t.Fatalf("expected SandboxRequestError, got %T %v", err, err)
	}
	if result == nil || result.Trace.Status != "error" {
		t.Fatalf("expected error trace, got %+v", result)
	}
	rawSandboxError, ok := result.Trace.Result["sandbox_error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected sandbox_error trace result, got %+v", result.Trace.Result)
	}
	if rawSandboxError["status_code"] != http.StatusForbidden || rawSandboxError["code"] != -403 {
		t.Fatalf("unexpected trace sandbox error: %+v", rawSandboxError)
	}
	data, ok := rawSandboxError["data"].(map[string]interface{})
	if !ok || data["error_type"] != "access_denied" {
		t.Fatalf("unexpected trace sandbox data: %+v", rawSandboxError["data"])
	}
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
	if archiveFileContent(t, archiveBase64, path) == "" {
		return
	}
}

func archiveFileContent(t *testing.T, archiveBase64 string, path string) string {
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
			rc, err := file.Open()
			if err != nil {
				t.Fatalf("open archive file %s: %v", path, err)
			}
			defer rc.Close()
			content, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read archive file %s: %v", path, err)
			}
			return string(content)
		}
	}
	t.Fatalf("expected archive to contain %s", path)
	return ""
}
