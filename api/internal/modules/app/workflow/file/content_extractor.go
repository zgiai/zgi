package file

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/internal/modules/file_process/service/extractor"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// ContentExtractor defines the interface for extracting text content from files in workflow execution.
//
// This interface provides methods to extract content from uploaded files and make it available
// to workflow nodes. It supports both single files and file lists, with automatic caching,
// timeout handling, and graceful error recovery.
//
// Usage Example:
//
//	extractor := NewContentExtractor(fileService, extractProcessor, config)
//
//	// Extract content from a single file
//	content, err := extractor.ExtractFileContent(ctx, "file-id-123", "tenant-id")
//	if err != nil {
//	    // Handle error - workflow continues with metadata only
//	    log.Warn("Content extraction failed", err)
//	}
//
//	// Process a file variable for workflow execution
//	variables, err := extractor.ProcessFileVariable(ctx, "document", fileData, "tenant-id")
//	// Returns: {"document": {...metadata...}, "document_content": "extracted text..."}
//
// Variable Naming Convention:
//   - Original variable: {variable_name} contains file metadata (ID, type, size, etc.)
//   - Content variable: {variable_name}_content contains extracted text content
//   - Example: "document" (metadata) and "document_content" (text)
//
// Error Handling:
//   - Extraction failures are logged but do not stop workflow execution
//   - On error, content variables contain error messages like "[Content extraction failed: ...]"
//   - Timeouts, unsupported formats, and missing files are handled gracefully
//   - Partial failures in file lists allow successful files to be processed
//
// Performance Considerations:
//   - Content is cached in the database after first extraction
//   - Extraction timeout is configurable (default 10 seconds)
//   - Large files (>50MB) are skipped automatically
//   - Content size is limited and truncated if necessary (default 1MB)
//   - Multiple files are processed in parallel with concurrency limit (5 concurrent)
type ContentExtractor interface {
	// ExtractFileContent extracts text content from a single file.
	//
	// This method retrieves a file by ID, checks for cached content, and extracts
	// text content if not cached. It handles various file formats including PDF,
	// DOCX, TXT, and more through the ExtractProcessor.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - fileID: Unique identifier of the file to extract
	//   - tenantID: Tenant identifier for access control
	//
	// Returns:
	//   - *FileContent: Struct containing extracted content, metadata, and any errors
	//   - error: Non-nil if extraction fails completely (file not found, timeout, etc.)
	//
	// Behavior:
	//   - Checks configuration to see if extraction is enabled
	//   - Returns cached content if available (when CacheEnabled is true)
	//   - Skips files larger than 50MB with appropriate message
	//   - Applies timeout from configuration (default 10 seconds)
	//   - Truncates content exceeding MaxContentSize limit
	//   - Updates cache after successful extraction
	//
	// Error Conditions:
	//   - File not found: Returns error with FileContent.Error set
	//   - Extraction timeout: Returns error after timeout period
	//   - Unsupported format: Returns error from ExtractProcessor
	//   - File too large: Returns FileContent with size limit message (no error)
	//   - Feature disabled: Returns empty content (no error)
	//
	// Example:
	//
	//	content, err := extractor.ExtractFileContent(ctx, "abc-123", "tenant-1")
	//	if err != nil {
	//	    log.Error("Extraction failed", err)
	//	    // Workflow continues with metadata only
	//	}
	//	fmt.Println("Extracted:", content.Content)
	ExtractFileContent(ctx context.Context, fileID string, tenantID string) (*FileContent, error)

	// ExtractMultipleFiles extracts content from multiple files in parallel.
	//
	// This method processes multiple files concurrently with a limit on concurrent
	// extractions to prevent resource exhaustion. It handles partial failures
	// gracefully, allowing successful extractions to complete even if some fail.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - fileIDs: Slice of file identifiers to extract
	//   - tenantID: Tenant identifier for access control
	//
	// Returns:
	//   - []*FileContent: Slice of results, one per file (same order as input)
	//   - error: Always nil (partial failures are captured in FileContent.Error)
	//
	// Behavior:
	//   - Processes up to 5 files concurrently (configurable via semaphore)
	//   - Each file extraction uses ExtractFileContent method
	//   - Waits for all extractions to complete before returning
	//   - Logs summary of successful vs failed extractions
	//   - Returns empty slice if input is empty
	//
	// Error Handling:
	//   - Individual file failures are stored in FileContent.Error
	//   - Method never returns error, only logs warnings
	//   - Partial results allow workflow to continue with available content
	//
	// Example:
	//
	//	fileIDs := []string{"file-1", "file-2", "file-3"}
	//	results, _ := extractor.ExtractMultipleFiles(ctx, fileIDs, "tenant-1")
	//	for i, result := range results {
	//	    if result.Error != nil {
	//	        log.Warn("File failed", result.FileID, result.Error)
	//	    } else {
	//	        fmt.Printf("File %d: %d bytes\n", i, len(result.Content))
	//	    }
	//	}
	ExtractMultipleFiles(ctx context.Context, fileIDs []string, tenantID string) ([]*FileContent, error)

	// ProcessFileVariable processes a file variable and returns both metadata and content.
	//
	// This method is designed for workflow execution, where file variables need to be
	// enriched with extracted content. It creates two variables: the original with
	// metadata and a new one with "_content" suffix containing the extracted text.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - variableName: Name of the variable (e.g., "document")
	//   - fileData: Map containing file metadata with "upload_file_id" field
	//   - tenantID: Tenant identifier for access control
	//
	// Returns:
	//   - map[string]interface{}: Map with two entries:
	//       {variableName}: original file metadata
	//       {variableName}_content: extracted text content
	//   - error: Non-nil if file ID is missing or extraction fails
	//
	// Behavior:
	//   - Extracts file ID from fileData["upload_file_id"]
	//   - Falls back to "id" or "related_id" fields if upload_file_id not found
	//   - Calls ExtractFileContent to get text content
	//   - Creates content variable with "_content" suffix
	//   - On error, content variable contains error message
	//
	// Variable Structure:
	//   Input fileData: {"upload_file_id": "abc", "name": "doc.pdf", "size": 1024, ...}
	//   Output: {
	//     "document": {"upload_file_id": "abc", "name": "doc.pdf", ...},
	//     "document_content": "This is the extracted text..."
	//   }
	//
	// Error Handling:
	//   - Missing file ID: Returns error, content set to empty string
	//   - Extraction failure: Returns error, content set to "[Content extraction failed: ...]"
	//   - Feature disabled: No error, content set to empty string
	//
	// Example:
	//
	//	fileData := map[string]interface{}{
	//	    "upload_file_id": "file-123",
	//	    "name": "report.pdf",
	//	    "size": 2048,
	//	}
	//	vars, err := extractor.ProcessFileVariable(ctx, "report", fileData, "tenant-1")
	//	// vars["report"] = metadata
	//	// vars["report_content"] = "extracted text..."
	ProcessFileVariable(ctx context.Context, variableName string, fileData map[string]interface{}, tenantID string) (map[string]interface{}, error)

	// ProcessFileListVariable processes a file list variable (array of files).
	//
	// This method handles file list variables where multiple files are uploaded.
	// It extracts content from each file and combines them into a single content
	// variable with clear separation between files.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - variableName: Name of the variable (e.g., "documents")
	//   - fileList: Slice of file metadata maps
	//   - tenantID: Tenant identifier for access control
	//
	// Returns:
	//   - map[string]interface{}: Map with two entries:
	//       {variableName}: original file list metadata
	//       {variableName}_content: combined extracted text from all files
	//   - error: Non-nil if no valid file IDs found or extraction fails
	//
	// Behavior:
	//   - Extracts file IDs from each item in fileList
	//   - Calls ExtractMultipleFiles to process all files in parallel
	//   - Combines content with "=== File N ===" separators
	//   - Includes error messages for failed files in output
	//   - Returns empty content if fileList is empty
	//
	// Content Format:
	//   === File 1 ===
	//   [content from first file]
	//
	//   === File 2 ===
	//   [content from second file]
	//
	//   [File 3: Content extraction failed: timeout]
	//
	// Error Handling:
	//   - Empty file list: No error, content set to empty string
	//   - No valid file IDs: Returns error, content set to empty string
	//   - Partial failures: No error, failed files show error messages in content
	//   - Feature disabled: No error, content set to empty string
	//
	// Example:
	//
	//	fileList := []interface{}{
	//	    map[string]interface{}{"upload_file_id": "file-1", "name": "doc1.pdf"},
	//	    map[string]interface{}{"upload_file_id": "file-2", "name": "doc2.pdf"},
	//	}
	//	vars, err := extractor.ProcessFileListVariable(ctx, "documents", fileList, "tenant-1")
	//	// vars["documents"] = [metadata array]
	//	// vars["documents_content"] = "=== File 1 ===\n...\n\n=== File 2 ===\n..."
	ProcessFileListVariable(ctx context.Context, variableName string, fileList []interface{}, tenantID string) (map[string]interface{}, error)
}

