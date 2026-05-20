package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// BasicInfo is the PDF structural probe result used by the local parser.
type BasicInfo struct {
	Version      string
	FileSize     int64
	PageCount    int
	CountSource  string
	Title        string
	Author       string
	Subject      string
	Producer     string
	Creator      string
	Keywords     string
	StartXRef    int
	XRefType     string
	HasEOFMarker bool
	HasTrailer   bool
}

// PageInfo contains minimal leaf page-tree data for later split/rotate operations.
type PageInfo struct {
	ObjectNumber int
	MediaBox     string
	CropBox      string
	BleedBox     string
	TrimBox      string
	ArtBox       string
}

// PageRenderSpec contains minimal render references needed for page write-back.
type PageRenderSpec struct {
	ObjectNumber       int
	MediaBox           string
	CropBox            string
	BleedBox           string
	TrimBox            string
	ArtBox             string
	ContentsRefObject  int
	ResourcesRefObject int
	ContentsObject     string
	ResourcesObject    string
}

const (
	ValidationModeStrict  = "strict"
	ValidationModeRelaxed = "relaxed"

	CountSourceRootChain    = "root_chain"
	CountSourceScanFallback = "scan_fallback"
)

var (
	// /Pages fallback: some generators use arrays or unusual whitespace, where
	// simple Fields splitting fails.
	pagesRefDirectRE  = regexp.MustCompile(`(?s)/Pages\s+(\d+)\s+(\d+)\s+R\b`)
	pagesRefBracketRE = regexp.MustCompile(`(?s)/Pages\s*\[\s*(\d+)\s+(\d+)\s+R\s*\]`)
	// Indirect reference after a key name; supports common compact forms:
	// 1) /Key 30 0 R
	// 2) /Key 30 0 R/NextKey, where R touches the next key
	// 3) /Key [18 0 R 19 0 R ...], taking the first array reference
	pdfIndirectRefAfterKeyRE = regexp.MustCompile(`(?s)^\s*\[?\s*(\d+)\s+(\d+)\s+R`)
	// Standalone indirect references in value position, excluding array forms.
	pdfLeadingIndirectRefRE = regexp.MustCompile(`^\s*(\d+)\s+(\d+)\s+R\b`)
	// All indirect references in the fragment, preserving order for /Contents arrays.
	pdfAnyIndirectRefRE = regexp.MustCompile(`(\d+)\s+(\d+)\s+R\b`)
)

// Supports /Type/Name and /Type /Name, while distinguishing /Type/Page from
// /Type/Pages without regexp negative lookahead.
func objectBlockHasCatalogType(objBlock []byte) bool {
	return pdfTypeMarkerIndex(objBlock, "Catalog") >= 0
}

func objectBlockHasPagesType(objBlock []byte) bool {
	return pdfTypeMarkerIndex(objBlock, "Pages") >= 0
}

func objectBlockHasPageType(objBlock []byte) bool {
	return pdfTypeMarkerIndex(objBlock, "Page") >= 0
}

func pdfTypeMarkerIndex(b []byte, want string) int {
	cursor := 0
	for {
		rel := bytes.Index(b[cursor:], []byte("/Type"))
		if rel < 0 {
			return -1
		}
		i := cursor + rel
		if got, ok := pdfTypeNameAfterMarker(b, i); ok && got == want {
			return i
		}
		cursor = i + 1
	}
}

// pdfTypeNameAfterMarker parses the PDF type name after "/Type", such as Catalog or Pages.
func pdfTypeNameAfterMarker(b []byte, i int) (string, bool) {
	if i < 0 || i+len("/Type") > len(b) || !bytes.HasPrefix(b[i:], []byte("/Type")) {
		return "", false
	}
	rest := b[i+len("/Type"):]
	rest = bytes.TrimLeft(rest, " \t\r\n")
	if len(rest) == 0 || rest[0] != '/' {
		return "", false
	}
	rest = rest[1:]
	if len(rest) == 0 {
		return "", false
	}
	// Name ends at a delimiter.
	end := 0
	for end < len(rest) {
		c := rest[end]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '>' || c == '/' || c == '[' || c == ']' {
			break
		}
		end++
	}
	if end == 0 {
		return "", false
	}
	return string(rest[:end]), true
}

type xrefTableStats struct {
	firstStartObj  int
	maxObjPlusOne  int
	trailerSize    int
	hasTrailerSize bool
	rootObjNumber  int
	hasRoot        bool
	inUseEntries   []xrefInUseEntry
}

type catalogInfo struct {
	objNumber   int
	pagesObjNum int
	hasPagesRef bool
}

type xrefInUseEntry struct {
	objNumber int
	offset    int
}

type xrefStreamStats struct {
	objNumber    int
	hasSize      bool
	size         int
	hasRoot      bool
	rootObjNum   int
	w            [3]int
	entryWidth   int
	entriesCount int
	indexRanges  []xrefIndexRange
	indexTotal   int
}

type xrefIndexRange struct {
	start int
	count int
}

type xrefStreamEntry struct {
	objNum int
	typ    int
	f2     int
	f3     int
}

// InspectBasic performs minimal PDF structural validation.
// 1) header (%PDF-x.y), 2) %%EOF, 3) startxref, 4) xref table or stream.
func InspectBasic(path string) (*BasicInfo, error) {
	return InspectBasicWithMode(path, ValidationModeRelaxed)
}

// InspectBasicBytes supports upload paths without requiring a temporary file.
func InspectBasicBytes(data []byte, mode string) (*BasicInfo, error) {
	return inspectBasicData(data, mode)
}

// InspectBasicWithMode runs structural validation for the selected mode.
func InspectBasicWithMode(path, mode string) (*BasicInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return inspectBasicData(data, mode)
}

func inspectBasicData(data []byte, mode string) (*BasicInfo, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("pdf too small: %d bytes", len(data))
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, fmt.Errorf("invalid pdf header: missing %%PDF-")
	}

	version := string(data[5:8])
	if !isVersionToken(version) {
		return nil, fmt.Errorf("invalid pdf version token: %q", version)
	}

	eofPos := bytes.LastIndex(data, []byte("%%EOF"))
	if eofPos < 0 {
		return nil, fmt.Errorf("invalid pdf: missing %%EOF")
	}

	startXRefTagPos := bytes.LastIndex(data[:eofPos], []byte("startxref"))
	if startXRefTagPos < 0 {
		return nil, fmt.Errorf("invalid pdf: missing startxref")
	}

	startXRef, err := parseStartXRefValue(data[startXRefTagPos+len("startxref") : eofPos])
	if err != nil {
		return nil, fmt.Errorf("invalid startxref value: %w", err)
	}
	if startXRef < 0 || startXRef >= len(data) {
		return nil, fmt.Errorf("startxref out of range: %d", startXRef)
	}

	xrefType, hasTrailer := detectXRefType(data, startXRef, eofPos)
	if xrefType == "" {
		return nil, fmt.Errorf("unsupported xref section at offset %d", startXRef)
	}
	if xrefType == "table" && !hasTrailer {
		return nil, fmt.Errorf("xref table found but trailer missing")
	}
	if xrefType == "table" {
		normMode := normalizeValidationMode(mode)
		stats, err := validateXRefTable(data, startXRef, eofPos, normMode)
		if err != nil {
			return nil, err
		}
		if normMode == ValidationModeStrict {
			if err := validateXRefTableStrictRules(data, startXRef, stats); err != nil {
				return nil, err
			}
		}
	}
	if xrefType == "stream" {
		normMode := normalizeValidationMode(mode)
		stats, err := validateXRefStream(data, startXRef, eofPos, normMode)
		if err != nil {
			return nil, err
		}
		if normMode == ValidationModeStrict {
			if err := validateXRefStreamStrictRules(data, startXRef, stats); err != nil {
				return nil, err
			}
		}
	}

	pageCount, countSource := detectPageCount(data, xrefType, startXRef, eofPos)
	meta := parseInfoMetadata(data, startXRef, eofPos)

	return &BasicInfo{
		Version:      version,
		FileSize:     int64(len(data)),
		PageCount:    pageCount,
		CountSource:  countSource,
		Title:        meta["Title"],
		Author:       meta["Author"],
		Subject:      meta["Subject"],
		Producer:     meta["Producer"],
		Creator:      meta["Creator"],
		Keywords:     meta["Keywords"],
		StartXRef:    startXRef,
		XRefType:     xrefType,
		HasEOFMarker: true,
		HasTrailer:   hasTrailer,
	}, nil
}

func detectPageCount(data []byte, xrefType string, startXRef, eofPos int) (int, string) {
	n, src := detectPageCountByRoot(data)
	if src == CountSourceRootChain || xrefType != "stream" {
		return n, src
	}
	rootObj, ok := parseRootObjectFromXRefStream(data, startXRef, eofPos)
	if !ok {
		return n, src
	}
	if cat, ok := parseCatalogObject(data, rootObj); ok && cat.hasPagesRef {
		if pages, ok := pagesObjectCount(data, cat.pagesObjNum); ok {
			return pages, CountSourceRootChain
		}
	}
	return n, src
}

func parseStartXRefValue(raw []byte) (int, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return 0, fmt.Errorf("empty startxref block")
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return 0, fmt.Errorf("empty startxref block")
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, err
	}
	return n, nil
}

func detectXRefType(data []byte, startXRef, eofPos int) (string, bool) {
	seg := bytes.TrimLeft(data[startXRef:eofPos], "\x00\t\r\n ")
	if bytes.HasPrefix(seg, []byte("xref")) {
		return "table", bytes.Contains(seg, []byte("trailer"))
	}
	if isXRefStream(data, startXRef, eofPos) {
		return "stream", bytes.Contains(seg, []byte("trailer"))
	}
	return "", false
}

