package filegenerator

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	pptxMaxSlides           = 80
	pptxMaxElementsPerSlide = 120
	pptxMaxTableRows        = 200
	pptxMaxTableCols        = 20
	pptxMaxSpecBytes        = 512 * 1024
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
	}
	return nil
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
	if err := validatePPTXBox(slideIdx, elementIdx, element); err != nil {
		return err
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

func validatePPTXBox(slideIdx int, elementIdx int, element *PPTXElementSpec) error {
	for _, dimension := range []struct {
		name  string
		value *float64
	}{
		{"x", element.X},
		{"y", element.Y},
		{"w", element.W},
		{"h", element.H},
	} {
		if dimension.value == nil {
			continue
		}
		if *dimension.value < 0 || *dimension.value > 20 {
			return fmt.Errorf("slides[%d].elements[%d].%s must be between 0 and 20 inches", slideIdx, elementIdx, dimension.name)
		}
	}
	return nil
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
