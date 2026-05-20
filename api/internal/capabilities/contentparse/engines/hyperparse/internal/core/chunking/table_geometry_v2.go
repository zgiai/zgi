package chunking

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	geomRowYTol       = 0.008
	geomColMergeTol   = 0.045
	geomNearestColV2  = 0.12
	rowBreakYGap      = 0.075
	tableV2Confidence = 0.78
)

// applyTableGeometryTokensV2 runs geometry tokens through row clustering,
// row-local x-gap column splitting, and global column alignment.
func applyTableGeometryTokensV2(input BuildInput) []Chunk {
	pointsByPage := make(map[int][]textPoint)
	for _, gt := range input.GeometryTokens {
		pageIndex := gt.PageIndex
		if pageIndex <= 0 {
			pageIndex = pageIndexFromTrace(gt.SourceTrace)
		}
		g, ok := input.PageGeoms[pageIndex]
		if !ok || g.Right <= g.Left || g.Top <= g.Bottom || (gt.GeomX == 0 && gt.GeomY == 0) {
			continue
		}
		x := clamp01((gt.GeomX - g.Left) / (g.Right - g.Left))
		y := clamp01((gt.GeomY - g.Bottom) / (g.Top - g.Bottom))
		pointsByPage[pageIndex] = append(pointsByPage[pageIndex], textPoint{
			seg: TextLike{
				Order:       gt.Order,
				SourceTrace: gt.SourceTrace,
				Text:        strings.TrimSpace(gt.Text),
				ChunkType:   "token",
				GeomX:       gt.GeomX,
				GeomY:       gt.GeomY,
				BBox:        gt.BBox,
			},
			page: pageIndex,
			x:    x,
			y:    y,
			bbox: func() *BBox {
				if gt.BBox != nil {
					return gt.BBox
				}
				return textAnchorToBBox(gt.GeomX, gt.GeomY, g)
			}(),
		})
	}

	var out []Chunk
	tableIdx := 0
	for pageIndex, pts := range pointsByPage {
		if len(pts) < 4 {
			continue
		}
		outBefore := len(out)
		sort.Slice(pts, func(i, j int) bool {
			if math.Abs(pts[i].y-pts[j].y) > 0.008 {
				return pts[i].y > pts[j].y
			}
			return pts[i].x < pts[j].x
		})
		rows := clusterRows(pts, geomRowYTol)
		maxCellsInAnyRow := 0
		rowCount := len(rows)
		var rowCells [][]cellRun
		for _, tr := range rows {
			cells := clusterRowIntoCellsByX(tr.points)
			rowCells = append(rowCells, cells)
			if len(cells) > maxCellsInAnyRow {
				maxCellsInAnyRow = len(cells)
			}
		}
		if rowCount < 2 || maxCellsInAnyRow < 2 {
			// No table candidate: still attach debug for this page once geometryTokens are present.
			if len(out) == outBefore {
				out = append(out, Chunk{
					ChunkID:    "table_debug_page_" + strconv.Itoa(pageIndex),
					Type:       "table_debug",
					PageIndex:  pageIndex,
					Order:      990000 + pageIndex,
					Source:     "native_pdf",
					Confidence: 0.0,
					Payload: map[string]any{
						"token_count":           len(pts),
						"row_count":             rowCount,
						"row_cells_count":       len(rowCells),
						"max_cells_in_used_row": maxCellsInAnyRow,
						"reason": func() string {
							if rowCount < 2 {
								return "insufficient_rows"
							}
							return "insufficient_multi_cell_row"
						}(),
					},
				})
			}
			continue
		}
		blocks := splitRowCellsIntoTableBlocks(rowCells)
		// Secondary fallback to avoid losing two-line evidence during chunking.
		if len(blocks) == 0 {
			blocks = [][][]cellRun{rowCells}
		}
		blocksCount := len(blocks)
		firstBlockRows := 0
		firstAnchorsLen := 0
		firstIsLikely := false
		firstCellsTotal := 0
		if blocksCount > 0 {
			first := blocks[0]
			firstBlockRows = len(first)
			anchors := deriveColumnAnchorsFromCellRows(first)
			firstAnchorsLen = len(anchors)
			firstIsLikely = isLikelyTableV2Rows(first, anchors)
			for _, r := range first {
				firstCellsTotal += len(r)
			}
		}
		for _, block := range blocks {
			if len(block) < 2 {
				continue
			}
			c := buildTableChunkV2(pageIndex, tableIdx, block)
			if c != nil {
				out = append(out, *c)
				tableIdx++
			}
		}
		// If this page produced no table, attach debug once.
		if len(out) == outBefore {
			out = append(out, Chunk{
				ChunkID:    "table_debug_page_" + strconv.Itoa(pageIndex),
				Type:       "table_debug",
				PageIndex:  pageIndex,
				Order:      990000 + pageIndex,
				Source:     "native_pdf",
				Confidence: 0.0,
				Payload: map[string]any{
					"token_count":           len(pts),
					"row_count":             rowCount,
					"row_cells_count_used":  len(rowCells),
					"max_cells_in_used_row": maxCellsInAnyRow,
					"reason":                "no_table_after_blocks",
					"blocks_count":          blocksCount,
					"first_block_rows":      firstBlockRows,
					"first_anchors_len":     firstAnchorsLen,
					"first_is_likely":       firstIsLikely,
					"first_cells_total":     firstCellsTotal,
				},
			})
		}
	}
	return out
}

