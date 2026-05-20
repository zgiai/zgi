package pdf

import "testing"

func TestExtractGeomTextRuns_ProducesBBox(t *testing.T) {
	block := []byte("BT /F1 12 Tf 1 0 0 1 10 20 Tm (AB) Tj ET")
	runs := extractGeomTextRuns(block, map[string]cmapUnicodeMap{}, nil, nil)
	if len(runs) != 1 {
		t.Fatalf("runs=%d, want=1", len(runs))
	}
	if runs[0].bbox == nil {
		t.Fatal("expected bbox on geom run")
	}
	if !(runs[0].bbox.Right > runs[0].bbox.Left) {
		t.Fatalf("invalid horizontal bbox=%+v", runs[0].bbox)
	}
	if !(runs[0].bbox.Top > runs[0].bbox.Bottom) {
		t.Fatalf("invalid vertical bbox=%+v", runs[0].bbox)
	}
}

func TestGeomBBoxFromRuns_UnionsRunBoxes(t *testing.T) {
	got := geomBBoxFromRuns([]geomTextRun{
		{bbox: &TextBBox{Left: 10, Bottom: 20, Right: 30, Top: 40}},
		{bbox: &TextBBox{Left: 28, Bottom: 18, Right: 60, Top: 42}},
	})
	if got == nil {
		t.Fatal("expected bbox")
	}
	if got.Left != 10 || got.Bottom != 18 || got.Right != 60 || got.Top != 42 {
		t.Fatalf("bbox=%+v", got)
	}
}
