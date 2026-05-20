package inspectsvc

import (
	"fmt"
	"strings"
)

type inspectPipelineOptions struct {
	RecognitionSource       string
	SuggestVLM              bool
	FullPageVLMDisabled     bool
	VLMRunning              bool
	VLMUsed                 bool
	VLMError                string
	VLMModel                string
	VLMRenderedPages        int
	VLMAnalyzedPages        int
	RegionalFallbackRunning bool
	RegionalFallbackUsed    bool
	RegionalFallbackCount   int
	RegionalFallbackModel   string
	RegionalFallbackWarning string
	ImageCaptionStatus      string
	ImageCaptionCount       int
	ImageCaptionModel       string
	NativeImageAppendCount  int
	NativeImagePreviewCount int
}

func buildInspectPipelineStages(fullDoc map[string]any, opts inspectPipelineOptions) []map[string]any {
	nativeCount := fullDocumentChunkCount(fullDoc)
	imageCount := fullDocumentImageChunkCount(fullDoc)
	repairCount := fullDocumentLayoutRepairCount(fullDoc)
	lowConfidenceRegionCount := fullDocumentLowConfidenceRegionCount(fullDoc)
	regionalFallbackCount := opts.RegionalFallbackCount
	if regionalFallbackCount == 0 {
		regionalFallbackCount = fullDocumentRegionalFallbackCount(fullDoc)
	}
	layoutSuggestHybrid := fullDocumentLayoutSuggestHybrid(fullDoc)
	layoutProcessorCount := fullDocumentLayoutProcessorElementCount(fullDoc)
	if layoutProcessorCount == 0 {
		layoutProcessorCount = nativeCount
	}

	bboxStatus := "done"
	if repairCount > 0 {
		bboxStatus = "used"
	} else if lowConfidenceRegionCount > 0 || layoutSuggestHybrid || opts.SuggestVLM {
		bboxStatus = "review"
	}

	fallbackStatus := "skipped"
	fallbackModel := opts.VLMModel
	if fallbackModel == "" {
		fallbackModel = opts.RegionalFallbackModel
	}
	fallbackDetail := opts.VLMError
	if fallbackDetail == "" {
		fallbackDetail = opts.RegionalFallbackWarning
	}
	fallbackCount := opts.VLMAnalyzedPages
	if regionalFallbackCount > 0 {
		fallbackCount = regionalFallbackCount
	}
	if opts.RegionalFallbackRunning {
		fallbackStatus = "running"
		fallbackCount = lowConfidenceRegionCount
	} else if opts.VLMRunning {
		fallbackStatus = "running"
	} else if opts.FullPageVLMDisabled && regionalFallbackCount == 0 && !opts.RegionalFallbackUsed {
		fallbackStatus = "skipped"
		if fallbackDetail == "" && opts.SuggestVLM {
			fallbackDetail = "full-page VLM disabled; image VLM only"
		}
	} else if strings.TrimSpace(fallbackDetail) != "" && regionalFallbackCount == 0 {
		fallbackStatus = "error"
	} else if regionalFallbackCount > 0 || opts.RegionalFallbackUsed || opts.VLMUsed || opts.RecognitionSource == "vlm" || opts.RecognitionSource == "hybrid" {
		fallbackStatus = "used"
	} else if lowConfidenceRegionCount > 0 || opts.SuggestVLM {
		fallbackStatus = "queued"
	}

	captionStatus := strings.TrimSpace(opts.ImageCaptionStatus)
	if captionStatus == "" {
		if opts.ImageCaptionCount > 0 {
			captionStatus = "used"
		} else if imageCount > 0 {
			captionStatus = "queued"
		} else {
			captionStatus = "skipped"
		}
	}

	return []map[string]any{
		stageMap("native_text", "native", "done", nativeCount, "", "", false),
		stageMap("visual_layout", "native", "done", 0, "", "", false),
		stageMap("chunk_semantics", "native", "done", nativeCount, "", "", false),
		stageMap("reading_order", "native", "done", layoutProcessorCount, "", "xycut", false),
		stageMap("bbox_alignment", "repaired", bboxStatus, repairCount, "", "", false),
		stageMap("ocr_vlm_fallback", fallbackStageSource(opts.RecognitionSource, regionalFallbackCount > 0 || opts.RegionalFallbackUsed || opts.RegionalFallbackRunning), fallbackStatus, fallbackCount, fallbackModel, fallbackDetail, false),
		stageMap("image_caption", "vlm", captionStatus, opts.ImageCaptionCount, opts.ImageCaptionModel, "", true),
		stageMap("dpt_export", "native", "done", nativeCount, "", "", false),
	}
}

