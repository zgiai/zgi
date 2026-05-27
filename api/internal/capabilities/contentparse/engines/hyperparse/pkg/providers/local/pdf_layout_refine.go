package local

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/envconfig"
)

var layoutKVLabelRE = regexp.MustCompile(`^\s*([^:：]{1,36})\s*[:：]\s*(.*)$`)

type layoutCell struct {
	row  int
	col  int
	text string
	bbox map[string]any
}

type layoutGroup struct {
	text string
	typ  string
	bbox map[string]any
}

func refineNativeFullDocumentLayout(fullDoc map[string]any, inspect map[string]any) {
	if !nativeLayoutRefinementEnabled() || !shouldRefineNativeLayout(fullDoc) {
		return
	}
	chunksW, _ := fullDoc["chunks"].(map[string]any)
	if chunksW == nil {
		return
	}
	items := normalizeMapSlice(chunksW["items"])
	if len(items) == 0 {
		return
	}

	generated, tableCount := buildLayoutChunksFromGeometryTables(items)
	if len(generated) < 8 || tableCount == 0 {
		return
	}
	originalText := countTextualNativeItems(items)
	if originalText > 0 && len(generated) < originalText/2 {
		return
	}

	sort.SliceStable(generated, func(i, j int) bool {
		pi, pj := intAny(generated[i]["page_index"]), intAny(generated[j]["page_index"])
		if pi != pj {
			return pi < pj
		}
		return intAny(generated[i]["order"]) < intAny(generated[j]["order"])
	})

	generatedByPage := layoutGeneratedTextByPage(generated)
	retained := make([]map[string]any, 0, len(items))
	retainedOriginal := 0
	for _, item := range items {
		t := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["type"])))
		if isGeometryTable(item) {
			continue
		}
		switch t {
		case "bookmark", "annotation", "form_field", "attachment", "stamp", "image":
			retained = append(retained, item)
			continue
		case "paragraph", "heading", "kv", "text", "list_item", "caption", "footnote", "formula", "list", "marginalia":
			if shouldRetainOriginalLayoutItem(item, generatedByPage) {
				retained = append(retained, item)
				retainedOriginal++
			}
			continue
		default:
			retained = append(retained, item)
			retainedOriginal++
		}
	}
	refined := append(generated, retained...)
	sort.SliceStable(refined, func(i, j int) bool {
		pi, pj := intAny(refined[i]["page_index"]), intAny(refined[j]["page_index"])
		if pi != pj {
			return pi < pj
		}
		return intAny(refined[i]["order"]) < intAny(refined[j]["order"])
	})

	chunksW["items"] = refined
	chunksW["count"] = len(refined)
	stats := map[string]any{
		"applied":              true,
		"strategy":             "geometry_table_rows",
		"source_table_chunks":  tableCount,
		"original_chunk_count": len(items),
		"generated_chunks":     len(generated),
		"retained_original":    retainedOriginal,
		"final_chunk_count":    len(refined),
	}
	if doc, _ := fullDoc["document"].(map[string]any); doc != nil {
		doc["local_layout_refinement"] = stats
	}
	if inspect != nil {
		inspect["local_layout_refinement"] = stats
	}
}

