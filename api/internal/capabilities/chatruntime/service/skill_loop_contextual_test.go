package service

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/memory"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestPartsFileIntentHelpersPreferModelIntentOverLegacyKeywords(t *testing.T) {
	managedFileQuery := "save this generated file to file management"
	if !isManagedFileCreateIntent(managedFileQuery) {
		t.Fatalf("test fixture query should match legacy managed file create intent")
	}
	answerParts := &chatRequestParts{
		Query:           managedFileQuery,
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "answer_or_explain_zgi_context"},
	}
	if partsRequestsManagedFileCreate(answerParts) {
		t.Fatal("managed file create intent used legacy keywords despite explicit model answer intent")
	}
	if partsRequestsTemporaryFileGenerateWithFallback(answerParts, "") {
		t.Fatal("temporary file generate intent used legacy keywords despite explicit model answer intent")
	}

	modelManagedParts := &chatRequestParts{
		Query:           "do it",
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "save_generated_file_to_file_management"},
	}
	if !partsRequestsManagedFileCreate(modelManagedParts) {
		t.Fatal("managed file create intent did not honor model intent")
	}

	continueParts := &chatRequestParts{
		Query:           managedFileQuery,
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "continue_previous_task"},
	}
	if partsRequestsManagedFileCreate(continueParts) {
		t.Fatal("managed file create intent used legacy keywords during continuation")
	}

	deleteQuery := "delete the first file"
	if !isFileDeleteIntent(deleteQuery) {
		t.Fatalf("test fixture query should match legacy file delete intent")
	}
	deleteAnswerParts := &chatRequestParts{
		Query:           deleteQuery,
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "answer_or_explain_zgi_context"},
	}
	if partsRequestsFileDeleteWithFallback(deleteAnswerParts, "") {
		t.Fatal("file delete intent used legacy keywords despite explicit model answer intent")
	}

	readParts := &chatRequestParts{
		Query:           "what is visible here?",
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "read_visible_file_content"},
	}
	if !partsRequestsFileReadWithFallback(readParts, "") {
		t.Fatal("file read intent did not honor model intent")
	}

	modelAgentContinuationParts := &chatRequestParts{
		Query:           "continue processing",
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "manage_agent_asset"},
	}
	if !partsRequestsContinuationWithFallback(modelAgentContinuationParts, "") {
		t.Fatal("explicit continuation command did not override stale model asset intent")
	}

	continueQuestionParts := &chatRequestParts{
		Query:           "what does continue mean",
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "answer_or_explain_zgi_context"},
	}
	if partsRequestsContinuationWithFallback(continueQuestionParts, "") {
		t.Fatal("continuation helper treated an explanatory question as a continue command")
	}
}

func TestRequestedManagedFileTargetsFromPartsUsesExplicitFilenamesOnly(t *testing.T) {
	implicitTargetQuery := "save two files, one txt and one svg"
	if got := requestedManagedFileTargetsFromQuery(implicitTargetQuery); len(got) != 0 {
		t.Fatalf("requestedManagedFileTargetsFromQuery() returned %d implicit targets, want 0", len(got))
	}

	answerParts := &chatRequestParts{
		Query:           implicitTargetQuery,
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "answer_or_explain_zgi_context"},
	}
	if got := requestedManagedFileTargetsFromParts(answerParts); len(got) != 0 {
		t.Fatalf("requestedManagedFileTargetsFromParts() returned %d targets for answer intent, want 0", len(got))
	}

	continueParts := &chatRequestParts{
		Query:           implicitTargetQuery,
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "continue_previous_task"},
	}
	if got := requestedManagedFileTargetsFromParts(continueParts); len(got) != 0 {
		t.Fatalf("requestedManagedFileTargetsFromParts() returned %d targets for continuation, want 0 without explicit save intent", len(got))
	}

	explicitTargetParts := &chatRequestParts{
		Query:           "save report.txt and chart.svg to file management",
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "save_generated_file_to_file_management"},
	}
	if got := requestedManagedFileTargetsFromParts(explicitTargetParts); len(got) != 2 {
		t.Fatalf("requestedManagedFileTargetsFromParts() returned %d explicit targets, want 2", len(got))
	}
}

func TestPartsConsoleNavigationHelpersPreferModelIntentOverLegacyKeywords(t *testing.T) {
	navigationQuery := "open /console/files"
	answerParts := &chatRequestParts{
		Query:           navigationQuery,
		Surface:         aiChatSurfaceContextualSidebar,
		SkillIDs:        []string{skills.SkillConsoleNavigator},
		SkillMode:       skillModeAuto,
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "answer_or_explain_zgi_context"},
	}
	if partsRequestsConsoleNavigationWithFallback(answerParts, "") {
		t.Fatal("console navigation intent used legacy keywords despite explicit model answer intent")
	}
	if targets := consoleNavigationResolvedTargetsForParts(answerParts); len(targets) != 0 {
		t.Fatalf("consoleNavigationResolvedTargetsForParts() = %#v, want no target for explicit model answer intent", targets)
	}
	strategy := contextualAIChatTurnStrategy(&PreparedChat{parts: answerParts})
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "answer_or_explain_zgi_context" || strategy.RouteRequired {
		t.Fatalf("strategy = %#v, want answer intent without route requirement", strategy)
	}

	legacyNavigationParts := &chatRequestParts{
		Query:     navigationQuery,
		Surface:   aiChatSurfaceContextualSidebar,
		SkillIDs:  []string{skills.SkillConsoleNavigator},
		SkillMode: skillModeAuto,
	}
	strategy = contextualAIChatTurnStrategy(&PreparedChat{parts: legacyNavigationParts})
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want default contextual strategy")
	}
	if strategy.Source != aiChatTurnStrategySourceDefault {
		t.Fatalf("Source = %q, want %q without no-contract navigation fallback; strategy=%#v", strategy.Source, aiChatTurnStrategySourceDefault, strategy)
	}
	if strategy.Intent != "answer_or_explain_zgi_context" || strategy.RouteRequired {
		t.Fatalf("strategy = %#v, want default model-led answer strategy without forced route", strategy)
	}

	modelNavigationParts := &chatRequestParts{
		Query:           navigationQuery,
		Surface:         aiChatSurfaceContextualSidebar,
		SkillIDs:        []string{skills.SkillConsoleNavigator},
		SkillMode:       skillModeAuto,
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "navigate_console_page", TargetPage: "/console/db"},
	}
	targets := consoleNavigationResolvedTargetsForParts(modelNavigationParts)
	if len(targets) != 1 {
		t.Fatalf("consoleNavigationResolvedTargetsForParts() = %#v, want one model target", targets)
	}
	target := targets[0]
	if target.Href != "/console/db" {
		t.Fatalf("target = %#v, want /console/db from model intent instead of query", target)
	}
	strategy = contextualAIChatTurnStrategy(&PreparedChat{parts: modelNavigationParts})
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "navigate_console_page" || strategy.TargetPage != "/console/db" || !strategy.RouteRequired {
		t.Fatalf("strategy = %#v, want navigation to /console/db", strategy)
	}
}

func TestAgentManagementExplicitDetailNavigationTargetPrefersModelTargetPage(t *testing.T) {
	parts := &chatRequestParts{
		Query: "open the old agent detail",
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent:     "manage_agent_asset",
			TargetPage: "/console/agents/model-agent-id/agent",
		},
	}

	target, ok := agentManagementExplicitDetailNavigationTarget(parts)
	if !ok {
		t.Fatal("agentManagementExplicitDetailNavigationTarget() ok = false, want model target page")
	}
	if target.Href != "/console/agents/model-agent-id/agent" {
		t.Fatalf("target href = %q, want model target page", target.Href)
	}
}

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

func TestSkillLoopFinalAnswerGuardDoesNotInferContinuationDeleteFromAnswerText(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesSemanticTestParts("\u7ee7\u7eed\u3002\u4efb\u52a1\u6807\u8bb0\uff1aSMOKE-CONTINUE", []consoleFilesTestFile{
			{ID: "file-saved-svg", Name: "smoke-continue.svg", Extension: "svg", MimeType: "image/svg+xml"},
			{ID: "file-saved-txt", Name: "smoke-continue.txt", Extension: "txt", MimeType: "text/plain"},
			{ID: "file-delete", Name: "old-third-file.txt", Extension: "txt", MimeType: "text/plain"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileManager, skills.SkillFileGenerator}
	prepared.parts.SkillMode = skillModeAuto

	guard := consoleFilesContinuationPendingDeleteFinalAnswerGuard(prepared.parts, nil)
	if guard == nil {
		t.Fatal("consoleFilesContinuationPendingDeleteFinalAnswerGuard() = nil")
	}
	answer := strings.Join([]string{
		"txt \u5df2\u4fdd\u5b58\uff1asmoke-continue.txt",
		"svg \u5df2\u4fdd\u5b58\uff1asmoke-continue.svg",
		"\u7b2c3\u4e2a\u6587\u4ef6\uff1aold-third-file.txt \u9700\u8981\u89c2\u5bdf\u51bb\u7ed3\u5e76\u5220\u9664\u3002",
		"\u662f\u5426\u9700\u8981\u6211\u7ee7\u7eed\u6267\u884c\u5220\u9664\u64cd\u4f5c\uff1f",
	}, "\n")
	_, blocked := guard(skillloop.FinalAnswerGuardRequest{Answer: answer})
	if blocked {
		t.Fatal("guard inferred a delete_file action from final answer text; want model/tool evidence to drive continuation")
	}
}

func TestSkillLoopFinalAnswerGuardAllowsContinuationDeleteSuccessFromMetadata(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"skill_invocations": []interface{}{
					map[string]interface{}{
						"kind":      "tool_call",
						"status":    "success",
						"skill_id":  skills.SkillFileManager,
						"tool_name": "delete_file",
						"arguments": map[string]interface{}{
							"file_id": "file-deleted",
						},
						"result": map[string]interface{}{
							"file_name": "deleted-third.svg",
						},
					},
				},
			},
		},
		parts: consoleFilesSemanticTestParts("\u7ee7\u7eed\u3002\u4efb\u52a1\u6807\u8bb0\uff1aSMOKE-CONTINUE", []consoleFilesTestFile{
			{ID: "file-new-third", Name: "new-third-after-delete.txt", Extension: "txt", MimeType: "text/plain"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileManager, skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want continuation delete guard")
	}
	_, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "TXT \u548c SVG \u5df2\u4fdd\u5b58\u5230\u6587\u4ef6\u7ba1\u7406\uff0c\u5e76\u5df2\u5220\u9664\u9501\u5b9a\u7684\u7b2c\u4e09\u4e2a\u6587\u4ef6 deleted-third.svg\u3002",
	})
	if blocked {
		t.Fatal("guard blocked a final answer after metadata recorded the successful frozen delete")
	}
}

func TestSkillLoopFinalAnswerGuardAllowsGenericDeleteSuccessFromMetadata(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"skill_invocations": []interface{}{
					map[string]interface{}{
						"kind":      "tool_call",
						"status":    "success",
						"skill_id":  skills.SkillFileManager,
						"tool_name": "delete_file",
						"arguments": map[string]interface{}{
							"file_id": "file-deleted",
						},
						"result": map[string]interface{}{
							"file_name": "deleted-third.svg",
						},
					},
				},
			},
		},
		parts: consoleFilesSemanticTestParts("\u7ee7\u7eed\u3002\u4efb\u52a1\u6807\u8bb0\uff1aSMOKE-CONTINUE", []consoleFilesTestFile{
			{ID: "file-new-third", Name: "new-third-after-delete.txt", Extension: "txt", MimeType: "text/plain"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileManager, skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want continuation delete guard")
	}
	_, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "TXT \u548c SVG \u5df2\u4fdd\u5b58\u5230\u6587\u4ef6\u7ba1\u7406\uff0c\u9501\u5b9a\u7684\u76ee\u6807\u6587\u4ef6\u4e5f\u5df2\u5220\u9664\u3002",
	})
	if blocked {
		t.Fatal("guard blocked a generic final answer after metadata recorded the successful frozen delete")
	}
}

func TestSkillLoopFinalAnswerGuardAllowsSecondConfirmationAfterFrozenContinuationDeleteSucceeds(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesSemanticTestParts("\u7ee7\u7eed\u3002\u4efb\u52a1\u6807\u8bb0\uff1aSMOKE-CONTINUE", []consoleFilesTestFile{
			{ID: "file-new-third", Name: "new-third-after-delete.txt", Extension: "txt", MimeType: "text/plain"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileManager, skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want continuation delete guard")
	}
	answer := "\u4e0a\u4e00\u4e2a .svg \u6587\u4ef6\u5df2\u7ecf\u5220\u9664\u3002\u5f53\u524d\u7b2c3\u4e2a\u6587\u4ef6\u53d8\u6210 new-third-after-delete.txt\uff0c\u662f\u5426\u786e\u8ba4\u5220\u9664\uff1f"
	_, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: answer,
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:  skills.SkillFileManager,
			ToolName: "delete_file",
			Arguments: map[string]interface{}{
				"file_id": "file-deleted",
			},
			Result: map[string]interface{}{
				"file_name": "deleted-third.svg",
			},
		}},
	})
	if blocked {
		t.Fatal("guard blocked a second delete confirmation after the frozen target was already deleted")
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

func TestSkillLoopAdditionalSystemMessagesDoesNotPreResolveRecentFileTarget(t *testing.T) {
	query := "\u8bf7\u57fa\u4e8e\u521a\u624d\u90a3\u4e2a\u6587\u4ef6\u63d0\u53d6\u7f34\u8d39\u8d26\u6237"
	prepared := &PreparedChat{
		parts: consoleFilesSnapshotTestParts(query, []consoleFilesTestFile{
			{ID: "file-1", Name: "invoice.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.ModelTurnIntent = &AIChatModelTurnIntent{
		Intent:     "read_visible_file_content",
		Confidence: 0.91,
	}
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
		`"file_id":"file-1"`,
		"read_file",
		"visible_files",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("contextual recent guidance missing %q in:\n%s", want, content)
		}
	}
	for _, unwanted := range []string{"resolved_targets_from_user_request", "target is already resolved"} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("contextual recent guidance still pre-resolved target %q in:\n%s", unwanted, content)
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
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "save_generated_file_to_file_management",
				Confidence: 0.91,
			},
		},
	}

	message, ok := contextualAIChatTurnStrategyMessage(prepared)
	if !ok {
		t.Fatal("contextualAIChatTurnStrategyMessage() ok = false, want true")
	}
	content := messageContentText(message.Content)
	for _, want := range []string{
		"ZGI AIChat turn task contract",
		`"intent":"save_generated_file_to_file_management"`,
		`"target_page":"/console/files"`,
		`"route_required":true`,
		skills.SkillConsoleNavigator,
		skills.SkillFileGenerator,
		skills.SkillFileManager,
		"exactly one temporary artifact",
		"asset_observation:file.create",
		"route_required and target_page as navigation context",
		"not as a fixed tool script",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("turn strategy missing %q in:\n%s", want, content)
		}
	}
	if strings.Contains(content, "do not answer, ask a question, or call another business tool before required_next_tool") {
		t.Fatalf("turn strategy still contains hard required_next_tool wording:\n%s", content)
	}
}

func TestContextualAIChatTurnStrategyUsesRecentArtifactWithoutRegeneration(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("\u628a\u8fd9\u4e2a\u6587\u4ef6\u4fdd\u5b58\u5230\u6587\u4ef6\u7ba1\u7406\u4e2d"),
	}
	prepared.parts.Surface = aiChatSurfaceContextualSidebar
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto
	routeRequired := false
	prepared.parts.ModelTurnIntent = &AIChatModelTurnIntent{
		Intent:        "save_generated_file_to_file_management",
		TargetPage:    "/console/files",
		RouteRequired: &routeRequired,
		Confidence:    0.95,
	}
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

	strategy := contextualManagedFileCreateStrategy(prepared.parts, &AIChatTurnStrategy{
		PrimarySkills: []string{skills.SkillFileGenerator},
	})
	strategy = enrichAIChatTurnStrategyPlannedTools(prepared.parts, strategy)
	if containsString(strategy.PrimarySkills, skills.SkillFileGenerator) {
		t.Fatalf("PrimarySkills = %#v, want recent-artifact save to remove producer skills", strategy.PrimarySkills)
	}
	if !containsString(strategy.PrimarySkills, skills.SkillFileManager) {
		t.Fatalf("PrimarySkills = %#v, want file-manager", strategy.PrimarySkills)
	}
	for _, tool := range strategy.PlannedTools {
		if tool.SkillID == skills.SkillFileGenerator {
			t.Fatalf("PlannedTools = %#v, want no generator tool for recent-artifact save", strategy.PlannedTools)
		}
	}
}

func TestContextualAIChatTurnStrategyGeneratesNewManagedFileDespiteRecentArtifact(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("\u8bf7\u751f\u6210\u4e00\u4e2a SVG \u6587\u4ef6\u5e76\u4fdd\u5b58\u5230\u6587\u4ef6\u7ba1\u7406"),
	}
	prepared.parts.Surface = aiChatSurfaceContextualSidebar
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.RecentGeneratedArtifacts = []map[string]interface{}{{
		"tool_file_id": "tool-recent-1",
		"filename":     "previous.svg",
	}}
	prepared.parts.ModelTurnIntent = &AIChatModelTurnIntent{
		Intent:                  "save_generated_file_to_file_management",
		RecommendedCapabilities: []string{"file_artifact"},
		Confidence:              0.91,
	}

	strategy := contextualAIChatTurnStrategyFromParts(prepared.parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if strategy.ArtifactSource == "recent_generated_file" {
		t.Fatalf("ArtifactSource = %q, want new artifact generation", strategy.ArtifactSource)
	}
	for _, want := range []string{skills.SkillFileGenerator, skills.SkillFileManager} {
		if !containsString(strategy.PrimarySkills, want) {
			t.Fatalf("PrimarySkills = %#v, want %s", strategy.PrimarySkills, want)
		}
	}

	if strategy.ToolChoiceMode != aiChatTurnToolChoiceModelDecides {
		t.Fatalf("ToolChoiceMode = %q, want %q", strategy.ToolChoiceMode, aiChatTurnToolChoiceModelDecides)
	}
}

func TestContextualAIChatTurnStrategyClassifiesTemporaryFileGeneration(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "\u7b2c\u4e00\u9636\u6bb5\u53ea\u751f\u6210\u4e00\u4e2a\u4e34\u65f6 SVG \u6587\u4ef6\uff0c\u7b49\u5f85\u6211\u8bf4\u201c\u7ee7\u7eed\u201d\uff0c\u4e0d\u8981\u4fdd\u5b58\u5230\u6587\u4ef6\u7ba1\u7406\u3002",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/files",
			SkillIDs: []string{
				skills.SkillConsoleNavigator,
				skills.SkillFileGenerator,
				skills.SkillFileManager,
			},
			SkillMode: skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "generate_temporary_file_artifact",
				Confidence: 0.91,
			},
		},
	}

	message, ok := contextualAIChatTurnStrategyMessage(prepared)
	if !ok {
		t.Fatal("contextualAIChatTurnStrategyMessage() ok = false, want true")
	}
	content := messageContentText(message.Content)
	for _, want := range []string{
		`"intent":"generate_temporary_file_artifact"`,
		`"primary_skills":["file-generator"]`,
		"do not call file-manager/save_file_to_management",
		"generated_files metadata records the temporary artifact",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("temporary generation strategy missing %q in:\n%s", want, content)
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

func TestContextualAIChatTurnStrategyUsesFileGeneratorForGenericSVGWhenChartGeneratorEnabled(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:   "\u751f\u6210\u4e00\u4e2a\u4e34\u65f6 SVG \u6587\u4ef6\uff0c\u5185\u5bb9\u753b\u4e00\u4e2a\u7eff\u8272\u5706\u70b9\uff0c\u4e0d\u8981\u4fdd\u5b58\u5230\u6587\u4ef6\u7ba1\u7406\u3002",
			Surface: aiChatSurfaceContextualSidebar,
			SkillIDs: []string{
				skills.SkillFileGenerator,
				skills.SkillChartGenerator,
			},
			SkillMode: skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "generate_temporary_file_artifact",
				Confidence: 0.91,
			},
		},
	}

	strategy := contextualAIChatTurnStrategyFromParts(prepared.parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if got, want := strategy.PrimarySkills, []string{skills.SkillFileGenerator}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("PrimarySkills = %#v, want %#v", got, want)
	}
	metadata := streamingMessageMetadataWithTaskID(prepared.parts, "task-svg")
	plan := metadata["operation_plan"].(map[string]interface{})
	stepStatus := plan["step_status"].(map[string]interface{})
	if _, ok := stepStatus["skill:"+skills.SkillChartGenerator]; ok {
		t.Fatalf("operation plan step_status includes chart-generator for generic SVG: %#v", stepStatus)
	}
}

func TestTemporaryFileGenerateIntentIgnoresReadOnlyNegativeOperations(t *testing.T) {
	query := "SMOKE-ORDER: \u53ea\u56de\u7b54\u5f53\u524d\u6587\u4ef6\u7ba1\u7406\u9875\u53ef\u89c1\u6587\u4ef6\u603b\u6570\u548c\u524d\u4e24\u4e2a\u6587\u4ef6\u540d\uff0c\u4e0d\u8981\u521b\u5efa\u3001\u4fdd\u5b58\u3001\u5220\u9664\u6216\u5bfc\u822a\u3002"
	if isTemporaryFileGenerateIntent(query) {
		t.Fatal("isTemporaryFileGenerateIntent() = true, want false for read-only request with negative operations")
	}
}

