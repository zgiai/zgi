package export

import (
	"math"
	"strings"
	"testing"
)

func TestBuildDPTExportFromFullDocument_minimal(t *testing.T) {
	fd := map[string]any{
		"chunks": map[string]any{
			"items": []map[string]any{
				{"type": "paragraph", "page_index": 1, "order": 0, "text": "Hello"},
				{
					"type": "table", "page_index": 1, "order": 1, "text": "t",
					"payload": map[string]any{
						"row_count": 1, "column_count": 2,
						"cells": []any{
							map[string]any{"row": 0, "col": 0, "text": "A"},
							map[string]any{"row": 0, "col": 1, "text": "B"},
						},
					},
				},
			},
		},
	}
	out := BuildDPTExportFromFullDocument(fd, "x.pdf", 1)
	if out["markdown"] == "" {
		t.Fatal("expected markdown")
	}
	ch := out["chunks"].([]any)
	if len(ch) != 2 {
		t.Fatalf("chunks len=%d", len(ch))
	}
	tablePayload, _ := ch[1].(map[string]any)["payload"].(map[string]any)
	if tablePayload == nil || tablePayload["row_count"] != 1 {
		t.Fatalf("expected table payload, got %#v", ch[1])
	}
	g := out["grounding"].(map[string]any)
	if len(g) != 0 {
		t.Fatalf("expected no grounding without bbox, got %d", len(g))
	}
	rag := out["rag"].(map[string]any)
	items := rag["embedding_items"].([]any)
	if len(items) != 2 {
		t.Fatalf("rag embedding_items len=%d", len(items))
	}
	m0 := items[0].(map[string]any)["metadata"].(map[string]any)
	if m0["section_identifier"] != "section_fallback_page_0" {
		t.Fatalf("meta section_identifier=%v", m0["section_identifier"])
	}
}

func TestBuildDPTExportFromFullDocument_figureWithNilTextGetsFallback(t *testing.T) {
	fd := map[string]any{
		"chunks": map[string]any{
			"items": []map[string]any{
				{
					"type":       "image",
					"page_index": 1,
					"order":      0,
					"text":       nil,
					"payload": map[string]any{
						"format": "jpeg",
						"width":  347,
						"height": 260,
					},
				},
			},
		},
	}
	out := BuildDPTExportFromFullDocument(fd, "x.pdf", 1)
	chunks := out["chunks"].([]any)
	if len(chunks) != 1 {
		t.Fatalf("chunks len=%d", len(chunks))
	}
	ch := chunks[0].(map[string]any)
	if ch["type"] != "figure" {
		t.Fatalf("chunk type=%v", ch["type"])
	}
	md := ch["markdown"].(string)
	if strings.Contains(md, "<nil>") || strings.Contains(strings.ToLower(md), "null") {
		t.Fatalf("markdown should not expose nil text: %q", md)
	}
	if !strings.Contains(md, "Embedded image (jpeg, 347x260)") {
		t.Fatalf("expected image fallback text, got %q", md)
	}
	rag := out["rag"].(map[string]any)
	item := rag["embedding_items"].([]any)[0].(map[string]any)
	if item["text"] != "Embedded image (jpeg, 347x260)" {
		t.Fatalf("rag text=%q", item["text"])
	}
}

func TestBuildDPTExportFromFullDocument_preservesChunkProvenance(t *testing.T) {
	fd := map[string]any{
		"chunks": map[string]any{
			"items": []map[string]any{
				{
					"type":         "paragraph",
					"page_index":   1,
					"order":        0,
					"text":         "Recovered line",
					"source":       "native_pdf_layout_repair",
					"source_trace": "layout:page#1",
					"confidence":   0.76,
					"bbox":         map[string]any{"left": 0.1, "top": 0.2, "right": 0.6, "bottom": 0.24},
					"payload": map[string]any{
						"repair":   "geometry_text_missing_from_chunks",
						"strategy": "quality_gate",
					},
				},
			},
		},
	}
	out := BuildDPTExportFromFullDocument(fd, "x.pdf", 1)
	chunks := out["chunks"].([]any)
	if len(chunks) != 1 {
		t.Fatalf("chunks len=%d", len(chunks))
	}
	ch := chunks[0].(map[string]any)
	if ch["source"] != "repaired" {
		t.Fatalf("source=%v", ch["source"])
	}
	if ch["pipeline_stage"] != "bbox_alignment_repair" {
		t.Fatalf("pipeline_stage=%v", ch["pipeline_stage"])
	}
	prov := ch["provenance"].(map[string]any)
	if prov["method"] != "quality_gate" {
		t.Fatalf("method=%v", prov["method"])
	}
	g := ch["grounding"].(map[string]any)
	if g["confidence"] != 0.76 {
		t.Fatalf("grounding confidence=%v", g["confidence"])
	}
	if spans, ok := g["low_confidence_spans"].([]map[string]any); !ok || len(spans) != 1 {
		t.Fatalf("expected low confidence span, got=%T %v", g["low_confidence_spans"], g["low_confidence_spans"])
	}
}

