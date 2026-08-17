package service

import (
	"context"
	"testing"

	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/retrieval"
)

func TestGetRetrievalOptionsUsesFixedGraphHopDepth(t *testing.T) {
	service := &hitTestingService{}
	options := service.getRetrievalOptions(
		context.Background(),
		map[string]interface{}{
			"hop_depth": float64(1),
			"reranking_model": map[string]interface{}{
				"reranking_provider_name": "test-provider",
				"reranking_model_name":    "test-model",
			},
		},
		&dataset_model.Dataset{EnableGraphFlow: true},
	)

	if options.HopDepth != defaultGraphRetrievalHopDepth {
		t.Fatalf("HopDepth = %d, want %d", options.HopDepth, defaultGraphRetrievalHopDepth)
	}
}

func TestGetRetrievalOptionsDefaultsByGraphAvailability(t *testing.T) {
	service := &hitTestingService{
		retrievalService: &RetrievalService{
			vectorRetrieval: retrieval.NewVectorRetrievalService(nil, nil, ""),
		},
	}
	retrievalModel := map[string]interface{}{
		"reranking_model": map[string]interface{}{
			"reranking_provider_name": "test-provider",
			"reranking_model_name":    "test-model",
		},
	}

	nonGraphOptions := service.getRetrievalOptions(
		context.Background(),
		retrievalModel,
		&dataset_model.Dataset{EnableGraphFlow: false},
	)
	if nonGraphOptions.SearchMethod != string(HybridSearch) {
		t.Fatalf("non-graph SearchMethod = %q, want %q", nonGraphOptions.SearchMethod, HybridSearch)
	}

	graphOptions := service.getRetrievalOptions(
		context.Background(),
		retrievalModel,
		&dataset_model.Dataset{EnableGraphFlow: true},
	)
	if graphOptions.SearchMethod != string(GraphSearch) {
		t.Fatalf("graph SearchMethod = %q, want %q", graphOptions.SearchMethod, GraphSearch)
	}
}
