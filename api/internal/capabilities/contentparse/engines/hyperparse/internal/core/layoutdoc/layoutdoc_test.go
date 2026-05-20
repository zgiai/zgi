package layoutdoc

import "testing"

func TestNormalizeAndOrderChunkMaps_AddsProvenanceAndOrdersTwoColumns(t *testing.T) {
	chunks := []map[string]any{
		testChunk("right_top", "native_pdf", 1, 0, 0.66, 0.94, 0.05, 0.10),
		testChunk("left_bottom", "native_pdf", 1, 1, 0.05, 0.52, 0.35, 0.42),
		testChunk("left_top", "native_pdf", 1, 2, 0.05, 0.52, 0.05, 0.10),
		testChunk("right_bottom", "native_pdf", 1, 3, 0.66, 0.94, 0.35, 0.42),
	}

	got, report, err := NormalizeAndOrderChunkMaps("two-column.pdf", 1, chunks)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{
		got[0]["chunk_id"].(string),
		got[1]["chunk_id"].(string),
		got[2]["chunk_id"].(string),
		got[3]["chunk_id"].(string),
	}
	want := []string{"left_top", "left_bottom", "right_top", "right_bottom"}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("order mismatch got=%v want=%v", ids, want)
		}
		if got[i]["order"] != i {
			t.Fatalf("order not rewritten at %d: %v", i, got[i]["order"])
		}
	}
	prov := got[0]["provenance"].(map[string]any)
	if prov["source"] != SourceNative || prov["stage"] != "native_text" {
		t.Fatalf("unexpected provenance: %+v", prov)
	}
	if report.SourceCounts[SourceNative] != 4 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestApplyChunkProvenance_ClassifiesFallbackSources(t *testing.T) {
	cases := []struct {
		name   string
		chunk  map[string]any
		source string
		stage  string
	}{
		{
			name: "regional ocr",
			chunk: map[string]any{
				"source": "regional_ocr",
				"payload": map[string]any{
					"regional_fallback": true,
					"ocr_engine":        "paddleocr",
				},
			},
			source: SourceOCR,
			stage:  "regional_ocr_vlm",
		},
		{
			name: "layout repair",
			chunk: map[string]any{
				"source": "native_pdf_layout_repair",
				"payload": map[string]any{
					"repair": "geometry_text_missing_from_chunks",
				},
			},
			source: SourceRepaired,
			stage:  "bbox_alignment_repair",
		},
		{
			name: "regional repair keeps repaired source",
			chunk: map[string]any{
				"source": "regional_repair",
				"payload": map[string]any{
					"regional_fallback": true,
					"repair":            "regional_bbox_alignment",
				},
			},
			source: SourceRepaired,
			stage:  "regional_ocr_vlm",
		},
		{
			name: "vlm",
			chunk: map[string]any{
				"source": "regional_vlm",
			},
			source: SourceVLM,
			stage:  "regional_ocr_vlm",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ApplyChunkProvenance(tc.chunk, "")
			if tc.chunk["source"] != tc.source {
				t.Fatalf("source=%v want=%v chunk=%+v", tc.chunk["source"], tc.source, tc.chunk)
			}
			if tc.chunk["pipeline_stage"] != tc.stage {
				t.Fatalf("stage=%v want=%v chunk=%+v", tc.chunk["pipeline_stage"], tc.stage, tc.chunk)
			}
			prov := tc.chunk["provenance"].(map[string]any)
			if prov["source"] != tc.source || prov["stage"] != tc.stage {
				t.Fatalf("provenance mismatch: %+v", prov)
			}
		})
	}
}

func TestSortElementsByReadingOrder_KeepsWideTitleBeforeColumns(t *testing.T) {
	elements := []Element{
		testElement("right", 0.66, 0.94, 0.20, 0.28, 0),
		testElement("title", 0.05, 0.95, 0.04, 0.08, 1),
		testElement("left", 0.05, 0.52, 0.20, 0.28, 2),
	}
	got := SortElementsByReadingOrder(elements)
	if got[0].ID != "title" || got[1].ID != "left" || got[2].ID != "right" {
		t.Fatalf("unexpected reading order: %+v", []string{got[0].ID, got[1].ID, got[2].ID})
	}
}

func testChunk(id, source string, page, order int, left, right, top, bottom float64) map[string]any {
	return map[string]any{
		"chunk_id":   id,
		"type":       "paragraph",
		"text":       id,
		"page_index": page,
		"order":      order,
		"source":     source,
		"bbox": map[string]any{
			"left": left, "right": right, "top": top, "bottom": bottom,
		},
	}
}

func testElement(id string, left, right, top, bottom float64, pos int) Element {
	return Element{
		ID:          id,
		Type:        "paragraph",
		PageIndex:   1,
		OriginalPos: pos,
		BBox: &BBox{
			Left: left, Right: right, Top: top, Bottom: bottom,
		},
	}
}
