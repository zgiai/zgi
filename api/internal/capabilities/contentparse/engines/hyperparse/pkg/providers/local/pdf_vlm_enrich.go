package local

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/inspectsvc"
	localocr "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/ocr"
	pdforchestrator "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/orchestrators/pdf"
	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/envconfig"
)

const envLocalVLMFallback = "LOCAL_VLM_FALLBACK"
const envLocalVLMFallbackMaxPages = "LOCAL_VLM_FALLBACK_MAX_PAGES"
const envLocalVLMFallbackTimeoutSeconds = "LOCAL_VLM_FALLBACK_TIMEOUT_SECONDS"
const envLocalVLMOCRBBoxAnchors = "LOCAL_VLM_OCR_BBOX_ANCHORS"
const envLocalVLMSidebarRecovery = "LOCAL_VLM_SIDEBAR_RECOVERY"
const envLocalVLMSidebarRecoveryMaxPages = "LOCAL_VLM_SIDEBAR_RECOVERY_MAX_PAGES"
const defaultLocalVLMFallbackMaxPages = 4
const defaultLocalVLMFallbackTimeoutSeconds = 35
const defaultLocalVLMSidebarRecoveryMaxPages = 4

func copyLocalDocumentDiagnostics(fullDoc map[string]any, inspect map[string]any) {
	if fullDoc == nil || inspect == nil {
		return
	}
	doc, _ := fullDoc["document"].(map[string]any)
	if doc == nil {
		return
	}
	for _, key := range []string{
		"suggest_vlm",
		"image_like_pdf",
		"business_doc_vlm_hint",
		"route_decision",
	} {
		if v, ok := doc[key]; ok {
			inspect[key] = v
		}
	}
}

func applyLocalVLMFallback(ctx context.Context, filename string, data []byte, mode string, pageCount int, fullDoc map[string]any, inspect map[string]any, opts extractcommon.ParseOptions) []int {
	if fullDoc == nil || inspect == nil {
		return nil
	}
	if localVLMAlreadyMerged(fullDoc) {
		inspect["recognition_source"] = "native+vlm"
		inspect["local_vlm_fallback"] = map[string]any{
			"status":  "already_applied",
			"applied": true,
			"reason":  "full_document_already_contains_vlm_merge",
		}
		return nil
	}

	setting := localVLMFallbackSettingForOptions(opts.ForceLocalVLM)
	if setting == "disabled" {
		inspect["local_vlm_fallback"] = map[string]any{
			"status": "disabled",
			"env":    envLocalVLMFallback,
		}
		return nil
	}

	candidatePages, reason := localVLMFallbackCandidatePages(fullDoc, pageCount)
	forced := setting == "force"
	if !forced && localFullDocSuspectGarbledText(fullDoc) {
		inspect["local_vlm_fallback"] = map[string]any{
			"status": "skipped_garbled_native_text",
			"reason": "prefer_local_ocr_for_corrupted_text_layer",
		}
		return nil
	}
	if !forced && localOCREnabled() && localFullDocRouteDecisionScanLike(fullDoc) {
		inspect["local_vlm_fallback"] = map[string]any{
			"status": "skipped_scan_like_ocr_first",
			"reason": "prefer_local_ocr_for_scan_like_page",
		}
		return nil
	}
	if !forced && len(candidatePages) == 0 {
		inspect["local_vlm_fallback"] = map[string]any{
			"status": "not_needed",
			"reason": "native_quality_ok",
		}
		return nil
	}
	if inspectsvc.VLMAPIKey() == "" {
		inspect["local_vlm_fallback"] = map[string]any{
			"status": "skipped_missing_vlm_api_key",
			"reason": coalesceString(reason, "forced"),
		}
		return nil
	}

	pages := candidatePages
	if len(pages) == 0 && forced {
		pages = localDocumentPages(pageCount, fullDoc)
		reason = "forced"
	}
	maxPages := localVLMFallbackMaxPages()
	pages = limitLocalVLMPages(pages, maxPages)
	if len(pages) == 0 {
		inspect["local_vlm_fallback"] = map[string]any{
			"status":    "skipped_no_pages",
			"reason":    coalesceString(reason, "no_candidate_pages"),
			"max_pages": maxPages,
		}
		return nil
	}

	log.Printf("[extractlocal.vlm] fallback render start file=%q pages=%v reason=%s", filename, pages, reason)
	renderStartedAt := time.Now()
	pageDataURLs, renderedPages, renderEngine, renderErr := inspectsvc.RenderPDFSelectedPagesToDataURLs(data, pages, maxPages)
	renderMs := time.Since(renderStartedAt).Milliseconds()
	if renderErr != nil {
		inspect["vlm_pipeline_error"] = renderErr.Error()
		inspect["local_vlm_fallback"] = map[string]any{
			"status":    "render_error",
			"reason":    coalesceString(reason, "candidate_pages"),
			"pages":     pages,
			"max_pages": maxPages,
			"render_ms": renderMs,
			"error":     renderErr.Error(),
		}
		return nil
	}
	if len(pageDataURLs) == 0 {
		inspect["local_vlm_fallback"] = map[string]any{
			"status":    "render_empty",
			"reason":    coalesceString(reason, "candidate_pages"),
			"pages":     pages,
			"max_pages": maxPages,
			"render_ms": renderMs,
		}
		return nil
	}

	log.Printf("[extractlocal.vlm] fallback call start file=%q rendered_pages=%v engine=%s", filename, renderedPages, renderEngine)
	vlmStartedAt := time.Now()
	vlmTimeout := localVLMFallbackTimeout()
	vlmCtx := ctx
	var vlmCancel context.CancelFunc
	if vlmTimeout > 0 {
		vlmCtx, vlmCancel = context.WithTimeout(ctx, vlmTimeout)
		defer vlmCancel()
	}
	vlmModel, vlmItems, rawReply, vlmErr := callLocalVLMFallbackPerPage(vlmCtx, pageDataURLs, renderedPages)
	vlmMs := time.Since(vlmStartedAt).Milliseconds()
	inspect["vlm_model"] = vlmModel
	inspect["vlm_render_engine"] = renderEngine
	inspect["vlm_rendered_pages"] = len(pageDataURLs)
	if vlmErr != nil {
		inspect["vlm_pipeline_error"] = vlmErr.Error()
		inspect["local_vlm_fallback"] = map[string]any{
			"status":    "vlm_error",
			"reason":    coalesceString(reason, "candidate_pages"),
			"pages":     renderedPages,
			"max_pages": maxPages,
			"model":     vlmModel,
			"render_ms": renderMs,
			"vlm_ms":    vlmMs,
			"timeout_s": int(vlmTimeout.Seconds()),
			"error":     vlmErr.Error(),
		}
		return nil
	}
	if len(vlmItems) == 0 {
		inspect["local_vlm_fallback"] = map[string]any{
			"status":        "no_vlm_chunks",
			"reason":        coalesceString(reason, "candidate_pages"),
			"pages":         renderedPages,
			"max_pages":     maxPages,
			"model":         vlmModel,
			"render_ms":     renderMs,
			"vlm_ms":        vlmMs,
			"raw_reply_len": len(rawReply),
		}
		return nil
	}

	chW, _ := fullDoc["chunks"].(map[string]any)
	if chW == nil {
		inspect["local_vlm_fallback"] = map[string]any{
			"status": "missing_native_chunks",
			"reason": coalesceString(reason, "candidate_pages"),
		}
		return nil
	}
	native := inspectsvc.CoerceChunkItems(chW["items"])
	ocrAnchors, anchorReport := localVLMOCRBBoxAnchors(ctx, filename, pageDataURLs, renderedPages, opts)
	if len(anchorReport) > 0 {
		inspect["local_vlm_ocr_bbox_anchors"] = anchorReport
	}
	alignmentNative := native
	if len(ocrAnchors) > 0 {
		alignmentNative = append(cloneChunkItems(native), ocrAnchors...)
	}
	merged, mergeStats := pdforchestrator.MergeNativeAndVLMChunkItemsForPages(alignmentNative, vlmItems, renderedPages)
	chW["items"] = merged
	chW["count"] = len(merged)
	if mergeStats.VLMMergedCount > 0 {
		chW["vlm_merge"] = true
		inspect["recognition_source"] = "native+vlm"
		inspect["vlm_recognition_text"] = inspectsvc.JoinVLMChunkTexts(vlmItems)
		if doc, ok := fullDoc["document"].(map[string]any); ok {
			pdforchestrator.RebuildTextSummaryAfterVLMMerge(doc, merged)
		}
	}
	mergeReport := map[string]any{
		"applied":               mergeStats.VLMMergedCount > 0,
		"native_kept_count":     mergeStats.NativeKeptCount,
		"native_residual_count": mergeStats.NativeResidualKept,
		"vlm_merged_count":      mergeStats.VLMMergedCount,
		"vlm_pages_applied":     mergeStats.VLMPagesApplied,
		"planned_vlm_pages":     pages,
		"max_pages":             maxPages,
		"processed_vlm_pages":   renderedPages,
		"native_input_count":    len(native),
		"merged_output_count":   len(merged),
		"raw_reply_len":         len(rawReply),
		"model":                 vlmModel,
		"render_engine":         renderEngine,
		"render_ms":             renderMs,
		"vlm_ms":                vlmMs,
		"vlm_timeout_s":         int(vlmTimeout.Seconds()),
		"vlm_call_mode":         "per_page_parallel",
		"trigger_reason":        coalesceString(reason, "candidate_pages"),
		"fallback_setting":      setting,
		"image_parts_submitted": len(pageDataURLs),
		"ocr_bbox_anchor_count": len(ocrAnchors),
	}
	inspect["merge_report"] = mergeReport
	inspect["local_vlm_fallback"] = map[string]any{
		"status":       localVLMFallbackStatus(mergeStats.VLMMergedCount),
		"applied":      mergeStats.VLMMergedCount > 0,
		"reason":       coalesceString(reason, "candidate_pages"),
		"pages":        renderedPages,
		"max_pages":    maxPages,
		"model":        vlmModel,
		"chunks":       len(vlmItems),
		"timeout_s":    int(vlmTimeout.Seconds()),
		"merge_report": mergeReport,
	}
	log.Printf("[extractlocal.vlm] fallback done file=%q vlm_items=%d merged=%d applied_pages=%v", filename, len(vlmItems), mergeStats.VLMMergedCount, mergeStats.VLMPagesApplied)
	return mergeStats.VLMPagesApplied
}

