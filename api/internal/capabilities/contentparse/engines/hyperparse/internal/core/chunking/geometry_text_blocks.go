package chunking

import (
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	geometryTextBlockMaxYGap      = 0.03
	geometryTextBlockMaxXDrift    = 0.1
	geometryTextFollowerMaxYGap   = 0.024
	geometryTextFollowerMaxXDrift = 0.08
	suspiciousTextSpanMaxYGap     = 0.085
	suspiciousTextSpanMaxXShift   = 0.22
)

type geometryTextLine struct {
	order       int
	pageIndex   int
	sourceTrace string
	text        string
	geomX       float64
	geomY       float64
	xNorm       float64
	yNorm       float64
}

// TextLikesFromGeometryLines rebuilds smaller text blocks from layout lines.
// It is intended for scan-fallback PDFs where native text segments are often
// overly coarse and merge unrelated regions into a single chunk.
func TextLikesFromGeometryLines(lines []GeometryLineLike, pageGeoms map[int]PageGeom) []TextLike {
	if len(lines) == 0 || len(pageGeoms) == 0 {
		return nil
	}

	prepared := make([]geometryTextLine, 0, len(lines))
	for _, gl := range lines {
		text := strings.TrimSpace(gl.Text)
		if text == "" {
			continue
		}
		pageIndex := gl.PageIndex
		if pageIndex <= 0 {
			pageIndex = pageIndexFromTrace(gl.SourceTrace)
		}
		if pageIndex <= 0 {
			continue
		}
		g, ok := pageGeoms[pageIndex]
		if !ok || g.Right <= g.Left || g.Top <= g.Bottom {
			continue
		}
		xNorm := clamp01((gl.GeomX - g.Left) / (g.Right - g.Left))
		yNorm := clamp01((gl.GeomY - g.Bottom) / (g.Top - g.Bottom))
		prepared = append(prepared, geometryTextLine{
			order:       gl.Order,
			pageIndex:   pageIndex,
			sourceTrace: gl.SourceTrace,
			text:        text,
			geomX:       gl.GeomX,
			geomY:       gl.GeomY,
			xNorm:       xNorm,
			yNorm:       yNorm,
		})
	}

	sort.SliceStable(prepared, func(i, j int) bool {
		if prepared[i].pageIndex != prepared[j].pageIndex {
			return prepared[i].pageIndex < prepared[j].pageIndex
		}
		if prepared[i].order != prepared[j].order {
			return prepared[i].order < prepared[j].order
		}
		if math.Abs(prepared[i].yNorm-prepared[j].yNorm) > 0.001 {
			return prepared[i].yNorm > prepared[j].yNorm
		}
		return prepared[i].xNorm < prepared[j].xNorm
	})

	out := make([]TextLike, 0, len(prepared))
	block := make([]geometryTextLine, 0, 4)
	flush := func() {
		if len(block) == 0 {
			return
		}
		textLines := make([]string, 0, len(block))
		for _, line := range block {
			textLines = append(textLines, line.text)
		}
		first := block[0]
		out = append(out, TextLike{
			Order:       first.order,
			SourceTrace: first.sourceTrace,
			Text:        strings.Join(textLines, "\n"),
			ChunkType:   "",
			GeomX:       first.geomX,
			GeomY:       first.geomY,
		})
		block = block[:0]
	}

	for _, line := range prepared {
		if len(block) == 0 {
			block = append(block, line)
			continue
		}
		if shouldBreakGeometryTextBlock(block, line) {
			flush()
		}
		block = append(block, line)
	}
	flush()

	return out
}

// ShouldPreferGeometryTextBlocks detects native text segments that merge
// spatially distant layout lines. When that happens, rebuilding text blocks from
// geometry lines is usually more accurate than trusting the native segments.
func ShouldPreferGeometryTextBlocks(
	texts []TextLike,
	lines []GeometryLineLike,
	pageGeoms map[int]PageGeom,
) bool {
	if len(texts) == 0 || len(lines) == 0 || len(pageGeoms) == 0 {
		return false
	}

	for _, text := range texts {
		if strings.Contains(strings.TrimSpace(text.ChunkKey), "_L") {
			continue
		}
		targets := normalizedTextLines(text.Text)
		if len(targets) < 2 {
			continue
		}

		pageIndex := pageIndexFromTrace(text.SourceTrace)
		if pageIndex <= 0 {
			pageIndex = pageIndexFromGeometryLines(text.SourceTrace, lines)
		}
		if pageIndex <= 0 {
			continue
		}
		if _, ok := pageGeoms[pageIndex]; !ok {
			continue
		}

		pageLines := geometryLineBoxesForPage(text.SourceTrace, pageIndex, lines, pageGeoms)
		if len(pageLines) == 0 {
			continue
		}

		matched := matchExactLineSequenceBoxes(pageLines, targets)
		if len(matched) == 0 {
			matched = matchOrderedLineSubsetBoxes(pageLines, targets)
		}
		if len(matched) < 2 {
			continue
		}
		if geometryTextSpanLooksSuspicious(matched) {
			return true
		}
	}

	return false
}

