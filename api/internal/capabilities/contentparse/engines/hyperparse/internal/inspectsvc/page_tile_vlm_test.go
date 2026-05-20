package inspectsvc

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestOversizedPageSetFromFullDoc(t *testing.T) {
	fullDoc := map[string]any{
		"document": map[string]any{
			"layout": map[string]any{
				"pages": []map[string]any{
					{"page_index": 1, "media_box": "0 0 595 842"},
					{"page_index": 2, "media_box": "0 0 3213 5712"},
				},
			},
		},
	}
	got := oversizedPageSetFromFullDoc(fullDoc)
	if len(got) != 1 || len(got[2].Tiles) == 0 {
		t.Fatalf("oversizedPageSetFromFullDoc=%v, want only page 2", got)
	}
	if first := got[2].Tiles[0].Coverage; first != (normalizedPageRect{Left: 0, Top: 0, Right: 1, Bottom: 0.5}) {
		t.Fatalf("portrait oversized tiles[0].coverage=%+v", first)
	}
}

func TestCallStructuredVLMFallbackForRenderedPageTiled(t *testing.T) {
	dataURL := mustPNGDataURL(t, 1200, 800)
	callCount := 0
	caller := func(images []map[string]any) (string, []map[string]any, string, VLMCallTimingBreakdown, error) {
		callCount++
		return "stub-model", []map[string]any{
			{
				"type":         "paragraph",
				"page_index":   1,
				"order":        1,
				"source_trace": "vlm:page#1",
				"text":         "tile text",
				"bbox": map[string]any{
					"left":   0.0,
					"top":    0.0,
					"right":  1.0,
					"bottom": 1.0,
				},
			},
		}, "{}", VLMCallTimingBreakdown{MainRequestMs: 10}, nil
	}

	plan := oversizedPagePlan{
		WidthPt:  3213,
		HeightPt: 5712,
		Tiles:    buildOversizedPageTiles(5, 3213, 5712),
	}
	model, items, raw, err := callStructuredVLMFallbackForRenderedPage(dataURL, 5, plan, caller)
	if err != nil {
		t.Fatalf("callStructuredVLMFallbackForRenderedPage err=%v", err)
	}
	if model != "stub-model" {
		t.Fatalf("model=%q, want stub-model", model)
	}
	if raw != "{}\n\n{}" {
		t.Fatalf("raw=%q", raw)
	}
	if callCount != 2 {
		t.Fatalf("callCount=%d, want 2", callCount)
	}
	if len(items) != 2 {
		t.Fatalf("len(items)=%d, want 2", len(items))
	}

	left1, top1, right1, bottom1, ok := chunkBBox(items[0])
	if !ok {
		t.Fatal("items[0] missing bbox")
	}
	if !approxFloat(left1, 0.0) || !approxFloat(top1, 0.0) || !approxFloat(right1, 1.0) || !approxFloat(bottom1, 0.54) {
		t.Fatalf("items[0] bbox=(%.2f, %.2f, %.2f, %.2f), want approx (0,0,1,0.54)", left1, top1, right1, bottom1)
	}
	left2, top2, right2, bottom2, ok := chunkBBox(items[1])
	if !ok {
		t.Fatal("items[1] missing bbox")
	}
	if !approxFloat(left2, 0.0) || !approxFloat(top2, 0.46) || !approxFloat(right2, 1.0) || !approxFloat(bottom2, 1.0) {
		t.Fatalf("items[1] bbox=(%.2f, %.2f, %.2f, %.2f), want approx (0,0.46,1,1)", left2, top2, right2, bottom2)
	}
	if got := IntFromChunkAny(items[0], "page_index"); got != 5 {
		t.Fatalf("items[0].page_index=%d, want 5", got)
	}
	if got := IntFromChunkAny(items[1], "order"); got != 10001 {
		t.Fatalf("items[1].order=%d, want 10001", got)
	}
	payload1, _ := items[0]["payload"].(map[string]any)
	tileRef1, _ := payload1["tile_ref"].(map[string]any)
	if got := IntFromChunkAny(tileRef1, "tile_index"); got != 1 {
		t.Fatalf("tile_ref[0].tile_index=%d, want 1", got)
	}
	payload2, _ := items[1]["payload"].(map[string]any)
	tileRef2, _ := payload2["tile_ref"].(map[string]any)
	if got := IntFromChunkAny(tileRef2, "tile_index"); got != 2 {
		t.Fatalf("tile_ref[1].tile_index=%d, want 2", got)
	}
}

func TestDedupeTileChunks(t *testing.T) {
	items := []map[string]any{
		{
			"page_index": 1,
			"order":      2,
			"type":       "paragraph",
			"text":       "生活中经常会遇见诸如奶粉中各种营养成分的配比、地图绘制时的比例尺、银行存款利率、商品的折扣等概念。我们将通过学习比与比例的相关知识来理解这些现象。",
			"payload": map[string]any{
				"tile_ref": map[string]any{"tile_index": 1},
			},
		},
		{
			"page_index": 1,
			"order":      3,
			"type":       "paragraph",
			"text":       "生活中经常会遇见诸如奶粉中各种营养成分的配比、地图绘制时的比例尺、银行存款利率、商品的折扣等概念. 我们将通过学习比与比例的知识来理解这些概念.",
			"payload": map[string]any{
				"tile_ref": map[string]any{"tile_index": 2},
			},
		},
		{
			"page_index": 1,
			"order":      4,
			"type":       "paragraph",
			"text":       "通过本章的学习，我们不仅能掌握小数、分数与百分数之间的互化。",
			"payload": map[string]any{
				"tile_ref": map[string]any{"tile_index": 2},
			},
		},
	}

	got := dedupeTileChunks(items)
	if len(got) != 2 {
		t.Fatalf("len(got)=%d, want 2", len(got))
	}
	if got[0]["text"] != items[0]["text"] {
		t.Fatalf("kept duplicate text=%q", got[0]["text"])
	}
	if IntFromChunkAny(got[0], "order") != 2 {
		t.Fatalf("deduped first order=%d, want 2", IntFromChunkAny(got[0], "order"))
	}
}

func mustPNGDataURL(t *testing.T, width, height int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}
