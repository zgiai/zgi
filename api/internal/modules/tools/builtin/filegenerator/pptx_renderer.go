package filegenerator

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
)

const (
	pptxMaxSlides           = 80
	pptxMaxElementsPerSlide = 120
	pptxMaxTableRows        = 200
	pptxMaxTableCols        = 20
	pptxMaxSpecBytes        = 512 * 1024
	pptxWideSlideWidth      = 13.333
	pptxFourThreeSlideWidth = 10.0
	pptxSlideHeight         = 7.5
	pptxCoordinateBleed     = 2.0
	pptxContentGap          = 0.18
	pptxTitleX              = 0.6
	pptxTitleY              = 0.35
	pptxContentX            = 0.75
	pptxTextStartY          = 1.25
	pptxTableStartY         = 1.45
	pptxDefaultShapeY       = 1.2
	pptxDefaultTitleHeight  = 0.7
	pptxDefaultTextHeight   = 1.0
	pptxDefaultTableHeight  = 0.7
	pptxDefaultShapeWidth   = 2.0
	pptxDefaultShapeHeight  = 1.0
	pptxLayoutEpsilon       = 0.01
	pptxTextFitTolerance    = 1.08
	pptxTextHardWrapRatio   = 0.92
	pptxMinHardWrapUnits    = 8.0
)

var pptxHexColorPattern = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)
var pptxLanguagePattern = regexp.MustCompile(`^[A-Za-z]{2,8}(-[A-Za-z0-9]{1,8})*$`)

type PPTXDocumentSpec struct {
	Layout       string            `json:"layout,omitempty"`
	Language     string            `json:"language,omitempty"`
	DefaultStyle PPTXTextStyleSpec `json:"default_style,omitempty"`
	Slides       []PPTXSlideSpec   `json:"slides"`
}

type PPTXSlideSpec struct {
	BackgroundColor string            `json:"background_color,omitempty"`
	Elements        []PPTXElementSpec `json:"elements"`
}

type PPTXElementSpec struct {
	Type            string            `json:"type"`
	Text            string            `json:"text,omitempty"`
	X               *float64          `json:"x,omitempty"`
	Y               *float64          `json:"y,omitempty"`
	W               *float64          `json:"w,omitempty"`
	H               *float64          `json:"h,omitempty"`
	Style           PPTXTextStyleSpec `json:"style,omitempty"`
	Headers         []string          `json:"headers,omitempty"`
	Rows            [][]string        `json:"rows,omitempty"`
	ColumnWidths    []float64         `json:"column_widths,omitempty"`
	FillColor       string            `json:"fill_color,omitempty"`
	LineColor       string            `json:"line_color,omitempty"`
	BorderColor     string            `json:"border_color,omitempty"`
	HeaderFillColor string            `json:"header_fill_color,omitempty"`
	HeaderColor     string            `json:"header_color,omitempty"`
	RowFillColor    string            `json:"row_fill_color,omitempty"`
	Margin          *float64          `json:"margin,omitempty"`
	BreakLine       *bool             `json:"break_line,omitempty"`
	LineSpacing     *float64          `json:"line_spacing,omitempty"`
	Rotation        *float64          `json:"rotation,omitempty"`
	Transparency    *float64          `json:"transparency,omitempty"`
}

type PPTXTextStyleSpec struct {
	FontFace    string   `json:"font_face,omitempty"`
	FontFamily  string   `json:"font_family,omitempty"`
	FontSize    *float64 `json:"font_size,omitempty"`
	FontWeight  string   `json:"font_weight,omitempty"`
	Color       string   `json:"color,omitempty"`
	Bold        *bool    `json:"bold,omitempty"`
	Italic      *bool    `json:"italic,omitempty"`
	Underline   *bool    `json:"underline,omitempty"`
	Strike      *bool    `json:"strike,omitempty"`
	Align       string   `json:"align,omitempty"`
	Valign      string   `json:"valign,omitempty"`
	LineSpacing *float64 `json:"line_spacing,omitempty"`
	Margin      *float64 `json:"margin,omitempty"`
	BreakLine   *bool    `json:"break_line,omitempty"`
}

type pptxSlideSize struct {
	Width  float64
	Height float64
}