func TestBuildDPTExportFromFullDocument_marksRegionalFallbackStage(t *testing.T) {
	fd := map[string]any{
		"chunks": map[string]any{
			"items": []map[string]any{
				{
					"type":         "paragraph",
					"page_index":   1,
					"order":        0,
					"text":         "Region repaired line",
					"source":       "regional_vlm",
					"source_trace": "regional_vlm:region_p1_0:bbox_mismatch_geometry_lines",
					"confidence":   0.82,
					"bbox":         map[string]any{"left": 0.1, "top": 0.2, "right": 0.8, "bottom": 0.28},
					"payload": map[string]any{
						"regional_fallback": true,
						"repair":            "regional_low_confidence_merge",
						"model":             "qwen-vl",
					},
				},
			},
		},
	}
	out := BuildDPTExportFromFullDocument(fd, "x.pdf", 1)
	ch := out["chunks"].([]any)[0].(map[string]any)
	if ch["source"] != "vlm" {
		t.Fatalf("source=%v", ch["source"])
	}
	if ch["pipeline_stage"] != "regional_ocr_vlm" {
		t.Fatalf("pipeline_stage=%v", ch["pipeline_stage"])
	}
	prov := ch["provenance"].(map[string]any)
	if prov["model"] != "qwen-vl" {
		t.Fatalf("model=%v", prov["model"])
	}
}

func TestBuildDPTExportFromFullDocument_classifyLogoAndMarginalia(t *testing.T) {
	fd := map[string]any{
		"chunks": map[string]any{
			"items": []map[string]any{
				{
					"type": "paragraph", "page_index": 1, "order": 0,
					"text": "LIBERTY MUTUAL INSURANCE COMPANY",
					"bbox": map[string]any{"left": 0.32, "top": 0.09, "right": 0.64, "bottom": 0.21},
				},
				{
					"type": "paragraph", "page_index": 1, "order": 1,
					"text": "Page 1 of 2",
					"bbox": map[string]any{"left": 0.84, "top": 0.92, "right": 0.95, "bottom": 0.95},
				},
			},
		},
	}
	out := BuildDPTExportFromFullDocument(fd, "x.pdf", 2)
	chunks := out["chunks"].([]any)
	if len(chunks) != 2 {
		t.Fatalf("chunks len=%d", len(chunks))
	}
	c0 := chunks[0].(map[string]any)
	if c0["type"] != "logo" {
		t.Fatalf("chunk0 type=%v", c0["type"])
	}
	c1 := chunks[1].(map[string]any)
	if c1["type"] != "marginalia" {
		t.Fatalf("chunk1 type=%v", c1["type"])
	}
	if c1["subtype"] != "page_number" {
		t.Fatalf("chunk1 subtype=%v", c1["subtype"])
	}
}

func TestBuildDPTExportFromFullDocument_tableCellGrounding(t *testing.T) {
	fd := map[string]any{
		"chunks": map[string]any{
			"items": []map[string]any{
				{
					"type": "table", "page_index": 1, "order": 0, "text": "tbl",
					"payload": map[string]any{
						"row_count": 1, "column_count": 2,
						"cells": []any{
							map[string]any{
								"row": 0, "col": 0, "text": "A",
								"bbox": map[string]any{"left": 0.1, "top": 0.2, "right": 0.2, "bottom": 0.1},
							},
							map[string]any{
								"row": 0, "col": 1, "text": "B",
								"bbox": map[string]any{"left": 0.2, "top": 0.2, "right": 0.3, "bottom": 0.1},
							},
						},
					},
					"bbox": map[string]any{"left": 0.1, "top": 0.2, "right": 0.3, "bottom": 0.1},
				},
			},
		},
	}
	out := BuildDPTExportFromFullDocument(fd, "t.pdf", 1)
	chunks := out["chunks"].([]any)
	if len(chunks) != 1 {
		t.Fatalf("chunks len=%d", len(chunks))
	}
	ch := chunks[0].(map[string]any)
	md := ch["markdown"].(string)
	if md == "" || !strings.Contains(md, `<table id="`) || !strings.Contains(md, `<td id="`) {
		t.Fatalf("expected HTML table with ids in markdown: %s", md)
	}
	g := out["grounding"].(map[string]any)
	hasTableCell := false
	for _, v := range g {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if m["type"] == "tableCell" {
			hasTableCell = true
			break
		}
	}
	if !hasTableCell {
		t.Fatalf("expected tableCell grounding entries, got=%v", g)
	}
}

