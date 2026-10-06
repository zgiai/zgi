package skillloop

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestKnowledgeRetrievalHistoryForToolCallsKeepsOnlyAgentKnowledge(t *testing.T) {
	calls := []SkillToolCallRef{
		{
			SkillID:   skills.SkillAgentKnowledge,
			ToolName:  "retrieve_agent_knowledge",
			Arguments: map[string]interface{}{"query": "Apple and Intel"},
			Result:    map[string]interface{}{"result_count": 2},
		},
		{
			SkillID:   "other-skill",
			ToolName:  "unrelated_tool",
			Arguments: map[string]interface{}{"query": "ignored"},
		},
	}

	history := knowledgeRetrievalHistoryForToolCalls(calls)
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1", len(history))
	}
	if got, _ := history[0]["arguments"].(map[string]interface{})["query"].(string); got != "Apple and Intel" {
		t.Fatalf("history query = %q, want original query", got)
	}
}

func TestRuntimeStateWithSuccessfulToolCallsInjectsRetrievalHistory(t *testing.T) {
	calls := []SkillToolCallRef{{
		SkillID:   skills.SkillAgentKnowledge,
		ToolName:  "retrieve_agent_knowledge",
		Arguments: map[string]interface{}{"query": "policy"},
		Result:    map[string]interface{}{"status": "success"},
	}}

	state := runtimeStateWithSuccessfulToolCalls(RunRequest{}, calls)
	history, ok := state[runtimeStateKnowledgeRetrievalHistoryKey].([]map[string]interface{})
	if !ok || len(history) != 1 {
		t.Fatalf("retrieval history = %#v, want one call", state[runtimeStateKnowledgeRetrievalHistoryKey])
	}
	if got, _ := history[0]["arguments"].(map[string]interface{})["query"].(string); got != "policy" {
		t.Fatalf("history query = %q, want policy", got)
	}
}
