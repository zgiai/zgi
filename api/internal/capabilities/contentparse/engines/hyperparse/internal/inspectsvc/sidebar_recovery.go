package inspectsvc

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	localocr "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/ocr"
	"golang.org/x/image/draw"
)

type sidebarRegionSpec struct {
	Name     string
	Crop     normalizedCropRect
	Coverage normalizedCropRect
	MinWidth int
}

func defaultSidebarRegions() []sidebarRegionSpec {
	return []sidebarRegionSpec{
		{
			Name:     "sidebar_contact_title",
			Crop:     normalizedCropRect{Left: 0.62, Top: 0.03, Right: 0.95, Bottom: 0.12},
			Coverage: normalizedCropRect{Left: 0.62, Top: 0.03, Right: 0.95, Bottom: 0.14},
			MinWidth: 1600,
		},
		{
			Name:     "sidebar_contact",
			Crop:     normalizedCropRect{Left: 0.62, Top: 0.10, Right: 0.97, Bottom: 0.42},
			Coverage: normalizedCropRect{Left: 0.62, Top: 0.10, Right: 0.97, Bottom: 0.45},
			MinWidth: 1800,
		},
		{
			Name:     "sidebar_payment",
			Crop:     normalizedCropRect{Left: 0.62, Top: 0.38, Right: 0.97, Bottom: 0.78},
			Coverage: normalizedCropRect{Left: 0.62, Top: 0.38, Right: 0.97, Bottom: 0.80},
			MinWidth: 1800,
		},
		{
			Name:     "sidebar_fuel_mix",
			Crop:     normalizedCropRect{Left: 0.62, Top: 0.72, Right: 0.97, Bottom: 0.98},
			Coverage: normalizedCropRect{Left: 0.62, Top: 0.72, Right: 0.97, Bottom: 0.98},
			MinWidth: 1800,
		},
	}
}

func defaultSidebarOCRRegions() []sidebarRegionSpec {
	return []sidebarRegionSpec{
		{
			Name:     "sidebar_right_column",
			Crop:     normalizedCropRect{Left: 0.62, Top: 0.03, Right: 0.97, Bottom: 0.98},
			Coverage: normalizedCropRect{Left: 0.62, Top: 0.03, Right: 0.97, Bottom: 0.98},
			MinWidth: 2200,
		},
	}
}

// AppendOCRRightSidebarTextFromRenderedPages recovers missing right-column text
// with the configured local OCR engine before the VLM fallback is considered.
func AppendOCRRightSidebarTextFromRenderedPages(ctx context.Context, fullDoc map[string]any, pageDataURLs []string, pageNumbers []int) (engine string, warn string, added int) {
	return AppendOCRRightSidebarTextFromRenderedPagesWithConfig(ctx, fullDoc, pageDataURLs, pageNumbers, sidebarOCRConfig())
}

