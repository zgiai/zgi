package pdf

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseVLMChunksJSON_plainAndFence(t *testing.T) {
	raw := `{"chunks":[{"type":"table","page_index":1,"text":"A|B","order":0}]}`
	got, err := ParseVLMChunksJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0]["type"] != "table" || got[0]["source"] != "vlm" {
		t.Fatalf("%+v", got[0])
	}
	if got[0]["text"] != "A|B" {
		t.Fatalf("text=%q", got[0]["text"])
	}
	fenced := "```json\n" + raw + "\n```\n"
	got2, err := ParseVLMChunksJSON(fenced)
	if err != nil {
		t.Fatal(err)
	}
	if len(got2) != 1 || got2[0]["text"] != "A|B" {
		t.Fatalf("%+v", got2[0])
	}
}

func TestParseVLMChunksJSON_tableMarkdownToHTML(t *testing.T) {
	raw := "{\"chunks\":[{\"type\":\"table\",\"page_index\":1,\"text\":\"| Name | Val |\\n| --- | --- |\\n| a | 1 |\\n\"}]}"
	got, err := ParseVLMChunksJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	txt := got[0]["text"].(string)
	if !strings.Contains(txt, `<table id="0-1">`) || !strings.Contains(txt, `<td id="0-2">`) {
		t.Fatalf("expected HTML table ids, got: %s", txt)
	}
	if !strings.Contains(txt, "Name") || !strings.Contains(txt, "a") {
		t.Fatalf("missing cell text: %s", txt)
	}
}

func TestParseVLMChunksJSON_tablePayloadToHTML(t *testing.T) {
	raw := `{"chunks":[{"type":"table","page_index":2,"text":"ignored","payload":{"row_count":1,"column_count":2,"cells":[{"row":0,"col":0,"text":"X"},{"row":0,"col":1,"text":"Y"}]}}]}`
	got, err := ParseVLMChunksJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	txt := got[0]["text"].(string)
	// page_index 2 -> page0 1 -> table id 1-1
	if !strings.Contains(txt, `<table id="1-1">`) || !strings.Contains(txt, "X") {
		t.Fatalf("got: %s", txt)
	}
}

func TestParseVLMChunksJSON_typeAliases(t *testing.T) {
	raw := `{"chunks":[
		{"type":"text","page_index":1,"text":"p"},
		{"type":"math","page_index":1,"text":"E=mc^2"},
		{"type":"figure","page_index":2,"text":"[fig]"}
	]}`
	got, err := ParseVLMChunksJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0]["type"] != "paragraph" || got[1]["type"] != "formula" || got[2]["type"] != "image" {
		t.Fatalf("%v %v %v", got[0]["type"], got[1]["type"], got[2]["type"])
	}
}

func TestParseVLMChunksJSON_errors(t *testing.T) {
	for _, s := range []string{
		"",
		"{}",
		`{"chunks":[]}`,
		`{"chunks":"bad"}`,
	} {
		if _, err := ParseVLMChunksJSON(s); err == nil {
			t.Fatalf("expected error for %q", s)
		}
	}
}

func TestVLMChunksFallbackSingle(t *testing.T) {
	got := VLMChunksFallbackSingle("  hello  ", 3)
	if len(got) != 1 || got[0]["text"] != "hello" {
		t.Fatalf("%+v", got[0])
	}
	if VLMChunksFallbackSingle("   ", 1) != nil {
		t.Fatal("expected nil")
	}
}

func TestMergeNativeAndVLMChunkItems(t *testing.T) {
	native := []map[string]any{
		{"type": "paragraph", "page_index": 1, "order": 0, "text": "native body", "chunk_id": "n1"},
		{"type": "bookmark", "page_index": 1, "order": 1, "text": "TOC", "chunk_id": "b1"},
		{"type": "stamp", "page_index": 1, "order": 2, "chunk_id": "stamp_0"},
	}
	vlm := []map[string]any{
		{"type": "paragraph", "page_index": 1, "order": 0, "text": "vlm p", "chunk_id": "v1", "source": "vlm"},
	}
	merged := MergeNativeAndVLMChunkItems(native, vlm)
	if len(merged) != 3 {
		t.Fatalf("len=%d %+v", len(merged), merged)
	}
	var kinds []string
	for _, c := range merged {
		kinds = append(kinds, c["type"].(string)+":"+strings.TrimSpace(c["vlm_merge"].(string)))
	}
	if kinds[0] != "bookmark:kept_native" || kinds[1] != "stamp:kept_native" || kinds[2] != "paragraph:from_vlm" {
		t.Fatalf("%v", kinds)
	}
	for i := range merged {
		if merged[i]["order"] != i {
			t.Fatalf("order[%d]=%v", i, merged[i]["order"])
		}
	}
}

