package pdf

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	pdfadapter "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/adapters/pdf"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/core/chunking"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/core/layoutdoc"
)

const (
	localQualityRightColumnMinX = 0.58
	localQualityMissingLineMin  = 4
	localQualityMissingRuneMin  = 80
	localQualityMissingRatioMin = 0.35
	localQualityRegionPad       = 0.008
	localQualityRegionMinRunes  = 24
	localQualityTableRowMin     = 3
	localQualityTableCellsMin   = 3
)

type layoutRepairOutcome struct {
	Chunks        []map[string]any
	Payload       map[string]any
	SuggestHybrid bool
	RepairAdded   int
}

type localLayoutLine struct {
	PageIndex int
	Order     int
	Text      string
	X         float64
	Y         float64
	BBox      map[string]float64
}

type localLayoutChunk struct {
	Chunk        map[string]any
	PageIndex    int
	OriginalPos  int
	OriginalOrd  int
	HasBBox      bool
	BBox         map[string]float64
	HasText      bool
	Text         string
	CenterX      float64
	CenterY      float64
	IsDiagnostic bool
}

func analyzeAndRepairLocalLayout(
	sourceName string,
	pdfBytes []byte,
	chunks []map[string]any,
	geometryLines []pdfadapter.GeometryLine,
	pageGeoms map[int]chunking.PageGeom,
) layoutRepairOutcome {
	outcome := layoutRepairOutcome{
		Chunks: append([]map[string]any(nil), chunks...),
		Payload: map[string]any{
			"strategy": "page_profile_quality_gate",
			"source":   "opendataloader_style_local_triage",
		},
	}
	linesByPage := prepareLocalLayoutLines(geometryLines, pageGeoms)
	if len(linesByPage) == 0 {
		outcome.Chunks = orderChunkMapsByLayout(outcome.Chunks)
		outcome.Payload["pages"] = []map[string]any{}
		outcome.Payload["repair_chunks_added"] = 0
		outcome.Payload["suggest_hybrid"] = false
		outcome.Payload["reasons"] = []string{}
		return outcome
	}

	chunksByPage := prepareLocalLayoutChunks(outcome.Chunks)
	pageIndexes := sortedLayoutPageIndexes(linesByPage)
	pages := make([]map[string]any, 0, len(pageIndexes))
	lowConfidenceRegions := make([]map[string]any, 0)
	reasons := make(map[string]bool)
	for _, pageIndex := range pageIndexes {
		pageLines := linesByPage[pageIndex]
		pageChunks := chunksByPage[pageIndex]
		missingLines, lineCoverageRatio := missingLayoutLines(pageLines, pageChunks)
		rightLines := filterRightColumnLines(pageLines)
		_, rightCoverageRatio := missingRightColumnLines(rightLines, pageChunks)
		tableCandidate, tableRows := detectGeometryTableCandidate(pageLines)
		hasTable := hasPageChunkType(pageChunks, "table")

		pagePayload := map[string]any{
			"page_index":                  pageIndex,
			"geometry_line_count":         len(pageLines),
			"line_coverage_ratio":         roundFloat(lineCoverageRatio, 4),
			"missing_line_count":          len(missingLines),
			"right_column_line_count":     len(rightLines),
			"right_column_coverage_ratio": roundFloat(rightCoverageRatio, 4),
			"table_candidate":             tableCandidate,
			"table_candidate_rows":        tableRows,
			"has_table_chunk":             hasTable,
		}

		pageRegions := buildLowConfidenceLayoutRegions(pageIndex, pageLines, pageChunks, missingLines, tableCandidate && !hasTable, len(lowConfidenceRegions))
		if len(pageRegions) > 0 {
			pagePayload["low_confidence_regions"] = pageRegions
			pagePayload["low_confidence_region_count"] = len(pageRegions)
			lowConfidenceRegions = append(lowConfidenceRegions, pageRegions...)
			reasons["low_confidence_regions_detected"] = true
		}

		if shouldRepairMissingLayoutLines(pageLines, missingLines, lineCoverageRatio) {
			repairChunks := buildLayoutRepairChunks(sourceName, pageIndex, missingLines, outcome.RepairAdded)
			if len(repairChunks) > 0 {
				outcome.Chunks = append(outcome.Chunks, repairChunks...)
				outcome.RepairAdded += len(repairChunks)
				pagePayload["layout_repair_chunks"] = len(repairChunks)
				reasons["geometry_text_missing_from_chunks"] = true
			}
		}
		if shouldOCRRightColumn(pageIndex, pageLines, rightLines, pageChunks, rightCoverageRatio) {
			ocrLines, ocrPayload := localOCRRightColumnLines(pdfBytes, pageIndex)
			pagePayload["right_column_ocr"] = ocrPayload
			if shouldAddOCRRightColumn(ocrLines, pageChunks) {
				repairChunks := buildLayoutRepairChunks(sourceName, pageIndex, ocrLines, outcome.RepairAdded)
				for _, ch := range repairChunks {
					ch["source"] = "local_ocr_layout_repair"
					ch["confidence"] = 0.68
					if payload, ok := ch["payload"].(map[string]any); ok {
						payload["repair"] = "right_column_local_ocr"
						if engine := strings.TrimSpace(fmt.Sprint(ocrPayload["engine"])); engine != "" {
							payload["ocr_engine"] = engine
						}
					}
				}
				if len(repairChunks) > 0 {
					outcome.Chunks = append(outcome.Chunks, repairChunks...)
					outcome.RepairAdded += len(repairChunks)
					pagePayload["right_column_ocr_repair_chunks"] = len(repairChunks)
					reasons["right_column_ocr_repair"] = true
				}
			}
		}

		if tableCandidate && !hasTable {
			reasons["table_candidate_without_table_chunk"] = true
		}
		pages = append(pages, pagePayload)
	}

	outcome.Chunks = orderChunkMapsByLayout(outcome.Chunks)
	reasonList := make([]string, 0, len(reasons))
	for reason := range reasons {
		reasonList = append(reasonList, reason)
	}
	sort.Strings(reasonList)
	outcome.Payload["pages"] = pages
	outcome.Payload["repair_chunks_added"] = outcome.RepairAdded
	outcome.Payload["low_confidence_region_count"] = len(lowConfidenceRegions)
	outcome.Payload["low_confidence_regions"] = lowConfidenceRegions
	outcome.Payload["suggest_hybrid"] = outcome.SuggestHybrid
	outcome.Payload["reasons"] = reasonList
	return outcome
}