type pptxBox struct {
	X float64
	Y float64
	W float64
	H float64
}

type pptxPlacedContentBox struct {
	index int
	box   pptxBox
}

type pptxTextMeasure struct {
	EstimatedHeight  float64
	LineCapacity     float64
	LongestLineUnits float64
}

func parsePPTXDocumentSpec(raw string) (*PPTXDocumentSpec, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", fmt.Errorf("presentation is required")
	}
	if len(raw) > pptxMaxSpecBytes {
		return nil, "", fmt.Errorf("presentation exceeds %d bytes", pptxMaxSpecBytes)
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	var spec PPTXDocumentSpec
	if err := decoder.Decode(&spec); err != nil {
		return nil, "", fmt.Errorf("presentation must be valid PPTX JSON: %w", err)
	}
	if err := normalizeAndValidatePPTXSpec(&spec); err != nil {
		return nil, "", err
	}
	normalized, err := json.Marshal(spec)
	if err != nil {
		return nil, "", err
	}
	return &spec, string(normalized), nil
}

func normalizeAndValidatePPTXSpec(spec *PPTXDocumentSpec) error {
	if len(spec.Slides) == 0 {
		return fmt.Errorf("presentation.slides must include at least one slide")
	}
	if len(spec.Slides) > pptxMaxSlides {
		return fmt.Errorf("presentation.slides exceeds maximum of %d", pptxMaxSlides)
	}
	spec.Layout = normalizeDefault(strings.ToLower(strings.TrimSpace(spec.Layout)), "wide")
	if spec.Layout != "wide" && spec.Layout != "4:3" {
		return fmt.Errorf("unsupported PPTX layout: %s", spec.Layout)
	}
	spec.Language = normalizePPTXLanguage(spec.Language)
	if spec.Language != "" && !pptxLanguagePattern.MatchString(spec.Language) {
		return fmt.Errorf("presentation.language must be a valid BCP 47 language tag")
	}
	if err := normalizeAndValidatePPTXTextStyle(&spec.DefaultStyle); err != nil {
		return fmt.Errorf("default_style: %w", err)
	}
	slideSize := pptxSlideSizeForLayout(spec.Layout)
	for slideIdx := range spec.Slides {
		slide := &spec.Slides[slideIdx]
		slide.BackgroundColor = normalizeHexColor(slide.BackgroundColor)
		if slide.BackgroundColor != "" && !pptxHexColorPattern.MatchString(slide.BackgroundColor) {
			return fmt.Errorf("slides[%d].background_color: expected RRGGBB hex color", slideIdx)
		}
		if len(slide.Elements) == 0 {
			return fmt.Errorf("slides[%d].elements must include at least one element", slideIdx)
		}
		if len(slide.Elements) > pptxMaxElementsPerSlide {
			return fmt.Errorf("slides[%d].elements exceeds maximum of %d", slideIdx, pptxMaxElementsPerSlide)
		}
		for elementIdx := range slide.Elements {
			element := &slide.Elements[elementIdx]
			element.Type = strings.ToLower(strings.TrimSpace(element.Type))
			if err := validatePPTXElement(slideIdx, elementIdx, element); err != nil {
				return err
			}
		}
		if err := normalizeAndValidatePPTXSlideLayout(slideIdx, slide, spec.DefaultStyle, slideSize); err != nil {
			return err
		}
	}
	return nil
}

func pptxSlideSizeForLayout(layout string) pptxSlideSize {
	if layout == "4:3" {
		return pptxSlideSize{Width: pptxFourThreeSlideWidth, Height: pptxSlideHeight}
	}
	return pptxSlideSize{Width: pptxWideSlideWidth, Height: pptxSlideHeight}
}

func normalizePPTXLanguage(language string) string {
	language = strings.TrimSpace(language)
	if language == "" {
		return "en-US"
	}
	parts := strings.Split(language, "-")
	for idx, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return language
		}
		switch idx {
		case 0:
			parts[idx] = strings.ToLower(part)
		case 1:
			if len(part) == 2 {
				parts[idx] = strings.ToUpper(part)
			} else {
				parts[idx] = part
			}
		default:
			parts[idx] = part
		}
	}
	return strings.Join(parts, "-")
}

