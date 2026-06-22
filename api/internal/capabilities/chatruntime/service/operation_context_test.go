package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestOperationContextRecordedAsOperationLedger(t *testing.T) {
	secretMarker := "VERY_SENSITIVE_OPERATION_PROMPT"
	resources := make([]interface{}, 0, maxOperationLedgerResources+2)
	for i := 0; i < maxOperationLedgerResources+2; i++ {
		resources = append(resources, map[string]interface{}{
			"id":          "resource-id",
			"type":        "document",
			"name":        strings.Repeat("resource-name-", 20),
			"description": secretMarker,
			"prompt":      secretMarker,
			"content":     secretMarker,
		})
	}
	capabilities := make([]interface{}, 0, maxOperationLedgerCapabilities+1)
	for i := 0; i < maxOperationLedgerCapabilities+1; i++ {
		capabilities = append(capabilities, map[string]interface{}{
			"id":                "capability-id",
			"tool_name":         "query_database",
			"requires_approval": true,
			"description":       secretMarker,
			"prompt":            secretMarker,
		})
	}

	parts, err := normalizeChatRequest(runtimedto.ChatRequest{
		Query: "Summarize the selected resource.",
		Model: "test-model",
		OperationContext: map[string]interface{}{
			"resources":    resources,
			"capabilities": capabilities,
			"risk_summary": map[string]interface{}{
				"level":             "Medium",
				"requires_approval": true,
				"summary":           secretMarker,
				"categories":        []interface{}{"database_write", strings.Repeat("long-risk-category-", 10)},
			},
			"prompt": secretMarker,
		},
	})
	if err != nil {
		t.Fatalf("normalizeChatRequest() error = %v", err)
	}

	message := newStreamingMessage(uuid.New(), nil, parts)
	ledger, ok := message.Metadata["operation_ledger"].(map[string]interface{})
	if !ok {
		t.Fatalf("operation_ledger metadata = %#v, want map", message.Metadata["operation_ledger"])
	}
	if ledger["version"] != operationLedgerVersion {
		t.Fatalf("operation ledger version = %#v, want %q", ledger["version"], operationLedgerVersion)
	}
	if ledger["status"] != operationLedgerStatusObserved {
		t.Fatalf("operation ledger status = %#v, want %q", ledger["status"], operationLedgerStatusObserved)
	}
	if ledger["resource_count"] != maxOperationLedgerResources+2 {
		t.Fatalf("resource_count = %#v, want %d", ledger["resource_count"], maxOperationLedgerResources+2)
	}
	if ledger["capability_count"] != maxOperationLedgerCapabilities+1 {
		t.Fatalf("capability_count = %#v, want %d", ledger["capability_count"], maxOperationLedgerCapabilities+1)
	}

	ledgerResources, ok := ledger["resources"].([]map[string]interface{})
	if !ok {
		t.Fatalf("ledger resources = %#v, want resource summaries", ledger["resources"])
	}
	if len(ledgerResources) != maxOperationLedgerResources {
		t.Fatalf("ledger resources len = %d, want %d", len(ledgerResources), maxOperationLedgerResources)
	}
	if got := len([]rune(ledgerResources[0]["name"].(string))); got > maxOperationLedgerFieldRunes {
		t.Fatalf("resource name length = %d, want at most %d", got, maxOperationLedgerFieldRunes)
	}
	for _, forbidden := range []string{"description", "prompt", "content"} {
		if _, exists := ledgerResources[0][forbidden]; exists {
			t.Fatalf("resource summary contains forbidden key %q: %#v", forbidden, ledgerResources[0])
		}
	}

	ledgerCapabilities, ok := ledger["capabilities"].([]map[string]interface{})
	if !ok {
		t.Fatalf("ledger capabilities = %#v, want capability summaries", ledger["capabilities"])
	}
	if len(ledgerCapabilities) != maxOperationLedgerCapabilities {
		t.Fatalf("ledger capabilities len = %d, want %d", len(ledgerCapabilities), maxOperationLedgerCapabilities)
	}
	if ledgerCapabilities[0]["name"] != "query_database" {
		t.Fatalf("capability name = %#v, want query_database", ledgerCapabilities[0]["name"])
	}
	if _, exists := ledgerCapabilities[0]["prompt"]; exists {
		t.Fatalf("capability summary contains prompt: %#v", ledgerCapabilities[0])
	}

	riskSummary, ok := ledger["risk_summary"].(map[string]interface{})
	if !ok {
		t.Fatalf("risk_summary = %#v, want map", ledger["risk_summary"])
	}
	if riskSummary["level"] != "medium" {
		t.Fatalf("risk level = %#v, want medium", riskSummary["level"])
	}
	if riskSummary["requires_approval"] != true {
		t.Fatalf("risk requires_approval = %#v, want true", riskSummary["requires_approval"])
	}
	riskCategories, ok := riskSummary["categories"].([]string)
	if !ok || len(riskCategories) != 1 || riskCategories[0] != "database_write" {
		t.Fatalf("risk categories = %#v, want only database_write", riskSummary["categories"])
	}

	metadataBytes, err := json.Marshal(message.Metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	metadataJSON := string(metadataBytes)
	if strings.Contains(metadataJSON, secretMarker) {
		t.Fatalf("message metadata leaked sensitive operation text: %s", metadataJSON)
	}
	if _, exists := message.Metadata["operation_context"]; exists {
		t.Fatalf("message metadata contains raw operation_context")
	}
}

func TestRegenerateRequestCarriesOperationContext(t *testing.T) {
	original := consoleFilesSnapshotTestParts("summary the second Excel", []consoleFilesTestFile{
		{
			ID:        "file-1",
			Name:      "notes.txt",
			Extension: "txt",
			MimeType:  "text/plain",
		},
		{
			ID:        "file-2",
			Name:      "budget.xlsx",
			Extension: "xlsx",
			MimeType:  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			FileType:  "excel",
		},
	})

	parts, err := normalizeRegenerateRequest(runtimedto.RegenerateMessageRequest{
		RuntimeContext:   original.RuntimeContext,
		OperationContext: original.RawOperationContext,
	}, &runtimemodel.Message{
		Query:           "summary the second Excel",
		ModelName:       "test-model",
		ModelParameters: map[string]interface{}{},
		Metadata:        map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("normalizeRegenerateRequest() error = %v", err)
	}
	if !isConsoleFilesContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		t.Fatalf("regenerate parts missing console files context: %#v", parts.OperationContext)
	}
	if !hasConsoleFilesReadCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		t.Fatalf("regenerate parts missing file.read capability: %#v", parts.OperationContext)
	}
	if parts.OperationLedger == nil {
		t.Fatal("regenerate parts missing operation ledger")
	}

	metadata := streamingMessageMetadata(parts)
	if _, ok := metadata["operation_context"]; ok {
		t.Fatalf("operation_context leaked into metadata: %#v", metadata["operation_context"])
	}
	if snapshot := mapFromOperationContext(metadata[consoleFilesContextSnapshotKey]); len(snapshot) == 0 {
		t.Fatalf("regenerate metadata missing console files snapshot: %#v", metadata)
	}
}