func localVLMOCRBBoxAnchors(ctx context.Context, filename string, pageDataURLs []string, renderedPages []int, opts extractcommon.ParseOptions) ([]map[string]any, map[string]any) {
	report := map[string]any{
		"enabled": false,
	}
	if !localVLMOCRBBoxAnchorsEnabled() {
		report["status"] = "disabled"
		return nil, report
	}
	if !localOCREnabled() {
		report["status"] = "skipped_ocr_disabled"
		return nil, report
	}
	if len(pageDataURLs) == 0 || len(pageDataURLs) != len(renderedPages) {
		report["status"] = "skipped_page_mismatch"
		report["rendered_pages"] = renderedPages
		report["image_parts"] = len(pageDataURLs)
		return nil, report
	}
	startedAt := time.Now()
	ocrConfig := localOCRConfigForFileWithOptions(filename, opts)
	concurrency := localOCRConcurrency()
	pageOCR := ocrPDFPageImages(ctx, pageDataURLs, ocrConfig, concurrency)
	anchors := make([]map[string]any, 0)
	for imageIdx, blocks := range pageOCR.BlocksByPage {
		if imageIdx < 0 || imageIdx >= len(renderedPages) {
			continue
		}
		page := renderedPages[imageIdx]
		for blockIdx, block := range blocks {
			if block.BBox == nil || strings.TrimSpace(block.Text) == "" {
				continue
			}
			anchors = append(anchors, map[string]any{
				"type":       "bbox_anchor",
				"page_index": page,
				"order":      blockIdx,
				"chunk_id":   fmt.Sprintf("ocr_bbox_anchor_p%d_%d", page, blockIdx),
				"text":       block.Text,
				"bbox": map[string]any{
					"left":   block.BBox.Left,
					"top":    block.BBox.Top,
					"right":  block.BBox.Right,
					"bottom": block.BBox.Bottom,
				},
				"payload": map[string]any{
					"bbox_anchor_only": true,
					"ocr_engine":       pageOCR.Engine,
					"ocr_bbox_source":  block.BBoxSource,
				},
			})
		}
	}
	report["enabled"] = true
	report["status"] = "done"
	report["engine"] = pageOCR.Engine
	report["pages"] = renderedPages
	report["anchors"] = len(anchors)
	report["duration_ms"] = time.Since(startedAt).Milliseconds()
	return anchors, report
}

func localVLMOCRBBoxAnchorsEnabled() bool {
	switch strings.ToLower(envconfig.String(envLocalVLMOCRBBoxAnchors)) {
	case "0", "false", "no", "off", "disabled":
		return false
	default:
		return true
	}
}

func localVLMFallbackTimeout() time.Duration {
	for _, key := range []string{envLocalVLMFallbackTimeoutSeconds, "CONTENT_PARSE_VLM_FALLBACK_TIMEOUT_SECONDS", "DOCSTILL_VLM_FALLBACK_TIMEOUT_SECONDS"} {
		raw := envconfig.String(key)
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}
		if n <= 0 {
			return 0
		}
		return time.Duration(n) * time.Second
	}
	return time.Duration(defaultLocalVLMFallbackTimeoutSeconds) * time.Second
}

func cloneChunkItems(in []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, item := range in {
		out = append(out, cloneMapAny(item))
	}
	return out
}

func localFullDocSuspectGarbledText(fullDoc map[string]any) bool {
	chW, _ := fullDoc["chunks"].(map[string]any)
	if chW == nil {
		return false
	}
	items := inspectsvc.CoerceChunkItems(chW["items"])
	if len(items) == 0 {
		return false
	}
	chunks := make([]extractcommon.Chunk, 0, len(items))
	for _, item := range items {
		chunks = append(chunks, extractcommon.Chunk{
			Type: strings.TrimSpace(fmt.Sprint(item["type"])),
			Text: strings.TrimSpace(fmt.Sprint(item["text"])),
		})
	}
	return suspectLocalGarbledText(chunks)
}

