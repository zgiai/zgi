package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestRuntimeLoadsCustomScriptSkillWhenRunnerConfigured(t *testing.T) {
	root := writeRuntimeScriptSkill(t)
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
	root := writeRuntimeScriptSkill(t)

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

func TestSandboxScriptRunnerLegacyCommandFlow(t *testing.T) {
	root := writeScriptSkill(t, false)
	fake := newFakeSandboxServer(t)
	defer fake.server.Close()

	persister := &fakeSkillArtifactPersister{}
	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: fake.server.URL, ArtifactPersister: persister})
	result, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{"input": "hello"}, ExecutionContext{
		OrganizationID: "org-1",
		UserID:         "user-1",
		ConversationID: "conversation-1",
		MessageID:      "message-1",
	}, "call_1")
	if err != nil {
		t.Fatalf("RunSkillScript() error = %v", err)
	}
	if fake.commandRequests != 1 {
		t.Fatalf("command requests = %d, want 1", fake.commandRequests)
	}
	if fake.skillRequests != 0 {
		t.Fatalf("skill requests = %d, want 0", fake.skillRequests)
	}
	if fake.uploadValidateManifest {
		t.Fatalf("legacy upload should not validate skill manifest")
	}
	if fake.deleted != 1 {
		t.Fatalf("deleted sandboxes = %d, want 1", fake.deleted)
	}
	if got := fake.lastCommand["profile"]; got != "skill-python" {
		t.Fatalf("profile = %v, want skill-python", got)
	}
	if got := fake.lastCommand["stdin"]; !strings.Contains(got.(string), `"input":"hello"`) {
		t.Fatalf("stdin = %v, want encoded arguments", got)
	}
	env, ok := fake.lastCommand["env"].(map[string]interface{})
	if !ok || env["ZGI_ORGANIZATION_ID"] != "org-1" || env["ZGI_USER_ID"] != "user-1" {
		t.Fatalf("env = %#v, want ZGI context", fake.lastCommand["env"])
	}
	if !fake.archiveNames["scripts/run.py"] || fake.archiveContainsBackslash {
		t.Fatalf("archive names = %#v contains_backslash=%v", fake.archiveNames, fake.archiveContainsBackslash)
	}
	if len(result.Messages) != 3 {
		t.Fatalf("messages = %#v, want stdout, file, and artifacts", result.Messages)
	}
	if result.Messages[0].Type != tools.ToolInvokeMessageTypeJSON || result.Messages[0].Data["ok"] != true {
		t.Fatalf("stdout message = %#v, want JSON success", result.Messages[0])
	}
	if result.Messages[1].Type != tools.ToolInvokeMessageTypeFile {
		t.Fatalf("message[1] = %#v, want file message", result.Messages[1])
	}
	if persister.persisted != 1 {
		t.Fatalf("persisted = %d, want 1", persister.persisted)
	}
	artifacts := artifactItems(t, result.Messages[2])
	if len(artifacts) != 1 ||
		artifacts[0]["content"] != base64.StdEncoding.EncodeToString([]byte("artifact")) ||
		artifacts[0]["persisted"] != true ||
		artifacts[0]["file_id"] == "" {
		t.Fatalf("artifact message = %#v", result.Messages[2])
	}
}

