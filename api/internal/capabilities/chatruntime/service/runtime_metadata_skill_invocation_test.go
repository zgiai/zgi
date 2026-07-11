package service

import (
	"encoding/json"
	"strings"
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestMergeSkillTraceMetadataStoresTurnStateOutsideVisibleInvocations(t *testing.T) {
	metadata := mergeSkillTraceMetadata(nil, []skills.SkillTrace{{
		Kind:   "turn_state",
		Status: "success",
		Result: map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{
					"kind":       "working_fact",
					"visibility": "model_only",
					"key":        "agent_theme",
					"value":      "water fee confirmation",
					"source":     "file-reader/read_file",
				},
			},
		},
	}})

	if invocations := skillInvocationsFromMetadata(metadata["skill_invocations"]); len(invocations) != 0 {
		t.Fatalf("skill_invocations = %#v, want turn_state hidden from visible timeline", invocations)
	}
	state := mapFromOperationContext(metadata["turn_state"])
	items := mapSliceFromAny(state["items"])
	if len(items) != 1 {
		t.Fatalf("turn_state.items = %#v, want one item", state["items"])
	}
	if got := stringFromAny(items[0]["key"]); got != "agent_theme" {
		t.Fatalf("turn_state key = %q, want agent_theme", got)
	}
	if got := stringFromAny(items[0]["value"]); got != "water fee confirmation" {
		t.Fatalf("turn_state value = %q, want water fee confirmation", got)
	}
}

func TestMergeSkillTraceMetadataAllowsWorkingFactToUpdateUserVisibleSummary(t *testing.T) {
	metadata := mergeSkillTraceMetadata(nil, []skills.SkillTrace{{
		Kind:   "turn_state",
		Status: "success",
		Result: map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{
					"kind":       "user_deliverable",
					"visibility": "user_visible",
					"key":        "worldview_summary",
					"title":      "灵澜学院世界观总结",
					"content":    "灵澜学院是一所湖畔寄宿制高中，现实与非现实边界模糊。",
					"source":     "file-reader/read_file",
				},
			},
		},
	}})
	metadata = mergeSkillTraceMetadata(metadata, []skills.SkillTrace{{
		Kind:   "turn_state",
		Status: "success",
		Result: map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{
					"kind":       "working_fact",
					"visibility": "model_only",
					"key":        "worldview_summary",
					"value":      "星渊纪元是一个太空歌剧世界观。",
					"source":     "file-reader/read_file",
					"used_for":   []interface{}{"agent.system_prompt"},
				},
			},
		},
	}})

	state := mapFromOperationContext(metadata["turn_state"])
	items := mapSliceFromAny(state["items"])
	if len(items) != 2 {
		t.Fatalf("turn_state.items = %#v, want visible summary plus later working fact", items)
	}
	var visibleSummary map[string]interface{}
	var workingFact map[string]interface{}
	for _, item := range items {
		switch stringFromAny(item["kind"]) {
		case "user_deliverable":
			visibleSummary = item
		case "working_fact":
			workingFact = item
		}
	}
	if visibleSummary == nil || !strings.Contains(stringFromAny(visibleSummary["content"]), "灵澜学院") {
		t.Fatalf("visible summary = %#v, want original user-visible summary retained", visibleSummary)
	}
	if workingFact == nil || !strings.Contains(stringFromAny(workingFact["value"]), "星渊纪元") {
		t.Fatalf("working fact = %#v, want later model-updated fact retained", workingFact)
	}
	usedFor := mapSliceOrStringListForPrompt(workingFact["used_for"], 8, 120)
	if len(usedFor) != 1 || stringFromAny(usedFor[0]) != "agent.system_prompt" {
		t.Fatalf("used_for = %#v, want working fact usage preserved", workingFact["used_for"])
	}
}

func TestTurnStateContinuationSummaryIncludesUserVisibleDeliverable(t *testing.T) {
	message := &runtimemodel.Message{
		Metadata: map[string]interface{}{
			"turn_state": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"kind":       "user_deliverable",
						"visibility": "user_visible",
						"key":        "worldview_summary",
						"content":    "灵澜学院是一所湖畔寄宿制高中。",
						"source":     "file-reader/read_file",
					},
				},
			},
		},
	}

	summary := turnStateContinuationSummary(message)
	items := mapSliceFromAny(summary["items"])
	if len(items) != 1 {
		t.Fatalf("continuation turn_state items = %#v, want user-visible deliverable included", summary["items"])
	}
	if got := stringFromAny(items[0]["content"]); !strings.Contains(got, "灵澜学院") {
		t.Fatalf("continuation content = %q, want recorded user-visible summary", got)
	}
}

