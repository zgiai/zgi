package indexing

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
)

func TestTableTransformCreatesMirrorChildChunk(t *testing.T) {
	processor := &TableIndexProcessor{}
	bbox := &dto.ExtractBoundingBox{Left: 1, Top: 2, Right: 3, Bottom: 4}
	output := &dto.ExtractOutput{
		Elements: []dto.ExtractElement{
			{
				Type:    "table",
				Content: "header | value",
				BBox:    bbox,
				Metadata: map[string]any{
					"source": "sheet",
				},
			},
		},
	}

	chunks, err := processor.Transform(context.Background(), output, nil)
	if err != nil {
		t.Fatalf("Transform returned error: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	if len(chunks[0].Children) != 1 {
		t.Fatalf("children = %d, want 1", len(chunks[0].Children))
	}
	child := chunks[0].Children[0]
	if child.Content != chunks[0].Content {
		t.Fatalf("child content = %q, want %q", child.Content, chunks[0].Content)
	}
	if child.BBox != bbox {
		t.Fatalf("child bbox pointer = %p, want %p", child.BBox, bbox)
	}
	if child.Metadata["source"] != "sheet" {
		t.Fatalf("child metadata source = %v", child.Metadata["source"])
	}
	child.Metadata["source"] = "changed"
	if chunks[0].Metadata["source"] != "sheet" {
		t.Fatalf("parent metadata was mutated: %v", chunks[0].Metadata["source"])
	}
}

func TestTableBuildIndexingItemsUsesChildVector(t *testing.T) {
	processor := &TableIndexProcessor{}
	dataset := &model.Dataset{ID: "dataset-1"}
	chunks := []dto.TransformedChunk{
		{
			Content: "table row",
			Metadata: map[string]any{
				"doc_id":      "parent-node-1",
				"doc_hash":    "parent-hash",
				"document_id": "document-1",
			},
			Children: []dto.TransformedChildChunk{
				{
					Content: "table row",
					Metadata: map[string]any{
						"doc_id":   "child-node-1",
						"doc_hash": "child-hash",
					},
				},
			},
		},
	}

	items, err := processor.buildIndexingItems(dataset, chunks)
	if err != nil {
		t.Fatalf("buildIndexingItems returned error: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	item := items[0]
	if item.ItemType != indexingItemTypeChild {
		t.Fatalf("item type = %q, want %q", item.ItemType, indexingItemTypeChild)
	}
	if item.IndexNodeID != "child-node-1" {
		t.Fatalf("index node id = %q, want child-node-1", item.IndexNodeID)
	}
	if item.ParentIndexNodeID != "parent-node-1" {
		t.Fatalf("parent index node id = %q, want parent-node-1", item.ParentIndexNodeID)
	}
	if item.Properties["doc_id"] != "child-node-1" {
		t.Fatalf("doc_id property = %v, want child-node-1", item.Properties["doc_id"])
	}
	if item.Properties["text"] != "table row" {
		t.Fatalf("text property = %v, want table row", item.Properties["text"])
	}
}
