package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestSupplementalHistoryBranchDeduplicatesAndBoundsFormalTurns(t *testing.T) {
	branch := []*runtimemodel.Message{
		{ID: uuid.New(), Query: "old one", Answer: "old answer one"},
		{ID: uuid.New(), Query: "old two", Answer: "old answer two"},
		{ID: uuid.New(), Query: " duplicated\nprovider text ", Answer: "duplicate answer"},
		{ID: uuid.New(), Query: "recent one", Answer: "recent answer one"},
		{ID: uuid.New(), Query: "recent two", Answer: "recent answer two"},
	}
	parts := &chatRequestParts{HistoryPolicy: &runtimedto.ConversationHistoryPolicy{
		Mode: runtimedto.ConversationHistoryModeSupplemental, MaxTurns: 4,
		ExcludedTexts: []string{"duplicated provider text"},
	}}

	selected := supplementalHistoryBranch(branch, parts)
	if len(selected) != 4 || selected[0].Query != "old one" || selected[3].Query != "recent two" {
		t.Fatalf("supplemental history = %#v", selected)
	}
	for _, message := range selected {
		if normalizedHistoryContent(message.Query) == "duplicated provider text" {
			t.Fatalf("provider-duplicate formal turn was retained: %#v", selected)
		}
	}
}

func TestConversationHistoryPolicyIsServerOnly(t *testing.T) {
	request := runtimedto.ChatRequest{
		Query: "hello", Model: "gpt-test",
		HistoryPolicy: &runtimedto.ConversationHistoryPolicy{
			Mode: runtimedto.ConversationHistoryModeSupplemental, MaxTurns: 4,
			ExcludedTexts: []string{"private provider context"},
		},
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || strings.Contains(string(encoded), "private provider context") || strings.Contains(string(encoded), "history") {
		t.Fatalf("server-only history policy leaked into JSON: %s", encoded)
	}
}
