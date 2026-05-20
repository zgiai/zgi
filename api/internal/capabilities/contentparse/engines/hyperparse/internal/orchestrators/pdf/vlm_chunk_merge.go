package pdf

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/core/chunking"
)

// Native chunk types kept during VLM merge. Structural metadata still comes
// from the PDF, while body text can be replaced by VLM output.
// stamp/image chunks come from native XObject scanning and are not reliably
// reproduced by VLM layout JSON, so they must be preserved.
var nativeChunkTypesPreserveWithVLM = map[string]bool{
	"bookmark":   true,
	"annotation": true,
	"form_field": true,
	"attachment": true,
	"stamp":      true,
	"image":      true,
}

type VLMChunkMergeStats struct {
	NativeKeptCount    int
	VLMMergedCount     int
	VLMPagesApplied    []int
	NativeResidualKept int
}

var comparableHTMLTagRE = regexp.MustCompile(`<[^>]+>`)
var residualStrongKeywordRE = regexp.MustCompile(`(?i)\b(account|invoice|date of issue|invoice number|customer service|complaints?|payment options?|pay by|direct debit|phone|call|email|address|mprn|iban|bic|service|networks?|vat reg)\b`)
var residualLongDigitRE = regexp.MustCompile(`\d(?:[\d ]{6,}\d)`)

const (
	legacyFormulaPrefixFullWidth = "\u516c\u5f0f\uff1a"
	legacyFormulaPrefixASCII     = "\u516c\u5f0f:"
)

// CountNativeChunksPreservedWithVLM returns the number of native chunks kept by the merge policy.
func CountNativeChunksPreservedWithVLM(native []map[string]any) int {
	count := 0
	for _, c := range native {
		t, _ := c["type"].(string)
		if nativeChunkTypesPreserveWithVLM[t] {
			count++
		}
	}
	return count
}

// MergeNativeAndVLMChunkItemsForPages allows VLM replacement only on selected pages.
// Pages without actual VLM chunks keep native content even if selected as candidates.
func MergeNativeAndVLMChunkItemsForPages(native []map[string]any, vlm []map[string]any, selectedPages []int) ([]map[string]any, VLMChunkMergeStats) {
	stats := VLMChunkMergeStats{}
	if len(vlm) == 0 {
		out := make([]map[string]any, len(native))
		copy(out, native)
		stats.NativeKeptCount = len(native)
		log.Printf("[vlm_chunk] merge skip (no vlm items) native=%d", len(native))
		return out, stats
	}

	selectedSet, selectedList := pageSet(selectedPages)
	filteredVLM := make([]map[string]any, 0, len(vlm))
	appliedPageSet := make(map[int]bool, len(vlm))
	for _, c := range vlm {
		page := chunkPageIndex(c)
		if len(selectedSet) > 0 && !selectedSet[page] {
			continue
		}
		filteredVLM = append(filteredVLM, c)
		if page > 0 {
			appliedPageSet[page] = true
		}
	}
	appliedPages := pageListFromSet(appliedPageSet)
	stats.VLMPagesApplied = appliedPages
	stats.VLMMergedCount = len(filteredVLM)
	if len(filteredVLM) == 0 {
		out := make([]map[string]any, len(native))
		copy(out, native)
		stats.NativeKeptCount = len(native)
		log.Printf("[vlm_chunk] merge skip (filtered vlm empty) native=%d selected=%v", len(native), selectedList)
		return out, stats
	}
	filteredVLM = alignVLMChunkGrounding(native, filteredVLM)

	pageVLMText := buildComparablePageTexts(filteredVLM)
	var kept []map[string]any
	for _, c := range native {
		page := chunkPageIndex(c)
		t, _ := c["type"].(string)
		if !appliedPageSet[page] || nativeChunkTypesPreserveWithVLM[t] {
			nc := cloneChunkMap(c)
			if appliedPageSet[page] {
				nc["vlm_merge"] = "kept_native"
			} else {
				nc["vlm_merge"] = "kept_native_page"
			}
			kept = append(kept, nc)
			continue
		}
		if shouldKeepNativeResidualChunk(c, pageVLMText[page]) {
			nc := cloneChunkMap(c)
			nc["vlm_merge"] = "kept_native_residual"
			kept = append(kept, nc)
			stats.NativeResidualKept++
		}
	}
	stats.NativeKeptCount = len(kept)

	maxOrd := 0
	for _, c := range kept {
		if o, ok := chunkOrder(c); ok && o > maxOrd {
			maxOrd = o
		}
	}
	base := maxOrd + 1
	vlmNext := base
	vlmAdj := make([]map[string]any, 0, len(filteredVLM))
	for _, c := range filteredVLM {
		nc := cloneChunkMap(c)
		nc["order"] = vlmNext
		nc["vlm_merge"] = "from_vlm"
		vlmNext++
		vlmAdj = append(vlmAdj, nc)
	}

	out := append(kept, vlmAdj...)
	sort.SliceStable(out, func(i, j int) bool {
		pi := chunkPageIndex(out[i])
		pj := chunkPageIndex(out[j])
		if pi != pj {
			return pi < pj
		}
		oi, _ := chunkOrder(out[i])
		oj, _ := chunkOrder(out[j])
		return oi < oj
	})
	for i := range out {
		out[i]["order"] = i
	}
	log.Printf("[vlm_chunk] merge done native_in=%d kept_native=%d residual_kept=%d vlm_in=%d vlm_used=%d selected_pages=%v applied_pages=%v merged=%d",
		len(native), len(kept), stats.NativeResidualKept, len(vlm), len(filteredVLM), selectedList, appliedPages, len(out))
	return out, stats
}

