// Package export provides DPT-style export objects for RAG and downstream retrieval.
// Full visual descriptions and cell-level grounding need the complete VLM layout
// pipeline; this package maps from Hyper Parse full_document on a best-effort basis.
package export

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/core/chunking"
)

type dptBuilt struct {
	id         string
	markdown   string
	dptType    string
	subtype    string
	srcType    string // full_document chunk type, used for section splitting
	page0      int
	grounding  map[string]any
	payload    map[string]any
	cellRefs   map[string]map[string]any
	provenance map[string]any
}

// BuildDPTExportFromFullDocument builds a DPT-style object from ParseFullDocument:
// markdown, chunks, splits, grounding, and metadata.
func BuildDPTExportFromFullDocument(fullDoc map[string]any, filename string, pageCount int) map[string]any {
	items := chunkItemsFromFullDoc(fullDoc)
	if len(items) == 0 {
		return map[string]any{
			"markdown":        "",
			"chunks":          []any{},
			"splits":          []any{},
			"splits_sections": []any{},
			"rag": map[string]any{
				"embedding_items": []any{},
			},
			"grounding":   map[string]any{},
			"metadata":    dptMetadata(filename, pageCount),
			"_hyperparse": map[string]any{"exporter": "dpt_v1", "note": "no chunks in full_document"},
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		pi, pj := chunkPageIndex(items[i]), chunkPageIndex(items[j])
		if pi != pj {
			return pi < pj
		}
		return chunkOrder(items[i]) < chunkOrder(items[j])
	})

	out := make([]dptBuilt, 0, len(items))
	groundingAll := make(map[string]any)

	for _, c := range items {
		if skipChunkType(fmt.Sprint(c["type"])) {
			continue
		}
		srcType := strings.ToLower(strings.TrimSpace(fmt.Sprint(c["type"])))
		id := newUUIDv4()
		typ, sub := mapDPTChunkType(c)
		page0 := chunkPageIndex(c) - 1
		if page0 < 0 {
			page0 = 0
		}
		body := dptChunkBody(c, typ)
		var cellRefs map[string]map[string]any
		if typ == "table" {
			if p, ok := c["payload"].(map[string]any); ok {
				html, refs := chunking.HTMLTableWithIDsFromPayload(p, page0)
				if html != "" {
					body = html
					cellRefs = refs
				}
			}
		}
		md := fmt.Sprintf("<a id='%s'></a>\n\n%s", id, body)
		g := chunkGrounding(c)
		prov := chunkProvenance(c)
		b := dptBuilt{
			id:         id,
			markdown:   md,
			dptType:    typ,
			subtype:    sub,
			srcType:    srcType,
			page0:      page0,
			grounding:  g,
			payload:    clonePayloadMap(c["payload"]),
			cellRefs:   cellRefs,
			provenance: prov,
		}
		out = append(out, b)
		if g != nil {
			gm := map[string]any{
				"box":  g["box"],
				"page": g["page"],
				"type": mapGroundingLabel(typ),
			}
			if conf, ok := g["confidence"]; ok {
				gm["confidence"] = conf
			}
			groundingAll[id] = gm
		}
		for cid, cg := range cellRefs {
			box := canonicalGroundingBoxAny(cg["box"])
			if box == nil {
				if raw, ok := cg["box"].(map[string]any); ok {
					box = raw
				}
			}
			groundingAll[cid] = map[string]any{
				"box":  box,
				"page": cg["page"],
				"type": "tableCell",
			}
		}
	}

	// Guardrail: collect chunk bboxes, count sharing, and mark invalid or
	// heavily shared boxes unreliable so bad coordinates are not sent to UI.
	boxByChunk := make(map[string]map[string]any, len(out))
	for _, b := range out {
		if b.grounding == nil {
			continue
		}
		if box, ok := b.grounding["box"].(map[string]any); ok {
			boxByChunk[b.id] = box
		}
	}
	reliabilityReport, unreliable := ClassifyBBoxReliability(boxByChunk)

	chunksArr := make([]any, 0, len(out))
	var mdParts []string
	for _, b := range out {
		cm := map[string]any{
			"markdown": b.markdown,
			"type":     b.dptType,
			"id":       b.id,
		}
		if b.subtype != "" {
			cm["subtype"] = b.subtype
		}
		if len(b.payload) > 0 {
			cm["payload"] = b.payload
		}
		if b.grounding != nil {
			g := map[string]any{"page": b.grounding["page"]}
			if unreliable[b.id] {
				// Keep markdown/text and hide only unreliable bbox so UI can skip the overlay.
				g["precision"] = "unreliable"
			} else {
				g["box"] = b.grounding["box"]
				g["precision"] = "reliable"
			}
			if conf, ok := b.grounding["confidence"]; ok {
				g["confidence"] = conf
			}
			if spans, ok := b.grounding["low_confidence_spans"]; ok {
				g["low_confidence_spans"] = spans
			}
			cm["grounding"] = g
		}
		if len(b.provenance) > 0 {
			cm["provenance"] = b.provenance
			if source, ok := b.provenance["source"].(string); ok && source != "" {
				cm["source"] = source
			}
			if stage, ok := b.provenance["stage"].(string); ok && stage != "" {
				cm["pipeline_stage"] = stage
			}
			if trace, ok := b.provenance["source_trace"].(string); ok && trace != "" {
				cm["source_trace"] = trace
			}
			if confidence, ok := b.provenance["confidence"].(float64); ok {
				cm["confidence"] = confidence
			}
		}
		chunksArr = append(chunksArr, cm)
		mdParts = append(mdParts, b.markdown)
	}

	// Keep global grounding consistent by removing unreliable chunk boxes.
	for id := range unreliable {
		if entry, ok := groundingAll[id].(map[string]any); ok {
			delete(entry, "box")
			entry["precision"] = "unreliable"
			groundingAll[id] = entry
		}
	}

	splits := buildSplits(out)
	sectionSplits, secMode := buildSectionSplits(out)
	secIdx, secTitle, secIdent := assignSectionMeta(out, secMode)
	ragItems := buildRAGEmbeddingItems(out, secIdx, secTitle, secIdent)

	return map[string]any{
		"markdown":        strings.Join(mdParts, "\n\n"),
		"chunks":          chunksArr,
		"splits":          splits,
		"splits_sections": sectionSplits,
		"rag": map[string]any{
			"embedding_items": ragItems,
		},
		"grounding": groundingAll,
		"metadata":  dptMetadata(filename, pageCount),
		"_hyperparse": map[string]any{
			"exporter":            "dpt_v1",
			"chunk_count":         len(out),
			"source_model":        "full_document",
			"section_split_mode":  secMode,
			"section_split_count": len(sectionSplits),
			"rag_embedding_count": len(ragItems),
			"bbox_reliability":    reliabilityReport,
			"bbox_reliable_ratio": reliabilityReportRatio(reliabilityReport),
			"bbox_downgraded_ids": reliabilityReport.Downgraded,
		},
	}
}

