package inspectsvc

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestLowConfidenceRegionsFromFullDoc(t *testing.T) {
	fullDoc := map[string]any{
		"document": map[string]any{
			"local_quality": map[string]any{
				"low_confidence_regions": []map[string]any{
					{
						"id":                "r1",
						"page_index":        2,
						"reason":            "bbox_mismatch_geometry_lines",
						"route":             "ocr_vlm_region",
						"text_preview":      "测试区域",
						"anchor_chunk_id":   "chunk-1",
						"confidence":        0.64,
						"bbox":              map[string]any{"left": 0.10, "right": 0.80, "top": 0.20, "bottom": 0.30},
						"ignored_extra_key": true,
					},
				},
			},
		},
	}

	regions := lowConfidenceRegionsFromFullDoc(fullDoc)
	if len(regions) != 1 {
		t.Fatalf("expected one region, got %+v", regions)
	}
	if regions[0].ID != "r1" || regions[0].PageIndex != 2 || regions[0].AnchorChunkID != "chunk-1" {
		t.Fatalf("region not normalized: %+v", regions[0])
	}
	if regions[0].BBox["right"] != 0.80 {
		t.Fatalf("bbox not parsed: %+v", regions[0].BBox)
	}
}

func TestMergeRegionalFallbackChunksRepairsAnchorWithoutDoubleCounting(t *testing.T) {
	fullDoc := map[string]any{
		"chunks": map[string]any{
			"items": []map[string]any{
				{
					"chunk_id":   "anchor",
					"type":       "paragraph",
					"text":       "old text",
					"page_index": 1,
					"order":      1,
					"bbox":       map[string]any{"left": 0.40, "right": 0.50, "top": 0.20, "bottom": 0.24},
				},
			},
		},
	}
	region := lowConfidenceRegion{
		ID:            "region_p1_0",
		PageIndex:     1,
		Reason:        "bbox_mismatch_geometry_lines",
		AnchorChunkID: "anchor",
		BBox:          map[string]float64{"left": 0.10, "right": 0.82, "top": 0.18, "bottom": 0.28},
	}
	chunks := []map[string]any{{"type": "paragraph", "text": "new repaired text"}}

	added, merged, anchorRepair := mergeRegionalFallbackChunks(fullDoc, region, chunks, "regional_vlm", "qwen-vl", 0)
	if added != 0 || merged != 1 || anchorRepair != 0 {
		t.Fatalf("unexpected counts added=%d merged=%d anchor=%d", added, merged, anchorRepair)
	}
	items := CoerceChunkItems(fullDoc["chunks"].(map[string]any)["items"])
	if len(items) != 1 {
		t.Fatalf("expected deduped anchor only, got %+v", items)
	}
	box := items[0]["bbox"].(map[string]any)
	if box["left"].(float64) != 0.10 || box["right"].(float64) != 0.82 {
		t.Fatalf("anchor bbox not repaired: %+v", box)
	}
	if items[0]["source"] != "vlm" {
		t.Fatalf("source not updated: %+v", items[0])
	}
	prov := items[0]["provenance"].(map[string]any)
	if prov["raw_source"] != "regional_vlm" || prov["stage"] != "regional_ocr_vlm" {
		t.Fatalf("provenance not preserved: %+v", prov)
	}
}

func TestMergeRegionalFallbackChunksRepairsAnchorWithNoModelChunks(t *testing.T) {
	fullDoc := map[string]any{
		"chunks": map[string]any{
			"items": []map[string]any{
				{
					"chunk_id":   "anchor",
					"type":       "paragraph",
					"text":       "kept text",
					"page_index": 1,
					"order":      1,
					"bbox":       map[string]any{"left": 0.40, "right": 0.50, "top": 0.20, "bottom": 0.24},
				},
			},
		},
	}
	region := lowConfidenceRegion{
		ID:            "region_p1_0",
		PageIndex:     1,
		Reason:        "bbox_mismatch_geometry_lines",
		AnchorChunkID: "anchor",
		BBox:          map[string]float64{"left": 0.08, "right": 0.90, "top": 0.18, "bottom": 0.28},
	}

	added, merged, anchorRepair := mergeRegionalFallbackChunks(fullDoc, region, nil, "regional_repair", "", 0)
	if added != 0 || merged != 0 || anchorRepair != 1 {
		t.Fatalf("unexpected counts added=%d merged=%d anchor=%d", added, merged, anchorRepair)
	}
	item := CoerceChunkItems(fullDoc["chunks"].(map[string]any)["items"])[0]
	if item["source"] != "repaired" {
		t.Fatalf("source=%v", item["source"])
	}
	prov := item["provenance"].(map[string]any)
	if prov["raw_source"] != "regional_repair" || prov["stage"] != "regional_ocr_vlm" {
		t.Fatalf("provenance=%+v", prov)
	}
	payload := item["payload"].(map[string]any)
	if payload["repair"] != "regional_bbox_alignment" {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestCropRegionDataURL(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.White)
		}
	}
	for y := 20; y < 40; y++ {
		for x := 10; x < 30; x++ {
			img.Set(x, y, color.Black)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	crop, err := cropRegionDataURL(dataURL, map[string]float64{"left": 0.10, "right": 0.30, "top": 0.20, "bottom": 0.40})
	if err != nil {
		t.Fatal(err)
	}
	if crop.Width <= 0 || crop.Height <= 0 || crop.DataURL == "" || len(crop.PNG) == 0 {
		t.Fatalf("invalid crop: %+v", crop)
	}
	if crop.NonBlankRate <= 0 {
		t.Fatalf("expected nonblank crop, got %+v", crop)
	}
}
