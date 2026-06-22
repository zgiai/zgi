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
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager, skills.SkillChartGenerator}
	prepared.parts.SkillMode = skillModeAuto

	messages := skillLoopAdditionalSystemMessages(prepared)
	if len(messages) != 1 {
		t.Fatalf("additional messages = %d, want 1", len(messages))
	}
	content := messageContentText(messages[0].Content)
	for _, want := range []string{
		"file-generator",
		"file-manager",
		"chart-generator",
		"generate_file",
		"save_file_to_management",
		`"capability_id":"file.create"`,
		"temporary_artifact",
		"generic SVG/vector",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("contextual create guidance missing %q in:\n%s", want, content)
		}
	}
}

func TestContextualAIChatTurnStrategyPlansRouteBeforeManagedFileCreate(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "please create an svg file in File Management",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/work/chat",
			SkillIDs: []string{
				skills.SkillConsoleNavigator,
				skills.SkillFileGenerator,
				skills.SkillFileManager,
				skills.SkillChartGenerator,
			},
			SkillMode: skillModeAuto,
		},
	}

	message, ok := contextualAIChatTurnStrategyMessage(prepared)
	if !ok {
		t.Fatal("contextualAIChatTurnStrategyMessage() ok = false, want true")
	}
	content := messageContentText(message.Content)
	for _, want := range []string{
		"ZGI AIChat turn strategy guidance",
		`"intent":"save_generated_file_to_file_management"`,
		`"target_page":"/console/files"`,
		`"route_required":true`,
		skills.SkillConsoleNavigator,
		skills.SkillFileGenerator,
		skills.SkillFileManager,
		"exactly one temporary artifact",
		"asset_observation:file.create",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("turn strategy missing %q in:\n%s", want, content)
		}
	}
}

func TestContextualAIChatTurnStrategyUsesRecentArtifactWithoutRegeneration(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("\u628a\u8fd9\u4e2a\u6587\u4ef6\u4fdd\u5b58\u5230\u6587\u4ef6\u7ba1\u7406\u4e2d"),
	}
	prepared.parts.Surface = aiChatSurfaceContextualSidebar
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.RecentGeneratedArtifacts = []map[string]interface{}{{
		"tool_file_id": "tool-recent-1",
		"filename":     "monthly-sales-bar.svg",
	}}

	message, ok := contextualAIChatTurnStrategyMessage(prepared)
	if !ok {
		t.Fatal("contextualAIChatTurnStrategyMessage() ok = false, want true")
	}
	content := messageContentText(message.Content)
	for _, want := range []string{
		`"artifact_source":"recent_generated_file"`,
		`"primary_skills":["file-manager"]`,
		"do not generate another file",
		"save_generated_file_to_file_management",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("recent-artifact strategy missing %q in:\n%s", want, content)
		}
	}
}

func TestContextualAIChatTurnStrategyClassifiesFilesPageRead(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesSnapshotTestParts("\u603b\u7ed3\u7b2c\u4e00\u4e2a\u6587\u4ef6", []consoleFilesTestFile{
			{ID: "file-1", Name: "notes.md", Extension: "md", MimeType: "text/markdown"},
		}),
	}
	prepared.parts.Surface = aiChatSurfaceContextualSidebar
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto

	message, ok := contextualAIChatTurnStrategyMessage(prepared)
	if !ok {
		t.Fatal("contextualAIChatTurnStrategyMessage() ok = false, want true")
	}
	content := messageContentText(message.Content)
	for _, want := range []string{
		`"intent":"read_visible_file_content"`,
		`"target_page":"/console/files"`,
		`"primary_skills":["file-reader"]`,
		"read_file_result",
		"final answer is based on the returned file content",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("files-read strategy missing %q in:\n%s", want, content)
		}
	}
}

