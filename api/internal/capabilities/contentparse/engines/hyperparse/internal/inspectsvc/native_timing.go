package inspectsvc

import pdforchestrator "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/orchestrators/pdf"

func buildInspectNativeTimingDetail(
	inspectBasicDurationMs int64,
	fullDocTiming pdforchestrator.FullDocumentTimingBreakdown,
	nativeDurationMs int64,
) map[string]any {
	detail := map[string]any{}
	setPositiveTiming(detail, "inspect_basic_ms", inspectBasicDurationMs)
	setPositiveTiming(detail, "page_infos_ms", fullDocTiming.PageInfosMs)
	setPositiveTiming(detail, "render_specs_ms", fullDocTiming.RenderSpecsMs)
	setPositiveTiming(detail, "text_extract_ms", fullDocTiming.TextExtractMs)
	setPositiveTiming(detail, "image_extract_ms", fullDocTiming.ImageExtractMs)
	setPositiveTiming(detail, "outline_extract_ms", fullDocTiming.OutlineExtractMs)
	setPositiveTiming(detail, "annotations_extract_ms", fullDocTiming.AnnotationsExtractMs)
	setPositiveTiming(detail, "forms_extract_ms", fullDocTiming.FormsExtractMs)
	setPositiveTiming(detail, "attachments_extract_ms", fullDocTiming.AttachmentsExtractMs)
	setPositiveTiming(detail, "parallel_extract_ms", fullDocTiming.ParallelExtractMs)
	setPositiveTiming(detail, "chunking_ms", fullDocTiming.ChunkingMs)
	setPositiveTiming(detail, "parse_full_document_ms", fullDocTiming.TotalMs)
	if len(fullDocTiming.TextExtractDetail) > 0 {
		detail["text_extract_detail"] = fullDocTiming.TextExtractDetail
	}
	if otherMs := nativeDurationMs - inspectBasicDurationMs - fullDocTiming.TotalMs; otherMs > 0 {
		detail["other_ms"] = otherMs
	}
	return detail
}

func setPositiveTiming(detail map[string]any, key string, value int64) {
	if detail == nil || value <= 0 {
		return
	}
	detail[key] = value
}
