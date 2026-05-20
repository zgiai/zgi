package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/contracts"
)

func TestBuildChunkArtifactSetItem(t *testing.T) {
	parseRunID := uuid.New()
	parseArtifactID := uuid.New()
	plan := &contracts.ChunkPlan{
		UseCase:      contracts.ChunkUseCaseDatasetIndex,
		ParentMode:   "paragraph",
		Segmentation: "semantic",
	}
	units := []contracts.ChunkUnit{{ChunkID: "u1", Kind: "parent", Order: 1, Content: "hello"}}
	summary := map[string]interface{}{
		"quality_score": map[string]interface{}{"label": "high"},
	}

	result := BuildChunkArtifactSetItem(ChunkArtifactSetBuildInput{
		ParseRunID:        parseRunID,
		ParseArtifactID:   &parseArtifactID,
		Artifact:          &contracts.ParseArtifact{Text: "hello"},
		Plan:              plan,
		Units:             units,
		ChunkingSummary:   summary,
		SourceContentHash: "source-hash",
	})

	if result.Item == nil {
		t.Fatal("expected chunk artifact item")
	}
	if result.SourceContentHash != "source-hash" || result.Item.SourceContentHash != "source-hash" {
		t.Fatalf("source hash=%q/%q", result.SourceContentHash, result.Item.SourceContentHash)
	}
	if result.ContentHash == "" || result.Signature == "" {
		t.Fatalf("content/signature empty: %+v", result)
	}
	if result.Item.ParseRunID == nil || *result.Item.ParseRunID != parseRunID || result.Item.ParseArtifactID == nil || *result.Item.ParseArtifactID != parseArtifactID {
		t.Fatalf("ids not assigned: %+v", result.Item)
	}
	if result.Item.PlannerName != DefaultChunkArtifactPlannerName || result.Item.ChunkerVersion != DefaultChunkArtifactChunkerVersion {
		t.Fatalf("planner/version=%q/%q", result.Item.PlannerName, result.Item.ChunkerVersion)
	}
	if result.Item.UnitCount != 1 {
		t.Fatalf("UnitCount=%d", result.Item.UnitCount)
	}
	if result.Item.QualityJSON["label"] != "high" {
		t.Fatalf("quality=%v", result.Item.QualityJSON)
	}
}

func TestBuildChunkArtifactSetItemSourceFallbacks(t *testing.T) {
	plan := &contracts.ChunkPlan{UseCase: contracts.ChunkUseCaseDatasetIndex}
	artifact := &contracts.ParseArtifact{
		Text:     "hello",
		Markdown: "world",
		Metadata: map[string]any{
			"source_content_hash": "artifact-hash",
		},
	}
	result := BuildChunkArtifactSetItem(ChunkArtifactSetBuildInput{
		ParseRunID: uuid.New(),
		Artifact:   artifact,
		Plan:       plan,
	})
	if result.SourceContentHash != "artifact-hash" {
		t.Fatalf("artifact source hash=%q", result.SourceContentHash)
	}

	artifact.Metadata = nil
	result = BuildChunkArtifactSetItem(ChunkArtifactSetBuildInput{
		ParseRunID: uuid.New(),
		Artifact:   artifact,
		Plan:       plan,
	})
	if result.SourceContentHash != SHA256Hex("hello\nworld") {
		t.Fatalf("hashed source=%q", result.SourceContentHash)
	}
}

func TestBuildChunkArtifactSetItemNilInputs(t *testing.T) {
	if got := BuildChunkArtifactSetItem(ChunkArtifactSetBuildInput{}); got.Item != nil || got.Signature != "" {
		t.Fatalf("nil result=%+v", got)
	}
}

func TestApplyChunkArtifactSetSummary(t *testing.T) {
	id := uuid.New()
	summary := map[string]interface{}{}
	chunkingSummary := map[string]interface{}{}
	ApplyChunkArtifactSetSummary(summary, chunkingSummary, id)
	if summary["chunk_artifact_set_id"] != id.String() {
		t.Fatalf("summary id=%v", summary["chunk_artifact_set_id"])
	}
	if chunkingSummary["chunk_artifact_set_id"] != id.String() {
		t.Fatalf("chunking summary id=%v", chunkingSummary["chunk_artifact_set_id"])
	}

	ApplyChunkArtifactSetSummary(summary, chunkingSummary, uuid.Nil)
	if summary["chunk_artifact_set_id"] != id.String() {
		t.Fatalf("nil id should not overwrite: %v", summary)
	}
}
