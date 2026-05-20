package chunking

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// maxNativeHeadingRunesPerLine aligns with the full-text limit in looksLikeHeading.
const maxNativeHeadingRunesPerLine = 120

// Short section headings are usually compact; a larger threshold can misclassify
// the first summary line as a heading.
const maxStructuredHeadingRunes = 18
const minUsefulLineRunes = 2

var reStructureInvalidRune = regexp.MustCompile(`[^\p{Han}\p{Latin}\pN\s\.,:;!?！？、—【】\(\)（）\-\+/%《》“”‘’]`)

// expandTextsForMultilineSegments splits TextLike values with deterministic rules before chunk rules:
// - short heading lines become standalone heading chunks;
// - consecutive body lines are merged into one paragraph;
// - blank lines and the next heading line close the current paragraph;
// - each split segment gets an independent chunk_id (parent_L0, parent_L1, ...).
func expandTextsForMultilineSegments(texts []TextLike) []TextLike {
	if len(texts) == 0 {
		return nil
	}
	out := make([]TextLike, 0, len(texts)+8)
	seq := 0
	for _, t := range texts {
		blocks := splitNativeTextBlocks(t.Text)
		if len(blocks) == 0 {
			nt := t
			nt.Text = strings.TrimSpace(t.Text)
			nt.Order = seq
			seq++
			out = append(out, nt)
			continue
		}
		if len(blocks) == 1 {
			nt := t
			nt.Text = blocks[0].text
			nt.Order = seq
			seq++
			nt.ChunkType = multilineSplitBlockChunkType(blocks[0])
			out = append(out, nt)
			continue
		}
		parentID := textLikeChunkID(t)
		parentBB := t.BBox
		totalLines := 0
		for _, b := range blocks {
			totalLines += b.lineCount
		}
		prevLines := 0
		for i, b := range blocks {
			nt := t
			nt.Text = b.text
			nt.Order = seq
			seq++
			nt.SegKeyBase = 0
			nt.ChunkKey = fmt.Sprintf("%s_L%d", parentID, i)
			nt.ChunkType = multilineSplitBlockChunkType(b)
			// Split the parent bbox by line-count ratio as a fallback position.
			// alignExpandedTextBBoxes can replace it with more precise geometry later.
			nt.BBox = sliceParentBBoxByLineRatio(parentBB, prevLines, b.lineCount, totalLines)
			prevLines += b.lineCount
			out = append(out, nt)
		}
	}
	return out
}

type nativeTextBlock struct {
	text      string
	lineCount int
	chunkType string
}

func splitNativeTextBlocks(s string) []nativeTextBlock {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	raw := strings.Split(s, "\n")
	lines := make([]string, 0, len(raw))
	for _, ln := range raw {
		lines = append(lines, strings.TrimSpace(ln))
	}

	blocks := make([]nativeTextBlock, 0, 4)
	paraLines := make([]string, 0, 4)
	flushParagraph := func() {
		if len(paraLines) == 0 {
			return
		}
		blocks = append(blocks, nativeTextBlock{
			text:      strings.Join(paraLines, "\n"),
			lineCount: len(paraLines),
			chunkType: "",
		})
		paraLines = paraLines[:0]
	}
	appendSingleLineBlock := func(line string, chunkType string) {
		if strings.TrimSpace(line) == "" {
			return
		}
		blocks = append(blocks, nativeTextBlock{
			text:      strings.TrimSpace(line),
			lineCount: 1,
			chunkType: chunkType,
		})
	}
	for i, ln := range lines {
		if ln == "" {
			flushParagraph()
			continue
		}
		if len([]rune(ln)) < minUsefulLineRunes {
			// Filter very short noise lines so they do not pollute paragraph boundaries.
			continue
		}
		if isStructuredHeadingLineAt(lines, i) &&
			(len(paraLines) == 0 || paragraphLastLineEndsWithSentenceTerminator(paraLines[len(paraLines)-1])) {
			flushParagraph()
			appendSingleLineBlock(ln, "heading")
			continue
		}
		if isStandaloneRuleLine(ln) {
			flushParagraph()
			appendSingleLineBlock(ln, "")
			continue
		}
		paraLines = append(paraLines, ln)
	}
	flushParagraph()
	return blocks
}

