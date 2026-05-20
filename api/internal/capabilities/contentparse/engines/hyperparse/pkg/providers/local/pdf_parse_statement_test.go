package local

import (
	"strings"
	"testing"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

func TestDemoteStatementInfoTable(t *testing.T) {
	chunk := extractcommon.Chunk{
		Type: "table",
		Markdown: `<table><tr><td>10 Test Road</td><td>IBAN IE00TEST123</td></tr>` +
			`<tr><td>Test City</td><td>BIC TESTIE22</td></tr></table>`,
	}

	demoteStatementInfoTable(&chunk)

	if chunk.Type != "text" || chunk.Subtype != "statement_info" {
		t.Fatalf("type=%q subtype=%q", chunk.Type, chunk.Subtype)
	}
	if strings.Contains(strings.ToLower(chunk.Markdown), "<table") {
		t.Fatalf("markdown should be plain text, got %q", chunk.Markdown)
	}
	if !strings.Contains(chunk.Text, "IBAN") || !strings.Contains(chunk.Text, "BIC") {
		t.Fatalf("text lost account identifiers: %q", chunk.Text)
	}
}

func TestDemoteStatementInfoTableKeepsRealFinancialTable(t *testing.T) {
	chunk := extractcommon.Chunk{
		Type: "table",
		Markdown: `<table><tr><td>Product</td><td>Opening balance</td><td>Money out</td></tr>` +
			`<tr><td>Account</td><td>100.00</td><td>10.00</td></tr></table>`,
	}

	demoteStatementInfoTable(&chunk)

	if chunk.Type != "table" {
		t.Fatalf("real table should stay table, got %q", chunk.Type)
	}
}

func TestBuildChunksPreservesExtractionProvenance(t *testing.T) {
	dpt := map[string]any{
		"chunks": []any{
			map[string]any{
				"id":       "vlm-sidebar",
				"type":     "text",
				"markdown": "Recovered sidebar text",
				"grounding": map[string]any{
					"page": 0,
					"box":  map[string]any{"left": 0.7, "top": 0.1, "right": 0.95, "bottom": 0.2},
				},
			},
		},
	}
	textByOrder := map[int]map[string]any{
		0: {
			"text":            "Recovered sidebar text",
			"source":          "vlm",
			"source_trace":    "vlm:right-sidebar:page#2",
			"vlm_merge":       "from_vlm_sidebar",
			"type":            "paragraph",
			"bbox_source":     "ocr_bbox_anchor_union",
			"bbox_precise":    true,
			"bbox_confidence": 0.98,
		},
	}

	chunks := buildChunks(dpt, textByOrder)
	if len(chunks) != 1 {
		t.Fatalf("chunks=%d", len(chunks))
	}
	payload := chunks[0].Payload
	if payload["extraction_method"] != "vlm" {
		t.Fatalf("extraction_method=%v", payload["extraction_method"])
	}
	if payload["extraction_source_raw"] != "vlm" || payload["vlm_merge"] != "from_vlm_sidebar" {
		t.Fatalf("payload=%#v", payload)
	}
	if payload["bbox_source"] != "ocr_bbox_anchor_union" || payload["bbox_precise"] != true {
		t.Fatalf("bbox provenance not preserved: %#v", payload)
	}
	if payload["bbox_confidence"] != 0.98 {
		t.Fatalf("bbox_confidence=%v", payload["bbox_confidence"])
	}
}
