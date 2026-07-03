package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
		artifacts[0]["content_type"] != "text/plain" ||
		artifacts[0]["persisted"] != true ||
		artifacts[0]["file_id"] == "" {
		t.Fatalf("artifact message = %#v", result.Messages[2])
	}
	if len(persister.requests) != 1 || persister.requests[0].ContentType != "text/plain" {
		t.Fatalf("persist requests = %#v, want stable text/plain content type", persister.requests)
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
	result, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{"input": "hello"}, ExecutionContext{
		OrganizationID: "org-1",
		UserID:         "user-1",
		ConversationID: "conversation-1",
		MessageID:      "message-1",
	}, "call_1")
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
	env, ok := fake.lastSkill["env"].(map[string]interface{})
	if !ok ||
		env["ZGI_ORGANIZATION_ID"] != "org-1" ||
		env["ZGI_USER_ID"] != "user-1" ||
		env["ZGI_CONVERSATION_ID"] != "conversation-1" ||
		env["ZGI_MESSAGE_ID"] != "message-1" {
		t.Fatalf("manifest env = %#v, want ZGI context", fake.lastSkill["env"])
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

func TestSandboxScriptRunnerManifestInputFilesUploadToInputsAndStdin(t *testing.T) {
	root := writeRuntimeScriptSkill(t)
	manifest := `{
  "entrypoint": "scripts/run.py",
  "language": "python3",
  "input_files": [
    {
      "name": "confirmation",
      "argument": "confirmation_file_id",
      "required": true,
      "extensions": [".xlsx"],
      "mime_types": ["application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"],
      "max_bytes": 1048576
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	fake := newFakeSandboxServer(t)
	defer fake.server.Close()

	provider := &fakeSkillInputFileProvider{files: map[string]SkillScriptInputFile{
		"file-1": {
			FileID:    "file-1",
			Filename:  "../original.xlsx",
			Extension: ".xlsx",
			MimeType:  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			Size:      4,
			Data:      []byte("xlsx"),
		},
	}}
	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{
		Endpoint:          fake.server.URL,
		ArtifactPersister: &fakeSkillArtifactPersister{},
		InputFileProvider: provider,
	})
	result, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{
		"confirmation_file_id": "file-1",
		"payer_name":           "Tenant A",
	}, ExecutionContext{OrganizationID: "org-1", UserID: "user-1"}, "call_1")
	if err != nil {
		t.Fatalf("RunSkillScript() error = %v", err)
	}
	if result == nil || fake.skillRequests != 1 {
		t.Fatalf("result=%#v skill_requests=%d, want manifest execution", result, fake.skillRequests)
	}
	if fake.uploadArchiveRequests != 2 {
		t.Fatalf("upload archive requests = %d, want skill package and input files", fake.uploadArchiveRequests)
	}
	if !fake.archiveNames["inputs/confirmation/original.xlsx"] {
		t.Fatalf("archive names = %#v, want sanitized input file path", fake.archiveNames)
	}
	stdin, _ := fake.lastSkill["stdin"].(string)
	if !strings.Contains(stdin, `"confirmation_file_id":"file-1"`) ||
		!strings.Contains(stdin, `"input_files"`) ||
		!strings.Contains(stdin, `"path":"inputs/confirmation/original.xlsx"`) ||
		!strings.Contains(stdin, `"filename":"original.xlsx"`) {
		t.Fatalf("stdin = %s, want original args and input_files metadata", stdin)
	}
}

func TestSandboxScriptRunnerManifestMultipleInputFilesUploadToInputsAndStdin(t *testing.T) {
	root := writeRuntimeScriptSkill(t)
	manifest := `{
  "entrypoint": "scripts/run.py",
  "language": "python3",
  "input_files": [
    {
      "name": "confirmations",
      "argument": "confirmation_file_ids",
      "required": true,
      "multiple": true,
      "max_count": 2,
      "extensions": [".xlsx"],
      "mime_types": ["application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"],
      "max_bytes": 1048576
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	fake := newFakeSandboxServer(t)
	defer fake.server.Close()

	provider := &fakeSkillInputFileProvider{files: map[string]SkillScriptInputFile{
		"file-a": {
			FileID:    "file-a",
			Filename:  "a.xlsx",
			Extension: ".xlsx",
			MimeType:  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			Size:      4,
			Data:      []byte("xlsx"),
		},
		"file-b": {
			FileID:    "file-b",
			Filename:  "b.xlsx",
			Extension: ".xlsx",
			MimeType:  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			Size:      4,
			Data:      []byte("xlsx"),
		},
	}}
	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{
		Endpoint:          fake.server.URL,
		ArtifactPersister: &fakeSkillArtifactPersister{},
		InputFileProvider: provider,
	})
	_, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{
		"confirmation_file_ids": []interface{}{"file-a", "file-b"},
	}, ExecutionContext{OrganizationID: "org-1", UserID: "user-1"}, "call_1")
	if err != nil {
		t.Fatalf("RunSkillScript() error = %v", err)
	}
	if !fake.archiveNames["inputs/confirmations/file-a/a.xlsx"] || !fake.archiveNames["inputs/confirmations/file-b/b.xlsx"] {
		t.Fatalf("archive names = %#v, want multiple input file paths", fake.archiveNames)
	}
	stdin, _ := fake.lastSkill["stdin"].(string)
	if !strings.Contains(stdin, `"confirmation_file_ids":["file-a","file-b"]`) ||
		!strings.Contains(stdin, `"confirmations":[`) ||
		!strings.Contains(stdin, `"path":"inputs/confirmations/file-a/a.xlsx"`) ||
		!strings.Contains(stdin, `"path":"inputs/confirmations/file-b/b.xlsx"`) {
		t.Fatalf("stdin = %s, want multiple input_files metadata", stdin)
	}
}

func TestSandboxScriptRunnerManifestInputFileValidationStopsBeforeSandbox(t *testing.T) {
	root := writeRuntimeScriptSkill(t)
	manifest := `{"entrypoint":"scripts/run.py","language":"python3","input_files":[{"name":"confirmation","argument":"confirmation_file_id","required":true,"extensions":[".xlsx"],"mime_types":["application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"],"max_bytes":4}]}`
	if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	t.Run("missing required file id", func(t *testing.T) {
		fake := newFakeSandboxServer(t)
		defer fake.server.Close()
		runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{
			Endpoint:          fake.server.URL,
			InputFileProvider: &fakeSkillInputFileProvider{},
		})
		_, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{}, ExecutionContext{}, "call_1")
		if err == nil || !strings.Contains(err.Error(), "requires argument confirmation_file_id") {
			t.Fatalf("error = %v, want missing file_id", err)
		}
		if fake.uploadArchiveRequests != 0 || fake.skillRequests != 0 || fake.commandRequests != 0 {
			t.Fatalf("sandbox calls before validation failure: uploads=%d skill=%d command=%d", fake.uploadArchiveRequests, fake.skillRequests, fake.commandRequests)
		}
	})

	t.Run("extension mismatch", func(t *testing.T) {
		fake := newFakeSandboxServer(t)
		defer fake.server.Close()
		runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{
			Endpoint: fake.server.URL,
			InputFileProvider: &fakeSkillInputFileProvider{files: map[string]SkillScriptInputFile{
				"file-1": {FileID: "file-1", Filename: "bad.txt", Extension: ".txt", MimeType: "text/plain", Size: 2, Data: []byte("no")},
			}},
		})
		_, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{"confirmation_file_id": "file-1"}, ExecutionContext{}, "call_1")
		if err == nil || !strings.Contains(err.Error(), "extension .txt is not allowed") {
			t.Fatalf("error = %v, want extension validation", err)
		}
		if fake.uploadArchiveRequests != 0 || fake.skillRequests != 0 || fake.commandRequests != 0 {
			t.Fatalf("sandbox calls before validation failure: uploads=%d skill=%d command=%d", fake.uploadArchiveRequests, fake.skillRequests, fake.commandRequests)
		}
	})

	t.Run("mime mismatch", func(t *testing.T) {
		fake := newFakeSandboxServer(t)
		defer fake.server.Close()
		runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{
			Endpoint: fake.server.URL,
			InputFileProvider: &fakeSkillInputFileProvider{files: map[string]SkillScriptInputFile{
				"file-1": {FileID: "file-1", Filename: "bad.xlsx", Extension: ".xlsx", MimeType: "text/plain", Size: 2, Data: []byte("no")},
			}},
		})
		_, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{"confirmation_file_id": "file-1"}, ExecutionContext{}, "call_1")
		if err == nil || !strings.Contains(err.Error(), "mime type text/plain is not allowed") {
			t.Fatalf("error = %v, want mime validation", err)
		}
		if fake.uploadArchiveRequests != 0 || fake.skillRequests != 0 || fake.commandRequests != 0 {
			t.Fatalf("sandbox calls before validation failure: uploads=%d skill=%d command=%d", fake.uploadArchiveRequests, fake.skillRequests, fake.commandRequests)
		}
	})

	t.Run("size limit", func(t *testing.T) {
		fake := newFakeSandboxServer(t)
		defer fake.server.Close()
		runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{
			Endpoint: fake.server.URL,
			InputFileProvider: &fakeSkillInputFileProvider{files: map[string]SkillScriptInputFile{
				"file-1": {FileID: "file-1", Filename: "big.xlsx", Extension: ".xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Size: 5, Data: []byte("12345")},
			}},
		})
		_, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{"confirmation_file_id": "file-1"}, ExecutionContext{}, "call_1")
		if err == nil || !strings.Contains(err.Error(), "exceeds max_bytes 4") {
			t.Fatalf("error = %v, want size validation", err)
		}
		if fake.uploadArchiveRequests != 0 || fake.skillRequests != 0 || fake.commandRequests != 0 {
			t.Fatalf("sandbox calls before validation failure: uploads=%d skill=%d command=%d", fake.uploadArchiveRequests, fake.skillRequests, fake.commandRequests)
		}
	})
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

