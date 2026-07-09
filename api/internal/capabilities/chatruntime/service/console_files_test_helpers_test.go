package service

import "strings"

type consoleFilesTestFile struct {
	ID          string
	Name        string
	Extension   string
	MimeType    string
	FileType    string
	WorkspaceID string
	Selected    bool
}

func consoleFilesSemanticTestParts(query string, files []consoleFilesTestFile) *chatRequestParts {
	const fileReadCapabilityID = "file.read"

	resources := make([]interface{}, 0, len(files)+1)
	selectedIDs := make([]string, 0)
	for _, file := range files {
		if file.Selected {
			selectedIDs = append(selectedIDs, file.ID)
		}
	}
	resources = append(resources, map[string]interface{}{
		"resource_type": "page",
		"resource_id":   "console.files",
		"title":         "console.files",
		"href":          "/console/files",
		"capability_ids": []interface{}{
			fileReadCapabilityID,
		},
		"metadata": map[string]interface{}{
			"page":               "console.files",
			"route":              "/console/files",
			"selected_file_ids":  strings.Join(selectedIDs, ","),
			"visible_file_count": len(files),
		},
	})

	fileTypeRanks := map[string]int{}
	extensionRanks := map[string]int{}
	for index, file := range files {
		fileType := strings.TrimSpace(file.FileType)
		if fileType == "" {
			fileType = consoleFilesTestFileType(file.Extension)
		}
		extension := strings.TrimSpace(file.Extension)
		fileTypeRankKey := firstNonEmptyString(fileType, extension, "file")
		fileTypeRanks[fileTypeRankKey]++
		extensionRankKey := firstNonEmptyString(extension, fileType, "file")
		extensionRanks[extensionRankKey]++
		resources = append(resources, map[string]interface{}{
			"resource_type": "file",
			"resource_id":   file.ID,
			"title":         file.Name,
			"subtitle":      file.Extension,
			"href":          "/console/files",
			"source":        "Files page",
			"status":        "available",
			"capability_ids": []interface{}{
				fileReadCapabilityID,
			},
			"metadata": map[string]interface{}{
				"page":           "console.files",
				"file_id":        file.ID,
				"selected":       file.Selected,
				"name":           file.Name,
				"visible_index":  index + 1,
				"extension":      file.Extension,
				"mime_type":      file.MimeType,
				"file_type":      fileType,
				"file_type_rank": fileTypeRanks[fileTypeRankKey],
				"extension_rank": extensionRanks[extensionRankKey],
				"workspace_id":   file.WorkspaceID,
			},
		})
	}
	operationContext := map[string]interface{}{
		"schema":    "zgi.aichat.operation_context.v1",
		"version":   1,
		"resources": resources,
		"capabilities": []interface{}{
			map[string]interface{}{
				"id":            fileReadCapabilityID,
				"title":         "Read file",
				"resource_id":   "console.files",
				"resource_type": "page",
				"risk":          "low",
				"status":        "available",
			},
		},
		"risk_summary": map[string]interface{}{
			"level":                 "low",
			"requires_confirmation": false,
		},
	}
	return &chatRequestParts{
		Query:               query,
		RuntimeContext:      "route=/console/files capabilities=file.read",
		RawOperationContext: operationContext,
		OperationContext:    operationContext,
	}
}

func consoleFilesCreateCapabilityTestParts(query string) *chatRequestParts {
	operationContext := map[string]interface{}{
		"schema":  "zgi.aichat.operation_context.v1",
		"version": 1,
		"resources": []interface{}{
			map[string]interface{}{
				"resource_type": "page",
				"resource_id":   "console.files",
				"title":         "console.files",
				"href":          "/console/files",
				"capability_ids": []interface{}{
					"file.create",
				},
				"metadata": map[string]interface{}{
					"page":  "console.files",
					"route": "/console/files",
				},
			},
		},
		"capabilities": []interface{}{
			map[string]interface{}{
				"id":            "file.create",
				"title":         "Create file",
				"resource_id":   "console.files",
				"resource_type": "page",
				"risk":          "medium",
				"status":        "available",
			},
		},
		"risk_summary": map[string]interface{}{
			"level":                 "medium",
			"requires_confirmation": true,
		},
	}
	return &chatRequestParts{
		Query:               query,
		RuntimeContext:      "route=/console/files capabilities=file.create",
		RawOperationContext: operationContext,
		OperationContext:    operationContext,
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent:     "save_generated_file_to_file_management",
			Confidence: 0.91,
		},
	}
}

func consoleFilesTestFileType(extension string) string {
	switch strings.ToLower(strings.TrimSpace(extension)) {
	case "xls", "xlsx", "xlsm", "xlsb":
		return "excel"
	case "pdf":
		return "pdf"
	case "csv":
		return "csv"
	case "png", "jpg", "jpeg", "gif", "webp", "bmp", "svg":
		return "image"
	default:
		return "document"
	}
}
