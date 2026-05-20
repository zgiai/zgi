package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/modules/app/workflow/file"
	"github.com/zgiai/ginext/pkg/logger"
)

func (h *WorkflowHandler) processAllFileInputs(ctx context.Context, inputs map[string]interface{}, workspaceID string, appID string) map[string]interface{} {
	if inputs == nil {
		return make(map[string]interface{})
	}

	logger.Info(fmt.Sprintf("[WorkflowHandler] Processing file inputs for app: %s", appID))

	processedInputs := make(map[string]interface{})

	for key, value := range inputs {
		// Skip system variables
		if strings.HasPrefix(key, "sys.") {
			processedInputs[key] = value
			continue
		}

		// Check if value is a file object (single file)
		if fileMap, ok := value.(map[string]interface{}); ok {
			if uploadFileID, exists := fileMap["upload_file_id"]; exists {
				if fileIDStr, ok := uploadFileID.(string); ok && fileIDStr != "" {
					logger.Info(fmt.Sprintf("[WorkflowHandler] Processing file input: %s, file_id: %s", key, fileIDStr))
					// Extract file content and replace the input
					processedInputs[key] = h.extractFileContent(ctx, fileMap, fileIDStr, workspaceID)
					continue
				}
			}
		}

		// Check if value is an array of file objects (file list)
		if fileList, ok := value.([]interface{}); ok {
			processedFiles := make([]interface{}, 0, len(fileList))
			hasFileObjects := false

			for _, item := range fileList {
				if fileMap, ok := item.(map[string]interface{}); ok {
					if uploadFileID, exists := fileMap["upload_file_id"]; exists {
						if fileIDStr, ok := uploadFileID.(string); ok && fileIDStr != "" {
							hasFileObjects = true
							logger.Info(fmt.Sprintf("[WorkflowHandler] Processing file in array: %s, file_id: %s", key, fileIDStr))
							processedFiles = append(processedFiles, h.extractFileContent(ctx, fileMap, fileIDStr, workspaceID))
							continue
						}
					}
				}
				// Not a file object, keep as-is
				processedFiles = append(processedFiles, item)
			}

			if hasFileObjects {
				processedInputs[key] = processedFiles
				continue
			}
		}

		// For #files# array, process each file
		if key == "#files#" {
			processedInputs[key] = h.processWorkflowFiles(ctx, value, workspaceID)
			continue
		}

		// Not a file input, keep as-is
		processedInputs[key] = value
	}

	logger.Info(fmt.Sprintf("[WorkflowHandler] File processing complete, processed %d inputs", len(processedInputs)))
	return processedInputs
}

func applyProcessedInputs(req *dto.DraftWorkflowRunRequest, processedInputs map[string]interface{}) {
	if req == nil {
		return
	}
	req.Inputs = processedInputs
}

func (h *WorkflowHandler) extractFileContent(ctx context.Context, fileMap map[string]interface{}, fileID string, workspaceID string) interface{} {
	// Get file metadata from database
	uploadFile, err := h.fileService.GetFileByID(ctx, fileID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get workflow file", "file_id", fileID, err)
		return fmt.Sprintf("[File ID: %s - Error: Unable to access file]", fileID)
	}

	fileType := ""
	if t, ok := fileMap["type"].(string); ok {
		fileType = t
	}

	extension := strings.ToLower(uploadFile.Extension)
	mimeType := strings.ToLower(uploadFile.MimeType)
	effectiveFileType := resolveEffectiveWorkflowFileType(fileType, extension, mimeType)

	if effectiveFileType == "video" || effectiveFileType == "audio" ||
		isVideoExtension(extension) || isAudioExtension(extension) {
		logger.Info(fmt.Sprintf("[WorkflowHandler] File %s is video/audio, returning metadata", fileID))
		result := make(map[string]interface{})
		for k, v := range fileMap {
			result[k] = v
		}
		result["_note"] = fmt.Sprintf("Video and audio files cannot be processed as text. File ID: %s", fileID)
		return result
	}

	if effectiveFileType == "document" || isTextExtension(extension) {
		// Return a hydrated file map instead of extracting text here.
		// The Start node's ContentExtractor will properly extract document content
		// and create both the metadata variable and the _content variable.
		logger.Info(fmt.Sprintf("[WorkflowHandler] File %s is document, creating File object for ContentExtractor (extension: %s)", fileID, extension))

		fileProcessor := file.NewFileProcessor(h.fileService)
		workflowFile, err := fileProcessor.ProcessFileForWorkflow(ctx, fileID, workspaceID)
		if err != nil {
			logger.ErrorContext(ctx, "failed to process document file for workflow", "file_id", fileID, "workspace_id", workspaceID, err)
			// Fallback: return hydrated file map so downstream can still attempt extraction
			result := make(map[string]interface{})
			for k, v := range fileMap {
				result[k] = v
			}
			result["name"] = uploadFile.Name
			result["extension"] = uploadFile.Extension
			result["mime_type"] = uploadFile.MimeType
			result["size"] = uploadFile.Size
			return result
		}

		return workflowFile.ToDict()
	}

	if effectiveFileType == "image" || isImageExtension(extension) {
		logger.Info(fmt.Sprintf("[WorkflowHandler] File %s is image, creating File object", fileID))

		fileProcessor := file.NewFileProcessor(h.fileService)
		workflowFile, err := fileProcessor.ProcessFileForWorkflow(ctx, fileID, workspaceID)
		if err != nil {
			logger.ErrorContext(ctx, "failed to process image file for workflow", "file_id", fileID, "workspace_id", workspaceID, err)
			return fmt.Sprintf("[File ID: %s - Error: Unable to process image file]", fileID)
		}

		return workflowFile.ToDict()
	}

	// Unknown file type: return hydrated file map to preserve file metadata for downstream processing
	logger.Info(fmt.Sprintf("[WorkflowHandler] Unknown file type for %s, creating File object", fileID))
	fileProcessor := file.NewFileProcessor(h.fileService)
	workflowFile, err := fileProcessor.ProcessFileForWorkflow(ctx, fileID, workspaceID)
	if err != nil {
		logger.WarnContext(ctx, "failed to process unknown-type file for workflow, returning original metadata", "file_id", fileID, "workspace_id", workspaceID, err)
		result := make(map[string]interface{})
		for k, v := range fileMap {
			result[k] = v
		}
		result["name"] = uploadFile.Name
		result["extension"] = uploadFile.Extension
		result["mime_type"] = uploadFile.MimeType
		result["size"] = uploadFile.Size
		return result
	}
	return workflowFile.ToDict()
}

