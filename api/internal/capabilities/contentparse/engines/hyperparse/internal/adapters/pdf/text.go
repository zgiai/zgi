package pdf

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
)

var (
	reStreamBlock = regexp.MustCompile(`(?s)stream[\r\n]+(.*?)endstream`)
	reTjLiteral   = regexp.MustCompile(`\((?:\\.|[^\\)])*\)\s*Tj`)
	reTjHex       = regexp.MustCompile(`<[\da-fA-F\s]+>\s*Tj`)
	reTJArray     = regexp.MustCompile(`\[(?s:.*?)\]\s*TJ`)
	reLiteral     = regexp.MustCompile(`\((?:\\.|[^\\)])*\)`)
	reHexToken    = regexp.MustCompile(`<[\da-fA-F\s]+>`)
	reTfOperator  = regexp.MustCompile(`/([^\s/\[\]<>]+)\s+([-+]?\d+(?:\.\d+)?)\s+Tf`)
	reNamedRef    = regexp.MustCompile(`/([^\s/\[\]<>]+)\s+(\d+)\s+(\d+)\s+R`)

	reBeginBFChar  = regexp.MustCompile(`(?s)beginbfchar(.*?)endbfchar`)
	reBFCharLine   = regexp.MustCompile(`<([\da-fA-F\s]+)>\s*<([\da-fA-F\s]+)>`)
	reBeginBFRange = regexp.MustCompile(`(?s)beginbfrange(.*?)endbfrange`)
	reBFRangePair  = regexp.MustCompile(`<([\da-fA-F\s]+)>\s*<([\da-fA-F\s]+)>\s*<([\da-fA-F\s]+)>`)
	reBFRangeList  = regexp.MustCompile(`<([\da-fA-F\s]+)>\s*<([\da-fA-F\s]+)>\s*\[(.*?)\]`)
	reCMapNameDef  = regexp.MustCompile(`/CMapName\s*/([^\s]+)\s+def`)
	reUseCMap      = regexp.MustCompile(`/([^\s]+)\s+usecmap`)

	// Text matrix and leading, used for geometry reading order inside one content stream.
	reTmPDF           = regexp.MustCompile(`([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+Tm\b`)
	reTdPDF           = regexp.MustCompile(`([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+Td\b`)
	reTDPDF           = regexp.MustCompile(`([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+TD\b`)
	reTStarPDF        = regexp.MustCompile(`\bT\*`)
	reTsPDF           = regexp.MustCompile(`([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+Ts\b`)
	reTLPDF           = regexp.MustCompile(`([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+TL\b`)
	reTcPDF           = regexp.MustCompile(`([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+Tc\b`)
	reTwPDF           = regexp.MustCompile(`([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+Tw\b`)
	reTzPDF           = regexp.MustCompile(`([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+Tz\b`)
	reBTPDF           = regexp.MustCompile(`\bBT\b`)
	reETPDF           = regexp.MustCompile(`\bET\b`)
	reqPDF            = regexp.MustCompile(`\bq\b`)
	reQPDF            = regexp.MustCompile(`\bQ\b`)
	reCmPDF           = regexp.MustCompile(`([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+([+\-]?(?:\d+\.\d*|\.\d+|\d+))\s+cm\b`)
	reSpacedASCIIWord = regexp.MustCompile(`\b(?:[A-Za-z]\s+){2,}[A-Za-z]\b`)
	reInvalidTextRune = regexp.MustCompile(`[^\p{Han}\p{Latin}\pN\s\.,:;!?！？、—【】\(\)（）\-\+/%《》“”‘’]`)
)

const pdfContentNumberPattern = `[+\-]?(?:\d+\.\d*|\.\d+|\d+)`

var reAnyContentPDFOp = regexp.MustCompile(
	`(?s)` +
		`(\bBT\b)|` +
		`(\bET\b)|` +
		`(\bq\b)|` +
		`(\bQ\b)|` +
		`((?:` + pdfContentNumberPattern + `)\s+(?:` + pdfContentNumberPattern + `)\s+(?:` + pdfContentNumberPattern + `)\s+(?:` + pdfContentNumberPattern + `)\s+(?:` + pdfContentNumberPattern + `)\s+(?:` + pdfContentNumberPattern + `)\s+cm\b)|` +
		`((?:` + pdfContentNumberPattern + `)\s+(?:` + pdfContentNumberPattern + `)\s+(?:` + pdfContentNumberPattern + `)\s+(?:` + pdfContentNumberPattern + `)\s+(?:` + pdfContentNumberPattern + `)\s+(?:` + pdfContentNumberPattern + `)\s+Tm\b)|` +
		`((?:` + pdfContentNumberPattern + `)\s+(?:` + pdfContentNumberPattern + `)\s+TD\b)|` +
		`((?:` + pdfContentNumberPattern + `)\s+(?:` + pdfContentNumberPattern + `)\s+Td\b)|` +
		`(\bT\*)|` +
		`((?:` + pdfContentNumberPattern + `)\s+Ts\b)|` +
		`((?:` + pdfContentNumberPattern + `)\s+TL\b)|` +
		`((?:` + pdfContentNumberPattern + `)\s+Tc\b)|` +
		`((?:` + pdfContentNumberPattern + `)\s+Tw\b)|` +
		`((?:` + pdfContentNumberPattern + `)\s+Tz\b)|` +
		`(/(?:[^\s/\[\]<>]+)\s+(?:` + pdfContentNumberPattern + `)\s+Tf)|` +
		`(\((?:\\.|[^\\)])*\)\s*Tj)|` +
		`(<[\da-fA-F\s]+>\s*Tj)|` +
		`(\[(?s:.*?)\]\s*TJ)`,
)

type TextSegment struct {
	StreamIndex int    `json:"stream_index"`
	Order       int    `json:"order"`
	SourceTrace string `json:"source_trace"`
	Text        string `json:"text"`
	ChunkType   string `json:"chunk_type,omitempty"`
	// GeomX/GeomY are the first visual-line anchors in user space for coarse
	// cross-column or illustration-aware ordering. Zero means unavailable.
	GeomX float64   `json:"geom_x,omitempty"`
	GeomY float64   `json:"geom_y,omitempty"`
	BBox  *TextBBox `json:"bbox,omitempty"`
}

type GeometryLine struct {
	PageIndex   int       `json:"page_index"`
	SourceTrace string    `json:"source_trace"`
	Order       int       `json:"order"`
	Text        string    `json:"text"`
	GeomX       float64   `json:"geom_x,omitempty"`
	GeomY       float64   `json:"geom_y,omitempty"`
	BBox        *TextBBox `json:"bbox,omitempty"`
}

type GeometryToken struct {
	PageIndex   int       `json:"page_index"`
	SourceTrace string    `json:"source_trace"`
	Order       int       `json:"order"`
	Text        string    `json:"text"`
	GeomX       float64   `json:"geom_x,omitempty"`
	GeomY       float64   `json:"geom_y,omitempty"`
	FontKey     string    `json:"font_key,omitempty"`
	BaseFont    string    `json:"base_font,omitempty"`
	FontSizePt  float64   `json:"font_size_pt,omitempty"`
	BBox        *TextBBox `json:"bbox,omitempty"`
}

type TextBBox struct {
	Left   float64 `json:"left,omitempty"`
	Bottom float64 `json:"bottom,omitempty"`
	Right  float64 `json:"right,omitempty"`
	Top    float64 `json:"top,omitempty"`
}

type TextExtractPageTiming struct {
	PageIndex             int   `json:"page_index,omitempty"`
	ContentRefs           int   `json:"content_refs,omitempty"`
	RawStreamBytes        int   `json:"raw_stream_bytes,omitempty"`
	DecodedBytes          int   `json:"decoded_bytes,omitempty"`
	DecodeMs              int64 `json:"decode_ms,omitempty"`
	GeomMs                int64 `json:"geom_ms,omitempty"`
	FallbackTextExtractMs int64 `json:"fallback_text_extract_ms,omitempty"`
	GeomScanned           bool  `json:"geom_scanned,omitempty"`
	GeomSkipped           bool  `json:"geom_skipped,omitempty"`
	TotalMs               int64 `json:"total_ms,omitempty"`
}

type TextExtractTimingBreakdown struct {
	CMapIndexMs                int64                   `json:"cmap_index_ms,omitempty"`
	PageCount                  int                     `json:"page_count,omitempty"`
	PageWithContentsCount      int                     `json:"page_with_contents_count,omitempty"`
	GeomScannedPages           int                     `json:"geom_scanned_pages,omitempty"`
	GeomSkippedPages           int                     `json:"geom_skipped_pages,omitempty"`
	FallbackTextExtractPages   int                     `json:"fallback_text_extract_pages,omitempty"`
	TotalDecodeMs              int64                   `json:"total_decode_ms,omitempty"`
	TotalGeomMs                int64                   `json:"total_geom_ms,omitempty"`
	TotalFallbackTextExtractMs int64                   `json:"total_fallback_text_extract_ms,omitempty"`
	TotalMs                    int64                   `json:"total_ms,omitempty"`
	SlowPages                  []TextExtractPageTiming `json:"slow_pages,omitempty"`
}

func (p TextExtractTimingBreakdown) ToMap() map[string]any {
	out := map[string]any{}
	if p.CMapIndexMs > 0 {
		out["cmap_index_ms"] = p.CMapIndexMs
	}
	if p.PageCount > 0 {
		out["page_count"] = p.PageCount
	}
	if p.PageWithContentsCount > 0 {
		out["page_with_contents_count"] = p.PageWithContentsCount
	}
	if p.GeomScannedPages > 0 {
		out["geom_scanned_pages"] = p.GeomScannedPages
	}
	if p.GeomSkippedPages > 0 {
		out["geom_skipped_pages"] = p.GeomSkippedPages
	}
	if p.FallbackTextExtractPages > 0 {
		out["fallback_text_extract_pages"] = p.FallbackTextExtractPages
	}
	if p.TotalDecodeMs > 0 {
		out["total_decode_ms"] = p.TotalDecodeMs
	}
	if p.TotalGeomMs > 0 {
		out["total_geom_ms"] = p.TotalGeomMs
	}
	if p.TotalFallbackTextExtractMs > 0 {
		out["total_fallback_text_extract_ms"] = p.TotalFallbackTextExtractMs
	}
	if p.TotalMs > 0 {
		out["total_ms"] = p.TotalMs
	}
	if len(p.SlowPages) > 0 {
		slowPages := make([]map[string]any, 0, len(p.SlowPages))
		for _, page := range p.SlowPages {
			row := map[string]any{
				"page_index":   page.PageIndex,
				"content_refs": page.ContentRefs,
				"total_ms":     page.TotalMs,
			}
			if page.RawStreamBytes > 0 {
				row["raw_stream_bytes"] = page.RawStreamBytes
			}
			if page.DecodedBytes > 0 {
				row["decoded_bytes"] = page.DecodedBytes
			}
			if page.DecodeMs > 0 {
				row["decode_ms"] = page.DecodeMs
			}
			if page.GeomMs > 0 {
				row["geom_ms"] = page.GeomMs
			}
			if page.FallbackTextExtractMs > 0 {
				row["fallback_text_extract_ms"] = page.FallbackTextExtractMs
			}
			if page.GeomScanned {
				row["geom_scanned"] = true
			}
			if page.GeomSkipped {
				row["geom_skipped"] = true
			}
			slowPages = append(slowPages, row)
		}
		out["slow_pages"] = slowPages
	}
	return out
}

type aff3 struct {
	a, b, c, d, e, f float64
}

type geomTextRun struct {
	s          string
	x          float64
	y          float64
	bbox       *TextBBox
	b          int     // Position inside the content stream, used for stable sorting.
	fontKey    string  // Resource name from the Tf operator, for example F1.
	fontSizePt float64 // Tf font size in points; zero when unknown.
	baseFont   string  // BaseFont PostScript name parsed from Resources; may be empty.
}

type pdfTextStateSnap struct {
	ctm        aff3
	tlm        aff3
	tm         aff3
	tl         float64
	textRise   float64
	fontSize   float64
	charSpace  float64
	wordSpace  float64
	horizScale float64 // Tz percentage, default 100.
	curFont    string
}

func affIdentity() aff3 {
	return aff3{a: 1, b: 0, c: 0, d: 1, e: 0, f: 0}
}

func affFromPDFMatrix(a, b, c, d, e, f float64) aff3 {
	return aff3{a: a, b: b, c: c, d: d, e: e, f: f}
}

func mulAff(A, B aff3) aff3 {
	return aff3{
		a: A.a*B.a + A.b*B.c,
		b: A.a*B.b + A.b*B.d,
		c: A.c*B.a + A.d*B.c,
		d: A.c*B.b + A.d*B.d,
		e: A.e*B.a + A.f*B.c + B.e,
		f: A.e*B.b + A.f*B.d + B.f,
	}
}

func translateAff(tx, ty float64) aff3 {
	return aff3{a: 1, b: 0, c: 0, d: 1, e: tx, f: ty}
}

// affApplyPoint applies a PDF affine matrix (a b c d e f) to row vector [x y 1].
func affApplyPoint(M aff3, x, y float64) (float64, float64) {
	return M.a*x + M.c*y + M.e, M.b*x + M.d*y + M.f
}

