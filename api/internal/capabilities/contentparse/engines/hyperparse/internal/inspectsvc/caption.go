package inspectsvc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	pdfadapter "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/adapters/pdf"
	pdforchestrator "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/orchestrators/pdf"
)

type imageCaptionJob struct {
	item map[string]any
	eb   pdfadapter.ExtractedImageBytes
}

type ImageCaptionProgress struct {
	Status       string
	Completed    int
	Total        int
	ObjectNumber int
	Mode         string
	Model        string
	Error        string
}

func likelyFullPageBackgroundImageForUI(eb pdfadapter.ExtractedImageBytes, objPageHits map[int]int) bool {
	if eb.Width <= 0 || eb.Height <= 0 {
		return false
	}
	area := eb.Width * eb.Height
	if area >= 1200*1600 {
		return true
	}
	if eb.ObjectNumber > 0 && objPageHits[eb.ObjectNumber] >= 2 && area >= 1000*1300 {
		return true
	}
	return false
}

func pageFilterSet(allowedPages []int) (map[int]bool, bool) {
	if allowedPages == nil {
		return nil, false
	}
	set := make(map[int]bool, len(allowedPages))
	for _, page := range allowedPages {
		if page > 0 {
			set[page] = true
		}
	}
	return set, true
}

// HasImageChunks reports whether chunks contain type=image.
func HasImageChunks(fullDoc map[string]any) bool {
	return HasImageChunksForPages(fullDoc, nil)
}

// HasImageChunksForPages reports whether selected pages contain type=image.
// allowedPages=nil means no page filter; allowedPages=[] matches no page.
func HasImageChunksForPages(fullDoc map[string]any, allowedPages []int) bool {
	chW, ok := fullDoc["chunks"].(map[string]any)
	if !ok {
		return false
	}
	allowedSet, restrictPages := pageFilterSet(allowedPages)
	items := CoerceChunkItems(chW["items"])
	for _, it := range items {
		page := IntFromChunkAny(it, "page_index")
		if restrictPages && !allowedSet[page] {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(it["type"])), "image") {
			return true
		}
	}
	return false
}

func existingImageObjectNumbers(items []map[string]any) map[int]bool {
	out := map[int]bool{}
	for _, it := range items {
		typ := strings.ToLower(strings.TrimSpace(fmt.Sprint(it["type"])))
		if typ != "image" && typ != "stamp" {
			continue
		}
		p, ok := it["payload"].(map[string]any)
		if !ok || p == nil {
			continue
		}
		on := IntFromChunkAny(p, "object_number")
		if on > 0 {
			out[on] = true
		}
	}
	return out
}

func likelyStampImageForUI(eb pdfadapter.ExtractedImageBytes) bool {
	if eb.Width <= 0 || eb.Height <= 0 {
		return false
	}
	aspect := float64(eb.Width) / float64(eb.Height)
	if aspect < 1 {
		aspect = 1 / aspect
	}
	if aspect > 1.8 {
		return false
	}
	area := eb.Width * eb.Height
	if area < 60*60 || area > 1400*1400 {
		return false
	}
	if len(eb.Bytes) == 0 {
		return false
	}
	img, _, err := image.Decode(bytes.NewReader(eb.Bytes))
	if err != nil {
		return false
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return false
	}
	stepX := max(1, w/512)
	stepY := max(1, h/512)
	var total, redLike, whiteLike, darkLike int
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			total++
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.R >= 130 && c.R >= c.G+18 && c.R >= c.B+18 {
				redLike++
			}
			if c.R >= 235 && c.G >= 235 && c.B >= 235 {
				whiteLike++
			}
			if c.R <= 85 && c.G <= 85 && c.B <= 85 {
				darkLike++
			}
		}
	}
	if total == 0 {
		return false
	}
	redRatio := float64(redLike) / float64(total)
	whiteRatio := float64(whiteLike) / float64(total)
	darkRatio := float64(darkLike) / float64(total)
	return redRatio >= 0.08 && whiteRatio >= 0.20 && darkRatio <= 0.55
}

type imageDecision struct {
	mode string
}

