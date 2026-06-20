package service

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestSkillLoopAdditionalSystemMessagesAddsConsoleFilesToolGuidance(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesSnapshotTestParts("delete the first file", []consoleFilesTestFile{
			{ID: "file-1", Name: "invoice.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillCalculator, skills.SkillFileManager, skills.SkillFileReader}

	messages := skillLoopAdditionalSystemMessages(prepared)
	if len(messages) != 1 {
		t.Fatalf("additional messages = %d, want 1", len(messages))
	}
	content := messageContentText(messages[0].Content)
	for _, want := range []string{
		"Answer in the user's language",
		"do not mention internal IDs",
		"mention only the file name and the user-visible action result",
		"When a file tool fails, explain the failure plainly",
		"file-manager/delete_file",
		"Tool governance handles the approval card",
		"session grant exists, it only skips the approval prompt",
		"must still call file-manager/delete_file",
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
		"Resolved internal target JSON for tool arguments only",
		"tool_argument_visibility_restriction",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("contextual read guidance missing %q in:\n%s", want, content)
		}
	}
}

func TestSkillLoopAdditionalSystemMessagesResolvesRecentFileTarget(t *testing.T) {
	query := "\u8bf7\u57fa\u4e8e\u521a\u624d\u90a3\u4e2a\u6587\u4ef6\u63d0\u53d6\u7f34\u8d39\u8d26\u6237"
	prepared := &PreparedChat{
		parts: consoleFilesSnapshotTestParts(query, []consoleFilesTestFile{
			{ID: "file-1", Name: "invoice.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.RecentAssetCandidates = []ResourceCandidate{{
		Type:      resourceTypeFile,
		ID:        "file-1",
		Name:      "invoice.xlsx",
		Source:    "recent_execution.read_file",
		Extension: "xlsx",
		Recent:    true,
	}}

	messages := skillLoopAdditionalSystemMessages(prepared)
	if len(messages) != 1 {
		t.Fatalf("additional messages = %d, want 1", len(messages))
	}
	content := messageContentText(messages[0].Content)
	for _, want := range []string{
		"resolved_targets_from_user_request",
		`"file_id":"file-1"`,
		"read_file",
		"target is already resolved",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("contextual recent guidance missing %q in:\n%s", want, content)
		}
	}
}

func TestSkillLoopAdditionalSystemMessagesAddsConsoleFilesListToolGuidance(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesSemanticTestParts("what files are visible", []consoleFilesTestFile{
			{ID: "file-1", Name: "one.txt", Extension: "txt", MimeType: "text/plain"},
			{ID: "file-2", Name: "two.pdf", Extension: "pdf", MimeType: "application/pdf", FileType: "pdf", WorkspaceID: "workspace-2", Selected: true},
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
		`"file_type":"pdf"`,
		`"selected":true`,
		`"workspace_id":"workspace-2"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("contextual list guidance missing %q in:\n%s", want, content)
		}
	}
}

func TestSkillLoopAdditionalSystemMessagesAddsConsoleFilesCreateGuidance(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("please create a txt file in File Management"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator}
	prepared.parts.SkillMode = skillModeAuto

	messages := skillLoopAdditionalSystemMessages(prepared)
	if len(messages) != 1 {
		t.Fatalf("additional messages = %d, want 1", len(messages))
	}
	content := messageContentText(messages[0].Content)
	for _, want := range []string{
		"file-generator",
		"generate_file",
		`"capability_id":"file.create"`,
		`target "managed_file"`,
		"temporary_artifact",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("contextual create guidance missing %q in:\n%s", want, content)
		}
	}
}

func TestSkillLoopFinalAnswerGuardBlocksManagedFileCreateWithoutToolCall(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("\u8bf7\u5728\u6587\u4ef6\u7ba1\u7406\u4e2d\u521b\u5efa\u4e00\u4e2a txt \u6587\u4ef6"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want guard for managed file creation")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "This is unsupported.",
	})
	if !blocked {
		t.Fatal("guard blocked = false, want true")
	}
	for _, want := range []string{skills.SkillFileGenerator, "managed_file", "Do not finish"} {
		if !strings.Contains(result.SystemMessage, want) && !strings.Contains(result.Message, want) {
			t.Fatalf("guard result missing %q: %#v", want, result)
		}
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		AttemptedToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:  skills.SkillFileGenerator,
			ToolName: "generate_file",
			Arguments: map[string]interface{}{
				"filename": "smoke.txt",
				"target":   "managed_file",
			},
		}},
	})
	if blocked {
		t.Fatal("guard blocked = true after managed file-generator tool call, want false")
	}
}

func TestSkillLoopAdditionalSystemMessagesAddsConsoleNavigationGuidance(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "\u5e26\u6211\u53bb\u5b9a\u65f6\u4efb\u52a1\u9875\u9762",
			SkillIDs:  []string{skills.SkillConsoleNavigator},
			SkillMode: skillModeAuto,
		},
	}

	messages := skillLoopAdditionalSystemMessages(prepared)
	if len(messages) != 1 {
		t.Fatalf("additional messages = %d, want 1", len(messages))
	}
	content := messageContentText(messages[0].Content)
	for _, want := range []string{
		"ZGI console navigation guidance",
		"console-navigator/navigate",
		"Do not use request_user_input",
		"Do not say a page has been opened unless",
		`"href":"/console/work/task"`,
		`"label":"定时任务"`,
		"resolved_target_from_user_request",
		`"/console/files"`,
		`"/console/agents"`,
		`"/console/dataset"`,
		`"/console/db"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("console navigation guidance missing %q in:\n%s", want, content)
		}
	}
}

func TestSkillLoopFinalAnswerGuardBlocksConsoleNavigationWithoutToolCall(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "\u5e26\u6211\u53bb\u5b9a\u65f6\u4efb\u52a1\u9875\u9762",
			SkillIDs:  []string{skills.SkillConsoleNavigator},
			SkillMode: skillModeAuto,
		},
	}

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want guard for known console route request")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "\u5df2\u6253\u5f00\u5b9a\u65f6\u4efb\u52a1\u7ba1\u7406\u9875\u9762\u3002",
	})
	if !blocked {
		t.Fatal("guard did not block navigation success claim without navigate tool")
	}
	for _, want := range []string{
		"console-navigator",
		"navigate",
		"/console/work/task",
		"Only after navigate succeeds",
	} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("guard message missing %q in:\n%s", want, result.Message)
		}
	}
	if !strings.Contains(result.SystemMessage, `"href":"/console/work/task"`) {
		t.Fatalf("guard system message missing resolved href: %s", result.SystemMessage)
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "\u5df2\u6253\u5f00\u5b9a\u65f6\u4efb\u52a1\u9875\u9762\u3002",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillConsoleNavigator, ToolName: "navigate", Arguments: map[string]interface{}{"href": "/console/files"}},
		},
	})
	if !blocked {
		t.Fatal("guard allowed navigate call for the wrong route")
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "\u5df2\u6253\u5f00\u5b9a\u65f6\u4efb\u52a1\u9875\u9762\u3002",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillConsoleNavigator, ToolName: "navigate", Arguments: map[string]interface{}{"href": "/console/work/task"}},
		},
	})
	if blocked {
		t.Fatal("guard blocked after navigate succeeded for the resolved route")
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "\u5df2\u5c1d\u8bd5\u6253\u5f00\uff0c\u4f46\u5bfc\u822a\u5de5\u5177\u5931\u8d25\u3002",
		AttemptedToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillConsoleNavigator, ToolName: "navigate", Arguments: map[string]interface{}{"href": "/console/work/task?source=aichat"}},
		},
	})
	if blocked {
		t.Fatal("guard blocked after navigate was attempted for the resolved route")
	}
}