func fontMapForOp(fontMaps map[string]cmapUnicodeMap, curFont string) cmapUnicodeMap {
	if fontMaps == nil {
		return cmapUnicodeMap{}
	}
	return fontMaps[curFont]
}

// geomTextAnchorUserSpace maps text-space point (0, textRise) through tm and ctm
// into user space, matching PDF Ts/text-matrix semantics.
func geomTextAnchorUserSpace(ctm, tm aff3, textRise float64) (float64, float64) {
	tx, ty := affApplyPoint(tm, 0, textRise)
	return affApplyPoint(ctm, tx, ty)
}

func geomRectUserSpace(ctm, tm aff3, left, bottom, right, top float64) *TextBBox {
	if right <= left || top <= bottom {
		return nil
	}
	corners := [4][2]float64{
		{left, bottom},
		{right, bottom},
		{left, top},
		{right, top},
	}
	x0, y0 := affApplyPoint(tm, corners[0][0], corners[0][1])
	x0, y0 = affApplyPoint(ctm, x0, y0)
	minX, maxX := x0, x0
	minY, maxY := y0, y0
	for i := 1; i < len(corners); i++ {
		ux, uy := affApplyPoint(tm, corners[i][0], corners[i][1])
		ux, uy = affApplyPoint(ctm, ux, uy)
		if ux < minX {
			minX = ux
		}
		if ux > maxX {
			maxX = ux
		}
		if uy < minY {
			minY = uy
		}
		if uy > maxY {
			maxY = uy
		}
	}
	if maxX <= minX || maxY <= minY {
		return nil
	}
	return &TextBBox{Left: minX, Bottom: minY, Right: maxX, Top: maxY}
}

func geomTextRunBBoxUserSpace(ctm, tm aff3, textRise, width, fontSize float64) *TextBBox {
	if width <= 0 {
		return nil
	}
	if fontSize <= 0 {
		fontSize = geomDefaultFontSize
	}
	descent := fontSize * 0.18
	ascent := fontSize * 0.82
	return geomRectUserSpace(ctm, tm, 0, textRise-descent, width, textRise+ascent)
}

const geomDefaultFontSize = 12.0

// estimateTextWidthTextSpace estimates horizontal advance in text-space units
// when no font width table is available.
func estimateTextWidthTextSpace(s string, fontSize float64) float64 {
	if fontSize <= 0 {
		fontSize = geomDefaultFontSize
	}
	var w float64
	for _, r := range s {
		if r <= 0x20 {
			continue
		}
		if unicode.Is(unicode.Han, r) || (r >= 0x3040 && r <= 0x30FF) || (r >= 0xAC00 && r <= 0xD7AF) {
			w += fontSize * 0.95
		} else {
			w += fontSize * 0.48
		}
	}
	return w
}

// estimateGeomShowAdvance estimates Tj horizontal advance including Tz, Tc, and Tw.
func estimateGeomShowAdvance(s string, fontSize, charSpace, wordSpace, horizScale float64) float64 {
	if fontSize <= 0 {
		fontSize = geomDefaultFontSize
	}
	if horizScale <= 0 {
		horizScale = 100
	}
	scale := horizScale / 100
	var w float64
	for _, r := range s {
		if r <= 0x20 && r != ' ' {
			continue
		}
		var gw float64
		if unicode.Is(unicode.Han, r) || (r >= 0x3040 && r <= 0x30FF) || (r >= 0xAC00 && r <= 0xD7AF) {
			gw = fontSize * 0.95
		} else {
			gw = fontSize * 0.48
		}
		w += gw*scale + charSpace
		if r == ' ' {
			w += wordSpace
		}
	}
	return w
}

func tjNumericAdjTextSpace(num, fontSize float64) float64 {
	if fontSize <= 0 {
		fontSize = geomDefaultFontSize
	}
	return num * fontSize / 1000.0
}

func advanceTmAfterHorizontalShow(tlm, tm *aff3, w float64) {
	if w == 0 {
		return
	}
	// Showing text advances the text matrix, but the text line matrix remains
	// anchored at the line start. Td/T* are relative to the line matrix.
	*tm = mulAff(*tm, translateAff(w, 0))
}

func endOfPDFLiteralParen(b []byte, open int) int {
	if open < 0 || open >= len(b) || b[open] != '(' {
		return -1
	}
	i := open + 1
	for i < len(b) {
		if b[i] == '\\' {
			i += 2
			continue
		}
		if b[i] == ')' {
			return i + 1
		}
		i++
	}
	return -1
}

func endOfPDFHexBracket(b []byte, open int) int {
	if open < 0 || open >= len(b) || b[open] != '<' {
		return -1
	}
	i := open + 1
	for i < len(b) && b[i] != '>' {
		i++
	}
	if i >= len(b) {
		return -1
	}
	return i + 1
}

func scanPDFNumberPrefix(b []byte, from int) (end int, v float64, ok bool) {
	i := from
	if i < len(b) && (b[i] == '-' || b[i] == '+') {
		i++
	}
	start := i
	for i < len(b) && (b[i] == '.' || (b[i] >= '0' && b[i] <= '9')) {
		i++
	}
	if start == i {
		return from, 0, false
	}
	v, ok = parsePDFContentFloat(string(b[from:i]))
	if !ok {
		return from, 0, false
	}
	return i, v, true
}

type tjScanElem struct {
	s     string
	raw   []byte // Raw show bytes for s, decoded from literal/hex strings for tm advance.
	num   float64
	isNum bool
}

func scanTJArrayElements(inner []byte, cm cmapUnicodeMap) []tjScanElem {
	var out []tjScanElem
	for i := 0; i < len(inner); {
		for i < len(inner) && isPDFWhitespace(inner[i]) {
			i++
		}
		if i >= len(inner) {
			break
		}
		switch inner[i] {
		case '(':
			end := endOfPDFLiteralParen(inner, i)
			if end < 0 {
				i++
				continue
			}
			lit := inner[i:end]
			raw := decodePDFLiteralBytes(lit)
			out = append(out, tjScanElem{s: decodePDFLiteralTokenWithCMap(lit, cm), raw: raw})
			i = end
		case '<':
			end := endOfPDFHexBracket(inner, i)
			if end < 0 {
				i++
				continue
			}
			tok := inner[i:end]
			var raw []byte
			ts := bytes.TrimSpace(tok)
			if len(ts) >= 3 && ts[0] == '<' {
				hexInner := normalizeHexString(string(ts[1 : len(ts)-1]))
				if hexInner != "" {
					if len(hexInner)%2 != 0 {
						hexInner += "0"
					}
					if b, err := hex.DecodeString(hexInner); err == nil {
						raw = b
					}
				}
			}
			out = append(out, tjScanElem{s: decodePDFHexTokenWithCMap(tok, cm), raw: raw})
			i = end
		default:
			if inner[i] == '-' || inner[i] == '+' || (inner[i] >= '0' && inner[i] <= '9') || inner[i] == '.' {
				end, v, ok := scanPDFNumberPrefix(inner, i)
				if ok {
					out = append(out, tjScanElem{isNum: true, num: v})
					i = end
					continue
				}
			}
			i++
		}
	}
	return out
}

func extractTJBracketInner(seg []byte) []byte {
	i := bytes.IndexByte(seg, '[')
	if i < 0 {
		return nil
	}
	j := bytes.LastIndexByte(seg, ']')
	if j <= i {
		return nil
	}
	return seg[i+1 : j]
}

func isPDFWhitespace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f', 0:
		return true
	default:
		return false
	}
}

func parsePDFContentFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// ExtractTextBasic extracts basic text from uncompressed content streams.
// It currently handles Tj/TJ literal strings and does not apply font maps or filters.
func ExtractTextBasic(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return ExtractTextBasicFromBytes(data), nil
}

func ExtractTextBasicFromBytes(data []byte) string {
	segments := ExtractTextBasicSegmentsFromBytes(data)
	lines := make([]string, 0, len(segments))
	for _, s := range segments {
		lines = append(lines, s.Text)
	}
	return strings.Join(lines, "\n")
}

func ExtractTextBasicSegments(path string) ([]TextSegment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ExtractTextBasicSegmentsFromBytes(data), nil
}

// maxObjectsToScanForEmbeddedCMap caps object-number scanning in
// buildEmbeddedCMapNameIndex to avoid long stalls on malformed object numbers.
const maxObjectsToScanForEmbeddedCMap = 200000

// maxGeomDecodeBytes disables regex geometry scanning for very large decoded
// content streams because extractGeomTextRuns is close to O(n^2) on large buffers.
const maxGeomDecodeBytes = 6 << 20

// maxTextExtractFromStreamBytes limits regex text extraction to a stream prefix
// so extremely long vector pages do not stall parsing.
const maxTextExtractFromStreamBytes = 12 << 20

// extractTextUnifiedFromPageSpecs decodes each page content stream once and
// emits segments, geometry lines, and geometry tokens as needed.
// hadPageContentStreams means at least one page had /Contents references; in
// that case whole-file stream fallback should be avoided, especially for scans.
func extractTextUnifiedFromPageSpecs(data []byte, specs []PageRenderSpec, wantSeg, wantGeomLines, wantGeomTokens bool) ([]TextSegment, []GeometryLine, []GeometryToken, bool) {
	segs, lines, tok, hadContent, _ := extractTextUnifiedFromPageSpecsProfiled(data, specs, wantSeg, wantGeomLines, wantGeomTokens)
	return segs, lines, tok, hadContent
}

