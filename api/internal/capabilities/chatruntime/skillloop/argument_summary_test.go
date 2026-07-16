package skillloop

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestSummarizeSkillToolArgumentsOmitsDatabaseInternalKeys(t *testing.T) {
	result := summarizeSkillToolArguments(skills.SkillAgentDatabase, "query_table_records", map[string]interface{}{
		"data_source_id": "db-1",
		"table_id":       "tbl-1",
		"records":        []interface{}{map[string]interface{}{"id": "1"}},
		"limit":          3,
		"order":          "created desc",
	})
	if result["limit"] != 3 {
		t.Fatalf("limit = %#v, want 3", result["limit"])
	}
	if result["order"] != "created desc" {
		t.Fatalf("order = %#v, want created desc", result["order"])
	}
	for _, key := range []string{"data_source_id", "table_id", "records"} {
		if _, ok := result[key]; ok {
			t.Fatalf("%s should not be included in database argument summary: %#v", key, result)
		}
	}
}

func TestSummarizeSkillToolArgumentsKeepsFileReaderAssetIDs(t *testing.T) {
	result := summarizeSkillToolArguments(skills.SkillFileReader, "read_file", map[string]interface{}{
		"file_id":         "file-1",
		"file_ids":        []interface{}{"file-1", "file-2"},
		"include_content": true,
		"max_chars":       12000,
		"content":         "do not expose this raw content",
	})

	if result["file_id"] != "file-1" {
		t.Fatalf("file_id = %#v, want file-1", result["file_id"])
	}
	if result["include_content"] != true {
		t.Fatalf("include_content = %#v, want true", result["include_content"])
	}
	fileIDs, ok := result["file_ids"].([]string)
	if !ok || len(fileIDs) != 2 {
		t.Fatalf("file_ids = %#v, want two ids", result["file_ids"])
	}
	if _, ok := result["content"]; ok {
		t.Fatalf("content should not be included in file-reader argument summary: %#v", result)
	}
}

func TestSummarizeSkillToolArgumentsKeepsConsoleNavigationHref(t *testing.T) {
	result := summarizeSkillToolArguments(skills.SkillConsoleNavigator, "navigate", map[string]interface{}{
		"href":   "/console/work/task",
		"reason": "User asked to open scheduled tasks.",
		"extra":  "not needed",
	})

	if result["href"] != "/console/work/task" {
		t.Fatalf("href = %#v, want route href", result["href"])
	}
	if result["reason"] != "User asked to open scheduled tasks." {
		t.Fatalf("reason = %#v, want user-facing reason", result["reason"])
	}
	if _, ok := result["extra"]; ok {
		t.Fatalf("extra should not be included in console navigation argument summary: %#v", result)
	}
}

func TestSummarizeAgentSystemPromptPatchOmitsBody(t *testing.T) {
	text := "exact prompt body that must not be persisted in compact arguments"
	result := summarizeSkillToolArguments(skills.SkillAgentManagement, "update_agent_config", map[string]interface{}{
		"agent_id": "agent-1",
		"system_prompt_patch": map[string]interface{}{
			"operation":                "append",
			"expected_base_sha256":     "sha256:base",
			"expected_base_characters": 10,
			"separator":                "\n\n",
			"separator_sha256":         "sha256:separator",
			"separator_characters":     2,
			"source": map[string]interface{}{
				"type": "text",
				"text": text,
			},
		},
	})
	patch, ok := result["system_prompt_patch"].(map[string]interface{})
	if !ok {
		t.Fatalf("system_prompt_patch = %#v, want summary object", result["system_prompt_patch"])
	}
	if _, leaked := patch["separator"]; leaked {
		t.Fatalf("separator body leaked into summary: %#v", patch)
	}
	if patch["separator_sha256"] != "sha256:separator" || patch["separator_characters"] != 2 {
		t.Fatalf("separator evidence = %#v, want digest and character count", patch)
	}
	source, ok := patch["source"].(map[string]interface{})
	if !ok {
		t.Fatalf("source = %#v, want summary object", patch["source"])
	}
	if _, leaked := source["text"]; leaked {
		t.Fatalf("prompt body leaked into summary: %#v", source)
	}
	if source["characters"] != len([]rune(text)) || !strings.HasPrefix(stringFromInterface(source["sha256"]), "sha256:") {
		t.Fatalf("source evidence = %#v, want digest and character count", source)
	}
	if strings.Contains(stringFromInterface(source["summary"]), text) {
		t.Fatalf("summary retained the complete prompt body: %#v", source["summary"])
	}
}

