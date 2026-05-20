package local

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	localocr "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/ocr"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/hyperparse"
	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

const (
	defaultLocalOCRMaxPages       = 8
	defaultLocalOCRTimeoutSeconds = 20
)

func (c *Client) parsePDF(ctx context.Context, filename string, data []byte, opts extractcommon.ParseOptions) (*extractcommon.DocumentResult, error) {
	startedAt := time.Now()
	mode := normalizeMode(opts.Mode)
	emitParseProgress(opts.OnProgress, extractcommon.ParseProgress{
		Stage:   "native_parse",
		Status:  "running",
		Message: "native_parse",
	})
	fullDoc, err := hyperparse.InspectPDFFullDocument(ctx, hyperparse.InspectPDFOptions{
		Filename:   filename,
		Data:       data,
		Mode:       mode,
		NativeOnly: true,
	})
	if err != nil {
		if !isRecoverablePDFStructureError(err) {
			return nil, err
		}
		return c.recoverFromFullDocumentFailure(ctx, filename, data, opts, err, startedAt)
	}
	pages := extractPages(fullDoc)
	pageCount := hyperparse.PDFPageCountRelaxed(data)
	if pageCount == 0 {
		pageCount = len(pages)
	}
	inspect := map[string]any{
		"full_document":      fullDoc,
		"page_count":         pageCount,
		"recognition_source": "native",
		"engine_contract":    localEngineContractForOptions(opts),
	}
	copyLocalDocumentDiagnostics(fullDoc, inspect)
	refineNativeFullDocumentLayout(fullDoc, inspect)
	emitParseProgress(opts.OnProgress, extractcommon.ParseProgress{
		Stage:   "native_parse",
		Status:  "done",
		Message: "native_parse",
	})

	vlmPagesApplied := applyLocalVLMFallback(ctx, filename, data, mode, pageCount, fullDoc, inspect, opts)
	applyLocalSidebarRecovery(ctx, data, mode, pageCount, fullDoc, inspect, opts.ForceLocalSidebarRecovery, opts)
	applyLocalImageCaptions(ctx, data, mode, pageCount, fullDoc, inspect, vlmPagesApplied, opts.OnProgress)
	finalizeLocalVLMChunksForExport(fullDoc, inspect)
	copyLocalDocumentDiagnostics(fullDoc, inspect)

	dpt := hyperparse.BuildDPTExport(fullDoc, filename, pageCount)
	inspect["dpt_export"] = dpt
	inspect["native_pipeline_duration_ms"] = time.Since(startedAt).Milliseconds()
	doc := buildFromInspect(filename, inspect)
	if doc.Source == "" {
		doc.Source = "native"
	}
	refineDocumentBBoxesWithPoppler(ctx, filename, data, doc, inspect)
	normalizeDecorativeLogoFigures(doc)
	dedupeDocumentFigureChunks(doc)

	ocrReason := ocrFallbackReason(doc)
	if !localOCREnabled() || ocrReason == "" {
		finalizeLocalParseDuration(doc, inspect, startedAt)
		return doc, nil
	}
	emitParseProgress(opts.OnProgress, extractcommon.ParseProgress{
		Stage:   "ocr_fallback",
		Status:  "running",
		Message: "ocr_fallback",
	})
	chunks, markdown, ocrEngine, ocrErr := extractOCRChunksFromPDF(ctx, filename, data, opts)
	if ocrErr != nil {
		if doc.Diagnostics == nil {
			doc.Diagnostics = map[string]any{}
		}
		doc.Diagnostics["ocr_fallback"] = map[string]any{"applied": false, "engine": ocrEngine, "reason": ocrErr.Error()}
		emitParseProgress(opts.OnProgress, extractcommon.ParseProgress{
			Stage:   "ocr_fallback",
			Status:  "warning",
			Message: ocrErr.Error(),
		})
		finalizeLocalParseDuration(doc, inspect, startedAt)
		return doc, nil
	}
	if len(chunks) == 0 {
		if doc.Diagnostics == nil {
			doc.Diagnostics = map[string]any{}
		}
		doc.Diagnostics["ocr_fallback"] = map[string]any{"applied": false, "engine": ocrEngine, "reason": "no text chunks"}
		emitParseProgress(opts.OnProgress, extractcommon.ParseProgress{
			Stage:   "ocr_fallback",
			Status:  "skipped",
			Message: "no text chunks",
		})
		finalizeLocalParseDuration(doc, inspect, startedAt)
		return doc, nil
	}
	doc.Chunks = chunks
	doc.Markdown = markdown
	doc.Source = "native+ocr:" + ocrEngine
	if doc.Diagnostics == nil {
		doc.Diagnostics = map[string]any{}
	}
	doc.Diagnostics["ocr_fallback"] = map[string]any{"applied": true, "engine": ocrEngine, "chunks": len(chunks), "concurrency": localOCRConcurrency(), "reason": ocrReason}
	emitParseProgress(opts.OnProgress, extractcommon.ParseProgress{
		Stage:   "ocr_fallback",
		Status:  "done",
		Message: "ocr_fallback",
		Current: len(chunks),
		Total:   len(chunks),
		Detail:  map[string]any{"engine": ocrEngine},
	})
	finalizeLocalParseDuration(doc, inspect, startedAt)
	return doc, nil
}

