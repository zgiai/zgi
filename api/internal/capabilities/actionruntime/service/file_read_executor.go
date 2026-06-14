package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	actiondto "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/dto"
	"github.com/zgiai/zgi/api/internal/dto"
	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

const (
	defaultFileReadMaxChars = 4000
	maxFileReadMaxChars     = 12000
	maxFileReadFileCount    = 10
)

type FileReadLookupService interface {
	GetFileByID(ctx context.Context, fileID string) (*dto.UploadFile, error)
	GetFileURL(ctx context.Context, fileID string) (string, error)
}

type FileReadContentExtractor interface {
	ExtractMultipleFiles(ctx context.Context, fileIDs []string, tenantID string) ([]*workflowfile.FileContent, error)
}

type FileReadWorkspacePermissionService interface {
	CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) (bool, error)
}

type fileReadExecutor struct {
	fileService      FileReadLookupService
	contentExtractor FileReadContentExtractor
	workspacePerms   FileReadWorkspacePermissionService
}

func NewFileReadExecutor(fileService FileReadLookupService, contentExtractor FileReadContentExtractor, workspacePerms FileReadWorkspacePermissionService) Executor {
	return &fileReadExecutor{
		fileService:      fileService,
		contentExtractor: contentExtractor,
		workspacePerms:   workspacePerms,
	}
}

func (e *fileReadExecutor) Execute(ctx context.Context, scope Scope, view ActionRunView, req actiondto.ExecuteActionRequest) (*ExecutionResult, error) {
	if e.fileService == nil {
		return nil, fmt.Errorf("%w: file service is unavailable", ErrInvalidInput)
	}
	if view.Run == nil {
		return nil, ErrNotFound
	}
	fileIDs := collectFileReadIDs(view.Run.Arguments, view.Run.Resources)
	if len(fileIDs) == 0 {
		return nil, fmt.Errorf("%w: file_id is required", ErrInvalidInput)
	}
	if len(fileIDs) > maxFileReadFileCount {
		return nil, fmt.Errorf("%w: too many files", ErrInvalidInput)
	}

	includeContent := boolArgument(view.Run.Arguments, "include_content", true)
	if req.DryRun {
		includeContent = false
	}
	includeURL := boolArgument(view.Run.Arguments, "include_url", false)
	maxChars := intArgument(view.Run.Arguments, "max_chars", defaultFileReadMaxChars, maxFileReadMaxChars)

	files := make([]map[string]interface{}, 0, len(fileIDs))
	byID := make(map[string]map[string]interface{}, len(fileIDs))
	for _, fileID := range fileIDs {
		uploadFile, err := e.fileService.GetFileByID(ctx, fileID)
		if err != nil || uploadFile == nil {
			return nil, fmt.Errorf("%w: file %s not found", ErrNotFound, fileID)
		}
		if err := e.ensureFileReadable(ctx, scope, uploadFile); err != nil {
			return nil, err
		}

		item := fileReadMetadata(uploadFile)
		item["content_status"] = "metadata_only"
		if req.DryRun {
			item["content_status"] = "dry_run"
		}
		if includeURL {
			if fileURL, err := e.fileService.GetFileURL(ctx, fileID); err == nil && strings.TrimSpace(fileURL) != "" {
				item["preview_url"] = strings.TrimSpace(fileURL)
			} else if err != nil {
				item["preview_url_error"] = err.Error()
			}
		}
		files = append(files, item)
		byID[fileID] = item
	}

	contentErrors := 0
	if includeContent {
		if e.contentExtractor == nil {
			for _, item := range files {
				item["content_status"] = "unavailable"
				item["content_error"] = "file content extraction is unavailable"
			}
			contentErrors = len(files)
		} else {
			contents, err := e.contentExtractor.ExtractMultipleFiles(ctx, fileIDs, scope.OrganizationID.String())
			if err != nil {
				return nil, fmt.Errorf("%w: failed to extract file content: %w", ErrInvalidInput, err)
			}
			contentErrors = applyFileReadContents(fileIDs, byID, contents, maxChars)
		}
	}

	output := map[string]interface{}{
		"status":               "completed",
		"file_count":           len(files),
		"files":                files,
		"include_content":      includeContent,
		"include_url":          includeURL,
		"max_preview_chars":    maxChars,
		"content_error_count":  contentErrors,
		"content_preview_only": true,
	}
	if req.DryRun {
		output["dry_run"] = true
	}

	return &ExecutionResult{
		Output: output,
		Ledger: map[string]interface{}{
			"file_read": map[string]interface{}{
				"file_count":           len(files),
				"include_content":      includeContent,
				"include_url":          includeURL,
				"content_error_count":  contentErrors,
				"content_preview_only": true,
			},
		},
	}, nil
}

