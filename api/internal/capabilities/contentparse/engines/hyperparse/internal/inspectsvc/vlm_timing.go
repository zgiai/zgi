package inspectsvc

type VLMCallTimingBreakdown struct {
	MainRequestMs      int64 `json:"main_request_ms,omitempty"`
	StrictRetryMs      int64 `json:"strict_retry_ms,omitempty"`
	BatchedCalls       int   `json:"batched_calls,omitempty"`
	TileCalls          int   `json:"tile_calls,omitempty"`
	TiledPages         int   `json:"tiled_pages,omitempty"`
	FallbackSingleUsed bool  `json:"fallback_single_used,omitempty"`
	StrictRetryUsed    bool  `json:"strict_retry_used,omitempty"`
}

func (b *VLMCallTimingBreakdown) Merge(other VLMCallTimingBreakdown) {
	if b == nil {
		return
	}
	b.MainRequestMs += other.MainRequestMs
	b.StrictRetryMs += other.StrictRetryMs
	b.BatchedCalls += other.BatchedCalls
	b.TileCalls += other.TileCalls
	b.TiledPages += other.TiledPages
	b.FallbackSingleUsed = b.FallbackSingleUsed || other.FallbackSingleUsed
	b.StrictRetryUsed = b.StrictRetryUsed || other.StrictRetryUsed
}

func (b VLMCallTimingBreakdown) ToMap() map[string]any {
	out := map[string]any{}
	if b.MainRequestMs > 0 {
		out["main_request_ms"] = b.MainRequestMs
	}
	if b.StrictRetryMs > 0 {
		out["strict_retry_ms"] = b.StrictRetryMs
	}
	if b.BatchedCalls > 0 {
		out["batched_calls"] = b.BatchedCalls
	}
	if b.TileCalls > 0 {
		out["tile_calls"] = b.TileCalls
	}
	if b.TiledPages > 0 {
		out["tiled_pages"] = b.TiledPages
	}
	if b.StrictRetryUsed {
		out["strict_retry_used"] = true
	}
	if b.FallbackSingleUsed {
		out["fallback_single_used"] = true
	}
	return out
}

func buildInspectVLMTimingDetail(main VLMCallTimingBreakdown, sidebarRecoveryMs int64, imageCaptionMs int64) map[string]any {
	detail := main.ToMap()
	if sidebarRecoveryMs > 0 {
		detail["sidebar_recovery_ms"] = sidebarRecoveryMs
	}
	if imageCaptionMs > 0 {
		detail["image_caption_ms"] = imageCaptionMs
	}
	return detail
}