func TestContextualAIChatTurnStrategyIsTypedAndRecordedInMetadata(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "please create an svg file in File Management",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/work/chat",
			SkillIDs: []string{
				skills.SkillConsoleNavigator,
				skills.SkillFileGenerator,
				skills.SkillFileManager,
			},
			SkillMode: skillModeAuto,
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "save_generated_file_to_file_management" {
		t.Fatalf("Intent = %q, want save_generated_file_to_file_management", strategy.Intent)
	}
	if strategy.TargetPage != "/console/files" || !strategy.RouteRequired {
		t.Fatalf("target/route = %q/%v, want /console/files/true", strategy.TargetPage, strategy.RouteRequired)
	}
	if len(strategy.PrimarySkills) == 0 || strategy.PrimarySkills[0] != skills.SkillConsoleNavigator {
		t.Fatalf("PrimarySkills = %#v, want console navigator first", strategy.PrimarySkills)
	}

	metadata := streamingMessageMetadata(prepared.parts)
	stored, ok := metadata["turn_strategy"].(*AIChatTurnStrategy)
	if !ok || stored == nil {
		t.Fatalf("metadata turn_strategy = %#v, want *AIChatTurnStrategy", metadata["turn_strategy"])
	}
	if stored.Intent != strategy.Intent || stored.TargetPage != strategy.TargetPage {
		t.Fatalf("stored strategy = %#v, want same intent and target as %#v", stored, strategy)
	}
}

func TestContextualAIChatTurnStrategyResolvesChineseFilesRoute(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "\u8bf7\u6253\u5f00\u6587\u4ef6\u7ba1\u7406\u9875\u9762",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillIDs:  []string{skills.SkillConsoleNavigator},
			SkillMode: skillModeAuto,
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "navigate_console_page" {
		t.Fatalf("Intent = %q, want navigate_console_page", strategy.Intent)
	}
	if strategy.TargetPage != "/console/files" || !strategy.RouteRequired {
		t.Fatalf("target/route = %q/%v, want /console/files/true", strategy.TargetPage, strategy.RouteRequired)
	}
}

func TestSkillLoopAdditionalSystemMessagesIncludesRecentGeneratedFiles(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("\u628a\u8fd9\u4e2a\u6587\u4ef6\u4e0a\u4f20\u5230\u6587\u4ef6\u7ba1\u7406"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.RecentGeneratedArtifacts = []map[string]interface{}{{
		"tool_file_id":      "tool-recent-1",
		"filename":          "monthly-sales-bar.svg",
		"extension":         ".svg",
		"mime_type":         "image/svg+xml",
		"source_message_id": "message-1",
	}}

	messages := skillLoopAdditionalSystemMessages(prepared)
	if len(messages) != 1 {
		t.Fatalf("additional messages = %d, want 1", len(messages))
	}
	content := messageContentText(messages[0].Content)
	for _, want := range []string{
		"recent_generated_files",
		`"tool_file_id":"tool-recent-1"`,
		`"filename":"monthly-sales-bar.svg"`,
		"before considering visible_files",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("recent generated file guidance missing %q in:\n%s", want, content)
		}
	}
}

func TestSkillLoopAdditionalSystemMessagesAddsChartOnlyCreateGuidance(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("please create a radar chart in File Management"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillChartGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto

	messages := skillLoopAdditionalSystemMessages(prepared)
	if len(messages) != 1 {
		t.Fatalf("additional messages = %d, want 1", len(messages))
	}
	content := messageContentText(messages[0].Content)
	for _, want := range []string{
		"chart-generator",
		"generate_chart",
		"file-manager",
		"save_file_to_management",
		"artifact-producing skill",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("chart-only create guidance missing %q in:\n%s", want, content)
		}
	}
	if strings.Contains(content, `"skill_id":"file-generator"`) {
		t.Fatalf("chart-only create guidance should not expose disabled file-generator tools:\n%s", content)
	}
}