func (e *fileReadExecutor) ensureFileReadable(ctx context.Context, scope Scope, file *dto.UploadFile) error {
	accountID := scope.AccountID.String()
	if file.IsTemporary {
		if strings.TrimSpace(file.CreatedBy) != accountID {
			return fmt.Errorf("%w: file is not accessible", ErrPermissionDenied)
		}
		return nil
	}

	organizationID := strings.TrimSpace(file.OrganizationID)
	if organizationID == "" {
		organizationID = strings.TrimSpace(file.TenantID)
	}
	if organizationID != scope.OrganizationID.String() {
		return fmt.Errorf("%w: file is not accessible", ErrPermissionDenied)
	}

	workspaceID := fileReadWorkspaceID(file)
	if workspaceID == "" {
		return nil
	}
	if e.workspacePerms == nil {
		return fmt.Errorf("%w: workspace permission service is unavailable", ErrPermissionDenied)
	}
	allowed, err := e.workspacePerms.CheckWorkspacePermission(ctx, organizationID, workspaceID, accountID, workspacemodel.WorkspacePermissionFileDownload)
	if err != nil {
		return fmt.Errorf("failed to check workspace file permission: %w", err)
	}
	if !allowed {
		return fmt.Errorf("%w: file is not accessible", ErrPermissionDenied)
	}
	return nil
}

func collectFileReadIDs(arguments map[string]interface{}, resources map[string]interface{}) []string {
	seen := map[string]struct{}{}
	ids := make([]string, 0)
	add := func(raw string) {
		id := strings.TrimSpace(raw)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	for _, key := range []string{"file_id", "upload_file_id", "id"} {
		add(stringArgument(arguments, key))
	}
	for _, key := range []string{"file_ids", "upload_file_ids"} {
		for _, id := range stringSliceArgument(arguments, key) {
			add(id)
		}
	}
	for _, item := range resourceItems(resources) {
		if resourceType := strings.TrimSpace(stringFromValue(item["type"])); resourceType != "" && resourceType != "file" {
			continue
		}
		add(stringFromValue(item["id"]))
	}
	return ids
}

func resourceItems(resources map[string]interface{}) []map[string]interface{} {
	raw, ok := resources["items"]
	if !ok || raw == nil {
		return nil
	}
	switch value := raw.(type) {
	case []map[string]interface{}:
		return value
	case []interface{}:
		items := make([]map[string]interface{}, 0, len(value))
		for _, item := range value {
			if typed, ok := item.(map[string]interface{}); ok {
				items = append(items, typed)
			}
		}
		return items
	default:
		return nil
	}
}

func applyFileReadContents(fileIDs []string, byID map[string]map[string]interface{}, contents []*workflowfile.FileContent, maxChars int) int {
	contentErrors := 0
	for index, fileID := range fileIDs {
		item := byID[fileID]
		if item == nil {
			continue
		}
		if index >= len(contents) || contents[index] == nil {
			item["content_status"] = "unavailable"
			item["content_error"] = "content extraction returned no result"
			contentErrors++
			continue
		}
		content := contents[index]
		if content.FileID != "" {
			if mapped := byID[content.FileID]; mapped != nil {
				item = mapped
			}
		}
		if content.Error != nil {
			item["content_status"] = "error"
			item["content_error"] = content.Error.Error()
			contentErrors++
			continue
		}
		text := strings.TrimSpace(content.Content)
		item["content_chars"] = len([]rune(text))
		item["from_cache"] = content.FromCache
		if text == "" {
			item["content_status"] = "empty"
			item["content_preview"] = ""
			item["content_truncated"] = false
			continue
		}
		preview, truncated := truncateFileReadPreview(text, maxChars)
		item["content_status"] = "extracted"
		item["content_preview"] = preview
		item["content_truncated"] = truncated
	}
	return contentErrors
}

func fileReadMetadata(file *dto.UploadFile) map[string]interface{} {
	item := map[string]interface{}{
		"id":           file.ID,
		"name":         file.Name,
		"size":         file.Size,
		"extension":    file.Extension,
		"mime_type":    file.MimeType,
		"is_temporary": file.IsTemporary,
		"created_by":   file.CreatedBy,
		"created_at":   file.CreatedAt.Unix(),
	}
	if workspaceID := fileReadWorkspaceID(file); workspaceID != "" {
		item["workspace_id"] = workspaceID
	}
	return item
}

func fileReadWorkspaceID(file *dto.UploadFile) string {
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

func stringArgument(arguments map[string]interface{}, key string) string {
	if len(arguments) == 0 {
		return ""
	}
	return stringFromValue(arguments[key])
}

func stringSliceArgument(arguments map[string]interface{}, key string) []string {
	if len(arguments) == 0 {
		return nil
	}
	raw := arguments[key]
	switch value := raw.(type) {
	case []string:
		return append([]string(nil), value...)
	case []interface{}:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if text := stringFromValue(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		if text := stringFromValue(raw); text != "" {
			return []string{text}
		}
		return nil
	}
}

func boolArgument(arguments map[string]interface{}, key string, fallback bool) bool {
	if len(arguments) == 0 {
		return fallback
	}
	switch value := arguments[key].(type) {
	case bool:
		return value
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err == nil {
			return parsed
		}
	default:
		return fallback
	}
	return fallback
}

func intArgument(arguments map[string]interface{}, key string, fallback int, max int) int {
	if len(arguments) == 0 {
		return fallback
	}
	value := intFromValue(arguments[key], fallback)
	if value <= 0 {
		return fallback
	}
	if max > 0 && value > max {
		return max
	}
	return value
}

func stringFromValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
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

func truncateFileReadPreview(value string, limit int) (string, bool) {
	if limit <= 0 {
		return "", strings.TrimSpace(value) != ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}
	return string(runes[:limit]), true
}