type cellRun struct {
	points []textPoint
}

func (c cellRun) minX() float64 {
	if len(c.points) == 0 {
		return 0
	}
	m := c.points[0].x
	for i := 1; i < len(c.points); i++ {
		if c.points[i].x < m {
			m = c.points[i].x
		}
	}
	return m
}

func (c cellRun) avgY() float64 {
	if len(c.points) == 0 {
		return 0
	}
	s := 0.0
	for _, p := range c.points {
		s += p.y
	}
	return s / float64(len(c.points))
}

func (c cellRun) cellText() string {
	var b strings.Builder
	for i, p := range c.points {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strings.TrimSpace(p.seg.Text))
	}
	return strings.TrimSpace(b.String())
}

func (c cellRun) cellBBox() *BBox {
	if len(c.points) == 0 {
		return nil
	}
	left, right := c.points[0].bbox.Left, c.points[0].bbox.Right
	top, bottom := c.points[0].bbox.Top, c.points[0].bbox.Bottom
	for i := 1; i < len(c.points); i++ {
		bb := c.points[i].bbox
		if bb == nil {
			continue
		}
		left = math.Min(left, bb.Left)
		right = math.Max(right, bb.Right)
		top = math.Max(top, bb.Top)
		bottom = math.Min(bottom, bb.Bottom)
	}
	return &BBox{Left: left, Right: right, Top: top, Bottom: bottom}
}

func clusterRowIntoCellsByX(row []textPoint) []cellRun {
	if len(row) == 0 {
		return nil
	}
	sort.Slice(row, func(i, j int) bool { return row[i].x < row[j].x })
	if len(row) == 1 {
		return []cellRun{{points: row}}
	}
	gaps := make([]float64, 0, len(row)-1)
	for i := 1; i < len(row); i++ {
		gaps = append(gaps, row[i].x-row[i-1].x)
	}
	sort.Float64s(gaps)
	med := gaps[len(gaps)/2]
	split := math.Max(0.012, math.Min(0.06, med*2.5))

	var cells []cellRun
	cur := []textPoint{row[0]}
	for i := 1; i < len(row); i++ {
		gap := row[i].x - row[i-1].x
		if gap > split {
			cells = append(cells, cellRun{points: cur})
			cur = []textPoint{row[i]}
		} else {
			cur = append(cur, row[i])
		}
	}
	cells = append(cells, cellRun{points: cur})

	if len(cells) >= 2 {
		return cells
	}
	return splitRowByLargestGaps(row, 0.018)
}

// splitRowByLargestGaps recursively splits a dense row by largest x gaps when
// median heuristics fail to separate columns.
func splitRowByLargestGaps(row []textPoint, minGap float64) []cellRun {
	if len(row) < 2 {
		if len(row) == 1 {
			return []cellRun{{points: row}}
		}
		return nil
	}
	sort.Slice(row, func(i, j int) bool { return row[i].x < row[j].x })
	maxGap := 0.0
	maxAt := 0
	for i := 1; i < len(row); i++ {
		g := row[i].x - row[i-1].x
		if g > maxGap {
			maxGap = g
			maxAt = i
		}
	}
	if maxGap < minGap {
		return []cellRun{{points: row}}
	}
	left := append([]textPoint(nil), row[:maxAt]...)
	right := append([]textPoint(nil), row[maxAt:]...)
	out := append(splitRowByLargestGaps(left, minGap), splitRowByLargestGaps(right, minGap)...)
	if len(out) < 2 {
		return []cellRun{{points: row}}
	}
	return out
}