func TestTemporaryFileGenerateFinalAnswerGuardRequiresArtifactTool(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:   "\u53ea\u751f\u6210\u4e00\u4e2a\u4e34\u65f6 SVG \u6587\u4ef6\uff0c\u4e0d\u8981\u4fdd\u5b58\u5230\u6587\u4ef6\u7ba1\u7406\u3002",
			Surface: aiChatSurfaceContextualSidebar,
			SkillIDs: []string{
				skills.SkillFileGenerator,
				skills.SkillFileManager,
			},
			SkillMode: skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:                  "generate_temporary_file_artifact",
				RecommendedCapabilities: []string{"file_artifact"},
			},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{}},
	}

	guard := skillLoopTemporaryFileGenerateFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopTemporaryFileGenerateFinalAnswerGuard() = nil, want guard")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "\u5df2\u751f\u6210\u4e34\u65f6\u6587\u4ef6 temporary.svg",
	})
	if !blocked {
		t.Fatal("guard allowed a temporary generation success claim before artifact tool success")
	}
	if result.SkillID != skills.SkillFileGenerator || result.ToolName != "generate_file" {
		t.Fatalf("guard result = %#v, want file-generator/generate_file", result)
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "\u5df2\u751f\u6210\u4e34\u65f6\u6587\u4ef6 temporary.svg",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:  skills.SkillFileGenerator,
			ToolName: "generate_file",
		}},
	})
	if blocked {
		t.Fatal("guard blocked after file-generator/generate_file succeeded")
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
	prepared.parts.ModelTurnIntent = &AIChatModelTurnIntent{
		Intent:                  "read_visible_file_content",
		TargetPage:              "/console/files",
		RecommendedCapabilities: []string{"visible_file_content"},
		Confidence:              0.96,
	}

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
	routeRequired := true
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
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:                  "save_generated_file_to_file_management",
				TargetPage:              "/console/files",
				RouteRequired:           &routeRequired,
				RecommendedCapabilities: []string{"file_artifact"},
				Confidence:              0.95,
			},
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "save_generated_file_to_file_management" {
		t.Fatalf("Intent = %q, want save_generated_file_to_file_management", strategy.Intent)
	}
	if strategy.ToolChoiceMode != aiChatTurnToolChoiceModelDecides {
		t.Fatalf("ToolChoiceMode = %q, want %q; strategy=%#v", strategy.ToolChoiceMode, aiChatTurnToolChoiceModelDecides, strategy)
	}
	if strategy.TargetPage != "/console/files" || !strategy.RouteRequired {
		t.Fatalf("target/route = %q/%v, want /console/files/true; strategy=%#v", strategy.TargetPage, strategy.RouteRequired, strategy)
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
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "navigate_console_page",
				TargetPage: "/console/files",
				Confidence: 0.91,
			},
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
		t.Fatalf("target/route = %q/%v, want /console/files/true; strategy=%#v", strategy.TargetPage, strategy.RouteRequired, strategy)
	}
}

func TestContextualAIChatTurnStrategyDoesNotNavigateForCurrentFilesPageQuestion(t *testing.T) {
	parts := consoleFilesSnapshotTestParts("show me the current files page table total count", []consoleFilesTestFile{
		{ID: "file-1", Name: "report.txt", Extension: "txt", MimeType: "text/plain"},
	})
	parts.Surface = aiChatSurfaceContextualSidebar
	parts.SkillIDs = []string{skills.SkillConsoleNavigator, skills.SkillFileReader}
	parts.SkillMode = skillModeAuto
	routeRequired := false
	parts.ModelTurnIntent = &AIChatModelTurnIntent{
		Intent:        "answer_or_explain_zgi_context",
		TargetPage:    "/console/files",
		RouteRequired: &routeRequired,
		Confidence:    0.96,
	}
	setConsoleFilesPageTestMetadata(parts, map[string]interface{}{
		"total_file_count":   86,
		"current_page":       1,
		"total_pages":        9,
		"visible_file_count": 1,
		"files_query_status": "success",
		"context_ready":      true,
	})
	prepared := &PreparedChat{parts: parts}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "answer_or_explain_zgi_context" {
		t.Fatalf("Intent = %q, want answer_or_explain_zgi_context; strategy=%#v", strategy.Intent, strategy)
	}
	if strategy.RouteRequired {
		t.Fatalf("RouteRequired = true, want false for already visible files page")
	}
}

func TestSkillLoopUsesPlainStreamForPassiveContextAnswer(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "你能做什么？",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/work/chat",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillFileReader},
			SkillMode:      skillModeAuto,
		},
	}

	if !skillLoopShouldUsePlainStreamForPassiveAnswer(prepared) {
		t.Fatalf("skillLoopShouldUsePlainStreamForPassiveAnswer() = false, want true")
	}
}

func TestSkillLoopKeepsAgentActionsInSkillLoop(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "创建一个新的智能体，名称叫测试助手",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement},
			SkillMode:      skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "manage_agent_asset",
				Confidence: 0.92,
			},
		},
	}

	if skillLoopShouldUsePlainStreamForPassiveAnswer(prepared) {
		t.Fatalf("skillLoopShouldUsePlainStreamForPassiveAnswer() = true, want false for agent action")
	}
}

func TestContextualAgentManagementStrategyUsesBackendEvidenceWithoutObservationStep(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "delete the first visible Agent, then create and configure a new Agent",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement},
			SkillMode:      skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "manage_agent_asset",
				Confidence: 0.93,
			},
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if got := strategy.Intent; got != "manage_agent_asset" {
		t.Fatalf("Intent = %q, want manage_agent_asset", got)
	}
	criteria := strings.Join(strategy.SuccessCriteria, "\n")
	if !strings.Contains(criteria, "agent-management tool results and get_agent_config reads are authoritative backend evidence") {
		t.Fatalf("SuccessCriteria = %#v, want backend evidence criterion", strategy.SuccessCriteria)
	}
	for _, point := range strategy.ObservationPoints {
		if strings.Contains(point, "asset_observation") || strings.Contains(point, "agent_page_context") {
			t.Fatalf("ObservationPoints = %#v, want no Agent page observation dependency", strategy.ObservationPoints)
		}
	}

	plan := operationPlanFromTurnStrategy("task-agent-backend-evidence", prepared.parts, strategy)
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if got := stringFromAny(step["id"]); got == "observe" {
			t.Fatalf("operation_plan steps = %#v, want no observe step for Agent backend-evidence flow", plan["steps"])
		}
	}
}

func TestContextualAIChatTurnStrategyUsesModelIntentBeforeRuleFallback(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "please handle this page task",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/files",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement, skills.SkillFileReader},
			SkillMode:      skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "manage_agent_asset",
				Confidence: 0.92,
			},
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "manage_agent_asset" {
		t.Fatalf("Intent = %q, want manage_agent_asset", strategy.Intent)
	}
	if strategy.Source != aiChatTurnStrategySourceModelIntent {
		t.Fatalf("Source = %q, want %q", strategy.Source, aiChatTurnStrategySourceModelIntent)
	}
	if !slices.Contains(strategy.PrimarySkills, skills.SkillConsoleNavigator) {
		t.Fatalf("PrimarySkills = %#v, want console navigator for route", strategy.PrimarySkills)
	}
	if !slices.Contains(strategy.SupportingSkills, skills.SkillAgentManagement) {
		t.Fatalf("SupportingSkills = %#v, want agent management while off the Agent page", strategy.SupportingSkills)
	}
}

func TestContextualAIChatTurnStrategyKeepsAgentModelIntentBeforeFileFallbackWhenSkillLoadsAfterRoute(t *testing.T) {
	routeRequired := true
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "到文件管理读取第一个文件，然后到智能体页面创建一个故事讲述者智能体，让它能生成文件和上传文件",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/files",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillFileReader, skills.SkillFileGenerator},
			SkillMode:      skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:        "manage_agent_asset",
				TaskType:      "complex_multi_step_workflow",
				TargetPage:    "/console/agents",
				RouteRequired: &routeRequired,
				Confidence:    0.91,
				Phases: []string{
					"read the requested file",
					"navigate to the Agents page",
					"create and configure the requested Agent",
				},
			},
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "manage_agent_asset" {
		t.Fatalf("Intent = %q, want manage_agent_asset; source=%s/%s", strategy.Intent, strategy.Source, strategy.SourceReason)
	}
	if strategy.Source != aiChatTurnStrategySourceModelIntent {
		t.Fatalf("Source = %q, want %q", strategy.Source, aiChatTurnStrategySourceModelIntent)
	}
	if strategy.TargetPage != "/console/agents" {
		t.Fatalf("TargetPage = %q, want /console/agents", strategy.TargetPage)
	}
	if !strategy.RouteRequired {
		t.Fatal("RouteRequired = false, want true before Agent page loads")
	}
	if strategy.Intent == "save_generated_file_to_file_management" || strategy.TargetPage == "/console/files" {
		t.Fatalf("strategy fell back to managed file create: %#v", strategy)
	}
}

func TestContextualAIChatTurnStrategyKeepsAgentIntentWithFileFirstPhase(t *testing.T) {
	routeRequired := true
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "read the first file, summarize it, then create and configure an Agent from that summary",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillFileReader, skills.SkillAgentManagement},
			SkillMode:      skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:        "manage_agent_asset",
				TaskType:      "agent_lifecycle_and_configuration",
				TargetPage:    "/console/files",
				RouteRequired: &routeRequired,
				Confidence:    0.9,
				RecommendedCapabilities: []string{
					"page_navigation",
					"visible_file_content",
					"agent.model_selection",
					"agent.system_prompt",
				},
				Phases: []string{
					"Navigate to File Management and read the first visible file",
					"Navigate to Agents and create a new Agent",
					"Configure the Agent model and prompt from the file summary",
				},
			},
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "manage_agent_asset" {
		t.Fatalf("Intent = %q, want manage_agent_asset; source=%s/%s", strategy.Intent, strategy.Source, strategy.SourceReason)
	}
	if strategy.TargetPage != "/console/files" {
		t.Fatalf("TargetPage = %q, want /console/files for the file-read precondition", strategy.TargetPage)
	}
	if !slices.Contains(strategy.SupportingSkills, skills.SkillFileReader) {
		t.Fatalf("SupportingSkills = %#v, want file-reader for file-first Agent setup", strategy.SupportingSkills)
	}
}

func TestContextualAIChatTurnStrategyDoesNotUseLegacyFallbackWhenModelIntentUnsupported(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "create a new agent named smoke assistant",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement},
			SkillMode:      skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "unsupported_intent",
				Confidence: 0.99,
			},
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "answer_or_explain_zgi_context" {
		t.Fatalf("Intent = %q, want safe model-intent default answer strategy; strategy=%#v", strategy.Intent, strategy)
	}
	if strategy.Source != aiChatTurnStrategySourceModelIntent {
		t.Fatalf("Source = %q, want %q", strategy.Source, aiChatTurnStrategySourceModelIntent)
	}
	if !strings.Contains(strategy.SourceReason, "model_intent_not_accepted") {
		t.Fatalf("SourceReason = %q, want model_intent_not_accepted reason", strategy.SourceReason)
	}
	if slices.Contains(strategy.PrimarySkills, skills.SkillAgentManagement) {
		t.Fatalf("PrimarySkills = %#v, want no agent-management legacy fallback", strategy.PrimarySkills)
	}
}

func TestContextualAIChatTurnStrategyUsesGenericContractForUnsupportedActionableModelIntent(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "create a story agent and let it generate files",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement, skills.SkillFileGenerator},
			SkillMode:      skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:                  "answer_or_explain_zgi_context",
				RawIntent:               "configure_story_agent",
				LowConfidence:           true,
				AssetEffect:             "update",
				AssetRisk:               "medium",
				RecommendedCapabilities: []string{"agent.skill_backed_capability:file generation:bind", "agent.accept_uploaded_files"},
				Phases:                  []string{"create or select target agent", "configure writing behavior", "enable file generation"},
				EvidenceRequired:        []string{"successful Agent config mutation", "post-update Agent config read"},
				CompletionCriteria:      []string{"the Agent is configured with file generation capability"},
			},
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "model_turn_contract" {
		t.Fatalf("Intent = %q, want model_turn_contract; strategy=%#v", strategy.Intent, strategy)
	}
	if strategy.Source != aiChatTurnStrategySourceModelIntent {
		t.Fatalf("Source = %q, want %q", strategy.Source, aiChatTurnStrategySourceModelIntent)
	}
	if skillLoopShouldUsePlainStreamForPassiveAnswer(prepared) {
		t.Fatal("skillLoopShouldUsePlainStreamForPassiveAnswer() = true, want skill loop for actionable model contract")
	}
	if !slices.Contains(strategy.SupportingSkills, skills.SkillAgentManagement) {
		t.Fatalf("SupportingSkills = %#v, want agent-management", strategy.SupportingSkills)
	}
	if !agentCapabilityGoalListHasCapability(strategy.CapabilityGoals, agentCapabilitySkillBacked) {
		t.Fatalf("CapabilityGoals = %#v, want skill-backed Agent capability goal", strategy.CapabilityGoals)
	}
	plan := operationPlanFromTurnStrategy("task-1", prepared.parts, strategy)
	if plan == nil {
		t.Fatal("operationPlanFromTurnStrategy() = nil, want phase-only model contract plan")
	}
	if got := stringFromAny(plan["tool_choice_mode"]); got != aiChatTurnToolChoiceModelDecides {
		t.Fatalf("tool_choice_mode = %q, want model_decides", got)
	}
	if got := stringFromAny(plan["intent"]); got != "model_turn_contract" {
		t.Fatalf("plan intent = %q, want model_turn_contract", got)
	}
	if !operationPlanCapabilityGoalsContainForTest(mapSliceFromAny(plan["capability_goals"]), agentCapabilitySkillBacked) {
		t.Fatalf("plan capability goals = %#v, want skill-backed goal", plan["capability_goals"])
	}
}

func TestContextualAIChatTurnStrategyKeepsPassiveAnswerWhenClassifierFailsOnAgentPage(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:                "what can you do here?",
			Surface:              aiChatSurfaceContextualSidebar,
			RuntimeContext:       "route=/console/agents/agent-1/agent",
			SkillIDs:             []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement},
			SkillMode:            skillModeAuto,
			ModelTurnIntentError: "empty classifier content",
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "answer_or_explain_zgi_context" {
		t.Fatalf("Intent = %q, want answer_or_explain_zgi_context; strategy=%#v", strategy.Intent, strategy)
	}
	if strategy.Source != aiChatTurnStrategySourceDefault {
		t.Fatalf("Source = %q, want %q", strategy.Source, aiChatTurnStrategySourceDefault)
	}
	if slices.Contains(strategy.PrimarySkills, skills.SkillAgentManagement) {
		t.Fatalf("PrimarySkills = %#v, want no agent-management primary skill for passive answer", strategy.PrimarySkills)
	}
	if !skillLoopShouldUsePlainStreamForPassiveAnswer(prepared) {
		t.Fatal("skillLoopShouldUsePlainStreamForPassiveAnswer() = false, want true")
	}
}

func TestParseModelTurnIntentContentAcceptsLooseClassifierJSON(t *testing.T) {
	intent, err := parseModelTurnIntentContent("```json\n{\"intent\":\"answer\",\"confidence\":\"0.91\",\"approval\":false,\"route_required\":\"true\"}\n```")
	if err != nil {
		t.Fatalf("parseModelTurnIntentContent() error = %v", err)
	}
	if got := normalizeModelTurnIntent(intent.Intent); got != "answer_or_explain_zgi_context" {
		t.Fatalf("Intent = %q, want answer_or_explain_zgi_context", got)
	}
	if intent.Confidence != 0.91 {
		t.Fatalf("Confidence = %v, want 0.91", intent.Confidence)
	}
	if intent.Approval != "none" {
		t.Fatalf("Approval = %q, want none", intent.Approval)
	}
	if intent.RouteRequired == nil || !*intent.RouteRequired {
		t.Fatalf("RouteRequired = %#v, want true", intent.RouteRequired)
	}
}

func TestParseModelTurnIntentContentAcceptsVisibleIndex(t *testing.T) {
	intent, err := parseModelTurnIntentContent(`{"intent":"manage_agent_asset","target_visible_index":"2","confidence":0.88}`)
	if err != nil {
		t.Fatalf("parseModelTurnIntentContent() error = %v", err)
	}
	if intent.TargetVisibleIndex != 2 {
		t.Fatalf("TargetVisibleIndex = %d, want 2", intent.TargetVisibleIndex)
	}
}

func TestModelTurnTaskContractKeepsUnsupportedIntentAsLowConfidenceContract(t *testing.T) {
	intent, err := parseModelTurnIntentContent(`{
		"intent":"inspect_and_prepare_agent_story_writer",
		"task_type":"agent_configuration",
		"phases":["understand requested Agent outcome","configure Agent capabilities","verify Agent configuration"],
		"evidence_required":["Agent config readback"],
		"recommended_capabilities":["agent.model_selection","agent.skill_backed_capability:file generation"],
		"completion_criteria":["Agent config matches the user's requested outcome"],
		"confidence":0.86
	}`)
	if err != nil {
		t.Fatalf("parseModelTurnIntentContent() error = %v", err)
	}
	finalizeModelTurnIntent(intent)
	if intent.Intent != "answer_or_explain_zgi_context" {
		t.Fatalf("Intent = %q, want compatibility fallback answer intent", intent.Intent)
	}
	if intent.RawIntent != "inspect_and_prepare_agent_story_writer" {
		t.Fatalf("RawIntent = %q, want original unsupported label", intent.RawIntent)
	}
	if !intent.LowConfidence {
		t.Fatal("LowConfidence = false, want true for unsupported compatibility label")
	}
	contract := modelTurnIntentTaskContract(intent)
	if got := stringFromAny(contract["raw_intent_label"]); got != "inspect_and_prepare_agent_story_writer" {
		t.Fatalf("raw_intent_label = %q, want original label; contract=%#v", got, contract)
	}
	if got := stringSliceFromAny(contract["phases"]); !slices.Contains(got, "configure Agent capabilities") {
		t.Fatalf("phases = %#v, want preserved model phases", got)
	}
	if got := stringSliceFromAny(contract["recommended_capabilities"]); !slices.Contains(got, "agent.skill_backed_capability:file generation") {
		t.Fatalf("recommended_capabilities = %#v, want preserved model capabilities", got)
	}
}

func TestModelTurnTaskContractKeepsLowConfidencePlanOnMainPath(t *testing.T) {
	intent, err := parseModelTurnIntentContent(`{
		"intent":"manage_agent_asset",
		"task_type":"agent_configuration",
		"phases":["create or identify the target Agent","configure requested Agent capabilities","verify saved configuration"],
		"evidence_required":["Agent config readback","successful tool results"],
		"recommended_capabilities":["agent.model_selection","agent.skill_backed_capability:file generation"],
		"completion_criteria":["final answer is grounded in Agent config evidence"],
		"confidence":0.31,
		"reason":"The user asks for Agent work but the page context may be incomplete."
	}`)
	if err != nil {
		t.Fatalf("parseModelTurnIntentContent() error = %v", err)
	}
	finalizeModelTurnIntent(intent)
	if !intent.LowConfidence {
		t.Fatalf("LowConfidence = false, want true; intent=%#v", intent)
	}
	parts := &chatRequestParts{
		Query:           "create an Agent and configure it",
		Surface:         aiChatSurfaceContextualSidebar,
		RuntimeContext:  "route=/console/agents",
		SkillIDs:        []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement},
		SkillMode:       skillModeAuto,
		ModelTurnIntent: intent,
	}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want model turn strategy")
	}
	if strategy.Source != aiChatTurnStrategySourceModelIntent {
		t.Fatalf("Source = %q, want model intent; strategy=%#v", strategy.Source, strategy)
	}
	if !strategy.LowConfidence {
		t.Fatalf("strategy.LowConfidence = false, want true; strategy=%#v", strategy)
	}
	if strategy.Intent != "model_turn_contract" {
		t.Fatalf("Intent = %q, want model_turn_contract", strategy.Intent)
	}
	if strategy.CompatibilityIntent != "manage_agent_asset" {
		t.Fatalf("CompatibilityIntent = %q, want manage_agent_asset", strategy.CompatibilityIntent)
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-low-confidence-contract")
	contract := mapFromOperationContext(metadata["turn_task_contract"])
	if !operationPlanBoolValue(contract["low_confidence"]) {
		t.Fatalf("metadata turn_task_contract = %#v, want low confidence marker", contract)
	}
	if got := stringFromAny(contract["intent_label"]); got != "manage_agent_asset" {
		t.Fatalf("turn_task_contract.intent_label = %q, want manage_agent_asset", got)
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["intent"]); got != "model_turn_contract" {
		t.Fatalf("operation_plan.intent = %q, want model_turn_contract", got)
	}
	planContract := mapFromOperationContext(plan["task_contract"])
	if !operationPlanBoolValue(planContract["low_confidence"]) {
		t.Fatalf("operation_plan.task_contract = %#v, want low confidence marker", planContract)
	}
	if got := stringFromAny(planContract["intent_label"]); got != "manage_agent_asset" {
		t.Fatalf("operation_plan.task_contract.intent_label = %q, want manage_agent_asset", got)
	}
	if got := stringFromAny(planContract["execution_intent"]); got != "model_turn_contract" {
		t.Fatalf("operation_plan.task_contract.execution_intent = %q, want model_turn_contract", got)
	}

	state := currentTurnAuthoritativeStatePayload(&runtimemodel.Message{
		Query:    parts.Query,
		Metadata: metadata,
	})
	if stateContract := mapFromOperationContext(state["turn_task_contract"]); stringFromAny(stateContract["intent_label"]) != "manage_agent_asset" {
		t.Fatalf("authoritative state contract = %#v, want preserved task contract", stateContract)
	}
}

