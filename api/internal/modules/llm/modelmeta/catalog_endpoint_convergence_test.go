package modelmeta

import (
	"testing"

	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
)

// TestEndpointColumnsFollowUseCasesInBothDirections pins the catalog sync's
// endpoint derivation: a non-authoritative endpoint payload may only add to the
// use-case projection, never leave a stale true behind after the use case was
// removed. An authoritative payload still wins, and an empty use_cases list
// says nothing.
func TestEndpointColumnsFollowUseCasesInBothDirections(t *testing.T) {
	t.Run("stale endpoint true is cleared when its use case is gone", func(t *testing.T) {
		payload := &llmmodel.ModelEndpoints{Embeddings: true, ChatCompletions: true}
		columns := endpointColumnsForPublishedModel([]string{"text-chat"}, payload, false)
		if columns["embeddings"] {
			t.Fatal("embeddings stayed enabled after its use case was removed")
		}
		if !columns["chat_completions"] {
			t.Fatal("chat_completions must stay enabled for text-chat")
		}
	})

	t.Run("use cases still add what the payload omitted", func(t *testing.T) {
		columns := endpointColumnsForPublishedModel([]string{"embedding"}, &llmmodel.ModelEndpoints{}, false)
		if !columns["embeddings"] {
			t.Fatal("the embedding use case must enable embeddings")
		}
	})

	t.Run("authoritative payload keeps winning", func(t *testing.T) {
		payload := &llmmodel.ModelEndpoints{Embeddings: true}
		columns := endpointColumnsForPublishedModel([]string{"text-chat"}, payload, true)
		if !columns["embeddings"] {
			t.Fatal("an authoritative payload must keep its endpoint")
		}
	})

	t.Run("empty use cases keep the additive projection", func(t *testing.T) {
		payload := &llmmodel.ModelEndpoints{Embeddings: true, Vision: true}
		columns := endpointColumnsForPublishedModel(nil, payload, false)
		if !columns["embeddings"] || !columns["vision"] {
			t.Fatal("an empty use_cases list must not clear stored flags")
		}
	})

	t.Run("non-routed keys stay additive", func(t *testing.T) {
		payload := &llmmodel.ModelEndpoints{Vision: true, Moderation: true, Translation: true}
		columns := endpointColumnsForPublishedModel([]string{"text-chat"}, payload, false)
		for _, key := range []string{"vision", "moderation", "translation"} {
			if !columns[key] {
				t.Fatalf("%s must keep the additive projection", key)
			}
		}
	})
}