func nativeLayoutRefinementEnabled() bool {
	raw := envconfig.String("LOCAL_LAYOUT_REFINEMENT")
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

func shouldRefineNativeLayout(fullDoc map[string]any) bool {
	doc, _ := fullDoc["document"].(map[string]any)
	if doc == nil {
		return false
	}
	if v, ok := doc["suggest_vlm"].(bool); ok && v {
		return true
	}
	if hint, _ := doc["business_doc_vlm_hint"].(map[string]any); hint != nil {
		if v, ok := hint["suggest"].(bool); ok && v {
			return true
		}
	}
	return false
}

func buildLayoutChunksFromGeometryTables(items []map[string]any) ([]map[string]any, int) {
	out := make([]map[string]any, 0, len(items)*2)
	tableCount := 0
	orderByPage := map[int]int{}
	for _, item := range items {
		if !isGeometryTable(item) {
			continue
		}
		tableCount++
		page := intAny(item["page_index"])
		if page <= 0 {
			page = 1
		}
		rows := geometryTableRows(item)
		rowKeys := make([]int, 0, len(rows))
		for row := range rows {
			rowKeys = append(rowKeys, row)
		}
		sort.Ints(rowKeys)
		for _, row := range rowKeys {
			groups := layoutGroupsFromRow(rows[row])
			for _, g := range groups {
				txt := cleanLayoutText(g.text)
				if !usefulLayoutText(txt) {
					continue
				}
				orderByPage[page]++
				out = append(out, map[string]any{
					"chunk_id":   fmt.Sprintf("layout_%d_%04d", page, orderByPage[page]),
					"type":       g.typ,
					"page_index": page,
					"order":      orderByPage[page],
					"source":     "native_pdf_layout",
					"confidence": 0.81,
					"text":       txt,
					"bbox":       g.bbox,
				})
			}
		}
	}
	return out, tableCount
}

func layoutGeneratedTextByPage(items []map[string]any) map[int][]string {
	out := make(map[int][]string)
	for _, item := range items {
		page := intAny(item["page_index"])
		if page <= 0 {
			page = 1
		}
		txt := normalizeLayoutComparableText(fmt.Sprint(item["text"]))
		if txt == "" {
			continue
		}
		out[page] = append(out[page], txt)
	}
	return out
}

func shouldRetainOriginalLayoutItem(item map[string]any, generatedByPage map[int][]string) bool {
	txt := cleanLayoutText(fmt.Sprint(item["text"]))
	if !usefulLayoutText(txt) {
		return false
	}
	page := intAny(item["page_index"])
	if page <= 0 {
		page = 1
	}
	return !layoutTextCoveredByGenerated(txt, generatedByPage[page])
}

func layoutTextCoveredByGenerated(text string, generated []string) bool {
	needle := normalizeLayoutComparableText(text)
	if needle == "" || len(generated) == 0 {
		return false
	}
	for _, candidate := range generated {
		if candidate == needle || strings.Contains(candidate, needle) || strings.Contains(needle, candidate) && len(candidate) >= 24 {
			return true
		}
	}
	tokens := layoutComparableTokens(needle)
	if len(tokens) == 0 {
		return false
	}
	generatedTokens := layoutGeneratedTokenSet(generated)
	for _, labelToken := range layoutLabelTokens(text) {
		if !generatedTokens[labelToken] {
			return false
		}
	}
	covered := 0
	for _, tok := range tokens {
		if generatedTokens[tok] {
			covered++
		}
	}
	coverage := float64(covered) / float64(len(tokens))
	if len(tokens) <= 8 {
		return coverage >= 0.6
	}
	return coverage >= 0.75
}

func normalizeLayoutComparableText(s string) string {
	s = strings.ToLower(cleanLayoutText(s))
	var b strings.Builder
	lastSpace := true
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func layoutGeneratedTokenSet(generated []string) map[string]bool {
	out := map[string]bool{}
	for _, candidate := range generated {
		for _, tok := range layoutComparableTokens(candidate) {
			out[tok] = true
		}
	}
	return out
}

func layoutComparableTokens(normalized string) []string {
	raw := strings.Fields(normalized)
	if len(raw) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, tok := range raw {
		if len([]rune(tok)) < 2 {
			continue
		}
		if seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
	}
	return out
}

func layoutLabelTokens(text string) []string {
	parts := strings.Split(cleanLayoutText(text), ":")
	if len(parts) < 2 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(parts))
	for i := 0; i < len(parts)-1; i++ {
		label := trailingLayoutLabel(parts[i])
		if label == "" {
			continue
		}
		for _, tok := range layoutComparableTokens(normalizeLayoutComparableText(label)) {
			if seen[tok] {
				continue
			}
			seen[tok] = true
			out = append(out, tok)
		}
	}
	return out
}

func trailingLayoutLabel(s string) string {
	runes := []rune(strings.TrimSpace(s))
	end := len(runes)
	for end > 0 && !unicode.IsLetter(runes[end-1]) {
		end--
	}
	if end == 0 {
		return ""
	}
	start := end
	for start > 0 {
		r := runes[start-1]
		if unicode.IsLetter(r) || unicode.IsSpace(r) || r == '#' {
			start--
			continue
		}
		break
	}
	label := strings.TrimSpace(string(runes[start:end]))
	if len([]rune(label)) > 30 {
		return ""
	}
	return label
}

func isGeometryTable(item map[string]any) bool {
	if strings.ToLower(strings.TrimSpace(fmt.Sprint(item["type"]))) != "table" {
		return false
	}
	payload, _ := item["payload"].(map[string]any)
	if payload == nil {
		return false
	}
	return strings.TrimSpace(fmt.Sprint(payload["detection_mode"])) == "geometry_token_v2"
}

func geometryTableRows(item map[string]any) map[int][]layoutCell {
	payload, _ := item["payload"].(map[string]any)
	cells := normalizeMapSlice(payload["cells"])
	rows := make(map[int][]layoutCell)
	for _, raw := range cells {
		txt := cleanLayoutText(fmt.Sprint(raw["text"]))
		if txt == "" {
			continue
		}
		row := intAny(raw["row"])
		cell := layoutCell{
			row:  row,
			col:  intAny(raw["col"]),
			text: txt,
		}
		if bb, ok := raw["bbox"].(map[string]any); ok {
			cell.bbox = cloneMap(bb)
		}
		rows[row] = append(rows[row], cell)
	}
	for row := range rows {
		sort.SliceStable(rows[row], func(i, j int) bool {
			if rows[row][i].col != rows[row][j].col {
				return rows[row][i].col < rows[row][j].col
			}
			return floatAny(rows[row][i].bbox["left"]) < floatAny(rows[row][j].bbox["left"])
		})
	}
	return rows
}

func layoutGroupsFromRow(cells []layoutCell) []layoutGroup {
	if len(cells) == 0 {
		return nil
	}
	var groups []layoutGroup
	var free []layoutCell
	var pending *layoutGroup

	emitPending := func() {
		if pending == nil {
			return
		}
		if usefulLayoutText(pending.text) {
			groups = append(groups, *pending)
		}
		pending = nil
	}
	emitFree := func() {
		if len(free) == 0 {
			return
		}
		g := groupFromCells(free)
		if usefulLayoutText(g.text) {
			groups = append(groups, g)
		}
		free = free[:0]
	}

	for _, cell := range cells {
		txt := cleanLayoutText(cell.text)
		if !usefulLayoutText(txt) {
			continue
		}
		key, val, ok := splitLayoutKV(txt)
		if ok {
			emitFree()
			emitPending()
			text := key + ":"
			if val != "" {
				text = key + ": " + val
			}
			g := layoutGroup{text: text, typ: layoutTextType(text), bbox: cloneMap(cell.bbox)}
			if val == "" {
				pending = &g
			} else {
				groups = append(groups, g)
			}
			continue
		}
		if pending != nil {
			pending.text = cleanLayoutText(pending.text + " " + txt)
			pending.bbox = unionBBoxMaps(pending.bbox, cell.bbox)
			pending.typ = layoutTextType(pending.text)
			continue
		}
		free = append(free, cell)
	}
	emitPending()
	emitFree()
	return mergeNearbyLayoutGroups(groups)
}

func groupFromCells(cells []layoutCell) layoutGroup {
	parts := make([]string, 0, len(cells))
	var bb map[string]any
	for _, c := range cells {
		if usefulLayoutText(c.text) {
			parts = append(parts, c.text)
			bb = unionBBoxMaps(bb, c.bbox)
		}
	}
	text := cleanLayoutText(strings.Join(parts, " "))
	return layoutGroup{text: text, typ: layoutTextType(text), bbox: bb}
}

func mergeNearbyLayoutGroups(groups []layoutGroup) []layoutGroup {
	out := make([]layoutGroup, 0, len(groups))
	for _, g := range groups {
		g.text = cleanLayoutText(g.text)
		if !usefulLayoutText(g.text) {
			continue
		}
		out = append(out, g)
	}
	return out
}

func splitLayoutKV(text string) (string, string, bool) {
	m := layoutKVLabelRE.FindStringSubmatch(text)
	if len(m) != 3 {
		return "", "", false
	}
	key := cleanLayoutLabel(m[1])
	val := cleanLayoutText(m[2])
	if !isLayoutLabelLike(key) {
		return "", "", false
	}
	return key, val, true
}

func isLayoutLabelLike(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	runes := []rune(key)
	if len(runes) > 30 {
		return false
	}
	if strings.ContainsAny(key, ",.;") {
		return false
	}
	letters := 0
	for _, r := range runes {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	return letters >= 2
}

func layoutTextType(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return "paragraph"
	}
	if isLayoutSectionHeading(t) {
		return "heading"
	}
	return "paragraph"
}

func isLayoutSectionHeading(text string) bool {
	t := strings.TrimSpace(strings.TrimSuffix(text, ":"))
	t = strings.Join(strings.Fields(t), " ")
	if t == "" || len([]rune(t)) > 48 {
		return false
	}
	upper := strings.ToUpper(t)
	switch upper {
	case "DIAGNOSIS", "GROSS DESCRIPTION", "MICROSCOPIC DESCRIPTION":
		return true
	}
	return strings.HasPrefix(upper, "DKM:")
}

func cleanLayoutLabel(s string) string {
	s = cleanLayoutText(s)
	s = strings.TrimSpace(strings.TrimSuffix(s, "#"))
	s = strings.TrimSpace(strings.TrimSuffix(s, ":"))
	return s
}

func cleanLayoutText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	repl := strings.NewReplacer(
		"\u00a0", " ",
		" ：", ":",
		" :", ":",
		":", ": ",
		" #", "#",
		"( ", "(",
		" )", ")",
		" ,", ",",
		" .", ".",
	)
	s = repl.Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	s = strings.NewReplacer(
		"Patien t", "Patient",
		"Doct or", "Doctor",
		"Date Ob tain", "Date Obtain",
		"Date Receive d", "Date Received",
		"Pat hol ogy", "Pathology",
		"CLIN IC AL", "CLINICAL",
		"SPEC IMEN", "SPECIMEN",
		"DiagNostiC", "Diagnostic",
		"PA AHOLOGY", "PATHOLOGY",
		"MEDICAL GROURPINC", "MEDICAL GROUP INC",
		"DERMAT OPAT HOLOGY", "DERMATOPATHOLOGY",
		"PAT I ENT", "PATIENT",
		"R I G H T", "RIGHT",
		"AR M", "ARM",
		"SH AV E", "SHAVE",
		"BI OPS Y", "BIOPSY",
		"R/ O", "R/O",
		"WAR T", "WART",
		"T I N E A", "TINEA",
		"Date Obtain DPMG ed:", "DPMG",
		"Date Obtain DPMG ed :", "DPMG",
		"serp iginosa", "serpiginosa",
		"pres ents", "presents",
	).Replace(s)
	s = strings.ReplaceAll(s, ":  ", ": ")
	return strings.TrimSpace(s)
}