func TestModelTurnTaskContractSanitizesImplicitBindingHints(t *testing.T) {
	intent, err := parseModelTurnIntentContent(`{
		"intent":"manage_agent_asset",
		"task_type":"multi_step_workflow",
		"phases":["navigate_to_file_management","read_first_md_file","update_agent_knowledge_binding"],
		"evidence_required":["visible_file_content of the first MD file","current knowledge bindings of the agent"],
		"recommended_capabilities":["visible_file_content","agent.knowledge_binding","agent.system_prompt"],
		"completion_criteria":["Agent knowledge bindings updated to include the new chapter"],
		"asset_effect":"update",
		"confidence":0.88
	}`)
	if err != nil {
		t.Fatalf("parseModelTurnIntentContent() error = %v", err)
	}
	finalizeModelTurnIntent(intent)

	if slices.Contains(intent.RecommendedCapabilities, agentCapabilityKnowledgeBinding) {
		t.Fatalf("RecommendedCapabilities = %#v, want implicit knowledge binding dropped", intent.RecommendedCapabilities)
	}
	if !slices.Contains(intent.RecommendedCapabilities, agentCapabilitySystemPrompt) {
		t.Fatalf("RecommendedCapabilities = %#v, want system prompt capability preserved", intent.RecommendedCapabilities)
	}
	if !slices.Contains(intent.Phases, "update_agent_context") {
		t.Fatalf("Phases = %#v, want neutral agent context phase", intent.Phases)
	}
	for _, values := range [][]string{intent.Phases, intent.EvidenceRequired, intent.CompletionCriteria} {
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), "knowledge_binding") || strings.Contains(strings.ToLower(value), "knowledge bindings") {
				t.Fatalf("sanitized advisory value still contains binding wording: %q; intent=%#v", value, intent)
			}
		}
	}

	parts := &chatRequestParts{
		Query:           "continue the first md file and update the current Agent with the new chapter",
		Surface:         aiChatSurfaceContextualSidebar,
		RuntimeContext:  "route=/console/agents/agent-1/agent",
		SkillIDs:        []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement, skills.SkillFileReader},
		SkillMode:       skillModeAuto,
		ModelTurnIntent: intent,
	}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	plan := operationPlanFromTurnStrategy("task-implicit-binding-sanitized", parts, strategy)
	if operationPlanCapabilityGoalsContainForTest(mapSliceFromAny(plan["capability_goals"]), agentCapabilityKnowledgeBinding) {
		t.Fatalf("capability_goals = %#v, want no implicit knowledge binding goal; plan=%#v", plan["capability_goals"], plan)
	}
	if !operationPlanCapabilityGoalsContainForTest(mapSliceFromAny(plan["capability_goals"]), agentCapabilitySystemPrompt) {
		t.Fatalf("capability_goals = %#v, want system prompt goal; plan=%#v", plan["capability_goals"], plan)
	}
}

func TestModelTurnTaskContractUsesGenericPathForRawCompatibilityAlias(t *testing.T) {
	intent, err := parseModelTurnIntentContent(`{
		"intent":"create_agent",
		"task_type":"agent_creation",
		"phases":["create the requested Agent","verify created Agent evidence"],
		"evidence_required":["create_agent tool result"],
		"recommended_capabilities":["agent.model_selection"],
		"completion_criteria":["final answer names the created Agent only after successful evidence"],
		"confidence":0.91
	}`)
	if err != nil {
		t.Fatalf("parseModelTurnIntentContent() error = %v", err)
	}
	finalizeModelTurnIntent(intent)
	if intent.Intent != "manage_agent_asset" {
		t.Fatalf("Intent = %q, want normalized manage_agent_asset", intent.Intent)
	}
	if intent.RawIntent != "create_agent" {
		t.Fatalf("RawIntent = %q, want create_agent", intent.RawIntent)
	}
	if intent.LowConfidence {
		t.Fatalf("LowConfidence = true, want false for confident compatibility alias")
	}
	parts := &chatRequestParts{
		Query:           "create an Agent",
		Surface:         aiChatSurfaceContextualSidebar,
		RuntimeContext:  "route=/console/agents",
		SkillIDs:        []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement},
		SkillMode:       skillModeAuto,
		ModelTurnIntent: intent,
	}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if strategy.Intent != "model_turn_contract" {
		t.Fatalf("Intent = %q, want model_turn_contract; strategy=%#v", strategy.Intent, strategy)
	}
	if strategy.CompatibilityIntent != "manage_agent_asset" {
		t.Fatalf("CompatibilityIntent = %q, want manage_agent_asset", strategy.CompatibilityIntent)
	}
	plan := operationPlanFromTurnStrategy("task-create-agent-alias", parts, strategy)
	if got := stringFromAny(plan["intent"]); got != "model_turn_contract" {
		t.Fatalf("operation_plan.intent = %q, want model_turn_contract", got)
	}
	planContract := mapFromOperationContext(plan["task_contract"])
	if got := stringFromAny(planContract["intent_label"]); got != "manage_agent_asset" {
		t.Fatalf("operation_plan.task_contract.intent_label = %q, want manage_agent_asset", got)
	}
	if got := stringFromAny(planContract["execution_intent"]); got != "model_turn_contract" {
		t.Fatalf("operation_plan.task_contract.execution_intent = %q, want model_turn_contract", got)
	}
}

func TestParseModelTurnIntentMessageUsesReasoningJSONWhenContentEmpty(t *testing.T) {
	intent, source, err := parseModelTurnIntentMessage(adapter.Message{
		ReasoningContent: `We need classify this request.
{"intent":"answer_or_explain_zgi_context","task_type":"agent_prompt_review","confidence":0.91,"approval":"none"}`,
	})
	if err != nil {
		t.Fatalf("parseModelTurnIntentMessage() error = %v", err)
	}
	if got := normalizeModelTurnIntent(intent.Intent); got != "answer_or_explain_zgi_context" {
		t.Fatalf("Intent = %q, want answer_or_explain_zgi_context", got)
	}
	if intent.TaskType != "agent_prompt_review" {
		t.Fatalf("TaskType = %q, want agent_prompt_review", intent.TaskType)
	}
	if !strings.Contains(source, `"intent"`) {
		t.Fatalf("source = %q, want reasoning JSON preview", source)
	}
}

func TestParseModelTurnIntentMessageRejectsReasoningOnlyProse(t *testing.T) {
	_, source, err := parseModelTurnIntentMessage(adapter.Message{
		ReasoningContent: "We need to classify this as an agent request, but I will not emit JSON.",
	})
	if err == nil {
		t.Fatal("parseModelTurnIntentMessage() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "reasoning content did not contain json") {
		t.Fatalf("error = %q, want reasoning json error", err.Error())
	}
	if !strings.Contains(source, "We need to classify") {
		t.Fatalf("source = %q, want reasoning preview", source)
	}
}

func TestParseModelTurnIntentContentRejectsPlainReasoningText(t *testing.T) {
	_, err := parseModelTurnIntentContent("We need to classify this request before returning JSON.")
	if err == nil {
		t.Fatal("parseModelTurnIntentContent() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "empty classifier content") {
		t.Fatalf("error = %q, want empty classifier content", err.Error())
	}
}

func TestContextualAIChatTurnStrategyUsesModelTurnPlanForExactAgentRuntime(t *testing.T) {
	intent, err := parseModelTurnIntentContent(`{
		"intent": "answer_or_explain_zgi_context",
		"task_type": "agent_config_analysis",
		"phases": ["confirm exact Agent runtime configuration", "analyze the actual prompt"],
		"evidence_required": ["actual system prompt", "runtime model", "enabled skills"],
		"recommended_capabilities": ["exact_agent_runtime"],
		"completion_criteria": ["answer is grounded in actual Agent runtime evidence"],
		"needs_exact_agent_runtime": true,
		"current_context_may_be_summary": true,
		"confidence": 0.93,
		"approval": "none"
	}`)
	if err != nil {
		t.Fatalf("parseModelTurnIntentContent() error = %v", err)
	}
	intent.Intent = normalizeModelTurnIntent(intent.Intent)
	intent.Phases = normalizeModelTurnPlanStrings(intent.Phases, 8, 160)
	intent.EvidenceRequired = normalizeModelTurnPlanStrings(intent.EvidenceRequired, 10, 160)
	intent.RecommendedCapabilities = normalizeModelTurnPlanStrings(intent.RecommendedCapabilities, 10, 120)
	intent.CompletionCriteria = normalizeModelTurnPlanStrings(intent.CompletionCriteria, 8, 180)

	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:           "based on the actual prompt, suggest improvements for this agent",
			Surface:         aiChatSurfaceContextualSidebar,
			RuntimeContext:  "route=/console/agents/agent-1/agent",
			SkillIDs:        []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement},
			SkillMode:       skillModeAuto,
			ModelTurnIntent: intent,
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if !strategy.NeedsExactAgentRuntime {
		t.Fatalf("NeedsExactAgentRuntime = false, want true; strategy=%#v", strategy)
	}
	if strategy.TaskType != "agent_config_analysis" {
		t.Fatalf("TaskType = %q, want agent_config_analysis", strategy.TaskType)
	}
	if !slices.Contains(strategy.SupportingSkills, skills.SkillAgentManagement) {
		t.Fatalf("SupportingSkills = %#v, want agent-management for exact runtime evidence", strategy.SupportingSkills)
	}
	if skillLoopShouldUsePlainStreamForPassiveAnswer(prepared) {
		t.Fatal("skillLoopShouldUsePlainStreamForPassiveAnswer() = true, want false when exact Agent runtime is needed")
	}

	plan := operationPlanFromTurnStrategy("task-1", prepared.parts, strategy)
	if got := stringFromAny(plan["task_type"]); got != "agent_config_analysis" {
		t.Fatalf("operation_plan.task_type = %q, want agent_config_analysis; plan=%#v", got, plan)
	}
	if got := stringSliceFromAny(plan["evidence_required"]); !slices.Contains(got, "actual system prompt") {
		t.Fatalf("operation_plan.evidence_required = %#v, want actual system prompt", got)
	}
	if got := operationPlanCompactPhasesForPrompt(plan["phases"], 8); len(got) < 2 {
		t.Fatalf("operation_plan phases = %#v, want semantic phases", got)
	}
}

func TestContextualAIChatTurnStrategyUsesModelCapabilitiesForAgentGoals(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "please complete the requested setup for this agent",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents/agent-1/agent",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement},
			SkillMode:      skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent: "manage_agent_asset",
				RecommendedCapabilities: []string{
					"agent.model_selection",
					"agent.system_prompt",
					"agent.skill_backed_capability:file generation",
					"agent.accept_uploaded_files",
				},
				Phases:     []string{"configure the Agent runtime and capabilities", "verify the saved Agent config"},
				Confidence: 0.94,
				Reason:     "The user wants the Agent runtime and abilities configured.",
			},
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "manage_agent_asset" {
		t.Fatalf("Intent = %q, want manage_agent_asset; strategy=%#v", strategy.Intent, strategy)
	}
	plan := operationPlanFromTurnStrategy("task-model-agent-capabilities", prepared.parts, strategy)
	capabilityGoals := mapSliceFromAny(plan["capability_goals"])
	for _, want := range []string{
		agentCapabilityModelSelection,
		agentCapabilitySystemPrompt,
		agentCapabilitySkillBacked,
		agentCapabilityAcceptUploaded,
	} {
		if !operationPlanCapabilityGoalsContainForTest(capabilityGoals, want) {
			t.Fatalf("capability_goals = %#v, missing model-provided capability %s; plan=%#v", capabilityGoals, want, plan)
		}
	}
	if !operationPlanCapabilityGoalsContainBindingActionForTest(capabilityGoals, "enabled_skill_ids", "bind") {
		t.Fatalf("capability_goals = %#v, want enabled_skill_ids bind for model-provided skill capability", capabilityGoals)
	}
	if !operationPlanCapabilityGoalsContainRequiredFieldForTest(capabilityGoals, "file_upload_enabled") {
		t.Fatalf("capability_goals = %#v, want file_upload_enabled field from model-provided capability", capabilityGoals)
	}
	if candidate := agentManagementSkillCandidateQueryForCapabilityGoals(strategy.CapabilityGoals); candidate != "file generation" {
		t.Fatalf("skill candidate query = %q, want file generation; goals=%#v", candidate, strategy.CapabilityGoals)
	}
}

func TestContextualAgentManagementStrategyUsesModelVisibleIndexForPageTargetGuidance(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "please inspect the target Agent",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "/console/agents",
		SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement},
		SkillMode:      skillModeAuto,
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent:             "manage_agent_asset",
			TargetVisibleIndex: 1,
		},
		RawOperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"resource_type": "page",
					"resource_id":   "/console/agents",
					"title":         "Agent list",
				},
				map[string]interface{}{
					"resource_type": "agent",
					"resource_id":   "agent-first",
					"title":         "First Agent",
					"href":          "/console/agents/agent-first/agent",
					"can_edit":      true,
				},
			},
		},
	}

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if !strings.Contains(strings.Join(strategy.Avoid, "\n"), "visible ordinal Agent targets") {
		t.Fatalf("Avoid = %#v, want visible ordinal Agent target guidance from model intent", strategy.Avoid)
	}
}

func TestContextualAIChatTurnStrategyUsesPassiveModelIntentFastPath(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "what can you do here?",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents/agent-1/agent",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement},
			SkillMode:      skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:      "answer_or_explain_zgi_context",
				Confidence:  1,
				Approval:    "none",
				AssetEffect: "none",
			},
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if len(strategy.PrimarySkills) != 0 {
		t.Fatalf("PrimarySkills = %#v, want no primary skill for passive model intent", strategy.PrimarySkills)
	}
	if !skillLoopShouldUsePlainStreamForPassiveAnswer(prepared) {
		t.Fatal("skillLoopShouldUsePlainStreamForPassiveAnswer() = false, want true")
	}
}

func TestUserMemoryPreflightRunsDuringPrepareForContextualSidebar(t *testing.T) {
	svc := &service{
		llmClient:     &fakeAgentMemoryPlannerLLM{},
		memoryService: fakeUserMemoryService{},
	}
	parts := &chatRequestParts{
		Query:          "what can you do here?",
		Surface:        aiChatSurfaceContextualSidebar,
		RuntimeContext: "route=/console/agents/agent-1/agent",
		UseMemory:      true,
		SkillIDs:       []string{skills.SkillConsoleNavigator},
		SkillMode:      skillModeAuto,
	}

	if !svc.shouldRunUserMemoryPreflightDuringPrepare(parts, &adapter.ChatRequest{}) {
		t.Fatal("shouldRunUserMemoryPreflightDuringPrepare() = false, want true for contextual sidebar")
	}

	parts.Surface = aiChatSurfaceWorkChat
	if svc.shouldRunUserMemoryPreflightDuringPrepare(parts, &adapter.ChatRequest{}) {
		t.Fatal("shouldRunUserMemoryPreflightDuringPrepare() = true, want false for work chat")
	}
}

func TestStreamingMetadataRecordsModelTurnIntent(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "what can you do",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillIDs:  []string{skills.SkillConsoleNavigator},
		SkillMode: skillModeAuto,
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent:     "answer_or_explain_zgi_context",
			Confidence: 0.88,
			Reason:     "passive context question",
		},
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-1")
	raw, ok := metadata["model_turn_intent"].(*AIChatModelTurnIntent)
	if !ok || raw.Intent != "answer_or_explain_zgi_context" {
		t.Fatalf("model_turn_intent = %#v, want recorded model intent", metadata["model_turn_intent"])
	}
	if _, ok := metadata["turn_strategy"].(*AIChatTurnStrategy); !ok {
		t.Fatalf("turn_strategy = %#v, want typed strategy", metadata["turn_strategy"])
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["strategy_source"]); got != aiChatTurnStrategySourceModelIntent {
		t.Fatalf("operation_plan.strategy_source = %q, want %q; plan=%#v", got, aiChatTurnStrategySourceModelIntent, plan)
	}
}

type fakeUserMemoryService struct{}

func (fakeUserMemoryService) IsEnabled(context.Context, uuid.UUID) (bool, error) {
	return true, nil
}

func (fakeUserMemoryService) RenderContext(context.Context, uuid.UUID, int) (string, error) {
	return "", nil
}

func (fakeUserMemoryService) GetModelState(context.Context, uuid.UUID) (*memory.MemoryMeResponse, error) {
	return &memory.MemoryMeResponse{}, nil
}

func (fakeUserMemoryService) CreateEntryWithMetadata(context.Context, uuid.UUID, memory.CreateEntryRequest, memory.MutationMetadata) (*memory.MemoryEntryResponse, error) {
	return &memory.MemoryEntryResponse{}, nil
}

func (fakeUserMemoryService) UpdateEntryWithMetadata(context.Context, uuid.UUID, uuid.UUID, memory.UpdateEntryRequest, memory.MutationMetadata) (*memory.MemoryEntryResponse, error) {
	return &memory.MemoryEntryResponse{}, nil
}

func (fakeUserMemoryService) DeleteEntryWithMetadata(context.Context, uuid.UUID, uuid.UUID, memory.MutationMetadata) error {
	return nil
}

func TestContextualAIChatTurnStrategyPrefersMultiRouteNavigationOverAgentManagement(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "\u8bf7\u5148\u5207\u5230\u667a\u80fd\u4f53\u9875\uff0c\u7136\u540e\u518d\u5207\u56de\u6587\u4ef6\u7ba1\u7406\u9875\uff1b\u5b8c\u6210\u540e\u53ea\u8bf4\u5df2\u56de\u5230\u6587\u4ef6\u7ba1\u7406\u9875\uff0c\u4e0d\u8981\u521b\u5efa\u3001\u4fdd\u5b58\u3001\u5220\u9664\u6587\u4ef6\u3002",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillIDs:  []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement, skills.SkillFileManager},
			SkillMode: skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "navigate_console_page",
				TargetPage: "/console/agents",
				Confidence: 0.91,
			},
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "navigate_console_page" {
		t.Fatalf("Intent = %q, want navigate_console_page", strategy.Intent)
	}
	if strategy.TargetPage != "/console/agents" || !strategy.RouteRequired {
		t.Fatalf("target/route = %q/%v, want /console/agents/true", strategy.TargetPage, strategy.RouteRequired)
	}
}

func TestContextualAIChatTurnStrategyTreatsAutoContinueRouteSequenceAsNavigation(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "\u8bf7\u4f9d\u6b21\u5bfc\u822a\uff1a\u9996\u9875 -> \u6587\u4ef6\u7ba1\u7406 -> \u667a\u80fd\u4f53 -> \u6570\u636e\u5e93 -> \u6587\u4ef6\u7ba1\u7406\u3002\u6bcf\u6b21\u5bfc\u822a\u6210\u529f\u540e\u81ea\u52a8\u7ee7\u7eed\u4e0b\u4e00\u6b65\u3002",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents capabilities=agent.list_visible",
			OperationContext: map[string]interface{}{
				"resources": []interface{}{map[string]interface{}{
					"resource_type": "page",
					"resource_id":   "console.agents",
					"title":         "Agents",
					"href":          "/console/agents",
					"metadata": map[string]interface{}{
						"route": "/console/agents",
					},
				}},
			},
			SkillIDs:  []string{skills.SkillConsoleNavigator, skills.SkillFileManager, skills.SkillFileGenerator},
			SkillMode: skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "navigate_console_page",
				TargetPage: "/console",
				Confidence: 0.91,
			},
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "navigate_console_page" {
		t.Fatalf("Intent = %q, want navigate_console_page", strategy.Intent)
	}
	if strategy.TargetPage != "/console" || !strategy.RouteRequired {
		t.Fatalf("target/route = %q/%v, want /console/true", strategy.TargetPage, strategy.RouteRequired)
	}
	metadata := streamingMessageMetadataWithTaskID(prepared.parts, "task-nav-sequence")
	plan := metadata["operation_plan"].(map[string]interface{})
	if plan["intent"] != "navigate_console_page" {
		t.Fatalf("operation plan intent = %#v, want navigate_console_page", plan["intent"])
	}
	if got := operationPlanRoutePagesForTest(plan); len(got) != 0 {
		t.Fatalf("operation plan route pages = %#v, want none", got)
	}
	stepStatus := plan["step_status"].(map[string]interface{})
	if stepStatus["route:/console"] != nil {
		t.Fatalf("operation plan step_status = %#v, want no route step", stepStatus)
	}
}

func TestContextualAIChatTurnStrategyScopesStagedContinuationToCurrentPhase(t *testing.T) {
	query := "\u7b2c\u4e00\u9636\u6bb5\u53ea\u5bfc\u822a\u5230\u667a\u80fd\u4f53\u9875\u5e76\u8bfb\u53d6\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53\u540d\u79f0\uff0c\u7b49\u6211\u8bf4\u201c\u7ee7\u7eed\u201d\u540e\u518d\u6267\u884c\uff1a\u5bfc\u822a\u5230\u6587\u4ef6\u7ba1\u7406\uff0c\u521b\u5efa\u5e76\u4fdd\u5b58 smoke.txt \u548c smoke.svg\uff0c\u7136\u540e\u5220\u9664\u7b2c\u4e09\u4e2a\u6587\u4ef6"
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          query,
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/files",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillFileGenerator, skills.SkillFileManager},
			SkillMode:      skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "navigate_console_page",
				TargetPage: "/console/agents",
				Confidence: 0.91,
			},
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if !strategy.WaitForContinue || strategy.ExecutionScope != "current_turn_before_continue" {
		t.Fatalf("strategy wait/scope = %v/%q, want current-turn staged wait", strategy.WaitForContinue, strategy.ExecutionScope)
	}
	if strategy.Intent != "navigate_console_page" {
		t.Fatalf("Intent = %q, want navigate_console_page for the current phase", strategy.Intent)
	}
	if strategy.TargetPage != "/console/agents" {
		t.Fatalf("TargetPage = %q, want /console/agents", strategy.TargetPage)
	}
	for _, tool := range strategy.PlannedTools {
		if tool.SkillID == skills.SkillFileGenerator || tool.SkillID == skills.SkillFileManager {
			t.Fatalf("PlannedTools = %#v, want no deferred file tools in current phase", strategy.PlannedTools)
		}
	}

	metadata := streamingMessageMetadataWithTaskID(prepared.parts, "task-staged")
	plan := metadata["operation_plan"].(map[string]interface{})
	if plan["status"] != operationPlanStatusRunning {
		t.Fatalf("operation plan status = %#v, want running while waiting for continue", plan["status"])
	}
	if plan["pending_next_action"] != "Wait for user continue" {
		t.Fatalf("pending_next_action = %#v, want wait before deferred work", plan["pending_next_action"])
	}
	stepStatus := plan["step_status"].(map[string]interface{})
	if stepStatus["wait:continue"] != operationPlanStepStatusPending {
		t.Fatalf("step_status = %#v, want wait:continue pending", stepStatus)
	}
	if _, ok := stepStatus["route:/console/files"]; ok {
		t.Fatalf("step_status = %#v, want no deferred /console/files route", stepStatus)
	}
}

