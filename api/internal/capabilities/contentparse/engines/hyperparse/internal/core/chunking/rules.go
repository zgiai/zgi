package chunking

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Rule interface {
	Name() string
	Apply(input BuildInput, chunks []Chunk) []Chunk
}

type ruleFn struct {
	name string
	fn   func(input BuildInput, chunks []Chunk) []Chunk
}

func (r ruleFn) Name() string { return r.name }

func (r ruleFn) Apply(input BuildInput, chunks []Chunk) []Chunk {
	return r.fn(input, chunks)
}

func DefaultRules() []Rule {
	return []Rule{
		ruleFn{name: "text_heading", fn: applyTextHeadingRule},
		ruleFn{name: "text_kv", fn: applyTextKVRule},
		ruleFn{name: "text_list", fn: applyTextListRule},
		ruleFn{name: "text_formula", fn: applyTextFormulaRule},
		ruleFn{name: "table", fn: applyTableRule},
		ruleFn{name: "text_paragraph", fn: applyTextParagraphRule},
		ruleFn{name: "stamp", fn: applyStampRule},
		ruleFn{name: "image", fn: applyImageRule},
		ruleFn{name: "bookmark", fn: applyBookmarkRule},
		ruleFn{name: "annotation", fn: applyAnnotationRule},
		ruleFn{name: "form", fn: applyFormRule},
		ruleFn{name: "attachment", fn: applyAttachmentRule},
	}
}

func applyTextHeadingRule(input BuildInput, chunks []Chunk) []Chunk {
	for _, s := range input.Texts {
		id := textLikeChunkID(s)
		if hasChunkID(chunks, id) {
			continue
		}
		if _, _, ok := splitKV(strings.TrimSpace(s.Text)); ok {
			continue
		}
		if looksLikeListItem(s.Text) {
			continue
		}
		t := strings.TrimSpace(s.ChunkType)
		if t == "heading" || looksLikeHeading(s.Text) {
			chunks = append(chunks, buildTextChunk(s, input.PageGeoms, "heading", 0.88))
		}
	}
	return chunks
}

func applyTextKVRule(input BuildInput, chunks []Chunk) []Chunk {
	for _, s := range input.Texts {
		id := textLikeChunkID(s)
		if hasChunkID(chunks, id) {
			continue
		}
		if k, v, ok := splitKV(s.Text); ok {
			c := buildTextChunk(s, input.PageGeoms, "kv", 0.86)
			c.Payload = map[string]any{
				"key":   k,
				"value": v,
			}
			chunks = append(chunks, c)
		}
	}
	return chunks
}

func applyTextListRule(input BuildInput, chunks []Chunk) []Chunk {
	for _, s := range input.Texts {
		id := textLikeChunkID(s)
		if hasChunkID(chunks, id) {
			continue
		}
		if looksLikeListItem(s.Text) {
			chunks = append(chunks, buildTextChunk(s, input.PageGeoms, "list_item", 0.84))
		}
	}
	return chunks
}

func applyTextFormulaRule(input BuildInput, chunks []Chunk) []Chunk {
	for _, s := range input.Texts {
		id := textLikeChunkID(s)
		if hasChunkID(chunks, id) {
			continue
		}
		if !looksLikeFormulaText(s.Text) {
			continue
		}
		c := buildTextChunk(s, input.PageGeoms, "formula", 0.87)
		c.Text = normalizeFormulaText(s.Text)
		if hint := latexHintFromNativeSegment(s.Text); hint != "" {
			if c.Payload == nil {
				c.Payload = map[string]any{}
			}
			c.Payload["latex_hint"] = hint
			c.Payload["native_formula_rule"] = "chunking_v2"
		}
		chunks = append(chunks, c)
	}
	return chunks
}

func applyTextParagraphRule(input BuildInput, chunks []Chunk) []Chunk {
	for _, s := range input.Texts {
		id := textLikeChunkID(s)
		if hasChunkID(chunks, id) {
			continue
		}
		baseType := strings.TrimSpace(s.ChunkType)
		if baseType == "" {
			baseType = "paragraph"
		}
		conf := 0.82
		if baseType != "paragraph" {
			conf = 0.75
		}
		chunks = append(chunks, buildTextChunk(s, input.PageGeoms, baseType, conf))
	}
	return chunks
}

