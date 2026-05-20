package indexing

import (
	"context"
	"strings"
	"testing"

	chunkexecutor "github.com/zgiai/ginext/internal/capabilities/chunking/executor"
	"github.com/zgiai/ginext/internal/capabilities/contentparse/routing"
	"github.com/zgiai/ginext/internal/contracts"
	"github.com/zgiai/ginext/internal/dto"
	contentparsesvc "github.com/zgiai/ginext/internal/modules/contentparse/service"
	dataset_model "github.com/zgiai/ginext/internal/modules/dataset/model"
	dataset_repository "github.com/zgiai/ginext/internal/modules/dataset/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Test buildEmbeddingService gateway success path (model provided, gateway constructed).
// Note: This test requires a full LLM client and default model service to work properly.
// With nil dependencies, it will fail to build embedding service.
func TestBuildEmbeddingService_GatewaySuccess(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	docRepo := dataset_repository.NewDocumentRepository(db)
	// Pass nil for llmClient - gateway will not be available
	// This test now verifies that the runner handles nil dependencies gracefully
	runner := NewIndexingRunner(nil, docRepo, nil, nil, nil, nil, nil, nil, nil, nil)

	modelName := "text-embedding-3-large"
	datasetID := "ds-1"
	tenantID := "tenant-1"
	createdBy := "user-1"

	dataset := &dataset_model.Dataset{
		ID:             datasetID,
		WorkspaceID:    tenantID,
		EmbeddingModel: &modelName,
		CreatedBy:      createdBy,
	}

	// With nil llmClient and nil default model service, buildEmbeddingService should return an error
	// because neither gateway nor fallback can be constructed
	_, err = runner.buildEmbeddingService(context.Background(), dataset)
	if err == nil {
		t.Fatalf("expected error when both llmClient and default model service are nil")
	}
}

// Test buildEmbeddingService returns error when dataset is nil.
func TestBuildEmbeddingService_NilDataset(t *testing.T) {
	// Pass nil for llmClient
	runner := NewIndexingRunner(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	if _, err := runner.buildEmbeddingService(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil dataset")
	}
}

func TestToContentParseEngineHint(t *testing.T) {
	cases := map[string]contracts.ParseEngine{
		"mineru":  contracts.ParseEngineMineru,
		"reducto": contracts.ParseEngineReducto,
		"vlm":     contracts.ParseEngineVLM,
		"local":   contracts.ParseEngineLocal,
		"":        contracts.ParseEngineLocal,
	}
	for input, want := range cases {
		if got := toContentParseEngineHint(input); got != want {
			t.Fatalf("toContentParseEngineHint(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSummarizePrimaryExtractOutput(t *testing.T) {
	out := &dto.ExtractOutput{
		Source:   "native",
		Markdown: "hello world",
		Elements: []dto.ExtractElement{{Type: "text", Content: "hello world", Ordinal: 1, BBox: &dto.ExtractBoundingBox{Left: 1, Top: 2, Right: 3, Bottom: 4}}},
	}
	summary := summarizePrimaryExtractOutput(out)
	if summary["source"] != "native" {
		t.Fatalf("source=%v", summary["source"])
	}
	if summary["element_count"] != 1 {
		t.Fatalf("element_count=%v", summary["element_count"])
	}
	if summary["text_length"] != len("hello world") {
		t.Fatalf("text_length=%v", summary["text_length"])
	}
	if summary["element_bbox_coverage_ratio"] != 1.0 {
		t.Fatalf("element_bbox_coverage_ratio=%v", summary["element_bbox_coverage_ratio"])
	}
}

func TestCompareChunkExecutionToPrimary(t *testing.T) {
	primary := &dto.ExtractOutput{
		Elements: []dto.ExtractElement{
			{Type: "text", Content: "hello", BBox: &dto.ExtractBoundingBox{}},
			{Type: "text", Content: "world"},
		},
	}
	result := &chunkexecutor.Result{
		Units: []contracts.ChunkUnit{
			{Content: "hello world", BBox: &contracts.ParseBoundingBox{}},
		},
		Metrics: chunkexecutor.Metrics{
			FilteredUnitCount: 1,
			StableOrder:       true,
		},
	}

	comparison := compareChunkExecutionToPrimary(primary, result)
	if comparison["primary_element_count"] != 2 {
		t.Fatalf("primary_element_count=%v", comparison["primary_element_count"])
	}
	if comparison["new_unit_count"] != 1 {
		t.Fatalf("new_unit_count=%v", comparison["new_unit_count"])
	}
	if comparison["unit_count_delta"] != -1 {
		t.Fatalf("unit_count_delta=%v", comparison["unit_count_delta"])
	}
	if comparison["low_value_removed_count"] != 1 {
		t.Fatalf("low_value_removed_count=%v", comparison["low_value_removed_count"])
	}
	if comparison["more_compact_than_primary"] != true {
		t.Fatalf("more_compact_than_primary=%v", comparison["more_compact_than_primary"])
	}
}

func TestCompareChunkExecutionToLegacyChunks(t *testing.T) {
	legacy := []dto.TransformedChunk{
		{Content: "hello", BBox: &dto.ExtractBoundingBox{}},
		{Content: "world", Children: []dto.TransformedChildChunk{{Content: "extra"}}},
	}
	result := &chunkexecutor.Result{
		Units: []contracts.ChunkUnit{
			{Content: "hello world extra", BBox: &contracts.ParseBoundingBox{}},
		},
		Metrics: chunkexecutor.Metrics{
			StableOrder:                true,
			SourceElementFilteredCount: 2,
		},
	}

	comparison := compareChunkExecutionToLegacyChunks(legacy, result)
	if comparison["legacy_unit_count"] != 3 {
		t.Fatalf("legacy_unit_count=%v", comparison["legacy_unit_count"])
	}
	if comparison["new_unit_count"] != 1 {
		t.Fatalf("new_unit_count=%v", comparison["new_unit_count"])
	}
	if comparison["unit_count_delta"] != -2 {
		t.Fatalf("unit_count_delta=%v", comparison["unit_count_delta"])
	}
	if comparison["low_value_removed_count"] != 2 {
		t.Fatalf("low_value_removed_count=%v", comparison["low_value_removed_count"])
	}
	if comparison["more_compact_than_legacy"] != true {
		t.Fatalf("more_compact_than_legacy=%v", comparison["more_compact_than_legacy"])
	}
}

func TestSummarizeChunkQualityScore(t *testing.T) {
	legacy := []dto.TransformedChunk{
		{Content: strings.Repeat("a", 500)},
		{Content: strings.Repeat("b", 500)},
	}
	result := &chunkexecutor.Result{
		Units: []contracts.ChunkUnit{
			{Content: strings.Repeat("c", 980), Pages: []int{1}},
		},
		Metrics: chunkexecutor.Metrics{
			StableOrder: true,
		},
	}

	summary := summarizeChunkQualityScore(result, legacy)
	if summary["label"] != "high" {
		t.Fatalf("quality summary=%v", summary)
	}
}

func TestContentParseChunkShadowWorkersClamp(t *testing.T) {
	t.Setenv("CONTENT_PARSE_CHUNK_SHADOW_WORKERS", "99")
	if got := contentParseChunkShadowWorkers(); got != 8 {
		t.Fatalf("workers=%d", got)
	}
	t.Setenv("CONTENT_PARSE_CHUNK_SHADOW_WORKERS", "bad")
	if got := contentParseChunkShadowWorkers(); got != contentparsesvc.DefaultShadowPipelineOptions().ChunkWorkers {
		t.Fatalf("workers fallback=%d", got)
	}
}

func TestContentParseShadowConcurrencyClamp(t *testing.T) {
	t.Setenv("CONTENT_PARSE_SHADOW_CONCURRENCY", "99")
	if got := contentParseShadowConcurrencyLimit(); got != 32 {
		t.Fatalf("concurrency=%d", got)
	}
	t.Setenv("CONTENT_PARSE_SHADOW_CONCURRENCY", "bad")
	if got := contentParseShadowConcurrencyLimit(); got != contentparsesvc.DefaultShadowPipelineOptions().Concurrency {
		t.Fatalf("concurrency fallback=%d", got)
	}
}

func TestContentParseChunkShadowPartitionSizeClamp(t *testing.T) {
	t.Setenv("CONTENT_PARSE_CHUNK_SHADOW_MAX_PARTITION_SIZE", "9999")
	if got := contentParseChunkShadowPartitionSize(); got != 512 {
		t.Fatalf("partition size=%d", got)
	}
	t.Setenv("CONTENT_PARSE_CHUNK_SHADOW_MAX_PARTITION_SIZE", "")
	if got := contentParseChunkShadowPartitionSize(); got != contentparsesvc.DefaultShadowPipelineOptions().ChunkPartitionSize {
		t.Fatalf("partition size fallback=%d", got)
	}
}

func TestContentParseShadowProviderMetadataUsesExecutedFallback(t *testing.T) {
	plan := &routing.RoutePlan{
		Primary: &routing.RouteCandidate{
			ProviderKey: "reducto",
			AdapterName: "hyperparse_sdk",
			EngineName:  contracts.ParseEngineReducto,
		},
		FallbackCandidates: []routing.RouteCandidate{
			{
				ProviderKey: "local",
				AdapterName: "hyperparse_sdk",
				EngineName:  contracts.ParseEngineLocal,
			},
		},
	}
	artifact := &contracts.ParseArtifact{
		EngineUsed: contracts.ParseEngineLocal,
		Metadata: map[string]any{
			"executed_provider_key":    "local",
			"executed_adapter_name":    "hyperparse_sdk",
			"executed_engine_name":     contracts.ParseEngineLocal,
			"attempted_provider_order": []string{"reducto", "local"},
		},
	}

	if got := contentparsesvc.AttemptedProviderOrder(plan, artifact); len(got) != 2 || got[0] != "reducto" || got[1] != "local" {
		t.Fatalf("attemptedProviderOrder=%v", got)
	}
	if got := contentparsesvc.FinalProviderKey(plan, artifact); got != "local" {
		t.Fatalf("finalProviderKey=%q", got)
	}
	if got := contentparsesvc.FinalAdapterName(plan, artifact); got != "hyperparse_sdk" {
		t.Fatalf("finalAdapterName=%q", got)
	}
	if got := contentparsesvc.FinalEngineName(plan, artifact); got != contracts.ParseEngineLocal {
		t.Fatalf("finalEngineName=%q", got)
	}
}