func TestSkillLoopFinalAnswerGuardBlocksManagedFileCreateWithoutToolCall(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("\u8bf7\u5728\u6587\u4ef6\u7ba1\u7406\u4e2d\u521b\u5efa\u4e00\u4e2a txt \u6587\u4ef6"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
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
	for _, want := range []string{skills.SkillFileGenerator, skills.SkillFileManager, "save_file_to_management", "Do not finish"} {
		if !strings.Contains(result.SystemMessage, want) && !strings.Contains(result.Message, want) {
			t.Fatalf("guard result missing %q: %#v", want, result)
		}
	}

	result, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "The file has been created.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:  skills.SkillFileGenerator,
			ToolName: "generate_file",
			Arguments: map[string]interface{}{
				"filename": "smoke",
				"format":   "txt",
			},
			Result: map[string]interface{}{
				"tool_file_id": "tool-1",
				"filename":     "smoke.txt",
			},
		}},
	})
	if !blocked {
		t.Fatal("guard allowed completion after temporary generation without file-manager save")
	}
	for _, want := range []string{
		"Do not generate another file",
		`"skill_id":"file-manager"`,
		`"tool_name":"save_file_to_management"`,
		`"tool_file_id":"tool-1"`,
		`"filename":"smoke.txt"`,
	} {
		if !strings.Contains(result.SystemMessage, want) {
			t.Fatalf("guard system message missing %q in:\n%s", want, result.SystemMessage)
		}
	}
	if strings.Contains(result.Message, "tool-1") {
		t.Fatalf("guard user-visible message exposed tool file id: %s", result.Message)
	}

	result, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "The chart has been generated.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:  skills.SkillChartGenerator,
			ToolName: "generate_chart",
			Arguments: map[string]interface{}{
				"output_filename": "chart",
			},
			Result: map[string]interface{}{
				"file_id":      "chart-tool-1",
				"filename":     "chart.svg",
				"mime_type":    "image/svg+xml",
				"download_url": "/tool-files/chart-tool-1?download=1",
			},
		}},
	})
	if !blocked {
		t.Fatal("guard allowed completion after chart artifact generation without file-manager save")
	}
	for _, want := range []string{`"tool_file_id":"chart-tool-1"`, `"filename":"chart.svg"`, `"tool_name":"save_file_to_management"`} {
		if !strings.Contains(result.SystemMessage, want) {
			t.Fatalf("chart artifact guard system message missing %q in:\n%s", want, result.SystemMessage)
		}
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		AttemptedToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:  skills.SkillFileManager,
			ToolName: "save_file_to_management",
			Arguments: map[string]interface{}{
				"source_type":  "tool_file",
				"tool_file_id": "tool-1",
				"filename":     "smoke.txt",
			},
		}},
	})
	if blocked {
		t.Fatal("guard blocked = true after file-manager save tool call, want false")
	}

	result, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "The file has been saved to File Management.",
		AttemptedToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:  skills.SkillFileManager,
			ToolName: "save_file_to_management",
			Arguments: map[string]interface{}{
				"source_type":  "tool_file",
				"tool_file_id": "tool-1",
				"filename":     "smoke.txt",
			},
		}},
	})
	if !blocked {
		t.Fatal("guard allowed a success claim after save_file_to_management was only attempted")
	}
	for _, want := range []string{"did not succeed", "Do not say the file was created", "actual tool result"} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("attempted-save guard message missing %q in:\n%s", want, result.Message)
		}
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "The save to File Management failed; please retry after checking permissions.",
		AttemptedToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:  skills.SkillFileManager,
			ToolName: "save_file_to_management",
			Arguments: map[string]interface{}{
				"source_type":  "tool_file",
				"tool_file_id": "tool-1",
				"filename":     "smoke.txt",
			},
		}},
	})
	if blocked {
		t.Fatal("guard blocked a failure report after save_file_to_management was attempted")
	}
}

func TestSkillLoopFinalAnswerGuardUsesRecentGeneratedArtifactForReferencedSave(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("\u5bfc\u822a\u540e\uff0c\u5982\u679c\u4e0d\u5728\u7ba1\u7406\u9875\u9762\uff0c\u5c31\u628a\u8fd9\u4e2a\u6587\u4ef6\u4e0a\u4f20\u5230\u7ba1\u7406\u91cc\u9762"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.RecentGeneratedArtifacts = []map[string]interface{}{{
		"tool_file_id": "tool-recent-1",
		"filename":     "monthly-sales-bar.svg",
		"extension":    ".svg",
		"mime_type":    "image/svg+xml",
	}}

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want guard for referenced recent artifact save")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "It is already in File Management.",
	})
	if !blocked {
		t.Fatal("guard allowed completion before saving the referenced recent artifact")
	}
	for _, want := range []string{
		"recent generated/downloadable file",
		`"tool_name":"save_file_to_management"`,
		`"tool_file_id":"tool-recent-1"`,
		`"filename":"monthly-sales-bar.svg"`,
	} {
		if !strings.Contains(result.SystemMessage, want) {
			t.Fatalf("guard system message missing %q in:\n%s", want, result.SystemMessage)
		}
	}
	if strings.Contains(result.Message, "tool-recent-1") {
		t.Fatalf("guard user-visible message exposed tool file id: %s", result.Message)
	}
}

