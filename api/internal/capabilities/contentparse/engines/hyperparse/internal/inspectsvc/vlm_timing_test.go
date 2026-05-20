package inspectsvc

import "testing"

func TestVLMCallTimingBreakdownMergeAndMap(t *testing.T) {
	var merged VLMCallTimingBreakdown
	merged.Merge(VLMCallTimingBreakdown{
		MainRequestMs:      1200,
		StrictRetryMs:      300,
		BatchedCalls:       2,
		TileCalls:          4,
		TiledPages:         2,
		FallbackSingleUsed: true,
		StrictRetryUsed:    true,
	})
	merged.Merge(VLMCallTimingBreakdown{
		MainRequestMs: 800,
		BatchedCalls:  1,
	})
	m := buildInspectVLMTimingDetail(merged, 900, 1100)
	if got, _ := m["main_request_ms"].(int64); got != 2000 {
		t.Fatalf("main_request_ms=%d", got)
	}
	if got, _ := m["strict_retry_ms"].(int64); got != 300 {
		t.Fatalf("strict_retry_ms=%d", got)
	}
	if got, _ := m["batched_calls"].(int); got != 3 {
		t.Fatalf("batched_calls=%d", got)
	}
	if got, _ := m["sidebar_recovery_ms"].(int64); got != 900 {
		t.Fatalf("sidebar_recovery_ms=%d", got)
	}
	if got, _ := m["image_caption_ms"].(int64); got != 1100 {
		t.Fatalf("image_caption_ms=%d", got)
	}
	if got, _ := m["fallback_single_used"].(bool); !got {
		t.Fatalf("fallback_single_used=%v", m["fallback_single_used"])
	}
}