// multilineSplitBlockChunkType is used only for split child blocks:
// - heading blocks keep their predicted type;
// - other blocks defer to later text_* rules, defaulting to paragraph.
func multilineSplitBlockChunkType(block nativeTextBlock) string {
	if block.chunkType != "" {
		return block.chunkType
	}
	if block.lineCount > 1 {
		return ""
	}
	return ""
}

func isStandaloneRuleLine(t string) bool {
	tt := normalizeStructureLine(strings.TrimSpace(t))
	if tt == "" {
		return false
	}
	if _, _, ok := splitKV(tt); ok {
		return true
	}
	if looksLikeListItem(tt) {
		return true
	}
	if looksLikeFormulaText(tt) {
		return true
	}
	return false
}

func isStructuredHeadingLineAt(lines []string, idx int) bool {
	if idx < 0 || idx >= len(lines) {
		return false
	}
	t := normalizeStructureLine(strings.TrimSpace(lines[idx]))
	if t == "" {
		return false
	}
	if isStandaloneRuleLine(t) {
		return false
	}
	n := len([]rune(t))
	if n == 0 || n > maxStructuredHeadingRunes || n > maxNativeHeadingRunesPerLine {
		return false
	}
	// Common document-title structure: short parallel phrases.
	if strings.Contains(t, "、") && n <= 30 {
		return true
	}
	// Sentence-ending punctuation tends to indicate body text, not headings.
	if strings.HasSuffix(t, "。") || strings.HasSuffix(t, ".") || strings.HasSuffix(t, "；") || strings.HasSuffix(t, ";") {
		return false
	}
	next := ""
	for j := idx + 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) != "" {
			cand := normalizeStructureLine(strings.TrimSpace(lines[j]))
			if len([]rune(cand)) < minUsefulLineRunes {
				// Skip short noise lines while searching for the next real line.
				continue
			}
			next = cand
			break
		}
		// A blank following line allows the current short line to be a paragraph heading.
		if strings.TrimSpace(lines[j]) == "" {
			return true
		}
	}
	// A final short line is usually an independent heading or label.
	if next == "" {
		return true
	}
	// If the next line looks like body text, treat the current line as a heading.
	nextRunes := len([]rune(next))
	if nextRunes > maxStructuredHeadingRunes {
		return true
	}
	// The first body line after a short heading can also be short, so keep this
	// pattern as heading + paragraph when the next line is still meaningfully longer.
	if nextRunes > n && nextRunes >= 10 {
		return true
	}
	if strings.HasSuffix(next, "。") || strings.HasSuffix(next, ".") || strings.HasSuffix(next, "；") || strings.HasSuffix(next, ";") {
		return true
	}
	return false
}

