package inspectsvc

import "testing"

func TestHasImageChunksForPages(t *testing.T) {
	fullDoc := map[string]any{
		"chunks": map[string]any{
			"items": []map[string]any{
				{"type": "image", "page_index": 2},
				{"type": "paragraph", "page_index": 1},
			},
		},
	}

	if !HasImageChunks(fullDoc) {
		t.Fatalf("HasImageChunks=false, want true")
	}
	if HasImageChunksForPages(fullDoc, []int{1}) {
		t.Fatalf("HasImageChunksForPages(page=1)=true, want false")
	}
	if !HasImageChunksForPages(fullDoc, []int{2}) {
		t.Fatalf("HasImageChunksForPages(page=2)=false, want true")
	}
	if HasImageChunksForPages(fullDoc, []int{}) {
		t.Fatalf("HasImageChunksForPages(empty)=true, want false")
	}
}
