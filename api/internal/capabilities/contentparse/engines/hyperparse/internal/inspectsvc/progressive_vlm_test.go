package inspectsvc

import "testing"

func TestFlattenProgressiveVLMPageItemsKeepsRenderOrder(t *testing.T) {
	results := map[int]progressiveVLMPageResult{
		2: {
			RenderIndex: 2,
			PageNumber:  4,
			DataURL:     "data:image/png;base64,page4",
			Items: []map[string]any{
				{"page_index": 4, "text": "page-4"},
			},
			Raw: "raw-4",
		},
		0: {
			RenderIndex: 0,
			PageNumber:  1,
			DataURL:     "data:image/png;base64,page1",
			Items: []map[string]any{
				{"page_index": 1, "text": "page-1"},
			},
			Raw: "raw-1",
		},
		1: {
			RenderIndex: 1,
			PageNumber:  3,
			DataURL:     "data:image/png;base64,page3",
			Items: []map[string]any{
				{"page_index": 3, "text": "page-3"},
			},
			Raw: "raw-3",
		},
	}

	items := flattenProgressiveVLMPageItems(results)
	if len(items) != 3 {
		t.Fatalf("items=%d", len(items))
	}
	if got := IntFromChunkAny(items[0], "page_index"); got != 1 {
		t.Fatalf("items[0].page_index=%d", got)
	}
	if got := IntFromChunkAny(items[1], "page_index"); got != 3 {
		t.Fatalf("items[1].page_index=%d", got)
	}
	if got := IntFromChunkAny(items[2], "page_index"); got != 4 {
		t.Fatalf("items[2].page_index=%d", got)
	}

	raw := joinProgressiveVLMRawReplies(results)
	if raw != "raw-1\n\nraw-3\n\nraw-4" {
		t.Fatalf("raw=%q", raw)
	}

	dataURLs, pageNumbers := rebuildProgressiveRenderedPages(results)
	if len(dataURLs) != 3 || len(pageNumbers) != 3 {
		t.Fatalf("rendered=%d/%d", len(dataURLs), len(pageNumbers))
	}
	if got := pageNumbers[0]; got != 1 {
		t.Fatalf("pageNumbers[0]=%d", got)
	}
	if got := pageNumbers[1]; got != 3 {
		t.Fatalf("pageNumbers[1]=%d", got)
	}
	if got := pageNumbers[2]; got != 4 {
		t.Fatalf("pageNumbers[2]=%d", got)
	}
}