type textPoint struct {
	seg  TextLike
	page int
	x    float64
	y    float64
	bbox *BBox
}

type tableRow struct {
	points []textPoint
}

func applyTableRule(input BuildInput, chunks []Chunk) []Chunk {
	if len(input.GeometryTokens) > 0 {
		v2 := applyTableGeometryTokensV2(input)
		return append(chunks, v2...)
	}

	pointsByPage := map[int][]textPoint{}
	if len(input.GeometryLines) > 0 {
		for _, gl := range input.GeometryLines {
			pageIndex := gl.PageIndex
			if pageIndex <= 0 {
				pageIndex = pageIndexFromTrace(gl.SourceTrace)
			}
			g, ok := input.PageGeoms[pageIndex]
			if !ok || g.Right <= g.Left || g.Top <= g.Bottom || (gl.GeomX == 0 && gl.GeomY == 0) {
				continue
			}
			x := clamp01((gl.GeomX - g.Left) / (g.Right - g.Left))
			y := clamp01((gl.GeomY - g.Bottom) / (g.Top - g.Bottom))
			pointsByPage[pageIndex] = append(pointsByPage[pageIndex], textPoint{
				seg: TextLike{
					Order:       gl.Order,
					SourceTrace: gl.SourceTrace,
					Text:        gl.Text,
					ChunkType:   "line",
					GeomX:       gl.GeomX,
					GeomY:       gl.GeomY,
					BBox:        gl.BBox,
				},
				page: pageIndex,
				x:    x,
				y:    y,
				bbox: func() *BBox {
					if gl.BBox != nil {
						return gl.BBox
					}
					return textAnchorToBBox(gl.GeomX, gl.GeomY, g)
				}(),
			})
		}
	} else {
		for _, s := range input.Texts {
			pageIndex := pageIndexFromTrace(s.SourceTrace)
			g, ok := input.PageGeoms[pageIndex]
			if !ok || g.Right <= g.Left || g.Top <= g.Bottom || (s.GeomX == 0 && s.GeomY == 0) {
				continue
			}
			x := clamp01((s.GeomX - g.Left) / (g.Right - g.Left))
			y := clamp01((s.GeomY - g.Bottom) / (g.Top - g.Bottom))
			pointsByPage[pageIndex] = append(pointsByPage[pageIndex], textPoint{
				seg:  s,
				page: pageIndex,
				x:    x,
				y:    y,
				bbox: func() *BBox {
					if s.BBox != nil {
						return s.BBox
					}
					return textAnchorToBBox(s.GeomX, s.GeomY, g)
				}(),
			})
		}
	}

	tableIdx := 0
	for pageIndex, pts := range pointsByPage {
		if len(pts) < 4 {
			continue
		}
		sort.Slice(pts, func(i, j int) bool {
			if math.Abs(pts[i].y-pts[j].y) > 0.008 {
				return pts[i].y > pts[j].y
			}
			return pts[i].x < pts[j].x
		})
		rows := clusterRows(pts, 0.02)
		if len(rows) < 2 {
			continue
		}
		blocks := detectTableBlocks(rows)
		for _, b := range blocks {
			if len(b) < 2 {
				continue
			}
			c := buildTableChunk(pageIndex, tableIdx, b)
			if c != nil {
				chunks = append(chunks, *c)
				tableIdx++
			}
		}
	}
	return chunks
}

func clusterRows(points []textPoint, yTol float64) []tableRow {
	rows := make([]tableRow, 0, 8)
	for _, p := range points {
		if len(rows) == 0 {
			rows = append(rows, tableRow{points: []textPoint{p}})
			continue
		}
		last := &rows[len(rows)-1]
		refY := avgRowY(*last)
		if math.Abs(refY-p.y) <= yTol {
			last.points = append(last.points, p)
			continue
		}
		rows = append(rows, tableRow{points: []textPoint{p}})
	}
	for i := range rows {
		sort.Slice(rows[i].points, func(a, b int) bool {
			return rows[i].points[a].x < rows[i].points[b].x
		})
	}
	return rows
}

