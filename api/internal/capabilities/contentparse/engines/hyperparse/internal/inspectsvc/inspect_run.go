package inspectsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	pdfadapter "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/adapters/pdf"
	"github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/export"
	pdforchestrator "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/orchestrators/pdf"
)

// PDFInspectInput mirrors the non-progressive playground inspect request.
type PDFInspectInput struct {
	Filename   string
	Data       []byte
	Mode       string
	PreferFull bool // true disables fast-first-pages subset VLM
}

// RunPDFInspect executes native full_document, optional VLM fallback, native
// image completion, image captioning, and DPT export.
// The returned map is the result object for HTTP responses, without the outer ok field.
func RunPDFInspect(ctx context.Context, in PDFInspectInput) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	startedAt := time.Now()
	mode := strings.TrimSpace(in.Mode)
	if mode == "" {
		mode = "relaxed"
	}
	filename := strings.TrimSpace(in.Filename)
	if filename == "" {
		filename = "upload.pdf"
	}
	if len(in.Data) == 0 {
		return nil, attachInspectFailureDebugDump(inspectFailureDebugState{
			Filename:    filename,
			Mode:        mode,
			Stage:       "input",
			SizeBytes:   len(in.Data),
			PreferFull:  in.PreferFull,
			Progressive: false,
			Err:         fmt.Errorf("empty pdf"),
		})
	}

	inspectBasicStartedAt := time.Now()
	info, err := pdfadapter.InspectBasicBytes(in.Data, mode)
	inspectBasicDurationMs := time.Since(inspectBasicStartedAt).Milliseconds()
	if err != nil {
		return nil, attachInspectFailureDebugDump(inspectFailureDebugState{
			Filename:    filename,
			Mode:        mode,
			Stage:       "inspect_basic",
			SizeBytes:   len(in.Data),
			PreferFull:  in.PreferFull,
			Progressive: false,
			Err:         err,
		})
	}
	fullDoc, fullDocTiming, err := pdforchestrator.ParseFullDocumentBytesWithBasicProfiled(in.Data, filename, mode, info)
	if err != nil {
		return nil, attachInspectFailureDebugDump(inspectFailureDebugState{
			Filename:    filename,
			Mode:        mode,
			Stage:       "parse_full_document",
			SizeBytes:   len(in.Data),
			PageCount:   info.PageCount,
			CountSource: info.CountSource,
			PDFVersion:  info.Version,
			PreferFull:  in.PreferFull,
			Progressive: false,
			Err:         fmt.Errorf("build full document failed: %w", err),
		})
	}
	effectivePageCount := effectiveInspectPageCount(info.PageCount, fullDoc)
	nativeDurationMs := time.Since(startedAt).Milliseconds()
	nativeTimingDetail := buildInspectNativeTimingDetail(inspectBasicDurationMs, fullDocTiming, nativeDurationMs)

	suggestVLM := false
	if doc, ok := fullDoc["document"].(map[string]any); ok {
		if v, ok := doc["suggest_vlm"].(bool); ok {
			suggestVLM = v
		}
	}
	if ForceVLM() {
		suggestVLM = true
		if doc, ok := fullDoc["document"].(map[string]any); ok {
			doc["force_vlm"] = true
			doc["suggest_vlm"] = true
		}
	}
	nativeChunkCount := 0
	routeDecision := routeDecisionFromFullDoc(fullDoc)
	oversizedPages := oversizedPageSetFromFullDoc(fullDoc)
	if EnvBoolLower("CONTENT_PARSE_UI_DEBUG_FULLDOC", false) {
		log.Printf("[ui.inspect] full_document: %+v", fullDoc)
	}
	if chW, ok := fullDoc["chunks"].(map[string]any); ok {
		nativeChunkCount = len(CoerceChunkItems(chW["items"]))
	}
	vlmConfigured := VLMConfigured()
	fullPageVLMEnabled := FullPageVLMFallbackEnabled()
	vlmPipelineErr := ""
	if suggestVLM && fullPageVLMEnabled && !vlmConfigured {
		vlmPipelineErr = missingVLMAPIKeyMessage
	}
	log.Printf("[ui.inspect] full_document ok file=%q mode=%s suggest_vlm=%v", filename, mode, suggestVLM)
	if len(nativeTimingDetail) > 0 {
		log.Printf("[ui.inspect] native_timing file=%q detail=%v", filename, nativeTimingDetail)
	}
	if suggestVLM && fullPageVLMEnabled && vlmConfigured {
		log.Printf("[ui.inspect] native content is insufficient; entering full-page VLM fallback. native_chunk_count=%d", nativeChunkCount)
		log.Printf("[ui.inspect] route_decision=native_then_vlm reason=insufficient_native_content")
	} else if suggestVLM && fullPageVLMEnabled {
		log.Printf("[ui.inspect] VLM fallback was suggested but no VLM API key is configured; keeping native output. native_chunk_count=%d", nativeChunkCount)
		log.Printf("[ui.inspect] route_decision=native_only reason=missing_vlm_api_key")
	} else if suggestVLM {
		log.Printf("[ui.inspect] VLM fallback was suggested but full-page VLM is disabled; keeping native output with regional/image repair. native_chunk_count=%d", nativeChunkCount)
		log.Printf("[ui.inspect] route_decision=rules_plus_image_vlm reason=full_page_vlm_disabled")
	} else {
		log.Printf("[ui.inspect] native output is sufficient; VLM fallback is not needed. native_chunk_count=%d", nativeChunkCount)
		log.Printf("[ui.inspect] route_decision=native_only reason=native_sufficient")
	}

	recognitionSource := "native"
	var vlmText, vlmModel, renderEngine, vlmRawResponse string
	var regionalFallback regionalFallbackResult
	if fullDocumentLowConfidenceRegionCount(fullDoc) > 0 && regionalFallbackEnabled() {
		log.Printf("[ui.inspect] regional: low-confidence fallback start regions=%d", fullDocumentLowConfidenceRegionCount(fullDoc))
		regionalFallback = ApplyRegionalOCRVLMFallback(ctx, in.Data, fullDoc)
		log.Printf("[ui.inspect] regional: low-confidence fallback done status=%s count=%d added=%d merged=%d anchor_bbox=%d model=%q warn=%q",
			regionalFallback.Status, regionalFallback.Count, regionalFallback.AddedCount, regionalFallback.MergedCount, regionalFallback.AnchorBBoxRepairCount, regionalFallback.Model, regionalFallback.Warning)
		if regionalFallback.Count > 0 {
			recognitionSource = "hybrid"
		}
	}
	vlmFastPreview := false
	vlmFastPreviewPages := 0
	vlmFastPreviewTotal := 0
	vlmRenderedPages := 0
	var plannedPages []int
	var processedPages []int
	var processedPageDataURLs []string
	var renderDurationMs int64
	var vlmDurationMs int64
	var vlmCallTiming VLMCallTimingBreakdown
	mergeNativeKeptCount := nativeChunkCount
	mergeVLMCount := 0
	var mergeVLMPagesApplied []int
	var vlmSidebarModel, vlmSidebarWarn string
	var vlmSidebarCount int
	var vlmSidebarDurationMs int64
	var vlmImageCaptionDurationMs int64
	if suggestVLM && fullPageVLMEnabled && vlmConfigured {
		maxPages := VLMFallbackMaxPages()
		pageSelection := vlmPageSelectionFromFullDoc(fullDoc, effectivePageCount, maxPages)
		renderScaleOverrides := buildOversizedPageRenderScaleOverrides(pageSelection, oversizedPages)
		plannedPages = normalizeProcessedPages(pageSelection)
		log.Printf("[ui.inspect] vlm: start render pages (max=%d selected=%v)", maxPages, pageSelection)
		renderStartedAt := time.Now()
		pageDataURLs, renderedPages, engine, rerr := RenderPDFSelectedPagesToDataURLsWithScales(in.Data, pageSelection, maxPages, renderScaleOverrides)
		renderDurationMs = time.Since(renderStartedAt).Milliseconds()
		renderEngine = engine
		if rerr != nil {
			vlmPipelineErr = rerr.Error()
			log.Printf("[ui.inspect] vlm: page render error: %v", rerr)
		} else if len(pageDataURLs) == 0 {
			vlmPipelineErr = "no rendered page images available for VLM"
			log.Printf("[ui.inspect] vlm: no page images engine=%q", engine)
		} else {
			plannedPages = normalizeProcessedPages(renderedPages)
			vlmRenderedPages = len(pageDataURLs)
			log.Printf("[ui.inspect] vlm: rendered engine=%q page_images=%d", engine, len(pageDataURLs))
			activePageDataURLs := pageDataURLs
			activePages := renderedPages
			fastN := VLMFallbackFastFirstPages()
			if !in.PreferFull && fastN > 0 && len(pageDataURLs) > fastN {
				vlmFastPreview = true
				vlmFastPreviewPages = fastN
				vlmFastPreviewTotal = len(pageDataURLs)
				activePageDataURLs = pageDataURLs[:fastN]
				activePages = renderedPages[:fastN]
				log.Printf("[ui.inspect] vlm: fast-preview enabled pages=%d/%d actual=%v", fastN, len(pageDataURLs), activePages)
			}
			processedPages = normalizeProcessedPages(activePages)
			processedPageDataURLs = append([]string(nil), activePageDataURLs...)
			vlmStartedAt := time.Now()
			vm, vlmItems, rawReply, callTiming, verr := CallDashscopeVLMFallbackStructuredForRenderedPagesProfiled(activePageDataURLs, activePages, oversizedPages)
			vlmDurationMs = time.Since(vlmStartedAt).Milliseconds()
			vlmCallTiming = callTiming
			vlmModel = vm
			if verr != nil {
				vlmPipelineErr = verr.Error()
				log.Printf("[ui.inspect] vlm: chunk_schema API error: %v", verr)
				log.Printf("[ui.inspect] VLM fallback failed; keeping native output. err=%v", verr)
			} else if len(vlmItems) == 0 {
				log.Printf("[ui.inspect] vlm: chunk_schema returned no items (model=%q)", vm)
				log.Printf("[ui.inspect] VLM fallback returned no items; keeping native output. model=%q", vm)
			} else {
				log.Printf("[ui.inspect] vlm: got %d chunk items from model=%q", len(vlmItems), vm)
				log.Printf("[ui.inspect] VLM fallback produced mergeable items. vlm_item_count=%d model=%q", len(vlmItems), vm)
				if chW, ok := fullDoc["chunks"].(map[string]any); ok {
					native := CoerceChunkItems(chW["items"])
					log.Printf("[ui.inspect] vlm: merge native_items=%d vlm_items=%d", len(native), len(vlmItems))
					merged, mergeStats := pdforchestrator.MergeNativeAndVLMChunkItemsForPages(native, vlmItems, processedPages)
					mergeNativeKeptCount = mergeStats.NativeKeptCount
					mergeVLMCount = mergeStats.VLMMergedCount
					mergeVLMPagesApplied = mergeStats.VLMPagesApplied
					chW["items"] = merged
					chW["count"] = len(merged)
					if mergeStats.VLMMergedCount > 0 {
						chW["vlm_merge"] = true
						recognitionSource = "vlm"
						vlmText = JoinVLMChunkTexts(vlmItems)
					}
					log.Printf("[ui.inspect] vlm: merge done merged_count=%d", len(merged))
				}
				if mergeVLMCount > 0 {
					vlmRawResponse = rawReply
				}
				if mergeVLMCount > 0 {
					if doc, ok := fullDoc["document"].(map[string]any); ok {
						if chW, ok := fullDoc["chunks"].(map[string]any); ok {
							merged := CoerceChunkItems(chW["items"])
							pdforchestrator.RebuildTextSummaryAfterVLMMerge(doc, merged)
							log.Printf("[ui.inspect] vlm: text_summary rebuilt from merged chunks")
						}
					}
				}
			}
		}
	}
	if mergeVLMCount > 0 && len(processedPageDataURLs) == len(processedPages) && len(processedPages) > 0 {
		sidebarStartedAt := time.Now()
		vlmSidebarModel, vlmSidebarWarn, vlmSidebarCount = AppendVLMRightSidebarTextFromRenderedPages(fullDoc, processedPageDataURLs, processedPages)
		vlmSidebarDurationMs = time.Since(sidebarStartedAt).Milliseconds()
		if vlmSidebarCount > 0 {
			mergeVLMCount += vlmSidebarCount
			if chW, ok := fullDoc["chunks"].(map[string]any); ok {
				if doc, ok := fullDoc["document"].(map[string]any); ok {
					merged := CoerceChunkItems(chW["items"])
					pdforchestrator.RebuildTextSummaryAfterVLMMerge(doc, merged)
					log.Printf("[ui.inspect] vlm: text_summary rebuilt after sidebar recovery")
				}
			}
		}
	}

	var nativeImageAppendWarn string
	var nativeImageAppendCount int
	var vlmImageCaptionModel, vlmImageCaptionWarn string
	var vlmImageCaptionStatus string
	var vlmImageCaptionCount int
	enrichStartedAt := time.Now()
	nativeImagePages := complementPages(effectivePageCount, nil)
	if mergeVLMCount > 0 {
		nativeImagePages = complementPages(effectivePageCount, mergeVLMPagesApplied)
	}
	if len(nativeImagePages) > 0 {
		log.Printf("[ui.inspect] native: image_append start pages=%v", nativeImagePages)
		nativeImageAppendWarn, nativeImageAppendCount = AppendNativeImageChunksIfMissingForPages(in.Data, mode, fullDoc, nativeImagePages)
		log.Printf("[ui.inspect] native: image_append done count=%d pages=%v warn=%q", nativeImageAppendCount, nativeImagePages, nativeImageAppendWarn)
		if HasImageChunksForPages(fullDoc, nativeImagePages) {
			if !EnvBoolLower("CONTENT_PARSE_VLM_IMAGE_CAPTION", true) {
				vlmImageCaptionStatus = "disabled"
				log.Printf("[ui.inspect] vlm: image_caption skipped: disabled by CONTENT_PARSE_VLM_IMAGE_CAPTION=0")
			} else if VLMAPIKey() == "" {
				vlmImageCaptionStatus = "skipped_missing_vlm_api_key"
				log.Printf("[ui.inspect] vlm: image_caption skipped: missing VLM_API_KEY/DASHSCOPE_API_KEY/GEMINI_API_KEY")
			} else {
				log.Printf("[ui.inspect] native pages contain image chunks; starting VLM image captioning. pages=%v", nativeImagePages)
				log.Printf("[ui.inspect] vlm: image_caption start pages=%v", nativeImagePages)
				imageCaptionStartedAt := time.Now()
				vlmImageCaptionModel, vlmImageCaptionWarn, vlmImageCaptionCount = EnrichImageChunksWithVLMCaptionsForPages(in.Data, mode, fullDoc, nativeImagePages)
				vlmImageCaptionDurationMs = time.Since(imageCaptionStartedAt).Milliseconds()
				if vlmImageCaptionCount > 0 {
					vlmImageCaptionStatus = "applied"
				} else if vlmImageCaptionWarn != "" {
					vlmImageCaptionStatus = "warning"
				} else {
					vlmImageCaptionStatus = "no_caption_generated"
				}
				log.Printf("[ui.inspect] vlm: image_caption done count=%d model=%q pages=%v warn=%q", vlmImageCaptionCount, vlmImageCaptionModel, nativeImagePages, vlmImageCaptionWarn)
			}
		} else {
			vlmImageCaptionStatus = "skipped_no_image_chunks"
			log.Printf("[ui.inspect] vlm: image_caption skipped: no image chunks in native-kept pages=%v", nativeImagePages)
		}
	} else {
		vlmImageCaptionStatus = "skipped_no_native_pages"
		log.Printf("[ui.inspect] image_enrich skipped: no native-kept pages after vlm merge")
	}
	enrichDurationMs := time.Since(enrichStartedAt).Milliseconds()
	if mergeVLMCount == 0 {
		mergeNativeKeptCount = countNativeOnlyChunks(fullDoc)
		mergeVLMPagesApplied = nil
	}
	resultScope, coveragePages, mergeReport := buildInspectSemantics(
		effectivePageCount,
		plannedPages,
		processedPages,
		mergeNativeKeptCount,
		mergeVLMCount,
		vlmFastPreview,
		mergeVLMPagesApplied,
	)
	routeDebug := buildRouteDebug(fullDoc, plannedPages, processedPages, mergeVLMPagesApplied)
	log.Printf("[ui.inspect] route_debug planned=%v processed=%v applied=%v", plannedPages, processedPages, mergeVLMPagesApplied)
	totalDurationMs := time.Since(startedAt).Milliseconds()
	vlmTimingDetail := buildInspectVLMTimingDetail(vlmCallTiming, vlmSidebarDurationMs, vlmImageCaptionDurationMs)
	timingBreakdown := buildInspectTimingBreakdown(
		nativeDurationMs,
		renderDurationMs,
		vlmDurationMs,
		enrichDurationMs,
		totalDurationMs,
		false,
		nativeTimingDetail,
		vlmTimingDetail,
	)

	res := map[string]any{
		"filename":             filename,
		"mode":                 mode,
		"duration_ms":          totalDurationMs,
		"timing_breakdown":     timingBreakdown,
		"size_bytes":           info.FileSize,
		"pdf_version":          info.Version,
		"page_count":           effectivePageCount,
		"count_source":         info.CountSource,
		"title":                info.Title,
		"author":               info.Author,
		"subject":              info.Subject,
		"producer":             info.Producer,
		"creator":              info.Creator,
		"xref_type":            info.XRefType,
		"startxref":            info.StartXRef,
		"has_trailer":          info.HasTrailer,
		"has_eof_marker":       info.HasEOFMarker,
		"full_document":        fullDoc,
		"recognition_source":   recognitionSource,
		"vlm_recognition_text": vlmText,
		"vlm_model":            vlmModel,
		"route_decision":       routeDecision,
		"route_debug":          routeDebug,
		"result_scope":         resultScope,
		"coverage_pages":       coveragePages,
		"merge_report":         mergeReport,
	}
	if renderEngine != "" {
		res["vlm_render_engine"] = renderEngine
	}
	if vlmFastPreview {
		res["vlm_fast_preview"] = true
		res["vlm_fast_preview_pages"] = vlmFastPreviewPages
		res["vlm_fast_preview_total_pages"] = vlmFastPreviewTotal
	}
	if vlmRenderedPages > 0 {
		res["vlm_rendered_pages"] = vlmRenderedPages
	}
	if vlmPipelineErr != "" {
		res["vlm_pipeline_error"] = vlmPipelineErr
	}
	if vlmRawResponse != "" {
		res["vlm_raw_response"] = vlmRawResponse
	}
	if vlmImageCaptionCount > 0 {
		res["vlm_image_caption_count"] = vlmImageCaptionCount
		if vlmImageCaptionModel != "" {
			res["vlm_image_caption_model"] = vlmImageCaptionModel
		}
	}
	if vlmImageCaptionStatus != "" {
		res["vlm_image_caption_status"] = vlmImageCaptionStatus
	}
	if vlmImageCaptionWarn != "" {
		res["vlm_image_caption_warning"] = vlmImageCaptionWarn
	}
	if vlmSidebarCount > 0 {
		res["vlm_sidebar_recovery_count"] = vlmSidebarCount
		if vlmSidebarModel != "" {
			res["vlm_sidebar_recovery_model"] = vlmSidebarModel
		}
	}
	if vlmSidebarWarn != "" {
		res["vlm_sidebar_recovery_warning"] = vlmSidebarWarn
	}
	if regionalFallback.RegionCount > 0 {
		res["regional_fallback_region_count"] = regionalFallback.RegionCount
		res["regional_fallback_count"] = regionalFallback.Count
		res["regional_fallback_status"] = regionalFallback.Status
		if regionalFallback.Model != "" {
			res["regional_fallback_model"] = regionalFallback.Model
		}
		if regionalFallback.Warning != "" {
			res["regional_fallback_warning"] = regionalFallback.Warning
		}
	}
	if nativeImageAppendCount > 0 {
		res["native_image_append_count"] = nativeImageAppendCount
	}
	if nativeImageAppendWarn != "" {
		res["native_image_append_warning"] = nativeImageAppendWarn
	}
	res["pipeline_stages"] = buildInspectPipelineStages(fullDoc, inspectPipelineOptions{
		RecognitionSource:       recognitionSource,
		SuggestVLM:              suggestVLM,
		FullPageVLMDisabled:     suggestVLM && !fullPageVLMEnabled,
		VLMUsed:                 mergeVLMCount > 0,
		VLMError:                vlmPipelineErr,
		VLMModel:                vlmModel,
		VLMRenderedPages:        vlmRenderedPages,
		VLMAnalyzedPages:        len(processedPages),
		RegionalFallbackUsed:    regionalFallback.Count > 0,
		RegionalFallbackCount:   regionalFallback.Count,
		RegionalFallbackModel:   regionalFallback.Model,
		RegionalFallbackWarning: regionalFallback.Warning,
		ImageCaptionStatus:      vlmImageCaptionStatus,
		ImageCaptionCount:       vlmImageCaptionCount,
		ImageCaptionModel:       vlmImageCaptionModel,
		NativeImageAppendCount:  nativeImageAppendCount,
	})
	res["dpt_export"] = export.BuildDPTExportFromFullDocument(fullDoc, filename, effectivePageCount)
	if doc, ok := fullDoc["document"].(map[string]any); ok {
		if v, ok := doc["suggest_vlm"].(bool); ok {
			res["suggest_vlm"] = v
		}
		if h, ok := doc["image_like_pdf"]; ok {
			res["image_like_pdf"] = h
		}
		if b, ok := doc["business_doc_vlm_hint"]; ok {
			res["business_doc_vlm_hint"] = b
		}
	}
	maybeAttachInspectDebugArtifacts(res, filename, "", false)
	return res, nil
}

// MarshalInspectResponse wraps result as {"ok":true,"result":...} for cache and HTTP output.
func MarshalInspectResponse(result map[string]any) ([]byte, error) {
	return json.Marshal(map[string]any{"ok": true, "result": result})
}