func AppendOCRRightSidebarTextFromRenderedPagesWithConfig(ctx context.Context, fullDoc map[string]any, pageDataURLs []string, pageNumbers []int, ocrConfig localocr.Config) (engine string, warn string, added int) {
	if len(pageDataURLs) == 0 || len(pageDataURLs) != len(pageNumbers) {
		return "", "", 0
	}
	engine = ocrConfig.EngineName()
	chW, ok := fullDoc["chunks"].(map[string]any)
	if !ok {
		return engine, "missing chunks", 0
	}

	items := CoerceChunkItems(chW["items"])
	pageComparableText := buildComparablePageTextByChunk(items)
	nextOrder := maxChunkOrder(items) + 1
	regions := defaultSidebarOCRRegions()
	pagesAdded := make([]int, 0, len(pageNumbers))
	pageSeen := map[int]bool{}
	var warns []string

	for i, dataURL := range pageDataURLs {
		page := 0
		if i < len(pageNumbers) {
			page = pageNumbers[i]
		}
		if page < 1 {
			continue
		}

		for _, region := range regions {
			if pageHasSidebarRegionCoverage(items, page, region.Coverage) {
				continue
			}
			croppedDataURL, err := cropPageDataURL(dataURL, region.Crop, region.MinWidth)
			if err != nil {
				warns = append(warns, fmt.Sprintf("page=%d region=%s crop: %v", page, region.Name, err))
				continue
			}
			imgBytes, err := decodeDataURLBytes(croppedDataURL)
			if err != nil {
				warns = append(warns, fmt.Sprintf("page=%d region=%s decode: %v", page, region.Name, err))
				continue
			}
			text, usedEngine, err := runSidebarOCR(ctx, ocrConfig, imgBytes)
			if usedEngine != "" {
				engine = usedEngine
			}
			if err != nil {
				warns = append(warns, fmt.Sprintf("page=%d region=%s ocr: %v", page, region.Name, err))
				continue
			}
			text = sanitizeSidebarOCRText(text)
			if text == "" || !sidebarTextAddsNovelty(text, pageComparableText[page]) {
				continue
			}

			recovered := buildSidebarRecoveryChunks(text, region, page, "ocr", engine, nextOrder, added, 0.68)
			if len(recovered) == 0 {
				continue
			}
			items = append(items, recovered...)
			pageComparableText[page] += normalizeSidebarComparableText(text)
			nextOrder += len(recovered)
			added += len(recovered)
			if !pageSeen[page] {
				pagesAdded = append(pagesAdded, page)
				pageSeen[page] = true
			}
		}
	}

	if added > 0 {
		chW["items"] = items
		chW["count"] = len(items)
		log.Printf("[ui.inspect] ocr: sidebar_recovery added=%d pages=%v", added, normalizeProcessedPages(pagesAdded))
	}
	if len(warns) > 0 {
		warn = strings.Join(warns, "; ")
		if len(warn) > 800 {
			warn = warn[:800] + "..."
		}
	}
	return engine, warn, added
}

// AppendVLMRightSidebarTextFromRenderedPages runs local crop enlargement OCR for
// pages with missing right-sidebar text and appends recovered text to chunks.
func AppendVLMRightSidebarTextFromRenderedPages(fullDoc map[string]any, pageDataURLs []string, pageNumbers []int) (usedModel string, warn string, added int) {
	if len(pageDataURLs) == 0 || len(pageDataURLs) != len(pageNumbers) {
		return "", "", 0
	}
	if VLMAPIKey() == "" {
		return "", "", 0
	}
	chW, ok := fullDoc["chunks"].(map[string]any)
	if !ok {
		return "", "", 0
	}

	items := CoerceChunkItems(chW["items"])
	pageComparableText := buildComparablePageTextByChunk(items)
	nextOrder := maxChunkOrder(items) + 1
	regions := defaultSidebarRegions()
	for _, region := range regions {
		if !region.Crop.valid() || !region.Coverage.valid() {
			return "", "invalid sidebar crop", 0
		}
	}

	pagesAdded := make([]int, 0, len(pageNumbers))
	pageSeen := map[int]bool{}
	var warns []string
	for i, dataURL := range pageDataURLs {
		page := 0
		if i < len(pageNumbers) {
			page = pageNumbers[i]
		}
		if page < 1 {
			continue
		}

		for _, region := range regions {
			if pageHasSidebarRegionCoverage(items, page, region.Coverage) {
				continue
			}

			croppedDataURL, err := cropPageDataURL(dataURL, region.Crop, region.MinWidth)
			if err != nil {
				warns = append(warns, fmt.Sprintf("page=%d region=%s crop: %v", page, region.Name, err))
				continue
			}

			text, model, err := callDashscopeVLMImageSidebarText(croppedDataURL)
			if usedModel == "" && strings.TrimSpace(model) != "" {
				usedModel = model
			}
			if err != nil {
				warns = append(warns, fmt.Sprintf("page=%d region=%s vlm: %v", page, region.Name, err))
				continue
			}

			text = sanitizeSidebarText(text)
			if text == "" || !sidebarTextAddsNovelty(text, pageComparableText[page]) {
				continue
			}

			recovered := buildSidebarRecoveryChunks(text, region, page, "vlm", model, nextOrder, added, 0.76)
			for _, item := range recovered {
				item["vlm_merge"] = "from_vlm_sidebar"
				if payload, ok := item["payload"].(map[string]any); ok {
					payload["vlm_sidebar_model"] = model
				}
			}
			if len(recovered) == 0 {
				continue
			}
			items = append(items, recovered...)
			pageComparableText[page] += normalizeSidebarComparableText(text)
			nextOrder += len(recovered)
			added += len(recovered)
			if !pageSeen[page] {
				pagesAdded = append(pagesAdded, page)
				pageSeen[page] = true
			}
		}
	}

	if added > 0 {
		chW["items"] = items
		chW["count"] = len(items)
		log.Printf("[ui.inspect] vlm: sidebar_recovery added=%d pages=%v", added, normalizeProcessedPages(pagesAdded))
	}

	if len(warns) > 0 {
		warn = strings.Join(warns, "; ")
		if len(warn) > 800 {
			warn = warn[:800] + "…"
		}
	}
	return usedModel, warn, added
}

