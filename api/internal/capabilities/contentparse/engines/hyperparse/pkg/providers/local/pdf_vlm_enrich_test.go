package local

import (
	"context"
	"strings"
	"testing"
	"time"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

func TestLocalVLMFallbackCandidatePagesUsesPageRoutes(t *testing.T) {
	fullDoc := map[string]any{
		"document": map[string]any{
			"layout": map[string]any{"page_count": 4},
			"page_route_candidates": []map[string]any{
				{"page_index": 1, "recommended_mode": "native_only"},
				{"page_index": 3, "recommended_mode": "vlm_candidate", "reasons": []map[string]any{{"code": "scan_like"}}},
				{"page_index": 2, "recommended_mode": "vlm_candidate", "reasons": []map[string]any{{"code": "native_quality_low"}}},
				{"page_index": 2, "recommended_mode": "vlm_candidate", "reasons": []map[string]any{{"code": "native_quality_low"}}},
			},
		},
	}

	pages, reason := localVLMFallbackCandidatePages(fullDoc, 4)
	if reason != "page_route_candidates" {
		t.Fatalf("reason=%q", reason)
	}
	if len(pages) != 2 || pages[0] != 2 || pages[1] != 3 {
		t.Fatalf("pages=%v, want [2 3]", pages)
	}
}

func TestLocalFullDocSuspectGarbledText(t *testing.T) {
	fullDoc := map[string]any{
		"chunks": map[string]any{
			"items": []map[string]any{
				{"type": "paragraph", "text": "EjQpIEkhjZIghqQjPgIjIjQgjINIGjPIGIpIYdZIjGYkEPNqIDGZDQYIddhNgGEjghGEYQQEhGgQpQOGQOQjYGdjQ"},
				{"type": "paragraph", "text": "GkEjIGkhIggIhIgEPGkhIgQjIgpQIqhjkEpIgpYkDYIQhQOPjhjQNgZdgGkEjGIEQhQh"},
				{"type": "paragraph", "text": "INQIIpIjhGDkQYGgIdgjhQEIDXYsjQEhYYDgjIqQjPjPIIhQOIgjGINQIkhIgNYqhGjIhjjPIdgjjsdIhjGIYQpIg"},
				{"type": "paragraph", "text": "YYDgjIGqQjPjPIIGNGkEjjDkQYGjPIdgGkEjgGZdGIrIEkjIdgGkEjhjgjIOsjPjYQOhqQjP"},
				{"type": "paragraph", "text": "hkDhEgQdjQGQdddkgEPhIEfkQgIGZjPYsEjQpIEkhjZIghqQjPQZjPhNjIgYkEPOIIgjQOZjPYs"},
			},
		},
	}
	if !localFullDocSuspectGarbledText(fullDoc) {
		t.Fatalf("expected garbled full document text to prefer OCR")
	}
}

func TestLocalFullDocRouteDecisionScanLike(t *testing.T) {
	fullDoc := map[string]any{
		"document": map[string]any{
			"route_decision": map[string]any{
				"recommended_mode": "vlm_candidate",
				"reasons": []map[string]any{
					{"code": "scan_like", "score": 0.85},
				},
			},
		},
	}
	if !localFullDocRouteDecisionScanLike(fullDoc) {
		t.Fatalf("expected scan-like route decision")
	}
}

func TestLocalEngineContractDisablesMinerU(t *testing.T) {
	t.Setenv(envLocalVLMFallback, "")
	t.Setenv(envLocalVLMFallbackMaxPages, "")
	t.Setenv("DOCSTILL_VLM_FALLBACK_MAX_PAGES", "")
	t.Setenv(envLocalVLMSidebarRecovery, "")

	contract := localEngineContract()
	if contract["engine"] != "local" || contract["backend"] != "local" {
		t.Fatalf("unexpected local engine contract: %#v", contract)
	}
	if contract["mineru_used"] != false {
		t.Fatalf("local engine contract must report mineru_used=false: %#v", contract)
	}
	if contract["gpu_required"] != false {
		t.Fatalf("local engine should not require GPU: %#v", contract)
	}
	if contract["native_first_pass"] != true || contract["global_force_vlm_ignored"] != true {
		t.Fatalf("local engine should start with native-only first pass: %#v", contract)
	}
	if contract["vlm_fallback_max_pages"] != defaultLocalVLMFallbackMaxPages {
		t.Fatalf("local fallback should use bounded default, got %#v", contract["vlm_fallback_max_pages"])
	}
}

func TestLocalEngineContractReflectsForcedOptions(t *testing.T) {
	t.Setenv(envLocalVLMFallback, "0")
	t.Setenv(envLocalVLMSidebarRecovery, "0")

	contract := localEngineContractForOptions(extractcommon.ParseOptions{
		ForceLocalVLM:             true,
		ForceLocalSidebarRecovery: true,
	})

	if contract["vlm_fallback_setting"] != "force" {
		t.Fatalf("expected forced VLM fallback setting, got %#v", contract["vlm_fallback_setting"])
	}
	if contract["vlm_sidebar_recovery_setting"] != "force" {
		t.Fatalf("expected forced sidebar recovery setting, got %#v", contract["vlm_sidebar_recovery_setting"])
	}
}

func TestLocalVLMFallbackMaxPagesEnv(t *testing.T) {
	t.Setenv(envLocalVLMFallbackMaxPages, "0")
	if got := localVLMFallbackMaxPages(); got != 0 {
		t.Fatalf("LOCAL_VLM_FALLBACK_MAX_PAGES=0 should mean unlimited, got %d", got)
	}
	t.Setenv(envLocalVLMFallbackMaxPages, "2")
	if got := localVLMFallbackMaxPages(); got != 2 {
		t.Fatalf("expected explicit local max pages, got %d", got)
	}
}

func TestLocalVLMFallbackTimeoutEnv(t *testing.T) {
	t.Setenv(envLocalVLMFallbackTimeoutSeconds, "")
	t.Setenv("DOCSTILL_VLM_FALLBACK_TIMEOUT_SECONDS", "")
	if got := localVLMFallbackTimeout(); got != time.Duration(defaultLocalVLMFallbackTimeoutSeconds)*time.Second {
		t.Fatalf("default timeout=%s", got)
	}
	t.Setenv(envLocalVLMFallbackTimeoutSeconds, "12")
	if got := localVLMFallbackTimeout(); got != 12*time.Second {
		t.Fatalf("env timeout=%s", got)
	}
	t.Setenv(envLocalVLMFallbackTimeoutSeconds, "0")
	if got := localVLMFallbackTimeout(); got != 0 {
		t.Fatalf("zero should disable internal timeout, got %s", got)
	}
}

func TestLocalVLMFallbackCandidatePagesIgnoresBusinessFormOnly(t *testing.T) {
	fullDoc := map[string]any{
		"document": map[string]any{
			"layout": map[string]any{"page_count": 2},
			"page_route_candidates": []map[string]any{
				{
					"page_index":        1,
					"recommended_mode":  "vlm_candidate",
					"selected_for_vlm":  true,
					"reasons":           []map[string]any{{"code": "business_form_like", "score": 0.78}},
					"native_signals":    map[string]any{"geometry_line_count": 30},
					"business_doc_hint": true,
				},
			},
			"route_decision": map[string]any{
				"recommended_mode": "vlm_candidate",
				"reasons":          []map[string]any{{"code": "business_form_like", "score": 0.78}},
			},
			"suggest_vlm": true,
			"business_doc_vlm_hint": map[string]any{
				"suggest": true,
			},
		},
		"chunks": map[string]any{
			"items": []map[string]any{
				{"type": "paragraph", "page_index": 1, "text": strings.Repeat("native text ", 20)},
				{"type": "paragraph", "page_index": 2, "text": strings.Repeat("native text ", 20)},
			},
		},
	}

	pages, reason := localVLMFallbackCandidatePages(fullDoc, 2)
	if len(pages) != 0 || reason != "" {
		t.Fatalf("pages=%v reason=%q, want no fallback", pages, reason)
	}
}

func TestLocalVLMFallbackCandidatePagesUsesPerPageNativeQuality(t *testing.T) {
	fullDoc := map[string]any{
		"document": map[string]any{
			"layout": map[string]any{"page_count": 3},
			"business_doc_vlm_hint": map[string]any{
				"suggest": true,
			},
		},
		"chunks": map[string]any{
			"items": []map[string]any{
				{"type": "paragraph", "page_index": 1, "text": strings.Repeat("native text ", 20)},
				{"type": "image", "page_index": 2, "text": "full page image"},
				{"type": "paragraph", "page_index": 3, "text": strings.Repeat("native text ", 20)},
			},
		},
	}

	pages, reason := localVLMFallbackCandidatePages(fullDoc, 3)
	if reason != "per_page_native_quality_low" {
		t.Fatalf("reason=%q", reason)
	}
	if len(pages) != 1 || pages[0] != 2 {
		t.Fatalf("pages=%v, want [2]", pages)
	}
}

func TestLocalVLMFallbackCandidatePagesSkipsGoodPerPageNativeText(t *testing.T) {
	fullDoc := map[string]any{
		"document": map[string]any{
			"layout": map[string]any{"page_count": 2},
		},
		"chunks": map[string]any{
			"items": []map[string]any{
				{"type": "paragraph", "page_index": 1, "text": strings.Repeat("left page ", 20)},
				{"type": "paragraph", "page_index": 2, "text": strings.Repeat("right page ", 20)},
			},
		},
	}

	pages, reason := localVLMFallbackCandidatePages(fullDoc, 2)
	if len(pages) != 0 || reason != "" {
		t.Fatalf("pages=%v reason=%q, want no fallback", pages, reason)
	}
}

func TestLocalVLMFallbackCandidatePagesUsesVisualTextSparseSignal(t *testing.T) {
	fullDoc := map[string]any{
		"document": map[string]any{
			"layout": map[string]any{"page_count": 1},
		},
		"chunks": map[string]any{
			"items": []map[string]any{
				{
					"type":       "text",
					"page_index": 1,
					"text":       "Lessons in Arabic Language, Book 1",
				},
				{
					"type":       "marginalia",
					"page_index": 1,
					"text":       "Courtesy of Fatwa-Online",
				},
				{
					"type":       "figure",
					"page_index": 1,
					"text":       "embedded page image",
				},
			},
		},
	}

	pages, reason := localVLMFallbackCandidatePages(fullDoc, 1)
	if reason != "visual_text_sparse" {
		t.Fatalf("reason=%q", reason)
	}
	if len(pages) != 1 || pages[0] != 1 {
		t.Fatalf("pages=%v, want [1]", pages)
	}
}

func TestLocalSidebarRecoveryCandidatePagesDetectsMissingRightRail(t *testing.T) {
	fullDoc := map[string]any{
		"document": map[string]any{
			"layout": map[string]any{"page_count": 2},
		},
		"chunks": map[string]any{
			"items": []map[string]any{
				{
					"type":       "heading",
					"page_index": 2,
					"text":       "Your electricity bill in more detail",
					"bbox":       map[string]any{"left": 0.06, "top": 0.05, "right": 0.50, "bottom": 0.09},
				},
				{
					"type":       "table",
					"page_index": 2,
					"text":       strings.Repeat("usage row amount ", 12),
					"bbox":       map[string]any{"left": 0.06, "top": 0.12, "right": 0.68, "bottom": 0.34},
				},
				{
					"type":       "paragraph",
					"page_index": 1,
					"text":       strings.Repeat("page one native text ", 12),
					"bbox":       map[string]any{"left": 0.08, "top": 0.10, "right": 0.92, "bottom": 0.40},
				},
			},
		},
	}

	pages, reason := localSidebarRecoveryCandidatePages(fullDoc, 2, false)
	if reason != "missing_right_sidebar" {
		t.Fatalf("reason=%q", reason)
	}
	if len(pages) != 1 || pages[0] != 2 {
		t.Fatalf("pages=%v, want [2]", pages)
	}
}

func TestLocalSidebarRecoveryCandidatePagesCanBeForced(t *testing.T) {
	fullDoc := map[string]any{
		"document": map[string]any{
			"layout": map[string]any{"page_count": 1},
		},
		"chunks": map[string]any{
			"items": []map[string]any{
				{
					"type":       "paragraph",
					"page_index": 1,
					"text":       "left text",
					"bbox":       map[string]any{"left": 0.06, "top": 0.10, "right": 0.30, "bottom": 0.14},
				},
			},
		},
	}

	pages, reason := localSidebarRecoveryCandidatePages(fullDoc, 1, true)
	if reason != "missing_right_sidebar" {
		t.Fatalf("reason=%q", reason)
	}
	if len(pages) != 1 || pages[0] != 1 {
		t.Fatalf("pages=%v, want [1]", pages)
	}
}

func TestLocalSidebarRecoveryCandidatePagesSkipsCoveredRightRail(t *testing.T) {
	fullDoc := map[string]any{
		"document": map[string]any{
			"layout": map[string]any{"page_count": 1},
		},
		"chunks": map[string]any{
			"items": []map[string]any{
				{
					"type":       "heading",
					"page_index": 1,
					"text":       "Your electricity bill in more detail",
					"bbox":       map[string]any{"left": 0.06, "top": 0.05, "right": 0.50, "bottom": 0.09},
				},
				{
					"type":       "paragraph",
					"page_index": 1,
					"text":       strings.Repeat("usage row amount ", 12),
					"bbox":       map[string]any{"left": 0.06, "top": 0.12, "right": 0.68, "bottom": 0.34},
				},
				{
					"type":       "paragraph",
					"page_index": 1,
					"text":       "Customer service\nPhone 1800 372 372",
					"bbox":       map[string]any{"left": 0.72, "top": 0.14, "right": 0.92, "bottom": 0.36},
				},
			},
		},
	}

	pages, reason := localSidebarRecoveryCandidatePages(fullDoc, 1, false)
	if len(pages) != 0 || reason != "" {
		t.Fatalf("pages=%v reason=%q, want no sidebar recovery", pages, reason)
	}
}

func TestLocalVLMFallbackCandidatePagesFallsBackToRouteDecision(t *testing.T) {
	fullDoc := map[string]any{
		"document": map[string]any{
			"layout":         map[string]any{"page_count": 2},
			"route_decision": map[string]any{"recommended_mode": "vlm_candidate"},
		},
	}

	pages, reason := localVLMFallbackCandidatePages(fullDoc, 0)
	if reason != "route_decision:vlm_candidate" {
		t.Fatalf("reason=%q", reason)
	}
	if len(pages) != 2 || pages[0] != 1 || pages[1] != 2 {
		t.Fatalf("pages=%v, want [1 2]", pages)
	}
}

func TestApplyLocalVLMFallbackSkipsBeforeRenderWhenKeyMissing(t *testing.T) {
	t.Setenv(envLocalVLMFallback, "1")
	t.Setenv("VLM_API_KEY", "")
	t.Setenv("DASHSCOPE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	fullDoc := map[string]any{
		"document": map[string]any{
			"layout": map[string]any{"page_count": 1},
		},
		"chunks": map[string]any{
			"items": []map[string]any{
				{"type": "paragraph", "page_index": 1, "text": "short"},
			},
		},
	}
	inspect := map[string]any{}

	applied := applyLocalVLMFallback(context.Background(), "dummy.pdf", nil, "relaxed", 1, fullDoc, inspect, extractcommon.ParseOptions{})
	if len(applied) != 0 {
		t.Fatalf("applied=%v, want none", applied)
	}
	diag, _ := inspect["local_vlm_fallback"].(map[string]any)
	if diag == nil {
		t.Fatalf("missing local_vlm_fallback diag")
	}
	if got := diag["status"]; got != "skipped_missing_vlm_api_key" {
		t.Fatalf("status=%v", got)
	}
}

func TestFinalizeLocalVLMChunksTransfersImageOrderToNativeImage(t *testing.T) {
	fullDoc := map[string]any{
		"chunks": map[string]any{
			"vlm_merge": true,
			"items": []map[string]any{
				{"type": "paragraph", "page_index": 1, "order": 1, "text": "before", "source": "vlm", "vlm_merge": "from_vlm"},
				{"type": "image", "page_index": 1, "order": 0, "text": "<<:figure: native::>", "bbox": map[string]any{"left": 0.1, "top": 0.5, "right": 0.4, "bottom": 0.7}},
				{"type": "image", "page_index": 1, "order": 2, "text": "<<:figure: duplicate::>", "source": "vlm", "vlm_merge": "from_vlm"},
				{"type": "paragraph", "page_index": 1, "order": 3, "text": "after", "source": "vlm", "vlm_merge": "from_vlm"},
			},
		},
	}
	inspect := map[string]any{}

	finalizeLocalVLMChunksForExport(fullDoc, inspect)

	chunks := normalizeMapSlice(fullDoc["chunks"].(map[string]any)["items"])
	if got := len(chunks); got != 3 {
		t.Fatalf("chunk count=%d want 3: %#v", got, chunks)
	}
	if got := inspect["local_vlm_image_dedupe_count"]; got != 1 {
		t.Fatalf("dedupe=%v want 1", got)
	}
	if chunks[0]["text"] != "before" || chunks[1]["text"] != "<<:figure: native::>" || chunks[2]["text"] != "after" {
		t.Fatalf("unexpected order: %#v", chunks)
	}
	if got := intAny(chunks[1]["order"]); got != 2 {
		t.Fatalf("native image order=%d want 2", got)
	}
}

func TestDedupeDocumentFigureChunksPrefersContextualVLMFigure(t *testing.T) {
	doc := &extractcommon.DocumentResult{
		Chunks: []extractcommon.Chunk{
			{
				Type:     "figure",
				Page:     0,
				Text:     "<<:figure: 这是一张组织切片图像::>",
				Markdown: "generic",
				Ordinal:  1,
				BBox:     &extractcommon.BBox{Left: 0.1, Top: 0.5, Right: 0.4, Bottom: 0.7},
				Payload: map[string]any{
					"preview_data_url": "data:image/jpeg;base64,abc",
					"vlm_caption":      "native caption",
				},
			},
			{Type: "text", Page: 0, Text: "DIAGNOSIS:", Markdown: "DIAGNOSIS:", Ordinal: 2},
			{Type: "figure", Page: 0, Text: "<<:figure: 图A: 右臂皮肤活检显微图像::>", Markdown: "contextual", Ordinal: 3, BBox: &extractcommon.BBox{Left: 0.11, Top: 0.51, Right: 0.39, Bottom: 0.69}},
		},
	}

	dedupeDocumentFigureChunks(doc)

	if got := len(doc.Chunks); got != 2 {
		t.Fatalf("chunk count=%d want 2", got)
	}
	if doc.Chunks[1].Text != "<<:figure: 图A: 右臂皮肤活检显微图像::>" {
		t.Fatalf("kept figure=%q", doc.Chunks[1].Text)
	}
	if got, _ := doc.Chunks[1].Payload["preview_data_url"].(string); got != "data:image/jpeg;base64,abc" {
		t.Fatalf("preview payload=%q", got)
	}
	if got, _ := doc.Chunks[1].Payload["vlm_caption"].(string); got != "native caption" {
		t.Fatalf("caption payload=%q", got)
	}
	if diag := doc.Diagnostics["local_figure_dedupe"].(map[string]any); diag["dropped"] != 1 {
		t.Fatalf("diag=%v", diag)
	}
}

func TestNormalizeDecorativeLogoFiguresDemotesHeaderLogo(t *testing.T) {
	doc := &extractcommon.DocumentResult{
		Chunks: []extractcommon.Chunk{
			{
				Type: "figure",
				Text: "<<:figure: Diagnostic Pathology Medical Group, Inc. Logo::>",
				Page: 1,
				BBox: &extractcommon.BBox{Left: 0.05, Top: 0.05, Right: 0.25, Bottom: 0.15},
			},
		},
	}

	normalizeDecorativeLogoFigures(doc)

	if doc.Chunks[0].Type != "marginalia" || doc.Chunks[0].Subtype != "logo" {
		t.Fatalf("chunk=%+v", doc.Chunks[0])
	}
	if diag := doc.Diagnostics["local_logo_figure_demote"].(map[string]any); diag["count"] != 1 {
		t.Fatalf("diag=%v", diag)
	}
}