func TestSkillLoopUserInputGuardBlocksConsoleNavigationClarification(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "\u5e26\u6211\u53bb\u5b9a\u65f6\u4efb\u52a1\u9875\u9762",
			SkillIDs:  []string{skills.SkillConsoleNavigator},
			SkillMode: skillModeAuto,
		},
	}

	guard := skillLoopUserInputGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopUserInputGuard() = nil, want guard for known console route request")
	}
	result, blocked := guard(skillloop.UserInputGuardRequest{
		Message: "\u8bf7\u95ee\u60a8\u8981\u6253\u5f00\u54ea\u4e2a\u5b9a\u65f6\u4efb\u52a1\u9875\u9762\uff1f",
		Questions: []map[string]interface{}{
			{"id": "which_page", "question": "\u8bf7\u9009\u62e9\u76ee\u6807\u9875\u9762"},
		},
	})
	if !blocked {
		t.Fatal("guard did not block redundant route clarification")
	}
	for _, want := range []string{
		"known ZGI console route",
		"already resolved from the site map",
		"console-navigator",
		"/console/work/task",
	} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("user input guard message missing %q in:\n%s", want, result.Message)
		}
	}
}

func TestSkillLoopFinalAnswerGuardSkipsConsoleNavigationWhenIntentIsInformational(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "\u5b9a\u65f6\u4efb\u52a1\u662f\u4ec0\u4e48",
			SkillIDs:  []string{skills.SkillConsoleNavigator},
			SkillMode: skillModeAuto,
		},
	}

	if guard := skillLoopFinalAnswerGuard(prepared); guard != nil {
		t.Fatal("skillLoopFinalAnswerGuard() returned guard for informational task question, want nil")
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
				"target is already resolved",
				"typed ordinal requests",
				"file_type_rank",
				"extension_rank",
				"Do not ask the user to select a file",
				`content_status "extracted"`,
				`"file_id":"` + tt.want + `"`,
				`"extension":"xlsx"`,
				`"selected":true`,
			} {
				if !strings.Contains(content, want) {
					t.Fatalf("contextual read guidance missing %q in:\n%s", want, content)
				}
			}
			if tt.name == "second excel" {
				for _, want := range []string{
					`"file_id":"file-5"`,
					`"file_type":"excel"`,
					`"file_type_rank":2`,
					`"extension_rank":2`,
				} {
					if !strings.Contains(content, want) {
						t.Fatalf("contextual second-excel guidance missing %q in:\n%s", want, content)
					}
				}
			}
		})
	}
}