func prepareLocalLayoutLines(lines []pdfadapter.GeometryLine, pageGeoms map[int]chunking.PageGeom) map[int][]localLayoutLine {
	out := make(map[int][]localLayoutLine)
	for _, gl := range lines {
		text := strings.TrimSpace(gl.Text)
		if text == "" {
			continue
		}
		pageIndex := gl.PageIndex
		if pageIndex <= 0 {
			continue
		}
		g, ok := pageGeoms[pageIndex]
		if !ok || g.Right <= g.Left || g.Top <= g.Bottom {
			continue
		}
		boxMap := adapterGeometryBBoxTopLeftMap(gl.BBox, g)
		x := 0.0
		y := 0.0
		if len(boxMap) > 0 {
			x = (boxMap["left"] + boxMap["right"]) / 2
			y = 1 - ((boxMap["top"] + boxMap["bottom"]) / 2)
		} else {
			x = clamp01Local((gl.GeomX - g.Left) / (g.Right - g.Left))
			y = clamp01Local((gl.GeomY - g.Bottom) / (g.Top - g.Bottom))
			box := chunking.AnchorToBBox(gl.GeomX, gl.GeomY, g)
			anyBox := chunking.BBoxTopLeftMap(box)
			if anyBox == nil {
				continue
			}
			boxMap = anyMapToFloatMap(anyBox)
		}
		out[pageIndex] = append(out[pageIndex], localLayoutLine{
			PageIndex: pageIndex,
			Order:     gl.Order,
			Text:      text,
			X:         x,
			Y:         y,
			BBox:      boxMap,
		})
	}
	for pageIndex := range out {
		sortLocalLayoutLines(out[pageIndex])
	}
	return out
}