func isXRefStream(data []byte, startXRef, eofPos int) bool {
	seg := data[startXRef:eofPos]
	window := seg
	if len(window) > 512 {
		window = window[:512]
	}
	// Common xref stream shape: object header + /Type /XRef + stream.
	return bytes.Contains(window, []byte(" obj")) &&
		bytes.Contains(window, []byte("/Type /XRef")) &&
		bytes.Contains(window, []byte("stream"))
}

func validateXRefStream(data []byte, startXRef, eofPos int, mode string) (*xrefStreamStats, error) {
	seg := data[startXRef:eofPos]
	trimmed := bytes.TrimLeft(seg, "\x00\t\r\n ")
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("invalid xref stream: empty section")
	}
	objNum, _, headerLen, ok := parseObjectHeaderPrefix(trimmed)
	if !ok {
		return nil, fmt.Errorf("invalid xref stream: missing object header")
	}
	body := trimmed[headerLen:]
	endObj := bytes.Index(body, []byte("endobj"))
	if endObj < 0 {
		return nil, fmt.Errorf("invalid xref stream: missing endobj")
	}
	objBlock := body[:endObj]
	if !bytes.Contains(objBlock, []byte("/Type /XRef")) {
		return nil, fmt.Errorf("invalid xref stream: missing /Type /XRef")
	}
	if !bytes.Contains(objBlock, []byte("stream")) || !bytes.Contains(objBlock, []byte("endstream")) {
		return nil, fmt.Errorf("invalid xref stream: missing stream block")
	}
	streamDataRaw, ok := parseObjectStreamData(objBlock)
	if !ok {
		return nil, fmt.Errorf("invalid xref stream: cannot read stream data")
	}
	streamData := decodeMaybeFlate(objBlock, streamDataRaw)
	if len(streamData) == 0 {
		return nil, fmt.Errorf("invalid xref stream: empty stream data")
	}

	fields := strings.Fields(string(objBlock))
	stats := &xrefStreamStats{objNumber: objNum}
	for i := 0; i < len(fields)-1; i++ {
		if fields[i] == "/Size" {
			n, err := strconv.Atoi(fields[i+1])
			if err == nil && n >= 0 {
				stats.hasSize = true
				stats.size = n
			}
		}
	}
	w, ok := parseXRefStreamW(objBlock)
	if !ok {
		return nil, fmt.Errorf("invalid xref stream: missing or invalid /W")
	}
	stats.w = w
	stats.entryWidth = w[0] + w[1] + w[2]
	if stats.entryWidth <= 0 {
		return nil, fmt.Errorf("invalid xref stream: /W total width must be > 0")
	}
	if len(streamData)%stats.entryWidth != 0 {
		return nil, fmt.Errorf("invalid xref stream: stream length %d not aligned with /W width %d", len(streamData), stats.entryWidth)
	}
	stats.entriesCount = len(streamData) / stats.entryWidth
	indexRanges, total, ok := parseXRefStreamIndex(objBlock, stats.size, stats.hasSize)
	if !ok {
		return nil, fmt.Errorf("invalid xref stream: invalid /Index")
	}
	stats.indexRanges = indexRanges
	stats.indexTotal = total
	if stats.indexTotal != stats.entriesCount {
		return nil, fmt.Errorf("invalid xref stream: index coverage %d != entries %d", stats.indexTotal, stats.entriesCount)
	}
	if mode == ValidationModeStrict {
		if err := validateXRefStreamIndexStrict(stats); err != nil {
			return nil, err
		}
	}
	entries, ok := parseXRefStreamEntries(streamData, stats.w, stats.indexRanges)
	if !ok {
		return nil, fmt.Errorf("invalid xref stream: unable to decode entries")
	}
	if err := validateXRefStreamEntries(data, entries, mode); err != nil {
		return nil, err
	}

	rootObj, hasRoot := parseRootRefFromFields(fields)
	stats.hasRoot = hasRoot
	if hasRoot {
		stats.rootObjNum = rootObj
	}
	if mode == ValidationModeStrict && !stats.hasSize {
		return nil, fmt.Errorf("strict mode: xref stream /Size is required")
	}
	return stats, nil
}

func validateXRefStreamStrictRules(data []byte, startXRef int, stats *xrefStreamStats) error {
	// Strict mode requires startxref to point exactly at the xref stream object header.
	if !matchesObjectHeaderAtExact(data, startXRef, stats.objNumber) {
		return fmt.Errorf("strict mode: startxref must point to exact xref stream object offset")
	}
	// Strict mode requires the xref stream dictionary to contain /Root pointing to Catalog.
	if !stats.hasRoot {
		return fmt.Errorf("strict mode: xref stream /Root is required")
	}
	cat, ok := parseCatalogObject(data, stats.rootObjNum)
	if !ok {
		return fmt.Errorf("strict mode: xref stream /Root obj=%d is missing or not /Type /Catalog", stats.rootObjNum)
	}
	if !cat.hasPagesRef {
		return fmt.Errorf("strict mode: catalog obj=%d missing /Pages reference", cat.objNumber)
	}
	if !objectHasTypePages(data, cat.pagesObjNum) {
		return fmt.Errorf("strict mode: catalog /Pages obj=%d missing or not /Type /Pages", cat.pagesObjNum)
	}
	return nil
}

func matchesObjectHeaderAtExact(data []byte, offset int, objNumber int) bool {
	if offset < 0 || offset >= len(data) {
		return false
	}
	seg := data[offset:]
	prefix := strconv.Itoa(objNumber) + " "
	if !bytes.HasPrefix(seg, []byte(prefix)) {
		return false
	}
	rest := seg[len(prefix):]
	genDigits := 0
	for genDigits < len(rest) && rest[genDigits] >= '0' && rest[genDigits] <= '9' {
		genDigits++
	}
	if genDigits == 0 || genDigits >= len(rest) {
		return false
	}
	rest = rest[genDigits:]
	if len(rest) == 0 || rest[0] != ' ' {
		return false
	}
	rest = rest[1:]
	return bytes.HasPrefix(rest, []byte("obj"))
}

func parseObjectHeaderPrefix(seg []byte) (objNum int, genNum int, consumed int, ok bool) {
	i := 0
	readNumber := func() (int, int, bool) {
		start := i
		for i < len(seg) && seg[i] >= '0' && seg[i] <= '9' {
			i++
		}
		if i == start {
			return 0, 0, false
		}
		n, err := strconv.Atoi(string(seg[start:i]))
		if err != nil {
			return 0, 0, false
		}
		return n, i - start, true
	}
	skipSpaces := func() {
		for i < len(seg) && (seg[i] == ' ' || seg[i] == '\t' || seg[i] == '\r' || seg[i] == '\n') {
			i++
		}
	}

	skipSpaces()
	obj, _, ok1 := readNumber()
	if !ok1 {
		return 0, 0, 0, false
	}
	skipSpaces()
	gen, _, ok2 := readNumber()
	if !ok2 {
		return 0, 0, 0, false
	}
	skipSpaces()
	if i+3 > len(seg) || !bytes.Equal(seg[i:i+3], []byte("obj")) {
		return 0, 0, 0, false
	}
	i += 3
	return obj, gen, i, true
}

func parseRootRefFromFields(fields []string) (int, bool) {
	for i := 0; i < len(fields)-3; i++ {
		if fields[i] != "/Root" {
			continue
		}
		objNum, err1 := strconv.Atoi(fields[i+1])
		genNum, err2 := strconv.Atoi(fields[i+2])
		if err1 != nil || err2 != nil || objNum <= 0 || genNum < 0 || fields[i+3] != "R" {
			return 0, false
		}
		return objNum, true
	}
	return 0, false
}

func parseXRefStreamW(objBlock []byte) ([3]int, bool) {
	var out [3]int
	pos := bytes.Index(objBlock, []byte("/W"))
	if pos < 0 {
		return out, false
	}
	rest := objBlock[pos+len("/W"):]
	lb := bytes.IndexByte(rest, '[')
	if lb < 0 {
		return out, false
	}
	rest = rest[lb+1:]
	rb := bytes.IndexByte(rest, ']')
	if rb < 0 {
		return out, false
	}
	nums := strings.Fields(string(rest[:rb]))
	if len(nums) < 3 {
		return out, false
	}
	a, err1 := strconv.Atoi(nums[0])
	b, err2 := strconv.Atoi(nums[1])
	c, err3 := strconv.Atoi(nums[2])
	if err1 != nil || err2 != nil || err3 != nil || a < 0 || b < 0 || c < 0 {
		return out, false
	}
	out[0], out[1], out[2] = a, b, c
	return out, true
}

func parseXRefStreamIndex(objBlock []byte, size int, hasSize bool) ([]xrefIndexRange, int, bool) {
	// Default semantics: missing /Index is equivalent to [0 /Size].
	pos := bytes.Index(objBlock, []byte("/Index"))
	if pos < 0 {
		if !hasSize || size < 0 {
			return nil, 0, false
		}
		return []xrefIndexRange{{start: 0, count: size}}, size, true
	}
	rest := objBlock[pos+len("/Index"):]
	lb := bytes.IndexByte(rest, '[')
	if lb < 0 {
		return nil, 0, false
	}
	rest = rest[lb+1:]
	rb := bytes.IndexByte(rest, ']')
	if rb < 0 {
		return nil, 0, false
	}
	nums := strings.Fields(string(rest[:rb]))
	if len(nums) == 0 || len(nums)%2 != 0 {
		return nil, 0, false
	}
	ranges := make([]xrefIndexRange, 0, len(nums)/2)
	total := 0
	for i := 0; i < len(nums); i += 2 {
		start, err1 := strconv.Atoi(nums[i])
		count, err2 := strconv.Atoi(nums[i+1])
		if err1 != nil || err2 != nil || start < 0 || count < 0 {
			return nil, 0, false
		}
		ranges = append(ranges, xrefIndexRange{start: start, count: count})
		total += count
	}
	return ranges, total, true
}