func TestSkillLoopFinalAnswerGuardBlocksConsoleFilesReadWithoutToolCall(t *testing.T) {
	prepared := preparedConsoleFilesGuardReadTest("\u5e2e\u6211\u6458\u8981\u7b2c\u4e8c\u4e2a Excel \u5e76\u7ffb\u8bd1")
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

func TestSkillLoopFinalAnswerGuardBlocksChineseReadOrdinalWithoutToolCall(t *testing.T) {
	prepared := preparedConsoleFilesGuardReadTest("\u8bfb\u7b2c\u56db\u4e2a\u6587\u4ef6")
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want guard for Chinese ordinal read request")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "budget-q2.xlsx is visible on the page.",
	})
	if !blocked {
		t.Fatal("guard did not block direct answer for Chinese ordinal read without read_file")
	}
	if !strings.Contains(result.Message, "budget-q2.xlsx") || !strings.Contains(result.Message, "read_file") {
		t.Fatalf("guard message = %q, want target and read_file", result.Message)
	}
	if strings.Contains(result.Message, "file-4") {
		t.Fatalf("guard message exposed internal file id in %q", result.Message)
	}
}

func TestSkillLoopFinalAnswerGuardBlocksRecentFileAnswerWithoutToolCall(t *testing.T) {
	query := "\u8bf7\u57fa\u4e8e\u521a\u624d\u90a3\u4e2a\u6587\u4ef6\u63d0\u53d6\u7f34\u8d39\u8d26\u6237"
	prepared := &PreparedChat{
		parts: consoleFilesSnapshotTestParts(query, []consoleFilesTestFile{
			{ID: "file-1", Name: "invoice.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.RecentAssetCandidates = []ResourceCandidate{{
		Type:      resourceTypeFile,
		ID:        "file-1",
		Name:      "invoice.xlsx",
		Source:    "recent_execution.read_file",
		Extension: "xlsx",
		Recent:    true,
	}}

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want guard for recent file read request")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "The account is 123 from the prior answer.",
	})
	if !blocked {
		t.Fatal("guard did not block direct recent-file answer without read_file")
	}
	if !strings.Contains(result.Message, "invoice.xlsx") || !strings.Contains(result.Message, "read_file") {
		t.Fatalf("guard message = %q, want target and read_file", result.Message)
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "Here is the extracted account from the file content.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillFileReader, ToolName: "read_file", Arguments: map[string]interface{}{"file_id": "file-1"}},
		},
	})
	if blocked {
		t.Fatal("guard blocked after read_file succeeded for recent file")
	}
}

func TestSkillLoopUserInputGuardBlocksConsoleFilesClarificationWhenTargetResolved(t *testing.T) {
	prepared := preparedConsoleFilesGuardReadTest("\u8bf7\u8bfb\u53d6\u7b2c\u4e8c\u4e2a Excel \u6587\u4ef6\uff0c\u5e76\u6458\u8981")
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopUserInputGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopUserInputGuard() = nil, want guard for concrete console file read")
	}
	result, blocked := guard(skillloop.UserInputGuardRequest{
		Message: "页面上有两个 Excel 文件，我需要确认您指的是哪一个。",
		Questions: []map[string]interface{}{
			{
				"id":       "which_excel",
				"question": "请选择要读取的 Excel 文件",
				"options": []map[string]interface{}{
					{"label": "budget-q1.xlsx"},
					{"label": "budget-q2.xlsx"},
				},
			},
		},
		AttemptedToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillFileReader, ToolName: "read_file", Arguments: map[string]interface{}{"file_id": "file-2"}},
		},
	})
	if !blocked {
		t.Fatal("guard did not block clarification after runtime resolved the target file")
	}
	for _, want := range []string{
		"request_user_input",
		"already resolved",
		"resolved_targets_from_user_request",
		"budget-q2.xlsx",
		"read_file",
	} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("guard message missing %q in:\n%s", want, result.Message)
		}
	}

	_, blocked = guard(skillloop.UserInputGuardRequest{
		Message: "读取后还需要确认输出格式。",
		AttemptedToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillFileReader, ToolName: "read_file", Arguments: map[string]interface{}{"file_id": "file-4"}},
		},
	})
	if blocked {
		t.Fatal("guard blocked after the resolved read_file target had already been attempted")
	}
}