func decideImagePipelineMode(eb pdfadapter.ExtractedImageBytes) imageDecision {
	if len(eb.Bytes) == 0 {
		return imageDecision{mode: "caption"}
	}
	img, _, err := image.Decode(bytes.NewReader(eb.Bytes))
	if err != nil {
		return imageDecision{mode: "caption"}
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return imageDecision{mode: "caption"}
	}
	stepX := max(1, w/512)
	stepY := max(1, h/512)
	var total, alphaLow, whiteLike, darkLike int
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			total++
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.A < 245 {
				alphaLow++
			}
			if c.R >= 240 && c.G >= 240 && c.B >= 240 {
				whiteLike++
			}
			if c.R <= 90 && c.G <= 90 && c.B <= 90 {
				darkLike++
			}
		}
	}
	if total == 0 {
		return imageDecision{mode: "caption"}
	}
	alphaRatio := float64(alphaLow) / float64(total)
	whiteRatio := float64(whiteLike) / float64(total)
	textAreaRatio := float64(darkLike) / float64(total)
	isTextHeavy := (alphaRatio >= 0.05 || whiteRatio >= 0.55) && textAreaRatio > 0.20
	ratio := float64(w) / float64(h)
	if ratio < 1 {
		ratio = 1 / ratio
	}
	isLayoutLike := (w >= 900 || h >= 900) && whiteRatio >= 0.60 && textAreaRatio >= 0.08 && ratio >= 1.20 && ratio <= 1.55
	if isLayoutLike {
		return imageDecision{mode: "structured"}
	}
	if isTextHeavy {
		return imageDecision{mode: "text"}
	}
	return imageDecision{mode: "caption"}
}

func formatStructuredImageExtraction(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(s), &root); err != nil {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
		if err2 := json.Unmarshal([]byte(s), &root); err2 != nil {
			return s
		}
	}
	arr, _ := root["chunks"].([]any)
	if len(arr) == 0 {
		return s
	}
	lines := make([]string, 0, len(arr))
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		typ := strings.TrimSpace(fmt.Sprint(m["type"]))
		txt := strings.TrimSpace(fmt.Sprint(m["text"]))
		if txt == "" {
			continue
		}
		lines = append(lines, "["+typ+"] "+txt)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func maxChunkOrder(items []map[string]any) int {
	mx := 0
	for _, it := range items {
		o := IntFromChunkAny(it, "order")
		if o > mx {
			mx = o
		}
	}
	return mx
}

func extractedImageBytesToDataURL(eb pdfadapter.ExtractedImageBytes) (string, error) {
	if len(eb.Bytes) == 0 {
		return "", fmt.Errorf("empty image bytes")
	}
	b, mime, err := ClampImageBytesForVLM(eb.Bytes)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(b)), nil
}

func extractedImageBytesToPreviewDataURL(eb pdfadapter.ExtractedImageBytes, maxSide int) (string, int, int, error) {
	if len(eb.Bytes) == 0 {
		return "", 0, 0, fmt.Errorf("empty image bytes")
	}
	img, format, err := image.Decode(bytes.NewReader(eb.Bytes))
	if err != nil {
		dataURL, err2 := extractedImageBytesToDataURL(eb)
		if err2 != nil {
			return "", 0, 0, err
		}
		return dataURL, eb.Width, eb.Height, nil
	}
	if maxSide <= 0 {
		maxSide = 480
	}
	simg := scaleImageMaxSide(img, maxSide)
	bounds := simg.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		w, h = eb.Width, eb.Height
	}

	var buf bytes.Buffer
	mime := "image/png"
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "jpeg", "jpg":
		if err := jpeg.Encode(&buf, simg, &jpeg.Options{Quality: 82}); err != nil {
			return "", 0, 0, err
		}
		mime = "image/jpeg"
	default:
		if err := png.Encode(&buf, simg); err != nil {
			return "", 0, 0, err
		}
	}
	return fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(buf.Bytes())), w, h, nil
}

