package quality

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/contracts"
)

func TestUnitFilterRemovesLowValueNoise(t *testing.T) {
	filter := NewUnitFilter()

	out, metrics := filter.FilterUnits([]contracts.ChunkUnit{
		{ChunkID: "1", Content: "Page 1 of 8"},
		{ChunkID: "2", Content: "The useful paragraph remains."},
		{ChunkID: "3", Content: "[QR Code]"},
		{ChunkID: "4", Content: "The image is a QR code and no text can be extracted."},
	})

	if len(out) != 1 {
		t.Fatalf("out len = %d", len(out))
	}
	if out[0].ChunkID != "2" {
		t.Fatalf("remaining chunk = %q", out[0].ChunkID)
	}
	if metrics.RemovedCount != 3 {
		t.Fatalf("removed = %d", metrics.RemovedCount)
	}
	if metrics.Reasons["page_counter"] != 1 {
		t.Fatalf("page_counter reasons = %d", metrics.Reasons["page_counter"])
	}
	if metrics.Reasons["barcode_placeholder"] != 1 {
		t.Fatalf("barcode_placeholder reasons = %d", metrics.Reasons["barcode_placeholder"])
	}
	if metrics.Reasons["vision_no_text_message"] != 1 {
		t.Fatalf("vision_no_text_message reasons = %d", metrics.Reasons["vision_no_text_message"])
	}
}

func TestUnitFilterRemovesTinyOCRFragmentsButKeepsLabels(t *testing.T) {
	filter := NewUnitFilter()

	out, metrics := filter.FilterElements([]contracts.ChunkSourceElement{
		{
			ElementID: "noise",
			Content:   "Sk",
			BBox:      &contracts.ParseBoundingBox{Left: 0.11, Top: 0.14, Right: 0.15, Bottom: 0.16},
			Metadata: map[string]any{
				"payload": map[string]any{"extraction_method": "ocr", "bbox_source": "ocr_line"},
			},
		},
		{
			ElementID: "label",
			Content:   "TO:",
			BBox:      &contracts.ParseBoundingBox{Left: 0.1, Top: 0.34, Right: 0.14, Bottom: 0.36},
			Metadata: map[string]any{
				"payload": map[string]any{"extraction_method": "ocr", "bbox_source": "ocr_line"},
			},
		},
		{
			ElementID: "body",
			Content:   "Useful form content",
			BBox:      &contracts.ParseBoundingBox{Left: 0.2, Top: 0.34, Right: 0.55, Bottom: 0.36},
			Metadata: map[string]any{
				"payload": map[string]any{"extraction_method": "ocr", "bbox_source": "ocr_line"},
			},
		},
	})

	if len(out) != 2 {
		t.Fatalf("out len = %d", len(out))
	}
	if out[0].ElementID != "label" || out[1].ElementID != "body" {
		t.Fatalf("remaining elements = %+v", out)
	}
	if metrics.RemovedCount != 1 || metrics.Reasons["tiny_ocr_fragment"] != 1 {
		t.Fatalf("metrics = %+v", metrics)
	}
}