func prepareLocalLayoutChunks(chunks []map[string]any) map[int][]localLayoutChunk {
	out := make(map[int][]localLayoutChunk)
	for i, ch := range chunks {
		pageIndex := intFromAnyLocal(ch["page_index"])
		if pageIndex <= 0 {
			pageIndex = 1
		}
		box, hasBox := chunkBBoxTopLeft(ch)
		item := localLayoutChunk{
			Chunk:        ch,
			PageIndex:    pageIndex,
			OriginalPos:  i,
			OriginalOrd:  intFromAnyLocal(ch["order"]),
			HasBBox:      hasBox,
			BBox:         box,
			Text:         strings.TrimSpace(stringFromAnyLocal(ch["text"])),
			IsDiagnostic: isDiagnosticChunk(ch),
		}
		item.HasText = item.Text != ""
		if hasBox {
			item.CenterX = (box["left"] + box["right"]) / 2
			item.CenterY = (box["top"] + box["bottom"]) / 2
		}
		out[pageIndex] = append(out[pageIndex], item)
	}
	return out
}

func sortedLayoutPageIndexes(linesByPage map[int][]localLayoutLine) []int {
	pages := make([]int, 0, len(linesByPage))
	for pageIndex := range linesByPage {
		pages = append(pages, pageIndex)
	}
	sort.Ints(pages)
	return pages
}

func filterRightColumnLines(lines []localLayoutLine) []localLayoutLine {
	out := make([]localLayoutLine, 0)
	for _, line := range lines {
		if line.X >= localQualityRightColumnMinX {
			out = append(out, line)
		}
	}
	return out
}

func missingRightColumnLines(rightLines []localLayoutLine, pageChunks []localLayoutChunk) ([]localLayoutLine, float64) {
	if len(rightLines) == 0 {
		return nil, 1
	}
	var rightChunkText, allChunkText strings.Builder
	for _, ch := range pageChunks {
		if !ch.HasText || ch.IsDiagnostic {
			continue
		}
		allChunkText.WriteByte('\n')
		allChunkText.WriteString(ch.Text)
		if ch.HasBBox && ch.CenterX >= localQualityRightColumnMinX-0.04 {
			rightChunkText.WriteByte('\n')
			rightChunkText.WriteString(ch.Text)
		}
	}
	rightNorm := comparableLayoutText(rightChunkText.String())
	allNorm := comparableLayoutText(allChunkText.String())
	matched := 0
	missing := make([]localLayoutLine, 0)
	for _, line := range rightLines {
		norm := comparableLayoutText(line.Text)
		if len(norm) < 2 {
			matched++
			continue
		}
		if strings.Contains(rightNorm, norm) || strings.Contains(allNorm, norm) {
			matched++
			continue
		}
		missing = append(missing, line)
	}
	return missing, float64(matched) / float64(len(rightLines))
}

func missingLayoutLines(lines []localLayoutLine, pageChunks []localLayoutChunk) ([]localLayoutLine, float64) {
	if len(lines) == 0 {
		return nil, 1
	}
	var chunkText strings.Builder
	for _, ch := range pageChunks {
		if !ch.HasText || ch.IsDiagnostic {
			continue
		}
		chunkText.WriteByte('\n')
		chunkText.WriteString(ch.Text)
	}
	chunkNorm := comparableLayoutText(chunkText.String())
	matched := 0
	missing := make([]localLayoutLine, 0)
	for _, line := range lines {
		norm := comparableLayoutText(line.Text)
		if len(norm) < 2 {
			matched++
			continue
		}
		if strings.Contains(chunkNorm, norm) {
			matched++
			continue
		}
		missing = append(missing, line)
	}
	return missing, float64(matched) / float64(len(lines))
}

