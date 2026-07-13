package files

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/dto"
	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	filemodel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

const (
	ProviderID           = "files"
	ToolListVisibleFiles = "list_visible_files"
	ToolReadFile         = "read_file"
	ToolDeleteFile       = "delete_file"
	ToolSaveFile         = "save_file_to_management"

	defaultReadFileMaxChars = 4000
	maxReadFileMaxChars     = 12000
	maxSaveFileBytes        = 25 * 1024 * 1024
	saveFileHTTPTimeout     = 30 * time.Second
	maxSaveFileRedirects    = 5

	governedFileDeleteSkillID       = "file-manager"
	legacyGovernedFileDeleteSkillID = "file-reader"
	governedFileDeleteToolID        = "file.delete"
	governedFileSaveToolID          = "file.save_to_management"
)

type FileService interface {
	GetFileByID(ctx context.Context, fileID string) (*dto.UploadFile, error)
	GetFile(ctx context.Context, fileID string) (string, error)
	UploadFile(ctx context.Context, filename string, content []byte, mimeType string, userID, organizationID string, userRole filemodel.CreatedByRole, source *interfaces.FileSource, workspaceID *string, isTemporary bool, isIcon bool) (*dto.UploadFile, error)
	GetFileURL(ctx context.Context, fileID string) (string, error)
	DeleteFiles(ctx context.Context, fileIDs []string) error
}

type FileListService interface {
	ListFilesInFolderWithFilters(ctx context.Context, folderID string, page, limit int, keyword, sort, extension, processingStatus string, startTime, endTime *time.Time, tenantID string, visibleWorkspaceIDs []string) ([]*filemodel.UploadFile, int64, error)
	ListAllFilesWithFilters(ctx context.Context, page, limit int, keyword, sort, extension, processingStatus string, startTime, endTime *time.Time, tenantID, accountID string, visibleWorkspaceIDs []string) ([]*filemodel.UploadFile, int64, error)
	CheckFolderViewPermission(ctx context.Context, folderID, accountID, tenantID string, visibleWorkspaceIDs []string) (bool, error)
}

type ContentExtractionService interface {
	ExtractMultipleFiles(ctx context.Context, fileIDs []string, scope workflowfile.ContentExtractionScope) ([]*workflowfile.FileContent, error)
}

type WorkspacePermissionService interface {
	CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) (bool, error)
}

type workspacePermissionLister interface {
	ListWorkspaceIDsByPermission(ctx context.Context, organizationID, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) ([]string, error)
}

type ToolFileStore interface {
	GetToolFileByID(ctx context.Context, toolFileID string) (*tool_file.ToolFile, error)
	GetFileBinary(ctx context.Context, toolFileID string) ([]byte, string, error)
}

type Provider struct {
	*builtin.BuiltinProvider
	fileService      FileService
	contentExtractor ContentExtractionService
	workspacePerms   WorkspacePermissionService
	fileListService  FileListService
	toolFiles        ToolFileStore
}

type ProviderOption func(*Provider)

func WithToolFileStore(store ToolFileStore) ProviderOption {
	return func(p *Provider) {
		p.toolFiles = store
	}
}

func WithFileListService(service FileListService) ProviderOption {
	return func(p *Provider) {
		p.fileListService = service
	}
}

func NewProvider(fileService FileService, contentExtractor ContentExtractionService, workspacePerms WorkspacePermissionService, options ...ProviderOption) *Provider {
	identity := tools.ToolProviderIdentity{
		Name:   ProviderID,
		Author: "System",
		Label: tools.I18nText{
			"en_US": "File Tools",
		},
		Description: tools.I18nText{
			"en_US": "Built-in tools for reading files the current user can access.",
		},
		Icon: "file-text",
		Tags: []string{"file", "system"},
	}
	provider := &Provider{
		BuiltinProvider:  builtin.NewBuiltinProvider(identity),
		fileService:      fileService,
		contentExtractor: contentExtractor,
		workspacePerms:   workspacePerms,
		toolFiles:        globalToolFileStore{},
	}
	for _, option := range options {
		if option != nil {
			option(provider)
		}
	}
	provider.RegisterTool(newReadFileTool(fileService, contentExtractor, workspacePerms))
	provider.RegisterTool(newListVisibleFilesTool(provider.fileListService, workspacePerms))
	provider.RegisterTool(newDeleteFileTool(fileService, workspacePerms))
	provider.RegisterTool(newSaveFileTool(fileService, workspacePerms, provider.toolFiles))
	return provider
}

type listVisibleFilesTool struct {
	*builtin.BuiltinTool
	fileListService FileListService
	workspacePerms  WorkspacePermissionService
}

type readFileTool struct {
	*builtin.BuiltinTool
	fileService      FileService
	contentExtractor ContentExtractionService
	workspacePerms   WorkspacePermissionService
}

type deleteFileTool struct {
	*builtin.BuiltinTool
	fileService    FileService
	workspacePerms WorkspacePermissionService
}

type saveFileTool struct {
	*builtin.BuiltinTool
	fileService    FileService
	workspacePerms WorkspacePermissionService
	toolFiles      ToolFileStore
}

type fileScope struct {
	OrganizationID string
	WorkspaceID    string
	AccountID      string
	InvokeFrom     tools.ToolInvokeFrom
}

type saveFileSource struct {
	Data       []byte
	Filename   string
	MimeType   string
	SourceType string
	SourceID   string
	SourceURL  string
	ExpiresAt  *time.Time
}

type readFileContent struct {
	Text      string
	FromCache bool
	Error     error
}

type globalToolFileStore struct{}

func newListVisibleFilesTool(fileListService FileListService, workspacePerms WorkspacePermissionService) tools.Tool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     ToolListVisibleFiles,
			Author:   "System",
			Provider: ProviderID,
			Label: tools.I18nText{
				"en_US": "List Visible Files",
			},
			Icon: "list",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US": "List files from the authoritative backend using the current Files page query.",
			},
			LLM: "List file assets from the backend with the same page, filter, folder, and workspace query as Console Files. Use the returned order for ordinal targets such as the first visible file. This does not read file contents.",
		},
		Parameters: []tools.ToolParameter{
			fileListStringParameter("workspace_id", "Workspace ID", "Optional current workspace filter."),
			fileListStringParameter("keyword", "Keyword", "Optional current search keyword."),
			fileListStringParameter("sort", "Sort", "Optional current sort, for example created_at_desc."),
			fileListStringParameter("extension", "Extension", "Optional current extension filter."),
			fileListStringParameter("processing_status", "Processing status", "Optional current processing status filter."),
			fileListStringParameter("category", "Category", "Current Files category: all, needs_action, uploaded, default, or a folder ID."),
			fileListStringParameter("folder_id", "Folder ID", "Optional explicit folder ID."),
			{Name: "page", Label: tools.I18nText{"en_US": "Page"}, LLMDescription: "Current one-based page number.", Type: tools.ToolParameterTypeNumber, Form: tools.ToolParameterFormLLM, SupportVariable: true},
			{Name: "page_size", Label: tools.I18nText{"en_US": "Page size"}, LLMDescription: "Current page size, capped at 100.", Type: tools.ToolParameterTypeNumber, Form: tools.ToolParameterFormLLM, SupportVariable: true},
			{Name: "selected_ids", Label: tools.I18nText{"en_US": "Selected IDs"}, LLMDescription: "Optional selected file IDs. Selection is verified against this backend result.", Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, SupportVariable: true},
		},
		OutputType: "json",
		Tags:       []string{"file", "system"},
	}
	return &listVisibleFilesTool{
		BuiltinTool:     builtin.NewBuiltinTool(entity, ""),
		fileListService: fileListService,
		workspacePerms:  workspacePerms,
	}
}

