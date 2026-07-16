package files

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	filemodel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
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
		!strings.Contains(instruction, "truncated") ||
		!strings.Contains(instruction, "submit_turn_state") {
		t.Fatalf("instruction = %q, want extracted truncated guidance", instruction)
	}
	if got := payload["content_lifetime"]; got != "current_tool_result_only" {
		t.Fatalf("content_lifetime = %#v, want current_tool_result_only", got)
	}
	if got := payload["content_redacted_in_history"]; got != true {
		t.Fatalf("content_redacted_in_history = %#v, want true", got)
	}
	if got := payload["handoff_recommended"]; got != true {
		t.Fatalf("handoff_recommended = %#v, want true", got)
	}
	if got := payload["recommended_next_tool"]; got != "submit_turn_state" {
		t.Fatalf("recommended_next_tool = %#v, want submit_turn_state", got)
	}
	if handoffInstruction := stringValue(payload, "handoff_instruction"); !strings.Contains(handoffInstruction, "source=file-reader/read_file") {
		t.Fatalf("handoff_instruction = %q, want file-reader/read_file guidance", handoffInstruction)
	}
	if when, ok := payload["handoff_required_when"].([]string); !ok || len(when) == 0 {
		t.Fatalf("handoff_required_when = %#v, want non-empty string slice", payload["handoff_required_when"])
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
	workspaceID := "workspace-1"
	listService := &fakeFileListService{files: []*filemodel.UploadFile{
		{ID: "file-1", OrganizationID: "org-1", Name: "one.txt", Extension: "txt", CreatedAt: time.Unix(1700000000, 0)},
		{ID: "file-2", OrganizationID: "org-1", WorkspaceID: &workspaceID, Name: "two.pdf", Extension: "pdf", CreatedAt: time.Unix(1700000001, 0)},
	}}
	provider := NewProvider(nil, nil, &fakeWorkspacePermissionService{allowed: true}, WithFileListService(listService))
	tool := listVisibleFilesRuntimeTool(t, provider, "org-1")

	messages, err := tool.Invoke(context.Background(), "acct-1", map[string]interface{}{
		"workspace_id": workspaceID,
		"selected_ids": "file-2",
	}, nil, nil, nil)
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

func TestListVisibleFilesToolNormalizesNeedsActionProcessingStatus(t *testing.T) {
	listService := &fakeFileListService{}
	provider := NewProvider(nil, nil, &fakeWorkspacePermissionService{allowed: true}, WithFileListService(listService))
	tool := listVisibleFilesRuntimeTool(t, provider, "org-1")

	if _, err := tool.Invoke(context.Background(), "acct-1", map[string]interface{}{
		"category": "needs_action",
	}, nil, nil, nil); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if got := listService.last["processing_status"]; got != "parse_failed" {
		t.Fatalf("processing_status = %#v, want parse_failed", got)
	}
}

func TestConsoleFilesReadFileSupportsDocumentFormats(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	workspaceID := uuid.NewString()
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
			WorkspaceID:    &workspaceID,
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
			"workspace_id":                workspaceID,
			"console_files_visible_files": visibleFiles,
		},
	}
	listFiles := make([]*filemodel.UploadFile, 0, len(cases))
	for _, tc := range cases {
		listFiles = append(listFiles, &filemodel.UploadFile{ID: tc.id, OrganizationID: organizationID, Name: tc.name, Extension: tc.extension, MimeType: tc.mimeType, CreatedBy: accountID, CreatedAt: time.Unix(1700000200, 0)})
	}
	provider := NewProvider(fileService, extractor, &fakeWorkspacePermissionService{allowed: true}, WithFileListService(&fakeFileListService{files: listFiles}))

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
		if len(extractor.requestedIDs) <= idx || len(extractor.scopes) <= idx {
			t.Fatalf("extractor calls = %d ids and %d scopes, want at least %d", len(extractor.requestedIDs), len(extractor.scopes), idx+1)
		}
		if gotIDs := extractor.requestedIDs[idx]; len(gotIDs) != 1 || gotIDs[0] != tc.id {
			t.Fatalf("extractor request %d = %#v, want [%s]", idx, gotIDs, tc.id)
		}
		if gotScope := extractor.scopes[idx]; gotScope.OrganizationID != organizationID || gotScope.WorkspaceID != workspaceID {
			t.Fatalf("extractor scope %d = %#v, want organization %q and workspace %q", idx, gotScope, organizationID, workspaceID)
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
	conversationID := uuid.NewString()
	tool := deleteFileRuntimeToolWithGrant(t, provider, organizationID, accountID, conversationID, "file-1", workspaceID)

	messages, err := tool.Invoke(ctx, accountID, map[string]interface{}{"file_id": "file-1"}, &conversationID, nil, nil)
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

func TestDeleteFileToolRejectsWithoutGovernanceGrant(t *testing.T) {
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	workspaceID := uuid.NewString()
	conversationID := uuid.NewString()
	fileService := &fakeFileService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: organizationID,
				WorkspaceID:    &workspaceID,
				Name:           "obsolete.pdf",
				CreatedBy:      accountID,
			},
		},
	}
	provider := NewProvider(fileService, nil, &fakeWorkspacePermissionService{allowed: true})
	tool := deleteFileRuntimeToolFrom(t, provider, organizationID, tools.ToolInvokeFromWorkflow, map[string]interface{}{
		"organization_id": organizationID,
		"workspace_id":    workspaceID,
	})

	_, err := tool.Invoke(context.Background(), accountID, map[string]interface{}{"file_id": "file-1"}, &conversationID, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "tool governance approval") {
		t.Fatalf("Invoke() error = %v, want governance approval requirement", err)
	}
	if len(fileService.deleted) != 0 {
		t.Fatalf("deleted = %#v, want no deletion", fileService.deleted)
	}
}

