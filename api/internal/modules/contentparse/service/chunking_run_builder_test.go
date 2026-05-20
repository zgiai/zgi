package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/contracts"
)

func TestBuildChunkingRun(t *testing.T) {
	parseRunID := uuid.New()
	chunkArtifactSetID := uuid.New()
	plan := &contracts.ChunkPlan{
		UseCase:      contracts.ChunkUseCaseDatasetIndex,
		ParentMode:   "paragraph",
		Segmentation: "semantic",
	}
	planJSON := map[string]interface{}{"unit_count": 3}

	run := BuildChunkingRun(ChunkingRunBuildInput{
		ParseRunID:         parseRunID,
		ChunkArtifactSetID: &chunkArtifactSetID,
		Plan:               plan,
		UnitCount:          3,
		PlanJSON:           planJSON,
	})
	if run == nil {
		t.Fatal("expected chunking run")
	}
	if run.ParseRunID != parseRunID || run.ChunkArtifactSetID == nil || *run.ChunkArtifactSetID != chunkArtifactSetID {
		t.Fatalf("ids not assigned: %+v", run)
	}
	if run.UseCase != string(contracts.ChunkUseCaseDatasetIndex) {
		t.Fatalf("UseCase=%q", run.UseCase)
	}
	if run.PlannerName != DefaultChunkArtifactPlannerName {
		t.Fatalf("PlannerName=%q", run.PlannerName)
	}
	if run.ParentMode != "paragraph" || run.Segmentation != "semantic" || run.UnitCount != 3 {
		t.Fatalf("strategy/count=%q/%q/%d", run.ParentMode, run.Segmentation, run.UnitCount)
	}
	if run.PlanJSON["unit_count"] != 3 {
		t.Fatalf("PlanJSON=%v", run.PlanJSON)
	}
}

func TestBuildChunkingRunNilPlan(t *testing.T) {
	if got := BuildChunkingRun(ChunkingRunBuildInput{}); got != nil {
		t.Fatalf("nil plan result=%+v", got)
	}
}
