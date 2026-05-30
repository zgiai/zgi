package service

import (
	"strings"
	"testing"
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