func TestMergeSkillInvocationMetadataBuildsStructuredTurnState(t *testing.T) {
	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{{
		"kind":       "tool_call",
		"skill_id":   skills.SkillAgentManagement,
		"tool_name":  "create_agent",
		"status":     "success",
		"runtime_id": "tool-call:agent-management:create_agent:test",
		"arguments": map[string]interface{}{
			"name": "Story Agent",
		},
		"result": map[string]interface{}{
			"agent_id":   "agent-1",
			"agent_name": "Story Agent",
			"status":     "completed",
		},
	}})

	state := mapFromOperationContext(metadata["turn_state"])
	if len(state) == 0 {
		t.Fatal("turn_state is empty, want structured state")
	}
	if steps := mapSliceFromAny(state["steps"]); len(steps) != 1 {
		t.Fatalf("turn_state.steps = %#v, want one step", state["steps"])
	}
	results := mapSliceFromAny(state["tool_results"])
	if len(results) != 1 {
		t.Fatalf("turn_state.tool_results = %#v, want one tool result", state["tool_results"])
	}
	target := mapFromOperationContext(results[0]["target"])
	if got := stringFromAny(target["name"]); got != "Story Agent" {
		t.Fatalf("tool result target name = %q, want Story Agent; result=%#v", got, results[0])
	}
	if assets := mapSliceFromAny(state["assets"]); len(assets) != 1 {
		t.Fatalf("turn_state.assets = %#v, want created asset", state["assets"])
	}
}

func TestTurnStateContinuationSummaryIncludesStructuredExecutionLedger(t *testing.T) {
	message := &runtimemodel.Message{
		Metadata: map[string]interface{}{
			"turn_state": map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{
						"kind":      "tool_call",
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "create_agent",
						"status":    "success",
						"target": map[string]interface{}{
							"asset_type": "agent",
							"name":       "Story Agent",
						},
					},
				},
				"tool_results": []interface{}{
					map[string]interface{}{
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "create_agent",
						"status":    "success",
						"target": map[string]interface{}{
							"asset_type": "agent",
							"name":       "Story Agent",
						},
					},
				},
			},
		},
	}

	summary := turnStateContinuationSummary(message)
	if len(summary) == 0 {
		t.Fatal("turnStateContinuationSummary() = nil, want structured summary")
	}
	if got := len(mapSliceFromAny(summary["steps"])); got != 1 {
		t.Fatalf("summary.steps len = %d, want 1; summary=%#v", got, summary)
	}
	if got := len(mapSliceFromAny(summary["tool_results"])); got != 1 {
		t.Fatalf("summary.tool_results len = %d, want 1; summary=%#v", got, summary)
	}
}

func TestCurrentTurnAuthoritativeStateMessageRequiresRuntimeState(t *testing.T) {
	message := &runtimemodel.Message{Query: "create an agent"}

	if got := currentTurnAuthoritativeStateMessage(message); got != nil {
		t.Fatalf("currentTurnAuthoritativeStateMessage() = %#v, want nil without runtime state", got)
	}
}

func TestCurrentTurnAuthoritativeStateMessageIncludesSameTurnFacts(t *testing.T) {
	message := &runtimemodel.Message{
		Query: "读取文件并用总结创建智能体",
		Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"intent":              "manage_agent_asset",
				"status":              "running",
				"pending_next_action": "configure created agent",
				"success_criteria": []interface{}{
					"read the file summary",
					"create and configure the agent",
				},
			},
			"operation_result_summary": map[string]interface{}{
				"operation":     "agent.create",
				"success_count": 1,
				"target":        "雪",
			},
			"turn_state": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"kind":       "user_deliverable",
						"visibility": "user_visible",
						"key":        "worldview_summary",
						"content":    "雪是灵潮学院世界观中的核心角色。",
						"source":     "file-reader/read_file",
						"used_for":   []interface{}{"agent.system_prompt"},
					},
				},
			},
		},
	}

	stateMessage := currentTurnAuthoritativeStateMessage(message)
	if stateMessage == nil {
		t.Fatal("currentTurnAuthoritativeStateMessage() = nil, want message")
	}
	content := messageContentText(stateMessage.Content)
	for _, want := range []string{
		"Current AIChat turn authoritative state JSON",
		"Continue only unfinished work",
		"manage_agent_asset",
		"configure created agent",
		"agent.create",
		"worldview_summary",
		"雪是灵潮学院世界观中的核心角色",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("current turn state message missing %q in:\n%s", want, content)
		}
	}
}

