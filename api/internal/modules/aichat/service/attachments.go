package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/ginext/internal/dto"
	aichatmodel "github.com/zgiai/ginext/internal/modules/aichat/model"
	workflowfile "github.com/zgiai/ginext/internal/modules/app/workflow/file"
	"github.com/zgiai/ginext/internal/modules/llm/multimodal"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	workspacemodel "github.com/zgiai/ginext/internal/modules/workspace/model"
)

const (
	defaultAIChatFileLimit             = 10
	attachmentPreviewRuneLimit         = 4000
	fallbackAttachmentContextRuneLimit = 12000

	attachmentContentStatusPending   = "pending"
	attachmentContentStatusExtracted = "extracted"
	attachmentContentStatusEmpty     = "empty"
	attachmentContentStatusVision    = "vision_ready"
	attachmentContentStatusFiltered  = "filtered"

	attachmentKindDocument = "document"
	attachmentKindImage    = "image"

	attachmentFilteredReasonModelWithoutVision = "model_without_vision"
)

type FileLookupService interface {
	GetUploadConfig() *interfaces.FileUploadConfigResponse
	GetFileByID(ctx context.Context, fileID string) (*dto.UploadFile, error)
	GetFileURL(ctx context.Context, fileID string) (string, error)
}

type ContentExtractionService interface {
	ExtractMultipleFiles(ctx context.Context, fileIDs []string, tenantID string) ([]*workflowfile.FileContent, error)
}

type WorkspacePermissionService interface {
	CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) (bool, error)
}

type attachmentBundle struct {
	Files []attachmentFile
}

type attachmentFile struct {
	ID             string
	Name           string
	Size           int64
	Extension      string
	MimeType       string
	WorkspaceID    *string
	IsTemporary    bool
	Kind           string
	Content        string
	ContentStatus  string
	ContentChars   int
	ContentPreview string
	FromCache      bool
	VisionDetail   string
	ImageURL       string
	FilteredReason string
}

func (s *service) resolveChatAttachmentReferences(ctx context.Context, scope Scope, fileIDs []string) (*attachmentBundle, error) {
	ids, err := normalizeAttachmentFileIDs(fileIDs, s.maxAttachmentFileCount())
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if s.fileService == nil {
		return nil, fmt.Errorf("%w: file service is unavailable", ErrInvalidInput)
	}

	bundle := &attachmentBundle{Files: make([]attachmentFile, 0, len(ids))}
	for _, id := range ids {
		file, err := s.fileService.GetFileByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("%w: file not found", ErrNotFound)
		}
		if file == nil {
			return nil, fmt.Errorf("%w: file not found", ErrNotFound)
		}
		if err := s.ensureAttachmentReadable(ctx, scope, file); err != nil {
			return nil, err
		}
		bundle.Files = append(bundle.Files, newPendingAttachmentFile(file))
	}
	return bundle, nil
}

func (s *service) extractPreparedAttachments(ctx context.Context, prepared *PreparedChat, onEvent func(StreamEvent) error) error {
	if prepared == nil || prepared.parts == nil || prepared.parts.Attachments == nil || len(prepared.parts.Attachments.Files) == 0 {
		return nil
	}
	if s.contentExtractor == nil && prepared.parts.Attachments.hasDocumentFiles() {
		return fmt.Errorf("%w: file extraction is unavailable", ErrInvalidInput)
	}

	total := len(prepared.parts.Attachments.Files)
	for index := range prepared.parts.Attachments.Files {
		file := &prepared.parts.Attachments.Files[index]
		s.emitPreparedEvent(ctx, prepared, streamEventFileParseStart, fileParseStartPayload(prepared, *file, index, total), onEvent)
		if file.isImage() {
			if !prepared.parts.ModelSupportsVision {
				file.applyFiltered(attachmentFilteredReasonModelWithoutVision)
				s.emitPreparedEvent(ctx, prepared, streamEventFileParseEnd, fileParseEndPayload(prepared, *file, index, total), onEvent)
				continue
			}
			imageURL, err := s.fileService.GetFileURL(ctx, file.ID)
			if err != nil {
				s.emitPreparedEvent(ctx, prepared, streamEventFileParseError, fileParseErrorPayload(prepared, *file, index, total, err.Error()), onEvent)
				return fmt.Errorf("%w: failed to prepare image input: %w", ErrInvalidInput, err)
			}
			file.applyVisionReady(imageURL)
			s.emitPreparedEvent(ctx, prepared, streamEventFileParseEnd, fileParseEndPayload(prepared, *file, index, total), onEvent)
			continue
		}
		content, err := s.extractSingleAttachment(ctx, prepared.Scope, file.ID)
		if err != nil {
			s.emitPreparedEvent(ctx, prepared, streamEventFileParseError, fileParseErrorPayload(prepared, *file, index, total, err.Error()), onEvent)
			return err
		}
		if content.Error != nil {
			err := fmt.Errorf("%w: failed to extract file content: %w", ErrInvalidInput, content.Error)
			s.emitPreparedEvent(ctx, prepared, streamEventFileParseError, fileParseErrorPayload(prepared, *file, index, total, err.Error()), onEvent)
			return err
		}
		file.applyExtractedContent(content.Content, content.FromCache)
		s.emitPreparedEvent(ctx, prepared, streamEventFileParseEnd, fileParseEndPayload(prepared, *file, index, total), onEvent)
	}
	return nil
}