func buildSidebarRecoveryChunks(text string, region sidebarRegionSpec, page int, source string, engine string, firstOrder int, firstIndex int, confidence float64) []map[string]any {
	groups := splitSidebarRecoveryTextGroups(text)
	if len(groups) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(groups))
	for i, group := range groups {
		groupText := strings.TrimSpace(strings.Join(group, "\n"))
		if groupText == "" {
			continue
		}
		chunkType := "paragraph"
		if len(group) == 1 && looksLikeSidebarHeadingText(group[0]) {
			chunkType = "heading"
		}
		if tableText, ok := markdownTableFromSidebarNumericRows(groupText); ok {
			chunkType = "table"
			groupText = tableText
		}
		payload := map[string]any{
			"sidebar_recovery":        true,
			"sidebar_recovery_region": region.Name,
			"crop":                    region.Crop.bboxMap(),
			"table_repair":            chunkType == "table",
		}
		if source == "ocr" {
			payload["sidebar_recovery_engine"] = engine
		}
		prefix := source
		if prefix == "" {
			prefix = "sidebar"
		}
		out = append(out, map[string]any{
			"bbox":         sidebarGroupBBox(region.Crop, i, len(groups)),
			"chunk_id":     fmt.Sprintf("%s_%s_p%d_%d", prefix, region.Name, page, firstIndex+i),
			"confidence":   confidence,
			"order":        firstOrder + i,
			"page_index":   page,
			"source":       source,
			"source_trace": fmt.Sprintf("%s:%s:page#%d", source, region.Name, page),
			"text":         groupText,
			"type":         chunkType,
			"payload":      payload,
		})
	}
	return out
}

func splitSidebarRecoveryTextGroups(text string) [][]string {
	lines := splitSidebarLines(text)
	if len(lines) == 0 {
		return nil
	}
	var groups [][]string
	current := make([]string, 0, 8)
	flush := func() {
		if len(current) > 0 {
			groups = append(groups, current)
			current = nil
		}
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(current) > 0 && (looksLikeSidebarHeadingText(line) || len(current) >= 12) {
			flush()
		}
		current = append(current, line)
	}
	flush()
	return groups
}

func looksLikeSidebarHeadingText(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" || len([]rune(t)) > 72 {
		return false
	}
	low := strings.ToLower(t)
	for _, kw := range []string{
		"customer service", "complaints", "payment options", "energy efficiency",
		"information on", "public service", "fuel mix", "disclosure label",
	} {
		if strings.Contains(low, kw) {
			return true
		}
	}
	if strings.ContainsAny(t, ".。:：,，;；") {
		return false
	}
	letters, digits := 0, 0
	for _, r := range t {
		if unicode.IsLetter(r) {
			letters++
		}
		if unicode.IsDigit(r) {
			digits++
		}
	}
	return letters > 0 && digits <= letters && len(strings.Fields(t)) <= 8
}

func markdownTableFromSidebarNumericRows(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	type row struct {
		label string
		a     string
		b     string
	}
	rows := make([]row, 0)
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 {
			continue
		}
		a := fields[len(fields)-2]
		b := fields[len(fields)-1]
		if !looksLikeSidebarPercentToken(a) || !looksLikeSidebarPercentToken(b) {
			continue
		}
		label := strings.Join(fields[:len(fields)-2], " ")
		if label == "" {
			continue
		}
		rows = append(rows, row{label: label, a: a, b: b})
	}
	if len(rows) < 3 {
		return "", false
	}
	var b strings.Builder
	b.WriteString("| Item | Value 1 | Value 2 |\n")
	b.WriteString("| --- | ---: | ---: |\n")
	for _, r := range rows {
		b.WriteString("| ")
		b.WriteString(escapeSidebarMarkdownTableCell(r.label))
		b.WriteString(" | ")
		b.WriteString(escapeSidebarMarkdownTableCell(r.a))
		b.WriteString(" | ")
		b.WriteString(escapeSidebarMarkdownTableCell(r.b))
		b.WriteString(" |\n")
	}
	return b.String(), true
}

