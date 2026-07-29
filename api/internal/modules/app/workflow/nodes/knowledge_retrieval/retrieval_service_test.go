package knowledgeretrieval

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	drepo "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	dservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
)

func TestParseKnowledgeRetrievalNodeRemovesLegacyDatasetPlaceholder(t *testing.T) {
	const datasetID = "43ea3443-7667-48b8-be43-8d63824d9bdf"
	nodeData, _, err := parseKnowledgeRetrievalNodeDataFromConfig(map[string]any{
		"id": "knowledge-1",
		"data": map[string]any{
			"type":        "knowledge-retrieval",
			"dataset_ids": []string{legacyKnowledgeDatasetIDPlaceholder, datasetID},
		},
	})
	if err != nil {
		t.Fatalf("parseKnowledgeRetrievalNodeDataFromConfig() error = %v", err)
	}
	if len(nodeData.DatasetIds) != 1 || nodeData.DatasetIds[0] != datasetID {
		t.Fatalf("DatasetIds = %#v, want only %q", nodeData.DatasetIds, datasetID)
	}
}

func TestFetchAvailableDatasetsRejectsInvalidIDBeforeDatabaseQuery(t *testing.T) {
	node := &Node{
		NodeData: NodeData{
			DatasetIds: []string{"not-a-uuid"},
		},
	}

	_, err := node.fetchAvailableDatasets()
	if err == nil {
		t.Fatal("fetchAvailableDatasets() error = nil, want invalid knowledge base ID error")
	}
	if !strings.Contains(err.Error(), "invalid knowledge base ID") {
		t.Fatalf("fetchAvailableDatasets() error = %q, want invalid knowledge base ID", err)
	}
}

func TestParseKnowledgeRetrievalNodeDefaultsToVectorRetrieval(t *testing.T) {
	nodeData, _, err := parseKnowledgeRetrievalNodeDataFromConfig(map[string]any{
		"id": "knowledge-1",
		"data": map[string]any{
			"type":           "knowledge-retrieval",
			"retrieval_mode": "multiple",
			"multiple_retrieval_config": map[string]any{
				"top_k": 4,
			},
		},
	})
	if err != nil {
		t.Fatalf("parseKnowledgeRetrievalNodeDataFromConfig() error = %v", err)
	}
	if nodeData.MultipleRetrievalConfig == nil {
		t.Fatal("MultipleRetrievalConfig is nil")
	}
	if nodeData.MultipleRetrievalConfig.SearchMethod != "semantic_search" {
		t.Fatalf("SearchMethod = %q, want semantic_search", nodeData.MultipleRetrievalConfig.SearchMethod)
	}
	if nodeData.MultipleRetrievalConfig.FallbackPolicy != "none" {
		t.Fatalf("FallbackPolicy = %q, want none", nodeData.MultipleRetrievalConfig.FallbackPolicy)
	}
}

func TestParseKnowledgeRetrievalNodePreservesGraphRetrieval(t *testing.T) {
	nodeData, _, err := parseKnowledgeRetrievalNodeDataFromConfig(map[string]any{
		"id": "knowledge-1",
		"data": map[string]any{
			"type":           "knowledge-retrieval",
			"retrieval_mode": "multiple",
			"multiple_retrieval_config": map[string]any{
				"top_k":           4,
				"search_method":   "graph_search",
				"fallback_policy": "none",
			},
		},
	})
	if err != nil {
		t.Fatalf("parseKnowledgeRetrievalNodeDataFromConfig() error = %v", err)
	}
	if nodeData.MultipleRetrievalConfig == nil {
		t.Fatal("MultipleRetrievalConfig is nil")
	}
	if nodeData.MultipleRetrievalConfig.SearchMethod != "graph_search" {
		t.Fatalf("SearchMethod = %q, want graph_search", nodeData.MultipleRetrievalConfig.SearchMethod)
	}
}

func TestBuildWorkflowRetrieveOptionsPreservesGraphOnlyExecution(t *testing.T) {
	base := &dservice.RetrievalOptions{
		SearchMethod:          "graph_search",
		TopK:                  4,
		ScoreThreshold:        0.5,
		ScoreThresholdEnabled: true,
	}

	opts := buildWorkflowRetrieveOptions(base)

	if opts.RetrievalMode != "graph" {
		t.Fatalf("RetrievalMode = %q, want %q", opts.RetrievalMode, "graph")
	}
	if opts.SearchMethod != "graph_search" {
		t.Fatalf("SearchMethod = %q, want graph_search", opts.SearchMethod)
	}
	if opts.FallbackPolicy != "none" {
		t.Fatalf("FallbackPolicy = %q, want none", opts.FallbackPolicy)
	}
	if base.SearchMethod != "graph_search" {
		t.Fatalf("base options were mutated: SearchMethod = %q", base.SearchMethod)
	}
}

func TestBuildWorkflowRetrieveOptionsPreservesExplicitGraphFallback(t *testing.T) {
	opts := buildWorkflowRetrieveOptions(&dservice.RetrievalOptions{
		SearchMethod:   "graph",
		FallbackPolicy: "vector",
	})
	if opts.RetrievalMode != "graph" {
		t.Fatalf("RetrievalMode = %q, want graph", opts.RetrievalMode)
	}
	if opts.FallbackPolicy != "vector" {
		t.Fatalf("FallbackPolicy = %q, want vector", opts.FallbackPolicy)
	}
}

func TestWorkflowGraphFailurePropagationPolicy(t *testing.T) {
	if !workflowGraphFailureMustPropagate("graph_search", "none") {
		t.Fatal("graph retrieval without fallback must propagate errors")
	}
	if workflowGraphFailureMustPropagate("graph_search", "vector") {
		t.Fatal("explicit vector fallback may continue to other retrieval paths")
	}
	if workflowGraphFailureMustPropagate("hybrid_search", "none") {
		t.Fatal("non-graph retrieval must keep existing multi-dataset behavior")
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
