package inspectsvc

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/image/draw"

	"github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/core/layoutdoc"
	localocr "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/ocr"
	pdforchestrator "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/orchestrators/pdf"
)

const regionalFallbackInstruction = `You are a document region repair assistant. The input is a cropped region from one PDF page; process only this local region.
Extract the real visible content in the region and output strict JSON with no Markdown fences and no explanations:
{"chunks":[{"type":"paragraph|heading|table|formula|image|list_item|kv|annotation|other","text":"...","order":0,"confidence":0.0}]}

Requirements:
1) Do not summarize the whole page; extract only content inside the crop.
2) For tables, prefer a payload grid. Use a GFM Markdown pipe table when uncertain.
3) Image or chart regions must be type=image and include a short description in text.
4) If recognition is unclear, return empty chunks instead of fabricating content.
5) page_index and bbox may be omitted; the server fills them from the crop region.`

type lowConfidenceRegion struct {
	ID            string
	PageIndex     int
	Reason        string
	Route         string
	BBox          map[string]float64
	TextPreview   string
	Confidence    float64
	AnchorChunkID string
}

type regionalFallbackResult struct {
	Enabled               bool
	Status                string
	Model                 string
	OCREngine             string
	Warning               string
	RenderEngine          string
	RegionCount           int
	ProcessedRegionCount  int
	Count                 int
	AddedCount            int
	MergedCount           int
	AnchorBBoxRepairCount int
	OCRCount              int
	VLMCount              int
	DurationMS            int64
	Regions               []map[string]any
}

type regionalCrop struct {
	DataURL      string
	PNG          []byte
	Width        int
	Height       int
	NonBlankRate float64
}

type regionalJob struct {
	region lowConfidenceRegion
	crop   regionalCrop
}

type regionalJobResult struct {
	region     lowConfidenceRegion
	source     string
	model      string
	chunks     []map[string]any
	err        error
	status     string
	ocrUsed    bool
	vlmUsed    bool
	ocrEngine  string
	durationMS int64
}

