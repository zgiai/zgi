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
