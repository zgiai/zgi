package service

import (
	"strings"
	"testing"

	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
)

func TestBuildRecentExecutionContextMessageLimitsToolAndIntermediateHistory(t *testing.T) {
	branch := []*aichatmodel.Message{
		recentExecutionTestMessage("old query", "old answer", []interface{}{
			map[string]interface{}{"kind": "tool_call", "status": "success", "skill_id": "old-skill", "tool_name": "old_tool"},
			map[string]interface{}{"kind": "intermediate_answer", "status": "success", "title": "Old draft", "message": "old intermediate draft"},
		}),
		recentExecutionTestMessage("third query", "third answer", []interface{}{
			map[string]interface{}{"kind": "intermediate_answer", "status": "success", "title": "Third draft", "message": "third intermediate draft"},
		}),
		recentExecutionTestMessage("second query", "second answer", []interface{}{
			map[string]interface{}{"kind": "tool_call", "status": "success", "skill_id": "second-skill", "tool_name": "second_tool"},
			map[string]interface{}{"kind": "intermediate_answer", "status": "success", "title": "Second draft", "message": "second intermediate draft"},
		}),
		recentExecutionTestMessage("latest query", "latest answer", []interface{}{
			map[string]interface{}{"kind": "skill_load", "status": "success", "skill_id": "latest-skill"},
			map[string]interface{}{"kind": "tool_call", "status": "success", "skill_id": "latest-skill", "tool_name": "latest_tool"},
			map[string]interface{}{"kind": "intermediate_answer", "status": "success", "title": "Latest draft", "message": "latest intermediate draft"},
		}),
	}

	message, stats := buildRecentExecutionContextMessage(branch)
	if message == nil {
		t.Fatalf("message is nil, want recent execution context")
	}
	content, _ := message.Content.(string)
	for _, want := range []string{
		"latest_tool",
		"latest intermediate draft",
		"second intermediate draft",
		"third intermediate draft",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q:\n%s", want, content)
		}
	}
	for _, unwanted := range []string{
		"second_tool",
		"old_tool",
		"old intermediate draft",
	} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("content contains %q, want it omitted:\n%s", unwanted, content)
		}
	}
	if stats.ToolHistoryTurns != 1 || stats.IntermediateAnswerTurns != 3 {
		t.Fatalf("stats turns = (%d, %d), want (1, 3)", stats.ToolHistoryTurns, stats.IntermediateAnswerTurns)
	}
}

func recentExecutionTestMessage(query string, answer string, invocations []interface{}) *aichatmodel.Message {
	return &aichatmodel.Message{
		Query:  query,
		Answer: answer,
		Status: aichatmodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"skill_invocations": invocations,
		},
	}
}
