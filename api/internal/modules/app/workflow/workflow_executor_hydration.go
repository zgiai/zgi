package workflow

import (
	"context"

	"github.com/zgiai/zgi/api/pkg/logger"
)

// HydrateInputs recursively traverses inputs and fills in file metadata from upload_files table
// KEY: File Metadata Hydration - Recursively populates file attributes from DB
func (e *WorkflowExecutor) HydrateInputs(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Deep copy inputs to avoid modifying the original map if something goes wrong (though we modify in place for efficiency here as we own the map)
	// For simplicity, we just traverse and modify.

	// Collect all file IDs that need to be fetched to potentially batch fetch (though current FileService might not support batch)
	// For now, we fetch one by one as we traverse.

	newInputs := make(map[string]interface{})
	for k, v := range inputs {
		newInputs[k] = e.hydrateValue(ctx, v)
	}

	return newInputs, nil
}

// KEY: Recursive Input Traversal
func (e *WorkflowExecutor) hydrateValue(ctx context.Context, value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		// Check if it's a file structure
		if uploadFileID, ok := v["upload_file_id"].(string); ok && uploadFileID != "" {
			// It's a file! Fetch details.
			fileInfo, err := e.fileService.GetFileByID(ctx, uploadFileID)

			if err != nil {
				logger.Warn("Failed to fetch file info during hydration", "upload_file_id", uploadFileID, "error", err)
				return v // Return original if fetch fails
			}

			// Create a new map to hold hydrated values
			hydratedMap := make(map[string]interface{})
			for mk, mv := range v {
				hydratedMap[mk] = mv
			}

			// Populate fields if they don't exist or are empty
			if isEmptyHydratedValue(hydratedMap["name"]) {
				hydratedMap["name"] = fileInfo.Name
			}
			if isEmptyHydratedValue(hydratedMap["filename"]) {
				hydratedMap["filename"] = fileInfo.Name
			}
			if isEmptyHydratedValue(hydratedMap["size"]) {
				hydratedMap["size"] = fileInfo.Size
			}
			if isEmptyHydratedValue(hydratedMap["extension"]) {
				hydratedMap["extension"] = fileInfo.Extension
			}
			if isEmptyHydratedValue(hydratedMap["mime_type"]) {
				hydratedMap["mime_type"] = fileInfo.MimeType
			}
			if isEmptyHydratedValue(hydratedMap["type"]) {
				hydratedMap["type"] = "document" // Default to document or derive
			}

			// Determine URL
			if isEmptyHydratedValue(hydratedMap["url"]) {
				hydratedMap["url"] = fileInfo.SourceURL
			}
			if isEmptyHydratedValue(hydratedMap["remote_url"]) {
				hydratedMap["remote_url"] = fileInfo.SourceURL
			}

			// Set transfer_method if not present
			if _, ok := hydratedMap["transfer_method"]; !ok {
				hydratedMap["transfer_method"] = "local_file"
			}

			logger.Debug("Hydrated file input", "upload_file_id", uploadFileID, "name", fileInfo.Name)
			return hydratedMap
		}

		// Not a file, recurse into map
		newMap := make(map[string]interface{})
		for mk, mv := range v {
			newMap[mk] = e.hydrateValue(ctx, mv)
		}
		return newMap

	case []interface{}:
		// Recurse into slice
		newSlice := make([]interface{}, len(v))
		for i, item := range v {
			newSlice[i] = e.hydrateValue(ctx, item)
		}
		return newSlice

	default:
		return v
	}
}

func isEmptyHydratedValue(value interface{}) bool {
	if value == nil {
		return true
	}

	if str, ok := value.(string); ok {
		return str == ""
	}

	return false
}
