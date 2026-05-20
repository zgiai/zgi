package inspectsvc

import "testing"

func TestBuildInspectTimingBreakdown(t *testing.T) {
	breakdown := buildInspectTimingBreakdown(1200, 3400, 5600, 700, 9100, true, map[string]any{"inspect_basic_ms": int64(120)}, map[string]any{"main_request_ms": int64(4300)})

	if got, _ := breakdown["native_ms"].(int64); got != 1200 {
		t.Fatalf("native_ms=%d", got)
	}
	if got, _ := breakdown["render_ms"].(int64); got != 3400 {
		t.Fatalf("render_ms=%d", got)
	}
	if got, _ := breakdown["vlm_ms"].(int64); got != 5600 {
		t.Fatalf("vlm_ms=%d", got)
	}
	if got, _ := breakdown["enrich_ms"].(int64); got != 700 {
		t.Fatalf("enrich_ms=%d", got)
	}
	if got, _ := breakdown["total_ms"].(int64); got != 9100 {
		t.Fatalf("total_ms=%d", got)
	}
	nativeDetail, ok := breakdown["native_detail"].(map[string]any)
	if !ok {
		t.Fatalf("native_detail=%T", breakdown["native_detail"])
	}
	if got, _ := nativeDetail["inspect_basic_ms"].(int64); got != 120 {
		t.Fatalf("native_detail.inspect_basic_ms=%d", got)
	}
	vlmDetail, ok := breakdown["vlm_detail"].(map[string]any)
	if !ok {
		t.Fatalf("vlm_detail=%T", breakdown["vlm_detail"])
	}
	if got, _ := vlmDetail["main_request_ms"].(int64); got != 4300 {
		t.Fatalf("vlm_detail.main_request_ms=%d", got)
	}
	if got, _ := breakdown["overlapped"].(bool); !got {
		t.Fatalf("overlapped=%v", breakdown["overlapped"])
	}
}

func TestBuildInspectTimingBreakdownOmitsZeroOptionalStages(t *testing.T) {
	breakdown := buildInspectTimingBreakdown(800, 0, 0, 0, 800, false, nil, nil)

	if got, _ := breakdown["native_ms"].(int64); got != 800 {
		t.Fatalf("native_ms=%d", got)
	}
	if got, _ := breakdown["total_ms"].(int64); got != 800 {
		t.Fatalf("total_ms=%d", got)
	}
	if _, ok := breakdown["render_ms"]; ok {
		t.Fatalf("render_ms should be omitted: %v", breakdown)
	}
	if _, ok := breakdown["vlm_ms"]; ok {
		t.Fatalf("vlm_ms should be omitted: %v", breakdown)
	}
	if _, ok := breakdown["enrich_ms"]; ok {
		t.Fatalf("enrich_ms should be omitted: %v", breakdown)
	}
	if _, ok := breakdown["overlapped"]; ok {
		t.Fatalf("overlapped should be omitted: %v", breakdown)
	}
	if _, ok := breakdown["native_detail"]; ok {
		t.Fatalf("native_detail should be omitted: %v", breakdown)
	}
	if _, ok := breakdown["vlm_detail"]; ok {
		t.Fatalf("vlm_detail should be omitted: %v", breakdown)
	}
}