func TestSkillArtifactMimeTypeUsesStableExtensionMapping(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"evaluation-summary.json", "application/json"},
		{"report.html", "text/html"},
		{"data.csv", "text/csv"},
		{"notes.txt", "text/plain"},
		{"README.md", "text/markdown"},
		{"paper.pdf", "application/pdf"},
		{"image.png", "image/png"},
		{"photo.jpg", "image/jpeg"},
		{"photo.jpeg", "image/jpeg"},
		{"preview.webp", "image/webp"},
		{"diagram.svg", "image/svg+xml"},
		{"bundle.zip", "application/zip"},
		{"workbook.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{"document.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"slides.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			if got := skillArtifactMimeType(tt.filename, "", []byte("{}")); got != tt.want {
				t.Fatalf("skillArtifactMimeType(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
	if got := skillArtifactMimeType("result.json", "application/x-custom; charset=utf-8", []byte("{}")); got != "application/x-custom" {
		t.Fatalf("explicit content type = %q, want application/x-custom", got)
	}
	if got := skillArtifactMimeType("result.json", "text/plain; charset=utf-8", []byte("{}")); got != "application/json" {
		t.Fatalf("generic content type override = %q, want application/json", got)
	}
	if got := skillArtifactMimeType("workbook.xlsx", "application/zip", []byte("PK\x03\x04")); got != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("office zip content type override = %q, want xlsx MIME", got)
	}
}