// ParseVLMChunksJSON parses model output shaped as {"chunks":[{...},...]}.
// Markdown JSON fences are tolerated.
func ParseVLMChunksJSON(modelContent string) ([]map[string]any, error) {
	s := stripMarkdownCodeFence(strings.TrimSpace(modelContent))
	if s == "" {
		return nil, fmt.Errorf("empty vlm content")
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(s), &root); err != nil {
		return nil, fmt.Errorf("json root: %w", err)
	}
	raw, ok := root["chunks"]
	if !ok {
		return nil, fmt.Errorf("missing chunks array")
	}
	arr, ok := raw.([]any)
	if !ok || len(arr) == 0 {
		return nil, fmt.Errorf("chunks not a non-empty array")
	}
	out := make([]map[string]any, 0, len(arr))
	for i, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		norm, err := normalizeVLMChunkItem(m, i)
		if err != nil {
			continue
		}
		out = append(out, norm)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid chunk objects")
	}
	log.Printf("[vlm_chunk] parse_json ok valid_chunks=%d (raw_array_len=%d)", len(out), len(arr))
	return out, nil
}

// VLMChunksFallbackSingle wraps non-JSON model output as one paragraph chunk for merge.
func VLMChunksFallbackSingle(raw string, pageCount int) []map[string]any {
	t := strings.TrimSpace(raw)
	if t == "" {
		return nil
	}
	pi := 1
	if pageCount > 1 {
		pi = 1
	}
	return []map[string]any{
		{
			"type":         "paragraph",
			"page_index":   pi,
			"text":         t,
			"order":        0,
			"confidence":   0.72,
			"source_trace": "vlm:fallback_plain",
			"chunk_id":     "vlm_fallback_0",
			"source":       "vlm",
		},
	}
}

func stripMarkdownCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	lines := strings.Split(s, "\n")
	var b strings.Builder
	in := false
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "```") {
			in = !in
			continue
		}
		if in {
			b.WriteString(ln)
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String())
}

