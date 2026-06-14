package files

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestReadFileToolReturnsAccessibleContent(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	workspaceID := uuid.NewString()
	fileService := &fakeFileService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: organizationID,
				WorkspaceID:    &workspaceID,
				Name:           "notes.txt",
				Size:           11,
				Extension:      "txt",
				MimeType:       "text/plain",
				CreatedBy:      accountID,
				CreatedAt:      time.Unix(1700000000, 0),
			},
		},
	}
	extractor := &fakeContentExtractor{
		contents: map[string]*workflowfile.FileContent{
			"file-1": {
				FileID:    "file-1",
				Content:   "hello world",
				FromCache: true,
			},
		},
	}
	provider := NewProvider(fileService, extractor, fakeWorkspacePermissionService{allowed: true})
	if provider.GetEntity().Identity.Name != ProviderID {
		t.Fatalf("provider name = %q, want %q", provider.GetEntity().Identity.Name, ProviderID)
	}
	tool := readFileRuntimeTool(t, provider, organizationID)

	messages, err := tool.Invoke(ctx, accountID, map[string]interface{}{
		"file_id":   "file-1",
		"max_chars": 5,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	payload := singleJSONPayload(t, messages)
	if got := payload["content"]; got != "hello" {
		t.Fatalf("content = %#v, want hello", got)
	}
	if got := payload["content_chars"]; got != 11 {
		t.Fatalf("content_chars = %#v, want 11", got)
	}
	if got := payload["content_truncated"]; got != true {
		t.Fatalf("content_truncated = %#v, want true", got)
	}
	if got := payload["from_cache"]; got != true {
		t.Fatalf("from_cache = %#v, want true", got)
	}
	if got := payload["content_status"]; got != "extracted" {
		t.Fatalf("content_status = %#v, want extracted", got)
	}
	file, ok := payload["file"].(map[string]interface{})
	if !ok {
		t.Fatalf("file payload type = %T, want map", payload["file"])
	}
	if got := file["workspace_id"]; got != workspaceID {
		t.Fatalf("workspace_id = %#v, want %s", got, workspaceID)
	}
}

func TestReadFileToolRejectsWorkspacePermissionDenied(t *testing.T) {
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	workspaceID := uuid.NewString()
	fileService := &fakeFileService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: organizationID,
				WorkspaceID:    &workspaceID,
				Name:           "private.txt",
				CreatedBy:      accountID,
			},
		},
	}
	provider := NewProvider(fileService, nil, fakeWorkspacePermissionService{allowed: false})
	tool := readFileRuntimeTool(t, provider, organizationID)

	_, err := tool.Invoke(context.Background(), accountID, map[string]interface{}{"file_id": "file-1"}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "file is not accessible") {
		t.Fatalf("Invoke() error = %v, want inaccessible file", err)
	}
}

func TestReadFileToolRejectsUnownedTemporaryFile(t *testing.T) {
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	fileService := &fakeFileService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:          "file-1",
				IsTemporary: true,
				Name:        "upload.txt",
				CreatedBy:   uuid.NewString(),
			},
		},
	}
	provider := NewProvider(fileService, nil, nil)
	tool := readFileRuntimeTool(t, provider, organizationID)

	_, err := tool.Invoke(context.Background(), accountID, map[string]interface{}{"file_id": "file-1"}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "file is not accessible") {
		t.Fatalf("Invoke() error = %v, want inaccessible file", err)
	}
}

func TestReadFileToolRejectsUnownedOrganizationFileWithoutWorkspace(t *testing.T) {
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	fileService := &fakeFileService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: organizationID,
				Name:           "shared.txt",
				CreatedBy:      uuid.NewString(),
			},
		},
	}
	provider := NewProvider(fileService, nil, nil)
	tool := readFileRuntimeTool(t, provider, organizationID)

	_, err := tool.Invoke(context.Background(), accountID, map[string]interface{}{"file_id": "file-1"}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "file is not accessible") {
		t.Fatalf("Invoke() error = %v, want inaccessible file", err)
	}
}

func TestReadFileToolFallsBackToFileServiceWhenExtractorMissing(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	fileService := &fakeFileService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: organizationID,
				Name:           "plain.txt",
				CreatedBy:      accountID,
			},
		},
		content: map[string]string{
			"file-1": "fallback content",
		},
	}
	provider := NewProvider(fileService, nil, nil)
	tool := readFileRuntimeTool(t, provider, organizationID)

	messages, err := tool.Invoke(ctx, accountID, map[string]interface{}{"file_id": "file-1"}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	payload := singleJSONPayload(t, messages)
	if got := payload["content"]; got != "fallback content" {
		t.Fatalf("content = %#v, want fallback content", got)
	}
	if got := payload["from_cache"]; got != false {
		t.Fatalf("from_cache = %#v, want false", got)
	}
}

func readFileRuntimeTool(t *testing.T, provider *Provider, organizationID string) tools.Tool {
	t.Helper()
	tool, err := provider.GetTool(ToolReadFile)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	return tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   organizationID,
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": organizationID,
		},
	})
}

func singleJSONPayload(t *testing.T, messages []tools.ToolInvokeMessage) map[string]interface{} {
	t.Helper()
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	if messages[0].Type != tools.ToolInvokeMessageTypeJSON {
		t.Fatalf("message type = %q, want json", messages[0].Type)
	}
	return messages[0].Data
}

type fakeFileService struct {
	files   map[string]*dto.UploadFile
	content map[string]string
}

func (s *fakeFileService) GetFileByID(_ context.Context, fileID string) (*dto.UploadFile, error) {
	file := s.files[fileID]
	if file == nil {
		return nil, errors.New("file not found")
	}
	return file, nil
}

func (s *fakeFileService) GetFile(_ context.Context, fileID string) (string, error) {
	content, ok := s.content[fileID]
	if !ok {
		return "", errors.New("file content not found")
	}
	return content, nil
}

type fakeContentExtractor struct {
	contents map[string]*workflowfile.FileContent
}

func (e *fakeContentExtractor) ExtractMultipleFiles(_ context.Context, fileIDs []string, _ string) ([]*workflowfile.FileContent, error) {
	out := make([]*workflowfile.FileContent, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		out = append(out, e.contents[fileID])
	}
	return out, nil
}

type fakeWorkspacePermissionService struct {
	allowed bool
}

func (s fakeWorkspacePermissionService) CheckWorkspacePermission(context.Context, string, string, string, workspacemodel.WorkspacePermissionCode) (bool, error) {
	return s.allowed, nil
}