func validatePPTXElement(slideIdx int, elementIdx int, element *PPTXElementSpec) error {
	switch element.Type {
	case "title", "text":
		if strings.TrimSpace(element.Text) == "" {
			return fmt.Errorf("slides[%d].elements[%d].text is required", slideIdx, elementIdx)
		}
	case "table":
		if len(element.Headers) == 0 && len(element.Rows) == 0 {
			return fmt.Errorf("slides[%d].elements[%d].table requires headers or rows", slideIdx, elementIdx)
		}
		if len(element.Rows) > pptxMaxTableRows {
			return fmt.Errorf("slides[%d].elements[%d].rows exceeds maximum of %d", slideIdx, elementIdx, pptxMaxTableRows)
		}
		if len(element.Headers) > pptxMaxTableCols {
			return fmt.Errorf("slides[%d].elements[%d].headers exceeds maximum of %d", slideIdx, elementIdx, pptxMaxTableCols)
		}
		if len(element.ColumnWidths) > pptxMaxTableCols {
			return fmt.Errorf("slides[%d].elements[%d].column_widths exceeds maximum of %d", slideIdx, elementIdx, pptxMaxTableCols)
		}
		for rowIdx, row := range element.Rows {
			if len(row) > pptxMaxTableCols {
				return fmt.Errorf("slides[%d].elements[%d].rows[%d] exceeds maximum of %d columns", slideIdx, elementIdx, rowIdx, pptxMaxTableCols)
			}
		}
	case "shape":
		if element.FillColor == "" && element.LineColor == "" {
			return fmt.Errorf("slides[%d].elements[%d].shape requires fill_color or line_color", slideIdx, elementIdx)
		}
	default:
		return fmt.Errorf("unsupported PPTX element type: %s", element.Type)
	}
	if err := normalizeAndValidatePPTXTextStyle(&element.Style); err != nil {
		return fmt.Errorf("slides[%d].elements[%d].style: %w", slideIdx, elementIdx, err)
	}
	element.FillColor = normalizeHexColor(element.FillColor)
	element.LineColor = normalizeHexColor(element.LineColor)
	element.BorderColor = normalizeHexColor(element.BorderColor)
	element.HeaderFillColor = normalizeHexColor(element.HeaderFillColor)
	element.HeaderColor = normalizeHexColor(element.HeaderColor)
	element.RowFillColor = normalizeHexColor(element.RowFillColor)
	for _, color := range []struct {
		name  string
		value string
	}{
		{"fill_color", element.FillColor},
		{"line_color", element.LineColor},
		{"border_color", element.BorderColor},
		{"header_fill_color", element.HeaderFillColor},
		{"header_color", element.HeaderColor},
		{"row_fill_color", element.RowFillColor},
	} {
		if color.value != "" && !pptxHexColorPattern.MatchString(color.value) {
			return fmt.Errorf("slides[%d].elements[%d].%s: expected RRGGBB hex color", slideIdx, elementIdx, color.name)
		}
	}
	if element.Margin != nil && (*element.Margin < 0 || *element.Margin > 1) {
		return fmt.Errorf("slides[%d].elements[%d].margin must be between 0 and 1 inches", slideIdx, elementIdx)
	}
	if element.LineSpacing != nil && (*element.LineSpacing < 0.5 || *element.LineSpacing > 3) {
		return fmt.Errorf("slides[%d].elements[%d].line_spacing must be between 0.5 and 3", slideIdx, elementIdx)
	}
	if element.Rotation != nil && (*element.Rotation < -360 || *element.Rotation > 360) {
		return fmt.Errorf("slides[%d].elements[%d].rotation must be between -360 and 360 degrees", slideIdx, elementIdx)
	}
	if element.Transparency != nil && (*element.Transparency < 0 || *element.Transparency > 100) {
		return fmt.Errorf("slides[%d].elements[%d].transparency must be between 0 and 100", slideIdx, elementIdx)
	}
	for colIdx, width := range element.ColumnWidths {
		if width <= 0 || width > 20 {
			return fmt.Errorf("slides[%d].elements[%d].column_widths[%d] must be between 0 and 20 inches", slideIdx, elementIdx, colIdx)
		}
	}
	return nil
}

