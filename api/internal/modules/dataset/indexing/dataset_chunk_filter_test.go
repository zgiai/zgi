package indexing

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
)

func TestCleanDatasetTransformedChunksDropsLowValueParserNoise(t *testing.T) {
	contractNumber := "\u5408\u540c\u7f16\u53f7:CU12-3702-2024-004545"
	pageCounter := "\u7b2c1\u9875\u51713\u9875"
	blankDate := "\u65e5\u671f\uff1a \u5e74 \u6708 \u65e5"
	companyFragment := "\u7f51\u7edc\u901a\u4fe1\u6709\u9650\u516c\u53f8"
	repeatedHeaderCode := "CMSD-JN-202405229"
	qrVLMNote := "The image provided is a QR code. It does not contain any text, mathematical formulas, tables, or figures that can be processed according to the OCR instructions. Therefore, no textual content can be extracted from this image."

	chunks := []dto.TransformedChunk{
		{
			Content: "Lease renewal terms include payment schedule, parties, rent amount, and effective dates.",
			Metadata: map[string]any{
				"doc_id": "parent-1",
			},
			Children: []dto.TransformedChildChunk{
				{Content: "[figure]"},
				{Content: "Payment schedule and renewal period are the useful child content."},
			},
		},
		{Content: contractNumber},
		{Content: "[Barcode]"},
		{Content: pageCounter},
		{Content: "2019071061023"},
		{Content: blankDate},
		{Content: "[figure]"},
		{Content: "[QR]"},
		{Content: qrVLMNote},
		{Content: companyFragment},
		{Content: contractNumber},
		{Content: repeatedHeaderCode},
		{Content: repeatedHeaderCode},
		{Content: repeatedHeaderCode},
		{Content: "1"},
		{Content: "2"},
		{Content: "3"},
		{Content: "4"},
		{Content: "\u4e03"},
		{Content: "'756"},
		{Content: "Appendix title with enough context to keep"},
	}

	got := cleanDatasetTransformedChunks(chunks)
	if len(got) != 2 {
		t.Fatalf("len(got)=%d, want 2: %+v", len(got), got)
	}
	if got[0].Content != chunks[0].Content {
		t.Fatalf("first content=%q, want useful paragraph", got[0].Content)
	}
	if len(got[0].Children) != 1 {
		t.Fatalf("children=%d, want 1: %+v", len(got[0].Children), got[0].Children)
	}
	if got[0].Children[0].Content == "[figure]" {
		t.Fatal("figure placeholder child was not filtered")
	}
	if got[1].Content != "Appendix title with enough context to keep" {
		t.Fatalf("second content=%q, want appendix title", got[1].Content)
	}
	if got[0].Metadata["total_chunks"] != 2 || got[1].Metadata["chunk_index"] != 1 {
		t.Fatalf("chunk metadata not reindexed: %+v %+v", got[0].Metadata, got[1].Metadata)
	}
}

func TestCleanDatasetTransformedChunksKeepsAllNoiseWhenNoUsefulContent(t *testing.T) {
	chunks := []dto.TransformedChunk{
		{Content: "[Barcode]"},
		{Content: "\u7b2c1\u9875\u51711\u9875"},
	}

	got := cleanDatasetTransformedChunks(chunks)
	if len(got) != len(chunks) {
		t.Fatalf("len(got)=%d, want %d", len(got), len(chunks))
	}
}