// AttachNativeImagePreviewPayloads adds lightweight preview data URLs for image/stamp chunks.
func AttachNativeImagePreviewPayloads(data []byte, mode string, fullDoc map[string]any) (warn string, attached int) {
	chW, ok := fullDoc["chunks"].(map[string]any)
	if !ok {
		return "", 0
	}
	items := CoerceChunkItems(chW["items"])
	if len(items) == 0 {
		return "", 0
	}

	targetsByObj := map[int][]map[string]any{}
	for _, it := range items {
		typ := strings.ToLower(strings.TrimSpace(fmt.Sprint(it["type"])))
		if typ != "image" && typ != "stamp" && typ != "figure" {
			continue
		}
		payload, ok := it["payload"].(map[string]any)
		if !ok || payload == nil {
			continue
		}
		if s, _ := payload["preview_data_url"].(string); strings.HasPrefix(s, "data:image/") {
			continue
		}
		on := IntFromChunkAny(payload, "object_number")
		if on <= 0 {
			continue
		}
		targetsByObj[on] = append(targetsByObj[on], it)
	}
	if len(targetsByObj) == 0 {
		return "", 0
	}

	pageInfos, err := pdfadapter.DetectPageInfosBytes(data, mode)
	if err != nil {
		return fmt.Sprintf("native_image_preview page_infos: %v", err), 0
	}
	specs, err := pdfadapter.DetectPageRenderSpecsFromPageInfos(data, pageInfos)
	if err != nil {
		return fmt.Sprintf("native_image_preview render_specs: %v", err), 0
	}
	extracted, err := pdfadapter.ExtractEmbeddedImagesFromBytesWithSpecs(data, mode, specs)
	if err != nil {
		return fmt.Sprintf("native_image_preview extract_images: %v", err), 0
	}

	type previewInfo struct {
		dataURL string
		width   int
		height  int
	}
	cache := map[int]previewInfo{}
	var errs []string
	for _, eb := range extracted {
		if eb.ObjectNumber <= 0 {
			continue
		}
		targets := targetsByObj[eb.ObjectNumber]
		if len(targets) == 0 {
			continue
		}
		info, ok := cache[eb.ObjectNumber]
		if !ok {
			dataURL, pw, ph, err := extractedImageBytesToPreviewDataURL(eb, 480)
			if err != nil {
				errs = append(errs, fmt.Sprintf("obj=%d: %v", eb.ObjectNumber, err))
				continue
			}
			info = previewInfo{dataURL: dataURL, width: pw, height: ph}
			cache[eb.ObjectNumber] = info
		}
		for _, it := range targets {
			payload, ok := it["payload"].(map[string]any)
			if !ok || payload == nil {
				continue
			}
			payload["preview_data_url"] = info.dataURL
			if info.width > 0 {
				payload["preview_width"] = info.width
			}
			if info.height > 0 {
				payload["preview_height"] = info.height
			}
			if strings.TrimSpace(fmt.Sprint(payload["image_kind"])) == "" {
				payload["image_kind"] = typOrDefault(strings.ToLower(strings.TrimSpace(fmt.Sprint(it["type"]))), "image")
			}
			attached++
		}
	}
	if len(errs) > 0 {
		return strings.Join(errs, "; "), attached
	}
	return "", attached
}

func typOrDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

// AppendNativeImageChunksIfMissing adds missing native image/stamp chunks.
func AppendNativeImageChunksIfMissing(data []byte, mode string, fullDoc map[string]any) (warn string, added int) {
	return AppendNativeImageChunksIfMissingForPages(data, mode, fullDoc, nil)
}

