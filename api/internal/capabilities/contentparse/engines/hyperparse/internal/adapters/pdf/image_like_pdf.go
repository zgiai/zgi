package pdf

import (
	"strings"
	"unicode/utf8"
)

// ImageLikePDFHints captures lightweight scan/form-layout signals: a very thin
// text layer, short content streams, and a relatively large file usually mean
// render+OCR/VLM will produce better results than native text extraction alone.
type ImageLikePDFHints struct {
	Likely                    bool     `json:"likely"`
	Reasons                   []string `json:"reasons"`
	TotalDecodedContentBytes  int      `json:"total_decoded_content_bytes"`
	ContentStreamObjectCount  int      `json:"content_stream_object_count"`
	BasicTextRuneCount        int      `json:"basic_text_rune_count"`
	BasicSegmentCount         int      `json:"basic_segment_count"`
	FileSizeBytes             int      `json:"file_size_bytes"`
	PageCount                 int      `json:"page_count"`
	BytesPerPage              int      `json:"bytes_per_page"`
	GeometryLineCount         int      `json:"geometry_line_count"`
	ShortGeometryLineCount    int      `json:"short_geometry_line_count"`
	FormLikeGeometryLineCount int      `json:"form_like_geometry_line_count"`
}

// sumDecodedPageContentBytes totals decoded /Contents stream bytes for each page,
// excluding XObject image bodies.
func sumDecodedPageContentBytes(data []byte, specs []PageRenderSpec, mode string) (total int, streamObjs int) {
	for _, sp := range specs {
		refs := extractContentsObjectRefsForPage(data, sp.ObjectNumber)
		if len(refs) == 0 && sp.ContentsRefObject > 0 {
			refs = append(refs, sp.ContentsRefObject)
		}
		for _, objNum := range refs {
			blk, err := ExtractObjectBlockByNumberBytes(data, objNum, mode)
			if err != nil {
				continue
			}
			objBlock := []byte(blk)
			dict := objectDictBytesBeforeStream(objBlock)
			raw, ok := ParseObjectStreamBytes(objBlock)
			if !ok {
				continue
			}
			decoded := DecodeStreamFiltersBestEffort(dict, raw)
			total += len(decoded)
			streamObjs++
		}
	}
	return total, streamObjs
}

// BuildImageLikePDFHints builds scan-like signals from existing page specs and
// basic text segments so full_document can reuse one parse pass.
func BuildImageLikePDFHints(data []byte, mode string, specs []PageRenderSpec, basicSegments []TextSegment) ImageLikePDFHints {
	return BuildImageLikePDFHintsWithLayout(data, mode, specs, basicSegments, nil)
}

// BuildImageLikePDFHintsWithLayout adds short-line and form-like layout signals
// on top of the basic scan-like hints.
func BuildImageLikePDFHintsWithLayout(data []byte, mode string, specs []PageRenderSpec, basicSegments []TextSegment, geometryLines []GeometryLine) ImageLikePDFHints {
	h := ImageLikePDFHints{
		FileSizeBytes: len(data),
		PageCount:     len(specs),
	}
	if len(specs) == 0 {
		return h
	}
	if h.PageCount > 0 {
		h.BytesPerPage = len(data) / h.PageCount
	}
	h.TotalDecodedContentBytes, h.ContentStreamObjectCount = sumDecodedPageContentBytes(data, specs, mode)
	h.BasicSegmentCount = len(basicSegments)
	for _, s := range basicSegments {
		h.BasicTextRuneCount += utf8.RuneCountInString(s.Text)
	}
	h.GeometryLineCount = len(geometryLines)
	for _, gl := range geometryLines {
		if looksLikeShortGeometryLine(gl.Text) {
			h.ShortGeometryLineCount++
		}
		if looksLikeFormLikeGeometryLine(gl.Text) {
			h.FormLikeGeometryLineCount++
		}
	}
	h.applyHeuristics()
	return h
}

func (h *ImageLikePDFHints) applyHeuristics() {
	seen := map[string]bool{}
	add := func(reason string) {
		if seen[reason] {
			return
		}
		seen[reason] = true
		h.Reasons = append(h.Reasons, reason)
		h.Likely = true
	}
	// Referenced page content streams decode to very little text while the file is large.
	if h.ContentStreamObjectCount > 0 && h.TotalDecodedContentBytes < 2048 && h.FileSizeBytes > 100000 && h.PageCount >= 1 {
		add("thin_page_content_streams")
	}
	// The extractable text layer is tiny compared with the file size.
	if h.BasicTextRuneCount < 50 && h.FileSizeBytes > 120000 && h.PageCount >= 1 {
		add("minimal_basic_text_layer")
	}
	// Large bytes per page with little text is common for page-image scans.
	if h.BytesPerPage > 300000 && h.BasicTextRuneCount < 100 && h.PageCount >= 1 {
		add("large_bytes_per_page_low_text")
	}
	// Dense short geometry lines often indicate bills/forms where native ordering can drift.
	if h.GeometryLineCount >= 18 &&
		h.ShortGeometryLineCount*100 >= h.GeometryLineCount*65 &&
		h.BytesPerPage > 40000 &&
		h.BasicTextRuneCount < 2500 {
		add("dense_short_geometry_lines")
	}
	// Many form-like lines indicate a layout document that usually benefits from render+VLM.
	if h.FormLikeGeometryLineCount >= 8 &&
		h.GeometryLineCount >= 12 &&
		h.BytesPerPage > 40000 &&
		h.PageCount >= 1 {
		add("form_like_geometry_lines")
	}
}

// DetectImageLikePDFBytes is a standalone entry point for paths that do not run full_document.
func DetectImageLikePDFBytes(data []byte, mode string) (ImageLikePDFHints, error) {
	specs, err := DetectPageRenderSpecsBytes(data, mode)
	if err != nil {
		return ImageLikePDFHints{FileSizeBytes: len(data)}, err
	}
	if len(specs) == 0 {
		return ImageLikePDFHints{FileSizeBytes: len(data)}, nil
	}
	segs := ExtractTextBasicSegmentsFromBytesWithSpecs(data, specs)
	return BuildImageLikePDFHints(data, mode, specs, segs), nil
}

func looksLikeShortGeometryLine(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	runes := utf8.RuneCountInString(text)
	return runes >= 1 && runes <= 24
}

func looksLikeFormLikeGeometryLine(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	low := normalizeRouteHintText(text)
	if idx := strings.IndexAny(text, ":："); idx >= 2 && idx <= 32 {
		return true
	}
	if strings.Contains(text, "☐") || strings.Contains(text, "☑") || strings.Contains(low, "[ ]") || strings.Contains(low, "[x]") {
		return true
	}
	return containsAnyNormalized(
		low,
		"account number", "invoice number", "billing period", "amount due", "total due",
		zhInvoiceNumber, zhInvoiceCode, zhIssueDate, zhTotalTaxIncluded, zhAccountNumber, zhAccountBalance,
	)
}