func looksLikeSidebarPercentToken(token string) bool {
	token = strings.TrimSpace(token)
	if !strings.HasSuffix(token, "%") {
		return false
	}
	hasDigit := false
	for _, r := range token {
		if unicode.IsDigit(r) {
			hasDigit = true
			continue
		}
		if r != '.' && r != '%' {
			return false
		}
	}
	return hasDigit
}

func escapeSidebarMarkdownTableCell(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "|", "\\|")
}

func sidebarGroupBBox(crop normalizedCropRect, idx int, total int) map[string]any {
	if total <= 1 {
		return crop.bboxMap()
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= total {
		idx = total - 1
	}
	height := crop.Bottom - crop.Top
	top := crop.Top + height*float64(idx)/float64(total)
	bottom := crop.Top + height*float64(idx+1)/float64(total)
	return normalizedCropRect{Left: crop.Left, Top: top, Right: crop.Right, Bottom: bottom}.bboxMap()
}

func pageHasRightSidebarCoverage(items []map[string]any, page int) bool {
	for _, region := range defaultSidebarRegions() {
		if pageHasSidebarRegionCoverage(items, page, region.Coverage) {
			return true
		}
	}
	return false
}

// PageHasRightSidebarCoverage reports whether native chunks already cover the
// standard right-sidebar regions. Local parse uses this to avoid unnecessary VLM
// calls while still recovering narrow bill/form sidebars that native extraction
// often misses.
func PageHasRightSidebarCoverage(items []map[string]any, page int) bool {
	return pageHasRightSidebarCoverage(items, page)
}

func pageHasSidebarRegionCoverage(items []map[string]any, page int, target normalizedCropRect) bool {
	for _, item := range items {
		if IntFromChunkAny(item, "page_index") != page {
			continue
		}
		typ := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["type"])))
		if typ == "image" || typ == "stamp" || typ == "table" {
			continue
		}
		left, top, right, bottom, ok := chunkBBox(item)
		if !ok {
			continue
		}
		if !target.overlaps(left, top, right, bottom) {
			continue
		}
		overlapW, overlapH := target.overlapSize(left, top, right, bottom)
		if overlapW < 0.08 || overlapH <= 0.08 {
			continue
		}
		centerX := (left + right) / 2
		width := right - left
		height := bottom - top
		if centerX < 0.68 || width > 0.5 || height < 0.03 {
			continue
		}
		if len([]rune(normalizeSidebarComparableText(fmt.Sprint(item["text"])))) >= 10 {
			return true
		}
	}
	return false
}