func (c *Client) recoverFromFullDocumentFailure(ctx context.Context, filename string, data []byte, opts extractcommon.ParseOptions, originalErr error, startedAt time.Time) (*extractcommon.DocumentResult, error) {
	if normalizeMode(opts.Mode) == "strict" {
		relaxedOpts := opts
		relaxedOpts.Mode = "relaxed"
		doc, err := c.parsePDF(ctx, filename, data, relaxedOpts)
		if err == nil && doc != nil {
			if doc.Diagnostics == nil {
				doc.Diagnostics = map[string]any{}
			}
			doc.Diagnostics["full_document_error"] = map[string]any{
				"stage":       "native_full_document",
				"recoverable": true,
				"reason":      originalErr.Error(),
			}
			doc.Diagnostics["strict_recovery"] = map[string]any{
				"strategy": "relaxed_native",
				"reason":   originalErr.Error(),
			}
			finalizeLocalParseDuration(doc, doc.Diagnostics, startedAt)
			return doc, nil
		}
	}

	pageCount := hyperparse.PDFPageCountRelaxed(data)
	diag := map[string]any{
		"full_document_error": map[string]any{
			"stage":       "native_full_document",
			"recoverable": true,
			"reason":      originalErr.Error(),
		},
		"recognition_source": "native_recovered",
		"engine_contract":    localEngineContractForOptions(opts),
	}
	doc := &extractcommon.DocumentResult{
		DocID:       newID(),
		FileName:    filename,
		PageCount:   pageCount,
		Pages:       makeFallbackPages(pageCount),
		Source:      "native:recovered",
		Diagnostics: diag,
	}
	if popplerDoc, popplerErr := recoverWithPopplerTextLayer(ctx, filename, data, pageCount, diag); popplerErr == nil && popplerDoc != nil && len(popplerDoc.Chunks) > 0 {
		finalizeLocalParseDuration(popplerDoc, diag, startedAt)
		return popplerDoc, nil
	} else if popplerErr != nil {
		doc.Diagnostics["poppler_text_recovery"] = map[string]any{"applied": false, "reason": popplerErr.Error()}
	}
	if !localOCREnabled() {
		doc.Diagnostics["ocr_fallback"] = map[string]any{"applied": false, "engine": localOCRConfigForFileWithOptions(filename, opts).EngineName(), "reason": "disabled"}
		finalizeLocalParseDuration(doc, diag, startedAt)
		return doc, nil
	}
	chunks, markdown, ocrEngine, ocrErr := extractOCRChunksFromPDF(ctx, filename, data, opts)
	if ocrErr != nil {
		doc.Diagnostics["ocr_fallback"] = map[string]any{"applied": false, "engine": ocrEngine, "reason": ocrErr.Error()}
		finalizeLocalParseDuration(doc, diag, startedAt)
		return doc, nil
	}
	if len(chunks) == 0 {
		doc.Diagnostics["ocr_fallback"] = map[string]any{"applied": false, "engine": ocrEngine, "reason": "no text chunks"}
		finalizeLocalParseDuration(doc, diag, startedAt)
		return doc, nil
	}
	doc.Chunks = chunks
	doc.Markdown = markdown
	doc.Source = "native+ocr:" + ocrEngine
	if doc.PageCount == 0 {
		doc.PageCount = pageCountFromChunks(chunks)
		doc.Pages = makeFallbackPages(doc.PageCount)
	}
	doc.Diagnostics["ocr_fallback"] = map[string]any{"applied": true, "engine": ocrEngine, "chunks": len(chunks), "concurrency": localOCRConcurrency(), "reason": "full_document_failed"}
	finalizeLocalParseDuration(doc, diag, startedAt)
	return doc, nil
}

func isRecoverablePDFStructureError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "missing page tree object") ||
		strings.Contains(msg, "unsupported xref section") ||
		strings.Contains(msg, "xref section") ||
		strings.Contains(msg, "xref subsection") ||
		strings.Contains(msg, "xref table") ||
		strings.Contains(msg, "xref stream") ||
		strings.Contains(msg, "page tree") ||
		strings.Contains(msg, "invalid page tree") ||
		strings.Contains(msg, "malformed page tree") ||
		strings.Contains(msg, "trailer /size is required") ||
		(strings.Contains(msg, "trailer") && strings.Contains(msg, "/size"))
}

func makeFallbackPages(pageCount int) []extractcommon.Page {
	if pageCount <= 0 {
		return nil
	}
	pages := make([]extractcommon.Page, pageCount)
	for i := range pages {
		pages[i] = extractcommon.Page{PageIndex: i}
	}
	return pages
}

func pageCountFromChunks(chunks []extractcommon.Chunk) int {
	maxPage := -1
	for _, chunk := range chunks {
		if chunk.Page > maxPage {
			maxPage = chunk.Page
		}
	}
	return maxPage + 1
}

func normalizeMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "strict") {
		return "strict"
	}
	return "relaxed"
}

func buildFromInspect(filename string, inspect map[string]any) *extractcommon.DocumentResult {
	if inspect == nil {
		return &extractcommon.DocumentResult{FileName: filename}
	}
	dpt, _ := inspect["dpt_export"].(map[string]any)
	full, _ := inspect["full_document"].(map[string]any)
	pages := extractPages(full)
	textByOrder := extractFullDocChunkText(full)
	chunks := buildChunks(dpt, textByOrder)
	doc := &extractcommon.DocumentResult{
		DocID:       coalesceString(stringField(inspect, "doc_id"), newID()),
		FileName:    coalesceString(stringField(inspect, "filename"), filename),
		PageCount:   pageCountFromInspect(inspect, pages),
		Pages:       pages,
		Chunks:      chunks,
		Markdown:    stringField(dpt, "markdown"),
		Diagnostics: buildDiagnostics(dpt, inspect),
		Source:      detectSource(full, inspect),
	}
	return doc
}

