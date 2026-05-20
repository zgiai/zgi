package chunking

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

var pageNumFromTraceRE = regexp.MustCompile(`page#(\d+)`)

func Build(input BuildInput) []map[string]any {
	rules := DefaultRules()
	if input.SkipTableRule {
		rules = excludeRuleByName(rules, "table")
	}
	chunks := BuildChunks(input, rules...)
	out := make([]map[string]any, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, chunkToMap(c))
	}
	return out
}

func excludeRuleByName(rules []Rule, name string) []Rule {
	if name == "" {
		return rules
	}
	out := make([]Rule, 0, len(rules))
	for _, r := range rules {
		if r.Name() == name {
			continue
		}
		out = append(out, r)
	}
	return out
}

func BuildChunks(input BuildInput, rules ...Rule) []Chunk {
	selected := rules
	if len(selected) == 0 {
		selected = DefaultRules()
	}
	inp := input
	inp.Texts = expandTextsForFormulaChunks(inp.Texts)
	inp.Texts = expandTextsForMultilineSegments(inp.Texts)
	inp.Texts = selectTextLikesForChunking(inp.Texts, inp.GeometryLines, inp.PageGeoms)
	inp.Texts = alignExpandedTextBBoxes(inp.Texts, inp.GeometryLines, inp.PageGeoms)
	out := make([]Chunk, 0, len(inp.Texts)+len(inp.Images)+len(inp.Bookmarks)+len(inp.Annotations)+len(inp.Forms)+len(inp.Attachments))
	for _, rule := range selected {
		out = rule.Apply(inp, out)
	}
	return out
}

func selectTextLikesForChunking(texts []TextLike, lines []GeometryLineLike, pageGeoms map[int]PageGeom) []TextLike {
	if !ShouldPreferGeometryTextBlocks(texts, lines, pageGeoms) {
		return texts
	}
	rebuilt := TextLikesFromGeometryLines(lines, pageGeoms)
	if len(rebuilt) == 0 {
		return texts
	}
	rebuilt = expandTextsForFormulaChunks(rebuilt)
	rebuilt = expandTextsForMultilineSegments(rebuilt)
	return rebuilt
}

func textLikeChunkID(s TextLike) string {
	if k := strings.TrimSpace(s.ChunkKey); k != "" {
		return k
	}
	base := s.SegKeyBase
	if base == 0 {
		base = s.Order
	}
	return "seg_" + strconv.Itoa(base)
}

func chunkToMap(c Chunk) map[string]any {
	m := map[string]any{
		"chunk_id":   c.ChunkID,
		"type":       c.Type,
		"page_index": c.PageIndex,
		"order":      c.Order,
		"source":     c.Source,
		"confidence": c.Confidence,
	}
	if strings.TrimSpace(c.Text) != "" {
		m["text"] = c.Text
	}
	if strings.TrimSpace(c.SourceTrace) != "" {
		m["source_trace"] = c.SourceTrace
	}
	if c.BBox != nil {
		m["bbox"] = BBoxTopLeftMap(c.BBox)
	}
	if c.Payload != nil {
		m["payload"] = c.Payload
	}
	return m
}

func BBoxTopLeftMap(box *BBox) map[string]any {
	if box == nil {
		return nil
	}
	top := round(1-box.Top, 6)
	bottom := round(1-box.Bottom, 6)
	if bottom < top {
		top, bottom = bottom, top
	}
	return map[string]any{
		"left":   round(box.Left, 6),
		"right":  round(box.Right, 6),
		"top":    top,
		"bottom": bottom,
	}
}

func pageIndexFromTrace(trace string) int {
	m := pageNumFromTraceRE.FindStringSubmatch(trace)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func pageIndexByPageObject(pages []PageRef, obj int) int {
	for _, p := range pages {
		if p.ObjectNumber == obj {
			return p.PageIndex
		}
	}
	return 0
}

func textAnchorToBBox(x, y float64, g PageGeom) *BBox {
	return AnchorToBBox(x, y, g)
}

func AnchorToBBox(x, y float64, g PageGeom) *BBox {
	if g.Right <= g.Left || g.Top <= g.Bottom || (x == 0 && y == 0) {
		return nil
	}

	left, right, top, bottom := 0.0, 1.0, 1.0, 0.0
	w := g.Right - g.Left
	h := g.Top - g.Bottom
	xr := (x - g.Left) / w
	yr := (y - g.Bottom) / h

	halfWidth := 0.03
	halfHeight := 0.02
	if halfWidth <= 0 {
		halfWidth = 0.01
	}
	if halfHeight <= 0 {
		halfHeight = 0.01
	}

	left = clamp01(xr - halfWidth)
	right = clamp01(xr + halfWidth)
	top = clamp01(yr + halfHeight)
	bottom = clamp01(yr - halfHeight)
	if right < left {
		left, right = right, left
	}
	if top < bottom {
		top, bottom = bottom, top
	}

	if (right - left) < 0.006 {
		c := (left + right) / 2
		left = clamp01(c - 0.003)
		right = clamp01(c + 0.003)
	}
	if (top - bottom) < 0.004 {
		c := (top + bottom) / 2
		bottom = clamp01(c - 0.002)
		top = clamp01(c + 0.002)
	}

	return &BBox{
		Left:   round(left, 6),
		Right:  round(right, 6),
		Top:    round(top, 6),
		Bottom: round(bottom, 6),
	}
}

func rectToBBox(rectRaw string, g PageGeom) *BBox {
	fields := strings.Fields(strings.TrimSpace(rectRaw))
	if len(fields) != 4 || g.Right <= g.Left || g.Top <= g.Bottom {
		return nil
	}
	v := make([]float64, 4)
	for i := range fields {
		n, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return nil
		}
		v[i] = n
	}
	l := clamp01((v[0] - g.Left) / (g.Right - g.Left))
	b := clamp01((v[1] - g.Bottom) / (g.Top - g.Bottom))
	r := clamp01((v[2] - g.Left) / (g.Right - g.Left))
	t := clamp01((v[3] - g.Bottom) / (g.Top - g.Bottom))
	if r < l {
		l, r = r, l
	}
	if t < b {
		b, t = t, b
	}
	return &BBox{
		Left:   round(l, 6),
		Right:  round(r, 6),
		Top:    round(t, 6),
		Bottom: round(b, 6),
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

func round(v float64, digits float64) float64 {
	p := math.Pow10(int(digits))
	return math.Round(v*p) / p
}