// ApplyRegionalOCRVLMFallback sends unstable local layout regions through small-crop OCR/VLM.
// It is intentionally region-scoped: local mode should not escalate the whole document unless
// the page-level quality gate already asked for full VLM fallback.
func ApplyRegionalOCRVLMFallback(ctx context.Context, pdfBytes []byte, fullDoc map[string]any) regionalFallbackResult {
	started := time.Now()
	res := regionalFallbackResult{Enabled: regionalFallbackEnabled(), Status: "skipped"}
	if !res.Enabled {
		res.Warning = "disabled"
		setRegionalFallbackDocMeta(fullDoc, res)
		return res
	}
	if len(pdfBytes) == 0 {
		res.Warning = "empty pdf"
		setRegionalFallbackDocMeta(fullDoc, res)
		return res
	}
	regions := lowConfidenceRegionsFromFullDoc(fullDoc)
	if len(regions) == 0 {
		setRegionalFallbackDocMeta(fullDoc, res)
		return res
	}
	res.RegionCount = len(regions)
	maxRegions := regionalFallbackMaxRegions()
	if maxRegions > 0 && len(regions) > maxRegions {
		regions = regions[:maxRegions]
	}
	maxPage := 0
	for _, region := range regions {
		if region.PageIndex > maxPage {
			maxPage = region.PageIndex
		}
	}
	pageDataURLs, engine, err := RenderPDFPagesToDataURLs(pdfBytes, maxPage)
	res.RenderEngine = engine
	if err != nil {
		res.Status = "error"
		res.Warning = "render pages: " + err.Error()
		res.DurationMS = time.Since(started).Milliseconds()
		setRegionalFallbackDocMeta(fullDoc, res)
		return res
	}

	jobs := make([]regionalJob, 0, len(regions))
	regionReports := make([]map[string]any, 0, len(regions))
	for _, region := range regions {
		report := map[string]any{
			"id":         region.ID,
			"page_index": region.PageIndex,
			"reason":     region.Reason,
			"bbox":       floatBoxToAny(region.BBox),
		}
		if region.PageIndex <= 0 || region.PageIndex > len(pageDataURLs) {
			report["status"] = "skipped_page_out_of_range"
			regionReports = append(regionReports, report)
			continue
		}
		crop, err := cropRegionDataURL(pageDataURLs[region.PageIndex-1], region.BBox)
		if err != nil {
			report["status"] = "crop_error"
			report["warning"] = err.Error()
			regionReports = append(regionReports, report)
			continue
		}
		report["crop_width"] = crop.Width
		report["crop_height"] = crop.Height
		report["non_blank_ratio"] = roundInspectFloat(crop.NonBlankRate, 4)
		if crop.NonBlankRate < 0.002 {
			report["status"] = "blank_crop"
			regionReports = append(regionReports, report)
			continue
		}
		regionReports = append(regionReports, report)
		jobs = append(jobs, regionalJob{region: region, crop: crop})
	}
	res.Regions = regionReports
	if len(jobs) == 0 {
		res.Status = "skipped"
		res.DurationMS = time.Since(started).Milliseconds()
		setRegionalFallbackDocMeta(fullDoc, res)
		return res
	}

	conc := regionalFallbackConcurrency()
	results := make([]regionalJobResult, len(jobs))
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for i := range jobs {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = regionalJobResult{region: jobs[i].region, err: ctx.Err(), status: "canceled"}
				return
			}
			defer func() { <-sem }()
			results[i] = processRegionalFallbackJob(ctx, jobs[i])
		}()
	}
	wg.Wait()

	var warnings []string
	firstModel := ""
	firstOCREngine := ""
	for i, r := range results {
		updateRegionalReport(regionReports, r)
		if r.err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", r.region.ID, r.err))
			if r.region.AnchorChunkID != "" {
				_, _, anchor := mergeRegionalFallbackChunks(fullDoc, r.region, nil, "regional_repair", r.model, i)
				res.AnchorBBoxRepairCount += anchor
			}
			continue
		}
		if r.model != "" && firstModel == "" {
			firstModel = r.model
		}
		if r.ocrUsed {
			res.OCRCount += len(r.chunks)
			if r.ocrEngine != "" && firstOCREngine == "" {
				firstOCREngine = r.ocrEngine
			}
		}
		if r.vlmUsed {
			res.VLMCount += len(r.chunks)
		}
		added, merged, anchor := mergeRegionalFallbackChunks(fullDoc, r.region, r.chunks, r.source, r.model, i)
		res.AddedCount += added
		res.MergedCount += merged
		res.AnchorBBoxRepairCount += anchor
	}
	res.Count = res.AddedCount + res.MergedCount + res.AnchorBBoxRepairCount
	res.ProcessedRegionCount = len(jobs)
	res.Model = firstModel
	res.OCREngine = firstOCREngine
	res.DurationMS = time.Since(started).Milliseconds()
	if len(warnings) > 0 {
		res.Warning = truncateInspectString(strings.Join(warnings, "; "), 900)
	}
	if res.Count > 0 {
		res.Status = "used"
		if doc, ok := fullDoc["document"].(map[string]any); ok {
			if chW, ok := fullDoc["chunks"].(map[string]any); ok {
				merged := CoerceChunkItems(chW["items"])
				pdforchestrator.RebuildTextSummaryAfterVLMMerge(doc, merged, "hybrid")
			}
		}
	} else if res.Warning != "" {
		res.Status = "error"
	} else {
		res.Status = "skipped"
	}
	setRegionalFallbackDocMeta(fullDoc, res)
	return res
}

