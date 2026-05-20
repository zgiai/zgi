package pdf

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ExtractedImage represents an embedded PDF image after filter decoding.
type ExtractedImage struct {
	PageIndex     int    `json:"page_index"`
	PageObject    int    `json:"page_object"`
	XObjectName   string `json:"xobject_name"`
	ObjectNumber  int    `json:"object_number"`
	Format        string `json:"format"` // jpeg, png, tiff
	Width         int    `json:"width,omitempty"`
	Height        int    `json:"height,omitempty"`
	DecodeWarning string `json:"decode_warning,omitempty"`
}

// ExtractedImageBytes includes decoded file bytes for an ExtractedImage.
type ExtractedImageBytes struct {
	ExtractedImage
	Bytes []byte `json:"-"`
	// StreamByteSize is /Length or raw stream length in light extraction paths,
	// used for chunk metadata when Bytes is not available.
	StreamByteSize int `json:"-"`
}

var xobjectNamedRefRE = regexp.MustCompile(`/([^\s/\[\]<>]+)\s+(\d+)\s+(\d+)\s+R`)
var contentsRefSegmentRE = regexp.MustCompile(`(?s)/Contents\s*(\[[^\]]*\]|\d+\s+\d+\s+R)`)
var anyIndirectRefRE = regexp.MustCompile(`(\d+)\s+(\d+)\s+R`)
var doOperatorNameRE = regexp.MustCompile(`/([^\s/\[\]<>]+)\s+Do\b`)

func resourcesDictLikelyHasXObject(objBlock []byte) bool {
	body := objectBodyFromBlockBytes(objBlock)
	if body == "" {
		return false
	}
	b := []byte(body)
	return bytes.Contains(b, []byte("/XObject")) || bytes.Contains(b, []byte("/XObject<<"))
}

// findResourcesObjectBlockForImageScan resolves a page /Resources object. Some
// generators or incremental updates leave a same-number direct object that is
// not the real resources dictionary, so candidates with /XObject are preferred.
func findResourcesObjectBlockForImageScan(data []byte, resObjNum int) ([]byte, bool) {
	if resObjNum <= 0 {
		return nil, false
	}
	d1, ok1 := findDirectObjectBlockByNumber(data, resObjNum)
	d2, ok2 := findObjectFromObjectStreams(data, resObjNum)
	x1 := ok1 && resourcesDictLikelyHasXObject(d1)
	x2 := ok2 && resourcesDictLikelyHasXObject(d2)
	if x2 && !x1 {
		return d2, true
	}
	if x1 && !x2 {
		return d1, true
	}
	if x1 && x2 {
		if ok1 && (bytes.Contains(d1, []byte("/Type /Font")) || bytes.Contains(d1, []byte("/Type/Font"))) {
			return d2, true
		}
		return d1, true
	}
	if ok1 {
		return d1, true
	}
	if ok2 {
		return d2, true
	}
	return nil, false
}

func objectBlockIsImageXObject(blob []byte) bool {
	return bytes.Contains(blob, []byte("/Subtype /Image")) || bytes.Contains(blob, []byte("/Subtype/Image"))
}