func TestRegenerateRequestInheritsMessageSurfaceWhenOmitted(t *testing.T) {
	message := &runtimemodel.Message{
		Query:           "open the agents page",
		ModelName:       "test-model",
		ModelParameters: map[string]interface{}{},
		Metadata: map[string]interface{}{
			"surface": aiChatSurfaceContextualSidebar,
		},
	}

	parts, err := normalizeRegenerateRequest(runtimedto.RegenerateMessageRequest{}, message)
	if err != nil {
		t.Fatalf("normalizeRegenerateRequest() error = %v", err)
	}
	if parts.Surface != aiChatSurfaceContextualSidebar {
		t.Fatalf("surface = %q, want inherited %q", parts.Surface, aiChatSurfaceContextualSidebar)
	}

	override := aiChatSurfaceExternalPageChat
	parts, err = normalizeRegenerateRequest(runtimedto.RegenerateMessageRequest{
		Surface: override,
	}, message)
	if err != nil {
		t.Fatalf("normalizeRegenerateRequest() with override error = %v", err)
	}
	if parts.Surface != override {
		t.Fatalf("surface = %q, want explicit override %q", parts.Surface, override)
	}
}

func TestOperationContextDoesNotEnterModelContentAndRuntimeContextRemainsTransient(t *testing.T) {
	svc := &service{}
	runtimeContext := "Page /console/resources with selected chips."
	operationResourceID := "operation-resource-not-for-model"
	operationCapabilityID := "operation-capability-not-for-model"

	parts, err := normalizeChatRequest(runtimedto.ChatRequest{
		Query:          "Use the visible page context.",
		Model:          "test-model",
		RuntimeContext: runtimeContext,
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"id": operationResourceID, "type": "file"},
			},
			"capabilities": []interface{}{
				map[string]interface{}{"id": operationCapabilityID, "name": "download"},
			},
		},
	})
	if err != nil {
		t.Fatalf("normalizeChatRequest() error = %v", err)
	}

	content, ok := svc.currentUserContent(parts, parts.Query).(string)
	if !ok {
		t.Fatalf("content type = %T, want string", content)
	}
	for _, want := range []string{
		"Transient ZGI page context",
		runtimeContext,
		"User request:",
		"Use the visible page context.",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q:\n%s", want, content)
		}
	}
	for _, unwanted := range []string{operationResourceID, operationCapabilityID} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("content leaked operation context value %q:\n%s", unwanted, content)
		}
	}

	message := newStreamingMessage(uuid.New(), nil, parts)
	if message.Query != parts.Query {
		t.Fatalf("message query = %q, want original query %q", message.Query, parts.Query)
	}
	if strings.Contains(message.Query, runtimeContext) {
		t.Fatalf("message query contains runtime context: %q", message.Query)
	}
	if _, ok := message.Metadata["operation_ledger"].(map[string]interface{}); !ok {
		t.Fatalf("operation_ledger metadata = %#v, want map", message.Metadata["operation_ledger"])
	}
	metadataBytes, err := json.Marshal(message.Metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if strings.Contains(string(metadataBytes), runtimeContext) {
		t.Fatalf("message metadata leaked runtime context content")
	}
}