func (s *service) extractSingleAttachment(ctx context.Context, scope Scope, fileID string) (*workflowfile.FileContent, error) {
	contents, err := s.contentExtractor.ExtractMultipleFiles(ctx, []string{fileID}, scope.OrganizationID.String())
	if err != nil {
		return nil, fmt.Errorf("%w: failed to extract file content: %w", ErrInvalidInput, err)
	}
	if len(contents) != 1 || contents[0] == nil {
		return nil, fmt.Errorf("%w: failed to extract file content", ErrInvalidInput)
	}
	return contents[0], nil
}

func (s *service) maxAttachmentFileCount() int {
	if s.fileService == nil {
		return defaultAIChatFileLimit
	}
	cfg := s.fileService.GetUploadConfig()
	if cfg == nil || cfg.WorkflowFileUploadLimit <= 0 {
		return defaultAIChatFileLimit
	}
	return cfg.WorkflowFileUploadLimit
}

func normalizeAttachmentFileIDs(fileIDs []string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = defaultAIChatFileLimit
	}
	seen := make(map[string]struct{}, len(fileIDs))
	ids := make([]string, 0, len(fileIDs))
	for _, raw := range fileIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) > limit {
		return nil, fmt.Errorf("%w: too many files", ErrInvalidInput)
	}
	return ids, nil
}

func (s *service) ensureAttachmentReadable(ctx context.Context, scope Scope, file *dto.UploadFile) error {
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

	workspaceID := uploadFileWorkspaceID(file)
	if workspaceID == "" {
		if strings.TrimSpace(file.CreatedBy) != accountID {
			return fmt.Errorf("%w: file is not accessible", ErrPermissionDenied)
		}
		return nil
	}
	if s.workspacePerms == nil {
		return fmt.Errorf("%w: workspace permission service is unavailable", ErrPermissionDenied)
	}
	allowed, err := s.workspacePerms.CheckWorkspacePermission(ctx, organizationID, workspaceID, accountID, workspacemodel.WorkspacePermissionFileDownload)
	if err != nil {
		return fmt.Errorf("failed to check workspace file permission: %w", err)
	}
	if !allowed {
		return fmt.Errorf("%w: file is not accessible", ErrPermissionDenied)
	}
	return nil
}

func uploadFileWorkspaceID(file *dto.UploadFile) string {
	if file == nil || file.WorkspaceID == nil {
		return ""
	}
	return strings.TrimSpace(*file.WorkspaceID)
}

func newPendingAttachmentFile(file *dto.UploadFile) attachmentFile {
	workspaceID := normalizeOptionalString(file.WorkspaceID)
	kind := attachmentKindDocument
	visionDetail := ""
	if multimodal.IsImageFile(file.Extension, file.MimeType) {
		kind = attachmentKindImage
		visionDetail = multimodal.ImageDetailHigh
	}
	return attachmentFile{
		ID:            file.ID,
		Name:          file.Name,
		Size:          file.Size,
		Extension:     file.Extension,
		MimeType:      file.MimeType,
		WorkspaceID:   workspaceID,
		IsTemporary:   file.IsTemporary,
		Kind:          kind,
		ContentStatus: attachmentContentStatusPending,
		VisionDetail:  visionDetail,
	}
}

func (f *attachmentFile) applyExtractedContent(content string, fromCache bool) {
	status := attachmentContentStatusExtracted
	if strings.TrimSpace(content) == "" {
		status = attachmentContentStatusEmpty
	}
	f.Content = content
	f.ContentStatus = status
	f.ContentChars = runeCount(content)
	f.ContentPreview = truncateRunes(content, attachmentPreviewRuneLimit)
	f.FromCache = fromCache
}

