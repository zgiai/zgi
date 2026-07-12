package reducto

import (
	"math"
	"testing"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

func TestToDocumentResultMapsBlocksAndBBoxes(t *testing.T) {
	doc := toDocumentResult("sample.pdf", &parseResponse{
		JobID: "job-1",
		Usage: map[string]any{"num_pages": float64(2)},
		Result: parseResult{Chunks: []parseChunk{
			{
				Content: "Account summary",
				Blocks: []parseBlock{
					{
						Type:    "Title",
						Content: "Account summary",
						BBox: map[string]any{
							"page":   float64(1),
							"left":   0.1,
							"top":    0.2,
							"width":  0.7,
							"height": 0.1,
						},
						Confidence: "high",
						GranularConfidence: map[string]any{
							"parse_confidence": 0.84,
						},
					},
				},
			},
		}},
	})
	doc = extractcommon.EnrichStructuredOutput(doc)

	if doc.DocID != "job-1" {
		t.Fatalf("DocID=%q", doc.DocID)
	}
	if doc.PageCount != 2 || len(doc.Pages) != 2 {
		t.Fatalf("page shape mismatch: page_count=%d pages=%d", doc.PageCount, len(doc.Pages))
	}
	if len(doc.Chunks) != 1 {
		t.Fatalf("chunks=%d", len(doc.Chunks))
	}
	ch := doc.Chunks[0]
	if ch.Type != "heading" || ch.Subtype != "title" || ch.Page != 0 || ch.BBox == nil || ch.Precision != "reliable" {
		t.Fatalf("chunk mapping mismatch: %#v", ch)
	}
	if math.Abs(ch.Confidence-0.84) > 0.000001 {
		t.Fatalf("confidence=%f", ch.Confidence)
	}
	if math.Abs(ch.BBox.Right-0.8) > 0.000001 || math.Abs(ch.BBox.Bottom-0.3) > 0.000001 {
		t.Fatalf("bbox width/height conversion mismatch: %#v", ch.BBox)
	}
	if doc.ExtractOutput == nil || len(doc.ExtractOutput.Elements) != 1 {
		t.Fatalf("extract output not enriched: %#v", doc.ExtractOutput)
	}
	if doc.Diagnostics["reducto_job_id"] != "job-1" {
		t.Fatalf("diagnostics=%v", doc.Diagnostics)
	}
}

func TestToDocumentResultMapsChunkLevelFields(t *testing.T) {
	doc := toDocumentResult("sample.pdf", &parseResponse{
		JobID:    "job-2",
		Duration: 1.2,
		PDFURL:   "https://files.example.com/doc.pdf",
		Result: parseResult{Chunks: []parseChunk{
			{
				Type:              "Text",
				Content:           "Alpha",
				Embed:             "Alpha summary",
				Enriched:          "Alpha enriched",
				EnrichmentSuccess: true,
			},
		}},
	})
	if len(doc.Chunks) != 1 {
		t.Fatalf("chunks=%d", len(doc.Chunks))
	}
	payload := doc.Chunks[0].Payload
	if payload["reducto_embed"] != "Alpha summary" {
		t.Fatalf("payload=%v", payload)
	}
	if payload["reducto_enriched"] != "Alpha enriched" {
		t.Fatalf("payload=%v", payload)
	}
	if payload["reducto_enrichment_success"] != true {
		t.Fatalf("payload=%v", payload)
	}
	if doc.Diagnostics["reducto_pdf_url"] != "https://files.example.com/doc.pdf" {
		t.Fatalf("diagnostics=%v", doc.Diagnostics)
	}
}

func TestNormalizeTypeMappings(t *testing.T) {
	cases := []struct {
		raw         string
		wantType    string
		wantSubtype string
	}{
		{"Title", "heading", "title"},
		{"Section Header", "heading", "section_header"},
		{"Header", "header", "header"},
		{"Footer", "footer", "footer"},
		{"Key Value", "text", "key_value"},
		{"List Item", "list", "list_item"},
		{"Checkbox", "text", "checkbox"},
	}
	for _, tc := range cases {
		gotType, gotSubtype := normalizeType(tc.raw)
		if gotType != tc.wantType || gotSubtype != tc.wantSubtype {
			t.Fatalf("%s => (%s,%s), want (%s,%s)", tc.raw, gotType, gotSubtype, tc.wantType, tc.wantSubtype)
		}
	}
}

func TestConfidenceScore(t *testing.T) {
	if got := confidenceScore("high", nil); got != 0.9 {
		t.Fatalf("high confidence=%v", got)
	}
	if got := confidenceScore("low", nil); got != 0.5 {
		t.Fatalf("low confidence=%v", got)
	}
	if got := confidenceScore(nil, map[string]any{"parse_confidence": 0.42}); math.Abs(got-0.42) > 0.000001 {
		t.Fatalf("granular confidence=%v", got)
	}
}

func TestRuntimeConfigForOptionsKeepsRequestCredentialsIsolated(t *testing.T) {
	t.Setenv("REDUCTO_API_KEY", "global-key")
	config := runtimeConfigForOptions(extractcommon.ParseOptions{
		ProviderRuntime: extractcommon.ProviderRuntimeConfig{
			ProviderKey: "reducto",
			BaseURL:     "https://tenant.example/",
			APIKey:      "tenant-key",
		},
	})
	if config.apiKey != "tenant-key" || config.baseURL != "https://tenant.example" {
		t.Fatalf("runtime config = %#v", config)
	}
}
