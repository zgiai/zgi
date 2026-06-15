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
	provider := NewProvider(fileService, extractor, &fakeWorkspacePermissionService{allowed: true})
	if provider.GetEntity().Identity.Name != ProviderID {
		t.Fatalf("provider name = %q, want %q", provider.GetEntity().Identity.Name, ProviderID)
	}
	if _, err := provider.GetTool(ToolListVisibleFiles); err != nil {
		t.Fatalf("list visible files tool not registered: %v", err)
	}
	if _, err := provider.GetTool(ToolDeleteFile); err != nil {
		t.Fatalf("delete tool not registered: %v", err)
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

func TestListVisibleFilesToolReturnsRuntimeContextFiles(t *testing.T) {
	provider := NewProvider(nil, nil, nil)
	tool := listVisibleFilesRuntimeTool(t, provider, "org-1")

	messages, err := tool.Invoke(context.Background(), "acct-1", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	payload := singleJSONPayload(t, messages)
	if got := payload["count"]; got != 2 {
		t.Fatalf("count = %#v, want 2", got)
	}
	if got := payload["selected_count"]; got != 1 {
		t.Fatalf("selected_count = %#v, want 1", got)
	}
	files, ok := payload["files"].([]interface{})
	if !ok || len(files) != 2 {
		t.Fatalf("files = %#v, want 2 files", payload["files"])
	}
	first, ok := files[0].(map[string]interface{})
	if !ok {
		t.Fatalf("first file type = %T, want map", files[0])
	}
	if first["file_id"] != "file-1" || first["name"] != "one.txt" || first["visible_index"] != 1 {
		t.Fatalf("first file = %#v, want ordered visible file metadata", first)
	}
	second, ok := files[1].(map[string]interface{})
	if !ok || second["selected"] != true || second["workspace_id"] != "workspace-1" {
		t.Fatalf("second file = %#v, want selected workspace file", files[1])
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
	provider := NewProvider(fileService, nil, &fakeWorkspacePermissionService{allowed: false})
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

func TestDeleteFileToolDeletesManageableWorkspaceFile(t *testing.T) {
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
				Name:           "obsolete.pdf",
				Size:           25,
				Extension:      "pdf",
				MimeType:       "application/pdf",
				CreatedBy:      accountID,
				CreatedAt:      time.Unix(1700000100, 0),
			},
		},
	}
	perms := &fakeWorkspacePermissionService{allowed: true}
	provider := NewProvider(fileService, nil, perms)
	tool := deleteFileRuntimeTool(t, provider, organizationID)

	messages, err := tool.Invoke(ctx, accountID, map[string]interface{}{"file_id": "file-1"}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(fileService.deleted) != 1 || fileService.deleted[0] != "file-1" {
		t.Fatalf("deleted = %#v, want file-1", fileService.deleted)
	}
	if len(perms.codes) != 1 || perms.codes[0] != workspacemodel.WorkspacePermissionFileManage {
		t.Fatalf("permission codes = %#v, want file.manage", perms.codes)
	}
	payload := singleJSONPayload(t, messages)
	if got := payload["status"]; got != "completed" {
		t.Fatalf("status = %#v, want completed", got)
	}
	if got := payload["deleted_count"]; got != 1 {
		t.Fatalf("deleted_count = %#v, want 1", got)
	}
	if got := payload["reversible"]; got != false {
		t.Fatalf("reversible = %#v, want false", got)
	}
	file, ok := payload["file"].(map[string]interface{})
	if !ok || file["id"] != "file-1" || file["name"] != "obsolete.pdf" {
		t.Fatalf("file payload = %#v, want deleted file metadata", payload["file"])
	}
}

func TestDeleteFileToolRejectsWorkspacePermissionDenied(t *testing.T) {
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	workspaceID := uuid.NewString()
	fileService := &fakeFileService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: organizationID,
				WorkspaceID:    &workspaceID,
				Name:           "private.pdf",
				CreatedBy:      accountID,
			},
		},
	}
	provider := NewProvider(fileService, nil, &fakeWorkspacePermissionService{allowed: false})
	tool := deleteFileRuntimeTool(t, provider, organizationID)

	_, err := tool.Invoke(context.Background(), accountID, map[string]interface{}{"file_id": "file-1"}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "file is not accessible") {
		t.Fatalf("Invoke() error = %v, want inaccessible file", err)
	}
	if len(fileService.deleted) != 0 {
		t.Fatalf("deleted = %#v, want no deletion", fileService.deleted)
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

func listVisibleFilesRuntimeTool(t *testing.T, provider *Provider, organizationID string) tools.Tool {
	t.Helper()
	tool, err := provider.GetTool(ToolListVisibleFiles)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	return tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   organizationID,
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": organizationID,
			"console_files_visible_files": []map[string]interface{}{
				{
					"visible_index": 1,
					"file_id":       "file-1",
					"name":          "one.txt",
					"extension":     "txt",
					"mime_type":     "text/plain",
				},
				{
					"visible_index": 2,
					"file_id":       "file-2",
					"name":          "two.pdf",
					"extension":     "pdf",
					"mime_type":     "application/pdf",
					"workspace_id":  "workspace-1",
					"selected":      true,
				},
			},
		},
	})
}

func deleteFileRuntimeTool(t *testing.T, provider *Provider, organizationID string) tools.Tool {
	t.Helper()
	tool, err := provider.GetTool(ToolDeleteFile)
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
	deleted []string
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

func (s *fakeFileService) DeleteFiles(_ context.Context, fileIDs []string) error {
	for _, fileID := range fileIDs {
		if s.files[fileID] == nil {
			return errors.New("file not found")
		}
		s.deleted = append(s.deleted, fileID)
		delete(s.files, fileID)
	}
	return nil
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
	codes   []workspacemodel.WorkspacePermissionCode
}

func (s *fakeWorkspacePermissionService) CheckWorkspacePermission(_ context.Context, _, _, _ string, code workspacemodel.WorkspacePermissionCode) (bool, error) {
	s.codes = append(s.codes, code)
	return s.allowed, nil
}