func localFullDocRouteDecisionScanLike(fullDoc map[string]any) bool {
	doc, _ := fullDoc["document"].(map[string]any)
	if doc == nil {
		return false
	}
	route, _ := doc["route_decision"].(map[string]any)
	if route == nil {
		return false
	}
	mode := strings.TrimSpace(fmt.Sprint(route["recommended_mode"]))
	if mode != "vlm_candidate" {
		return false
	}
	for _, reason := range normalizeMapSlice(route["reasons"]) {
		if strings.TrimSpace(fmt.Sprint(reason["code"])) == "scan_like" {
			return true
		}
	}
	return false
}

func callLocalVLMFallbackPerPage(ctx context.Context, pageDataURLs []string, pageNumbers []int) (model string, chunks []map[string]any, raw string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(pageDataURLs) == 0 {
		return "", nil, "", nil
	}
	if len(pageDataURLs) != len(pageNumbers) {
		return "", nil, "", fmt.Errorf("rendered pages mismatch: data_urls=%d page_numbers=%d", len(pageDataURLs), len(pageNumbers))
	}
	concurrency := inspectsvc.VLMFallbackConcurrency()
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > len(pageDataURLs) {
		concurrency = len(pageDataURLs)
	}
	type pageResult struct {
		idx   int
		page  int
		model string
		items []map[string]any
		raw   string
		err   error
	}
	results := make([]pageResult, len(pageDataURLs))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i := range pageDataURLs {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = pageResult{idx: i, page: pageNumbers[i], err: ctx.Err()}
				return
			}
			defer func() { <-sem }()
			if err := ctx.Err(); err != nil {
				results[i] = pageResult{idx: i, page: pageNumbers[i], err: err}
				return
			}
			page := pageNumbers[i]
			m, items, rw, callErr := inspectsvc.CallDashscopeVLMFallbackStructuredContext(
				ctx,
				inspectsvc.VLMImageContentFromDataURLs([]string{pageDataURLs[i]}),
			)
			if callErr == nil {
				inspectsvc.RemapVLMChunkPages(items, []int{page})
			}
			results[i] = pageResult{idx: i, page: page, model: m, items: items, raw: rw, err: callErr}
		}()
	}
	wg.Wait()

	rawParts := make([]string, 0, len(results))
	merged := make([]map[string]any, 0, len(results)*4)
	var errs []string
	for _, result := range results {
		if result.err != nil {
			errs = append(errs, fmt.Sprintf("page %d: %v", result.page, result.err))
			continue
		}
		if model == "" && strings.TrimSpace(result.model) != "" {
			model = result.model
		}
		if strings.TrimSpace(result.raw) != "" {
			rawParts = append(rawParts, result.raw)
		}
		merged = append(merged, result.items...)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		pi, pj := intAny(merged[i]["page_index"]), intAny(merged[j]["page_index"])
		if pi != pj {
			return pi < pj
		}
		return intAny(merged[i]["order"]) < intAny(merged[j]["order"])
	})
	raw = strings.Join(rawParts, "\n\n")
	if len(merged) == 0 && len(errs) > 0 {
		return model, nil, raw, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	if len(errs) > 0 {
		log.Printf("[extractlocal.vlm] per-page partial errors: %s", strings.Join(errs, "; "))
	}
	return model, merged, raw, nil
}

func applyLocalImageCaptions(ctx context.Context, data []byte, mode string, pageCount int, fullDoc map[string]any, inspect map[string]any, _ []int, progress func(extractcommon.ParseProgress)) {
	if ctx == nil {
		ctx = context.Background()
	}
	if fullDoc == nil || inspect == nil {
		return
	}
	pages := localDocumentPages(pageCount, fullDoc)
	appendWarn, appendCount := inspectsvc.AppendNativeImageChunksIfMissingForPages(data, mode, fullDoc, pages)
	if appendCount > 0 {
		inspect["native_image_append_count"] = appendCount
	}
	if appendWarn != "" {
		inspect["native_image_append_warning"] = appendWarn
	}

	previewWarn, previewCount := inspectsvc.AttachNativeImagePreviewPayloads(data, mode, fullDoc)
	if previewCount > 0 {
		inspect["native_image_preview_count"] = previewCount
	}
	if previewWarn != "" {
		inspect["native_image_preview_warning"] = previewWarn
	}

	if strings.TrimSpace(firstNonEmptyEnv("CONTENT_PARSE_VLM_IMAGE_CAPTION", "DOCSTILL_VLM_IMAGE_CAPTION")) == "0" {
		inspect["vlm_image_caption_status"] = "disabled"
		emitParseProgress(progress, extractcommon.ParseProgress{
			Stage:   "image_caption",
			Status:  "disabled",
			Message: "image_caption_disabled",
		})
		return
	}
	if !inspectsvc.HasImageChunksForPages(fullDoc, pages) {
		inspect["vlm_image_caption_status"] = "skipped_no_image_chunks"
		emitParseProgress(progress, extractcommon.ParseProgress{
			Stage:   "image_caption",
			Status:  "skipped",
			Message: "skipped_no_image_chunks",
		})
		return
	}
	if inspectsvc.VLMAPIKey() == "" {
		inspect["vlm_image_caption_status"] = "skipped_missing_vlm_api_key"
		emitParseProgress(progress, extractcommon.ParseProgress{
			Stage:   "image_caption",
			Status:  "skipped",
			Message: "skipped_missing_vlm_api_key",
		})
		return
	}

	startedAt := time.Now()
	model, warn, count := inspectsvc.EnrichImageChunksWithVLMCaptionsForPagesProgressContext(ctx, data, mode, fullDoc, pages, func(p inspectsvc.ImageCaptionProgress) {
		status := p.Status
		if status == "done" {
			status = "running"
		}
		detail := map[string]any{}
		if p.Model != "" {
			detail["model"] = p.Model
		}
		if p.Mode != "" {
			detail["mode"] = p.Mode
		}
		if p.ObjectNumber > 0 {
			detail["object_number"] = p.ObjectNumber
		}
		if p.Error != "" {
			detail["error"] = p.Error
		}
		emitParseProgress(progress, extractcommon.ParseProgress{
			Stage:   "image_caption",
			Status:  status,
			Message: "image_caption",
			Current: p.Completed,
			Total:   p.Total,
			Detail:  detail,
		})
	})
	if count > 0 {
		inspect["vlm_image_caption_count"] = count
	}
	if model != "" {
		inspect["vlm_image_caption_model"] = model
	}
	if warn != "" {
		inspect["vlm_image_caption_warning"] = warn
	}
	inspect["vlm_image_caption_duration_ms"] = time.Since(startedAt).Milliseconds()
	switch {
	case count > 0:
		inspect["vlm_image_caption_status"] = "applied"
		emitParseProgress(progress, extractcommon.ParseProgress{
			Stage:   "image_caption",
			Status:  "done",
			Message: "image_caption",
			Current: count,
			Total:   count,
			Detail:  map[string]any{"model": model},
		})
	case warn != "":
		inspect["vlm_image_caption_status"] = "warning"
		emitParseProgress(progress, extractcommon.ParseProgress{
			Stage:   "image_caption",
			Status:  "warning",
			Message: warn,
			Current: count,
			Total:   count,
			Detail:  map[string]any{"model": model},
		})
	default:
		inspect["vlm_image_caption_status"] = "no_caption_generated"
		emitParseProgress(progress, extractcommon.ParseProgress{
			Stage:   "image_caption",
			Status:  "skipped",
			Message: "no_caption_generated",
		})
	}
}