func extractTextUnifiedFromPageSpecsProfiled(data []byte, specs []PageRenderSpec, wantSeg, wantGeomLines, wantGeomTokens bool) ([]TextSegment, []GeometryLine, []GeometryToken, bool, TextExtractTimingBreakdown) {
	startedAt := time.Now()
	profile := TextExtractTimingBreakdown{PageCount: len(specs)}
	var segments []TextSegment
	var geomLines []GeometryLine
	var geomTokens []GeometryToken
	segOrder := 0
	lineOrder := 0
	tokenOrder := 0
	hadPageContentStreams := false

	tCmap := time.Now()
	// The CMap name index depends on the file, not pages. Building it per page
	// multiplies cost and is very slow for large object-number upper bounds.
	cmapNameIdx := buildEmbeddedCMapNameIndex(data, ValidationModeRelaxed)
	idxNums := IndexedDirectObjectNumbers(data)
	profile.CMapIndexMs = time.Since(tCmap).Milliseconds()
	fullDocDebugf("extractTextUnified cmap_index cmap_names=%d indexed_objs=%d elapsed_ms=%d",
		len(cmapNameIdx), len(idxNums), profile.CMapIndexMs)

	for pageIdx, sp := range specs {
		pageStartedAt := time.Now()
		pageProfile := TextExtractPageTiming{PageIndex: pageIdx + 1}
		refs := extractContentsObjectRefsForPage(data, sp.ObjectNumber)
		if len(refs) == 0 && sp.ContentsRefObject > 0 {
			refs = append(refs, sp.ContentsRefObject)
		}
		if len(refs) == 0 {
			continue
		}
		pageProfile.ContentRefs = len(refs)
		profile.PageWithContentsCount++
		hadPageContentStreams = true
		fullDocDebugf("extractTextUnified page=%d/%d content_refs=%d", pageIdx+1, len(specs), len(refs))
		fontMaps := buildPageFontUnicodeMaps(data, sp, ValidationModeRelaxed, cmapNameIdx)
		fontBases := buildPageFontBaseNameMap(data, sp, ValidationModeRelaxed)
		fontWidths := buildPageFontWidthsMap(data, sp, ValidationModeRelaxed)
		for _, objNum := range refs {
			blk, err := ExtractObjectBlockByNumberBytes(data, objNum, ValidationModeRelaxed)
			if err != nil {
				continue
			}
			objBlock := []byte(blk)
			dict := objectDictBytesBeforeStream(objBlock)
			raw, ok := ParseObjectStreamBytes(objBlock)
			if !ok {
				continue
			}
			tDec := time.Now()
			decoded := DecodeStreamFiltersBestEffort(dict, raw)
			decodeMs := time.Since(tDec).Milliseconds()
			pageProfile.RawStreamBytes += len(raw)
			pageProfile.DecodedBytes += len(decoded)
			pageProfile.DecodeMs += decodeMs
			profile.TotalDecodeMs += decodeMs
			fullDocDebugf("extractTextUnified page=%d obj=%d raw_stream=%d decoded=%d decode_ms=%d",
				pageIdx+1, objNum, len(raw), len(decoded), decodeMs)

			var runs []geomTextRun
			tGeom := time.Now()
			if len(decoded) <= maxGeomDecodeBytes {
				runs = extractGeomTextRuns(decoded, fontMaps, fontBases, fontWidths)
				pageProfile.GeomScanned = true
			} else {
				pageProfile.GeomSkipped = true
				fullDocDebugf("extractTextUnified skip_geom page=%d obj=%d decoded=%d (limit=%d)",
					pageIdx+1, objNum, len(decoded), maxGeomDecodeBytes)
			}
			geomMs := time.Since(tGeom).Milliseconds()
			pageProfile.GeomMs += geomMs
			profile.TotalGeomMs += geomMs
			fullDocDebugf("extractTextUnified geom page=%d obj=%d runs=%d geom_ms=%d",
				pageIdx+1, objNum, len(runs), geomMs)

			if wantSeg {
				var txt string
				var gx, gy float64
				if len(runs) > 0 {
					txt = assembleGeomTextRuns(runs, fontWidths)
					txt = normalizeExtractedText(txt)
					gx, gy = geomAnchorFromRuns(runs)
				}
				if strings.TrimSpace(txt) == "" {
					texSrc := decoded
					if len(texSrc) > maxTextExtractFromStreamBytes {
						fullDocDebugf("extractTextUnified truncate_text_extract page=%d obj=%d %d -> %d",
							pageIdx+1, objNum, len(texSrc), maxTextExtractFromStreamBytes)
						texSrc = texSrc[:maxTextExtractFromStreamBytes]
					}
					tTxt := time.Now()
					txt = extractTextFromContentStream(texSrc, fontMaps)
					textExtractMs := time.Since(tTxt).Milliseconds()
					pageProfile.FallbackTextExtractMs += textExtractMs
					profile.TotalFallbackTextExtractMs += textExtractMs
					fullDocDebugf("extractTextUnified extractTextFromContentStream page=%d obj=%d bytes=%d text_extract_ms=%d",
						pageIdx+1, objNum, len(texSrc), textExtractMs)
					txt = normalizeExtractedText(txt)
					gx, gy = 0, 0
				}
				if strings.TrimSpace(txt) != "" {
					segments = append(segments, TextSegment{
						StreamIndex: objNum,
						Order:       segOrder,
						SourceTrace: fmt.Sprintf("page#%d obj#%d", pageIdx+1, objNum),
						Text:        txt,
						ChunkType:   classifyTextChunkType(txt),
						GeomX:       gx,
						GeomY:       gy,
						BBox:        geomBBoxFromRuns(runs),
					})
					segOrder++
				}
			}

			if wantGeomLines && len(runs) > 0 {
				lines := splitGeomRunsToLines(runs)
				for _, ln := range lines {
					text := strings.TrimSpace(mergeGeomLineRunsByXGap(ln, fontWidths))
					if text == "" {
						continue
					}
					ax, ay := geomAnchorFromRuns(ln)
					geomLines = append(geomLines, GeometryLine{
						PageIndex:   pageIdx + 1,
						SourceTrace: fmt.Sprintf("page#%d obj#%d", pageIdx+1, objNum),
						Order:       lineOrder,
						Text:        normalizeExtractedText(text),
						GeomX:       ax,
						GeomY:       ay,
						BBox:        geomBBoxFromRuns(ln),
					})
					lineOrder++
				}
			}

			if wantGeomTokens && len(runs) > 0 {
				for _, r := range runs {
					text := strings.TrimSpace(r.s)
					if text == "" {
						continue
					}
					geomTokens = append(geomTokens, GeometryToken{
						PageIndex:   pageIdx + 1,
						SourceTrace: fmt.Sprintf("page#%d obj#%d", pageIdx+1, objNum),
						Order:       tokenOrder,
						Text:        text,
						GeomX:       r.x,
						GeomY:       r.y,
						FontKey:     r.fontKey,
						BaseFont:    r.baseFont,
						FontSizePt:  r.fontSizePt,
						BBox:        r.bbox,
					})
					tokenOrder++
				}
			}
		}
		if pageProfile.GeomScanned {
			profile.GeomScannedPages++
		}
		if pageProfile.GeomSkipped {
			profile.GeomSkippedPages++
		}
		if pageProfile.FallbackTextExtractMs > 0 {
			profile.FallbackTextExtractPages++
		}
		pageProfile.TotalMs = time.Since(pageStartedAt).Milliseconds()
		profile.SlowPages = append(profile.SlowPages, pageProfile)
	}
	profile.TotalMs = time.Since(startedAt).Milliseconds()
	sort.Slice(profile.SlowPages, func(i, j int) bool {
		if profile.SlowPages[i].TotalMs == profile.SlowPages[j].TotalMs {
			return profile.SlowPages[i].PageIndex < profile.SlowPages[j].PageIndex
		}
		return profile.SlowPages[i].TotalMs > profile.SlowPages[j].TotalMs
	})
	if len(profile.SlowPages) > 5 {
		profile.SlowPages = append([]TextExtractPageTiming(nil), profile.SlowPages[:5]...)
	}
	return segments, geomLines, geomTokens, hadPageContentStreams, profile
}

// ExtractAllTextFromPageSpecs emits basic segments, geometry lines, and tokens
// in a single pass for aggregate paths such as full_document.
func ExtractAllTextFromPageSpecs(data []byte, specs []PageRenderSpec) ([]TextSegment, []GeometryLine, []GeometryToken) {
	t0 := time.Now()
	fullDocDebugf("ExtractAllTextFromPageSpecs begin pages=%d", len(specs))
	if len(specs) == 0 {
		segs := extractTextSegmentsByScanningStreams(data)
		fullDocDebugf("ExtractAllTextFromPageSpecs done (scan_fallback) segments=%d elapsed_ms=%d", len(segs), time.Since(t0).Milliseconds())
		return segs, nil, nil
	}
	segs, lines, tok, hadContent := extractTextUnifiedFromPageSpecs(data, specs, true, true, true)
	if len(segs) == 0 && !hadContent {
		fullDocDebugf("ExtractAllTextFromPageSpecs fallback extractTextSegmentsByScanningStreams (no_page_contents_refs)")
		segs = extractTextSegmentsByScanningStreams(data)
	} else if len(segs) == 0 && hadContent {
		fullDocDebugf("ExtractAllTextFromPageSpecs skip_global_stream_scan empty_text_image_like_pdf=%v", hadContent)
	}
	fullDocDebugf("ExtractAllTextFromPageSpecs done segments=%d geom_lines=%d geom_tokens=%d elapsed_ms=%d",
		len(segs), len(lines), len(tok), time.Since(t0).Milliseconds())
	return segs, lines, tok
}

func ExtractAllTextFromPageSpecsProfiled(data []byte, specs []PageRenderSpec) ([]TextSegment, []GeometryLine, []GeometryToken, TextExtractTimingBreakdown) {
	t0 := time.Now()
	fullDocDebugf("ExtractAllTextFromPageSpecsProfiled begin pages=%d", len(specs))
	if len(specs) == 0 {
		segs := extractTextSegmentsByScanningStreams(data)
		profile := TextExtractTimingBreakdown{
			PageCount: len(specs),
			TotalMs:   time.Since(t0).Milliseconds(),
		}
		fullDocDebugf("ExtractAllTextFromPageSpecsProfiled done (scan_fallback) segments=%d elapsed_ms=%d", len(segs), profile.TotalMs)
		return segs, nil, nil, profile
	}
	segs, lines, tok, hadContent, profile := extractTextUnifiedFromPageSpecsProfiled(data, specs, true, true, true)
	if len(segs) == 0 && !hadContent {
		fullDocDebugf("ExtractAllTextFromPageSpecsProfiled fallback extractTextSegmentsByScanningStreams (no_page_contents_refs)")
		segs = extractTextSegmentsByScanningStreams(data)
	} else if len(segs) == 0 && hadContent {
		fullDocDebugf("ExtractAllTextFromPageSpecsProfiled skip_global_stream_scan empty_text_image_like_pdf=%v", hadContent)
	}
	profile.TotalMs = time.Since(t0).Milliseconds()
	fullDocDebugf("ExtractAllTextFromPageSpecsProfiled done segments=%d geom_lines=%d geom_tokens=%d elapsed_ms=%d",
		len(segs), len(lines), len(tok), profile.TotalMs)
	return segs, lines, tok, profile
}

// ExtractTextGeometryLinesFromBytesWithSpecs reuses parsed page render specs.
func ExtractTextGeometryLinesFromBytesWithSpecs(data []byte, specs []PageRenderSpec) []GeometryLine {
	if len(specs) == 0 {
		return nil
	}
	_, lines, _, _ := extractTextUnifiedFromPageSpecs(data, specs, false, true, false)
	return lines
}

func ExtractTextGeometryLinesFromBytes(data []byte) []GeometryLine {
	specs, err := DetectPageRenderSpecsBytes(data, ValidationModeRelaxed)
	if err != nil || len(specs) == 0 {
		return nil
	}
	return ExtractTextGeometryLinesFromBytesWithSpecs(data, specs)
}

// ExtractTextGeometryTokensFromBytesWithSpecs reuses parsed page render specs.
func ExtractTextGeometryTokensFromBytesWithSpecs(data []byte, specs []PageRenderSpec) []GeometryToken {
	if len(specs) == 0 {
		return nil
	}
	_, _, tok, _ := extractTextUnifiedFromPageSpecs(data, specs, false, false, true)
	return tok
}

func ExtractTextGeometryTokensFromBytes(data []byte) []GeometryToken {
	specs, err := DetectPageRenderSpecsBytes(data, ValidationModeRelaxed)
	if err != nil || len(specs) == 0 {
		return nil
	}
	return ExtractTextGeometryTokensFromBytesWithSpecs(data, specs)
}

// ExtractTextBasicSegmentsFromBytesWithSpecs reuses parsed page render specs.
func ExtractTextBasicSegmentsFromBytesWithSpecs(data []byte, specs []PageRenderSpec) []TextSegment {
	if len(specs) == 0 {
		return extractTextSegmentsByScanningStreams(data)
	}
	segs, _, _, hadContent := extractTextUnifiedFromPageSpecs(data, specs, true, false, false)
	if len(segs) == 0 && !hadContent {
		return extractTextSegmentsByScanningStreams(data)
	}
	return segs
}

func ExtractTextBasicSegmentsFromBytes(data []byte) []TextSegment {
	specs, err := DetectPageRenderSpecsBytes(data, ValidationModeRelaxed)
	if err != nil || len(specs) == 0 {
		return extractTextSegmentsByScanningStreams(data)
	}
	return ExtractTextBasicSegmentsFromBytesWithSpecs(data, specs)
}

func splitGeomRunsToLines(runs []geomTextRun) [][]geomTextRun {
	if len(runs) == 0 {
		return nil
	}
	sorted := append([]geomTextRun(nil), runs...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].y != sorted[j].y {
			return sorted[i].y > sorted[j].y
		}
		if sorted[i].x != sorted[j].x {
			return sorted[i].x < sorted[j].x
		}
		return sorted[i].b < sorted[j].b
	})
	yBand := geomYClusterBand(sorted)
	lines := make([][]geomTextRun, 0, 32)
	for _, r := range sorted {
		if len(lines) == 0 {
			lines = append(lines, []geomTextRun{r})
			continue
		}
		last := lines[len(lines)-1]
		refY := last[0].y
		if math.Abs(r.y-refY) <= yBand {
			lines[len(lines)-1] = append(last, r)
		} else {
			lines = append(lines, []geomTextRun{r})
		}
	}
	for i := range lines {
		lines[i] = orderGeomLineRunsForReadingOrder(lines[i])
	}
	return lines
}

// geomScriptClass splits visual lines into script runs because pure x-order can
// conflict with PDF drawing order in mixed CJK/Latin text.
const (
	geomScriptCJK = 0
	geomScriptLat = 1
	geomScriptNeu = 2
)

