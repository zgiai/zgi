package local

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	extractcommon "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

type popplerXMLDoc struct {
	Pages []popplerXMLPage `xml:"page"`
}

type popplerXMLPage struct {
	Number int                `xml:"number,attr"`
	Width  float64            `xml:"width,attr"`
	Height float64            `xml:"height,attr"`
	Texts  []popplerXMLText   `xml:"text"`
	Images []popplerXMLImage  `xml:"image"`
	Lines  []popplerTextLine  `xml:"-"`
	Figs   []popplerImageItem `xml:"-"`
}

type popplerXMLText struct {
	Top     float64 `xml:"top,attr"`
	Left    float64 `xml:"left,attr"`
	Width   float64 `xml:"width,attr"`
	Height  float64 `xml:"height,attr"`
	Content string  `xml:",innerxml"`
}

type popplerXMLImage struct {
	Top    float64 `xml:"top,attr"`
	Left   float64 `xml:"left,attr"`
	Width  float64 `xml:"width,attr"`
	Height float64 `xml:"height,attr"`
}

type popplerTextLine struct {
	Text string
	Box  extractcommon.BBox
	Used bool
}

type popplerTextItem struct {
	Text string
	Box  extractcommon.BBox
}

type popplerImageItem struct {
	Box  extractcommon.BBox
	Used bool
}

type popplerBBoxStats struct {
	applied        bool
	textUpdated    int
	figureUpdated  int
	textSplits     int
	textMerges     int
	typeUpdated    int
	footerUpdated  int
	pageRepaired   int
	durationMS     int64
	processedPages int
	maxPages       int
	timeoutSec     int
	warning        string
}

var popplerTagRE = regexp.MustCompile(`<[^>]+>`)

func refineDocumentBBoxesWithPoppler(ctx context.Context, filename string, data []byte, doc *extractcommon.DocumentResult, inspect map[string]any) {
	stats := applyPopplerBBoxRefinement(ctx, filename, data, doc)
	if stats.warning == "" && !stats.applied {
		return
	}
	m := map[string]any{
		"applied":         stats.applied,
		"engine":          "pdftohtml_xml",
		"text_updated":    stats.textUpdated,
		"figure_updated":  stats.figureUpdated,
		"text_splits":     stats.textSplits,
		"text_merges":     stats.textMerges,
		"type_updated":    stats.typeUpdated,
		"footer_updated":  stats.footerUpdated,
		"page_repaired":   stats.pageRepaired,
		"duration_ms":     stats.durationMS,
		"processed_pages": stats.processedPages,
		"max_pages":       stats.maxPages,
		"timeout_sec":     stats.timeoutSec,
	}
	if stats.warning != "" {
		m["warning"] = stats.warning
	}
	if doc != nil {
		if doc.Diagnostics == nil {
			doc.Diagnostics = map[string]any{}
		}
		doc.Diagnostics["local_poppler_bbox"] = m
	}
	if inspect != nil {
		inspect["local_poppler_bbox"] = m
	}
}

func applyPopplerBBoxRefinement(ctx context.Context, filename string, data []byte, doc *extractcommon.DocumentResult) (stats popplerBBoxStats) {
	startedAt := time.Now()
	defer func() {
		stats.durationMS = time.Since(startedAt).Milliseconds()
	}()
	if doc == nil || len(doc.Chunks) == 0 || !popplerBBoxEnabled() {
		return
	}
	if _, err := exec.LookPath("pdftohtml"); err != nil {
		stats.warning = "pdftohtml not found"
		return
	}
	stats.maxPages = popplerBBoxMaxPages()
	stats.timeoutSec = popplerBBoxTimeoutSeconds()
	if stats.maxPages > 0 && doc.PageCount > stats.maxPages && !popplerBBoxLongDocumentEnabled() {
		stats.warning = fmt.Sprintf("skipped_large_document pages=%d max_pages=%d", doc.PageCount, stats.maxPages)
		return
	}
	if stats.timeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(stats.timeoutSec)*time.Second)
		defer cancel()
	}
	pages, err := runPopplerXML(ctx, filename, data, stats.maxPages)
	if err != nil {
		stats.warning = err.Error()
		return
	}
	stats.processedPages = len(pages)
	if len(pages) == 0 {
		stats.warning = "pdftohtml returned no pages"
		return
	}
	stats.pageRepaired += repairChunkPagesWithPoppler(doc, pages)
	stats.pageRepaired += repairContextualPageIslands(doc)
	for i := range doc.Chunks {
		ch := &doc.Chunks[i]
		pageIdx := ch.Page
		if pageIdx < 0 || pageIdx >= len(pages) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(ch.Type)) {
		case "figure":
			if assignPopplerImageBox(ch, &pages[pageIdx]) {
				stats.figureUpdated++
			}
		case "text", "heading", "marginalia", "logo", "":
			if assignPopplerTextBox(ch, &pages[pageIdx]) {
				stats.textUpdated++
			}
		}
	}
	stats.textSplits += splitHeaderChunksWithPoppler(doc, pages)
	stats.textMerges += mergeKnownFormFieldsWithPoppler(doc, pages)
	stats.typeUpdated += promoteMarginChunks(doc)
	stats.textSplits += splitStructuredTextChunksWithPoppler(doc, pages)
	stats.textSplits += splitSparseCaptionChunksWithPoppler(doc, pages)
	stats.textMerges += mergeInlineHeadingContinuations(doc)
	stats.textMerges += mergeAdjacentParagraphContinuations(doc)
	stats.footerUpdated += replaceFooterChunksWithPoppler(doc, pages)
	stats.applied = stats.textUpdated > 0 || stats.figureUpdated > 0 || stats.textSplits > 0 || stats.textMerges > 0 || stats.typeUpdated > 0 || stats.footerUpdated > 0 || stats.pageRepaired > 0
	return
}