func normalizeVLMChunkItem(m map[string]any, seq int) (map[string]any, error) {
	out := make(map[string]any)
	typ := strings.ToLower(strings.TrimSpace(fmt.Sprint(m["type"])))
	captionPayload := false
	switch typ {
	case "", "text", "body":
		typ = "paragraph"
	case "list", "bullet":
		typ = "list_item"
	case "math", "equation":
		typ = "formula"
	case "figure", "photo", "pic":
		typ = "image"
	case "handwritten", "handwrite", "comment", "note", "margin_note", "marginal_note":
		typ = "annotation"
	case "caption":
		typ = "paragraph"
		captionPayload = true
	}
	out["type"] = typ

	pi := intFromAny(m["page_index"])
	if pi < 1 {
		pi = 1
	}
	out["page_index"] = pi

	ord := seq
	if m["order"] != nil {
		if o := intFromAny(m["order"]); o >= 0 {
			ord = o
		}
	}
	out["order"] = ord

	txt := strings.TrimSpace(fmt.Sprint(m["text"]))
	if typ == "image" && txt != "" && !strings.Contains(txt, "<<:figure:") {
		txt = fmt.Sprintf("<<:figure: %s::>", txt)
	}
	if typ == "formula" {
		txt = normalizeFormulaText(txt, m)
	}
	out["text"] = txt

	conf := 0.82
	if v, ok := toFloat64(m["confidence"]); ok {
		conf = v
	}
	conf = math.Max(0, math.Min(1, conf))
	out["confidence"] = conf

	st := strings.TrimSpace(fmt.Sprint(m["source_trace"]))
	if st == "" {
		st = fmt.Sprintf("vlm:page#%d", pi)
	}
	out["source_trace"] = st

	out["chunk_id"] = fmt.Sprintf("vlm_p%d_%d", pi, seq)
	out["source"] = "vlm"

	if captionPayload {
		out["payload"] = map[string]any{"kind": "caption"}
	}
	if typ == "annotation" {
		if existing, ok := out["payload"].(map[string]any); ok {
			existing["handwritten"] = true
			out["payload"] = existing
		} else {
			out["payload"] = map[string]any{"handwritten": true}
		}
	}
	if typ == "image" {
		if existing, ok := out["payload"].(map[string]any); ok {
			existing["vlm_image_caption"] = true
			out["payload"] = existing
		} else {
			out["payload"] = map[string]any{"vlm_image_caption": true}
		}
	}
	if p, ok := m["payload"].(map[string]any); ok && len(p) > 0 {
		if existing, ok := out["payload"].(map[string]any); ok {
			for k, v := range p {
				existing[k] = v
			}
			out["payload"] = existing
		} else {
			out["payload"] = p
		}
	}

	if bb, ok := m["bbox"].(map[string]any); ok {
		if nb := normalizeBBoxMap(bb); nb != nil {
			out["bbox"] = nb
		}
	}
	nb, _ := out["bbox"].(map[string]any)
	// Images without bbox can easily become whole-page summaries; downgrade to
	// paragraph to avoid preview/result mismatch.
	if typ == "image" && nb == nil && looksLikePageSummaryForImage(txt, nil) {
		out["type"] = "paragraph"
		if p, ok := out["payload"].(map[string]any); ok {
			p["vlm_image_downgraded"] = true
			p["vlm_image_downgraded_reason"] = "missing_bbox"
			out["payload"] = p
		} else {
			out["payload"] = map[string]any{
				"vlm_image_downgraded":        true,
				"vlm_image_downgraded_reason": "missing_bbox",
			}
		}
	}
	// Filter mojibake text chunks before they pollute results.
	if looksLikeMojibakeText(txt) && (typ == "paragraph" || typ == "table" || typ == "kv") {
		return nil, fmt.Errorf("mojibake text chunk")
	}
	// If the model labels whole-page or large-summary text as image, downgrade
	// to paragraph to avoid flooding the list with figure captions.
	if typ == "image" {
		if looksLikePageSummaryForImage(txt, nb) {
			out["type"] = "paragraph"
			if p, ok := out["payload"].(map[string]any); ok {
				p["vlm_image_downgraded"] = true
				p["vlm_image_downgraded_reason"] = "likely_page_summary"
				out["payload"] = p
			} else {
				out["payload"] = map[string]any{
					"vlm_image_downgraded":        true,
					"vlm_image_downgraded_reason": "likely_page_summary",
				}
			}
		}
	}
	if finalTyp := strings.ToLower(strings.TrimSpace(fmt.Sprint(out["type"]))); finalTyp == "table" {
		page0 := intFromAny(out["page_index"]) - 1
		if page0 < 0 {
			page0 = 0
		}
		if html := vlmTableChunkToHTML(out, page0); html != "" {
			out["text"] = html
		}
	}
	return out, nil
}