func TestCurrentTurnAuthoritativeStateMessageIncludesGeneratedArtifacts(t *testing.T) {
	message := &runtimemodel.Message{
		Query: "draw a pie chart",
		Metadata: map[string]interface{}{
			"generated_files": []interface{}{
				map[string]interface{}{
					"artifact_id":     "artifact-1",
					"artifact_type":   "file",
					"target":          "temporary_artifact",
					"transfer_method": "tool_file",
					"tool_file_id":    "tool-file-1",
					"filename":        "agents-distribution-pie.svg",
					"skill_id":        "chart-generator",
					"tool_name":       "generate_chart",
				},
			},
		},
	}

	stateMessage := currentTurnAuthoritativeStateMessage(message)
	if stateMessage == nil {
		t.Fatal("currentTurnAuthoritativeStateMessage() = nil, want generated artifact state")
	}
	content := messageContentText(stateMessage.Content)
	for _, want := range []string{
		"generated_artifacts",
		"generated_artifact_count",
		"agents-distribution-pie.svg",
		"temporary_artifact",
		"tool-file-1",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("current turn state message missing %q in:\n%s", want, content)
		}
	}
}

func TestRecentTurnStateSectionIncludesUserVisibleDeliverable(t *testing.T) {
	branch := []*runtimemodel.Message{{
		Status: runtimemodel.MessageStatusCompleted,
		Query:  "读取文件并基于总结创建智能体",
		Metadata: map[string]interface{}{
			"turn_state": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"kind":       "user_deliverable",
						"visibility": "user_visible",
						"key":        "worldview_summary",
						"content":    "灵澜学院是一所湖畔寄宿制高中。",
						"source":     "file-reader/read_file",
					},
				},
			},
		},
	}}

	section, stats := recentTurnStateSection(branch, 2000)
	if stats.IncludedTurnStateFacts != 1 {
		t.Fatalf("included turn_state facts = %d, want 1; section=%s", stats.IncludedTurnStateFacts, section)
	}
	if !strings.Contains(section, "灵澜学院") {
		t.Fatalf("turn_state section = %q, want user-visible summary content", section)
	}
}

func TestMergeSkillTraceMetadataRedactsFileReaderResultContent(t *testing.T) {
	const rawContent = "SKILL_TRACE_SECRET_SHOULD_NOT_PERSIST"

	metadata := mergeSkillTraceMetadata(nil, []skills.SkillTrace{{
		Kind:     "tool_call",
		SkillID:  skills.SkillFileReader,
		ToolName: "read_file",
		Status:   "success",
		Result: map[string]interface{}{
			"status":            "completed",
			"content":           rawContent,
			"content_chars":     500,
			"content_truncated": true,
			"content_status":    "extracted",
			"file": map[string]interface{}{
				"id":           "file-1",
				"name":         "invoice.xlsx",
				"workspace_id": "workspace-1",
				"created_by":   "account-1",
			},
		},
	}})
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if strings.Contains(string(encoded), rawContent) {
		t.Fatalf("skill invocation metadata leaked raw file content: %s", string(encoded))
	}

	invocations, ok := metadata["skill_invocations"].([]interface{})
	if !ok || len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want one invocation", metadata["skill_invocations"])
	}
	invocation, _ := invocations[0].(map[string]interface{})
	result, ok := invocation["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("result = %#v, want map", invocation["result"])
	}
	if result["content_redacted"] != true || result["content_chars"] != 500 || result["content_returned_chars"] != len([]rune(rawContent)) {
		t.Fatalf("result content summary = %#v, want redaction with original and returned char counts", result)
	}
	if _, ok := result["content"]; ok {
		t.Fatalf("content should not be persisted in skill invocation result: %#v", result)
	}
	file, ok := result["file"].(map[string]interface{})
	if !ok {
		t.Fatalf("file = %#v, want safe file summary", result["file"])
	}
	if file["id"] != "file-1" || file["name"] != "invoice.xlsx" || file["workspace_id"] != "workspace-1" {
		t.Fatalf("file summary = %#v, want safe file metadata", file)
	}
	if _, ok := file["created_by"]; ok {
		t.Fatalf("created_by should not be persisted in skill invocation file summary: %#v", file)
	}
}

func TestMergeSkillTraceMetadataKeepsShortPlainTextReadValuePreview(t *testing.T) {
	const rawContent = "\u6d4b\u8bd5\u4ee3\u7801111"

	metadata := mergeSkillTraceMetadata(nil, []skills.SkillTrace{{
		Kind:     "tool_call",
		SkillID:  skills.SkillFileReader,
		ToolName: "read_file",
		Status:   "success",
		Result: map[string]interface{}{
			"status":            "completed",
			"content":           rawContent,
			"content_chars":     len([]rune(rawContent)),
			"content_status":    "extracted",
			"content_truncated": false,
			"file_id":           "file-1",
			"file_name":         "name-source.txt",
			"file_extension":    "txt",
			"file_mime_type":    "text/plain",
		},
	}})

	invocations, ok := metadata["skill_invocations"].([]interface{})
	if !ok || len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want one invocation", metadata["skill_invocations"])
	}
	invocation, _ := invocations[0].(map[string]interface{})
	result, ok := invocation["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("result = %#v, want map", invocation["result"])
	}
	if _, ok := result["content"]; ok {
		t.Fatalf("content should not be persisted in skill invocation result: %#v", result)
	}
	if got := result["content_value_preview"]; got != rawContent {
		t.Fatalf("content_value_preview = %#v, want %q; result=%#v", got, rawContent, result)
	}
	if got := result["content_value_source"]; got != "read_file.content" {
		t.Fatalf("content_value_source = %#v, want read_file.content; result=%#v", got, result)
	}
}

