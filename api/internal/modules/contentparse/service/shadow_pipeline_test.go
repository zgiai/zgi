package service

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
)

func TestShadowPipelineOptionsFromEnvDefaults(t *testing.T) {
	t.Setenv("CONTENT_PARSE_SHADOW_CONCURRENCY", "")
	t.Setenv("CONTENT_PARSE_CHUNK_SHADOW_WORKERS", "")
	t.Setenv("CONTENT_PARSE_CHUNK_SHADOW_MAX_PARTITION_SIZE", "")

	got := ShadowPipelineOptionsFromEnv()
	if got.Concurrency != defaultShadowPipelineConcurrency {
		t.Fatalf("Concurrency = %d, want %d", got.Concurrency, defaultShadowPipelineConcurrency)
	}
	if got.ChunkWorkers != defaultShadowPipelineChunkWorkers {
		t.Fatalf("ChunkWorkers = %d, want %d", got.ChunkWorkers, defaultShadowPipelineChunkWorkers)
	}
	if got.ChunkPartitionSize != defaultShadowPipelineChunkPartitionSize {
		t.Fatalf("ChunkPartitionSize = %d, want %d", got.ChunkPartitionSize, defaultShadowPipelineChunkPartitionSize)
	}
}

func TestShadowPipelineOptionsFromEnvBounds(t *testing.T) {
	t.Setenv("CONTENT_PARSE_SHADOW_CONCURRENCY", "99")
	t.Setenv("CONTENT_PARSE_CHUNK_SHADOW_WORKERS", "99")
	t.Setenv("CONTENT_PARSE_CHUNK_SHADOW_MAX_PARTITION_SIZE", "9999")

	got := ShadowPipelineOptionsFromEnv()
	if got.Concurrency != maxShadowPipelineConcurrency {
		t.Fatalf("Concurrency = %d, want %d", got.Concurrency, maxShadowPipelineConcurrency)
	}
	if got.ChunkWorkers != maxShadowPipelineChunkWorkers {
		t.Fatalf("ChunkWorkers = %d, want %d", got.ChunkWorkers, maxShadowPipelineChunkWorkers)
	}
	if got.ChunkPartitionSize != maxShadowPipelineChunkPartitionSize {
		t.Fatalf("ChunkPartitionSize = %d, want %d", got.ChunkPartitionSize, maxShadowPipelineChunkPartitionSize)
	}
}

func TestShadowPipelineOptionsFromEnvInvalid(t *testing.T) {
	t.Setenv("CONTENT_PARSE_SHADOW_CONCURRENCY", "invalid")
	t.Setenv("CONTENT_PARSE_CHUNK_SHADOW_WORKERS", "invalid")
	t.Setenv("CONTENT_PARSE_CHUNK_SHADOW_MAX_PARTITION_SIZE", "0")

	got := ShadowPipelineOptionsFromEnv()
	if got.Concurrency != defaultShadowPipelineConcurrency {
		t.Fatalf("Concurrency = %d, want %d", got.Concurrency, defaultShadowPipelineConcurrency)
	}
	if got.ChunkWorkers != defaultShadowPipelineChunkWorkers {
		t.Fatalf("ChunkWorkers = %d, want %d", got.ChunkWorkers, defaultShadowPipelineChunkWorkers)
	}
	if got.ChunkPartitionSize != defaultShadowPipelineChunkPartitionSize {
		t.Fatalf("ChunkPartitionSize = %d, want %d", got.ChunkPartitionSize, defaultShadowPipelineChunkPartitionSize)
	}
}