func shouldRepairMissingLayoutLines(lines, missing []localLayoutLine, coverageRatio float64) bool {
	if len(lines) < localQualityMissingLineMin || len(missing) < localQualityMissingLineMin {
		return false
	}
	if 1-coverageRatio < localQualityMissingRatioMin {
		return false
	}
	runes := 0
	for _, line := range missing {
		runes += utf8.RuneCountInString(line.Text)
	}
	return runes >= localQualityMissingRuneMin
}

func buildLowConfidenceLayoutRegions(
	pageIndex int,
	pageLines []localLayoutLine,
	pageChunks []localLayoutChunk,
	missingLines []localLayoutLine,
	tableMissing bool,
	start int,
) []map[string]any {
	regions := make([]map[string]any, 0)
	add := func(reason string, lines []localLayoutLine, confidence float64, anchor string) {
		if len(lines) == 0 {
			return
		}
		text := joinLayoutLineTexts(lines)
		if utf8.RuneCountInString(strings.TrimSpace(text)) < localQualityRegionMinRunes && reason != "table_candidate_without_table_chunk" {
			return
		}
		box := paddedLayoutBox(anyMapToFloatMap(unionLayoutLineBoxes(lines)), localQualityRegionPad)
		if len(box) == 0 {
			return
		}
		region := map[string]any{
			"id":           fmt.Sprintf("region_p%d_%d", pageIndex, start+len(regions)),
			"page_index":   pageIndex,
			"reason":       reason,
			"route":        "ocr_vlm_region",
			"bbox":         floatMapToAnyMap(box),
			"line_count":   len(lines),
			"confidence":   roundFloat(confidence, 4),
			"text_preview": truncateLayoutText(text, 180),
		}
		if anchor != "" {
			region["anchor_chunk_id"] = anchor
		}
		if !hasSimilarLayoutRegion(regions, pageIndex, box) {
			regions = append(regions, region)
		}
	}

	for _, group := range groupLayoutRepairLines(missingLines) {
		add("geometry_text_missing_from_chunks", group, 0.62, "")
	}
	if tableMissing {
		if lines := tableCandidateRegionLines(pageLines); len(lines) > 0 {
			add("table_candidate_without_table_chunk", lines, 0.58, "")
		}
	}
	for _, region := range unstableChunkBBoxRegions(pageIndex, pageLines, pageChunks) {
		if !hasSimilarLayoutRegion(regions, pageIndex, anyMapToFloatMap(region["bbox"].(map[string]any))) {
			region["id"] = fmt.Sprintf("region_p%d_%d", pageIndex, start+len(regions))
			regions = append(regions, region)
		}
	}
	return regions
}

func tableCandidateRegionLines(lines []localLayoutLine) []localLayoutLine {
	rows := clusterLayoutRows(lines, 0.012)
	out := make([]localLayoutLine, 0)
	for _, row := range rows {
		if countSeparatedCells(row) >= localQualityTableCellsMin {
			out = append(out, row...)
		}
	}
	return out
}

func unstableChunkBBoxRegions(pageIndex int, pageLines []localLayoutLine, pageChunks []localLayoutChunk) []map[string]any {
	out := make([]map[string]any, 0)
	for _, ch := range pageChunks {
		if !ch.HasText || !ch.HasBBox || ch.IsDiagnostic {
			continue
		}
		if skipUnstableBBoxChunkType(ch.Chunk) || utf8.RuneCountInString(ch.Text) < 12 {
			continue
		}
		matched := geometryLinesForChunkText(ch.Text, pageLines)
		if len(matched) == 0 {
			continue
		}
		lineBox := paddedLayoutBox(anyMapToFloatMap(unionLayoutLineBoxes(matched)), localQualityRegionPad)
		if len(lineBox) == 0 || !chunkBBoxLooksUnstable(ch.BBox, lineBox) {
			continue
		}
		out = append(out, map[string]any{
			"id":              "",
			"page_index":      pageIndex,
			"reason":          "bbox_mismatch_geometry_lines",
			"route":           "ocr_vlm_region",
			"bbox":            floatMapToAnyMap(lineBox),
			"line_count":      len(matched),
			"confidence":      0.64,
			"text_preview":    truncateLayoutText(joinLayoutLineTexts(matched), 180),
			"anchor_chunk_id": strings.TrimSpace(fmt.Sprint(ch.Chunk["chunk_id"])),
		})
	}
	return out
}