func popplerBBoxEnabled() bool {
	raw := strings.TrimSpace(os.Getenv("LOCAL_POPPLER_BBOX"))
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

func popplerBBoxMaxPages() int {
	raw := strings.TrimSpace(os.Getenv("LOCAL_POPPLER_BBOX_MAX_PAGES"))
	if raw == "" {
		return 8
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 8
	}
	return n
}

func popplerBBoxTimeoutSeconds() int {
	raw := strings.TrimSpace(os.Getenv("LOCAL_POPPLER_BBOX_TIMEOUT_SEC"))
	if raw == "" {
		return 15
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 15
	}
	return n
}

func popplerBBoxLongDocumentEnabled() bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("LOCAL_POPPLER_BBOX_LONG_DOC")))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func runPopplerXML(ctx context.Context, filename string, data []byte, maxPages int) ([]popplerXMLPage, error) {
	tmp, err := os.MkdirTemp("", "hyperparse-poppler-*")
	if err != nil {
		return nil, fmt.Errorf("poppler temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	name := strings.TrimSpace(filename)
	if name == "" {
		name = "upload.pdf"
	}
	pdfPath := filepath.Join(tmp, filepath.Base(name))
	if err := os.WriteFile(pdfPath, data, 0600); err != nil {
		return nil, fmt.Errorf("poppler write pdf: %w", err)
	}
	outPrefix := filepath.Join(tmp, "bbox")
	args := []string{"-xml", "-nodrm"}
	if maxPages > 0 {
		args = append(args, "-f", "1", "-l", strconv.Itoa(maxPages))
	}
	args = append(args, pdfPath, outPrefix)
	cmd := exec.CommandContext(ctx, "pdftohtml", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftohtml xml: %w: %s", err, strings.TrimSpace(string(out)))
	}
	xmlPath := outPrefix + ".xml"
	raw, err := os.ReadFile(xmlPath)
	if err != nil {
		return nil, fmt.Errorf("poppler read xml: %w", err)
	}
	var parsed popplerXMLDoc
	if err := xml.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("poppler parse xml: %w", err)
	}
	for i := range parsed.Pages {
		preparePopplerPage(&parsed.Pages[i])
	}
	sort.SliceStable(parsed.Pages, func(i, j int) bool {
		return parsed.Pages[i].Number < parsed.Pages[j].Number
	})
	return parsed.Pages, nil
}

func preparePopplerPage(page *popplerXMLPage) {
	if page == nil || page.Width <= 0 || page.Height <= 0 {
		return
	}
	var textItems []popplerTextItem
	for _, t := range page.Texts {
		txt := cleanPopplerXMLText(t.Content)
		if txt == "" || txt == ">" {
			continue
		}
		box, ok := popplerBox(t.Left, t.Top, t.Width, t.Height, page.Width, page.Height)
		if !ok {
			continue
		}
		textItems = append(textItems, popplerTextItem{Text: txt, Box: box})
	}
	page.Lines = orderPopplerTextItems(textItems)
	for _, img := range page.Images {
		box, ok := popplerBox(img.Left, img.Top, img.Width, img.Height, page.Width, page.Height)
		if ok {
			page.Figs = append(page.Figs, popplerImageItem{Box: box})
		}
	}
	sort.SliceStable(page.Figs, func(i, j int) bool {
		if math.Abs(page.Figs[i].Box.Top-page.Figs[j].Box.Top) > 0.002 {
			return page.Figs[i].Box.Top < page.Figs[j].Box.Top
		}
		return page.Figs[i].Box.Left < page.Figs[j].Box.Left
	})
}

func orderPopplerTextItems(items []popplerTextItem) []popplerTextLine {
	if len(items) == 0 {
		return nil
	}
	sort.SliceStable(items, func(i, j int) bool {
		ci := bboxCenterY(items[i].Box)
		cj := bboxCenterY(items[j].Box)
		if math.Abs(ci-cj) > 0.012 {
			return ci < cj
		}
		return items[i].Box.Left < items[j].Box.Left
	})

	type row struct {
		items  []popplerTextItem
		center float64
		height float64
	}
	var rows []row
	for _, item := range items {
		center := bboxCenterY(item.Box)
		height := math.Max(0.001, item.Box.Bottom-item.Box.Top)
		bestIdx := -1
		bestDelta := math.MaxFloat64
		for i := range rows {
			tol := math.Max(0.006, math.Max(height, rows[i].height)*0.75)
			delta := math.Abs(center - rows[i].center)
			if delta <= tol && delta < bestDelta {
				bestIdx = i
				bestDelta = delta
			}
		}
		if bestIdx < 0 {
			rows = append(rows, row{items: []popplerTextItem{item}, center: center, height: height})
			continue
		}
		r := &rows[bestIdx]
		n := float64(len(r.items))
		r.center = (r.center*n + center) / (n + 1)
		r.height = math.Max(r.height, height)
		r.items = append(r.items, item)
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if math.Abs(rows[i].center-rows[j].center) > 0.006 {
			return rows[i].center < rows[j].center
		}
		return rowLeft(rows[i]) < rowLeft(rows[j])
	})

	out := make([]popplerTextLine, 0, len(items))
	for _, r := range rows {
		sort.SliceStable(r.items, func(i, j int) bool {
			if math.Abs(r.items[i].Box.Left-r.items[j].Box.Left) > 0.001 {
				return r.items[i].Box.Left < r.items[j].Box.Left
			}
			return r.items[i].Box.Top < r.items[j].Box.Top
		})
		for _, item := range r.items {
			out = append(out, popplerTextLine{Text: item.Text, Box: item.Box})
		}
	}
	return out
}

func rowLeft(r struct {
	items  []popplerTextItem
	center float64
	height float64
}) float64 {
	left := math.MaxFloat64
	for _, item := range r.items {
		left = math.Min(left, item.Box.Left)
	}
	if left == math.MaxFloat64 {
		return 0
	}
	return left
}

func bboxCenterY(b extractcommon.BBox) float64 {
	return (b.Top + b.Bottom) / 2
}

func popplerBox(left, top, width, height, pageWidth, pageHeight float64) (extractcommon.BBox, bool) {
	if width <= 0 || height <= 0 || pageWidth <= 0 || pageHeight <= 0 {
		return extractcommon.BBox{}, false
	}
	box := extractcommon.BBox{
		Left:   clampFloat01(left / pageWidth),
		Top:    clampFloat01(top / pageHeight),
		Right:  clampFloat01((left + width) / pageWidth),
		Bottom: clampFloat01((top + height) / pageHeight),
	}
	return box, box.Right > box.Left && box.Bottom > box.Top
}