func TestNewDatasetShadowSummary(t *testing.T) {
	input := DatasetShadowInput{
		InitialSummary: map[string]interface{}{
			"captured_at": int64(10),
			"existing":    "value",
		},
	}

	summary := NewDatasetShadowSummary(input, 99)
	if summary["enabled"] != true {
		t.Fatalf("enabled=%v", summary["enabled"])
	}
	if summary["captured_at"] != int64(10) {
		t.Fatalf("captured_at=%v", summary["captured_at"])
	}
	if summary["existing"] != "value" {
		t.Fatalf("existing=%v", summary["existing"])
	}
	summary["existing"] = "changed"
	if input.InitialSummary["existing"] != "value" {
		t.Fatal("summary must not mutate input InitialSummary")
	}

	withoutCaptured := NewDatasetShadowSummary(DatasetShadowInput{}, 123)
	if withoutCaptured["captured_at"] != int64(123) {
		t.Fatalf("captured_at fallback=%v", withoutCaptured["captured_at"])
	}
}

func TestApplyDatasetShadowSourceContext(t *testing.T) {
	summary, source := ApplyDatasetShadowSourceContext(
		map[string]interface{}{},
		DatasetShadowInput{SourceContentHash: " input-hash ", WorkspaceID: " input-workspace "},
		"file-hash",
		"file-workspace",
	)
	if summary["source_content_hash"] != "input-hash" {
		t.Fatalf("source_content_hash=%v", summary["source_content_hash"])
	}
	if source.SourceContentHash != "input-hash" || source.WorkspaceID != "input-workspace" {
		t.Fatalf("source=%+v", source)
	}

	existing := map[string]interface{}{"source_content_hash": "existing-hash"}
	summary, source = ApplyDatasetShadowSourceContext(existing, DatasetShadowInput{}, " file-hash ", " file-workspace ")
	if summary["source_content_hash"] != "existing-hash" {
		t.Fatalf("existing source_content_hash overwritten: %v", summary["source_content_hash"])
	}
	if source.SourceContentHash != "file-hash" || source.WorkspaceID != "file-workspace" {
		t.Fatalf("fallback source=%+v", source)
	}
}

func TestRoutePlanSummary(t *testing.T) {
	plan := &routing.RoutePlan{
		Mode:            contracts.ParseProfileDatasetIndex,
		RequestedEngine: contracts.ParseEngineMineru,
		Primary: &routing.RouteCandidate{
			ProviderKey: "mineru-primary",
			AdapterName: "remote",
			EngineName:  contracts.ParseEngineMineru,
			Reason:      map[string]any{"priority": "high"},
		},
		FallbackCandidates: []routing.RouteCandidate{
			{ProviderKey: "local-fallback"},
			{ProviderKey: " "},
			{ProviderKey: "vlm-fallback"},
		},
		Metadata: map[string]any{"profile": "dataset"},
	}

	summary := RoutePlanSummary(plan)
	if summary["mode"] != string(contracts.ParseProfileDatasetIndex) {
		t.Fatalf("mode=%v", summary["mode"])
	}
	if summary["primary_provider"] != "mineru-primary" {
		t.Fatalf("primary_provider=%v", summary["primary_provider"])
	}
	planned, ok := summary["planned_providers"].([]string)
	if !ok || len(planned) != 3 || planned[0] != "mineru-primary" || planned[1] != "local-fallback" || planned[2] != "vlm-fallback" {
		t.Fatalf("planned_providers=%v", summary["planned_providers"])
	}
	fallback, ok := summary["fallback_providers"].([]string)
	if !ok || len(fallback) != 2 || fallback[0] != "local-fallback" || fallback[1] != "vlm-fallback" {
		t.Fatalf("fallback_providers=%v", summary["fallback_providers"])
	}
	metadata := summary["metadata"].(map[string]interface{})
	metadata["profile"] = "changed"
	if plan.Metadata["profile"] != "dataset" {
		t.Fatal("summary metadata must not mutate route plan metadata")
	}
}

