package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/zgiai/zgi/api/internal/contracts/chatfile"
	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

type chatPolicyFiles struct {
	interfaces.FileService
	files map[string]*dto.UploadFile
}

func TestChatInputsWithFilesReachesSystemFilePool(t *testing.T) {
	inputs := map[string]interface{}{"query": "inspect", "#files#": "legacy", "sys.files": "untrusted"}
	files := []dto.FileInfo{{Type: "image", TransferMethod: "local_file", UploadFileID: "image-id"}}
	prepared := chatInputsWithFiles(inputs, files)
	got, err := ChatAttachmentReferences(nil, prepared)
	if err != nil || len(got) != 1 || got[0] != files[0] {
		t.Fatalf("prepared files = %#v, error = %v", got, err)
	}
	if inputs["#files#"] != "legacy" || prepared["query"] != "inspect" {
		t.Fatal("chat preparation changed caller inputs or dropped typed variables")
	}
	if _, exists := prepared["sys.files"]; exists {
		t.Fatal("client-provided system files bypassed chat admission")
	}
	legacy := chatInputsWithFiles(inputs, nil)
	if legacy["#files#"] != "legacy" {
		t.Fatal("legacy file inputs were changed")
	}
}

func (f chatPolicyFiles) GetFileByID(_ context.Context, id string) (*dto.UploadFile, error) {
	return f.files[id], nil
}

func TestWorkflowChatUploadPolicyPreservesLegacyAndCount(t *testing.T) {
	config := &interfaces.FileUploadConfigResponse{FileSizeLimit: 70, ImageFileSizeLimit: 10, WorkflowFileUploadLimit: 10}
	for _, tc := range []struct {
		name, extension   string
		types, extensions []string
		allowed           bool
	}{
		{"document", "pdf", []string{"document"}, nil, true},
		{"unsupported", "zip", []string{"document", "image"}, nil, false},
		{"legacy custom", "zip", []string{"custom"}, []string{".ZIP"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			features := map[string]any{"file_upload": map[string]any{"enabled": true, "allowed_file_types": tc.types, "allowed_file_extensions": tc.extensions, "number_limits": 8}}
			policy := WorkflowChatUploadPolicy(features, config)
			if policy.NumberLimit != 8 || (policy.ValidateFile(tc.extension, 70*chatfile.MiB) == nil) != tc.allowed {
				t.Fatalf("policy = %#v", policy)
			}
		})
	}
	if err := WorkflowChatUploadPolicy(nil, config).ValidateFile("pdf", 1); !errors.Is(err, chatfile.ErrUnsupportedType) {
		t.Fatalf("disabled policy error = %v", err)
	}
}

func TestValidateChatAttachmentsUsesUniqueAuthorizedOriginalSizes(t *testing.T) {
	service := chatPolicyFiles{files: map[string]*dto.UploadFile{
		"one": {ID: "one", Name: "one.pdf", Size: 50 * chatfile.MiB},
		"two": {ID: "two", Name: "two.pdf", Size: 50 * chatfile.MiB},
	}}
	policy := chatfile.Resolve(nil, 50, 10, 5)
	files := []dto.FileInfo{{UploadFileID: "one"}, {UploadFileID: "one"}, {UploadFileID: "two"}}
	calls := 0
	authorize := func(context.Context, *dto.UploadFile) error { calls++; return nil }
	if err := ValidateChatAttachments(t.Context(), files, service, policy, authorize); err != nil || calls != 2 {
		t.Fatalf("exact limit with duplicate: err=%v calls=%d", err, calls)
	}
	service.files["three"] = &dto.UploadFile{ID: "three", Name: "three.txt", Size: 1}
	files = append(files, dto.FileInfo{UploadFileID: "three"})
	if err := ValidateChatAttachments(t.Context(), files, service, policy, authorize); !errors.Is(err, chatfile.ErrTurnTooLarge) {
		t.Fatalf("over limit: %v", err)
	}
	denied := errors.New("access denied")
	if err := ValidateChatAttachments(t.Context(), files[:1], service, policy, func(context.Context, *dto.UploadFile) error { return denied }); !errors.Is(err, denied) {
		t.Fatalf("cross-user authorization: %v", err)
	}
	service.files["one"].Name = "one.zip"
	if err := ValidateChatAttachments(t.Context(), files[:1], service, policy, authorize); !errors.Is(err, chatfile.ErrUnsupportedType) {
		t.Fatalf("file library type bypass: %v", err)
	}
}
