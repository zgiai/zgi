package chunking

import (
	"fmt"
	"strings"
)

// HTMLTableWithIDsFromPayload builds table/tr/td HTML with ids from a native
// table payload. Ids look like "0-1" or "0-2" and match cell grounding keys.
func HTMLTableWithIDsFromPayload(p map[string]any, page0 int) (html string, cellGrounding map[string]map[string]any) {
	if p == nil {
		return "", nil
	}
	rowCount := payloadTableInt(p["row_count"])
	colCount := payloadTableInt(p["column_count"])
	if rowCount < 1 || colCount < 1 {
		return "", nil
	}
	list := cellsSliceFromPayload(p["cells"])
	if len(list) == 0 {
		return "", nil
	}
	grid := mergeTableCellsToGrid(rowCount, colCount, list)
	boxes := make([][]map[string]any, rowCount)
	for i := range boxes {
		boxes[i] = make([]map[string]any, colCount)
	}
	for _, c := range list {
		r := payloadTableInt(c["row"])
		k := payloadTableInt(c["col"])
		if r < 0 || r >= rowCount || k < 0 || k >= colCount {
			continue
		}
		if bb, ok := c["bbox"].(map[string]any); ok {
			if n := normalizeHTMLCellBBox(bb); n != nil {
				boxes[r][k] = n
			}
		}
	}
	return renderHTMLTableWithIDs(page0, grid, boxes)
}

func normalizeHTMLCellBBox(bb map[string]any) map[string]any {
	l := htmlPayloadF64(bb["left"])
	r := htmlPayloadF64(bb["right"])
	t := htmlPayloadF64(bb["top"])
	b := htmlPayloadF64(bb["bottom"])
	if l == 0 && r == 0 && t == 0 && b == 0 {
		return nil
	}
	top := round(1-t, 6)
	bottom := round(1-b, 6)
	if bottom < top {
		top, bottom = bottom, top
	}
	return map[string]any{
		"left":   round(l, 6),
		"right":  round(r, 6),
		"top":    top,
		"bottom": bottom,
	}
}

func htmlPayloadF64(v any) float64 {
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

func tableIDHex(v int) string {
	if v <= 0 {
		return "0"
	}
	return strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%x", v)))
}

func renderHTMLTableWithIDs(page0 int, grid [][]string, boxes [][]map[string]any) (string, map[string]map[string]any) {
	if len(grid) == 0 {
		return "", nil
	}
	rows := len(grid)
	cols := 0
	for _, rw := range grid {
		if len(rw) > cols {
			cols = len(rw)
		}
	}
	if cols == 0 {
		return "", nil
	}
	ref := make(map[string]map[string]any)
	next := 1
	tableID := fmt.Sprintf("%d-%s", page0, tableIDHex(next))
	next++
	var b strings.Builder
	b.WriteString(`<table id="`)
	b.WriteString(tableID)
	b.WriteString(`">`)
	b.WriteByte('\n')
	for r := 0; r < rows; r++ {
		b.WriteString("<tr>")
		for c := 0; c < cols; c++ {
			cellID := fmt.Sprintf("%d-%s", page0, tableIDHex(next))
			next++
			cell := ""
			if c < len(grid[r]) {
				cell = grid[r][c]
			}
			b.WriteString(`<td id="`)
			b.WriteString(cellID)
			b.WriteString(`">`)
			b.WriteString(htmlEscTableCell(cell))
			b.WriteString("</td>")
			if r < len(boxes) && c < len(boxes[r]) && boxes[r][c] != nil {
				ref[cellID] = map[string]any{
					"box":  boxes[r][c],
					"page": page0,
					"position": map[string]any{
						"row":     r,
						"col":     c,
						"rowspan": 1,
						"colspan": 1,
					},
				}
			}
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</table>")
	return b.String(), ref
}

func htmlEscTableCell(s string) string {
	return strings.NewReplacer(
		`&`, "&amp;",
		`<`, "&lt;",
		`>`, "&gt;",
		`"`, "&quot;",
	).Replace(s)
}

// HTMLTableWithIDsFromGrid builds native-compatible HTML from cell text only,
// for sources such as VLM that do not provide per-cell geometry.
func HTMLTableWithIDsFromGrid(page0 int, grid [][]string) string {
	if len(grid) == 0 {
		return ""
	}
	cols := 0
	for _, rw := range grid {
		if len(rw) > cols {
			cols = len(rw)
		}
	}
	if cols == 0 {
		return ""
	}
	rows := len(grid)
	boxes := make([][]map[string]any, rows)
	for i := range boxes {
		boxes[i] = make([]map[string]any, cols)
	}
	html, _ := renderHTMLTableWithIDs(page0, grid, boxes)
	return html
}