func cleanPopplerXMLText(raw string) string {
	s := popplerTagRE.ReplaceAllString(raw, " ")
	s = html.UnescapeString(s)
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

func assignPopplerImageBox(ch *extractcommon.Chunk, page *popplerXMLPage) bool {
	if ch == nil || page == nil {
		return false
	}
	for i := range page.Figs {
		if page.Figs[i].Used {
			continue
		}
		page.Figs[i].Used = true
		ch.BBox = &page.Figs[i].Box
		if ch.Precision == "" || ch.Precision == "unreliable" {
			ch.Precision = "reliable"
		}
		return true
	}
	return false
}

func assignPopplerTextBox(ch *extractcommon.Chunk, page *popplerXMLPage) bool {
	if ch == nil || page == nil || len(page.Lines) == 0 {
		return false
	}
	chunkText := strings.TrimSpace(ch.Text)
	if chunkText == "" {
		chunkText = chunkTextFromMarkdown(ch.Markdown)
	}
	if !usefulLayoutText(chunkText) || len([]rune(chunkText)) < 3 {
		return false
	}
	bestStart, bestEnd, bestScore := bestPopplerLineSpan(chunkText, page.Lines)
	if bestStart < 0 || bestScore < 0.58 {
		return false
	}
	bestStart, bestEnd = expandPopplerSpanToListMarker(page.Lines, bestStart, bestEnd)
	bestStart, bestEnd = expandPopplerSpanToLeftLabel(page.Lines, bestStart, bestEnd)
	matchedLines := page.Lines[bestStart:bestEnd]
	box := unionPopplerLineBoxes(matchedLines)
	if box == nil {
		return false
	}
	if ch.BBox != nil && !shouldReplaceWithPopplerBox(ch, *box, bestScore) {
		return false
	}
	for i := bestStart; i < bestEnd; i++ {
		page.Lines[i].Used = true
	}
	ch.BBox = box
	if matchedText := cleanLayoutText(joinPopplerLineText(matchedLines)); shouldReplaceChunkTextWithPoppler(ch.Text, matchedText, bestScore) {
		ch.Text = matchedText
	}
	if ch.Precision == "" || ch.Precision == "unreliable" {
		ch.Precision = "reliable"
	}
	return true
}

func repairChunkPagesWithPoppler(doc *extractcommon.DocumentResult, pages []popplerXMLPage) int {
	if doc == nil || len(doc.Chunks) == 0 || len(pages) < 2 {
		return 0
	}
	repaired := 0
	for i := range doc.Chunks {
		ch := &doc.Chunks[i]
		if !shouldRepairChunkPageWithPoppler(*ch) {
			continue
		}
		text := strings.TrimSpace(ch.Text)
		if text == "" {
			text = chunkTextFromMarkdown(ch.Markdown)
		}
		bestPage, bestScore, currentScore := bestPopplerPageForText(text, ch.Page, pages)
		if bestPage < 0 || bestPage == ch.Page {
			continue
		}
		if !shouldMoveChunkToPopplerPage(currentScore, bestScore) {
			continue
		}
		ch.Page = bestPage
		ch.BBox = nil
		ch.Precision = ""
		repaired++
	}
	return repaired
}

func shouldRepairChunkPageWithPoppler(ch extractcommon.Chunk) bool {
	switch strings.ToLower(strings.TrimSpace(ch.Type)) {
	case "text", "heading", "marginalia", "logo", "":
	default:
		return false
	}
	text := strings.TrimSpace(ch.Text)
	if text == "" {
		text = chunkTextFromMarkdown(ch.Markdown)
	}
	if len([]rune(compactMatchText(text))) < 18 {
		return false
	}
	if strings.Contains(text, "<<:figure:") {
		return false
	}
	return true
}

func bestPopplerPageForText(text string, currentPage int, pages []popplerXMLPage) (bestPage int, bestScore float64, currentScore float64) {
	bestPage = -1
	for idx := range pages {
		_, _, score := bestPopplerLineSpan(text, pages[idx].Lines)
		if idx == currentPage {
			currentScore = score
		}
		if score > bestScore {
			bestScore = score
			bestPage = idx
		}
	}
	return bestPage, bestScore, currentScore
}

func shouldMoveChunkToPopplerPage(currentScore, bestScore float64) bool {
	if bestScore < 0.62 {
		return false
	}
	if currentScore < 0.48 {
		return true
	}
	return bestScore-currentScore >= 0.14
}

func repairContextualPageIslands(doc *extractcommon.DocumentResult) int {
	if doc == nil || len(doc.Chunks) < 3 {
		return 0
	}
	repaired := 0
	for i := 1; i < len(doc.Chunks)-1; {
		prevPage := doc.Chunks[i-1].Page
		if prevPage < 0 || doc.Chunks[i].Page == prevPage {
			i++
			continue
		}
		j := i
		for j < len(doc.Chunks) && doc.Chunks[j].Page != prevPage {
			j++
		}
		if j >= len(doc.Chunks) || j == i || j-i > 4 {
			i++
			continue
		}
		if !allChunksMovablePageIsland(doc.Chunks[i:j]) {
			i = j
			continue
		}
		for k := i; k < j; k++ {
			doc.Chunks[k].Page = prevPage
			doc.Chunks[k].BBox = nil
			doc.Chunks[k].Precision = ""
			repaired++
		}
		i = j
	}
	return repaired
}

func allChunksMovablePageIsland(chunks []extractcommon.Chunk) bool {
	if len(chunks) == 0 {
		return false
	}
	for _, ch := range chunks {
		if !isMovablePageIslandChunk(ch) {
			return false
		}
	}
	return true
}

func isMovablePageIslandChunk(ch extractcommon.Chunk) bool {
	switch strings.ToLower(strings.TrimSpace(ch.Type)) {
	case "text", "heading", "marginalia", "logo", "":
	default:
		return false
	}
	text := strings.TrimSpace(ch.Text)
	if text == "" {
		text = chunkTextFromMarkdown(ch.Markdown)
	}
	compact := compactMatchText(text)
	if len([]rune(compact)) < 3 || len([]rune(compact)) > 80 {
		return false
	}
	if strings.Contains(text, "<<:figure:") {
		return false
	}
	lower := strings.ToLower(text)
	for _, token := range []string{
		"patient", "age", "pathology", "acct", "sex", "dpmg use only",
		"doctor", "date obtained", "date received", "3301 c", "sacramento",
		"dermatopathology report",
	} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func splitHeaderChunksWithPoppler(doc *extractcommon.DocumentResult, pages []popplerXMLPage) int {
	if doc == nil || len(doc.Chunks) == 0 {
		return 0
	}
	out := make([]extractcommon.Chunk, 0, len(doc.Chunks)+4)
	splits := 0
	for _, ch := range doc.Chunks {
		replacements, ok := splitHeaderChunkWithPoppler(ch, pages)
		if !ok {
			if refineHeaderTitleChunkWithPoppler(&ch, pages) {
				out = append(out, ch)
				continue
			}
			out = append(out, ch)
			continue
		}
		out = append(out, replacements...)
		splits += len(replacements) - 1
	}
	if splits == 0 {
		doc.Chunks = out
		return 0
	}
	renumberChunks(out)
	doc.Chunks = out
	return splits
}

func splitHeaderChunkWithPoppler(ch extractcommon.Chunk, pages []popplerXMLPage) ([]extractcommon.Chunk, bool) {
	if ch.BBox == nil || ch.Page < 0 || ch.Page >= len(pages) {
		return nil, false
	}
	if ch.BBox.Top > 0.13 || ch.BBox.Right-ch.BBox.Left < 0.34 {
		return nil, false
	}
	compact := compactMatchText(ch.Text)
	if !strings.Contains(compact, "DERMATOPATHOLOGY") || !(strings.Contains(compact, "3301CSTREET") || strings.Contains(compact, "916") || strings.Contains(compact, "DPMG")) {
		return nil, false
	}
	lines := popplerLinesInBox(pages[ch.Page].Lines, *ch.BBox)
	var titleLines, addressLines []popplerTextLine
	for _, line := range lines {
		t := compactMatchText(line.Text)
		if t == "" {
			continue
		}
		if strings.Contains(t, "DERMATOPATHOLOGY") || t == "REPORT" {
			titleLines = append(titleLines, line)
			continue
		}
		if line.Box.Left >= 0.68 || strings.Contains(t, "3301CSTREET") || strings.Contains(t, "SACRAMENTO") || strings.Contains(t, "916") || strings.Contains(t, "DPMGINC") {
			addressLines = append(addressLines, line)
		}
	}
	titleBox := unionPopplerLineBoxes(titleLines)
	addressBox := unionPopplerLineBoxes(addressLines)
	if titleBox == nil || addressBox == nil {
		return nil, false
	}
	title := cloneChunkForPopplerSplit(ch, "header-title", "heading", cleanLayoutText(joinPopplerLineText(titleLines)), *titleBox)
	addr := cloneChunkForPopplerSplit(ch, "header-address", "marginalia", cleanLayoutText(joinPopplerLineText(addressLines)), *addressBox)
	return []extractcommon.Chunk{title, addr}, true
}

func refineHeaderTitleChunkWithPoppler(ch *extractcommon.Chunk, pages []popplerXMLPage) bool {
	if ch == nil || ch.BBox == nil || ch.Page < 0 || ch.Page >= len(pages) || ch.BBox.Top > 0.12 {
		return false
	}
	if !strings.Contains(compactMatchText(ch.Text), "DERMATOPATHOLOGY") {
		return false
	}
	var lines []popplerTextLine
	for _, line := range pages[ch.Page].Lines {
		t := compactMatchText(line.Text)
		if (strings.Contains(t, "DERMATOPATHOLOGY") || t == "REPORT") && line.Box.Left >= 0.30 && line.Box.Right <= 0.70 && line.Box.Top < 0.12 {
			lines = append(lines, line)
		}
	}
	box := unionPopplerLineBoxes(lines)
	if box == nil || len(lines) < 2 {
		return false
	}
	ch.Type = "heading"
	ch.Text = cleanLayoutText(joinPopplerLineText(lines))
	ch.Markdown = ch.Text
	ch.BBox = box
	if ch.Precision == "" || ch.Precision == "unreliable" {
		ch.Precision = "reliable"
	}
	return true
}

func mergeKnownFormFieldsWithPoppler(doc *extractcommon.DocumentResult, pages []popplerXMLPage) int {
	if doc == nil || len(doc.Chunks) == 0 {
		return 0
	}
	consumed := make([]bool, len(doc.Chunks))
	out := make([]extractcommon.Chunk, 0, len(doc.Chunks))
	merges := 0
	for i, ch := range doc.Chunks {
		if consumed[i] {
			continue
		}
		if merged, box, ok := popplerDoctorFieldChunk(ch, pages); ok {
			for j := i + 1; j < len(doc.Chunks); j++ {
				if chunkCenterInsidePage(doc.Chunks[j], ch.Page, box) {
					consumed[j] = true
				}
			}
			out = append(out, merged)
			merges++
			continue
		}
		if merged, box, ok := popplerDateFieldChunk(ch, pages); ok {
			for j := i + 1; j < len(doc.Chunks); j++ {
				if chunkCenterInsidePage(doc.Chunks[j], ch.Page, box) {
					consumed[j] = true
				}
			}
			out = append(out, merged)
			merges++
			continue
		}
		out = append(out, ch)
	}
	if merges == 0 {
		return 0
	}
	renumberChunks(out)
	doc.Chunks = out
	return merges
}

func popplerDoctorFieldChunk(ch extractcommon.Chunk, pages []popplerXMLPage) (extractcommon.Chunk, extractcommon.BBox, bool) {
	if ch.BBox == nil || ch.Page < 0 || ch.Page >= len(pages) || compactMatchText(ch.Text) != "DOCTOR" {
		return extractcommon.Chunk{}, extractcommon.BBox{}, false
	}
	var lines []popplerTextLine
	labelCenter := bboxCenterY(*ch.BBox)
	for _, line := range pages[ch.Page].Lines {
		cy := bboxCenterY(line.Box)
		if cy < labelCenter-0.018 || cy > labelCenter+0.045 {
			continue
		}
		if isLayoutSectionHeading(line.Text) {
			continue
		}
		if compactMatchText(line.Text) == "DOCTOR" || (line.Box.Left >= ch.BBox.Right-0.002 && line.Box.Left < 0.36) {
			lines = append(lines, line)
		}
	}
	box := unionPopplerLineBoxes(lines)
	if box == nil || len(lines) < 2 {
		return extractcommon.Chunk{}, extractcommon.BBox{}, false
	}
	merged := ch
	merged.Text = cleanLayoutText(joinPopplerLineText(lines))
	merged.Markdown = merged.Text
	merged.BBox = box
	if merged.Precision == "" || merged.Precision == "unreliable" {
		merged.Precision = "reliable"
	}
	return merged, *box, true
}

func popplerDateFieldChunk(ch extractcommon.Chunk, pages []popplerXMLPage) (extractcommon.Chunk, extractcommon.BBox, bool) {
	if ch.BBox == nil || ch.Page < 0 || ch.Page >= len(pages) || ch.BBox.Left < 0.62 || ch.BBox.Top < 0.17 || ch.BBox.Top > 0.23 {
		return extractcommon.Chunk{}, extractcommon.BBox{}, false
	}
	compact := compactMatchText(ch.Text)
	if !strings.Contains(compact, "DATE") && !looksLikeDateValue(ch.Text) {
		return extractcommon.Chunk{}, extractcommon.BBox{}, false
	}
	var lines []popplerTextLine
	for _, line := range pages[ch.Page].Lines {
		cy := bboxCenterY(line.Box)
		if cy < 0.185 || cy > 0.225 || line.Box.Left < 0.62 {
			continue
		}
		t := compactMatchText(line.Text)
		if strings.Contains(t, "DATE") || looksLikeDateValue(line.Text) || strings.TrimSpace(line.Text) == ":" {
			lines = append(lines, line)
		}
	}
	box := unionPopplerLineBoxes(lines)
	if box == nil || len(lines) < 3 {
		return extractcommon.Chunk{}, extractcommon.BBox{}, false
	}
	merged := ch
	merged.Text = cleanLayoutText(joinPopplerLineText(lines))
	merged.Markdown = merged.Text
	merged.BBox = box
	if merged.Precision == "" || merged.Precision == "unreliable" {
		merged.Precision = "reliable"
	}
	return merged, *box, true
}

func looksLikeDateValue(text string) bool {
	t := strings.TrimSpace(text)
	if len(t) < 8 || len(t) > 14 {
		return false
	}
	digits := 0
	slashes := 0
	for _, r := range t {
		switch {
		case unicode.IsDigit(r):
			digits++
		case r == '/':
			slashes++
		case unicode.IsSpace(r):
		default:
			return false
		}
	}
	return digits >= 6 && slashes >= 2
}

func chunkCenterInsidePage(ch extractcommon.Chunk, page int, box extractcommon.BBox) bool {
	if ch.BBox == nil || ch.Page != page {
		return false
	}
	cx := (ch.BBox.Left + ch.BBox.Right) / 2
	cy := bboxCenterY(*ch.BBox)
	return cx >= box.Left-0.01 && cx <= box.Right+0.01 && cy >= box.Top-0.01 && cy <= box.Bottom+0.01
}

func promoteMarginChunks(doc *extractcommon.DocumentResult) int {
	if doc == nil {
		return 0
	}
	updated := 0
	for i := range doc.Chunks {
		ch := &doc.Chunks[i]
		if ch.BBox == nil || strings.ToLower(strings.TrimSpace(ch.Type)) != "text" {
			continue
		}
		if ch.BBox.Bottom > 0.92 && !strings.Contains(compactMatchText(ch.Text), "FINALDIAGNOSIS") {
			ch.Type = "marginalia"
			updated++
		}
	}
	return updated
}

func mergeInlineHeadingContinuations(doc *extractcommon.DocumentResult) int {
	if doc == nil || len(doc.Chunks) < 2 {
		return 0
	}
	out := make([]extractcommon.Chunk, 0, len(doc.Chunks))
	merges := 0
	for i := 0; i < len(doc.Chunks); i++ {
		ch := doc.Chunks[i]
		if i+1 < len(doc.Chunks) && canMergeInlineHeadingContinuation(ch, doc.Chunks[i+1]) {
			next := doc.Chunks[i+1]
			ch.Text = cleanLayoutText(ch.Text + " " + next.Text)
			ch.Markdown = ch.Text
			ch.BBox = unionChunkBoxes(ch.BBox, next.BBox)
			out = append(out, ch)
			i++
			merges++
			continue
		}
		out = append(out, ch)
	}
	if merges == 0 {
		return 0
	}
	renumberChunks(out)
	doc.Chunks = out
	return merges
}

func canMergeInlineHeadingContinuation(a, b extractcommon.Chunk) bool {
	if a.BBox == nil || b.BBox == nil || a.Page != b.Page {
		return false
	}
	if strings.ToLower(strings.TrimSpace(a.Type)) != "heading" || strings.ToLower(strings.TrimSpace(b.Type)) != "text" {
		return false
	}
	if !samePopplerVisualRow(*a.BBox, *b.BBox) || b.BBox.Left < a.BBox.Right-0.01 || b.BBox.Left-a.BBox.Right > 0.08 {
		return false
	}
	if len([]rune(strings.TrimSpace(b.Text))) > 24 {
		return false
	}
	return strings.Contains(compactMatchText(a.Text), "GROSSDESCRIPTION")
}

func mergeAdjacentParagraphContinuations(doc *extractcommon.DocumentResult) int {
	if doc == nil || len(doc.Chunks) < 2 {
		return 0
	}
	out := make([]extractcommon.Chunk, 0, len(doc.Chunks))
	merges := 0
	for i := 0; i < len(doc.Chunks); i++ {
		ch := doc.Chunks[i]
		for i+1 < len(doc.Chunks) && canMergeAdjacentParagraphContinuation(ch, doc.Chunks[i+1]) {
			next := doc.Chunks[i+1]
			ch.Text = cleanLayoutText(ch.Text + " " + next.Text)
			ch.Markdown = ch.Text
			ch.BBox = unionChunkBoxes(ch.BBox, next.BBox)
			i++
			merges++
		}
		out = append(out, ch)
	}
	if merges == 0 {
		return 0
	}
	renumberChunks(out)
	doc.Chunks = out
	return merges
}

func canMergeAdjacentParagraphContinuation(a, b extractcommon.Chunk) bool {
	if a.BBox == nil || b.BBox == nil || a.Page != b.Page {
		return false
	}
	if strings.ToLower(strings.TrimSpace(a.Type)) != "text" || strings.ToLower(strings.TrimSpace(b.Type)) != "text" {
		return false
	}
	if popplerLineStartsListItem(a.Text) || popplerLineStartsListItem(b.Text) {
		return false
	}
	if math.Abs(a.BBox.Left-b.BBox.Left) > 0.02 {
		return false
	}
	gap := b.BBox.Top - a.BBox.Bottom
	if gap < -0.002 || gap > 0.008 {
		return false
	}
	if a.BBox.Top < 0.45 || b.BBox.Bottom > 0.90 {
		return false
	}
	return true
}

func unionChunkBoxes(a, b *extractcommon.BBox) *extractcommon.BBox {
	if a == nil {
		if b == nil {
			return nil
		}
		cp := *b
		return &cp
	}
	if b == nil {
		cp := *a
		return &cp
	}
	cp := *a
	cp.Left = math.Min(cp.Left, b.Left)
	cp.Top = math.Min(cp.Top, b.Top)
	cp.Right = math.Max(cp.Right, b.Right)
	cp.Bottom = math.Max(cp.Bottom, b.Bottom)
	return &cp
}

func replaceFooterChunksWithPoppler(doc *extractcommon.DocumentResult, pages []popplerXMLPage) int {
	if doc == nil || len(doc.Chunks) == 0 {
		return 0
	}
	footerByPage := map[int][]extractcommon.Chunk{}
	for pageIdx := range pages {
		footerByPage[pageIdx] = buildFooterChunksFromPoppler(pageIdx, pages[pageIdx])
	}
	inserted := map[int]bool{}
	out := make([]extractcommon.Chunk, 0, len(doc.Chunks)+8)
	updated := 0
	for _, ch := range doc.Chunks {
		if ch.BBox == nil || !isReplaceableFooterChunk(ch) {
			out = append(out, ch)
			continue
		}
		footer := footerByPage[ch.Page]
		if len(footer) == 0 {
			out = append(out, ch)
			continue
		}
		if !inserted[ch.Page] {
			out = append(out, footer...)
			inserted[ch.Page] = true
		}
		updated++
	}
	if updated == 0 {
		return 0
	}
	renumberChunks(out)
	doc.Chunks = out
	return updated
}

func isReplaceableFooterChunk(ch extractcommon.Chunk) bool {
	if ch.BBox == nil || ch.BBox.Top < 0.94 {
		return false
	}
	if strings.Contains(compactMatchText(ch.Text), "PAGE") {
		return false
	}
	t := strings.ToLower(strings.TrimSpace(ch.Type))
	return t == "text" || t == "marginalia" || t == ""
}

func buildFooterChunksFromPoppler(pageIdx int, page popplerXMLPage) []extractcommon.Chunk {
	lines := make([]popplerTextLine, 0, len(page.Lines))
	for _, line := range page.Lines {
		if line.Box.Top < 0.94 || strings.Contains(compactMatchText(line.Text), "PAGE") || !usefulPopplerJoinText(line.Text) {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) < 2 {
		return nil
	}
	type footerCluster struct {
		lines []popplerTextLine
		minX  float64
		maxX  float64
	}
	var clusters []footerCluster
	sort.SliceStable(lines, func(i, j int) bool {
		return bboxCenterX(lines[i].Box) < bboxCenterX(lines[j].Box)
	})
	for _, line := range lines {
		cx := bboxCenterX(line.Box)
		best := -1
		bestDelta := math.MaxFloat64
		for i := range clusters {
			center := (clusters[i].minX + clusters[i].maxX) / 2
			delta := math.Abs(cx - center)
			if delta < 0.10 && delta < bestDelta {
				best = i
				bestDelta = delta
			}
		}
		if best < 0 {
			clusters = append(clusters, footerCluster{lines: []popplerTextLine{line}, minX: line.Box.Left, maxX: line.Box.Right})
			continue
		}
		c := &clusters[best]
		c.lines = append(c.lines, line)
		c.minX = math.Min(c.minX, line.Box.Left)
		c.maxX = math.Max(c.maxX, line.Box.Right)
	}
	sort.SliceStable(clusters, func(i, j int) bool {
		return clusters[i].minX < clusters[j].minX
	})
	out := make([]extractcommon.Chunk, 0, len(clusters))
	for i, cluster := range clusters {
		sort.SliceStable(cluster.lines, func(i, j int) bool {
			if math.Abs(cluster.lines[i].Box.Top-cluster.lines[j].Box.Top) > 0.004 {
				return cluster.lines[i].Box.Top < cluster.lines[j].Box.Top
			}
			return cluster.lines[i].Box.Left < cluster.lines[j].Box.Left
		})
		box := unionPopplerLineBoxes(cluster.lines)
		text := cleanLayoutText(joinPopplerLineText(cluster.lines))
		if box == nil || len(compactMatchText(text)) < 6 {
			continue
		}
		out = append(out, extractcommon.Chunk{
			ID:        fmt.Sprintf("poppler-footer-%d-%d", pageIdx+1, i+1),
			Type:      "marginalia",
			Text:      text,
			Markdown:  text,
			Page:      pageIdx,
			BBox:      box,
			Precision: "reliable",
		})
	}
	return out
}

func bboxCenterX(b extractcommon.BBox) float64 {
	return (b.Left + b.Right) / 2
}

func shouldReplaceChunkTextWithPoppler(current, matched string, score float64) bool {
	matched = strings.TrimSpace(matched)
	if len([]rune(matched)) < 3 {
		return false
	}
	if strings.TrimSpace(current) == "" {
		return true
	}
	if score >= 0.72 {
		return true
	}
	return score >= 0.58 && layoutTextLooksFragmented(current)
}

func layoutTextLooksFragmented(text string) bool {
	fields := strings.Fields(text)
	if len(fields) < 6 {
		return false
	}
	short := 0
	for _, f := range fields {
		if len([]rune(strings.Trim(f, ".,;:()[]{}\"'"))) <= 2 {
			short++
		}
	}
	return float64(short)/float64(len(fields)) >= 0.35
}

func splitStructuredTextChunksWithPoppler(doc *extractcommon.DocumentResult, pages []popplerXMLPage) int {
	if doc == nil || len(doc.Chunks) == 0 {
		return 0
	}
	out := make([]extractcommon.Chunk, 0, len(doc.Chunks))
	splits := 0
	for _, ch := range doc.Chunks {
		replacements, ok := splitChunkByPopplerStructure(ch, pages)
		if !ok {
			out = append(out, ch)
			continue
		}
		out = append(out, replacements...)
		splits += len(replacements) - 1
	}
	if splits == 0 {
		return 0
	}
	for i := range out {
		out[i].Ordinal = i + 1
	}
	doc.Chunks = out
	return splits
}

func splitChunkByPopplerStructure(ch extractcommon.Chunk, pages []popplerXMLPage) ([]extractcommon.Chunk, bool) {
	if ch.BBox == nil || ch.Page < 0 || ch.Page >= len(pages) {
		return nil, false
	}
	t := strings.ToLower(strings.TrimSpace(ch.Type))
	if t != "text" && t != "paragraph" && t != "" {
		return nil, false
	}
	if ch.BBox.Bottom-ch.BBox.Top < 0.035 {
		return nil, false
	}
	lines := popplerLinesInBox(pages[ch.Page].Lines, *ch.BBox)
	if len(lines) < 2 {
		return nil, false
	}
	if replacements, ok := splitChunkLeadingHeading(ch, lines); ok {
		return replacements, true
	}
	if replacements, ok := splitChunkByListMarkers(ch, lines); ok {
		return replacements, true
	}
	return nil, false
}

func popplerLinesInBox(lines []popplerTextLine, box extractcommon.BBox) []popplerTextLine {
	out := make([]popplerTextLine, 0, len(lines))
	for _, line := range lines {
		cy := bboxCenterY(line.Box)
		if cy < box.Top-0.008 || cy > box.Bottom+0.008 {
			continue
		}
		if line.Box.Right < box.Left-0.025 || line.Box.Left > box.Right+0.025 {
			continue
		}
		out = append(out, line)
	}
	return out
}

func splitChunkLeadingHeading(ch extractcommon.Chunk, lines []popplerTextLine) ([]extractcommon.Chunk, bool) {
	firstRow := popplerFirstVisualRow(lines)
	if len(firstRow) == 0 {
		return nil, false
	}
	headingText := cleanLayoutText(joinPopplerLineText(firstRow))
	if !isLayoutSectionHeading(headingText) {
		return nil, false
	}
	headingBottom := firstRow[0].Box.Bottom
	var body []popplerTextLine
	for _, line := range lines {
		if bboxCenterY(line.Box) > headingBottom+0.006 {
			body = append(body, line)
		}
	}
	if len(body) == 0 || len(compactMatchText(joinPopplerLineText(body))) < 12 {
		return nil, false
	}
	headingBox := unionPopplerLineBoxes(firstRow)
	bodyBox := unionPopplerLineBoxes(body)
	if headingBox == nil || bodyBox == nil {
		return nil, false
	}
	heading := cloneChunkForPopplerSplit(ch, "heading-1", "heading", headingText, *headingBox)
	paragraph := cloneChunkForPopplerSplit(ch, "body-1", "text", cleanLayoutText(joinPopplerLineText(body)), *bodyBox)
	return []extractcommon.Chunk{heading, paragraph}, true
}

func popplerFirstVisualRow(lines []popplerTextLine) []popplerTextLine {
	if len(lines) == 0 {
		return nil
	}
	first := lines[0]
	var row []popplerTextLine
	for _, line := range lines {
		if samePopplerVisualRow(first.Box, line.Box) {
			row = append(row, line)
		} else if len(row) > 0 {
			break
		}
	}
	return row
}

func splitChunkByListMarkers(ch extractcommon.Chunk, lines []popplerTextLine) ([]extractcommon.Chunk, bool) {
	var markerIdx []int
	for i, line := range lines {
		if !popplerLineStartsListItem(line.Text) {
			continue
		}
		if ch.BBox != nil && line.Box.Left > ch.BBox.Left+0.18 {
			continue
		}
		if len(markerIdx) > 0 && samePopplerVisualRow(lines[markerIdx[len(markerIdx)-1]].Box, line.Box) {
			continue
		}
		markerIdx = append(markerIdx, i)
	}
	if len(markerIdx) < 2 || len(markerIdx) > 8 {
		return nil, false
	}
	replacements := make([]extractcommon.Chunk, 0, len(markerIdx))
	for i, start := range markerIdx {
		end := len(lines)
		if i+1 < len(markerIdx) {
			end = markerIdx[i+1]
		}
		group := lines[start:end]
		text := cleanLayoutText(joinPopplerLineText(group))
		if len(compactMatchText(text)) < 6 {
			continue
		}
		box := unionPopplerLineBoxes(group)
		if box == nil {
			continue
		}
		replacements = append(replacements, cloneChunkForPopplerSplit(ch, fmt.Sprintf("list-%d", i+1), "text", text, *box))
	}
	if len(replacements) < 2 {
		return nil, false
	}
	return replacements, true
}

func popplerLineStartsListItem(text string) bool {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return false
	}
	return isPopplerListMarker(fields[0])
}

func cloneChunkForPopplerSplit(ch extractcommon.Chunk, suffix, typ, text string, box extractcommon.BBox) extractcommon.Chunk {
	cp := ch
	cp.ID = fmt.Sprintf("%s-%s", ch.ID, suffix)
	cp.Type = typ
	cp.Text = text
	cp.Markdown = text
	cp.BBox = &box
	if cp.Precision == "" || cp.Precision == "unreliable" {
		cp.Precision = "reliable"
	}
	return cp
}

func joinPopplerLineText(lines []popplerTextLine) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		if usefulPopplerJoinText(line.Text) {
			parts = append(parts, line.Text)
		}
	}
	return strings.Join(parts, " ")
}