func applyLocalSidebarRecovery(ctx context.Context, data []byte, mode string, pageCount int, fullDoc map[string]any, inspect map[string]any, force bool, opts extractcommon.ParseOptions) []int {
	if fullDoc == nil || inspect == nil {
		return nil
	}
	setting := localVLMSidebarRecoverySettingForOptions(force)
	if setting == "disabled" {
		inspect["vlm_sidebar_recovery_status"] = "disabled"
		return nil
	}

	pages, reason := localSidebarRecoveryCandidatePages(fullDoc, pageCount, setting == "force")
	if len(pages) == 0 {
		inspect["vlm_sidebar_recovery_status"] = "not_needed"
		return nil
	}

	maxPages := localVLMSidebarRecoveryMaxPages()
	pages = limitLocalVLMPages(pages, maxPages)
	if len(pages) == 0 {
		inspect["vlm_sidebar_recovery_status"] = "skipped_no_pages"
		inspect["vlm_sidebar_recovery_reason"] = reason
		inspect["vlm_sidebar_recovery_max_pages"] = maxPages
		return nil
	}

	log.Printf("[extractlocal.vlm] sidebar recovery render start pages=%v reason=%s", pages, reason)
	renderStartedAt := time.Now()
	pageDataURLs, renderedPages, renderEngine, renderErr := inspectsvc.RenderPDFSelectedPagesToDataURLs(data, pages, maxPages)
	renderMs := time.Since(renderStartedAt).Milliseconds()
	if renderErr != nil {
		inspect["vlm_sidebar_recovery_status"] = "render_error"
		inspect["vlm_sidebar_recovery_reason"] = reason
		inspect["vlm_sidebar_recovery_pages"] = pages
		inspect["vlm_sidebar_recovery_render_ms"] = renderMs
		inspect["vlm_sidebar_recovery_warning"] = renderErr.Error()
		return nil
	}
	if len(pageDataURLs) == 0 {
		inspect["vlm_sidebar_recovery_status"] = "render_empty"
		inspect["vlm_sidebar_recovery_reason"] = reason
		inspect["vlm_sidebar_recovery_pages"] = pages
		inspect["vlm_sidebar_recovery_render_ms"] = renderMs
		return nil
	}

	if localSidebarOCREnabled() {
		ocrStartedAt := time.Now()
		ocrEngine, ocrWarn, ocrCount := inspectsvc.AppendOCRRightSidebarTextFromRenderedPagesWithConfig(ctx, fullDoc, pageDataURLs, renderedPages, localSidebarOCRConfigForOptions(opts))
		inspect["local_sidebar_ocr_status"] = localSidebarOCRStatus(ocrCount, ocrWarn)
		if ocrEngine != "" {
			inspect["local_sidebar_ocr_engine"] = ocrEngine
		}
		inspect["local_sidebar_ocr_count"] = ocrCount
		inspect["local_sidebar_ocr_duration_ms"] = time.Since(ocrStartedAt).Milliseconds()
		if ocrWarn != "" {
			inspect["local_sidebar_ocr_warning"] = ocrWarn
		}
		if ocrCount > 0 {
			inspect["recognition_source"] = "native+ocr"
			if doc, ok := fullDoc["document"].(map[string]any); ok {
				if chW, ok := fullDoc["chunks"].(map[string]any); ok {
					pdforchestrator.RebuildTextSummaryAfterVLMMerge(doc, inspectsvc.CoerceChunkItems(chW["items"]))
				}
			}
			inspect["vlm_sidebar_recovery_status"] = "skipped_after_local_ocr"
			inspect["vlm_sidebar_recovery_reason"] = reason
			inspect["vlm_sidebar_recovery_pages"] = renderedPages
			inspect["vlm_sidebar_recovery_max_pages"] = maxPages
			inspect["vlm_sidebar_recovery_render_engine"] = renderEngine
			inspect["vlm_sidebar_recovery_render_ms"] = renderMs
			return renderedPages
		}
	} else {
		inspect["local_sidebar_ocr_status"] = "disabled"
	}

	if inspectsvc.VLMAPIKey() == "" {
		inspect["vlm_sidebar_recovery_status"] = "skipped_missing_vlm_api_key"
		inspect["vlm_sidebar_recovery_reason"] = reason
		inspect["vlm_sidebar_recovery_pages"] = renderedPages
		inspect["vlm_sidebar_recovery_max_pages"] = maxPages
		inspect["vlm_sidebar_recovery_render_engine"] = renderEngine
		inspect["vlm_sidebar_recovery_render_ms"] = renderMs
		return renderedPages
	}

	startedAt := time.Now()
	model, warn, count := inspectsvc.AppendVLMRightSidebarTextFromRenderedPages(fullDoc, pageDataURLs, renderedPages)
	vlmMs := time.Since(startedAt).Milliseconds()
	inspect["vlm_sidebar_recovery_status"] = localSidebarRecoveryStatus(count)
	inspect["vlm_sidebar_recovery_reason"] = reason
	inspect["vlm_sidebar_recovery_pages"] = renderedPages
	inspect["vlm_sidebar_recovery_max_pages"] = maxPages
	inspect["vlm_sidebar_recovery_count"] = count
	inspect["vlm_sidebar_recovery_render_engine"] = renderEngine
	inspect["vlm_sidebar_recovery_render_ms"] = renderMs
	inspect["vlm_sidebar_recovery_duration_ms"] = vlmMs
	if model != "" {
		inspect["vlm_sidebar_recovery_model"] = model
	}
	if warn != "" {
		inspect["vlm_sidebar_recovery_warning"] = warn
	}
	if count > 0 {
		inspect["recognition_source"] = "native+vlm"
		if doc, ok := fullDoc["document"].(map[string]any); ok {
			if chW, ok := fullDoc["chunks"].(map[string]any); ok {
				pdforchestrator.RebuildTextSummaryAfterVLMMerge(doc, inspectsvc.CoerceChunkItems(chW["items"]))
			}
		}
	}
	log.Printf("[extractlocal.vlm] sidebar recovery done count=%d pages=%v", count, renderedPages)
	return renderedPages
}

