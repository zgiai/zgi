package pdf

import "testing"

func TestTextExtractTimingBreakdownToMap(t *testing.T) {
	profile := TextExtractTimingBreakdown{
		CMapIndexMs:                12,
		PageCount:                  3,
		PageWithContentsCount:      2,
		GeomScannedPages:           2,
		GeomSkippedPages:           1,
		FallbackTextExtractPages:   1,
		TotalDecodeMs:              44,
		TotalGeomMs:                55,
		TotalFallbackTextExtractMs: 66,
		TotalMs:                    99,
		SlowPages: []TextExtractPageTiming{
			{
				PageIndex:    2,
				ContentRefs:  3,
				DecodedBytes: 4096,
				DecodeMs:     17,
				GeomMs:       23,
				GeomScanned:  true,
				TotalMs:      50,
			},
		},
	}

	m := profile.ToMap()
	if got, _ := m["cmap_index_ms"].(int64); got != 12 {
		t.Fatalf("cmap_index_ms=%d", got)
	}
	slowPages, ok := m["slow_pages"].([]map[string]any)
	if !ok || len(slowPages) != 1 {
		t.Fatalf("slow_pages=%T %#v", m["slow_pages"], m["slow_pages"])
	}
	if got, _ := slowPages[0]["page_index"].(int); got != 2 {
		t.Fatalf("slow_pages[0].page_index=%d", got)
	}
	if got, _ := slowPages[0]["geom_scanned"].(bool); !got {
		t.Fatalf("slow_pages[0].geom_scanned=%v", slowPages[0]["geom_scanned"])
	}
}