func validateXRefStreamIndexStrict(stats *xrefStreamStats) error {
	if !stats.hasSize {
		return fmt.Errorf("strict mode: xref stream /Size is required")
	}
	prevEnd := -1
	for _, r := range stats.indexRanges {
		if r.start < 0 || r.count < 0 {
			return fmt.Errorf("strict mode: xref stream /Index contains negative range")
		}
		end := r.start + r.count
		if end < r.start {
			return fmt.Errorf("strict mode: xref stream /Index range overflow")
		}
		if end > stats.size {
			return fmt.Errorf("strict mode: xref stream /Index range [%d,%d) exceeds /Size %d", r.start, end, stats.size)
		}
		if prevEnd > r.start {
			return fmt.Errorf("strict mode: xref stream /Index ranges overlap or are unsorted")
		}
		prevEnd = end
	}
	return nil
}

func parseXRefStreamEntries(streamData []byte, w [3]int, ranges []xrefIndexRange) ([]xrefStreamEntry, bool) {
	width := w[0] + w[1] + w[2]
	if width <= 0 || len(streamData)%width != 0 {
		return nil, false
	}
	total := len(streamData) / width
	out := make([]xrefStreamEntry, 0, total)
	pos := 0
	for _, r := range ranges {
		for i := 0; i < r.count; i++ {
			if pos+width > len(streamData) {
				return nil, false
			}
			b := streamData[pos : pos+width]
			pos += width
			t := decodeXRefFieldValue(b[:w[0]])
			// PDF spec: when w0=0, type defaults to 1.
			if w[0] == 0 {
				t = 1
			}
			f2 := decodeXRefFieldValue(b[w[0] : w[0]+w[1]])
			f3 := decodeXRefFieldValue(b[w[0]+w[1] : w[0]+w[1]+w[2]])
			out = append(out, xrefStreamEntry{
				objNum: r.start + i,
				typ:    t,
				f2:     f2,
				f3:     f3,
			})
		}
	}
	return out, pos == len(streamData)
}

func decodeXRefFieldValue(b []byte) int {
	v := 0
	for _, x := range b {
		v = (v << 8) | int(x)
	}
	return v
}

func validateXRefStreamEntries(data []byte, entries []xrefStreamEntry, mode string) error {
	for _, e := range entries {
		switch e.typ {
		case 0, 1, 2:
			// ok
		default:
			return fmt.Errorf("invalid xref stream: entry obj=%d has invalid type=%d", e.objNum, e.typ)
		}
		if mode == ValidationModeStrict {
			// Strict mode: free entry(type=0) f2 is next free object number; f3 is generation.
			if e.typ == 0 {
				if e.f2 < 0 || e.f3 < 0 || e.f3 > 65535 {
					return fmt.Errorf("strict mode: xref stream entry obj=%d type=0 has invalid next/gen", e.objNum)
				}
			}
			// Strict mode: normal in-use objects(type=1) need nonzero offsets except obj0.
			if e.typ == 1 && e.objNum > 0 && e.f2 == 0 {
				return fmt.Errorf("strict mode: xref stream entry obj=%d type=1 has zero offset", e.objNum)
			}
			// Strict mode: type=1 offsets must locate the matching object header.
			if e.typ == 1 && e.objNum > 0 && !matchesObjectHeaderAt(data, e.f2, e.objNum) {
				return fmt.Errorf("strict mode: xref stream entry obj=%d type=1 points to invalid offset %d", e.objNum, e.f2)
			}
			// Strict mode: compressed objects(type=2) must point to a valid ObjStm and in-range index.
			if e.typ == 2 {
				if e.f2 <= 0 || e.f3 < 0 {
					return fmt.Errorf("strict mode: xref stream entry obj=%d type=2 has invalid objstm/index", e.objNum)
				}
				objStmBlock, ok := findDirectObjectBlockByNumber(data, e.f2)
				if !ok || pdfTypeMarkerIndex(objStmBlock, "ObjStm") < 0 {
					return fmt.Errorf("strict mode: xref stream entry obj=%d type=2 points to invalid objstm=%d", e.objNum, e.f2)
				}
				first, n, ok := parseObjStmFirstAndN(objStmBlock)
				if !ok || e.f3 >= n {
					return fmt.Errorf("strict mode: xref stream entry obj=%d type=2 index=%d out of objstm n=%d", e.objNum, e.f3, n)
				}
				decoded, ok := decodeObjStmPayloadStrict(objStmBlock)
				if !ok || len(decoded) <= first {
					return fmt.Errorf("strict mode: xref stream entry obj=%d type=2 cannot decode objstm payload", e.objNum)
				}
				entries, ok := parseObjStmIndexEntries(decoded[:first], n)
				if !ok || e.f3 >= len(entries) {
					return fmt.Errorf("strict mode: xref stream entry obj=%d type=2 cannot read objstm index entry", e.objNum)
				}
				if entries[e.f3].obj != e.objNum {
					return fmt.Errorf("strict mode: xref stream entry obj=%d type=2 index=%d points to obj=%d", e.objNum, e.f3, entries[e.f3].obj)
				}
			}
		}
	}
	return nil
}

func parseRootObjectFromXRefStream(data []byte, startXRef, eofPos int) (int, bool) {
	seg := bytes.TrimLeft(data[startXRef:eofPos], "\x00\t\r\n ")
	if len(seg) == 0 {
		return 0, false
	}
	_, _, headerLen, ok := parseObjectHeaderPrefix(seg)
	if !ok {
		return 0, false
	}
	body := seg[headerLen:]
	endObj := bytes.Index(body, []byte("endobj"))
	if endObj < 0 {
		return 0, false
	}
	objBlock := body[:endObj]
	fields := strings.Fields(string(objBlock))
	return parseRootRefFromFields(fields)
}

func isVersionToken(v string) bool {
	return len(v) == 3 && v[1] == '.' && v[0] >= '0' && v[0] <= '9' && v[2] >= '0' && v[2] <= '9'
}

func validateXRefTable(data []byte, startXRef, eofPos int, mode string) (*xrefTableStats, error) {
	lines := splitXRefRegionNonEmptyLines(data[startXRef:eofPos])
	if len(lines) < 3 {
		return nil, fmt.Errorf("invalid xref table: too few lines")
	}
	if strings.TrimSpace(lines[0]) != "xref" {
		return nil, fmt.Errorf("invalid xref table: missing xref marker")
	}
	stats := &xrefTableStats{firstStartObj: -1}

	i := 1
	for i < len(lines) {
		cur := strings.TrimSpace(lines[i])
		if cur == "trailer" {
			size, ok := parseTrailerSize(lines, i+1)
			stats.hasTrailerSize = ok
			if ok {
				stats.trailerSize = size
			}
			rootObj, hasRoot := parseTrailerRootObject(lines, i+1)
			stats.hasRoot = hasRoot
			if hasRoot {
				stats.rootObjNumber = rootObj
			}
			return stats, nil
		}

		startObj, count, ok := parseXRefSubsectionHeader(cur)
		if !ok {
			return nil, fmt.Errorf("invalid xref subsection header: %q", lines[i])
		}
		if stats.firstStartObj < 0 {
			stats.firstStartObj = startObj
		}
		if v := startObj + count; v > stats.maxObjPlusOne {
			stats.maxObjPlusOne = v
		}
		i++
		if i+count > len(lines) {
			return nil, fmt.Errorf("invalid xref table: subsection entry count overflow")
		}
		for j := 0; j < count; j++ {
			entryLine := lines[i+j]
			if !isValidXRefEntryLine(entryLine, mode) {
				return nil, fmt.Errorf("invalid xref entry: %q", lines[i+j])
			}
			if mode == ValidationModeStrict {
				off, inUse, ok := parseXRefEntryOffsetAndUsage(entryLine)
				if !ok {
					return nil, fmt.Errorf("invalid xref entry payload: %q", lines[i+j])
				}
				if inUse {
					stats.inUseEntries = append(stats.inUseEntries, xrefInUseEntry{
						objNumber: startObj + j,
						offset:    off,
					})
				}
			}
		}
		i += count
	}
	return nil, fmt.Errorf("invalid xref table: missing trailer marker")
}

func splitNonEmptyRawLines(b []byte) []string {
	// PDF text may mix \n, \r\n, and bare \r; splitting only on \n can join trailer\rstartxref.
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	raw := strings.Split(s, "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

// splitXRefRegionNonEmptyLines splits the xref-to-EOF text region and expands
// compact forms such as trailer<<...>>.
func splitXRefRegionNonEmptyLines(b []byte) []string {
	return expandMergedTrailerLines(splitNonEmptyRawLines(b))
}

func expandMergedTrailerLines(lines []string) []string {
	out := make([]string, 0, len(lines)+2)
	for _, line := range lines {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "trailer") && len(s) > len("trailer") {
			rest := strings.TrimSpace(s[len("trailer"):])
			if strings.HasPrefix(rest, "<<") {
				out = append(out, "trailer", rest)
				continue
			}
		}
		out = append(out, line)
	}
	return out
}

func parseXRefSubsectionHeader(line string) (int, int, bool) {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		return 0, 0, false
	}
	startObj, err := strconv.Atoi(fields[0])
	if err != nil || startObj < 0 {
		return 0, 0, false
	}
	count, err := strconv.Atoi(fields[1])
	if err != nil || count <= 0 {
		return 0, 0, false
	}
	return startObj, count, true
}