func fullDocumentLayoutProcessorElementCount(fullDoc map[string]any) int {
	doc, ok := fullDoc["document"].(map[string]any)
	if !ok {
		return 0
	}
	lp, ok := doc["layout_processor_pipeline"].(map[string]any)
	if !ok {
		return 0
	}
	return intFromAnyPipeline(lp["element_count"])
}

func stageMap(id, source, status string, count int, model, detail string, async bool) map[string]any {
	m := map[string]any{
		"id":     id,
		"source": source,
		"status": status,
	}
	if count > 0 {
		m["count"] = count
	}
	if model != "" {
		m["model"] = model
	}
	if detail != "" {
		m["detail"] = detail
	}
	if async {
		m["async"] = true
	}
	return m
}

func fallbackStageSource(recognitionSource string, regional bool) string {
	if regional {
		return "ocr/vlm"
	}
	switch strings.ToLower(strings.TrimSpace(recognitionSource)) {
	case "vlm", "hybrid":
		return "vlm"
	default:
		return "native"
	}
}

func imageCaptionStatus(done int, warn string, imageCount int) string {
	if done > 0 {
		return "used"
	}
	if strings.TrimSpace(warn) != "" {
		return "error"
	}
	if imageCount > 0 {
		return "skipped"
	}
	return "skipped"
}

func fullDocumentChunkCount(fullDoc map[string]any) int {
	if chW, ok := fullDoc["chunks"].(map[string]any); ok {
		return len(CoerceChunkItems(chW["items"]))
	}
	return 0
}

func fullDocumentImageChunkCount(fullDoc map[string]any) int {
	if chW, ok := fullDoc["chunks"].(map[string]any); ok {
		count := 0
		for _, ch := range CoerceChunkItems(chW["items"]) {
			switch strings.ToLower(strings.TrimSpace(fmt.Sprint(ch["type"]))) {
			case "image", "stamp":
				count++
			}
		}
		return count
	}
	return 0
}

func fullDocumentLayoutRepairCount(fullDoc map[string]any) int {
	lq := fullDocumentLocalQuality(fullDoc)
	return intFromAnyPipeline(lq["repair_chunks_added"])
}

func fullDocumentLowConfidenceRegionCount(fullDoc map[string]any) int {
	lq := fullDocumentLocalQuality(fullDoc)
	if n := intFromAnyPipeline(lq["low_confidence_region_count"]); n > 0 {
		return n
	}
	switch raw := lq["low_confidence_regions"].(type) {
	case []map[string]any:
		return len(raw)
	case []any:
		return len(raw)
	default:
		return 0
	}
}

func fullDocumentRegionalFallbackCount(fullDoc map[string]any) int {
	doc, ok := fullDoc["document"].(map[string]any)
	if !ok {
		return 0
	}
	meta, _ := doc["regional_ocr_vlm"].(map[string]any)
	return intFromAnyPipeline(meta["count"])
}

func fullDocumentLayoutSuggestHybrid(fullDoc map[string]any) bool {
	lq := fullDocumentLocalQuality(fullDoc)
	return boolFromAnyPipeline(lq["suggest_hybrid"])
}

func fullDocumentLocalQuality(fullDoc map[string]any) map[string]any {
	doc, ok := fullDoc["document"].(map[string]any)
	if !ok {
		return nil
	}
	lq, _ := doc["local_quality"].(map[string]any)
	return lq
}

func intFromAnyPipeline(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case float32:
		return int(x)
	default:
		return 0
	}
}

func boolFromAnyPipeline(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}
