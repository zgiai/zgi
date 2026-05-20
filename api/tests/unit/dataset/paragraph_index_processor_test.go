package dataset_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/dataset/indexing"
)

func TestParagraphIndexProcessorTransformUsesElementBoundaries(t *testing.T) {
	processor := indexing.NewParagraphIndexProcessor(nil, nil, nil, "tenant-1")
	tableBox := &dto.ExtractBoundingBox{Left: 0.1, Top: 0.2, Right: 0.8, Bottom: 0.4}

	output := &dto.ExtractOutput{
		Source: "landingai:ade",
		Elements: []dto.ExtractElement{
			{Type: "heading", Content: "Overview", Ordinal: 1},
			{Type: "text", Content: "The document starts with text.", Ordinal: 2},
			{
				Type:    "table",
				Content: "| Name | Score |\n| --- | --- |\n| Alice | 90 |",
				BBox:    tableBox,
				Ordinal: 3,
				Metadata: map[string]any{
					"source": "sample.pdf",
				},
			},
			{Type: "figure", Content: "Figure: architecture diagram", Ordinal: 4},
			{Type: "text", Content: "The document ends with text.", Ordinal: 5},
		},
	}

	chunks, err := processor.Transform(context.Background(), output, &indexing.ProcessOptions{Mode: "automatic"})
	require.NoError(t, err)
	require.Len(t, chunks, 4)

	require.Contains(t, chunks[0].Content, "Overview")
	require.Contains(t, chunks[0].Content, "starts with text")
	require.Equal(t, "| Name | Score |\n| --- | --- |\n| Alice | 90 |", chunks[1].Content)
	require.NotNil(t, chunks[1].BBox)
	require.Equal(t, *tableBox, *chunks[1].BBox)
	require.Equal(t, "table", chunks[1].Metadata["element_type"])
	require.Equal(t, "sample.pdf", chunks[1].Metadata["source"])
	require.Equal(t, "landingai:ade", chunks[1].Metadata["provider"])
	require.Equal(t, "Figure: architecture diagram", chunks[2].Content)
	require.Contains(t, chunks[3].Content, "ends with text")

	for i, chunk := range chunks {
		require.Equal(t, i, chunk.Metadata["chunk_index"])
		require.Equal(t, len(chunks), chunk.Metadata["total_chunks"])
		require.NotEmpty(t, chunk.Metadata["doc_id"])
		require.NotEmpty(t, chunk.Metadata["doc_hash"])
	}
}