func TestSandboxScriptRunnerManifestFlowUsesExecSkill(t *testing.T) {
	root := writeScriptSkill(t, true)
	fake := newFakeSandboxServer(t)
	fake.commandStdout = `{"mode":"manifest"}`
	fake.manifestArtifacts = []sandboxFileManifest{{
		Path: "artifacts",
		Items: []sandboxFileManifestItem{
			{Path: "artifacts/report.txt", Size: 6},
			{Path: "artifacts/large.bin", Size: maxSkillScriptArtifactBytes + 1},
		},
		FileCount: 2,
		TotalSize: maxSkillScriptArtifactBytes + 7,
	}}
	defer fake.server.Close()

	persister := &fakeSkillArtifactPersister{}
	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: fake.server.URL, ArtifactPersister: persister})
	result, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{"input": "hello"}, ExecutionContext{}, "call_1")
	if err != nil {
		t.Fatalf("RunSkillScript() error = %v", err)
	}
	if fake.commandRequests != 0 {
		t.Fatalf("command requests = %d, want 0", fake.commandRequests)
	}
	if fake.skillRequests != 1 {
		t.Fatalf("skill requests = %d, want 1", fake.skillRequests)
	}
	if !fake.uploadValidateManifest {
		t.Fatalf("manifest upload should request validation")
	}
	if fake.treeRequests != 0 {
		t.Fatalf("tree requests = %d, want 0 for manifest artifacts", fake.treeRequests)
	}
	if got := fake.lastSkill["stdin"]; !strings.Contains(got.(string), `"input":"hello"`) {
		t.Fatalf("stdin = %v, want encoded arguments", got)
	}
	if result.Messages[0].Data["mode"] != "manifest" {
		t.Fatalf("stdout message = %#v, want manifest JSON", result.Messages[0])
	}
	if result.Messages[1].Type != tools.ToolInvokeMessageTypeFile {
		t.Fatalf("message[1] = %#v, want file message", result.Messages[1])
	}
	artifacts := artifactItems(t, result.Messages[2])
	if len(artifacts) != 2 {
		t.Fatalf("artifacts = %#v, want 2", artifacts)
	}
	if artifacts[0]["content"] == "" || artifacts[0]["persisted"] != true {
		t.Fatalf("small artifact should include content: %#v", artifacts[0])
	}
	if _, ok := artifacts[1]["content"]; ok || artifacts[1]["persisted"] != false || artifacts[1]["reason"] != "size_limit_exceeded" {
		t.Fatalf("large artifact should be metadata only with size reason: %#v", artifacts[1])
	}
}

func TestSandboxScriptRunnerArtifactPersistenceFailureIsRecoverable(t *testing.T) {
	root := writeScriptSkill(t, false)
	fake := newFakeSandboxServer(t)
	defer fake.server.Close()

	persister := &fakeSkillArtifactPersister{fail: true}
	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: fake.server.URL, ArtifactPersister: persister})
	result, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{"input": "hello"}, ExecutionContext{}, "call_1")
	if err != nil {
		t.Fatalf("RunSkillScript() error = %v", err)
	}
	if len(result.Messages) != 2 {
		t.Fatalf("messages = %#v, want stdout and artifact summary", result.Messages)
	}
	artifacts := artifactItems(t, result.Messages[1])
	if len(artifacts) != 1 || artifacts[0]["persisted"] != false || artifacts[0]["reason"] != "persist_failed" {
		t.Fatalf("artifact message = %#v, want recoverable persist failure", result.Messages[1])
	}
}

func TestSandboxScriptRunnerManifestUploadFailureDoesNotFallback(t *testing.T) {
	root := writeScriptSkill(t, true)
	fake := newFakeSandboxServer(t)
	fake.failManifestUpload = true
	defer fake.server.Close()

	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: fake.server.URL})
	result, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{"input": "hello"}, ExecutionContext{}, "call_1")
	if err == nil {
		t.Fatalf("RunSkillScript() error = nil, want manifest validation failure")
	}
	if result == nil || result.Trace.Status != "error" {
		t.Fatalf("result trace = %#v, want error trace", result)
	}
	if fake.commandRequests != 0 || fake.skillRequests != 0 {
		t.Fatalf("fallback executed command=%d skill=%d", fake.commandRequests, fake.skillRequests)
	}
	if fake.deleted != 1 {
		t.Fatalf("deleted sandboxes = %d, want 1", fake.deleted)
	}
}