func fileListStringParameter(name string, label string, description string) tools.ToolParameter {
	return tools.ToolParameter{Name: name, Label: tools.I18nText{"en_US": label}, LLMDescription: description, Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, SupportVariable: true}
}

func newReadFileTool(fileService FileService, contentExtractor ContentExtractionService, workspacePerms WorkspacePermissionService) tools.Tool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     ToolReadFile,
			Author:   "System",
			Provider: ProviderID,
			Label: tools.I18nText{
				"en_US": "Read File",
			},
			Icon: "file-text",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US": "Read extracted text from a file the current user can access.",
			},
			LLM: "Read extracted text content from one uploaded file the current user can access. Pass a resolved file_id and inspect content_status, content_truncated, and content_error before answering. The returned content field is not durable across history, approvals, navigation, refresh, or later tool phases; if later steps depend on the file body or a derived summary, first record a concise reusable fact with submit_turn_state.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:            "file_id",
				Label:           tools.I18nText{"en_US": "File ID"},
				LLMDescription:  "Required file ID resolved from the current page context, attachment, or governed asset resolution.",
				Type:            tools.ToolParameterTypeString,
				Form:            tools.ToolParameterFormLLM,
				Required:        true,
				SupportVariable: true,
			},
			{
				Name:            "max_chars",
				Label:           tools.I18nText{"en_US": "Max characters"},
				LLMDescription:  "Maximum returned content characters. Defaults to 4000 and is capped at 12000.",
				Type:            tools.ToolParameterTypeNumber,
				Form:            tools.ToolParameterFormLLM,
				Required:        false,
				Default:         defaultReadFileMaxChars,
				SupportVariable: true,
			},
		},
		OutputType: "json",
		Tags:       []string{"file", "system"},
	}
	return &readFileTool{
		BuiltinTool:      builtin.NewBuiltinTool(entity, ""),
		fileService:      fileService,
		contentExtractor: contentExtractor,
		workspacePerms:   workspacePerms,
	}
}

func newDeleteFileTool(fileService FileService, workspacePerms WorkspacePermissionService) tools.Tool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     ToolDeleteFile,
			Author:   "System",
			Provider: ProviderID,
			Label: tools.I18nText{
				"en_US": "Delete File",
			},
			Icon: "trash-2",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US": "Delete one file the current user can manage.",
			},
			LLM: "Delete one uploaded file the current AIChat user can manage. This is irreversible and must only run after tool governance approval. Pass a resolved file_id from the current page context or governed asset resolution.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:            "file_id",
				Label:           tools.I18nText{"en_US": "File ID"},
				LLMDescription:  "Required file ID resolved from the current page context, attachment, or governed asset resolution. Do not invent IDs.",
				Type:            tools.ToolParameterTypeString,
				Form:            tools.ToolParameterFormLLM,
				Required:        true,
				SupportVariable: true,
			},
		},
		OutputType: "json",
		Tags:       []string{"file", "system", "destructive"},
	}
	return &deleteFileTool{
		BuiltinTool:    builtin.NewBuiltinTool(entity, ""),
		fileService:    fileService,
		workspacePerms: workspacePerms,
	}
}

func newSaveFileTool(fileService FileService, workspacePerms WorkspacePermissionService, toolFiles ToolFileStore) tools.Tool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     ToolSaveFile,
			Author:   "System",
			Provider: ProviderID,
			Label: tools.I18nText{
				"en_US": "Save File to Management",
			},
			Icon: "file-plus-2",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US": "Save a generated file or external file URL into File Management.",
			},
			LLM: "Copy an already generated tool file or a public external file URL into File Management. The source tool file remains available until its original expiry time; this tool does not move, consume, delete, or extend the source. Use this only when the user explicitly asks to save, create, upload, import, or add the file to File Management/current Files page. If the file content still needs to be generated, first call file-generator to create a temporary artifact, then call this tool with source_type=tool_file and the generated tool_file_id. This operation mutates File Management and is governed by file.create approval.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:            "source_type",
				Label:           tools.I18nText{"en_US": "Source type"},
				LLMDescription:  "Required source type. Use tool_file for a file just produced by another tool, or url for a public external file URL supplied by the user.",
				Type:            tools.ToolParameterTypeSelect,
				Form:            tools.ToolParameterFormLLM,
				Required:        true,
				Default:         "tool_file",
				SupportVariable: true,
				Options: []tools.ToolParameterOption{
					{Value: "tool_file", Label: tools.I18nText{"en_US": "Generated tool file"}},
					{Value: "url", Label: tools.I18nText{"en_US": "External URL"}},
				},
			},
			{
				Name:            "tool_file_id",
				Label:           tools.I18nText{"en_US": "Tool file ID"},
				LLMDescription:  "Required when source_type is tool_file. Use the file_id/tool_file_id returned by the generation tool. Do not invent IDs.",
				Type:            tools.ToolParameterTypeString,
				Form:            tools.ToolParameterFormLLM,
				Required:        false,
				SupportVariable: true,
			},
			{
				Name:            "url",
				Label:           tools.I18nText{"en_US": "URL"},
				LLMDescription:  "Required when source_type is url. Must be an absolute public http or https URL supplied by the user.",
				Type:            tools.ToolParameterTypeString,
				Form:            tools.ToolParameterFormLLM,
				Required:        false,
				SupportVariable: true,
			},
			{
				Name:            "filename",
				Label:           tools.I18nText{"en_US": "Filename"},
				LLMDescription:  "Required destination filename shown in File Management. Include a suitable extension and do not include path separators.",
				Type:            tools.ToolParameterTypeString,
				Form:            tools.ToolParameterFormLLM,
				Required:        true,
				SupportVariable: true,
			},
			{
				Name:            "workspace_id",
				Label:           tools.I18nText{"en_US": "Workspace ID"},
				LLMDescription:  "Optional target workspace ID. Usually omit so the current AIChat workspace context is used. Do not invent IDs.",
				Type:            tools.ToolParameterTypeString,
				Form:            tools.ToolParameterFormLLM,
				Required:        false,
				SupportVariable: true,
			},
		},
		OutputType: "file",
		Tags:       []string{"file", "system"},
	}
	return &saveFileTool{
		BuiltinTool:    builtin.NewBuiltinTool(entity, ""),
		fileService:    fileService,
		workspacePerms: workspacePerms,
		toolFiles:      toolFiles,
	}
}

