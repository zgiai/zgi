package local

import (
	"strings"
	"time"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

const (
	traceKindRule  = "rule"
	traceKindOCR   = "ocr"
	traceKindVLM   = "vlm"
	traceKindCache = "cache"

	traceStatusApplied  = "applied"
	traceStatusSkipped  = "skipped"
	traceStatusWarning  = "warning"
	traceStatusDisabled = "disabled"
)

type localPipelineTraceEvent struct {
	Stage      string         `json:"stage"`
	Kind       string         `json:"kind"`
	Status     string         `json:"status"`
	Reason     string         `json:"reason,omitempty"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	Count      int            `json:"count,omitempty"`
	Model      string         `json:"model,omitempty"`
	Pages      []int          `json:"pages,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

func finalizeLocalParseObservability(doc *extractcommon.DocumentResult, inspect map[string]any, startedAt time.Time) {
	durationMS := time.Since(startedAt).Milliseconds()
	if inspect != nil {
		inspect["duration_ms"] = durationMS
	}
	if doc == nil {
		return
	}
	if doc.Diagnostics == nil {
		doc.Diagnostics = map[string]any{}
	}
	doc.Diagnostics["duration_ms"] = durationMS
	doc.Diagnostics["local_total_duration_ms"] = durationMS
	attachLocalRegionSummary(doc)
	attachLocalPipelineTrace(doc, inspect)
}

func attachLocalPipelineTrace(doc *extractcommon.DocumentResult, inspect map[string]any) {
	if doc == nil {
		return
	}
	if doc.Diagnostics == nil {
		doc.Diagnostics = map[string]any{}
	}
	trace := buildLocalPipelineTrace(doc.Diagnostics, inspect, doc.Source)
	if len(trace) > 0 {
		doc.Diagnostics["pipeline_trace"] = traceEventsAsMaps(trace)
	}
}

func buildLocalPipelineTrace(diag map[string]any, inspect map[string]any, source string) []localPipelineTraceEvent {
	if diag == nil {
		diag = map[string]any{}
	}
	trace := make([]localPipelineTraceEvent, 0, 10)
	source = strings.ToLower(strings.TrimSpace(source))

	if status := stringAny(diag["local_image_vlm_status"]); status != "" {
		trace = append(trace, localPipelineTraceEvent{
			Stage:      "image_vlm",
			Kind:       traceKindVLM,
			Status:     traceStatusFromRaw(status, diag["local_image_vlm_fallback"]),
			Reason:     coalesceString(stringAny(diag["local_image_vlm_warning"]), status),
			DurationMS: int64Any(diag["duration_ms"]),
		})
		return trace
	}

	if source == "local:light" {
		trace = append(trace, localPipelineTraceEvent{
			Stage:      "light_parse",
			Kind:       traceKindRule,
			Status:     traceStatusApplied,
			DurationMS: int64Any(diag["duration_ms"]),
		})
		return trace
	}

	nativeStatus := traceStatusApplied
	nativeReason := ""
	if fullErr, ok := firstMapAny(diag["full_document_error"], nil); ok {
		nativeStatus = traceStatusWarning
		nativeReason = coalesceString(stringAny(fullErr["reason"]), "full_document_failed")
	}
	trace = append(trace, localPipelineTraceEvent{
		Stage:      "native_parse",
		Kind:       traceKindRule,
		Status:     nativeStatus,
		Reason:     nativeReason,
		DurationMS: firstInt64Any(diag["native_pipeline_duration_ms"], diag["duration_ms"]),
	})

	if recovery, ok := firstMapAny(diag["poppler_text_recovery"], inspectValue(inspect, "poppler_text_recovery")); ok {
		trace = append(trace, localPipelineTraceEvent{
			Stage:      "poppler_text_recovery",
			Kind:       traceKindRule,
			Status:     boolStatus(recovery["applied"]),
			Count:      intAny(recovery["chunks"]),
			DurationMS: int64Any(recovery["duration_ms"]),
			Reason:     stringAny(recovery["source"]),
		})
	}

	if refinement, ok := firstMapAny(diag["local_layout_refinement"], inspectValue(inspect, "local_layout_refinement")); ok {
		trace = append(trace, localPipelineTraceEvent{
			Stage:  "layout_refinement",
			Kind:   traceKindRule,
			Status: boolStatus(refinement["applied"]),
			Count:  intAny(refinement["generated_chunks"]),
			Reason: stringAny(refinement["strategy"]),
		})
	}

	if poppler, ok := firstMapAny(diag["local_poppler_bbox"], inspectValue(inspect, "local_poppler_bbox")); ok {
		trace = append(trace, localPipelineTraceEvent{
			Stage:      "bbox_refine",
			Kind:       traceKindRule,
			Status:     boolStatus(poppler["applied"]),
			Count:      intAny(poppler["updated"]),
			DurationMS: int64Any(poppler["duration_ms"]),
			Reason:     stringAny(poppler["reason"]),
		})
	}

	if fallback, ok := firstMapAny(diag["local_vlm_fallback"], inspectValue(inspect, "local_vlm_fallback")); ok {
		trace = append(trace, localPipelineTraceEvent{
			Stage:      "vlm_fallback",
			Kind:       traceKindVLM,
			Status:     traceStatusFromRaw(stringAny(fallback["status"]), fallback["applied"]),
			Reason:     coalesceString(stringAny(fallback["reason"]), stringAny(fallback["status"])),
			DurationMS: int64Any(fallback["vlm_ms"]),
			Count:      intAny(fallback["chunks"]),
			Model:      stringAny(fallback["model"]),
			Pages:      intSliceAny(fallback["pages"]),
		})
	}

	if status := firstStringAny(diag["local_sidebar_ocr_status"], inspectValue(inspect, "local_sidebar_ocr_status")); status != "" {
		trace = append(trace, localPipelineTraceEvent{
			Stage:      "sidebar_ocr",
			Kind:       traceKindOCR,
			Status:     traceStatusFromRaw(status, nil),
			Reason:     coalesceString(firstStringAny(diag["local_sidebar_ocr_warning"], inspectValue(inspect, "local_sidebar_ocr_warning")), status),
			DurationMS: firstInt64Any(diag["local_sidebar_ocr_duration_ms"], inspectValue(inspect, "local_sidebar_ocr_duration_ms")),
			Count:      firstIntAny(diag["local_sidebar_ocr_count"], inspectValue(inspect, "local_sidebar_ocr_count")),
			Model:      firstStringAny(diag["local_sidebar_ocr_engine"], inspectValue(inspect, "local_sidebar_ocr_engine")),
		})
	}

	if status := firstStringAny(diag["vlm_sidebar_recovery_status"], inspectValue(inspect, "vlm_sidebar_recovery_status")); status != "" {
		trace = append(trace, localPipelineTraceEvent{
			Stage:      "sidebar_vlm",
			Kind:       traceKindVLM,
			Status:     traceStatusFromRaw(status, nil),
			Reason:     coalesceString(firstStringAny(diag["vlm_sidebar_recovery_reason"], inspectValue(inspect, "vlm_sidebar_recovery_reason")), status),
			DurationMS: firstInt64Any(diag["vlm_sidebar_recovery_duration_ms"], inspectValue(inspect, "vlm_sidebar_recovery_duration_ms")),
			Count:      firstIntAny(diag["vlm_sidebar_recovery_count"], inspectValue(inspect, "vlm_sidebar_recovery_count")),
			Model:      firstStringAny(diag["vlm_sidebar_recovery_model"], inspectValue(inspect, "vlm_sidebar_recovery_model")),
			Pages:      intSliceAny(firstAny(diag["vlm_sidebar_recovery_pages"], inspectValue(inspect, "vlm_sidebar_recovery_pages"))),
		})
	}

	if status := firstStringAny(diag["vlm_image_caption_status"], inspectValue(inspect, "vlm_image_caption_status")); status != "" {
		trace = append(trace, localPipelineTraceEvent{
			Stage:      "image_caption",
			Kind:       traceKindVLM,
			Status:     traceStatusFromRaw(status, nil),
			Reason:     coalesceString(firstStringAny(diag["vlm_image_caption_warning"], inspectValue(inspect, "vlm_image_caption_warning")), status),
			DurationMS: firstInt64Any(diag["vlm_image_caption_duration_ms"], inspectValue(inspect, "vlm_image_caption_duration_ms")),
			Count:      firstIntAny(diag["vlm_image_caption_count"], inspectValue(inspect, "vlm_image_caption_count")),
			Model:      firstStringAny(diag["vlm_image_caption_model"], inspectValue(inspect, "vlm_image_caption_model")),
		})
	}

	if ocr, ok := firstMapAny(diag["ocr_fallback"], nil); ok {
		trace = append(trace, localPipelineTraceEvent{
			Stage:  "ocr_fallback",
			Kind:   traceKindOCR,
			Status: boolStatus(ocr["applied"]),
			Reason: coalesceString(stringAny(ocr["reason"]), stringAny(ocr["engine"])),
			Count:  intAny(ocr["chunks"]),
		})
	}

	return trace
}

func traceEventsAsMaps(trace []localPipelineTraceEvent) []map[string]any {
	out := make([]map[string]any, 0, len(trace))
	for _, event := range trace {
		m := map[string]any{
			"stage":  event.Stage,
			"kind":   event.Kind,
			"status": event.Status,
		}
		if event.Reason != "" {
			m["reason"] = event.Reason
		}
		if event.DurationMS > 0 {
			m["duration_ms"] = event.DurationMS
		}
		if event.Count > 0 {
			m["count"] = event.Count
		}
		if event.Model != "" {
			m["model"] = event.Model
		}
		if len(event.Pages) > 0 {
			m["pages"] = event.Pages
		}
		if len(event.Metadata) > 0 {
			m["metadata"] = event.Metadata
		}
		out = append(out, m)
	}
	return out
}

func traceStatusFromRaw(raw string, applied any) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if b, ok := applied.(bool); ok {
		if b {
			return traceStatusApplied
		}
		if raw == "" || strings.HasPrefix(raw, "no_") || strings.HasPrefix(raw, "skipped") {
			return traceStatusSkipped
		}
	}
	switch {
	case strings.Contains(raw, "applied") || strings.Contains(raw, "recovered"):
		return traceStatusApplied
	case strings.Contains(raw, "disabled"):
		return traceStatusDisabled
	case strings.Contains(raw, "warning") || strings.Contains(raw, "error") || strings.Contains(raw, "fallback"):
		return traceStatusWarning
	case raw == "" || raw == "no_merge" || strings.HasPrefix(raw, "no_") || strings.HasPrefix(raw, "skipped"):
		return traceStatusSkipped
	default:
		return traceStatusSkipped
	}
}

func boolStatus(v any) string {
	if b, ok := v.(bool); ok && b {
		return traceStatusApplied
	}
	return traceStatusSkipped
}

func inspectValue(inspect map[string]any, key string) any {
	if inspect == nil {
		return nil
	}
	return inspect[key]
}

func firstAny(values ...any) any {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func firstStringAny(values ...any) string {
	for _, v := range values {
		if s := stringAny(v); s != "" {
			return s
		}
	}
	return ""
}

func firstIntAny(values ...any) int {
	for _, v := range values {
		if n := intAny(v); n > 0 {
			return n
		}
	}
	return 0
}

func firstInt64Any(values ...any) int64 {
	for _, v := range values {
		if n := int64Any(v); n > 0 {
			return n
		}
	}
	return 0
}

func firstMapAny(values ...any) (map[string]any, bool) {
	for _, v := range values {
		if m, ok := v.(map[string]any); ok && len(m) > 0 {
			return m, true
		}
	}
	return nil, false
}

func stringAny(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func int64Any(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case int32:
		return int64(x)
	case float64:
		return int64(x)
	case float32:
		return int64(x)
	default:
		return 0
	}
}

func intSliceAny(v any) []int {
	switch x := v.(type) {
	case []int:
		return x
	case []any:
		out := make([]int, 0, len(x))
		for _, item := range x {
			if n := intAny(item); n > 0 {
				out = append(out, n)
			}
		}
		return out
	default:
		return nil
	}
}