func detectTableBlocks(rows []tableRow) [][]tableRow {
	blocks := make([][]tableRow, 0, 4)
	cur := make([]tableRow, 0, 4)
	for _, r := range rows {
		if len(r.points) >= 2 {
			cur = append(cur, r)
			continue
		}
		if len(cur) >= 2 {
			blocks = append(blocks, cur)
		}
		cur = nil
	}
	if len(cur) >= 2 {
		blocks = append(blocks, cur)
	}
	return blocks
}

func buildTableChunk(pageIndex, tableIdx int, rows []tableRow) *Chunk {
	if len(rows) < 2 {
		return nil
	}
	columnAnchors := deriveColumnAnchors(rows)
	if len(columnAnchors) < 2 {
		return nil
	}
	if !isLikelyTable(rows, columnAnchors) {
		return nil
	}
	cells := make([]map[string]any, 0, len(rows)*len(columnAnchors))
	left, right, top, bottom := 1.0, 0.0, 0.0, 1.0
	lineTexts := make([]string, 0, len(rows))
	for rIdx, r := range rows {
		rowTexts := make([]string, 0, len(r.points))
		for _, p := range r.points {
			colIdx := nearestColumnIndex(p.x, columnAnchors)
			if colIdx < 0 {
				continue
			}
			bb := p.bbox
			if bb == nil {
				continue
			}
			left = math.Min(left, bb.Left)
			right = math.Max(right, bb.Right)
			top = math.Max(top, bb.Top)
			bottom = math.Min(bottom, bb.Bottom)
			cells = append(cells, map[string]any{
				"row":  rIdx,
				"col":  colIdx,
				"text": p.seg.Text,
				"bbox": map[string]any{
					"left":   bb.Left,
					"right":  bb.Right,
					"top":    bb.Top,
					"bottom": bb.Bottom,
				},
			})
			rowTexts = append(rowTexts, p.seg.Text)
		}
		lineTexts = append(lineTexts, strings.Join(rowTexts, " | "))
	}
	if len(cells) < 4 || right <= left || top <= bottom {
		return nil
	}
	nRow, nCol := len(rows), len(columnAnchors)
	payload := map[string]any{
		"row_count":    nRow,
		"column_count": nCol,
		"cells":        cells,
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
		Confidence: 0.76,
		BBox: &BBox{
			Left:   round(left, 6),
			Right:  round(right, 6),
			Top:    round(top, 6),
			Bottom: round(bottom, 6),
		},
		Payload: payload,
	}
}

func deriveColumnAnchors(rows []tableRow) []float64 {
	all := make([]float64, 0, 16)
	for _, r := range rows {
		for _, p := range r.points {
			all = append(all, p.x)
		}
	}
	if len(all) == 0 {
		return nil
	}
	sort.Float64s(all)
	anchors := make([]float64, 0, len(all))
	for _, x := range all {
		if len(anchors) == 0 || math.Abs(x-anchors[len(anchors)-1]) > 0.06 {
			anchors = append(anchors, x)
			continue
		}
		anchors[len(anchors)-1] = (anchors[len(anchors)-1] + x) / 2
	}
	return anchors
}

func isLikelyTable(rows []tableRow, anchors []float64) bool {
	if len(rows) < 2 || len(anchors) < 2 {
		return false
	}
	coveredRows := 0
	maxCols := 0
	for _, r := range rows {
		cols := map[int]bool{}
		for _, p := range r.points {
			ci := nearestColumnIndex(p.x, anchors)
			if ci >= 0 {
				cols[ci] = true
			}
		}
		if len(cols) >= 2 {
			coveredRows++
			if len(cols) > maxCols {
				maxCols = len(cols)
			}
		}
	}
	// Need at least 2 rows spanning >=2 columns.
	if coveredRows < 2 || maxCols < 2 {
		return false
	}
	// Reject list-like "single left gutter + text column".
	if len(anchors) == 2 && (anchors[1]-anchors[0]) < 0.12 {
		return false
	}
	return true
}

func avgRowY(r tableRow) float64 {
	if len(r.points) == 0 {
		return 0
	}
	sum := 0.0
	for _, p := range r.points {
		sum += p.y
	}
	return sum / float64(len(r.points))
}

func nearestColumnIndex(x float64, cols []float64) int {
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
	if bestDist > 0.08 {
		return -1
	}
	return best
}

