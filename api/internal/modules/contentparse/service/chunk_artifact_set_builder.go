package service

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
)

const (
	DefaultChunkArtifactPlannerName    = "default_planner_parallel_executor"
	DefaultChunkArtifactChunkerVersion = "chunk_executor_v1"
)

type ChunkArtifactSetBuildInput struct {
	ParseRunID        uuid.UUID
	ParseArtifactID   *uuid.UUID
	Artifact          *contracts.ParseArtifact
	Plan              *contracts.ChunkPlan
	Units             []contracts.ChunkUnit
	ChunkingSummary   map[string]interface{}
	SourceContentHash string
}

type ChunkArtifactSetBuildResult struct {
	Item              *model.ChunkArtifactSet
	SourceContentHash string
	ContentHash       string
	Signature         string
}

func BuildChunkArtifactSetItem(input ChunkArtifactSetBuildInput) ChunkArtifactSetBuildResult {
	if input.Artifact == nil || input.Plan == nil {
		return ChunkArtifactSetBuildResult{}
	}
	sourceHash := input.SourceContentHash
	if sourceHash == "" {
		sourceHash = ArtifactMetadataString(input.Artifact, "source_content_hash")
	}
	if sourceHash == "" {
		sourceHash = SHA256Hex(input.Artifact.Text + "\n" + input.Artifact.Markdown)
	}
	contentHash := ChunkUnitsContentHash(input.Units)
	signature := ChunkArtifactSignature(sourceHash, *input.Plan, DefaultChunkArtifactPlannerName, DefaultChunkArtifactChunkerVersion, contentHash)
	quality, _ := input.ChunkingSummary["quality_score"].(map[string]interface{})
	if quality == nil {
		quality = map[string]interface{}{}
	}
	return ChunkArtifactSetBuildResult{
		SourceContentHash: sourceHash,
		ContentHash:       contentHash,
		Signature:         signature,
		Item: &model.ChunkArtifactSet{
			ParseArtifactID:    input.ParseArtifactID,
			ParseRunID:         &input.ParseRunID,
			SourceContentHash:  sourceHash,
			UseCase:            string(input.Plan.UseCase),
			PlannerName:        DefaultChunkArtifactPlannerName,
			ParentMode:         input.Plan.ParentMode,
			Segmentation:       input.Plan.Segmentation,
			ChunkerVersion:     DefaultChunkArtifactChunkerVersion,
			Signature:          signature,
			Status:             "succeeded",
			UnitCount:          len(input.Units),
			ContentHash:        contentHash,
			QualityJSON:        quality,
			SummaryJSON:        input.ChunkingSummary,
			ArtifactStorageKey: "",
		},
	}
}

func ApplyChunkArtifactSetSummary(summary map[string]interface{}, chunkingSummary map[string]interface{}, id uuid.UUID) {
	if id == uuid.Nil {
		return
	}
	value := id.String()
	if chunkingSummary != nil {
		chunkingSummary["chunk_artifact_set_id"] = value
	}
	if summary != nil {
		summary["chunk_artifact_set_id"] = value
	}
}
