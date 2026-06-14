package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestFormatAttachmentSectionsIncludesFileID(t *testing.T) {
	sections := formatAttachmentSections([]attachmentFile{{
		ID:        "2d9cdfaa-5ecb-4f89-bc21-d2c5704844a7",
		Name:      "2号楼电费确认单公区.xlsx",
		Extension: "xlsx",
		Content:   "序号;用电位置;本月底数",
	}}, func(file attachmentFile) string {
		return file.Content
	})

	if !strings.Contains(sections, "File: 2号楼电费确认单公区.xlsx\n") {
		t.Fatalf("formatted sections = %q, want display name without duplicate extension", sections)
	}
	if strings.Contains(sections, ".xlsx .xlsx") {
		t.Fatalf("formatted sections = %q, want no duplicate extension", sections)
	}
	if !strings.Contains(sections, "File ID: 2d9cdfaa-5ecb-4f89-bc21-d2c5704844a7\n") {
		t.Fatalf("formatted sections = %q, want file ID", sections)
	}
}

func TestRuntimeContextIsTransientUserContent(t *testing.T) {
	svc := &service{}
	parts := &chatRequestParts{
		Query:          "Summarize this page.",
		RuntimeContext: "Page /console/agents with 2 context chips.",
	}

	content, ok := svc.currentUserContent(parts, parts.Query).(string)
	if !ok {
		t.Fatalf("content type = %T, want string", content)
	}
	for _, want := range []string{
		"Transient ZGI page context",
		"Page /console/agents with 2 context chips.",
		"User request:",
		"Summarize this page.",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q:\n%s", want, content)
		}
	}

	message := newStreamingMessage(uuid.New(), nil, parts)
	if message.Query != parts.Query {
		t.Fatalf("message query = %q, want original query %q", message.Query, parts.Query)
	}
	if strings.Contains(message.Query, parts.RuntimeContext) {
		t.Fatalf("message query contains runtime context: %q", message.Query)
	}
	runtimeContextMetadata, ok := message.Metadata["runtime_context"].(map[string]interface{})
	if !ok {
		t.Fatalf("runtime_context metadata = %#v, want metadata summary", message.Metadata["runtime_context"])
	}
	if runtimeContextMetadata["included"] != true {
		t.Fatalf("runtime_context metadata included = %#v, want true", runtimeContextMetadata["included"])
	}
	metadataBytes, err := json.Marshal(message.Metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if strings.Contains(string(metadataBytes), parts.RuntimeContext) {
		t.Fatalf("message metadata leaked runtime context content")
	}
}