func applyStampRule(input BuildInput, chunks []Chunk) []Chunk {
	repeatedStampLike := samePageStampLikeImageCounts(input.Images)
	stampIdx := 0
	for _, im := range input.Images {
		if !isLikelyStampImage(im) {
			continue
		}
		if repeatedStampLike[stampLikeImageKey(im)] > 1 {
			continue
		}
		payload := map[string]any{
			"page_index":    im.PageIndex,
			"page_object":   im.PageObject,
			"xobject_name":  im.XObjectName,
			"object_number": im.ObjectNumber,
			"format":        im.Format,
			"width":         im.Width,
			"height":        im.Height,
			"byte_size":     im.ByteSize,
			"image_kind":    "stamp",
		}
		if im.DecodeWarning != "" {
			payload["decode_warning"] = im.DecodeWarning
		}
		chunks = append(chunks, Chunk{
			ChunkID:    "stamp_" + strconv.Itoa(stampIdx),
			Type:       "stamp",
			PageIndex:  im.PageIndex,
			Order:      95000 + stampIdx,
			Source:     "native_pdf",
			Confidence: 0.9,
			Payload:    payload,
		})
		stampIdx++
	}
	return chunks
}

func samePageStampLikeImageCounts(images []ImageLike) map[string]int {
	out := map[string]int{}
	for _, im := range images {
		if isLikelyStampImage(im) {
			out[stampLikeImageKey(im)]++
		}
	}
	return out
}

func stampLikeImageKey(im ImageLike) string {
	return strconv.Itoa(im.PageIndex) + ":" + strconv.Itoa(im.Width) + "x" + strconv.Itoa(im.Height)
}

func applyImageRule(input BuildInput, chunks []Chunk) []Chunk {
	stampObjects := map[int]bool{}
	for _, c := range chunks {
		if c.Type != "stamp" || c.Payload == nil {
			continue
		}
		if on, ok := c.Payload["object_number"].(int); ok && on > 0 {
			stampObjects[on] = true
		} else if onf, ok := c.Payload["object_number"].(float64); ok && int(onf) > 0 {
			stampObjects[int(onf)] = true
		}
	}
	objHits := map[int]int{}
	for _, im := range input.Images {
		if im.ObjectNumber > 0 {
			objHits[im.ObjectNumber]++
		}
	}
	for i, im := range input.Images {
		if im.ObjectNumber > 0 && stampObjects[im.ObjectNumber] {
			continue
		}
		if isLikelyFullPageBackgroundImage(im, input.PageGeoms) || isLikelyRepeatedBannerImage(im, objHits) {
			// Ignore near full-page background images, such as scanned-page JPEGs,
			// so they do not pollute the chunk list.
			continue
		}
		payload := map[string]any{
			"page_index":    im.PageIndex,
			"page_object":   im.PageObject,
			"xobject_name":  im.XObjectName,
			"object_number": im.ObjectNumber,
			"format":        im.Format,
			"width":         im.Width,
			"height":        im.Height,
			"byte_size":     im.ByteSize,
		}
		if im.DecodeWarning != "" {
			payload["decode_warning"] = im.DecodeWarning
		}
		chunks = append(chunks, Chunk{
			ChunkID:    "img_" + strconv.Itoa(i),
			Type:       "image",
			PageIndex:  im.PageIndex,
			Order:      100000 + i,
			Source:     "native_pdf",
			Confidence: 0.95,
			Payload:    payload,
		})
	}
	return chunks
}

func isLikelyFullPageBackgroundImage(im ImageLike, pageGeoms map[int]PageGeom) bool {
	if im.Width <= 0 || im.Height <= 0 {
		return false
	}
	g, ok := pageGeoms[im.PageIndex]
	if !ok {
		return false
	}
	pw := g.Right - g.Left
	ph := g.Top - g.Bottom
	if pw <= 0 || ph <= 0 {
		return false
	}
	// Approximate area threshold: enough pixels for A4@150dpi scale and a page-like aspect ratio.
	pxArea := float64(im.Width * im.Height)
	if pxArea < 1200*1600 {
		return false
	}
	imgAspect := float64(im.Width) / float64(im.Height)
	pageAspect := pw / ph
	if imgAspect <= 0 || pageAspect <= 0 {
		return false
	}
	diff := math.Abs(imgAspect-pageAspect) / math.Max(imgAspect, pageAspect)
	// Aspect-ratio difference within 12% is considered page-like.
	return diff <= 0.12
}

