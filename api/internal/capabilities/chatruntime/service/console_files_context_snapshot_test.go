package service

import "testing"

func TestConsoleFilesContextSnapshotStoredOutsideOperationContextMetadata(t *testing.T) {
	parts := consoleFilesSnapshotTestParts("delete the first file", []consoleFilesTestFile{
		{
			ID:          "file-1",
			Name:        "report.txt",
			Extension:   "txt",
			MimeType:    "text/plain",
			FileType:    "document",
			WorkspaceID: "workspace-1",
			Selected:    true,
		},
	})

	metadata := streamingMessageMetadata(parts)
	if _, ok := metadata["operation_context"]; ok {
		t.Fatalf("operation_context leaked into message metadata: %#v", metadata["operation_context"])
	}
	snapshot := mapFromOperationContext(metadata[consoleFilesContextSnapshotKey])
	if len(snapshot) == 0 {
		t.Fatalf("missing console files context snapshot in metadata: %#v", metadata)
	}
	if got := stringMetadataValue(snapshot["schema"]); got != consoleFilesContextSnapshotSchema {
		t.Fatalf("snapshot schema = %q, want %q", got, consoleFilesContextSnapshotSchema)
	}
	if got := stringMetadataValue(snapshot["page"]); got != "console.files" {
		t.Fatalf("snapshot page = %q, want console.files", got)
	}
	files := mapSliceFromAny(snapshot["visible_files"])
	if len(files) != 1 {
		t.Fatalf("snapshot visible_files length = %d, want 1: %#v", len(files), snapshot["visible_files"])
	}
	if files[0]["file_id"] != "file-1" || files[0]["name"] != "report.txt" {
		t.Fatalf("snapshot file = %#v, want file-1/report.txt", files[0])
	}
	if !snapshotHasCapability(snapshot, "file.list_visible") ||
		!snapshotHasCapability(snapshot, "file.read") ||
		!snapshotHasCapability(snapshot, "file.delete") {
		t.Fatalf("snapshot capabilities = %#v, want list/read/delete", snapshot["capabilities"])
	}
}

func TestConsoleFilesContextSnapshotPreservesCreateCapability(t *testing.T) {
	parts := consoleFilesSnapshotTestParts("create a managed text file", []consoleFilesTestFile{
		{ID: "file-1", Name: "report.txt", Extension: "txt", MimeType: "text/plain"},
	})
	parts.RuntimeContext = parts.RuntimeContext + ",file.create"
	for _, context := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		capabilities := append(mapSliceFromAny(context["capabilities"]), map[string]interface{}{"id": "file.create"})
		capabilityItems := make([]interface{}, 0, len(capabilities))
		for _, capability := range capabilities {
			capabilityItems = append(capabilityItems, capability)
		}
		context["capabilities"] = capabilityItems
	}

	snapshot := consoleFilesContextSnapshot(parts)
	if len(snapshot) == 0 {
		t.Fatal("consoleFilesContextSnapshot() returned empty snapshot")
	}
	if !snapshotHasCapability(snapshot, "file.create") {
		t.Fatalf("snapshot capabilities = %#v, want file.create", snapshot["capabilities"])
	}
}