func TestSandboxScriptRunnerNonzeroExitReturnsToolMessage(t *testing.T) {
	root := writeScriptSkill(t, false)
	fake := newFakeSandboxServer(t)
	fake.commandStdout = "partial output"
	fake.commandStderr = "boom"
	fake.commandExitCode = 2
	defer fake.server.Close()

	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: fake.server.URL})
	result, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{}, ExecutionContext{}, "call_1")
	if err == nil {
		t.Fatalf("RunSkillScript() error = nil, want nonzero exit error")
	}
	if result == nil || result.ToolMessage.Content == "" {
		t.Fatalf("result = %#v, want tool message content", result)
	}
	if len(result.Messages) < 2 || result.Messages[0].Type != tools.ToolInvokeMessageTypeText || result.Messages[1].Type != tools.ToolInvokeMessageTypeLog {
		t.Fatalf("messages = %#v, want text stdout and stderr log", result.Messages)
	}
}

func TestSandboxScriptRunnerRealSandboxE2E(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("ZGI_SANDBOX_E2E_ENDPOINT"))
	if endpoint == "" {
		t.Skip("set ZGI_SANDBOX_E2E_ENDPOINT to run real sandbox E2E")
	}
	root := writeRuntimeScriptSkill(t)
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

func writeScriptSkill(t *testing.T, manifest bool) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "run.py"), []byte("print('ok')\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(run.py) error = %v", err)
	}
	if manifest {
		content := `{"entrypoint":"scripts/run.py","language":"python3","timeout_ms":30000,"allowed_artifact_paths":["artifacts"],"max_artifact_count":10,"max_artifact_bytes":1048576,"result_mode":"mixed"}`
		if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(skill.manifest.json) error = %v", err)
		}
	}
	return root
}

