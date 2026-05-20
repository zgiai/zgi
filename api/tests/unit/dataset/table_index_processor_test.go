package dataset_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/modules/dataset/indexing"
)

func TestTableIndexProcessorTransform(t *testing.T) {
	processor := indexing.NewTableIndexProcessor(nil, nil, nil, "tenant-1")
	bbox := &dto.ExtractBoundingBox{Left: 0.1, Top: 0.2, Right: 0.8, Bottom: 0.6}

	output := &dto.ExtractOutput{
		Source: "zgi:excel",
		Elements: []dto.ExtractElement{
			{
				Type:    "table",
				Content: `  "Name":"Alice";"Score":"90"  `,
				BBox:    bbox,
				Metadata: map[string]any{
					"doc_id":   "table-row-1",
					"doc_hash": "table-row-hash-1",
					"sheet":    "Sheet1",
					"source":   "test.xlsx",
				},
			},
			{
				Type:    "table",
				Content: `"Name":"Bob";"Score":"82"`,
				Metadata: map[string]any{
					"doc_id":   "table-row-2",
					"doc_hash": "table-row-hash-2",
					"sheet":    "Sheet1",
					"source":   "test.xlsx",
				},
			},
			{
				Type:    "table",
				Content: "   ",
				Metadata: map[string]any{
					"sheet": "Sheet1",
				},
			},
		},
	}

	transformed, err := processor.Transform(context.Background(), output, &indexing.ProcessOptions{
		Mode: "table",
	})
	require.NoError(t, err)
	require.Len(t, transformed, 2)

	require.Equal(t, `"Name":"Alice";"Score":"90"`, transformed[0].Content)
	require.NotNil(t, transformed[0].BBox)
	require.Equal(t, *bbox, *transformed[0].BBox)
	require.Equal(t, "Sheet1", transformed[0].Metadata["sheet"])
	require.Equal(t, "test.xlsx", transformed[0].Metadata["source"])
	require.Equal(t, "table-row-1", transformed[0].Metadata["doc_id"])
	require.Equal(t, "table-row-hash-1", transformed[0].Metadata["doc_hash"])

	require.Equal(t, `"Name":"Bob";"Score":"82"`, transformed[1].Content)
	require.Nil(t, transformed[1].BBox)
	require.Equal(t, "table-row-2", transformed[1].Metadata["doc_id"])
}