func (t *listVisibleFilesTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	if t.fileListService == nil {
		return nil, fmt.Errorf("file list service is not configured")
	}
	scope, err := fileScopeFromRuntime(t.Runtime(), t.GetTenantID(), userID)
	if err != nil {
		return nil, err
	}
	page := intParam(params, "page", 1, 100000)
	pageSize := intParam(params, "page_size", intParam(params, "limit", 20, 100), 100)
	workspaceID := firstNonEmptyString(stringValue(params, "workspace_id"), scope.WorkspaceID)
	visibleWorkspaceIDs, err := t.visibleWorkspaceIDs(ctx, scope, workspaceID)
	if err != nil {
		return nil, err
	}
	keyword := strings.TrimSpace(stringValue(params, "keyword"))
	sortValue := strings.TrimSpace(stringValue(params, "sort"))
	extension := strings.TrimSpace(stringValue(params, "extension"))
	processingStatus := strings.TrimSpace(stringValue(params, "processing_status"))
	category := strings.ToLower(strings.TrimSpace(stringValue(params, "category")))
	if category == "" {
		category = "all"
	}
	if processingStatus == "" && category == "needs_action" {
		processingStatus = "parse_failed"
	}
	folderID := strings.TrimSpace(stringValue(params, "folder_id"))
	if folderID == "" && category != "all" && category != "needs_action" && category != "uploaded" && category != "default" {
		folderID = category
	}
	if folderID != "" {
		allowed, permissionErr := t.fileListService.CheckFolderViewPermission(ctx, folderID, scope.AccountID, scope.OrganizationID, visibleWorkspaceIDs)
		if permissionErr != nil {
			return nil, permissionErr
		}
		if !allowed {
			return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(map[string]interface{}{
				"status": "completed", "source": "backend_api", "page": page, "page_size": pageSize, "total": int64(0), "count": 0, "files": []interface{}{},
			})}, nil
		}
	}
	var startTime *time.Time
	if category == "uploaded" {
		value := time.Now().AddDate(0, -3, 0)
		startTime = &value
	}
	var files []*filemodel.UploadFile
	var total int64
	if category == "default" || folderID != "" {
		files, total, err = t.fileListService.ListFilesInFolderWithFilters(ctx, folderID, page, pageSize, keyword, sortValue, extension, processingStatus, startTime, nil, scope.OrganizationID, visibleWorkspaceIDs)
	} else {
		files, total, err = t.fileListService.ListAllFilesWithFilters(ctx, page, pageSize, keyword, sortValue, extension, processingStatus, startTime, nil, scope.OrganizationID, scope.AccountID, visibleWorkspaceIDs)
	}
	if err != nil {
		return nil, err
	}
	selected := stringSetFromAny(params["selected_ids"])
	payloadFiles := make([]interface{}, 0, len(files))
	selectedCount := 0
	for idx, file := range files {
		if file == nil {
			continue
		}
		payload := uploadFileModelPayload(file)
		payload["visible_index"] = idx + 1
		if _, ok := selected[file.ID]; ok {
			payload["selected"] = true
			selectedCount++
		}
		payloadFiles = append(payloadFiles, payload)
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(map[string]interface{}{
		"status":         "completed",
		"source":         "backend_api",
		"page":           page,
		"page_size":      pageSize,
		"total":          total,
		"has_more":       int64(page*pageSize) < total,
		"count":          len(payloadFiles),
		"selected_count": selectedCount,
		"query": map[string]interface{}{
			"workspace_id": workspaceID, "keyword": keyword, "sort": sortValue, "extension": extension,
			"processing_status": processingStatus, "category": category, "folder_id": folderID,
		},
		"files": payloadFiles,
	})}, nil
}

func (t *listVisibleFilesTool) visibleWorkspaceIDs(ctx context.Context, scope fileScope, workspaceID string) ([]string, error) {
	if workspaceID != "" {
		if t.workspacePerms == nil {
			return nil, fmt.Errorf("workspace permission service is not configured")
		}
		allowed, err := t.workspacePerms.CheckWorkspacePermission(ctx, scope.OrganizationID, workspaceID, scope.AccountID, workspacemodel.WorkspacePermissionWorkspaceView)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return []string{}, nil
		}
		return []string{workspaceID}, nil
	}
	lister, ok := t.workspacePerms.(workspacePermissionLister)
	if !ok {
		return []string{}, nil
	}
	return lister.ListWorkspaceIDsByPermission(ctx, scope.OrganizationID, scope.AccountID, workspacemodel.WorkspacePermissionWorkspaceView)
}

func (t *listVisibleFilesTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &listVisibleFilesTool{
		BuiltinTool:     t.BuiltinTool.ForkToolRuntime(runtime),
		fileListService: t.fileListService,
		workspacePerms:  t.workspacePerms,
	}
}

func uploadFileModelPayload(file *filemodel.UploadFile) map[string]interface{} {
	payload := map[string]interface{}{
		"id": file.ID, "file_id": file.ID, "name": file.Name, "size": file.Size, "extension": file.Extension,
		"mime_type": file.MimeType, "is_temporary": file.IsTemporary, "created_by": file.CreatedBy,
		"created_at": file.CreatedAt.Unix(),
	}
	if file.WorkspaceID != nil && strings.TrimSpace(*file.WorkspaceID) != "" {
		payload["workspace_id"] = strings.TrimSpace(*file.WorkspaceID)
	}
	return payload
}

func stringSetFromAny(value interface{}) map[string]struct{} {
	out := map[string]struct{}{}
	appendValue := func(raw interface{}) {
		for _, part := range strings.Split(strings.TrimSpace(fmt.Sprint(raw)), ",") {
			if part = strings.TrimSpace(part); part != "" {
				out[part] = struct{}{}
			}
		}
	}
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			appendValue(item)
		}
	case []interface{}:
		for _, item := range typed {
			appendValue(item)
		}
	default:
		if value != nil {
			appendValue(value)
		}
	}
	return out
}

func (t *readFileTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID

	if t.fileService == nil {
		return nil, fmt.Errorf("file service is not configured")
	}
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	fileID := fileIDParam(params)
	if fileID == "" {
		return nil, fmt.Errorf("file_id is required")
	}
	maxChars := intParam(params, "max_chars", defaultReadFileMaxChars, maxReadFileMaxChars)

	file, err := t.fileService.GetFileByID(ctx, fileID)
	if err != nil || file == nil {
		return nil, fmt.Errorf("file %s not found", fileID)
	}
	if err := t.ensureFileReadable(ctx, scope, file); err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"status":    "completed",
		"file":      uploadFilePayload(file),
		"max_chars": maxChars,
	}
	content := t.readContent(ctx, scope, fileID)
	applyContent(payload, content, maxChars)
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
}

func (t *readFileTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &readFileTool{
		BuiltinTool:      t.BuiltinTool.ForkToolRuntime(runtime),
		fileService:      t.fileService,
		contentExtractor: t.contentExtractor,
		workspacePerms:   t.workspacePerms,
	}
}

func (t *readFileTool) scope(userID string) (fileScope, error) {
	return fileScopeFromRuntime(t.Runtime(), t.GetTenantID(), userID)
}