func TestSummarizeSkillToolArgumentsAddsFileGeneratorExtension(t *testing.T) {
	result := summarizeSkillToolArguments(skills.SkillFileGenerator, "generate_file", map[string]interface{}{
		"format":   "svg",
		"filename": "monthly-sales-chart",
		"content":  "<svg></svg>",
	})

	if result["filename"] != "monthly-sales-chart.svg" {
		t.Fatalf("filename = %#v, want monthly-sales-chart.svg", result["filename"])
	}
	if result["format"] != "svg" {
		t.Fatalf("format = %#v, want svg", result["format"])
	}
	if result["content_length"] != len("<svg></svg>") {
		t.Fatalf("content_length = %#v, want %d", result["content_length"], len("<svg></svg>"))
	}
}

func TestSummarizeSkillToolArgumentsAddsReusableContentEvidence(t *testing.T) {
	result := summarizeSkillToolArguments(skills.SkillFileGenerator, "generate_file", map[string]interface{}{
		"format":  "md",
		"content": "第一章\nA reusable summary.",
	})
	if result["content_chars"] != len([]rune("第一章\nA reusable summary.")) {
		t.Fatalf("content_chars = %#v", result["content_chars"])
	}
	if !strings.HasPrefix(stringFromInterface(result["content_sha256"]), "sha256:") {
		t.Fatalf("content_sha256 = %#v", result["content_sha256"])
	}
	if stringFromInterface(result["content_summary"]) == "" {
		t.Fatalf("content_summary = %#v", result["content_summary"])
	}
}

func TestSummarizeSkillToolArgumentsCorrectsFileGeneratorExtension(t *testing.T) {
	result := summarizeSkillToolArguments(skills.SkillFileGenerator, "generate_file", map[string]interface{}{
		"format":   "txt",
		"filename": "monthly-sales-chart.svg",
	})

	if result["filename"] != "monthly-sales-chart.txt" {
		t.Fatalf("filename = %#v, want monthly-sales-chart.txt", result["filename"])
	}
}

func TestSummarizeSkillToolArgumentsKeepsFileManagerSaveFilename(t *testing.T) {
	result := summarizeSkillToolArguments(skills.SkillFileManager, "save_file_to_management", map[string]interface{}{
		"source_type":  "tool_file",
		"tool_file_id": "tool-file-1",
		"filename":     "monthly-sales-chart.svg",
		"content":      "do not expose",
	})

	if result["filename"] != "monthly-sales-chart.svg" {
		t.Fatalf("filename = %#v, want monthly-sales-chart.svg", result["filename"])
	}
	if result["source_type"] != "tool_file" {
		t.Fatalf("source_type = %#v, want tool_file", result["source_type"])
	}
	for _, key := range []string{"tool_file_id", "content"} {
		if _, ok := result[key]; ok {
			t.Fatalf("%s should not be included in file-manager save summary: %#v", key, result)
		}
	}
}

func TestSummarizeSkillToolArgumentsOmitsFileManagerDeleteFileID(t *testing.T) {
	result := summarizeSkillToolArguments(skills.SkillFileManager, "delete_file", map[string]interface{}{
		"file_id":  "file-secret-id",
		"filename": "old-report.txt",
	})

	if result["filename"] != "old-report.txt" {
		t.Fatalf("filename = %#v, want old-report.txt", result["filename"])
	}
	if _, ok := result["file_id"]; ok {
		t.Fatalf("file_id should not be included in file-manager delete summary: %#v", result)
	}
}

func TestApplyGovernedAssetArgumentsUsesAllowedGovernanceAsset(t *testing.T) {
	trace := skills.SkillTrace{
		Arguments: map[string]interface{}{
			"file_id":   "file-wrong",
			"max_chars": 8000,
		},
		Governance: &toolgovernance.Decision{
			Status: toolgovernance.DecisionStatusAllowed,
			Manifest: toolgovernance.Manifest{
				Effect:    toolgovernance.EffectRead,
				AssetType: "file",
			},
			Assets: []toolgovernance.AssetRef{{ID: "file-expected", Type: "file"}},
		},
	}

	applyGovernedAssetArguments(&trace)

	if trace.Arguments["file_id"] != "file-expected" {
		t.Fatalf("file_id = %#v, want governed asset", trace.Arguments["file_id"])
	}
	rewrite, ok := trace.Arguments["governance_argument_rewrite"].(map[string]interface{})
	if !ok || rewrite["from_file_id"] != "file-wrong" || rewrite["to_file_id"] != "file-expected" {
		t.Fatalf("rewrite = %#v, want from/to ids", trace.Arguments["governance_argument_rewrite"])
	}
}