func TestSkillLoopFinalAnswerGuardAllowsConsoleFilesListWithoutToolCall(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesSemanticTestParts("\u6211\u73b0\u5728\u6709\u54ea\u4e9b\u6587\u4ef6", []consoleFilesTestFile{
			{ID: "file-1", Name: "notes.txt", Extension: "txt", MimeType: "text/plain"},
			{ID: "file-2", Name: "budget-q1.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto

	if guard := skillLoopFinalAnswerGuard(prepared); guard != nil {
		t.Fatal("skillLoopFinalAnswerGuard() returned guard for file listing request, want nil")
	}
}

func TestConsoleFilesRequiredToolFinalAnswerGuardRequiresAllTargetFileIDs(t *testing.T) {
	guard := consoleFilesRequiredToolFinalAnswerGuard(skills.SkillFileReader, []map[string]interface{}{
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

func preparedConsoleFilesGuardReadTest(query string) *PreparedChat {
	return &PreparedChat{
		parts: consoleFilesSemanticTestParts(query, []consoleFilesTestFile{
			{ID: "file-1", Name: "notes.txt", Extension: "txt", MimeType: "text/plain"},
			{ID: "file-2", Name: "budget-q1.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
			{ID: "file-3", Name: "invoice.pdf", Extension: "pdf", MimeType: "application/pdf"},
			{ID: "file-4", Name: "budget-q2.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		}),
	}
}

func TestSkillLoopFinalAnswerGuardBlocksConsoleFilesDeleteWithoutToolCall(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesSnapshotTestParts("delete the first file", []consoleFilesTestFile{
			{ID: "file-1", Name: "invoice.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileManager}

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
		"file-manager",
		"delete_file",
		"session approval grant may skip the approval card",
	} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("guard message missing %q in:\n%s", want, result.Message)
		}
	}
	if strings.Contains(result.Message, "file-1") {
		t.Fatalf("guard message exposed internal file id in %q", result.Message)
	}
	if !strings.Contains(result.SystemMessage, "file-1") {
		t.Fatalf("guard system message missing internal file id for tool arguments in %q", result.SystemMessage)
	}
	if !strings.Contains(result.SystemMessage, "tool arguments only") ||
		!strings.Contains(result.SystemMessage, "do not reveal internal IDs") {
		t.Fatalf("guard system message missing internal-only visibility instruction in %q", result.SystemMessage)
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "The file has been deleted.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillFileManager, ToolName: "delete_file", Arguments: map[string]interface{}{"file_id": "file-1"}},
		},
	})
	if blocked {
		t.Fatal("guard blocked after delete_file succeeded")
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "The file has been deleted.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillFileManager, ToolName: "delete_file", Arguments: map[string]interface{}{"file_id": "file-2"}},
		},
	})
	if !blocked {
		t.Fatal("guard allowed delete_file for the wrong resolved file_id")
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "I tried to delete the file, but the tool reported it was not found.",
		AttemptedToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillFileManager, ToolName: "delete_file", Arguments: map[string]interface{}{"file_id": "file-1"}},
		},
	})
	if blocked {
		t.Fatal("guard blocked after delete_file was attempted and failed")
	}
}

func TestConsoleFilesGuardTargetSummaryUsesUserVisibleNames(t *testing.T) {
	got := consoleFilesGuardTargetSummary([]map[string]interface{}{
		{"file_id": "file-1", "name": "invoice.xlsx"},
		{"file_id": "file-2", "name": "report.pdf"},
	})
	if got != "invoice.xlsx, report.pdf" {
		t.Fatalf("consoleFilesGuardTargetSummary() = %q, want visible names only", got)
	}
	for _, hidden := range []string{"file-1", "file-2"} {
		if strings.Contains(got, hidden) {
			t.Fatalf("target summary exposed %q in %q", hidden, got)
		}
	}

	if got := consoleFilesGuardTargetSummary([]map[string]interface{}{{"file_id": "file-1"}}); got != "the resolved visible file" {
		t.Fatalf("consoleFilesGuardTargetSummary() = %q, want generic fallback without file id", got)
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