func reliabilityReportRatio(r BBoxReliabilityReport) float64 {
	if r.TotalChunks == 0 {
		return 1.0
	}
	return float64(r.GeomReliable) / float64(r.TotalChunks)
}

func dptMetadata(filename string, pageCount int) map[string]any {
	return map[string]any{
		"filename":     filename,
		"org_id":       nil,
		"page_count":   pageCount,
		"duration_ms":  nil,
		"credit_usage": nil,
		"job_id":       nil,
		"version":      "hyperparse-dpt-export-v1",
	}
}

func chunkItemsFromFullDoc(fd map[string]any) []map[string]any {
	ch, ok := fd["chunks"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := ch["items"]
	if !ok {
		return nil
	}
	switch x := raw.(type) {
	case []map[string]any:
		return x
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, it := range x {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func clonePayloadMap(v any) map[string]any {
	in, ok := v.(map[string]any)
	if !ok || len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, val := range in {
		out[key] = val
	}
	return out
}

func skipChunkType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "table_debug":
		return true
	default:
		return false
	}
}

func mapDPTChunkType(c map[string]any) (string, string) {
	t := strings.ToLower(strings.TrimSpace(fmt.Sprint(c["type"])))
	switch t {
	case "table":
		return "table", ""
	case "image":
		return "figure", ""
	case "stamp":
		return "figure", "stamp"
	case "bookmark", "annotation", "form_field", "attachment":
		return "marginalia", ""
	case "paragraph", "heading", "list_item", "caption", "footnote", "formula", "list":
		txt := dptCleanText(c["text"])
		if isLikelyPageNumber(txt) {
			return "marginalia", "page_number"
		}
		if isLikelyLogoText(txt, c) {
			return "logo", ""
		}
		if isLikelyMarginaliaText(txt, c) {
			return "marginalia", ""
		}
		return "text", ""
	default:
		return "text", ""
	}
}

func mapGroundingLabel(dptChunkType string) string {
	switch dptChunkType {
	case "table":
		return "chunkTable"
	case "figure":
		return "chunkFigure"
	case "logo":
		return "chunkLogo"
	case "marginalia":
		return "chunkMarginalia"
	default:
		return "chunkText"
	}
}

var pageNumberLikeRE = regexp.MustCompile(`(?i)^\s*(page\s+\d+\s*(of|/)\s*\d+|\d{1,3})\s*$`)

// Markdown heading at the first line of a chunk.
var mdHeadingLineRE = regexp.MustCompile(`^\s{0,3}#{1,6}\s+\S`)

// Strip simple HTML tags before vector embedding.
var htmlTagStripRE = regexp.MustCompile(`<[^>]+>`)

// extraSectionRE reads an optional Go regexp for additional section starts.
func extraSectionRE() *regexp.Regexp {
	pat := strings.TrimSpace(os.Getenv("CONTENT_PARSE_DPT_SECTION_LINE_RE"))
	if pat == "" {
		pat = strings.TrimSpace(os.Getenv("DOCSTILL_DPT_SECTION_LINE_RE"))
	}
	if pat == "" {
		return nil
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil
	}
	return re
}

func isLikelyPageNumber(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	return pageNumberLikeRE.MatchString(text)
}

func isLikelyLogoText(text string, c map[string]any) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	// Prefer header regions that look like organization marks or names.
	_, top, _, bottom, ok := chunkBBox(c)
	if !ok {
		return false
	}
	if !(top <= 0.22 && bottom <= 0.35) {
		return false
	}
	if strings.Contains(lower, "logo") {
		return true
	}
	// Combine organization keywords with uppercase names to avoid treating body headings as logos.
	hasOrgWord := strings.Contains(lower, "inc") ||
		strings.Contains(lower, "corp") ||
		strings.Contains(lower, "company") ||
		strings.Contains(lower, "group") ||
		strings.Contains(lower, "insurance")
	upper := strings.ToUpper(text)
	return hasOrgWord && upper == text
}

func isLikelyMarginaliaText(text string, c map[string]any) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	left, top, right, bottom, ok := chunkBBox(c)
	if !ok {
		return false
	}
	_ = top
	width := right - left
	// Narrow footer blocks are usually margin notes, signatures, or page numbers.
	return bottom >= 0.90 && width <= 0.40 && len([]rune(text)) <= 96
}

