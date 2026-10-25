package agentmemoryworker

import "testing"

func TestCompletedTurnIncludesUserInputAnswers(t *testing.T) {
	turn := completedTurn{
		Query:              "我在考虑换城市工作，候选是成都和杭州。",
		UserInputResponses: []byte(`[{"request_id":"r1","answers":[{"question_id":"q1","value":"我决定去成都了"}]},{"answers":[{"value":" "}]}]`),
	}.withUserInputAnswers()
	if turn.Query != "我在考虑换城市工作，候选是成都和杭州。\n我决定去成都了" {
		t.Fatalf("query = %q", turn.Query)
	}
	if plain := (completedTurn{Query: "q"}).withUserInputAnswers(); plain.Query != "q" {
		t.Fatalf("turn without answers = %q", plain.Query)
	}
	if bad := (completedTurn{Query: "q", UserInputResponses: []byte("null")}).withUserInputAnswers(); bad.Query != "q" {
		t.Fatalf("null responses = %q", bad.Query)
	}
}