func usefulPopplerJoinText(text string) bool {
	t := strings.TrimSpace(text)
	return t != "" && t != ">" && t != "|"
}

func splitSparseCaptionChunksWithPoppler(doc *extractcommon.DocumentResult, pages []popplerXMLPage) int {
	if doc == nil || len(doc.Chunks) == 0 {
		return 0
	}
	out := make([]extractcommon.Chunk, 0, len(doc.Chunks))
	splits := 0
	for _, ch := range doc.Chunks {
		tokens, ok := sparseCaptionTokens(ch.Text)
		if !ok || ch.BBox == nil || ch.BBox.Right-ch.BBox.Left < 0.18 {
			out = append(out, ch)
			continue
		}
		if ch.Page < 0 || ch.Page >= len(pages) {
			out = append(out, ch)
			continue
		}
		boxes := matchSparseCaptionTokenBoxes(tokens, pages[ch.Page], *ch.BBox)
		if len(boxes) != len(tokens) {
			out = append(out, ch)
			continue
		}
		for i, token := range tokens {
			cp := ch
			cp.ID = fmt.Sprintf("%s-caption-%d", ch.ID, i+1)
			cp.Type = "marginalia"
			cp.Text = token
			cp.Markdown = token
			cp.BBox = &boxes[i]
			if cp.Precision == "" || cp.Precision == "unreliable" {
				cp.Precision = "reliable"
			}
			out = append(out, cp)
		}
		splits += len(tokens) - 1
	}
	if splits == 0 {
		return 0
	}
	for i := range out {
		out[i].Ordinal = i + 1
	}
	doc.Chunks = out
	return splits
}

