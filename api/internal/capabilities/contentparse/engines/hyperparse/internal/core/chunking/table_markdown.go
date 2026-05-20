package chunking

import (
	"fmt"
	"strings"
)

// MarkdownTableFromNativePayload builds a GFM pipe table from native table
// payload fields such as row_count, column_count, and cells.
// The first row is rendered as header, followed by data rows.
func MarkdownTableFromNativePayload(p map[string]any) string {
	if p == nil {
		return ""
	}
	rowCount := payloadTableInt(p["row_count"])
	colCount := payloadTableInt(p["column_count"])
	if rowCount < 1 || colCount < 1 {
		return ""
	}
	list := cellsSliceFromPayload(p["cells"])
	if len(list) == 0 {
		return ""
	}
	return MarkdownTableFromMergedCells(rowCount, colCount, list)
}

func payloadTableInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int32:
		return int(x)
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}

func cellsSliceFromPayload(raw any) []map[string]any {
	switch v := raw.(type) {
	case []map[string]any:
		return v
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, it := range v {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

// MarkdownTableFromMergedCells merges row/col/text cells into a grid before rendering Markdown.
func MarkdownTableFromMergedCells(rowCount, colCount int, cells []map[string]any) string {
	grid := mergeTableCellsToGrid(rowCount, colCount, cells)
	return MarkdownTableFromGrid(grid)
}

func mergeTableCellsToGrid(rowCount, colCount int, cells []map[string]any) [][]string {
	grid := make([][]string, rowCount)
	for i := range grid {
		grid[i] = make([]string, colCount)
	}
	for _, c := range cells {
		r := payloadTableInt(c["row"])
		k := payloadTableInt(c["col"])
		if r < 0 || r >= rowCount || k < 0 || k >= colCount {
			continue
		}
		txt := strings.TrimSpace(fmt.Sprint(c["text"]))
		if txt == "" {
			continue
		}
		if grid[r][k] != "" {
			grid[r][k] += " " + txt
		} else {
			grid[r][k] = txt
		}
	}
	return grid
}

// MarkdownTableFromGrid renders a row-major grid as a GFM table.
func MarkdownTableFromGrid(grid [][]string) string {
	if len(grid) == 0 {
		return ""
	}
	cols := 0
	for _, r := range grid {
		if len(r) > cols {
			cols = len(r)
		}
	}
	if cols == 0 {
		return ""
	}
	var b strings.Builder
	writeRow := func(r []string) {
		b.WriteString("| ")
		for c := 0; c < cols; c++ {
			cell := ""
			if c < len(r) {
				cell = r[c]
			}
			b.WriteString(escapeMarkdownTableCell(cell))
			if c < cols-1 {
				b.WriteString(" | ")
			} else {
				b.WriteString(" |")
			}
		}
		b.WriteByte('\n')
	}
	writeSep := func() {
		for c := 0; c < cols; c++ {
			if c == 0 {
				b.WriteString("|")
			}
			b.WriteString(" --- |")
		}
		b.WriteByte('\n')
	}

	writeRow(padTableRow(grid[0], cols))
	writeSep()
	for ri := 1; ri < len(grid); ri++ {
		writeRow(padTableRow(grid[ri], cols))
	}
	return strings.TrimSpace(b.String())
}

func padTableRow(r []string, cols int) []string {
	out := make([]string, cols)
	for i := 0; i < cols && i < len(r); i++ {
		out[i] = r[i]
	}
	return out
}

func escapeMarkdownTableCell(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}
