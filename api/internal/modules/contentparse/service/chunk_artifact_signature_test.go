package service

import (
	"testing"

	"github.com/zgiai/ginext/internal/contracts"
)

func TestChunkUnitsContentHashIsStable(t *testing.T) {
	units := []contracts.ChunkUnit{
		{ChunkID: "b", Kind: contracts.ChunkKindText, Order: 2, Content: " second "},
		{ChunkID: "a", Kind: contracts.ChunkKindText, Order: 1, Content: "first"},
	}

	first := ChunkUnitsContentHash(units)
	second := ChunkUnitsContentHash(units)
	if first == "" {
		t.Fatal("expected content hash")
	}
	if first != second {
		t.Fatalf("content hash not stable: %s != %s", first, second)
	}
}

func TestChunkUnitsContentHashIncludesIdentityAndOrder(t *testing.T) {
	base := []contracts.ChunkUnit{
		{ChunkID: "a", Kind: contracts.ChunkKindText, Order: 1, Content: "same"},
	}
	reordered := []contracts.ChunkUnit{
		{ChunkID: "a", Kind: contracts.ChunkKindText, Order: 2, Content: "same"},
	}

	if ChunkUnitsContentHash(base) == ChunkUnitsContentHash(reordered) {
		t.Fatal("expected different hashes when chunk order changes")
	}
}

func TestChunkArtifactSignatureIncludesStrategy(t *testing.T) {
	plan := contracts.ChunkPlan{
		UseCase:      contracts.ChunkUseCaseDatasetIndex,
		ParentMode:   "document",
		Segmentation: "semantic",
	}
	sigA := ChunkArtifactSignature("source", plan, "planner", "v1", "content")
	plan.Segmentation = "page"
	sigB := ChunkArtifactSignature("source", plan, "planner", "v1", "content")

	if sigA == "" || sigB == "" {
		t.Fatal("expected signatures")
	}
	if sigA == sigB {
		t.Fatal("expected different signatures when chunk strategy changes")
	}
}
