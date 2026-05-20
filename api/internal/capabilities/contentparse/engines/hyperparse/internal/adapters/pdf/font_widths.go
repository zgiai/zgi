package pdf

import (
	"bytes"
	"encoding/hex"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// pdfFontWidthModel stores embedded font horizontal widths in glyph-space units
// matching PDF Widths/W, used to advance tm after Tj/TJ.
type pdfFontWidthModel struct {
	kind string // "simple" | "cid"
	// simple: Widths[code-firstChar]
	firstChar int
	widths    []float64
	// cid: /DW + /W
	defaultW1000 float64
	cid          map[int]float64
}

func (m *pdfFontWidthModel) ok() bool {
	if m == nil || m.kind == "" {
		return false
	}
	switch m.kind {
	case "simple":
		return len(m.widths) > 0
	case "cid":
		// Default width alone is often biased when /W is missing; rough estimates are safer.
		return len(m.cid) > 0
	default:
		return false
	}
}

var (
	rePDFFirstChar = regexp.MustCompile(`/(?i)FirstChar\s+(\d+)`)
	rePDFLastChar  = regexp.MustCompile(`/(?i)LastChar\s+(\d+)`)
	rePDFDW        = regexp.MustCompile(`/(?i)DW\s+([-+]?(?:\d+\.\d+|\d+\.|\.\d+|\d+))`)
)

// buildPageFontWidthsMap parses /Widths or CID /W from page Resources, keyed by Tf resource name.
func buildPageFontWidthsMap(data []byte, sp PageRenderSpec, mode string) map[string]*pdfFontWidthModel {
	out := map[string]*pdfFontWidthModel{}
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
		fontKey := m[1]
		fontObj, err := strconv.Atoi(m[2])
		if err != nil || fontObj <= 0 {
			continue
		}
		fontBlock, err := ExtractObjectBlockByNumberBytes(data, fontObj, mode)
		if err != nil {
			continue
		}
		if model := parseFontWidthModelFromFontObject(data, []byte(fontBlock), mode); model != nil && model.ok() {
			out[fontKey] = model
		}
	}
	return out
}

func parseFontWidthModelFromFontObject(data []byte, fontBlock []byte, mode string) *pdfFontWidthModel {
	body := objectBodyFromBlockBytes(fontBlock)
	if body == "" {
		body = objectPayloadTrimmedForFont(fontBlock)
	}
	if body == "" {
		return nil
	}
	if strings.Contains(body, "/Subtype /Type0") || strings.Contains(body, "/Subtype/Type0") {
		if cidBody := extractDescendantCIDFontDict(data, body, mode); cidBody != "" {
			return parseCIDFontWidths(data, cidBody, mode)
		}
	}
	return parseSimpleFontWidths(data, body, mode)
}

func objectPayloadTrimmedForFont(block []byte) string {
	s := string(block)
	if i := strings.Index(s, "endobj"); i >= 0 {
		s = s[:i]
	}
	if idx := strings.Index(s, "obj"); idx >= 0 {
		s = strings.TrimSpace(s[idx+3:])
	}
	return strings.TrimSpace(s)
}

func parseSimpleFontWidths(data []byte, fontBody string, mode string) *pdfFontWidthModel {
	fm := rePDFFirstChar.FindStringSubmatch(fontBody)
	lm := rePDFLastChar.FindStringSubmatch(fontBody)
	if len(fm) < 2 || len(lm) < 2 {
		return nil
	}
	first, err1 := strconv.Atoi(fm[1])
	last, err2 := strconv.Atoi(lm[1])
	if err1 != nil || err2 != nil || last < first || last-first > 4096 {
		return nil
	}
	arr := extractPDFArrayForKey(fontBody, "/Widths", data, mode)
	if arr == "" {
		return nil
	}
	nums := parsePDFArrayFloats(arr)
	want := last - first + 1
	if len(nums) < want {
		return nil
	}
	if len(nums) > want {
		nums = nums[:want]
	}
	return &pdfFontWidthModel{kind: "simple", firstChar: first, widths: nums}
}

func parseCIDFontWidths(data []byte, cidBody string, mode string) *pdfFontWidthModel {
	dw := 1000.0
	if dm := rePDFDW.FindStringSubmatch(cidBody); len(dm) >= 2 {
		if v, err := strconv.ParseFloat(dm[1], 64); err == nil && v > 0 {
			dw = v
		}
	}
	arr := extractPDFArrayForKey(cidBody, "/W", data, mode)
	if arr == "" {
		return nil
	}
	m := parseCIDWArray(arr)
	if len(m) == 0 {
		return nil
	}
	return &pdfFontWidthModel{kind: "cid", defaultW1000: dw, cid: m}
}

func extractDescendantFontsInner(fontBody string) string {
	idx := strings.Index(fontBody, "/DescendantFonts")
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(fontBody[idx+len("/DescendantFonts"):])
	if !strings.HasPrefix(rest, "[") {
		return ""
	}
	return balancedSquareBracketInner(rest, 0)
}

