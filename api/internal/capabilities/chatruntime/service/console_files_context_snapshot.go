package service

import "strings"

const (
	consoleFilesContextSnapshotKey     = "console_files_context_snapshot"
	consoleFilesContextSnapshotSchema  = "zgi.aichat.console_files_context_snapshot.v1"
	consoleFilesContextSnapshotMaxFile = 20
)

var consoleFilesPageSnapshotMetadataKeys = []string{
	"context_ready",
	"files_query_status",
	"files_query_settled",
	"total_file_count",
	"total_pages",
	"current_page",
	"page_size",
	"visible_range_start",
	"visible_range_end",
	"more_pages_available",
	"visible_file_count",
	"selected_file_count",
	"selected_visible_file_count",
	"omitted_context_file_count",
	"context_visible_limit",
	"ordinal_scope",
	"visible_order_basis",
	"sort",
	"sort_key",
	"sort_direction",
	"category",
	"folder_name",
	"search",
	"extension_filter",
	"workspace_id",
	"workspace_name",
	"organization_mode",
}

func consoleFilesContextSnapshot(parts *chatRequestParts) map[string]interface{} {
	if parts == nil || !isConsoleFilesContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		return nil
	}
	files := consoleFilesPromptVisibleFiles(parts)
	if len(files) == 0 {
		return nil
	}
	if len(files) > consoleFilesContextSnapshotMaxFile {
		files = files[:consoleFilesContextSnapshotMaxFile]
	}

	capabilities := consoleFilesContextSnapshotCapabilities(parts)
	if len(capabilities) == 0 {
		return nil
	}

	snapshot := map[string]interface{}{
		"schema":        consoleFilesContextSnapshotSchema,
		"page":          "console.files",
		"route":         "/console/files",
		"capabilities":  capabilities,
		"visible_files": copyMapSlice(files),
	}
	for key, value := range consoleFilesPageSnapshotMetadata(parts) {
		snapshot[key] = value
	}
	if _, ok := snapshot["visible_file_count"]; !ok {
		snapshot["visible_file_count"] = len(files)
	}
	return snapshot
}

func consoleFilesPageSnapshotMetadata(parts *chatRequestParts) map[string]interface{} {
	if parts == nil {
		return nil
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		metadata := consoleFilesPageResourceMetadata(source)
		if len(metadata) > 0 {
			return metadata
		}
	}
	return nil
}

func consoleFilesPageResourceMetadata(source map[string]interface{}) map[string]interface{} {
	if len(source) == 0 {
		return nil
	}
	metadata := make(map[string]interface{}, len(consoleFilesPageSnapshotMetadataKeys))
	for _, resource := range mapSliceFromAny(source["resources"]) {
		if !isConsoleFilesPageResource(resource) {
			continue
		}
		rawMetadata := mapFromOperationContext(resource["metadata"])
		if len(rawMetadata) == 0 {
			continue
		}
		for _, key := range consoleFilesPageSnapshotMetadataKeys {
			if _, exists := metadata[key]; exists {
				continue
			}
			value, ok := rawMetadata[key]
			if !ok {
				continue
			}
			if sanitized, ok := sanitizedOperationScalar(value); ok {
				metadata[key] = sanitized
			}
		}
	}
	if len(metadata) > 0 {
		return metadata
	}
	return nil
}

func consoleFilesContextSnapshotCapabilities(parts *chatRequestParts) []interface{} {
	if parts == nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := []interface{}{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, map[string]interface{}{"id": id})
	}

	if hasConsoleFilesReadCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		add("file.list_visible")
		add("file.read")
	}
	if hasConsoleFilesCapability(parts.RuntimeContext, consoleFilesDeleteCapabilityPattern, parts.RawOperationContext, parts.OperationContext) {
		add("file.delete")
	}
	if hasConsoleFilesCreateCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		add("file.create")
	}
	return out
}

func restoreConsoleFilesContextFromMetadata(parts *chatRequestParts, metadata map[string]interface{}, event map[string]interface{}) {
	if parts == nil {
		return
	}
	if isConsoleFilesContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) &&
		len(consoleFilesPromptVisibleFiles(parts)) > 0 {
		return
	}

	snapshot := mapFromOperationContext(metadata[consoleFilesContextSnapshotKey])
	if len(snapshot) == 0 {
		snapshot = consoleFilesContextSnapshotFromApprovalEvent(event)
	}
	operationContext := consoleFilesOperationContextFromSnapshot(snapshot)
	if operationContext == nil {
		return
	}

	parts.RuntimeContext = "Restored Console Files page context from the original assistant turn."
	parts.RawOperationContext = copyStringAnyMap(operationContext)
	normalized, ledger := normalizeOperationContext(operationContext)
	parts.OperationContext = normalized
	parts.OperationLedger = ledger
}