func (f *attachmentFile) applyVisionReady(imageURL string) {
	f.ImageURL = strings.TrimSpace(imageURL)
	f.Content = ""
	f.ContentStatus = attachmentContentStatusVision
	f.ContentChars = 0
	f.ContentPreview = ""
	f.FromCache = false
	f.FilteredReason = ""
	if strings.TrimSpace(f.VisionDetail) == "" {
		f.VisionDetail = multimodal.ImageDetailHigh
	}
}

func (f *attachmentFile) applyFiltered(reason string) {
	f.ImageURL = ""
	f.Content = ""
	f.ContentStatus = attachmentContentStatusFiltered
	f.ContentChars = 0
	f.ContentPreview = ""
	f.FromCache = false
	f.FilteredReason = strings.TrimSpace(reason)
}

func (b *attachmentBundle) metadataFiles() []map[string]interface{} {
	if b == nil || len(b.Files) == 0 {
		return nil
	}
	files := make([]map[string]interface{}, 0, len(b.Files))
	for _, file := range b.Files {
		files = append(files, map[string]interface{}{
			"id":              file.ID,
			"name":            file.Name,
			"size":            file.Size,
			"extension":       file.Extension,
			"mime_type":       file.MimeType,
			"workspace_id":    optionalStringValue(file.WorkspaceID),
			"is_temporary":    file.IsTemporary,
			"kind":            file.kind(),
			"content_status":  file.ContentStatus,
			"content_chars":   file.ContentChars,
			"content_preview": file.ContentPreview,
			"from_cache":      file.FromCache,
			"vision_detail":   optionalNonEmptyString(file.VisionDetail),
			"filtered_reason": optionalNonEmptyString(file.FilteredReason),
		})
	}
	return files
}

func (b *attachmentBundle) fullContentSections() string {
	if b == nil || len(b.Files) == 0 {
		return ""
	}
	return formatAttachmentSections(b.Files, func(file attachmentFile) string {
		return file.Content
	})
}

func (b *attachmentBundle) previewContentSections() string {
	if b == nil || len(b.Files) == 0 {
		return ""
	}
	return formatAttachmentSections(b.Files, func(file attachmentFile) string {
		return file.ContentPreview
	})
}

func (b *attachmentBundle) hasDocumentFiles() bool {
	if b == nil {
		return false
	}
	for _, file := range b.Files {
		if !file.isImage() {
			return true
		}
	}
	return false
}

func (b *attachmentBundle) imageParts() []adapter.MessageContentPart {
	if b == nil {
		return nil
	}
	parts := make([]adapter.MessageContentPart, 0, len(b.Files))
	for _, file := range b.Files {
		if !file.isVisionReadyImage() {
			continue
		}
		parts = append(parts, multimodal.BuildImageURLPart(file.ImageURL, file.VisionDetail))
	}
	return parts
}

func attachmentBundleFromMessageMetadata(metadata map[string]interface{}) *attachmentBundle {
	files := metadataFiles(metadata)
	if len(files) == 0 {
		return nil
	}
	bundle := &attachmentBundle{Files: make([]attachmentFile, 0, len(files))}
	for _, item := range files {
		file := attachmentFile{
			ID:             stringFromMetadata(item["id"]),
			Name:           stringFromMetadata(item["name"]),
			Size:           int64FromMetadata(item["size"]),
			Extension:      stringFromMetadata(item["extension"]),
			MimeType:       stringFromMetadata(item["mime_type"]),
			WorkspaceID:    stringPtrFromMetadata(item["workspace_id"]),
			IsTemporary:    boolFromMetadata(item["is_temporary"]),
			Kind:           stringFromMetadata(item["kind"]),
			ContentStatus:  stringFromMetadata(item["content_status"]),
			ContentChars:   intFromMetadata(item["content_chars"]),
			ContentPreview: stringFromMetadata(item["content_preview"]),
			FromCache:      boolFromMetadata(item["from_cache"]),
			VisionDetail:   stringFromMetadata(item["vision_detail"]),
			FilteredReason: stringFromMetadata(item["filtered_reason"]),
		}
		if strings.TrimSpace(file.Kind) == "" {
			file.Kind = inferAttachmentKind(file.Extension, file.MimeType)
		}
		if file.isImage() && strings.TrimSpace(file.VisionDetail) == "" {
			file.VisionDetail = multimodal.ImageDetailHigh
		}
		file.Content = file.ContentPreview
		bundle.Files = append(bundle.Files, file)
	}
	return bundle
}