func (t *deleteFileTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID

	if t.fileService == nil {
		return nil, fmt.Errorf("file service is not configured")
	}
	scope, err := fileScopeFromRuntime(t.Runtime(), t.GetTenantID(), userID)
	if err != nil {
		return nil, err
	}
	fileID := fileIDParam(params)
	if fileID == "" {
		return nil, fmt.Errorf("file_id is required")
	}

	file, err := t.fileService.GetFileByID(ctx, fileID)
	if err != nil || file == nil {
		return nil, fmt.Errorf("file %s not found", fileID)
	}
	if err := t.ensureFileManageable(ctx, scope, file); err != nil {
		return nil, err
	}
	if err := t.ensureGovernedDeleteApproved(scope, file, conversationID); err != nil {
		return nil, err
	}
	filePayload := uploadFilePayload(file)
	if err := t.fileService.DeleteFiles(ctx, []string{fileID}); err != nil {
		return nil, fmt.Errorf("failed to delete file: %w", err)
	}

	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(map[string]interface{}{
		"status":        "completed",
		"deleted_count": 1,
		"reversible":    false,
		"file":          filePayload,
	})}, nil
}

func (t *deleteFileTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &deleteFileTool{
		BuiltinTool:    t.BuiltinTool.ForkToolRuntime(runtime),
		fileService:    t.fileService,
		workspacePerms: t.workspacePerms,
	}
}

func (t *saveFileTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = appID
	_ = messageID

	if t.fileService == nil {
		return nil, fmt.Errorf("file service is not configured")
	}
	scope, err := fileScopeFromRuntime(t.Runtime(), t.GetTenantID(), userID)
	if err != nil {
		return nil, err
	}
	workspaceID := firstNonEmptyString(stringValue(params, "workspace_id"), scope.WorkspaceID)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required to save a file into File Management")
	}
	if err := t.ensureFileCreatable(ctx, scope, workspaceID); err != nil {
		return nil, err
	}
	sourceType := normalizedSaveFileSourceType(params)
	if isURLSaveFileSourceType(sourceType) {
		asset, err := saveFileAssetFromURLParams(params, workspaceID)
		if err != nil {
			return nil, err
		}
		if err := t.ensureGovernedSaveAllowed(scope, asset, conversationID); err != nil {
			return nil, err
		}
	}
	source, err := t.resolveSource(ctx, scope, params, conversationID)
	if err != nil {
		return nil, err
	}
	source.Filename = finalizeFileManagementFilename(stringValue(params, "filename"), source.Filename, source.MimeType)
	if source.Filename == "" {
		return nil, fmt.Errorf("filename is required")
	}
	if len(source.Data) == 0 {
		return nil, fmt.Errorf("source file is empty")
	}
	if !isURLSaveFileSourceType(source.SourceType) {
		if err := t.ensureGovernedSaveAllowed(scope, saveFileAsset(source.Filename, workspaceID, source.SourceType), conversationID); err != nil {
			return nil, err
		}
	}

	uploadFile, err := t.fileService.UploadFile(
		ctx,
		source.Filename,
		source.Data,
		source.MimeType,
		scope.AccountID,
		scope.OrganizationID,
		filemodel.CreatedByRoleAccount,
		nil,
		&workspaceID,
		false,
		false,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to save file into File Management: %w", err)
	}

	filePayload := uploadFilePayload(uploadFile)
	urlValue, _ := t.fileService.GetFileURL(ctx, uploadFile.ID)
	downloadURL := fmt.Sprintf("/console/api/files/%s/download", uploadFile.ID)

	payload := map[string]interface{}{
		"status":          "completed",
		"operation":       "copy",
		"source_retained": true,
		"file":            filePayload,
		"file_id":         uploadFile.ID,
		"upload_file_id":  uploadFile.ID,
		"filename":        uploadFile.Name,
		"mime_type":       uploadFile.MimeType,
		"size":            uploadFile.Size,
		"target":          "managed_file",
		"workspace_id":    workspaceID,
		"transfer_method": string(workflowfile.FileTransferMethodLocalFile),
		"download_url":    downloadURL,
		"source_type":     source.SourceType,
	}
	if urlValue != "" {
		payload["url"] = urlValue
	}
	if source.SourceID != "" {
		payload["source_file_id"] = source.SourceID
		if source.SourceType == "tool_file" {
			payload["source_tool_file_id"] = source.SourceID
		}
	}
	if source.ExpiresAt != nil {
		payload["source_expires_at"] = source.ExpiresAt.Unix()
	}
	if source.SourceURL != "" {
		payload["source_url"] = source.SourceURL
	}

	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
}

func (t *saveFileTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &saveFileTool{
		BuiltinTool:    t.BuiltinTool.ForkToolRuntime(runtime),
		fileService:    t.fileService,
		workspacePerms: t.workspacePerms,
		toolFiles:      t.toolFiles,
	}
}

func (t *deleteFileTool) ensureFileManageable(ctx context.Context, scope fileScope, file *dto.UploadFile) error {
	return ensureScopedFilePermission(ctx, scope, file, t.workspacePerms, workspacemodel.WorkspacePermissionFileManage)
}

func (t *deleteFileTool) ensureGovernedDeleteApproved(scope fileScope, file *dto.UploadFile, conversationID *string) error {
	conversation := ""
	if conversationID != nil {
		conversation = strings.TrimSpace(*conversationID)
	}
	if conversation == "" {
		return fmt.Errorf("file delete requires tool governance approval")
	}
	if t.governedDeleteApprovedWithSkill(scope, file, conversation, governedFileDeleteSkillID) {
		return nil
	}
	if t.governedDeleteApprovedWithSkill(scope, file, conversation, legacyGovernedFileDeleteSkillID) {
		return nil
	}
	return fmt.Errorf("file delete requires tool governance approval")
}

func (t *deleteFileTool) governedDeleteApprovedWithSkill(scope fileScope, file *dto.UploadFile, conversation string, skillID string) bool {
	decision := toolgovernance.Decide(toolgovernance.Request{
		Manifest:       governedFileDeleteManifest(skillID),
		PermissionTier: toolgovernance.PermissionTierBasic,
		ConversationID: conversation,
		OrganizationID: scope.OrganizationID,
		UserID:         scope.AccountID,
		SkillID:        skillID,
		ProviderType:   string(tools.ToolProviderTypeBuiltin),
		ProviderID:     ProviderID,
		Assets:         []toolgovernance.AssetRef{governedFileAsset(file)},
		SessionGrants:  toolGovernanceSessionGrantsFromRuntime(t.Runtime()),
	}, toolgovernance.DefaultPolicy())
	if decision.Status == toolgovernance.DecisionStatusAllowed &&
		decision.MatchedGrant != nil &&
		toolGovernanceGrantMatchesFile(*decision.MatchedGrant, file) {
		return true
	}
	return false
}

func (t *saveFileTool) resolveSource(ctx context.Context, scope fileScope, params map[string]interface{}, conversationID *string) (saveFileSource, error) {
	sourceType := normalizedSaveFileSourceType(params)
	switch sourceType {
	case "tool_file", "generated_file", "artifact":
		return t.resolveToolFileSource(ctx, scope, params, conversationID)
	case "url", "external_url":
		return resolveURLFileSource(ctx, params)
	default:
		return saveFileSource{}, fmt.Errorf("unsupported source_type: %s", sourceType)
	}
}

func normalizedSaveFileSourceType(params map[string]interface{}) string {
	sourceType := strings.ToLower(strings.TrimSpace(stringValue(params, "source_type")))
	if sourceType != "" {
		return sourceType
	}
	if stringValue(params, "url") != "" {
		return "url"
	}
	return "tool_file"
}