func consoleFilesOperationContextFromSnapshot(snapshot map[string]interface{}) map[string]interface{} {
	if len(snapshot) == 0 || !strings.EqualFold(strings.TrimSpace(stringMetadataValue(snapshot["page"])), "console.files") {
		return nil
	}
	files := mapSliceFromAny(snapshot["visible_files"])
	if len(files) == 0 {
		return nil
	}

	pageMetadata := map[string]interface{}{
		"page":          "console.files",
		"route":         "/console/files",
		"resource_kind": "page",
	}
	for _, key := range consoleFilesPageSnapshotMetadataKeys {
		if value, ok := snapshot[key]; ok && value != nil {
			pageMetadata[key] = value
		}
	}

	resources := make([]interface{}, 0, len(files)+1)
	resources = append(resources, map[string]interface{}{
		"resource_id":   "console.files",
		"resource_type": "page",
		"type":          "page",
		"title":         "console.files",
		"href":          "/console/files",
		"metadata":      pageMetadata,
	})
	for _, file := range files {
		fileID := strings.TrimSpace(firstNonEmptyString(file["file_id"], file["id"], file["resource_id"]))
		name := strings.TrimSpace(firstNonEmptyString(file["name"], file["title"], file["filename"], file["file_name"]))
		if fileID == "" || name == "" {
			continue
		}
		metadata := map[string]interface{}{
			"resource_kind": "file",
			"file_id":       fileID,
			"name":          name,
		}
		for _, key := range []string{"visible_index", "file_type_rank", "extension_rank", "extension", "mime_type", "file_type", "workspace_id", "selected"} {
			if value, ok := file[key]; ok && value != nil {
				metadata[key] = value
			}
		}
		resources = append(resources, map[string]interface{}{
			"resource_id":   fileID,
			"resource_type": "file",
			"type":          "file",
			"title":         name,
			"source":        "Files page",
			"status":        "available",
			"metadata":      metadata,
		})
	}
	if len(resources) <= 1 {
		return nil
	}

	capabilities := mapSliceFromAny(snapshot["capabilities"])
	if len(capabilities) == 0 {
		capabilities = []map[string]interface{}{
			{"id": "file.list_visible"},
			{"id": "file.read"},
			{"id": "file.delete"},
		}
	}
	capabilityItems := make([]interface{}, 0, len(capabilities))
	for _, capability := range capabilities {
		id := strings.TrimSpace(firstNonEmptyString(capability["id"], capability["tool_id"], capability["capability_id"]))
		if id == "" {
			continue
		}
		capabilityItems = append(capabilityItems, map[string]interface{}{"id": id})
	}
	if len(capabilityItems) == 0 {
		return nil
	}

	return map[string]interface{}{
		"schema":       "zgi.aichat.operation_context.v1",
		"version":      1,
		"resources":    resources,
		"capabilities": capabilityItems,
		"summary": map[string]interface{}{
			"resource_count":   len(resources),
			"capability_count": len(capabilityItems),
		},
	}
}

func consoleFilesContextSnapshotFromApprovalEvent(event map[string]interface{}) map[string]interface{} {
	approvalEvent := toolGovernanceApprovalEventFromEvent(event)
	if len(approvalEvent) == 0 {
		return nil
	}
	toolID := strings.TrimSpace(firstNonEmptyString(approvalEvent["tool_id"], event["tool_id"]))
	if toolID != "file.delete" {
		return nil
	}
	assets := mapSliceFromAny(approvalEvent["assets"])
	if len(assets) == 0 {
		return nil
	}
	files := make([]map[string]interface{}, 0, len(assets))
	for index, asset := range assets {
		fileID := strings.TrimSpace(stringFromAny(asset["id"]))
		name := strings.TrimSpace(firstNonEmptyString(asset["name"], asset["title"], asset["filename"], asset["file_name"]))
		if fileID == "" || name == "" {
			continue
		}
		file := map[string]interface{}{
			"visible_index": index + 1,
			"file_id":       fileID,
			"name":          name,
		}
		if workspaceID := strings.TrimSpace(stringFromAny(asset["workspace_id"])); workspaceID != "" {
			file["workspace_id"] = workspaceID
		}
		if metadata := mapFromOperationContext(asset["metadata"]); metadata != nil {
			for _, key := range []string{"extension", "mime_type", "file_type"} {
				if value, ok := metadata[key]; ok && value != nil {
					file[key] = value
				}
			}
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		return nil
	}
	return map[string]interface{}{
		"schema":             "zgi.aichat.console_files_context_snapshot.approval_fallback.v1",
		"page":               "console.files",
		"route":              "/console/files",
		"visible_file_count": len(files),
		"capabilities": []interface{}{
			map[string]interface{}{"id": "file.list_visible"},
			map[string]interface{}{"id": "file.read"},
			map[string]interface{}{"id": "file.delete"},
		},
		"visible_files": files,
	}
}

func copyMapSlice(input []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(input))
	for _, item := range input {
		out = append(out, copyStringAnyMap(item))
	}
	return out
}