func TestSandboxScriptRunnerNodeManifestUsesExecSkill(t *testing.T) {
	root := writeNodeManifestSkill(t)
	fake := newFakeSandboxServer(t)
	fake.commandStdout = `{"mode":"node"}`
	defer fake.server.Close()

	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: fake.server.URL, ArtifactPersister: &fakeSkillArtifactPersister{}})
	result, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{"input": "hello"}, ExecutionContext{
		MessageID: "message-node",
	}, "call_1")
	if err != nil {
		t.Fatalf("RunSkillScript() error = %v", err)
	}
	if fake.commandRequests != 0 || fake.skillRequests != 1 {
		t.Fatalf("requests command=%d skill=%d, want node manifest through exec/skill only", fake.commandRequests, fake.skillRequests)
	}
	if !fake.uploadValidateManifest {
		t.Fatal("node manifest upload should request manifest validation")
	}
	if !fake.archiveNames["scripts/run.js"] || fake.archiveNames["scripts/run.py"] {
		t.Fatalf("archive names = %#v, want node entrypoint without python fallback", fake.archiveNames)
	}
	env, ok := fake.lastSkill["env"].(map[string]interface{})
	if !ok || env["ZGI_MESSAGE_ID"] != "message-node" {
		t.Fatalf("node manifest env = %#v, want message env", fake.lastSkill["env"])
	}
	if result.Messages[0].Data["mode"] != "node" {
		t.Fatalf("stdout message = %#v, want node JSON", result.Messages[0])
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

func TestSandboxScriptRunnerAutoBuildsManifestSkillDependencies(t *testing.T) {
	root := writeTestScriptSkill(t)
	if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(`{"entrypoint":"scripts/run.py","language":"python3","dependency_profile":"workflow-safe"}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "requirements.txt"), []byte("openpyxl==3.1.5\n"), 0o644); err != nil {
		t.Fatalf("write requirements: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandbox/dependencies/builds":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			if req["organization_id"] != "organization-auto-office" {
				t.Fatalf("expected organization id in dependency build request, got %#v", req)
			}
			if archiveFileContent(t, req["archive_base64"].(string), "requirements.txt") == "" {
				t.Fatalf("expected dependency build archive to include requirements.txt")
			}
			writeSandboxOK(t, w, map[string]interface{}{
				"build_id":     "depbuild_office",
				"fingerprint":  "sha256:office",
				"status":       "ready",
				"profile_name": "auto-office",
				"next_action":  "use_dependency_profile",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandbox/dependencies":
			writeSandboxDependencyCatalog(t, w)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			if req["dependency_profile"] != "auto-office" {
				t.Fatalf("expected prepared dependency profile, got %#v", req)
			}
			writeSandboxOK(t, w, map[string]interface{}{"id": "sbx_test"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files/upload-archive":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			archive, _ := req["archive_base64"].(string)
			manifestContent := archiveFileContent(t, archive, "skill.manifest.json")
			if strings.Contains(manifestContent, "workflow-safe") || !strings.Contains(manifestContent, `"dependency_profile":"auto-office"`) {
				t.Fatalf("expected normalized manifest with prepared dependency profile, got %s", manifestContent)
			}
			writeSandboxOK(t, w, map[string]interface{}{"file_count": 3})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/exec/skill":
			writeSandboxOK(t, w, map[string]interface{}{
				"command": map[string]interface{}{
					"stdout":      "{\"result\":\"ok\"}\n",
					"error":       "",
					"exit_code":   0,
					"duration_ms": 12,
					"truncated":   false,
					"command":     "python3",
				},
				"artifact_manifests": []map[string]interface{}{},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sandboxes/sbx_test":
			writeSandboxOK(t, w, map[string]interface{}{"deleted": true})
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
	if _, err := runtime.CallSkillTool(context.Background(), &ResolvedSkills{Skills: []SkillDocument{doc}}, "script-skill", SkillScriptToolRun, map[string]interface{}{"input": "hello"}, ExecutionContext{OrganizationID: "organization-auto-office"}, "call_1"); err != nil {
		t.Fatalf("run skill script: %v", err)
	}
}

func TestSandboxScriptRunnerAutoBuildsDefaultDependencyProfile(t *testing.T) {
	root := writeTestScriptSkill(t)
	if err := os.WriteFile(filepath.Join(root, "requirements.txt"), []byte("pandas==2.2.3\n"), 0o644); err != nil {
		t.Fatalf("write requirements: %v", err)
	}

	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandbox/dependencies/builds":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			if req["organization_id"] != "organization-auto-deps" {
				t.Fatalf("expected organization id in dependency build request, got %#v", req)
			}
			if archiveFileContent(t, req["archive_base64"].(string), "requirements.txt") == "" {
				t.Fatalf("expected dependency build archive to include requirements.txt")
			}
			writeSandboxOK(t, w, map[string]interface{}{
				"build_id":     "depbuild_auto",
				"fingerprint":  "sha256:auto",
				"status":       "queued",
				"profile_name": "auto-deps",
				"next_action":  "wait_for_dependency_build",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandbox/dependencies/builds/sha256:auto":
			writeSandboxOK(t, w, map[string]interface{}{
				"build_id":          "depbuild_auto",
				"fingerprint":       "sha256:auto",
				"status":            "ready",
				"profile_name":      "auto-deps",
				"artifact_checksum": "sha256:artifact",
				"next_action":       "use_dependency_profile",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandbox/dependencies":
			writeSandboxOK(t, w, map[string]interface{}{
				"language":             "python3",
				"mode":                 "managed-profiles",
				"supports_user_update": false,
				"profiles": []map[string]interface{}{
					{"name": "auto-deps", "version": "sha256-auto", "status": "ready", "enabled": true, "languages": []string{"python3"}},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			if req["dependency_profile"] != "auto-deps" {
				t.Fatalf("expected auto dependency profile, got %#v", req)
			}
			writeSandboxOK(t, w, map[string]interface{}{"id": "sbx_test"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files/upload-archive":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			manifestContent := archiveFileContent(t, req["archive_base64"].(string), "skill.manifest.json")
			if !strings.Contains(manifestContent, `"dependency_profile":"auto-deps"`) {
				t.Fatalf("expected uploaded manifest to use auto dependency profile, got %s", manifestContent)
			}
			writeSandboxOK(t, w, map[string]interface{}{"file_count": 4})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/exec/command":
			writeSandboxOK(t, w, map[string]interface{}{
				"stdout":      "{\"result\":\"ok\"}\n",
				"error":       "",
				"exit_code":   0,
				"duration_ms": 12,
				"truncated":   false,
				"command":     "python3",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/tree":
			writeSandboxOK(t, w, map[string]interface{}{"items": []map[string]interface{}{}})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sandboxes/sbx_test":
			writeSandboxOK(t, w, map[string]interface{}{"deleted": true})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{
		Endpoint:                    server.URL,
		DependencyBuildTimeout:      2 * time.Second,
		DependencyBuildPollInterval: time.Millisecond,
	})
	if _, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{"input": "hello"}, ExecutionContext{OrganizationID: "organization-auto-deps"}, "call_1"); err != nil {
		t.Fatalf("run skill script: %v; requests=%v", err, requests)
	}
}

func TestSkillPackageDependencyHintsDetectThirdPartyImports(t *testing.T) {
	root := writePythonScenarioSkill(t, "import json\nimport pandas as pd\nprint('ok')")
	if !skillPackageHasDependencyHints(root) {
		t.Fatal("expected third-party Python import to trigger dependency prepare")
	}

	stdlibRoot := writePythonScenarioSkill(t, "import json\nfrom pathlib import Path\nprint(Path('.'))")
	if skillPackageHasDependencyHints(stdlibRoot) {
		t.Fatal("did not expect stdlib-only Python imports to trigger dependency prepare")
	}
}

func TestSandboxScriptRunnerIgnoresManifestDependencyProfile(t *testing.T) {
	root := writeTestScriptSkill(t)
	if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(`{"entrypoint":"scripts/run.py","language":"python3","dependency_profile":"missing-profile"}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandbox/dependencies":
			writeSandboxDependencyCatalog(t, w)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			if req["dependency_profile"] != "stdlib" {
				t.Fatalf("expected manifest dependency profile to be ignored, got %#v", req)
			}
			writeSandboxOK(t, w, map[string]interface{}{"id": "sbx_test"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files/upload-archive":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			manifestContent := archiveFileContent(t, req["archive_base64"].(string), "skill.manifest.json")
			if strings.Contains(manifestContent, "missing-profile") || !strings.Contains(manifestContent, `"dependency_profile":"stdlib"`) {
				t.Fatalf("expected normalized manifest to use platform-selected stdlib, got %s", manifestContent)
			}
			writeSandboxOK(t, w, map[string]interface{}{"file_count": 3})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/exec/skill":
			writeSandboxOK(t, w, map[string]interface{}{
				"command": map[string]interface{}{
					"stdout":      "{\"result\":\"ok\"}\n",
					"error":       "",
					"exit_code":   0,
					"duration_ms": 12,
					"truncated":   false,
					"command":     "python3",
				},
				"artifact_manifests": []map[string]interface{}{},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sandboxes/sbx_test":
			writeSandboxOK(t, w, map[string]interface{}{"deleted": true})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: server.URL})
	result, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{"input": "hello"}, ExecutionContext{}, "call_1")
	if err != nil {
		t.Fatalf("manifest dependency profile should be ignored and reset to stdlib, got %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
}

func TestSandboxScriptRunnerSendsSandboxAPIKeyHeader(t *testing.T) {
	root := writeTestScriptSkill(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "sandbox-key" {
			t.Fatalf("X-API-Key = %q, want sandbox-key", got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandbox/dependencies/prepare":
			writeSandboxOK(t, w, dependencyBuildResponse("ready", "stdlib"))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandbox/dependencies":
			writeSandboxDependencyCatalog(t, w)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			writeSandboxOK(t, w, map[string]interface{}{"id": "sbx_test"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files/upload-archive":
			writeSandboxOK(t, w, map[string]interface{}{"file_count": 3})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/exec/command":
			writeSandboxOK(t, w, map[string]interface{}{
				"stdout":      "{\"result\":\"ok\"}\n",
				"error":       "",
				"exit_code":   0,
				"duration_ms": 12,
				"truncated":   false,
				"command":     "python3",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/tree":
			writeSandboxOK(t, w, map[string]interface{}{"items": []map[string]interface{}{}})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sandboxes/sbx_test":
			writeSandboxOK(t, w, map[string]interface{}{"deleted": true})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: server.URL, APIKey: "sandbox-key"})
	if _, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{"input": "hello"}, ExecutionContext{}, "call_1"); err != nil {
		t.Fatalf("run skill script: %v", err)
	}
}

func TestSandboxScriptRunnerFailsClosedWhenDependencyCatalogUnavailable(t *testing.T) {
	root := writeTestScriptSkill(t)
	catalogRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandbox/dependencies/prepare":
			writeSandboxOK(t, w, dependencyBuildResponse("ready", "stdlib"))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandbox/dependencies":
			catalogRequests++
			writeSandboxError(t, w, http.StatusServiceUnavailable, -503, "catalog unavailable")
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			t.Fatalf("sandbox should not be created when dependency catalog is unavailable")
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: server.URL})
	_, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{"input": "hello"}, ExecutionContext{}, "call_1")
	if err == nil || !strings.Contains(err.Error(), "skill dependency profile preflight failed") {
		t.Fatalf("expected fail-closed dependency catalog error, got %v", err)
	}
	if catalogRequests != defaultSandboxIdempotentAttempts {
		t.Fatalf("catalog requests = %d, want %d retry attempts", catalogRequests, defaultSandboxIdempotentAttempts)
	}
}

func TestSandboxScriptRunnerAppliesManifestRuntimePolicies(t *testing.T) {
	root := writeTestScriptSkill(t)
	manifest := `{
  "entrypoint": "scripts/run.py",
  "language": "python3",
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
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandbox/dependencies/prepare":
			writeSandboxOK(t, w, dependencyBuildResponse("ready", "stdlib"))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandbox/dependencies":
			writeSandboxDependencyCatalog(t, w)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			if req["dependency_profile"] != "stdlib" {
				t.Fatalf("expected prepared default dependency profile, got %#v", req)
			}
			writeSandboxOK(t, w, map[string]interface{}{"id": "sbx_test"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files/upload-archive":
			writeSandboxOK(t, w, map[string]interface{}{"file_count": 3})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/exec/skill":
			var req map[string]interface{}
			decodeJSON(t, r, &req)
			writeSandboxOK(t, w, map[string]interface{}{
				"command": map[string]interface{}{
					"stdout":      "{\"result\":\"ok\"}\n",
					"error":       "",
					"exit_code":   0,
					"duration_ms": 12,
					"truncated":   false,
					"command":     "python3",
				},
				"artifact_manifests": []map[string]interface{}{
					{
						"path": "artifacts",
						"items": []map[string]interface{}{
							{"path": "artifacts/private/skip.txt", "size": 4},
							{"path": "artifacts/public/ok.txt", "size": 4},
							{"path": "artifacts/public/ignored-by-count.txt", "size": 4},
						},
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/download":
			path := r.URL.Query().Get("path")
			downloaded = append(downloaded, path)
			writeSandboxOK(t, w, map[string]interface{}{
				"path":     path,
				"content":  "b2s=",
				"encoding": "base64",
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sandboxes/sbx_test":
			writeSandboxOK(t, w, map[string]interface{}{"deleted": true})
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
	if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(`{"entrypoint":"references/other.py","language":"python3"}`), 0o644); err != nil {
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
	if err == nil || !strings.Contains(err.Error(), "entrypoint must be under scripts/") {
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
	err := runner.uploadArchive(context.Background(), "sbx_test", "archive", ExecutionContext{}, true)
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
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandbox/dependencies/prepare":
			writeSandboxOK(t, w, dependencyBuildResponse("ready", "stdlib"))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandbox/dependencies":
			writeSandboxDependencyCatalog(t, w)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			writeSandboxOK(t, w, map[string]interface{}{"id": "sbx_test"})
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
			writeSandboxOK(t, w, map[string]interface{}{"deleted": true})
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

func TestSandboxScriptRunnerRetriesIdempotentSandboxRequests(t *testing.T) {
	treeCalls := 0
	downloadCalls := 0
	deleteCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/tree":
			treeCalls++
			if treeCalls == 1 {
				writeSandboxError(t, w, http.StatusServiceUnavailable, -503, "temporary tree failure")
				return
			}
			writeSandboxOK(t, w, map[string]interface{}{
				"items": []map[string]interface{}{
					{"path": "artifacts/report.txt", "size": 4, "is_directory": false},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/download":
			downloadCalls++
			if downloadCalls == 1 {
				writeSandboxError(t, w, http.StatusGatewayTimeout, -504, "temporary download failure")
				return
			}
			writeSandboxOK(t, w, map[string]interface{}{
				"path":     "artifacts/report.txt",
				"content":  "b2s=",
				"encoding": "base64",
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sandboxes/sbx_test":
			deleteCalls++
			if deleteCalls == 1 {
				writeSandboxError(t, w, http.StatusBadGateway, -502, "temporary delete failure")
				return
			}
			writeSandboxOK(t, w, map[string]interface{}{"deleted": true})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: server.URL, ArtifactPersister: &fakeSkillArtifactPersister{}})
	artifacts, err := runner.collectArtifacts(context.Background(), "sbx_test", ExecutionContext{}, skillScriptManifest{
		AllowedArtifactPaths: []string{"artifacts"},
		MaxArtifactCount:     10,
		MaxArtifactBytes:     32 * 1024,
	})
	if err != nil {
		t.Fatalf("collect artifacts: %v", err)
	}
	runner.prepareArtifacts(context.Background(), "sbx_test", artifacts, ExecutionContext{OrganizationID: "org-1", UserID: "user-1"}, 32*1024)
	if len(artifacts) != 1 || artifacts[0].Path != "artifacts/report.txt" || artifacts[0].Content != "b2s=" {
		t.Fatalf("unexpected artifacts after retry: %+v", artifacts)
	}
	if err := runner.deleteSandbox(context.Background(), "sbx_test", ExecutionContext{}); err != nil {
		t.Fatalf("delete sandbox after retry: %v", err)
	}
	if treeCalls != 2 || downloadCalls != 2 || deleteCalls != 2 {
		t.Fatalf("expected one retry per idempotent request, got tree=%d download=%d delete=%d", treeCalls, downloadCalls, deleteCalls)
	}
}

func TestSandboxScriptRunnerDoesNotRetryNonIdempotentUpload(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		writeSandboxError(t, w, http.StatusServiceUnavailable, -503, "temporary upload failure")
	}))
	defer server.Close()

	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: server.URL})
	err := runner.uploadArchive(context.Background(), "sbx_test", "archive", ExecutionContext{}, true)
	if err == nil {
		t.Fatal("expected upload error")
	}
	if calls != 1 {
		t.Fatalf("expected non-idempotent upload to run once, got %d calls", calls)
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

func TestSandboxScriptRunnerRealSandboxBacktestScenarios(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("ZGI_SANDBOX_E2E_ENDPOINT"))
	if endpoint == "" {
		t.Skip("set ZGI_SANDBOX_E2E_ENDPOINT to run real sandbox backtest")
	}
	apiKey := strings.TrimSpace(os.Getenv("ZGI_SANDBOX_E2E_API_KEY"))

	t.Run("json artifact", func(t *testing.T) {
		persister := &fakeSkillArtifactPersister{}
		result := runRealSandboxSkill(t, endpoint, apiKey, writePythonScenarioSkill(t, `
import json, os
os.makedirs("artifacts", exist_ok=True)
open("artifacts/evaluation-summary.json", "w", encoding="utf-8").write(json.dumps({"case": "json", "count": 1}))
print(json.dumps({"success": True, "case": "json"}))
`), persister)
		artifacts := firstArtifactItems(t, result.Messages)
		if len(artifacts) != 1 || artifacts[0]["name"] != "evaluation-summary.json" || artifacts[0]["content_type"] != "application/json" {
			t.Fatalf("json artifacts = %#v", artifacts)
		}
		if len(persister.requests) != 1 || persister.requests[0].ContentType != "application/json" {
			t.Fatalf("persist requests = %#v, want application/json", persister.requests)
		}
	})

	t.Run("html artifact", func(t *testing.T) {
		persister := &fakeSkillArtifactPersister{}
		result := runRealSandboxSkill(t, endpoint, apiKey, writePythonScenarioSkill(t, `
import json, os
os.makedirs("artifacts", exist_ok=True)
open("artifacts/report.html", "w", encoding="utf-8").write("<!doctype html><title>Skill Report</title><h1>OK</h1>")
print(json.dumps({"success": True, "case": "html"}))
`), persister)
		artifacts := firstArtifactItems(t, result.Messages)
		if len(artifacts) != 1 || artifacts[0]["name"] != "report.html" || artifacts[0]["content_type"] != "text/html" {
			t.Fatalf("html artifacts = %#v", artifacts)
		}
	})

	t.Run("large artifact skipped", func(t *testing.T) {
		persister := &fakeSkillArtifactPersister{}
		result := runRealSandboxSkill(t, endpoint, apiKey, writePythonScenarioSkill(t, `
import json, os
os.makedirs("artifacts", exist_ok=True)
open("artifacts/large.bin", "wb").write(b"x" * (2 * 1024 * 1024 + 1))
print(json.dumps({"success": True, "case": "large"}))
`), persister)
		artifacts := firstArtifactItems(t, result.Messages)
		if len(artifacts) != 1 || artifacts[0]["persisted"] != false || artifacts[0]["reason"] != "size_limit_exceeded" {
			t.Fatalf("large artifacts = %#v", artifacts)
		}
		if persister.persisted != 0 {
			t.Fatalf("persisted = %d, want 0 for large artifact", persister.persisted)
		}
	})

	t.Run("stderr warning", func(t *testing.T) {
		result := runRealSandboxSkill(t, endpoint, apiKey, writePythonScenarioSkill(t, `
import json, sys
sys.stderr.write("warning from skill\n")
print(json.dumps({"success": True, "case": "stderr"}))
`), &fakeSkillArtifactPersister{})
		if !messagesContainLog(result.Messages, "warning from skill") {
			t.Fatalf("messages = %#v, want stderr log", result.Messages)
		}
	})

	t.Run("nonzero exit", func(t *testing.T) {
		root := writePythonScenarioSkill(t, `
import sys
sys.stderr.write("boom\n")
print("partial output")
sys.exit(7)
`)
		runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: endpoint, APIKey: apiKey})
		result, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{}, ExecutionContext{}, "call_real_nonzero")
		if err == nil {
			t.Fatal("RunSkillScript() error = nil, want nonzero exit error")
		}
		if result == nil || result.ToolMessage.Content == "" || !messagesContainLog(result.Messages, "boom") {
			t.Fatalf("result = %#v, want tool message and stderr log", result)
		}
	})

	t.Run("node manifest", func(t *testing.T) {
		persister := &fakeSkillArtifactPersister{}
		result := runRealSandboxSkill(t, endpoint, apiKey, writeNodeScenarioSkill(t), persister)
		artifacts := firstArtifactItems(t, result.Messages)
		if len(artifacts) != 1 || artifacts[0]["name"] != "node-result.json" || artifacts[0]["content_type"] != "application/json" {
			t.Fatalf("node artifacts = %#v", artifacts)
		}
	})
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

func writeTestScriptSkill(t *testing.T) string {
	t.Helper()
	return writeRuntimeScriptSkill(t)
}

func writeNodeManifestSkill(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "run.js"), []byte("console.log(JSON.stringify({ok:true}))\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(run.js) error = %v", err)
	}
	content := `{"entrypoint":"scripts/run.js","language":"nodejs","timeout_ms":30000,"allowed_artifact_paths":["artifacts"],"max_artifact_count":10,"max_artifact_bytes":1048576,"result_mode":"mixed"}`
	if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(skill.manifest.json) error = %v", err)
	}
	return root
}

func writePythonScenarioSkill(t *testing.T, script string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "run.py"), []byte(strings.TrimSpace(script)+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(run.py) error = %v", err)
	}
	return root
}

func writeNodeScenarioSkill(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	script := `
const fs = require("fs");
fs.mkdirSync("artifacts", { recursive: true });
fs.writeFileSync("artifacts/node-result.json", JSON.stringify({ runtime: "nodejs", ok: true }));
console.log(JSON.stringify({ success: true, runtime: "nodejs" }));
`
	if err := os.WriteFile(filepath.Join(root, "scripts", "run.js"), []byte(strings.TrimSpace(script)+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(run.js) error = %v", err)
	}
	content := `{"entrypoint":"scripts/run.js","language":"nodejs","timeout_ms":30000,"allowed_artifact_paths":["artifacts"],"max_artifact_count":10,"max_artifact_bytes":1048576,"result_mode":"mixed"}`
	if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(skill.manifest.json) error = %v", err)
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

func messagesContainLog(messages []tools.ToolInvokeMessage, text string) bool {
	for _, message := range messages {
		if message.Type == tools.ToolInvokeMessageTypeLog && strings.Contains(message.Text, text) {
			return true
		}
	}
	return false
}

func firstArtifactItems(t *testing.T, messages []tools.ToolInvokeMessage) []map[string]interface{} {
	t.Helper()
	for _, message := range messages {
		if _, ok := message.Data["artifacts"]; ok {
			return artifactItems(t, message)
		}
	}
	t.Fatalf("messages = %#v, want artifact summary", messages)
	return nil
}

func runRealSandboxSkill(t *testing.T, endpoint string, apiKey string, root string, persister SkillScriptArtifactPersister) *ToolInvocationResult {
	t.Helper()
	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{
		Endpoint:          endpoint,
		APIKey:            apiKey,
		ArtifactPersister: persister,
	})
	result, err := runner.RunSkillScript(context.Background(), scriptSkillDocument(root), map[string]interface{}{"input": "hello"}, ExecutionContext{
		OrganizationID: "org-1",
		UserID:         "user-1",
		ConversationID: "conversation-1",
		MessageID:      "message-1",
	}, "call_real")
	if err != nil {
		t.Fatalf("RunSkillScript() error = %v", err)
	}
	return result
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
	uploadArchiveRequests    int
	uploadValidateManifest   bool
	lastCommand              map[string]interface{}
	lastSkill                map[string]interface{}
	lastSandboxCreate        map[string]interface{}
	commandRequests          int
	skillRequests            int
	dependencyPrepareStatus  string
	dependencyBuildStatus    string
	dependencyBuildProfile   string
	treeRequests             int
	deleted                  int
}

type fakeSkillArtifactPersister struct {
	fail      bool
	persisted int
	requests  []SkillScriptArtifactPersistRequest
}

type fakeSkillInputFileProvider struct {
	files map[string]SkillScriptInputFile
	err   error
}

func (f *fakeSkillInputFileProvider) GetSkillScriptInputFile(_ context.Context, fileID string, maxBytes int64, _ ExecutionContext) (SkillScriptInputFile, error) {
	if f.err != nil {
		return SkillScriptInputFile{}, f.err
	}
	if f.files == nil {
		return SkillScriptInputFile{}, os.ErrNotExist
	}
	file, ok := f.files[fileID]
	if !ok {
		return SkillScriptInputFile{}, os.ErrNotExist
	}
	if file.FileID == "" {
		file.FileID = fileID
	}
	if maxBytes > 0 && file.Size > maxBytes {
		return SkillScriptInputFile{}, fmt.Errorf("file exceeds max_bytes %d", maxBytes)
	}
	return file, nil
}

func (f *fakeSkillArtifactPersister) PersistSkillScriptArtifact(_ context.Context, request SkillScriptArtifactPersistRequest) (map[string]interface{}, error) {
	if f.fail {
		return nil, io.ErrUnexpectedEOF
	}
	f.persisted++
	f.requests = append(f.requests, request)
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
		"mime_type":          request.ContentType,
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
	case r.Method == http.MethodGet && r.URL.Path == "/v1/sandbox/dependencies":
		writeSandboxDependencyCatalog(f.t, w)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/sandbox/dependencies/prepare":
		status := f.dependencyPrepareStatus
		if status == "" {
			status = "ready"
		}
		writeSandboxEnvelope(w, http.StatusOK, dependencyBuildResponse(status, "stdlib"))
	case r.Method == http.MethodPost && r.URL.Path == "/v1/sandbox/dependencies/builds":
		status := f.dependencyBuildStatus
		if status == "" {
			status = "ready"
		}
		profile := f.dependencyBuildProfile
		if profile == "" {
			profile = "auto-test"
		}
		writeSandboxEnvelope(w, http.StatusOK, dependencyBuildResponse(status, profile))
	case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
		f.lastSandboxCreate = readJSONBody(f.t, r)
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
	f.uploadArchiveRequests++
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

func dependencyBuildResponse(status string, profile string) map[string]interface{} {
	nextAction := "use_dependency_profile"
	if status == "queued" || status == "building" {
		nextAction = "wait_for_dependency_build"
	}
	if status == "build_required" {
		nextAction = "queue_dependency_build"
	}
	return map[string]interface{}{
		"build_id":      "depbuild_test",
		"fingerprint":   "sha256:test",
		"status":        status,
		"next_action":   nextAction,
		"profile_name":  profile,
		"package_count": 1,
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

func decodeJSON(t *testing.T, r *http.Request, out interface{}) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		t.Fatalf("decode request: %v", err)
	}
}

func writeSandboxOK(t *testing.T, w http.ResponseWriter, data interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "message": "success", "data": data}); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func writeSandboxDependencyCatalog(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	writeSandboxOK(t, w, map[string]interface{}{
		"language":             "python3",
		"mode":                 "managed-profiles",
		"supports_user_update": false,
		"profiles": []map[string]interface{}{
			{"name": "stdlib", "version": "2026.05.01", "status": "ready", "enabled": true, "languages": []string{"python3", "nodejs"}},
			{"name": "auto-office", "version": "2026.05.31", "status": "ready", "enabled": true, "languages": []string{"python3", "nodejs"}},
			{"name": "auto-test", "version": "2026.05.31", "status": "ready", "enabled": true, "languages": []string{"python3", "nodejs"}},
			{"name": "workflow-safe", "version": "2026.05.01", "status": "ready", "enabled": true, "languages": []string{"python3"}},
			{"name": "node-basic", "version": "2026.05.01", "status": "ready", "enabled": true, "languages": []string{"nodejs"}},
			{"name": "python-data-preview", "version": "2026.05.01", "status": "disabled", "enabled": false, "languages": []string{"python3"}},
		},
	})
}

func writeSandboxError(t *testing.T, w http.ResponseWriter, status int, code int, message string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"code": code, "message": message}); err != nil {
		t.Fatalf("write error response: %v", err)
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