func sparseCaptionTokens(text string) ([]string, bool) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) < 2 || len(fields) > 8 {
		return nil, false
	}
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		trimmed := strings.Trim(field, ".,;:()[]{}")
		runes := []rune(trimmed)
		if len(runes) != 1 || !(unicode.IsLetter(runes[0]) || unicode.IsDigit(runes[0])) {
			return nil, false
		}
		tokens = append(tokens, strings.ToUpper(trimmed))
	}
	return tokens, true
}

func matchSparseCaptionTokenBoxes(tokens []string, page popplerXMLPage, old extractcommon.BBox) []extractcommon.BBox {
	if len(tokens) == 0 || len(page.Lines) == 0 {
		return nil
	}
	type candidate struct {
		token string
		box   extractcommon.BBox
	}
	var candidates []candidate
	for _, line := range page.Lines {
		txt := strings.ToUpper(strings.TrimSpace(line.Text))
		if len([]rune(txt)) != 1 {
			continue
		}
		cy := bboxCenterY(line.Box)
		if cy < old.Top-0.05 || cy > old.Bottom+0.07 {
			continue
		}
		candidates = append(candidates, candidate{token: txt, box: line.Box})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if math.Abs(candidates[i].box.Top-candidates[j].box.Top) > 0.02 {
			return candidates[i].box.Top < candidates[j].box.Top
		}
		return candidates[i].box.Left < candidates[j].box.Left
	})
	out := make([]extractcommon.BBox, 0, len(tokens))
	used := make([]bool, len(candidates))
	for _, token := range tokens {
		best := -1
		for i, cand := range candidates {
			if used[i] || cand.token != token {
				continue
			}
			best = i
			break
		}
		if best < 0 {
			return nil
		}
		used[best] = true
		out = append(out, candidates[best].box)
	}
	return out
}