func extractDescendantCIDFontDict(data []byte, fontBody string, mode string) string {
	inner := extractDescendantFontsInner(fontBody)
	if inner == "" {
		return ""
	}
	inner = strings.TrimSpace(inner)
	if m := pdfLeadingIndirectRefRE.FindStringSubmatch(inner); len(m) >= 2 {
		n, err := strconv.Atoi(m[1])
		if err != nil || n <= 0 {
			return ""
		}
		blk, err := ExtractObjectBlockByNumberBytes(data, n, mode)
		if err != nil {
			return ""
		}
		body := objectBodyFromBlockBytes([]byte(blk))
		if body != "" {
			return body
		}
		return objectPayloadTrimmedForFont([]byte(blk))
	}
	if pos := strings.Index(inner, "<<"); pos >= 0 {
		return extractFirstPDFDict(inner[pos:])
	}
	return ""
}

func extractFirstPDFDict(s string) string {
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "<<") {
		return ""
	}
	depth := 0
	for i := 0; i < len(t)-1; i++ {
		if t[i] == '<' && t[i+1] == '<' {
			depth++
			i++
			continue
		}
		if t[i] == '>' && t[i+1] == '>' {
			depth--
			i++
			if depth == 0 {
				return t[:i+1]
			}
		}
	}
	return ""
}

func extractPDFArrayForKey(dictBody string, key string, data []byte, mode string) string {
	pos := strings.Index(dictBody, key)
	if pos < 0 {
		return ""
	}
	rest := strings.TrimSpace(dictBody[pos+len(key):])
	if strings.HasPrefix(rest, "[") {
		return balancedSquareBracketInner(rest, 0)
	}
	if m := pdfLeadingIndirectRefRE.FindSubmatch([]byte(rest)); len(m) >= 2 {
		n, err := strconv.Atoi(string(m[1]))
		if err != nil || n <= 0 {
			return ""
		}
		blk, err := ExtractObjectBlockByNumberBytes(data, n, mode)
		if err != nil {
			return ""
		}
		payload := objectPayloadTrimmedForFont([]byte(blk))
		payload = strings.TrimSpace(payload)
		if strings.HasPrefix(payload, "[") {
			return balancedSquareBracketInner(payload, 0)
		}
	}
	return ""
}

func balancedSquareBracketInner(s string, openIdx int) string {
	if openIdx < 0 || openIdx >= len(s) || s[openIdx] != '[' {
		return ""
	}
	depth := 0
	for i := openIdx; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return s[openIdx+1 : i]
			}
		}
	}
	return ""
}

