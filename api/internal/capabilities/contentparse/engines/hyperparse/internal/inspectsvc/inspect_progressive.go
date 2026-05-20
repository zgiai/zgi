package inspectsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	pdfadapter "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/adapters/pdf"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/export"
	pdforchestrator "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/orchestrators/pdf"
)

// InspectProgressSink matches playground inspectTask progress callbacks.
// partial contains a complete {"ok":true,"result":...} JSON payload.
type InspectProgressSink interface {
	SetProgress(currentPage, totalPages int, message string, partialOKWrappedJSON []byte)
	SetDone(okWrappedJSON []byte)
	SetError(err error)
}

// RunPDFInspectProgressive matches runInspectTask behavior: batched VLM parsing
// with partial JSON updates while work is in progress.
// taskID is used only for log prefixes.
func RunPDFInspectProgressive(ctx context.Context, in PDFInspectInput, taskID string, sink InspectProgressSink) {
	if ctx == nil {
		ctx = context.Background()
	}
	startedAt := time.Now()
	filename := strings.TrimSpace(in.Filename)
	if filename == "" {
		filename = "upload.pdf"
	}
	mode := strings.TrimSpace(in.Mode)
	if mode == "" {
		mode = "relaxed"
	}
	if len(in.Data) == 0 {
		sink.SetError(attachInspectFailureDebugDump(inspectFailureDebugState{
			Filename:    filename,
			Mode:        mode,
			Stage:       "input",
			TaskID:      taskID,
			SizeBytes:   len(in.Data),
			PreferFull:  in.PreferFull,
			Progressive: true,
			Err:         fmt.Errorf("empty pdf"),
		}))
		return
	}

	log.Printf("[ui.inspect.task] running task_id=%s file=%q mode=%s", taskID, filename, mode)
	inspectBasicStartedAt := time.Now()
	info, err := pdfadapter.InspectBasicBytes(in.Data, mode)
	inspectBasicDurationMs := time.Since(inspectBasicStartedAt).Milliseconds()
	if err != nil {
		log.Printf("[ui.inspect.task] failed task_id=%s stage=inspect_basic err=%v", taskID, err)
		sink.SetError(attachInspectFailureDebugDump(inspectFailureDebugState{
			Filename:    filename,
			Mode:        mode,
			Stage:       "inspect_basic",
			TaskID:      taskID,
			SizeBytes:   len(in.Data),
			PreferFull:  in.PreferFull,
			Progressive: true,
			Err:         err,
		}))
		return
	}
	fullDoc, fullDocTiming, err := pdforchestrator.ParseFullDocumentBytesWithBasicProfiled(in.Data, filename, mode, info)
	if err != nil {
		log.Printf("[ui.inspect.task] failed task_id=%s stage=parse_full_document err=%v", taskID, err)
		sink.SetError(attachInspectFailureDebugDump(inspectFailureDebugState{
			Filename:    filename,
			Mode:        mode,
			Stage:       "parse_full_document",
			TaskID:      taskID,
			SizeBytes:   len(in.Data),
			PageCount:   info.PageCount,
			CountSource: info.CountSource,
			PDFVersion:  info.Version,
			PreferFull:  in.PreferFull,
			Progressive: true,
			Err:         fmt.Errorf("build full document failed: %w", err),
		}))
		return
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
	if chW, ok := fullDoc["chunks"].(map[string]any); ok {
		nativeChunkCount = len(CoerceChunkItems(chW["items"]))
	}
	vlmConfigured := VLMConfigured()
	fullPageVLMEnabled := FullPageVLMFallbackEnabled()
	vlmPipelineErr := ""
	if suggestVLM && fullPageVLMEnabled && !vlmConfigured {
		vlmPipelineErr = missingVLMAPIKeyMessage
	}
	if suggestVLM {
		log.Printf("[ui.inspect.task] native summary task_id=%s suggest_vlm=true native_chunk_count=%d", taskID, nativeChunkCount)
	} else {
		log.Printf("[ui.inspect.task] native summary task_id=%s suggest_vlm=false native_chunk_count=%d", taskID, nativeChunkCount)
	}
	if len(nativeTimingDetail) > 0 {
		log.Printf("[ui.inspect.task] native_timing task_id=%s detail=%v", taskID, nativeTimingDetail)
	}
	if suggestVLM && fullPageVLMEnabled && vlmConfigured {
		log.Printf("[ui.inspect.task] route_decision=native_then_vlm reason=insufficient_native_content task_id=%s", taskID)
	} else if suggestVLM && fullPageVLMEnabled {
		log.Printf("[ui.inspect.task] route_decision=native_only reason=missing_vlm_api_key task_id=%s", taskID)
	} else if suggestVLM {
		log.Printf("[ui.inspect.task] route_decision=rules_plus_image_vlm reason=full_page_vlm_disabled task_id=%s", taskID)
	}
	recognitionSource := "native"
	vlmModel := ""
	vlmText := ""
	vlmRenderEngine := ""
	var plannedPages []int
	var processedPages []int
	var renderedPageDataURLs []string
	var renderedPageNumbers []int
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
	var regionalFallback regionalFallbackResult
	if fullDocumentLowConfidenceRegionCount(fullDoc) > 0 && regionalFallbackEnabled() {
		sink.SetProgress(0, effectivePageCount, "Repairing low-confidence regions with OCR/VLM", nil)
		log.Printf("[ui.inspect.task] regional fallback start task_id=%s regions=%d", taskID, fullDocumentLowConfidenceRegionCount(fullDoc))
		regionalFallback = ApplyRegionalOCRVLMFallback(ctx, in.Data, fullDoc)
		log.Printf("[ui.inspect.task] regional fallback done task_id=%s status=%s count=%d added=%d merged=%d anchor_bbox=%d model=%q warn=%q",
			taskID, regionalFallback.Status, regionalFallback.Count, regionalFallback.AddedCount, regionalFallback.MergedCount, regionalFallback.AnchorBBoxRepairCount, regionalFallback.Model, regionalFallback.Warning)
		if regionalFallback.Count > 0 {
			recognitionSource = "hybrid"
		}
	}
	if suggestVLM && fullPageVLMEnabled && vlmConfigured {
		maxPages := VLMFallbackMaxPages()
		pageSelection := vlmPageSelectionFromFullDoc(fullDoc, effectivePageCount, maxPages)
		renderScaleOverrides := buildOversizedPageRenderScaleOverrides(pageSelection, oversizedPages)
		renderConcurrency := PDFRenderConcurrency()
		renderedPagesCh, renderedPages, renderEngine, rerr := StreamRenderPDFSelectedPagesToDataURLsWithScales(in.Data, pageSelection, maxPages, renderConcurrency, renderScaleOverrides)
		if rerr != nil {
			log.Printf("[ui.inspect.task] failed task_id=%s stage=render_pages err=%v", taskID, rerr)
			sink.SetError(attachInspectFailureDebugDump(inspectFailureDebugState{
				Filename:       filename,
				Mode:           mode,
				Stage:          "render_pages",
				TaskID:         taskID,
				SizeBytes:      len(in.Data),
				PageCount:      effectivePageCount,
				CountSource:    info.CountSource,
				PDFVersion:     info.Version,
				PreferFull:     in.PreferFull,
				Progressive:    true,
				Err:            rerr,
				FullDoc:        fullDoc,
				PlannedPages:   plannedPages,
				ProcessedPages: processedPages,
				AppliedPages:   mergeVLMPagesApplied,
			}))
			return
		}
		plannedPages = normalizeProcessedPages(renderedPages)
		total := len(renderedPages)
		timingOverlapped := total > 1
		if total == 0 {
			log.Printf("[ui.inspect.task] failed task_id=%s stage=render_pages err=no_page_images", taskID)
			sink.SetError(attachInspectFailureDebugDump(inspectFailureDebugState{
				Filename:       filename,
				Mode:           mode,
				Stage:          "render_pages_no_images",
				TaskID:         taskID,
				SizeBytes:      len(in.Data),
				PageCount:      effectivePageCount,
				CountSource:    info.CountSource,
				PDFVersion:     info.Version,
				PreferFull:     in.PreferFull,
				Progressive:    true,
				Err:            fmt.Errorf("no rendered page images available for VLM"),
				FullDoc:        fullDoc,
				PlannedPages:   plannedPages,
				ProcessedPages: processedPages,
				AppliedPages:   mergeVLMPagesApplied,
			}))
			return
		}
		vlmRenderEngine = renderEngine
		log.Printf("[ui.inspect.task] vlm mode task_id=%s rendered_pages=%d render_engine=%s render_conc=%d", taskID, total, renderEngine, renderConcurrency)
		concurrency := VLMFallbackConcurrency()
		if concurrency <= 0 {
			concurrency = 1
		}
		if concurrency > total {
			concurrency = total
		}
		log.Printf("[ui.inspect.task] vlm paging task_id=%s page_jobs=%d conc=%d", taskID, total, concurrency)
		sink.SetProgress(0, total, "Running page-level VLM parsing", nil)
		nativeBaseItems := chunkItemsFromFullDoc(fullDoc)
		renderedPageResults := make(map[int]progressiveVLMPageResult, total)
		vlmPageResults := make(map[int]progressiveVLMPageResult, total)
		vlmPageErrors := make([]string, 0)
		completedCount := 0
		for pageResult := range streamProgressiveVLMPageResultsFromRenderedPages(renderedPagesCh, concurrency, oversizedPages) {
			completedCount++
			renderDurationMs += pageResult.RenderElapsedMs
			vlmDurationMs += pageResult.VLMElapsedMs
			vlmCallTiming.Merge(pageResult.VLMTiming)
			if pageResult.Attempted {
				processedPages = normalizeProcessedPages(append(processedPages, pageResult.PageNumber))
			}
			if pageResult.DataURL != "" {
				renderedPageResults[pageResult.RenderIndex] = pageResult
			}
			if pageResult.Err != nil {
				vlmPageErrors = append(vlmPageErrors, fmt.Sprintf("page %d: %v", pageResult.PageNumber, pageResult.Err))
				log.Printf("[ui.inspect.task] vlm page failed task_id=%s page=%d err=%v", taskID, pageResult.PageNumber, pageResult.Err)
			} else if len(pageResult.Items) == 0 {
				log.Printf("[ui.inspect.task] vlm page empty task_id=%s page=%d model=%q", taskID, pageResult.PageNumber, pageResult.Model)
			} else {
				vlmPageResults[pageResult.RenderIndex] = pageResult
				if vlmModel == "" && pageResult.Model != "" {
					vlmModel = pageResult.Model
				}
			}

			vlmItemsAll := flattenProgressiveVLMPageItems(vlmPageResults)
			if chW, ok := fullDoc["chunks"].(map[string]any); ok {
				merged, mergeStats := pdforchestrator.MergeNativeAndVLMChunkItemsForPages(nativeBaseItems, vlmItemsAll, processedPages)
				mergeNativeKeptCount = mergeStats.NativeKeptCount
				mergeVLMCount = mergeStats.VLMMergedCount
				mergeVLMPagesApplied = mergeStats.VLMPagesApplied
				chW["items"] = merged
				chW["count"] = len(merged)
				if mergeStats.VLMMergedCount > 0 {
					chW["vlm_merge"] = true
					if doc, ok := fullDoc["document"].(map[string]any); ok {
						pdforchestrator.RebuildTextSummaryAfterVLMMerge(doc, merged)
					}
				}
			}
			resultScope, coveragePages, mergeReport := buildInspectSemantics(
				effectivePageCount,
				plannedPages,
				processedPages,
				func() int {
					if mergeVLMCount > 0 {
						return mergeNativeKeptCount
					}
					return countNativeOnlyChunks(fullDoc)
				}(),
				mergeVLMCount,
				true,
				mergeVLMPagesApplied,
			)
			routeDebug := buildRouteDebug(fullDoc, plannedPages, processedPages, mergeVLMPagesApplied)
			totalDurationMs := time.Since(startedAt).Milliseconds()
			partialRecognitionSource := "native"
			if mergeVLMCount > 0 {
				partialRecognitionSource = "vlm"
			} else if regionalFallback.Count > 0 {
				partialRecognitionSource = "hybrid"
			}
			partial := map[string]any{
				"filename":    filename,
				"mode":        mode,
				"duration_ms": totalDurationMs,
				"timing_breakdown": buildInspectTimingBreakdown(
					nativeDurationMs,
					renderDurationMs,
					vlmDurationMs,
					0,
					totalDurationMs,
					timingOverlapped,
					nativeTimingDetail,
					buildInspectVLMTimingDetail(vlmCallTiming, 0, 0),
				),
				"page_count":                   effectivePageCount,
				"full_document":                fullDoc,
				"recognition_source":           partialRecognitionSource,
				"vlm_model":                    vlmModel,
				"vlm_rendered_pages":           len(renderedPageResults),
				"vlm_fast_preview":             true,
				"vlm_fast_preview_pages":       len(processedPages),
				"vlm_fast_preview_total_pages": total,
				"route_decision":               routeDecision,
				"route_debug":                  routeDebug,
				"suggest_vlm":                  suggestVLM,
				"result_scope":                 resultScope,
				"coverage_pages":               coveragePages,
				"merge_report":                 mergeReport,
			}
			if vlmRenderEngine != "" {
				partial["vlm_render_engine"] = vlmRenderEngine
			}
			if len(vlmPageErrors) > 0 {
				vlmPipelineErr = strings.Join(vlmPageErrors, "; ")
				partial["vlm_pipeline_error"] = vlmPipelineErr
			}
			partial["pipeline_stages"] = buildInspectPipelineStages(fullDoc, inspectPipelineOptions{
				RecognitionSource:       partialRecognitionSource,
				SuggestVLM:              suggestVLM,
				VLMRunning:              completedCount < total,
				VLMUsed:                 mergeVLMCount > 0,
				VLMError:                vlmPipelineErr,
				VLMModel:                vlmModel,
				VLMRenderedPages:        len(renderedPageResults),
				VLMAnalyzedPages:        len(processedPages),
				RegionalFallbackUsed:    regionalFallback.Count > 0,
				RegionalFallbackCount:   regionalFallback.Count,
				RegionalFallbackModel:   regionalFallback.Model,
				RegionalFallbackWarning: regionalFallback.Warning,
			})
			partial["dpt_export"] = export.BuildDPTExportFromFullDocument(fullDoc, filename, effectivePageCount)
			pb, _ := json.Marshal(map[string]any{"ok": true, "result": partial})
			sink.SetProgress(completedCount, total, fmt.Sprintf("Running page-level VLM parsing (%d/%d)", completedCount, total), pb)
			if completedCount == total || completedCount == 1 || completedCount%2 == 0 {
				log.Printf("[ui.inspect.task] progress task_id=%s page=%d/%d merged_chunks=%d", taskID, completedCount, total, len(vlmItemsAll))
			}
		}
		renderedPageDataURLs, renderedPageNumbers = rebuildProgressiveRenderedPages(renderedPageResults)
		if mergeVLMCount > 0 {
			recognitionSource = "vlm"
		}
		if chW, ok := fullDoc["chunks"].(map[string]any); ok {
			vlmText = JoinVLMChunkTexts(CoerceChunkItems(chW["items"]))
		}
	} else {
		log.Printf("[ui.inspect.task] route_decision=native_only reason=native_sufficient task_id=%s", taskID)
	}
	enrichStartedAt := time.Now()
	var nativeImageAppendWarn string
	var nativeImageAppendCount int
	var vlmImageCaptionModel, vlmImageCaptionWarn string
	var vlmImageCaptionStatus string
	var vlmImageCaptionCount int
	if mergeVLMCount > 0 && len(renderedPageDataURLs) == len(renderedPageNumbers) && len(renderedPageNumbers) > 0 {
		sidebarStartedAt := time.Now()
		vlmSidebarModel, vlmSidebarWarn, vlmSidebarCount = AppendVLMRightSidebarTextFromRenderedPages(fullDoc, renderedPageDataURLs, renderedPageNumbers)
		vlmSidebarDurationMs = time.Since(sidebarStartedAt).Milliseconds()
		if vlmSidebarCount > 0 {
			mergeVLMCount += vlmSidebarCount
			if chW, ok := fullDoc["chunks"].(map[string]any); ok {
				if doc, ok := fullDoc["document"].(map[string]any); ok {
					merged := CoerceChunkItems(chW["items"])
					pdforchestrator.RebuildTextSummaryAfterVLMMerge(doc, merged)
					log.Printf("[ui.inspect.task] vlm text_summary rebuilt after sidebar recovery task_id=%s", taskID)
				}
			}
		}
	}
	nativeImagePages := complementPages(effectivePageCount, nil)
	if mergeVLMCount > 0 {
		nativeImagePages = complementPages(effectivePageCount, mergeVLMPagesApplied)
	}
	if len(nativeImagePages) > 0 {
		log.Printf("[ui.inspect.task] native image_append start task_id=%s pages=%v", taskID, nativeImagePages)
		nativeImageAppendWarn, nativeImageAppendCount = AppendNativeImageChunksIfMissingForPages(in.Data, mode, fullDoc, nativeImagePages)
		log.Printf("[ui.inspect.task] native image_append done task_id=%s count=%d pages=%v warn=%q", taskID, nativeImageAppendCount, nativeImagePages, nativeImageAppendWarn)
		if HasImageChunksForPages(fullDoc, nativeImagePages) {
			log.Printf("[ui.inspect.task] native pages contain image chunks; starting VLM image captioning task_id=%s pages=%v", taskID, nativeImagePages)
			log.Printf("[ui.inspect.task] vlm image_caption start task_id=%s pages=%v", taskID, nativeImagePages)
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
			log.Printf("[ui.inspect.task] vlm image_caption done task_id=%s count=%d model=%q pages=%v warn=%q", taskID, vlmImageCaptionCount, vlmImageCaptionModel, nativeImagePages, vlmImageCaptionWarn)
		} else {
			vlmImageCaptionStatus = "skipped_no_image_chunks"
			log.Printf("[ui.inspect.task] vlm image_caption skipped task_id=%s reason=no_image_chunks pages=%v", taskID, nativeImagePages)
		}
	} else {
		vlmImageCaptionStatus = "skipped_no_native_pages"
		log.Printf("[ui.inspect.task] image_enrich skipped task_id=%s reason=no_native_kept_pages", taskID)
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
		false,
		mergeVLMPagesApplied,
	)
	routeDebug := buildRouteDebug(fullDoc, plannedPages, processedPages, mergeVLMPagesApplied)
	log.Printf("[ui.inspect.task] route_debug task_id=%s planned=%v processed=%v applied=%v", taskID, plannedPages, processedPages, mergeVLMPagesApplied)
	totalDurationMs := time.Since(startedAt).Milliseconds()
	vlmTimingDetail := buildInspectVLMTimingDetail(vlmCallTiming, vlmSidebarDurationMs, vlmImageCaptionDurationMs)
	res := map[string]any{
		"filename":             filename,
		"mode":                 mode,
		"duration_ms":          totalDurationMs,
		"timing_breakdown":     buildInspectTimingBreakdown(nativeDurationMs, renderDurationMs, vlmDurationMs, enrichDurationMs, totalDurationMs, len(plannedPages) > 1, nativeTimingDetail, vlmTimingDetail),
		"size_bytes":           info.FileSize,
		"pdf_version":          info.Version,
		"page_count":           effectivePageCount,
		"full_document":        fullDoc,
		"recognition_source":   recognitionSource,
		"vlm_model":            vlmModel,
		"vlm_recognition_text": vlmText,
		"route_decision":       routeDecision,
		"route_debug":          routeDebug,
		"suggest_vlm":          suggestVLM,
		"result_scope":         resultScope,
		"coverage_pages":       coveragePages,
		"merge_report":         mergeReport,
	}
	if vlmRenderEngine != "" {
		res["vlm_render_engine"] = vlmRenderEngine
	}
	if len(renderedPageNumbers) > 0 {
		res["vlm_rendered_pages"] = len(renderedPageNumbers)
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
	switch {
	case vlmPipelineErr != "" && vlmSidebarWarn != "":
		res["vlm_pipeline_error"] = vlmPipelineErr + "; " + vlmSidebarWarn
	case vlmPipelineErr != "":
		res["vlm_pipeline_error"] = vlmPipelineErr
	case vlmSidebarWarn != "":
		res["vlm_pipeline_error"] = vlmSidebarWarn
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
		VLMRenderedPages:        len(renderedPageNumbers),
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
	maybeAttachInspectDebugArtifacts(res, filename, taskID, true)
	finalBytes, _ := json.Marshal(map[string]any{"ok": true, "result": res})
	sink.SetDone(finalBytes)
	log.Printf("[ui.inspect.task] done task_id=%s recognition_source=%s page_count=%d", taskID, recognitionSource, effectivePageCount)
}
