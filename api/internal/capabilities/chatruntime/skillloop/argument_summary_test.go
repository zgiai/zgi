package skillloop

import (
	"testing"

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