func finalizeLocalParseDuration(doc *extractcommon.DocumentResult, inspect map[string]any, startedAt time.Time) {
	finalizeLocalParseObservability(doc, inspect, startedAt)
}

func emitParseProgress(cb func(extractcommon.ParseProgress), progress extractcommon.ParseProgress) {
	if cb == nil {
		return
	}
	cb(progress)
}

func buildChunks(dpt map[string]any, textByOrder map[int]map[string]any) []extractcommon.Chunk {
	if dpt == nil {
		return nil
	}
	rawChunks, _ := dpt["chunks"].([]any)
	if len(rawChunks) == 0 {
		return nil
	}
	out := make([]extractcommon.Chunk, 0, len(rawChunks))
	for i, raw := range rawChunks {
		c, _ := raw.(map[string]any)
		if c == nil {
			continue
		}
		grounding, _ := c["grounding"].(map[string]any)
		precision := stringField(grounding, "precision")
		chunk := extractcommon.Chunk{
			ID:        stringField(c, "id"),
			Type:      stringField(c, "type"),
			Subtype:   stringField(c, "subtype"),
			Markdown:  stringField(c, "markdown"),
			Ordinal:   i + 1,
			Precision: precision,
		}
		if payload, ok := c["payload"].(map[string]any); ok && len(payload) > 0 {
			chunk.Payload = cloneMapAny(payload)
		}
		if grounding != nil {
			chunk.Page = intField(grounding, "page")
			if precision != "unreliable" {
				chunk.BBox = bboxFromMap(grounding["box"])
			}
		}
		if fc, ok := textByOrder[i]; ok {
			chunk.Text = stringField(fc, "text")
			if chunk.Confidence == 0 {
				chunk.Confidence = floatField(fc, "confidence")
			}
			if chunk.Payload == nil {
				if payload, ok := fc["payload"].(map[string]any); ok && len(payload) > 0 {
					chunk.Payload = cloneMapAny(payload)
				}
			}
			sourceType := stringField(fc, "type")
			if sourceType == "heading" && chunk.Type == "text" {
				chunk.Type = "heading"
			}
			if chunk.Type == "" {
				chunk.Type = sourceType
			}
			attachChunkProvenancePayload(&chunk, fc)
		}
		if chunk.Text == "" {
			chunk.Text = chunkTextFromMarkdown(chunk.Markdown)
		}
		demoteStatementInfoTable(&chunk)
		if chunk.Type == "text" && isLayoutSectionHeading(chunk.Text) {
			chunk.Type = "heading"
		}
		if chunk.ID == "" {
			chunk.ID = fmt.Sprintf("local-%d", i)
		}
		out = append(out, chunk)
	}
	return out
}

func attachChunkProvenancePayload(chunk *extractcommon.Chunk, item map[string]any) {
	if chunk == nil || item == nil {
		return
	}
	source := stringField(item, "source")
	sourceTrace := stringField(item, "source_trace")
	vlmMerge := stringField(item, "vlm_merge")
	method := inferChunkExtractionMethod(chunk, item, source, vlmMerge)
	bboxSource := stringField(item, "bbox_source")
	bboxPrecise, hasBBoxPrecise := item["bbox_precise"].(bool)
	bboxConfidence := floatField(item, "bbox_confidence")
	if source == "" && sourceTrace == "" && vlmMerge == "" && method == "" && bboxSource == "" && !hasBBoxPrecise && bboxConfidence == 0 {
		return
	}
	if chunk.Payload == nil {
		chunk.Payload = map[string]any{}
	}
	if source != "" {
		chunk.Payload["extraction_source_raw"] = source
	}
	if sourceTrace != "" {
		chunk.Payload["source_trace"] = sourceTrace
	}
	if vlmMerge != "" {
		chunk.Payload["vlm_merge"] = vlmMerge
	}
	if method != "" {
		chunk.Payload["extraction_method"] = method
	}
	if bboxSource != "" {
		chunk.Payload["bbox_source"] = bboxSource
	}
	if hasBBoxPrecise {
		chunk.Payload["bbox_precise"] = bboxPrecise
	}
	if bboxConfidence > 0 {
		chunk.Payload["bbox_confidence"] = bboxConfidence
	}
}

func inferChunkExtractionMethod(chunk *extractcommon.Chunk, item map[string]any, source, vlmMerge string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	vlmMerge = strings.ToLower(strings.TrimSpace(vlmMerge))
	if source == "vlm" || strings.HasPrefix(vlmMerge, "from_vlm") {
		return "vlm"
	}
	if source == "ocr" {
		return "ocr"
	}
	if chunk != nil {
		if _, ok := chunk.Payload["vlm_caption"]; ok {
			return "vlm_caption"
		}
		if _, ok := chunk.Payload["sidebar_recovery_engine"]; ok {
			return "ocr"
		}
		if payloadString(chunk.Payload, "detection_mode") != "" ||
			payloadString(chunk.Payload, "local_table_demoted") != "" {
			return "rule"
		}
	}
	if strings.Contains(strings.ToLower(stringField(item, "source_trace")), "native") {
		return "rule"
	}
	return ""
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	switch v := payload[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return ""
	}
}

var (
	htmlTableRowRE  = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	htmlTableCellRE = regexp.MustCompile(`(?is)<t[dh][^>]*>(.*?)</t[dh]>`)
	htmlTagRE       = regexp.MustCompile(`(?is)<[^>]+>`)
)