func parsePDFArrayFloats(inner string) []float64 {
	re := regexp.MustCompile(`[-+]?(?:\d+\.\d+|\d+\.|\.\d+|\d+)`)
	ms := re.FindAllString(inner, -1)
	out := make([]float64, 0, len(ms))
	for _, t := range ms {
		v, err := strconv.ParseFloat(t, 64)
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	return out
}

func parseCIDWArray(inner string) map[int]float64 {
	out := make(map[int]float64)
	b := []byte(strings.TrimSpace(inner))
	i := skipPDFWsIdx(b, 0)
	for i < len(b) {
		c, ni, ok := scanPDFNumberPrefixBytes(b, i)
		if !ok {
			break
		}
		i = skipPDFWsIdx(b, ni)
		if i >= len(b) {
			break
		}
		if b[i] == '[' {
			inArr := balancedSquareBracketInner(string(b), i)
			nums := parsePDFArrayFloats(inArr)
			cc := int(c)
			for j, w := range nums {
				out[cc+j] = w
			}
			// Skip the closing bracket.
			depth := 0
			for j := i; j < len(b); j++ {
				if b[j] == '[' {
					depth++
				}
				if b[j] == ']' {
					depth--
					if depth == 0 {
						i = j + 1
						break
					}
				}
			}
			i = skipPDFWsIdx(b, i)
			continue
		}
		c2, i2, ok2 := scanPDFNumberPrefixBytes(b, i)
		if !ok2 {
			break
		}
		i = skipPDFWsIdx(b, i2)
		w, i3, ok3 := scanPDFNumberPrefixBytes(b, i)
		if !ok3 {
			break
		}
		lo, hi := int(c), int(c2)
		if hi < lo {
			lo, hi = hi, lo
		}
		for gid := lo; gid <= hi; gid++ {
			out[gid] = w
		}
		i = skipPDFWsIdx(b, i3)
	}
	return out
}

func skipPDFWsIdx(b []byte, i int) int {
	for i < len(b) && isPDFWhitespace(b[i]) {
		i++
	}
	return i
}

func scanPDFNumberPrefixBytes(b []byte, from int) (float64, int, bool) {
	i := skipPDFWsIdx(b, from)
	if i >= len(b) {
		return 0, from, false
	}
	start := i
	if b[i] == '-' || b[i] == '+' {
		i++
	}
	okDigit := false
	for i < len(b) && ((b[i] >= '0' && b[i] <= '9') || b[i] == '.') {
		okDigit = true
		i++
	}
	if !okDigit {
		return 0, from, false
	}
	v, err := strconv.ParseFloat(string(b[start:i]), 64)
	if err != nil {
		return 0, from, false
	}
	return v, i, true
}

func forEachCmapCodeWord(src []byte, cm cmapUnicodeMap, fn func(word []byte)) {
	if len(cm.byCodeHex) == 0 || len(cm.keyLens) == 0 {
		for i := range src {
			fn(src[i : i+1])
		}
		return
	}
	i := 0
	for i < len(src) {
		matched := false
		for _, n := range cm.keyLens {
			if n <= 0 || i+n > len(src) {
				continue
			}
			key := strings.ToUpper(hex.EncodeToString(src[i : i+n]))
			if _, ok := cm.byCodeHex[key]; ok {
				fn(src[i : i+n])
				i += n
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		fn(src[i : i+1])
		i++
	}
}

func intFromBE(word []byte) int {
	n := 0
	for _, b := range word {
		n = n<<8 | int(b)
	}
	return n
}

func (m *pdfFontWidthModel) width1000Simple(code int) float64 {
	if code >= m.firstChar && code < m.firstChar+len(m.widths) {
		return m.widths[code-m.firstChar]
	}
	return 480
}

func (m *pdfFontWidthModel) width1000CID(cid int) float64 {
	if m.cid != nil {
		if w, ok := m.cid[cid]; ok {
			return w
		}
	}
	if m.defaultW1000 > 0 {
		return m.defaultW1000
	}
	return 1000
}

func widthModelAdvanceRaw(raw []byte, cm cmapUnicodeMap, wm *pdfFontWidthModel, fontSize, charSpace, wordSpace, horizScale float64) float64 {
	if wm == nil || !wm.ok() || len(raw) == 0 {
		return 0
	}
	fs := fontSize
	if fs <= 0 {
		fs = geomDefaultFontSize
	}
	hz := horizScale
	if hz <= 0 {
		hz = 100
	}
	sc := hz / 100
	var sum float64
	switch wm.kind {
	case "simple":
		forEachCmapCodeWord(raw, cm, func(word []byte) {
			for _, bb := range word {
				gw := wm.width1000Simple(int(bb))
				sum += (gw / 1000) * fs * sc
				sum += charSpace
				if bb == ' ' {
					sum += wordSpace
				}
			}
		})
	case "cid":
		forEachCmapCodeWord(raw, cm, func(word []byte) {
			cid := intFromBE(word)
			gw := wm.width1000CID(cid)
			sum += (gw / 1000) * fs * sc
			sum += charSpace
		})
	default:
		return 0
	}
	return sum
}

// geomHorizontalAdvanceForTextShow estimates Tj horizontal advance from embedded widths.
// geomPrevRunWidthTextSpace estimates decoded simple-font visual width in text
// space; it is trusted only for ASCII compatible with single-byte encoding.
func geomPrevRunWidthTextSpace(prev geomTextRun, wm *pdfFontWidthModel) float64 {
	fs := geomFontSizeForRun(prev)
	s := strings.TrimSpace(prev.s)
	if wm == nil || !wm.ok() || wm.kind != "simple" || s == "" {
		return estimateTextWidthTextSpace(s, fs)
	}
	var sum float64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 128 {
			return estimateTextWidthTextSpace(s, fs)
		}
		gw := wm.width1000Simple(int(c))
		sum += (gw / 1000) * fs
	}
	if sum <= 0 {
		return estimateTextWidthTextSpace(s, fs)
	}
	return sum
}

func geomHorizontalAdvanceForTextShow(decoded string, raw []byte, cm cmapUnicodeMap, wm *pdfFontWidthModel, fontSize, charSpace, wordSpace, horizScale float64) float64 {
	if wm == nil || !wm.ok() || len(raw) == 0 {
		return estimateGeomShowAdvance(decoded, fontSize, charSpace, wordSpace, horizScale)
	}
	adv := widthModelAdvanceRaw(raw, cm, wm, fontSize, charSpace, wordSpace, horizScale)
	if adv <= 0 || math.IsNaN(adv) {
		return estimateGeomShowAdvance(decoded, fontSize, charSpace, wordSpace, horizScale)
	}
	return adv
}

func concatLiteralRawBytes(seg []byte) []byte {
	var buf []byte
	for _, lit := range reLiteral.FindAll(seg, -1) {
		b := decodePDFLiteralBytes(lit)
		if len(b) > 0 {
			buf = append(buf, b...)
		}
	}
	return buf
}

func concatHexRawBytes(seg []byte) []byte {
	var buf []byte
	for _, tok := range reHexToken.FindAll(seg, -1) {
		t := bytes.TrimSpace(tok)
		if len(t) < 3 || t[0] != '<' {
			continue
		}
		raw := normalizeHexString(string(t[1 : len(t)-1]))
		if raw == "" {
			continue
		}
		if len(raw)%2 != 0 {
			raw += "0"
		}
		b, err := hex.DecodeString(raw)
		if err == nil && len(b) > 0 {
			buf = append(buf, b...)
		}
	}
	return buf
}
