package inspectsvc

import "testing"

func TestRemapVLMChunkPages(t *testing.T) {
	items := []map[string]any{
		{"page_index": 1, "source_trace": "vlm:page#1"},
		{"page_index": 2, "source_trace": "vlm:page#2"},
		{"page_index": 4, "source_trace": "vlm:page#4"},
	}

	RemapVLMChunkPages(items, []int{2, 5, 8})

	want := []int{2, 5, 8}
	for i, item := range items {
		if got := IntFromChunkAny(item, "page_index"); got != want[i] {
			t.Fatalf("item[%d].page_index=%d, want %d", i, got, want[i])
		}
	}
	if got, _ := items[2]["source_trace"].(string); got != "vlm:page#8" {
		t.Fatalf("item[2].source_trace=%q", got)
	}
}