func processRegionalFallbackJob(ctx context.Context, job regionalJob) regionalJobResult {
	started := time.Now()
	region := job.region
	if regionalFallbackShouldPreferVLM(region) && regionalFallbackVLMEnabled() {
		model, chunks, err := callDashscopeRegionalFallbackStructured(ctx, job.crop.DataURL, region)
		if err == nil && len(chunks) > 0 {
			return regionalJobResult{region: region, source: "regional_vlm", model: model, chunks: chunks, status: "vlm", vlmUsed: true, durationMS: time.Since(started).Milliseconds()}
		}
		if !regionalFallbackOCREnabled() {
			return regionalJobResult{region: region, source: "regional_vlm", model: model, err: err, status: "vlm_error", vlmUsed: true, durationMS: time.Since(started).Milliseconds()}
		}
	}
	if regionalFallbackOCREnabled() {
		ocrResult, err := localocr.LoadConfig(regionalOCRTimeout()).RunText(ctx, job.crop.PNG)
		if err == nil && regionalOCRTextUseful(ocrResult.Text) {
			ch := regionalOCRChunk(region, ocrResult.Text, ocrResult.Engine)
			return regionalJobResult{region: region, source: "regional_ocr", chunks: []map[string]any{ch}, status: "ocr", ocrUsed: true, ocrEngine: ocrResult.Engine, durationMS: time.Since(started).Milliseconds()}
		}
		if !regionalFallbackVLMEnabled() {
			engine := localocr.LoadConfig(regionalOCRTimeout()).EngineName()
			if ocrResult.Engine != "" {
				engine = ocrResult.Engine
			}
			return regionalJobResult{region: region, source: "regional_ocr", err: err, status: "ocr_error", ocrUsed: true, ocrEngine: engine, durationMS: time.Since(started).Milliseconds()}
		}
	}
	if regionalFallbackVLMEnabled() {
		model, chunks, err := callDashscopeRegionalFallbackStructured(ctx, job.crop.DataURL, region)
		if err != nil {
			return regionalJobResult{region: region, source: "regional_vlm", model: model, err: err, status: "vlm_error", vlmUsed: true, durationMS: time.Since(started).Milliseconds()}
		}
		return regionalJobResult{region: region, source: "regional_vlm", model: model, chunks: chunks, status: "vlm", vlmUsed: true, durationMS: time.Since(started).Milliseconds()}
	}
	return regionalJobResult{region: region, status: "skipped_no_ocr_or_vlm", durationMS: time.Since(started).Milliseconds()}
}

func callDashscopeRegionalFallbackStructured(ctx context.Context, dataURL string, region lowConfidenceRegion) (string, []map[string]any, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
	}
	meta := fmt.Sprintf("Region info: page_index=%d, region_id=%s, reason=%s, bbox=%v, native_preview=%q",
		region.PageIndex, region.ID, region.Reason, floatBoxToAny(region.BBox), truncateInspectString(region.TextPreview, 220))
	parts := []map[string]any{
		{"type": "text", "text": regionalFallbackInstruction + "\n\n" + meta},
		{"type": "image_url", "image_url": map[string]any{"url": dataURL}},
	}
	raw, model, err := dashscopeChatCompletion(parts)
	if err != nil {
		return model, nil, err
	}
	chunks, perr := pdforchestrator.ParseVLMChunksJSON(raw)
	if perr != nil {
		return model, nil, perr
	}
	return model, chunks, nil
}

func mergeRegionalFallbackChunks(fullDoc map[string]any, region lowConfidenceRegion, chunks []map[string]any, source, model string, regionOrdinal int) (added int, merged int, anchorRepair int) {
	chW, ok := fullDoc["chunks"].(map[string]any)
	if !ok {
		return 0, 0, 0
	}
	items := CoerceChunkItems(chW["items"])
	if len(items) == 0 {
		items = []map[string]any{}
	}
	nextOrder := maxChunkOrder(items) + 1
	if source == "" {
		source = "regional_fallback"
	}
	anchorAlreadyMerged := false
	for i, ch := range chunks {
		if ch == nil {
			continue
		}
		normalizeRegionalFallbackChunk(ch, region, source, model, nextOrder+i, regionOrdinal, i)
		if target := findRegionalDuplicateChunk(items, region, ch); target != nil {
			if region.AnchorChunkID != "" && strings.TrimSpace(fmt.Sprint(target["chunk_id"])) == region.AnchorChunkID {
				anchorAlreadyMerged = true
			}
			mergeRegionalChunkIntoTarget(target, ch, region, source, model)
			merged++
			continue
		}
		items = append(items, ch)
		added++
	}
	if region.AnchorChunkID != "" && !anchorAlreadyMerged {
		if repairRegionalAnchorBBox(items, region, source, model) {
			anchorRepair++
		}
	}
	if added > 0 || merged > 0 || anchorRepair > 0 {
		sort.SliceStable(items, func(i, j int) bool {
			pi := IntFromChunkAny(items[i], "page_index")
			pj := IntFromChunkAny(items[j], "page_index")
			if pi != pj {
				return pi < pj
			}
			return IntFromChunkAny(items[i], "order") < IntFromChunkAny(items[j], "order")
		})
		chW["items"] = items
		chW["count"] = len(items)
		chW["regional_ocr_vlm_merge"] = true
	}
	return added, merged, anchorRepair
}