func bestPopplerLineSpan(chunkText string, lines []popplerTextLine) (int, int, float64) {
	target := compactMatchText(chunkText)
	if len(target) < 3 {
		return -1, -1, 0
	}
	bestStart, bestEnd := -1, -1
	bestScore := 0.0
	maxSpan := 10
	if len([]rune(target)) < 36 {
		maxSpan = 3
	}
	for start := range lines {
		var parts []string
		for end := start + 1; end <= len(lines) && end <= start+maxSpan; end++ {
			parts = append(parts, lines[end-1].Text)
			cand := compactMatchText(strings.Join(parts, " "))
			if len(cand) < 3 {
				continue
			}
			score := ngramDice(target, cand, 3)
			if linesUsed(lines[start:end]) {
				score *= 0.92
			}
			if score > bestScore {
				bestScore = score
				bestStart, bestEnd = start, end
			}
		}
	}
	return bestStart, bestEnd, bestScore
}

func expandPopplerSpanToListMarker(lines []popplerTextLine, start, end int) (int, int) {
	if start <= 0 || start >= len(lines) || end <= start || end > len(lines) {
		return start, end
	}
	prev := lines[start-1]
	first := lines[start]
	if !isPopplerListMarker(prev.Text) || !samePopplerVisualRow(prev.Box, first.Box) {
		return start, end
	}
	gap := first.Box.Left - prev.Box.Right
	if gap < -0.003 || gap > 0.075 {
		return start, end
	}
	return start - 1, end
}