func geomRunScriptClass(s string) int {
	han, lat := 0, 0
	for _, r := range strings.TrimSpace(s) {
		switch {
		case unicode.Is(unicode.Han, r) || (r >= 0x3040 && r <= 0x30FF) || (r >= 0xAC00 && r <= 0xD7AF):
			han++
		case (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || unicode.IsDigit(r):
			lat++
		}
	}
	if han > lat {
		return geomScriptCJK
	}
	if lat > han {
		return geomScriptLat
	}
	return geomScriptNeu
}

func minGeomRunX(runs []geomTextRun) float64 {
	if len(runs) == 0 {
		return 0
	}
	m := runs[0].x
	for _, r := range runs[1:] {
		if r.x < m {
			m = r.x
		}
	}
	return m
}

func geomBlockHasLatinPrimaryRun(blk []geomTextRun) bool {
	for _, r := range blk {
		if geomRunScriptClass(r.s) == geomScriptLat {
			return true
		}
	}
	return false
}

// orderGeomLineRunsForReadingOrder first splits runs into CJK/Latin script
// blocks by content-stream order, then orders blocks by leftmost x.
// Latin blocks use (x,b); CJK/punctuation blocks use (b,x) to avoid punctuation
// being inserted inside words when exporters draw glyphs in unusual order.
func orderGeomLineRunsForReadingOrder(ln []geomTextRun) []geomTextRun {
	if len(ln) <= 1 {
		return ln
	}
	byB := append([]geomTextRun(nil), ln...)
	sort.Slice(byB, func(i, j int) bool {
		if byB[i].b != byB[j].b {
			return byB[i].b < byB[j].b
		}
		return byB[i].x < byB[j].x
	})
	var blocks [][]geomTextRun
	for _, r := range byB {
		c := geomRunScriptClass(r.s)
		if c == geomScriptNeu && len(blocks) > 0 {
			blocks[len(blocks)-1] = append(blocks[len(blocks)-1], r)
			continue
		}
		if len(blocks) == 0 {
			blocks = append(blocks, []geomTextRun{r})
			continue
		}
		last := blocks[len(blocks)-1]
		lastRun := last[len(last)-1]
		if geomRunScriptClass(lastRun.s) == c {
			blocks[len(blocks)-1] = append(last, r)
		} else {
			blocks = append(blocks, []geomTextRun{r})
		}
	}
	sort.SliceStable(blocks, func(i, j int) bool {
		xi, xj := minGeomRunX(blocks[i]), minGeomRunX(blocks[j])
		if xi != xj {
			return xi < xj
		}
		if len(blocks[i]) == 0 || len(blocks[j]) == 0 {
			return false
		}
		return blocks[i][0].b < blocks[j][0].b
	})
	out := make([]geomTextRun, 0, len(ln))
	for _, blk := range blocks {
		if geomBlockHasLatinPrimaryRun(blk) {
			sort.SliceStable(blk, func(a, b int) bool {
				if blk[a].x != blk[b].x {
					return blk[a].x < blk[b].x
				}
				return blk[a].b < blk[b].b
			})
		} else {
			sort.SliceStable(blk, func(a, b int) bool {
				if blk[a].b != blk[b].b {
					return blk[a].b < blk[b].b
				}
				return blk[a].x < blk[b].x
			})
		}
		out = append(out, blk...)
	}
	return out
}

func extractTextSegmentsByScanningStreams(data []byte) []TextSegment {
	idxs := reStreamBlock.FindAllSubmatchIndex(data, -1)
	if len(idxs) == 0 {
		return nil
	}
	segments := make([]TextSegment, 0, len(idxs))
	for i, m := range idxs {
		if len(m) < 4 {
			continue
		}
		streamBlock := data[m[2]:m[3]]
		dict := findNearestObjectDict(data, m[0])
		decoded := decodeStreamData(dict, streamBlock)
		var text string
		var gx, gy float64
		runs := extractGeomTextRuns(decoded, nil, nil, nil)
		if len(runs) > 0 {
			text = assembleGeomTextRuns(runs, nil)
			text = normalizeExtractedText(text)
			gx, gy = geomAnchorFromRuns(runs)
		}
		if strings.TrimSpace(text) == "" {
			text = extractTextFromStreamBlock(decoded)
			text = normalizeExtractedText(text)
			gx, gy = 0, 0
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		segments = append(segments, TextSegment{
			StreamIndex: i,
			Order:       len(segments),
			SourceTrace: "stream#" + strconv.Itoa(i),
			Text:        text,
			ChunkType:   classifyTextChunkType(text),
			GeomX:       gx,
			GeomY:       gy,
		})
	}
	return segments
}

func extractTextFromContentStream(block []byte, fontMaps map[string]cmapUnicodeMap) string {
	var out strings.Builder
	curFont := ""
	cursor := 0
	for cursor < len(block) {
		relTf := reTfOperator.FindSubmatchIndex(block[cursor:])
		relTjL := reTjLiteral.FindSubmatchIndex(block[cursor:])
		relTjH := reTjHex.FindSubmatchIndex(block[cursor:])
		relTJ := reTJArray.FindSubmatchIndex(block[cursor:])
		nextKind, nextIdx := nearestOperator(relTf, relTjL, relTjH, relTJ)
		if nextIdx < 0 {
			break
		}
		absStart := cursor + nextIdx
		switch nextKind {
		case "tf":
			m := reTfOperator.FindSubmatch(block[absStart:])
			if len(m) >= 2 {
				curFont = string(m[1])
			}
			loc := reTfOperator.FindSubmatchIndex(block[absStart:])
			if len(loc) >= 2 {
				cursor = absStart + loc[1]
				continue
			}
		case "tjl":
			loc := reTjLiteral.FindSubmatchIndex(block[absStart:])
			if len(loc) >= 2 {
				op := block[absStart : absStart+loc[1]]
				for _, lit := range reLiteral.FindAll(op, -1) {
					out.WriteString(decodePDFLiteralTokenWithCMap(lit, fontMaps[curFont]))
				}
				cursor = absStart + loc[1]
				continue
			}
		case "tjh":
			loc := reTjHex.FindSubmatchIndex(block[absStart:])
			if len(loc) >= 2 {
				op := block[absStart : absStart+loc[1]]
				for _, tok := range reHexToken.FindAll(op, -1) {
					out.WriteString(decodePDFHexTokenWithCMap(tok, fontMaps[curFont]))
				}
				cursor = absStart + loc[1]
				continue
			}
		case "tjarr":
			loc := reTJArray.FindSubmatchIndex(block[absStart:])
			if len(loc) >= 2 {
				op := block[absStart : absStart+loc[1]]
				for _, lit := range reLiteral.FindAll(op, -1) {
					out.WriteString(decodePDFLiteralTokenWithCMap(lit, fontMaps[curFont]))
				}
				for _, tok := range reHexToken.FindAll(op, -1) {
					out.WriteString(decodePDFHexTokenWithCMap(tok, fontMaps[curFont]))
				}
				cursor = absStart + loc[1]
				continue
			}
		}
		cursor = absStart + 1
	}
	return strings.TrimSpace(out.String())
}

func findNextContentPDFOp(block []byte, from int) (kind string, absStart, absEnd int) {
	loc := reAnyContentPDFOp.FindSubmatchIndex(block[from:])
	if loc == nil || len(loc) < 4 {
		return "", -1, -1
	}
	absStart = from + loc[0]
	absEnd = from + loc[1]
	kinds := [...]string{
		"bt",
		"et",
		"q",
		"Q",
		"cm",
		"tm",
		"tD",
		"td",
		"tstar",
		"ts",
		"tl",
		"tc",
		"tw",
		"tz",
		"tf",
		"tjl",
		"tjh",
		"tjarr",
	}
	for idx, opKind := range kinds {
		groupAt := 2 + idx*2
		if groupAt+1 >= len(loc) {
			break
		}
		if loc[groupAt] >= 0 {
			return opKind, absStart, absEnd
		}
	}
	return "", -1, -1
}

func extractGeomTextRuns(block []byte, fontMaps map[string]cmapUnicodeMap, fontBases map[string]string, fontWidths map[string]*pdfFontWidthModel) []geomTextRun {
	var runs []geomTextRun
	ctm := affIdentity()
	tlm := affIdentity()
	tm := affIdentity()
	tlLeading := 0.0
	textRise := 0.0
	fontSize := 0.0
	charSpace := 0.0
	wordSpace := 0.0
	horizScale := 100.0
	curFont := ""
	var gstack []pdfTextStateSnap
	cursor := 0
	for cursor < len(block) {
		kind, st, en := findNextContentPDFOp(block, cursor)
		if kind == "" {
			break
		}
		seg := block[st:en]
		switch kind {
		case "bt":
			// BT resets text state but does not change CTM per the PDF spec.
			tlm = affIdentity()
			tm = affIdentity()
			tlLeading = 0
			textRise = 0
			fontSize = 0
			charSpace = 0
			wordSpace = 0
			horizScale = 100
			curFont = ""
			cursor = en
		case "et":
			cursor = en
		case "q":
			gstack = append(gstack, pdfTextStateSnap{
				ctm: ctm, tlm: tlm, tm: tm, tl: tlLeading, textRise: textRise, fontSize: fontSize,
				charSpace: charSpace, wordSpace: wordSpace, horizScale: horizScale, curFont: curFont,
			})
			cursor = en
		case "Q":
			if len(gstack) > 0 {
				sn := gstack[len(gstack)-1]
				gstack = gstack[:len(gstack)-1]
				ctm = sn.ctm
				tlm = sn.tlm
				tm = sn.tm
				tlLeading = sn.tl
				textRise = sn.textRise
				fontSize = sn.fontSize
				charSpace = sn.charSpace
				wordSpace = sn.wordSpace
				horizScale = sn.horizScale
				curFont = sn.curFont
			}
			cursor = en
		case "cm":
			if m := reCmPDF.FindSubmatch(seg); len(m) >= 7 {
				a, okA := parsePDFContentFloat(string(m[1]))
				b, okB := parsePDFContentFloat(string(m[2]))
				c, okC := parsePDFContentFloat(string(m[3]))
				d, okD := parsePDFContentFloat(string(m[4]))
				e, okE := parsePDFContentFloat(string(m[5]))
				f, okF := parsePDFContentFloat(string(m[6]))
				if okA && okB && okC && okD && okE && okF {
					M := affFromPDFMatrix(a, b, c, d, e, f)
					ctm = mulAff(ctm, M)
				}
			}
			cursor = en
		case "tm":
			if m := reTmPDF.FindSubmatch(seg); len(m) >= 7 {
				a, okA := parsePDFContentFloat(string(m[1]))
				b, okB := parsePDFContentFloat(string(m[2]))
				c, okC := parsePDFContentFloat(string(m[3]))
				d, okD := parsePDFContentFloat(string(m[4]))
				e, okE := parsePDFContentFloat(string(m[5]))
				f, okF := parsePDFContentFloat(string(m[6]))
				if okA && okB && okC && okD && okE && okF {
					tm = affFromPDFMatrix(a, b, c, d, e, f)
					tlm = tm
				}
			}
			cursor = en
		case "td":
			if m := reTdPDF.FindSubmatch(seg); len(m) >= 3 {
				tx, ok1 := parsePDFContentFloat(string(m[1]))
				ty, ok2 := parsePDFContentFloat(string(m[2]))
				if ok1 && ok2 {
					tlm = mulAff(tlm, translateAff(tx, ty))
					tm = tlm
				}
			}
			cursor = en
		case "tD":
			if m := reTDPDF.FindSubmatch(seg); len(m) >= 3 {
				tx, ok1 := parsePDFContentFloat(string(m[1]))
				ty, ok2 := parsePDFContentFloat(string(m[2]))
				if ok1 && ok2 {
					// In PDF, TD is the same as Td and sets leading to -ty.
					tlLeading = -ty
					tlm = mulAff(tlm, translateAff(tx, ty))
					tm = tlm
				}
			}
			cursor = en
		case "tstar":
			if tlLeading != 0 {
				tlm = mulAff(tlm, translateAff(0, -tlLeading))
				tm = tlm
			}
			cursor = en
		case "ts":
			if m := reTsPDF.FindSubmatch(seg); len(m) >= 2 {
				if v, ok := parsePDFContentFloat(string(m[1])); ok {
					textRise = v
				}
			}
			cursor = en
		case "tl":
			if m := reTLPDF.FindSubmatch(seg); len(m) >= 2 {
				if v, ok := parsePDFContentFloat(string(m[1])); ok {
					tlLeading = v
				}
			}
			cursor = en
		case "tc":
			if m := reTcPDF.FindSubmatch(seg); len(m) >= 2 {
				if v, ok := parsePDFContentFloat(string(m[1])); ok {
					charSpace = v
				}
			}
			cursor = en
		case "tw":
			if m := reTwPDF.FindSubmatch(seg); len(m) >= 2 {
				if v, ok := parsePDFContentFloat(string(m[1])); ok {
					wordSpace = v
				}
			}
			cursor = en
		case "tz":
			if m := reTzPDF.FindSubmatch(seg); len(m) >= 2 {
				if v, ok := parsePDFContentFloat(string(m[1])); ok {
					horizScale = v
				}
			}
			cursor = en
		case "tf":
			if m := reTfOperator.FindSubmatch(seg); len(m) >= 2 {
				curFont = string(m[1])
				if len(m) >= 3 {
					if fs, ok := parsePDFContentFloat(string(m[2])); ok {
						fontSize = fs
					}
				}
			}
			cursor = en
		case "tjl":
			var sb strings.Builder
			cm := fontMapForOp(fontMaps, curFont)
			for _, lit := range reLiteral.FindAll(seg, -1) {
				sb.WriteString(decodePDFLiteralTokenWithCMap(lit, cm))
			}
			raw := sb.String()
			if strings.TrimSpace(raw) == "" {
				cursor = en
				continue
			}
			s := strings.TrimSpace(raw)
			px, py := geomTextAnchorUserSpace(ctm, tm, textRise)
			bf := ""
			if fontBases != nil {
				bf = fontBases[curFont]
			}
			var wm *pdfFontWidthModel
			if fontWidths != nil {
				wm = fontWidths[curFont]
			}
			rawBytes := concatLiteralRawBytes(seg)
			adv := geomHorizontalAdvanceForTextShow(raw, rawBytes, cm, wm, fontSize, charSpace, wordSpace, horizScale)
			runs = append(runs, geomTextRun{
				s:          s,
				x:          px,
				y:          py,
				bbox:       geomTextRunBBoxUserSpace(ctm, tm, textRise, adv, fontSize),
				b:          st,
				fontKey:    curFont,
				fontSizePt: fontSize,
				baseFont:   bf,
			})
			advanceTmAfterHorizontalShow(&tlm, &tm, adv)
			cursor = en
		case "tjh":
			var sb strings.Builder
			cm := fontMapForOp(fontMaps, curFont)
			for _, tok := range reHexToken.FindAll(seg, -1) {
				sb.WriteString(decodePDFHexTokenWithCMap(tok, cm))
			}
			raw := sb.String()
			if strings.TrimSpace(raw) == "" {
				cursor = en
				continue
			}
			s := strings.TrimSpace(raw)
			px, py := geomTextAnchorUserSpace(ctm, tm, textRise)
			bf := ""
			if fontBases != nil {
				bf = fontBases[curFont]
			}
			var wm *pdfFontWidthModel
			if fontWidths != nil {
				wm = fontWidths[curFont]
			}
			rawBytes := concatHexRawBytes(seg)
			adv := geomHorizontalAdvanceForTextShow(raw, rawBytes, cm, wm, fontSize, charSpace, wordSpace, horizScale)
			runs = append(runs, geomTextRun{
				s:          s,
				x:          px,
				y:          py,
				bbox:       geomTextRunBBoxUserSpace(ctm, tm, textRise, adv, fontSize),
				b:          st,
				fontKey:    curFont,
				fontSizePt: fontSize,
				baseFont:   bf,
			})
			advanceTmAfterHorizontalShow(&tlm, &tm, adv)
			cursor = en
		case "tjarr":
			cm := fontMapForOp(fontMaps, curFont)
			inner := extractTJBracketInner(seg)
			fs := fontSize
			if fs <= 0 {
				fs = geomDefaultFontSize
			}
			var wm *pdfFontWidthModel
			if fontWidths != nil {
				wm = fontWidths[curFont]
			}
			for _, el := range scanTJArrayElements(inner, cm) {
				if el.isNum {
					adj := tjNumericAdjTextSpace(el.num, fs)
					advanceTmAfterHorizontalShow(&tlm, &tm, adj)
					continue
				}
				raw := el.s
				if strings.TrimSpace(raw) == "" {
					continue
				}
				s := strings.TrimSpace(raw)
				px, py := geomTextAnchorUserSpace(ctm, tm, textRise)
				bf := ""
				if fontBases != nil {
					bf = fontBases[curFont]
				}
				adv := geomHorizontalAdvanceForTextShow(raw, el.raw, cm, wm, fs, charSpace, wordSpace, horizScale)
				runs = append(runs, geomTextRun{
					s:          s,
					x:          px,
					y:          py,
					bbox:       geomTextRunBBoxUserSpace(ctm, tm, textRise, adv, fs),
					b:          st,
					fontKey:    curFont,
					fontSizePt: fs,
					baseFont:   bf,
				})
				advanceTmAfterHorizontalShow(&tlm, &tm, adv)
			}
			cursor = en
		default:
			cursor = en
		}
	}
	return runs
}

const geomLineYBand = 8.0

// geomYClusterBand estimates the Y tolerance for grouping visual lines from font size.
// A fixed 8pt band can merge multiple lines in some coordinate systems; adaptive
// sizing preserves line breaks more reliably.
func geomYClusterBand(runs []geomTextRun) float64 {
	if len(runs) == 0 {
		return geomLineYBand
	}
	fs := 0.0
	for _, r := range runs {
		if r.fontSizePt > fs {
			fs = r.fontSizePt
		}
	}
	if fs <= 0 {
		fs = geomDefaultFontSize
	}
	b := fs * 0.38
	if b < 2.2 {
		b = 2.2
	}
	if b > 9.0 {
		b = 9.0
	}
	return b
}

// Adjacent runs on the same line both use start x. The gap is roughly previous
// visual width plus TJ adjustment. Insert a space only when the adjustment is
// clearly larger than glyph-width noise.
const geomInterRunMinExtraOverEst = 2.0
const geomInterRunWidthFactor = 1.22

func shouldInsertSpaceBetweenGeomFragments(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if strings.HasSuffix(a, "-") || strings.HasSuffix(a, "/") {
		return false
	}
	ra, rb := []rune(a), []rune(b)
	last, first := ra[len(ra)-1], rb[0]
	if last <= 0x20 || first <= 0x20 {
		return false
	}
	if unicode.Is(unicode.Han, last) && unicode.Is(unicode.Han, first) {
		return false
	}
	if first == '.' && unicode.IsDigit(last) {
		return false
	}
	if last == '(' || first == ')' {
		return false
	}
	isWordChar := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsNumber(r)
	}
	return isWordChar(last) && isWordChar(first)
}

func geomFontSizeForRun(r geomTextRun) float64 {
	if r.fontSizePt > 0 {
		return r.fontSizePt
	}
	return geomDefaultFontSize
}

func mergeGeomLineRunsByXGap(ln []geomTextRun, fontWidths map[string]*pdfFontWidthModel) string {
	if len(ln) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(ln[0].s)
	for i := 1; i < len(ln); i++ {
		prev, cur := ln[i-1], ln[i]
		if script, ok := tryAttachFormulaScript(prev, cur); ok {
			sb.WriteString(script)
			continue
		}
		gap := cur.x - prev.x
		pfs := geomFontSizeForRun(prev)
		est := estimateTextWidthTextSpace(prev.s, pfs)
		if fontWidths != nil {
			if wm := fontWidths[prev.fontKey]; wm != nil {
				if w := geomPrevRunWidthTextSpace(prev, wm); w > 0 {
					est = w
				}
			}
		}
		if est < pfs*0.2 {
			est = pfs * 0.35
		}
		threshold := est*geomInterRunWidthFactor + geomInterRunMinExtraOverEst
		if gap > threshold && shouldInsertSpaceBetweenGeomFragments(prev.s, cur.s) {
			sb.WriteByte(' ')
		}
		sb.WriteString(cur.s)
	}
	return sb.String()
}

func tryAttachFormulaScript(prev, cur geomTextRun) (string, bool) {
	if strings.TrimSpace(prev.s) == "" || strings.TrimSpace(cur.s) == "" {
		return "", false
	}
	if !looksFormulaLikeFragment(prev) && !looksFormulaLikeFragment(cur) &&
		!isDocMathFontBaseName(prev.baseFont) && !isDocMathFontBaseName(cur.baseFont) {
		return "", false
	}
	baseSize := prev.fontSizePt
	if baseSize <= 0 {
		baseSize = geomDefaultFontSize
	}
	scriptSize := cur.fontSizePt
	if scriptSize <= 0 {
		scriptSize = baseSize
	}
	// Footnotes and super/subscripts are usually much smaller; 0.88 supports
	// exporters with less pronounced size differences.
	if scriptSize > baseSize*0.88 {
		return "", false
	}
	// Horizontally, super/subscripts are near the base glyph start or just to
	// its right, so symmetric |dx| is too strict after wide base glyphs.
	wPrev := estimateTextWidthTextSpace(strings.TrimSpace(prev.s), baseSize)
	slack := math.Max(baseSize*0.55, 5.0)
	left := prev.x - math.Max(baseSize*0.35, 3.0)
	// Estimated glyph width is conservative, so expand the factor for common
	// base-glyph plus subscript spacing.
	right := prev.x + wPrev*1.7 + slack
	if cur.x < left || cur.x > right {
		return "", false
	}
	yTol := math.Max(baseSize*0.18, 1.5)
	dy := cur.y - prev.y
	s := strings.TrimSpace(cur.s)
	if dy > yTol {
		return "^{" + s + "}", true
	}
	if dy < -yTol {
		return "_{" + s + "}", true
	}
	return "", false
}

func looksFormulaLikeFragment(r geomTextRun) bool {
	t := strings.TrimSpace(r.s)
	if t == "" {
		return false
	}
	for _, ch := range t {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
			return true
		}
		switch ch {
		case '=', '+', '-', '*', '/', '^', '_', '(', ')', '[', ']', '{', '}', '×', '÷',
			'≤', '≥', '≈', '≠', '∑', '∏', '√', '∫', '∂', '∇', '∞', 'α', 'β', 'γ', 'δ', 'θ', 'λ', 'μ', 'π', 'σ', 'φ', 'ω':
			return true
		}
	}
	return false
}