func writeRuntimeScriptSkill(t *testing.T) string {
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

func scriptSkillDocument(root string) SkillDocument {
	return SkillDocument{
		Metadata: SkillMetadata{
			ID:               "script-skill",
			RootPath:         root,
			HasScripts:       true,
			ScriptsSupported: true,
		},
	}
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

type fakeSandboxServer struct {
	t *testing.T

	server *httptest.Server

	commandStdout            string
	commandStderr            string
	commandExitCode          int
	manifestArtifacts        []sandboxFileManifest
	failManifestUpload       bool
	archiveNames             map[string]bool
	archiveContainsBackslash bool
	uploadValidateManifest   bool
	lastCommand              map[string]interface{}
	lastSkill                map[string]interface{}
	commandRequests          int
	skillRequests            int
	treeRequests             int
	deleted                  int
}

type fakeSkillArtifactPersister struct {
	fail      bool
	persisted int
}

func (f *fakeSkillArtifactPersister) PersistSkillScriptArtifact(_ context.Context, request SkillScriptArtifactPersistRequest) (map[string]interface{}, error) {
	if f.fail {
		return nil, io.ErrUnexpectedEOF
	}
	f.persisted++
	filename := request.Name
	if filename == "" {
		filename = filepath.Base(request.Path)
	}
	return map[string]interface{}{
		"zgi_model_identity": "__zgi__file__",
		"id":                 "tool-file-" + filename,
		"related_id":         "tool-file-" + filename,
		"tenant_id":          request.ExecContext.OrganizationID,
		"type":               "document",
		"transfer_method":    "tool_file",
		"filename":           filename,
		"extension":          filepath.Ext(filename),
		"mime_type":          "text/plain",
		"size":               int64(len(request.Data)),
		"url":                "http://example.test/files/" + filename,
		"download_url":       "http://example.test/files/" + filename + "?download=1",
	}, nil
}

func newFakeSandboxServer(t *testing.T) *fakeSandboxServer {
	t.Helper()
	fake := &fakeSandboxServer{
		t:             t,
		commandStdout: `{"ok":true}`,
		archiveNames:  map[string]bool{},
		manifestArtifacts: []sandboxFileManifest{{
			Path:      "artifacts",
			Items:     []sandboxFileManifestItem{{Path: "artifacts/report.txt", Size: int64(len("artifact"))}},
			FileCount: 1,
			TotalSize: int64(len("artifact")),
		}},
	}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	return fake
}

func (f *fakeSandboxServer) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
		writeSandboxEnvelope(w, http.StatusOK, map[string]interface{}{"id": "sbx_test"})
	case r.Method == http.MethodPost && r.URL.Path == "/v1/files/upload-archive":
		f.handleUploadArchive(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/exec/command":
		f.commandRequests++
		f.lastCommand = readJSONBody(f.t, r)
		writeSandboxEnvelope(w, http.StatusOK, f.commandResult())
	case r.Method == http.MethodPost && r.URL.Path == "/v1/exec/skill":
		f.skillRequests++
		f.lastSkill = readJSONBody(f.t, r)
		writeSandboxEnvelope(w, http.StatusOK, map[string]interface{}{
			"execution_id":       "exec_skill",
			"sandbox_id":         "sbx_test",
			"path":               ".",
			"manifest":           map[string]interface{}{"entrypoint": "scripts/run.py", "language": "python3"},
			"command":            f.commandResult(),
			"artifact_manifests": f.manifestArtifacts,
		})
	case r.Method == http.MethodGet && r.URL.Path == "/v1/files/tree":
		f.treeRequests++
		writeSandboxEnvelope(w, http.StatusOK, map[string]interface{}{
			"items": []map[string]interface{}{
				{"path": "artifacts/report.txt", "size": int64(len("artifact")), "is_directory": false},
			},
		})
	case r.Method == http.MethodGet && r.URL.Path == "/v1/files/download":
		writeSandboxEnvelope(w, http.StatusOK, map[string]interface{}{
			"path":     r.URL.Query().Get("path"),
			"content":  base64.StdEncoding.EncodeToString([]byte("artifact")),
			"encoding": "base64",
		})
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/sandboxes/"):
		f.deleted++
		writeSandboxEnvelope(w, http.StatusOK, map[string]interface{}{})
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeSandboxServer) handleUploadArchive(w http.ResponseWriter, r *http.Request) {
	body := readJSONBody(f.t, r)
	validate, _ := body["validate_skill_manifest"].(bool)
	f.uploadValidateManifest = validate
	if validate && f.failManifestUpload {
		writeSandboxEnvelopeWithMessage(w, http.StatusBadRequest, -400, "invalid skill manifest")
		return
	}
	rawArchive, _ := body["archive_base64"].(string)
	archiveBytes, err := base64.StdEncoding.DecodeString(rawArchive)
	if err != nil {
		f.t.Fatalf("DecodeString(archive_base64) error = %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		f.t.Fatalf("zip.NewReader() error = %v", err)
	}
	for _, file := range reader.File {
		f.archiveNames[file.Name] = true
		if strings.Contains(file.Name, `\`) {
			f.archiveContainsBackslash = true
		}
	}
	writeSandboxEnvelope(w, http.StatusOK, map[string]interface{}{})
}

func (f *fakeSandboxServer) commandResult() map[string]interface{} {
	return map[string]interface{}{
		"stdout":      f.commandStdout,
		"error":       f.commandStderr,
		"exit_code":   f.commandExitCode,
		"duration_ms": int64(12),
		"truncated":   false,
		"command":     "python3",
		"args":        []string{"scripts/run.py"},
		"backend":     "fake",
	}
}

func readJSONBody(t *testing.T, r *http.Request) map[string]interface{} {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", string(raw), err)
	}
	return body
}

func writeSandboxEnvelope(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "success",
		"data":    data,
	})
}

func writeSandboxEnvelopeWithMessage(w http.ResponseWriter, status int, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    code,
		"message": message,
		"data":    nil,
	})
}

func artifactItems(t *testing.T, message tools.ToolInvokeMessage) []map[string]interface{} {
	t.Helper()
	if message.Type != tools.ToolInvokeMessageTypeJSON {
		t.Fatalf("message type = %s, want json", message.Type)
	}
	switch rawItems := message.Data["artifacts"].(type) {
	case []map[string]interface{}:
		return rawItems
	case []interface{}:
		items := make([]map[string]interface{}, 0, len(rawItems))
		for _, raw := range rawItems {
			item, ok := raw.(map[string]interface{})
			if !ok {
				t.Fatalf("artifact item = %#v, want map", raw)
			}
			items = append(items, item)
		}
		return items
	default:
		t.Fatalf("artifacts = %#v, want artifact list", message.Data["artifacts"])
		return nil
	}
}