func normalizeRegionalFallbackChunk(ch map[string]any, region lowConfidenceRegion, source, model string, order int, regionOrdinal int, chunkOrdinal int) {
	if strings.TrimSpace(fmt.Sprint(ch["chunk_id"])) == "" {
		ch["chunk_id"] = fmt.Sprintf("regional_%s_%d_%d", safeIDPart(region.ID), regionOrdinal, chunkOrdinal)
	}
	ch["page_index"] = region.PageIndex
	ch["bbox"] = floatBoxToAny(region.BBox)
	ch["order"] = order
	ch["source"] = source
	ch["source_trace"] = fmt.Sprintf("%s:%s:%s", source, region.ID, region.Reason)
	if _, ok := toInspectFloat(ch["confidence"]); !ok {
		ch["confidence"] = 0.74
	}
	payload, _ := ch["payload"].(map[string]any)
	if payload == nil {
		payload = map[string]any{}
		ch["payload"] = payload
	}
	payload["repair"] = "regional_low_confidence_fallback"
	payload["regional_fallback"] = true
	payload["region_id"] = region.ID
	payload["region_reason"] = region.Reason
	payload["region_bbox"] = floatBoxToAny(region.BBox)
	if region.AnchorChunkID != "" {
		payload["anchor_chunk_id"] = region.AnchorChunkID
	}
	if model != "" {
		payload["model"] = model
	}
	layoutdoc.ApplyChunkProvenance(ch, "")
}

func regionalOCRChunk(region lowConfidenceRegion, text string, engine string) map[string]any {
	typ := "paragraph"
	if looksLikeRegionalHeading(text) {
		typ = "heading"
	}
	if engine == "" {
		engine = localocr.LoadConfig(regionalOCRTimeout()).EngineName()
	}
	return map[string]any{
		"type":       typ,
		"text":       strings.TrimSpace(text),
		"page_index": region.PageIndex,
		"bbox":       floatBoxToAny(region.BBox),
		"source":     "regional_ocr",
		"confidence": 0.70,
		"payload": map[string]any{
			"ocr_engine":        engine,
			"regional_fallback": true,
			"region_id":         region.ID,
			"region_reason":     region.Reason,
		},
	}
}

func repairRegionalAnchorBBox(items []map[string]any, region lowConfidenceRegion, source, model string) bool {
	for _, it := range items {
		if strings.TrimSpace(fmt.Sprint(it["chunk_id"])) != region.AnchorChunkID {
			continue
		}
		it["bbox"] = floatBoxToAny(region.BBox)
		it["confidence"] = 0.78
		it["source"] = source
		it["source_trace"] = fmt.Sprintf("%s:%s:%s", source, region.ID, region.Reason)
		payload, _ := it["payload"].(map[string]any)
		if payload == nil {
			payload = map[string]any{}
			it["payload"] = payload
		}
		payload["repair"] = "regional_bbox_alignment"
		payload["regional_fallback"] = true
		payload["region_id"] = region.ID
		payload["region_reason"] = region.Reason
		payload["region_bbox"] = floatBoxToAny(region.BBox)
		if model != "" {
			payload["model"] = model
		}
		layoutdoc.ApplyChunkProvenance(it, "")
		return true
	}
	return false
}