func localSidebarOCRConfigForOptions(opts extractcommon.ParseOptions) localocr.Config {
	cfg := localocr.LoadConfig(15 * time.Second)
	if engine := normalizeRequestedOCREngine(opts.OCREngine); engine != "" {
		cfg.Engine = engine
	}
	if lang := explicitLocalOCRLang(); lang != "" {
		cfg.Lang = lang
	} else if cfg.EngineName() == localocr.EnginePaddleOCR {
		cfg.Lang = "en"
	} else {
		cfg.Lang = "eng"
	}
	return cfg
}

func finalizeLocalVLMChunksForExport(fullDoc map[string]any, inspect map[string]any) {
	if fullDoc == nil || inspect == nil || !localVLMAlreadyMerged(fullDoc) {
		return
	}
	dropped := mergeDuplicateVLMImageChunksIntoNativeImages(fullDoc)
	if dropped > 0 {
		inspect["local_vlm_image_dedupe_count"] = dropped
	}
	sortLocalChunksByPageOrder(fullDoc)
}

func mergeDuplicateVLMImageChunksIntoNativeImages(fullDoc map[string]any) int {
	chW, _ := fullDoc["chunks"].(map[string]any)
	if chW == nil {
		return 0
	}
	items := inspectsvc.CoerceChunkItems(chW["items"])
	if len(items) == 0 {
		return 0
	}
	type nativeImage struct {
		page int
		box  localBox
		item map[string]any
	}
	nativeByPage := map[int][]nativeImage{}
	for _, item := range items {
		if !localChunkIsImage(item) || localChunkIsFromVLM(item) {
			continue
		}
		box, ok := localChunkBox(item)
		if !ok {
			continue
		}
		page := intAny(item["page_index"])
		nativeByPage[page] = append(nativeByPage[page], nativeImage{page: page, box: box, item: item})
	}
	if len(nativeByPage) == 0 {
		return 0
	}
	for page := range nativeByPage {
		sort.SliceStable(nativeByPage[page], func(i, j int) bool {
			a, b := nativeByPage[page][i].box, nativeByPage[page][j].box
			if a.top != b.top {
				return a.top < b.top
			}
			return a.left < b.left
		})
	}

	filtered := make([]map[string]any, 0, len(items))
	dropped := 0
	nativeCursor := map[int]int{}
	for _, item := range items {
		if !localChunkIsImage(item) || !localChunkIsFromVLM(item) {
			filtered = append(filtered, item)
			continue
		}
		page := intAny(item["page_index"])
		box, hasBox := localChunkBox(item)
		overlapsNative := false
		var target map[string]any
		natives := nativeByPage[page]
		cursor := nativeCursor[page]
		if cursor < len(natives) {
			target = natives[cursor].item
			nativeCursor[page] = cursor + 1
			overlapsNative = true
		}
		if !overlapsNative && hasBox {
			for _, native := range natives {
				if localBoxIoU(box, native.box) >= 0.45 {
					overlapsNative = true
					target = native.item
					break
				}
			}
		}
		if overlapsNative {
			mergeVLMImageOrderIntoNative(target, item)
			dropped++
			continue
		}
		filtered = append(filtered, item)
	}
	if dropped == 0 {
		return 0
	}
	chW["items"] = filtered
	chW["count"] = len(filtered)
	return dropped
}

func mergeVLMImageOrderIntoNative(native map[string]any, vlm map[string]any) {
	if native == nil || vlm == nil {
		return
	}
	if order := intAny(vlm["order"]); order > 0 {
		native["order"] = order
	}
	nativeText := strings.TrimSpace(fmt.Sprint(native["text"]))
	if nativeText == "" || nativeText == "<nil>" || strings.EqualFold(nativeText, "null") {
		if vlmText := strings.TrimSpace(fmt.Sprint(vlm["text"])); vlmText != "" && vlmText != "<nil>" {
			native["text"] = vlmText
		}
	}
	payload, _ := native["payload"].(map[string]any)
	if payload == nil {
		payload = map[string]any{}
		native["payload"] = payload
	}
	payload["vlm_image_order_source"] = "vlm_page_chunk"
}

func sortLocalChunksByPageOrder(fullDoc map[string]any) {
	chW, _ := fullDoc["chunks"].(map[string]any)
	if chW == nil {
		return
	}
	items := inspectsvc.CoerceChunkItems(chW["items"])
	if len(items) == 0 {
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		pi, pj := intAny(items[i]["page_index"]), intAny(items[j]["page_index"])
		if pi != pj {
			return pi < pj
		}
		return intAny(items[i]["order"]) < intAny(items[j]["order"])
	})
	chW["items"] = items
	chW["count"] = len(items)
}

func localChunkIsImage(item map[string]any) bool {
	typ := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["type"])))
	return typ == "image" || typ == "figure"
}

func localChunkIsFromVLM(item map[string]any) bool {
	if strings.TrimSpace(fmt.Sprint(item["source"])) == "vlm" {
		return true
	}
	return strings.TrimSpace(fmt.Sprint(item["vlm_merge"])) == "from_vlm"
}

type localBox struct {
	left   float64
	top    float64
	right  float64
	bottom float64
}

func localChunkBox(item map[string]any) (localBox, bool) {
	bb, _ := item["bbox"].(map[string]any)
	if bb == nil {
		return localBox{}, false
	}
	left := floatAny(bb["left"])
	right := floatAny(bb["right"])
	top := floatAny(bb["top"])
	bottom := floatAny(bb["bottom"])
	if right < left {
		left, right = right, left
	}
	if bottom < top {
		top, bottom = bottom, top
	}
	if right <= left || bottom <= top {
		return localBox{}, false
	}
	return localBox{left: left, top: top, right: right, bottom: bottom}, true
}