func normalizeAndValidatePPTXSlideLayout(slideIdx int, slide *PPTXSlideSpec, defaultStyle PPTXTextStyleSpec, size pptxSlideSize) error {
	cursorY := pptxTitleY
	placed := make([]pptxPlacedContentBox, 0, len(slide.Elements))
	for elementIdx := range slide.Elements {
		element := &slide.Elements[elementIdx]
		hasExplicitHeight := element.H != nil
		box := resolvedPPTXBox(element, defaultStyle, size, cursorY)
		if err := validatePPTXBox(slideIdx, elementIdx, box, size); err != nil {
			return err
		}
		if element.Type == "title" || element.Type == "text" {
			normalizePPTXTextWrapping(element, defaultStyle, box.W, element.Type == "title")
			if !hasExplicitHeight {
				box.H = maxFloat64(box.H, estimatePPTXTextHeight(element.Text, element, defaultStyle, box.W, element.Type == "title"))
			}
			if err := validatePPTXBox(slideIdx, elementIdx, box, size); err != nil {
				return err
			}
		}
		if isPPTXContentElement(element.Type) {
			if err := validatePPTXContentBox(slideIdx, elementIdx, box, size); err != nil {
				return err
			}
			if err := validatePPTXContentFits(slideIdx, elementIdx, element, defaultStyle, box); err != nil {
				return err
			}
			for _, previous := range placed {
				if pptxBoxesOverlap(previous.box, box) {
					return fmt.Errorf("slides[%d].elements[%d] overlaps elements[%d]; move one element or split content into another slide", slideIdx, elementIdx, previous.index)
				}
			}
			placed = append(placed, pptxPlacedContentBox{index: elementIdx, box: box})
			if nextY := box.Y + box.H + pptxContentGap; nextY > cursorY {
				cursorY = nextY
			}
		}
		applyPPTXBox(element, box)
	}
	return nil
}

func resolvedPPTXBox(element *PPTXElementSpec, defaultStyle PPTXTextStyleSpec, size pptxSlideSize, cursorY float64) pptxBox {
	box := defaultPPTXBox(element, defaultStyle, size, cursorY)
	if element.X != nil {
		box.X = *element.X
	}
	if element.Y != nil {
		box.Y = *element.Y
	}
	if element.W != nil {
		box.W = *element.W
	}
	if element.H != nil {
		box.H = *element.H
	} else {
		box.H = defaultPPTXHeight(element, defaultStyle, box.W)
	}
	return box
}

func defaultPPTXBox(element *PPTXElementSpec, defaultStyle PPTXTextStyleSpec, size pptxSlideSize, cursorY float64) pptxBox {
	switch element.Type {
	case "title":
		width := size.Width - 2*pptxTitleX
		return pptxBox{
			X: pptxTitleX,
			Y: maxFloat64(cursorY, pptxTitleY),
			W: width,
			H: defaultPPTXHeight(element, defaultStyle, width),
		}
	case "text":
		width := size.Width - 2*pptxContentX
		return pptxBox{
			X: pptxContentX,
			Y: maxFloat64(cursorY, pptxTextStartY),
			W: width,
			H: defaultPPTXHeight(element, defaultStyle, width),
		}
	case "table":
		width := size.Width - 2*pptxContentX
		return pptxBox{
			X: pptxContentX,
			Y: maxFloat64(cursorY, pptxTableStartY),
			W: width,
			H: defaultPPTXHeight(element, defaultStyle, width),
		}
	default:
		return pptxBox{
			X: pptxContentX,
			Y: maxFloat64(cursorY, pptxDefaultShapeY),
			W: pptxDefaultShapeWidth,
			H: pptxDefaultShapeHeight,
		}
	}
}

func defaultPPTXHeight(element *PPTXElementSpec, defaultStyle PPTXTextStyleSpec, width float64) float64 {
	switch element.Type {
	case "title":
		return maxFloat64(pptxDefaultTitleHeight, estimatePPTXTextHeight(element.Text, element, defaultStyle, width, true))
	case "text":
		return maxFloat64(pptxDefaultTextHeight, estimatePPTXTextHeight(element.Text, element, defaultStyle, width, false))
	case "table":
		return maxFloat64(pptxDefaultTableHeight, estimatePPTXTableHeight(element, defaultStyle))
	default:
		return pptxDefaultShapeHeight
	}
}