func TestDeleteFileToolRejectsAssetlessGovernanceGrant(t *testing.T) {
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	workspaceID := uuid.NewString()
	conversationID := uuid.NewString()
	fileService := &fakeFileService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: organizationID,
				WorkspaceID:    &workspaceID,
				Name:           "obsolete.pdf",
				CreatedBy:      accountID,
			},
		},
	}
	provider := NewProvider(fileService, nil, &fakeWorkspacePermissionService{allowed: true})
	tool := deleteFileRuntimeToolWithAssetlessGrant(t, provider, organizationID, accountID, conversationID, workspaceID)

	_, err := tool.Invoke(context.Background(), accountID, map[string]interface{}{"file_id": "file-1"}, &conversationID, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "tool governance approval") {
		t.Fatalf("Invoke() error = %v, want governance approval requirement", err)
	}
	if len(fileService.deleted) != 0 {
		t.Fatalf("deleted = %#v, want no deletion", fileService.deleted)
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

func TestSaveFileToolSavesGeneratedToolFileWithAdvancedPermission(t *testing.T) {
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	workspaceID := uuid.NewString()
	conversationID := uuid.NewString()
	expiresAt := time.Now().Add(time.Hour)
	fileService := &fakeFileService{
		files: map[string]*dto.UploadFile{},
	}
	toolFiles := &fakeToolFileStore{
		files: map[string]*tool_file.ToolFile{
			"tool-1": {
				ID:             "tool-1",
				UserID:         accountID,
				TenantID:       organizationID,
				ConversationID: &conversationID,
				Name:           "draft.md",
				MimeType:       "text/markdown",
				Size:           13,
				Lifecycle:      string(tool_file.ToolFileLifecycleTemporary),
				ExpiresAt:      &expiresAt,
			},
		},
		data: map[string][]byte{
			"tool-1": []byte("# Draft\nHello"),
		},
	}
	perms := &fakeWorkspacePermissionService{allowed: true}
	provider := NewProvider(fileService, nil, perms, WithToolFileStore(toolFiles))
	tool := saveFileRuntimeToolFrom(t, provider, organizationID, tools.ToolInvokeFromAIChat, map[string]interface{}{
		"organization_id":                 organizationID,
		"workspace_id":                    workspaceID,
		"tool_governance_permission_tier": "advanced",
	})

	messages, err := tool.Invoke(context.Background(), accountID, map[string]interface{}{
		"source_type":  "tool_file",
		"tool_file_id": "tool-1",
		"filename":     "saved-draft.md",
	}, &conversationID, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(fileService.uploads) != 1 {
		t.Fatalf("uploads = %#v, want 1 upload", fileService.uploads)
	}
	upload := fileService.uploads[0]
	if upload.filename != "saved-draft.md" || string(upload.content) != "# Draft\nHello" || upload.workspaceID != workspaceID {
		t.Fatalf("upload = %#v, want saved markdown into workspace", upload)
	}
	if len(perms.codes) != 1 || perms.codes[0] != workspacemodel.WorkspacePermissionFileUploadCreate {
		t.Fatalf("permission codes = %#v, want file upload create", perms.codes)
	}
	if len(messages) != 1 || messages[0].Type != tools.ToolInvokeMessageTypeJSON {
		t.Fatalf("messages = %#v, want json message only", messages)
	}
	payload := messages[0].Data
	if payload["target"] != "managed_file" || payload["transfer_method"] != "local_file" || payload["upload_file_id"] == "" {
		t.Fatalf("payload = %#v, want managed local file metadata", payload)
	}
	if payload["operation"] != "copy" || payload["source_retained"] != true || payload["source_tool_file_id"] != "tool-1" {
		t.Fatalf("payload = %#v, want retained tool-file copy metadata", payload)
	}
	if payload["source_expires_at"] != expiresAt.Unix() {
		t.Fatalf("source_expires_at = %#v, want %d", payload["source_expires_at"], expiresAt.Unix())
	}
	if handoff := stringValue(payload, "handoff_instruction"); !strings.Contains(handoff, "managed-file reference") || !strings.Contains(handoff, "file-reader/read_file") {
		t.Fatalf("handoff_instruction = %q, want durable reference and no-reread guidance", handoff)
	}
}

func TestSaveFileToolRejectsExpiredGeneratedToolFile(t *testing.T) {
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	workspaceID := uuid.NewString()
	conversationID := uuid.NewString()
	expiresAt := time.Now().Add(-time.Minute)
	toolFiles := &fakeToolFileStore{
		files: map[string]*tool_file.ToolFile{
			"tool-expired": {
				ID: "tool-expired", UserID: accountID, TenantID: organizationID,
				ConversationID: &conversationID, Name: "expired.md", MimeType: "text/markdown",
				Lifecycle: string(tool_file.ToolFileLifecycleTemporary), ExpiresAt: &expiresAt,
			},
		},
		data: map[string][]byte{"tool-expired": []byte("expired")},
	}
	fileService := &fakeFileService{files: map[string]*dto.UploadFile{}}
	provider := NewProvider(fileService, nil, &fakeWorkspacePermissionService{allowed: true}, WithToolFileStore(toolFiles))
	tool := saveFileRuntimeToolFrom(t, provider, organizationID, tools.ToolInvokeFromAIChat, map[string]interface{}{
		"organization_id": organizationID, "workspace_id": workspaceID,
		"tool_governance_permission_tier": "advanced",
	})

	_, err := tool.Invoke(context.Background(), accountID, map[string]interface{}{
		"source_type": "tool_file", "tool_file_id": "tool-expired", "filename": "expired.md",
	}, &conversationID, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "has expired") {
		t.Fatalf("Invoke() error = %v, want expired generated file", err)
	}
	if len(fileService.uploads) != 0 {
		t.Fatalf("uploads = %#v, want no managed copy", fileService.uploads)
	}
}

func TestSaveFileToolRequiresGovernanceApprovalOnBasicPermission(t *testing.T) {
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	workspaceID := uuid.NewString()
	conversationID := uuid.NewString()
	fileService := &fakeFileService{files: map[string]*dto.UploadFile{}}
	toolFiles := &fakeToolFileStore{
		files: map[string]*tool_file.ToolFile{
			"tool-1": {
				ID:             "tool-1",
				UserID:         accountID,
				TenantID:       organizationID,
				ConversationID: &conversationID,
				Name:           "draft.md",
				MimeType:       "text/markdown",
			},
		},
		data: map[string][]byte{"tool-1": []byte("# Draft")},
	}
	provider := NewProvider(fileService, nil, &fakeWorkspacePermissionService{allowed: true}, WithToolFileStore(toolFiles))
	tool := saveFileRuntimeToolFrom(t, provider, organizationID, tools.ToolInvokeFromAIChat, map[string]interface{}{
		"organization_id": organizationID,
		"workspace_id":    workspaceID,
	})

	_, err := tool.Invoke(context.Background(), accountID, map[string]interface{}{
		"source_type":  "tool_file",
		"tool_file_id": "tool-1",
		"filename":     "saved-draft.md",
	}, &conversationID, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "tool governance approval") {
		t.Fatalf("Invoke() error = %v, want governance approval requirement", err)
	}
	if len(fileService.uploads) != 0 {
		t.Fatalf("uploads = %#v, want no upload before approval", fileService.uploads)
	}
}

func TestSaveFileToolURLChecksPermissionBeforeInspectingSource(t *testing.T) {
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	workspaceID := uuid.NewString()
	conversationID := uuid.NewString()
	fileService := &fakeFileService{files: map[string]*dto.UploadFile{}}
	provider := NewProvider(fileService, nil, &fakeWorkspacePermissionService{allowed: false})
	tool := saveFileRuntimeToolFrom(t, provider, organizationID, tools.ToolInvokeFromAIChat, map[string]interface{}{
		"organization_id": organizationID,
		"workspace_id":    workspaceID,
	})

	_, err := tool.Invoke(context.Background(), accountID, map[string]interface{}{
		"source_type": "url",
		"url":         "not-a-url",
		"filename":    "remote.txt",
	}, &conversationID, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("Invoke() error = %v, want permission error before URL inspection", err)
	}
	if len(fileService.uploads) != 0 {
		t.Fatalf("uploads = %#v, want no upload", fileService.uploads)
	}
}

func TestSaveFileToolURLRequiresGovernanceBeforeDownload(t *testing.T) {
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	workspaceID := uuid.NewString()
	conversationID := uuid.NewString()
	fileService := &fakeFileService{files: map[string]*dto.UploadFile{}}
	provider := NewProvider(fileService, nil, &fakeWorkspacePermissionService{allowed: true})
	tool := saveFileRuntimeToolFrom(t, provider, organizationID, tools.ToolInvokeFromAIChat, map[string]interface{}{
		"organization_id": organizationID,
		"workspace_id":    workspaceID,
	})

	_, err := tool.Invoke(context.Background(), accountID, map[string]interface{}{
		"source_type": "url",
		"url":         "https://example.com/remote.txt",
		"filename":    "remote.txt",
	}, &conversationID, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "tool governance approval") {
		t.Fatalf("Invoke() error = %v, want governance approval before download", err)
	}
	if len(fileService.uploads) != 0 {
		t.Fatalf("uploads = %#v, want no upload", fileService.uploads)
	}
}

func TestSaveFileURLRedirectRejectsPrivateTarget(t *testing.T) {
	current, err := url.Parse("https://example.com/source.txt")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	if next, err := publicSaveFileRedirectURL(current, "http://127.0.0.1/private.txt"); err == nil {
		t.Fatalf("publicSaveFileRedirectURL() = %q, want private target error", next)
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
	return deleteFileRuntimeToolFrom(t, provider, organizationID, tools.ToolInvokeFromAIChat, map[string]interface{}{
		"organization_id": organizationID,
	})
}

func deleteFileRuntimeToolWithGrant(t *testing.T, provider *Provider, organizationID string, accountID string, conversationID string, fileID string, workspaceID string) tools.Tool {
	t.Helper()
	return deleteFileRuntimeToolFrom(t, provider, organizationID, tools.ToolInvokeFromAIChat, map[string]interface{}{
		"organization_id": organizationID,
		"workspace_id":    workspaceID,
		"tool_governance": map[string]interface{}{
			"session_grants": []map[string]interface{}{
				{
					"conversation_id":         conversationID,
					"organization_id":         organizationID,
					"user_id":                 accountID,
					"skill_id":                governedFileDeleteSkillID,
					"provider_type":           string(tools.ToolProviderTypeBuiltin),
					"provider_id":             ProviderID,
					"tool_id":                 governedFileDeleteToolID,
					"effect":                  "delete",
					"asset_type":              "file",
					"risk_level":              "high",
					"approval_correlation_id": "corr-delete",
					"expires_at":              time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
					"assets": []map[string]interface{}{
						{
							"id":           fileID,
							"type":         "file",
							"workspace_id": workspaceID,
						},
					},
				},
			},
		},
	})
}

func deleteFileRuntimeToolWithAssetlessGrant(t *testing.T, provider *Provider, organizationID string, accountID string, conversationID string, workspaceID string) tools.Tool {
	t.Helper()
	return deleteFileRuntimeToolFrom(t, provider, organizationID, tools.ToolInvokeFromAIChat, map[string]interface{}{
		"organization_id": organizationID,
		"workspace_id":    workspaceID,
		"tool_governance": map[string]interface{}{
			"session_grants": []map[string]interface{}{
				{
					"conversation_id":         conversationID,
					"organization_id":         organizationID,
					"user_id":                 accountID,
					"skill_id":                governedFileDeleteSkillID,
					"provider_type":           string(tools.ToolProviderTypeBuiltin),
					"provider_id":             ProviderID,
					"tool_id":                 governedFileDeleteToolID,
					"effect":                  "delete",
					"asset_type":              "file",
					"risk_level":              "high",
					"approval_correlation_id": "corr-delete-assetless",
					"expires_at":              time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
				},
			},
		},
	})
}

func deleteFileRuntimeToolFrom(t *testing.T, provider *Provider, organizationID string, invokeFrom tools.ToolInvokeFrom, runtimeParameters map[string]interface{}) tools.Tool {
	t.Helper()
	tool, err := provider.GetTool(ToolDeleteFile)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	return tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID:          organizationID,
		InvokeFrom:        invokeFrom,
		RuntimeParameters: runtimeParameters,
	})
}

func saveFileRuntimeToolFrom(t *testing.T, provider *Provider, organizationID string, invokeFrom tools.ToolInvokeFrom, runtimeParameters map[string]interface{}) tools.Tool {
	t.Helper()
	tool, err := provider.GetTool(ToolSaveFile)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	return tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID:          organizationID,
		InvokeFrom:        invokeFrom,
		RuntimeParameters: runtimeParameters,
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
	uploads []fakeFileUpload
}

type fakeFileListService struct {
	files []*filemodel.UploadFile
	last  map[string]interface{}
}

func (s *fakeFileListService) ListFilesInFolderWithFilters(_ context.Context, folderID string, page, limit int, keyword, sort, extension, processingStatus string, _ *time.Time, _ *time.Time, _ string, _ []string) ([]*filemodel.UploadFile, int64, error) {
	s.last = map[string]interface{}{"folder_id": folderID, "page": page, "limit": limit, "keyword": keyword, "sort": sort, "extension": extension, "processing_status": processingStatus}
	return s.files, int64(len(s.files)), nil
}

func (s *fakeFileListService) ListAllFilesWithFilters(_ context.Context, page, limit int, keyword, sort, extension, processingStatus string, _ *time.Time, _ *time.Time, _ string, _ string, _ []string) ([]*filemodel.UploadFile, int64, error) {
	s.last = map[string]interface{}{"page": page, "limit": limit, "keyword": keyword, "sort": sort, "extension": extension, "processing_status": processingStatus}
	return s.files, int64(len(s.files)), nil
}

func (s *fakeFileListService) CheckFolderViewPermission(_ context.Context, _ string, _ string, _ string, _ []string) (bool, error) {
	return true, nil
}

type fakeFileUpload struct {
	filename       string
	content        []byte
	mimeType       string
	userID         string
	organizationID string
	workspaceID    string
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

func (s *fakeFileService) UploadFile(_ context.Context, filename string, content []byte, mimeType string, userID, organizationID string, userRole filemodel.CreatedByRole, source *interfaces.FileSource, workspaceID *string, isTemporary bool, isIcon bool) (*dto.UploadFile, error) {
	_ = userRole
	_ = source
	_ = isTemporary
	_ = isIcon
	workspace := ""
	if workspaceID != nil {
		workspace = *workspaceID
	}
	if s.files == nil {
		s.files = map[string]*dto.UploadFile{}
	}
	id := "upload-" + strconv.Itoa(len(s.uploads)+1)
	upload := fakeFileUpload{
		filename:       filename,
		content:        append([]byte(nil), content...),
		mimeType:       mimeType,
		userID:         userID,
		organizationID: organizationID,
		workspaceID:    workspace,
	}
	s.uploads = append(s.uploads, upload)
	file := &dto.UploadFile{
		ID:             id,
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		Name:           filename,
		Size:           int64(len(content)),
		Extension:      strings.TrimPrefix(filepath.Ext(filename), "."),
		MimeType:       mimeType,
		CreatedBy:      userID,
		CreatedAt:      time.Unix(1700000200, 0),
	}
	s.files[id] = file
	return file, nil
}

func (s *fakeFileService) GetFileURL(_ context.Context, fileID string) (string, error) {
	if s.files[fileID] == nil {
		return "", errors.New("file not found")
	}
	return "https://files.example/" + fileID, nil
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

type fakeToolFileStore struct {
	files map[string]*tool_file.ToolFile
	data  map[string][]byte
}

func (s *fakeToolFileStore) GetToolFileByID(_ context.Context, toolFileID string) (*tool_file.ToolFile, error) {
	file := s.files[toolFileID]
	if file == nil {
		return nil, errors.New("tool file not found")
	}
	return file, nil
}

func (s *fakeToolFileStore) GetFileBinary(_ context.Context, toolFileID string) ([]byte, string, error) {
	file := s.files[toolFileID]
	data := s.data[toolFileID]
	if file == nil || data == nil {
		return nil, "", errors.New("tool file not found")
	}
	return append([]byte(nil), data...), file.MimeType, nil
}

type fakeContentExtractor struct {
	contents     map[string]*workflowfile.FileContent
	requestedIDs [][]string
	scopes       []workflowfile.ContentExtractionScope
}

func (e *fakeContentExtractor) ExtractMultipleFiles(_ context.Context, fileIDs []string, scope workflowfile.ContentExtractionScope) ([]*workflowfile.FileContent, error) {
	e.requestedIDs = append(e.requestedIDs, append([]string(nil), fileIDs...))
	e.scopes = append(e.scopes, scope)
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