func splitRowCellsIntoTableBlocks(rowCells [][]cellRun) [][][]cellRun {
	if len(rowCells) < 2 {
		return nil
	}
	var blocks [][][]cellRun
	var cur [][]cellRun
	flush := func() {
		if len(cur) >= 2 {
			blocks = append(blocks, cur)
		}
		cur = nil
	}
	for i, row := range rowCells {
		if !rowLooksLikeTableV2(row) {
			flush()
			continue
		}
		if len(cur) > 0 && i > 0 {
			prevY := rowAvgY(rowCells[i-1])
			curY := rowAvgY(row)
			if math.Abs(prevY-curY) > rowBreakYGap {
				flush()
			}
		}
		cur = append(cur, row)
	}
	flush()
	return blocks
}

func rowLooksLikeTableV2(row []cellRun) bool {
	nonEmpty := 0
	shortOrNumeric := 0
	for _, cell := range row {
		text := strings.TrimSpace(cell.cellText())
		if text == "" || isSeparatorOnlyText(text) {
			continue
		}
		nonEmpty++
		if cellTextHasNumericSignal(text) || cellTextLooksCompact(text) {
			shortOrNumeric++
		}
	}
	if nonEmpty < 2 {
		return false
	}
	if nonEmpty >= 3 {
		return shortOrNumeric >= 2
	}
	return shortOrNumeric == 2
}

func cellTextLooksCompact(text string) bool {
	fields := strings.Fields(text)
	if len(fields) <= 2 {
		return true
	}
	return len([]rune(text)) <= 24
}

func cellTextHasNumericSignal(text string) bool {
	for _, r := range text {
		if (r >= '0' && r <= '9') || r == '%' || r == '€' || r == '$' || r == '£' {
			return true
		}
	}
	return false
}

func cellTextHasMeasureSignal(text string) bool {
	if strings.ContainsAny(text, "%€$£") {
		return true
	}
	for i, r := range text {
		if r != '.' {
			continue
		}
		prevDigit := i > 0 && text[i-1] >= '0' && text[i-1] <= '9'
		nextDigit := i+1 < len(text) && text[i+1] >= '0' && text[i+1] <= '9'
		if prevDigit && nextDigit {
			return true
		}
	}
	return false
}

func isSeparatorOnlyText(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return true
	}
	nonSep := 0
	for _, r := range trimmed {
		switch r {
		case '-', '_', '.', '·', '|', ':', ' ':
			continue
		default:
			nonSep++
		}
	}
	return nonSep == 0
}