// ExtractEmbeddedImagesFromBytesWithSpecs reuses parsed page render specs.
func ExtractEmbeddedImagesFromBytesWithSpecs(data []byte, mode string, specs []PageRenderSpec) ([]ExtractedImageBytes, error) {
	var out []ExtractedImageBytes
	for pageIdx, sp := range specs {
		resBody := resolveResourcesBodyForPageText(data, sp, mode)
		if resBody == "" {
			continue
		}
		xFrag := extractInlineDictAfterKey(resBody, "/XObject")
		if xFrag == "" {
			continue
		}
		refs := parseNamedXObjectRefs(xFrag)
		usedNames := detectUsedXObjectNamesForPage(data, sp, mode)
		for _, ref := range refs {
			// Extract only XObjects actually invoked by "Name Do" on this page.
			if len(usedNames) > 0 && !usedNames[ref.Name] {
				continue
			}
			blob, err := ExtractObjectBlockByNumberBytes(data, ref.Obj, mode)
			if err != nil {
				continue
			}
			blobB := []byte(blob)
			if !objectBlockIsImageXObject(blobB) {
				continue
			}
			dict := objectDictBytesBeforeStream(blobB)
			raw, ok := ParseObjectStreamBytes(blobB)
			if !ok {
				continue
			}
			decoded, err := DecodeStreamFilters(dict, raw)
			warn := ""
			if err != nil {
				warn = err.Error()
				decoded = DecodeStreamFiltersBestEffort(dict, raw)
				if len(decoded) == 0 {
					continue
				}
			}
			fmtName, w, h := classifyImageFormat(dict, decoded)
			if fmtName == "" {
				if warn == "" {
					warn = "unrecognized image format after decode"
				}
				continue
			}
			outBytes := decoded
			outFmt := fmtName
			if strings.EqualFold(strings.TrimSpace(fmtName), "pdf_raster") {
				pngb, err := pdfRasterDecodedToPNG(dict, decoded)
				if err != nil {
					if warn == "" {
						warn = "pdf_raster_to_png: " + err.Error()
					} else {
						warn = warn + "; pdf_raster_to_png: " + err.Error()
					}
					continue
				}
				outBytes = pngb
				outFmt = "png"
			}
			out = append(out, ExtractedImageBytes{
				ExtractedImage: ExtractedImage{
					PageIndex:     pageIdx + 1,
					PageObject:    sp.ObjectNumber,
					XObjectName:   ref.Name,
					ObjectNumber:  ref.Obj,
					Format:        outFmt,
					Width:         w,
					Height:        h,
					DecodeWarning: warn,
				},
				Bytes: outBytes,
			})
		}
	}
	return out, nil
}

// ExtractEmbeddedImagesFromBytesWithSpecsLight uses the same traversal and
// reference filtering as WithSpecs but avoids image stream decoding for speed.
// Unknown XObject image formats are skipped. Use ExtractEmbeddedImagesFromBytesWithSpecs
// when full decoded pixels are required.
func ExtractEmbeddedImagesFromBytesWithSpecsLight(data []byte, mode string, specs []PageRenderSpec) (out []ExtractedImageBytes, err error) {
	t0 := time.Now()
	fullDocDebugf("ExtractEmbeddedImagesFromSpecsLight begin pages=%d", len(specs))
	defer func() {
		fullDocDebugf("ExtractEmbeddedImagesFromSpecsLight done images=%d elapsed_ms=%d", len(out), time.Since(t0).Milliseconds())
	}()
	for pageIdx, sp := range specs {
		resBody := resolveResourcesBodyForPageText(data, sp, mode)
		if resBody == "" {
			continue
		}
		xFrag := extractInlineDictAfterKey(resBody, "/XObject")
		if xFrag == "" {
			continue
		}
		refs := parseNamedXObjectRefs(xFrag)
		usedNames := detectUsedXObjectNamesForPage(data, sp, mode)
		for _, ref := range refs {
			if len(usedNames) > 0 && !usedNames[ref.Name] {
				continue
			}
			blob, err := ExtractObjectBlockByNumberBytes(data, ref.Obj, mode)
			if err != nil {
				continue
			}
			blobB := []byte(blob)
			if !objectBlockIsImageXObject(blobB) {
				continue
			}
			dict := objectDictBytesBeforeStream(blobB)
			raw, ok := ParseObjectStreamBytes(blobB)
			if !ok {
				continue
			}
			fmtName := inferImageFormatFromDictAndRaw(dict, raw)
			if fmtName == "" {
				continue
			}
			w := parseIntKey(dict, "/Width")
			h := parseIntKey(dict, "/Height")
			encLen := parseIntKey(dict, "/Length")
			bs := encLen
			if bs <= 0 {
				bs = len(raw)
			}
			out = append(out, ExtractedImageBytes{
				ExtractedImage: ExtractedImage{
					PageIndex:     pageIdx + 1,
					PageObject:    sp.ObjectNumber,
					XObjectName:   ref.Name,
					ObjectNumber:  ref.Obj,
					Format:        fmtName,
					Width:         w,
					Height:        h,
					DecodeWarning: "",
				},
				Bytes:          nil,
				StreamByteSize: bs,
			})
		}
	}
	return out, nil
}

// ExtractEmbeddedImagesFromBytes traverses page Resources/XObject and decodes image streams.
func ExtractEmbeddedImagesFromBytes(data []byte, mode string) ([]ExtractedImageBytes, error) {
	specs, err := DetectPageRenderSpecsBytes(data, mode)
	if err != nil {
		return nil, err
	}
	return ExtractEmbeddedImagesFromBytesWithSpecs(data, mode, specs)
}

