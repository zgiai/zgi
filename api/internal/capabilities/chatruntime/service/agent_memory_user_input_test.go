package service

import (
	"strings"
	"testing"
)

func TestAgentMemoryEvidenceIncludesUserInputAnswers(t *testing.T) {
	metadata := resolveUserInputContinuationMetadata(nil, "message-1",
		map[string]interface{}{"message": "选哪个城市？"},
		map[string]interface{}{"request_id": "r1", "answers": []interface{}{
			map[string]interface{}{"question_id": "q1", "value": "嗯，我决定去成都了，帮我记住。"},
		}})
	parts := &chatRequestParts{Query: "我在考虑换城市工作，候选是成都和杭州。"}
	parts.UserInputAnswers = userInputAnswerValues(mapSliceFromAny(metadata["user_input_responses"]))

	text := currentTurnUserText(parts)
	if !strings.Contains(text, parts.Query) || !strings.Contains(text, "嗯，我决定去成都了，帮我记住。") {
		t.Fatalf("current turn text = %q, want the query and the answer", text)
	}
	if got := currentTurnUserText(&chatRequestParts{Query: "只有问题"}); got != "只有问题" {
		t.Fatalf("turn without answers = %q", got)
	}
}
