package digitalhuman

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	datasetmodel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	llmruntime "github.com/zgiai/zgi/api/internal/modules/llm/runtime"
)

func TestAuthoritativeMemoryResultKeepsActiveMemoriesAndDropsUnknownOrInactiveHits(t *testing.T) {
	allowedID := uuid.New()
	legacyBusinessID := uuid.New()
	inactiveID := uuid.New()
	memories := map[uuid.UUID]*DigitalHumanMemory{
		allowedID:        {ID: allowedID, MemoryKey: "allowed-key", Revision: 3, MemoryClass: memoryruntime.MemoryClassSelf, Status: MemoryStatusActive, Content: "allowed"},
		legacyBusinessID: {ID: legacyBusinessID, MemoryKey: "legacy-business-key", Revision: 1, MemoryClass: memoryruntime.MemoryClassBusiness, ScopeType: memoryruntime.MemoryScopeCustomer, ScopeID: uuid.New().String(), Status: MemoryStatusActive, Content: "legacy business stays visible"},
		inactiveID:       {ID: inactiveID, MemoryKey: "archived-key", Revision: 1, MemoryClass: memoryruntime.MemoryClassSelf, Status: MemoryStatusArchived, Content: "archived"},
	}
	response := &datasetservice.KnowledgeRetrieveResponse{Resources: []datasetservice.KnowledgeRetrieverResource{
		{SegmentID: allowedID.String(), Score: 0.9},
		{SegmentID: legacyBusinessID.String(), Score: 0.8},
		{SegmentID: inactiveID.String(), Score: 0.7},
		{SegmentID: uuid.New().String(), Score: 1.0},
	}}
	result := (&DigitalHumanMemoryProjectionProvider{}).authoritativeResult(response, memories)
	if result.ResultCount != 2 || len(result.Items) != 2 || result.Items[0].ID != allowedID || result.Items[1].ID != legacyBusinessID {
		t.Fatalf("result = %#v, want both active memories regardless of class or scope", result)
	}
	if result.Context != "[memory 1] allowed\n\n[memory 2] legacy business stays visible" {
		t.Fatalf("context = %q, want authoritative context", result.Context)
	}
	// The mutation contract is id-, key- and revision-addressed, so every match
	// must hand the model the values it needs to correct, clear or replace that
	// memory later.
	wantBlocks := []struct {
		id       uuid.UUID
		key      string
		revision int64
	}{{allowedID, "allowed-key", 3}, {legacyBusinessID, "legacy-business-key", 1}}
	if len(result.ContextBlocks) != len(wantBlocks) {
		t.Fatalf("blocks = %#v, want one context block per match", result.ContextBlocks)
	}
	for index, want := range wantBlocks {
		block := result.ContextBlocks[index]
		if block["memory_id"] != want.id.String() || block["memory_key"] != want.key || block["revision"] != want.revision {
			t.Fatalf("block %d = %#v, want id %s key %q revision %d", index, block, want.id, want.key, want.revision)
		}
	}
}

func TestRecentMemoryResultOrdersFallbackAndFlagsDegradedRecall(t *testing.T) {
	newerID := uuid.New()
	olderID := uuid.New()
	result := recentMemoryResult([]*DigitalHumanMemory{
		{ID: newerID, MemoryKey: "newest-key", Revision: 2, Status: MemoryStatusActive, Content: "newest"},
		{ID: olderID, MemoryKey: "older-key", Revision: 5, Status: MemoryStatusActive, Content: "older"},
		{ID: uuid.New(), MemoryKey: "inactive-key", Revision: 1, Status: MemoryStatusInvalidated, Content: "not active"},
		nil,
	}, "vector recall failed")
	if result.Status != datasetservice.KnowledgeRetrieveStatusSuccess || result.ResultCount != 2 {
		t.Fatalf("result = %#v, want two active fallback memories", result)
	}
	if result.Items[0].ID != newerID || result.Items[1].ID != olderID {
		t.Fatalf("items = %#v, want repository recency order preserved", result.Items)
	}
	if result.Context != "[memory 1] newest\n\n[memory 2] older" {
		t.Fatalf("context = %q, want fallback context", result.Context)
	}
	if result.TopScore != nil {
		t.Fatalf("top score = %v, want nil because recency carries no relevance signal", *result.TopScore)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %#v, want the degraded recall to be flagged", result.Warnings)
	}
	for index, want := range []struct {
		id       uuid.UUID
		key      string
		revision int64
	}{{newerID, "newest-key", 2}, {olderID, "older-key", 5}} {
		block := result.ContextBlocks[index]
		if block["memory_id"] != want.id.String() || block["memory_key"] != want.key || block["revision"] != want.revision {
			t.Fatalf("fallback block %d = %#v, want id %s key %q revision %d", index, block, want.id, want.key, want.revision)
		}
	}
}

func TestRecentMemoryResultWithoutActiveMemoriesStaysEmpty(t *testing.T) {
	result := recentMemoryResult(nil, "projection unavailable")
	if result.Status != datasetservice.KnowledgeRetrieveStatusNoResults || result.ResultCount != 0 || result.Context != "" {
		t.Fatalf("result = %#v, want an empty no_results fallback", result)
	}
}

func TestProjectionEmbeddingChangedDetectsDefaultModelDrift(t *testing.T) {
	value := func(text string) *string { return &text }
	dataset := &datasetmodel.Dataset{EmbeddingModelProvider: value("provider-a"), EmbeddingModel: value("embed-a")}
	if projectionEmbeddingChanged(dataset, &llmruntime.ResolvedModel{Provider: "provider-a", Model: "embed-a"}) {
		t.Fatal("an unchanged default embedding model must not force a rebuild")
	}
	if !projectionEmbeddingChanged(dataset, &llmruntime.ResolvedModel{Provider: "provider-a", Model: "embed-b"}) {
		t.Fatal("a changed embedding model must force a rebuild")
	}
	if !projectionEmbeddingChanged(dataset, &llmruntime.ResolvedModel{Provider: "provider-b", Model: "embed-a"}) {
		t.Fatal("a changed embedding provider must force a rebuild")
	}
	providerOnly := &datasetmodel.Dataset{EmbeddingModelProvider: value(""), EmbeddingModel: value("embed-a")}
	if projectionEmbeddingChanged(providerOnly, &llmruntime.ResolvedModel{Provider: "provider-b", Model: "embed-a"}) {
		t.Fatal("an unrecorded provider must not force a rebuild")
	}
	if projectionEmbeddingChanged(&datasetmodel.Dataset{}, &llmruntime.ResolvedModel{Provider: "provider-a", Model: "embed-a"}) {
		t.Fatal("a projection that never stored vectors must not force a rebuild")
	}
}

func TestMemoryProjectionStableDocumentID(t *testing.T) {
	id := uuid.New()
	if stableProjectionUUID("document", id) != stableProjectionUUID("document", id) {
		t.Fatal("stable projection id changed between calls")
	}
	if stableProjectionUUID("document", id) == stableProjectionUUID("document", uuid.New()) {
		t.Fatal("different DigitalHuman ids share projection document id")
	}
}