func TestSkillLoopToolCallGuardRoutesManagedFileCreateBeforeGeneration(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "please create an svg file in File Management",
			RuntimeContext: "route=/console/work/chat",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillFileGenerator, skills.SkillFileManager, skills.SkillChartGenerator},
			SkillMode:      skillModeAuto,
		},
	}

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want managed-file route guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileGenerator,
		ToolName: "generate_file",
		Arguments: map[string]interface{}{
			"filename": "hello.svg",
			"format":   "svg",
		},
	})
	if !blocked {
		t.Fatal("tool guard allowed file generation before Files page navigation")
	}
	for _, want := range []string{skills.SkillConsoleNavigator, "navigate", "/console/files"} {
		if !strings.Contains(result.SystemMessage, want) && !strings.Contains(result.Message, want) {
			t.Fatalf("guard result missing %q: %#v", want, result)
		}
	}

	_, blocked = guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillConsoleNavigator,
		ToolName:  "navigate",
		Arguments: map[string]interface{}{"href": "/console/files"},
	})
	if blocked {
		t.Fatal("tool guard blocked the required Files page navigation")
	}

	result, blocked = guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillChartGenerator,
		ToolName: "generate_chart",
		Arguments: map[string]interface{}{
			"chart_type":      "radar",
			"output_filename": "wrong",
		},
	})
	if !blocked {
		t.Fatal("tool guard allowed chart generation before Files page navigation")
	}
	for _, want := range []string{skills.SkillConsoleNavigator, "navigate", "/console/files"} {
		if !strings.Contains(result.SystemMessage, want) && !strings.Contains(result.Message, want) {
			t.Fatalf("chart route guard result missing %q: %#v", want, result)
		}
	}
}

func TestSkillLoopToolCallGuardPreventsDuplicateManagedFileGeneration(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("please create a txt file in File Management"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want managed-file duplicate generation guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileGenerator,
		ToolName: "generate_file",
		Arguments: map[string]interface{}{
			"filename": "second.txt",
			"format":   "txt",
		},
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:  skills.SkillFileGenerator,
			ToolName: "generate_file",
			Arguments: map[string]interface{}{
				"filename": "first",
				"format":   "txt",
			},
			Result: map[string]interface{}{
				"tool_file_id": "tool-1",
				"filename":     "first.txt",
			},
		}},
	})
	if !blocked {
		t.Fatal("tool guard allowed duplicate file generation after a temporary artifact already existed")
	}
	for _, want := range []string{skills.SkillFileManager, "save_file_to_management", `"tool_file_id":"tool-1"`, `"filename":"first.txt"`} {
		if !strings.Contains(result.SystemMessage, want) {
			t.Fatalf("guard system message missing %q in:\n%s", want, result.SystemMessage)
		}
	}
	if strings.Contains(result.Message, "tool-1") {
		t.Fatalf("guard user-visible message exposed tool file id: %s", result.Message)
	}

	_, blocked = guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileManager,
		ToolName: "save_file_to_management",
		Arguments: map[string]interface{}{
			"source_type":  "tool_file",
			"tool_file_id": "tool-1",
			"filename":     "first.txt",
		},
	})
	if blocked {
		t.Fatal("tool guard blocked file-manager save")
	}

	result, blocked = guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileGenerator,
		ToolName: "generate_file",
		Arguments: map[string]interface{}{
			"filename": "second.txt",
			"format":   "txt",
		},
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:  skills.SkillChartGenerator,
			ToolName: "generate_chart",
			Result: map[string]interface{}{
				"file_id":      "chart-tool-1",
				"filename":     "scores.svg",
				"format":       "svg",
				"download_url": "/tool-files/chart-tool-1?download=1",
			},
		}},
	})
	if !blocked {
		t.Fatal("tool guard allowed duplicate generation after a chart artifact already existed")
	}
	for _, want := range []string{skills.SkillFileManager, "save_file_to_management", `"tool_file_id":"chart-tool-1"`, `"filename":"scores.svg"`} {
		if !strings.Contains(result.SystemMessage, want) {
			t.Fatalf("chart duplicate guard system message missing %q in:\n%s", want, result.SystemMessage)
		}
	}
}

