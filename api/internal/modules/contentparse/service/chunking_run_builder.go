package service

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
)

type ChunkingRunBuildInput struct {
	ParseRunID         uuid.UUID
	ChunkArtifactSetID *uuid.UUID
	Plan               *contracts.ChunkPlan
	UnitCount          int
	PlanJSON           map[string]interface{}
}

func BuildChunkingRun(input ChunkingRunBuildInput) *model.ChunkingRun {
	if input.Plan == nil {
		return nil
	}
	return &model.ChunkingRun{
		ParseRunID:         input.ParseRunID,
		ChunkArtifactSetID: input.ChunkArtifactSetID,
		UseCase:            string(input.Plan.UseCase),
		PlannerName:        DefaultChunkArtifactPlannerName,
		ParentMode:         input.Plan.ParentMode,
		Segmentation:       input.Plan.Segmentation,
		UnitCount:          input.UnitCount,
		PlanJSON:           input.PlanJSON,
		ArtifactStorageKey: "",
	}
}