func mergeRegionalChunkIntoTarget(target, src map[string]any, region lowConfidenceRegion, source, model string) {
	srcText := strings.TrimSpace(fmt.Sprint(src["text"]))
	dstText := strings.TrimSpace(fmt.Sprint(target["text"]))
	if srcText != "" && (dstText == "" || utf8.RuneCountInString(srcText) > utf8.RuneCountInString(dstText)+12 || strings.EqualFold(strings.TrimSpace(fmt.Sprint(src["type"])), "table")) {
		target["text"] = srcText
	}
	if typ := strings.TrimSpace(fmt.Sprint(src["type"])); typ != "" && typ != "other" {
		if strings.EqualFold(typ, "table") || strings.TrimSpace(fmt.Sprint(target["type"])) == "" {
			target["type"] = typ
		}
	}
	target["bbox"] = floatBoxToAny(region.BBox)
	target["source"] = source
	target["source_trace"] = fmt.Sprintf("%s:%s:%s", source, region.ID, region.Reason)
	if conf, ok := toInspectFloat(src["confidence"]); ok {
		target["confidence"] = math.Max(0.72, math.Min(0.92, conf))
	}
	payload, _ := target["payload"].(map[string]any)
	if payload == nil {
		payload = map[string]any{}
		target["payload"] = payload
	}
	payload["repair"] = "regional_low_confidence_merge"
	payload["regional_fallback"] = true
	payload["region_id"] = region.ID
	payload["region_reason"] = region.Reason
	payload["region_bbox"] = floatBoxToAny(region.BBox)
	if model != "" {
		payload["model"] = model
	}
	layoutdoc.ApplyChunkProvenance(target, "")
}

func findRegionalDuplicateChunk(items []map[string]any, region lowConfidenceRegion, candidate map[string]any) map[string]any {
	candText := normalizeRegionalText(fmt.Sprint(candidate["text"]))
	for _, it := range items {
		if IntFromChunkAny(it, "page_index") != region.PageIndex {
			continue
		}
		if region.AnchorChunkID != "" && strings.TrimSpace(fmt.Sprint(it["chunk_id"])) == region.AnchorChunkID {
			return it
		}
		if candText != "" {
			existing := normalizeRegionalText(fmt.Sprint(it["text"]))
			if existing != "" && (strings.Contains(existing, candText) || strings.Contains(candText, existing)) {
				return it
			}
		}
		box, ok := inspectBBox(it["bbox"])
		if ok && inspectBoxIoU(box, region.BBox) >= 0.72 {
			return it
		}
	}
	return nil
}

func cropRegionDataURL(pageDataURL string, bbox map[string]float64) (regionalCrop, error) {
	img, err := imageFromDataURL(pageDataURL)
	if err != nil {
		return regionalCrop{}, err
	}
	b := img.Bounds()
	box := padInspectBox(bbox, 0.006)
	x0 := b.Min.X + int(math.Floor(box["left"]*float64(b.Dx())))
	x1 := b.Min.X + int(math.Ceil(box["right"]*float64(b.Dx())))
	y0 := b.Min.Y + int(math.Floor(box["top"]*float64(b.Dy())))
	y1 := b.Min.Y + int(math.Ceil(box["bottom"]*float64(b.Dy())))
	rect := image.Rect(x0, y0, x1, y1).Intersect(b)
	if rect.Empty() || rect.Dx() < 8 || rect.Dy() < 8 {
		return regionalCrop{}, fmt.Errorf("empty or tiny crop")
	}
	dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(dst, dst.Bounds(), img, rect.Min, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return regionalCrop{}, err
	}
	raw := buf.Bytes()
	clamped, mime, err := ClampImageBytesForVLM(raw)
	if err != nil {
		return regionalCrop{}, err
	}
	return regionalCrop{
		DataURL:      "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(clamped),
		PNG:          raw,
		Width:        rect.Dx(),
		Height:       rect.Dy(),
		NonBlankRate: estimateInspectNonBlankRatio(dst),
	}, nil
}

func imageFromDataURL(dataURL string) (image.Image, error) {
	idx := strings.Index(dataURL, ",")
	if idx < 0 {
		return nil, fmt.Errorf("invalid data url")
	}
	raw, err := base64.StdEncoding.DecodeString(dataURL[idx+1:])
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return img, nil
}

