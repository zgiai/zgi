package service

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestSkillLoopAdditionalSystemMessagesAddsConsoleFilesToolGuidance(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "delete the first file",
			RuntimeContext: "route=/console/files capabilities=file.delete",
			SkillIDs:       []string{skills.SkillCalculator, skills.SkillFileReader},
			RawOperationContext: map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"resource_type": "file",
						"resource_id":   "file-1",
						"title":         "invoice.xlsx",
						"capabilities": []interface{}{
							map[string]interface{}{"id": "file.delete"},
						},
					},
				},
			},
		},
	}

	messages := skillLoopAdditionalSystemMessages(prepared)
	if len(messages) != 1 {
		t.Fatalf("additional messages = %d, want 1", len(messages))
	}
	content := messageContentText(messages[0].Content)
	for _, want := range []string{
		"file-reader/delete_file",
		"Tool governance handles the approval card",
		"do not ask for a separate natural-language confirmation",
		"Do not call unrelated discovery",
		`"file_id":"file-1"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("contextual guidance missing %q in:\n%s", want, content)
		}
	}
}

func TestSkillLoopAdditionalSystemMessagesSkipsConsoleFilesGuidanceWithoutFileReader(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			RuntimeContext: "route=/console/files capabilities=file.delete",
			SkillIDs:       []string{skills.SkillCalculator},
			RawOperationContext: map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"resource_type": "file",
						"resource_id":   "file-1",
						"title":         "invoice.xlsx",
						"capabilities": []interface{}{
							map[string]interface{}{"id": "file.delete"},
						},
					},
				},
			},
		},
	}

	if messages := skillLoopAdditionalSystemMessages(prepared); len(messages) != 0 {
		t.Fatalf("additional messages = %#v, want none without file-reader", messages)
	}
}
