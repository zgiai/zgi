package service

import (
	"context"
	"testing"

	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
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
		&dataset_model.Dataset{},
	)

	if options.HopDepth != defaultGraphRetrievalHopDepth {
		t.Fatalf("HopDepth = %d, want %d", options.HopDepth, defaultGraphRetrievalHopDepth)
	}
}