func TestMergeSkillInvocationMetadataRedactsFileReaderFilePreviews(t *testing.T) {
	const rawPreview = "SKILL_EVENT_PREVIEW_SHOULD_NOT_PERSIST"

	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{{
		"kind":      "tool_call",
		"skill_id":  skills.SkillFileReader,
		"tool_name": "list_visible_files",
		"status":    "success",
		"result": map[string]interface{}{
			"status":         "completed",
			"count":          1,
			"selected_count": 1,
			"files": []map[string]interface{}{{
				"visible_index":   1,
				"file_id":         "file-1",
				"name":            "notes.pdf",
				"workspace_id":    "workspace-1",
				"content_preview": rawPreview,
				"content_chars":   300,
			}},
		},
	}})
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if strings.Contains(string(encoded), rawPreview) {
		t.Fatalf("skill invocation metadata leaked raw file preview: %s", string(encoded))
	}

	invocations := metadata["skill_invocations"].([]interface{})
	invocation := invocations[0].(map[string]interface{})
	result := invocation["result"].(map[string]interface{})
	files, ok := result["files"].([]map[string]interface{})
	if !ok || len(files) != 1 {
		t.Fatalf("files = %#v, want one summarized file", result["files"])
	}
	if files[0]["content_preview_redacted"] != true || files[0]["content_preview_chars"] != len([]rune(rawPreview)) {
		t.Fatalf("file preview summary = %#v, want redaction markers", files[0])
	}
	if files[0]["content_chars"] != 300 || files[0]["file_id"] != "file-1" {
		t.Fatalf("file audit fields = %#v, want content_chars and file_id preserved", files[0])
	}
	if result["files_content_redacted"] != true {
		t.Fatalf("files_content_redacted = %#v, want true", result["files_content_redacted"])
	}
}

func TestMergeSkillInvocationMetadataKeepsNonFileResultContent(t *testing.T) {
	const calculatorContent = "not sensitive calculator explanation"
	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{{
		"kind":      "tool_call",
		"skill_id":  skills.SkillCalculator,
		"tool_name": "calculate",
		"status":    "success",
		"result": map[string]interface{}{
			"content": calculatorContent,
		},
	}})

	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if !strings.Contains(string(encoded), calculatorContent) {
		t.Fatalf("non-file skill result content was unexpectedly redacted: %s", string(encoded))
	}
}

func TestMergeSkillInvocationMetadataSortsTimelineByCreatedAt(t *testing.T) {
	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{
		{
			"kind":       "tool_call",
			"skill_id":   "file-manager",
			"tool_name":  "save_file_to_management",
			"status":     "success",
			"created_at": 300,
			"runtime_id": "save-late",
		},
		{
			"kind":       "client_action",
			"skill_id":   "console-navigator",
			"tool_name":  "navigate",
			"status":     "succeeded",
			"created_at": 100,
			"runtime_id": "route-early",
		},
		{
			"kind":       "reference_read",
			"skill_id":   "file-generator",
			"path":       "format-svg.md",
			"status":     "success",
			"created_at": "200",
			"runtime_id": "read-middle",
		},
	})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	if len(invocations) != 3 {
		t.Fatalf("skill_invocations len = %d in %#v, want 3", len(invocations), metadata["skill_invocations"])
	}
	got := []string{
		stringFromAny(invocations[0]["runtime_id"]),
		stringFromAny(invocations[1]["runtime_id"]),
		stringFromAny(invocations[2]["runtime_id"]),
	}
	want := []string{"route-early", "read-middle", "save-late"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("runtime_id order = %#v, want %#v", got, want)
		}
	}
}