func demoteStatementInfoTable(chunk *extractcommon.Chunk) {
	if chunk == nil || strings.ToLower(strings.TrimSpace(chunk.Type)) != "table" {
		return
	}
	plain := plainTextFromHTMLTable(chunk.Markdown)
	if strings.TrimSpace(plain) == "" {
		plain = strings.TrimSpace(chunk.Text)
	}
	if !looksLikeStatementInfoBlock(plain) {
		return
	}
	chunk.Type = "text"
	chunk.Subtype = "statement_info"
	chunk.Text = plain
	chunk.Markdown = plain
	if chunk.Payload == nil {
		chunk.Payload = map[string]any{}
	}
	chunk.Payload["local_table_demoted"] = "statement_info"
}

func looksLikeStatementInfoBlock(text string) bool {
	compact := strings.ToUpper(strings.Join(strings.Fields(text), " "))
	if compact == "" {
		return false
	}
	hasAccountIdentifier := strings.Contains(compact, "IBAN") || strings.Contains(compact, " BIC ")
	if !hasAccountIdentifier {
		return false
	}
	if strings.Contains(compact, "PRODUCT") && strings.Contains(compact, "OPENING BALANCE") {
		return false
	}
	if strings.Contains(compact, "DATE") && strings.Contains(compact, "DESCRIPTION") && strings.Contains(compact, "BALANCE") {
		return false
	}
	return len([]rune(compact)) <= 260
}