func isValidXRefEntryLine(line string, mode string) bool {
	if mode == ValidationModeStrict {
		return isValidXRefEntryLineStrict(line)
	}
	return isValidXRefEntryLineRelaxed(line)
}

func isValidXRefEntryLineRelaxed(line string) bool {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 3 {
		return false
	}
	if len(fields[0]) != 10 || len(fields[1]) != 5 {
		return false
	}
	if _, err := strconv.Atoi(fields[0]); err != nil {
		return false
	}
	if _, err := strconv.Atoi(fields[1]); err != nil {
		return false
	}
	return fields[2] == "n" || fields[2] == "f"
}

func isValidXRefEntryLineStrict(line string) bool {
	s := strings.TrimRight(line, " ")
	if len(s) != 18 {
		return false
	}
	if s[10] != ' ' || s[16] != ' ' {
		return false
	}
	for i := 0; i < 10; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	for i := 11; i < 16; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s[17] == 'n' || s[17] == 'f'
}

func normalizeValidationMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ValidationModeStrict:
		return ValidationModeStrict
	default:
		return ValidationModeRelaxed
	}
}

func validateXRefTableStrictRules(data []byte, startXRef int, stats *xrefTableStats) error {
	// Strict mode requires startxref to point exactly at "xref".
	if !bytes.HasPrefix(data[startXRef:], []byte("xref")) {
		return fmt.Errorf("strict mode: startxref must point to exact xref offset")
	}
	// Strict mode requires the first subsection to start at 0.
	if stats.firstStartObj != 0 {
		return fmt.Errorf("strict mode: first xref subsection must start at object 0")
	}
	// Strict mode requires trailer /Size and it must cover the xref range.
	if !stats.hasTrailerSize {
		return fmt.Errorf("strict mode: trailer /Size is required")
	}
	if stats.trailerSize < stats.maxObjPlusOne {
		return fmt.Errorf("strict mode: trailer /Size (%d) smaller than xref coverage (%d)", stats.trailerSize, stats.maxObjPlusOne)
	}
	// Strict mode requires trailer /Root to locate a Catalog object.
	if !stats.hasRoot {
		return fmt.Errorf("strict mode: trailer /Root is required")
	}
	cat, ok := parseCatalogObject(data, stats.rootObjNumber)
	if !ok {
		return fmt.Errorf("strict mode: trailer /Root obj=%d is missing or not /Type /Catalog", stats.rootObjNumber)
	}
	// Strict mode requires Catalog to reference /Pages and target a /Pages object.
	if !cat.hasPagesRef {
		return fmt.Errorf("strict mode: catalog obj=%d missing /Pages reference", cat.objNumber)
	}
	if !objectHasTypePages(data, cat.pagesObjNum) {
		return fmt.Errorf("strict mode: catalog /Pages obj=%d missing or not /Type /Pages", cat.pagesObjNum)
	}
	// Strict mode requires Catalog Pages /Count to match detected page count.
	catalogPagesCount, ok := pagesObjectCount(data, cat.pagesObjNum)
	if !ok {
		return fmt.Errorf("strict mode: catalog /Pages obj=%d missing valid /Count", cat.pagesObjNum)
	}
	detectedCount, _ := detectPageCountByRoot(data)
	if catalogPagesCount != detectedCount {
		return fmt.Errorf("strict mode: page count mismatch catalog=%d detected=%d", catalogPagesCount, detectedCount)
	}
	// Strict mode requires in-use xref offsets to locate matching object headers.
	for _, e := range stats.inUseEntries {
		if !matchesObjectHeaderAt(data, e.offset, e.objNumber) {
			return fmt.Errorf("strict mode: xref in-use entry obj=%d points to invalid offset %d", e.objNumber, e.offset)
		}
	}
	return nil
}

func parseXRefEntryOffsetAndUsage(line string) (int, bool, bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 3 {
		return 0, false, false
	}
	off, err := strconv.Atoi(fields[0])
	if err != nil || off < 0 {
		return 0, false, false
	}
	switch fields[2] {
	case "n":
		return off, true, true
	case "f":
		return off, false, true
	default:
		return 0, false, false
	}
}

func matchesObjectHeaderAt(data []byte, offset int, objNumber int) bool {
	if offset < 0 || offset >= len(data) {
		return false
	}
	seg := data[offset:]
	// Allow small whitespace before object headers.
	seg = bytes.TrimLeft(seg, "\x00\t\r\n ")
	// Shape: "<objNumber> <gen> obj".
	prefix := strconv.Itoa(objNumber) + " "
	if !bytes.HasPrefix(seg, []byte(prefix)) {
		return false
	}
	rest := seg[len(prefix):]
	genDigits := 0
	for genDigits < len(rest) && rest[genDigits] >= '0' && rest[genDigits] <= '9' {
		genDigits++
	}
	if genDigits == 0 || genDigits >= len(rest) {
		return false
	}
	rest = rest[genDigits:]
	rest = bytes.TrimLeft(rest, " ")
	return bytes.HasPrefix(rest, []byte("obj"))
}

