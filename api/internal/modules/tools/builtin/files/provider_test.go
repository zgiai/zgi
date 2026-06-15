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
	if instruction := stringValue(payload, "instruction"); !strings.Contains(instruction, "Use the returned content field") ||
		!strings.Contains(instruction, "truncated") {
		t.Fatalf("instruction = %q, want extracted truncated guidance", instruction)
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

func TestConsoleFilesReadFileSupportsDocumentFormats(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	cases := []struct {
		id        string
		name      string
		extension string
		mimeType  string
		content   string
	}{
		{
			id:        "file-md",
			name:      "notes.md",
			extension: "md",
			mimeType:  "text/markdown",
			content:   "# Notes\n\nShip the console files reader.",
		},
		{
			id:        "file-xlsx",
			name:      "budget.xlsx",
			extension: "xlsx",
			mimeType:  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			content:   "| Item | Amount |\n| --- | ---: |\n| Hosting | 42 |",
		},
		{
			id:        "file-pdf",
			name:      "brief.pdf",
			extension: "pdf",
			mimeType:  "application/pdf",
			content:   "PDF executive brief content",
		},
	}
	fileService := &fakeFileService{files: map[string]*dto.UploadFile{}}
	extractor := &fakeContentExtractor{contents: map[string]*workflowfile.FileContent{}}
	visibleFiles := make([]map[string]interface{}, 0, len(cases))
	for idx, tc := range cases {
		fileService.files[tc.id] = &dto.UploadFile{
			ID:             tc.id,
			OrganizationID: organizationID,
			Name:           tc.name,
			Extension:      tc.extension,
			MimeType:       tc.mimeType,
			CreatedBy:      accountID,
			CreatedAt:      time.Unix(1700000200+int64(idx), 0),
		}
		extractor.contents[tc.id] = &workflowfile.FileContent{
			FileID:      tc.id,
			Content:     tc.content,
			ContentType: tc.mimeType,
		}
		visibleFiles = append(visibleFiles, map[string]interface{}{
			"visible_index": idx + 1,
			"file_id":       tc.id,
			"name":          tc.name,
			"extension":     tc.extension,
			"mime_type":     tc.mimeType,
			"selected":      idx == 1,
		})
	}
	runtime := &tools.ToolRuntime{
		TenantID:   organizationID,
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id":             organizationID,
			"console_files_visible_files": visibleFiles,
		},
	}
	provider := NewProvider(fileService, extractor, nil)

	listTool, err := provider.GetTool(ToolListVisibleFiles)
	if err != nil {
		t.Fatalf("GetTool(list) error = %v", err)
	}
	listMessages, err := listTool.ForkToolRuntime(runtime).Invoke(ctx, accountID, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke(list) error = %v", err)
	}
	listPayload := singleJSONPayload(t, listMessages)
	if got := listPayload["count"]; got != len(cases) {
		t.Fatalf("list count = %#v, want %d", got, len(cases))
	}
	listedFiles, ok := listPayload["files"].([]interface{})
	if !ok || len(listedFiles) != len(cases) {
		t.Fatalf("listed files = %#v, want %d files", listPayload["files"], len(cases))
	}
	for idx, tc := range cases {
		listed, ok := listedFiles[idx].(map[string]interface{})
		if !ok {
			t.Fatalf("listed file %d type = %T, want map", idx, listedFiles[idx])
		}
		if listed["file_id"] != tc.id || listed["extension"] != tc.extension || listed["mime_type"] != tc.mimeType {
			t.Fatalf("listed file %d = %#v, want %s %s %s", idx, listed, tc.id, tc.extension, tc.mimeType)
		}
	}

	readTool, err := provider.GetTool(ToolReadFile)
	if err != nil {
		t.Fatalf("GetTool(read) error = %v", err)
	}
	readTool = readTool.ForkToolRuntime(runtime)
	for idx, tc := range cases {
		messages, err := readTool.Invoke(ctx, accountID, map[string]interface{}{"file_id": tc.id}, nil, nil, nil)
		if err != nil {
			t.Fatalf("Invoke(read %s) error = %v", tc.id, err)
		}
		payload := singleJSONPayload(t, messages)
		if got := payload["content_status"]; got != "extracted" {
			t.Fatalf("read %s content_status = %#v, want extracted", tc.id, got)
		}
		if got := payload["content"]; got != tc.content {
			t.Fatalf("read %s content = %#v, want %#v", tc.id, got, tc.content)
		}
		file, ok := payload["file"].(map[string]interface{})
		if !ok {
			t.Fatalf("read %s file payload type = %T, want map", tc.id, payload["file"])
		}
		if file["extension"] != tc.extension || file["mime_type"] != tc.mimeType {
			t.Fatalf("read %s file payload = %#v, want extension %s and mime %s", tc.id, file, tc.extension, tc.mimeType)
		}
		if len(extractor.requestedIDs) <= idx || len(extractor.tenantIDs) <= idx {
			t.Fatalf("extractor calls = %d ids and %d tenants, want at least %d", len(extractor.requestedIDs), len(extractor.tenantIDs), idx+1)
		}
		if gotIDs := extractor.requestedIDs[idx]; len(gotIDs) != 1 || gotIDs[0] != tc.id {
			t.Fatalf("extractor request %d = %#v, want [%s]", idx, gotIDs, tc.id)
		}
		if gotTenant := extractor.tenantIDs[idx]; gotTenant != organizationID {
			t.Fatalf("extractor tenant %d = %q, want %q", idx, gotTenant, organizationID)
		}
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

func TestReadFileToolInstructionReflectsContentStatus(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	fileService := &fakeFileService{
		files: map[string]*dto.UploadFile{
			"empty-file": {
				ID:             "empty-file",
				OrganizationID: organizationID,
				Name:           "empty.xlsx",
				Extension:      "xlsx",
				MimeType:       "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
				CreatedBy:      accountID,
			},
			"error-file": {
				ID:             "error-file",
				OrganizationID: organizationID,
				Name:           "broken.pdf",
				Extension:      "pdf",
				MimeType:       "application/pdf",
				CreatedBy:      accountID,
			},
		},
	}
	extractor := &fakeContentExtractor{
		contents: map[string]*workflowfile.FileContent{
			"empty-file": {FileID: "empty-file", Content: ""},
			"error-file": {FileID: "error-file", Error: errors.New("parser unavailable")},
		},
	}
	provider := NewProvider(fileService, extractor, nil)
	tool := readFileRuntimeTool(t, provider, organizationID)

	emptyMessages, err := tool.Invoke(ctx, accountID, map[string]interface{}{"file_id": "empty-file"}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke(empty-file) error = %v", err)
	}
	emptyPayload := singleJSONPayload(t, emptyMessages)
	if got := emptyPayload["content_status"]; got != "empty" {
		t.Fatalf("empty content_status = %#v, want empty", got)
	}
	if instruction := stringValue(emptyPayload, "instruction"); !strings.Contains(instruction, "no extractable text content") {
		t.Fatalf("empty instruction = %q, want empty-content guidance", instruction)
	}

	errorMessages, err := tool.Invoke(ctx, accountID, map[string]interface{}{"file_id": "error-file"}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke(error-file) error = %v", err)
	}
	errorPayload := singleJSONPayload(t, errorMessages)
	if got := errorPayload["content_status"]; got != "error" {
		t.Fatalf("error content_status = %#v, want error", got)
	}
	if got := errorPayload["content_error"]; got != "parser unavailable" {
		t.Fatalf("content_error = %#v, want parser unavailable", got)
	}
	if instruction := stringValue(errorPayload, "instruction"); !strings.Contains(instruction, "could not be read") ||
		!strings.Contains(instruction, "do not claim") {
		t.Fatalf("error instruction = %q, want error guidance", instruction)
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
					"type":        "page",
					"resource_id": "console.files",
					"title":       "console.files",
				},
				{
					"resource_type": "selection",
					"resource_id":   "selected-files",
					"title":         "Current selection",
				},
				{
					"kind":        "log",
					"resource_id": "trace-1",
					"title":       "Trace context",
				},
				{
					"resource_id": "console.files",
					"title":       "console.files",
					"metadata": map[string]interface{}{
						"resource_kind": "page",
					},
				},
				{
					"visible_index": 1,
					"file_id":       "file-1",
					"name":          "one.txt",
					"extension":     "txt",
					"mime_type":     "text/plain",
				},
				{
					"visible_index": 2,
					"resource_type": "file",
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
	contents     map[string]*workflowfile.FileContent
	requestedIDs [][]string
	tenantIDs    []string
}

func (e *fakeContentExtractor) ExtractMultipleFiles(_ context.Context, fileIDs []string, tenantID string) ([]*workflowfile.FileContent, error) {
	e.requestedIDs = append(e.requestedIDs, append([]string(nil), fileIDs...))
	e.tenantIDs = append(e.tenantIDs, tenantID)
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
