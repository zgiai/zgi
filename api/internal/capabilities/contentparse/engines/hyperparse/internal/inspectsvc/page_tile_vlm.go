package inspectsvc

import (
	"fmt"
	"log"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	oversizedPageLongSidePt         = 2000.0
	oversizedPageAreaPt2            = 5_000_000.0
	oversizedPageTileCount          = 2
	oversizedPageTileMinWidth       = 1800
	oversizedPageInspectRenderScale = 2048
)

type structuredVLMCaller func(images []map[string]any) (model string, chunks []map[string]any, raw string, timing VLMCallTimingBreakdown, err error)

type oversizedPagePlan struct {
	WidthPt  float64
	HeightPt float64
	Tiles    []pageTileRef
}

func oversizedPageSetFromFullDoc(fullDoc map[string]any) map[int]oversizedPagePlan {
	rawPages := fullDocPages(fullDoc)
	if rawPages == nil {
		return nil
	}
	rv := reflect.ValueOf(rawPages)
	out := map[int]oversizedPagePlan{}
	for i := 0; i < rv.Len(); i++ {
		pageIndex, mediaBox, cropBox, ok := extractFullDocPageBoxInfo(rv.Index(i).Interface())
		if !ok || pageIndex < 1 {
			continue
		}
		box := cropBox
		if strings.TrimSpace(box) == "" {
			box = mediaBox
		}
		widthPt, heightPt, ok := parsePDFBoxSizePt(box)
		if !ok {
			continue
		}
		longSide := math.Max(widthPt, heightPt)
		area := widthPt * heightPt
		if longSide >= oversizedPageLongSidePt || area >= oversizedPageAreaPt2 {
			out[pageIndex] = oversizedPagePlan{
				WidthPt:  widthPt,
				HeightPt: heightPt,
				Tiles:    buildOversizedPageTiles(pageIndex, widthPt, heightPt),
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func fullDocPages(fullDoc map[string]any) any {
	if fullDoc == nil {
		return nil
	}
	if rawPages, ok := fullDoc["pages"]; ok && isSliceValue(rawPages) {
		return rawPages
	}
	doc, _ := fullDoc["document"].(map[string]any)
	if doc == nil {
		return nil
	}
	layout, _ := doc["layout"].(map[string]any)
	if layout == nil {
		return nil
	}
	rawPages, ok := layout["pages"]
	if !ok || !isSliceValue(rawPages) {
		return nil
	}
	return rawPages
}

func CallDashscopeVLMFallbackStructuredForRenderedPages(pageDataURLs []string, pageNumbers []int, oversizedPages map[int]oversizedPagePlan) (model string, chunks []map[string]any, raw string, err error) {
	model, chunks, raw, _, err = CallDashscopeVLMFallbackStructuredForRenderedPagesProfiled(pageDataURLs, pageNumbers, oversizedPages)
	return model, chunks, raw, err
}

func CallDashscopeVLMFallbackStructuredForRenderedPagesProfiled(pageDataURLs []string, pageNumbers []int, oversizedPages map[int]oversizedPagePlan) (model string, chunks []map[string]any, raw string, timing VLMCallTimingBreakdown, err error) {
	if len(pageDataURLs) == 0 {
		return "", nil, "", timing, nil
	}
	if len(pageDataURLs) != len(pageNumbers) {
		return "", nil, "", timing, fmt.Errorf("rendered pages mismatch: data_urls=%d page_numbers=%d", len(pageDataURLs), len(pageNumbers))
	}
	if !hasOversizedRenderedPages(pageNumbers, oversizedPages) {
		return CallDashscopeVLMFallbackStructuredBatchedProfiled(VLMImageContentFromDataURLs(pageDataURLs), pageNumbers)
	}
	concurrency := VLMFallbackConcurrency()
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(pageDataURLs) {
		concurrency = len(pageDataURLs)
	}
	type pageResult struct {
		idx    int
		model  string
		items  []map[string]any
		raw    string
		timing VLMCallTimingBreakdown
		err    error
	}
	results := make([]pageResult, len(pageDataURLs))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i := range pageDataURLs {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			page := pageNumbers[i]
			m, items, rw, tt, callErr := callStructuredVLMFallbackForRenderedPageProfiled(pageDataURLs[i], page, oversizedPages[page], CallDashscopeVLMFallbackStructuredProfiled)
			results[i] = pageResult{idx: i, model: m, items: items, raw: rw, timing: tt, err: callErr}
		}()
	}
	wg.Wait()

	firstModel := ""
	rawParts := make([]string, 0, len(results))
	merged := make([]map[string]any, 0, len(results)*2)
	var errs []string
	for _, result := range results {
		timing.Merge(result.timing)
		if result.err != nil {
			errs = append(errs, fmt.Sprintf("page %d: %v", pageNumbers[result.idx], result.err))
			continue
		}
		if firstModel == "" && strings.TrimSpace(result.model) != "" {
			firstModel = result.model
		}
		if strings.TrimSpace(result.raw) != "" {
			rawParts = append(rawParts, result.raw)
		}
		merged = append(merged, result.items...)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		pi := IntFromChunkAny(merged[i], "page_index")
		pj := IntFromChunkAny(merged[j], "page_index")
		if pi != pj {
			return pi < pj
		}
		return IntFromChunkAny(merged[i], "order") < IntFromChunkAny(merged[j], "order")
	})
	if len(merged) == 0 && len(errs) > 0 {
		return firstModel, nil, strings.Join(rawParts, "\n\n"), timing, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	if len(errs) > 0 {
		log.Printf("[ui.vlm.fallback] tiled partial errors: %s", strings.Join(errs, "; "))
	}
	return firstModel, merged, strings.Join(rawParts, "\n\n"), timing, nil
}

func callStructuredVLMFallbackForRenderedPage(dataURL string, pageNumber int, plan oversizedPagePlan, caller structuredVLMCaller) (model string, items []map[string]any, raw string, err error) {
	model, items, raw, _, err = callStructuredVLMFallbackForRenderedPageProfiled(dataURL, pageNumber, plan, caller)
	return model, items, raw, err
}

func callStructuredVLMFallbackForRenderedPageProfiled(dataURL string, pageNumber int, plan oversizedPagePlan, caller structuredVLMCaller) (model string, items []map[string]any, raw string, timing VLMCallTimingBreakdown, err error) {
	if len(plan.Tiles) == 0 {
		model, items, raw, timing, err = caller(VLMImageContentFromDataURLs([]string{dataURL}))
		if err == nil {
			RemapVLMChunkPages(items, []int{pageNumber})
		}
		return model, items, raw, timing, err
	}

	tiles := plan.Tiles
	timing.TiledPages = 1
	timing.TileCalls = len(tiles)
	log.Printf("[ui.vlm.fallback] oversized page tiled page=%d tiles=%d", pageNumber, len(tiles))
	var firstModel string
	merged := make([]map[string]any, 0, len(tiles)*2)
	rawParts := make([]string, 0, len(tiles))
	for _, tile := range tiles {
		croppedDataURL, cropErr := cropPageDataURL(dataURL, tile.Crop, oversizedPageTileMinWidth)
		if cropErr != nil {
			return firstModel, nil, strings.Join(rawParts, "\n\n"), timing, fmt.Errorf("tile %d crop: %w", tile.TileIndex, cropErr)
		}
		tileModel, tileItems, tileRaw, tileTiming, callErr := caller(VLMImageContentFromDataURLs([]string{croppedDataURL}))
		timing.Merge(tileTiming)
		if callErr != nil {
			return firstModel, nil, strings.Join(rawParts, "\n\n"), timing, fmt.Errorf("tile %d vlm: %w", tile.TileIndex, callErr)
		}
		if firstModel == "" && strings.TrimSpace(tileModel) != "" {
			firstModel = tileModel
		}
		if strings.TrimSpace(tileRaw) != "" {
			rawParts = append(rawParts, tileRaw)
		}
		RemapVLMChunkPages(tileItems, []int{pageNumber})
		applyTileMetadataToChunks(tileItems, tile)
		merged = append(merged, tileItems...)
	}
	merged = dedupeTileChunks(merged)
	return firstModel, merged, strings.Join(rawParts, "\n\n"), timing, nil
}

func applyTileMetadataToChunks(items []map[string]any, tile pageTileRef) {
	orderBase := (tile.TileIndex - 1) * 10000
	for idx, item := range items {
		attachChunkTileRef(item, tile)
		RemapChunkBBoxFromTile(item, tile)
		sourceTrace := strings.TrimSpace(fmt.Sprint(item["source_trace"]))
		if sourceTrace == "" {
			sourceTrace = fmt.Sprintf("vlm:page#%d", tile.PageIndex)
		}
		item["source_trace"] = fmt.Sprintf("%s:tile#%d", sourceTrace, tile.TileIndex)
		order := IntFromChunkAny(item, "order")
		if order <= 0 {
			order = idx + 1
		}
		item["order"] = orderBase + order
	}
}

func hasOversizedRenderedPages(pageNumbers []int, oversizedPages map[int]oversizedPagePlan) bool {
	if len(pageNumbers) == 0 || len(oversizedPages) == 0 {
		return false
	}
	for _, page := range pageNumbers {
		if len(oversizedPages[page].Tiles) > 0 {
			return true
		}
	}
	return false
}

func buildOversizedPageTiles(pageIndex int, widthPt, heightPt float64) []pageTileRef {
	if widthPt <= 0 || heightPt <= 0 {
		return SplitPageIntoGridTiles(pageIndex, 1, oversizedPageTileCount, 0, 0.04)
	}
	if heightPt >= widthPt {
		return SplitPageIntoGridTiles(pageIndex, 1, oversizedPageTileCount, 0, 0.04)
	}
	return SplitPageIntoGridTiles(pageIndex, oversizedPageTileCount, 1, 0.04, 0)
}

func buildOversizedPageRenderScaleOverrides(pageNumbers []int, oversizedPages map[int]oversizedPagePlan) pageRenderScaleOverrides {
	if len(pageNumbers) == 0 || len(oversizedPages) == 0 {
		return nil
	}
	out := make(pageRenderScaleOverrides)
	for _, page := range pageNumbers {
		if len(oversizedPages[page].Tiles) > 0 {
			out[page] = oversizedPageInspectRenderScale
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func dedupeTileChunks(items []map[string]any) []map[string]any {
	if len(items) <= 1 {
		return items
	}
	kept := make([]map[string]any, 0, len(items))
	for _, item := range items {
		duplicateIndex := -1
		for i, existing := range kept {
			if tileChunksDuplicate(existing, item) {
				duplicateIndex = i
				break
			}
		}
		if duplicateIndex < 0 {
			kept = append(kept, item)
			continue
		}
		existing := kept[duplicateIndex]
		if tileChunkNormalizedTextLen(item) > tileChunkNormalizedTextLen(existing) {
			item["order"] = existing["order"]
			kept[duplicateIndex] = item
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		pi := IntFromChunkAny(kept[i], "page_index")
		pj := IntFromChunkAny(kept[j], "page_index")
		if pi != pj {
			return pi < pj
		}
		return IntFromChunkAny(kept[i], "order") < IntFromChunkAny(kept[j], "order")
	})
	return kept
}

func tileChunksDuplicate(a, b map[string]any) bool {
	if IntFromChunkAny(a, "page_index") != IntFromChunkAny(b, "page_index") {
		return false
	}
	ta := strings.ToLower(strings.TrimSpace(fmt.Sprint(a["type"])))
	tb := strings.ToLower(strings.TrimSpace(fmt.Sprint(b["type"])))
	if ta == "" || ta != tb {
		return false
	}
	if !chunkHasTileRef(a) || !chunkHasTileRef(b) {
		return false
	}
	normA := normalizeTileChunkText(fmt.Sprint(a["text"]))
	normB := normalizeTileChunkText(fmt.Sprint(b["text"]))
	if len([]rune(normA)) < 12 || len([]rune(normB)) < 12 {
		return false
	}
	if strings.Contains(normA, normB) || strings.Contains(normB, normA) {
		return true
	}
	return normalizedTextsLikelyDuplicate(normA, normB)
}

func chunkHasTileRef(item map[string]any) bool {
	payload, _ := item["payload"].(map[string]any)
	if payload == nil {
		return false
	}
	_, ok := payload["tile_ref"].(map[string]any)
	return ok
}

func tileChunkNormalizedTextLen(item map[string]any) int {
	return len([]rune(normalizeTileChunkText(fmt.Sprint(item["text"]))))
}

func normalizeTileChunkText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	var b strings.Builder
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 0x4e00 && r <= 0x9fff:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func normalizedTextsLikelyDuplicate(a, b string) bool {
	prefix := sharedPrefixRunes(a, b)
	if prefix < 20 {
		return false
	}
	minLen := len([]rune(a))
	if n := len([]rune(b)); n < minLen {
		minLen = n
	}
	return float64(prefix) >= float64(minLen)*0.6
}

func sharedPrefixRunes(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	n := len(ar)
	if len(br) < n {
		n = len(br)
	}
	count := 0
	for i := 0; i < n; i++ {
		if ar[i] != br[i] {
			break
		}
		count++
	}
	return count
}

func extractFullDocPageBoxInfo(page any) (pageIndex int, mediaBox string, cropBox string, ok bool) {
	if page == nil {
		return 0, "", "", false
	}
	if m, ok := page.(map[string]any); ok {
		return IntFromChunkAny(m, "page_index"), stringAnyTrim(m["media_box"]), stringAnyTrim(m["crop_box"]), true
	}
	rv := reflect.ValueOf(page)
	if !rv.IsValid() {
		return 0, "", "", false
	}
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return 0, "", "", false
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return 0, "", "", false
	}
	pageIndex = intStructField(rv, "PageIndex")
	mediaBox = stringStructField(rv, "MediaBox")
	cropBox = stringStructField(rv, "CropBox")
	return pageIndex, mediaBox, cropBox, pageIndex > 0
}

func isSliceValue(v any) bool {
	if v == nil {
		return false
	}
	rv := reflect.ValueOf(v)
	return rv.IsValid() && rv.Kind() == reflect.Slice
}

func intStructField(rv reflect.Value, name string) int {
	field := rv.FieldByName(name)
	if !field.IsValid() {
		return 0
	}
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(field.Int())
	default:
		return 0
	}
}

func stringStructField(rv reflect.Value, name string) string {
	field := rv.FieldByName(name)
	if !field.IsValid() || field.Kind() != reflect.String {
		return ""
	}
	return strings.TrimSpace(field.String())
}

func stringAnyTrim(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func parsePDFBoxSizePt(raw string) (widthPt, heightPt float64, ok bool) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) != 4 {
		return 0, 0, false
	}
	left, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, 0, false
	}
	bottom, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 0, false
	}
	right, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return 0, 0, false
	}
	top, err := strconv.ParseFloat(fields[3], 64)
	if err != nil {
		return 0, 0, false
	}
	widthPt = math.Abs(right - left)
	heightPt = math.Abs(top - bottom)
	if widthPt <= 0 || heightPt <= 0 {
		return 0, 0, false
	}
	return widthPt, heightPt, true
}