func isURLSaveFileSourceType(sourceType string) bool {
	sourceType = strings.ToLower(strings.TrimSpace(sourceType))
	return sourceType == "url" || sourceType == "external_url"
}

func saveFileAssetFromURLParams(params map[string]interface{}, workspaceID string) (toolgovernance.AssetRef, error) {
	rawURL := strings.TrimSpace(stringValue(params, "url"))
	if rawURL == "" {
		return toolgovernance.AssetRef{}, fmt.Errorf("url is required when source_type is url")
	}
	if _, err := inspectPublicSaveFileURL(rawURL); err != nil {
		return toolgovernance.AssetRef{}, err
	}
	filename := finalizeFileManagementFilename(stringValue(params, "filename"), filenameFromURL(rawURL), "")
	return saveFileAsset(filename, workspaceID, "url"), nil
}

func saveFileAsset(filename string, workspaceID string, sourceType string) toolgovernance.AssetRef {
	return toolgovernance.AssetRef{
		Type:        "file",
		Name:        filename,
		WorkspaceID: workspaceID,
		Source:      "tool_arguments",
		Metadata: map[string]interface{}{
			"source_type": sourceType,
		},
	}
}

func (t *saveFileTool) resolveToolFileSource(ctx context.Context, scope fileScope, params map[string]interface{}, conversationID *string) (saveFileSource, error) {
	if t.toolFiles == nil {
		return saveFileSource{}, fmt.Errorf("tool file store is not configured")
	}
	toolFileID := firstNonEmptyString(stringValue(params, "tool_file_id"), stringValue(params, "source_file_id"), stringValue(params, "file_id"))
	if toolFileID == "" {
		return saveFileSource{}, fmt.Errorf("tool_file_id is required when source_type is tool_file")
	}
	toolFile, err := t.toolFiles.GetToolFileByID(ctx, toolFileID)
	if err != nil {
		return saveFileSource{}, fmt.Errorf("failed to load generated file metadata: %w", err)
	}
	if toolFile == nil {
		return saveFileSource{}, fmt.Errorf("generated file %s not found", toolFileID)
	}
	if toolFile.IsExpired(time.Now()) {
		return saveFileSource{}, fmt.Errorf("generated file %s has expired", toolFileID)
	}
	if strings.TrimSpace(toolFile.TenantID) != scope.OrganizationID {
		return saveFileSource{}, fmt.Errorf("generated file is not accessible")
	}
	if strings.TrimSpace(toolFile.UserID) != "" && strings.TrimSpace(toolFile.UserID) != scope.AccountID {
		return saveFileSource{}, fmt.Errorf("generated file is not accessible")
	}
	if conversationID != nil && strings.TrimSpace(*conversationID) != "" && toolFile.ConversationID != nil && strings.TrimSpace(*toolFile.ConversationID) != strings.TrimSpace(*conversationID) {
		return saveFileSource{}, fmt.Errorf("generated file is not accessible in this conversation")
	}
	data, mimeType, err := t.toolFiles.GetFileBinary(ctx, toolFileID)
	if err != nil {
		return saveFileSource{}, fmt.Errorf("failed to read generated file: %w", err)
	}
	if mimeType == "" {
		mimeType = toolFile.MimeType
	}
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	return saveFileSource{
		Data:       data,
		Filename:   toolFile.Name,
		MimeType:   normalizeMimeType(mimeType),
		SourceType: "tool_file",
		SourceID:   toolFileID,
		ExpiresAt:  toolFile.ExpiresAt,
	}, nil
}

func resolveURLFileSource(ctx context.Context, params map[string]interface{}) (saveFileSource, error) {
	rawURL := strings.TrimSpace(stringValue(params, "url"))
	if rawURL == "" {
		return saveFileSource{}, fmt.Errorf("url is required when source_type is url")
	}
	data, mimeType, finalURL, err := downloadPublicSaveFileURL(ctx, rawURL)
	if err != nil {
		return saveFileSource{}, err
	}
	return saveFileSource{
		Data:       data,
		Filename:   filenameFromURL(finalURL),
		MimeType:   mimeType,
		SourceType: "url",
		SourceURL:  rawURL,
	}, nil
}

func downloadPublicSaveFileURL(ctx context.Context, rawURL string) ([]byte, string, string, error) {
	client := publicSaveFileHTTPClient()
	currentURL := rawURL
	for redirects := 0; redirects <= maxSaveFileRedirects; redirects++ {
		parsed, err := inspectPublicSaveFileURL(currentURL)
		if err != nil {
			return nil, "", "", err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to create file download request: %w", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to download file URL: %w", err)
		}
		if isSaveFileRedirectStatus(resp.StatusCode) {
			nextURL, redirectErr := publicSaveFileRedirectURL(parsed, resp.Header.Get("Location"))
			resp.Body.Close()
			if redirectErr != nil {
				return nil, "", "", redirectErr
			}
			currentURL = nextURL
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return nil, "", "", fmt.Errorf("failed to download file URL: status %d", resp.StatusCode)
		}
		reader := io.LimitReader(resp.Body, maxSaveFileBytes+1)
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to read downloaded file: %w", err)
		}
		if len(data) > maxSaveFileBytes {
			return nil, "", "", fmt.Errorf("downloaded file exceeds %d bytes", maxSaveFileBytes)
		}
		mimeType := normalizeMimeType(resp.Header.Get("Content-Type"))
		if shouldPreferDetectedFileMimeType(mimeType, http.DetectContentType(data)) {
			mimeType = http.DetectContentType(data)
		}
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		return data, mimeType, parsed.String(), nil
	}
	return nil, "", "", fmt.Errorf("file URL exceeded %d redirects", maxSaveFileRedirects)
}

func publicSaveFileHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = publicSaveFileDialContext
	return &http.Client{
		Timeout: saveFileHTTPTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: transport,
	}
}

func publicSaveFileDialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve file URL host: %w", err)
	}
	dialer := &net.Dialer{}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip.IP)
		if !ok || !isPublicSaveFileAddr(addr) {
			continue
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
	}
	return nil, fmt.Errorf("file URL host resolved to a non-public address")
}

func inspectPublicSaveFileURL(rawURL string) (*url.URL, error) {
	info, err := workflowfile.InspectExternalURL(rawURL)
	if err != nil {
		return nil, err
	}
	if !info.IsPublic {
		return nil, fmt.Errorf("file URL must be publicly reachable")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("file URL is invalid: %w", err)
	}
	return parsed, nil
}

func publicSaveFileRedirectURL(current *url.URL, location string) (string, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return "", fmt.Errorf("file URL redirect missing location")
	}
	next, err := current.Parse(location)
	if err != nil {
		return "", fmt.Errorf("file URL redirect is invalid: %w", err)
	}
	if _, err := inspectPublicSaveFileURL(next.String()); err != nil {
		return "", err
	}
	return next.String(), nil
}