func TestConsoleFilesContextSnapshotPreservesPageCounts(t *testing.T) {
	parts := consoleFilesSnapshotTestParts("show me the current files page count", []consoleFilesTestFile{
		{ID: "file-1", Name: "report.txt", Extension: "txt", MimeType: "text/plain"},
		{ID: "file-2", Name: "invoice.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	})
	setConsoleFilesPageTestMetadata(parts, map[string]interface{}{
		"context_ready":        true,
		"files_query_status":   "success",
		"total_file_count":     86,
		"current_page":         1,
		"total_pages":          9,
		"page_size":            10,
		"visible_range_start":  1,
		"visible_range_end":    10,
		"visible_file_count":   2,
		"selected_file_count":  0,
		"sort_key":             "created_at",
		"sort_direction":       "desc",
		"unsupported_metadata": []string{"ignored"},
	})
	prependSparseConsoleFilesPageTestResource(parts)

	snapshot := consoleFilesContextSnapshot(parts)
	if len(snapshot) == 0 {
		t.Fatal("consoleFilesContextSnapshot() returned empty snapshot")
	}
	for key, want := range map[string]int{
		"total_file_count":    86,
		"current_page":        1,
		"total_pages":         9,
		"page_size":           10,
		"visible_range_start": 1,
		"visible_range_end":   10,
		"visible_file_count":  2,
		"selected_file_count": 0,
	} {
		if got := intValueFromAny(snapshot[key]); got != want {
			t.Fatalf("snapshot[%s] = %#v (%d), want %d", key, snapshot[key], got, want)
		}
	}
	if got := stringMetadataValue(snapshot["files_query_status"]); got != "success" {
		t.Fatalf("snapshot files_query_status = %q, want success", got)
	}
	if _, ok := snapshot["unsupported_metadata"]; ok {
		t.Fatalf("snapshot copied unsupported metadata: %#v", snapshot["unsupported_metadata"])
	}
}

func TestRestoreConsoleFilesContextFromSnapshotMetadata(t *testing.T) {
	original := consoleFilesSnapshotTestParts("delete the first file", []consoleFilesTestFile{
		{
			ID:          "file-1",
			Name:        "report.txt",
			Extension:   "txt",
			MimeType:    "text/plain",
			FileType:    "document",
			WorkspaceID: "workspace-1",
		},
	})
	metadata := streamingMessageMetadata(original)

	parts := &chatRequestParts{Query: "delete the first file"}
	restoreConsoleFilesContextFromMetadata(parts, metadata, nil)

	if !isConsoleFilesContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		t.Fatalf("restored parts are not console files context: %#v", parts.OperationContext)
	}
	if !hasConsoleFilesReadCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		t.Fatalf("restored parts missing read capability: %#v", parts.OperationContext)
	}
	if !hasConsoleFilesCapability(parts.RuntimeContext, consoleFilesDeleteCapabilityPattern, parts.RawOperationContext, parts.OperationContext) {
		t.Fatalf("restored parts missing delete capability: %#v", parts.OperationContext)
	}
	files := consoleFilesPromptVisibleFiles(parts)
	if len(files) != 1 {
		t.Fatalf("restored visible files length = %d, want 1: %#v", len(files), files)
	}
	if files[0]["file_id"] != "file-1" || files[0]["name"] != "report.txt" {
		t.Fatalf("restored visible file = %#v, want file-1/report.txt", files[0])
	}
}

func TestRestoreConsoleFilesContextPreservesPageCounts(t *testing.T) {
	original := consoleFilesSnapshotTestParts("show me the file count", []consoleFilesTestFile{
		{ID: "file-1", Name: "report.txt", Extension: "txt", MimeType: "text/plain"},
	})
	setConsoleFilesPageTestMetadata(original, map[string]interface{}{
		"context_ready":       true,
		"total_file_count":    86,
		"current_page":        1,
		"total_pages":         9,
		"visible_file_count":  1,
		"files_query_status":  "success",
		"visible_order_basis": "current_visible_page_order",
	})
	metadata := streamingMessageMetadata(original)

	parts := &chatRequestParts{Query: "show me the file count"}
	restoreConsoleFilesContextFromMetadata(parts, metadata, nil)

	pageMetadata := restoredConsoleFilesPageMetadataForTest(parts.RawOperationContext)
	if len(pageMetadata) == 0 {
		t.Fatalf("restored page metadata missing in raw operation context: %#v", parts.RawOperationContext)
	}
	if got := intValueFromAny(pageMetadata["total_file_count"]); got != 86 {
		t.Fatalf("restored total_file_count = %#v (%d), want 86", pageMetadata["total_file_count"], got)
	}
	if got := intValueFromAny(pageMetadata["total_pages"]); got != 9 {
		t.Fatalf("restored total_pages = %#v (%d), want 9", pageMetadata["total_pages"], got)
	}
	if got := stringMetadataValue(pageMetadata["files_query_status"]); got != "success" {
		t.Fatalf("restored files_query_status = %q, want success", got)
	}
}

func TestRestoreConsoleFilesContextFromApprovalEventFallback(t *testing.T) {
	parts := &chatRequestParts{Query: "delete report.txt"}
	event := map[string]interface{}{
		"approval_event": map[string]interface{}{
			"tool_id": "file.delete",
			"assets": []interface{}{
				map[string]interface{}{
					"id":           "file-1",
					"name":         "report.txt",
					"workspace_id": "workspace-1",
					"metadata": map[string]interface{}{
						"extension": "txt",
						"mime_type": "text/plain",
						"file_type": "document",
					},
				},
			},
		},
	}

	restoreConsoleFilesContextFromMetadata(parts, nil, event)

	if !isConsoleFilesContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		t.Fatalf("fallback restored parts are not console files context: %#v", parts.OperationContext)
	}
	if !hasConsoleFilesCapability(parts.RuntimeContext, consoleFilesDeleteCapabilityPattern, parts.RawOperationContext, parts.OperationContext) {
		t.Fatalf("fallback restored parts missing delete capability: %#v", parts.OperationContext)
	}
	files := consoleFilesPromptVisibleFiles(parts)
	if len(files) != 1 {
		t.Fatalf("fallback visible files length = %d, want 1: %#v", len(files), files)
	}
	if files[0]["file_id"] != "file-1" || files[0]["name"] != "report.txt" {
		t.Fatalf("fallback visible file = %#v, want file-1/report.txt", files[0])
	}
}