func skipUnstableBBoxChunkType(ch map[string]any) bool {
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(ch["type"]))) {
	case "table", "image", "stamp", "bookmark", "annotation", "form_field", "attachment":
		return true
	default:
		return false
	}
}

func geometryLinesForChunkText(text string, lines []localLayoutLine) []localLayoutLine {
	chunkNorm := comparableLayoutText(text)
	if len(chunkNorm) < 8 {
		return nil
	}
	out := make([]localLayoutLine, 0)
	for _, line := range lines {
		norm := comparableLayoutText(line.Text)
		if len(norm) < 4 {
			continue
		}
		if strings.Contains(chunkNorm, norm) || strings.Contains(norm, chunkNorm) {
			out = append(out, line)
		}
	}
	return out
}

func chunkBBoxLooksUnstable(chunkBox map[string]float64, lineBox map[string]float64) bool {
	chunkW := math.Max(0, chunkBox["right"]-chunkBox["left"])
	chunkH := math.Max(0, chunkBox["bottom"]-chunkBox["top"])
	lineW := math.Max(0, lineBox["right"]-lineBox["left"])
	lineH := math.Max(0, lineBox["bottom"]-lineBox["top"])
	if chunkW <= 0 || chunkH <= 0 || lineW <= 0 || lineH <= 0 {
		return false
	}
	if lineW > chunkW*1.35 && lineW-chunkW > 0.035 {
		return true
	}
	if lineH > chunkH*1.8 && lineH-chunkH > 0.025 {
		return true
	}
	return !layoutBoxContains(chunkBox, lineBox, 0.012) && layoutBoxArea(lineBox) > layoutBoxArea(chunkBox)*1.25
}

func layoutBoxContains(outer, inner map[string]float64, tol float64) bool {
	return outer["left"] <= inner["left"]+tol &&
		outer["right"] >= inner["right"]-tol &&
		outer["top"] <= inner["top"]+tol &&
		outer["bottom"] >= inner["bottom"]-tol
}

func layoutBoxArea(box map[string]float64) float64 {
	return math.Max(0, box["right"]-box["left"]) * math.Max(0, box["bottom"]-box["top"])
}

func paddedLayoutBox(box map[string]float64, pad float64) map[string]float64 {
	if len(box) == 0 {
		return nil
	}
	return map[string]float64{
		"left":   clamp01Local(box["left"] - pad),
		"right":  clamp01Local(box["right"] + pad),
		"top":    clamp01Local(box["top"] - pad),
		"bottom": clamp01Local(box["bottom"] + pad),
	}
}

func hasSimilarLayoutRegion(regions []map[string]any, pageIndex int, box map[string]float64) bool {
	for _, region := range regions {
		if intFromAnyLocal(region["page_index"]) != pageIndex {
			continue
		}
		existingRaw, _ := region["bbox"].(map[string]any)
		existing := anyMapToFloatMap(existingRaw)
		if layoutBoxIoU(existing, box) >= 0.72 {
			return true
		}
	}
	return false
}

func layoutBoxIoU(a, b map[string]float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	left := math.Max(a["left"], b["left"])
	right := math.Min(a["right"], b["right"])
	top := math.Max(a["top"], b["top"])
	bottom := math.Min(a["bottom"], b["bottom"])
	inter := math.Max(0, right-left) * math.Max(0, bottom-top)
	if inter <= 0 {
		return 0
	}
	union := layoutBoxArea(a) + layoutBoxArea(b) - inter
	if union <= 0 {
		return 0
	}
	return inter / union
}

