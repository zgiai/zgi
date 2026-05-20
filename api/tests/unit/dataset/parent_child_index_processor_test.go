package dataset_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/modules/dataset/indexing"
)

func TestParentChildSubchunkSplitterUsesPunctuationFallbacks(t *testing.T) {
	processor := indexing.NewParentChildIndexProcessor(nil, nil, nil, nil, "tenant-1")
	output := &dto.ExtractOutput{
		Source: "landingai:ade",
		Elements: []dto.ExtractElement{
			{
				Type:    "text",
				Content: "第一句内容很多。第二句内容很多。第三句内容很多。",
				Metadata: map[string]any{
					"source": "sample.pdf",
				},
			},
		},
	}

	chunks, err := processor.Transform(context.Background(), output, &indexing.ProcessOptions{
		ProcessRule: map[string]interface{}{
			"parent_mode": "full-doc",
			"subchunk_segmentation": map[string]interface{}{
				"separator":     "\n",
				"max_tokens":    12,
				"chunk_overlap": 0,
			},
		},
	})

	require.NoError(t, err)
	require.Len(t, chunks, 1)
	require.Len(t, chunks[0].Children, 3)
	require.Equal(t, "第一句内容很多", chunks[0].Children[0].Content)
	require.Equal(t, "第二句内容很多", chunks[0].Children[1].Content)
	require.Equal(t, "第三句内容很多。", chunks[0].Children[2].Content)
}

func TestParentChildTransformParagraphUsesExtractElementParents(t *testing.T) {
	processor := indexing.NewParentChildIndexProcessor(nil, nil, nil, nil, "tenant-1")
	textBox := &dto.ExtractBoundingBox{Left: 0.1, Top: 0.1, Right: 0.9, Bottom: 0.2}
	tableBox := &dto.ExtractBoundingBox{Left: 0.2, Top: 0.3, Right: 0.8, Bottom: 0.6}

	output := &dto.ExtractOutput{
		Source: "landingai:ade",
		Metadata: map[string]any{
			"source":   "sample.pdf",
			"children": []dto.Document{{PageContent: "legacy child must be ignored"}},
		},
		Elements: []dto.ExtractElement{
			{
				Type:    "heading",
				Content: "Overview",
				BBox:    textBox,
				Ordinal: 1,
				Metadata: map[string]any{
					"children": []dto.Document{{PageContent: "legacy heading child must be ignored"}},
				},
			},
			{Type: "text", Content: "Intro paragraph.", Ordinal: 2},
			{
				Type:    "table",
				Content: "| Name | Score |\n| --- | --- |\n| Alice | 90 |",
				BBox:    tableBox,
				Ordinal: 3,
				Metadata: map[string]any{
					"source": "sample.pdf",
				},
			},
			{Type: "text", Content: "Closing paragraph.", Ordinal: 4},
		},
	}

	chunks, err := processor.Transform(context.Background(), output, &indexing.ProcessOptions{
		ProcessRule: map[string]interface{}{
			"parent_mode": "paragraph",
			"segmentation": map[string]interface{}{
				"separator":     "\n\n",
				"max_tokens":    500,
				"chunk_overlap": 0,
			},
			"subchunk_segmentation": map[string]interface{}{
				"separator":     "\n\n",
				"max_tokens":    500,
				"chunk_overlap": 0,
			},
		},
	})

	require.NoError(t, err)
	require.Len(t, chunks, 3)

	require.Contains(t, chunks[0].Content, "Overview")
	require.Contains(t, chunks[0].Content, "Intro paragraph.")
	require.NotNil(t, chunks[0].BBox)
	require.Equal(t, *textBox, *chunks[0].BBox)
	require.NotContains(t, chunks[0].Metadata, "children")
	require.Len(t, chunks[0].Children, 1)
	require.NotContains(t, chunks[0].Children[0].Metadata, "children")

	require.Equal(t, "| Name | Score |\n| --- | --- |\n| Alice | 90 |", chunks[1].Content)
	require.NotNil(t, chunks[1].BBox)
	require.Equal(t, *tableBox, *chunks[1].BBox)
	require.Equal(t, "table", chunks[1].Metadata["element_type"])
	require.Equal(t, true, chunks[1].Metadata["is_parent"])
	require.Equal(t, 1, chunks[1].Metadata["child_count"])
	require.Len(t, chunks[1].Children, 1)
	require.Equal(t, chunks[1].Content, chunks[1].Children[0].Content)

	parentID, ok := chunks[1].Metadata["doc_id"].(string)
	require.True(t, ok)
	require.Equal(t, parentID, chunks[1].Children[0].Metadata["parent_id"])
	require.Equal(t, true, chunks[1].Children[0].Metadata["is_child"])
	require.Equal(t, 0, chunks[1].Children[0].Metadata["child_index"])
	require.NotEmpty(t, chunks[1].Children[0].Metadata["doc_id"])
	require.NotEmpty(t, chunks[1].Children[0].Metadata["doc_hash"])

	require.Equal(t, "Closing paragraph.", chunks[2].Content)
	require.Len(t, chunks[2].Children, 1)
}