// AppendNativeImageChunksIfMissingForPages adds native image/stamp chunks only for selected pages.
// allowedPages=nil means no page filter; allowedPages=[] matches no page.
func AppendNativeImageChunksIfMissingForPages(data []byte, mode string, fullDoc map[string]any, allowedPages []int) (warn string, added int) {
	chW, ok := fullDoc["chunks"].(map[string]any)
	if !ok {
		return "", 0
	}
	allowedSet, restrictPages := pageFilterSet(allowedPages)
	pageInfos, err := pdfadapter.DetectPageInfosBytes(data, mode)
	if err != nil {
		return fmt.Sprintf("native_image_append page_infos: %v", err), 0
	}
	specs, err := pdfadapter.DetectPageRenderSpecsFromPageInfos(data, pageInfos)
	if err != nil {
		return fmt.Sprintf("native_image_append render_specs: %v", err), 0
	}
	extracted, err := pdfadapter.ExtractEmbeddedImagesFromBytesWithSpecs(data, mode, specs)
	if err != nil {
		return fmt.Sprintf("native_image_append extract_images: %v", err), 0
	}
	objPageHits := map[int]int{}
	for _, eb := range extracted {
		if eb.ObjectNumber > 0 {
			objPageHits[eb.ObjectNumber]++
		}
	}

	maxItems := EnvIntDefault("CONTENT_PARSE_NATIVE_IMAGE_APPEND_MAX", 64)
	items := CoerceChunkItems(chW["items"])
	existingObj := existingImageObjectNumbers(items)
	nextOrder := maxChunkOrder(items) + 1
	nextID := 0
	for _, eb := range extracted {
		if eb.ObjectNumber > 0 && existingObj[eb.ObjectNumber] {
			continue
		}
		pi := eb.PageIndex
		if pi <= 0 {
			pi = 1
		}
		if restrictPages && !allowedSet[pi] {
			continue
		}
		if likelyStampImageForUI(eb) {
			txt := strings.TrimSpace(fmt.Sprintf("Stamp image (%s, %dx%d)", strings.ToLower(strings.TrimSpace(eb.Format)), eb.Width, eb.Height))
			ch := map[string]any{
				"chunk_id":     fmt.Sprintf("native_stamp_append_%d", nextID),
				"type":         "stamp",
				"text":         txt,
				"page_index":   pi,
				"order":        nextOrder,
				"source":       "native_pdf",
				"confidence":   0.90,
				"source_trace": fmt.Sprintf("native:stamp_append:page#%d", pi),
				"payload": map[string]any{
					"native_image_append": true,
					"image_kind":          "stamp",
					"page_index":          pi,
					"page_object":         eb.PageObject,
					"xobject_name":        eb.XObjectName,
					"object_number":       eb.ObjectNumber,
					"format":              strings.ToLower(strings.TrimSpace(eb.Format)),
					"width":               eb.Width,
					"height":              eb.Height,
					"byte_size":           len(eb.Bytes),
				},
			}
			items = append(items, ch)
			if eb.ObjectNumber > 0 {
				existingObj[eb.ObjectNumber] = true
			}
			nextOrder++
			nextID++
			added++
			if maxItems > 0 && added >= maxItems {
				break
			}
			continue
		}
		if likelyFullPageBackgroundImageForUI(eb, objPageHits) {
			continue
		}
		txt := strings.TrimSpace(fmt.Sprintf("Embedded image (%s, %dx%d)", strings.ToLower(strings.TrimSpace(eb.Format)), eb.Width, eb.Height))
		ch := map[string]any{
			"chunk_id":     fmt.Sprintf("native_img_append_%d", nextID),
			"type":         "image",
			"text":         txt,
			"page_index":   pi,
			"order":        nextOrder,
			"source":       "native_pdf",
			"confidence":   0.88,
			"source_trace": fmt.Sprintf("native:image_append:page#%d", pi),
			"payload": map[string]any{
				"native_image_append": true,
				"page_index":          pi,
				"page_object":         eb.PageObject,
				"xobject_name":        eb.XObjectName,
				"object_number":       eb.ObjectNumber,
				"format":              strings.ToLower(strings.TrimSpace(eb.Format)),
				"width":               eb.Width,
				"height":              eb.Height,
				"byte_size":           len(eb.Bytes),
			},
		}
		items = append(items, ch)
		if eb.ObjectNumber > 0 {
			existingObj[eb.ObjectNumber] = true
		}
		nextOrder++
		nextID++
		added++
		if maxItems > 0 && added >= maxItems {
			break
		}
	}
	if added == 0 {
		return "", 0
	}
	chW["items"] = items
	chW["count"] = len(items)
	log.Printf("[ui.inspect] native: image_append added=%d", added)
	return "", added
}

// EnrichImageChunksWithVLMCaptions enriches image chunks with VLM caption, OCR text, and structure.
func EnrichImageChunksWithVLMCaptions(data []byte, mode string, fullDoc map[string]any) (usedModel string, warn string, count int) {
	return EnrichImageChunksWithVLMCaptionsForPages(data, mode, fullDoc, nil)
}

// EnrichImageChunksWithVLMCaptionsForPages enriches image chunks only for selected pages.
// allowedPages=nil means no page filter; allowedPages=[] matches no page.
func EnrichImageChunksWithVLMCaptionsForPages(data []byte, mode string, fullDoc map[string]any, allowedPages []int) (usedModel string, warn string, count int) {
	return EnrichImageChunksWithVLMCaptionsForPagesProgressContext(context.Background(), data, mode, fullDoc, allowedPages, nil)
}

// EnrichImageChunksWithVLMCaptionsForPagesProgress behaves like
// EnrichImageChunksWithVLMCaptionsForPages and reports progress as image VLM jobs start and finish.
func EnrichImageChunksWithVLMCaptionsForPagesProgress(data []byte, mode string, fullDoc map[string]any, allowedPages []int, progress func(ImageCaptionProgress)) (usedModel string, warn string, count int) {
	return EnrichImageChunksWithVLMCaptionsForPagesProgressContext(context.Background(), data, mode, fullDoc, allowedPages, progress)
}