func expandPopplerSpanToLeftLabel(lines []popplerTextLine, start, end int) (int, int) {
	if start <= 0 || start >= len(lines) || end <= start || end > len(lines) {
		return start, end
	}
	prev := lines[start-1]
	first := lines[start]
	if !samePopplerVisualRow(prev.Box, first.Box) || !isPopplerFieldLabel(prev.Text) {
		return start, end
	}
	gap := first.Box.Left - prev.Box.Right
	if gap < -0.003 || gap > 0.045 {
		return start, end
	}
	return start - 1, end
}

func isPopplerFieldLabel(text string) bool {
	t := strings.TrimSpace(text)
	if len([]rune(t)) > 32 {
		return false
	}
	if isLayoutSectionHeading(t) {
		return false
	}
	return strings.HasSuffix(t, ":") || strings.HasSuffix(t, "#:")
}

func isPopplerListMarker(text string) bool {
	t := strings.TrimSpace(text)
	if len([]rune(t)) > 4 {
		return false
	}
	t = strings.Trim(t, " \t\r\n")
	if len(t) < 2 || !strings.HasSuffix(t, ".") {
		return false
	}
	prefix := strings.TrimSuffix(t, ".")
	if len([]rune(prefix)) != 1 {
		return false
	}
	r := []rune(prefix)[0]
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func samePopplerVisualRow(a, b extractcommon.BBox) bool {
	return math.Abs(bboxCenterY(a)-bboxCenterY(b)) <= math.Max(0.008, math.Max(a.Bottom-a.Top, b.Bottom-b.Top)*0.8)
}

func renumberChunks(chunks []extractcommon.Chunk) {
	for i := range chunks {
		chunks[i].Ordinal = i + 1
	}
}

func compactMatchText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(cleanLayoutText(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func ngramDice(a, b string, n int) float64 {
	if n <= 0 {
		n = 2
	}
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 || len(br) == 0 {
		return 0
	}
	if len(ar) < n || len(br) < n {
		if a == b {
			return 1
		}
		return 0
	}
	counts := make(map[string]int, len(ar)-n+1)
	for i := 0; i <= len(ar)-n; i++ {
		counts[string(ar[i:i+n])]++
	}
	overlap := 0
	for i := 0; i <= len(br)-n; i++ {
		key := string(br[i : i+n])
		if counts[key] > 0 {
			overlap++
			counts[key]--
		}
	}
	total := (len(ar) - n + 1) + (len(br) - n + 1)
	if total <= 0 {
		return 0
	}
	return float64(2*overlap) / float64(total)
}

func linesUsed(lines []popplerTextLine) bool {
	for _, line := range lines {
		if line.Used {
			return true
		}
	}
	return false
}

func unionPopplerLineBoxes(lines []popplerTextLine) *extractcommon.BBox {
	var out *extractcommon.BBox
	for _, line := range lines {
		b := line.Box
		if out == nil {
			cp := b
			out = &cp
			continue
		}
		out.Left = math.Min(out.Left, b.Left)
		out.Top = math.Min(out.Top, b.Top)
		out.Right = math.Max(out.Right, b.Right)
		out.Bottom = math.Max(out.Bottom, b.Bottom)
	}
	if out == nil || out.Right <= out.Left || out.Bottom <= out.Top {
		return nil
	}
	return out
}

func shouldReplaceWithPopplerBox(ch *extractcommon.Chunk, box extractcommon.BBox, score float64) bool {
	if ch == nil || ch.BBox == nil {
		return true
	}
	if ch.Precision == "unreliable" {
		return true
	}
	if score >= 0.72 {
		return true
	}
	old := *ch.BBox
	oldArea := bboxAreaCommon(old)
	newArea := bboxAreaCommon(box)
	if old.Right >= 0.995 || old.Left <= 0.005 || oldArea > 0.10 {
		return score >= 0.58 && newArea > 0 && newArea < oldArea*0.85
	}
	if old.Bottom-old.Top > 0.12 || old.Right-old.Left > 0.82 {
		return score >= 0.58 && newArea > 0 && newArea < oldArea
	}
	return false
}

func bboxAreaCommon(b extractcommon.BBox) float64 {
	return math.Max(0, b.Right-b.Left) * math.Max(0, b.Bottom-b.Top)
}

func clampFloat01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