func floatMapToAnyMap(box map[string]float64) map[string]any {
	if len(box) == 0 {
		return nil
	}
	return map[string]any{
		"left":   roundFloat(box["left"], 6),
		"right":  roundFloat(box["right"], 6),
		"top":    roundFloat(box["top"], 6),
		"bottom": roundFloat(box["bottom"], 6),
	}
}

func truncateLayoutText(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(text)
	if len(r) <= maxRunes {
		return text
	}
	return string(r[:maxRunes])
}

func buildLayoutRepairChunks(sourceName string, pageIndex int, lines []localLayoutLine, start int) []map[string]any {
	if len(lines) == 0 {
		return nil
	}
	sortLocalLayoutLines(lines)
	groups := groupLayoutRepairLines(lines)
	out := make([]map[string]any, 0, len(groups))
	for i, group := range groups {
		text := joinLayoutLineTexts(group)
		if strings.TrimSpace(text) == "" {
			continue
		}
		box := unionLayoutLineBoxes(group)
		if len(box) == 0 {
			continue
		}
		chunkType := "paragraph"
		if len(group) == 1 && looksLikeLayoutHeading(group[0].Text) {
			chunkType = "heading"
		}
		if tableText, ok := markdownTableFromNumericRows(text); ok {
			chunkType = "table"
			text = tableText
		}
		out = append(out, map[string]any{
			"chunk_id":   fmt.Sprintf("layout_repair_p%d_%d", pageIndex, start+i),
			"type":       chunkType,
			"text":       text,
			"page_index": pageIndex,
			"order":      880000 + pageIndex*100 + start + i,
			"source":     "native_pdf_layout_repair",
			"confidence": 0.76,
			"bbox":       box,
			"payload": map[string]any{
				"layout_region": "page_region",
				"repair":        "geometry_text_missing_from_chunks",
				"strategy":      "opendataloader_style_quality_gate",
				"source":        sourceName,
				"table_repair":  chunkType == "table",
			},
		})
	}
	return out
}

func groupLayoutRepairLines(lines []localLayoutLine) [][]localLayoutLine {
	if len(lines) == 0 {
		return nil
	}
	var groups [][]localLayoutLine
	current := make([]localLayoutLine, 0, 8)
	flush := func() {
		if len(current) > 0 {
			groups = append(groups, current)
			current = nil
		}
	}
	for _, line := range lines {
		if len(current) == 0 {
			current = append(current, line)
			continue
		}
		prev := current[len(current)-1]
		gap := line.BBox["top"] - prev.BBox["bottom"]
		columnShift := math.Abs(line.X - prev.X)
		if looksLikeLayoutHeading(line.Text) || gap > 0.055 || columnShift > 0.18 || len(current) >= 12 {
			flush()
		}
		current = append(current, line)
	}
	flush()
	return groups
}

func looksLikeLayoutHeading(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" || utf8.RuneCountInString(t) > 72 {
		return false
	}
	if strings.HasSuffix(t, ":") || strings.HasSuffix(t, "：") {
		return true
	}
	if strings.ContainsAny(t, ".。:：,，;；") {
		return false
	}
	letters, digits := 0, 0
	for _, r := range t {
		if unicode.IsLetter(r) {
			letters++
		}
		if unicode.IsDigit(r) {
			digits++
		}
	}
	if letters == 0 || digits > letters {
		return false
	}
	words := strings.Fields(t)
	if len(words) > 0 {
		return len(words) <= 8
	}
	return utf8.RuneCountInString(t) <= 18
}

