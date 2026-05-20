package local

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	extractcommon "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

func recoverWithPopplerTextLayer(ctx context.Context, filename string, data []byte, pageCount int, diag map[string]any) (*extractcommon.DocumentResult, error) {
	started := time.Now()
	pages, err := runPopplerXML(ctx, filename, data, pageCount)
	if err != nil {
		return nil, err
	}
	doc := buildPopplerRecoveryDocument(filename, pages, diag)
	if doc.PageCount == 0 && pageCount > 0 {
		doc.PageCount = pageCount
		doc.Pages = makeFallbackPages(pageCount)
	}
	if doc.Diagnostics == nil {
		doc.Diagnostics = diag
	}
	doc.Diagnostics["poppler_text_recovery"] = map[string]any{
		"applied":     len(doc.Chunks) > 0,
		"chunks":      len(doc.Chunks),
		"pages":       doc.PageCount,
		"duration_ms": time.Since(started).Milliseconds(),
		"source":      "pdftohtml_xml",
	}
	if len(doc.Chunks) == 0 {
		return doc, fmt.Errorf("poppler text recovery produced no chunks")
	}
	return doc, nil
}

type popplerRecoveryItem struct {
	typ  string
	text string
	box  extractcommon.BBox
	page int
}

func buildPopplerRecoveryDocument(filename string, pages []popplerXMLPage, diag map[string]any) *extractcommon.DocumentResult {
	if diag == nil {
		diag = map[string]any{}
	}
	out := &extractcommon.DocumentResult{
		DocID:       newID(),
		FileName:    filename,
		PageCount:   len(pages),
		Pages:       make([]extractcommon.Page, 0, len(pages)),
		Source:      "native+poppler:text",
		Diagnostics: diag,
	}
	var markdown []string
	for seq, page := range pages {
		pageIndex := page.Number - 1
		if pageIndex < 0 {
			pageIndex = seq
		}
		out.Pages = append(out.Pages, extractcommon.Page{
			PageIndex: pageIndex,
			Width:     page.Width,
			Height:    page.Height,
		})
		items := make([]popplerRecoveryItem, 0, len(page.Lines)+len(page.Figs))
		for _, line := range mergePopplerRecoveryLines(page.Lines) {
			text := strings.TrimSpace(line.Text)
			if text == "" {
				continue
			}
			items = append(items, popplerRecoveryItem{typ: "text", text: text, box: line.Box, page: pageIndex})
		}
		for _, fig := range page.Figs {
			items = append(items, popplerRecoveryItem{typ: "figure", text: "Embedded image", box: fig.Box, page: pageIndex})
		}
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].box.Top != items[j].box.Top {
				return items[i].box.Top < items[j].box.Top
			}
			return items[i].box.Left < items[j].box.Left
		})
		for _, item := range items {
			ord := len(out.Chunks) + 1
			chunkType := classifyPopplerRecoveryItem(item)
			out.Chunks = append(out.Chunks, extractcommon.Chunk{
				ID:         fmt.Sprintf("poppler-recovery-%d", ord),
				Type:       chunkType,
				Page:       item.page,
				BBox:       &extractcommon.BBox{Left: item.box.Left, Top: item.box.Top, Right: item.box.Right, Bottom: item.box.Bottom},
				Text:       item.text,
				Markdown:   item.text,
				Ordinal:    ord,
				Confidence: 0.82,
				Precision:  "reliable",
				Payload: map[string]any{
					"extraction_method": "poppler_text_recovery",
					"source_trace":      "full_document_failed.poppler_xml",
				},
			})
			if chunkType != "figure" {
				markdown = append(markdown, item.text)
			}
		}
	}
	out.Markdown = strings.Join(markdown, "\n\n")
	return out
}

func classifyPopplerRecoveryItem(item popplerRecoveryItem) string {
	if item.typ == "figure" {
		return "figure"
	}
	text := strings.TrimSpace(item.text)
	if text == "" {
		return "text"
	}
	height := item.box.Bottom - item.box.Top
	width := item.box.Right - item.box.Left
	if height >= 0.028 && width <= 0.85 && len([]rune(text)) <= 80 {
		return "heading"
	}
	return "text"
}