func isSaveFileRedirectStatus(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func isPublicSaveFileAddr(addr netip.Addr) bool {
	return !addr.IsPrivate() &&
		!addr.IsLoopback() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsLinkLocalMulticast() &&
		!addr.IsMulticast() &&
		!addr.IsUnspecified()
}

func (t *saveFileTool) ensureFileCreatable(ctx context.Context, scope fileScope, workspaceID string) error {
	if t.workspacePerms == nil {
		return fmt.Errorf("workspace permission service is not configured")
	}
	allowed, err := t.workspacePerms.CheckWorkspacePermission(ctx, scope.OrganizationID, workspaceID, scope.AccountID, workspacemodel.WorkspacePermissionFileUploadCreate)
	if err != nil {
		return fmt.Errorf("failed to check workspace file creation permission: %w", err)
	}
	if !allowed {
		return fmt.Errorf("user does not have permission to create files in this workspace")
	}
	return nil
}

func (t *saveFileTool) ensureGovernedSaveAllowed(scope fileScope, asset toolgovernance.AssetRef, conversationID *string) error {
	conversation := ""
	if conversationID != nil {
		conversation = strings.TrimSpace(*conversationID)
	}
	if conversation == "" {
		return fmt.Errorf("file save requires tool governance context")
	}
	decision := toolgovernance.Decide(toolgovernance.Request{
		Manifest:       governedFileSaveManifest(),
		PermissionTier: toolGovernancePermissionTierFromRuntime(t.Runtime()),
		ConversationID: conversation,
		OrganizationID: scope.OrganizationID,
		UserID:         scope.AccountID,
		SkillID:        governedFileDeleteSkillID,
		ProviderType:   string(tools.ToolProviderTypeBuiltin),
		ProviderID:     ProviderID,
		Assets:         []toolgovernance.AssetRef{asset},
		SessionGrants:  toolGovernanceSessionGrantsFromRuntime(t.Runtime()),
	}, toolgovernance.DefaultPolicy())
	if decision.Status == toolgovernance.DecisionStatusAllowed {
		return nil
	}
	return fmt.Errorf("file save requires tool governance approval")
}

func toolGovernanceGrantMatchesFile(grant toolgovernance.SessionGrant, file *dto.UploadFile) bool {
	if file == nil || strings.TrimSpace(file.ID) == "" || len(grant.Assets) == 0 {
		return false
	}
	expected := governedFileAsset(file)
	for _, asset := range grant.Assets {
		if strings.TrimSpace(asset.ID) != "" && strings.TrimSpace(asset.ID) != expected.ID {
			continue
		}
		if strings.TrimSpace(asset.ID) == "" {
			continue
		}
		if strings.TrimSpace(asset.Type) != "" && strings.TrimSpace(asset.Type) != expected.Type {
			continue
		}
		if strings.TrimSpace(asset.WorkspaceID) != "" && expected.WorkspaceID != "" && strings.TrimSpace(asset.WorkspaceID) != expected.WorkspaceID {
			continue
		}
		return true
	}
	return false
}

func governedFileDeleteManifest(skillID string) toolgovernance.Manifest {
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		skillID = governedFileDeleteSkillID
	}
	return toolgovernance.Manifest{
		ToolID:                 governedFileDeleteToolID,
		SkillID:                skillID,
		Domain:                 "files",
		Effect:                 toolgovernance.EffectDelete,
		AssetType:              "file",
		RiskLevel:              toolgovernance.RiskLevelHigh,
		PermissionScopes:       []string{"file:manage"},
		DefaultApprovalPolicy:  toolgovernance.ApprovalPolicyAlwaysAsk,
		AllowedPermissionTiers: []toolgovernance.PermissionTier{toolgovernance.PermissionTierBasic, toolgovernance.PermissionTierAdvanced, toolgovernance.PermissionTierFull},
		AuditRequired:          true,
	}
}

func governedFileSaveManifest() toolgovernance.Manifest {
	return toolgovernance.Manifest{
		ToolID:                 governedFileSaveToolID,
		SkillID:                governedFileDeleteSkillID,
		Domain:                 "files",
		Effect:                 toolgovernance.EffectCreate,
		AssetType:              "file",
		RiskLevel:              toolgovernance.RiskLevelMedium,
		PermissionScopes:       []string{"file:create"},
		DefaultApprovalPolicy:  toolgovernance.ApprovalPolicyAutoByPermissionTier,
		AllowedPermissionTiers: []toolgovernance.PermissionTier{toolgovernance.PermissionTierBasic, toolgovernance.PermissionTierAdvanced, toolgovernance.PermissionTierFull},
		AuditRequired:          true,
	}
}

func governedFileAsset(file *dto.UploadFile) toolgovernance.AssetRef {
	if file == nil {
		return toolgovernance.AssetRef{Type: "file"}
	}
	return toolgovernance.AssetRef{
		ID:          strings.TrimSpace(file.ID),
		Type:        "file",
		Name:        strings.TrimSpace(file.Name),
		WorkspaceID: uploadFileWorkspaceID(file),
	}
}

func toolGovernancePermissionTierFromRuntime(runtime *tools.ToolRuntime) toolgovernance.PermissionTier {
	if runtime == nil || len(runtime.RuntimeParameters) == 0 {
		return ""
	}
	params := runtime.RuntimeParameters
	if value := strings.TrimSpace(stringValue(params, "tool_governance_permission_tier")); value != "" {
		return toolgovernance.PermissionTier(value)
	}
	if governance := mapFromAny(params["tool_governance"]); len(governance) > 0 {
		if value := strings.TrimSpace(stringValue(governance, "permission_tier")); value != "" {
			return toolgovernance.PermissionTier(value)
		}
	}
	return ""
}

func toolGovernanceSessionGrantsFromRuntime(runtime *tools.ToolRuntime) []toolgovernance.SessionGrant {
	if runtime == nil || len(runtime.RuntimeParameters) == 0 {
		return nil
	}
	params := runtime.RuntimeParameters
	grants := toolGovernanceSessionGrantsFromAny(params["tool_governance_session_grants"])
	grants = append(grants, toolGovernanceSessionGrantsFromAny(params["tool_governance_one_shot_grants"])...)
	if governance := mapFromAny(params["tool_governance"]); len(governance) > 0 {
		grants = append(grants, toolGovernanceSessionGrantsFromAny(governance["session_grants"])...)
		grants = append(grants, toolGovernanceSessionGrantsFromAny(governance["one_shot_grants"])...)
	}
	return grants
}

func toolGovernanceSessionGrantsFromAny(value interface{}) []toolgovernance.SessionGrant {
	switch typed := value.(type) {
	case []toolgovernance.SessionGrant:
		return typed
	case []map[string]interface{}:
		out := make([]toolgovernance.SessionGrant, 0, len(typed))
		for _, item := range typed {
			if grant, ok := toolGovernanceSessionGrantFromAny(item); ok {
				out = append(out, grant)
			}
		}
		return out
	case []interface{}:
		out := make([]toolgovernance.SessionGrant, 0, len(typed))
		for _, item := range typed {
			if grant, ok := toolGovernanceSessionGrantFromAny(item); ok {
				out = append(out, grant)
			}
		}
		return out
	default:
		if grant, ok := toolGovernanceSessionGrantFromAny(value); ok {
			return []toolgovernance.SessionGrant{grant}
		}
		return nil
	}
}