func normalizeStructureLine(s string) string {
	if s == "" {
		return s
	}
	// Clean common glyph-mapping noise before heading/paragraph boundary detection.
	s = strings.NewReplacer(
		"Ÿ", "", "™", "", "Ä", "", "Ñ", "", "\u00a0", " ",
		"，", ",", "。", ".", "：", ":", "；", ";", "､", "、",
	).Replace(s)
	s = reStructureInvalidRune.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

// paragraphLastLineEndsWithSentenceTerminator detects that body text has ended.
// When the previous line ends with sentence punctuation, a following short line
// can start a new heading instead of being merged into the same paragraph.
func paragraphLastLineEndsWithSentenceTerminator(lastLine string) bool {
	s := strings.TrimSpace(lastLine)
	if s == "" {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(s)
	switch r {
	case '。', '！', '？', '.', '!', '?', '；', ';':
		return true
	default:
		return false
	}
}

func alignExpandedTextBBoxes(texts []TextLike, geometryLines []GeometryLineLike, pageGeoms map[int]PageGeom) []TextLike {
	if len(texts) == 0 || len(geometryLines) == 0 {
		return texts
	}
	linesByTrace := make(map[string][]GeometryLineLike)
	for _, gl := range geometryLines {
		trace := strings.TrimSpace(gl.SourceTrace)
		if trace == "" {
			continue
		}
		linesByTrace[trace] = append(linesByTrace[trace], gl)
	}
	for trace := range linesByTrace {
		sort.SliceStable(linesByTrace[trace], func(i, j int) bool {
			return linesByTrace[trace][i].Order < linesByTrace[trace][j].Order
		})
	}
	textIndexesByTrace := make(map[string][]int)
	for i, t := range texts {
		trace := strings.TrimSpace(t.SourceTrace)
		if trace == "" {
			continue
		}
		textIndexesByTrace[trace] = append(textIndexesByTrace[trace], i)
	}
	for trace, idxs := range textIndexesByTrace {
		lines := linesByTrace[trace]
		if len(lines) == 0 {
			continue
		}
		cursor := 0
		for _, idx := range idxs {
			t := texts[idx]
			if shouldSkipPreciseBBoxAlignment(t) {
				continue
			}
			childLines := extractAlignableChildLines(t.Text)
			if len(childLines) == 0 {
				continue
			}
			start, end, ok := findAlignedGeometryLineRange(childLines, lines, cursor)
			if !ok {
				continue
			}
			if bb := unionGeometryLineBBox(lines[start:end], pageGeoms); bb != nil {
				texts[idx].BBox = bb
				cursor = end
			}
		}
	}
	return texts
}

func shouldSkipPreciseBBoxAlignment(t TextLike) bool {
	// Native TextLike values that were never split and already have bbox can
	// trust upstream geometry directly.
	// Split child items with _m / _L suffixes must still enter alignment, or
	// siblings may incorrectly share the parent anchor.
	if t.BBox != nil && t.ChunkKey == "" {
		return true
	}
	return false
}

// sliceParentBBoxByLineRatio slices a parent bbox vertically using (prev, cnt, total).
// chunking.BBox uses native PDF bottom-origin semantics, so moving downward means
// subtracting a ratio of height from Top. Nil or invalid parents return nil.
func sliceParentBBoxByLineRatio(parent *BBox, prev, cnt, total int) *BBox {
	if parent == nil || total <= 0 || cnt <= 0 {
		return nil
	}
	height := parent.Top - parent.Bottom
	width := parent.Right - parent.Left
	if height <= 0 || width <= 0 {
		// Invalid dimensions are left for alignExpandedTextBBoxes or downstream
		// reliability guards rather than spreading bad coordinates to child lines.
		return nil
	}
	topRatio := float64(prev) / float64(total)
	bottomRatio := float64(prev+cnt) / float64(total)
	return &BBox{
		Left:   parent.Left,
		Right:  parent.Right,
		Top:    round(parent.Top-topRatio*height, 6),
		Bottom: round(parent.Top-bottomRatio*height, 6),
	}
}

func extractAlignableChildLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	raw := strings.Split(text, "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		n := normalizeBBoxAlignLine(line)
		if n == "" {
			continue
		}
		out = append(out, n)
	}
	return out
}

func normalizeBBoxAlignLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.NewReplacer(
		"\u00a0", "",
		" ", "",
		"\t", "",
		"\n", "",
		"\r", "",
	).Replace(s)
	return strings.TrimSpace(s)
}

func findAlignedGeometryLineRange(childLines []string, lines []GeometryLineLike, cursor int) (int, int, bool) {
	if len(childLines) == 0 || len(lines) == 0 {
		return 0, 0, false
	}
	if cursor < 0 {
		cursor = 0
	}
	for start := cursor; start < len(lines); start++ {
		first := normalizeBBoxAlignLine(lines[start].Text)
		if first == "" || first != childLines[0] {
			continue
		}
		j := start
		k := 0
		for j < len(lines) && k < len(childLines) {
			cur := normalizeBBoxAlignLine(lines[j].Text)
			if cur == "" {
				j++
				continue
			}
			if cur != childLines[k] {
				break
			}
			j++
			k++
		}
		if k == len(childLines) {
			return start, j, true
		}
	}
	return 0, 0, false
}

func unionGeometryLineBBox(lines []GeometryLineLike, pageGeoms map[int]PageGeom) *BBox {
	var out *BBox
	for _, gl := range lines {
		bb := gl.BBox
		if bb == nil {
			g := pageGeoms[gl.PageIndex]
			bb = textAnchorToBBox(gl.GeomX, gl.GeomY, g)
		}
		if bb == nil {
			continue
		}
		if out == nil {
			cp := *bb
			out = &cp
			continue
		}
		if bb.Left < out.Left {
			out.Left = bb.Left
		}
		if bb.Right > out.Right {
			out.Right = bb.Right
		}
		if bb.Top > out.Top {
			out.Top = bb.Top
		}
		if bb.Bottom < out.Bottom {
			out.Bottom = bb.Bottom
		}
	}
	return out
}