func TestSkillLoopToolCallGuardBlocksDuplicateGenerationForReferencedRecentArtifact(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("\u628a\u8fd9\u4e2a\u6587\u4ef6\u4e0a\u4f20\u5230\u6587\u4ef6\u7ba1\u7406\u91cc\u9762"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.RecentGeneratedArtifacts = []map[string]interface{}{{
		"tool_file_id": "tool-recent-1",
		"filename":     "monthly-sales-bar.svg",
	}}

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want guard for recent artifact save")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileGenerator,
		ToolName: "generate_file",
		Arguments: map[string]interface{}{
			"filename": "duplicate.svg",
		},
	})
	if !blocked {
		t.Fatal("tool guard allowed duplicate generation instead of saving recent artifact")
	}
	for _, want := range []string{`"tool_file_id":"tool-recent-1"`, `"filename":"monthly-sales-bar.svg"`, "save_file_to_management"} {
		if !strings.Contains(result.SystemMessage, want) {
			t.Fatalf("tool guard system message missing %q in:\n%s", want, result.SystemMessage)
		}
	}
}

func TestSkillLoopToolCallGuardKeepsGenericSVGOnFileGenerator(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("please create an svg file in File Management"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager, skills.SkillChartGenerator}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want managed-file generator guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillChartGenerator,
		ToolName: "generate_chart",
		Arguments: map[string]interface{}{
			"chart_type":      "radar",
			"output_filename": "wrong",
		},
	})
	if !blocked {
		t.Fatal("tool guard allowed chart-generator for generic SVG creation")
	}
	for _, want := range []string{skills.SkillFileGenerator, "generate_file", "generic SVG"} {
		if !strings.Contains(result.SystemMessage, want) && !strings.Contains(result.Message, want) {
			t.Fatalf("generic SVG guard result missing %q: %#v", want, result)
		}
	}
}

func TestSkillLoopToolCallGuardBlocksRepeatedFilesNavigationAfterContinuation(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "please create an svg file in File Management",
			RuntimeContext: "route=/console/work/chat",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillFileGenerator, skills.SkillFileManager},
			SkillMode:      skillModeAuto,
			OperationContext: map[string]interface{}{
				"client_action_continuation": map[string]interface{}{
					"action_type": "route_navigation",
					"status":      clientActionStatusSucceeded,
					"href":        "/console/files",
					"result": map[string]interface{}{
						"href":          "/console/files",
						"observed_path": "/console/files",
					},
				},
			},
		},
	}

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want repeated navigation guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillConsoleNavigator,
		ToolName:  "navigate",
		Arguments: map[string]interface{}{"href": "/console/files"},
	})
	if !blocked {
		t.Fatal("tool guard allowed repeated Files page navigation after continuation")
	}
	for _, want := range []string{"already loaded", "Do not navigate"} {
		if !strings.Contains(result.SystemMessage, want) && !strings.Contains(result.Message, want) {
			t.Fatalf("guard result missing %q: %#v", want, result)
		}
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

func TestSkillLoopFinalAnswerGuardAllowsAgentDetailRouteForAgentsModule(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "open the first agent page and inspect its config",
			SkillIDs:  []string{skills.SkillConsoleNavigator},
			SkillMode: skillModeAuto,
		},
	}

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want guard before navigation")
	}
	_, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "Opened the first agent.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillConsoleNavigator, ToolName: "navigate", Arguments: map[string]interface{}{"href": "/console/agents/3806ca05-55c0-4380-a07a-e1cbf6fdcdd1/agent"}},
		},
	})
	if blocked {
		t.Fatal("guard blocked after navigating to an Agent detail route under /console/agents")
	}
}

func TestSkillLoopFinalAnswerGuardSkipsAfterClientActionLoadedAgentDetailRoute(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "open the first agent page and inspect its config",
			SkillIDs:  []string{skills.SkillConsoleNavigator},
			SkillMode: skillModeAuto,
			OperationContext: map[string]interface{}{
				"client_action_continuation": map[string]interface{}{
					"action_type": "route_navigation",
					"status":      clientActionStatusSucceeded,
					"result": map[string]interface{}{
						"event_type":     "route_loaded",
						"href":           "/console/agents/3806ca05-55c0-4380-a07a-e1cbf6fdcdd1/agent",
						"observed_path":  "/console/agents/3806ca05-55c0-4380-a07a-e1cbf6fdcdd1/agent",
						"context_scope":  "agent-runtime",
						"context_status": "ready",
					},
				},
			},
		},
	}

	if guard := skillLoopFinalAnswerGuard(prepared); guard != nil {
		if _, blocked := guard(skillloop.FinalAnswerGuardRequest{Answer: "Here is the agent configuration."}); blocked {
			t.Fatal("guard blocked after client action continuation loaded the Agent detail route")
		}
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