func TestMergeSkillInvocationMetadataKeepsStableOrderForMissingCreatedAt(t *testing.T) {
	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{
		{
			"kind":       "tool_call",
			"skill_id":   "file-generator",
			"tool_name":  "generate_file",
			"status":     "success",
			"created_at": 100,
			"runtime_id": "dated",
		},
		{
			"kind":       "guardrail",
			"skill_id":   "file-manager",
			"tool_name":  "save_file_to_management",
			"status":     "blocked",
			"runtime_id": "missing-one",
		},
		{
			"kind":       "guardrail",
			"skill_id":   "file-generator",
			"tool_name":  "generate_file",
			"status":     "blocked",
			"runtime_id": "missing-two",
		},
	})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	got := []string{
		stringFromAny(invocations[0]["runtime_id"]),
		stringFromAny(invocations[1]["runtime_id"]),
		stringFromAny(invocations[2]["runtime_id"]),
	}
	want := []string{"dated", "missing-one", "missing-two"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("runtime_id order = %#v, want %#v", got, want)
		}
	}
}

func TestMergeSkillInvocationMetadataOmitsInternalPlannerGuardrail(t *testing.T) {
	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{
		{
			"kind":      "guardrail",
			"skill_id":  skills.SkillFileGenerator,
			"tool_name": "generate_file",
			"status":    "blocked",
			"error":     "use the existing temporary artifact",
			"arguments": map[string]interface{}{
				"next_step": "continue_planning",
			},
			"runtime_id": "internal-feedback",
		},
		{
			"kind":       "guardrail",
			"skill_id":   skills.SkillFileManager,
			"tool_name":  "save_file_to_management",
			"status":     "blocked",
			"error":      "visible governance guardrail",
			"runtime_id": "visible-guardrail",
		},
	})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	if len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want only the visible guardrail", invocations)
	}
	if got := stringFromAny(invocations[0]["runtime_id"]); got != "visible-guardrail" {
		t.Fatalf("runtime_id = %q, want visible-guardrail", got)
	}
	if metadata["guardrail_count"] != 1 {
		t.Fatalf("guardrail_count = %#v, want 1", metadata["guardrail_count"])
	}
}

func TestMergeSkillInvocationMetadataDedupesAgentBatchOperationBySemanticIdentity(t *testing.T) {
	result := map[string]interface{}{
		"status":         "completed",
		"effect":         "deleted",
		"operation_type": "agent.delete.batch",
		"target_count":   2,
		"deleted_count":  2,
		"item_results": []interface{}{
			map[string]interface{}{"agent_id": "agent-1", "agent_name": "Novel Writer", "status": "succeeded"},
			map[string]interface{}{"agent_id": "agent-2", "agent_name": "Browser Agent", "status": "succeeded"},
		},
		"operation_group": map[string]interface{}{
			"operation":     "agent.delete",
			"asset_type":    "agent",
			"target_count":  2,
			"success_count": 2,
			"item_results": []interface{}{
				map[string]interface{}{"agent_id": "agent-1", "agent_name": "Novel Writer", "status": "succeeded"},
				map[string]interface{}{"agent_id": "agent-2", "agent_name": "Browser Agent", "status": "succeeded"},
			},
		},
	}

	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{
		{
			"kind":       "tool_call",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "delete_agent",
			"status":     "success",
			"runtime_id": "tool_call:agent-management:delete_agent::#1",
			"result":     result,
		},
	})
	metadata = mergeSkillInvocationMetadata(metadata, []map[string]interface{}{
		{
			"kind":        "tool_call",
			"skill_id":    skills.SkillAgentManagement,
			"tool_name":   "delete_agent",
			"status":      "success",
			"runtime_id":  "trace:000000:tool_call:agent-management:delete_agent::",
			"duration_ms": int64(17),
			"result":      result,
		},
	})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	if len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want one deduped batch delete invocation", invocations)
	}
	if got := stringFromAny(invocations[0]["runtime_id"]); got != "trace:000000:tool_call:agent-management:delete_agent::" {
		t.Fatalf("runtime_id = %q, want merged incoming runtime id", got)
	}
}

func TestMergeSkillInvocationMetadataDedupesAgentConfigOperationBySemanticIdentity(t *testing.T) {
	first := map[string]interface{}{
		"status":            "completed",
		"effect":            "updated",
		"agent_id":          "agent-1",
		"agent_name":        "Novel Writer",
		"updated_fields":    []interface{}{"enabled_skill_ids", "model", "file_upload_enabled"},
		"enabled_skill_ids": []interface{}{"file-generator"},
		"model_provider":    "deepseek",
		"model":             "deepseek-v4-flash",
	}
	second := copyStringAnyMap(first)
	second["file_upload_enabled"] = true

	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{
		{
			"kind":       "tool_call",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "update_agent_config",
			"status":     "success",
			"runtime_id": "tool_call:agent-management:update_agent_config::#1",
			"result":     first,
		},
	})
	metadata = mergeSkillInvocationMetadata(metadata, []map[string]interface{}{
		{
			"kind":       "tool_call",
			"skill_id":   skills.SkillAgentManagement,
			"tool_name":  "update_agent_config",
			"status":     "success",
			"runtime_id": "trace:000000:tool_call:agent-management:update_agent_config::",
			"result":     second,
		},
	})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	if len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want one deduped update invocation", invocations)
	}
	result := mapFromOperationContext(invocations[0]["result"])
	if result["file_upload_enabled"] != true {
		t.Fatalf("result = %#v, want merged config result", result)
	}
}