func resolveEffectiveWorkflowFileType(declaredType, extension, mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/") || isImageExtension(extension):
		return "image"
	case strings.HasPrefix(mimeType, "audio/") || isAudioExtension(extension):
		return "audio"
	case strings.HasPrefix(mimeType, "video/") || isVideoExtension(extension):
		return "video"
	default:
		return declaredType
	}
}

func isVideoExtension(ext string) bool {
	videoExts := []string{"mp4", "mov", "avi", "mkv", "flv", "wmv", "webm", "m4v", "3gp"}
	for _, ve := range videoExts {
		if ext == ve {
			return true
		}
	}
	return false
}

func isAudioExtension(ext string) bool {
	audioExts := []string{"mp3", "wav", "flac", "aac", "ogg", "wma", "m4a", "opus"}
	for _, ae := range audioExts {
		if ext == ae {
			return true
		}
	}
	return false
}

func isImageExtension(ext string) bool {
	imageExts := []string{"jpg", "jpeg", "png", "gif", "webp", "svg", "bmp", "tiff", "tif", "ico"}
	for _, ie := range imageExts {
		if ext == ie {
			return true
		}
	}
	return false
}

func isTextExtension(ext string) bool {
	textExts := []string{"txt", "md", "markdown", "mdx", "html", "htm", "xml", "csv", "json", "yaml", "yml"}
	for _, te := range textExts {
		if ext == te {
			return true
		}
	}
	return false
}

func (h *WorkflowHandler) processWorkflowFiles(ctx context.Context, filesInput interface{}, workspaceID string) interface{} {
	if filesInput == nil {
		return nil
	}

	switch files := filesInput.(type) {
	case []interface{}:
		var processedFiles []interface{}
		for _, file := range files {
			if fileStr, ok := file.(string); ok {
				if processedFile := h.processFileID(ctx, fileStr, workspaceID); processedFile != nil {
					processedFiles = append(processedFiles, processedFile)
				}
			} else if fileMap, ok := file.(map[string]interface{}); ok {
				if fileID, exists := fileMap["id"]; exists {
					if fileIDStr, ok := fileID.(string); ok {
						if processedFile := h.processFileID(ctx, fileIDStr, workspaceID); processedFile != nil {
							processedFiles = append(processedFiles, processedFile)
						}
					}
				}
			}
		}
		return processedFiles
	case []string:
		var processedFiles []interface{}
		for _, fileID := range files {
			if processedFile := h.processFileID(ctx, fileID, workspaceID); processedFile != nil {
				processedFiles = append(processedFiles, processedFile)
			}
		}
		return processedFiles
	case string:
		return h.processFileID(ctx, files, workspaceID)
	default:
		return filesInput
	}
}

func (h *WorkflowHandler) processFileID(ctx context.Context, fileID string, workspaceID string) interface{} {
	if fileID == "" {
		return nil
	}

	fileProcessor := file.NewFileProcessor(h.fileService)
	workflowFile, err := fileProcessor.ProcessFileForWorkflow(ctx, fileID, workspaceID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to process file for workflow", "file_id", fileID, "workspace_id", workspaceID, err)
		return nil
	}

	return workflowFile.ToDict()
}
