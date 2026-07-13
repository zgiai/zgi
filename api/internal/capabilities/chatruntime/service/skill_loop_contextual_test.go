package service

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
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
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "answer_or_explain_console_context"},
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
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "answer_or_explain_console_context"},
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
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "answer_or_explain_console_context"},
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
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "answer_or_explain_console_context"},
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
		ModelTurnIntent: &AIChatModelTurnIntent{Intent: "answer_or_explain_console_context"},
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
	if strategy.Intent != "answer_or_explain_console_context" || strategy.RouteRequired {
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
	if strategy.Intent != "model_decides" || strategy.RouteRequired {
		t.Fatalf("strategy = %#v, want default model-decides strategy without forced route", strategy)
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
		"best matches the requested output",
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
		"Current assistant turn task contract",
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

func TestTemporaryFileGenerateIntentIgnoresReadOnlyNegativeOperations(t *testing.T) {
	query := "SMOKE-ORDER: \u53ea\u56de\u7b54\u5f53\u524d\u6587\u4ef6\u7ba1\u7406\u9875\u53ef\u89c1\u6587\u4ef6\u603b\u6570\u548c\u524d\u4e24\u4e2a\u6587\u4ef6\u540d\uff0c\u4e0d\u8981\u521b\u5efa\u3001\u4fdd\u5b58\u3001\u5220\u9664\u6216\u5bfc\u822a\u3002"
	if isTemporaryFileGenerateIntent(query) {
		t.Fatal("isTemporaryFileGenerateIntent() = true, want false for read-only request with negative operations")
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
		Intent:        "answer_or_explain_console_context",
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
	if strategy.Intent != "answer_or_explain_console_context" {
		t.Fatalf("Intent = %q, want answer_or_explain_console_context; strategy=%#v", strategy.Intent, strategy)
	}
	if strategy.RouteRequired {
		t.Fatalf("RouteRequired = true, want false for already visible files page")
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

	if !prepared.toolLoopEnabled() {
		t.Fatal("toolLoopEnabled() = false, want main tool loop path")
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
	if strategy.Intent != "model_decides" {
		t.Fatalf("Intent = %q, want safe model-decides strategy; strategy=%#v", strategy.Intent, strategy)
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

func TestContextualAIChatTurnStrategyUsesSkillLoopWhenClassifierFailsOnAgentPage(t *testing.T) {
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
	if strategy.Intent != "model_decides" {
		t.Fatalf("Intent = %q, want model_decides; strategy=%#v", strategy.Intent, strategy)
	}
	if strategy.Source != aiChatTurnStrategySourceDefault {
		t.Fatalf("Source = %q, want %q", strategy.Source, aiChatTurnStrategySourceDefault)
	}
	if slices.Contains(strategy.PrimarySkills, skills.SkillAgentManagement) {
		t.Fatalf("PrimarySkills = %#v, want no agent-management primary skill for passive answer", strategy.PrimarySkills)
	}
	if !prepared.toolLoopEnabled() {
		t.Fatal("toolLoopEnabled() = false, want main tool loop path")
	}
}

func TestParseModelTurnIntentContentAcceptsLooseClassifierJSON(t *testing.T) {
	intent, err := parseModelTurnIntentContent("```json\n{\"intent\":\"answer\",\"confidence\":\"0.91\",\"approval\":false,\"route_required\":\"true\"}\n```")
	if err != nil {
		t.Fatalf("parseModelTurnIntentContent() error = %v", err)
	}
	if got := normalizeModelTurnIntent(intent.Intent); got != "answer_or_explain_console_context" {
		t.Fatalf("Intent = %q, want answer_or_explain_console_context", got)
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

func TestNormalizeModelTurnIntentAcceptsLegacyBrandedAnswerIntent(t *testing.T) {
	if got := normalizeModelTurnIntent("answer_or_explain_zgi_context"); got != "answer_or_explain_console_context" {
		t.Fatalf("normalizeModelTurnIntent() = %q, want answer_or_explain_console_context", got)
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

func TestParseModelTurnIntentContentNormalizesObjectPhases(t *testing.T) {
	intent, err := parseModelTurnIntentContent(`{
		"intent":"manage_agent_asset",
		"phases":[
			{"id":"phase-1","action":"read the source file"},
			{"step":"update the Agent configuration"},
			{"unexpected":"ignored"}
		],
		"confidence":0.9
	}`)
	if err != nil {
		t.Fatalf("parseModelTurnIntentContent() error = %v", err)
	}
	want := []string{"read the source file", "update the Agent configuration"}
	if !slices.Equal(intent.Phases, want) {
		t.Fatalf("Phases = %#v, want %#v", intent.Phases, want)
	}
	for _, phase := range intent.Phases {
		if strings.HasPrefix(phase, "map[") {
			t.Fatalf("phase leaked Go map formatting: %q", phase)
		}
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
	if intent.Intent != "answer_or_explain_console_context" {
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

func TestParseModelTurnIntentMessageIgnoresReasoningJSONWhenContentEmpty(t *testing.T) {
	_, source, err := parseModelTurnIntentMessage(adapter.Message{
		ReasoningContent: `We need classify this request.
{"intent":"answer_or_explain_console_context","task_type":"agent_prompt_review","confidence":0.91,"approval":"none"}`,
	})
	if err == nil {
		t.Fatal("parseModelTurnIntentMessage() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "empty classifier content") {
		t.Fatalf("error = %q, want empty classifier content", err.Error())
	}
	if source != "" {
		t.Fatalf("source = %q, want empty source because reasoning content is ignored", source)
	}
}

func TestParseModelTurnIntentMessageRejectsReasoningOnlyProse(t *testing.T) {
	_, source, err := parseModelTurnIntentMessage(adapter.Message{
		ReasoningContent: "We need to classify this as an agent request, but I will not emit JSON.",
	})
	if err == nil {
		t.Fatal("parseModelTurnIntentMessage() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "empty classifier content") {
		t.Fatalf("error = %q, want empty classifier content", err.Error())
	}
	if source != "" {
		t.Fatalf("source = %q, want empty source because reasoning content is ignored", source)
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
		"intent": "answer_or_explain_console_context",
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
	if !prepared.toolLoopEnabled() {
		t.Fatal("toolLoopEnabled() = false, want main tool loop path")
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

func TestContextualAIChatTurnStrategyDoesNotUseLegacyPassiveFastPath(t *testing.T) {
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:          "what can you do here?",
			Surface:        aiChatSurfaceContextualSidebar,
			RuntimeContext: "route=/console/agents/agent-1/agent",
			SkillIDs:       []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement},
			SkillMode:      skillModeAuto,
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:      "answer_or_explain_console_context",
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
	if !prepared.toolLoopEnabled() {
		t.Fatal("toolLoopEnabled() = false, want main tool loop path")
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
			Intent:     "answer_or_explain_console_context",
			Confidence: 0.88,
			Reason:     "passive context question",
		},
	}

	metadata := streamingMessageMetadataWithTaskID(parts, "task-1")
	raw, ok := metadata["model_turn_intent"].(*AIChatModelTurnIntent)
	if !ok || raw.Intent != "answer_or_explain_console_context" {
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
		"Console navigation guidance",
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

func TestArtifactProducerSkillsExposeEnabledProducersWithModelHintOrder(t *testing.T) {
	chartParts := &chatRequestParts{
		Query:    "generate an SVG report",
		SkillIDs: []string{skills.SkillFileGenerator, skills.SkillChartGenerator},
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent:                  "generate_temporary_file_artifact",
			RecommendedCapabilities: []string{"chart_artifact"},
		},
	}
	if got, want := appendArtifactProducerSkills(nil, chartParts), []string{skills.SkillChartGenerator, skills.SkillFileGenerator}; !reflect.DeepEqual(got, want) {
		t.Fatalf("appendArtifactProducerSkills(chart hint) = %#v, want %#v", got, want)
	}

	fileParts := &chatRequestParts{
		Query:    "generate a pie chart SVG",
		SkillIDs: []string{skills.SkillFileGenerator, skills.SkillChartGenerator},
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent:                  "generate_temporary_file_artifact",
			RecommendedCapabilities: []string{"file_artifact"},
		},
	}
	if got, want := appendArtifactProducerSkills(nil, fileParts), []string{skills.SkillFileGenerator, skills.SkillChartGenerator}; !reflect.DeepEqual(got, want) {
		t.Fatalf("appendArtifactProducerSkills(file hint) = %#v, want %#v", got, want)
	}

	unclassifiedParts := &chatRequestParts{
		Query:    "generate a pie chart SVG",
		SkillIDs: []string{skills.SkillFileGenerator, skills.SkillChartGenerator},
	}
	if got, want := appendArtifactProducerSkills(nil, unclassifiedParts), []string{skills.SkillFileGenerator, skills.SkillChartGenerator}; !reflect.DeepEqual(got, want) {
		t.Fatalf("appendArtifactProducerSkills(no model hint) = %#v, want %#v", got, want)
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

func successfulFileManagerSaveInvocation(toolFileID string, filename string) map[string]interface{} {
	return map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillFileManager,
		"tool_name": "save_file_to_management",
		"arguments": map[string]interface{}{
			"source_type":  "tool_file",
			"tool_file_id": toolFileID,
			"filename":     filename,
		},
		"result": map[string]interface{}{
			"status":              "completed",
			"filename":            filename,
			"source_tool_file_id": toolFileID,
			"target":              "managed_file",
		},
	}
}