// FileContent represents the result of file content extraction.
//
// This struct contains both the extracted text content and metadata about the extraction.
// It is returned by ExtractFileContent and ExtractMultipleFiles methods.
//
// Fields:
//   - FileID: Unique identifier of the file that was processed
//   - Content: Extracted text content (empty if extraction failed or feature disabled)
//   - ContentType: MIME type of the file (e.g., "application/pdf", "text/plain")
//   - Size: Length of the extracted content in bytes
//   - Error: Non-nil if extraction failed, contains error details
//
// Usage:
//   - Check Error field first to determine if extraction succeeded
//   - Content may be truncated if it exceeds size limits
//   - Content may contain error messages like "[Content extraction failed: ...]"
//   - Size reflects the actual content length after any truncation
type FileContent struct {
	FileID      string // Unique identifier of the file
	Content     string // Extracted text content
	ContentType string // MIME type of the file
	Size        int    // Length of extracted content in bytes
	Error       error  // Error if extraction failed, nil on success
	FromCache   bool   // Whether content came from upload_files.content_text cache
}

// contentExtractor implements the ContentExtractor interface
type contentExtractor struct {
	fileService      interfaces.FileService
	extractProcessor *extractor.ExtractProcessor
	config           *Config
}

// NewContentExtractor creates a new ContentExtractor instance with the provided dependencies.
//
// This factory function initializes a ContentExtractor with all required services and
// configuration. It should be called during application startup or service container
// initialization.
//
// Parameters:
//   - fileService: Service for file storage and retrieval operations
//   - extractProcessor: Processor for extracting text from various file formats
//   - config: Configuration for extraction behavior (timeouts, limits, feature flags)
//
// Returns:
//   - ContentExtractor: Initialized extractor ready for use in workflow execution
//
// Example:
//
//	config := &Config{
//	    Enabled: true,
//	    MaxContentSize: 1048576,      // 1MB
//	    ExtractionTimeout: 10 * time.Second,
//	    CacheEnabled: true,
//	}
//	extractor := NewContentExtractor(fileService, extractProcessor, config)
func NewContentExtractor(fileService interfaces.FileService, extractProcessor *extractor.ExtractProcessor, config *Config) ContentExtractor {
	return &contentExtractor{
		fileService:      fileService,
		extractProcessor: extractProcessor,
		config:           config,
	}
}