// vlmTableChunkToHTML normalizes VLM tables to native-compatible <table id>/<td id>
// HTML. It prefers payload.cells, then GFM pipe tables, and keeps existing HTML.
func vlmTableChunkToHTML(out map[string]any, page0 int) string {
	txt := strings.TrimSpace(fmt.Sprint(out["text"]))
	if strings.Contains(strings.ToLower(txt), "<table") {
		return txt
	}
	if p, ok := out["payload"].(map[string]any); ok {
		if html, _ := chunking.HTMLTableWithIDsFromPayload(p, page0); html != "" {
			return html
		}
	}
	if grid, ok := parseMarkdownPipeTable(txt); ok && len(grid) > 0 {
		return chunking.HTMLTableWithIDsFromGrid(page0, grid)
	}
	return ""
}

func parseMarkdownPipeTable(s string) ([][]string, bool) {
	var rows [][]string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			continue
		}
		if isMarkdownTableSeparatorLine(line) {
			continue
		}
		inner := strings.Trim(line, "|")
		parts := strings.Split(inner, "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(strings.ReplaceAll(parts[i], "\\|", "|"))
		}
		if len(parts) > 0 {
			rows = append(rows, parts)
		}
	}
	if len(rows) < 1 {
		return nil, false
	}
	return rows, true
}

func isMarkdownTableSeparatorLine(line string) bool {
	if !strings.Contains(line, "---") {
		return false
	}
	for _, r := range strings.TrimSpace(line) {
		if r != '|' && r != '-' && r != ':' && r != ' ' {
			return false
		}
	}
	return true
}

func looksLikePageSummaryForImage(text string, bbox map[string]any) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return true
	}
	if bbox != nil {
		l := numFromMap(bbox, "left")
		r := numFromMap(bbox, "right")
		top := numFromMap(bbox, "top")
		bottom := numFromMap(bbox, "bottom")
		area := math.Max(0, r-l) * math.Max(0, bottom-top)
		if area > 0.62 {
			return true
		}
	}
	// Without bbox, downgrade very long whole-page summaries; local image
	// descriptions are usually shorter.
	if bbox == nil && len([]rune(t)) > 120 {
		return true
	}
	return strings.Count(t, "。") >= 4 && len([]rune(t)) > 140
}

func normalizeFormulaText(text string, raw map[string]any) string {
	t := strings.TrimSpace(text)
	// Keep canonical formula text stable for downstream chunking.
	if strings.HasPrefix(t, "formula:") && strings.Contains(t, "|") {
		return t
	}
	expr := ""
	desc := ""
	if p, ok := raw["payload"].(map[string]any); ok {
		expr = strings.TrimSpace(fmt.Sprint(p["expression"]))
		desc = strings.TrimSpace(fmt.Sprint(p["description"]))
	}
	if expr == "" {
		// Try splitting an existing expression and description from text.
		if strings.Contains(t, "|") {
			ps := strings.SplitN(t, "|", 2)
			expr = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(ps[0], "formula:"), legacyFormulaPrefixFullWidth), legacyFormulaPrefixASCII))
			desc = strings.TrimSpace(ps[1])
		} else {
			expr = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(t, "formula:"), legacyFormulaPrefixFullWidth), legacyFormulaPrefixASCII))
		}
	}
	if desc == "" {
		desc = "no description provided"
	}
	if expr == "" {
		expr = "unrecognized expression"
	}
	return fmt.Sprintf("formula:%s|%s", expr, desc)
}