func assembleGeomTextRuns(runs []geomTextRun, fontWidths map[string]*pdfFontWidthModel) string {
	if len(runs) == 0 {
		return ""
	}
	lines := splitGeomRunsToLines(runs)
	parts := make([]string, 0, len(lines))
	for _, ln := range lines {
		t := strings.TrimSpace(mergeGeomLineRunsByXGap(ln, fontWidths))
		if t != "" {
			parts = append(parts, t)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func geomAnchorFromRuns(runs []geomTextRun) (float64, float64) {
	if len(runs) == 0 {
		return 0, 0
	}
	maxY := runs[0].y
	for _, r := range runs[1:] {
		if r.y > maxY {
			maxY = r.y
		}
	}
	minX := 0.0
	found := false
	yBand := geomYClusterBand(runs)
	for _, r := range runs {
		if math.Abs(r.y-maxY) <= yBand {
			if !found || r.x < minX {
				minX = r.x
				found = true
			}
		}
	}
	if !found {
		return runs[0].x, runs[0].y
	}
	return minX, maxY
}

func geomBBoxFromRuns(runs []geomTextRun) *TextBBox {
	if len(runs) == 0 {
		return nil
	}
	var out *TextBBox
	for _, r := range runs {
		if r.bbox == nil {
			continue
		}
		if out == nil {
			cp := *r.bbox
			out = &cp
			continue
		}
		if r.bbox.Left < out.Left {
			out.Left = r.bbox.Left
		}
		if r.bbox.Right > out.Right {
			out.Right = r.bbox.Right
		}
		if r.bbox.Bottom < out.Bottom {
			out.Bottom = r.bbox.Bottom
		}
		if r.bbox.Top > out.Top {
			out.Top = r.bbox.Top
		}
	}
	if out == nil || out.Right <= out.Left || out.Top <= out.Bottom {
		return nil
	}
	return out
}

func geomLineBBoxFromRuns(runs []geomTextRun, fontWidths map[string]*pdfFontWidthModel, sp PageRenderSpec) *TextBBox {
	if len(runs) == 0 {
		return nil
	}
	pageLeft, pageBottom, pageRight, pageTop, ok := pageRectFromRenderSpec(sp)
	if !ok || pageRight <= pageLeft || pageTop <= pageBottom {
		return nil
	}

	left, right := math.Inf(1), math.Inf(-1)
	bottom, top := math.Inf(1), math.Inf(-1)
	for _, r := range runs {
		text := strings.TrimSpace(r.s)
		if text == "" {
			continue
		}
		fs := geomFontSizeForRun(r)
		width := estimateTextWidthTextSpace(text, fs)
		if fontWidths != nil {
			if wm := fontWidths[r.fontKey]; wm != nil {
				if w := geomPrevRunWidthTextSpace(r, wm); w > 0 {
					width = w
				}
			}
		}
		if width <= 0 {
			width = fs * 0.5
		}
		runLeft := r.x
		runRight := r.x + width
		if runRight < runLeft {
			runLeft, runRight = runRight, runLeft
		}
		runBottom := r.y - fs*0.28
		runTop := r.y + fs*0.92
		left = math.Min(left, runLeft)
		right = math.Max(right, runRight)
		bottom = math.Min(bottom, runBottom)
		top = math.Max(top, runTop)
	}
	if math.IsInf(left, 0) || math.IsInf(right, 0) || math.IsInf(bottom, 0) || math.IsInf(top, 0) {
		return nil
	}
	nLeft := pdfClamp01((left - pageLeft) / (pageRight - pageLeft))
	nRight := pdfClamp01((right - pageLeft) / (pageRight - pageLeft))
	nBottom := pdfClamp01((bottom - pageBottom) / (pageTop - pageBottom))
	nTop := pdfClamp01((top - pageBottom) / (pageTop - pageBottom))
	if nRight < nLeft {
		nLeft, nRight = nRight, nLeft
	}
	if nTop < nBottom {
		nTop, nBottom = nBottom, nTop
	}
	if nRight <= nLeft || nTop <= nBottom {
		return nil
	}
	return &TextBBox{
		Left:   roundPDFGeometry(nLeft),
		Right:  roundPDFGeometry(nRight),
		Top:    roundPDFGeometry(nTop),
		Bottom: roundPDFGeometry(nBottom),
	}
}

func pageRectFromRenderSpec(sp PageRenderSpec) (left, bottom, right, top float64, ok bool) {
	raw := strings.TrimSpace(sp.CropBox)
	if raw == "" {
		raw = strings.TrimSpace(sp.MediaBox)
	}
	fields := strings.Fields(raw)
	if len(fields) != 4 {
		return 0, 0, 0, 0, false
	}
	vals := make([]float64, 4)
	for i, f := range fields {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return 0, 0, 0, 0, false
		}
		vals[i] = v
	}
	return vals[0], vals[1], vals[2], vals[3], vals[2] > vals[0] && vals[3] > vals[1]
}

func pdfClamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func roundPDFGeometry(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}

func nearestOperator(tf, tjl, tjh, tjarr []int) (string, int) {
	type cand struct {
		kind string
		idx  int
	}
	cands := []cand{}
	if len(tf) >= 2 {
		cands = append(cands, cand{kind: "tf", idx: tf[0]})
	}
	if len(tjl) >= 2 {
		cands = append(cands, cand{kind: "tjl", idx: tjl[0]})
	}
	if len(tjh) >= 2 {
		cands = append(cands, cand{kind: "tjh", idx: tjh[0]})
	}
	if len(tjarr) >= 2 {
		cands = append(cands, cand{kind: "tjarr", idx: tjarr[0]})
	}
	if len(cands) == 0 {
		return "", -1
	}
	best := cands[0]
	for _, c := range cands[1:] {
		if c.idx < best.idx {
			best = c
		}
	}
	return best.kind, best.idx
}

type cmapUnicodeMap struct {
	byCodeHex map[string]string
	keyLens   []int
}

// buildPageFontUnicodeMaps parses font /ToUnicode maps from page Resources.
// cmapNameIdx may be nil; aggregate paths should pass a prebuilt index to
// avoid scanning the full file per page.
func buildPageFontUnicodeMaps(data []byte, sp PageRenderSpec, mode string, cmapNameIdx map[string]int) map[string]cmapUnicodeMap {
	out := map[string]cmapUnicodeMap{}
	if cmapNameIdx == nil {
		cmapNameIdx = buildEmbeddedCMapNameIndex(data, mode)
	}
	resBody := resolveResourcesBodyForPageText(data, sp, mode)
	if resBody == "" {
		return out
	}
	fontFrag := extractFontDictFragmentFromResources(data, resBody, mode)
	if fontFrag == "" {
		return out
	}
	for _, m := range reNamedRef.FindAllStringSubmatch(fontFrag, -1) {
		if len(m) < 3 {
			continue
		}
		fontName := m[1]
		fontObj, err := strconv.Atoi(m[2])
		if err != nil || fontObj <= 0 {
			continue
		}
		fontBlock, err := ExtractObjectBlockByNumberBytes(data, fontObj, mode)
		if err != nil {
			continue
		}
		fontBody := objectBodyFromBlockBytes([]byte(fontBlock))
		toUnicodeObj, hasToUnicode := parseIndirectRefObjectNumberByKey([]byte(fontBlock), "/ToUnicode")
		if hasToUnicode && toUnicodeObj > 0 {
			if cm, ok := buildToUnicodeMapFromObject(data, toUnicodeObj, mode, cmapNameIdx, map[int]bool{}); ok {
				out[fontName] = cm
				continue
			}
		}
		// Fall back to single-byte /Encoding(+Differences) when /ToUnicode is missing.
		if cm, ok := buildEncodingFallbackMapFromFontDict(fontBody); ok {
			out[fontName] = cm
		}
	}
	return out
}

func buildEncodingFallbackMapFromFontDict(fontBody string) (cmapUnicodeMap, bool) {
	body := strings.TrimSpace(fontBody)
	if body == "" {
		return cmapUnicodeMap{}, false
	}
	if strings.Contains(body, "/Subtype /Type0") || strings.Contains(body, "/Subtype/Type0") {
		// Type0/CID fonts without ToUnicode are unreliable with /Encoding alone.
		return cmapUnicodeMap{}, false
	}
	encName := extractEncodingNameFromFontBody(body)
	base := baseEncodingMap(encName)
	if len(base.byCodeHex) == 0 {
		return cmapUnicodeMap{}, false
	}
	if dict := extractInlineDictAfterKey(body, "/Encoding"); dict != "" {
		if arr := extractArrayAfterKey(dict, "/Differences"); arr != "" {
			applyDifferencesToMap(base.byCodeHex, arr)
		}
	}
	refreshCMapKeyLens(&base)
	return base, len(base.byCodeHex) > 0
}

func extractEncodingNameFromFontBody(fontBody string) string {
	// First handle /Encoding << ... /BaseEncoding /WinAnsiEncoding ... >>.
	if dict := extractInlineDictAfterKey(fontBody, "/Encoding"); dict != "" {
		if m := regexp.MustCompile(`/BaseEncoding\s*/([^\s<>\[\]/]+)`).FindStringSubmatch(dict); len(m) >= 2 {
			return strings.TrimSpace(m[1])
		}
	}
	// Then handle /Encoding /WinAnsiEncoding, excluding the inline dictionary case.
	if m := regexp.MustCompile(`/Encoding\s*/([^\s<>\[\]/]+)`).FindStringSubmatch(fontBody); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return "WinAnsiEncoding"
}

func baseEncodingMap(name string) cmapUnicodeMap {
	enc := strings.TrimSpace(strings.TrimPrefix(name, "/"))
	var cm *charmap.Charmap
	switch strings.ToLower(enc) {
	case "macromanencoding", "macroman":
		cm = charmap.Macintosh
	default:
		// Use Windows-1252 for WinAnsi/Standard/PDFDocEncoding fallback to favor readability.
		cm = charmap.Windows1252
	}
	out := cmapUnicodeMap{byCodeHex: map[string]string{}, keyLens: []int{1}}
	dec := cm.NewDecoder()
	for i := 0; i <= 255; i++ {
		b := []byte{byte(i)}
		s, err := dec.Bytes(b)
		if err != nil || len(s) == 0 {
			continue
		}
		out.byCodeHex[fmt.Sprintf("%02X", i)] = string(s)
	}
	return out
}

func extractArrayAfterKey(body string, key string) string {
	i := strings.Index(body, key)
	if i < 0 {
		return ""
	}
	s := body[i+len(key):]
	l := strings.Index(s, "[")
	if l < 0 {
		return ""
	}
	s = s[l+1:]
	depth := 1
	for idx := 0; idx < len(s); idx++ {
		switch s[idx] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return s[:idx]
			}
		}
	}
	return ""
}