func estimatePPTXTextHeight(text string, element *PPTXElementSpec, defaultStyle PPTXTextStyleSpec, width float64, title bool) float64 {
	return measurePPTXText(text, element, defaultStyle, width, title).EstimatedHeight
}

func measurePPTXText(text string, element *PPTXElementSpec, defaultStyle PPTXTextStyleSpec, width float64, title bool) pptxTextMeasure {
	fontSize := pptxEffectiveFontSize(element.Style, defaultStyle, title)
	lineSpacing := pptxEffectiveLineSpacing(element, defaultStyle)
	margin := pptxEffectiveMargin(element, defaultStyle, 0.04)
	usableWidth := width - 2*margin
	if usableWidth < 0.2 {
		usableWidth = 0.2
	}
	unitWidth := fontSize / 72.0
	if unitWidth <= 0 {
		unitWidth = 12.0 / 72.0
	}
	lineCapacity := usableWidth / unitWidth
	if lineCapacity < 1 {
		lineCapacity = 1
	}
	lines := 0
	longestLineUnits := 0.0
	for _, paragraph := range strings.Split(text, "\n") {
		units := pptxTextUnits(paragraph)
		longestLineUnits = maxFloat64(longestLineUnits, units)
		lines += maxInt(1, int(math.Ceil(units/lineCapacity)))
	}
	lineHeight := unitWidth * 1.2 * lineSpacing
	return pptxTextMeasure{
		EstimatedHeight:  float64(lines)*lineHeight + 2*margin + 0.08,
		LineCapacity:     lineCapacity,
		LongestLineUnits: longestLineUnits,
	}
}

func normalizePPTXTextWrapping(element *PPTXElementSpec, defaultStyle PPTXTextStyleSpec, width float64, title bool) {
	measure := measurePPTXText(element.Text, element, defaultStyle, width, title)
	maxLineUnits := maxFloat64(pptxMinHardWrapUnits, measure.LineCapacity*pptxTextHardWrapRatio)
	if measure.LongestLineUnits <= maxLineUnits {
		return
	}
	text := strings.ReplaceAll(element.Text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	paragraphs := strings.Split(text, "\n")
	wrapped := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		wrapped = append(wrapped, wrapPPTXTextLine(paragraph, maxLineUnits)...)
	}
	element.Text = strings.Join(wrapped, "\n")
}