func chunkBBox(c map[string]any) (left, top, right, bottom float64, ok bool) {
	bb, has := c["bbox"].(map[string]any)
	if !has || len(bb) == 0 {
		return 0, 0, 0, 0, false
	}
	left = toF64(bb["left"])
	top = toF64(bb["top"])
	right = toF64(bb["right"])
	bottom = toF64(bb["bottom"])
	if left == 0 && top == 0 && right == 0 && bottom == 0 {
		return 0, 0, 0, 0, false
	}
	return left, top, right, bottom, true
}

func chunkProvenance(c map[string]any) map[string]any {
	out := map[string]any{}
	if existing, ok := c["provenance"].(map[string]any); ok {
		for k, v := range existing {
			out[k] = v
		}
	}
	rawSource := strings.ToLower(strings.TrimSpace(fmt.Sprint(c["source"])))
	source := normalizeChunkSource(rawSource)
	if source != "" {
		out["source"] = source
	}
	if stage := chunkPipelineStage(c, rawSource, source); stage != "" {
		out["stage"] = stage
	}
	if method := chunkPipelineMethod(c, rawSource, source); method != "" {
		out["method"] = method
	}
	if trace := strings.TrimSpace(fmt.Sprint(c["source_trace"])); trace != "" {
		out["source_trace"] = trace
	}
	if conf, ok := toFloat64(c["confidence"]); ok {
		out["confidence"] = clamp01(conf)
	}
	if payload, ok := c["payload"].(map[string]any); ok {
		if model := firstNonEmptyString(payload["vlm_caption_model"], payload["model"]); model != "" {
			out["model"] = model
		}
		var flags []string
		if boolFromAny(payload["table_repair"]) {
			flags = append(flags, "table_repair")
		}
		if boolFromAny(payload["vlm_image_caption"]) || firstNonEmptyString(payload["vlm_caption"]) != "" {
			flags = append(flags, "image_caption")
		}
		if repair := firstNonEmptyString(payload["repair"]); repair != "" {
			flags = append(flags, repair)
		}
		if len(flags) > 0 {
			out["flags"] = flags
		}
	}
	return out
}