func TestMergeNativeAndVLMChunkItemsForPages(t *testing.T) {
	native := []map[string]any{
		{"type": "paragraph", "page_index": 1, "order": 0, "text": "native p1", "chunk_id": "n1"},
		{"type": "paragraph", "page_index": 2, "order": 1, "text": "native p2", "chunk_id": "n2"},
		{"type": "bookmark", "page_index": 2, "order": 2, "text": "TOC", "chunk_id": "b2"},
		{"type": "paragraph", "page_index": 3, "order": 3, "text": "native p3", "chunk_id": "n3"},
	}
	vlm := []map[string]any{
		{"type": "paragraph", "page_index": 2, "order": 0, "text": "vlm p2", "chunk_id": "v2", "source": "vlm"},
	}
	merged, stats := MergeNativeAndVLMChunkItemsForPages(native, vlm, []int{2})
	if len(merged) != 4 {
		t.Fatalf("len=%d %+v", len(merged), merged)
	}
	if stats.NativeKeptCount != 3 || stats.VLMMergedCount != 1 {
		t.Fatalf("stats=%+v", stats)
	}
	if len(stats.VLMPagesApplied) != 1 || stats.VLMPagesApplied[0] != 2 {
		t.Fatalf("applied=%v", stats.VLMPagesApplied)
	}
	got := make([]string, 0, len(merged))
	for _, c := range merged {
		got = append(got, c["text"].(string))
	}
	want := []string{"native p1", "TOC", "vlm p2", "native p3"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got=%v want=%v", got, want)
	}
}

func TestMergeNativeAndVLMChunkItemsForPagesKeepsNativeWhenPageHasNoVLM(t *testing.T) {
	native := []map[string]any{
		{"type": "paragraph", "page_index": 2, "order": 0, "text": "native p2", "chunk_id": "n2"},
	}
	vlm := []map[string]any{
		{"type": "paragraph", "page_index": 3, "order": 0, "text": "vlm p3", "chunk_id": "v3", "source": "vlm"},
	}
	merged, stats := MergeNativeAndVLMChunkItemsForPages(native, vlm, []int{2})
	if len(merged) != 1 || merged[0]["text"] != "native p2" {
		t.Fatalf("merged=%+v", merged)
	}
	if stats.NativeKeptCount != 1 || stats.VLMMergedCount != 0 {
		t.Fatalf("stats=%+v", stats)
	}
}

func TestMergeNativeAndVLMChunkItemsForPagesKeepsNovelNativeResidualText(t *testing.T) {
	native := []map[string]any{
		{"type": "paragraph", "page_index": 1, "order": 0, "text": "Your electricity bill at a glance", "chunk_id": "n1"},
		{"type": "paragraph", "page_index": 1, "order": 1, "text": "Your account number 951857880", "chunk_id": "n2"},
		{"type": "paragraph", "page_index": 1, "order": 2, "text": "To ask about this bill call 1800 372 372", "chunk_id": "n3"},
	}
	vlm := []map[string]any{
		{"type": "heading", "page_index": 1, "order": 0, "text": "Your electricity bill at a glance", "chunk_id": "v1", "source": "vlm"},
	}

	merged, stats := MergeNativeAndVLMChunkItemsForPages(native, vlm, []int{1})
	if stats.NativeResidualKept != 2 {
		t.Fatalf("stats.NativeResidualKept=%d", stats.NativeResidualKept)
	}

	got := make([]string, 0, len(merged))
	kinds := make([]string, 0, len(merged))
	for _, c := range merged {
		got = append(got, c["text"].(string))
		kinds = append(kinds, c["type"].(string)+":"+strings.TrimSpace(c["vlm_merge"].(string)))
	}
	if strings.Join(got, "|") != "Your account number 951857880|To ask about this bill call 1800 372 372|Your electricity bill at a glance" {
		t.Fatalf("got=%v", got)
	}
	if strings.Join(kinds, "|") != "paragraph:kept_native_residual|paragraph:kept_native_residual|heading:from_vlm" {
		t.Fatalf("kinds=%v", kinds)
	}
}