func looksLikeMojibakeText(text string) bool {
	r := []rune(strings.TrimSpace(text))
	if len(r) == 0 {
		return false
	}
	bad := 0
	for _, ch := range r {
		if ch == '\ufffd' || ch == '�' {
			bad++
			continue
		}
		if (ch >= 0x2500 && ch <= 0x257f) || (ch >= 0xfff0 && ch <= 0xffff) {
			bad++
		}
	}
	ratio := float64(bad) / float64(len(r))
	return ratio >= 0.12 || bad >= 18
}

func normalizeBBoxMap(bb map[string]any) map[string]any {
	l := numFromMap(bb, "left")
	r := numFromMap(bb, "right")
	t := numFromMap(bb, "top")
	bo := numFromMap(bb, "bottom")
	if l == 0 && r == 0 && t == 0 && bo == 0 {
		return nil
	}
	return map[string]any{
		"left": l, "right": r, "top": t, "bottom": bo,
	}
}

func numFromMap(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	f, _ := toFloat64(v)
	return f
}

func intFromAny(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	default:
		return 0
	}
}

func toFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// MergeNativeAndVLMChunkItems keeps native structural chunks and replaces body/table
// chunks with VLM output, then rebuilds global order.
func MergeNativeAndVLMChunkItems(native []map[string]any, vlm []map[string]any) []map[string]any {
	out, _ := MergeNativeAndVLMChunkItemsForPages(native, vlm, nil)
	return out
}

func cloneChunkMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func pageSet(pages []int) (map[int]bool, []int) {
	seen := make(map[int]bool, len(pages))
	out := make([]int, 0, len(pages))
	for _, page := range pages {
		if page < 1 || seen[page] {
			continue
		}
		seen[page] = true
		out = append(out, page)
	}
	sort.Ints(out)
	set := make(map[int]bool, len(out))
	for _, page := range out {
		set[page] = true
	}
	return set, out
}

func pageListFromSet(set map[int]bool) []int {
	if len(set) == 0 {
		return nil
	}
	out := make([]int, 0, len(set))
	for page := range set {
		if page > 0 {
			out = append(out, page)
		}
	}
	sort.Ints(out)
	return out
}

func buildComparablePageTexts(items []map[string]any) map[int]string {
	out := map[int]string{}
	for _, item := range items {
		page := chunkPageIndex(item)
		if page < 1 {
			continue
		}
		text := normalizeComparableText(fmt.Sprint(item["text"]))
		if text == "" {
			continue
		}
		out[page] += text
	}
	return out
}

func shouldKeepNativeResidualChunk(chunk map[string]any, pageComparableText string) bool {
	if strings.TrimSpace(pageComparableText) == "" {
		return false
	}
	typ := strings.ToLower(strings.TrimSpace(fmt.Sprint(chunk["type"])))
	switch typ {
	case "paragraph", "heading", "kv", "list_item", "other":
	default:
		return false
	}

	text := strings.TrimSpace(fmt.Sprint(chunk["text"]))
	if text == "" {
		return false
	}
	if strings.Contains(strings.ToLower(text), "<table") {
		return false
	}
	if len([]rune(text)) > 360 {
		return false
	}

	strongNovelLines := 0
	for _, line := range splitResidualCandidateLines(text) {
		normalized := normalizeComparableText(line)
		if len([]rune(normalized)) < 6 {
			continue
		}
		if strings.Contains(pageComparableText, normalized) {
			continue
		}
		if looksLikeLowSignalResidual(normalized) {
			continue
		}
		if residualLineLooksStrong(line, normalized) {
			strongNovelLines++
		}
	}

	return strongNovelLines >= 1
}