func localBoxIoU(a, b localBox) float64 {
	left := maxFloat(a.left, b.left)
	right := minFloat(a.right, b.right)
	top := maxFloat(a.top, b.top)
	bottom := minFloat(a.bottom, b.bottom)
	if right <= left || bottom <= top {
		return 0
	}
	inter := (right - left) * (bottom - top)
	areaA := (a.right - a.left) * (a.bottom - a.top)
	areaB := (b.right - b.left) * (b.bottom - b.top)
	if areaA <= 0 || areaB <= 0 {
		return 0
	}
	return inter / (areaA + areaB - inter)
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func dedupeDocumentFigureChunks(doc *extractcommon.DocumentResult) {
	if doc == nil || len(doc.Chunks) < 2 {
		return
	}
	drop := map[int]bool{}
	dropped := 0
	for i := range doc.Chunks {
		if drop[i] || !documentChunkIsFigure(doc.Chunks[i]) || doc.Chunks[i].BBox == nil {
			continue
		}
		for j := i + 1; j < len(doc.Chunks); j++ {
			if drop[j] || !documentChunkIsFigure(doc.Chunks[j]) || doc.Chunks[j].BBox == nil {
				continue
			}
			if doc.Chunks[i].Page != doc.Chunks[j].Page {
				continue
			}
			if documentBBoxIoU(doc.Chunks[i].BBox, doc.Chunks[j].BBox) < 0.78 {
				continue
			}
			keepJ := preferDocumentFigureChunk(doc.Chunks[j], doc.Chunks[i])
			if keepJ {
				mergeDocumentFigurePayload(&doc.Chunks[j], doc.Chunks[i])
				drop[i] = true
				dropped++
				break
			}
			mergeDocumentFigurePayload(&doc.Chunks[i], doc.Chunks[j])
			drop[j] = true
			dropped++
		}
	}
	if dropped == 0 {
		return
	}
	kept := make([]extractcommon.Chunk, 0, len(doc.Chunks)-dropped)
	for i, chunk := range doc.Chunks {
		if drop[i] {
			continue
		}
		chunk.Ordinal = len(kept) + 1
		kept = append(kept, chunk)
	}
	doc.Chunks = kept
	doc.Markdown = rebuildDocumentMarkdown(kept)
	if doc.Diagnostics == nil {
		doc.Diagnostics = map[string]any{}
	}
	doc.Diagnostics["local_figure_dedupe"] = map[string]any{
		"applied":  true,
		"dropped":  dropped,
		"strategy": "bbox_iou_prefer_contextual_vlm_figure",
	}
}

func mergeDocumentFigurePayload(target *extractcommon.Chunk, source extractcommon.Chunk) {
	if target == nil || len(source.Payload) == 0 {
		return
	}
	if target.Payload == nil {
		target.Payload = cloneMapAny(source.Payload)
		return
	}
	for k, v := range source.Payload {
		if _, exists := target.Payload[k]; !exists {
			target.Payload[k] = v
		}
	}
}

func normalizeDecorativeLogoFigures(doc *extractcommon.DocumentResult) {
	if doc == nil {
		return
	}
	updated := 0
	for i := range doc.Chunks {
		ch := &doc.Chunks[i]
		if !documentChunkIsFigure(*ch) || !looksLikeDecorativeLogoFigure(*ch) {
			continue
		}
		ch.Type = "marginalia"
		ch.Subtype = "logo"
		updated++
	}
	if updated == 0 {
		return
	}
	if doc.Diagnostics == nil {
		doc.Diagnostics = map[string]any{}
	}
	doc.Diagnostics["local_logo_figure_demote"] = map[string]any{
		"applied": true,
		"count":   updated,
	}
}

func looksLikeDecorativeLogoFigure(ch extractcommon.Chunk) bool {
	text := strings.ToLower(strings.TrimSpace(ch.Text + "\n" + ch.Markdown))
	if !strings.Contains(text, "logo") {
		return false
	}
	if ch.BBox == nil {
		return true
	}
	width := ch.BBox.Right - ch.BBox.Left
	height := ch.BBox.Bottom - ch.BBox.Top
	if width <= 0 || height <= 0 {
		return true
	}
	return ch.BBox.Top <= 0.18 && width <= 0.35 && height <= 0.20
}

func documentChunkIsFigure(chunk extractcommon.Chunk) bool {
	typ := strings.ToLower(strings.TrimSpace(chunk.Type))
	return typ == "figure" || typ == "image"
}

func preferDocumentFigureChunk(candidate extractcommon.Chunk, current extractcommon.Chunk) bool {
	candidateScore := documentFigureScore(candidate)
	currentScore := documentFigureScore(current)
	if candidateScore != currentScore {
		return candidateScore > currentScore
	}
	return candidate.Ordinal > current.Ordinal
}

func documentFigureScore(chunk extractcommon.Chunk) int {
	txt := strings.ToLower(strings.TrimSpace(chunk.Text + "\n" + chunk.Markdown))
	score := 0
	for _, token := range []string{"\u56fea", "\u56feb", "figure a", "figure b", "image a", "image b", "photomicrograph"} {
		if strings.Contains(txt, token) {
			score += 4
			break
		}
	}
	if strings.Contains(txt, "<<:figure:") {
		score++
	}
	if !strings.Contains(txt, "\u8fd9\u662f\u4e00\u5f20") {
		score++
	}
	return score
}

func documentBBoxIoU(a, b *extractcommon.BBox) float64 {
	if a == nil || b == nil {
		return 0
	}
	left := maxFloat(a.Left, b.Left)
	right := minFloat(a.Right, b.Right)
	top := maxFloat(a.Top, b.Top)
	bottom := minFloat(a.Bottom, b.Bottom)
	if right <= left || bottom <= top {
		return 0
	}
	inter := (right - left) * (bottom - top)
	areaA := (a.Right - a.Left) * (a.Bottom - a.Top)
	areaB := (b.Right - b.Left) * (b.Bottom - b.Top)
	if areaA <= 0 || areaB <= 0 {
		return 0
	}
	return inter / (areaA + areaB - inter)
}

func rebuildDocumentMarkdown(chunks []extractcommon.Chunk) string {
	parts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		md := strings.TrimSpace(chunk.Markdown)
		if md == "" {
			md = strings.TrimSpace(chunk.Text)
		}
		if md != "" {
			parts = append(parts, md)
		}
	}
	return strings.Join(parts, "\n\n")
}

func localVLMFallbackStatus(merged int) string {
	if merged > 0 {
		return "applied"
	}
	return "no_merge"
}

func localVLMAlreadyMerged(fullDoc map[string]any) bool {
	chW, _ := fullDoc["chunks"].(map[string]any)
	if chW == nil {
		return false
	}
	if v, ok := chW["vlm_merge"].(bool); ok && v {
		return true
	}
	for _, item := range inspectsvc.CoerceChunkItems(chW["items"]) {
		if strings.HasPrefix(strings.TrimSpace(fmt.Sprint(item["vlm_merge"])), "from_vlm") {
			return true
		}
	}
	return false
}

func localVLMFallbackSetting() string {
	return localVLMFallbackSettingForOptions(false)
}

func localVLMFallbackSettingForOptions(force bool) string {
	if force {
		return "force"
	}
	raw := strings.ToLower(envconfig.String(envLocalVLMFallback))
	switch raw {
	case "0", "false", "no", "off", "disabled":
		return "disabled"
	case "1", "true", "yes", "on", "force", "always":
		return "force"
	default:
		return "auto"
	}
}

func localVLMSidebarRecoverySetting() string {
	return localVLMSidebarRecoverySettingForOptions(false)
}

func localVLMSidebarRecoverySettingForOptions(force bool) string {
	if force {
		return "force"
	}
	raw := strings.ToLower(envconfig.String(envLocalVLMSidebarRecovery))
	switch raw {
	case "0", "false", "no", "off", "disabled":
		return "disabled"
	case "1", "true", "yes", "on", "force", "always":
		return "force"
	default:
		return "auto"
	}
}

func localVLMFallbackMaxPages() int {
	raw := envconfig.String(envLocalVLMFallbackMaxPages)
	if raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return defaultLocalVLMFallbackMaxPages
		}
		return n
	}
	if n := inspectsvc.VLMFallbackMaxPages(); n > 0 {
		return n
	}
	return defaultLocalVLMFallbackMaxPages
}

func localVLMSidebarRecoveryMaxPages() int {
	raw := envconfig.String(envLocalVLMSidebarRecoveryMaxPages)
	if raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return defaultLocalVLMSidebarRecoveryMaxPages
		}
		return n
	}
	return defaultLocalVLMSidebarRecoveryMaxPages
}