func TestContextualAIChatTurnStrategyResumesStagedContinuationFromDeferredGoal(t *testing.T) {
	originalGoal := "\u7b2c\u4e00\u9636\u6bb5\u53ea\u5bfc\u822a\u5230\u667a\u80fd\u4f53\u9875\u5e76\u8bfb\u53d6\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53\u540d\u79f0\uff0c\u7b49\u6211\u8bf4\u201c\u7ee7\u7eed\u201d\u540e\u518d\u6267\u884c\uff1a\u5bfc\u822a\u5230\u6587\u4ef6\u7ba1\u7406\uff0c\u521b\u5efa\u5e76\u4fdd\u5b58 smoke.txt \u548c smoke.svg"
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "\u7ee7\u7eed",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillFileGenerator, skills.SkillFileManager},
			SkillMode:      skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "navigate_console_page",
				TargetPage: "/console/files",
				Confidence: 0.91,
			},
			RecentOperationPlans: []map[string]interface{}{{
				"original_user_goal":  originalGoal,
				"status":              operationPlanStatusRunning,
				"pending_next_action": "Wait for user continue",
				"steps": []interface{}{
					map[string]interface{}{
						"id":     "wait:continue",
						"title":  "Wait for user continue",
						"status": operationPlanStepStatusCompleted,
					},
					map[string]interface{}{
						"id":          "route:/console/files",
						"title":       "Navigate to File Management",
						"status":      operationPlanStepStatusPending,
						"skill_id":    skills.SkillConsoleNavigator,
						"tool_name":   "navigate",
						"href":        "/console/files",
						"arguments":   map[string]interface{}{"href": "/console/files"},
						"target_page": "/console/files",
					},
				},
				"step_status": map[string]interface{}{
					"wait:continue":        operationPlanStepStatusCompleted,
					"route:/console/files": operationPlanStepStatusPending,
				},
			}},
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.ExecutionScope != "staged_goal_after_continue" {
		t.Fatalf("ExecutionScope = %q, want staged_goal_after_continue", strategy.ExecutionScope)
	}
	if strategy.TargetPage != "/console/files" || !strategy.RouteRequired {
		t.Fatalf("target/route = %q/%v, want /console/files/true", strategy.TargetPage, strategy.RouteRequired)
	}
	for _, want := range []string{skills.SkillFileGenerator, skills.SkillFileManager} {
		if !slices.Contains(strategy.SupportingSkills, want) && !slices.Contains(strategy.PrimarySkills, want) {
			t.Fatalf("skills primary=%#v supporting=%#v, want %s available for deferred goal", strategy.PrimarySkills, strategy.SupportingSkills, want)
		}
	}
	if len(strategy.PlannedTools) != 0 {
		t.Fatalf("PlannedTools = %#v, want no scripted tools for model-decides deferred goal", strategy.PlannedTools)
	}
}

func TestContextualAIChatTurnStrategyResumesStagedFileGoalWithoutAgentNameRoute(t *testing.T) {
	originalGoal := "\u5148\u5207\u5230\u667a\u80fd\u4f53\u9875\u8bfb\u53d6\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53\u540d\u79f0\uff0c\u7136\u540e\u6682\u505c\u5e76\u7b49\u5f85\u6211\u8bf4\u201c\u7ee7\u7eed\u201d\u3002\u6211\u8bf4\u7ee7\u7eed\u540e\uff0c\u518d\u8fdb\u5165\u6587\u4ef6\u7ba1\u7406\uff0c\u521b\u5efa\u5e76\u4fdd\u5b58 smoke.txt \u548c smoke.svg\uff1btxt \u5185\u5bb9\u5199\u5165\u8bfb\u53d6\u5230\u7684\u667a\u80fd\u4f53\u540d\u79f0\uff1b\u4fdd\u5b58\u540e\u5220\u9664\u7b2c\u4e09\u4e2a\u6587\u4ef6"
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "\u7ee7\u7eed",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillFileGenerator, skills.SkillFileManager},
			SkillMode:      skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "navigate_console_page",
				TargetPage: "/console/files",
				Confidence: 0.91,
			},
			RecentOperationPlans: []map[string]interface{}{{
				"original_user_goal":  originalGoal,
				"status":              operationPlanStatusRunning,
				"pending_next_action": "Wait for user continue",
				"steps": []interface{}{
					map[string]interface{}{
						"id":     "wait:continue",
						"title":  "Wait for user continue",
						"status": operationPlanStepStatusCompleted,
					},
					map[string]interface{}{
						"id":          "route:/console/files",
						"title":       "Navigate to File Management",
						"status":      operationPlanStepStatusPending,
						"skill_id":    skills.SkillConsoleNavigator,
						"tool_name":   "navigate",
						"href":        "/console/files",
						"arguments":   map[string]interface{}{"href": "/console/files"},
						"target_page": "/console/files",
					},
				},
				"step_status": map[string]interface{}{
					"wait:continue":        operationPlanStepStatusCompleted,
					"route:/console/files": operationPlanStepStatusPending,
				},
			}},
		},
	}

	scopedParts, stagedCurrent, stagedResume := stagedExecutionScopedParts(prepared.parts)
	if scopedParts == nil || !stagedResume || stagedCurrent {
		t.Fatalf("staged scoped parts = %#v current=%v resume=%v, want deferred resume", scopedParts, stagedCurrent, stagedResume)
	}
	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.ExecutionScope != "staged_goal_after_continue" {
		t.Fatalf("ExecutionScope = %q, want staged_goal_after_continue", strategy.ExecutionScope)
	}
	if strategy.TargetPage != "/console/files" || !strategy.RouteRequired {
		t.Fatalf("target/route = %q/%v, want /console/files/true; scoped query=%q agentIntent=%v managedIntent=%v strategy=%#v", strategy.TargetPage, strategy.RouteRequired, scopedParts.Query, isAgentManagementIntent(scopedParts.Query), isManagedFileCreateIntent(scopedParts.Query), strategy)
	}
	for _, want := range []string{skills.SkillFileGenerator, skills.SkillFileManager} {
		if !slices.Contains(strategy.SupportingSkills, want) && !slices.Contains(strategy.PrimarySkills, want) {
			t.Fatalf("skills primary=%#v supporting=%#v, want %s available for deferred file goal", strategy.PrimarySkills, strategy.SupportingSkills, want)
		}
	}
	if len(strategy.PlannedTools) != 0 {
		t.Fatalf("PlannedTools = %#v, want no scripted tools for model-decides file goal", strategy.PlannedTools)
	}
}

func TestContextualAIChatTurnStrategyKeepsStagedCurrentScopeDuringClientActionResume(t *testing.T) {
	originalGoal := "\u7b2c\u4e00\u9636\u6bb5\u53ea\u5bfc\u822a\u5230\u667a\u80fd\u4f53\u9875\u5e76\u8bfb\u53d6\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53\u540d\u79f0\uff0c\u7b49\u6211\u8bf4\u201c\u7ee7\u7eed\u201d\u540e\u518d\u6267\u884c\uff1a\u5bfc\u822a\u5230\u6587\u4ef6\u7ba1\u7406\uff0c\u521b\u5efa\u5e76\u4fdd\u5b58 smoke.txt \u548c smoke.svg"
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          originalGoal,
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillFileGenerator, skills.SkillFileManager},
			SkillMode:      skillModeAuto,
			RecentOperationPlans: []map[string]interface{}{{
				"original_user_goal":  originalGoal,
				"status":              operationPlanStatusRunning,
				"pending_next_action": "Wait for user continue",
				"steps": []interface{}{
					map[string]interface{}{
						"id":     "wait:continue",
						"title":  "Wait for user continue",
						"status": operationPlanStepStatusPending,
					},
				},
				"step_status": map[string]interface{}{"wait:continue": operationPlanStepStatusPending},
			}},
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.ExecutionScope != "current_turn_before_continue" || !strategy.WaitForContinue {
		t.Fatalf("scope/wait = %q/%v, want current turn staged wait", strategy.ExecutionScope, strategy.WaitForContinue)
	}
	if strategy.TargetPage != "/console/agents" {
		t.Fatalf("TargetPage = %q, want /console/agents instead of deferred /console/files", strategy.TargetPage)
	}
	for _, tool := range strategy.PlannedTools {
		if tool.SkillID == skills.SkillFileGenerator || tool.SkillID == skills.SkillFileManager {
			t.Fatalf("PlannedTools = %#v, want no deferred file tools during client action resume", strategy.PlannedTools)
		}
	}
}

func TestSkillLoopToolCallGuardBlocksDeferredStagedContinuationTools(t *testing.T) {
	query := "\u7b2c\u4e00\u9636\u6bb5\u53ea\u5bfc\u822a\u5230\u667a\u80fd\u4f53\u9875\uff0c\u7b49\u6211\u8bf4\u201c\u7ee7\u7eed\u201d\u540e\u518d\u6267\u884c\uff1a\u5bfc\u822a\u5230\u6587\u4ef6\u7ba1\u7406\uff0c\u521b\u5efa\u5e76\u4fdd\u5b58 smoke.svg\uff0c\u7136\u540e\u5220\u9664\u7b2c\u4e09\u4e2a\u6587\u4ef6"
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     query,
			Surface:   aiChatSurfaceContextualSidebar,
			SkillIDs:  []string{skills.SkillConsoleNavigator, skills.SkillFileGenerator, skills.SkillFileManager},
			SkillMode: skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "navigate_console_page",
				TargetPage: "/console/agents",
				Confidence: 0.91,
			},
		},
	}

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want staged continuation guard")
	}
	_, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillConsoleNavigator,
		ToolName:  "navigate",
		Arguments: map[string]interface{}{"href": "/console/agents"},
	})
	if blocked {
		t.Fatal("guard blocked current-phase /console/agents navigation")
	}
	for _, req := range []skillloop.ToolCallGuardRequest{
		{SkillID: skills.SkillConsoleNavigator, ToolName: "navigate", Arguments: map[string]interface{}{"href": "/console/files"}},
		{SkillID: skills.SkillFileGenerator, ToolName: "generate_file", Arguments: map[string]interface{}{"filename": "smoke.svg"}},
		{SkillID: skills.SkillFileManager, ToolName: "save_file_to_management", Arguments: map[string]interface{}{"filename": "smoke.svg"}},
		{SkillID: skills.SkillFileManager, ToolName: "delete_file", Arguments: map[string]interface{}{"file_id": "file-3"}},
	} {
		result, blocked := guard(req)
		if !blocked {
			t.Fatalf("guard allowed deferred tool call %#v", req)
		}
		if !strings.Contains(result.SystemMessage, "continue marker") {
			t.Fatalf("guard message = %q, want staged continuation explanation", result.SystemMessage)
		}
	}
}

func TestConsoleNavigationModelTargetsRemainAdvisoryAfterCompletedRoutes(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "\u8bf7\u5148\u5207\u5230\u667a\u80fd\u4f53\u9875\uff0c\u7136\u540e\u518d\u5207\u56de\u6587\u4ef6\u7ba1\u7406\u9875",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillIDs:  []string{skills.SkillConsoleNavigator},
		SkillMode: skillModeAuto,
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent:     "navigate_console_page",
			TargetPage: "/console/files",
			Confidence: 0.91,
		},
		OperationContext: map[string]interface{}{
			"client_action_continuation": map[string]interface{}{
				"action_type": "route_navigation",
				"status":      clientActionStatusSucceeded,
				"href":        "/console/agents",
			},
		},
	}

	targets := consoleNavigationResolvedTargetsForParts(parts)
	if len(targets) != 1 {
		t.Fatalf("consoleNavigationResolvedTargetsForParts() = %#v, want one model target", targets)
	}
	if targets[0].Href != "/console/files" {
		t.Fatalf("target = %#v, want /console/files from model intent", targets[0])
	}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil")
	}
}

func TestContextualAIChatTurnStrategyResolvesShortChineseNavigation(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "帮我切到数据库",
			Surface:   aiChatSurfaceContextualSidebar,
			SkillIDs:  []string{skills.SkillConsoleNavigator},
			SkillMode: skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "navigate_console_page",
				TargetPage: "/console/db",
				Confidence: 0.91,
			},
		},
	}

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "navigate_console_page" {
		t.Fatalf("Intent = %q, want navigate_console_page", strategy.Intent)
	}
	if strategy.TargetPage != "/console/db" || !strategy.RouteRequired {
		t.Fatalf("target/route = %q/%v, want /console/db/true", strategy.TargetPage, strategy.RouteRequired)
	}
}

func TestContextualAIChatTurnStrategyClassifiesContinuation(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("继续"),
	}
	prepared.parts.Surface = aiChatSurfaceContextualSidebar
	prepared.parts.SkillIDs = []string{
		skills.SkillConsoleNavigator,
		skills.SkillFileGenerator,
		skills.SkillFileManager,
	}
	prepared.parts.SkillMode = skillModeAuto

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "continue_previous_task" {
		t.Fatalf("Intent = %q, want continue_previous_task", strategy.Intent)
	}
	for _, want := range []string{skills.SkillConsoleNavigator, skills.SkillFileGenerator, skills.SkillFileManager} {
		if !slices.Contains(strategy.SupportingSkills, want) {
			t.Fatalf("SupportingSkills = %#v, missing %q", strategy.SupportingSkills, want)
		}
	}
}

func TestContextualAIChatTurnStrategyContinuationManagedCreateKeepsFilePlan(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("\u7ee7\u7eed\u6267\u884c\u7b2c\u4e8c\u9636\u6bb5\uff1a\u5230\u6587\u4ef6\u7ba1\u7406\u521b\u5efa\u5e76\u4fdd\u5b58 smoke.txt \u548c smoke.svg"),
	}
	prepared.parts.RuntimeContext = "route=/console/agents"
	prepared.parts.RawOperationContext = map[string]interface{}{
		"resources": []interface{}{map[string]interface{}{
			"resource_type": "page",
			"href":          "/console/agents",
		}},
	}
	prepared.parts.OperationContext = prepared.parts.RawOperationContext
	prepared.parts.Surface = aiChatSurfaceContextualSidebar
	prepared.parts.SkillIDs = []string{
		skills.SkillConsoleNavigator,
		skills.SkillFileGenerator,
		skills.SkillFileManager,
	}
	prepared.parts.SkillMode = skillModeAuto

	strategy := contextualAIChatTurnStrategy(prepared)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategy() = nil, want strategy")
	}
	if strategy.Intent != "continue_previous_task" {
		t.Fatalf("Intent = %q, want continue_previous_task", strategy.Intent)
	}
	if strategy.TargetPage != "/console/files" || !strategy.RouteRequired {
		t.Fatalf("target/route = %q/%v, want /console/files/true", strategy.TargetPage, strategy.RouteRequired)
	}
	if !slices.Contains(strategy.PrimarySkills, skills.SkillConsoleNavigator) {
		t.Fatalf("PrimarySkills = %#v, missing %q", strategy.PrimarySkills, skills.SkillConsoleNavigator)
	}
	for _, want := range []string{skills.SkillFileGenerator, skills.SkillFileManager} {
		if !slices.Contains(strategy.SupportingSkills, want) {
			t.Fatalf("SupportingSkills = %#v, missing %q", strategy.SupportingSkills, want)
		}
	}
	plan := operationPlanFromTurnStrategy("task-continuation-save", prepared.parts, strategy)
	stepStatus := plan["step_status"].(map[string]interface{})
	for _, notWant := range []string{
		"tool:console-navigator/navigate",
		"route:/console/files",
	} {
		if stepStatus[notWant] != nil {
			t.Fatalf("step_status = %#v, want no scripted %s", stepStatus, notWant)
		}
	}
}

func TestIsContinuationIntentAllowsTaskMarker(t *testing.T) {
	query := "\u7ee7\u7eed\u3002\u4efb\u52a1\u6807\u8bb0\uff1aSMOKE-CONTINUE-1782312653811"
	if !isContinuationIntent(query) {
		t.Fatalf("isContinuationIntent(%q) = false, want true", query)
	}
}

func TestIsContinuationIntentAllowsLongStagedContinuationCommand(t *testing.T) {
	query := "\u7ee7\u7eed\u6267\u884c\u7b2c\u4e8c\u9636\u6bb5\uff1a\u5230\u6587\u4ef6\u7ba1\u7406\u521b\u5efa\u5e76\u4fdd\u5b58\u4e24\u4e2a\u6587\u4ef6\uff0ctxt \u5185\u5bb9\u5199\u4e0a\u4e00\u9636\u6bb5\u8bfb\u53d6\u5230\u7684\u667a\u80fd\u4f53\u540d\u79f0\uff0csvg \u5185\u5bb9\u753b\u4e00\u4e2a\u5c0f\u56fe\u6807\uff0c\u5b8c\u6210\u540e\u6682\u505c\u3002"
	if !isContinuationIntent(query) {
		t.Fatalf("isContinuationIntent(%q) = false, want true for a long staged continuation command", query)
	}
}

func TestIsContinuationIntentDoesNotTreatStagedTaskDefinitionAsContinuation(t *testing.T) {
	query := "\u5148\u5207\u5230\u667a\u80fd\u4f53\u9875\u8bfb\u53d6\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53\u540d\u79f0\uff0c\u7136\u540e\u7b49\u5f85\u6211\u8bf4\u7ee7\u7eed\u540e\u518d\u8fdb\u5165\u6587\u4ef6\u7ba1\u7406\u521b\u5efa\u6587\u4ef6\u3002"
	if isContinuationIntent(query) {
		t.Fatalf("isContinuationIntent(%q) = true, want false for a staged task definition", query)
	}
}

func TestIsContinuationIntentDoesNotTreatQuotedContinueInstructionAsContinuation(t *testing.T) {
	query := "\u7b2c\u4e00\u9636\u6bb5\u5b8c\u6210\u540e\u8bf7\u6682\u505c\uff0c\u7b49\u5f85\u6211\u8bf4\u201c\u7ee7\u7eed\u201d\u3002\u6ce8\u610f\uff1a\u7b2c\u4e00\u9636\u6bb5\u7edd\u5bf9\u4e0d\u8981\u5220\u9664\u4efb\u4f55\u6587\u4ef6\u3002"
	if isContinuationIntent(query) {
		t.Fatalf("isContinuationIntent(%q) = true, want false for a quoted continue instruction", query)
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

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
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
	if blocked {
		t.Fatal("guard blocked a report after save_file_to_management was attempted; model should use the actual tool result")
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

func TestSkillLoopFinalAnswerGuardAllowsReadOnlyFilesQuestionWithNegativeOperations(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("SMOKE-ORDER: \u53ea\u56de\u7b54\u5f53\u524d\u6587\u4ef6\u7ba1\u7406\u9875\u53ef\u89c1\u6587\u4ef6\u603b\u6570\u548c\u524d\u4e24\u4e2a\u6587\u4ef6\u540d\uff0c\u4e0d\u8981\u521b\u5efa\u3001\u4fdd\u5b58\u3001\u5220\u9664\u6216\u5bfc\u822a\u3002"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.ModelTurnIntent = &AIChatModelTurnIntent{
		Intent:     "answer_or_explain_zgi_context",
		Confidence: 0.91,
	}

	if isManagedFileCreateIntent(prepared.parts.Query) {
		t.Fatal("isManagedFileCreateIntent() = true, want false for read-only request with negative operations")
	}
	if guard := skillLoopFinalAnswerGuard(prepared); guard != nil {
		_, blocked := guard(skillloop.FinalAnswerGuardRequest{
			Answer: "当前可见文件共有 41 个，前两个文件是 a.svg 和 b.txt。",
		})
		if blocked {
			t.Fatal("skillLoopFinalAnswerGuard blocked read-only files-page answer with negative operations")
		}
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

func TestSkillLoopToolCallGuardAllowsManagedFileWorkBeforeOptionalFilesRoute(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "please create an svg file in File Management",
			RuntimeContext: "route=/console/work/chat",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillFileGenerator, skills.SkillFileManager, skills.SkillChartGenerator},
			SkillMode:      skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "save_generated_file_to_file_management",
				Confidence: 0.91,
			},
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
	if blocked {
		t.Fatalf("tool guard blocked file generation only because Files route was not loaded; result=%#v", result)
	}
	result, blocked = guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileManager,
		ToolName: "save_file_to_management",
		Arguments: map[string]interface{}{
			"source_type":  "tool_file",
			"tool_file_id": "tool-1",
			"filename":     "hello.svg",
		},
	})
	if blocked {
		t.Fatalf("tool guard blocked file save only because Files route was not loaded; result=%#v", result)
	}

	_, blocked = guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillConsoleNavigator,
		ToolName:  "navigate",
		Arguments: map[string]interface{}{"href": "/console/files"},
	})
	if blocked {
		t.Fatal("tool guard blocked optional Files page navigation")
	}

	_, blocked = guard(skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillConsoleNavigator,
		ToolName:  "navigate",
		Arguments: map[string]interface{}{"href": "/console/agents"},
	})
	if blocked {
		t.Fatal("tool guard blocked model-decided navigation away from Files during managed file creation")
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
		t.Fatal("tool guard allowed chart generation for a generic SVG request")
	}
	for _, want := range []string{skills.SkillFileGenerator, "generate_file", "generic SVG"} {
		if !strings.Contains(result.SystemMessage, want) && !strings.Contains(result.Message, want) {
			t.Fatalf("chart guard result missing %q: %#v", want, result)
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

	savedFirstFile := []skillloop.SkillToolCallRef{
		{
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
		},
		{
			SkillID:  skills.SkillFileManager,
			ToolName: "save_file_to_management",
			Arguments: map[string]interface{}{
				"source_type":  "tool_file",
				"tool_file_id": "tool-1",
				"filename":     "first.txt",
			},
			Result: map[string]interface{}{
				"file_name":      "first.txt",
				"source_file_id": "tool-1",
				"target":         "managed_file",
			},
		},
	}
	_, blocked = guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileGenerator,
		ToolName: "generate_file",
		Arguments: map[string]interface{}{
			"filename": "second",
			"format":   "svg",
		},
		SuccessfulToolCalls: savedFirstFile,
	})
	if blocked {
		t.Fatal("tool guard blocked generating a second requested file after the first artifact was saved")
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

func TestSkillLoopPlanToolGuardAppliesContextualManagedFileDuplicateGenerationGuard(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status":             operationPlanStatusRunning,
					"original_user_goal": "please create a txt file in File Management",
				},
			},
		},
		parts: consoleFilesCreateCapabilityTestParts("please create a txt file in File Management"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopPlanToolCallGuard(prepared)
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
		t.Fatal("plan tool guard allowed duplicate file generation after a temporary artifact already existed")
	}
	for _, want := range []string{skills.SkillFileManager, "save_file_to_management", `"tool_file_id":"tool-1"`, `"filename":"first.txt"`} {
		if !strings.Contains(result.SystemMessage, want) {
			t.Fatalf("guard system message missing %q in:\n%s", want, result.SystemMessage)
		}
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	blockedDeviations := mapSliceFromAny(plan["blocked_deviations"])
	if len(blockedDeviations) != 1 {
		t.Fatalf("blocked_deviations = %#v, want one contextual guard deviation", plan["blocked_deviations"])
	}
	if got := stringFromAny(blockedDeviations[0]["reason"]); got != "contextual_execution_evidence_requires_different_next_step" {
		t.Fatalf("blocked deviation reason = %q, want contextual guard reason", got)
	}
}