func TestRebuildTextSummaryAfterVLMMerge(t *testing.T) {
	doc := map[string]any{
		"text_summary": map[string]any{
			"combined_text": "OLD",
		},
	}
	merged := []map[string]any{
		{"type": "paragraph", "page_index": 1, "order": 1, "text": "second"},
		{"type": "paragraph", "page_index": 1, "order": 0, "text": "first"},
	}
	RebuildTextSummaryAfterVLMMerge(doc, merged)
	ts := doc["text_summary"].(map[string]any)
	if ts["native_combined_text"] != "OLD" {
		t.Fatalf("native=%v", ts["native_combined_text"])
	}
	if ts["combined_text"] != "first\nsecond" {
		t.Fatalf("combined=%q", ts["combined_text"])
	}
	if ts["recognition_source"] != "vlm" {
		t.Fatalf("%v", ts["recognition_source"])
	}
}

func TestMergeNativeAndVLMChunkItemsForPagesAlignsTextBBoxToNative(t *testing.T) {
	nativeBBox := map[string]any{
		"left":   0.12,
		"right":  0.68,
		"top":    0.82,
		"bottom": 0.74,
	}
	native := []map[string]any{
		{
			"type":       "paragraph",
			"page_index": 1,
			"order":      0,
			"chunk_id":   "n_text_1",
			"text":       "第六章 实验结果及其分析",
			"bbox":       nativeBBox,
		},
	}
	vlm := []map[string]any{
		{
			"type":       "paragraph",
			"page_index": 1,
			"order":      0,
			"chunk_id":   "v_text_1",
			"text":       "第六章实验结果及其分析",
			"source":     "vlm",
			"bbox": map[string]any{
				"left":   0.05,
				"right":  0.95,
				"top":    0.10,
				"bottom": 0.26,
			},
		},
	}

	merged, _ := MergeNativeAndVLMChunkItemsForPages(native, vlm, []int{1})
	if len(merged) != 1 {
		t.Fatalf("len=%d merged=%+v", len(merged), merged)
	}
	gotBBox, _ := merged[0]["bbox"].(map[string]any)
	if gotBBox == nil {
		t.Fatalf("missing bbox: %+v", merged[0])
	}
	if gotBBox["left"] != nativeBBox["left"] || gotBBox["right"] != nativeBBox["right"] ||
		gotBBox["top"] != nativeBBox["top"] || gotBBox["bottom"] != nativeBBox["bottom"] {
		t.Fatalf("bbox=%v want=%v", gotBBox, nativeBBox)
	}
	if merged[0]["bbox_source"] != "native_text_align" {
		t.Fatalf("bbox_source=%v", merged[0]["bbox_source"])
	}
	if precise, _ := merged[0]["bbox_precise"].(bool); !precise {
		t.Fatalf("bbox_precise=%v", merged[0]["bbox_precise"])
	}
	payload, _ := merged[0]["payload"].(map[string]any)
	if payload == nil || payload["bbox_aligned_from_chunk_id"] != "n_text_1" {
		t.Fatalf("payload=%v", payload)
	}
	if rawBBox, _ := payload["vlm_raw_bbox"].(map[string]any); rawBBox == nil {
		t.Fatalf("missing vlm_raw_bbox payload=%v", payload)
	}
}

func TestMergeNativeAndVLMChunkItemsForPagesAlignsTableBBoxToNative(t *testing.T) {
	nativeBBox := map[string]any{
		"left":   0.10,
		"right":  0.88,
		"top":    0.63,
		"bottom": 0.41,
	}
	native := []map[string]any{
		{
			"type":       "table",
			"page_index": 1,
			"order":      0,
			"chunk_id":   "n_table_1",
			"text":       "<table><tr><td>参数名称</td><td>参数值</td></tr></table>",
			"bbox":       nativeBBox,
		},
	}
	vlm := []map[string]any{
		{
			"type":       "table",
			"page_index": 1,
			"order":      0,
			"chunk_id":   "v_table_1",
			"text":       "<table><tr><td>参数名称</td><td>参数值</td></tr></table>",
			"source":     "vlm",
		},
	}

	merged, _ := MergeNativeAndVLMChunkItemsForPages(native, vlm, []int{1})
	if len(merged) != 1 {
		t.Fatalf("len=%d merged=%+v", len(merged), merged)
	}
	gotBBox, _ := merged[0]["bbox"].(map[string]any)
	if gotBBox == nil {
		t.Fatalf("missing bbox: %+v", merged[0])
	}
	if gotBBox["left"] != nativeBBox["left"] || gotBBox["right"] != nativeBBox["right"] ||
		gotBBox["top"] != nativeBBox["top"] || gotBBox["bottom"] != nativeBBox["bottom"] {
		t.Fatalf("bbox=%v want=%v", gotBBox, nativeBBox)
	}
	if merged[0]["bbox_source"] != "native_table_align" {
		t.Fatalf("bbox_source=%v", merged[0]["bbox_source"])
	}
	if precise, _ := merged[0]["bbox_precise"].(bool); !precise {
		t.Fatalf("bbox_precise=%v", merged[0]["bbox_precise"])
	}
}

