package chunking

import (
	"strings"
	"testing"
)

func TestHTMLTableWithIDsFromPayload(t *testing.T) {
	p := map[string]any{
		"row_count":    2,
		"column_count": 2,
		"cells": []any{
			map[string]any{"row": 0, "col": 0, "text": "A"},
			map[string]any{"row": 0, "col": 1, "text": "B"},
			map[string]any{"row": 1, "col": 0, "text": "1"},
			map[string]any{"row": 1, "col": 1, "text": "2"},
		},
	}
	html, refs := HTMLTableWithIDsFromPayload(p, 0)
	if !strings.Contains(html, `<table id="0-1">`) {
		t.Fatalf("missing table id: %s", html)
	}
	if !strings.Contains(html, `<td id="0-2">`) || !strings.Contains(html, "A") {
		t.Fatalf("missing cell: %s", html)
	}
	if len(refs) != 0 {
		t.Fatalf("expected no grounding without bbox, got %d", len(refs))
	}
}

func TestHTMLTableWithIDsFromGrid(t *testing.T) {
	grid := [][]string{{"A", "B"}, {"1", "2"}}
	html := HTMLTableWithIDsFromGrid(0, grid)
	if !strings.Contains(html, `<table id="0-1">`) || !strings.Contains(html, `<td id="0-5">`) {
		t.Fatalf("%s", html)
	}
}

func TestHTMLTableWithIDsFromPayload_FlipsCellBBoxToTopLeft(t *testing.T) {
	p := map[string]any{
		"row_count":    1,
		"column_count": 1,
		"cells": []any{
			map[string]any{
				"row":  0,
				"col":  0,
				"text": "A",
				"bbox": map[string]any{
					"left":   0.1,
					"right":  0.2,
					"top":    0.62,
					"bottom": 0.57,
				},
			},
		},
	}
	_, refs := HTMLTableWithIDsFromPayload(p, 0)
	if len(refs) != 1 {
		t.Fatalf("expected one grounding ref, got %d", len(refs))
	}
	ref := refs["0-2"]
	box, _ := ref["box"].(map[string]any)
	if box == nil {
		t.Fatalf("missing box: %+v", ref)
	}
	if box["top"] != 0.38 || box["bottom"] != 0.43 {
		t.Fatalf("expected flipped cell bbox, got %+v", box)
	}
}