func TestSkillLoopToolCallGuardAllowsDistinctExplicitManagedFileTargets(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("please create and save aichat-one.txt and aichat-two.svg to File Management"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want managed-file duplicate generation guard")
	}

	generatedTXT := []skillloop.SkillToolCallRef{{
		SkillID:  skills.SkillFileGenerator,
		ToolName: "generate_file",
		Arguments: map[string]interface{}{
			"filename": "aichat-one",
			"format":   "txt",
		},
		Result: map[string]interface{}{
			"tool_file_id": "tool-1",
			"filename":     "aichat-one.txt",
		},
	}}

	_, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileGenerator,
		ToolName: "generate_file",
		Arguments: map[string]interface{}{
			"filename": "aichat-two",
			"format":   "svg",
		},
		SuccessfulToolCalls: generatedTXT,
	})
	if blocked {
		t.Fatal("tool guard blocked generating the second explicit target file")
	}

	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileGenerator,
		ToolName: "generate_file",
		Arguments: map[string]interface{}{
			"filename": "aichat-one",
			"format":   "txt",
		},
		SuccessfulToolCalls: generatedTXT,
	})
	if !blocked {
		t.Fatal("tool guard allowed duplicate generation for the same explicit target")
	}
	if !strings.Contains(result.SystemMessage, `"tool_file_id":"tool-1"`) {
		t.Fatalf("duplicate guard should still point at the existing artifact, got:\n%s", result.SystemMessage)
	}
}

func TestSkillLoopToolCallGuardAllowsNonFilesNavigationDuringManagedFileCreate(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("先切到智能体页读取第一个智能体，然后回到文件管理创建文件"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillConsoleNavigator, skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want route guard")
	}
	_, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillConsoleNavigator,
		ToolName: "navigate",
		Arguments: map[string]interface{}{
			"href": "/console/agents",
		},
	})
	if blocked {
		t.Fatal("tool guard blocked navigation to agents while only duplicate Files navigation should be blocked")
	}
}

func TestSkillLoopToolCallGuardBlocksReturningToCompletedPrecursorRoute(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"client_actions": []interface{}{
					map[string]interface{}{
						"action_type": "route_navigation",
						"status":      clientActionStatusSucceeded,
						"href":        "/console/agents",
						"result": map[string]interface{}{
							"href":          "/console/agents",
							"observed_path": "/console/agents",
						},
					},
					map[string]interface{}{
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
		},
		parts: consoleFilesCreateCapabilityTestParts("先切到智能体页读取第一个智能体，然后到文件管理创建并保存 aichat-one.txt"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillConsoleNavigator, skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want route loop guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillConsoleNavigator,
		ToolName: "navigate",
		Arguments: map[string]interface{}{
			"href": "/console/agents",
		},
	})
	if !blocked {
		t.Fatal("tool guard allowed returning to an already completed precursor route after Files was loaded")
	}
	if !strings.Contains(result.SystemMessage, "already loaded and observed") {
		t.Fatalf("guard message missing route-loop explanation:\n%s", result.SystemMessage)
	}
}

func TestSkillLoopFinalAnswerGuardRequiresAllExplicitManagedFilesSaved(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("请创建并保存 aichat-one.txt 和 aichat-two.svg 到文件管理"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want managed file create guard")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "两个文件都已保存到文件管理。",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{
				SkillID:  skills.SkillFileManager,
				ToolName: "save_file_to_management",
				Arguments: map[string]interface{}{
					"source_type":  "tool_file",
					"tool_file_id": "tool-1",
					"filename":     "aichat-one.txt",
				},
				Result: map[string]interface{}{
					"file_name":      "aichat-one.txt",
					"source_file_id": "tool-1",
					"target":         "managed_file",
				},
			},
		},
	})
	if !blocked {
		t.Fatal("guard allowed final answer after only one explicit target file was saved")
	}
	if !strings.Contains(result.Message, "aichat-two.svg") {
		t.Fatalf("guard message missing unsaved target in:\n%s", result.Message)
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "两个文件都已保存到文件管理。",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{
				SkillID:  skills.SkillFileManager,
				ToolName: "save_file_to_management",
				Arguments: map[string]interface{}{
					"source_type":  "tool_file",
					"tool_file_id": "tool-1",
					"filename":     "aichat-one.txt",
				},
				Result: map[string]interface{}{
					"file_name":      "aichat-one.txt",
					"source_file_id": "tool-1",
					"target":         "managed_file",
				},
			},
			{
				SkillID:  skills.SkillFileManager,
				ToolName: "save_file_to_management",
				Arguments: map[string]interface{}{
					"source_type":  "tool_file",
					"tool_file_id": "tool-2",
					"filename":     "aichat-two.svg",
				},
				Result: map[string]interface{}{
					"file_name":      "aichat-two.svg",
					"source_file_id": "tool-2",
					"target":         "managed_file",
				},
			},
		},
	})
	if blocked {
		t.Fatal("guard blocked final answer after all explicit target files were saved")
	}
}

func TestSkillLoopFinalAnswerGuardUsesMessageMetadataForUnsavedExplicitTargetAfterApproval(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"generated_files": []interface{}{
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-1",
						"filename": "aichat-one.txt",
					},
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-2",
						"filename": "aichat-two.svg",
					},
					map[string]interface{}{
						"target":         "managed_file",
						"upload_file_id": "managed-1",
						"source_file_id": "tool-1",
						"filename":       "aichat-one.txt",
					},
				},
			},
		},
		parts: &chatRequestParts{
			Query:     "please create and save aichat-one.txt and aichat-two.svg to File Management",
			SkillIDs:  []string{skills.SkillFileGenerator, skills.SkillFileManager},
			SkillMode: skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "save_generated_file_to_file_management",
				Confidence: 0.91,
			},
		},
	}

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want managed file create guard without files page context")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "The text file was saved. Next I will create the SVG file.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{
				SkillID:  skills.SkillFileManager,
				ToolName: "save_file_to_management",
				Arguments: map[string]interface{}{
					"source_type":  "tool_file",
					"tool_file_id": "tool-1",
					"filename":     "aichat-one.txt",
				},
				Result: map[string]interface{}{
					"file_name":      "aichat-one.txt",
					"source_file_id": "tool-1",
					"target":         "managed_file",
				},
			},
		},
	})
	if !blocked {
		t.Fatal("guard allowed completion while an explicit SVG target was still only a temporary artifact")
	}
	for _, want := range []string{"aichat-two.svg", `"tool_file_id":"tool-2"`, skills.SkillFileManager, "save_file_to_management"} {
		if !strings.Contains(result.SystemMessage, want) {
			t.Fatalf("guard system message missing %q in:\n%s", want, result.SystemMessage)
		}
	}
}

func TestSkillLoopFinalAnswerGuardUsesMetadataSaveStateAfterAssetObservation(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"generated_files": []interface{}{
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-1",
						"filename": "aichat-one.txt",
					},
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-2",
						"filename": "aichat-two.svg",
					},
					map[string]interface{}{
						"target":         "managed_file",
						"upload_file_id": "managed-1",
						"source_file_id": "tool-1",
						"filename":       "aichat-one.txt",
					},
				},
			},
		},
		parts: &chatRequestParts{
			Query:     "please create and save aichat-one.txt and aichat-two.svg to File Management",
			SkillIDs:  []string{skills.SkillFileGenerator, skills.SkillFileManager},
			SkillMode: skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "save_generated_file_to_file_management",
				Confidence: 0.91,
			},
		},
	}

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want managed file create guard")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "Both files were saved to File Management.",
	})
	if !blocked {
		t.Fatal("guard allowed completion from asset observation metadata while SVG was still temporary")
	}
	for _, want := range []string{"aichat-two.svg", `"tool_file_id":"tool-2"`, skills.SkillFileManager, "save_file_to_management"} {
		if !strings.Contains(result.SystemMessage, want) {
			t.Fatalf("guard system message missing %q in:\n%s", want, result.SystemMessage)
		}
	}
}

func TestSkillLoopFinalAnswerGuardUsesContinuationMetadataSaveFlow(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"generated_files": []interface{}{
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-1",
						"filename": "smoke-continue.txt",
					},
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-2",
						"filename": "smoke-continue.svg",
					},
					map[string]interface{}{
						"target":         "managed_file",
						"upload_file_id": "managed-1",
						"source_file_id": "tool-1",
						"filename":       "smoke-continue.txt",
					},
				},
			},
		},
		parts: consoleFilesCreateCapabilityTestParts("\u7ee7\u7eed\u3002\u4efb\u52a1\u6807\u8bb0\uff1aSMOKE-CONTINUE-1782312653811"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want continuation managed-file save guard")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "The files were saved.",
	})
	if !blocked {
		t.Fatal("guard allowed completion while continuation metadata still had an unsaved SVG artifact")
	}
	for _, want := range []string{"smoke-continue.svg", `"tool_file_id":"tool-2"`, skills.SkillFileManager, "save_file_to_management"} {
		if !strings.Contains(result.SystemMessage, want) {
			t.Fatalf("guard system message missing %q in:\n%s", want, result.SystemMessage)
		}
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:  skills.SkillFileManager,
			ToolName: "save_file_to_management",
			Arguments: map[string]interface{}{
				"source_type":  "tool_file",
				"tool_file_id": "tool-2",
				"filename":     "smoke-continue.svg",
			},
			Result: map[string]interface{}{
				"file_name":      "smoke-continue.svg",
				"source_file_id": "tool-2",
				"target":         "managed_file",
			},
		}},
	})
	if blocked {
		t.Fatal("guard blocked after the remaining continuation SVG artifact was saved")
	}
}

func TestSkillLoopFinalAnswerGuardPrioritizesUnsavedArtifactBeforeContinuationDelete(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"generated_files": []interface{}{
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-1",
						"filename": "smoke-continue.txt",
					},
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-2",
						"filename": "smoke-continue.svg",
					},
					map[string]interface{}{
						"target":         "managed_file",
						"upload_file_id": "managed-1",
						"source_file_id": "tool-1",
						"filename":       "smoke-continue.txt",
					},
				},
			},
		},
		parts: consoleFilesCreateCapabilityTestParts("continue task marker SMOKE-CONTINUE: create and save smoke-continue.txt and smoke-continue.svg to File Management, then delete the current third file"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager, skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want continuation guard")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "TXT \u5df2\u4fdd\u5b58\u3002\u63a5\u4e0b\u6765\u9700\u8981\u5220\u9664\u5f53\u524d\u7b2c3\u4e2a\u6587\u4ef6\uff0c\u662f\u5426\u786e\u8ba4\u5220\u9664\uff1f",
	})
	if !blocked {
		t.Fatal("guard allowed continuation delete messaging while an SVG artifact was still unsaved")
	}
	for _, want := range []string{"smoke-continue.svg", `"tool_file_id":"tool-2"`, skills.SkillFileManager, "save_file_to_management", "not been saved yet"} {
		if !strings.Contains(result.SystemMessage, want) && !strings.Contains(result.Message, want) {
			t.Fatalf("guard result missing %q: %#v", want, result)
		}
	}
}

func TestSkillLoopFinalAnswerGuardDoesNotForceTemporaryContinuationArtifact(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"generated_files": []interface{}{
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-1",
						"filename": "temporary-only.svg",
					},
				},
			},
		},
		parts: consoleFilesCreateCapabilityTestParts("\u7ee7\u7eed\u3002\u4efb\u52a1\u6807\u8bb0\uff1aSMOKE-TEMP-1782312140380"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.ModelTurnIntent = &AIChatModelTurnIntent{
		Intent:     "continue_previous_task",
		Confidence: 0.91,
	}

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard != nil {
		if _, blocked := guard(skillloop.FinalAnswerGuardRequest{Answer: "Temporary file generated."}); blocked {
			t.Fatal("guard forced a File Management save for a continuation turn with only temporary artifacts")
		}
	}
}

func TestClientActionAssetObservationContinuesWhenExplicitManagedTargetUnsaved(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"generated_files": []interface{}{
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-1",
						"filename": "aichat-one.txt",
					},
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-2",
						"filename": "aichat-two.svg",
					},
					map[string]interface{}{
						"target":         "managed_file",
						"upload_file_id": "managed-1",
						"source_file_id": "tool-1",
						"filename":       "aichat-one.txt",
					},
				},
			},
		},
		parts: &chatRequestParts{
			Query:     "please create and save aichat-one.txt and aichat-two.svg to File Management",
			SkillIDs:  []string{skills.SkillFileGenerator, skills.SkillFileManager},
			SkillMode: skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "save_generated_file_to_file_management",
				Confidence: 0.91,
			},
		},
	}
	if !managedFileCreateHasUnsavedExplicitTargets(prepared) {
		t.Fatal("managedFileCreateHasUnsavedExplicitTargets() = false, want true while an explicit managed file target is still unsaved")
	}

	prepared.Message.Metadata["generated_files"] = append(prepared.Message.Metadata["generated_files"].([]interface{}), map[string]interface{}{
		"target":         "managed_file",
		"upload_file_id": "managed-2",
		"source_file_id": "tool-2",
		"filename":       "aichat-two.svg",
	})
	if managedFileCreateHasUnsavedExplicitTargets(prepared) {
		t.Fatal("managedFileCreateHasUnsavedExplicitTargets() = true, want false after all explicit managed file targets are saved")
	}
}

func TestClientActionAssetObservationContinuesWhenContinuationArtifactUnsaved(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"generated_files": []interface{}{
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-1",
						"filename": "smoke-continue.txt",
					},
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-2",
						"filename": "smoke-continue.svg",
					},
					map[string]interface{}{
						"target":         "managed_file",
						"upload_file_id": "managed-1",
						"source_file_id": "tool-1",
						"filename":       "smoke-continue.txt",
					},
				},
			},
		},
		parts: consoleFilesCreateCapabilityTestParts("\u7ee7\u7eed\u3002\u4efb\u52a1\u6807\u8bb0\uff1aSMOKE-CONTINUE-1782312653811"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto
	if !managedFileCreateHasUnsavedExplicitTargets(prepared) {
		t.Fatal("managedFileCreateHasUnsavedExplicitTargets() = false, want true while continuation metadata still has an unsaved SVG artifact")
	}

	prepared.Message.Metadata["generated_files"] = append(prepared.Message.Metadata["generated_files"].([]interface{}), map[string]interface{}{
		"target":         "managed_file",
		"upload_file_id": "managed-2",
		"source_file_id": "tool-2",
		"filename":       "smoke-continue.svg",
	})
	if managedFileCreateHasUnsavedExplicitTargets(prepared) {
		t.Fatal("managedFileCreateHasUnsavedExplicitTargets() = true, want false after continuation artifacts are all saved")
	}
}

func TestSkillLoopToolCallGuardUsesMetadataArtifactInsteadOfRegeneratingMissingTarget(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"generated_files": []interface{}{
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-1",
						"filename": "aichat-one.txt",
					},
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-2",
						"filename": "aichat-two.svg",
					},
					map[string]interface{}{
						"target":         "managed_file",
						"upload_file_id": "managed-1",
						"source_file_id": "tool-1",
						"filename":       "aichat-one.txt",
					},
				},
			},
		},
		parts: &chatRequestParts{
			Query:     "please create and save aichat-one.txt and aichat-two.svg to File Management",
			SkillIDs:  []string{skills.SkillFileGenerator, skills.SkillFileManager},
			SkillMode: skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "save_generated_file_to_file_management",
				Confidence: 0.91,
			},
		},
	}

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want metadata artifact reuse guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileGenerator,
		ToolName: "generate_file",
		Arguments: map[string]interface{}{
			"filename": "aichat-two",
			"format":   "svg",
		},
	})
	if !blocked {
		t.Fatal("tool guard allowed regenerating a missing target that already exists as a temporary artifact")
	}
	for _, want := range []string{`"tool_file_id":"tool-2"`, `"filename":"aichat-two.svg"`, "save_file_to_management"} {
		if !strings.Contains(result.SystemMessage, want) {
			t.Fatalf("tool guard system message missing %q in:\n%s", want, result.SystemMessage)
		}
	}
}

func TestSkillLoopToolCallGuardUsesContinuationMetadataArtifactInsteadOfRegenerating(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"generated_files": []interface{}{
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-1",
						"filename": "smoke-continue.txt",
					},
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-2",
						"filename": "smoke-continue.svg",
					},
					map[string]interface{}{
						"target":         "managed_file",
						"upload_file_id": "managed-1",
						"source_file_id": "tool-1",
						"filename":       "smoke-continue.txt",
					},
				},
			},
		},
		parts: consoleFilesCreateCapabilityTestParts("\u7ee7\u7eed\u3002\u4efb\u52a1\u6807\u8bb0\uff1aSMOKE-CONTINUE-1782312653811"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want continuation metadata artifact reuse guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileGenerator,
		ToolName: "generate_file",
		Arguments: map[string]interface{}{
			"filename": "smoke-continue",
			"format":   "svg",
		},
	})
	if !blocked {
		t.Fatal("tool guard allowed regenerating a continuation SVG artifact that already exists as a temporary artifact")
	}
	for _, want := range []string{`"tool_file_id":"tool-2"`, `"filename":"smoke-continue.svg"`, "save_file_to_management"} {
		if !strings.Contains(result.SystemMessage, want) {
			t.Fatalf("tool guard system message missing %q in:\n%s", want, result.SystemMessage)
		}
	}
}

func TestSkillLoopToolCallGuardBlocksMismatchedContinuationSaveArguments(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"generated_files": []interface{}{
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-1",
						"filename": "smoke-continue.txt",
					},
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-2",
						"filename": "smoke-continue.svg",
					},
					map[string]interface{}{
						"target":         "managed_file",
						"upload_file_id": "managed-1",
						"source_file_id": "tool-1",
						"filename":       "smoke-continue.txt",
					},
				},
			},
		},
		parts: consoleFilesCreateCapabilityTestParts("\u7ee7\u7eed\u3002\u4efb\u52a1\u6807\u8bb0\uff1aSMOKE-CONTINUE-1782312653811"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want continuation metadata save argument guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileManager,
		ToolName: "save_file_to_management",
		Arguments: map[string]interface{}{
			"source_type":  "tool_file",
			"tool_file_id": "managed-1",
			"filename":     "smoke-continue-step2.txt",
		},
	})
	if !blocked {
		t.Fatal("tool guard allowed save_file_to_management with a managed-file id and wrong filename")
	}
	for _, want := range []string{`"tool_file_id":"tool-2"`, `"filename":"smoke-continue.svg"`, "save_file_to_management"} {
		if !strings.Contains(result.SystemMessage, want) {
			t.Fatalf("tool guard system message missing %q in:\n%s", want, result.SystemMessage)
		}
	}

	_, blocked = guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileManager,
		ToolName: "save_file_to_management",
		Arguments: map[string]interface{}{
			"source_type":  "tool_file",
			"tool_file_id": "tool-2",
			"filename":     "smoke-continue.svg",
		},
	})
	if blocked {
		t.Fatal("tool guard blocked the correct continuation SVG save arguments")
	}
}