func toolGovernanceSessionGrantFromAny(value interface{}) (toolgovernance.SessionGrant, bool) {
	switch typed := value.(type) {
	case toolgovernance.SessionGrant:
		return typed, true
	case map[string]interface{}, map[string]string:
		data, err := json.Marshal(typed)
		if err != nil {
			return toolgovernance.SessionGrant{}, false
		}
		var grant toolgovernance.SessionGrant
		if err := json.Unmarshal(data, &grant); err != nil {
			return toolgovernance.SessionGrant{}, false
		}
		if strings.TrimSpace(grant.ConversationID) == "" || strings.TrimSpace(grant.ToolID) == "" {
			return toolgovernance.SessionGrant{}, false
		}
		return grant, true
	default:
		return toolgovernance.SessionGrant{}, false
	}
}

func fileScopeFromRuntime(runtime *tools.ToolRuntime, tenantID string, userID string) (fileScope, error) {
	tenantID = strings.TrimSpace(tenantID)
	organizationID := ""
	workspaceID := ""
	invokeFrom := tools.ToolInvokeFromAIChat
	if runtime != nil {
		if strings.TrimSpace(runtime.TenantID) != "" {
			tenantID = strings.TrimSpace(runtime.TenantID)
		}
		organizationID = strings.TrimSpace(stringValue(runtime.RuntimeParameters, "organization_id"))
		workspaceID = strings.TrimSpace(stringValue(runtime.RuntimeParameters, "workspace_id"))
		if runtime.InvokeFrom != "" {
			invokeFrom = runtime.InvokeFrom
		}
	}
	if organizationID == "" {
		organizationID = tenantID
	}
	accountID := strings.TrimSpace(userID)
	if accountID == "" {
		return fileScope{}, fmt.Errorf("account_id is required")
	}
	if organizationID == "" {
		return fileScope{}, fmt.Errorf("organization_id is required")
	}
	return fileScope{
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		AccountID:      accountID,
		InvokeFrom:     invokeFrom,
	}, nil
}

func (t *readFileTool) ensureFileReadable(ctx context.Context, scope fileScope, file *dto.UploadFile) error {
	return ensureScopedFilePermission(ctx, scope, file, t.workspacePerms, workspacemodel.WorkspacePermissionFilePreview)
}

func ensureScopedFilePermission(ctx context.Context, scope fileScope, file *dto.UploadFile, workspacePerms WorkspacePermissionService, permission workspacemodel.WorkspacePermissionCode) error {
	if file == nil {
		return fmt.Errorf("file is not accessible")
	}
	if file.IsTemporary {
		if strings.TrimSpace(file.CreatedBy) != scope.AccountID {
			return fmt.Errorf("file is not accessible")
		}
		return nil
	}

	organizationID := strings.TrimSpace(file.OrganizationID)
	if organizationID == "" {
		organizationID = strings.TrimSpace(file.TenantID)
	}
	if organizationID == "" || organizationID != scope.OrganizationID {
		return fmt.Errorf("file is not accessible")
	}

	workspaceID := uploadFileWorkspaceID(file)
	if workspaceID == "" {
		if strings.TrimSpace(file.CreatedBy) != scope.AccountID {
			return fmt.Errorf("file is not accessible")
		}
		return nil
	}
	if workspacePerms == nil {
		return fmt.Errorf("workspace permission service is not configured")
	}
	allowed, err := workspacePerms.CheckWorkspacePermission(ctx, organizationID, workspaceID, scope.AccountID, permission)
	if err != nil {
		return fmt.Errorf("failed to check workspace file permission: %w", err)
	}
	if !allowed {
		return fmt.Errorf("file is not accessible")
	}
	return nil
}

func (t *readFileTool) readContent(ctx context.Context, scope fileScope, fileID string) readFileContent {
	if t.contentExtractor != nil {
		contents, err := t.contentExtractor.ExtractMultipleFiles(ctx, []string{fileID}, workflowfile.ContentExtractionScope{
			OrganizationID: scope.OrganizationID,
			WorkspaceID:    scope.WorkspaceID,
		})
		if err != nil {
			return readFileContent{Error: fmt.Errorf("failed to extract file content: %w", err)}
		}
		if len(contents) != 1 || contents[0] == nil {
			return readFileContent{Error: fmt.Errorf("file content extraction returned no result")}
		}
		content := contents[0]
		if content.Error != nil {
			return readFileContent{Error: content.Error}
		}
		return readFileContent{Text: content.Content, FromCache: content.FromCache}
	}

	content, err := t.fileService.GetFile(ctx, fileID)
	if err != nil {
		return readFileContent{Error: fmt.Errorf("failed to read file content: %w", err)}
	}
	return readFileContent{Text: content}
}

func uploadFilePayload(file *dto.UploadFile) map[string]interface{} {
	payload := map[string]interface{}{
		"id":           file.ID,
		"name":         file.Name,
		"size":         file.Size,
		"extension":    file.Extension,
		"mime_type":    file.MimeType,
		"is_temporary": file.IsTemporary,
		"created_by":   file.CreatedBy,
		"created_at":   file.CreatedAt.Unix(),
	}
	if workspaceID := uploadFileWorkspaceID(file); workspaceID != "" {
		payload["workspace_id"] = workspaceID
	}
	return payload
}

func applyContent(payload map[string]interface{}, content readFileContent, maxChars int) {
	if content.Error != nil {
		payload["content_status"] = "error"
		payload["content"] = ""
		payload["content_chars"] = 0
		payload["content_truncated"] = false
		payload["from_cache"] = false
		payload["content_error"] = content.Error.Error()
		payload["instruction"] = "The file content could not be read. Explain this error to the user and do not claim to have inspected the file body."
		applyReadFileHandoffMetadata(payload, "error")
		return
	}
	text := strings.TrimSpace(content.Text)
	preview, truncated := truncateRunes(text, maxChars)
	status := "extracted"
	if text == "" {
		status = "empty"
	}
	payload["content_status"] = status
	payload["content"] = preview
	payload["content_chars"] = len([]rune(text))
	payload["content_truncated"] = truncated
	payload["from_cache"] = content.FromCache
	payload["instruction"] = readFileInstruction(status, truncated)
	applyReadFileHandoffMetadata(payload, status)
}

func readFileInstruction(status string, truncated bool) string {
	switch status {
	case "extracted":
		if truncated {
			return "Use the returned content field as the file body preview to answer the user's request. Mention that the content was truncated if the omitted tail could affect the answer, and ask for a narrower question or retry with a higher max_chars when needed. If later steps depend on this content or a derived summary after navigation, approval, refresh, or another tool phase, call submit_turn_state first with a concise summary or exact reusable fact."
		}
		return "Use the returned content field as the file body to answer the user's request. Do not ask the user to select the file again or say the file cannot be read. If later steps depend on this content or a derived summary after navigation, approval, refresh, or another tool phase, call submit_turn_state first with a concise summary or exact reusable fact."
	case "empty":
		return "The file was accessible but no extractable text content was found. Tell the user the file has no extractable text content."
	default:
		return "Inspect content_status, content, and content_error before answering."
	}
}