func TestBuildDPTExportFromFullDocument_groundingUsesTopOriginAndAddsSize(t *testing.T) {
	fd := map[string]any{
		"chunks": map[string]any{
			"items": []map[string]any{
				{
					"type": "paragraph", "page_index": 1, "order": 0, "text": "bbox",
					"bbox": map[string]any{"left": 0.1, "top": 0.9, "right": 0.4, "bottom": 0.8},
				},
			},
		},
	}
	out := BuildDPTExportFromFullDocument(fd, "x.pdf", 1)
	chunks := out["chunks"].([]any)
	g := chunks[0].(map[string]any)["grounding"].(map[string]any)
	box := g["box"].(map[string]any)
	if box["left"] != 0.1 || box["right"] != 0.4 {
		t.Fatalf("unexpected horizontal box=%v", box)
	}
	top := box["top"].(float64)
	bottom := box["bottom"].(float64)
	if math.Abs(top-0.1) > 1e-9 || math.Abs(bottom-0.2) > 1e-9 {
		t.Fatalf("expected top-origin box, got=%v", box)
	}
	width := box["width"].(float64)
	height := box["height"].(float64)
	if math.Abs(width-0.3) > 1e-9 {
		t.Fatalf("unexpected width=%v", box["width"])
	}
	if math.Abs(height-0.1) > 1e-9 {
		t.Fatalf("unexpected height=%v", box["height"])
	}
}

func TestBuildDPTExportFromFullDocument_groundingKeepsTopOriginWhenAlreadyCanonical(t *testing.T) {
	fd := map[string]any{
		"chunks": map[string]any{
			"items": []map[string]any{
				{
					"type": "paragraph", "page_index": 1, "order": 0, "text": "vlm",
					"bbox": map[string]any{"left": 0.1, "top": 0.2, "right": 0.4, "bottom": 0.3},
				},
			},
		},
	}
	out := BuildDPTExportFromFullDocument(fd, "x.pdf", 1)
	chunks := out["chunks"].([]any)
	g := chunks[0].(map[string]any)["grounding"].(map[string]any)
	box := g["box"].(map[string]any)
	top := box["top"].(float64)
	bottom := box["bottom"].(float64)
	if math.Abs(top-0.2) > 1e-9 || math.Abs(bottom-0.3) > 1e-9 {
		t.Fatalf("expected canonical box to stay unchanged, got=%v", box)
	}
}

func TestBuildDPTExport_sectionSplits_heading(t *testing.T) {
	fd := map[string]any{
		"chunks": map[string]any{
			"items": []map[string]any{
				{"type": "paragraph", "page_index": 1, "order": 0, "text": "intro"},
				{"type": "heading", "page_index": 1, "order": 1, "text": "DIAGNOSIS"},
				{"type": "paragraph", "page_index": 1, "order": 2, "text": "body"},
			},
		},
	}
	out := BuildDPTExportFromFullDocument(fd, "x.pdf", 1)
	secs := out["splits_sections"].([]any)
	if len(secs) != 2 {
		t.Fatalf("splits_sections len=%d", len(secs))
	}
	s1 := secs[1].(map[string]any)
	if s1["title"] != "DIAGNOSIS" {
		t.Fatalf("section1 title=%v", s1["title"])
	}
	doc := out["_hyperparse"].(map[string]any)
	if doc["section_split_mode"] != "heading_markdown" {
		t.Fatalf("section_split_mode=%v", doc["section_split_mode"])
	}
}

func TestBuildDPTExport_sectionSplits_fallbackByPage(t *testing.T) {
	fd := map[string]any{
		"chunks": map[string]any{
			"items": []map[string]any{
				{"type": "paragraph", "page_index": 1, "order": 0, "text": "only"},
			},
		},
	}
	out := BuildDPTExportFromFullDocument(fd, "x.pdf", 1)
	secs := out["splits_sections"].([]any)
	if len(secs) != 1 {
		t.Fatalf("splits_sections len=%d", len(secs))
	}
	id := secs[0].(map[string]any)["identifier"].(string)
	if id != "section_fallback_page_0" {
		t.Fatalf("identifier=%s", id)
	}
	doc := out["_hyperparse"].(map[string]any)
	if doc["section_split_mode"] != "fallback_by_page" {
		t.Fatalf("section_split_mode=%v", doc["section_split_mode"])
	}
}

func TestBuildDPTExport_extraSectionLineRE(t *testing.T) {
	t.Setenv("DOCSTILL_DPT_SECTION_LINE_RE", `(?i)^(DIAGNOSIS)\s*:`)
	fd := map[string]any{
		"chunks": map[string]any{
			"items": []map[string]any{
				{"type": "paragraph", "page_index": 1, "order": 0, "text": "intro"},
				{"type": "paragraph", "page_index": 1, "order": 1, "text": "DIAGNOSIS: melanoma"},
				{"type": "paragraph", "page_index": 1, "order": 2, "text": "details"},
			},
		},
	}
	out := BuildDPTExportFromFullDocument(fd, "x.pdf", 1)
	secs := out["splits_sections"].([]any)
	if len(secs) != 2 {
		t.Fatalf("splits_sections len=%d", len(secs))
	}
	s1 := secs[1].(map[string]any)
	if s1["title"] != "DIAGNOSIS" {
		t.Fatalf("section1 title=%v", s1["title"])
	}
	rag := out["rag"].(map[string]any)
	items := rag["embedding_items"].([]any)
	m := items[2].(map[string]any)["metadata"].(map[string]any)
	if m["section_title"] != "DIAGNOSIS" {
		t.Fatalf("rag meta section_title=%v", m["section_title"])
	}
}