func applyDifferencesToMap(dst map[string]string, arr string) {
	if dst == nil {
		return
	}
	// Simplified tokenizer robust enough for Differences: number + /GlyphName sequence.
	repl := strings.NewReplacer("[", " [ ", "]", " ] ", "\r", " ", "\n", " ", "\t", " ")
	toks := strings.Fields(repl.Replace(arr))
	curCode := -1
	for _, tok := range toks {
		if n, err := strconv.Atoi(tok); err == nil {
			curCode = n
			continue
		}
		if !strings.HasPrefix(tok, "/") || curCode < 0 || curCode > 255 {
			continue
		}
		name := strings.TrimPrefix(tok, "/")
		if u := glyphNameToUnicode(name); u != "" {
			dst[fmt.Sprintf("%02X", curCode)] = u
		}
		curCode++
	}
}

func glyphNameToUnicode(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return ""
	}
	if len(n) == 1 {
		return n
	}
	if strings.HasPrefix(n, "uni") && len(n) == 7 {
		if v, err := strconv.ParseInt(n[3:], 16, 32); err == nil {
			return string(rune(v))
		}
	}
	if strings.HasPrefix(n, "u") && (len(n) == 5 || len(n) == 6 || len(n) == 7) {
		if v, err := strconv.ParseInt(n[1:], 16, 32); err == nil {
			return string(rune(v))
		}
	}
	switch n {
	case "space":
		return " "
	case "comma":
		return ","
	case "period":
		return "."
	case "colon":
		return ":"
	case "semicolon":
		return ";"
	case "parenleft":
		return "("
	case "parenright":
		return ")"
	case "hyphen", "minus":
		return "-"
	case "plus":
		return "+"
	case "slash":
		return "/"
	case "backslash":
		return "\\"
	case "equal":
		return "="
	}
	return ""
}

func resolveResourcesBodyForPageText(data []byte, sp PageRenderSpec, mode string) string {
	// Prefer inline page /Resources << ... >>.
	pageBlock, err := ExtractObjectBlockByNumberBytes(data, sp.ObjectNumber, mode)
	if err == nil {
		pbody := objectBodyFromBlockBytes([]byte(pageBlock))
		if inlineRes := extractInlineDictAfterKey(pbody, "/Resources"); inlineRes != "" {
			return inlineRes
		}
	}
	// Fallback to traditional /Resources N 0 R.
	if sp.ResourcesRefObject <= 0 {
		return ""
	}
	resBlock, ok := findResourcesObjectBlockForImageScan(data, sp.ResourcesRefObject)
	if !ok {
		return ""
	}
	return objectBodyFromBlockBytes(resBlock)
}

func extractFontDictFragmentFromResources(data []byte, resBody, mode string) string {
	if frag := extractInlineDictAfterKey(resBody, "/Font"); frag != "" {
		return frag
	}
	// /Font N 0 R
	if n, ok := parseIndirectRefObjectNumberByKey([]byte(resBody), "/Font"); ok && n > 0 {
		blk, err := ExtractObjectBlockByNumberBytes(data, n, mode)
		if err == nil {
			return objectBodyFromBlockBytes([]byte(blk))
		}
	}
	return ""
}

func buildToUnicodeMapFromObject(data []byte, objNum int, mode string, cmapNameIdx map[string]int, visiting map[int]bool) (cmapUnicodeMap, bool) {
	if objNum <= 0 || visiting[objNum] {
		return cmapUnicodeMap{}, false
	}
	visiting[objNum] = true
	defer delete(visiting, objNum)
	blk, err := ExtractObjectBlockByNumberBytes(data, objNum, mode)
	if err != nil {
		return cmapUnicodeMap{}, false
	}
	objBlock := []byte(blk)
	dict := objectDictBytesBeforeStream(objBlock)
	raw, ok := ParseObjectStreamBytes(objBlock)
	if !ok {
		return cmapUnicodeMap{}, false
	}
	decoded := DecodeStreamFiltersBestEffort(dict, raw)
	mp, useName := parseToUnicodeCMap(decoded)
	if useName != "" {
		if parentObj, ok := cmapNameIdx[useName]; ok {
			if parent, ok := buildToUnicodeMapFromObject(data, parentObj, mode, cmapNameIdx, visiting); ok {
				for k, v := range parent.byCodeHex {
					if _, exists := mp.byCodeHex[k]; !exists {
						mp.byCodeHex[k] = v
					}
				}
				if len(mp.keyLens) == 0 {
					mp.keyLens = append(mp.keyLens, parent.keyLens...)
				}
			}
		}
	}
	refreshCMapKeyLens(&mp)
	return mp, len(mp.byCodeHex) > 0
}

func parseToUnicodeCMap(decoded []byte) (cmapUnicodeMap, string) {
	out := cmapUnicodeMap{byCodeHex: map[string]string{}}
	text := string(decoded)
	useName := ""
	if m := reUseCMap.FindStringSubmatch(text); len(m) >= 2 {
		useName = strings.TrimSpace(m[1])
	}
	for _, blk := range reBeginBFChar.FindAllStringSubmatch(text, -1) {
		if len(blk) < 2 {
			continue
		}
		for _, m := range reBFCharLine.FindAllStringSubmatch(blk[1], -1) {
			if len(m) < 3 {
				continue
			}
			src := normalizeHexString(m[1])
			dst := decodeUnicodeHexData(m[2])
			if src == "" || dst == "" {
				continue
			}
			out.byCodeHex[src] = dst
		}
	}
	for _, blk := range reBeginBFRange.FindAllStringSubmatch(text, -1) {
		if len(blk) < 2 {
			continue
		}
		body := blk[1]
		for _, m := range reBFRangePair.FindAllStringSubmatch(body, -1) {
			if len(m) < 4 {
				continue
			}
			addBFRangePair(out.byCodeHex, m[1], m[2], m[3])
		}
		for _, m := range reBFRangeList.FindAllStringSubmatch(body, -1) {
			if len(m) < 4 {
				continue
			}
			addBFRangeList(out.byCodeHex, m[1], m[2], m[3])
		}
	}
	seenLens := map[int]bool{}
	for k := range out.byCodeHex {
		n := len(k) / 2
		if n > 0 {
			seenLens[n] = true
		}
	}
	for n := range seenLens {
		out.keyLens = append(out.keyLens, n)
	}
	sort.Slice(out.keyLens, func(i, j int) bool { return out.keyLens[i] > out.keyLens[j] })
	return out, useName
}

// maxBFCharRangeExpand limits expansion per beginbfrange to avoid malicious or
// malformed CMaps exhausting memory or time.
const maxBFCharRangeExpand = 65536

func addBFRangePair(out map[string]string, srcStart, srcEnd, dstStart string) {
	s := normalizeHexString(srcStart)
	e := normalizeHexString(srcEnd)
	d := normalizeHexString(dstStart)
	if s == "" || e == "" || d == "" || len(s) != len(e) || len(d) < 4 {
		return
	}
	sn, err1 := strconv.ParseUint(s, 16, 64)
	en, err2 := strconv.ParseUint(e, 16, 64)
	dn, err3 := strconv.ParseUint(d, 16, 64)
	if err1 != nil || err2 != nil || err3 != nil || en < sn {
		return
	}
	span := en - sn
	if span > maxBFCharRangeExpand {
		return
	}
	srcWidth := len(s)
	for i := uint64(0); i <= span; i++ {
		src := fmt.Sprintf("%0*X", srcWidth, sn+i)
		dst := fmt.Sprintf("%04X", dn+i)
		out[src] = decodeUnicodeHexData(dst)
	}
}

func addBFRangeList(out map[string]string, srcStart, srcEnd, listBody string) {
	s := normalizeHexString(srcStart)
	e := normalizeHexString(srcEnd)
	if s == "" || e == "" || len(s) != len(e) {
		return
	}
	sn, err1 := strconv.ParseUint(s, 16, 64)
	en, err2 := strconv.ParseUint(e, 16, 64)
	if err1 != nil || err2 != nil || en < sn {
		return
	}
	if en-sn > maxBFCharRangeExpand {
		return
	}
	hexItems := reHexToken.FindAllString(listBody, -1)
	srcWidth := len(s)
	for i, it := range hexItems {
		srcNum := sn + uint64(i)
		if srcNum > en {
			break
		}
		src := fmt.Sprintf("%0*X", srcWidth, srcNum)
		dst := decodeUnicodeHexData(strings.Trim(it, "<>"))
		if dst != "" {
			out[src] = dst
		}
	}
}

func findNearestObjectDict(data []byte, streamTokenPos int) []byte {
	if streamTokenPos <= 0 {
		return nil
	}
	left := data[:streamTokenPos]
	dictEnd := bytes.LastIndex(left, []byte(">>"))
	if dictEnd < 0 {
		return nil
	}
	dictStart := bytes.LastIndex(left[:dictEnd], []byte("<<"))
	if dictStart < 0 {
		return nil
	}
	// Treat the preceding dictionary as current only when nearby, avoiding
	// accidental matches against older objects.
	if streamTokenPos-(dictStart) > 4096 {
		return nil
	}
	return left[dictStart : dictEnd+2]
}