func normalizeChunkSource(raw string) string {
	switch {
	case raw == "":
		return "native"
	case strings.Contains(raw, "vlm"):
		return "vlm"
	case strings.Contains(raw, "ocr"):
		return "ocr"
	case strings.Contains(raw, "repair"):
		return "repaired"
	case strings.Contains(raw, "native"), strings.Contains(raw, "pdf"):
		return "native"
	default:
		return raw
	}
}

func chunkPipelineStage(c map[string]any, rawSource, source string) string {
	payload, _ := c["payload"].(map[string]any)
	if payload != nil {
		if firstNonEmptyString(payload["vlm_caption"]) != "" || boolFromAny(payload["vlm_image_caption"]) {
			return "image_caption"
		}
		if boolFromAny(payload["regional_fallback"]) || strings.Contains(firstNonEmptyString(payload["repair"]), "regional") {
			return "regional_ocr_vlm"
		}
		if firstNonEmptyString(payload["repair"]) != "" || boolFromAny(payload["table_repair"]) {
			if source == "ocr" {
				return "ocr_layout_repair"
			}
			return "bbox_alignment_repair"
		}
	}
	if strings.Contains(rawSource, "regional") {
		return "regional_ocr_vlm"
	}
	switch source {
	case "vlm":
		return "vlm_fallback"
	case "ocr":
		return "ocr_layout_repair"
	case "repaired":
		return "bbox_alignment_repair"
	}
	t := strings.ToLower(strings.TrimSpace(fmt.Sprint(c["type"])))
	if t == "image" || t == "stamp" || strings.Contains(rawSource, "image") {
		return "image_detection"
	}
	return "native_text"
}

func chunkPipelineMethod(c map[string]any, rawSource, source string) string {
	payload, _ := c["payload"].(map[string]any)
	if payload != nil {
		if engine := firstNonEmptyString(payload["ocr_engine"]); engine != "" {
			return engine
		}
		if firstNonEmptyString(payload["vlm_caption"]) != "" {
			mode := firstNonEmptyString(payload["vlm_caption_mode"])
			if mode != "" {
				return "vlm_" + mode
			}
			return "vlm_caption"
		}
		if strategy := firstNonEmptyString(payload["strategy"]); strategy != "" {
			return strategy
		}
	}
	if rawSource != "" {
		return rawSource
	}
	return source
}