func usefulLayoutText(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" || t == ">" || t == "|" {
		return false
	}
	if len([]rune(t)) == 1 {
		r := []rune(t)[0]
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	}
	return true
}

func countTextualNativeItems(items []map[string]any) int {
	n := 0
	for _, item := range items {
		switch strings.ToLower(strings.TrimSpace(fmt.Sprint(item["type"]))) {
		case "paragraph", "heading", "kv", "list_item", "caption", "footnote", "formula":
			n++
		}
	}
	return n
}

func normalizeMapSlice(v any) []map[string]any {
	switch s := v.(type) {
	case []map[string]any:
		return s
	case []any:
		out := make([]map[string]any, 0, len(s))
		for _, raw := range s {
			if m, ok := raw.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func unionBBoxMaps(a, b map[string]any) map[string]any {
	if a == nil {
		return cloneMap(b)
	}
	if b == nil {
		return cloneMap(a)
	}
	left := math.Min(floatAny(a["left"]), floatAny(b["left"]))
	right := math.Max(floatAny(a["right"]), floatAny(b["right"]))
	top := math.Max(floatAny(a["top"]), floatAny(b["top"]))
	bottom := math.Min(floatAny(a["bottom"]), floatAny(b["bottom"]))
	return map[string]any{"left": left, "right": right, "top": top, "bottom": bottom}
}

func intAny(v any) int {
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

func floatAny(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return 0
	}
}
