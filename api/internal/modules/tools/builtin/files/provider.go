package files

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

const (
	ProviderID           = "files"
	ToolListVisibleFiles = "list_visible_files"
	ToolReadFile         = "read_file"
	ToolDeleteFile       = "delete_file"

	defaultReadFileMaxChars = 4000
	maxReadFileMaxChars     = 12000
)

type FileService interface {
	GetFileByID(ctx context.Context, fileID string) (*dto.UploadFile, error)
	GetFile(ctx context.Context, fileID string) (string, error)
	DeleteFiles(ctx context.Context, fileIDs []string) error
}

type ContentExtractionService interface {
	ExtractMultipleFiles(ctx context.Context, fileIDs []string, tenantID string) ([]*workflowfile.FileContent, error)
}

type WorkspacePermissionService interface {
	CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) (bool, error)
}

type Provider struct {
	*builtin.BuiltinProvider
	fileService      FileService
	contentExtractor ContentExtractionService
	workspacePerms   WorkspacePermissionService
}

func NewProvider(fileService FileService, contentExtractor ContentExtractionService, workspacePerms WorkspacePermissionService) *Provider {
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
	}
	provider.RegisterTool(newReadFileTool(fileService, contentExtractor, workspacePerms))
	provider.RegisterTool(newListVisibleFilesTool())
	provider.RegisterTool(newDeleteFileTool(fileService, workspacePerms))
	return provider
}

type listVisibleFilesTool struct {
	*builtin.BuiltinTool
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

type fileScope struct {
	OrganizationID string
	WorkspaceID    string
	AccountID      string
	InvokeFrom     tools.ToolInvokeFrom
}

type readFileContent struct {
	Text      string
	FromCache bool
	Error     error
}

func newListVisibleFilesTool() tools.Tool {
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
				"en_US": "List files visible in the current AIChat file page context.",
			},
			LLM: "List visible file assets from the current Console Files page context. Use this for questions like what files are visible, current files, selected files, or available files. This does not read file contents.",
		},
		OutputType: "json",
		Tags:       []string{"file", "system"},
	}
	return &listVisibleFilesTool{
		BuiltinTool: builtin.NewBuiltinTool(entity, ""),
	}
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
			LLM: "Read extracted text content from one uploaded file the current user can access. Pass a resolved file_id and inspect content_status, content_truncated, and content_error before answering.",
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

func (t *listVisibleFilesTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = ctx
	_ = userID
	_ = params
	_ = conversationID
	_ = appID
	_ = messageID

	files := visibleFilesFromRuntime(t.Runtime())
	if len(files) == 0 {
		return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(map[string]interface{}{
			"status": "completed",
			"count":  0,
			"files":  []interface{}{},
		})}, nil
	}
	payloadFiles := make([]interface{}, 0, len(files))
	selectedCount := 0
	for _, file := range files {
		if boolFromAny(file["selected"]) {
			selectedCount++
		}
		payloadFiles = append(payloadFiles, file)
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(map[string]interface{}{
		"status":         "completed",
		"count":          len(files),
		"selected_count": selectedCount,
		"files":          payloadFiles,
	})}, nil
}

func (t *listVisibleFilesTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &listVisibleFilesTool{
		BuiltinTool: t.BuiltinTool.ForkToolRuntime(runtime),
	}
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

func (t *deleteFileTool) ensureFileManageable(ctx context.Context, scope fileScope, file *dto.UploadFile) error {
	return ensureScopedFilePermission(ctx, scope, file, t.workspacePerms, workspacemodel.WorkspacePermissionFileManage)
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
	return ensureScopedFilePermission(ctx, scope, file, t.workspacePerms, workspacemodel.WorkspacePermissionFileDownload)
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
		contents, err := t.contentExtractor.ExtractMultipleFiles(ctx, []string{fileID}, scope.OrganizationID)
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
}

func readFileInstruction(status string, truncated bool) string {
	switch status {
	case "extracted":
		if truncated {
			return "Use the returned content field as the file body preview to answer the user's request. Mention that the content was truncated if the omitted tail could affect the answer, and ask for a narrower question or retry with a higher max_chars when needed."
		}
		return "Use the returned content field as the file body to answer the user's request. Do not ask the user to select the file again or say the file cannot be read."
	case "empty":
		return "The file was accessible but no extractable text content was found. Tell the user the file has no extractable text content."
	default:
		return "Inspect content_status, content, and content_error before answering."
	}
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