func isLikelyRepeatedBannerImage(im ImageLike, objHits map[int]int) bool {
	if im.ObjectNumber <= 0 || objHits[im.ObjectNumber] < 2 {
		return false
	}
	if im.Width <= 0 || im.Height <= 0 {
		return false
	}
	area := im.Width * im.Height
	// Reused wide strips across pages are often header/footer backgrounds.
	if area >= 1800*900 && float64(im.Width)/float64(im.Height) >= 2.1 {
		return true
	}
	// Extremely wide strips are filtered even when their area is slightly smaller.
	return im.Width >= 2200 && im.Height <= 1200
}

func isLikelyStampImage(im ImageLike) bool {
	if im.Width <= 0 || im.Height <= 0 {
		return false
	}
	if im.ObjectNumber <= 0 {
		return false
	}
	// Stamps are usually near-square or slightly elliptical, not huge full-page images.
	aspect := float64(im.Width) / float64(im.Height)
	if aspect <= 0 {
		return false
	}
	if aspect < 1 {
		aspect = 1 / aspect
	}
	// Slightly relaxed for bank statement e-stamps, which are often horizontal
	// ellipses or RGB+SMask strips around 1.8:1 and can be below 150^2 pixels.
	if aspect > 2.0 {
		return false
	}
	area := im.Width * im.Height
	if area < 100*100 || area > 1100*1100 {
		return false
	}
	// Typical stamp byte size range: very large images are usually backgrounds,
	// while very small images are often noise or icons.
	if im.ByteSize > 0 && (im.ByteSize < 4000 || im.ByteSize > 900000) {
		return false
	}
	return true
}

func applyBookmarkRule(input BuildInput, chunks []Chunk) []Chunk {
	for i, bm := range input.Bookmarks {
		chunks = append(chunks, Chunk{
			ChunkID:    "bookmark_" + strconv.Itoa(i),
			Type:       "bookmark",
			Text:       bm.Title,
			PageIndex:  pageIndexByPageObject(input.Pages, bm.PageObject),
			Order:      200000 + i,
			Source:     "native_pdf",
			Confidence: 0.9,
			Payload: map[string]any{
				"title":               bm.Title,
				"page_object":         bm.PageObject,
				"outline_item_object": bm.Object,
				"level":               bm.Level,
				"dest":                bm.Dest,
				"target_raw":          bm.TargetRaw,
				"target_kind":         bm.TargetKind,
			},
		})
	}
	return chunks
}

func applyAnnotationRule(input BuildInput, chunks []Chunk) []Chunk {
	for i, an := range input.Annotations {
		chunks = append(chunks, Chunk{
			ChunkID:    "annot_" + strconv.Itoa(i),
			Type:       "annotation",
			Text:       an.Contents,
			PageIndex:  an.PageIndex,
			Order:      300000 + i,
			Source:     "native_pdf",
			Confidence: 0.9,
			BBox:       rectToBBox(an.Rect, input.PageGeoms[an.PageIndex]),
			Payload: map[string]any{
				"page_index":    an.PageIndex,
				"object_number": an.ObjectNumber,
				"subtype":       an.Subtype,
				"rect":          an.Rect,
				"contents":      an.Contents,
			},
		})
	}
	return chunks
}

func applyFormRule(input BuildInput, chunks []Chunk) []Chunk {
	for i, f := range input.Forms {
		pi := pageIndexByPageObject(input.Pages, f.PageObject)
		chunks = append(chunks, Chunk{
			ChunkID:    "form_" + strconv.Itoa(i),
			Type:       "form_field",
			Text:       f.Name,
			PageIndex:  pi,
			Order:      400000 + i,
			Source:     "native_pdf",
			Confidence: 0.9,
			BBox:       rectToBBox(f.Rect, input.PageGeoms[pi]),
			Payload: map[string]any{
				"object_number": f.ObjectNumber,
				"name":          f.Name,
				"alt_name":      f.AltName,
				"field_type":    f.FieldType,
				"value":         f.Value,
				"flags":         f.Flags,
				"page_object":   f.PageObject,
				"rect":          f.Rect,
			},
		})
	}
	return chunks
}