func localEngineContract() map[string]any {
	return localEngineContractForOptions(extractcommon.ParseOptions{})
}

func localEngineContractForOptions(opts extractcommon.ParseOptions) map[string]any {
	imageCaptionSetting := strings.TrimSpace(firstNonEmptyEnv("CONTENT_PARSE_VLM_IMAGE_CAPTION", "DOCSTILL_VLM_IMAGE_CAPTION"))
	return map[string]any{
		"engine":                          "local",
		"backend":                         "local",
		"mineru_used":                     false,
		"gpu_required":                    false,
		"native_first_pass":               true,
		"global_force_vlm_ignored":        true,
		"vlm_fallback_setting":            localVLMFallbackSettingForOptions(opts.ForceLocalVLM),
		"vlm_fallback_max_pages":          localVLMFallbackMaxPages(),
		"vlm_fallback_min_runes_per_page": localVLMFallbackMinRunesPerPage(),
		"vlm_sidebar_recovery_setting":    localVLMSidebarRecoverySettingForOptions(opts.ForceLocalSidebarRecovery),
		"vlm_sidebar_recovery_max_pages":  localVLMSidebarRecoveryMaxPages(),
		"local_sidebar_ocr_enabled":       localSidebarOCREnabled(),
		"local_image_vlm_setting":         localImageVLMSetting(),
		"local_image_vlm_enabled":         localImageVLMEnabled(),
		"vlm_image_caption_enabled":       imageCaptionSetting != "0",
	}
}

func localVLMFallbackCandidatePages(fullDoc map[string]any, pageCount int) ([]int, string) {
	doc, _ := fullDoc["document"].(map[string]any)
	if doc == nil {
		if pages := localPerPageNativeQualityLowPages(fullDoc, pageCount); len(pages) > 0 {
			return pages, "per_page_native_quality_low"
		}
		if pages := localVisualTextSparsePages(fullDoc, pageCount); len(pages) > 0 {
			return pages, "visual_text_sparse"
		}
		if localNativeQualityLow(fullDoc, pageCount) {
			return localDocumentPages(pageCount, fullDoc), "native_quality_low"
		}
		return nil, ""
	}
	if pages := localPageRouteCandidatePages(doc); len(pages) > 0 {
		return pages, "page_route_candidates"
	}
	if route, _ := doc["route_decision"].(map[string]any); route != nil {
		mode, _ := route["recommended_mode"].(string)
		if mode != "" && mode != "native_only" && localRouteDecisionNeedsVLM(route) {
			return localDocumentPages(pageCount, fullDoc), "route_decision:" + mode
		}
	}
	if v, ok := doc["suggest_vlm"].(bool); ok && v && localNativeQualityLow(fullDoc, pageCount) {
		return localDocumentPages(pageCount, fullDoc), "suggest_vlm"
	}
	if hint, _ := doc["image_like_pdf"].(map[string]any); hint != nil {
		if v, ok := hint["likely"].(bool); ok && v {
			return localDocumentPages(pageCount, fullDoc), "image_like_pdf"
		}
	}
	if hint, _ := doc["business_doc_vlm_hint"].(map[string]any); hint != nil {
		if v, ok := hint["suggest"].(bool); ok && v {
			if pages := localPerPageNativeQualityLowPages(fullDoc, pageCount); len(pages) > 0 {
				return pages, "per_page_native_quality_low"
			}
			if pages := localVisualTextSparsePages(fullDoc, pageCount); len(pages) > 0 {
				return pages, "visual_text_sparse"
			}
			return nil, ""
		}
	}
	if pages := localPerPageNativeQualityLowPages(fullDoc, pageCount); len(pages) > 0 {
		return pages, "per_page_native_quality_low"
	}
	if pages := localVisualTextSparsePages(fullDoc, pageCount); len(pages) > 0 {
		return pages, "visual_text_sparse"
	}
	if localNativeQualityLow(fullDoc, pageCount) {
		return localDocumentPages(pageCount, fullDoc), "native_quality_low"
	}
	return nil, ""
}

func localPageRouteCandidatePages(doc map[string]any) []int {
	candidates := normalizeMapSlice(doc["page_route_candidates"])
	if len(candidates) == 0 {
		return nil
	}
	out := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		mode := strings.TrimSpace(fmt.Sprint(candidate["recommended_mode"]))
		if mode == "" || mode == "native_only" {
			continue
		}
		if !localRouteCandidateNeedsVLM(candidate) {
			continue
		}
		page := intAny(candidate["page_index"])
		if page > 0 {
			out = append(out, page)
		}
	}
	return normalizeLocalPages(out)
}

func localRouteDecisionNeedsVLM(route map[string]any) bool {
	reasons := normalizeMapSlice(route["reasons"])
	if len(reasons) == 0 {
		return true
	}
	for _, reason := range reasons {
		if localRouteReasonNeedsVLM(reason) {
			return true
		}
	}
	return false
}

func localRouteCandidateNeedsVLM(candidate map[string]any) bool {
	reasons := normalizeMapSlice(candidate["reasons"])
	if len(reasons) == 0 {
		return true
	}
	for _, reason := range reasons {
		if localRouteReasonNeedsVLM(reason) {
			return true
		}
	}
	return false
}

func localRouteReasonNeedsVLM(reason map[string]any) bool {
	switch strings.TrimSpace(fmt.Sprint(reason["code"])) {
	case "force_vlm", "scan_like", "native_quality_low":
		return true
	default:
		return false
	}
}

func localNativeQualityLow(fullDoc map[string]any, pageCount int) bool {
	if fullDoc == nil {
		return false
	}
	chW, _ := fullDoc["chunks"].(map[string]any)
	if chW == nil {
		return true
	}
	items := inspectsvc.CoerceChunkItems(chW["items"])
	if len(items) == 0 {
		return true
	}
	textual := 0
	textRunes := 0
	for _, item := range items {
		typ := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["type"])))
		switch typ {
		case "paragraph", "heading", "text", "kv", "list_item", "table", "formula":
			textual++
			textRunes += len([]rune(strings.TrimSpace(fmt.Sprint(item["text"]))))
		}
	}
	pages := pageCount
	if pages <= 0 {
		pages = len(localDocumentPages(pageCount, fullDoc))
	}
	if pages <= 0 {
		pages = 1
	}
	return textual < pages || textRunes < pages*80
}

func localSidebarRecoveryCandidatePages(fullDoc map[string]any, pageCount int, forced bool) ([]int, string) {
	if fullDoc == nil {
		return nil, ""
	}
	chW, _ := fullDoc["chunks"].(map[string]any)
	if chW == nil {
		return nil, ""
	}
	items := inspectsvc.CoerceChunkItems(chW["items"])
	if len(items) == 0 {
		return nil, ""
	}
	pages := localDocumentPages(pageCount, fullDoc)
	if len(pages) == 0 {
		return nil, ""
	}
	out := make([]int, 0, len(pages))
	for _, page := range pages {
		if inspectsvc.PageHasRightSidebarCoverage(items, page) {
			continue
		}
		if forced || localPageLooksLikeMissingRightSidebar(items, page) {
			out = append(out, page)
		}
	}
	if len(out) == 0 {
		return nil, ""
	}
	return normalizeLocalPages(out), "missing_right_sidebar"
}