func (ce *contentExtractor) getWorkflowRunIDFromContext(ctx context.Context) string {
	if workflowRunID, ok := ctx.Value("workflow_run_id").(string); ok && workflowRunID != "" {
		return workflowRunID
	}
	return ""
}

func (ce *contentExtractor) ExtractFileContent(ctx context.Context, fileID string, tenantID string) (*FileContent, error) {
	if !ce.config.Enabled {
		workflowRunID := ce.getWorkflowRunIDFromContext(ctx)
		logFields := []interface{}{
			"file_id", fileID,
			"tenant_id", tenantID,
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Info("File content extraction is disabled by configuration, returning metadata only", logFields...)

		return &FileContent{
			FileID:    fileID,
			Content:   "",
			FromCache: false,
		}, nil
	}

	extractCtx, cancel := context.WithTimeout(ctx, ce.config.ExtractionTimeout)
	defer cancel()

	workflowRunID := ce.getWorkflowRunIDFromContext(ctx)

	uploadFile, err := ce.fileService.GetFileByID(extractCtx, fileID)
	if err != nil {
		logger.Error("Failed to get file by ID", err)
		return &FileContent{
			FileID: fileID,
			Error:  fmt.Errorf("failed to get file: %w", err),
		}, err
	}

	// Skip content extraction for image/video/audio files - these should be handled as multimodal content
	ext := strings.ToLower(uploadFile.Extension)
	if isImageExtension(ext) || isVideoExtension(ext) || isAudioExtension(ext) {
		logFields := []interface{}{
			"file_id", fileID,
			"extension", ext,
			"mime_type", uploadFile.MimeType,
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Info("Skipping text content extraction for media file (image/video/audio)", logFields...)

		return &FileContent{
			FileID:      fileID,
			Content:     "", // Empty content - media files should be handled via vision/multimodal
			ContentType: uploadFile.MimeType,
			Size:        0,
			FromCache:   false,
		}, nil
	}

	const maxFileSizeForExtraction = 50 * 1024 * 1024
	if uploadFile.Size > maxFileSizeForExtraction {
		logFields := []interface{}{
			"file_id", fileID,
			"file_size", uploadFile.Size,
			"max_size", maxFileSizeForExtraction,
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Warn("File too large for content extraction, skipping", logFields...)

		sizeLimitMsg := fmt.Sprintf("[Content extraction skipped: file size %d bytes exceeds maximum %d bytes for extraction]", uploadFile.Size, maxFileSizeForExtraction)
		return &FileContent{
			FileID:      fileID,
			Content:     sizeLimitMsg,
			ContentType: uploadFile.MimeType,
			Size:        len(sizeLimitMsg),
			FromCache:   false,
		}, nil
	}

	if ce.config.CacheEnabled && uploadFile.ContentText != nil && *uploadFile.ContentText != "" {
		content := strings.ReplaceAll(*uploadFile.ContentText, "¶", "\n")

		logFields := []interface{}{
			"file_id", fileID,
			"content_size", len(content),
			"source", "database_cache",
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Info("Using pre-extracted file content from database", logFields...)

		return &FileContent{
			FileID:      fileID,
			Content:     content,
			ContentType: uploadFile.MimeType,
			Size:        len(content),
			FromCache:   true,
		}, nil
	}

	{
		cacheReason := "no_cached_content"
		if !ce.config.CacheEnabled {
			cacheReason = "cache_disabled"
		}
		logFields := []interface{}{
			"file_id", fileID,
			"reason", cacheReason,
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Info("No cached content found, performing real-time extraction", logFields...)
	}

	modelUploadFile := &model.UploadFile{
		ID:             uploadFile.ID,
		OrganizationID: uploadFile.TenantID,
		StorageType:    uploadFile.StorageType,
		Key:            uploadFile.Key,
		Name:           uploadFile.Name,
		Size:           uploadFile.Size,
		Extension:      uploadFile.Extension,
		MimeType:       uploadFile.MimeType,
		CreatedByRole:  model.CreatedByRole(uploadFile.CreatedByRole),
		CreatedBy:      uploadFile.CreatedBy,
		CreatedAt:      uploadFile.CreatedAt,
		Used:           uploadFile.Used,
		UsedBy:         uploadFile.UsedBy,
		UsedAt:         uploadFile.UsedAt,
		Hash:           uploadFile.Hash,
	}

	startTime := time.Now()
	extractSetting := &extractor.ExtractSetting{
		DatasourceType:     extractor.DatasourceTypeFile,
		DocumentModel:      "text_model",
		ExtractionStrategy: ce.config.Strategy,
		// ExtractionFallbackEnabled nil → defaults to true, allowing automatic
		// degradation through the strategy chain on failure.
	}
	extractOutput, text, err := ce.extractProcessor.LoadFromUploadFileWithSetting(extractCtx, modelUploadFile, true, false, extractSetting)
	elapsedMs := time.Since(startTime).Milliseconds()

	if err != nil {
		logger.Error("File content extraction failed", err)
		return &FileContent{
			FileID: fileID,
			Error:  fmt.Errorf("content extraction failed: %w", err),
		}, err
	}

	select {
	case <-extractCtx.Done():
		logFields := []interface{}{
			"file_id", fileID,
			"extraction_time_ms", elapsedMs,
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Warn("File content extraction timed out", logFields...)
		return &FileContent{
			FileID: fileID,
			Error:  fmt.Errorf("content extraction timed out"),
		}, fmt.Errorf("content extraction timed out")
	default:
	}

	contentSize := len(text)
	truncated := false
	if contentSize > ce.config.MaxContentSize {
		text = text[:ce.config.MaxContentSize] + "\n... (content truncated due to size limit)"
		truncated = true
		logFields := []interface{}{
			"file_id", fileID,
			"original_size", contentSize,
			"max_size", ce.config.MaxContentSize,
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Warn("File content truncated due to size limit", logFields...)
	}

	docCount := 0
	if extractOutput != nil {
		docCount = len(extractOutput.Elements)
	}

	logFields := []interface{}{
		"file_id", fileID,
		"content_size", len(text),
		"extraction_time_ms", elapsedMs,
		"cached", false,
		"doc_count", docCount,
		"truncated", truncated,
	}
	if workflowRunID != "" {
		logFields = append(logFields, "workflow_run_id", workflowRunID)
	}
	logger.Info("File content extracted successfully", logFields...)

	if ce.config.CacheEnabled {
		cacheCtx := context.Background()
		if err := ce.updateCache(cacheCtx, fileID, text, workflowRunID); err != nil {
			cacheLogFields := []interface{}{
				"file_id", fileID,
				"error", err.Error(),
			}
			if workflowRunID != "" {
				cacheLogFields = append(cacheLogFields, "workflow_run_id", workflowRunID)
			}
			logger.Warn("Failed to update content cache", cacheLogFields...)
		}
	}

	return &FileContent{
		FileID:      fileID,
		Content:     text,
		ContentType: uploadFile.MimeType,
		Size:        len(text),
		FromCache:   false,
	}, nil
}

func (ce *contentExtractor) updateCache(ctx context.Context, fileID string, content string, workflowRunID string) error {
	if err := ce.fileService.UpdateContentText(ctx, fileID, content); err != nil {
		return fmt.Errorf("failed to update content text: %w", err)
	}

	logFields := []interface{}{
		"file_id", fileID,
		"content_size", len(content),
	}
	if workflowRunID != "" {
		logFields = append(logFields, "workflow_run_id", workflowRunID)
	}
	logger.Info("Content cache updated", logFields...)

	return nil
}

func (ce *contentExtractor) ExtractMultipleFiles(ctx context.Context, fileIDs []string, tenantID string) ([]*FileContent, error) {
	if len(fileIDs) == 0 {
		return []*FileContent{}, nil
	}

	workflowRunID := ce.getWorkflowRunIDFromContext(ctx)

	if !ce.config.Enabled {
		logFields := []interface{}{
			"file_count", len(fileIDs),
			"tenant_id", tenantID,
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Info("File content extraction is disabled by configuration, returning empty content for all files", logFields...)

		results := make([]*FileContent, len(fileIDs))
		for i, fileID := range fileIDs {
			results[i] = &FileContent{
				FileID:    fileID,
				Content:   "",
				FromCache: false,
			}
		}
		return results, nil
	}

	const maxConcurrent = 5
	semaphore := make(chan struct{}, maxConcurrent)

	var wg sync.WaitGroup
	results := make([]*FileContent, len(fileIDs))

	for i, fileID := range fileIDs {
		wg.Add(1)
		go func(index int, fid string) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			content, err := ce.ExtractFileContent(ctx, fid, tenantID)
			if err != nil {
				logFields := []interface{}{
					"file_id", fid,
					"tenant_id", tenantID,
					"error", err.Error(),
				}
				if workflowRunID != "" {
					logFields = append(logFields, "workflow_run_id", workflowRunID)
				}
				logger.Warn("Failed to extract content for file in batch", logFields...)
				results[index] = content
			} else {
				results[index] = content
			}
		}(i, fileID)
	}

	wg.Wait()

	successCount := 0
	for _, result := range results {
		if result != nil && result.Error == nil {
			successCount++
		}
	}

	logFields := []interface{}{
		"total_files", len(fileIDs),
		"successful", successCount,
		"failed", len(fileIDs) - successCount,
	}
	if workflowRunID != "" {
		logFields = append(logFields, "workflow_run_id", workflowRunID)
	}
	logger.Info("Multiple file content extraction completed", logFields...)

	return results, nil
}

func (ce *contentExtractor) ProcessFileVariable(ctx context.Context, variableName string, fileData map[string]interface{}, tenantID string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	workflowRunID := ce.getWorkflowRunIDFromContext(ctx)

	result[variableName] = fileData

	if !ce.config.Enabled {
		logFields := []interface{}{
			"variable_name", variableName,
			"tenant_id", tenantID,
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Info("File content extraction is disabled by configuration, returning metadata only", logFields...)

		// Return metadata with empty content
		result[variableName+"_content"] = ""
		return result, nil
	}

	fileID, ok := fileData["upload_file_id"].(string)
	if !ok || fileID == "" {
		if id, exists := fileData["id"].(string); exists && id != "" {
			fileID = id
		} else if relatedID, exists := fileData["related_id"].(string); exists && relatedID != "" {
			fileID = relatedID
		} else {
			logFields := []interface{}{
				"variable_name", variableName,
				"tenant_id", tenantID,
			}
			if workflowRunID != "" {
				logFields = append(logFields, "workflow_run_id", workflowRunID)
			}
			logger.Warn("File variable missing upload_file_id", logFields...)
			result[variableName+"_content"] = ""
			return result, fmt.Errorf("missing file ID in variable data")
		}
	}

	fileContent, err := ce.ExtractFileContent(ctx, fileID, tenantID)
	if err != nil {
		logFields := []interface{}{
			"variable_name", variableName,
			"file_id", fileID,
			"tenant_id", tenantID,
			"error", err.Error(),
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Warn("Failed to extract content for file variable", logFields...)
		result[variableName+"_content"] = fmt.Sprintf("[Content extraction failed: %s]", err.Error())
		return result, err
	}

	if fileContent.Error != nil {
		result[variableName+"_content"] = fmt.Sprintf("[Content extraction failed: %s]", fileContent.Error.Error())
	} else {
		result[variableName+"_content"] = fileContent.Content
	}

	logFields := []interface{}{
		"variable_name", variableName,
		"file_id", fileID,
		"content_size", len(fileContent.Content),
	}
	if workflowRunID != "" {
		logFields = append(logFields, "workflow_run_id", workflowRunID)
	}
	logger.Info("File variable processed successfully", logFields...)

	return result, nil
}

func (ce *contentExtractor) ProcessFileListVariable(ctx context.Context, variableName string, fileList []interface{}, tenantID string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	workflowRunID := ce.getWorkflowRunIDFromContext(ctx)

	result[variableName] = fileList

	if !ce.config.Enabled {
		logFields := []interface{}{
			"variable_name", variableName,
			"tenant_id", tenantID,
			"file_count", len(fileList),
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Info("File content extraction is disabled by configuration, returning metadata only", logFields...)

		// Return metadata with empty content
		result[variableName+"_content"] = ""
		return result, nil
	}

	if len(fileList) == 0 {
		result[variableName+"_content"] = ""
		return result, nil
	}

	fileIDs := make([]string, 0, len(fileList))
	for _, item := range fileList {
		if fileData, ok := item.(map[string]interface{}); ok {
			if fileID, ok := fileData["upload_file_id"].(string); ok && fileID != "" {
				fileIDs = append(fileIDs, fileID)
			} else if id, ok := fileData["id"].(string); ok && id != "" {
				fileIDs = append(fileIDs, id)
			} else if relatedID, ok := fileData["related_id"].(string); ok && relatedID != "" {
				fileIDs = append(fileIDs, relatedID)
			}
		}
	}

	if len(fileIDs) == 0 {
		logFields := []interface{}{
			"variable_name", variableName,
			"tenant_id", tenantID,
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Warn("File list variable has no valid file IDs", logFields...)
		result[variableName+"_content"] = ""
		return result, fmt.Errorf("no valid file IDs in file list")
	}

	fileContents, err := ce.ExtractMultipleFiles(ctx, fileIDs, tenantID)
	if err != nil {
		logFields := []interface{}{
			"variable_name", variableName,
			"file_count", len(fileIDs),
			"tenant_id", tenantID,
			"error", err.Error(),
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Warn("Failed to extract content for file list variable", logFields...)
	}

	var contentBuilder strings.Builder
	successCount := 0
	for i, fileContent := range fileContents {
		if fileContent != nil {
			if fileContent.Error != nil {
				contentBuilder.WriteString(fmt.Sprintf("[File %d: Content extraction failed: %s]\n\n", i+1, fileContent.Error.Error()))
			} else {
				contentBuilder.WriteString(fmt.Sprintf("=== File %d ===\n%s\n\n", i+1, fileContent.Content))
				successCount++
			}
		}
	}

	result[variableName+"_content"] = contentBuilder.String()

	logFields := []interface{}{
		"variable_name", variableName,
		"total_files", len(fileIDs),
		"successful", successCount,
		"content_size", contentBuilder.Len(),
	}
	if workflowRunID != "" {
		logFields = append(logFields, "workflow_run_id", workflowRunID)
	}
	logger.Info("File list variable processed", logFields...)

	return result, nil
}

// Global content extractor instance
var globalContentExtractor ContentExtractor

// InitGlobalContentExtractor initializes the global content extractor instance.
//
// This function should be called during application startup to initialize the
// global ContentExtractor that will be used by workflow nodes.
//
// Parameters:
//   - fileService: Service for file storage and retrieval operations
//   - extractProcessor: Processor for extracting text from various file formats
//
// Usage:
//
//	fileService := file_service.NewFileService(...)
//	extractProcessor := extractor.NewExtractProcessor(...)
//	file.InitGlobalContentExtractor(fileService, extractProcessor)
func InitGlobalContentExtractor(fileService interfaces.FileService, extractProcessor *extractor.ExtractProcessor) {
	config := GetContentExtractorConfig()
	globalContentExtractor = NewContentExtractor(fileService, extractProcessor, config)
	logger.Info("Global content extractor initialized",
		"enabled", config.Enabled,
		"max_content_size", config.MaxContentSize,
		"extraction_timeout", config.ExtractionTimeout,
		"cache_enabled", config.CacheEnabled,
	)
}

// GetGlobalContentExtractor returns the global content extractor instance.
//
// This function returns the global ContentExtractor that was initialized during
// application startup. If the global instance is not initialized, it returns nil.
//
// Returns:
//   - ContentExtractor: The global content extractor instance, or nil if not initialized
//
// Usage:
//
//	extractor := file.GetGlobalContentExtractor()
//	if extractor == nil {
//	    return fmt.Errorf("content extractor not initialized")
//	}
//	content, err := extractor.ExtractFileContent(ctx, fileID, tenantID)
func GetGlobalContentExtractor() ContentExtractor {
	return globalContentExtractor
}

// isImageExtension checks if a file extension is an image type
func isImageExtension(ext string) bool {
	imageExts := []string{"jpg", "jpeg", "png", "gif", "webp", "svg", "bmp", "tiff", "tif", "ico", "heic", "heif"}
	for _, ie := range imageExts {
		if ext == ie {
			return true
		}
	}
	return false
}

// isVideoExtension checks if a file extension is a video type
func isVideoExtension(ext string) bool {
	videoExts := []string{"mp4", "mov", "avi", "mkv", "flv", "wmv", "webm", "m4v", "3gp"}
	for _, ve := range videoExts {
		if ext == ve {
			return true
		}
	}
	return false
}

// isAudioExtension checks if a file extension is an audio type
func isAudioExtension(ext string) bool {
	audioExts := []string{"mp3", "wav", "flac", "aac", "ogg", "wma", "m4a", "opus"}
	for _, ae := range audioExts {
		if ext == ae {
			return true
		}
	}
	return false
}
