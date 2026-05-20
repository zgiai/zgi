package quality

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/zgiai/ginext/internal/contracts"
)

var (
	barcodePlaceholderRE = regexp.MustCompile(`(?i)^\s*\[\s*(barcode|bar\s*code|qr(?:\s*code)?)\s*\]\s*$`)
	figurePlaceholderRE  = regexp.MustCompile(`(?i)^\s*\[\s*(figure|fig|image|picture)\s*\]\s*$`)
	pageCounterENRE      = regexp.MustCompile(`(?i)^\s*page\s+\d+\s*(of|/)\s*\d+\s*$`)
	longNumberRE         = regexp.MustCompile(`^\s*\d{8,}\s*$`)
	standaloneNumberRE   = regexp.MustCompile(`^\s*\d{1,3}\s*$`)
	singleCJKRE          = regexp.MustCompile(`^\s*\p{Han}\s*$`)
)

// FilterMetrics describes what the quality layer removed. It is designed for
// shadow inspection before any business cutover.
type FilterMetrics struct {
	InputCount   int            `json:"input_count"`
	OutputCount  int            `json:"output_count"`
	RemovedCount int            `json:"removed_count"`
	Reasons      map[string]int `json:"reasons,omitempty"`
}

type UnitFilter struct{}

func NewUnitFilter() *UnitFilter {
	return &UnitFilter{}
}

func (f *UnitFilter) FilterUnits(units []contracts.ChunkUnit) ([]contracts.ChunkUnit, FilterMetrics) {
	metrics := FilterMetrics{
		InputCount: len(units),
		Reasons:    map[string]int{},
	}
	if len(units) == 0 {
		return units, metrics
	}

	out := make([]contracts.ChunkUnit, 0, len(units))
	for _, unit := range units {
		reason := lowValueReason(chunkUnitFilterText(unit))
		if reason != "" {
			metrics.RemovedCount++
			metrics.Reasons[reason]++
			continue
		}
		out = append(out, unit)
	}
	metrics.OutputCount = len(out)
	return out, metrics
}

func (f *UnitFilter) FilterElements(elements []contracts.ChunkSourceElement) ([]contracts.ChunkSourceElement, FilterMetrics) {
	metrics := FilterMetrics{
		InputCount: len(elements),
		Reasons:    map[string]int{},
	}
	if len(elements) == 0 {
		return elements, metrics
	}

	out := make([]contracts.ChunkSourceElement, 0, len(elements))
	for _, element := range elements {
		reason := lowValueElementReason(element)
		if reason != "" {
			metrics.RemovedCount++
			metrics.Reasons[reason]++
			continue
		}
		out = append(out, element)
	}
	metrics.OutputCount = len(out)
	return out, metrics
}

func lowValueElementReason(element contracts.ChunkSourceElement) string {
	content := strings.TrimSpace(element.Content)
	markdown := strings.TrimSpace(element.Markdown)
	if content == "" && markdown == "" {
		return "empty"
	}
	if content == "" {
		return lowValueReason(markdown)
	}

	reason := lowValueReason(content)
	if reason != "" {
		if markdown != "" && lowValueReason(markdown) == "" {
			return ""
		}
		return reason
	}
	return lowValueLayoutReason(element, content)
}

func chunkUnitFilterText(unit contracts.ChunkUnit) string {
	content := strings.TrimSpace(unit.Content)
	if content != "" {
		return content
	}
	return strings.TrimSpace(unit.Markdown)
}

func lowValueReason(content string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if normalized == "" {
		return "empty"
	}
	if barcodePlaceholderRE.MatchString(normalized) {
		return "barcode_placeholder"
	}
	if figurePlaceholderRE.MatchString(normalized) {
		return "figure_placeholder"
	}
	if pageCounterENRE.MatchString(normalized) {
		return "page_counter"
	}
	if longNumberRE.MatchString(normalized) {
		return "long_number"
	}
	if standaloneNumberRE.MatchString(normalized) {
		return "standalone_number"
	}
	if singleCJKRE.MatchString(normalized) {
		return "single_cjk"
	}
	if isNoTextVisionMessage(normalized) {
		return "vision_no_text_message"
	}
	return ""
}

func lowValueLayoutReason(element contracts.ChunkSourceElement, content string) string {
	if !isOCRElement(element) || !validBBox(element.BBox) {
		return ""
	}
	normalized := strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if normalized == "" || usefulShortLabel(normalized) {
		return ""
	}
	if isPunctuationOnly(normalized) {
		return "ocr_punctuation_noise"
	}
	runeCount := len([]rune(normalized))
	width := element.BBox.Right - element.BBox.Left
	height := element.BBox.Bottom - element.BBox.Top
	area := width * height
	if runeCount <= 4 && area <= 0.0012 {
		return "tiny_ocr_fragment"
	}
	if runeCount <= 4 && width <= 0.07 && height <= 0.035 && nearPageEdge(element.BBox) {
		return "edge_ocr_fragment"
	}
	return ""
}

func isOCRElement(element contracts.ChunkSourceElement) bool {
	if len(element.Metadata) == 0 {
		return false
	}
	if strings.Contains(strings.ToLower(readStringMetadata(element.Metadata, "bbox_source")), "ocr") {
		return true
	}
	if strings.EqualFold(readStringMetadata(element.Metadata, "extraction_method"), "ocr") {
		return true
	}
	payload, ok := element.Metadata["payload"].(map[string]any)
	if !ok || len(payload) == 0 {
		return false
	}
	return strings.Contains(strings.ToLower(readStringMetadata(payload, "bbox_source")), "ocr") ||
		strings.EqualFold(readStringMetadata(payload, "extraction_method"), "ocr")
}

func validBBox(box *contracts.ParseBoundingBox) bool {
	return box != nil && box.Right > box.Left && box.Bottom > box.Top
}

func nearPageEdge(box *contracts.ParseBoundingBox) bool {
	if box == nil {
		return false
	}
	return box.Top <= 0.16 || box.Bottom >= 0.92 || box.Left <= 0.04 || box.Right >= 0.96
}

func usefulShortLabel(content string) bool {
	value := strings.Trim(strings.ToLower(strings.TrimSpace(content)), " :：._-—")
	if value == "" {
		return false
	}
	switch value {
	case "to", "from", "date", "note", "fax", "tel", "phone", "no", "re", "cc", "attn":
		return true
	}
	return strings.HasSuffix(strings.TrimSpace(content), ":") || strings.HasSuffix(strings.TrimSpace(content), "：")
}

func isPunctuationOnly(content string) bool {
	hasPunctuation := false
	for _, r := range content {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			hasPunctuation = true
		}
	}
	return hasPunctuation
}

func readStringMetadata(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func isNoTextVisionMessage(normalized string) bool {
	lower := strings.ToLower(normalized)
	return strings.Contains(lower, "does not contain any text") ||
		strings.Contains(lower, "no textual content can be extracted") ||
		strings.Contains(lower, "qr code") && strings.Contains(lower, "no text")
}