func TestMergeSkillTraceMetadataOmitsInternalPlannerGuardrail(t *testing.T) {
	metadata := mergeSkillTraceMetadata(nil, []skills.SkillTrace{{
		Kind:     "guardrail",
		SkillID:  skills.SkillFileGenerator,
		ToolName: "generate_file",
		Status:   "blocked",
		Error:    "use the existing temporary artifact",
		Arguments: map[string]interface{}{
			"next_step": "continue_planning",
		},
	}})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	if len(invocations) != 0 {
		t.Fatalf("skill_invocations = %#v, want no user-visible planner feedback guardrail", invocations)
	}
	if metadata["guardrail_count"] != 0 {
		t.Fatalf("guardrail_count = %#v, want 0", metadata["guardrail_count"])
	}
}

func TestMergeSkillTraceMetadataOmitsPlannerFeedback(t *testing.T) {
	metadata := mergeSkillTraceMetadata(nil, []skills.SkillTrace{{
		Kind:     "planner_feedback",
		SkillID:  skills.SkillFileReader,
		ToolName: "read_file",
		Status:   "advisory",
		Error:    "skill must be loaded before calling its tools",
		Arguments: map[string]interface{}{
			"next_step": "load_skill",
		},
	}})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	if len(invocations) != 0 {
		t.Fatalf("skill_invocations = %#v, want no user-visible planner feedback", invocations)
	}
	if metadata["guardrail_count"] != 0 {
		t.Fatalf("guardrail_count = %#v, want 0", metadata["guardrail_count"])
	}
}