func rowAvgY(cells []cellRun) float64 {
	var s float64
	n := 0
	for _, c := range cells {
		for _, p := range c.points {
			s += p.y
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return s / float64(n)
}

func buildTableChunkV2(pageIndex, tableIdx int, rows [][]cellRun) *Chunk {
	if len(rows) < 2 {
		return nil
	}
	colAnchors := deriveColumnAnchorsFromCellRows(rows)
	if len(colAnchors) < 2 {
		return nil
	}
	if !isLikelyTableV2Rows(rows, colAnchors) {
		return nil
	}

	nCol := len(colAnchors)
	nRow := len(rows)
	cellsOut := make([]map[string]any, 0, nRow*nCol)
	lineTexts := make([]string, 0, nRow)
	left, right, top, bottom := 1.0, 0.0, 0.0, 1.0

	for rIdx, row := range rows {
		rowTextParts := make([]string, nCol)
		acc := make([]string, nCol)
		for _, cell := range row {
			ci := nearestColumnIndexV2(cell.minX(), colAnchors)
			if ci < 0 {
				continue
			}
			txt := cell.cellText()
			if acc[ci] != "" {
				acc[ci] = acc[ci] + " " + txt
			} else {
				acc[ci] = txt
			}
			bb := cell.cellBBox()
			if bb == nil {
				continue
			}
			left = math.Min(left, bb.Left)
			right = math.Max(right, bb.Right)
			top = math.Max(top, bb.Top)
			bottom = math.Min(bottom, bb.Bottom)
			cellsOut = append(cellsOut, map[string]any{
				"row":  rIdx,
				"col":  ci,
				"text": txt,
				"bbox": map[string]any{
					"left": bb.Left, "right": bb.Right, "top": bb.Top, "bottom": bb.Bottom,
				},
			})
		}
		for i := range rowTextParts {
			rowTextParts[i] = strings.TrimSpace(acc[i])
		}
		lineTexts = append(lineTexts, strings.Join(rowTextParts, " | "))
	}

	if len(cellsOut) < 4 || right <= left || top <= bottom {
		return nil
	}

	payload := map[string]any{
		"detection_mode": "geometry_token_v2",
		"row_count":      nRow,
		"column_count":   nCol,
		"cells":          cellsOut,
	}
	page0 := pageIndex - 1
	if page0 < 0 {
		page0 = 0
	}
	html, _ := HTMLTableWithIDsFromPayload(payload, page0)
	if html == "" {
		html = strings.Join(lineTexts, "\n")
	}

	return &Chunk{
		ChunkID:    "table_" + strconv.Itoa(tableIdx),
		Type:       "table",
		Text:       html,
		PageIndex:  pageIndex,
		Order:      90000 + tableIdx,
		Source:     "native_pdf",
		Confidence: tableV2Confidence,
		BBox: &BBox{
			Left: round(left, 6), Right: round(right, 6), Top: round(top, 6), Bottom: round(bottom, 6),
		},
		Payload: payload,
	}
}

func deriveColumnAnchorsFromCellRows(rows [][]cellRun) []float64 {
	// If every micro-cell minX participates in clustering, adjacent distances
	// often fall below geomColMergeTol and collapse page-width columns into one anchor.
	// Each row now contributes only its min and max cell minX to preserve left/right columns.
	var xs []float64
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		if len(row) == 1 {
			xs = append(xs, row[0].minX())
			continue
		}
		mn, mx := row[0].minX(), row[0].minX()
		for _, c := range row[1:] {
			x := c.minX()
			if x < mn {
				mn = x
			}
			if x > mx {
				mx = x
			}
		}
		xs = append(xs, mn, mx)
	}
	if len(xs) == 0 {
		return nil
	}
	sort.Float64s(xs)
	anchors := make([]float64, 0, len(xs))
	for _, x := range xs {
		if len(anchors) == 0 || math.Abs(x-anchors[len(anchors)-1]) > geomColMergeTol {
			anchors = append(anchors, x)
			continue
		}
		anchors[len(anchors)-1] = (anchors[len(anchors)-1] + x) / 2
	}
	return anchors
}

func isLikelyTableV2Rows(rows [][]cellRun, anchors []float64) bool {
	if len(rows) < 2 || len(anchors) < 2 {
		return false
	}
	covered := 0
	maxCols := 0
	rowsWithMeasure := 0
	rowsWithThreeCols := 0
	for _, row := range rows {
		cols := map[int]struct{}{}
		rowHasMeasure := false
		for _, cell := range row {
			text := strings.TrimSpace(cell.cellText())
			if text == "" || isSeparatorOnlyText(text) {
				continue
			}
			ci := nearestColumnIndexV2(cell.minX(), anchors)
			if ci >= 0 {
				cols[ci] = struct{}{}
			}
			if cellTextHasMeasureSignal(text) {
				rowHasMeasure = true
			}
		}
		if rowHasMeasure {
			rowsWithMeasure++
		}
		if len(cols) > maxCols {
			maxCols = len(cols)
		}
		if len(cols) >= 3 {
			rowsWithThreeCols++
		}
		if len(cols) >= 2 {
			covered++
		}
	}
	if covered < 2 || maxCols < 2 {
		return false
	}
	if len(anchors) == 2 && (anchors[1]-anchors[0]) < 0.10 {
		return false
	}
	if maxCols == 2 && rowsWithMeasure == 0 {
		return false
	}
	if maxCols >= 3 && len(rows) <= 3 && rowsWithThreeCols == 0 {
		return false
	}
	if maxCols >= 3 && len(rows) > 4 && rowsWithThreeCols < 2 && rowsWithMeasure < 2 {
		return false
	}
	return true
}

func nearestColumnIndexV2(x float64, cols []float64) int {
	if len(cols) == 0 {
		return -1
	}
	best := 0
	bestDist := math.Abs(x - cols[0])
	for i := 1; i < len(cols); i++ {
		d := math.Abs(x - cols[i])
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	if bestDist > geomNearestColV2 {
		return -1
	}
	return best
}
