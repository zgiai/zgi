package chunking

import (
	"math"
	"strings"
)

type geometryLineBox struct {
	pageIndex   int
	sourceTrace string
	order       int
	text        string
	normText    string
	bbox        *BBox
}

func textBBoxForTextLike(s TextLike, input BuildInput) *BBox {
	pageIndex := pageIndexFromTrace(s.SourceTrace)
	if pageIndex <= 0 {
		pageIndex = pageIndexFromGeometryLines(s.SourceTrace, input.GeometryLines)
	}
	if bbox := matchTextBBoxFromGeometryLines(s, pageIndex, input); bbox != nil {
		return bbox
	}
	return textAnchorToBBox(s.GeomX, s.GeomY, input.PageGeoms[pageIndex])
}

func pageIndexFromGeometryLines(sourceTrace string, lines []GeometryLineLike) int {
	for _, gl := range lines {
		if strings.TrimSpace(gl.SourceTrace) != strings.TrimSpace(sourceTrace) {
			continue
		}
		if gl.PageIndex > 0 {
			return gl.PageIndex
		}
		if pi := pageIndexFromTrace(gl.SourceTrace); pi > 0 {
			return pi
		}
	}
	return 0
}

func matchTextBBoxFromGeometryLines(s TextLike, pageIndex int, input BuildInput) *BBox {
	if len(input.GeometryLines) == 0 || pageIndex <= 0 {
		return nil
	}
	if _, ok := input.PageGeoms[pageIndex]; !ok {
		return nil
	}
	lines := geometryLineBoxesForPage(s.SourceTrace, pageIndex, input.GeometryLines, input.PageGeoms)
	if len(lines) == 0 {
		return nil
	}
	targets := normalizedTextLines(s.Text)
	if len(targets) == 0 {
		return nil
	}
	if matched := matchExactLineSequence(lines, targets); matched != nil {
		return unionBBoxes(matched)
	}
	if matched := matchOrderedLineSubset(lines, targets); matched != nil {
		return unionBBoxes(matched)
	}
	if len(targets) == 1 {
		if matched := matchSingleLineFuzzy(lines, targets[0]); matched != nil {
			return matched
		}
	}
	return nil
}

func geometryLineBoxesForPage(sourceTrace string, pageIndex int, lines []GeometryLineLike, pageGeoms map[int]PageGeom) []geometryLineBox {
	trace := strings.TrimSpace(sourceTrace)
	out := make([]geometryLineBox, 0, len(lines))
	for _, gl := range lines {
		pi := gl.PageIndex
		if pi <= 0 {
			pi = pageIndexFromTrace(gl.SourceTrace)
		}
		if pi != pageIndex {
			continue
		}
		if trace != "" && strings.TrimSpace(gl.SourceTrace) != trace {
			continue
		}
		bbox := geometryLineBBox(gl, pageGeoms[pi])
		if bbox == nil {
			continue
		}
		out = append(out, geometryLineBox{
			pageIndex:   pi,
			sourceTrace: gl.SourceTrace,
			order:       gl.Order,
			text:        gl.Text,
			normText:    normalizeBBoxMatchText(gl.Text),
			bbox:        bbox,
		})
	}
	if len(out) > 0 || trace == "" {
		return out
	}
	// SourceTrace can lose precision after splitting; fall back to all geometry
	// lines on the same page.
	for _, gl := range lines {
		pi := gl.PageIndex
		if pi <= 0 {
			pi = pageIndexFromTrace(gl.SourceTrace)
		}
		if pi != pageIndex {
			continue
		}
		bbox := geometryLineBBox(gl, pageGeoms[pi])
		if bbox == nil {
			continue
		}
		out = append(out, geometryLineBox{
			pageIndex:   pi,
			sourceTrace: gl.SourceTrace,
			order:       gl.Order,
			text:        gl.Text,
			normText:    normalizeBBoxMatchText(gl.Text),
			bbox:        bbox,
		})
	}
	return out
}