func cropPageDataURL(dataURL string, crop normalizedCropRect, minWidth int) (string, error) {
	if !crop.valid() {
		return "", fmt.Errorf("invalid crop rect")
	}
	raw, err := decodeDataURLBytes(dataURL)
	if err != nil {
		return "", err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("decode crop image: %w", err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return "", fmt.Errorf("empty crop image")
	}

	x0 := b.Min.X + int(float64(w)*crop.Left)
	x1 := b.Min.X + int(float64(w)*crop.Right)
	y0 := b.Min.Y + int(float64(h)*crop.Top)
	y1 := b.Min.Y + int(float64(h)*crop.Bottom)
	if x1 <= x0 || y1 <= y0 {
		return "", fmt.Errorf("crop resolved to empty rect")
	}

	dst := image.NewRGBA(image.Rect(0, 0, x1-x0, y1-y0))
	draw.Draw(dst, dst.Bounds(), img, image.Point{X: x0, Y: y0}, draw.Src)
	upscaled := scaleImageUpToWidth(dst, minWidth)

	var buf bytes.Buffer
	if err := png.Encode(&buf, upscaled); err != nil {
		return "", fmt.Errorf("encode crop image: %w", err)
	}
	clamped, mime, err := ClampImageBytesForVLM(buf.Bytes())
	if err != nil {
		return "", err
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(clamped), nil
}

func decodeDataURLBytes(dataURL string) ([]byte, error) {
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 || !strings.Contains(parts[0], ";base64") {
		return nil, fmt.Errorf("invalid data url")
	}
	b, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode data url: %w", err)
	}
	return b, nil
}

func scaleImageUpToWidth(img image.Image, minWidth int) image.Image {
	if minWidth <= 0 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w >= minWidth || w <= 0 || h <= 0 {
		return img
	}
	newH := max(1, h*minWidth/w)
	dst := image.NewRGBA(image.Rect(0, 0, minWidth, newH))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return scaleImageMaxSide(dst, 4096)
}

func runSidebarOCR(ctx context.Context, ocrConfig localocr.Config, imageBytes []byte) (string, string, error) {
	res, err := ocrConfig.RunText(ctx, imageBytes)
	if err != nil {
		return "", ocrConfig.EngineName(), err
	}
	text := res.Text
	if strings.TrimSpace(text) == "" {
		text = res.Raw
	}
	return text, res.Engine, nil
}

func sidebarOCRConfig() localocr.Config {
	cfg := localocr.LoadConfig(sidebarOCRTimeout())
	cfg.Lang = sidebarOCRLangForEngine(cfg.EngineName())
	return cfg
}

func sidebarOCRLangForEngine(engine string) string {
	if v := contentParseEnv("CONTENT_PARSE_OCR_LANG"); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("LOCAL_SIDEBAR_OCR_LANG")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("LOCAL_OCR_LANG")); v != "" {
		return v
	}
	if v := contentParseEnv("CONTENT_PARSE_LOCAL_OCR_LANG"); v != "" {
		return v
	}
	if engine == localocr.EnginePaddleOCR {
		return "en"
	}
	return "eng"
}

func sidebarOCRTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("LOCAL_SIDEBAR_OCR_TIMEOUT_SECONDS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 15 * time.Second
}

func sanitizeSidebarText(text string) string {
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "```"))
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	if strings.EqualFold(text, "unrecognized") || text == "\u672a\u8bc6\u522b" {
		return ""
	}
	return text
}

func sanitizeSidebarOCRText(text string) string {
	text = sanitizeSidebarText(text)
	if text == "" {
		return ""
	}
	lines := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(strings.TrimSpace(line)), " ")
		if line == "" {
			continue
		}
		normalized := normalizeSidebarComparableText(line)
		if len([]rune(normalized)) <= 1 || looksLowSignalSidebarLine(normalized) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func sidebarTextAddsNovelty(text string, pageComparableText string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	if pageComparableText == "" {
		return true
	}
	novel := 0
	for _, line := range splitSidebarLines(text) {
		normalized := normalizeSidebarComparableText(line)
		if len([]rune(normalized)) < 6 {
			continue
		}
		if strings.Contains(pageComparableText, normalized) {
			continue
		}
		if looksLowSignalSidebarLine(normalized) {
			continue
		}
		novel++
	}
	return novel > 0
}

func splitSidebarLines(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func normalizeSidebarComparableText(text string) string {
	low := strings.ToLower(strings.TrimSpace(text))
	var b strings.Builder
	for _, r := range low {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func looksLowSignalSidebarLine(text string) bool {
	if text == "" {
		return true
	}
	return strings.HasPrefix(text, "page") || strings.HasPrefix(text, "archivep")
}

func buildComparablePageTextByChunk(items []map[string]any) map[int]string {
	out := map[int]string{}
	for _, item := range items {
		page := IntFromChunkAny(item, "page_index")
		if page < 1 {
			continue
		}
		out[page] += normalizeSidebarComparableText(fmt.Sprint(item["text"]))
	}
	return out
}

func chunkBBox(c map[string]any) (left, top, right, bottom float64, ok bool) {
	bb, has := c["bbox"].(map[string]any)
	if !has {
		return 0, 0, 0, 0, false
	}
	left = chunkBBoxFloat(bb["left"])
	top = chunkBBoxFloat(bb["top"])
	right = chunkBBoxFloat(bb["right"])
	bottom = chunkBBoxFloat(bb["bottom"])
	if left == 0 && top == 0 && right == 0 && bottom == 0 {
		return 0, 0, 0, 0, false
	}
	return left, top, right, bottom, true
}

func chunkBBoxFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	default:
		return 0
	}
}
