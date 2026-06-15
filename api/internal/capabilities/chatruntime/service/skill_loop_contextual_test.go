package service

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
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
		"session grant exists, it only skips the approval prompt",
		"must still call file-reader/delete_file",
		"do not ask for a separate natural-language confirmation",
		"Do not call unrelated discovery",
		`"file_id":"file-1"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("contextual guidance missing %q in:\n%s", want, content)
		}
	}
}

func TestSkillLoopAdditionalSystemMessagesAddsConsoleFilesReadTarget(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesSemanticTestParts("\u8bfb\u7b2c\u56db\u4e2a\u6587\u4ef6", []consoleFilesTestFile{
			{ID: "file-1", Name: "one.txt", Extension: "txt", MimeType: "text/plain"},
			{ID: "file-2", Name: "two.txt", Extension: "txt", MimeType: "text/plain"},
			{ID: "file-3", Name: "three.pdf", Extension: "pdf", MimeType: "application/pdf"},
			{ID: "file-4", Name: "four.pdf", Extension: "pdf", MimeType: "application/pdf"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto

	messages := skillLoopAdditionalSystemMessages(prepared)
	if len(messages) != 1 {
		t.Fatalf("additional messages = %d, want 1", len(messages))
	}
	content := messageContentText(messages[0].Content)
	for _, want := range []string{
		"file-reader/list_visible_files",
		"file-reader/read_file",
		`"capability_id":"file.list_visible"`,
		`"capability_id":"file.read"`,
		"resolved_targets_from_user_request",
		`"file_id":"file-4"`,
		`"visible_index":4`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("contextual read guidance missing %q in:\n%s", want, content)
		}
	}
}

func TestSkillLoopAdditionalSystemMessagesAddsConsoleFilesListToolGuidance(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesSemanticTestParts("what files are visible", []consoleFilesTestFile{
			{ID: "file-1", Name: "one.txt", Extension: "txt", MimeType: "text/plain"},
			{ID: "file-2", Name: "two.pdf", Extension: "pdf", MimeType: "application/pdf", Selected: true},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto

	messages := skillLoopAdditionalSystemMessages(prepared)
	if len(messages) != 1 {
		t.Fatalf("additional messages = %d, want 1", len(messages))
	}
	content := messageContentText(messages[0].Content)
	for _, want := range []string{
		"file-reader/list_visible_files",
		`"capability_id":"file.list_visible"`,
		`"file_id":"file-1"`,
		`"file_id":"file-2"`,
		`"selected":true`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("contextual list guidance missing %q in:\n%s", want, content)
		}
	}
}

func TestSkillLoopAdditionalSystemMessagesResolvesConsoleFilesReadTargets(t *testing.T) {
	files := []consoleFilesTestFile{
		{ID: "file-1", Name: "notes.txt", Extension: "txt", MimeType: "text/plain"},
		{ID: "file-2", Name: "budget-q1.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{ID: "file-3", Name: "proposal.pdf", Extension: "pdf", MimeType: "application/pdf"},
		{ID: "file-4", Name: "contract.md", Extension: "md", MimeType: "text/markdown", Selected: true},
		{ID: "file-5", Name: "budget-q2.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{ID: "file-6", Name: "signed.pdf", Extension: "pdf", MimeType: "application/pdf"},
	}
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "fourth file", query: "\u8bfb\u7b2c\u56db\u4e2a\u6587\u4ef6", want: "file-4"},
		{name: "second excel", query: "\u6458\u8981\u7b2c\u4e8c\u4e2a Excel", want: "file-5"},
		{name: "second spreadsheet", query: "\u6458\u8981\u7b2c\u4e8c\u4e2a\u8868\u683c", want: "file-5"},
		{name: "last pdf", query: "\u603b\u7ed3\u6700\u540e\u4e00\u4e2a PDF", want: "file-6"},
		{name: "selected file", query: "\u603b\u7ed3\u5f53\u524d\u9009\u4e2d\u7684\u6587\u4ef6", want: "file-4"},
		{name: "exact file name", query: "summarize proposal.pdf", want: "file-3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepared := &PreparedChat{parts: consoleFilesSemanticTestParts(tt.query, files)}
			prepared.parts.SkillIDs = []string{skills.SkillFileReader}
			prepared.parts.SkillMode = skillModeAuto

			messages := skillLoopAdditionalSystemMessages(prepared)
			if len(messages) != 1 {
				t.Fatalf("additional messages = %d, want 1", len(messages))
			}
			content := messageContentText(messages[0].Content)
			for _, want := range []string{
				"file-reader/read_file",
				"resolved_targets_from_user_request",
				`"file_id":"` + tt.want + `"`,
				`"extension":"xlsx"`,
				`"selected":true`,
			} {
				if !strings.Contains(content, want) {
					t.Fatalf("contextual read guidance missing %q in:\n%s", want, content)
				}
			}
		})
	}
}

func TestSkillLoopFinalAnswerGuardBlocksConsoleFilesReadWithoutToolCall(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesSemanticTestParts("\u5e2e\u6211\u6458\u8981\u7b2c\u4e8c\u4e2a Excel \u5e76\u7ffb\u8bd1", []consoleFilesTestFile{
			{ID: "file-1", Name: "notes.txt", Extension: "txt", MimeType: "text/plain"},
			{ID: "file-2", Name: "budget-q1.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
			{ID: "file-3", Name: "invoice.pdf", Extension: "pdf", MimeType: "application/pdf"},
			{ID: "file-4", Name: "budget-q2.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want guard for concrete console file read")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "budget-q2.xlsx is a quarterly budget spreadsheet.",
	})
	if !blocked {
		t.Fatal("guard did not block direct file-content answer without read_file")
	}
	for _, want := range []string{
		"budget-q2.xlsx",
		"file-reader",
		"read_file",
		"actual content",
	} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("guard message missing %q in:\n%s", want, result.Message)
		}
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "Here is the summary from the file content.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillFileReader, ToolName: "read_file", Arguments: map[string]interface{}{"file_id": "file-4"}},
		},
	})
	if blocked {
		t.Fatal("guard blocked after read_file succeeded")
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "Here is a summary, but it came from a different file.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillFileReader, ToolName: "read_file", Arguments: map[string]interface{}{"file_id": "file-2"}},
		},
	})
	if !blocked {
		t.Fatal("guard allowed read_file for the wrong resolved file_id")
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "I tried to read the file, but the tool returned file not found.",
		AttemptedToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillFileReader, ToolName: "read_file", Arguments: map[string]interface{}{"file_id": "file-4"}},
		},
	})
	if blocked {
		t.Fatal("guard blocked after read_file was attempted and failed")
	}
}

func TestSkillLoopFinalAnswerGuardBlocksConsoleFilesListWithoutToolCall(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesSemanticTestParts("\u6211\u73b0\u5728\u6709\u54ea\u4e9b\u6587\u4ef6", []consoleFilesTestFile{
			{ID: "file-1", Name: "notes.txt", Extension: "txt", MimeType: "text/plain"},
			{ID: "file-2", Name: "budget-q1.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want guard for file listing request")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "You have notes.txt and budget-q1.xlsx.",
	})
	if !blocked {
		t.Fatal("guard did not block direct file listing answer without list_visible_files")
	}
	for _, want := range []string{
		"file-reader",
		"list_visible_files",
		"visible files",
	} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("guard message missing %q in:\n%s", want, result.Message)
		}
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "Here are the visible files from the tool result.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillFileReader, ToolName: "list_visible_files"},
		},
	})
	if blocked {
		t.Fatal("guard blocked after list_visible_files succeeded")
	}
}

func TestConsoleFilesRequiredToolFinalAnswerGuardRequiresAllTargetFileIDs(t *testing.T) {
	guard := consoleFilesRequiredToolFinalAnswerGuard([]map[string]interface{}{
		{"file_id": "file-1", "name": "one.pdf"},
		{"file_id": "file-2", "name": "two.pdf"},
	}, "read_file", []string{"read {target}"})

	if guard == nil {
		t.Fatal("guard = nil, want guard")
	}

	_, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "I read one file.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillFileReader, ToolName: "read_file", Arguments: map[string]interface{}{"file_id": "file-1"}},
		},
	})
	if !blocked {
		t.Fatal("guard allowed completion after only one of two target files")
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "I read both files.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillFileReader, ToolName: "read_file", Arguments: map[string]interface{}{"file_id": "file-1"}},
			{SkillID: skills.SkillFileReader, ToolName: "read_file", Arguments: map[string]interface{}{"file_id": "file-2"}},
		},
	})
	if blocked {
		t.Fatal("guard blocked after both target files were read")
	}
}

func TestSkillLoopFinalAnswerGuardBlocksConsoleFilesDeleteWithoutToolCall(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "delete the first file",
			RuntimeContext: "route=/console/files capabilities=file.delete",
			SkillIDs:       []string{skills.SkillFileReader},
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

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want guard for console file deletion")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "The file has been deleted.",
	})
	if !blocked {
		t.Fatal("guard did not block direct deletion answer without delete_file")
	}
	for _, want := range []string{
		"invoice.xlsx",
		"file-reader",
		"delete_file",
		"session approval grant may skip the approval card",
	} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("guard message missing %q in:\n%s", want, result.Message)
		}
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "The file has been deleted.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillFileReader, ToolName: "delete_file", Arguments: map[string]interface{}{"file_id": "file-1"}},
		},
	})
	if blocked {
		t.Fatal("guard blocked after delete_file succeeded")
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "The file has been deleted.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillFileReader, ToolName: "delete_file", Arguments: map[string]interface{}{"file_id": "file-2"}},
		},
	})
	if !blocked {
		t.Fatal("guard allowed delete_file for the wrong resolved file_id")
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "I tried to delete the file, but the tool reported it was not found.",
		AttemptedToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillFileReader, ToolName: "delete_file", Arguments: map[string]interface{}{"file_id": "file-1"}},
		},
	})
	if blocked {
		t.Fatal("guard blocked after delete_file was attempted and failed")
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