func decodeStreamData(dict []byte, streamBlock []byte) []byte {
	return DecodeStreamFiltersBestEffort(dict, streamBlock)
}

func extractTextFromStreamBlock(block []byte) string {
	var out strings.Builder

	// Handle "(... ) Tj".
	tjMatches := reTjLiteral.FindAll(block, -1)
	for _, m := range tjMatches {
		lits := reLiteral.FindAll(m, -1)
		for _, lit := range lits {
			out.WriteString(decodePDFLiteralToken(lit))
		}
		out.WriteByte('\n')
	}

	// Handle "<...> Tj" hex-string text.
	tjHexMatches := reTjHex.FindAll(block, -1)
	for _, m := range tjHexMatches {
		hexToks := reHexToken.FindAll(m, -1)
		for _, tok := range hexToks {
			out.WriteString(decodePDFHexToken(tok))
		}
		out.WriteByte('\n')
	}

	// Handle "[ (... ) ... ] TJ".
	tjArrMatches := reTJArray.FindAll(block, -1)
	for _, m := range tjArrMatches {
		lits := reLiteral.FindAll(m, -1)
		for _, lit := range lits {
			out.WriteString(decodePDFLiteralToken(lit))
		}
		hexToks := reHexToken.FindAll(m, -1)
		for _, tok := range hexToks {
			out.WriteString(decodePDFHexToken(tok))
		}
		out.WriteByte('\n')
	}

	return strings.TrimSpace(out.String())
}

func decodePDFLiteralToken(tok []byte) string {
	b := decodePDFLiteralBytes(tok)
	if len(b) == 0 {
		return ""
	}
	return decodeTextBytesBestEffort(b)
}

func decodePDFLiteralTokenWithCMap(tok []byte, cm cmapUnicodeMap) string {
	b := decodePDFLiteralBytes(tok)
	if len(b) == 0 {
		return ""
	}
	if len(cm.byCodeHex) == 0 || len(cm.keyLens) == 0 {
		return normalizeAdobeSymbolPUAText(decodeTextBytesBestEffort(b))
	}
	var out strings.Builder
	for i := 0; i < len(b); {
		matched := false
		for _, n := range cm.keyLens {
			if n <= 0 || i+n > len(b) {
				continue
			}
			key := strings.ToUpper(hex.EncodeToString(b[i : i+n]))
			if u, ok := cm.byCodeHex[key]; ok {
				out.WriteString(u)
				i += n
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		out.WriteByte(b[i])
		i++
	}
	s := out.String()
	if strings.TrimSpace(s) == "" {
		return s
	}
	return normalizeAdobeSymbolPUAText(maybeRepairUTF8Mojibake(s))
}

func decodePDFLiteralBytes(tok []byte) []byte {
	tok = bytes.TrimSpace(tok)
	if len(tok) < 2 || tok[0] != '(' || tok[len(tok)-1] != ')' {
		return nil
	}
	src := tok[1 : len(tok)-1]
	out := make([]byte, 0, len(src))
	for i := 0; i < len(src); i++ {
		ch := src[i]
		if ch != '\\' {
			out = append(out, ch)
			continue
		}
		if i+1 >= len(src) {
			out = append(out, '\\')
			break
		}
		n := src[i+1]
		i++
		switch n {
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case '(', ')', '\\':
			out = append(out, n)
		case '\r':
			// line continuation
			if i+1 < len(src) && src[i+1] == '\n' {
				i++
			}
		case '\n':
			// line continuation
		default:
			if n >= '0' && n <= '7' {
				oct := []byte{n}
				for k := 0; k < 2 && i+1 < len(src); k++ {
					c := src[i+1]
					if c < '0' || c > '7' {
						break
					}
					i++
					oct = append(oct, c)
				}
				v, err := strconv.ParseUint(string(oct), 8, 8)
				if err == nil {
					out = append(out, byte(v))
				}
			} else {
				out = append(out, n)
			}
		}
	}
	return out
}

func decodePDFHexToken(tok []byte) string {
	tok = bytes.TrimSpace(tok)
	if len(tok) < 2 || tok[0] != '<' || tok[len(tok)-1] != '>' {
		return ""
	}
	raw := string(tok[1 : len(tok)-1])
	raw = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n':
			return -1
		default:
			return r
		}
	}, raw)
	if raw == "" {
		return ""
	}
	if len(raw)%2 != 0 {
		raw += "0"
	}
	b, err := hex.DecodeString(raw)
	if err != nil || len(b) == 0 {
		return ""
	}
	// Support common UTF-16BE BOM hex text.
	if len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF {
		return decodeUTF16Bytes(b[2:], true)
	}
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		return decodeUTF16Bytes(b[2:], false)
	}
	return decodeTextBytesBestEffort(b)
}

func decodePDFHexTokenWithCMap(tok []byte, cm cmapUnicodeMap) string {
	tok = bytes.TrimSpace(tok)
	if len(tok) < 2 || tok[0] != '<' || tok[len(tok)-1] != '>' {
		return ""
	}
	raw := normalizeHexString(string(tok[1 : len(tok)-1]))
	if raw == "" {
		return ""
	}
	if len(cm.byCodeHex) == 0 || len(cm.keyLens) == 0 {
		return normalizeAdobeSymbolPUAText(decodePDFHexToken(tok))
	}
	srcBytes, err := hex.DecodeString(raw)
	if err != nil || len(srcBytes) == 0 {
		return normalizeAdobeSymbolPUAText(decodePDFHexToken(tok))
	}
	var out strings.Builder
	for i := 0; i < len(srcBytes); {
		matched := false
		for _, n := range cm.keyLens {
			if n <= 0 || i+n > len(srcBytes) {
				continue
			}
			key := strings.ToUpper(hex.EncodeToString(srcBytes[i : i+n]))
			if u, ok := cm.byCodeHex[key]; ok {
				out.WriteString(u)
				i += n
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		// Single-byte fallback to avoid infinite loops.
		out.WriteString(string([]byte{srcBytes[i]}))
		i++
	}
	return normalizeAdobeSymbolPUAText(out.String())
}

func appendEmbeddedCMapNamesFromObject(data []byte, n int, mode string, out map[string]int) {
	if n <= 0 || out == nil {
		return
	}
	blk, err := ExtractObjectBlockByNumberBytes(data, n, mode)
	if err != nil {
		return
	}
	objBlock := []byte(blk)
	dict := objectDictBytesBeforeStream(objBlock)
	raw, ok := ParseObjectStreamBytes(objBlock)
	if !ok {
		return
	}
	decoded := DecodeStreamFiltersBestEffort(dict, raw)
	if !bytes.Contains(decoded, []byte("begincmap")) {
		return
	}
	if m := reCMapNameDef.FindSubmatch(decoded); len(m) >= 2 {
		name := strings.TrimSpace(string(m[1]))
		if name != "" {
			out[name] = n
		}
	}
}

func buildEmbeddedCMapNameIndex(data []byte, mode string) map[string]int {
	out := map[string]int{}
	// Prefer scanning only objects known to exist; aggregate paths register an object index.
	if nums := IndexedDirectObjectNumbers(data); len(nums) > 0 {
		for _, n := range nums {
			appendEmbeddedCMapNamesFromObject(data, n, mode, out)
		}
		return out
	}
	// Without an index, such as tests or unregistered paths, keep capped 1..upper scanning.
	upper := detectNextObjectNumberByScan(data)
	if upper < 2 {
		return out
	}
	if upper > maxObjectsToScanForEmbeddedCMap {
		upper = maxObjectsToScanForEmbeddedCMap
	}
	for n := 1; n < upper; n++ {
		appendEmbeddedCMapNamesFromObject(data, n, mode, out)
	}
	return out
}

func refreshCMapKeyLens(cm *cmapUnicodeMap) {
	if cm == nil {
		return
	}
	seen := map[int]bool{}
	cm.keyLens = cm.keyLens[:0]
	for k := range cm.byCodeHex {
		n := len(k) / 2
		if n > 0 {
			seen[n] = true
		}
	}
	for n := range seen {
		cm.keyLens = append(cm.keyLens, n)
	}
	sort.Slice(cm.keyLens, func(i, j int) bool { return cm.keyLens[i] > cm.keyLens[j] })
}

func normalizeHexString(raw string) string {
	raw = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n':
			return -1
		default:
			return r
		}
	}, raw)
	if len(raw)%2 != 0 {
		raw += "0"
	}
	return strings.ToUpper(raw)
}

func decodeUnicodeHexData(raw string) string {
	raw = normalizeHexString(raw)
	if raw == "" {
		return ""
	}
	// ToUnicode CMap targets are usually UTF-16BE even without a BOM.
	if len(raw)%4 == 0 {
		if b, err := hex.DecodeString(raw); err == nil && len(b) >= 2 {
			if s := decodeUTF16Bytes(b, true); s != "" {
				return s
			}
		}
	}
	return decodePDFHexToken([]byte("<" + raw + ">"))
}

func decodeUTF16Bytes(b []byte, bigEndian bool) string {
	if len(b) < 2 {
		return ""
	}
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	u16 := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		var v uint16
		if bigEndian {
			v = uint16(b[i])<<8 | uint16(b[i+1])
		} else {
			v = uint16(b[i+1])<<8 | uint16(b[i])
		}
		u16 = append(u16, v)
	}
	return string(utf16.Decode(u16))
}

func decodeTextBytesBestEffort(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	// 1) For non-UTF-8 raw bytes, try GB18030 directly before rune fallback.
	if !utf8.Valid(b) {
		if dec, err := simplifiedchinese.GB18030.NewDecoder().Bytes(b); err == nil && len(dec) > 0 && utf8.Valid(dec) {
			cand := string(dec)
			if strings.TrimSpace(cand) != "" && !looksLikeCJKMojibakeByMarkers(cand) {
				return cand
			}
			// Keep this semantically richer path even if the candidate still has minor noise.
			if strings.TrimSpace(cand) != "" {
				return cand
			}
		}
	}

	// 2) Normal path: UTF-8 or rune fallback.
	base := ""
	if utf8.Valid(b) {
		base = string(b)
	} else {
		base = string(bytes.Runes(b))
	}
	// 3) Try secondary repair for wrong-direction decoding artifacts.
	if repaired := maybeRepairUTF8Mojibake(base); repaired != "" {
		return repaired
	}
	return base
}

func maybeRepairUTF8Mojibake(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	// Try GB18030 re-decoding only for likely mojibake to avoid touching normal UTF-8.
	if !looksLikeGBKMojibake(s) && !looksLikeCJKMojibakeByMarkers(s) {
		return s
	}

	origScore := cjkRepairScore(s)
	best := s
	bestScore := origScore

	// Direction A: decode current bytes as GB18030, common for original GBK bytes.
	if decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes([]byte(s)); err == nil && len(decoded) > 0 && utf8.Valid(decoded) {
		cand := string(decoded)
		if betterCJKText(cand, best) {
			best = cand
			bestScore = cjkRepairScore(cand)
		}
	}

	// Direction B: encode mojibake text as GB18030 bytes, then interpret as UTF-8.
	if enc, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(s)); err == nil && len(enc) > 0 && utf8.Valid(enc) {
		cand := string(enc)
		if betterCJKText(cand, best) {
			best = cand
			bestScore = cjkRepairScore(cand)
		}
	}

	if bestScore > origScore {
		return best
	}
	return s
}

func looksLikeGBKMojibake(s string) bool {
	rs := []rune(s)
	if len(rs) < 4 {
		return false
	}
	weird := 0
	han := 0
	for _, r := range rs {
		if unicode.Is(unicode.Han, r) {
			han++
			continue
		}
		// These fragments appear frequently in mojibake samples.
		if (r >= 0x0200 && r <= 0x03FF) || (r >= 0x1D00 && r <= 0x1D7F) {
			weird++
		}
	}
	// Relax thresholds for mixed text when clear Greek/extended-Latin fragments appear.
	if weird >= 2 && han == 0 {
		return true
	}
	return weird >= 2 && weird*3 >= han
}

func betterCJKText(candidate string, current string) bool {
	ch := func(s string) int {
		n := 0
		for _, r := range s {
			if unicode.Is(unicode.Han, r) {
				n++
			}
		}
		return n
	}
	wd := func(s string) int {
		n := 0
		for _, r := range s {
			if (r >= 0x0200 && r <= 0x03FF) || (r >= 0x1D00 && r <= 0x1D7F) || r == '\uFFFD' {
				n++
			}
		}
		return n
	}
	cHan, oHan := ch(candidate), ch(current)
	cWeird, oWeird := wd(candidate), wd(current)
	// Prefer candidates with more readable CJK text and fewer suspicious symbols.
	if cHan >= oHan+1 && cWeird <= oWeird {
		return true
	}
	// Fallback: accept a candidate when suspicious symbols drop significantly.
	if oWeird >= 3 && cWeird*2 <= oWeird {
		return true
	}
	// Prefer the side with fewer common CJK mojibake marker strings.
	return mojibakeMarkerCount(candidate)*2 < mojibakeMarkerCount(current)
}