func plainTextFromHTMLTable(md string) string {
	md = strings.TrimSpace(md)
	if md == "" || !strings.Contains(strings.ToLower(md), "<table") {
		return ""
	}
	rows := htmlTableRowRE.FindAllStringSubmatch(md, -1)
	if len(rows) == 0 {
		return cleanHTMLText(md)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		cells := htmlTableCellRE.FindAllStringSubmatch(row[1], -1)
		if len(cells) == 0 {
			if txt := cleanHTMLText(row[1]); txt != "" {
				out = append(out, txt)
			}
			continue
		}
		parts := make([]string, 0, len(cells))
		for _, cell := range cells {
			if txt := cleanHTMLText(cell[1]); txt != "" {
				parts = append(parts, txt)
			}
		}
		if len(parts) > 0 {
			out = append(out, strings.Join(parts, " "))
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func cleanHTMLText(s string) string {
	s = htmlTagRE.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

func cloneMapAny(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func extractFullDocChunkText(full map[string]any) map[int]map[string]any {
	out := make(map[int]map[string]any)
	if full == nil {
		return out
	}
	chunks, _ := full["chunks"].(map[string]any)
	if chunks == nil {
		return out
	}
	itemsRaw, _ := chunks["items"].([]any)
	for i, raw := range itemsRaw {
		if m, ok := raw.(map[string]any); ok {
			out[i] = m
		}
	}
	return out
}

func extractPages(full map[string]any) []extractcommon.Page {
	if full == nil {
		return nil
	}
	doc, _ := full["document"].(map[string]any)
	if doc == nil {
		return nil
	}
	layout, _ := doc["layout"].(map[string]any)
	if layout == nil {
		return nil
	}
	rawPages := normalizeSlice(layout["pages"])
	out := make([]extractcommon.Page, 0, len(rawPages))
	for i, raw := range rawPages {
		p, _ := raw.(map[string]any)
		if p == nil {
			continue
		}
		idx := intField(p, "page_index") - 1
		if idx < 0 {
			idx = i
		}
		w, h := dimsFromBox(stringField(p, "media_box"))
		if w == 0 || h == 0 {
			w, h = dimsFromBox(stringField(p, "crop_box"))
		}
		out = append(out, extractcommon.Page{PageIndex: idx, Width: w, Height: h})
	}
	return out
}

func normalizeSlice(v any) []any {
	if v == nil {
		return nil
	}
	if s, ok := v.([]any); ok {
		return s
	}
	if s, ok := v.([]map[string]any); ok {
		out := make([]any, len(s))
		for i, m := range s {
			out[i] = m
		}
		return out
	}
	buf, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out []any
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil
	}
	return out
}

func dimsFromBox(box string) (float64, float64) {
	box = strings.TrimSpace(box)
	if box == "" {
		return 0, 0
	}
	parts := strings.Fields(box)
	if len(parts) != 4 {
		return 0, 0
	}
	x0, err1 := strconv.ParseFloat(parts[0], 64)
	y0, err2 := strconv.ParseFloat(parts[1], 64)
	x1, err3 := strconv.ParseFloat(parts[2], 64)
	y1, err4 := strconv.ParseFloat(parts[3], 64)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return 0, 0
	}
	w := x1 - x0
	h := y1 - y0
	if w < 0 {
		w = -w
	}
	if h < 0 {
		h = -h
	}
	return w, h
}

func pageCountFromInspect(inspect map[string]any, pages []extractcommon.Page) int {
	if v := intField(inspect, "page_count"); v > 0 {
		return v
	}
	return len(pages)
}

func bboxFromMap(v any) *extractcommon.BBox {
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return nil
	}
	return &extractcommon.BBox{Left: floatField(m, "left"), Top: floatField(m, "top"), Right: floatField(m, "right"), Bottom: floatField(m, "bottom")}
}

func chunkTextFromMarkdown(md string) string {
	md = strings.TrimSpace(md)
	if md == "" {
		return ""
	}
	if idx := strings.Index(md, "</a>"); idx >= 0 {
		md = md[idx+len("</a>"):]
	}
	return strings.TrimSpace(md)
}

func detectSource(full map[string]any, inspect map[string]any) string {
	if v := stringField(inspect, "recognition_source"); v != "" {
		return v
	}
	if full != nil {
		if doc, ok := full["document"].(map[string]any); ok {
			if v := stringField(doc, "source"); v != "" {
				return v
			}
		}
	}
	return ""
}

func buildDiagnostics(dpt map[string]any, inspect map[string]any) map[string]any {
	out := map[string]any{}
	if dpt != nil {
		dd, _ := dpt["_hyperparse"].(map[string]any)
		if dd == nil {
			// Backward compatibility for cached or older SDK results.
			dd, _ = dpt["_deepdistill"].(map[string]any)
		}
		if dd != nil {
			if v, ok := dd["bbox_reliability"]; ok {
				out["bbox_reliability"] = v
			}
			if v, ok := dd["bbox_reliable_ratio"]; ok {
				out["bbox_reliable_ratio"] = v
			}
			if v, ok := dd["bbox_downgraded_ids"]; ok {
				out["bbox_downgraded_ids"] = v
			}
		}
	}
	if inspect != nil {
		for _, key := range []string{
			"duration_ms",
			"native_pipeline_duration_ms",
			"recognition_source",
			"suggest_vlm",
			"image_like_pdf",
			"local_layout_refinement",
			"local_poppler_bbox",
			"native_image_append_count",
			"native_image_append_warning",
			"native_image_preview_count",
			"native_image_preview_warning",
			"local_vlm_fallback",
			"local_vlm_ocr_bbox_anchors",
			"local_vlm_image_dedupe_count",
			"merge_report",
			"local_logo_figure_demote",
			"business_doc_vlm_hint",
			"route_decision",
			"vlm_model",
			"vlm_render_engine",
			"vlm_rendered_pages",
			"vlm_image_caption_status",
			"vlm_image_caption_count",
			"vlm_image_caption_model",
			"vlm_image_caption_warning",
			"vlm_image_caption_duration_ms",
			"local_sidebar_ocr_status",
			"local_sidebar_ocr_engine",
			"local_sidebar_ocr_count",
			"local_sidebar_ocr_warning",
			"local_sidebar_ocr_duration_ms",
			"vlm_sidebar_recovery_status",
			"vlm_sidebar_recovery_reason",
			"vlm_sidebar_recovery_pages",
			"vlm_sidebar_recovery_max_pages",
			"vlm_sidebar_recovery_count",
			"vlm_sidebar_recovery_model",
			"vlm_sidebar_recovery_warning",
			"vlm_sidebar_recovery_render_engine",
			"vlm_sidebar_recovery_render_ms",
			"vlm_sidebar_recovery_duration_ms",
			"vlm_pipeline_error",
			"engine_contract",
		} {
			if v, ok := inspect[key]; ok {
				out[key] = v
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func intField(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	}
	return 0
}

func floatField(m map[string]any, key string) float64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return 0
}

func coalesceString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", buf)
}

func localOCREnabled() bool {
	raw := strings.TrimSpace(os.Getenv("LOCAL_OCR_FALLBACK"))
	if raw == "" {
		return true
	}
	switch strings.ToLower(raw) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func needsOCRFallback(doc *extractcommon.DocumentResult) bool {
	return ocrFallbackReason(doc) != ""
}

func ocrFallbackReason(doc *extractcommon.DocumentResult) string {
	if doc == nil || len(doc.Chunks) == 0 {
		return "empty_document"
	}
	meaningful := 0
	for _, c := range doc.Chunks {
		if isNonBodyOCRCoverageChunkType(c.Type) {
			continue
		}
		txt := strings.TrimSpace(c.Text)
		md := strings.TrimSpace(c.Markdown)
		if txt != "" && txt != "<nil>" {
			meaningful++
			continue
		}
		if md != "" && md != "<nil>" && !strings.Contains(md, "<a id='") {
			meaningful++
		}
	}
	if meaningful > 0 {
		if suspectLocalGarbledText(doc.Chunks) {
			return "suspect_garbled_text"
		}
		return ""
	}
	for _, c := range doc.Chunks {
		if c.Type != "figure" {
			return "no_meaningful_text"
		}
	}
	return "only_non_body_coverage"
}

func isNonBodyOCRCoverageChunkType(typ string) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "figure", "image", "stamp", "logo", "scan_code", "marginalia":
		return true
	default:
		return false
	}
}

func suspectLocalGarbledText(chunks []extractcommon.Chunk) bool {
	longASCIIChunks := 0
	suspiciousChunks := 0
	for _, chunk := range chunks {
		letters, vowels, separators, nonASCIILetters := localASCIITextStats(chunk.Text)
		totalLetters := letters + nonASCIILetters
		if totalLetters < 40 {
			continue
		}
		if nonASCIILetters >= 8 && float64(nonASCIILetters)/float64(totalLetters) >= 0.08 {
			continue
		}
		longASCIIChunks++
		textLen := len([]rune(chunk.Text))
		separatorRatio := 0.0
		if textLen > 0 {
			separatorRatio = float64(separators) / float64(textLen)
		}
		vowelRatio := float64(vowels) / float64(letters)
		if vowelRatio < 0.24 || separatorRatio < 0.06 {
			suspiciousChunks++
		}
	}
	return longASCIIChunks >= 5 && float64(suspiciousChunks)/float64(longASCIIChunks) >= 0.35
}

func localASCIITextStats(text string) (letters int, vowels int, separators int, nonASCIILetters int) {
	for _, r := range text {
		switch {
		case r >= 'A' && r <= 'Z':
			letters++
			if strings.ContainsRune("AEIOU", r) {
				vowels++
			}
		case r >= 'a' && r <= 'z':
			letters++
			if strings.ContainsRune("aeiou", r) {
				vowels++
			}
		case r == ' ' || r == '\n' || r == '\t':
			separators++
		case r > 127 && unicode.IsLetter(r):
			nonASCIILetters++
		}
	}
	return letters, vowels, separators, nonASCIILetters
}

func extractOCRChunksFromPDF(ctx context.Context, filename string, pdfData []byte, opts extractcommon.ParseOptions) ([]extractcommon.Chunk, string, string, error) {
	ocrConfig := localOCRConfigForFileWithOptions(filename, opts)
	ocrEngine := ocrConfig.EngineName()
	maxPages := localOCRMaxPages()
	if pc := hyperparse.PDFPageCountRelaxed(pdfData); pc > 0 && (maxPages <= 0 || maxPages > pc) {
		maxPages = pc
	}
	concurrency := localOCRConcurrency()
	pageImages, _, err := hyperparse.RenderPDFPagesToDataURLsWithConcurrency(pdfData, maxPages, concurrency)
	if err != nil {
		return nil, "", ocrEngine, fmt.Errorf("local ocr render pages: %w", err)
	}
	if len(pageImages) == 0 {
		return nil, "", ocrEngine, fmt.Errorf("local ocr render pages: no page image")
	}

	pageOCR := ocrPDFPageImages(ctx, pageImages, ocrConfig, concurrency)
	if pageOCR.Engine != "" {
		ocrEngine = pageOCR.Engine
	}
	chunks := make([]extractcommon.Chunk, 0, len(pageImages))
	mdParts := make([]string, 0, len(pageImages)*4)
	for i := range pageImages {
		blocks := pageOCR.BlocksByPage[i]
		if len(blocks) == 0 {
			continue
		}
		for _, block := range blocks {
			text := normalizeLocalOCRText(block.Text)
			ord := len(chunks) + 1
			precision := "unreliable"
			if block.BBox != nil {
				precision = "reliable"
			}
			chunks = append(chunks, extractcommon.Chunk{
				ID:        fmt.Sprintf("local-ocr-%d", ord-1),
				Type:      "text",
				Page:      i,
				BBox:      block.BBox,
				Text:      text,
				Markdown:  text,
				Ordinal:   ord,
				Precision: precision,
				Payload: map[string]any{
					"extraction_method": "ocr",
					"ocr_engine":        ocrEngine,
					"ocr_bbox_source":   block.BBoxSource,
				},
			})
			mdParts = append(mdParts, text)
		}
	}
	return chunks, strings.Join(mdParts, "\n\n"), ocrEngine, nil
}

type ocrPDFPageImageResult struct {
	BlocksByPage map[int][]ocrTextBlock
	Engine       string
}

type ocrTextBlock struct {
	Text       string
	BBox       *extractcommon.BBox
	BBoxSource string
}

func ocrPDFPageImages(ctx context.Context, pageImages []string, ocrConfig localocr.Config, concurrency int) ocrPDFPageImageResult {
	out := ocrPDFPageImageResult{
		BlocksByPage: make(map[int][]ocrTextBlock),
		Engine:       ocrConfig.EngineName(),
	}
	if len(pageImages) == 0 {
		return out
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(pageImages) {
		concurrency = len(pageImages)
	}
	type pageResult struct {
		page   int
		blocks []ocrTextBlock
		engine string
	}
	jobs := make(chan int)
	results := make(chan pageResult, len(pageImages))
	var wg sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for page := range jobs {
				imgBytes, err := decodeDataURLImage(pageImages[page])
				if err != nil {
					continue
				}
				width, height := imageDimensions(imgBytes)
				ocrResult, err := runOCRLinesWithFallback(ctx, ocrConfig, imgBytes, width, height)
				if err != nil {
					continue
				}
				blocks := ocrLinesToTextBlocks(ocrResult.Lines, width, height)
				if len(blocks) == 0 {
					text := ocrResult.Text
					if strings.TrimSpace(text) == "" {
						text = ocrResult.Raw
					}
					blocks = ocrTextBlocksFromText(text)
				}
				if len(blocks) > 0 {
					results <- pageResult{page: page, blocks: blocks, engine: ocrResult.Engine}
				}
			}
		}()
	}
	go func() {
		for i := range pageImages {
			select {
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				close(results)
				return
			case jobs <- i:
			}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	for result := range results {
		if result.engine != "" {
			out.Engine = result.engine
		}
		out.BlocksByPage[result.page] = result.blocks
	}
	return out
}

func imageDimensions(imageBytes []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(imageBytes))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func ocrLinesToTextBlocks(lines []localocr.Line, width, height int) []ocrTextBlock {
	if len(lines) == 0 {
		return nil
	}
	out := make([]ocrTextBlock, 0, len(lines))
	for _, line := range lines {
		text := normalizeLocalOCRText(strings.Join(strings.Fields(strings.TrimSpace(line.Text)), " "))
		if text == "" {
			continue
		}
		block := ocrTextBlock{
			Text:       text,
			BBox:       normalizedOCRLineBBox(line, width, height),
			BBoxSource: "ocr_line",
		}
		if len([]rune(text)) < 3 && len(out) > 0 {
			prev := &out[len(out)-1]
			prev.Text = strings.TrimSpace(prev.Text + " " + text)
			prev.BBox = unionExtractBBoxes(prev.BBox, block.BBox)
			continue
		}
		out = append(out, block)
	}
	return out
}

func ocrTextBlocksFromText(text string) []ocrTextBlock {
	blocks := splitOCRTextBlocks(text)
	if len(blocks) == 0 {
		return nil
	}
	out := make([]ocrTextBlock, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, ocrTextBlock{Text: normalizeLocalOCRText(block), BBoxSource: "ocr_text"})
	}
	return out
}

func normalizeLocalOCRText(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" {
		return ""
	}
	runes := []rune(text)
	var out []rune
	for i, r := range runes {
		if r == ' ' && isCJKRune(prevNonSpaceRune(runes[:i])) && isCJKRune(nextNonSpaceRune(runes[i+1:])) {
			continue
		}
		out = append(out, r)
	}
	return strings.TrimSpace(string(out))
}

func prevNonSpaceRune(in []rune) rune {
	for i := len(in) - 1; i >= 0; i-- {
		if in[i] != ' ' {
			return in[i]
		}
	}
	return 0
}

func nextNonSpaceRune(in []rune) rune {
	for _, r := range in {
		if r != ' ' {
			return r
		}
	}
	return 0
}

func isCJKRune(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r)
}