func TestMergeSkillTraceMetadataRecordsMissingAgentTargetPlannerFeedbackInOperationPlan(t *testing.T) {
	deleteStepID := operationPlanToolStepID(skills.SkillAgentManagement, "delete_agent")
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusRunning,
			"steps": []interface{}{
				map[string]interface{}{
					"id":        deleteStepID,
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "delete_agent",
					"asset_target": map[string]interface{}{
						"effect":     "delete",
						"asset_type": "agent",
					},
				},
			},
			"step_status": map[string]interface{}{
				deleteStepID: operationPlanStepStatusPending,
			},
		},
	}

	metadata = mergeSkillTraceMetadata(metadata, []skills.SkillTrace{{
		Kind:     "planner_feedback",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "list_agents",
		Status:   "advisory",
		Arguments: map[string]interface{}{
			"next_step":                  "answer_missing_agent_target",
			"reason":                     "agent_target_resolution_exhausted",
			"target_name":                "AICHAT-NOT-EXIST",
			"previous_list_agents_calls": 2,
			"empty_result_calls":         2,
		},
	}})

	if invocations := skillInvocationsFromMetadata(metadata["skill_invocations"]); len(invocations) != 0 {
		t.Fatalf("skill_invocations = %#v, want no user-visible planner feedback", invocations)
	}
	if metadata["guardrail_count"] != 0 {
		t.Fatalf("guardrail_count = %#v, want 0", metadata["guardrail_count"])
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusFailed {
		t.Fatalf("operation_plan.status = %q, want %q; plan=%#v", got, operationPlanStatusFailed, plan)
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	if got := stringFromAny(stepStatus[deleteStepID]); got != operationPlanStepStatusFailed {
		t.Fatalf("step_status[%s] = %q, want %q; plan=%#v", deleteStepID, got, operationPlanStepStatusFailed, plan)
	}
	targetResolution := mapFromOperationContext(plan["target_resolution"])
	if got := stringFromAny(targetResolution["status"]); got != "missing" {
		t.Fatalf("target_resolution.status = %q, want missing; target_resolution=%#v", got, targetResolution)
	}
	if got := stringFromAny(targetResolution["target_name"]); got != "AICHAT-NOT-EXIST" {
		t.Fatalf("target_resolution.target_name = %q, want AICHAT-NOT-EXIST", got)
	}
	deviations := mapSliceFromAny(plan["deviations"])
	foundDeviation := false
	for _, deviation := range deviations {
		if stringFromAny(deviation["skill_id"]) == skills.SkillAgentManagement &&
			stringFromAny(deviation["tool_name"]) == "list_agents" &&
			stringFromAny(deviation["reason"]) == "agent_target_resolution_exhausted" &&
			stringFromAny(deviation["outcome"]) == "failed" {
			foundDeviation = true
			break
		}
	}
	if !foundDeviation {
		t.Fatalf("deviations = %#v, want failed agent target resolution deviation", deviations)
	}
	strategyState := mapFromOperationContext(plan["strategy_state"])
	lastFeedback := mapFromOperationContext(strategyState["last_feedback"])
	if got := stringFromAny(lastFeedback["next_step"]); got != "answer_missing_agent_target" {
		t.Fatalf("strategy_state.last_feedback.next_step = %q, want answer_missing_agent_target; state=%#v", got, strategyState)
	}
	failedSteps := mapSliceFromAny(plan["failed_steps"])
	if len(failedSteps) != 1 || stringFromAny(failedSteps[0]["id"]) != deleteStepID {
		t.Fatalf("failed_steps = %#v, want failed delete step %s", failedSteps, deleteStepID)
	}
}

func TestFinalizeOperationPlanMarksMissingAgentTargetFromEmptyListEvidence(t *testing.T) {
	deleteStepID := operationPlanToolStepID(skills.SkillAgentManagement, "delete_agent")
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":             operationPlanStatusRunning,
			"original_user_goal": "请删除不存在的智能体 AICHAT-NOT-EXIST-FINALIZE。",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        deleteStepID,
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "delete_agent",
					"asset_target": map[string]interface{}{
						"effect":     "delete",
						"asset_type": "agent",
					},
				},
			},
			"step_status": map[string]interface{}{
				deleteStepID: operationPlanStepStatusPending,
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "list_agents",
				"status":    "success",
				"result": map[string]interface{}{
					"status":       "completed",
					"count":        0,
					"agents_count": 0,
				},
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "list_agents",
				"status":    "success",
				"result": map[string]interface{}{
					"status":       "completed",
					"count":        0,
					"agents_count": 0,
				},
			},
		},
	}

	finalizeOperationPlanForResult(metadata)

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusFailed {
		t.Fatalf("operation_plan.status = %q, want %q; plan=%#v", got, operationPlanStatusFailed, plan)
	}
	targetResolution := mapFromOperationContext(plan["target_resolution"])
	if got := stringFromAny(targetResolution["target_name"]); got != "AICHAT-NOT-EXIST-FINALIZE" {
		t.Fatalf("target_resolution.target_name = %q, want AICHAT-NOT-EXIST-FINALIZE; target_resolution=%#v", got, targetResolution)
	}
	if got := stringFromAny(targetResolution["reason"]); got != "agent_target_resolution_exhausted" {
		t.Fatalf("target_resolution.reason = %q, want agent_target_resolution_exhausted", got)
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	if got := stringFromAny(stepStatus[deleteStepID]); got != operationPlanStepStatusFailed {
		t.Fatalf("step_status[%s] = %q, want %q; plan=%#v", deleteStepID, got, operationPlanStepStatusFailed, plan)
	}
	strategyState := mapFromOperationContext(plan["strategy_state"])
	lastFeedback := mapFromOperationContext(strategyState["last_feedback"])
	if got := stringFromAny(lastFeedback["reason"]); got != "agent_target_resolution_exhausted" {
		t.Fatalf("strategy_state.last_feedback.reason = %q, want agent_target_resolution_exhausted; state=%#v", got, strategyState)
	}
}

func TestMergeClientActionMetadataDoesNotReviveInternalPlannerGuardrail(t *testing.T) {
	metadata := map[string]interface{}{
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "guardrail",
				"skill_id":  skills.SkillFileGenerator,
				"tool_name": "generate_file",
				"status":    "blocked",
				"error":     "generated artifact already exists",
				"arguments": map[string]interface{}{
					"next_step": "continue_planning",
				},
				"runtime_id": "internal-feedback",
			},
			map[string]interface{}{
				"kind":       "tool_call",
				"skill_id":   skills.SkillConsoleNavigator,
				"tool_name":  "navigate",
				"status":     "success",
				"runtime_id": "route-tool",
			},
		},
	}

	metadata = mergeClientActionMetadata(metadata, map[string]interface{}{
		"action_id":   "route_navigation:call-files",
		"action_type": "route_navigation",
		"skill_id":    skills.SkillConsoleNavigator,
		"tool_name":   "navigate",
		"status":      "succeeded",
		"href":        "/console/files",
	})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	for _, invocation := range invocations {
		if stringFromAny(invocation["runtime_id"]) == "internal-feedback" {
			t.Fatalf("skill_invocations = %#v, want internal planner guardrail omitted", invocations)
		}
	}
	if metadata["guardrail_count"] != 0 {
		t.Fatalf("guardrail_count = %#v, want 0", metadata["guardrail_count"])
	}
}