func applyReadFileHandoffMetadata(payload map[string]interface{}, status string) {
	payload["content_lifetime"] = "current_tool_result_only"
	payload["content_redacted_in_history"] = true
	payload["handoff_recommended"] = status == "extracted"
	if status != "extracted" {
		return
	}
	payload["recommended_next_tool"] = "submit_turn_state"
	payload["handoff_required_when"] = []string{
		"later steps need the file content, summary, theme, topic, quote, title, prompt, config, generated asset, or final answer",
		"the task will cross navigation, approval, refresh, user confirmation, or another tool/skill phase before using the content",
	}
	payload["handoff_instruction"] = "Before leaving this file-reading phase, record the concise reusable summary or exact short fact with submit_turn_state using source=file-reader/read_file, kind=working_fact, and visibility=model_only. Do not rely on the raw content being available after continuation boundaries."
}

func uploadFileWorkspaceID(file *dto.UploadFile) string {
	if file == nil {
		return ""
	}
	if file.WorkspaceID != nil {
		return strings.TrimSpace(*file.WorkspaceID)
	}
	if file.TeamTenantID != nil {
		return strings.TrimSpace(*file.TeamTenantID)
	}
	return ""
}

func fileIDParam(params map[string]interface{}) string {
	for _, key := range []string{"file_id", "upload_file_id", "id"} {
		if value := strings.TrimSpace(stringValue(params, key)); value != "" {
			return value
		}
	}
	return ""
}

func visibleFilesFromRuntime(runtime *tools.ToolRuntime) []map[string]interface{} {
	if runtime == nil || len(runtime.RuntimeParameters) == 0 {
		return nil
	}
	raw, ok := runtime.RuntimeParameters["console_files_visible_files"]
	if !ok {
		return nil
	}
	items := interfaceSlice(raw)
	if len(items) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		mapped, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if hasNonFileResourceMarker(mapped) {
			continue
		}
		fileID := strings.TrimSpace(firstStringFromMap(mapped, "file_id", "id", "resource_id"))
		name := strings.TrimSpace(firstStringFromMap(mapped, "name", "title", "filename", "file_name"))
		if fileID == "" && name == "" {
			continue
		}
		file := map[string]interface{}{}
		if ordinal := intFromAny(firstValueFromMap(mapped, "visible_index", "visible_ordinal", "ordinal")); ordinal > 0 {
			file["visible_index"] = ordinal
		}
		if fileID != "" {
			file["file_id"] = fileID
		}
		if name != "" {
			file["name"] = name
		}
		if extension := strings.TrimSpace(firstStringFromMap(mapped, "extension", "ext")); extension != "" {
			file["extension"] = extension
		}
		if mimeType := strings.TrimSpace(firstStringFromMap(mapped, "mime_type", "mime")); mimeType != "" {
			file["mime_type"] = mimeType
		}
		if workspaceID := strings.TrimSpace(firstStringFromMap(mapped, "workspace_id", "workspaceId")); workspaceID != "" {
			file["workspace_id"] = workspaceID
		}
		if selected := boolFromAny(firstValueFromMap(mapped, "selected", "is_selected")); selected {
			file["selected"] = true
		}
		out = append(out, file)
	}
	return out
}

func hasNonFileResourceMarker(mapped map[string]interface{}) bool {
	for _, key := range []string{"type", "resource_type", "kind", "resource_kind"} {
		if value := stringValue(mapped, key); value != "" && !strings.EqualFold(value, "file") {
			return true
		}
	}
	metadata := mapFromAny(mapped["metadata"])
	if value := firstStringFromMap(metadata, "resource_kind"); value != "" && !strings.EqualFold(value, "file") {
		return true
	}
	return false
}

func mapFromAny(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed
	case map[string]string:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out
	default:
		return nil
	}
}

func (globalToolFileStore) GetToolFileByID(ctx context.Context, toolFileID string) (*tool_file.ToolFile, error) {
	if tool_file.GlobalToolFileManager == nil {
		return nil, fmt.Errorf("tool file manager is not configured")
	}
	return tool_file.GlobalToolFileManager.GetToolFileByID(ctx, toolFileID)
}

func (globalToolFileStore) GetFileBinary(ctx context.Context, toolFileID string) ([]byte, string, error) {
	return tool_file.GetFileBinaryGlobal(ctx, toolFileID)
}

func finalizeFileManagementFilename(requested string, sourceName string, mimeType string) string {
	name := cleanFileManagementFilename(requested)
	if name == "" {
		name = cleanFileManagementFilename(sourceName)
	}
	if name == "" {
		name = "imported-file"
	}
	if filepath.Ext(name) == "" {
		if ext := filepath.Ext(cleanFileManagementFilename(sourceName)); ext != "" {
			name += ext
		} else if ext := extensionFromMimeType(mimeType); ext != "" {
			name += ext
		}
	}
	if len(name) > 200 {
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		if len(base) > 200-len(ext) {
			base = base[:200-len(ext)]
		}
		name = base + ext
	}
	return name
}

func cleanFileManagementFilename(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.ReplaceAll(raw, "\\", "/")
	if unescaped, err := url.PathUnescape(raw); err == nil {
		raw = unescaped
	}
	name := path.Base(raw)
	if name == "." || name == "/" {
		return ""
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r < 32:
			return -1
		case r == '/' || r == '\\':
			return '_'
		default:
			return r
		}
	}, name)
	name = strings.Trim(name, " ._-")
	return name
}

func filenameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return cleanFileManagementFilename(parsed.Path)
}

func normalizeMimeType(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if mediaType, _, err := mime.ParseMediaType(raw); err == nil {
		return mediaType
	}
	return raw
}

func shouldPreferDetectedFileMimeType(currentMimeType, detectedMimeType string) bool {
	currentMimeType = strings.TrimSpace(strings.ToLower(currentMimeType))
	detectedMimeType = strings.TrimSpace(strings.ToLower(detectedMimeType))
	if detectedMimeType == "" || detectedMimeType == "application/octet-stream" {
		return false
	}
	return currentMimeType == "" || currentMimeType == "application/octet-stream"
}

func extensionFromMimeType(mimeType string) string {
	extensions, err := mime.ExtensionsByType(strings.TrimSpace(mimeType))
	if err != nil || len(extensions) == 0 {
		return ""
	}
	return extensions[0]
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func stringValue(params map[string]interface{}, key string) string {
	if len(params) == 0 {
		return ""
	}
	value := params[key]
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func firstStringFromMap(params map[string]interface{}, keys ...string) string {
	value := firstValueFromMap(params, keys...)
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func firstValueFromMap(params map[string]interface{}, keys ...string) interface{} {
	if len(params) == 0 {
		return nil
	}
	for _, key := range keys {
		if value, ok := params[key]; ok {
			return value
		}
	}
	return nil
}

func interfaceSlice(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		return typed
	case []map[string]interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func boolFromAny(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func intFromAny(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}

func intParam(params map[string]interface{}, key string, defaultValue int, maxValue int) int {
	if len(params) == 0 {
		return defaultValue
	}
	value := intFromValue(params[key], defaultValue)
	if value <= 0 {
		return defaultValue
	}
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
}

func intFromValue(value interface{}, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func truncateRunes(value string, limit int) (string, bool) {
	if limit <= 0 {
		return "", strings.TrimSpace(value) != ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}
	return string(runes[:limit]), true
}

var _ tools.ToolProvider = (*Provider)(nil)
var _ tools.Tool = (*readFileTool)(nil)
var _ tools.Tool = (*deleteFileTool)(nil)