func normalizedOCRLineBBox(line localocr.Line, width, height int) *extractcommon.BBox {
	if width <= 0 || height <= 0 || line.Right <= line.Left || line.Bottom <= line.Top {
		return nil
	}
	left := clamp01(float64(line.Left) / float64(width))
	top := clamp01(float64(line.Top) / float64(height))
	right := clamp01(float64(line.Right) / float64(width))
	bottom := clamp01(float64(line.Bottom) / float64(height))
	if right <= left || bottom <= top {
		return nil
	}
	return &extractcommon.BBox{Left: left, Top: top, Right: right, Bottom: bottom}
}

func unionExtractBBoxes(a, b *extractcommon.BBox) *extractcommon.BBox {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return &extractcommon.BBox{
		Left:   minFloat64(a.Left, b.Left),
		Top:    minFloat64(a.Top, b.Top),
		Right:  maxFloat64(a.Right, b.Right),
		Bottom: maxFloat64(a.Bottom, b.Bottom),
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func minFloat64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func splitOCRTextBlocks(raw string) []string {
	text := strings.ReplaceAll(raw, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	// Step 1: split paragraphs by blank lines.
	lines := strings.Split(text, "\n")
	paras := make([]string, 0, 16)
	cur := make([]string, 0, 8)
	flush := func() {
		if len(cur) == 0 {
			return
		}
		for i := range cur {
			cur[i] = strings.TrimSpace(cur[i])
		}
		p := strings.TrimSpace(strings.Join(cur, " "))
		p = strings.Join(strings.Fields(p), " ")
		if p != "" {
			paras = append(paras, p)
		}
		cur = cur[:0]
	}
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			flush()
			continue
		}
		cur = append(cur, ln)
	}
	flush()

	// Step 2: if OCR text has no paragraph breaks, fallback to line-level chunks.
	if len(paras) <= 1 {
		lineBlocks := make([]string, 0, len(lines))
		for _, ln := range lines {
			ln = strings.Join(strings.Fields(strings.TrimSpace(ln)), " ")
			if ln != "" {
				lineBlocks = append(lineBlocks, ln)
			}
		}
		if len(lineBlocks) > 1 {
			return mergeTinyBlocks(lineBlocks)
		}
	}
	return mergeTinyBlocks(paras)
}

func mergeTinyBlocks(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, b := range in {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		// Merge ultra-short scraps (often OCR noise) into previous block.
		if len([]rune(b)) < 3 && len(out) > 0 {
			out[len(out)-1] = strings.TrimSpace(out[len(out)-1] + " " + b)
			continue
		}
		out = append(out, b)
	}
	return out
}

func decodeDataURLImage(dataURL string) ([]byte, error) {
	idx := strings.Index(dataURL, ",")
	if idx < 0 {
		return nil, fmt.Errorf("invalid data url")
	}
	raw := dataURL[idx+1:]
	return base64.StdEncoding.DecodeString(raw)
}

func candidateOCRLangs(preferred string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	add(preferred)
	add("chi_sim+eng")
	add("eng")
	add("chi_sim")
	return out
}

func runOCRTextWithFallback(ctx context.Context, ocrConfig localocr.Config, imageBytes []byte) (localocr.Result, error) {
	if ocrConfig.EngineName() != localocr.EngineTesseract {
		return ocrConfig.RunText(ctx, imageBytes)
	}
	langs := candidateOCRLangs(ocrConfig.Lang)
	var lastErr error
	for _, lang := range langs {
		cfg := ocrConfig
		cfg.Lang = lang
		res, err := cfg.RunText(ctx, imageBytes)
		if err == nil {
			if ocrResultHasLanguageLoadError(res) {
				lastErr = fmt.Errorf("lang=%s err=ocr language unavailable", lang)
				continue
			}
			return res, nil
		}
		lastErr = fmt.Errorf("lang=%s err=%w", lang, err)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("ocr command failed")
	}
	return localocr.Result{Engine: ocrConfig.EngineName()}, lastErr
}

func runOCRLinesWithFallback(ctx context.Context, ocrConfig localocr.Config, imageBytes []byte, width, height int) (localocr.Result, error) {
	if len(imageBytes) == 0 {
		return localocr.Result{Engine: ocrConfig.EngineName()}, fmt.Errorf("empty crop")
	}
	tmpDir, err := os.MkdirTemp("", "hyperparse-ocr-lines-*")
	if err != nil {
		return localocr.Result{Engine: ocrConfig.EngineName()}, err
	}
	defer os.RemoveAll(tmpDir)
	imgPath := tmpDir + "/input.png"
	if err := os.WriteFile(imgPath, imageBytes, 0600); err != nil {
		return localocr.Result{Engine: ocrConfig.EngineName()}, err
	}

	run := func(cfg localocr.Config) (localocr.Result, error) {
		res, err := cfg.RunLinesFile(ctx, imgPath, width, height)
		if err == nil {
			if ocrResultHasLanguageLoadError(res) {
				return res, fmt.Errorf("ocr language unavailable")
			}
			if len(res.Lines) > 0 {
				return res, nil
			}
		}
		textRes, textErr := cfg.RunTextFile(ctx, imgPath)
		if textErr == nil {
			if ocrResultHasLanguageLoadError(textRes) {
				return textRes, fmt.Errorf("ocr language unavailable")
			}
			if textRes.Engine == "" {
				textRes.Engine = res.Engine
			}
			return textRes, nil
		}
		if err != nil {
			return res, err
		}
		return textRes, textErr
	}

	if ocrConfig.EngineName() != localocr.EngineTesseract {
		return run(ocrConfig)
	}

	langs := candidateOCRLangs(ocrConfig.Lang)
	var lastErr error
	for _, lang := range langs {
		cfg := ocrConfig
		cfg.Lang = lang
		res, err := run(cfg)
		if err == nil {
			return res, nil
		}
		lastErr = fmt.Errorf("lang=%s err=%w", lang, err)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("ocr command failed")
	}
	return localocr.Result{Engine: ocrConfig.EngineName()}, lastErr
}

func ocrResultHasLanguageLoadError(res localocr.Result) bool {
	raw := strings.ToLower(res.Raw + "\n" + res.Text)
	return strings.Contains(raw, "error opening data file") ||
		strings.Contains(raw, "failed loading language") ||
		strings.Contains(raw, "couldn't load any languages") ||
		strings.Contains(raw, "could not initialize tesseract")
}

func localOCRConfigForFile(filename string) localocr.Config {
	return localOCRConfigForFileWithOptions(filename, extractcommon.ParseOptions{})
}

func localOCRConfigForFileWithOptions(filename string, opts extractcommon.ParseOptions) localocr.Config {
	cfg := localocr.LoadConfig(localOCRTimeout())
	if engine := normalizeRequestedOCREngine(opts.OCREngine); engine != "" {
		cfg.Engine = engine
	}
	if lang := explicitLocalOCRLang(); lang != "" {
		cfg.Lang = lang
	} else {
		cfg.Lang = localOCRLangForEngine(filename, cfg.EngineName())
	}
	if cfg.EngineName() == localocr.EngineTesseract && explicitTesseractPSM() == 0 && localOCRFileHasNonASCII(filename) {
		cfg.TesseractPSM = 11
	}
	return cfg
}

func normalizeRequestedOCREngine(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "default", "auto":
		return ""
	case "tesseract", "tess":
		return localocr.EngineTesseract
	case "paddle", "paddleocr", "paddle_ocr", "ppocr":
		return localocr.EnginePaddleOCR
	default:
		return ""
	}
}

