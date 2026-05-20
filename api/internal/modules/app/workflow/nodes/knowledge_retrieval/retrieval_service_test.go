package knowledgeretrieval

import (
	"testing"

	"github.com/zgiai/ginext/internal/dto"
	drepo "github.com/zgiai/ginext/internal/modules/dataset/repository"
	dservice "github.com/zgiai/ginext/internal/modules/dataset/service"
)

func TestBuildWorkflowRetrieveOptionsPromotesGraphSearchToHybridCandidates(t *testing.T) {
	base := &dservice.RetrievalOptions{
		SearchMethod:          "graph_search",
		TopK:                  4,
		ScoreThreshold:        0.5,
		ScoreThresholdEnabled: true,
	}

	opts := buildWorkflowRetrieveOptions(base)

	if opts.RetrievalMode != "hybrid" {
		t.Fatalf("RetrievalMode = %q, want %q", opts.RetrievalMode, "hybrid")
	}
	if opts.SearchMethod != "" {
		t.Fatalf("SearchMethod = %q, want empty string to enable vector fallback", opts.SearchMethod)
	}
	if base.SearchMethod != "graph_search" {
		t.Fatalf("base options were mutated: SearchMethod = %q", base.SearchMethod)
	}
}

func TestPreferWorkflowRecordsKeepsOnlyGraphWhenPresent(t *testing.T) {
	records := []dto.HitTestingRecordResponse{
		{
			Score: 0.9,
			RetrievalSource: &dto.RetrievalSourceResponse{
				Method: "semantic_search",
			},
		},
		{
			Score: 0.8,
			RetrievalSource: &dto.RetrievalSourceResponse{
				Method: "graph_knowledge",
			},
		},
	}

	preferred := preferWorkflowRecords(records)
	if len(preferred) != 1 {
		t.Fatalf("preferred record count = %d, want 1 graph record", len(preferred))
	}
	if preferred[0].RetrievalSource == nil || preferred[0].RetrievalSource.Method != "graph_knowledge" {
		t.Fatalf("preferred record method = %#v, want graph_knowledge", preferred[0].RetrievalSource)
	}
}

func TestPreferWorkflowRecordsFallsBackToVectorWhenGraphMissing(t *testing.T) {
	records := []dto.HitTestingRecordResponse{
		{
			Score: 0.9,
			RetrievalSource: &dto.RetrievalSourceResponse{
				Method: "semantic_search",
			},
		},
		{
			Score: 0.7,
			RetrievalSource: &dto.RetrievalSourceResponse{
				Method: "full_text_search",
			},
		},
	}

	preferred := preferWorkflowRecords(records)
	if len(preferred) != len(records) {
		t.Fatalf("preferred record count = %d, want %d", len(preferred), len(records))
	}
	for i := range records {
		if preferred[i].RetrievalSource == nil || records[i].RetrievalSource == nil {
			t.Fatalf("expected retrieval source to be preserved at index %d", i)
		}
		if preferred[i].RetrievalSource.Method != records[i].RetrievalSource.Method {
			t.Fatalf("method at index %d = %q, want %q", i, preferred[i].RetrievalSource.Method, records[i].RetrievalSource.Method)
		}
	}
}

func TestBuildDatasetRetrievalServiceRequiresGatewayEmbeddingFactory(t *testing.T) {
	svc := &defaultRetrievalService{}

	retrievalService, err := svc.buildDatasetRetrievalService(
		nil,
		drepo.DocumentRepository(nil),
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatalf("expected error when embedding factory is nil")
	}
	if retrievalService != nil {
		t.Fatalf("expected nil retrieval service when embedding factory is nil")
	}
}

func TestMakeEmbeddingFactoryReturnsNilWithoutInvoker(t *testing.T) {
	svc := &defaultRetrievalService{}

	factory := svc.makeEmbeddingFactory("account-1")
	if factory != nil {
		t.Fatalf("expected nil factory when embInvoker is nil")
	}
}
