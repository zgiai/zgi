package gateway

import (
	"testing"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
)

func TestFilterRoutesForOperation(t *testing.T) {
	chatID, videoID := uuid.New(), uuid.New()
	routes := []*channelmodel.LLMRoute{
		{ID: chatID, ModelOperations: map[string][]string{"seedance": {operationChatCompletions}}},
		{ID: videoID, ModelOperations: map[string][]string{"seedance": {operationVideoCreate, operationVideoRetrieve}}},
	}
	got := filterRoutesForOperation(routes, "seedance", operationVideoCreate)
	if len(got) != 1 || got[0].ID != videoID {
		t.Fatalf("video candidates = %#v, want only %s", got, videoID)
	}
	got = filterRoutesForOperation(routes, "seedance", operationChatCompletions)
	if len(got) != 1 || got[0].ID != chatID {
		t.Fatalf("chat candidates = %#v, want only %s", got, chatID)
	}
}

func TestFilterRoutesForOperationKeepsLegacyRoutes(t *testing.T) {
	route := &channelmodel.LLMRoute{ID: uuid.New()}
	got := filterRoutesForOperation([]*channelmodel.LLMRoute{route}, "legacy", operationVideoCreate)
	if len(got) != 1 || got[0] != route {
		t.Fatalf("legacy route was filtered: %#v", got)
	}
}
