package local

import (
	"testing"
	"time"

	extractcommon "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

func TestBuildLocalPipelineTraceNormalizesDiagnostics(t *testing.T) {
	diag := map[string]any{
		"native_pipeline_duration_ms": int64(12),
		"local_vlm_fallback": map[string]any{
			"status":  "applied",
			"applied": true,
			"reason":  "table_structure_low",
			"vlm_ms":  int64(1200),
			"chunks":  3,
			"model":   "qwen-vl-max-latest",
			"pages":   []any{1.0, 2.0},
		},
		"local_sidebar_ocr_status":      "no_text_added",
		"local_sidebar_ocr_duration_ms": 30,
		"vlm_image_caption_status":      "skipped_no_image_chunks",
	}

	trace := buildLocalPipelineTrace(diag, nil, "native+vlm")
	if len(trace) != 4 {
		t.Fatalf("trace len=%d trace=%#v", len(trace), trace)
	}
	if trace[0].Stage != "native_parse" || trace[0].Status != traceStatusApplied {
		t.Fatalf("native trace=%#v", trace[0])
	}
	if trace[1].Stage != "vlm_fallback" || trace[1].Status != traceStatusApplied || trace[1].Count != 3 {
		t.Fatalf("vlm trace=%#v", trace[1])
	}
	if got := trace[1].Pages; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("pages=%v", got)
	}
	if trace[2].Stage != "sidebar_ocr" || trace[2].Status != traceStatusSkipped {
		t.Fatalf("sidebar trace=%#v", trace[2])
	}
}

func TestBuildLocalPipelineTraceShowsRecoverableNativeFailure(t *testing.T) {
	diag := map[string]any{
		"full_document_error": map[string]any{
			"reason": "missing page tree object: 12",
		},
		"ocr_fallback": map[string]any{
			"applied": true,
			"chunks":  4,
			"engine":  "tesseract",
		},
	}

	trace := buildLocalPipelineTrace(diag, nil, "native+ocr:tesseract")
	if len(trace) != 2 {
		t.Fatalf("trace len=%d trace=%#v", len(trace), trace)
	}
	if trace[0].Stage != "native_parse" || trace[0].Status != traceStatusWarning {
		t.Fatalf("native trace=%#v", trace[0])
	}
	if trace[1].Stage != "ocr_fallback" || trace[1].Status != traceStatusApplied || trace[1].Count != 4 {
		t.Fatalf("ocr trace=%#v", trace[1])
	}
}

func TestBuildLocalPipelineTraceShowsPopplerRecovery(t *testing.T) {
	diag := map[string]any{
		"full_document_error": map[string]any{
			"reason": "unsupported xref section at offset 848577",
		},
		"poppler_text_recovery": map[string]any{
			"applied":     true,
			"chunks":      581,
			"duration_ms": int64(250),
			"source":      "pdftohtml_xml",
		},
	}

	trace := buildLocalPipelineTrace(diag, nil, "native+poppler:text")
	if len(trace) != 2 {
		t.Fatalf("trace len=%d trace=%#v", len(trace), trace)
	}
	if trace[0].Stage != "native_parse" || trace[0].Status != traceStatusWarning {
		t.Fatalf("native trace=%#v", trace[0])
	}
	if trace[1].Stage != "poppler_text_recovery" || trace[1].Status != traceStatusApplied || trace[1].Count != 581 {
		t.Fatalf("poppler trace=%#v", trace[1])
	}
}

func TestFinalizeLocalParseObservabilityAddsTraceAndRegionSummary(t *testing.T) {
	doc := &extractcommon.DocumentResult{
		FileName:  "sample.pdf",
		PageCount: 1,
		Source:    "native",
		Chunks: []extractcommon.Chunk{
			{
				ID:        "table-1",
				Type:      "table",
				Page:      0,
				Precision: "reliable",
				BBox:      &extractcommon.BBox{Left: 0.1, Top: 0.2, Right: 0.9, Bottom: 0.4},
				Payload:   map[string]any{"extraction_method": "rule"},
			},
			{
				ID:        "ocr-sidebar",
				Type:      "text",
				Page:      0,
				Precision: "reliable",
				BBox:      &extractcommon.BBox{Left: 0.7, Top: 0.2, Right: 0.95, Bottom: 0.6},
				Payload:   map[string]any{"sidebar_recovery": true, "extraction_method": "ocr"},
			},
		},
		Diagnostics: map[string]any{
			"native_pipeline_duration_ms": int64(8),
			"ocr_fallback":                map[string]any{"applied": false, "reason": "native_quality_ok"},
		},
	}

	finalizeLocalParseObservability(doc, nil, time.Now().Add(-time.Millisecond))

	if doc.Diagnostics["pipeline_trace"] == nil {
		t.Fatalf("missing pipeline_trace")
	}
	summary, ok := doc.Diagnostics["region_summary"].(map[string]any)
	if !ok {
		t.Fatalf("missing region_summary: %#v", doc.Diagnostics["region_summary"])
	}
	if summary["tables"] != 1 || summary["sidebar"] != 1 || summary["ocr_chunks"] != 1 {
		t.Fatalf("bad summary=%#v", summary)
	}
}