func cjkRepairScore(s string) int {
	rs := []rune(s)
	if len(rs) == 0 {
		return -1 << 30
	}
	han := 0
	weird := 0
	for _, r := range rs {
		if unicode.Is(unicode.Han, r) {
			han++
		}
		if (r >= 0x0200 && r <= 0x03FF) || (r >= 0x1D00 && r <= 0x1D7F) || r == '\uFFFD' {
			weird++
		}
	}
	return han*3 - weird*2 - mojibakeMarkerCount(s)*3
}

func looksLikeCJKMojibakeByMarkers(s string) bool {
	return mojibakeMarkerCount(s) >= 2
}

func mojibakeMarkerCount(s string) int {
	// Common marker fragments caused by UTF-8/GBK mismatches.
	markers := []string{
		"\u951f\u65a4\u62f7", "\u951f", "\u9496", "\u7f01", "\u93b7", "\u6d3f", "\u9359",
		"\u935a", "\u93c2", "\u6769", "\u951b", "\u951f\u72e1", "\u951f\u65a4",
	}
	n := 0
	for _, m := range markers {
		n += strings.Count(s, m)
	}
	return n
}

func lineContainsHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// collapseHorizontalSpaceToSingle collapses horizontal whitespace while
// preserving single inter-word spaces and layout line boundaries.
func collapseHorizontalSpaceToSingle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	spacePending := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\u3000' {
			spacePending = true
			continue
		}
		if spacePending && b.Len() > 0 {
			b.WriteByte(' ')
		}
		spacePending = false
		b.WriteRune(r)
	}
	return b.String()
}

// collapseSpacedUpperLatinPairsIter merges uppercase Latin letters separated
// only by spaces, a common one-letter-per-Tj PDF pattern. It triggers with at
// least three letters, treats long uppercase blocks as words unless an acronym
// is already being built, and avoids merging when the last uppercase letter is
// immediately followed by lowercase text.
func collapseSpacedUpperLatinPairsIter(s string) string {
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(runes))
	w := 0
outer:
	for w < len(runes) {
		if runes[w] < 'A' || runes[w] > 'Z' {
			b.WriteRune(runes[w])
			w++
			continue
		}
		e := w
		letters := 0
		for e < len(runes) {
			if runes[e] < 'A' || runes[e] > 'Z' {
				break
			}
			j := e
			for j < len(runes) && runes[j] >= 'A' && runes[j] <= 'Z' {
				j++
			}
			runLen := j - e
			if runLen >= 3 {
				if letters == 0 {
					for k := w; k < j; k++ {
						b.WriteRune(runes[k])
					}
					w = j
					continue outer
				}
				if letters == 1 && runLen >= 4 {
					letters += runLen
					e = j
					if e < len(runes) && runes[e] == ' ' && e+1 < len(runes) && runes[e+1] >= 'A' && runes[e+1] <= 'Z' {
						e++
						continue
					}
					break
				}
				break
			}
			letters += runLen
			e = j
			if e < len(runes) && runes[e] == ' ' && e+1 < len(runes) && runes[e+1] >= 'A' && runes[e+1] <= 'Z' {
				e++
				continue
			}
			break
		}
		if letters >= 3 {
			if e < len(runes) && unicode.IsLower(runes[e]) {
				b.WriteRune(runes[w])
				w++
				continue
			}
			for k := w; k < e; k++ {
				if runes[k] != ' ' {
					b.WriteRune(runes[k])
				}
			}
			w = e
			continue
		}
		b.WriteRune(runes[w])
		w++
	}
	return b.String()
}

func normalizeExtractedText(s string) string {
	s = maybeRepairUTF8Mojibake(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		var normalized string
		if strings.TrimSpace(line) == "" {
			normalized = ""
		} else if lineContainsHan(line) {
			normalized = collapseHorizontalSpaceToSingle(line)
			normalized = collapseSpacedUpperLatinPairsIter(normalized)
		} else {
			normalized = strings.Join(strings.Fields(line), " ")
		}
		normalized = normalizeLineForExtraction(normalized)
		// Preserve blank-line structure from the PDF stream.
		clean = append(clean, normalized)
	}
	out := strings.Join(clean, "\n")
	out = normalizeSpacedASCIIWord(out)
	out = compactFragmentedSingleRuneLines(out)
	return cleanupCJKExtractionArtifacts(out)
}

func normalizeSpacedASCIIWord(s string) string {
	if s == "" {
		return s
	}
	return reSpacedASCIIWord.ReplaceAllStringFunc(s, func(m string) string {
		return strings.ReplaceAll(m, " ", "")
	})
}

func normalizeLineForExtraction(s string) string {
	if s == "" {
		return s
	}
	s = strings.NewReplacer(
		"\x00", "", "\xFF", "", "\t", " ",
		"Ÿ", "", "™", "", "Ä", "", "Ñ", "",
		"､", "、", // halfwidth ideographic comma -> ideographic comma
		"，", ",", "。", ".", "：", ":", "；", ";",
	).Replace(s)
	s = reInvalidTextRune.ReplaceAllString(s, "")
	if lineContainsHan(s) {
		s = collapseHorizontalSpaceToSingle(s)
	} else {
		s = strings.Join(strings.Fields(s), " ")
	}
	return strings.TrimSpace(s)
}

// compactFragmentedSingleRuneLines merges OCR/CMap outputs where one glyph is emitted per line.
func compactFragmentedSingleRuneLines(s string) string {
	lines := strings.Split(s, "\n")
	nonEmpty := 0
	short := 0
	totalRunes := 0
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		nonEmpty++
		rn := utf8.RuneCountInString(t)
		totalRunes += rn
		if rn <= 2 {
			short++
		}
	}
	// Only trigger when heavily fragmented; keep normal documents unchanged.
	if nonEmpty < 20 {
		return s
	}
	avgRunes := float64(totalRunes) / float64(nonEmpty)
	if short*100/nonEmpty < 45 && avgRunes > 2.5 {
		return s
	}
	var out strings.Builder
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			if out.Len() > 0 && !strings.HasSuffix(out.String(), "\n") {
				out.WriteString("\n")
			}
			continue
		}
		if utf8.RuneCountInString(t) <= 2 {
			out.WriteString(t)
			continue
		}
		if out.Len() > 0 && !strings.HasSuffix(out.String(), "\n") {
			out.WriteString("\n")
		}
		out.WriteString(t)
	}
	return strings.Trim(out.String(), " \t\r")
}

// cleanupCJKExtractionArtifacts removes common glyph-mapping artifacts without OCR.
func cleanupCJKExtractionArtifacts(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	// Drop most control chars except line breaks/tabs.
	runes := []rune(s)
	filtered := make([]rune, 0, len(runes))
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\n' || r == '\t' || r == '\r' {
			filtered = append(filtered, r)
			continue
		}
		if r < 32 {
			continue
		}
		// Map PUA/icon-font bullets to list prefixes, outputting consecutive prefixes once.
		if isPUAListLikeRune(r) {
			atLineStart := len(filtered) == 0 || filtered[len(filtered)-1] == '\n'
			if atLineStart {
				filtered = append(filtered, '-', ' ')
			}
			// Consume consecutive icon prefix characters and separators.
			for i+1 < len(runes) {
				nx := runes[i+1]
				if isPUAListLikeRune(nx) || isLikelyGlyphNoiseRune(nx) || nx == ' ' || nx == '\t' || nx == '·' || nx == '•' || nx == ':' || nx == '：' {
					i++
					continue
				}
				break
			}
			continue
		}
		filtered = append(filtered, r)
	}
	runes = filtered
	filtered = make([]rune, 0, len(runes))
	for i, r := range runes {
		// Remove isolated ASCII letters that are likely glyph-map residue
		// when they are surrounded by CJK characters.
		if isASCIIAlpha(r) {
			prev := nearestNonSpaceRune(runes, i, -1)
			next := nearestNonSpaceRune(runes, i, +1)
			if (isCJKRune(prev) || isCJKPunct(prev)) && (isCJKRune(next) || isCJKPunct(next) || isDigitRune(next)) {
				continue
			}
			if (isCJKRune(next) || isCJKPunct(next)) && (isCJKRune(prev) || isCJKPunct(prev) || isDigitRune(prev)) {
				continue
			}
		}
		// Replacement rune between CJK chars is usually unmapped glyph noise.
		if r == '\uFFFD' {
			prev := nearestNonSpaceRune(runes, i, -1)
			next := nearestNonSpaceRune(runes, i, +1)
			if (isCJKRune(prev) || isCJKPunct(prev)) && (isCJKRune(next) || isCJKPunct(next) || next == 0) {
				continue
			}
			if (isCJKRune(next) || isCJKPunct(next)) && (isCJKRune(prev) || isCJKPunct(prev) || prev == 0) {
				continue
			}
		}
		// Filter high-frequency glyph-mapping noise while preserving semantic text and punctuation.
		if isLikelyGlyphNoiseRune(r) {
			prev := nearestNonSpaceRune(runes, i, -1)
			next := nearestNonSpaceRune(runes, i, +1)
			if (isCJKRune(prev) || isCJKPunct(prev) || prev == '-') && (isCJKRune(next) || isCJKPunct(next) || next == 0 || next == '-') {
				continue
			}
		}
		filtered = append(filtered, r)
	}
	filtered = removeShortASCIIIslandsBetweenCJK(filtered)
	filtered = normalizeCJKPunctuationInContext(filtered)
	// Trim only horizontal whitespace and preserve leading/trailing newlines.
	return strings.Trim(string(filtered), " \t\r")
}

func isPUAListLikeRune(r rune) bool {
	return r >= 0xE000 && r <= 0xF8FF
}

func isLikelyGlyphNoiseRune(r rune) bool {
	switch r {
	case 'Ÿ', '™', 'Ä', 'Ñ', '�':
		return true
	default:
		return false
	}
}

func removeShortASCIIIslandsBetweenCJK(rs []rune) []rune {
	if len(rs) == 0 {
		return rs
	}
	out := make([]rune, 0, len(rs))
	i := 0
	for i < len(rs) {
		if isASCIIAlpha(rs[i]) {
			j := i
			for j < len(rs) && isASCIIAlpha(rs[j]) {
				j++
			}
			prev := nearestNonSpaceRune(rs, i, -1)
			next := nearestNonSpaceRune(rs, j-1, +1)
			// Remove only single-letter islands; keep 2+ letters such as TO,
			// AGI, or brand acronyms.
			if (isCJKRune(prev) || isCJKPunct(prev)) && (isCJKRune(next) || isCJKPunct(next)) && (j-i) == 1 {
				i = j
				continue
			}
			out = append(out, rs[i:j]...)
			i = j
			continue
		}
		out = append(out, rs[i])
		i++
	}
	return out
}

func normalizeCJKPunctuationInContext(rs []rune) []rune {
	if len(rs) == 0 {
		return rs
	}
	out := make([]rune, 0, len(rs))
	for i, r := range rs {
		prev := nearestNonSpaceRune(rs, i, -1)
		next := nearestNonSpaceRune(rs, i, +1)
		if (isCJKRune(prev) || isCJKPunct(prev)) && (isCJKRune(next) || isCJKPunct(next) || isDigitRune(next)) {
			switch r {
			case ',':
				r = '，'
			case ';':
				r = '；'
			case ':':
				r = '：'
			}
		}
		out = append(out, r)
	}
	return out
}

func isASCIIAlpha(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func nearestNonSpaceRune(rs []rune, i int, dir int) rune {
	for j := i + dir; j >= 0 && j < len(rs); j += dir {
		if rs[j] == ' ' || rs[j] == '\t' || rs[j] == '\n' || rs[j] == '\r' {
			continue
		}
		return rs[j]
	}
	return 0
}

func isCJKRune(r rune) bool {
	if r == 0 {
		return false
	}
	if unicode.Is(unicode.Han, r) {
		return true
	}
	// CJK punctuation/fullwidth forms seen in Chinese PDFs.
	if (r >= 0x3000 && r <= 0x303F) || (r >= 0xFF00 && r <= 0xFFEF) {
		return true
	}
	return false
}

func isCJKPunct(r rune) bool {
	switch r {
	case '，', '。', '；', '：', '！', '？', '、', '（', '）', '《', '》', '“', '”', '‘', '’', '≥', '≤', '%', '＋', '+', '-', '×', '÷':
		return true
	default:
		return false
	}
}

func isDigitRune(r rune) bool {
	return r >= '0' && r <= '9'
}

func classifyTextChunkType(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return "other"
	}

	// Simple code-block signal: code symbols plus newlines or statement semicolons.
	if (strings.Contains(t, "{") && strings.Contains(t, "}")) ||
		(strings.Contains(t, ";") && strings.Contains(t, "(") && strings.Contains(t, ")")) {
		return "code"
	}

	// Rule-based heading signal: short text, no sentence-ending punctuation, common section prefix.
	if len([]rune(t)) <= 60 &&
		!strings.HasSuffix(t, "。") &&
		!strings.HasSuffix(t, ".") &&
		!strings.HasSuffix(t, "；") &&
		!strings.HasSuffix(t, ";") {
		if hasHeadingPrefix(t) || !strings.Contains(t, "\n") {
			return "heading"
		}
	}

	// Default to paragraph.
	return "paragraph"
}

func hasHeadingPrefix(s string) bool {
	prefixes := []string{
		"\u7b2c", "\u7ae0", "\u8282", "\u9644\u5f55", "Chapter", "Section",
		"1.", "2.", "3.", "4.", "5.", "6.", "7.", "8.", "9.",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