func TestSkillLoopToolCallGuardBlocksDeleteUntilAllManagedFileArtifactsAreSaved(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"generated_files": []interface{}{
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-1",
						"filename": "aichat-one.txt",
					},
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-2",
						"filename": "aichat-two.svg",
					},
					map[string]interface{}{
						"target":         "managed_file",
						"upload_file_id": "managed-1",
						"source_file_id": "tool-1",
						"filename":       "aichat-one.txt",
					},
				},
			},
		},
		parts: consoleFilesCreateCapabilityTestParts("continue task marker SMOKE-CONTINUE: create and save aichat-one.txt and aichat-two.svg to File Management, then delete the current third file"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want save-before-delete guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileManager,
		ToolName: "delete_file",
		Arguments: map[string]interface{}{
			"file_id": "file-third",
		},
	})
	if !blocked {
		t.Fatal("tool guard allowed delete_file before all generated artifacts were saved")
	}
	for _, want := range []string{"not saved to File Management", "save_file_to_management", `"tool_file_id":"tool-2"`, `"filename":"aichat-two.svg"`} {
		if !strings.Contains(result.SystemMessage, want) && !strings.Contains(result.Message, want) {
			t.Fatalf("save-before-delete guard result missing %q: %#v", want, result)
		}
	}
}

func TestSkillLoopToolCallGuardBlocksContinuationGenerationAfterArtifactsSaved(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"generated_files": []interface{}{
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-1",
						"filename": "smoke-continue.txt",
					},
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-2",
						"filename": "smoke-continue.svg",
					},
					map[string]interface{}{
						"target":         "managed_file",
						"upload_file_id": "managed-1",
						"source_file_id": "tool-1",
						"filename":       "smoke-continue.txt",
					},
					map[string]interface{}{
						"target":         "managed_file",
						"upload_file_id": "managed-2",
						"source_file_id": "tool-2",
						"filename":       "smoke-continue.svg",
					},
				},
			},
		},
		parts: consoleFilesCreateCapabilityTestParts("\u7ee7\u7eed\u3002\u4efb\u52a1\u6807\u8bb0\uff1aSMOKE-CONTINUE-1782312653811"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want continuation generated asset guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileGenerator,
		ToolName: "generate_file",
		Arguments: map[string]interface{}{
			"filename": "smoke-continue",
			"format":   "svg",
		},
	})
	if !blocked {
		t.Fatal("tool guard allowed regeneration after all continuation artifacts were already saved")
	}
	for _, want := range []string{"already has generated file artifacts", "Do not generate or save another file"} {
		if !strings.Contains(result.SystemMessage, want) {
			t.Fatalf("guard system message missing %q in:\n%s", want, result.SystemMessage)
		}
	}
}

func TestSkillLoopToolCallGuardBlocksContinuationSaveAfterArtifactsSaved(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"generated_files": []interface{}{
					map[string]interface{}{
						"target":   "temporary_artifact",
						"file_id":  "tool-1",
						"filename": "smoke-continue.txt",
					},
					map[string]interface{}{
						"target":         "managed_file",
						"upload_file_id": "managed-1",
						"source_file_id": "tool-1",
						"filename":       "smoke-continue.txt",
					},
				},
			},
		},
		parts: consoleFilesCreateCapabilityTestParts("\u7ee7\u7eed\u3002\u4efb\u52a1\u6807\u8bb0\uff1aSMOKE-CONTINUE-1782312653811"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want continuation generated asset guard")
	}
	_, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileManager,
		ToolName: "save_file_to_management",
		Arguments: map[string]interface{}{
			"source_type":  "tool_file",
			"tool_file_id": "tool-1",
			"filename":     "smoke-continue.txt",
		},
	})
	if !blocked {
		t.Fatal("tool guard allowed a duplicate save after all continuation artifacts were already saved")
	}
}

func TestSkillLoopToolCallGuardBlocksSecondContinuationDeleteAfterSuccess(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("\u7ee7\u7eed\u3002\u4efb\u52a1\u6807\u8bb0\uff1aSMOKE-CONTINUE-1782312653811"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want continuation delete guard")
	}
	_, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileManager,
		ToolName: "delete_file",
		Arguments: map[string]interface{}{
			"file_id": "file-2",
		},
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{
				SkillID:  skills.SkillFileManager,
				ToolName: "delete_file",
				Arguments: map[string]interface{}{
					"file_id": "file-1",
				},
				Result: map[string]interface{}{
					"file_name": "frozen-third-file.txt",
				},
			},
		},
	})
	if !blocked {
		t.Fatal("tool guard allowed a second delete_file after a continuation delete already succeeded")
	}
}

func TestSkillLoopToolCallGuardBlocksContinuationDeleteFromMetadataAfterApprovalResume(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"skill_invocations": []interface{}{
					map[string]interface{}{
						"kind":      "tool_call",
						"status":    "success",
						"skill_id":  skills.SkillFileManager,
						"tool_name": "delete_file",
						"arguments": map[string]interface{}{
							"file_id": "file-1",
						},
						"result": map[string]interface{}{
							"file_name": "frozen-third-file.txt",
						},
					},
				},
			},
		},
		parts: consoleFilesCreateCapabilityTestParts("\u7ee7\u7eed\u3002\u4efb\u52a1\u6807\u8bb0\uff1aSMOKE-CONTINUE-1782312653811"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopToolCallGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopToolCallGuard() = nil, want metadata-backed continuation delete guard")
	}
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillFileManager,
		ToolName: "delete_file",
		Arguments: map[string]interface{}{
			"file_id": "file-2",
		},
	})
	if !blocked {
		t.Fatal("tool guard allowed a second delete_file after metadata recorded a successful delete")
	}
	if !strings.Contains(result.SystemMessage, "frozen-third-file.txt") {
		t.Fatalf("guard result did not mention metadata deleted file: %#v", result)
	}
}