func mergePopplerRecoveryLines(lines []popplerTextLine) []popplerTextLine {
	if len(lines) <= 1 {
		return lines
	}
	rows := make([][]popplerTextLine, 0, len(lines))
	sorted := append([]popplerTextLine(nil), lines...)
	sort.SliceStable(sorted, func(i, j int) bool {
		ci := bboxCenterY(sorted[i].Box)
		cj := bboxCenterY(sorted[j].Box)
		if ci != cj {
			return ci < cj
		}
		return sorted[i].Box.Left < sorted[j].Box.Left
	})
	for _, line := range sorted {
		if strings.TrimSpace(line.Text) == "" {
			continue
		}
		center := bboxCenterY(line.Box)
		height := line.Box.Bottom - line.Box.Top
		bestIdx := -1
		bestDelta := 1.0
		for i := range rows {
			rowCenter := popplerRecoveryRowCenter(rows[i])
			rowHeight := popplerRecoveryRowHeight(rows[i])
			tol := maxFloat(0.006, maxFloat(height, rowHeight)*0.85)
			delta := absFloat(center - rowCenter)
			if delta <= tol && delta < bestDelta {
				bestIdx = i
				bestDelta = delta
			}
		}
		if bestIdx < 0 {
			rows = append(rows, []popplerTextLine{line})
			continue
		}
		rows[bestIdx] = append(rows[bestIdx], line)
	}
	out := make([]popplerTextLine, 0, len(rows))
	for _, row := range rows {
		sort.SliceStable(row, func(i, j int) bool {
			if row[i].Box.Left != row[j].Box.Left {
				return row[i].Box.Left < row[j].Box.Left
			}
			return row[i].Box.Top < row[j].Box.Top
		})
		out = append(out, splitPopplerRecoveryRow(row)...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Box.Top != out[j].Box.Top {
			return out[i].Box.Top < out[j].Box.Top
		}
		return out[i].Box.Left < out[j].Box.Left
	})
	return out
}

func splitPopplerRecoveryRow(row []popplerTextLine) []popplerTextLine {
	if len(row) <= 1 {
		return row
	}
	var out []popplerTextLine
	var seg []popplerTextLine
	flush := func() {
		if len(seg) == 0 {
			return
		}
		out = append(out, joinPopplerRecoverySegment(seg))
		seg = nil
	}
	for _, line := range row {
		if len(seg) > 0 {
			prev := seg[len(seg)-1]
			gap := line.Box.Left - prev.Box.Right
			if gap > 0.035 {
				flush()
			}
		}
		seg = append(seg, line)
	}
	flush()
	return out
}

func joinPopplerRecoverySegment(seg []popplerTextLine) popplerTextLine {
	if len(seg) == 0 {
		return popplerTextLine{}
	}
	box := seg[0].Box
	parts := make([]string, 0, len(seg))
	for _, line := range seg {
		txt := strings.TrimSpace(line.Text)
		if txt == "" {
			continue
		}
		parts = append(parts, txt)
		box.Left = minFloat(box.Left, line.Box.Left)
		box.Top = minFloat(box.Top, line.Box.Top)
		box.Right = maxFloat(box.Right, line.Box.Right)
		box.Bottom = maxFloat(box.Bottom, line.Box.Bottom)
	}
	return popplerTextLine{Text: joinPopplerRecoveryText(parts), Box: box}
}

func joinPopplerRecoveryText(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		if b.Len() > 0 && needsPopplerRecoverySpace(lastRuneString(b.String()), firstRuneString(part)) {
			b.WriteByte(' ')
		}
		b.WriteString(part)
	}
	return b.String()
}

func needsPopplerRecoverySpace(prev, next string) bool {
	if prev == "" || next == "" {
		return false
	}
	pr := []rune(prev)[0]
	nr := []rune(next)[0]
	return isASCIIWordRune(pr) && isASCIIWordRune(nr)
}

func firstRuneString(s string) string {
	for _, r := range s {
		return string(r)
	}
	return ""
}

func lastRuneString(s string) string {
	var last rune
	for _, r := range s {
		last = r
	}
	if last == 0 {
		return ""
	}
	return string(last)
}

func isASCIIWordRune(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

func popplerRecoveryRowCenter(row []popplerTextLine) float64 {
	if len(row) == 0 {
		return 0
	}
	var sum float64
	for _, line := range row {
		sum += bboxCenterY(line.Box)
	}
	return sum / float64(len(row))
}

func popplerRecoveryRowHeight(row []popplerTextLine) float64 {
	var height float64
	for _, line := range row {
		height = maxFloat(height, line.Box.Bottom-line.Box.Top)
	}
	return height
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