func localPageLooksLikeMissingRightSidebar(items []map[string]any, page int) bool {
	leftTextual := 0
	leftRunes := 0
	wideTextual := 0
	rightTextual := 0
	for _, item := range items {
		if intAny(item["page_index"]) != page {
			continue
		}
		typ := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["type"])))
		if !localVLMFallbackTextualType(typ) {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(item["text"]))
		if text == "" || text == "<nil>" || strings.EqualFold(text, "null") {
			continue
		}
		box, ok := localChunkBox(item)
		if !ok {
			continue
		}
		width := box.right - box.left
		centerX := (box.left + box.right) / 2
		runes := len([]rune(text))
		if box.right <= 0.72 && box.left <= 0.20 {
			leftTextual++
			leftRunes += runes
		}
		if width >= 0.68 && runes >= 80 {
			wideTextual++
		}
		if centerX >= 0.70 && runes >= 20 {
			rightTextual++
		}
	}
	return leftTextual >= 2 && leftRunes >= localVLMFallbackMinRunesPerPage() && wideTextual == 0 && rightTextual == 0
}

func localPerPageNativeQualityLowPages(fullDoc map[string]any, pageCount int) []int {
	pages := localDocumentPages(pageCount, fullDoc)
	if len(pages) <= 1 {
		return nil
	}
	chW, _ := fullDoc["chunks"].(map[string]any)
	if chW == nil {
		return pages
	}
	items := inspectsvc.CoerceChunkItems(chW["items"])
	if len(items) == 0 {
		return pages
	}

	type pageStats struct {
		textual   int
		textRunes int
	}
	statsByPage := map[int]pageStats{}
	for _, item := range items {
		page := intAny(item["page_index"])
		if page <= 0 {
			continue
		}
		typ := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["type"])))
		if !localVLMFallbackTextualType(typ) {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(item["text"]))
		if text == "" || text == "<nil>" || strings.EqualFold(text, "null") {
			continue
		}
		st := statsByPage[page]
		st.textual++
		st.textRunes += len([]rune(text))
		statsByPage[page] = st
	}

	out := make([]int, 0, len(pages))
	for _, page := range pages {
		st := statsByPage[page]
		if st.textual == 0 || st.textRunes < localVLMFallbackMinRunesPerPage() {
			out = append(out, page)
		}
	}
	return normalizeLocalPages(out)
}

func localVisualTextSparsePages(fullDoc map[string]any, pageCount int) []int {
	pages := localDocumentPages(pageCount, fullDoc)
	if len(pages) == 0 {
		return nil
	}
	chW, _ := fullDoc["chunks"].(map[string]any)
	if chW == nil {
		return nil
	}
	items := inspectsvc.CoerceChunkItems(chW["items"])
	if len(items) == 0 {
		return nil
	}

	type pageStats struct {
		textual   int
		textRunes int
		visual    int
	}
	statsByPage := map[int]pageStats{}
	for _, item := range items {
		page := intAny(item["page_index"])
		if page <= 0 {
			continue
		}
		typ := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["type"])))
		st := statsByPage[page]
		if localVLMFallbackTextualType(typ) {
			text := strings.TrimSpace(fmt.Sprint(item["text"]))
			if text != "" && text != "<nil>" && !strings.EqualFold(text, "null") {
				st.textual++
				st.textRunes += len([]rune(text))
			}
		}
		if localVLMFallbackVisualType(typ) {
			st.visual++
		}
		statsByPage[page] = st
	}

	out := make([]int, 0, len(pages))
	for _, page := range pages {
		st := statsByPage[page]
		if st.visual > 0 && st.textual <= 2 && st.textRunes < 600 {
			out = append(out, page)
		}
	}
	return normalizeLocalPages(out)
}

func localVLMFallbackTextualType(typ string) bool {
	switch typ {
	case "paragraph", "heading", "text", "kv", "list_item", "table", "formula", "caption", "footnote":
		return true
	default:
		return false
	}
}

func localVLMFallbackVisualType(typ string) bool {
	switch typ {
	case "figure", "image", "stamp", "logo", "scan_code":
		return true
	default:
		return false
	}
}

func localVLMFallbackMinRunesPerPage() int {
	raw := envconfig.String("LOCAL_VLM_FALLBACK_MIN_RUNES_PER_PAGE")
	if raw != "" {
		n, err := strconv.Atoi(raw)
		if err == nil && n >= 0 {
			return n
		}
	}
	return 80
}

func localSidebarOCREnabled() bool {
	raw := strings.ToLower(envconfig.String("LOCAL_SIDEBAR_OCR"))
	switch raw {
	case "0", "false", "no", "off", "disabled":
		return false
	default:
		return true
	}
}

func localSidebarOCRStatus(count int, warn string) string {
	if count > 0 {
		return "applied"
	}
	if strings.TrimSpace(warn) != "" {
		return "warning"
	}
	return "no_text_added"
}

func localSidebarRecoveryStatus(count int) string {
	if count > 0 {
		return "applied"
	}
	return "no_sidebar_text_added"
}

func localDocumentPages(pageCount int, fullDoc map[string]any) []int {
	n := pageCount
	if n <= 0 {
		n = localLayoutPageCount(fullDoc)
	}
	if n <= 0 {
		n = localMaxChunkPage(fullDoc)
	}
	if n <= 0 {
		return nil
	}
	out := make([]int, n)
	for i := range out {
		out[i] = i + 1
	}
	return out
}

func localLayoutPageCount(fullDoc map[string]any) int {
	doc, _ := fullDoc["document"].(map[string]any)
	if doc == nil {
		return 0
	}
	layout, _ := doc["layout"].(map[string]any)
	if layout == nil {
		return 0
	}
	if n := intAny(layout["page_count"]); n > 0 {
		return n
	}
	return len(normalizeSlice(layout["pages"]))
}

func localMaxChunkPage(fullDoc map[string]any) int {
	chW, _ := fullDoc["chunks"].(map[string]any)
	if chW == nil {
		return 0
	}
	maxPage := 0
	for _, item := range inspectsvc.CoerceChunkItems(chW["items"]) {
		if page := intAny(item["page_index"]); page > maxPage {
			maxPage = page
		}
	}
	return maxPage
}

func limitLocalVLMPages(pages []int, maxPage int) []int {
	pages = normalizeLocalPages(pages)
	if len(pages) == 0 || maxPage <= 0 {
		return pages
	}
	out := make([]int, 0, len(pages))
	for _, page := range pages {
		if page <= maxPage {
			out = append(out, page)
		}
	}
	return out
}

func normalizeLocalPages(pages []int) []int {
	if len(pages) == 0 {
		return nil
	}
	seen := map[int]bool{}
	out := make([]int, 0, len(pages))
	for _, page := range pages {
		if page < 1 || seen[page] {
			continue
		}
		seen[page] = true
		out = append(out, page)
	}
	sort.Ints(out)
	return out
}
