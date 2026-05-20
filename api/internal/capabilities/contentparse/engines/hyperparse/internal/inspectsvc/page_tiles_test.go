package inspectsvc

import (
	"math"
	"testing"
)

func TestSplitPageIntoHorizontalTiles(t *testing.T) {
	tiles := SplitPageIntoHorizontalTiles(7, 2, 0.05)
	if len(tiles) != 2 {
		t.Fatalf("len(tiles)=%d, want 2", len(tiles))
	}

	if got := tiles[0].PageIndex; got != 7 {
		t.Fatalf("tiles[0].PageIndex=%d, want 7", got)
	}
	if got := tiles[0].TileIndex; got != 1 {
		t.Fatalf("tiles[0].TileIndex=%d, want 1", got)
	}
	if got := tiles[1].TileIndex; got != 2 {
		t.Fatalf("tiles[1].TileIndex=%d, want 2", got)
	}

	if got := tiles[0].Coverage; got != (normalizedPageRect{Left: 0, Top: 0, Right: 0.5, Bottom: 1}) {
		t.Fatalf("tiles[0].Coverage=%+v", got)
	}
	if got := tiles[0].Crop; got != (normalizedPageRect{Left: 0, Top: 0, Right: 0.55, Bottom: 1}) {
		t.Fatalf("tiles[0].Crop=%+v", got)
	}
	if got := tiles[1].Coverage; got != (normalizedPageRect{Left: 0.5, Top: 0, Right: 1, Bottom: 1}) {
		t.Fatalf("tiles[1].Coverage=%+v", got)
	}
	if got := tiles[1].Crop; got != (normalizedPageRect{Left: 0.45, Top: 0, Right: 1, Bottom: 1}) {
		t.Fatalf("tiles[1].Crop=%+v", got)
	}
}

func TestNormalizedPageRectMapNormalizedBox(t *testing.T) {
	rect := normalizedPageRect{Left: 0.45, Top: 0.10, Right: 1.00, Bottom: 0.90}
	got, ok := rect.mapNormalizedBox(0.25, 0.10, 0.75, 0.90)
	if !ok {
		t.Fatal("mapNormalizedBox returned ok=false")
	}
	want := normalizedPageRect{
		Left:   0.5875,
		Top:    0.18,
		Right:  0.8625,
		Bottom: 0.82,
	}
	if !approxRect(got, want) {
		t.Fatalf("mapped=%+v, want %+v", got, want)
	}
}

func TestRemapChunkBBoxFromTile(t *testing.T) {
	tile := pageTileRef{
		PageIndex: 2,
		TileIndex: 3,
		Crop:      normalizedPageRect{Left: 0.5, Top: 0.2, Right: 1, Bottom: 0.8},
		Coverage:  normalizedPageRect{Left: 0.55, Top: 0.2, Right: 1, Bottom: 0.8},
	}
	chunk := map[string]any{
		"bbox": map[string]any{
			"left":   0.10,
			"top":    0.25,
			"right":  0.90,
			"bottom": 0.75,
		},
	}
	if ok := RemapChunkBBoxFromTile(chunk, tile); !ok {
		t.Fatal("RemapChunkBBoxFromTile returned ok=false")
	}

	left, top, right, bottom, ok := chunkBBox(chunk)
	if !ok {
		t.Fatal("chunkBBox returned ok=false")
	}
	if !approxFloat(left, 0.55) || !approxFloat(top, 0.35) || !approxFloat(right, 0.95) || !approxFloat(bottom, 0.65) {
		t.Fatalf("bbox=(%.2f, %.2f, %.2f, %.2f), want (0.55, 0.35, 0.95, 0.65)", left, top, right, bottom)
	}

	payload, _ := chunk["payload"].(map[string]any)
	if payload == nil {
		t.Fatal("payload is nil")
	}
	tileRef, _ := payload["tile_ref"].(map[string]any)
	if tileRef == nil {
		t.Fatal("payload.tile_ref is nil")
	}
	if got := IntFromChunkAny(tileRef, "page_index"); got != 2 {
		t.Fatalf("tile_ref.page_index=%d, want 2", got)
	}
	if got := IntFromChunkAny(tileRef, "tile_index"); got != 3 {
		t.Fatalf("tile_ref.tile_index=%d, want 3", got)
	}
}

func approxRect(a, b normalizedPageRect) bool {
	return approxFloat(a.Left, b.Left) &&
		approxFloat(a.Top, b.Top) &&
		approxFloat(a.Right, b.Right) &&
		approxFloat(a.Bottom, b.Bottom)
}

func approxFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