func TestNewDatasetShadowParseRequest(t *testing.T) {
	input := DatasetShadowInput{
		DocumentID:        "doc-1",
		DatasetID:         "dataset-1",
		OrganizationID:    "org-1",
		FileID:            "file-1",
		FileName:          "a.pdf",
		EngineHint:        contracts.ParseEngineLocal,
		RecognitionSource: "native",
		Source:            "upload",
	}

	request := NewDatasetShadowParseRequest(input)
	if request.SourceType != contracts.ParseSourceTypeBytes {
		t.Fatalf("SourceType=%q", request.SourceType)
	}
	if request.Intent != contracts.ParseIntentDatasetIndex {
		t.Fatalf("Intent=%q", request.Intent)
	}
	if request.Profile != contracts.ParseProfileDatasetIndex {
		t.Fatalf("Profile=%q", request.Profile)
	}
	if request.EngineHint != contracts.ParseEngineLocal {
		t.Fatalf("EngineHint=%q", request.EngineHint)
	}
	if request.Metadata["document_id"] != "doc-1" || request.Metadata["recognition_source"] != "native" {
		t.Fatalf("metadata=%v", request.Metadata)
	}
}

func TestApplyDatasetShadowArtifactSummary(t *testing.T) {
	summary := map[string]interface{}{"enabled": true}
	artifact := &contracts.ParseArtifact{
		ArtifactID:   "artifact-1",
		Status:       contracts.ParseStatusSucceeded,
		QualityLevel: contracts.ParseQualityHigh,
		EngineUsed:   contracts.ParseEngineLocal,
		FallbackUsed: true,
		Text:         "hello",
		Markdown:     "# hello",
		Elements:     []contracts.ParsedElement{{ID: "el-1"}},
		Metadata:     map[string]any{"page_count": 1},
		Diagnostics:  map[string]any{"recognition_source": "native"},
	}

	got := ApplyDatasetShadowArtifactSummary(summary, artifact)
	if got["status"] != contracts.ParseStatusSucceeded {
		t.Fatalf("status=%v", got["status"])
	}
	if got["text_length"] != 5 || got["markdown_length"] != 7 || got["element_count"] != 1 {
		t.Fatalf("length summary=%v", got)
	}
	if got["artifact_id"] != "artifact-1" {
		t.Fatalf("artifact_id=%v", got["artifact_id"])
	}
	diagnostics, ok := got["diagnostics"].(map[string]interface{})
	if !ok || diagnostics["recognition_source"] != "native" {
		t.Fatalf("diagnostics=%v", got["diagnostics"])
	}
}

func TestApplyDatasetShadowFailure(t *testing.T) {
	summary := ApplyDatasetShadowFailure(map[string]interface{}{"enabled": true}, assertErr("download file: denied"))
	if summary["status"] != "failed" {
		t.Fatalf("status=%v", summary["status"])
	}
	if summary["error"] != "download file: denied" {
		t.Fatalf("error=%v", summary["error"])
	}

	withoutErr := ApplyDatasetShadowFailure(nil, nil)
	if withoutErr["status"] != "failed" {
		t.Fatalf("nil error status=%v", withoutErr["status"])
	}
	if _, exists := withoutErr["error"]; exists {
		t.Fatalf("nil error should not set error field: %v", withoutErr)
	}
}

func TestNewDatasetShadowResult(t *testing.T) {
	summary := map[string]interface{}{
		"chunking_run_id":       "chunking-run-1",
		"chunk_artifact_set_id": "chunk-artifact-set-1",
	}
	result := NewDatasetShadowResult(summary, " parse-run-1 ", " artifact-1 ")
	if result.Summary["chunking_run_id"] != "chunking-run-1" {
		t.Fatal("expected summary to be attached")
	}
	if result.ParseRunID != "parse-run-1" {
		t.Fatalf("ParseRunID=%q", result.ParseRunID)
	}
	if result.ParseArtifactID != "artifact-1" {
		t.Fatalf("ParseArtifactID=%q", result.ParseArtifactID)
	}
	if result.ChunkingRunID != "chunking-run-1" || result.ChunkArtifactSetID != "chunk-artifact-set-1" {
		t.Fatalf("chunk ids=%q/%q", result.ChunkingRunID, result.ChunkArtifactSetID)
	}
}

type assertErr string

func (e assertErr) Error() string {
	return string(e)
}