func wrapPPTXTextLine(text string, maxLineUnits float64) []string {
	if strings.TrimSpace(text) == "" {
		return []string{""}
	}
	lines := make([]string, 0, int(math.Ceil(pptxTextUnits(text)/maxLineUnits)))
	var builder strings.Builder
	lineUnits := 0.0
	for _, r := range text {
		unit := pptxRuneUnits(r)
		if lineUnits > 0 && lineUnits+unit > maxLineUnits {
			line := strings.TrimRightFunc(builder.String(), unicode.IsSpace)
			if line != "" {
				lines = append(lines, line)
			}
			builder.Reset()
			lineUnits = 0
			if unicode.IsSpace(r) {
				continue
			}
		}
		if lineUnits == 0 && unicode.IsSpace(r) {
			continue
		}
		builder.WriteRune(r)
		lineUnits += unit
	}
	line := strings.TrimRightFunc(builder.String(), unicode.IsSpace)
	if line != "" {
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func estimatePPTXTableHeight(element *PPTXElementSpec, defaultStyle PPTXTextStyleSpec) float64 {
	rowCount := len(element.Rows)
	if len(element.Headers) > 0 {
		rowCount++
	}
	rowCount = maxInt(1, rowCount)
	fontSize := pptxEffectiveFontSize(element.Style, defaultStyle, false)
	if element.Style.FontSize == nil && defaultStyle.FontSize == nil {
		fontSize = 12
	}
	rowHeight := maxFloat64(0.3, (fontSize/72.0)*1.7)
	margin := pptxEffectiveMargin(element, defaultStyle, 0.05)
	return float64(rowCount)*rowHeight + 2*margin
}

func pptxEffectiveFontSize(style PPTXTextStyleSpec, defaultStyle PPTXTextStyleSpec, title bool) float64 {
	if style.FontSize != nil {
		return *style.FontSize
	}
	if defaultStyle.FontSize != nil {
		return *defaultStyle.FontSize
	}
	if title {
		return 30
	}
	return 18
}

func pptxEffectiveLineSpacing(element *PPTXElementSpec, defaultStyle PPTXTextStyleSpec) float64 {
	if element.LineSpacing != nil {
		return *element.LineSpacing
	}
	if element.Style.LineSpacing != nil {
		return *element.Style.LineSpacing
	}
	if defaultStyle.LineSpacing != nil {
		return *defaultStyle.LineSpacing
	}
	return 1.1
}

func pptxEffectiveMargin(element *PPTXElementSpec, defaultStyle PPTXTextStyleSpec, fallback float64) float64 {
	if element.Margin != nil {
		return *element.Margin
	}
	if element.Style.Margin != nil {
		return *element.Style.Margin
	}
	if defaultStyle.Margin != nil {
		return *defaultStyle.Margin
	}
	return fallback
}

func pptxTextUnits(text string) float64 {
	units := 0.0
	for _, r := range text {
		units += pptxRuneUnits(r)
	}
	return maxFloat64(1, units)
}

func pptxRuneUnits(r rune) float64 {
	switch {
	case r == '\t':
		return 1.2
	case r == ' ':
		return 0.35
	case r <= 0x7f:
		return 0.55
	case isCJKRune(r):
		return 1.0
	default:
		return 0.8
	}
}

func isCJKRune(r rune) bool {
	return (r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0x3040 && r <= 0x30FF) ||
		(r >= 0xAC00 && r <= 0xD7AF)
}

func validatePPTXBox(slideIdx int, elementIdx int, box pptxBox, size pptxSlideSize) error {
	if box.X < -pptxCoordinateBleed || box.X > size.Width+pptxCoordinateBleed {
		return fmt.Errorf("slides[%d].elements[%d].x must be between %.2f and %.2f inches", slideIdx, elementIdx, -pptxCoordinateBleed, size.Width+pptxCoordinateBleed)
	}
	if box.Y < -pptxCoordinateBleed || box.Y > size.Height+pptxCoordinateBleed {
		return fmt.Errorf("slides[%d].elements[%d].y must be between %.2f and %.2f inches", slideIdx, elementIdx, -pptxCoordinateBleed, size.Height+pptxCoordinateBleed)
	}
	if box.W <= 0 || box.W > 20 {
		return fmt.Errorf("slides[%d].elements[%d].w must be greater than 0 and no more than 20 inches", slideIdx, elementIdx)
	}
	if box.H <= 0 || box.H > 40 {
		return fmt.Errorf("slides[%d].elements[%d].h must be greater than 0 and no more than 40 inches", slideIdx, elementIdx)
	}
	if box.X+box.W <= 0 || box.Y+box.H <= 0 || box.X >= size.Width || box.Y >= size.Height {
		return fmt.Errorf("slides[%d].elements[%d] must be at least partially visible on the slide", slideIdx, elementIdx)
	}
	return nil
}

func validatePPTXContentBox(slideIdx int, elementIdx int, box pptxBox, size pptxSlideSize) error {
	if box.X < -pptxLayoutEpsilon || box.Y < -pptxLayoutEpsilon ||
		box.X+box.W > size.Width+pptxLayoutEpsilon ||
		box.Y+box.H > size.Height+pptxLayoutEpsilon {
		return fmt.Errorf("slides[%d].elements[%d].content box must fit within the slide bounds; split dense content into another slide", slideIdx, elementIdx)
	}
	return nil
}

func validatePPTXContentFits(slideIdx int, elementIdx int, element *PPTXElementSpec, defaultStyle PPTXTextStyleSpec, box pptxBox) error {
	switch element.Type {
	case "title", "text":
		measure := measurePPTXText(element.Text, element, defaultStyle, box.W, element.Type == "title")
		if measure.EstimatedHeight > box.H+pptxFitHeightSlack(box.H) {
			return fmt.Errorf("slides[%d].elements[%d].text does not fit in its box; estimated height %.2f inches exceeds box height %.2f inches, split dense content into another slide or increase h", slideIdx, elementIdx, measure.EstimatedHeight, box.H)
		}
		if measure.LongestLineUnits > measure.LineCapacity+pptxLayoutEpsilon {
			return fmt.Errorf("slides[%d].elements[%d].text line is too long for its box; add line breaks or increase w", slideIdx, elementIdx)
		}
	case "table":
		requiredHeight := estimatePPTXTableHeight(element, defaultStyle)
		if requiredHeight > box.H+pptxFitHeightSlack(box.H) {
			return fmt.Errorf("slides[%d].elements[%d].table does not fit in its box; estimated height %.2f inches exceeds box height %.2f inches, split rows into another slide or increase h", slideIdx, elementIdx, requiredHeight, box.H)
		}
	}
	return nil
}

func pptxFitHeightSlack(height float64) float64 {
	return maxFloat64(0.08, height*(pptxTextFitTolerance-1))
}

func pptxBoxesOverlap(left pptxBox, right pptxBox) bool {
	return left.X < right.X+right.W-pptxLayoutEpsilon &&
		right.X < left.X+left.W-pptxLayoutEpsilon &&
		left.Y < right.Y+right.H-pptxLayoutEpsilon &&
		right.Y < left.Y+left.H-pptxLayoutEpsilon
}

func isPPTXContentElement(elementType string) bool {
	switch elementType {
	case "title", "text", "table":
		return true
	default:
		return false
	}
}

func applyPPTXBox(element *PPTXElementSpec, box pptxBox) {
	element.X = float64Ptr(box.X)
	element.Y = float64Ptr(box.Y)
	element.W = float64Ptr(box.W)
	element.H = float64Ptr(box.H)
}

func float64Ptr(value float64) *float64 {
	return &value
}

func maxFloat64(left float64, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func normalizeAndValidatePPTXTextStyle(style *PPTXTextStyleSpec) error {
	if strings.TrimSpace(style.FontFace) == "" && strings.TrimSpace(style.FontFamily) != "" {
		style.FontFace = style.FontFamily
	}
	style.FontFace = strings.TrimSpace(style.FontFace)
	style.FontFamily = strings.TrimSpace(style.FontFamily)
	if style.FontSize != nil && (*style.FontSize < 4 || *style.FontSize > 96) {
		return fmt.Errorf("font_size must be between 4 and 96")
	}
	if style.FontWeight != "" {
		style.FontWeight = strings.ToLower(strings.TrimSpace(style.FontWeight))
		switch style.FontWeight {
		case "normal", "regular", "medium", "semibold", "bold", "bolder", "400", "500", "600", "700", "800", "900":
		default:
			return fmt.Errorf("unsupported font_weight: %s", style.FontWeight)
		}
		if style.Bold == nil {
			switch style.FontWeight {
			case "bold", "bolder", "semibold", "600", "700", "800", "900":
				value := true
				style.Bold = &value
			}
		}
	}
	style.Color = normalizeHexColor(style.Color)
	if style.Color != "" && !pptxHexColorPattern.MatchString(style.Color) {
		return fmt.Errorf("color: expected RRGGBB hex color")
	}
	if style.Align != "" {
		style.Align = strings.ToLower(strings.TrimSpace(style.Align))
		switch style.Align {
		case "left", "center", "right", "justify":
		default:
			return fmt.Errorf("unsupported align: %s", style.Align)
		}
	}
	if style.Valign != "" {
		style.Valign = strings.ToLower(strings.TrimSpace(style.Valign))
		switch style.Valign {
		case "top", "mid", "bottom":
		default:
			return fmt.Errorf("unsupported valign: %s", style.Valign)
		}
	}
	if style.LineSpacing != nil && (*style.LineSpacing < 0.5 || *style.LineSpacing > 3) {
		return fmt.Errorf("line_spacing must be between 0.5 and 3")
	}
	if style.Margin != nil && (*style.Margin < 0 || *style.Margin > 1) {
		return fmt.Errorf("margin must be between 0 and 1 inches")
	}
	return nil
}

func normalizeHexColor(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "#")
	return strings.ToUpper(value)
}
