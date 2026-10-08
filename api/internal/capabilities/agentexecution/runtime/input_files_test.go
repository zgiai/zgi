package executionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeplatform"
	"github.com/zgiai/zgi/api/internal/dto"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type inputTestSource struct {
	file  *dto.UploadFile
	data  []byte
	reads int
}

func (s *inputTestSource) GetFileByID(context.Context, string) (*dto.UploadFile, error) {
	return s.file, nil
}
func (s *inputTestSource) DownloadFile(context.Context, string) ([]byte, error) {
	s.reads++
	return s.data, nil
}
func allowInputTestFile(context.Context, *dto.UploadFile) error { return nil }

func TestScopedInputsFreezeReferencesAndCheckBeforeBytes(t *testing.T) {
	ref := runtimeservice.RuntimeAttachmentRef{ID: uuid.NewString(), Name: "input.csv", Size: 5}
	source := &inputTestSource{file: &dto.UploadFile{ID: ref.ID, Name: ref.Name, Size: ref.Size}, data: []byte("input")}
	refs := []runtimeservice.RuntimeAttachmentRef{ref}
	scope := execution.Scope{OrganizationID: uuid.NewString(), SubjectID: uuid.NewString()}
	revoked := false
	reader, err := NewScopedAttachmentReader(source, scope, refs, func(context.Context, *dto.UploadFile) error {
		if revoked {
			return errors.New("revoked")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	refs[0].ID = uuid.NewString()
	if data, err := reader.DownloadFile(t.Context(), ref.ID); err != nil || string(data) != "input" {
		t.Fatalf("read=%q err=%v", data, err)
	}
	for _, scenario := range []string{"arbitrary", "revoked", "metadata", "deleted"} {
		t.Run(scenario, func(t *testing.T) {
			source.file = &dto.UploadFile{ID: ref.ID, Name: ref.Name, Size: ref.Size}
			revoked = false
			id := ref.ID
			switch scenario {
			case "arbitrary":
				id = uuid.NewString()
			case "revoked":
				revoked = true
			case "metadata":
				source.file.Size++
			case "deleted":
				source.file = nil
			}
			before := source.reads
			if _, err := reader.DownloadFile(t.Context(), id); err == nil || source.reads != before {
				t.Fatalf("denied input read bytes: err=%v reads=%d before=%d", err, source.reads, before)
			}
		})
	}
}

func TestScopedInputsRejectInvalidReferences(t *testing.T) {
	ref := runtimeservice.RuntimeAttachmentRef{ID: uuid.NewString(), Name: "input", Size: 5}
	scope := execution.Scope{OrganizationID: uuid.NewString(), SubjectID: uuid.NewString()}
	for _, refs := range [][]runtimeservice.RuntimeAttachmentRef{
		{ref, ref}, {{ID: "invalid"}}, {{ID: uuid.NewString(), Size: -1}},
		{{ID: uuid.NewString(), Size: agentExecutionFileLimit + 1}},
		{{ID: uuid.NewString(), Size: agentExecutionFileLimit}, ref},
	} {
		if _, err := NewScopedAttachmentReader(&inputTestSource{}, scope, refs, allowInputTestFile); err == nil {
			t.Fatalf("accepted refs=%+v", refs)
		}
	}
	if _, err := NewScopedAttachmentReader(&inputTestSource{}, scope, nil, nil); err == nil {
		t.Fatal("missing live authorizer accepted")
	}
}

func TestScopedInputsProtectShellAndPlatformReads(t *testing.T) {
	ref := runtimeservice.RuntimeAttachmentRef{ID: uuid.NewString(), Name: "input.csv", Size: 5}
	source := &inputTestSource{file: &dto.UploadFile{ID: ref.ID, Name: ref.Name, Size: ref.Size}, data: []byte("input")}
	scope := execution.Scope{OrganizationID: "org-1", SubjectID: "user-1"}
	revoked := true
	reader, err := NewScopedAttachmentReader(source, scope, []runtimeservice.RuntimeAttachmentRef{ref}, func(context.Context, *dto.UploadFile) error {
		if revoked {
			return errors.New("revoked")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	gateway := &fakeAgentExecutionGateway{binding: agentExecutionBinding{ID: "binding", Status: "active"},
		job: agentExecutionJob{ID: "job", Status: "succeeded"}, capabilities: agentExecutionCapabilities{Shell: true, NetworkMode: "disabled", FileTransfer: true, MaxUploadBytes: agentExecutionFileLimit}}
	shell := newAgentExecutionShellTool(gateway, scope, "agent", "conversation", "message", "profile", "fake", []runtimeservice.RuntimeAttachmentRef{ref}, reader)
	result := shell.Handler(t.Context(), adapter.ToolCall{Function: adapter.FunctionCall{Arguments: `{"command":"cat inputs/*"}`}})
	if result.Error == nil || source.reads != 0 || len(gateway.uploads) != 0 || gateway.command != "" {
		t.Fatalf("revoked input reached command: result=%+v reads=%d command=%q", result, source.reads, gateway.command)
	}
	files := &agentPlatformFiles{gateway: gateway, reader: reader, attachments: []runtimeservice.RuntimeAttachmentRef{ref}}
	args, _ := json.Marshal(map[string]string{"ref": ref.ID, "path": "inputs/input.csv"})
	request := runtimeplatform.Request{ID: "read", Action: runtimeplatform.ReadFile, Arguments: args}
	if _, err := files.Handle(t.Context(), platformTestScope(), request); err == nil || source.reads != 0 {
		t.Fatalf("revoked CLI input: err=%v reads=%d", err, source.reads)
	}
	revoked = false
	if _, err := files.Handle(t.Context(), platformTestScope(), request); err != nil || source.reads != 1 {
		t.Fatalf("authorized CLI input: err=%v reads=%d", err, source.reads)
	}
	revoked = true
	if err := reader.CheckAttachments(t.Context()); err == nil {
		t.Fatal("cached input bypassed revocation")
	}
}
