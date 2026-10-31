package service

import (
	"strings"
	"testing"

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

func TestSkillLoopAdditionalSystemMessagesAddsConsoleFilesContextWithoutPreResolvedTarget(t *testing.T) {
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
		`"file_id":"file-4"`,
		`"visible_index":4`,
		"decide the target from visible_files",
		`content_status "extracted"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("contextual read guidance missing %q in:\n%s", want, content)
		}
	}
	for _, unwanted := range []string{
		"resolved_targets_from_user_request",
		"Resolved internal target JSON for tool arguments only",
		"tool_argument_visibility_restriction",
		"target is already resolved",
	} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("contextual read guidance still contains pre-resolved target marker %q in:\n%s", unwanted, content)
		}
	}
}

func TestSkillLoopAdditionalSystemMessagesDoesNotPreResolveRecentManagedFileDeleteTarget(t *testing.T) {
	query := "\u8bf7\u5220\u9664\u521a\u521a\u521b\u5efa\u7684\u6587\u4ef6 aichat-plan-smoke.md\uff0c\u53ea\u5220\u9664\u8fd9\u4e2a\u6d4b\u8bd5\u6587\u4ef6\u3002"
	prepared := &PreparedChat{
		parts: consoleFilesSnapshotTestParts(query, nil),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileManager, skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.RecentAssetCandidates = []ResourceCandidate{{
		Type:      resourceTypeFile,
		ID:        "managed-file-other",
		Name:      "other-recent.md",
		Title:     "other-recent.md",
		Source:    "recent_execution.save_file_to_management",
		Extension: "md",
		Recent:    true,
	}, {
		Type:      resourceTypeFile,
		ID:        "managed-file-1",
		Name:      "aichat-plan-smoke.md",
		Title:     "aichat-plan-smoke.md",
		Source:    "recent_execution.save_file_to_management",
		Extension: "md",
		Recent:    true,
	}}

	messages := skillLoopAdditionalSystemMessages(prepared)
	if len(messages) != 1 {
		t.Fatalf("additional messages = %d, want 1", len(messages))
	}
	content := messageContentText(messages[0].Content)
	for _, want := range []string{
		"file-manager/delete_file",
		"Tool governance handles the approval card",
		"do not ask for a separate natural-language confirmation",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("contextual recent delete guidance missing %q in:\n%s", want, content)
		}
	}
	for _, unwanted := range []string{
		"resolved_targets_from_user_request",
		`"file_id":"managed-file-1"`,
		`"file_id":"managed-file-other"`,
		`"name":"aichat-plan-smoke.md"`,
		`"name":"other-recent.md"`,
	} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("contextual recent delete guidance included unintended target %q in:\n%s", unwanted, content)
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

func TestContextualAIChatTurnStrategyDoesNotPromoteAgentIntentWithoutModelIntent(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "\u521b\u5efa\u4e00\u4e2a\u65b0\u667a\u80fd\u4f53\uff0c\u53d6\u540d smoke agent\uff0c\u7136\u540e\u914d\u7f6e\u6a21\u578b\u548c skill",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillConsoleNavigator,
			skills.SkillAgentManagement,
		},
	}

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want default contextual strategy")
	}
	if strategy.Intent == "manage_agent_asset" {
		t.Fatalf("strategy.Intent = %q, want no legacy Agent-management promotion without model intent; strategy=%#v", strategy.Intent, strategy)
	}
	if strategy.Source == aiChatTurnStrategySourceTurnProtocol {
		t.Fatalf("strategy.Source = %q, want model-led default path without legacy semantic fallback", strategy.Source)
	}
	if containsString(strategy.PrimarySkills, skills.SkillAgentManagement) {
		t.Fatalf("PrimarySkills = %#v, want no rule-selected Agent-management primary skill without model intent", strategy.PrimarySkills)
	}
}

func TestContextualAIChatTurnStrategyDoesNotPromoteFileIntentWithoutModelIntent(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "\u8bfb\u53d6\u5f53\u524d\u6587\u4ef6\u9875\u7684\u7b2c\u4e00\u4e2a\u6587\u4ef6\u5e76\u603b\u7ed3",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/files",
		SkillMode:      skillModeAuto,
		SkillIDs: []string{
			skills.SkillConsoleNavigator,
			skills.SkillFileReader,
			skills.SkillFileManager,
		},
	}

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want default contextual strategy")
	}
	if strategy.Intent == "read_file" || strategy.Intent == "delete_file" || strategy.Intent == "create_managed_file" || strategy.Intent == "generate_temporary_file" {
		t.Fatalf("strategy.Intent = %q, want no legacy file-operation promotion without model intent; strategy=%#v", strategy.Intent, strategy)
	}
	if strategy.Source == aiChatTurnStrategySourceTurnProtocol {
		t.Fatalf("strategy.Source = %q, want model-led default path without legacy semantic fallback", strategy.Source)
	}
	if containsString(strategy.PrimarySkills, skills.SkillFileReader) || containsString(strategy.PrimarySkills, skills.SkillFileManager) {
		t.Fatalf("PrimarySkills = %#v, want no rule-selected file primary skill without model intent", strategy.PrimarySkills)
	}
}

func TestSkillLoopUsesMainLoopWithoutClassifiedIntent(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "你能做什么？",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/work/chat",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillFileReader},
			SkillMode:      skillModeAuto,
		},
	}

	if !prepared.toolLoopEnabled() {
		t.Fatal("toolLoopEnabled() = false, want main tool loop path")
	}
}

func TestSkillLoopConsoleNavigationGuidanceUsesAccessibleRouteSnapshot(t *testing.T) {
	operationContext := map[string]interface{}{
		"resources": []interface{}{
			map[string]interface{}{
				"resource_id": "assistant.platform_operation",
				"metadata": map[string]interface{}{
					"navigation_access_scope": "current_user",
					"accessible_routes":       "/console,/console/files,/console/skills",
				},
			},
		},
	}
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			SkillIDs:            []string{skills.SkillConsoleNavigator},
			RawOperationContext: operationContext,
			OperationContext:    operationContext,
		},
	}

	message, ok := contextualConsoleNavigationSkillMessageForResolved(prepared, nil)
	if !ok {
		t.Fatal("contextualConsoleNavigationSkillMessageForResolved() ok = false, want guidance")
	}
	content := messageContentText(message.Content)
	for _, want := range []string{
		`"href":"/console"`,
		`"href":"/console/files"`,
		`"href":"/console/skills"`,
		"current user's accessible pages",
		"trusted server-side workspace permissions",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("permission-aware navigation guidance missing %q in:\n%s", want, content)
		}
	}
	for _, denied := range []string{
		`"href":"/console/agents"`,
		`"href":"/console/workflows"`,
		`"href":"/console/dataset"`,
		`"href":"/console/db"`,
	} {
		if strings.Contains(content, denied) {
			t.Fatalf("permission-aware navigation guidance exposed denied route %q:\n%s", denied, content)
		}
	}
}

func TestSkillLoopAdditionalSystemMessagesLeavesConsoleFilesTargetsForModelDecision(t *testing.T) {
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
	}{
		{name: "fourth file", query: "\u8bfb\u7b2c\u56db\u4e2a\u6587\u4ef6"},
		{name: "second excel", query: "\u6458\u8981\u7b2c\u4e8c\u4e2a Excel"},
		{name: "second spreadsheet", query: "\u6458\u8981\u7b2c\u4e8c\u4e2a\u8868\u683c"},
		{name: "last pdf", query: "\u603b\u7ed3\u6700\u540e\u4e00\u4e2a PDF"},
		{name: "selected file", query: "\u603b\u7ed3\u5f53\u524d\u9009\u4e2d\u7684\u6587\u4ef6"},
		{name: "exact file name", query: "summarize proposal.pdf"},
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
				"typed ordinal requests",
				"file_type_rank",
				"extension_rank",
				`content_status "extracted"`,
				`"file_id":"file-1"`,
				`"file_id":"file-2"`,
				`"file_id":"file-3"`,
				`"file_id":"file-4"`,
				`"file_id":"file-5"`,
				`"file_id":"file-6"`,
				`"selected":true`,
			} {
				if !strings.Contains(content, want) {
					t.Fatalf("contextual read guidance missing %q in:\n%s", want, content)
				}
			}
			for _, unwanted := range []string{
				"resolved_targets_from_user_request",
				"target is already resolved",
				"Resolved internal target JSON for tool arguments only",
				"tool_argument_visibility_restriction",
			} {
				if strings.Contains(content, unwanted) {
					t.Fatalf("contextual read guidance still contains pre-resolved target marker %q in:\n%s", unwanted, content)
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