func explicitLocalOCRLang() string {
	for _, key := range []string{"CONTENT_PARSE_OCR_LANG", "CONTENT_PARSE_LOCAL_OCR_LANG", "DOCSTILL_OCR_LANG", "LOCAL_OCR_LANG", "DOCSTILL_LOCAL_OCR_LANG"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func explicitTesseractPSM() int {
	for _, key := range []string{"CONTENT_PARSE_TESSERACT_PSM", "CONTENT_PARSE_OCR_TESSERACT_PSM", "DOCSTILL_TESSERACT_PSM", "DOCSTILL_OCR_TESSERACT_PSM"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

func localOCRLangForEngine(filename string, engine string) string {
	if engine == localocr.EnginePaddleOCR {
		if localOCRFileHasNonASCII(filename) {
			return "ch"
		}
		return "en"
	}
	if localOCRFileHasNonASCII(filename) {
		return "chi_sim"
	}
	return "eng"
}

func localOCRFileHasNonASCII(filename string) bool {
	for _, r := range filename {
		if r > 127 {
			return true
		}
	}
	return false
}

func localOCRTimeout() time.Duration {
	if v := strings.TrimSpace(firstNonEmptyEnv("CONTENT_PARSE_OCR_TIMEOUT_SECONDS", "DOCSTILL_OCR_TIMEOUT_SECONDS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	if v := strings.TrimSpace(os.Getenv("LOCAL_OCR_TIMEOUT_SECONDS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultLocalOCRTimeoutSeconds * time.Second
}

func localOCRMaxPages() int {
	if v := strings.TrimSpace(firstNonEmptyEnv("CONTENT_PARSE_LOCAL_OCR_MAX_PAGES", "DOCSTILL_LOCAL_OCR_MAX_PAGES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	if v := strings.TrimSpace(os.Getenv("LOCAL_OCR_MAX_PAGES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultLocalOCRMaxPages
}

func localOCRConcurrency() int {
	defaultConcurrency := 2
	if runtime.NumCPU() < defaultConcurrency {
		defaultConcurrency = runtime.NumCPU()
	}
	if defaultConcurrency < 1 {
		defaultConcurrency = 1
	}
	raw := strings.TrimSpace(firstNonEmptyEnv("CONTENT_PARSE_LOCAL_OCR_CONCURRENCY", "DOCSTILL_LOCAL_OCR_CONCURRENCY"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("LOCAL_OCR_CONCURRENCY"))
	}
	if raw == "" {
		return defaultConcurrency
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultConcurrency
	}
	if n > 8 {
		return 8
	}
	return n
}