func lowConfidenceRegionsFromFullDoc(fullDoc map[string]any) []lowConfidenceRegion {
	lq := fullDocumentLocalQuality(fullDoc)
	raw, ok := lq["low_confidence_regions"]
	if !ok || raw == nil {
		return nil
	}
	var maps []map[string]any
	switch x := raw.(type) {
	case []map[string]any:
		maps = x
	case []any:
		for _, it := range x {
			if m, ok := it.(map[string]any); ok {
				maps = append(maps, m)
			}
		}
	}
	out := make([]lowConfidenceRegion, 0, len(maps))
	for i, m := range maps {
		pageIndex := intFromAnyPipeline(m["page_index"])
		box, ok := inspectBBox(m["bbox"])
		if pageIndex <= 0 || !ok {
			continue
		}
		id := strings.TrimSpace(fmt.Sprint(m["id"]))
		if id == "" {
			id = fmt.Sprintf("region_p%d_%d", pageIndex, i)
		}
		out = append(out, lowConfidenceRegion{
			ID:            id,
			PageIndex:     pageIndex,
			Reason:        strings.TrimSpace(fmt.Sprint(m["reason"])),
			Route:         strings.TrimSpace(fmt.Sprint(m["route"])),
			BBox:          box,
			TextPreview:   strings.TrimSpace(fmt.Sprint(m["text_preview"])),
			Confidence:    inspectFloatDefault(m["confidence"], 0),
			AnchorChunkID: strings.TrimSpace(fmt.Sprint(m["anchor_chunk_id"])),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].PageIndex != out[j].PageIndex {
			return out[i].PageIndex < out[j].PageIndex
		}
		if math.Abs(out[i].BBox["top"]-out[j].BBox["top"]) > 0.006 {
			return out[i].BBox["top"] < out[j].BBox["top"]
		}
		return out[i].BBox["left"] < out[j].BBox["left"]
	})
	return out
}

func setRegionalFallbackDocMeta(fullDoc map[string]any, res regionalFallbackResult) {
	doc, ok := fullDoc["document"].(map[string]any)
	if !ok {
		return
	}
	meta := map[string]any{
		"enabled":                  res.Enabled,
		"status":                   res.Status,
		"region_count":             res.RegionCount,
		"processed_region_count":   res.ProcessedRegionCount,
		"count":                    res.Count,
		"added_count":              res.AddedCount,
		"merged_count":             res.MergedCount,
		"anchor_bbox_repair_count": res.AnchorBBoxRepairCount,
		"ocr_count":                res.OCRCount,
		"vlm_count":                res.VLMCount,
		"duration_ms":              res.DurationMS,
	}
	if res.Model != "" {
		meta["model"] = res.Model
	}
	if res.OCREngine != "" {
		meta["ocr_engine"] = res.OCREngine
	}
	if res.Warning != "" {
		meta["warning"] = res.Warning
	}
	if res.RenderEngine != "" {
		meta["render_engine"] = res.RenderEngine
	}
	if len(res.Regions) > 0 {
		meta["regions"] = res.Regions
	}
	doc["regional_ocr_vlm"] = meta
}

func updateRegionalReport(reports []map[string]any, res regionalJobResult) {
	for _, report := range reports {
		if strings.TrimSpace(fmt.Sprint(report["id"])) != res.region.ID {
			continue
		}
		report["status"] = res.status
		report["duration_ms"] = res.durationMS
		if res.source != "" {
			report["source"] = res.source
		}
		if res.model != "" {
			report["model"] = res.model
		}
		if res.ocrEngine != "" {
			report["ocr_engine"] = res.ocrEngine
		}
		if len(res.chunks) > 0 {
			report["chunk_count"] = len(res.chunks)
		}
		if res.err != nil {
			report["warning"] = res.err.Error()
		}
		return
	}
}

func regionalFallbackEnabled() bool {
	return EnvBoolLower("CONTENT_PARSE_REGIONAL_OCR_VLM", true)
}

func regionalFallbackOCREnabled() bool {
	return EnvBoolLower("CONTENT_PARSE_REGIONAL_OCR", true)
}

func regionalFallbackVLMEnabled() bool {
	return EnvBoolLower("CONTENT_PARSE_REGIONAL_VLM", true) && VLMAPIKey() != ""
}

func regionalFallbackMaxRegions() int {
	n := EnvIntDefault("CONTENT_PARSE_REGIONAL_OCR_VLM_MAX", 4)
	if n <= 0 {
		return 0
	}
	if n > 24 {
		return 24
	}
	return n
}

func regionalFallbackConcurrency() int {
	n := EnvIntDefault("CONTENT_PARSE_REGIONAL_OCR_VLM_CONCURRENCY", 2)
	if n <= 0 {
		return 1
	}
	if n > 6 {
		return 6
	}
	return n
}

func regionalOCRTimeout() time.Duration {
	n := EnvIntDefault("CONTENT_PARSE_REGIONAL_OCR_TIMEOUT_SECONDS", 8)
	if n <= 0 {
		n = 8
	}
	if n > 45 {
		n = 45
	}
	return time.Duration(n) * time.Second
}