func splitResidualCandidateLines(text string) []string {
	decoded := html.UnescapeString(strings.TrimSpace(text))
	if decoded == "" {
		return nil
	}
	decoded = comparableHTMLTagRE.ReplaceAllString(decoded, "\n")
	fields := strings.FieldsFunc(decoded, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func normalizeComparableText(text string) string {
	decoded := html.UnescapeString(strings.ToLower(strings.TrimSpace(text)))
	if decoded == "" {
		return ""
	}
	decoded = comparableHTMLTagRE.ReplaceAllString(decoded, " ")
	var b strings.Builder
	for _, r := range decoded {
		switch {
		case unicode.IsLetter(r):
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsNumber(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

func looksLikeLowSignalResidual(normalized string) bool {
	if normalized == "" {
		return true
	}
	if len([]rune(normalized)) <= 5 {
		return true
	}
	return strings.HasPrefix(normalized, "page") || strings.HasPrefix(normalized, "archivep")
}

func looksMostlyNumericComparable(normalized string) bool {
	if normalized == "" {
		return false
	}
	digits := 0
	for _, r := range normalized {
		if unicode.IsDigit(r) {
			digits++
		}
	}
	return digits*2 >= len([]rune(normalized))
}

func residualLineLooksStrong(rawLine string, normalized string) bool {
	rawLower := strings.ToLower(strings.TrimSpace(rawLine))
	if rawLower == "" || normalized == "" {
		return false
	}
	if residualStrongKeywordRE.MatchString(rawLower) {
		return true
	}
	if residualLongDigitRE.MatchString(rawLower) && !looksMostlyNumericComparable(normalized) {
		return true
	}
	return false
}

func chunkPageIndex(c map[string]any) int {
	return intFromAny(c["page_index"])
}

func chunkOrder(c map[string]any) (int, bool) {
	if c == nil {
		return 0, false
	}
	o := intFromAny(c["order"])
	return o, true
}

// RebuildTextSummaryAfterVLMMerge rebuilds document.text_summary.combined_text
// from merged chunk text while preserving native_combined_text.
func RebuildTextSummaryAfterVLMMerge(doc map[string]any, merged []map[string]any, recognitionSource ...string) {
	ts, ok := doc["text_summary"].(map[string]any)
	if !ok {
		return
	}
	if _, exists := ts["native_combined_text"]; !exists {
		if nat, ok := ts["combined_text"].(string); ok {
			ts["native_combined_text"] = nat
		}
	}
	lines := chunkItemsToLines(merged)
	combined, trunc := joinSegmentLinesTruncated(lines, maxFullDocumentCombinedTextBytes)
	ts["combined_text"] = combined
	if trunc {
		ts["combined_text_truncated"] = true
	} else {
		delete(ts, "combined_text_truncated")
	}
	source := "vlm"
	if len(recognitionSource) > 0 {
		if s := strings.TrimSpace(recognitionSource[0]); s != "" {
			source = s
		}
	}
	ts["recognition_source"] = source
	log.Printf("[vlm_chunk] text_summary rebuilt combined_bytes=%d truncated=%v source_lines=%d",
		len(combined), trunc, len(lines))
}

func chunkItemsToLines(items []map[string]any) []string {
	if len(items) == 0 {
		return nil
	}
	sorted := make([]map[string]any, len(items))
	copy(sorted, items)
	sort.SliceStable(sorted, func(i, j int) bool {
		pi := chunkPageIndex(sorted[i])
		pj := chunkPageIndex(sorted[j])
		if pi != pj {
			return pi < pj
		}
		oi, _ := chunkOrder(sorted[i])
		oj, _ := chunkOrder(sorted[j])
		return oi < oj
	})
	var out []string
	for _, c := range sorted {
		t := strings.TrimSpace(fmt.Sprint(c["text"]))
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}