func EnrichImageChunksWithVLMCaptionsForPagesProgressContext(ctx context.Context, data []byte, mode string, fullDoc map[string]any, allowedPages []int, progress func(ImageCaptionProgress)) (usedModel string, warn string, count int) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !EnvBoolLower("CONTENT_PARSE_VLM_IMAGE_CAPTION", true) {
		if progress != nil {
			progress(ImageCaptionProgress{Status: "disabled"})
		}
		return "", "", 0
	}
	if VLMAPIKey() == "" {
		if progress != nil {
			progress(ImageCaptionProgress{Status: "skipped_missing_vlm_api_key"})
		}
		return "", "", 0
	}
	chW, ok := fullDoc["chunks"].(map[string]any)
	if !ok {
		if progress != nil {
			progress(ImageCaptionProgress{Status: "skipped_no_chunks"})
		}
		return "", "", 0
	}
	allowedSet, restrictPages := pageFilterSet(allowedPages)
	items := CoerceChunkItems(chW["items"])
	if len(items) == 0 {
		if progress != nil {
			progress(ImageCaptionProgress{Status: "skipped_no_chunks"})
		}
		return "", "", 0
	}

	pageInfos, err := pdfadapter.DetectPageInfosBytes(data, mode)
	if err != nil {
		if progress != nil {
			progress(ImageCaptionProgress{Status: "warning", Error: err.Error()})
		}
		return "", fmt.Sprintf("page_infos: %v", err), 0
	}
	specs, err := pdfadapter.DetectPageRenderSpecsFromPageInfos(data, pageInfos)
	if err != nil {
		if progress != nil {
			progress(ImageCaptionProgress{Status: "warning", Error: err.Error()})
		}
		return "", fmt.Sprintf("render_specs: %v", err), 0
	}
	extracted, err := pdfadapter.ExtractEmbeddedImagesFromBytesWithSpecs(data, mode, specs)
	if err != nil {
		if progress != nil {
			progress(ImageCaptionProgress{Status: "warning", Error: err.Error()})
		}
		return "", fmt.Sprintf("extract_images: %v", err), 0
	}
	byObj := make(map[int]pdfadapter.ExtractedImageBytes)
	for _, eb := range extracted {
		if eb.ObjectNumber <= 0 {
			continue
		}
		if _, exists := byObj[eb.ObjectNumber]; !exists {
			byObj[eb.ObjectNumber] = eb
		}
	}

	var jobs []imageCaptionJob
	for _, it := range items {
		if strings.ToLower(strings.TrimSpace(fmt.Sprint(it["type"]))) != "image" {
			continue
		}
		if restrictPages {
			page := IntFromChunkAny(it, "page_index")
			if !allowedSet[page] {
				continue
			}
		}
		p, ok := it["payload"].(map[string]any)
		if !ok {
			continue
		}
		on := IntFromChunkAny(p, "object_number")
		if on <= 0 {
			continue
		}
		eb, ok := byObj[on]
		if !ok || len(eb.Bytes) == 0 {
			continue
		}
		if len(strings.TrimSpace(fmt.Sprint(it["text"]))) > 120 {
			continue
		}
		jobs = append(jobs, imageCaptionJob{item: it, eb: eb})
	}
	if len(jobs) == 0 {
		if progress != nil {
			progress(ImageCaptionProgress{Status: "skipped_no_image_chunks"})
		}
		return "", "", 0
	}

	sort.Slice(jobs, func(i, j int) bool {
		pi := IntFromChunkAny(jobs[i].item, "page_index")
		pj := IntFromChunkAny(jobs[j].item, "page_index")
		if pi != pj {
			return pi < pj
		}
		oi := IntFromChunkAny(jobs[i].item, "order")
		oj := IntFromChunkAny(jobs[j].item, "order")
		return oi < oj
	})

	maxN := EnvIntDefault("CONTENT_PARSE_VLM_IMAGE_CAPTION_MAX", 16)
	if maxN > 0 && len(jobs) > maxN {
		jobs = jobs[:maxN]
	}
	conc := EnvIntDefault("CONTENT_PARSE_VLM_IMAGE_CAPTION_CONCURRENCY", 2)
	if conc < 1 {
		conc = 1
	}
	if progress != nil {
		progress(ImageCaptionProgress{Status: "running", Completed: 0, Total: len(jobs)})
	}

	type capRes struct {
		cap   string
		model string
		err   error
		mode  string
	}
	results := make([]capRes, len(jobs))

	var wg sync.WaitGroup
	sem := make(chan struct{}, conc)
	var progressMu sync.Mutex
	completed := 0
	t0 := time.Now()
	for i := range jobs {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i].err = ctx.Err()
				reportImageCaptionProgress(progress, &progressMu, &completed, len(jobs), jobs[i], results[i])
				return
			}
			defer func() { <-sem }()
			if err := ctx.Err(); err != nil {
				results[i].err = err
				reportImageCaptionProgress(progress, &progressMu, &completed, len(jobs), jobs[i], results[i])
				return
			}
			dataURL, err := extractedImageBytesToDataURL(jobs[i].eb)
			if err != nil {
				results[i].err = err
				reportImageCaptionProgress(progress, &progressMu, &completed, len(jobs), jobs[i], results[i])
				return
			}
			decision := decideImagePipelineMode(jobs[i].eb)
			if decision.mode == "structured" {
				txt, model, err := callDashscopeVLMImageStructuredContext(ctx, dataURL)
				results[i].cap = strings.TrimSpace(formatStructuredImageExtraction(txt))
				results[i].model = model
				results[i].err = err
				results[i].mode = "structured"
				reportImageCaptionProgress(progress, &progressMu, &completed, len(jobs), jobs[i], results[i])
				return
			}
			if decision.mode == "text" {
				txt, model, err := callDashscopeVLMImageTextContext(ctx, dataURL)
				results[i].cap = strings.TrimSpace(txt)
				results[i].model = model
				results[i].err = err
				results[i].mode = "text"
				reportImageCaptionProgress(progress, &progressMu, &completed, len(jobs), jobs[i], results[i])
				return
			}
			cap, model, err := callDashscopeVLMImageCaptionContext(ctx, dataURL)
			results[i].cap = strings.TrimSpace(cap)
			results[i].model = model
			results[i].err = err
			results[i].mode = "caption"
			reportImageCaptionProgress(progress, &progressMu, &completed, len(jobs), jobs[i], results[i])
		}()
	}
	wg.Wait()
	elapsed := time.Since(t0).Milliseconds()
	log.Printf("[ui.inspect] vlm: image_caption batch jobs=%d conc=%d elapsed_ms=%d", len(jobs), conc, elapsed)

	var firstModel string
	var errs []string
	for i := range jobs {
		if results[i].err != nil {
			errs = append(errs, fmt.Sprintf("obj=%d: %v", jobs[i].eb.ObjectNumber, results[i].err))
			continue
		}
		cap := results[i].cap
		if cap == "" {
			continue
		}
		if firstModel == "" && results[i].model != "" {
			firstModel = results[i].model
		}
		it := jobs[i].item
		old := strings.TrimSpace(fmt.Sprint(it["text"]))
		if old == "<nil>" || strings.EqualFold(old, "null") {
			old = ""
		}
		wrapped := fmt.Sprintf("<<:figure: %s::>", cap)
		if results[i].mode == "text" {
			wrapped = cap
		}
		if old != "" {
			it["text"] = old + "\n\n" + wrapped
		} else {
			it["text"] = wrapped
		}
		if p, ok := it["payload"].(map[string]any); ok {
			p["vlm_caption"] = cap
			if results[i].model != "" {
				p["vlm_caption_model"] = results[i].model
			}
			p["vlm_caption_source"] = "vlm_openai_compatible"
			p["vlm_caption_mode"] = results[i].mode
		}
		count++
	}

	if len(errs) > 0 {
		warn = strings.Join(errs, "; ")
		if len(warn) > 800 {
			warn = warn[:800] + "…"
		}
	}

	if doc, ok := fullDoc["document"].(map[string]any); ok && count > 0 {
		merged := CoerceChunkItems(chW["items"])
		source := ""
		if ts, ok := doc["text_summary"].(map[string]any); ok {
			source = strings.TrimSpace(fmt.Sprint(ts["recognition_source"]))
		}
		pdforchestrator.RebuildTextSummaryAfterVLMMerge(doc, merged, source)
	}

	return firstModel, warn, count
}

func reportImageCaptionProgress(progress func(ImageCaptionProgress), mu *sync.Mutex, completed *int, total int, job imageCaptionJob, res struct {
	cap   string
	model string
	err   error
	mode  string
}) {
	if progress == nil {
		return
	}
	mu.Lock()
	*completed = *completed + 1
	current := *completed
	mu.Unlock()
	status := "done"
	errMsg := ""
	if res.err != nil {
		status = "warning"
		errMsg = res.err.Error()
	}
	progress(ImageCaptionProgress{
		Status:       status,
		Completed:    current,
		Total:        total,
		ObjectNumber: job.eb.ObjectNumber,
		Mode:         res.mode,
		Model:        res.model,
		Error:        errMsg,
	})
}
