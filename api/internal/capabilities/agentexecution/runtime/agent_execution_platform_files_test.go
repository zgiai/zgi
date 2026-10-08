package executionruntime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeplatform"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func platformTestScope() runtimeplatform.Scope {
	return runtimeplatform.Scope{OrganizationID: "org-1", SubjectID: "user-1", AgentID: "agent-1", ConversationID: "conversation-1", RunID: "message-1", BindingID: "binding-1", BindingGeneration: 1, JobID: "job-1"}
}

func TestPlatformFilesReadOnlyLoadedSkillVersion(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: cli-example\ndescription: Example.\n---\nExample instructions."), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := skills.SkillPackageSHA256(root)
	if err != nil {
		t.Fatal(err)
	}
	gateway := &fakeAgentExecutionGateway{capabilities: agentExecutionCapabilities{NetworkMode: "disabled", FileTransfer: true, MaxUploadBytes: agentExecutionFileLimit}}
	loaded := true
	files := &agentPlatformFiles{gateway: gateway, skillAllowed: func(context.Context, string) (bool, error) { return loaded, nil }, skillResolver: func(context.Context, string) (agentSkillPackage, error) {
		return agentSkillPackage{ID: "cli-example", Root: root, ExpectedDigest: digest}, nil
	}}
	read := func(version string) error {
		args, _ := json.Marshal(map[string]string{"ref": "skill:cli-example:" + version + ":SKILL.md", "path": "inputs/skill.md"})
		_, err := files.Handle(t.Context(), platformTestScope(), runtimeplatform.Request{ID: "skill-read", Action: runtimeplatform.ReadFile, Arguments: args})
		return err
	}
	if err := read(digest); err != nil || !strings.Contains(string(gateway.uploads["inputs/skill.md"]), "Example instructions") {
		t.Fatalf("read=%v files=%v", err, gateway.uploads)
	}
	if err := read(strings.Repeat("a", 64)); !apperror.IsCode(err, catalog.CodeResourceConflict) {
		t.Fatalf("wrong version=%v", err)
	}
	loaded = false
	if err := read(digest); !apperror.IsCode(err, catalog.CodePermissionDenied) {
		t.Fatalf("unloaded Skill=%v", err)
	}
}

func TestPlatformFilesUseAdmittedInputAndExistingArtifactStore(t *testing.T) {
	gateway := &fakeAgentExecutionGateway{capabilities: agentExecutionCapabilities{NetworkMode: "disabled", FileTransfer: true, MaxUploadBytes: agentExecutionFileLimit, MaxDownloadBytes: agentExecutionFileLimit}, downloads: map[string][]byte{"result.txt": []byte("result")}}
	reader := &fakeRuntimeAttachmentReader{files: map[string][]byte{"input-1": []byte("input")}}
	persister := &fakeAgentExecutionArtifactPersister{}
	files := &agentPlatformFiles{gateway: gateway, persister: persister, reader: reader, attachments: []runtimeservice.RuntimeAttachmentRef{{ID: "input-1", Size: 5}}}
	response, err := files.Handle(t.Context(), platformTestScope(), runtimeplatform.Request{ID: "read", Action: runtimeplatform.ReadFile, Arguments: json.RawMessage(`{"ref":"input-1","path":"inputs/input.txt"}`)})
	if err != nil || response.Status != runtimeplatform.Completed || string(gateway.uploads["inputs/input.txt"]) != "input" {
		t.Fatalf("read=%+v err=%v", response, err)
	}
	response, err = files.Handle(t.Context(), platformTestScope(), runtimeplatform.Request{ID: "export", Action: runtimeplatform.ExportFile, Arguments: json.RawMessage(`{"path":"result.txt"}`)})
	if err != nil || response.Status != runtimeplatform.Completed || string(persister.data) != "result" || len(files.Artifacts()) != 1 {
		t.Fatalf("export=%+v err=%v artifacts=%d", response, err, len(files.Artifacts()))
	}
	artifact := files.Artifacts()[0]
	if artifact["conversation_id"] != "conversation-1" || artifact["message_id"] != "message-1" {
		t.Fatalf("artifact scope=%+v", artifact)
	}
	artifact["file_id"] = "changed"
	if files.Artifacts()[0]["file_id"] != "file-1" {
		t.Fatal("artifact ownership leaked")
	}
}

func TestPlatformFilesRejectArbitraryReferencesAndUnsafePaths(t *testing.T) {
	gateway := &fakeAgentExecutionGateway{capabilities: agentExecutionCapabilities{NetworkMode: "disabled", FileTransfer: true, MaxUploadBytes: agentExecutionFileLimit, MaxDownloadBytes: agentExecutionFileLimit}}
	reader := &fakeRuntimeAttachmentReader{files: map[string][]byte{"other-file": []byte("private")}}
	files := &agentPlatformFiles{gateway: gateway, reader: reader, attachments: []runtimeservice.RuntimeAttachmentRef{{ID: "input-1", Size: 5}}}
	if _, err := files.Handle(t.Context(), platformTestScope(), runtimeplatform.Request{ID: "read", Action: runtimeplatform.ReadFile, Arguments: json.RawMessage(`{"ref":"other-file","path":"inputs/input.txt"}`)}); !apperror.IsCode(err, catalog.CodePermissionDenied) || reader.calls != 0 {
		t.Fatalf("unadmitted reference: err=%v reads=%d", err, reader.calls)
	}
	for _, unsafe := range []string{"../escape", "/etc/passwd", "a/../b", "a\\b", ".", "a\nsecret"} {
		args, _ := json.Marshal(map[string]string{"ref": "input-1", "path": unsafe})
		if _, err := files.Handle(t.Context(), platformTestScope(), runtimeplatform.Request{ID: "read", Action: runtimeplatform.ReadFile, Arguments: args}); !apperror.IsCode(err, catalog.CodeRequestInvalid) {
			t.Fatalf("path %q accepted: %v", unsafe, err)
		}
	}
	if reader.calls != 0 || len(gateway.uploads) > 0 {
		t.Fatal("invalid request performed file I/O")
	}
}

func TestPlatformFilesRejectChangedInputSizeBeforeUpload(t *testing.T) {
	gateway := &fakeAgentExecutionGateway{capabilities: agentExecutionCapabilities{NetworkMode: "disabled", FileTransfer: true, MaxUploadBytes: agentExecutionFileLimit}}
	reader := &fakeRuntimeAttachmentReader{files: map[string][]byte{"input-1": []byte("changed")}}
	files := &agentPlatformFiles{gateway: gateway, reader: reader, attachments: []runtimeservice.RuntimeAttachmentRef{{ID: "input-1", Size: 5}}}
	_, err := files.Handle(t.Context(), platformTestScope(), runtimeplatform.Request{ID: "read", Action: runtimeplatform.ReadFile, Arguments: json.RawMessage(`{"ref":"input-1","path":"inputs/input.txt"}`)})
	if !apperror.IsCode(err, catalog.CodeRequestInvalid) || len(gateway.uploads) != 0 {
		t.Fatalf("changed input uploaded: err=%v uploads=%d", err, len(gateway.uploads))
	}
}