func TestMergeNativeAndVLMChunkItemsForPagesKeepsRawVLMBBoxWhenNoNativeMatch(t *testing.T) {
	rawBBox := map[string]any{
		"left":   0.22,
		"right":  0.44,
		"top":    0.12,
		"bottom": 0.28,
	}
	native := []map[string]any{
		{
			"type":       "paragraph",
			"page_index": 1,
			"order":      0,
			"chunk_id":   "n_text_1",
			"text":       "完全不同的原生文本",
			"bbox": map[string]any{
				"left":   0.1,
				"right":  0.2,
				"top":    0.8,
				"bottom": 0.7,
			},
		},
	}
	vlm := []map[string]any{
		{
			"type":       "paragraph",
			"page_index": 1,
			"order":      0,
			"chunk_id":   "v_text_1",
			"text":       "this text should keep its original box",
			"source":     "vlm",
			"bbox":       rawBBox,
		},
	}

	merged, _ := MergeNativeAndVLMChunkItemsForPages(native, vlm, []int{1})
	if len(merged) != 1 {
		t.Fatalf("len=%d merged=%+v", len(merged), merged)
	}
	gotBBox, _ := merged[0]["bbox"].(map[string]any)
	if gotBBox == nil {
		t.Fatalf("missing bbox: %+v", merged[0])
	}
	if gotBBox["left"] != rawBBox["left"] || gotBBox["right"] != rawBBox["right"] ||
		gotBBox["top"] != rawBBox["top"] || gotBBox["bottom"] != rawBBox["bottom"] {
		t.Fatalf("bbox=%v want=%v", gotBBox, rawBBox)
	}
	if merged[0]["bbox_source"] != "vlm_bbox_raw" {
		t.Fatalf("bbox_source=%v", merged[0]["bbox_source"])
	}
	if precise, _ := merged[0]["bbox_precise"].(bool); precise {
		t.Fatalf("bbox_precise=%v", merged[0]["bbox_precise"])
	}
}

func TestMergeNativeAndVLMChunkItemsForPagesUsesOCRBBoxAnchorWithoutKeepingIt(t *testing.T) {
	anchorBBox := map[string]any{
		"left":   0.11,
		"right":  0.55,
		"top":    0.20,
		"bottom": 0.24,
	}
	native := []map[string]any{
		{
			"type":       "bbox_anchor",
			"page_index": 1,
			"order":      0,
			"chunk_id":   "ocr_bbox_anchor_p1_0",
			"text":       "Subtotal 123.45",
			"bbox":       anchorBBox,
			"payload":    map[string]any{"bbox_anchor_only": true},
		},
	}
	vlm := []map[string]any{
		{
			"type":       "kv",
			"page_index": 1,
			"order":      0,
			"chunk_id":   "v_kv_1",
			"text":       "Subtotal: 123.45",
			"source":     "vlm",
		},
	}

	merged, _ := MergeNativeAndVLMChunkItemsForPages(native, vlm, []int{1})
	if len(merged) != 1 {
		t.Fatalf("anchor should not be kept as output chunk, len=%d merged=%+v", len(merged), merged)
	}
	if merged[0]["type"] != "kv" {
		t.Fatalf("expected VLM chunk output, got %+v", merged[0])
	}
	gotBBox, _ := merged[0]["bbox"].(map[string]any)
	if gotBBox == nil || gotBBox["left"] != anchorBBox["left"] || gotBBox["right"] != anchorBBox["right"] {
		t.Fatalf("bbox=%v want=%v", gotBBox, anchorBBox)
	}
	if merged[0]["bbox_source"] != "ocr_bbox_anchor" {
		t.Fatalf("bbox_source=%v", merged[0]["bbox_source"])
	}
	payload, _ := merged[0]["payload"].(map[string]any)
	if payload == nil || payload["bbox_aligned_from_chunk_id"] != "ocr_bbox_anchor_p1_0" {
		t.Fatalf("payload=%v", payload)
	}
	if payload["bbox_source"] != "ocr_bbox_anchor" || payload["bbox_precise"] != true {
		t.Fatalf("bbox payload provenance=%v", payload)
	}
}

