package executor

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/contracts"
)

func TestExecutorRunsPartitionsAndStableMerges(t *testing.T) {
	doc := &contracts.ChunkSourceDocument{
		DocumentID: "doc-1",
		DatasetID:  "dataset-1",
		Elements: []contracts.ChunkSourceElement{
			{ElementID: "h1", Type: "heading", Content: "Intro", Ordinal: 1},
			{ElementID: "p1", Type: "text", Content: "Body 1", Ordinal: 2},
			{ElementID: "h2", Type: "heading", Content: "Details", Ordinal: 3},
			{ElementID: "p2", Type: "text", Content: "Body 2", Ordinal: 4},
		},
	}
	plan := &contracts.ChunkPlan{
		UseCase:      contracts.ChunkUseCaseDatasetIndex,
		ParentMode:   "section",
		Segmentation: "section_aware",
	}
	exec := New(WithLimits(Limits{MaxWorkers: 4, MaxPartitionSize: 64}))

	result, err := exec.Execute(context.Background(), doc, plan)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.Partitions) != 2 {
		t.Fatalf("partitions = %d", len(result.Partitions))
	}
	if len(result.Units) != 2 {
		t.Fatalf("units = %d", len(result.Units))
	}
	if result.Units[0].Content != "Intro\nBody 1" {
		t.Fatalf("first unit content = %q", result.Units[0].Content)
	}
	if result.Units[1].Content != "Details\nBody 2" {
		t.Fatalf("second unit content = %q", result.Units[1].Content)
	}
	if !result.Metrics.StableOrder {
		t.Fatal("expected stable order")
	}
	if result.Metrics.WorkerCount != 2 {
		t.Fatalf("worker count = %d", result.Metrics.WorkerCount)
	}
}

func TestExecutorFiltersLowValueUnits(t *testing.T) {
	doc := &contracts.ChunkSourceDocument{
		DocumentID: "doc-1",
		Elements: []contracts.ChunkSourceElement{
			{ElementID: "p1", Type: "text", Content: "Page 1 of 8", Page: 1, Ordinal: 1},
			{ElementID: "p2", Type: "text", Content: "Useful content", Page: 2, Ordinal: 2},
		},
	}
	plan := &contracts.ChunkPlan{Segmentation: "page_layout_aware"}

	result, err := New(WithLimits(Limits{MaxWorkers: 2})).Execute(context.Background(), doc, plan)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.Units) != 1 {
		t.Fatalf("units = %d", len(result.Units))
	}
	if result.Units[0].Content != "Useful content" {
		t.Fatalf("remaining content = %q", result.Units[0].Content)
	}
	if result.Metrics.FilteredUnitCount != 0 {
		t.Fatalf("unit filtered = %d", result.Metrics.FilteredUnitCount)
	}
	if result.Metrics.SourceElementFilteredCount != 1 {
		t.Fatalf("source filtered = %d", result.Metrics.SourceElementFilteredCount)
	}
}

func TestExecutorFiltersLowValueSourceElementsInsidePartition(t *testing.T) {
	doc := &contracts.ChunkSourceDocument{
		DocumentID: "doc-1",
		Elements: []contracts.ChunkSourceElement{
			{ElementID: "p1", Type: "text", Content: "Page 1 of 8", Ordinal: 1},
			{ElementID: "p2", Type: "text", Content: "Useful body", Ordinal: 2},
		},
	}
	plan := &contracts.ChunkPlan{Segmentation: "section_aware"}

	result, err := New(WithLimits(Limits{MaxWorkers: 2})).Execute(context.Background(), doc, plan)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.Units) != 1 {
		t.Fatalf("units = %d", len(result.Units))
	}
	if result.Units[0].Content != "Useful body" {
		t.Fatalf("remaining content = %q", result.Units[0].Content)
	}
	if result.Metrics.SourceElementFilteredCount != 1 {
		t.Fatalf("source filtered = %d", result.Metrics.SourceElementFilteredCount)
	}
	if result.Metrics.SourceElementFilterReasons["page_counter"] != 1 {
		t.Fatalf("source reasons = %#v", result.Metrics.SourceElementFilterReasons)
	}
}

func TestExecutorBBoxAndPagesUseFilteredSourceElements(t *testing.T) {
	doc := &contracts.ChunkSourceDocument{
		DocumentID: "doc-1",
		Elements: []contracts.ChunkSourceElement{
			{
				ElementID: "noise",
				Type:      "text",
				Content:   "Page 1 of 8",
				Page:      1,
				Ordinal:   1,
				BBox:      &contracts.ParseBoundingBox{Left: 0, Top: 0, Right: 1, Bottom: 1},
			},
			{
				ElementID: "body",
				Type:      "text",
				Content:   "Useful body",
				Page:      2,
				Ordinal:   2,
				BBox:      &contracts.ParseBoundingBox{Left: 0.2, Top: 0.3, Right: 0.4, Bottom: 0.5},
			},
		},
	}
	plan := &contracts.ChunkPlan{Segmentation: "section_aware"}

	result, err := New(WithLimits(Limits{MaxWorkers: 2})).Execute(context.Background(), doc, plan)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.Units) != 1 {
		t.Fatalf("units = %d", len(result.Units))
	}
	if len(result.Units[0].Pages) != 1 || result.Units[0].Pages[0] != 2 {
		t.Fatalf("pages = %#v", result.Units[0].Pages)
	}
	box := result.Units[0].BBox
	if box == nil || box.Left != 0.2 || box.Right != 0.4 {
		t.Fatalf("bbox = %#v", box)
	}
}

func TestExecutorUsesUnionBBoxForChunkUnit(t *testing.T) {
	doc := &contracts.ChunkSourceDocument{
		DocumentID: "doc-1",
		Elements: []contracts.ChunkSourceElement{
			{ElementID: "a", Type: "text", Content: "Left", Ordinal: 1, BBox: &contracts.ParseBoundingBox{Left: 0.1, Top: 0.2, Right: 0.2, Bottom: 0.22}},
			{ElementID: "b", Type: "text", Content: "Right", Ordinal: 2, BBox: &contracts.ParseBoundingBox{Left: 0.5, Top: 0.24, Right: 0.8, Bottom: 0.27}},
		},
	}
	plan := &contracts.ChunkPlan{Segmentation: "section_aware"}

	result, err := New(WithLimits(Limits{MaxWorkers: 2})).Execute(context.Background(), doc, plan)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.Units) != 1 {
		t.Fatalf("units = %d", len(result.Units))
	}
	box := result.Units[0].BBox
	if box == nil {
		t.Fatal("expected bbox")
	}
	if box.Left != 0.1 || box.Top != 0.2 || box.Right != 0.8 || box.Bottom != 0.27 {
		t.Fatalf("bbox = %+v", box)
	}
}

func TestDefaultPartitionerTableAware(t *testing.T) {
	partitions, err := NewDefaultPartitioner().Partition(&contracts.ChunkSourceDocument{
		Elements: []contracts.ChunkSourceElement{
			{Type: "text", Content: "before", Ordinal: 1},
			{Type: "table", Content: "| a |", Ordinal: 2},
			{Type: "text", Content: "after", Ordinal: 3},
		},
	}, &contracts.ChunkPlan{Segmentation: "table_aware"}, Limits{MaxPartitionSize: 64})
	if err != nil {
		t.Fatalf("Partition() error = %v", err)
	}
	if len(partitions) != 3 {
		t.Fatalf("partitions = %d", len(partitions))
	}
	if partitions[1].Kind != PartitionKindTable {
		t.Fatalf("middle partition kind = %q", partitions[1].Kind)
	}
}