func applyAttachmentRule(input BuildInput, chunks []Chunk) []Chunk {
	for i, at := range input.Attachments {
		name := at.FileName
		if strings.TrimSpace(name) == "" {
			name = at.UnicodeFileName
		}
		chunks = append(chunks, Chunk{
			ChunkID:    "attachment_" + strconv.Itoa(i),
			Type:       "attachment",
			Text:       name,
			PageIndex:  0,
			Order:      500000 + i,
			Source:     "native_pdf",
			Confidence: 0.9,
			Payload: map[string]any{
				"filespec_object":      at.FileSpecObject,
				"file_name":            at.FileName,
				"unicode_file_name":    at.UnicodeFileName,
				"embedded_file_object": at.EmbeddedFileObj,
				"embedded_size_bytes":  at.EmbeddedSizeBytes,
				"embedded_subtype":     at.EmbeddedSubtype,
			},
		})
	}
	return chunks
}

var (
	headingRE    = regexp.MustCompile(`^[A-Z][A-Z0-9 \-_/()]{2,}$`)
	listRE       = regexp.MustCompile(`^\s*(?:[\-\*\x{2022}]|\d+[.)]|[A-Za-z][.)])\s+`)
	formulaVarRE = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_]*`)
)

func hasChunkID(chunks []Chunk, id string) bool {
	for i := range chunks {
		if chunks[i].ChunkID == id {
			return true
		}
	}
	return false
}

func buildTextChunk(s TextLike, pageGeoms map[int]PageGeom, typ string, confidence float64) Chunk {
	pageIndex := pageIndexFromTrace(s.SourceTrace)
	bbox := s.BBox
	if bbox == nil {
		bbox = textAnchorToBBox(s.GeomX, s.GeomY, pageGeoms[pageIndex])
	}
	return Chunk{
		ChunkID:     textLikeChunkID(s),
		Type:        typ,
		Text:        s.Text,
		PageIndex:   pageIndex,
		Order:       s.Order,
		Source:      "native_pdf",
		Confidence:  round(confidence, 3),
		SourceTrace: s.SourceTrace,
		BBox:        bbox,
	}
}

func looksLikeHeading(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" || len(t) > 120 {
		return false
	}
	if _, _, ok := splitKV(t); ok {
		return false
	}
	if looksLikeListItem(t) {
		return false
	}
	if strings.HasSuffix(t, ":") && len(t) <= 80 {
		return true
	}
	return headingRE.MatchString(t)
}

func splitKV(text string) (string, string, bool) {
	t := strings.TrimSpace(text)
	if t == "" {
		return "", "", false
	}
	pos := strings.Index(t, ":")
	if pos <= 0 || pos >= len(t)-1 {
		return "", "", false
	}
	k := strings.TrimSpace(t[:pos])
	v := strings.TrimSpace(t[pos+1:])
	if k == "" || v == "" || len(k) > 80 {
		return "", "", false
	}
	return k, v, true
}

func looksLikeListItem(text string) bool {
	return listRE.MatchString(strings.TrimSpace(text))
}

func normalizeFormulaText(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return ""
	}
	if strings.HasPrefix(t, "formula:") {
		if strings.Contains(t, "|") {
			return t
		}
		tt := strings.TrimSpace(strings.TrimPrefix(t, "formula:"))
		if tt == "" {
			return "formula:|"
		}
		return "formula:" + tt + "|"
	}
	if strings.HasPrefix(t, legacyFormulaPrefixFullWidth) || strings.HasPrefix(t, legacyFormulaPrefixASCII) {
		tt := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(t, legacyFormulaPrefixFullWidth), legacyFormulaPrefixASCII))
		if strings.Contains(tt, "|") {
			return "formula:" + tt
		}
		if tt == "" {
			return "formula:|"
		}
		return "formula:" + tt + "|"
	}
	expr := t
	desc := ""
	if idx := strings.Index(expr, legacyFormulaWhereKeyword); idx > 0 {
		desc = strings.TrimSpace(expr[idx:])
		expr = strings.TrimSpace(expr[:idx])
	}
	return "formula:" + expr + "|" + desc
}