func TestSkillLoopFinalAnswerGuardDoesNotForceIntermediateNavigationForManagedFileCreate(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesCreateCapabilityTestParts("先切到智能体页读取第一个智能体，然后到文件管理创建并保存 aichat-one.txt"),
	}
	prepared.parts.SkillIDs = []string{skills.SkillConsoleNavigator, skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want managed file create guard")
	}
	_, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "文件 aichat-one.txt 已保存到文件管理。",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{
				SkillID:  skills.SkillFileManager,
				ToolName: "save_file_to_management",
				Arguments: map[string]interface{}{
					"source_type":  "tool_file",
					"tool_file_id": "tool-1",
					"filename":     "aichat-one.txt",
				},
				Result: map[string]interface{}{
					"file_name":      "aichat-one.txt",
					"source_file_id": "tool-1",
					"target":         "managed_file",
				},
			},
		},
	})
	if blocked {
		t.Fatal("guard forced an intermediate navigation target after the managed file create succeeded")
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
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "save_generated_file_to_file_management",
				Confidence: 0.91,
			},
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
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "navigate_console_page",
				TargetPage: "/console/work/task",
				Confidence: 0.91,
			},
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
		"low-risk observe/read/list step",
		"Choose the destination from the route catalog",
		"Do not say a different page has been opened unless",
		`"href":"/console/work/task"`,
		`"label":"定时任务"`,
		`"/console/files"`,
		`"/console/agents"`,
		`"/console/dataset"`,
		`"/console/db"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("console navigation guidance missing %q in:\n%s", want, content)
		}
	}
	if strings.Contains(content, "the next skill-loop action is constrained") ||
		strings.Contains(content, "before answering, asking the user, or using another business tool") ||
		strings.Contains(content, "required_next_tool") ||
		strings.Contains(content, "preferred_route_action") ||
		strings.Contains(content, "remaining_route_sequence") {
		t.Fatalf("console navigation guidance still contains hard required_next_tool wording:\n%s", content)
	}
}

func TestConsoleNavigationStrategyDoesNotScriptCompletedRouteSequence(t *testing.T) {
	parts := &chatRequestParts{
		Query:     "\u8bf7\u4f9d\u6b21\u6253\u5f00\u6587\u4ef6\u7ba1\u7406\u3001\u667a\u80fd\u4f53\u3001\u6570\u636e\u5e93\u3001\u6587\u4ef6\u7ba1\u7406",
		SkillIDs:  []string{skills.SkillConsoleNavigator},
		SkillMode: skillModeAuto,
		Surface:   aiChatSurfaceContextualSidebar,
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent:     "navigate_console_page",
			TargetPage: "/console/files",
			Confidence: 0.91,
		},
		OperationContext: map[string]interface{}{
			"completed_client_actions": []interface{}{
				map[string]interface{}{
					"action_type": "route_navigation",
					"status":      clientActionStatusSucceeded,
					"href":        "/console/files",
				},
				map[string]interface{}{
					"action_type": "route_navigation",
					"status":      clientActionStatusSucceeded,
					"href":        "/console/agents",
				},
				map[string]interface{}{
					"action_type": "route_navigation",
					"status":      clientActionStatusSucceeded,
					"href":        "/console/db",
				},
			},
		},
	}

	targets := consoleNavigationResolvedTargetsForParts(parts)
	if len(targets) != 1 {
		t.Fatalf("consoleNavigationResolvedTargetsForParts() = %#v, want one model target", targets)
	}
	if targets[0].Href != "/console/files" {
		t.Fatalf("target href = %s, want model /console/files", targets[0].Href)
	}

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatalf("strategy = nil")
	}
}

func TestStagedContinuationDeferredExecutionQueryCleansContinuePreamble(t *testing.T) {
	got := stagedContinuationDeferredExecutionQuery("\u7b49\u5f85\u6211\u8bf4\u201c\u7ee7\u7eed\u201d\u3002\u6211\u8bf4\u7ee7\u7eed\u540e\uff0c\u518d\u8fdb\u5165\u6587\u4ef6\u7ba1\u7406\uff0c\u521b\u5efa\u5e76\u4fdd\u5b58 smoke.svg")
	if !strings.HasPrefix(got, "\u518d\u8fdb\u5165\u6587\u4ef6\u7ba1\u7406") {
		t.Fatalf("deferred query = %q, want cleaned file-management instruction", got)
	}
	for _, blocked := range []string{"\u7ee7\u7eed", "\u6211\u8bf4"} {
		if strings.Contains(got, blocked) {
			t.Fatalf("deferred query = %q, still contains %q", got, blocked)
		}
	}
}

func TestStagedContinuationResumeQueryUsesDeferredPhaseOnly(t *testing.T) {
	originalGoal := "\u5148\u5207\u5230\u667a\u80fd\u4f53\u9875\u8bfb\u53d6\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53\u540d\u79f0\uff0c\u7136\u540e\u6682\u505c\u5e76\u7b49\u5f85\u6211\u8bf4\u201c\u7ee7\u7eed\u201d\u3002\u6211\u8bf4\u7ee7\u7eed\u540e\uff0c\u518d\u8fdb\u5165\u6587\u4ef6\u7ba1\u7406\uff0c\u521b\u5efa\u5e76\u4fdd\u5b58 smoke.txt \u548c smoke.svg\uff1btxt \u5185\u5bb9\u5199\u5165\u8bfb\u53d6\u5230\u7684\u667a\u80fd\u4f53\u540d\u79f0"
	parts := &chatRequestParts{
		Query: "\u7ee7\u7eed",
		RecentOperationPlans: []map[string]interface{}{{
			"status":              operationPlanStatusRunning,
			"original_user_goal":  originalGoal,
			"pending_next_action": "Wait for user continue",
		}},
	}
	got, ok := stagedContinuationResumeQuery(parts)
	if !ok {
		t.Fatal("stagedContinuationResumeQuery() ok = false, want true")
	}
	if !strings.HasPrefix(got, "\u518d\u8fdb\u5165\u6587\u4ef6\u7ba1\u7406") {
		t.Fatalf("resume query = %q, want deferred file-management phase", got)
	}
	if strings.Contains(got, "\u667a\u80fd\u4f53\u9875") {
		t.Fatalf("resume query = %q, still contains current-phase agent page instruction", got)
	}
}

func TestSkillLoopFinalAnswerGuardDoesNotForceConsoleNavigationToolCall(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "\u5e26\u6211\u53bb\u5b9a\u65f6\u4efb\u52a1\u9875\u9762",
			SkillIDs:  []string{skills.SkillConsoleNavigator},
			SkillMode: skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "navigate_console_page",
				TargetPage: "/console/work/task",
				Confidence: 0.91,
			},
		},
	}

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		return
	}
	_, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "\u5df2\u6253\u5f00\u5b9a\u65f6\u4efb\u52a1\u7ba1\u7406\u9875\u9762\u3002",
	})
	if blocked {
		t.Fatal("guard forced console navigation tool call for an advisory route target")
	}
}

func TestSkillLoopFinalAnswerGuardSkipsConsoleNavigationWhenTargetRouteAlreadyAvailable(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "open /console/agents",
		RuntimeContext: "route=/console/agents",
		SkillIDs:       []string{skills.SkillConsoleNavigator},
		SkillMode:      skillModeAuto,
		Surface:        aiChatSurfaceContextualSidebar,
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent:     "navigate_console_page",
			TargetPage: "/console/agents",
			Confidence: 0.91,
		},
		RawOperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"resource_type": "page",
					"resource_id":   "console.agents",
					"title":         "console.agents",
					"href":          "/console/agents",
					"metadata": map[string]interface{}{
						"route": "/console/agents",
					},
				},
			},
		},
	}
	parts.OperationContext = parts.RawOperationContext
	prepared := &PreparedChat{parts: parts}

	if guard := skillLoopFinalAnswerGuard(prepared); guard != nil {
		if result, blocked := guard(skillloop.FinalAnswerGuardRequest{Answer: "Already on the Agents page."}); blocked {
			t.Fatalf("navigation guard blocked current route answer: %#v", result)
		}
	}

	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		t.Fatal("contextualAIChatTurnStrategyFromParts() = nil, want strategy")
	}
	if strategy.RouteRequired {
		t.Fatal("strategy.RouteRequired = true, want false for current route")
	}
	evidence := skillLoopCompletionEvidence(prepared)()
	pageContext := mapFromOperationContext(evidence["page_context"])
	if len(pageContext) == 0 {
		t.Fatalf("page_context evidence = %#v, want current page evidence", evidence["page_context"])
	}
	if resources := operationItemsFromValue(pageContext["resources"]); len(resources) == 0 {
		t.Fatalf("page_context.resources = %#v, want compact resources", pageContext["resources"])
	}

	messages := skillLoopAdditionalSystemMessages(prepared)
	if len(messages) == 0 {
		t.Fatal("skillLoopAdditionalSystemMessages() = 0, want navigation guidance")
	}
	contents := make([]string, 0, len(messages))
	for _, message := range messages {
		contents = append(contents, messageContentText(message.Content))
	}
	content := strings.Join(contents, "\n")
	for _, want := range []string{
		"ZGI console navigation guidance",
		"Choose the destination from the route catalog",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("current-route navigation guidance missing %q in:\n%s", want, content)
		}
	}
}

func TestSkillLoopFinalAnswerGuardAllowsAgentDetailRouteForAgentsModule(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "open the first agent page and inspect its config",
			SkillIDs:  []string{skills.SkillConsoleNavigator},
			SkillMode: skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "navigate_console_page",
				TargetPage: "/console/agents",
				Confidence: 0.91,
			},
		},
	}

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		return
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

func TestSkillLoopFinalAnswerGuardBlocksIncompleteAgentBindingMutations(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query: "bind database table test2 with replace_agent_database_bindings and bind workflow \u65b0\u529f\u80fd\u6d4b\u8bd5 with replace_agent_workflow_bindings for this Agent",
			SkillIDs: []string{
				skills.SkillAgentManagement,
				skills.SkillConsoleNavigator,
			},
			SkillMode: skillModeAuto,
			RawOperationContext: map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"resource_type": "agent",
						"resource_id":   "agent-1",
						"title":         "Support Agent",
						"selected":      true,
						"can_edit":      true,
					},
				},
			},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"capability_goals": []interface{}{
					map[string]interface{}{
						"capability_id": agentCapabilityDatabaseBinding,
						"goal_action":   agentCapabilityActionBind,
						"required_binding_actions": map[string]interface{}{
							"database_bindings": "bind",
						},
					},
					map[string]interface{}{
						"capability_id": agentCapabilityWorkflowBinding,
						"goal_action":   agentCapabilityActionBind,
						"required_binding_actions": map[string]interface{}{
							"workflow_bindings": "bind",
						},
					},
				},
			},
		}},
	}

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want guard for explicit Agent binding mutation request")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "Database and workflow bindings are updated.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillAgentManagement, ToolName: "replace_agent_database_bindings"},
		},
	})
	if !blocked {
		t.Fatal("guard allowed final answer after database binding without workflow binding")
	}
	if result.SkillID != skills.SkillAgentManagement ||
		result.ToolName != "update_agent_config" ||
		!strings.Contains(result.SystemMessage, "update_agent_config.database_bindings") ||
		!strings.Contains(result.SystemMessage, "update_agent_config.workflow_bindings") ||
		!strings.Contains(result.SystemMessage, "accepted agent-management binding mutation tool for this turn is update_agent_config") {
		t.Fatalf("guard result = %#v, want unified config update instruction with missing workflow evidence", result)
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "Database and workflow bindings are updated.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillAgentManagement, ToolName: "replace_agent_database_bindings"},
			{SkillID: skills.SkillAgentManagement, ToolName: "replace_agent_workflow_bindings"},
		},
	})
	if blocked {
		t.Fatal("guard blocked after both requested binding mutation tools succeeded")
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "Database updated, but workflow binding failed.",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillAgentManagement, ToolName: "replace_agent_database_bindings"},
		},
		AttemptedToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillAgentManagement, ToolName: "replace_agent_workflow_bindings"},
		},
	})
	if blocked {
		t.Fatal("guard blocked after workflow binding was attempted and can be explained")
	}
}

func TestSkillLoopFinalAnswerGuardRequiresAgentConfigReadEvidence(t *testing.T) {
	configReadStepID := operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config")
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"steps": []interface{}{
						map[string]interface{}{
							"id":        configReadStepID,
							"status":    operationPlanStepStatusPending,
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "get_agent_config",
						},
					},
					"step_status": map[string]interface{}{
						configReadStepID: operationPlanStepStatusPending,
					},
				},
			},
		},
		parts: &chatRequestParts{
			Query:     "这个智能体启用了哪些 Skill？只告诉我当前状态，不要修改任何配置。",
			SkillIDs:  []string{skills.SkillAgentManagement},
			SkillMode: skillModeAuto,
			RawOperationContext: map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"resource_type": "agent",
						"resource_id":   "agent-1",
						"title":         "Support Agent",
						"selected":      true,
						"can_edit":      true,
					},
				},
			},
		},
	}

	if !agentManagementConfigReadRequested(prepared.parts.Query) {
		t.Fatal("agentManagementConfigReadRequested() = false, want read-only Skill binding state query")
	}
	if agentManagementConfigUpdateRequested(prepared.parts.Query) ||
		agentManagementIdentityUpdateRequested(prepared.parts.Query) ||
		agentManagementSkillBindingRequested(prepared.parts.Query) ||
		len(requiredAgentBindingMutationTools(prepared.parts.Query)) > 0 {
		t.Fatal("explicit read-only config query was classified as an Agent mutation")
	}
	if current := currentConsoleAgentID(prepared.parts); current != "agent-1" {
		t.Fatalf("currentConsoleAgentID() = %q, want agent-1", current)
	}

	guard := skillLoopAgentManagementFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopAgentManagementFinalAnswerGuard() = nil, want Agent config read guard without console navigator")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "当前未启用任何 Skill。",
	})
	if !blocked {
		t.Fatal("guard allowed Agent config answer without get_agent_config evidence")
	}
	if result.SkillID != skills.SkillAgentManagement ||
		result.ToolName != "get_agent_config" ||
		!strings.Contains(result.SystemMessage, "Call agent-management/get_agent_config") {
		t.Fatalf("guard result = %#v, want get_agent_config instruction", result)
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "根据配置读取结果，当前未启用任何 Skill。",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillAgentManagement, ToolName: "get_agent_config"},
		},
	})
	if blocked {
		t.Fatal("guard blocked after successful get_agent_config evidence")
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "我无法读取当前配置：get_agent_config 调用失败。",
		AttemptedToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillAgentManagement, ToolName: "get_agent_config"},
		},
	})
	if blocked {
		t.Fatal("guard blocked after attempted get_agent_config evidence that can be explained")
	}
}

func TestSkillLoopFinalAnswerGuardRequiresFirstVisibleAgentConfigReadEvidence(t *testing.T) {
	query := "\u8bf7\u53ea\u8bfb\u68c0\u67e5\u5f53\u524d\u9875\u9762\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53\u7684\u914d\u7f6e\uff1a\u8bfb\u53d6\u5b83\u7684\u57fa\u7840\u4fe1\u606f\u3001\u8fd0\u884c\u914d\u7f6e\u548c\u53ef\u7f16\u8f91\u9879\u76ee\uff0c\u4e0d\u8981\u4fee\u6539\u4efb\u4f55\u8d44\u4ea7\u3002"
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status": operationPlanStatusRunning,
					"steps": []interface{}{
						map[string]interface{}{
							"id":        operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"),
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "get_agent_config",
							"status":    operationPlanStepStatusPending,
						},
					},
				},
			},
		},
		parts: &chatRequestParts{
			Query:          query,
			SkillIDs:       []string{skills.SkillAgentManagement},
			SkillMode:      skillModeAuto,
			RuntimeContext: "/console/agents",
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:             "manage_agent_asset",
				TargetVisibleIndex: 1,
			},
			RawOperationContext: map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"resource_type": "page",
						"resource_id":   "/console/agents",
						"title":         "Agent list",
					},
					map[string]interface{}{
						"resource_type": "agent",
						"resource_id":   "agent-first",
						"title":         "First Agent",
						"href":          "/console/agents/agent-first/agent",
						"can_edit":      true,
					},
					map[string]interface{}{
						"resource_type": "agent",
						"resource_id":   "agent-second",
						"title":         "Second Agent",
						"href":          "/console/agents/agent-second/agent",
						"can_edit":      true,
					},
				},
			},
		},
	}

	if current := currentConsoleAgentID(prepared.parts); current != "" {
		t.Fatalf("currentConsoleAgentID() = %q, want empty for multi-Agent list without selection", current)
	}
	if target := agentManagementConfigReadTargetID(prepared.parts); target != "agent-first" {
		t.Fatalf("agentManagementConfigReadTargetID() = %q, want first visible Agent", target)
	}
	guard := skillLoopAgentManagementFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopAgentManagementFinalAnswerGuard() = nil, want first visible Agent config read guard")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "\u6839\u636e\u9875\u9762\u4e0a\u4e0b\u6587\uff0c\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53\u7684\u6a21\u578b\u662f DeepSeek Chat\u3002",
	})
	if !blocked {
		t.Fatal("guard allowed first visible Agent config answer without get_agent_config evidence")
	}
	if result.SkillID != skills.SkillAgentManagement || result.ToolName != "get_agent_config" {
		t.Fatalf("guard result = %#v, want agent-management/get_agent_config", result)
	}
	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "\u6839\u636e get_agent_config \u7ed3\u679c\uff0c\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53\u914d\u7f6e\u5df2\u8bfb\u53d6\u3002",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillAgentManagement, ToolName: "get_agent_config"},
		},
	})
	if blocked {
		t.Fatal("guard blocked first visible Agent config answer after successful get_agent_config evidence")
	}
}

func TestSkillLoopPlanToolGuardBlocksUnrequestedAgentConfigMutationForReadOnlyNavigation(t *testing.T) {
	query := "\u8bf7\u6253\u5f00\u521a\u521a\u521b\u5efa\u7684\u6d4b\u8bd5\u667a\u80fd\u4f53 GOAL-CONFIG \u7684\u8be6\u60c5/\u7f16\u8f91\u9875\u9762\u3002\u53ea\u505a\u5bfc\u822a\u548c\u786e\u8ba4\u5f53\u524d\u9875\u9762\u4e0a\u4e0b\u6587\uff0c\u4e0d\u8981\u4fee\u6539\u4efb\u4f55\u914d\u7f6e\u3002"
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status":             operationPlanStatusRunning,
					"original_user_goal": query,
					"steps": []interface{}{
						map[string]interface{}{
							"id":        operationPlanRouteStepID("/console/agents/agent-1/agent", 1),
							"skill_id":  skills.SkillConsoleNavigator,
							"tool_name": "navigate",
							"status":    operationPlanStepStatusCompleted,
						},
						map[string]interface{}{
							"id":        operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"),
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "get_agent_config",
							"status":    operationPlanStepStatusCompleted,
						},
					},
				},
			},
		},
		parts: &chatRequestParts{
			Query:          query,
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents/agent-1/agent",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement},
			SkillMode:      skillModeAuto,
		},
	}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement},
		Tools: []skills.SkillToolDefinition{{
			Name: "update_agent_config",
			Governance: &toolgovernance.Manifest{
				Effect:    toolgovernance.EffectUpdate,
				AssetType: "agent",
				RiskLevel: toolgovernance.RiskLevelMedium,
			},
		}},
	}}}

	guard := skillLoopPlanToolCallGuardWithResolved(prepared, resolved)
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Arguments: map[string]interface{}{
			"agent_id":          "agent-1",
			"system_prompt":     "unexpected prompt",
			"home_title":        "unexpected title",
			"input_placeholder": "unexpected placeholder",
		},
	})

	if blocked {
		t.Fatalf("plan tool guard blocked update_agent_config through removed read-only text matcher: %#v", result)
	}
}

func TestSkillLoopPlanToolGuardBlocksStalePlannedAgentConfigMutationForLatestReadOnlyRequest(t *testing.T) {
	query := "复测只读配置闭环：请只读取当前 Agent 配置并回答当前首页标题、模型 provider/model、绑定的 Skill/知识库/数据库表/工作流数量。不要修改任何配置，不要发起审批，不要查询可用模型或候选资源。"
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status":             operationPlanStatusRunning,
					"original_user_goal": "请修改当前智能体配置",
					"steps": []interface{}{
						map[string]interface{}{
							"id":        operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"),
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "get_agent_config",
							"status":    operationPlanStepStatusCompleted,
						},
						map[string]interface{}{
							"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "update_agent_config",
							"status":    operationPlanStepStatusPending,
						},
					},
				},
			},
		},
		parts: &chatRequestParts{
			Query:          query,
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents/agent-1/agent",
			SkillIDs:       []string{skills.SkillAgentManagement},
			SkillMode:      skillModeAuto,
		},
	}
	prepared.parts.Query = strings.Join([]string{
		"Regression read-only config loop:",
		"only read the current Agent config and answer the current home title, provider/model,",
		"and the number of bound Skill/knowledge/database table/workflow resources.",
		"Do not modify any configuration, do not request approval,",
		"and do not query available models or candidate resources.",
	}, " ")
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement},
		Tools: []skills.SkillToolDefinition{{
			Name: "update_agent_config",
			Governance: &toolgovernance.Manifest{
				Effect:    toolgovernance.EffectUpdate,
				AssetType: "agent",
				RiskLevel: toolgovernance.RiskLevelMedium,
			},
		}},
	}}}

	guard := skillLoopPlanToolCallGuardWithResolved(prepared, resolved)
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Arguments: map[string]interface{}{
			"agent_id":   "agent-1",
			"home_title": "unexpected title",
		},
	})

	if blocked {
		t.Fatalf("plan tool guard blocked update_agent_config through removed latest-read-only text matcher: %#v", result)
	}
}

func TestSkillLoopReadOnlyCandidateLookupUsesCapabilityGoalsOverQuery(t *testing.T) {
	query := strings.Join([]string{
		"Only read the current Agent config and answer the current home title.",
		"Do not modify any configuration.",
	}, " ")
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status":             operationPlanStatusRunning,
					"planning_mode":      "phase_only_model_decides",
					"original_user_goal": query,
					"capability_goals": mapsToInterfaceSlice(agentCapabilityGoalsToMaps([]AIChatAgentCapabilityGoal{
						agentCapabilityGoalWithDefaults(AIChatAgentCapabilityGoal{
							CapabilityID:         agentCapabilitySystemPrompt,
							GoalAction:           agentCapabilityActionUpdate,
							RequiredConfigFields: []string{"system_prompt"},
						}),
					})),
				},
			},
		},
		parts: &chatRequestParts{
			Query:          query,
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents/agent-1/agent",
			SkillIDs:       []string{skills.SkillAgentManagement},
			SkillMode:      skillModeAuto,
		},
	}

	if goals := preparedAgentCapabilityGoals(prepared); !agentCapabilityGoalsRequireConfigMutation(goals) {
		t.Fatalf("preparedAgentCapabilityGoals() = %#v, want mutation capability goal", goals)
	}
	if skillLoopShouldAllowReadOnlyAgentCandidateLookup(prepared, skills.SkillAgentManagement, "list_available_models") {
		t.Fatal("skillLoopShouldAllowReadOnlyAgentCandidateLookup() = true, want false when operation_plan capability_goals require mutation")
	}
}

func TestTemporaryArtifactProducerPrefersModelCapabilityHint(t *testing.T) {
	chartParts := &chatRequestParts{
		Query:    "generate an SVG report",
		SkillIDs: []string{skills.SkillFileGenerator, skills.SkillChartGenerator},
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent:                  "generate_temporary_file_artifact",
			RecommendedCapabilities: []string{"chart_artifact"},
		},
	}
	if skillID, toolName := temporaryFileGenerateRequiredTool(chartParts); skillID != skills.SkillChartGenerator || toolName != "generate_chart" {
		t.Fatalf("temporaryFileGenerateRequiredTool(chart hint) = %s/%s, want chart-generator/generate_chart", skillID, toolName)
	}

	fileParts := &chatRequestParts{
		Query:    "generate a pie chart SVG",
		SkillIDs: []string{skills.SkillFileGenerator, skills.SkillChartGenerator},
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent:                  "generate_temporary_file_artifact",
			RecommendedCapabilities: []string{"file_artifact"},
		},
	}
	if skillID, toolName := temporaryFileGenerateRequiredTool(fileParts); skillID != skills.SkillFileGenerator || toolName != "generate_file" {
		t.Fatalf("temporaryFileGenerateRequiredTool(file hint) = %s/%s, want file-generator/generate_file", skillID, toolName)
	}

	unclassifiedParts := &chatRequestParts{
		Query:    "generate a pie chart SVG",
		SkillIDs: []string{skills.SkillFileGenerator, skills.SkillChartGenerator},
	}
	if skillID, toolName := temporaryFileGenerateRequiredTool(unclassifiedParts); skillID != skills.SkillFileGenerator || toolName != "generate_file" {
		t.Fatalf("temporaryFileGenerateRequiredTool(no model hint) = %s/%s, want file-generator/generate_file", skillID, toolName)
	}
}

func TestSkillLoopPlanToolGuardAllowsAgentConfigUpdateWithExcludedFields(t *testing.T) {
	query := strings.Join([]string{
		"Update current Agent runtime config: set system prompt to CONFIG-SMOKE prompt;",
		"set home title to CONFIG-SMOKE home;",
		"set input placeholder to CONFIG-SMOKE placeholder;",
		"set opening questions to Check config, Generate a test reply, Explain capability.",
		"Do not modify name, description, icon, model, bindings, memory, or file upload.",
		"After completion check config again and verify the updated fields.",
	}, " ")
	if !agentManagementConfigUpdateRequested(query) {
		t.Fatal("agentManagementConfigUpdateRequested() = false, want explicit runtime config update")
	}
	if agentManagementExplicitReadOnlyConfigCheck(query) {
		t.Fatal("agentManagementExplicitReadOnlyConfigCheck() = true, want false for update request with excluded fields")
	}
	capabilityGoals := []AIChatAgentCapabilityGoal{
		agentCapabilityGoalWithDefaults(AIChatAgentCapabilityGoal{
			CapabilityID:         agentCapabilitySystemPrompt,
			GoalAction:           agentCapabilityActionUpdate,
			RequiredConfigFields: []string{"system_prompt"},
		}),
		agentCapabilityGoalWithDefaults(AIChatAgentCapabilityGoal{
			CapabilityID:         agentCapabilitySuggestedQuestion,
			GoalAction:           agentCapabilityActionUpdate,
			RequiredConfigFields: []string{"suggested_questions"},
		}),
	}
	fields := agentCapabilityGoalsExpectedConfigFields(capabilityGoals)
	for _, want := range []string{"system_prompt"} {
		if !stringSliceContainsFold(fields, want) {
			t.Fatalf("expected config fields = %#v, missing requested field %s", fields, want)
		}
	}
	for _, unexpected := range []string{"model", "agent_memory_enabled", "file_upload_enabled"} {
		if stringSliceContainsFold(fields, unexpected) {
			t.Fatalf("expected config fields = %#v, want excluded field %s absent", fields, unexpected)
		}
	}

	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status":             operationPlanStatusRunning,
					"original_user_goal": query,
					"capability_goals":   mapsToInterfaceSlice(agentCapabilityGoalsToMaps(capabilityGoals)),
					"steps": []interface{}{
						map[string]interface{}{
							"id":        operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config"),
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "get_agent_config",
							"status":    operationPlanStepStatusCompleted,
						},
						map[string]interface{}{
							"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "update_agent_config",
							"status":    operationPlanStepStatusPending,
						},
					},
				},
			},
		},
		parts: &chatRequestParts{
			Query:          query,
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents/agent-1/agent",
			SkillIDs:       []string{skills.SkillAgentManagement},
			SkillMode:      skillModeAuto,
		},
	}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement},
		Tools: []skills.SkillToolDefinition{{
			Name: "update_agent_config",
			Governance: &toolgovernance.Manifest{
				Effect:    toolgovernance.EffectUpdate,
				AssetType: "agent",
				RiskLevel: toolgovernance.RiskLevelMedium,
			},
		}},
	}}}

	guard := skillLoopPlanToolCallGuardWithResolved(prepared, resolved)
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Arguments: map[string]interface{}{
			"agent_id":            "agent-1",
			"system_prompt":       "CONFIG-SMOKE prompt",
			"home_title":          "CONFIG-SMOKE home",
			"input_placeholder":   "CONFIG-SMOKE placeholder",
			"suggested_questions": []interface{}{"Check config", "Generate a test reply", "Explain capability"},
		},
	})

	if blocked {
		t.Fatalf("guard blocked requested update_agent_config with excluded fields: %#v", result)
	}
}

func TestAgentManagementExplicitlyNegatedCreateDeleteDoesNotTriggerCreateDeleteIntent(t *testing.T) {
	query := strings.Join([]string{
		"Update the current Agent description to AIChat edit loop regression.",
		"Set icon to puzzle and set home title to Edit loop regression.",
		"Do not create or delete Agents.",
		"After approval, reread the config and verify only those requested fields changed.",
	}, " ")
	if agentManagementCreateRequested(query) {
		t.Fatalf("agentManagementCreateRequested(%q) = true, want false for negated create", query)
	}
	if agentManagementDeleteRequested(query) {
		t.Fatalf("agentManagementDeleteRequested(%q) = true, want false for negated delete", query)
	}
}

func TestSkillLoopPlanToolGuardDoesNotUseOriginalGoalTextToForbidAgentDelete(t *testing.T) {
	originalGoal := strings.Join([]string{
		"Update the current Agent description to AIChat edit loop regression.",
		"Set icon to puzzle and set home title to Edit loop regression.",
		"Do not create or delete Agents.",
		"After approval, reread the config and verify only those requested fields changed.",
	}, " ")
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status":             operationPlanStatusRunning,
					"original_user_goal": originalGoal,
					"steps": []interface{}{
						map[string]interface{}{
							"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity"),
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "update_agent_identity",
							"status":    operationPlanStepStatusCompleted,
						},
						map[string]interface{}{
							"id":        operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config"),
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "update_agent_config",
							"status":    operationPlanStepStatusCompleted,
						},
					},
				},
			},
		},
		parts: &chatRequestParts{
			Query:          "approved",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents/agent-1/agent",
			SkillIDs:       []string{skills.SkillAgentManagement},
			SkillMode:      skillModeAuto,
		},
	}

	guard := skillLoopPlanToolCallGuard(prepared)
	result, blocked := guard(skillloop.ToolCallGuardRequest{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agents",
		Arguments: map[string]interface{}{
			"agent_ids": []interface{}{"agent-1"},
		},
	})
	if blocked {
		t.Fatalf("plan tool guard blocked delete_agents through removed original-goal text matcher: %#v", result)
	}
}

func TestAgentManagementSkillBindingIntentAllowsExplicitBindWithNoRepeatClause(t *testing.T) {
	query := "\u8bf7\u628a Skill\u300c\u56fe\u8868\u751f\u6210\u5668\u300d\u7ed1\u5b9a\u5230\u8fd9\u4e2a\u667a\u80fd\u4f53\uff1b\u5982\u679c\u5b83\u5df2\u7ecf\u7ed1\u5b9a\uff0c\u8bf7\u5982\u5b9e\u8bf4\u660e\u5e76\u4e0d\u8981\u91cd\u590d\u7ed1\u5b9a\u3002"
	if !agentManagementSkillBindingRequested(query) {
		t.Fatal("agentManagementSkillBindingRequested() = false, want explicit Skill bind intent despite no-repeat clause")
	}
	if !agentManagementConfigUpdateRequested(query) {
		t.Fatal("agentManagementConfigUpdateRequested() = false, want update_agent_config to match explicit Skill bind intent")
	}
}

func TestAgentManagementSkillBindingIntentAllowsExplicitBindWithPostReadVerificationClause(t *testing.T) {
	query := "\u8bf7\u5148\u8bfb\u53d6\u5b83\u5f53\u524d\u771f\u5b9e\u914d\u7f6e\uff1b\u5982\u679c Skill\u300c\u56fe\u8868\u751f\u6210\u5668\u300d\u5f53\u524d\u672a\u7ed1\u5b9a\uff0c\u8bf7\u628a\u5b83\u7ed1\u5b9a\u5230\u8fd9\u4e2a\u667a\u80fd\u4f53\uff1b\u5982\u679c\u5df2\u7ecf\u7ed1\u5b9a\uff0c\u8bf7\u5982\u5b9e\u8bf4\u660e\u5e76\u4e0d\u8981\u91cd\u590d\u7ed1\u5b9a\u3002\u66f4\u65b0\u5b8c\u6210\u540e\u5fc5\u987b\u518d\u6b21\u8bfb\u53d6\u8be5\u667a\u80fd\u4f53\u914d\u7f6e\u9a8c\u8bc1\uff0c\u5e76\u8bf4\u660e\u590d\u8bfb\u914d\u7f6e\u540e\u5b83\u662f\u5426\u5904\u4e8e\u5df2\u7ed1\u5b9a\u72b6\u6001\u3002"
	if !agentManagementSkillBindingRequested(query) {
		t.Fatal("agentManagementSkillBindingRequested() = false, want explicit Skill bind intent despite post-read state question")
	}
	if !agentManagementConfigUpdateRequested(query) {
		t.Fatal("agentManagementConfigUpdateRequested() = false, want update_agent_config to match explicit Skill bind intent with post-read verification")
	}
}

func TestSkillLoopUserInputGuardBlocksConsoleNavigationClarification(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "\u5e26\u6211\u53bb\u5b9a\u65f6\u4efb\u52a1\u9875\u9762",
			SkillIDs:  []string{skills.SkillConsoleNavigator},
			SkillMode: skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:     "navigate_console_page",
				TargetPage: "/console/work/task",
				Confidence: 0.91,
			},
		},
	}

	guard := skillLoopUserInputGuard(prepared)
	if guard == nil {
		return
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

func TestSkillLoopUserInputGuardDoesNotRewriteResolvedAgentBatchDeleteConfirmation(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleAgentsVisibleTargetsTestParts("delete the first two visible agents on this page"),
	}

	guard := skillLoopUserInputGuard(prepared)
	if guard == nil {
		return
	}
	_, blocked := guard(skillloop.UserInputGuardRequest{
		Message: "I found two target Agents. Please confirm deletion.",
		Questions: []map[string]interface{}{
			{
				"id":       "confirm_delete",
				"question": "Confirm deleting these Agents?",
				"options": []map[string]interface{}{
					{"label": "Confirm delete"},
				},
			},
		},
	})
	if blocked {
		t.Fatal("guard rewrote Agent delete confirmation; model-decides flow should rely on tool governance instead")
	}
}

func TestSkillLoopUserInputGuardDoesNotRewriteAgentConfigMutationConfirmation(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "\u8bf7\u628a Skill\u300c\u56fe\u8868\u751f\u6210\u5668\u300d\u7ed1\u5b9a\u5230\u8fd9\u4e2a\u667a\u80fd\u4f53\uff1b\u9700\u8981\u5ba1\u6279\u65f6\u6211\u4f1a\u540c\u610f\u3002",
			SkillIDs:  []string{skills.SkillAgentManagement},
			SkillMode: skillModeAuto,
			RawOperationContext: map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"resource_type": "agent",
						"resource_id":   "agent-1",
						"title":         "Support Agent",
						"selected":      true,
						"can_edit":      true,
					},
				},
			},
		},
	}

	guard := skillLoopUserInputGuard(prepared)
	if guard == nil {
		return
	}
	_, blocked := guard(skillloop.UserInputGuardRequest{
		Message: "\u5df2\u786e\u8ba4\u56fe\u8868\u751f\u6210\u5668\u5f53\u524d\u672a\u7ed1\u5b9a\uff0c\u8bf7\u786e\u8ba4\u662f\u5426\u6267\u884c\u7ed1\u5b9a\u64cd\u4f5c\u3002",
		Questions: []map[string]interface{}{
			{
				"id":       "confirm_bind",
				"question": "\u662f\u5426\u786e\u8ba4\u5c06 Skill\u300c\u56fe\u8868\u751f\u6210\u5668\u300d\u7ed1\u5b9a\u5230\u8be5\u667a\u80fd\u4f53\uff1f",
				"options": []map[string]interface{}{
					{"label": "\u786e\u8ba4\uff0c\u6267\u884c\u7ed1\u5b9a"},
				},
			},
		},
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillAgentManagement, ToolName: "get_agent_config"},
		},
	})
	if blocked {
		t.Fatal("guard rewrote Agent config confirmation; model-decides flow should rely on tool governance instead")
	}
}

func TestSkillLoopUserInputGuardSkipsAgentConfirmationRewriteForModelDecidesPlan(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "\u8bf7\u628a Skill\u300c\u56fe\u8868\u751f\u6210\u5668\u300d\u7ed1\u5b9a\u5230\u8fd9\u4e2a\u667a\u80fd\u4f53\uff1b\u9700\u8981\u5ba1\u6279\u65f6\u6211\u4f1a\u540c\u610f\u3002",
			SkillIDs:  []string{skills.SkillAgentManagement},
			SkillMode: skillModeAuto,
		},
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"planning_mode":    "phase_only_model_decides",
					"tool_choice_mode": aiChatTurnToolChoiceModelDecides,
				},
			},
		},
	}

	if guard := skillLoopUserInputGuard(prepared); guard != nil {
		t.Fatalf("skillLoopUserInputGuard() = %#v, want nil for model-decides operation plan", guard)
	}
}

func TestSkillLoopPlanAmendmentAllowsModelDecidesFileToolsWithoutContract(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:     "explain the visible files",
			SkillIDs:  []string{skills.SkillFileReader, skills.SkillFileManager},
			SkillMode: skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent: "answer_or_explain_zgi_context",
			},
		},
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status":           operationPlanStatusRunning,
					"planning_mode":    "phase_only_model_decides",
					"tool_choice_mode": aiChatTurnToolChoiceModelDecides,
				},
			},
		},
	}

	for _, tc := range []struct {
		name     string
		skillID  string
		toolName string
	}{
		{name: "read file", skillID: skills.SkillFileReader, toolName: "read_file"},
		{name: "list files", skillID: skills.SkillFileReader, toolName: "list_visible_files"},
		{name: "delete file", skillID: skills.SkillFileManager, toolName: "delete_file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !skillLoopCanAmendOperationPlanForTool(prepared, tc.skillID, tc.toolName) {
				t.Fatalf("skillLoopCanAmendOperationPlanForTool(%s, %s) = false, want true under model-decides plan", tc.skillID, tc.toolName)
			}
		})
	}

	if !skillLoopShouldAllowUnplannedEvidenceTool(prepared, skills.SkillFileReader, "read_file") {
		t.Fatal("skillLoopShouldAllowUnplannedEvidenceTool(read_file) = false, want true under model-decides plan")
	}
}

func TestSkillLoopModelDecidesSafetyGuardLetsToolValidateSemanticErrors(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			SkillIDs: []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{}},
	}
	requests := []skillloop.ToolCallGuardRequest{
		{
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "update_agent_config",
			Arguments: map[string]interface{}{"agent_id": "agent-1", operationPlanConfigGoalKey: "bind file generator"},
		},
		{
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "update_agent_config",
			Arguments: map[string]interface{}{"agent_id": "agent-1", "model": "deepseek-v4-flash"},
		},
		{
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "update_agent_config",
			Arguments: map[string]interface{}{"agent_id": "agent-1", "add_enabled_skill_ids": []interface{}{"File Generator"}},
		},
		{
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "update_agent_identity",
			Arguments: map[string]interface{}{"agent_id": "agent-1"},
		},
	}

	for _, req := range requests {
		if result, blocked := skillLoopModelDecidesSafetyToolCallGuard(prepared, nil, req); blocked {
			t.Fatalf("model-decides safety guard blocked %#v with result %#v; want tool/runtime validation to handle semantic errors", req, result)
		}
	}
}

func TestSkillLoopModelDecidesSafetyGuardStillBlocksDuplicateMutation(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			SkillIDs: []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{}},
	}
	req := skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentManagement,
		ToolName:  "delete_agent",
		Arguments: map[string]interface{}{"agent_id": "agent-1"},
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{
				SkillID:   skills.SkillAgentManagement,
				ToolName:  "delete_agent",
				Arguments: map[string]interface{}{"agent_id": "agent-1"},
			},
		},
	}

	result, blocked := skillLoopModelDecidesSafetyToolCallGuard(prepared, nil, req)
	if !blocked {
		t.Fatal("model-decides safety guard allowed duplicate asset mutation")
	}
	if result.SkillID != skills.SkillAgentManagement || result.ToolName != "delete_agent" {
		t.Fatalf("guard result = %#v, want agent-management/delete_agent duplicate guard", result)
	}
}

func TestSkillLoopModelDecidesSafetyGuardAllowsDistinctAgentConfigUpdates(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			SkillIDs: []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{}},
	}
	req := skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentManagement,
		ToolName:  "update_agent_config",
		Arguments: map[string]interface{}{"agent_id": "agent-1", "model": "deepseek-v4-flash"},
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{
				SkillID:   skills.SkillAgentManagement,
				ToolName:  "update_agent_config",
				Arguments: map[string]interface{}{"agent_id": "agent-1", "add_enabled_skill_ids": []interface{}{"file-generator"}},
			},
		},
	}

	if result, blocked := skillLoopModelDecidesSafetyToolCallGuard(prepared, nil, req); blocked {
		t.Fatalf("model-decides safety guard blocked distinct config update with result %#v; want model/tool loop to continue", result)
	}
}

func TestSkillLoopModelDecidesSafetyGuardBlocksExactAgentConfigUpdateDuplicate(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			SkillIDs: []string{skills.SkillAgentManagement},
		},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{}},
	}
	args := map[string]interface{}{"agent_id": "agent-1", "model": "deepseek-v4-flash"}
	req := skillloop.ToolCallGuardRequest{
		SkillID:   skills.SkillAgentManagement,
		ToolName:  "update_agent_config",
		Arguments: args,
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{
			{
				SkillID:   skills.SkillAgentManagement,
				ToolName:  "update_agent_config",
				Arguments: args,
			},
		},
	}

	result, blocked := skillLoopModelDecidesSafetyToolCallGuard(prepared, nil, req)
	if !blocked {
		t.Fatal("model-decides safety guard allowed exact duplicate config update")
	}
	if result.SkillID != skills.SkillAgentManagement || result.ToolName != "update_agent_config" {
		t.Fatalf("guard result = %#v, want agent-management/update_agent_config duplicate guard", result)
	}
}

func TestSkillLoopUserInputGuardDoesNotRewriteSidebarAgentDeleteConfirmationWithoutInitialSkillID(t *testing.T) {
	parts := consoleAgentsVisibleTargetsTestParts("\u8bf7\u6279\u91cf\u5220\u9664\u5f53\u524d\u9875\u9762\u524d\u4e24\u4e2a\u540d\u5b57\u4ee5 AICHAT-GOAL-BIND-SMOKE \u5f00\u5934\u7684\u6d4b\u8bd5\u667a\u80fd\u4f53\u3002\u53ea\u5220\u9664\u8fd9\u4e24\u4e2a\u6d4b\u8bd5\u667a\u80fd\u4f53\u3002")
	parts.SkillIDs = []string{skills.SkillConsoleNavigator}
	prepared := &PreparedChat{parts: parts}

	guard := skillLoopUserInputGuard(prepared)
	if guard == nil {
		return
	}
	_, blocked := guard(skillloop.UserInputGuardRequest{
		Message: "\u5373\u5c06\u6279\u91cf\u5220\u9664\u5f53\u524d\u9875\u9762\u524d\u4e24\u4e2a AICHAT-GOAL-BIND-SMOKE \u5f00\u5934\u7684\u6d4b\u8bd5\u667a\u80fd\u4f53\u3002\u9700\u8981\u4f60\u786e\u8ba4\u624d\u80fd\u6267\u884c\u3002",
		Questions: []map[string]interface{}{
			{
				"id":       "confirm_delete",
				"question": "\u786e\u8ba4\u5220\u9664 Visible Agent One \u548c Visible Agent Two \u8fd9\u4e24\u4e2a\u6d4b\u8bd5\u667a\u80fd\u4f53\u5417\uff1f",
				"options": []map[string]interface{}{
					{"label": "\u786e\u8ba4\u5220\u9664"},
					{"label": "\u53d6\u6d88"},
				},
			},
		},
	})
	if blocked {
		t.Fatal("guard rewrote sidebar Agent delete confirmation; model should choose governed tools itself")
	}
}

func TestSkillLoopUserInputGuardDoesNotRewriteNamedAgentDeleteConfirmation(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleAgentsVisibleTargetsTestParts("delete Visible Agent One and Visible Agent Two"),
	}

	guard := skillLoopUserInputGuard(prepared)
	if guard == nil {
		return
	}
	_, blocked := guard(skillloop.UserInputGuardRequest{
		Message: "These two Agents are resolved. Please confirm before I continue.",
		Questions: []map[string]interface{}{
			{
				"id":       "confirm_delete",
				"question": "Do you approve deleting Visible Agent One and Visible Agent Two?",
				"options": []map[string]interface{}{
					{"label": "Approve deletion"},
				},
			},
		},
	})
	if blocked {
		t.Fatal("guard rewrote named Agent delete confirmation; model should choose governed tools itself")
	}
}

func TestSkillLoopUserInputGuardAllowsAmbiguousAgentDeleteClarification(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleAgentsVisibleTargetsTestParts("delete an agent"),
	}

	guard := skillLoopUserInputGuard(prepared)
	if guard == nil {
		return
	}
	_, blocked := guard(skillloop.UserInputGuardRequest{
		Message: "Which Agent should I delete?",
		Questions: []map[string]interface{}{
			{
				"id":       "which_agent",
				"question": "Which Agent should I delete?",
				"options": []map[string]interface{}{
					{"label": "Visible Agent One"},
					{"label": "Visible Agent Two"},
				},
			},
		},
	})
	if blocked {
		t.Fatal("guard blocked an actually ambiguous Agent delete clarification")
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

func TestSkillLoopFinalAnswerGuardDoesNotForceConsoleFilesReadToolCall(t *testing.T) {
	prepared := preparedConsoleFilesGuardReadTest("\u5e2e\u6211\u6458\u8981\u7b2c\u4e8c\u4e2a Excel \u5e76\u7ffb\u8bd1")
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto

	if guard := skillLoopFinalAnswerGuard(prepared); guard != nil {
		if result, blocked := guard(skillloop.FinalAnswerGuardRequest{
			Answer: "The model should decide whether file content evidence is needed.",
		}); blocked {
			t.Fatalf("skillLoopFinalAnswerGuard blocked read-file completion with pre-resolved tool requirement: %#v", result)
		}
	}
}

func TestSkillLoopFinalAnswerGuardDoesNotPreResolveChineseReadOrdinal(t *testing.T) {
	prepared := preparedConsoleFilesGuardReadTest("\u8bfb\u7b2c\u56db\u4e2a\u6587\u4ef6")
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto

	if guard := skillLoopFinalAnswerGuard(prepared); guard != nil {
		if result, blocked := guard(skillloop.FinalAnswerGuardRequest{
			Answer: "The model should resolve the ordinal from visible_files if it needs a tool.",
		}); blocked {
			t.Fatalf("skillLoopFinalAnswerGuard blocked Chinese ordinal read with pre-resolved tool requirement: %#v", result)
		}
	}
}

func TestSkillLoopFinalAnswerGuardDoesNotPreResolveRecentFileRead(t *testing.T) {
	query := "\u8bf7\u57fa\u4e8e\u521a\u624d\u90a3\u4e2a\u6587\u4ef6\u63d0\u53d6\u7f34\u8d39\u8d26\u6237"
	prepared := &PreparedChat{
		parts: consoleFilesSnapshotTestParts(query, []consoleFilesTestFile{
			{ID: "file-1", Name: "invoice.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.ModelTurnIntent = &AIChatModelTurnIntent{
		Intent:     "read_visible_file_content",
		Confidence: 0.91,
	}
	prepared.parts.RecentAssetCandidates = []ResourceCandidate{{
		Type:      resourceTypeFile,
		ID:        "file-1",
		Name:      "invoice.xlsx",
		Source:    "recent_execution.read_file",
		Extension: "xlsx",
		Recent:    true,
	}}

	if guard := skillLoopFinalAnswerGuard(prepared); guard != nil {
		if result, blocked := guard(skillloop.FinalAnswerGuardRequest{
			Answer: "The model should decide whether the recent file needs to be read again.",
		}); blocked {
			t.Fatalf("skillLoopFinalAnswerGuard blocked recent-file read with pre-resolved tool requirement: %#v", result)
		}
	}
}

func TestSkillLoopUserInputGuardDoesNotBlockConsoleFilesClarificationFromPreResolvedTarget(t *testing.T) {
	prepared := preparedConsoleFilesGuardReadTest("\u8bf7\u8bfb\u53d6\u7b2c\u4e8c\u4e2a Excel \u6587\u4ef6\uff0c\u5e76\u6458\u8981")
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto

	if guard := skillLoopUserInputGuard(prepared); guard != nil {
		result, blocked := guard(skillloop.UserInputGuardRequest{
			Message: "The model may ask a clarification when the visible file target is genuinely ambiguous.",
			Questions: []map[string]interface{}{{
				"id":       "which_excel",
				"question": "Which Excel file should be read?",
				"options": []map[string]interface{}{
					{"label": "budget-q1.xlsx"},
					{"label": "budget-q2.xlsx"},
				},
			}},
			AttemptedToolCalls: []skillloop.SkillToolCallRef{
				{SkillID: skills.SkillFileReader, ToolName: "read_file", Arguments: map[string]interface{}{"file_id": "file-2"}},
			},
		})
		if blocked {
			t.Fatalf("skillLoopUserInputGuard blocked clarification with pre-resolved file target requirement: %#v", result)
		}
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

func TestSkillLoopFinalAnswerGuardSkipsCompletedNavigationObservationOnFilesPage(t *testing.T) {
	query := "\u8bf7\u6309\u987a\u5e8f\u8fde\u7eed\u5bfc\u822a\u5e76\u89c2\u5bdf\u9875\u9762\uff0c\u4e0d\u8981\u521b\u5efa\u6216\u5220\u9664\u4efb\u4f55\u8d44\u4ea7\uff1a\u5148\u5230\u9996\u9875\uff0c\u518d\u5230\u6587\u4ef6\u7ba1\u7406\uff0c\u518d\u5230\u667a\u80fd\u4f53\uff0c\u518d\u5230\u6570\u636e\u5e93\uff0c\u6700\u540e\u56de\u5230\u6587\u4ef6\u7ba1\u7406\u3002\u5b8c\u6210\u540e\u53ea\u603b\u7ed3\u6bcf\u4e00\u6b65\u662f\u5426\u6210\u529f\u3002"
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"intent":              "navigate_console_page",
					"status":              operationPlanStatusCompleted,
					"pending_next_action": "none",
					"task_contract": map[string]interface{}{
						"intent_label": "navigate_console_page",
					},
				},
			},
		},
		parts: consoleFilesSemanticTestParts(query, []consoleFilesTestFile{
			{ID: "file-1", Name: "SMOKE-BATCH-NO-GUARDRAIL-1782418200001.svg", Extension: "svg", MimeType: "image/svg+xml"},
			{ID: "file-2", Name: "notes.md", Extension: "md", MimeType: "text/markdown"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillConsoleNavigator, skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.Surface = aiChatSurfaceContextualSidebar

	if guard := skillLoopFinalAnswerGuard(prepared); guard != nil {
		result, blocked := guard(skillloop.FinalAnswerGuardRequest{
			Answer: "\u9996\u9875\u3001\u6587\u4ef6\u7ba1\u7406\u3001\u667a\u80fd\u4f53\u3001\u6570\u636e\u5e93\u3001\u6587\u4ef6\u7ba1\u7406\u90fd\u5df2\u6210\u529f\u5bfc\u822a\u5e76\u89c2\u5bdf\u3002",
		})
		if blocked {
			t.Fatalf("guard blocked pure navigation observation final answer: %#v", result)
		}
	}
}

func TestSkillLoopFinalAnswerGuardSkipsNavigationReadOfNonFileResourceOnFilesPage(t *testing.T) {
	query := "\u8bf7\u5f00\u59cb\u4e00\u4e2a\u5206\u9636\u6bb5\u4efb\u52a1\uff1a\u5148\u5207\u5230\u667a\u80fd\u4f53\u9875\u9762\uff0c\u8bfb\u53d6\u5f53\u524d\u5217\u8868\u91cc\u7684\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53\u540d\u79f0\uff1b\u7136\u540e\u56de\u5230\u6587\u4ef6\u7ba1\u7406\u9875\u9762\u3002\u7b2c\u4e00\u9636\u6bb5\u53ea\u505a\u5230\u8fd9\u91cc\uff0c\u4e0d\u8981\u521b\u5efa\u6587\u4ef6\u3001\u4e0d\u8981\u5220\u9664\u6587\u4ef6\u3002"
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"intent":              "navigate_console_page",
					"status":              operationPlanStatusCompleted,
					"pending_next_action": "none",
					"task_contract": map[string]interface{}{
						"intent_label": "navigate_console_page",
					},
				},
			},
		},
		parts: consoleFilesSemanticTestParts(query, []consoleFilesTestFile{
			{ID: "file-1", Name: "SMOKE-MANAGED-GUARD-FIX-1782396486164.svg", Extension: "svg", MimeType: "image/svg+xml"},
			{ID: "file-2", Name: "notes.md", Extension: "md", MimeType: "text/markdown"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillConsoleNavigator, skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.Surface = aiChatSurfaceContextualSidebar

	if guard := skillLoopFinalAnswerGuard(prepared); guard != nil {
		result, blocked := guard(skillloop.FinalAnswerGuardRequest{
			Answer: "\u5df2\u8bfb\u53d6\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53\u540d\u79f0\uff0c\u5e76\u5df2\u56de\u5230\u6587\u4ef6\u7ba1\u7406\u3002\u7b49\u5f85\u7ee7\u7eed\u3002",
		})
		if blocked {
			t.Fatalf("guard blocked navigation read of non-file resource as file read: %#v", result)
		}
	}
}

func TestSkillLoopFinalAnswerGuardDoesNotForceReadAfterNavigationObservation(t *testing.T) {
	query := "\u5bfc\u822a\u5230\u6587\u4ef6\u7ba1\u7406\u540e\u8bfb\u53d6\u7b2c\u4e00\u4e2a\u6587\u4ef6\u5185\u5bb9"
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"intent":              "navigate_console_page",
					"status":              operationPlanStatusCompleted,
					"pending_next_action": "none",
				},
			},
		},
		parts: consoleFilesSemanticTestParts(query, []consoleFilesTestFile{
			{ID: "file-1", Name: "invoice.txt", Extension: "txt", MimeType: "text/plain"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillConsoleNavigator, skills.SkillFileReader}
	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.Surface = aiChatSurfaceContextualSidebar
	prepared.parts.ModelTurnIntent = &AIChatModelTurnIntent{
		Intent:     "read_visible_file_content",
		Confidence: 0.91,
	}

	if guard := skillLoopFinalAnswerGuard(prepared); guard != nil {
		if result, blocked := guard(skillloop.FinalAnswerGuardRequest{
			Answer: "The model should decide whether the visible file needs read_file after navigation.",
		}); blocked {
			t.Fatalf("skillLoopFinalAnswerGuard blocked navigation+read with pre-resolved tool requirement: %#v", result)
		}
	}
}

func TestSkillLoopFinalAnswerGuardBlocksPartialMultiFileManagementSave(t *testing.T) {
	query := "\u7ee7\u7eed\u3002\u7b2c\u4e8c\u9636\u6bb5\uff1a\u8bf7\u5728\u6587\u4ef6\u7ba1\u7406\u4e2d\u521b\u5efa\u5e76\u4fdd\u5b58\u4e24\u4e2a\u6587\u4ef6\u5230\u6587\u4ef6\u7ba1\u7406\uff1a1\uff09SMOKE-COMPLEX-GUARD-FIX2-1782397301095.txt\uff1b2\uff09SMOKE-COMPLEX-GUARD-FIX2-1782397301095.svg\u3002\u8981\u6c42\u6bcf\u4e2a\u6587\u4ef6\u53ea\u751f\u6210\u4e00\u6b21\u3001\u53ea\u4fdd\u5b58\u4e00\u6b21\u3002"
	prepared := &PreparedChat{
		parts: consoleFilesSemanticTestParts(query, []consoleFilesTestFile{
			{ID: "file-txt", Name: "SMOKE-COMPLEX-GUARD-FIX2-1782397301095.txt", Extension: "txt", MimeType: "text/plain"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileGenerator, skills.SkillFileManager}
	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.Surface = aiChatSurfaceContextualSidebar
	prepared.parts.ModelTurnIntent = &AIChatModelTurnIntent{
		Intent:     "save_generated_file_to_file_management",
		TargetPage: "/console/files",
		Confidence: 0.91,
	}

	guard := skillLoopFinalAnswerGuard(prepared)
	if guard == nil {
		t.Fatal("skillLoopFinalAnswerGuard() = nil, want multi-file save guard")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "TXT \u5df2\u4fdd\u5b58\uff0cSVG \u672c\u56de\u5408\u6ca1\u6709\u53ef\u8bc1\u660e\u7684\u6210\u529f\u4fdd\u5b58\u7ed3\u679c\u3002\u7b49\u5f85\u7ee7\u7eed\u3002",
		SuccessfulToolCalls: []skillloop.SkillToolCallRef{{
			SkillID:  skills.SkillFileManager,
			ToolName: "save_file_to_management",
			Arguments: map[string]interface{}{
				"filename":     "SMOKE-COMPLEX-GUARD-FIX2-1782397301095.txt",
				"source_type":  "tool_file",
				"tool_file_id": "tool-txt",
			},
			Result: map[string]interface{}{
				"file_name": "SMOKE-COMPLEX-GUARD-FIX2-1782397301095.txt",
			},
		}},
	})
	if !blocked {
		t.Fatal("guard allowed final answer after only one of two requested files was saved")
	}
	for _, want := range []string{"smoke-complex-guard-fix2-1782397301095.svg", "multiple files", "save_file_to_management"} {
		if !strings.Contains(result.Message, want) && !strings.Contains(result.SystemMessage, want) {
			t.Fatalf("guard result missing %q: %#v", want, result)
		}
	}
}

func preparedConsoleFilesGuardReadTest(query string) *PreparedChat {
	parts := consoleFilesSemanticTestParts(query, []consoleFilesTestFile{
		{ID: "file-1", Name: "notes.txt", Extension: "txt", MimeType: "text/plain"},
		{ID: "file-2", Name: "budget-q1.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{ID: "file-3", Name: "invoice.pdf", Extension: "pdf", MimeType: "application/pdf"},
		{ID: "file-4", Name: "budget-q2.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	})
	parts.ModelTurnIntent = &AIChatModelTurnIntent{
		Intent:     "read_visible_file_content",
		Confidence: 0.91,
	}
	return &PreparedChat{
		parts: parts,
	}
}

func TestSkillLoopFinalAnswerGuardDoesNotForceConsoleFilesDeleteToolCall(t *testing.T) {
	prepared := &PreparedChat{
		parts: consoleFilesSnapshotTestParts("delete the first file", []consoleFilesTestFile{
			{ID: "file-1", Name: "invoice.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		}),
	}
	prepared.parts.SkillIDs = []string{skills.SkillFileManager}
	prepared.parts.ModelTurnIntent = &AIChatModelTurnIntent{
		Intent:     "delete_visible_file",
		Confidence: 0.91,
	}

	if guard := skillLoopFinalAnswerGuard(prepared); guard != nil {
		if result, blocked := guard(skillloop.FinalAnswerGuardRequest{
			Answer: "The model should decide whether delete_file must be called from the visible file context.",
		}); blocked {
			t.Fatalf("skillLoopFinalAnswerGuard blocked deletion with pre-resolved tool requirement: %#v", result)
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