func markdownTableFromNumericRows(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	type row struct {
		label string
		a     string
		b     string
	}
	rows := make([]row, 0)
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 {
			continue
		}
		a := fields[len(fields)-2]
		b := fields[len(fields)-1]
		if !looksLikePercentToken(a) || !looksLikePercentToken(b) {
			continue
		}
		label := strings.Join(fields[:len(fields)-2], " ")
		if label == "" {
			continue
		}
		rows = append(rows, row{label: label, a: a, b: b})
	}
	if len(rows) < 3 {
		return "", false
	}
	var b strings.Builder
	b.WriteString("| Item | Value 1 | Value 2 |\n")
	b.WriteString("| --- | ---: | ---: |\n")
	for _, r := range rows {
		b.WriteString("| ")
		b.WriteString(escapeMarkdownTableCell(r.label))
		b.WriteString(" | ")
		b.WriteString(escapeMarkdownTableCell(r.a))
		b.WriteString(" | ")
		b.WriteString(escapeMarkdownTableCell(r.b))
		b.WriteString(" |\n")
	}
	return b.String(), true
}

func looksLikePercentToken(token string) bool {
	token = strings.TrimSpace(token)
	if !strings.HasSuffix(token, "%") {
		return false
	}
	hasDigit := false
	for _, r := range token {
		if unicode.IsDigit(r) {
			hasDigit = true
			continue
		}
		if r != '.' && r != '%' {
			return false
		}
	}
	return hasDigit
}

func escapeMarkdownTableCell(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "|", "\\|")
}

func detectGeometryTableCandidate(lines []localLayoutLine) (bool, int) {
	if len(lines) < localQualityTableRowMin*localQualityTableCellsMin {
		return false, 0
	}
	rows := clusterLayoutRows(lines, 0.012)
	candidateRows := 0
	for _, row := range rows {
		if countSeparatedCells(row) >= localQualityTableCellsMin {
			candidateRows++
		}
	}
	return candidateRows >= localQualityTableRowMin, candidateRows
}

func clusterLayoutRows(lines []localLayoutLine, yTol float64) [][]localLayoutLine {
	sorted := append([]localLayoutLine(nil), lines...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if math.Abs(sorted[i].Y-sorted[j].Y) > yTol {
			return sorted[i].Y > sorted[j].Y
		}
		return sorted[i].X < sorted[j].X
	})
	var rows [][]localLayoutLine
	for _, line := range sorted {
		if len(rows) == 0 {
			rows = append(rows, []localLayoutLine{line})
			continue
		}
		last := rows[len(rows)-1]
		if math.Abs(avgLayoutRowY(last)-line.Y) <= yTol {
			rows[len(rows)-1] = append(last, line)
			continue
		}
		rows = append(rows, []localLayoutLine{line})
	}
	for i := range rows {
		sort.SliceStable(rows[i], func(a, b int) bool { return rows[i][a].X < rows[i][b].X })
	}
	return rows
}

func countSeparatedCells(row []localLayoutLine) int {
	if len(row) < localQualityTableCellsMin {
		return len(row)
	}
	cells := 1
	for i := 1; i < len(row); i++ {
		gap := row[i].X - row[i-1].X
		if gap > 0.055 {
			cells++
		}
	}
	return cells
}

func avgLayoutRowY(row []localLayoutLine) float64 {
	if len(row) == 0 {
		return 0
	}
	sum := 0.0
	for _, line := range row {
		sum += line.Y
	}
	return sum / float64(len(row))
}

func hasPageChunkType(chunks []localLayoutChunk, typ string) bool {
	for _, ch := range chunks {
		if strings.EqualFold(strings.TrimSpace(stringFromAnyLocal(ch.Chunk["type"])), typ) {
			return true
		}
	}
	return false
}

func orderChunkMapsByLayout(chunks []map[string]any) []map[string]any {
	ordered, _, err := layoutdoc.NormalizeAndOrderChunkMaps("", 0, chunks)
	if err != nil {
		return chunks
	}
	return ordered
}

func sortLocalLayoutLines(lines []localLayoutLine) {
	sort.SliceStable(lines, func(i, j int) bool {
		if math.Abs(lines[i].BBox["top"]-lines[j].BBox["top"]) > 0.012 {
			return lines[i].BBox["top"] < lines[j].BBox["top"]
		}
		if math.Abs(lines[i].X-lines[j].X) > 0.008 {
			return lines[i].X < lines[j].X
		}
		return lines[i].Order < lines[j].Order
	})
}