func TestMergeToolGovernanceDecisionMetadataDoesNotReviveInternalPlannerGuardrail(t *testing.T) {
	metadata := map[string]interface{}{
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "guardrail",
				"skill_id":  skills.SkillFileManager,
				"tool_name": "save_file_to_management",
				"status":    "blocked",
				"error":     "continue with the existing generated artifact",
				"arguments": map[string]interface{}{
					"next_step": "continue_planning",
				},
				"runtime_id": "internal-feedback",
			},
			map[string]interface{}{
				"kind":           "tool_call",
				"skill_id":       skills.SkillFileManager,
				"tool_name":      "save_file_to_management",
				"status":         "waiting_approval",
				"runtime_id":     "save-tool",
				"correlation_id": "corr-save",
			},
		},
	}

	metadata = mergeToolGovernanceDecisionMetadata(metadata, map[string]interface{}{
		"correlation_id":    "corr-save",
		"skill_id":          skills.SkillFileManager,
		"tool_name":         "save_file_to_management",
		"approval_status":   "approved",
		"status":            "allowed",
		"requires_approval": true,
	})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	for _, invocation := range invocations {
		if stringFromAny(invocation["runtime_id"]) == "internal-feedback" {
			t.Fatalf("skill_invocations = %#v, want internal planner guardrail omitted", invocations)
		}
	}
	if metadata["guardrail_count"] != 0 {
		t.Fatalf("guardrail_count = %#v, want 0", metadata["guardrail_count"])
	}
}

func TestMergeToolGovernanceDecisionMetadataUpdatesOperationPlan(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusRunning,
			"steps": []interface{}{
				map[string]interface{}{
					"id":        operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"),
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillFileManager,
					"tool_name": "save_file_to_management",
				},
			},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"): operationPlanStepStatusPending,
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":           "tool_call",
				"skill_id":       skills.SkillFileManager,
				"tool_name":      "save_file_to_management",
				"status":         "waiting_approval",
				"runtime_id":     "save-tool",
				"correlation_id": "corr-save",
			},
		},
	}

	metadata = mergeToolGovernanceDecisionMetadata(metadata, map[string]interface{}{
		"correlation_id":  "corr-save",
		"skill_id":        skills.SkillFileManager,
		"tool_name":       "save_file_to_management",
		"approval_status": "approved",
		"status":          "allowed",
	})

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusRunning {
		t.Fatalf("operation_plan.status = %q, want %q until save result evidence arrives; plan=%#v", got, operationPlanStatusRunning, plan)
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	stepID := operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management")
	if got := stringFromAny(stepStatus[stepID]); got != operationPlanStepStatusPending {
		t.Fatalf("step_status[%s] = %q, want %q until save result evidence arrives; plan=%#v", stepID, got, operationPlanStepStatusPending, plan)
	}
}

func TestMergeToolGovernanceDecisionMetadataRejectedGovernanceClosesOperationPlan(t *testing.T) {
	stepID := operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management")
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status":              operationPlanStatusRunning,
			"pending_next_action": "Save generated file",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        stepID,
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillFileManager,
					"tool_name": "save_file_to_management",
				},
			},
			"step_status": map[string]interface{}{
				stepID: operationPlanStepStatusPending,
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":       "tool_governance",
				"skill_id":   skills.SkillFileManager,
				"tool_name":  "save_file_to_management",
				"status":     "needs_approval",
				"runtime_id": "tool_governance:corr-save",
				"governance": map[string]interface{}{
					"status":            "needs_approval",
					"correlation_id":    "corr-save",
					"requires_approval": true,
					"approval_event": map[string]interface{}{
						"correlation_id": "corr-save",
						"skill_id":       skills.SkillFileManager,
						"tool_name":      "save_file_to_management",
						"tool_id":        "file.create",
					},
				},
			},
		},
	}

	metadata = mergeToolGovernanceDecisionMetadata(metadata, map[string]interface{}{
		"correlation_id":  "corr-save",
		"skill_id":        skills.SkillFileManager,
		"tool_name":       "save_file_to_management",
		"approval_status": "rejected",
		"status":          "rejected",
	})

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusFailed {
		t.Fatalf("operation_plan.status = %q, want %q after rejected governance decision; plan=%#v", got, operationPlanStatusFailed, plan)
	}
	if got := stringFromAny(plan["pending_next_action"]); got != "none" {
		t.Fatalf("operation_plan.pending_next_action = %q, want none after rejection; plan=%#v", got, plan)
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	if got := stringFromAny(stepStatus[stepID]); got != operationPlanStepStatusFailed {
		t.Fatalf("step_status[%s] = %q, want %q after rejected governance decision; plan=%#v", stepID, got, operationPlanStepStatusFailed, plan)
	}
	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	if len(invocations) != 1 || stringFromAny(invocations[0]["status"]) != "rejected" {
		t.Fatalf("skill_invocations = %#v, want governance invocation status rejected", invocations)
	}
}