func regionalFallbackShouldPreferVLM(region lowConfidenceRegion) bool {
	reason := strings.ToLower(region.Reason)
	if strings.Contains(reason, "table") || strings.Contains(reason, "bbox_mismatch") {
		return true
	}
	return containsNonLatin(region.TextPreview)
}

func containsNonLatin(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

func regionalOCRTextUseful(text string) bool {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) < 12 {
		return false
	}
	letters, digits := 0, 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
		}
		if unicode.IsDigit(r) {
			digits++
		}
	}
	return letters+digits >= 8
}

func looksLikeRegionalHeading(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || strings.Contains(text, "\n") || utf8.RuneCountInString(text) > 80 {
		return false
	}
	if strings.HasSuffix(text, ":") || strings.HasSuffix(text, "：") {
		return true
	}
	return !strings.ContainsAny(text, ".。;；,，") && len(strings.Fields(text)) <= 8
}

func inspectBBox(v any) (map[string]float64, bool) {
	raw, ok := v.(map[string]any)
	if !ok || raw == nil {
		return nil, false
	}
	out := map[string]float64{}
	for _, k := range []string{"left", "right", "top", "bottom"} {
		f, ok := toInspectFloat(raw[k])
		if !ok {
			return nil, false
		}
		out[k] = clampInspect01(f)
	}
	if out["right"] <= out["left"] || out["bottom"] <= out["top"] {
		return nil, false
	}
	return out, true
}

func toInspectFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		var f float64
		if _, err := fmt.Sscanf(strings.TrimSpace(x), "%f", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

func inspectFloatDefault(v any, def float64) float64 {
	if f, ok := toInspectFloat(v); ok {
		return f
	}
	return def
}

func floatBoxToAny(box map[string]float64) map[string]any {
	if len(box) == 0 {
		return nil
	}
	return map[string]any{
		"left":   roundInspectFloat(box["left"], 6),
		"right":  roundInspectFloat(box["right"], 6),
		"top":    roundInspectFloat(box["top"], 6),
		"bottom": roundInspectFloat(box["bottom"], 6),
	}
}

func padInspectBox(box map[string]float64, pad float64) map[string]float64 {
	return map[string]float64{
		"left":   clampInspect01(box["left"] - pad),
		"right":  clampInspect01(box["right"] + pad),
		"top":    clampInspect01(box["top"] - pad),
		"bottom": clampInspect01(box["bottom"] + pad),
	}
}

func inspectBoxIoU(a, b map[string]float64) float64 {
	left := math.Max(a["left"], b["left"])
	right := math.Min(a["right"], b["right"])
	top := math.Max(a["top"], b["top"])
	bottom := math.Min(a["bottom"], b["bottom"])
	inter := math.Max(0, right-left) * math.Max(0, bottom-top)
	if inter <= 0 {
		return 0
	}
	areaA := math.Max(0, a["right"]-a["left"]) * math.Max(0, a["bottom"]-a["top"])
	areaB := math.Max(0, b["right"]-b["left"]) * math.Max(0, b["bottom"]-b["top"])
	union := areaA + areaB - inter
	if union <= 0 {
		return 0
	}
	return inter / union
}

func normalizeRegionalText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 160 {
		return out[:160]
	}
	return out
}

func estimateInspectNonBlankRatio(img image.Image) float64 {
	b := img.Bounds()
	stepX := max(1, b.Dx()/240)
	stepY := max(1, b.Dy()/240)
	total, nonBlank := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			r, g, bl, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			total++
			if r < 61000 || g < 61000 || bl < 61000 {
				nonBlank++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(nonBlank) / float64(total)
}

func roundInspectFloat(v float64, digits int) float64 {
	if digits < 0 {
		return v
	}
	p := math.Pow10(digits)
	return math.Round(v*p) / p
}

func clampInspect01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func truncateInspectString(s string, maxRunes int) string {
	r := []rune(strings.TrimSpace(s))
	if maxRunes <= 0 || len(r) <= maxRunes {
		return strings.TrimSpace(s)
	}
	return string(r[:maxRunes])
}

func safeIDPart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "region"
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	return b.String()
}
