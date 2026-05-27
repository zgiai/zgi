package indexing

import (
	"context"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
)

func TestParentChildTransformMergesShortLineParentChunksBeforeSplittingChildren(t *testing.T) {
	processor := &ParentChildIndexProcessor{
		BaseIndexProcessorImpl: NewBaseIndexProcessorImpl(nil, nil, nil, ""),
	}
	output := &dto.ExtractOutput{
		Elements: []dto.ExtractElement{
			{Type: "text", Content: "alpha", Ordinal: 0},
			{Type: "text", Content: "beta", Ordinal: 1},
			{Type: "text", Content: "gamma", Ordinal: 2},
		},
	}
	options := &ProcessOptions{
		ProcessRule: map[string]interface{}{
			"parent_mode": "parent_child",
			"segmentation": map[string]interface{}{
				"separator":     "\n",
				"max_tokens":    50,
				"chunk_overlap": 0,
			},
			"subchunk_segmentation": map[string]interface{}{
				"separator":     "\n",
				"max_tokens":    50,
				"chunk_overlap": 0,
			},
		},
	}

	got, err := processor.Transform(context.Background(), output, options)
	if err != nil {
		t.Fatalf("Transform returned error: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Content != "alpha\nbeta\ngamma" {
		t.Fatalf("parent content = %q", got[0].Content)
	}
	if len(got[0].Children) != 3 {
		t.Fatalf("child count = %d, want 3", len(got[0].Children))
	}
	if got[0].Children[0].Content != "alpha" || got[0].Children[1].Content != "beta" || got[0].Children[2].Content != "gamma" {
		t.Fatalf("children = %#v", got[0].Children)
	}
	if got[0].Metadata["child_count"] != 3 {
		t.Fatalf("metadata child_count = %v, want 3", got[0].Metadata["child_count"])
	}
}

func TestMergeSmallParentChunksCombinesAdjacentShortChunks(t *testing.T) {
	chunks := []dto.TransformedChunk{
		{
			Content: "line one",
			Metadata: map[string]any{
				"doc_id":       "old-parent-id",
				"doc_hash":     "old-parent-hash",
				"chunk_index":  0,
				"total_chunks": 3,
				"element_type": "text",
			},
		},
		{
			Content: "line two",
			Metadata: map[string]any{
				"element_type": "text",
			},
		},
		{
			Content: "line three",
			Metadata: map[string]any{
				"element_type": "text",
			},
		},
	}

	got := mergeSmallParentChunks(chunks, 32)

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Content != "line one\nline two\nline three" {
		t.Fatalf("content = %q", got[0].Content)
	}
	if _, ok := got[0].Metadata["doc_id"]; ok {
		t.Fatal("merged metadata should drop doc_id so addChunkMetadata can regenerate it")
	}
	if _, ok := got[0].Metadata["doc_hash"]; ok {
		t.Fatal("merged metadata should drop doc_hash so addChunkMetadata can regenerate it")
	}
	if _, ok := got[0].Metadata["chunk_index"]; ok {
		t.Fatal("merged metadata should drop stale chunk_index")
	}
}

func TestMergeSmallParentChunksFlushesNearTarget(t *testing.T) {
	chunks := []dto.TransformedChunk{
		{Content: "12345", Metadata: map[string]any{"element_type": "text"}},
		{Content: "67890", Metadata: map[string]any{"element_type": "text"}},
		{Content: "abcde", Metadata: map[string]any{"element_type": "text"}},
	}

	got := mergeSmallParentChunks(chunks, 11)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Content != "12345\n67890" {
		t.Fatalf("first content = %q", got[0].Content)
	}
	if got[1].Content != "abcde" {
		t.Fatalf("second content = %q", got[1].Content)
	}
}

func TestMergeSmallParentChunksKeepsOversizedChunkIntact(t *testing.T) {
	longContent := strings.Repeat("x", 20)
	chunks := []dto.TransformedChunk{
		{Content: longContent, Metadata: map[string]any{"element_type": "text"}},
		{Content: "short", Metadata: map[string]any{"element_type": "text"}},
		{Content: "tail", Metadata: map[string]any{"element_type": "text"}},
	}

	got := mergeSmallParentChunks(chunks, 10)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Content != longContent {
		t.Fatalf("first content length = %d, want %d", len([]rune(got[0].Content)), len([]rune(longContent)))
	}
	if got[1].Content != "short\ntail" {
		t.Fatalf("second content = %q", got[1].Content)
	}
}

func TestMergeSmallParentChunksDoesNotMergeStandaloneChunks(t *testing.T) {
	chunks := []dto.TransformedChunk{
		{Content: "before", Metadata: map[string]any{"element_type": "text"}},
		{Content: "| a | b |", Metadata: map[string]any{"element_type": "table"}},
		{Content: "after", Metadata: map[string]any{"element_type": "text"}},
		{Content: "tail", Metadata: map[string]any{"element_type": "text"}},
	}

	got := mergeSmallParentChunks(chunks, 100)

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[0].Content != "before" {
		t.Fatalf("first content = %q", got[0].Content)
	}
	if got[1].Content != "| a | b |" {
		t.Fatalf("standalone content = %q", got[1].Content)
	}
	if got[2].Content != "after\ntail" {
		t.Fatalf("tail content = %q", got[2].Content)
	}
}

func TestParentChunkMergeTarget(t *testing.T) {
	if got := parentChunkMergeTarget(&SegmentationRule{MaxTokens: 128}); got != 128 {
		t.Fatalf("target = %d, want 128", got)
	}
	if got := parentChunkMergeTarget(nil); got != defaultParentChunkMergeTarget {
		t.Fatalf("default target = %d, want %d", got, defaultParentChunkMergeTarget)
	}
}

func TestBuildSubchunkSeparatorsPreservesNewlineSeparator(t *testing.T) {
	fixedSeparator, separators := buildSubchunkSeparators("\n")

	if fixedSeparator != "\n" {
		t.Fatalf("fixedSeparator = %q, want newline", fixedSeparator)
	}
	if len(separators) == 0 || separators[0] != "\n" {
		t.Fatalf("first separator = %q, want newline", separators[0])
	}
}
