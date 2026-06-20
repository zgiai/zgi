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

func snapshotHasCapability(snapshot map[string]interface{}, capabilityID string) bool {
	for _, capability := range mapSliceFromAny(snapshot["capabilities"]) {
		if stringMetadataValue(capability["id"]) == capabilityID {
			return true
		}
	}
	return false
}