func TestMergeNativeAndVLMChunkItemsForPagesUnionsOCRBBoxAnchorsForLongVLMChunk(t *testing.T) {
	native := []map[string]any{
		{
			"type":       "bbox_anchor",
			"page_index": 1,
			"order":      0,
			"chunk_id":   "ocr_bbox_anchor_p1_0",
			"text":       "Guest Name Jens Walter",
			"bbox":       map[string]any{"left": 0.10, "right": 0.45, "top": 0.20, "bottom": 0.23},
		},
		{
			"type":       "bbox_anchor",
			"page_index": 1,
			"order":      1,
			"chunk_id":   "ocr_bbox_anchor_p1_1",
			"text":       "Room No 305",
			"bbox":       map[string]any{"left": 0.10, "right": 0.30, "top": 0.25, "bottom": 0.28},
		},
		{
			"type":       "bbox_anchor",
			"page_index": 1,
			"order":      2,
			"chunk_id":   "ocr_bbox_anchor_p1_2",
			"text":       "Arrival Date 30.11.19",
			"bbox":       map[string]any{"left": 0.10, "right": 0.52, "top": 0.30, "bottom": 0.33},
		},
	}
	vlm := []map[string]any{
		{
			"type":       "paragraph",
			"page_index": 1,
			"order":      0,
			"chunk_id":   "v_guest_block",
			"text":       "Guest Name: Jens Walter\nRoom No.: 305\nArrival Date: 30.11.19",
			"source":     "vlm",
		},
	}

	merged, _ := MergeNativeAndVLMChunkItemsForPages(native, vlm, []int{1})
	if len(merged) != 1 {
		t.Fatalf("len=%d merged=%+v", len(merged), merged)
	}
	gotBBox, _ := merged[0]["bbox"].(map[string]any)
	if gotBBox == nil {
		t.Fatalf("missing bbox: %+v", merged[0])
	}
	if gotBBox["left"] != 0.10 || gotBBox["right"] != 0.52 || gotBBox["top"] != 0.20 || gotBBox["bottom"] != 0.33 {
		t.Fatalf("expected union bbox, got=%v", gotBBox)
	}
	payload, _ := merged[0]["payload"].(map[string]any)
	if payload == nil || !strings.Contains(fmt.Sprint(payload["bbox_aligned_from_chunk_id"]), "ocr_bbox_anchor_p1_2") {
		t.Fatalf("payload=%v", payload)
	}
	if merged[0]["bbox_source"] != "ocr_bbox_anchor_union" {
		t.Fatalf("bbox_source=%v", merged[0]["bbox_source"])
	}
}

func TestMergeNativeAndVLMChunkItemsForPagesMarksTableOCRAnchorUnionSource(t *testing.T) {
	native := []map[string]any{
		{
			"type":       "bbox_anchor",
			"page_index": 1,
			"order":      0,
			"chunk_id":   "ocr_bbox_anchor_p1_0",
			"text":       "VAT Detail Net EUR",
			"bbox":       map[string]any{"left": 0.10, "right": 0.40, "top": 0.20, "bottom": 0.23},
		},
		{
			"type":       "bbox_anchor",
			"page_index": 1,
			"order":      1,
			"chunk_id":   "ocr_bbox_anchor_p1_1",
			"text":       "VAT 19% 10.00",
			"bbox":       map[string]any{"left": 0.10, "right": 0.42, "top": 0.25, "bottom": 0.28},
		},
	}
	vlm := []map[string]any{
		{
			"type":       "table",
			"page_index": 1,
			"order":      0,
			"chunk_id":   "v_table_1",
			"text":       "<table><tr><td>VAT Detail</td><td>Net EUR</td></tr><tr><td>VAT 19%</td><td>10.00</td></tr></table>",
			"source":     "vlm",
		},
	}

	merged, _ := MergeNativeAndVLMChunkItemsForPages(native, vlm, []int{1})
	if len(merged) != 1 {
		t.Fatalf("len=%d merged=%+v", len(merged), merged)
	}
	if merged[0]["bbox_source"] != "ocr_table_bbox_anchor_union" {
		t.Fatalf("bbox_source=%v", merged[0]["bbox_source"])
	}
}