func geometryTextSpanLooksSuspicious(lines []geometryLineBox) bool {
	if len(lines) < 2 {
		return false
	}
	for i := 1; i < len(lines); i++ {
		prev := lines[i-1]
		next := lines[i]
		if prev.bbox == nil || next.bbox == nil {
			continue
		}
		prevCenterY := (prev.bbox.Top + prev.bbox.Bottom) / 2
		nextCenterY := (next.bbox.Top + next.bbox.Bottom) / 2
		if math.Abs(prevCenterY-nextCenterY) > suspiciousTextSpanMaxYGap {
			return true
		}
		prevCenterX := (prev.bbox.Left + prev.bbox.Right) / 2
		nextCenterX := (next.bbox.Left + next.bbox.Right) / 2
		if math.Abs(prevCenterX-nextCenterX) > suspiciousTextSpanMaxXShift {
			return true
		}
	}
	return false
}

func shouldBreakGeometryTextBlock(block []geometryTextLine, next geometryTextLine) bool {
	if len(block) == 0 {
		return false
	}
	last := block[len(block)-1]
	if next.pageIndex != last.pageIndex {
		return true
	}
	if isGeometryDividerLine(last.text) || isGeometryDividerLine(next.text) {
		return true
	}
	if geometryBlockColumnShift(block, next) > geometryTextBlockMaxXDrift {
		return true
	}
	if geometryBlockVerticalGap(last, next) > geometryTextBlockMaxYGap {
		return true
	}
	if canAttachGeometryFollower(block, next) {
		return false
	}
	if startsNewGeometryStructuredBlock(next.text) {
		return true
	}
	if paragraphLastLineEndsWithSentenceTerminator(last.text) && startsNewGeometryStructuredBlock(next.text) {
		return true
	}
	return false
}

func geometryBlockVerticalGap(prev, next geometryTextLine) float64 {
	if prev.yNorm <= next.yNorm {
		return 0
	}
	return prev.yNorm - next.yNorm
}

func geometryBlockColumnShift(block []geometryTextLine, next geometryTextLine) float64 {
	minX, maxX := block[0].xNorm, block[0].xNorm
	for _, line := range block[1:] {
		if line.xNorm < minX {
			minX = line.xNorm
		}
		if line.xNorm > maxX {
			maxX = line.xNorm
		}
	}
	if next.xNorm < minX {
		return minX - next.xNorm
	}
	if next.xNorm > maxX {
		return next.xNorm - maxX
	}
	return 0
}

func canAttachGeometryFollower(block []geometryTextLine, next geometryTextLine) bool {
	if len(block) != 1 {
		return false
	}
	label := block[0]
	if !looksLikePureGeometryLabel(label.text) {
		return false
	}
	if geometryBlockVerticalGap(label, next) > geometryTextFollowerMaxYGap {
		return false
	}
	if math.Abs(label.xNorm-next.xNorm) > geometryTextFollowerMaxXDrift {
		return false
	}
	if isGeometryDividerLine(next.text) {
		return false
	}
	if looksLikeGeometryValueLine(next.text) {
		return true
	}
	return !startsNewGeometryStructuredBlock(next.text)
}

func startsNewGeometryStructuredBlock(text string) bool {
	t := normalizeStructureLine(strings.TrimSpace(text))
	if t == "" {
		return false
	}
	if isGeometryDividerLine(t) {
		return true
	}
	if _, _, ok := splitKV(t); ok {
		return true
	}
	if looksLikeListItem(t) {
		return true
	}
	if looksLikeHeading(t) || strings.HasSuffix(t, ":") {
		return true
	}
	runes := utf8.RuneCountInString(t)
	if runes == 0 || runes > 40 {
		return false
	}
	if strings.ContainsAny(t, ".!?;。！？；") {
		return false
	}
	return len(strings.Fields(t)) <= 6
}

func looksLikePureGeometryLabel(text string) bool {
	t := normalizeStructureLine(strings.TrimSpace(text))
	if t == "" || isGeometryDividerLine(t) {
		return false
	}
	if strings.ContainsAny(t, "0123456789€$£/%") {
		return false
	}
	if strings.ContainsAny(t, ".!?;。！？；") {
		return false
	}
	return utf8.RuneCountInString(t) <= 40 && len(strings.Fields(t)) <= 6
}

func looksLikeGeometryValueLine(text string) bool {
	t := normalizeStructureLine(strings.TrimSpace(text))
	if t == "" || isGeometryDividerLine(t) {
		return false
	}
	if strings.ContainsAny(t, "0123456789€$£/%") {
		return true
	}
	if strings.Contains(t, "/") || strings.Contains(t, "-") {
		return true
	}
	return false
}

func isGeometryDividerLine(text string) bool {
	t := strings.ReplaceAll(normalizeStructureLine(strings.TrimSpace(text)), " ", "")
	if len(t) < 8 {
		return false
	}
	return strings.Trim(t, "-_=.") == ""
}