func unionLayoutLineBoxes(lines []localLayoutLine) map[string]any {
	if len(lines) == 0 {
		return nil
	}
	left, right := lines[0].BBox["left"], lines[0].BBox["right"]
	top, bottom := lines[0].BBox["top"], lines[0].BBox["bottom"]
	for _, line := range lines[1:] {
		left = math.Min(left, line.BBox["left"])
		right = math.Max(right, line.BBox["right"])
		top = math.Min(top, line.BBox["top"])
		bottom = math.Max(bottom, line.BBox["bottom"])
	}
	return map[string]any{
		"left":   roundFloat(left, 6),
		"right":  roundFloat(right, 6),
		"top":    roundFloat(top, 6),
		"bottom": roundFloat(bottom, 6),
	}
}

func joinLayoutLineTexts(lines []localLayoutLine) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		t := strings.TrimSpace(line.Text)
		if t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}

func chunkTextLinesForSummary(chunks []map[string]any) []string {
	out := make([]string, 0, len(chunks))
	for _, ch := range chunks {
		if isDiagnosticChunk(ch) {
			continue
		}
		text := strings.TrimSpace(stringFromAnyLocal(ch["text"]))
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

func comparableLayoutText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func chunkBBoxTopLeft(ch map[string]any) (map[string]float64, bool) {
	raw, ok := ch["bbox"].(map[string]any)
	if !ok {
		return nil, false
	}
	box := anyMapToFloatMap(raw)
	for _, key := range []string{"left", "right", "top", "bottom"} {
		if _, ok := box[key]; !ok {
			return nil, false
		}
	}
	if box["right"] <= box["left"] || box["bottom"] <= box["top"] {
		return nil, false
	}
	return box, true
}

func adapterGeometryBBoxTopLeftMap(box *pdfadapter.TextBBox, g chunking.PageGeom) map[string]float64 {
	if box == nil {
		return nil
	}
	if box.Left >= -0.01 && box.Left <= 1.01 &&
		box.Right >= -0.01 && box.Right <= 1.01 &&
		box.Top >= -0.01 && box.Top <= 1.01 &&
		box.Bottom >= -0.01 && box.Bottom <= 1.01 &&
		(g.Right-g.Left) > 2 && (g.Top-g.Bottom) > 2 {
		left := clamp01Local(math.Min(box.Left, box.Right))
		right := clamp01Local(math.Max(box.Left, box.Right))
		top := clamp01Local(math.Min(box.Top, box.Bottom))
		bottom := clamp01Local(math.Max(box.Top, box.Bottom))
		if right > left && bottom > top {
			return map[string]float64{
				"left": left, "right": right, "top": top, "bottom": bottom,
			}
		}
	}
	normalized := normalizeAdapterBBox(box, g)
	if normalized == nil {
		return nil
	}
	anyBox := chunking.BBoxTopLeftMap(normalized)
	if anyBox == nil {
		return nil
	}
	return anyMapToFloatMap(anyBox)
}

func anyMapToFloatMap(raw map[string]any) map[string]float64 {
	out := make(map[string]float64, len(raw))
	for k, v := range raw {
		switch n := v.(type) {
		case float64:
			out[k] = n
		case float32:
			out[k] = float64(n)
		case int:
			out[k] = float64(n)
		case int64:
			out[k] = float64(n)
		case jsonNumberLike:
			if f, err := n.Float64(); err == nil {
				out[k] = f
			}
		}
	}
	return out
}

type jsonNumberLike interface {
	Float64() (float64, error)
}

func isDiagnosticChunk(ch map[string]any) bool {
	typ := strings.TrimSpace(stringFromAnyLocal(ch["type"]))
	return typ == "table_debug" || typ == "bookmark" || typ == "attachment"
}

func intFromAnyLocal(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	default:
		return 0
	}
}

func stringFromAnyLocal(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func clamp01Local(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func roundFloat(v float64, digits int) float64 {
	p := math.Pow10(digits)
	return math.Round(v*p) / p
}