func chunkGrounding(c map[string]any) map[string]any {
	bb, ok := c["bbox"].(map[string]any)
	if !ok || len(bb) == 0 {
		return nil
	}
	box := canonicalGroundingBox(bb)
	if box == nil {
		return nil
	}
	pi := chunkPageIndex(c)
	if pi < 1 {
		pi = 1
	}
	out := map[string]any{
		"box":  box,
		"page": pi - 1,
	}
	if conf, ok := toFloat64(c["confidence"]); ok {
		conf = clamp01(conf)
		out["confidence"] = conf
		if conf > 0 && conf < 0.78 {
			out["low_confidence_spans"] = []map[string]any{
				{
					"text":       truncateRunes(dptCleanText(c["text"]), 80),
					"confidence": conf,
				},
			}
		}
	}
	return out
}

func canonicalGroundingBoxAny(v any) map[string]any {
	bb, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return canonicalGroundingBox(bb)
}

func canonicalGroundingBox(bb map[string]any) map[string]any {
	if len(bb) == 0 {
		return nil
	}
	l := toF64(bb["left"])
	r := toF64(bb["right"])
	t := toF64(bb["top"])
	b := toF64(bb["bottom"])
	if l == 0 && r == 0 && t == 0 && b == 0 {
		return nil
	}
	if r < l {
		l, r = r, l
	}
	top := t
	bottom := b
	// native bbox historically used bottom-origin semantics with top > bottom;
	// VLM bbox already uses UI-friendly top-origin semantics with top < bottom.
	if t > b {
		top = 1 - t
		bottom = 1 - b
	}
	if bottom < top {
		top, bottom = bottom, top
	}
	top = clamp01(top)
	bottom = clamp01(bottom)
	width := math.Max(0, r-l)
	height := math.Max(0, bottom-top)
	return map[string]any{
		"left":   l,
		"right":  r,
		"top":    top,
		"bottom": bottom,
		"width":  width,
		"height": height,
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

func stripFirstAnchor(s string) string {
	s = strings.TrimSpace(s)
	low := strings.ToLower(s)
	idx := strings.Index(low, "</a>")
	if idx < 0 {
		return s
	}
	return strings.TrimSpace(s[idx+4:])
}

func isSectionBreak(b dptBuilt) bool {
	if b.srcType == "heading" {
		return true
	}
	rest := stripFirstAnchor(b.markdown)
	for _, line := range strings.Split(rest, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if mdHeadingLineRE.MatchString(t) {
			return true
		}
		if re := extraSectionRE(); re != nil && re.MatchString(t) {
			return true
		}
		return false
	}
	return false
}

func sectionTitleFromFirstHeading(b dptBuilt) string {
	rest := stripFirstAnchor(b.markdown)
	for _, line := range strings.Split(rest, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if mdHeadingLineRE.MatchString(t) {
			t = strings.TrimLeft(t, "#")
			return strings.TrimSpace(t)
		}
		if re := extraSectionRE(); re != nil && re.MatchString(t) {
			if m := re.FindStringSubmatch(t); len(m) > 1 && strings.TrimSpace(m[1]) != "" {
				return strings.TrimSpace(m[1])
			}
			return strings.TrimSpace(re.FindString(t))
		}
		if b.srcType == "heading" {
			return t
		}
		return ""
	}
	return ""
}

func sectionSplitEntry(seq int, parts []dptBuilt) map[string]any {
	pageSet := map[int]struct{}{}
	ids := make([]any, 0, len(parts))
	mds := make([]string, 0, len(parts))
	for _, p := range parts {
		pageSet[p.page0] = struct{}{}
		ids = append(ids, p.id)
		mds = append(mds, p.markdown)
	}
	pages := make([]int, 0, len(pageSet))
	for p := range pageSet {
		pages = append(pages, p)
	}
	sort.Ints(pages)
	title := ""
	if len(parts) > 0 {
		title = sectionTitleFromFirstHeading(parts[0])
	}
	return map[string]any{
		"class_":     "section",
		"identifier": fmt.Sprintf("section_%d", seq),
		"title":      title,
		"pages":      pages,
		"markdown":   strings.Join(mds, "\n\n"),
		"chunks":     ids,
	}
}

func detectSectionMode(out []dptBuilt) string {
	if len(out) == 0 {
		return "none"
	}
	for _, b := range out {
		if isSectionBreak(b) {
			return "heading_markdown"
		}
	}
	return "fallback_by_page"
}

func buildHeadingSectionSplits(out []dptBuilt) []any {
	res := make([]any, 0, 4)
	cur := make([]dptBuilt, 0, 16)
	seq := 0
	flush := func() {
		if len(cur) == 0 {
			return
		}
		res = append(res, sectionSplitEntry(seq, cur))
		seq++
		cur = cur[:0]
	}
	for _, b := range out {
		if isSectionBreak(b) && len(cur) > 0 {
			flush()
		}
		cur = append(cur, b)
	}
	flush()
	return res
}

// buildSectionSplits creates secondary splits on headings, Markdown headings,
// or optional regex matches; otherwise it keeps one split per page.
func buildSectionSplits(out []dptBuilt) ([]any, string) {
	if len(out) == 0 {
		return nil, "none"
	}
	mode := detectSectionMode(out)
	if mode == "fallback_by_page" {
		return buildPageSectionsAsFallback(out), mode
	}
	return buildHeadingSectionSplits(out), "heading_markdown"
}

func assignSectionMeta(out []dptBuilt, mode string) (idx []int, title []string, ident []string) {
	n := len(out)
	idx = make([]int, n)
	title = make([]string, n)
	ident = make([]string, n)
	if n == 0 {
		return
	}
	if mode == "fallback_by_page" || mode == "none" {
		for i, b := range out {
			idx[i] = b.page0
			title[i] = ""
			ident[i] = fmt.Sprintf("section_fallback_page_%d", b.page0)
		}
		return
	}
	var groups [][]int
	cur := make([]int, 0, 8)
	for i, b := range out {
		if isSectionBreak(b) && len(cur) > 0 {
			groups = append(groups, cur)
			cur = nil
		}
		cur = append(cur, i)
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}
	for gi, ids := range groups {
		t := ""
		idStr := fmt.Sprintf("section_%d", gi)
		if len(ids) > 0 {
			t = sectionTitleFromFirstHeading(out[ids[0]])
		}
		for _, j := range ids {
			idx[j] = gi
			title[j] = t
			ident[j] = idStr
		}
	}
	return
}

func plainTextForRAG(md string) string {
	s := stripFirstAnchor(md)
	s = htmlTagStripRE.ReplaceAllString(s, " ")
	s = strings.NewReplacer(
		"&nbsp;", " ",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
	).Replace(s)
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

func buildRAGEmbeddingItems(out []dptBuilt, secIdx []int, secTitle []string, secIdent []string) []any {
	items := make([]any, 0, len(out))
	for i, b := range out {
		meta := map[string]any{
			"section_index":      secIdx[i],
			"section_identifier": secIdent[i],
			"section_title":      secTitle[i],
			"page_index":         b.page0 + 1,
			"page_index0":        b.page0,
			"chunk_type":         b.dptType,
			"chunk_src_type":     b.srcType,
		}
		if b.subtype != "" {
			meta["subtype"] = b.subtype
		}
		items = append(items, map[string]any{
			"id":       b.id,
			"text":     plainTextForRAG(b.markdown),
			"metadata": meta,
		})
	}
	return items
}

func buildPageSectionsAsFallback(out []dptBuilt) []any {
	s := buildSplits(out)
	res := make([]any, 0, len(s))
	for _, it := range s {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		pages, _ := m["pages"].([]int)
		p := 0
		if len(pages) > 0 {
			p = pages[0]
		}
		res = append(res, map[string]any{
			"class_":     "section",
			"identifier": fmt.Sprintf("section_fallback_page_%d", p),
			"title":      "",
			"pages":      m["pages"],
			"markdown":   m["markdown"],
			"chunks":     m["chunks"],
		})
	}
	return res
}

func buildSplits(out []dptBuilt) []any {
	if len(out) == 0 {
		return []any{}
	}
	pageOrder := make([]int, 0)
	seen := map[int]bool{}
	for _, b := range out {
		if !seen[b.page0] {
			seen[b.page0] = true
			pageOrder = append(pageOrder, b.page0)
		}
	}
	sort.Ints(pageOrder)
	splits := make([]any, 0, len(pageOrder))
	for _, p := range pageOrder {
		var ids []any
		var parts []string
		for _, b := range out {
			if b.page0 != p {
				continue
			}
			ids = append(ids, b.id)
			parts = append(parts, b.markdown)
		}
		splits = append(splits, map[string]any{
			"class_":     "page",
			"identifier": fmt.Sprintf("page_%d", p),
			"pages":      []int{p},
			"markdown":   strings.Join(parts, "\n\n"),
			"chunks":     ids,
		})
	}
	return splits
}

func chunkPageIndex(c map[string]any) int {
	return intFromAny(c["page_index"])
}

func chunkOrder(c map[string]any) int {
	return intFromAny(c["order"])
}

func intFromAny(v any) int {
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

func toF64(v any) float64 {
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

func toFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int8:
		return float64(x), true
	case int16:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	default:
		return 0, false
	}
}

func boolFromAny(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func firstNonEmptyString(values ...any) string {
	for _, v := range values {
		if s := dptCleanText(v); s != "" {
			return s
		}
	}
	return ""
}

func dptCleanText(v any) string {
	if v == nil {
		return ""
	}
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case []byte:
		s = string(x)
	default:
		s = fmt.Sprint(v)
	}
	s = strings.TrimSpace(s)
	if s == "" || s == "<nil>" || strings.EqualFold(s, "null") {
		return ""
	}
	return s
}

func dptChunkBody(c map[string]any, typ string) string {
	if body := dptCleanText(c["text"]); body != "" {
		return body
	}
	if typ == "figure" {
		return dptFigureFallbackText(c)
	}
	return ""
}

func dptFigureFallbackText(c map[string]any) string {
	payload, _ := c["payload"].(map[string]any)
	srcType := strings.ToLower(dptCleanText(c["type"]))
	kind := strings.ToLower(firstNonEmptyString(payload["image_kind"], srcType))
	label := "Embedded image"
	switch kind {
	case "stamp":
		label = "Stamp image"
	case "logo":
		label = "Logo image"
	case "figure":
		label = "Figure"
	}

	format := strings.ToLower(firstNonEmptyString(payload["format"], payload["mime_type"]))
	width := dptIntAny(firstNonEmptyString(payload["preview_width"], payload["width"]))
	height := dptIntAny(firstNonEmptyString(payload["preview_height"], payload["height"]))
	if width > 0 && height > 0 && format != "" {
		return fmt.Sprintf("%s (%s, %dx%d)", label, format, width, height)
	}
	if width > 0 && height > 0 {
		return fmt.Sprintf("%s (%dx%d)", label, width, height)
	}
	if format != "" {
		return fmt.Sprintf("%s (%s)", label, format)
	}
	return label
}

func dptIntAny(v any) int {
	if f, ok := toFloat64(v); ok {
		return int(math.Round(f))
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" {
		return 0
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err == nil {
		return n
	}
	return 0
}

func truncateRunes(s string, max int) string {
	if max <= 0 || len([]rune(s)) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max])
}

func newUUIDv4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}