func consoleFilesSnapshotTestParts(query string, files []consoleFilesTestFile) *chatRequestParts {
	parts := consoleFilesSemanticTestParts(query, files)
	operationContext := copyStringAnyMap(parts.RawOperationContext)
	capabilities := []interface{}{
		map[string]interface{}{"id": "file.list_visible"},
		map[string]interface{}{"id": "file.read"},
		map[string]interface{}{"id": "file.delete"},
	}
	operationContext["capabilities"] = capabilities
	if resources := mapSliceFromAny(operationContext["resources"]); len(resources) > 0 {
		updated := make([]interface{}, 0, len(resources))
		for _, resource := range resources {
			resource = copyStringAnyMap(resource)
			resource["capability_ids"] = []interface{}{"file.list_visible", "file.read", "file.delete"}
			updated = append(updated, resource)
		}
		operationContext["resources"] = updated
	}
	parts.RuntimeContext = "route=/console/files capabilities=file.list_visible,file.read,file.delete"
	parts.RawOperationContext = operationContext
	parts.OperationContext = operationContext
	return parts
}

func setConsoleFilesPageTestMetadata(parts *chatRequestParts, additions map[string]interface{}) {
	if parts == nil {
		return
	}
	for _, context := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		for _, resource := range mapSliceFromAny(context["resources"]) {
			if !isConsoleFilesPageResource(resource) {
				continue
			}
			metadata := mapFromOperationContext(resource["metadata"])
			for key, value := range additions {
				metadata[key] = value
			}
		}
	}
}

func prependSparseConsoleFilesPageTestResource(parts *chatRequestParts) {
	if parts == nil {
		return
	}
	for _, context := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		resources := operationItemsFromValue(context["resources"])
		sparse := map[string]interface{}{
			"resource_type": "page",
			"resource_id":   "/console/files",
			"title":         "Files",
			"href":          "/console/files",
			"metadata": map[string]interface{}{
				"route": "/console/files",
			},
		}
		updated := make([]interface{}, 0, len(resources)+1)
		updated = append(updated, sparse)
		updated = append(updated, resources...)
		context["resources"] = updated
	}
}

func restoredConsoleFilesPageMetadataForTest(operationContext map[string]interface{}) map[string]interface{} {
	for _, resource := range mapSliceFromAny(operationContext["resources"]) {
		if isConsoleFilesPageResource(resource) {
			return mapFromOperationContext(resource["metadata"])
		}
	}
	return nil
}

func snapshotHasCapability(snapshot map[string]interface{}, capabilityID string) bool {
	for _, capability := range mapSliceFromAny(snapshot["capabilities"]) {
		if stringMetadataValue(capability["id"]) == capabilityID {
			return true
		}
	}
	return false
}
