package inspectsvc

func buildInspectTimingBreakdown(
	nativeDurationMs int64,
	renderDurationMs int64,
	vlmDurationMs int64,
	enrichDurationMs int64,
	totalDurationMs int64,
	overlapped bool,
	nativeDetail map[string]any,
	vlmDetail map[string]any,
) map[string]any {
	breakdown := map[string]any{
		"native_ms": nativeDurationMs,
		"total_ms":  totalDurationMs,
	}
	if len(nativeDetail) > 0 {
		breakdown["native_detail"] = nativeDetail
	}
	if len(vlmDetail) > 0 {
		breakdown["vlm_detail"] = vlmDetail
	}
	if renderDurationMs > 0 {
		breakdown["render_ms"] = renderDurationMs
	}
	if vlmDurationMs > 0 {
		breakdown["vlm_ms"] = vlmDurationMs
	}
	if enrichDurationMs > 0 {
		breakdown["enrich_ms"] = enrichDurationMs
	}
	if overlapped {
		breakdown["overlapped"] = true
	}
	return breakdown
}