func parseTrailerSize(lines []string, start int) (int, bool) {
	if start >= len(lines) {
		return 0, false
	}
	var b strings.Builder
	for i := start; i < len(lines); i++ {
		cur := strings.TrimSpace(lines[i])
		if cur == "startxref" {
			break
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(cur)
	}
	fields := strings.Fields(b.String())
	for i := 0; i < len(fields)-1; i++ {
		if fields[i] != "/Size" {
			continue
		}
		n, err := strconv.Atoi(fields[i+1])
		if err != nil || n < 0 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

func parseTrailerRootObject(lines []string, start int) (int, bool) {
	fields := collectTrailerFields(lines, start)
	return parseRootRefFromFields(fields)
}

func collectTrailerFields(lines []string, start int) []string {
	if start >= len(lines) {
		return nil
	}
	var b strings.Builder
	for i := start; i < len(lines); i++ {
		cur := strings.TrimSpace(lines[i])
		if cur == "startxref" {
			break
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(cur)
	}
	return strings.Fields(b.String())
}

func objectIsCatalog(data []byte, objNum int) bool {
	if objNum <= 0 {
		return false
	}
	objBlock, ok := findObjectBlockByNumber(data, objNum)
	if !ok {
		return false
	}
	return objectBlockHasCatalogType(objBlock)
}

func objectHasTypePages(data []byte, objNum int) bool {
	if objNum <= 0 {
		return false
	}
	objBlock, ok := findObjectBlockByNumber(data, objNum)
	if !ok {
		return false
	}
	return objectBlockHasPagesType(objBlock)
}

func parseCatalogObject(data []byte, objNum int) (*catalogInfo, bool) {
	if objNum <= 0 {
		return nil, false
	}
	objBlock, ok := findObjectBlockByNumber(data, objNum)
	if !ok {
		return nil, false
	}
	if !objectBlockHasCatalogType(objBlock) {
		return nil, false
	}
	pagesObj, hasPages := parsePagesRefFromCatalog(objBlock)
	return &catalogInfo{
		objNumber:   objNum,
		pagesObjNum: pagesObj,
		hasPagesRef: hasPages,
	}, true
}

func parsePagesRefFromCatalog(objBlock []byte) (int, bool) {
	s := string(objBlock)
	fields := strings.Fields(s)
	for i := 0; i < len(fields)-3; i++ {
		if fields[i] != "/Pages" {
			continue
		}
		objNum, err1 := strconv.Atoi(fields[i+1])
		genNum, err2 := strconv.Atoi(fields[i+2])
		if err1 != nil || err2 != nil || objNum <= 0 || genNum < 0 || fields[i+3] != "R" {
			continue
		}
		return objNum, true
	}
	if m := pagesRefDirectRE.FindStringSubmatch(s); len(m) >= 3 {
		objNum, err1 := strconv.Atoi(m[1])
		genNum, err2 := strconv.Atoi(m[2])
		if err1 == nil && err2 == nil && objNum > 0 && genNum >= 0 {
			return objNum, true
		}
	}
	if m := pagesRefBracketRE.FindStringSubmatch(s); len(m) >= 3 {
		objNum, err1 := strconv.Atoi(m[1])
		genNum, err2 := strconv.Atoi(m[2])
		if err1 == nil && err2 == nil && objNum > 0 && genNum >= 0 {
			return objNum, true
		}
	}
	return 0, false
}

func pagesObjectCount(data []byte, objNum int) (int, bool) {
	objBlock, ok := findObjectBlockByNumber(data, objNum)
	if !ok {
		return 0, false
	}
	if !objectBlockHasPagesType(objBlock) {
		return 0, false
	}
	n := extractCountFromObject(objBlock)
	if n < 0 {
		return 0, false
	}
	return n, true
}

func findObjectBlockByNumber(data []byte, objNum int) ([]byte, bool) {
	if obj, ok := findDirectObjectBlockByNumber(data, objNum); ok {
		return obj, true
	}
	// Fallback: extract compressed objects from /Type /ObjStm object streams.
	return findObjectFromObjectStreams(data, objNum)
}

func findDirectObjectBlockByNumber(data []byte, objNum int) ([]byte, bool) {
	if idx := lookupPDFObjectIndex(data); idx != nil {
		return idx.blockByNumber(data, objNum)
	}
	return findDirectObjectBlockByNumberLinear(data, objNum)
}

func findDirectObjectBlockByNumberLinear(data []byte, objNum int) ([]byte, bool) {
	b, _, ok := findDirectObjectBlockByNumberLinearOffset(data, objNum)
	return b, ok
}

// findDirectObjectBlockByNumberLinearOffset is like Linear but also returns
// the object header offset for index construction.
func findDirectObjectBlockByNumberLinearOffset(data []byte, objNum int) ([]byte, int, bool) {
	needle := []byte(strconv.Itoa(objNum) + " ")
	cursor := 0
	for {
		rel := bytes.Index(data[cursor:], needle)
		if rel < 0 {
			return nil, 0, false
		}
		pos := cursor + rel
		// Avoid matching "12 0 obj" inside "312 0 obj": object number must start a digit token.
		if pos > 0 {
			prev := data[pos-1]
			if prev >= '0' && prev <= '9' {
				cursor = pos + len(needle)
				continue
			}
		}
		rest := data[pos+len(needle):]
		genDigits := 0
		for genDigits < len(rest) && rest[genDigits] >= '0' && rest[genDigits] <= '9' {
			genDigits++
		}
		if genDigits == 0 {
			cursor = pos + len(needle)
			continue
		}
		rest = rest[genDigits:]
		rest = bytes.TrimLeft(rest, " ")
		if !bytes.HasPrefix(rest, []byte("obj")) {
			cursor = pos + len(needle)
			continue
		}
		start := pos
		endRel := bytes.Index(data[start:], []byte("endobj"))
		if endRel < 0 {
			return nil, 0, false
		}
		end := start + endRel + len("endobj")
		return data[start:end], start, true
	}
}

func findObjectFromObjectStreams(data []byte, objNum int) ([]byte, bool) {
	// Do not scan the whole file for /Type/ObjStm because binary streams can contain the same bytes.
	if idx := lookupPDFObjectIndex(data); idx != nil {
		for _, n := range idx.objStmObjectNumbers(data) {
			objBlock, ok := idx.blockByNumber(data, n)
			if !ok {
				continue
			}
			content, ok := extractObjectFromObjStmBlock(objBlock, objNum)
			if ok {
				out := []byte(strconv.Itoa(objNum) + " 0 obj\n" + string(content) + "\nendobj")
				return out, true
			}
		}
		return nil, false
	}
	upper := detectNextObjectNumberByScan(data)
	if upper < 2 {
		upper = 8192
	}
	if upper > 200000 {
		upper = 200000
	}
	for n := 1; n < upper; n++ {
		objBlock, ok := findDirectObjectBlockByNumberLinear(data, n)
		if !ok || pdfTypeMarkerIndex(objBlock, "ObjStm") < 0 {
			continue
		}
		content, ok := extractObjectFromObjStmBlock(objBlock, objNum)
		if ok {
			out := []byte(strconv.Itoa(objNum) + " 0 obj\n" + string(content) + "\nendobj")
			return out, true
		}
	}
	return nil, false
}

func extractObjectFromObjStmBlock(objBlock []byte, targetObj int) ([]byte, bool) {
	if pdfTypeMarkerIndex(objBlock, "ObjStm") < 0 {
		return nil, false
	}
	first, n, ok := parseObjStmFirstAndN(objBlock)
	if !ok || first < 0 || n <= 0 {
		return nil, false
	}
	decoded, ok := decodeObjStmPayload(objBlock)
	if len(decoded) <= first {
		return nil, false
	}
	entries, ok := parseObjStmIndexEntries(decoded[:first], n)
	if !ok {
		return nil, false
	}
	for i := 0; i < len(entries); i++ {
		if entries[i].obj != targetObj {
			continue
		}
		start := first + entries[i].off
		end := len(decoded)
		if i+1 < len(entries) {
			end = first + entries[i+1].off
		}
		if start < 0 || start >= len(decoded) || end <= start || end > len(decoded) {
			return nil, false
		}
		return bytes.TrimSpace(decoded[start:end]), true
	}
	return nil, false
}

type objStmIndexEntry struct {
	obj int
	off int
}

func decodeObjStmPayload(objBlock []byte) ([]byte, bool) {
	streamData, ok := parseObjectStreamData(objBlock)
	if !ok {
		return nil, false
	}
	decoded := decodeObjStmStream(objBlock, streamData)
	if len(decoded) == 0 {
		return nil, false
	}
	return decoded, true
}

func decodeObjStmPayloadStrict(objBlock []byte) ([]byte, bool) {
	streamData, ok := parseObjectStreamData(objBlock)
	if !ok {
		return nil, false
	}
	decoded, ok := decodeMaybeFlateStrict(objBlock, streamData)
	if !ok || len(decoded) == 0 {
		return nil, false
	}
	return decoded, true
}

func parseObjStmIndexEntries(indexHeader []byte, n int) ([]objStmIndexEntry, bool) {
	pairs := strings.Fields(string(indexHeader))
	if len(pairs) < n*2 {
		return nil, false
	}
	entries := make([]objStmIndexEntry, 0, n)
	for i := 0; i < n; i++ {
		objID, err1 := strconv.Atoi(pairs[i*2])
		off, err2 := strconv.Atoi(pairs[i*2+1])
		if err1 != nil || err2 != nil || objID <= 0 || off < 0 {
			return nil, false
		}
		entries = append(entries, objStmIndexEntry{obj: objID, off: off})
	}
	return entries, true
}

func parseObjStmFirstAndN(objBlock []byte) (int, int, bool) {
	// Compact dictionary support: `/Filter/FlateDecode/First 144/...`.
	first := parseIntAfterPDFNameKey(objBlock, []byte("/First"))
	n := parseIntAfterPDFNameKey(objBlock, []byte("/N"))
	if first < 0 || n <= 0 {
		return 0, 0, false
	}
	return first, n, true
}

func parseIntAfterPDFNameKey(dict []byte, key []byte) int {
	search := 0
	for {
		i := bytes.Index(dict[search:], key)
		if i < 0 {
			return -1
		}
		i += search
		rest := dict[i+len(key):]
		if len(rest) > 0 && (rest[0] >= 'A' && rest[0] <= 'Z' || rest[0] >= 'a' && rest[0] <= 'z') {
			search = i + 1
			continue
		}
		rest = bytes.TrimLeft(rest, " \t\r\n")
		var num []byte
		for j := 0; j < len(rest); j++ {
			c := rest[j]
			if c < '0' || c > '9' {
				break
			}
			num = append(num, c)
		}
		if len(num) == 0 {
			search = i + 1
			continue
		}
		v, err := strconv.Atoi(string(num))
		if err != nil || v < 0 {
			search = i + 1
			continue
		}
		return v
	}
}

// ParseObjectStreamBytes extracts raw bytes between stream and endstream without filter decoding.
func ParseObjectStreamBytes(objBlock []byte) ([]byte, bool) {
	return parseObjectStreamData(objBlock)
}

func parseObjectStreamData(objBlock []byte) ([]byte, bool) {
	streamPos := bytes.Index(objBlock, []byte("stream"))
	if streamPos < 0 {
		return nil, false
	}
	start := streamPos + len("stream")
	// PDF allows stream to be followed by \n or \r\n.
	if start < len(objBlock) && objBlock[start] == '\r' {
		start++
	}
	if start < len(objBlock) && objBlock[start] == '\n' {
		start++
	}
	if l, ok := parseStreamLengthFromDict(objBlock); ok {
		if start+l > len(objBlock) || l < 0 {
			return nil, false
		}
		return objBlock[start : start+l], true
	}
	end := bytes.Index(objBlock[start:], []byte("endstream"))
	if end < 0 {
		return nil, false
	}
	return objBlock[start : start+end], true
}

func parseStreamLengthFromDict(objBlock []byte) (int, bool) {
	// Compact dictionary support: `/Length 1126` and `/Filter/FlateDecode/Length 1126/...`.
	search := 0
	for {
		i := bytes.Index(objBlock[search:], []byte("/Length"))
		if i < 0 {
			break
		}
		i += search
		after := i + len("/Length")
		if after < len(objBlock) {
			c := objBlock[after]
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
				search = i + 1
				continue
			}
		}
		rest := bytes.TrimLeft(objBlock[after:], " \t\r\n")
		var num []byte
		for j := 0; j < len(rest) && rest[j] >= '0' && rest[j] <= '9'; j++ {
			num = append(num, rest[j])
		}
		if len(num) > 0 {
			n, err := strconv.Atoi(string(num))
			if err == nil && n >= 0 {
				return n, true
			}
		}
		search = i + 1
	}
	fields := strings.Fields(string(objBlock))
	for i := 0; i < len(fields)-1; i++ {
		if fields[i] != "/Length" {
			continue
		}
		n, err := strconv.Atoi(fields[i+1])
		if err != nil || n < 0 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

func decodeObjStmStream(objBlock []byte, raw []byte) []byte {
	return decodeMaybeFlate(objBlock, raw)
}

func decodeMaybeFlate(objBlock []byte, raw []byte) []byte {
	if !bytes.Contains(objBlock, []byte("/FlateDecode")) {
		return raw
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return raw
	}
	defer zr.Close()
	b, err := io.ReadAll(zr)
	if err != nil {
		return raw
	}
	return b
}

func decodeMaybeFlateStrict(objBlock []byte, raw []byte) ([]byte, bool) {
	if !bytes.Contains(objBlock, []byte("/FlateDecode")) {
		return raw, true
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, false
	}
	defer zr.Close()
	b, err := io.ReadAll(zr)
	if err != nil {
		return nil, false
	}
	return b, true
}

func detectPageCountByRoot(data []byte) (int, string) {
	// Prefer the Root->Catalog->Pages chain to avoid unrelated /Pages objects.
	if rootObj, ok := parseRootObjectFromTrailer(data); ok {
		if cat, ok := parseCatalogObject(data, rootObj); ok && cat.hasPagesRef {
			if n, ok := pagesObjectCount(data, cat.pagesObjNum); ok {
				return n, CountSourceRootChain
			}
		}
	}
	// Fall back to loose scanning for incomplete structures.
	return detectPageCountByScan(data), CountSourceScanFallback
}

// DetectPageObjectNumbers parses the page tree and returns leaf /Page object numbers.
func DetectPageObjectNumbers(path string, mode string) ([]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DetectPageObjectNumbersBytes(data, mode)
}

// DetectPageInfos parses leaf page-tree data: object number, MediaBox, and optional boxes.
func DetectPageInfos(path string, mode string) ([]PageInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DetectPageInfosBytes(data, mode)
}

// DetectPageInfosBytes supports upload/in-memory paths.
func DetectPageInfosBytes(data []byte, mode string) ([]PageInfo, error) {
	rootObj, ok := detectRootObjectNumber(data)
	if !ok {
		if mode == ValidationModeRelaxed {
			return detectPageInfosRelaxedFallback(data)
		}
		return nil, fmt.Errorf("cannot detect root object")
	}
	cat, ok := parseCatalogObject(data, rootObj)
	if !ok || !cat.hasPagesRef {
		if mode == ValidationModeRelaxed {
			return detectPageInfosRelaxedFallback(data)
		}
		return nil, fmt.Errorf("invalid catalog/pages chain")
	}
	visited := map[int]bool{}
	out := make([]PageInfo, 0, 16)
	if err := collectPageLeafInfos(data, cat.pagesObjNum, visited, &out, inheritedPageBoxes{}); err != nil {
		if mode == ValidationModeRelaxed {
			if fb, e2 := detectPageInfosRelaxedFallback(data); e2 == nil && len(fb) > 0 {
				return fb, nil
			}
		}
		return nil, err
	}
	if len(out) == 0 {
		if mode == ValidationModeRelaxed {
			if fb, e2 := detectPageInfosRelaxedFallback(data); e2 == nil && len(fb) > 0 {
				return fb, nil
			}
		}
		return nil, fmt.Errorf("no page leaf objects found")
	}
	return out, nil
}

func detectPageInfosRelaxedFallback(data []byte) ([]PageInfo, error) {
	var best []PageInfo
	seenRoots := map[int]bool{}
	for _, pagesRoot := range discoverCatalogPagesRoots(data) {
		if seenRoots[pagesRoot] {
			continue
		}
		seenRoots[pagesRoot] = true
		visited := map[int]bool{}
		var out []PageInfo
		if err := collectPageLeafInfos(data, pagesRoot, visited, &out, inheritedPageBoxes{}); err != nil {
			continue
		}
		if len(out) > len(best) {
			best = out
		}
	}
	if len(best) > 0 {
		return best, nil
	}
	if out, ok := pageInfosByScanningPageObjects(data); ok && len(out) > 0 {
		return out, nil
	}
	return nil, fmt.Errorf("invalid catalog/pages chain")
}

func discoverCatalogPagesRoots(data []byte) []int {
	var roots []int
	seen := map[int]bool{}
	cursor := 0
	for {
		rel := pdfTypeMarkerIndex(data[cursor:], "Catalog")
		if rel < 0 {
			break
		}
		pos := cursor + rel
		objNum, ok := parseObjectNumberContainingBytePos(data, pos)
		if !ok {
			cursor = pos + 1
			continue
		}
		objBlock, ok := findObjectBlockByNumber(data, objNum)
		if !ok || !objectBlockHasCatalogType(objBlock) {
			cursor = pos + 1
			continue
		}
		pagesObj, ok := parsePagesRefFromCatalog(objBlock)
		if !ok || pagesObj <= 0 {
			cursor = pos + 1
			continue
		}
		if !seen[pagesObj] {
			seen[pagesObj] = true
			roots = append(roots, pagesObj)
		}
		cursor = pos + 1
	}
	return roots
}

// parseObjectNumberContainingBytePos finds the indirect object containing a byte offset.
func parseObjectNumberContainingBytePos(data []byte, pos int) (int, bool) {
	if pos <= 0 || pos > len(data) {
		return 0, false
	}
	searchEnd := pos
	for attempt := 0; attempt < 24; attempt++ {
		objWord := bytes.LastIndex(data[:searchEnd], []byte(" obj"))
		if objWord < 0 {
			return 0, false
		}
		lineStart := bytes.LastIndex(data[:objWord], []byte("\n"))
		if lineStart < 0 {
			lineStart = 0
		} else {
			lineStart++
		}
		header := strings.Fields(string(data[lineStart : objWord+len(" obj")]))
		if len(header) < 3 || header[len(header)-1] != "obj" {
			searchEnd = objWord
			continue
		}
		n, e1 := strconv.Atoi(header[len(header)-3])
		g, e2 := strconv.Atoi(header[len(header)-2])
		if e1 != nil || e2 != nil || n <= 0 || g < 0 {
			searchEnd = objWord
			continue
		}
		blk, ok := findObjectBlockByNumber(data, n)
		if !ok {
			searchEnd = objWord
			continue
		}
		idx := bytes.Index(data, blk)
		if idx < 0 {
			return n, true
		}
		if pos >= idx && pos < idx+len(blk) {
			return n, true
		}
		searchEnd = objWord
	}
	return 0, false
}

func pageInfosByScanningPageObjects(data []byte) ([]PageInfo, bool) {
	var out []PageInfo
	seen := map[int]bool{}
	cursor := 0
	for {
		rel := pdfTypeMarkerIndex(data[cursor:], "Page")
		if rel < 0 {
			break
		}
		pos := cursor + rel
		objNum, ok := parseObjectNumberContainingBytePos(data, pos)
		if !ok || seen[objNum] {
			cursor = pos + 1
			continue
		}
		blk, ok := findObjectBlockByNumber(data, objNum)
		if !ok || !objectBlockHasPageType(blk) {
			cursor = pos + 1
			continue
		}
		seen[objNum] = true
		pi := PageInfo{
			ObjectNumber: objNum,
			MediaBox:     parseMediaBoxString(data, blk),
		}
		fillPageInfoOptionalBoxes(data, &pi, blk)
		out = append(out, pi)
		cursor = pos + 1
	}
	return out, len(out) > 0
}

// DetectPageRenderSpecs parses basic references needed to write each page back.
func DetectPageRenderSpecs(path string, mode string) ([]PageRenderSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DetectPageRenderSpecsBytes(data, mode)
}

// ExtractObjectBlockByNumber returns the full object block, including obj/endobj.
func ExtractObjectBlockByNumber(path string, objNum int, mode string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return ExtractObjectBlockByNumberBytes(data, objNum, mode)
}

// ExtractObjectBlockByNumberBytes supports upload/in-memory paths.
func ExtractObjectBlockByNumberBytes(data []byte, objNum int, mode string) (string, error) {
	_ = mode
	if objNum <= 0 {
		return "", fmt.Errorf("invalid object number: %d", objNum)
	}
	b, ok := findObjectBlockByNumber(data, objNum)
	if !ok {
		return "", fmt.Errorf("object not found: %d", objNum)
	}
	return strings.TrimSpace(string(b)), nil
}

// DetectPageRenderSpecsFromPageInfos builds render specs from already parsed page info.
func DetectPageRenderSpecsFromPageInfos(data []byte, infos []PageInfo) ([]PageRenderSpec, error) {
	if len(infos) == 0 {
		return nil, fmt.Errorf("no page infos")
	}
	out := make([]PageRenderSpec, 0, len(infos))
	for _, pi := range infos {
		pageBlock, ok := findObjectBlockByNumber(data, pi.ObjectNumber)
		if !ok {
			return nil, fmt.Errorf("missing page object: %d", pi.ObjectNumber)
		}
		spec := PageRenderSpec{
			ObjectNumber: pi.ObjectNumber,
			MediaBox:     pi.MediaBox,
			CropBox:      pi.CropBox,
			BleedBox:     pi.BleedBox,
			TrimBox:      pi.TrimBox,
			ArtBox:       pi.ArtBox,
		}
		if n, ok := parseIndirectRefObjectNumberByKey(pageBlock, "/Contents"); ok {
			spec.ContentsRefObject = n
			if b, ok := findObjectBlockByNumber(data, n); ok {
				spec.ContentsObject = strings.TrimSpace(string(b))
			}
		}
		if n := resolveResourcesRefObjectByInheritance(data, pi.ObjectNumber); n > 0 {
			spec.ResourcesRefObject = n
			if b, ok := findObjectBlockByNumber(data, n); ok {
				spec.ResourcesObject = strings.TrimSpace(string(b))
			}
		}
		out = append(out, spec)
	}
	return out, nil
}

// DetectPageRenderSpecsBytes supports upload/in-memory paths.
func DetectPageRenderSpecsBytes(data []byte, mode string) ([]PageRenderSpec, error) {
	infos, err := DetectPageInfosBytes(data, mode)
	if err != nil {
		return nil, err
	}
	return DetectPageRenderSpecsFromPageInfos(data, infos)
}

// DetectPageObjectNumbersBytes supports upload/in-memory paths.
func DetectPageObjectNumbersBytes(data []byte, mode string) ([]int, error) {
	infos, err := DetectPageInfosBytes(data, mode)
	if err != nil {
		return nil, err
	}
	out := make([]int, len(infos))
	for i := range infos {
		out[i] = infos[i].ObjectNumber
	}
	return out, nil
}

func detectRootObjectNumber(data []byte) (int, bool) {
	if n, ok := parseRootObjectFromTrailer(data); ok {
		return n, true
	}
	eofPos := bytes.LastIndex(data, []byte("%%EOF"))
	if eofPos < 0 {
		return 0, false
	}
	startXRefTagPos := bytes.LastIndex(data[:eofPos], []byte("startxref"))
	if startXRefTagPos < 0 {
		return 0, false
	}
	startXRef, err := parseStartXRefValue(data[startXRefTagPos+len("startxref") : eofPos])
	if err != nil || startXRef < 0 || startXRef >= len(data) {
		return 0, false
	}
	return parseRootObjectFromXRefStream(data, startXRef, eofPos)
}

func collectPageLeafObjects(data []byte, objNum int, visited map[int]bool, out *[]int) error {
	if objNum <= 0 {
		return fmt.Errorf("invalid page tree object number: %d", objNum)
	}
	if visited[objNum] {
		return nil
	}
	visited[objNum] = true
	objBlock, ok := findObjectBlockByNumber(data, objNum)
	if !ok {
		return fmt.Errorf("missing page tree object: %d", objNum)
	}
	if objectBlockHasPagesType(objBlock) {
		kids := parseKidsObjectRefs(objBlock)
		for _, kid := range kids {
			if err := collectPageLeafObjects(data, kid, visited, out); err != nil {
				return err
			}
		}
		return nil
	}
	if objectBlockHasPageType(objBlock) {
		*out = append(*out, objNum)
		return nil
	}
	return fmt.Errorf("unexpected page tree node type: %d", objNum)
}

type inheritedPageBoxes struct {
	media string
	crop  string
	bleed string
	trim  string
	art   string
}

func collectPageLeafInfos(data []byte, objNum int, visited map[int]bool, out *[]PageInfo, inherited inheritedPageBoxes) error {
	if objNum <= 0 {
		return fmt.Errorf("invalid page tree object number: %d", objNum)
	}
	if visited[objNum] {
		return nil
	}
	visited[objNum] = true
	objBlock, ok := findObjectBlockByNumber(data, objNum)
	if !ok {
		return fmt.Errorf("missing page tree object: %d", objNum)
	}
	if objectBlockHasPagesType(objBlock) {
		nextInherited := resolveInheritedPageBoxes(data, objBlock, inherited)
		kids := parseKidsObjectRefs(objBlock)
		for _, kid := range kids {
			if err := collectPageLeafInfos(data, kid, visited, out, nextInherited); err != nil {
				return err
			}
		}
		return nil
	}
	if objectBlockHasPageType(objBlock) {
		pi := resolvePageInfoWithInheritance(data, objBlock, objNum, inherited)
		*out = append(*out, pi)
		return nil
	}
	return fmt.Errorf("unexpected page tree node type: %d", objNum)
}

func resolveInheritedPageBoxes(data []byte, objBlock []byte, inherited inheritedPageBoxes) inheritedPageBoxes {
	out := inherited
	if s := parsePageDictionaryRectangle(data, objBlock, "/MediaBox"); s != "" {
		out.media = s
	}
	if s := parsePageDictionaryRectangle(data, objBlock, "/CropBox"); s != "" {
		out.crop = s
	}
	if s := parsePageDictionaryRectangle(data, objBlock, "/BleedBox"); s != "" {
		out.bleed = s
	}
	if s := parsePageDictionaryRectangle(data, objBlock, "/TrimBox"); s != "" {
		out.trim = s
	}
	if s := parsePageDictionaryRectangle(data, objBlock, "/ArtBox"); s != "" {
		out.art = s
	}
	return out
}

func resolvePageInfoWithInheritance(data []byte, objBlock []byte, objNum int, inherited inheritedPageBoxes) PageInfo {
	media := inherited.media
	if s := parsePageDictionaryRectangle(data, objBlock, "/MediaBox"); s != "" {
		media = s
	}
	if strings.TrimSpace(media) == "" {
		media = "0 0 595 842"
	}
	crop := inherited.crop
	if s := parsePageDictionaryRectangle(data, objBlock, "/CropBox"); s != "" {
		crop = s
	}
	bleed := inherited.bleed
	if s := parsePageDictionaryRectangle(data, objBlock, "/BleedBox"); s != "" {
		bleed = s
	}
	trim := inherited.trim
	if s := parsePageDictionaryRectangle(data, objBlock, "/TrimBox"); s != "" {
		trim = s
	}
	art := inherited.art
	if s := parsePageDictionaryRectangle(data, objBlock, "/ArtBox"); s != "" {
		art = s
	}
	return PageInfo{
		ObjectNumber: objNum,
		MediaBox:     media,
		CropBox:      crop,
		BleedBox:     bleed,
		TrimBox:      trim,
		ArtBox:       art,
	}
}

func fillPageInfoOptionalBoxes(data []byte, pi *PageInfo, objBlock []byte) {
	pi.CropBox = parsePageDictionaryRectangle(data, objBlock, "/CropBox")
	pi.BleedBox = parsePageDictionaryRectangle(data, objBlock, "/BleedBox")
	pi.TrimBox = parsePageDictionaryRectangle(data, objBlock, "/TrimBox")
	pi.ArtBox = parsePageDictionaryRectangle(data, objBlock, "/ArtBox")
}

func parseKidsObjectRefs(objBlock []byte) []int {
	pos := bytes.Index(objBlock, []byte("/Kids"))
	if pos < 0 {
		return nil
	}
	rest := objBlock[pos+len("/Kids"):]
	lb := bytes.IndexByte(rest, '[')
	if lb < 0 {
		return nil
	}
	rest = rest[lb+1:]
	rb := bytes.IndexByte(rest, ']')
	if rb < 0 {
		return nil
	}
	fields := strings.Fields(string(rest[:rb]))
	out := make([]int, 0, 8)
	for i := 0; i < len(fields)-2; i++ {
		objNum, err1 := strconv.Atoi(fields[i])
		genNum, err2 := strconv.Atoi(fields[i+1])
		if err1 != nil || err2 != nil || objNum <= 0 || genNum < 0 || fields[i+2] != "R" {
			continue
		}
		out = append(out, objNum)
		i += 2
	}
	return out
}

func parseMediaBoxString(data []byte, objBlock []byte) string {
	// Default to A4 so minimal write-back remains openable.
	const def = "0 0 595 842"
	if s := parsePageDictionaryRectangle(data, objBlock, "/MediaBox"); s != "" {
		return s
	}
	return def
}

func pageDictionaryKeyBoundaryOK(pageBlock []byte, pos int, keyLen int) bool {
	if pos+keyLen >= len(pageBlock) {
		return true
	}
	c := pageBlock[pos+keyLen]
	switch c {
	case ' ', '\t', '\n', '\r', '[', '/', '(', '<':
		return true
	default:
		if c >= '0' && c <= '9' {
			return true
		}
		return false
	}
}

// parsePageDictionaryRectangle parses a /Key rectangle from a page dictionary.
func parsePageDictionaryRectangle(data []byte, pageBlock []byte, key string) string {
	if len(key) == 0 || key[0] != '/' {
		return ""
	}
	pos := bytes.Index(pageBlock, []byte(key))
	if pos < 0 {
		return ""
	}
	if !pageDictionaryKeyBoundaryOK(pageBlock, pos, len(key)) {
		return ""
	}
	rest := bytes.TrimSpace(pageBlock[pos+len(key):])
	if len(rest) == 0 {
		return ""
	}
	if rest[0] == '[' {
		return parsePDFRectangleBracketValue(data, rest)
	}
	m := pdfLeadingIndirectRefRE.FindSubmatch(rest)
	if len(m) < 2 {
		return ""
	}
	objNum, err := strconv.Atoi(string(m[1]))
	if err != nil || objNum <= 0 {
		return ""
	}
	seen := map[int]bool{}
	return resolvePDFRectangleIndirectObject(data, objNum, 0, seen)
}

// parsePDFRectangleBracketValue parses rectangle values starting with '['.
func parsePDFRectangleBracketValue(data []byte, b []byte) string {
	if len(b) == 0 || b[0] != '[' {
		return ""
	}
	rb := bytes.IndexByte(b[1:], ']')
	if rb < 0 {
		return ""
	}
	inner := bytes.TrimSpace(b[1 : 1+rb])
	if len(inner) == 0 {
		return ""
	}
	items := strings.Fields(string(inner))
	if len(items) == 4 {
		allNum := true
		for _, it := range items {
			if _, err := strconv.ParseFloat(it, 64); err != nil {
				allNum = false
				break
			}
		}
		if allNum {
			return strings.Join(items, " ")
		}
	}
	m := pdfLeadingIndirectRefRE.FindSubmatch(inner)
	if len(m) < 2 {
		return ""
	}
	objNum, err := strconv.Atoi(string(m[1]))
	if err != nil || objNum <= 0 {
		return ""
	}
	seen := map[int]bool{}
	return resolvePDFRectangleIndirectObject(data, objNum, 0, seen)
}

func pdfRawObjectInnerBody(block []byte) []byte {
	trimmed := bytes.TrimSpace(block)
	posObj := bytes.Index(trimmed, []byte(" obj"))
	if posObj < 0 {
		return nil
	}
	rest := bytes.TrimSpace(trimmed[posObj+len(" obj"):])
	end := bytes.LastIndex(rest, []byte("endobj"))
	if end < 0 {
		return nil
	}
	return bytes.TrimSpace(rest[:end])
}

func resolvePDFRectangleIndirectObject(data []byte, objNum int, depth int, seen map[int]bool) string {
	const maxDepth = 12
	if depth > maxDepth || objNum <= 0 || seen[objNum] {
		return ""
	}
	seen[objNum] = true
	defer delete(seen, objNum)

	block, ok := findObjectBlockByNumber(data, objNum)
	if !ok {
		return ""
	}
	body := pdfRawObjectInnerBody(block)
	if len(body) == 0 {
		return ""
	}
	if body[0] == '[' {
		return parsePDFRectangleBracketValue(data, body)
	}
	m := pdfLeadingIndirectRefRE.FindSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	next, err := strconv.Atoi(string(m[1]))
	if err != nil || next <= 0 {
		return ""
	}
	return resolvePDFRectangleIndirectObject(data, next, depth+1, seen)
}

func parseIndirectRefObjectNumberByKey(objBlock []byte, key string) (int, bool) {
	pos := bytes.Index(objBlock, []byte(key))
	if pos < 0 {
		return 0, false
	}
	rest := objBlock[pos+len(key):]
	m := pdfIndirectRefAfterKeyRE.FindSubmatch(rest)
	if len(m) < 3 {
		return 0, false
	}
	objNum, err1 := strconv.Atoi(string(m[1]))
	genNum, err2 := strconv.Atoi(string(m[2]))
	if err1 != nil || err2 != nil || objNum <= 0 || genNum < 0 {
		return 0, false
	}
	return objNum, true
}

// ContentsIndirectRefObjectNumbers parses indirect references after /Contents,
// preserving order and duplicates.
func ContentsIndirectRefObjectNumbers(pageBlock []byte) []int {
	pos := bytes.Index(pageBlock, []byte("/Contents"))
	if pos < 0 {
		return nil
	}
	rest := bytes.TrimSpace(pageBlock[pos+len("/Contents"):])
	if len(rest) == 0 {
		return nil
	}
	if rest[0] == '[' {
		depth := 1
		i := 1
		for i < len(rest) && depth > 0 {
			switch rest[i] {
			case '[':
				depth++
			case ']':
				depth--
			}
			i++
		}
		if depth != 0 {
			return nil
		}
		inner := bytes.TrimSpace(rest[1 : i-1])
		return indirectRefNumbersInOrder(inner)
	}
	m := pdfLeadingIndirectRefRE.FindSubmatch(rest)
	if len(m) < 2 {
		return nil
	}
	n, err := strconv.Atoi(string(m[1]))
	if err != nil || n <= 0 {
		return nil
	}
	return []int{n}
}

func indirectRefNumbersInOrder(frag []byte) []int {
	ms := pdfAnyIndirectRefRE.FindAllSubmatch(frag, -1)
	out := make([]int, 0, len(ms))
	for _, m := range ms {
		if len(m) < 2 {
			continue
		}
		n, err := strconv.Atoi(string(m[1]))
		if err != nil || n <= 0 {
			continue
		}
		out = append(out, n)
	}
	return out
}

// resolveResourcesRefObjectByInheritance walks the page tree upward via /Parent until it finds /Resources.
// Many generators put Resources only on intermediate /Pages nodes, not on each /Page leaf.
func resolveResourcesRefObjectByInheritance(data []byte, pageObj int) int {
	const maxHops = 64
	seen := make(map[int]bool)
	cur := pageObj
	for hops := 0; hops < maxHops && cur > 0; hops++ {
		if seen[cur] {
			break
		}
		seen[cur] = true
		block, ok := findObjectBlockByNumber(data, cur)
		if !ok {
			break
		}
		if n, ok := parseIndirectRefObjectNumberByKey(block, "/Resources"); ok && n > 0 {
			return n
		}
		parentNum, ok := parseIndirectRefObjectNumberByKey(block, "/Parent")
		if !ok || parentNum <= 0 {
			break
		}
		cur = parentNum
	}
	return 0
}

func detectPageCountByScan(data []byte) int {
	// First local strategy: find the object containing "/Type /Pages", then
	// read "/Count N" from the same object.
	cursor := 0
	for {
		rel := pdfTypeMarkerIndex(data[cursor:], "Pages")
		if rel < 0 {
			return 0
		}
		pos := cursor + rel
		objStart := bytes.LastIndex(data[:pos], []byte("obj"))
		if objStart < 0 {
			cursor = pos + 1
			continue
		}
		endRel := bytes.Index(data[pos:], []byte("endobj"))
		if endRel < 0 {
			cursor = pos + 1
			continue
		}
		objBlock := data[objStart : pos+endRel]
		count := extractCountFromObject(objBlock)
		if count >= 0 {
			return count
		}
		cursor = pos + 1
	}
}

func parseRootObjectFromTrailer(data []byte) (int, bool) {
	eofPos := bytes.LastIndex(data, []byte("%%EOF"))
	if eofPos < 0 {
		return 0, false
	}
	startXRefTagPos := bytes.LastIndex(data[:eofPos], []byte("startxref"))
	if startXRefTagPos < 0 {
		return 0, false
	}
	startXRef, err := parseStartXRefValue(data[startXRefTagPos+len("startxref") : eofPos])
	if err != nil || startXRef < 0 || startXRef >= len(data) {
		return 0, false
	}
	_, hasTrailer := detectXRefType(data, startXRef, eofPos)
	if !hasTrailer {
		return 0, false
	}
	lines := splitXRefRegionNonEmptyLines(data[startXRef:eofPos])
	trailerIdx := -1
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "trailer" {
			trailerIdx = i
			break
		}
	}
	if trailerIdx < 0 {
		return 0, false
	}
	return parseTrailerRootObject(lines, trailerIdx+1)
}

func parseInfoMetadata(data []byte, startXRef, eofPos int) map[string]string {
	out := map[string]string{}
	lines := splitXRefRegionNonEmptyLines(data[startXRef:eofPos])
	trailerIdx := -1
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "trailer" {
			trailerIdx = i
			break
		}
	}
	if trailerIdx < 0 {
		return out
	}
	infoObjNum, ok := parseTrailerInfoObject(lines, trailerIdx+1)
	if !ok {
		return out
	}
	objBlock, ok := findObjectBlockByNumber(data, infoObjNum)
	if !ok {
		return out
	}
	out["Title"] = extractPDFStringByKey(objBlock, "Title")
	out["Author"] = extractPDFStringByKey(objBlock, "Author")
	out["Subject"] = extractPDFStringByKey(objBlock, "Subject")
	out["Producer"] = extractPDFStringByKey(objBlock, "Producer")
	out["Creator"] = extractPDFStringByKey(objBlock, "Creator")
	out["Keywords"] = extractPDFStringByKey(objBlock, "Keywords")
	return out
}

func parseTrailerInfoObject(lines []string, start int) (int, bool) {
	fields := collectTrailerFields(lines, start)
	for i := 0; i < len(fields)-3; i++ {
		if fields[i] != "/Info" {
			continue
		}
		objNum, err1 := strconv.Atoi(fields[i+1])
		genNum, err2 := strconv.Atoi(fields[i+2])
		if err1 != nil || err2 != nil || genNum < 0 || fields[i+3] != "R" || objNum <= 0 {
			return 0, false
		}
		return objNum, true
	}
	return 0, false
}

func extractPDFStringByKey(objBlock []byte, key string) string {
	keyMarker := []byte("/" + key)
	idx := bytes.Index(objBlock, keyMarker)
	if idx < 0 {
		return ""
	}
	rest := bytes.TrimLeft(objBlock[idx+len(keyMarker):], " \t\r\n")
	if len(rest) == 0 || rest[0] != '(' {
		return ""
	}
	raw, ok := readPDFLiteralString(rest)
	if !ok {
		return ""
	}
	return unescapePDFLiteralString(raw)
}

func readPDFLiteralString(b []byte) (string, bool) {
	if len(b) == 0 || b[0] != '(' {
		return "", false
	}
	level := 0
	escaped := false
	start := 1
	for i := 0; i < len(b); i++ {
		ch := b[i]
		if i == 0 {
			level = 1
			continue
		}
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '(' {
			level++
			continue
		}
		if ch == ')' {
			level--
			if level == 0 {
				return string(b[start:i]), true
			}
		}
	}
	return "", false
}

func unescapePDFLiteralString(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if !escaped {
			if ch == '\\' {
				escaped = true
				continue
			}
			out.WriteByte(ch)
			continue
		}
		switch ch {
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case '(', ')', '\\':
			out.WriteByte(ch)
		default:
			out.WriteByte(ch)
		}
		escaped = false
	}
	if escaped {
		out.WriteByte('\\')
	}
	return out.String()
}

func extractCountFromObject(objBlock []byte) int {
	idx := bytes.Index(objBlock, []byte("/Count"))
	if idx < 0 {
		return -1
	}
	rest := strings.TrimSpace(string(objBlock[idx+len("/Count"):]))
	if rest == "" {
		return -1
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return -1
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil || n < 0 {
		return -1
	}
	return n
}