func (s *service) historicalUserMessage(ctx context.Context, message *aichatmodel.Message, includeImages bool) *adapter.Message {
	if message == nil {
		return nil
	}
	bundle := attachmentBundleFromMessageMetadata(message.Metadata)
	sections := bundle.previewContentSections()
	text := userContentWithAttachments(message.Query, sections)
	imageParts := s.historicalImageParts(ctx, bundle, includeImages)
	content := multimodal.BuildUserContent(text, imageParts)
	if isEmptyAdapterContent(content) {
		return nil
	}
	return &adapter.Message{Role: "user", Content: content}
}

func (s *service) historicalImageParts(ctx context.Context, bundle *attachmentBundle, includeImages bool) []adapter.MessageContentPart {
	if !includeImages || bundle == nil || s.fileService == nil {
		return nil
	}
	parts := make([]adapter.MessageContentPart, 0, len(bundle.Files))
	for _, file := range bundle.Files {
		if !file.isImage() || file.ContentStatus == attachmentContentStatusFiltered {
			continue
		}
		imageURL, err := s.fileService.GetFileURL(ctx, file.ID)
		if err != nil {
			continue
		}
		parts = append(parts, multimodal.BuildImageURLPart(imageURL, file.VisionDetail))
	}
	return parts
}

func (s *service) currentUserContent(parts *chatRequestParts, text string) interface{} {
	if parts == nil || parts.Attachments == nil {
		return strings.TrimSpace(text)
	}
	return multimodal.BuildUserContent(text, parts.Attachments.imageParts())
}

func userContentWithAttachments(query string, attachmentSections string) string {
	query = strings.TrimSpace(query)
	attachmentSections = strings.TrimSpace(attachmentSections)
	if attachmentSections == "" {
		return query
	}
	if query == "" {
		return "Attached file content:\n" + attachmentSections
	}
	return query + "\n\nAttached file content:\n" + attachmentSections
}

func formatAttachmentSections(files []attachmentFile, contentFor func(attachmentFile) string) string {
	var builder strings.Builder
	for _, file := range files {
		if file.isImage() {
			continue
		}
		content := strings.TrimSpace(contentFor(file))
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString("File: ")
		if strings.TrimSpace(file.Name) != "" {
			builder.WriteString(strings.TrimSpace(file.Name))
		} else {
			builder.WriteString(file.ID)
		}
		if strings.TrimSpace(file.Extension) != "" {
			builder.WriteString(" .")
			builder.WriteString(strings.TrimPrefix(strings.TrimSpace(file.Extension), "."))
		}
		builder.WriteString("\n")
		if content != "" {
			builder.WriteString(content)
		} else {
			builder.WriteString("[No extractable text content]")
		}
	}
	return builder.String()
}

func (f attachmentFile) isImage() bool {
	return f.kind() == attachmentKindImage
}

func (f attachmentFile) isVisionReadyImage() bool {
	return f.isImage() && f.ContentStatus == attachmentContentStatusVision && strings.TrimSpace(f.ImageURL) != ""
}

func (f attachmentFile) kind() string {
	kind := strings.TrimSpace(f.Kind)
	if kind != "" {
		return kind
	}
	return inferAttachmentKind(f.Extension, f.MimeType)
}

func inferAttachmentKind(extension string, mimeType string) string {
	if multimodal.IsImageFile(extension, mimeType) {
		return attachmentKindImage
	}
	return attachmentKindDocument
}

func metadataFiles(metadata map[string]interface{}) []map[string]interface{} {
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata["files"]
	if !ok || raw == nil {
		return nil
	}
	switch files := raw.(type) {
	case []map[string]interface{}:
		return files
	case []interface{}:
		output := make([]map[string]interface{}, 0, len(files))
		for _, item := range files {
			if file, ok := item.(map[string]interface{}); ok {
				output = append(output, file)
			}
		}
		return output
	default:
		return nil
	}
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func optionalStringValue(value *string) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func optionalNonEmptyString(value string) interface{} {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func isEmptyAdapterContent(content interface{}) bool {
	switch value := content.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(value) == ""
	case []adapter.MessageContentPart:
		return len(value) == 0
	default:
		return false
	}
}

func stringPtrFromMetadata(value interface{}) *string {
	text := strings.TrimSpace(stringFromMetadata(value))
	if text == "" {
		return nil
	}
	return &text
}

func stringFromMetadata(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

func boolFromMetadata(value interface{}) bool {
	v, ok := value.(bool)
	return ok && v
}

func intFromMetadata(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func int64FromMetadata(value interface{}) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func runeCount(value string) int {
	return len([]rune(value))
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
