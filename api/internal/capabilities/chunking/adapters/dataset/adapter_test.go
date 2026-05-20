package dataset

import (
	"testing"

	"github.com/zgiai/ginext/internal/contracts"
)

func TestUnitsToTransformedChunks(t *testing.T) {
	chunks := UnitsToTransformedChunks([]contracts.ChunkUnit{
		{
			ChunkID:    "chunk-1",
			DocumentID: "doc-1",
			Kind:       contracts.ChunkKindText,
			Content:    "hello",
			Pages:      []int{1},
			Order:      3,
			BBox:       &contracts.ParseBoundingBox{Left: 1, Top: 2, Right: 3, Bottom: 4},
		},
	})

	if len(chunks) != 1 {
		t.Fatalf("chunks len = %d", len(chunks))
	}
	if chunks[0].Content != "hello" {
		t.Fatalf("content = %q", chunks[0].Content)
	}
	if chunks[0].BBox == nil || chunks[0].BBox.Right != 3 {
		t.Fatalf("bbox = %#v", chunks[0].BBox)
	}
	if chunks[0].Metadata["chunk_id"] != "chunk-1" {
		t.Fatalf("chunk_id = %#v", chunks[0].Metadata["chunk_id"])
	}
	if chunks[0].Metadata["document_id"] != "doc-1" {
		t.Fatalf("document_id = %#v", chunks[0].Metadata["document_id"])
	}
}

func TestUnitsToTransformedChunksGroupsExplicitChildren(t *testing.T) {
	chunks := UnitsToTransformedChunks([]contracts.ChunkUnit{
		{
			ChunkID:    "parent-1",
			DocumentID: "doc-1",
			Kind:       contracts.ChunkKindParent,
			Content:    "parent",
			Order:      1,
		},
		{
			ChunkID:       "child-1",
			ParentChunkID: "parent-1",
			DocumentID:    "doc-1",
			Kind:          contracts.ChunkKindChild,
			Content:       "child",
			Order:         2,
		},
	})

	if len(chunks) != 1 {
		t.Fatalf("chunks len = %d", len(chunks))
	}
	if len(chunks[0].Children) != 1 {
		t.Fatalf("children len = %d", len(chunks[0].Children))
	}
	if chunks[0].Metadata["is_parent"] != true {
		t.Fatalf("is_parent = %#v", chunks[0].Metadata["is_parent"])
	}
	if chunks[0].Children[0].Metadata["parent_id"] != "parent-1" {
		t.Fatalf("parent_id = %#v", chunks[0].Children[0].Metadata["parent_id"])
	}
}

func TestUnitsToTransformedChunksBuildsChildrenFromParentContent(t *testing.T) {
	chunks := UnitsToTransformedChunksWithOptions([]contracts.ChunkUnit{
		{
			ChunkID: "parent-1",
			Kind:    contracts.ChunkKindParent,
			Content: "first paragraph\n\nsecond paragraph",
			Order:   1,
		},
	}, AdapterOptions{
		BuildChildren:     true,
		SubchunkMaxTokens: 20,
		SubchunkOverlap:   0,
		SubchunkSeparator: "\n\n",
	})

	if len(chunks) != 1 {
		t.Fatalf("chunks len = %d", len(chunks))
	}
	if len(chunks[0].Children) != 2 {
		t.Fatalf("children len = %d, chunks=%#v", len(chunks[0].Children), chunks[0].Children)
	}
	if chunks[0].Children[0].Content != "first paragraph" {
		t.Fatalf("first child = %q", chunks[0].Children[0].Content)
	}
	if chunks[0].Children[1].Metadata["child_index"] != 1 {
		t.Fatalf("second child index = %#v", chunks[0].Children[1].Metadata["child_index"])
	}
}