func geometryLineBBox(gl GeometryLineLike, pageGeom PageGeom) *BBox {
	if gl.BBox != nil {
		if gl.BBox.Right <= gl.BBox.Left || gl.BBox.Top <= gl.BBox.Bottom {
			return nil
		}
		return gl.BBox
	}
	return textAnchorToBBox(gl.GeomX, gl.GeomY, pageGeom)
}

func normalizedTextLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	raw := strings.Split(text, "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		norm := normalizeBBoxMatchText(line)
		if norm == "" {
			continue
		}
		out = append(out, norm)
	}
	return out
}

func normalizeBBoxMatchText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"\u00a0", " ",
		"：", ":",
		"，", ",",
		"。", ".",
		"；", ";",
		"（", "(",
		"）", ")",
		"【", "[",
		"】", "]",
		"—", "-",
	)
	text = replacer.Replace(text)
	text = strings.Join(strings.Fields(text), " ")
	return strings.ToLower(strings.TrimSpace(text))
}

func matchExactLineSequence(lines []geometryLineBox, targets []string) []*BBox {
	matched := matchExactLineSequenceBoxes(lines, targets)
	if len(matched) == 0 {
		return nil
	}
	boxes := make([]*BBox, 0, len(matched))
	for _, line := range matched {
		boxes = append(boxes, line.bbox)
	}
	return boxes
}

func matchExactLineSequenceBoxes(lines []geometryLineBox, targets []string) []geometryLineBox {
	if len(lines) == 0 || len(targets) == 0 || len(lines) < len(targets) {
		return nil
	}
	for start := 0; start <= len(lines)-len(targets); start++ {
		matched := true
		for i := range targets {
			if lines[start+i].normText != targets[i] {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		boxes := make([]geometryLineBox, 0, len(targets))
		for i := range targets {
			boxes = append(boxes, lines[start+i])
		}
		return boxes
	}
	return nil
}

func matchOrderedLineSubset(lines []geometryLineBox, targets []string) []*BBox {
	matched := matchOrderedLineSubsetBoxes(lines, targets)
	if len(matched) == 0 {
		return nil
	}
	boxes := make([]*BBox, 0, len(matched))
	for _, line := range matched {
		boxes = append(boxes, line.bbox)
	}
	return boxes
}

func matchOrderedLineSubsetBoxes(lines []geometryLineBox, targets []string) []geometryLineBox {
	if len(lines) == 0 || len(targets) == 0 {
		return nil
	}
	boxes := make([]geometryLineBox, 0, len(targets))
	from := 0
	for _, target := range targets {
		found := -1
		for idx := from; idx < len(lines); idx++ {
			if lines[idx].normText != target {
				continue
			}
			found = idx
			break
		}
		if found < 0 {
			return nil
		}
		boxes = append(boxes, lines[found])
		from = found + 1
	}
	return boxes
}

func matchSingleLineFuzzy(lines []geometryLineBox, target string) *BBox {
	if target == "" {
		return nil
	}
	var best *BBox
	bestScore := math.MaxFloat64
	for _, line := range lines {
		if line.normText == "" {
			continue
		}
		if line.normText == target {
			return line.bbox
		}
		if !strings.Contains(line.normText, target) && !strings.Contains(target, line.normText) {
			continue
		}
		score := math.Abs(float64(len(line.normText) - len(target)))
		if score >= bestScore {
			continue
		}
		bestScore = score
		best = line.bbox
	}
	if best != nil {
		return best
	}
	return nil
}

func unionBBoxes(boxes []*BBox) *BBox {
	if len(boxes) == 0 {
		return nil
	}
	left, right, top, bottom := 1.0, 0.0, 0.0, 1.0
	found := false
	for _, box := range boxes {
		if box == nil {
			continue
		}
		left = math.Min(left, box.Left)
		right = math.Max(right, box.Right)
		top = math.Max(top, box.Top)
		bottom = math.Min(bottom, box.Bottom)
		found = true
	}
	if !found {
		return nil
	}
	if right < left {
		left, right = right, left
	}
	if top < bottom {
		top, bottom = bottom, top
	}
	return &BBox{
		Left:   round(left, 6),
		Right:  round(right, 6),
		Top:    round(top, 6),
		Bottom: round(bottom, 6),
	}
}