func detectUsedXObjectNamesForPage(data []byte, sp PageRenderSpec, mode string) map[string]bool {
	contentRefs := extractContentsObjectRefsForPage(data, sp.ObjectNumber)
	if len(contentRefs) == 0 && sp.ContentsRefObject > 0 {
		contentRefs = append(contentRefs, sp.ContentsRefObject)
	}
	if len(contentRefs) == 0 {
		return nil
	}
	used := make(map[string]bool)
	for _, objNum := range contentRefs {
		blk, err := ExtractObjectBlockByNumberBytes(data, objNum, mode)
		if err != nil {
			continue
		}
		names := extractDoOperatorNamesFromContentObject([]byte(blk))
		for n := range names {
			used[n] = true
		}
	}
	if len(used) == 0 {
		return nil
	}
	return used
}

func extractContentsObjectRefsForPage(data []byte, pageObjNum int) []int {
	if pageObjNum <= 0 {
		return nil
	}
	pageBlock, err := ExtractObjectBlockByNumberBytes(data, pageObjNum, ValidationModeRelaxed)
	if err != nil {
		return nil
	}
	m := contentsRefSegmentRE.FindStringSubmatch(pageBlock)
	if len(m) < 2 {
		return nil
	}
	raw := m[1]
	matches := anyIndirectRefRE.FindAllStringSubmatch(raw, -1)
	out := make([]int, 0, len(matches))
	seen := map[int]bool{}
	for _, one := range matches {
		if len(one) < 2 {
			continue
		}
		n, err := strconv.Atoi(one[1])
		if err != nil || n <= 0 || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

func extractDoOperatorNamesFromContentObject(objBlock []byte) map[string]bool {
	out := map[string]bool{}
	dict := objectDictBytesBeforeStream(objBlock)
	raw, ok := ParseObjectStreamBytes(objBlock)
	if !ok {
		return out
	}
	decoded, err := DecodeStreamFilters(dict, raw)
	if err != nil {
		decoded = DecodeStreamFiltersBestEffort(dict, raw)
	}
	if len(decoded) == 0 {
		decoded = raw
	}
	matches := doOperatorNameRE.FindAllSubmatch(decoded, -1)
	for _, m := range matches {
		if len(m) < 2 || len(m[1]) == 0 {
			continue
		}
		out[string(m[1])] = true
	}
	if len(out) == 0 {
		// If content stream Length is wrong, ParseObjectStreamBytes may be truncated;
		// fall back to searching Do operators in the full object block.
		matches = doOperatorNameRE.FindAllSubmatch(objBlock, -1)
		for _, m := range matches {
			if len(m) < 2 || len(m[1]) == 0 {
				continue
			}
			out[string(m[1])] = true
		}
	}
	return out
}

type xobjectRef struct {
	Name string
	Obj  int
}

func parseNamedXObjectRefs(frag string) []xobjectRef {
	matches := xobjectNamedRefRE.FindAllStringSubmatch(frag, -1)
	out := make([]xobjectRef, 0, len(matches))
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		n, err := strconv.Atoi(m[2])
		if err != nil || n <= 0 {
			continue
		}
		out = append(out, xobjectRef{Name: m[1], Obj: n})
	}
	return out
}

func objectBodyFromBlockBytes(block []byte) string {
	s := string(block)
	p := strings.Index(s, "<<")
	if p < 0 {
		return ""
	}
	e := strings.Index(s, "endobj")
	if e < 0 || e <= p {
		return s[p:]
	}
	return s[p:e]
}

func objectDictBytesBeforeStream(objBlock []byte) []byte {
	streamPos := bytes.Index(objBlock, []byte("stream"))
	if streamPos < 0 {
		return nil
	}
	left := objBlock[:streamPos]
	dictEnd := bytes.LastIndex(left, []byte(">>"))
	if dictEnd < 0 {
		return nil
	}
	dictStart := bytes.LastIndex(left[:dictEnd], []byte("<<"))
	if dictStart < 0 {
		return nil
	}
	return left[dictStart : dictEnd+2]
}

func extractInlineDictAfterKey(body, key string) string {
	pos := strings.Index(body, key)
	if pos < 0 {
		return ""
	}
	after := strings.TrimSpace(body[pos+len(key):])
	if !strings.HasPrefix(after, "<<") {
		return ""
	}
	depth := 0
	for i := 0; i < len(after)-1; i++ {
		if after[i] == '<' && after[i+1] == '<' {
			depth++
			i++
			continue
		}
		if after[i] == '>' && after[i+1] == '>' {
			depth--
			i++
			if depth == 0 {
				return after[:i+1]
			}
			continue
		}
	}
	return ""
}

// inferImageFormatFromDictAndRaw infers image format from /Filter and raw magic
// bytes without decoding filters.
func inferImageFormatFromDictAndRaw(dict, raw []byte) string {
	if len(dict) > 0 {
		if bytes.Contains(dict, []byte("/DCTDecode")) || bytes.Contains(dict, []byte("DCTDecode")) {
			return "jpeg"
		}
		if bytes.Contains(dict, []byte("/JPXDecode")) {
			return "jpeg"
		}
		if bytes.Contains(dict, []byte("/CCITTFaxDecode")) || bytes.Contains(dict, []byte("CCITTFaxDecode")) {
			return "tiff"
		}
	}
	if len(raw) >= 3 && raw[0] == 0xff && raw[1] == 0xd8 && raw[2] == 0xff {
		return "jpeg"
	}
	if len(raw) >= 8 && bytes.Equal(raw[:8], []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		return "png"
	}
	if len(raw) >= 4 && (bytes.HasPrefix(raw, []byte("II*\x00")) || bytes.HasPrefix(raw, []byte("MM\x00*"))) {
		return "tiff"
	}
	// Common vector/e-stamp composition: raw scanlines with /Subtype /Image and
	// Flate/LZW filters but no JPEG/PNG magic. Dictionary dimensions are still useful.
	if len(dict) > 0 && objectBlockIsImageXObject(dict) {
		w := parseIntKey(dict, "/Width")
		h := parseIntKey(dict, "/Height")
		if w > 0 && h > 0 {
			if bytes.Contains(dict, []byte("/FlateDecode")) || bytes.Contains(dict, []byte("FlateDecode")) {
				return "pdf_raster"
			}
			if bytes.Contains(dict, []byte("/LZWDecode")) || bytes.Contains(dict, []byte("LZWDecode")) {
				return "pdf_raster"
			}
			if bytes.Contains(dict, []byte("/RunLengthDecode")) || bytes.Contains(dict, []byte("RunLengthDecode")) {
				return "pdf_raster"
			}
		}
	}
	return ""
}

func classifyImageFormat(dict, decoded []byte) (string, int, int) {
	w := parseIntKey(dict, "/Width")
	h := parseIntKey(dict, "/Height")
	if len(decoded) >= 3 && decoded[0] == 0xff && decoded[1] == 0xd8 && decoded[2] == 0xff {
		return "jpeg", w, h
	}
	if len(decoded) >= 8 && bytes.Equal(decoded[:8], []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		return "png", w, h
	}
	if len(decoded) >= 4 && (bytes.HasPrefix(decoded, []byte("II*\x00")) || bytes.HasPrefix(decoded, []byte("MM\x00*"))) {
		return "tiff", w, h
	}
	if len(decoded) > 0 && w > 0 && h > 0 && objectBlockIsImageXObject(dict) {
		if bytes.Contains(dict, []byte("/FlateDecode")) || bytes.Contains(dict, []byte("FlateDecode")) ||
			bytes.Contains(dict, []byte("/LZWDecode")) || bytes.Contains(dict, []byte("LZWDecode")) ||
			bytes.Contains(dict, []byte("/RunLengthDecode")) || bytes.Contains(dict, []byte("RunLengthDecode")) {
			return "pdf_raster", w, h
		}
	}
	return "", 0, 0
}

// ImageFileExtension returns the file extension for Format.
func ImageFileExtension(format string) string {
	switch format {
	case "jpeg":
		return ".jpg"
	case "png":
		return ".png"
	case "tiff":
		return ".tif"
	default:
		return ".bin"
	}
}

// SanitizeXObjectFilePart converts an XObject name to a safe file-name fragment.
func SanitizeXObjectFilePart(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	s := b.String()
	if s == "" {
		return "img"
	}
	return s
}
