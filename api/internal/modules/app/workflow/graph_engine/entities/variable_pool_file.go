package entities

import workflowfile "github.com/zgiai/ginext/internal/modules/app/workflow/file"

func (vp *VariablePool) GetFile(selector []string) *FileSegment {
	variable := vp.Get(selector)
	if variable != nil {
		if fileSegment, ok := variable.(*variableWrapper); ok {
			if fs, isFile := fileSegment.segment.(*FileSegment); isFile {
				return fs
			}
		}
	}
	return nil
}

func (vp *VariablePool) mapToFile(data map[string]interface{}) *File {
	file := &File{}

	// Extract ID (priority: upload_file_id > id > related_id)
	if id, ok := data["upload_file_id"].(string); ok && id != "" {
		file.ID = id
	} else if id, ok := data["id"].(string); ok && id != "" {
		file.ID = id
	} else if id, ok := data["related_id"].(string); ok && id != "" {
		file.ID = id
	}

	// Extract transfer_method
	if transferMethod, ok := data["transfer_method"].(string); ok && transferMethod != "" {
		file.TransferMethod = transferMethod
	} else {
		file.TransferMethod = "local_file"
	}

	// Extract remote_url
	if remoteURL, ok := data["remote_url"].(string); ok {
		file.RemoteURL = remoteURL
	} else if url, ok := data["url"].(string); ok {
		file.RemoteURL = url
	}

	// Extract filename (priority: filename > name)
	if filename, ok := data["filename"].(string); ok && filename != "" {
		file.Filename = filename
	} else if name, ok := data["name"].(string); ok && name != "" {
		file.Filename = name
	}

	// Extract extension (priority: extension > ext)
	if extension, ok := data["extension"].(string); ok && extension != "" {
		file.Extension = extension
	} else if ext, ok := data["ext"].(string); ok && ext != "" {
		file.Extension = ext
	}

	// Extract mime_type (priority: mime_type > content_type)
	if mimeType, ok := data["mime_type"].(string); ok && mimeType != "" {
		file.MimeType = mimeType
	} else if contentType, ok := data["content_type"].(string); ok && contentType != "" {
		file.MimeType = contentType
	}

	rawType, _ := data["type"].(string)
	file.Type = workflowfile.NormalizeFileType(rawType, file.Extension, file.MimeType)

	// Extract size
	if size, ok := data["size"].(int64); ok {
		file.Size = size
	} else if size, ok := data["size"].(int); ok {
		file.Size = int64(size)
	} else if size, ok := data["size"].(float64); ok {
		file.Size = int64(size)
	}

	// Extract storage_key
	if storageKey, ok := data["storage_key"].(string); ok {
		file.StorageKey = storageKey
	}

	// Extract workspace_id, while still accepting legacy tenant_id as input.
	if workspaceID, ok := data["workspace_id"].(string); ok {
		file.WorkspaceID = workspaceID
	} else if tenantID, ok := data["tenant_id"].(string); ok {
		file.WorkspaceID = tenantID
	}

	return file
}

func (vp *VariablePool) workflowFileToFile(data *workflowfile.File) *File {
	if data == nil {
		return nil
	}

	file := &File{
		WorkspaceID:    data.TenantID,
		Type:           string(data.Type),
		TransferMethod: string(data.TransferMethod),
		Size:           data.Size,
	}

	if data.ID != nil {
		file.ID = *data.ID
	}
	if data.Filename != nil {
		file.Filename = *data.Filename
	}
	if data.Extension != nil {
		file.Extension = *data.Extension
	}
	if data.MimeType != nil {
		file.MimeType = *data.MimeType
	}

	// Prefer the already-signed URL when present so downstream rendering is stable.
	if data.URL != nil && *data.URL != "" {
		file.RemoteURL = *data.URL
	} else if data.RemoteURL != nil {
		file.RemoteURL = *data.RemoteURL
	}

	return file
}

func (vp *VariablePool) workflowFilesToFiles(data []*workflowfile.File) []*File {
	files := make([]*File, len(data))
	for i, item := range data {
		files[i] = vp.workflowFileToFile(item)
	}
	return files
}

// mapArrayToFiles converts an array of maps to File entities
func (vp *VariablePool) mapArrayToFiles(data []interface{}) []*File {
	files := make([]*File, 0, len(data))

	for _, item := range data {
		if itemMap, ok := item.(map[string]interface{}); ok {
			file := vp.mapToFile(itemMap)
			files = append(files, file)
		}
	}

	return files
}

// isFileStructure checks if a map represents a file structure
// A map is considered a file structure if it has any of the following:
// - type field with value "document", "image", "video", "audio", or "file"
// - upload_file_id field with non-empty string value
// - id field with non-empty string AND transfer_method field
// - related_id field with non-empty string value
func (vp *VariablePool) isFileStructure(data map[string]interface{}) bool {
	// Check for type field
	if typeVal, ok := data["type"]; ok {
		if typeStr, isString := typeVal.(string); isString && typeStr != "" {
			validTypes := []string{"document", "image", "video", "audio", "file"}
			for _, validType := range validTypes {
				if typeStr == validType {
					return true
				}
			}
		}
	}

	// Check for upload_file_id
	if uploadFileID, ok := data["upload_file_id"]; ok {
		if idStr, isString := uploadFileID.(string); isString && idStr != "" {
			return true
		}
	}

	// Check for id + transfer_method combination
	if id, ok := data["id"]; ok {
		if idStr, isString := id.(string); isString && idStr != "" {
			if _, hasTransferMethod := data["transfer_method"]; hasTransferMethod {
				return true
			}
		}
	}

	// Check for related_id
	if relatedID, ok := data["related_id"]; ok {
		if idStr, isString := relatedID.(string); isString && idStr != "" {
			return true
		}
	}

	return false
}

// isFileArray checks if an array contains file structures
// Returns true if the array is non-empty and all elements are file structures
func (vp *VariablePool) isFileArray(data []interface{}) bool {
	if len(data) == 0 {
		return false
	}

	for _, item := range data {
		if itemMap, ok := item.(map[string]interface{}); ok {
			if !vp.isFileStructure(itemMap) {
				return false
			}
		} else {
			return false
		}
	}

	return true
}
