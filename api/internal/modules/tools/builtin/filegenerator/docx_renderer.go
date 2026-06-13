package filegenerator

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	docx "github.com/fumiama/go-docx"
)

const (
	docxDefaultFontFamily = "Arial"
	docxDefaultFontSize   = 12
	docxMaxBlocks         = 200
	docxMaxTableRows      = 200
	docxMaxTableCols      = 20
)

var docxHexColorPattern = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)
var docxSpacingAfterPattern = regexp.MustCompile(`<w:spacing([^>]*) w:val="([0-9]+)"([^>]*)>`)

type DocxDocumentSpec struct {
	Page         DocxPageSpec           `json:"page,omitempty"`
	DefaultStyle DocxParagraphStyleSpec `json:"default_style,omitempty"`
	Blocks       []DocxBlockSpec        `json:"blocks"`
}

type DocxPageSpec struct {
	Size        string          `json:"size,omitempty"`
	Orientation string          `json:"orientation,omitempty"`
	Margins     DocxMarginsSpec `json:"margins,omitempty"`
}

type DocxMarginsSpec struct {
	Top    *float64 `json:"top,omitempty"`
	Right  *float64 `json:"right,omitempty"`
	Bottom *float64 `json:"bottom,omitempty"`
	Left   *float64 `json:"left,omitempty"`
}

type DocxBlockSpec struct {
	Type         string                 `json:"type"`
	Level        int                    `json:"level,omitempty"`
	Text         string                 `json:"text,omitempty"`
	Runs         []DocxRunSpec          `json:"runs,omitempty"`
	Style        DocxParagraphStyleSpec `json:"style,omitempty"`
	Headers      []DocxTableCellSpec    `json:"headers,omitempty"`
	Rows         [][]DocxTableCellSpec  `json:"rows,omitempty"`
	ColumnWidths []float64              `json:"column_widths,omitempty"`
	Border       *bool                  `json:"border,omitempty"`
	HeaderBold   *bool                  `json:"header_bold,omitempty"`
	TableStyle   DocxTableStyleSpec     `json:"table_style,omitempty"`
}

type DocxRunSpec struct {
	Text string `json:"text"`
	DocxTextStyleSpec
}

type DocxTextStyleSpec struct {
	FontFamily   string      `json:"font_family,omitempty"`
	EastAsiaFont string      `json:"east_asia_font,omitempty"`
	AsciiFont    string      `json:"ascii_font,omitempty"`
	FontSize     *float64    `json:"font_size,omitempty"`
	Bold         *bool       `json:"bold,omitempty"`
	Italic       *bool       `json:"italic,omitempty"`
	Underline    interface{} `json:"underline,omitempty"`
	Color        string      `json:"color,omitempty"`
	Highlight    string      `json:"highlight,omitempty"`
}

type DocxParagraphStyleSpec struct {
	DocxTextStyleSpec
	Alignment       string   `json:"alignment,omitempty"`
	LineSpacing     *float64 `json:"line_spacing,omitempty"`
	SpaceBefore     *float64 `json:"space_before,omitempty"`
	SpaceAfter      *float64 `json:"space_after,omitempty"`
	IndentLeft      *float64 `json:"indent_left,omitempty"`
	FirstLineIndent *float64 `json:"first_line_indent,omitempty"`
}

type DocxTableStyleSpec struct {
	Alignment   string   `json:"alignment,omitempty"`
	Width       *float64 `json:"width,omitempty"`
	Border      *bool    `json:"border,omitempty"`
	BorderColor string   `json:"border_color,omitempty"`
}

type DocxTableCellSpec struct {
	Text            string                 `json:"text,omitempty"`
	Runs            []DocxRunSpec          `json:"runs,omitempty"`
	Style           DocxParagraphStyleSpec `json:"style,omitempty"`
	BackgroundColor string                 `json:"background_color,omitempty"`
	VerticalAlign   string                 `json:"vertical_align,omitempty"`
	ColSpan         int                    `json:"col_span,omitempty"`
}

func (c *DocxTableCellSpec) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		c.Text = text
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		c.Text = number.String()
		return nil
	}
	var raw struct {
		Text            string                 `json:"text,omitempty"`
		Runs            []DocxRunSpec          `json:"runs,omitempty"`
		Style           DocxParagraphStyleSpec `json:"style,omitempty"`
		BackgroundColor string                 `json:"background_color,omitempty"`
		VerticalAlign   string                 `json:"vertical_align,omitempty"`
		ColSpan         int                    `json:"col_span,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Text = raw.Text
	c.Runs = raw.Runs
	c.Style = raw.Style
	c.BackgroundColor = raw.BackgroundColor
	c.VerticalAlign = raw.VerticalAlign
	c.ColSpan = raw.ColSpan
	return nil
}

func parseDocxDocumentSpec(raw string) (*DocxDocumentSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("document is required")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var spec DocxDocumentSpec
	if err := decoder.Decode(&spec); err != nil {
		return nil, fmt.Errorf("document must be valid DOCX JSON: %w", err)
	}
	if err := normalizeAndValidateDocxSpec(&spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

func renderDocx(content string) ([]byte, error) {
	doc := docx.New().WithDefaultTheme()
	for _, line := range splitDocumentLines(content) {
		doc.AddParagraph().AddText(line)
	}
	return writeDocxToBytes(doc)
}

func renderRichDocx(spec *DocxDocumentSpec) ([]byte, error) {
	if spec == nil {
		return nil, fmt.Errorf("document spec is required")
	}
	doc := docx.New().WithDefaultTheme()
	for idx := range spec.Blocks {
		block := spec.Blocks[idx]
		switch block.Type {
		case "heading":
			renderDocxHeading(doc, spec, block)
		case "paragraph":
			renderDocxParagraph(doc.AddParagraph(), spec.DefaultStyle, block.Style, block.Text, block.Runs)
		case "table":
			renderDocxTable(doc, spec, block)
		case "page_break":
			doc.AddParagraph().AddPageBreaks()
		default:
			return nil, fmt.Errorf("unsupported DOCX block type: %s", block.Type)
		}
	}
	appendDocxSection(doc, spec.Page)
	return writeDocxToBytes(doc)
}

func writeDocxToBytes(doc *docx.Docx) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("failed to render DOCX: %w", err)
	}
	data, err := normalizeDocxSpacingAfter(buf.Bytes())
	if err != nil {
		return nil, err
	}
	return data, nil
}

func normalizeAndValidateDocxSpec(spec *DocxDocumentSpec) error {
	if len(spec.Blocks) == 0 {
		return fmt.Errorf("document.blocks must include at least one block")
	}
	if len(spec.Blocks) > docxMaxBlocks {
		return fmt.Errorf("document.blocks exceeds maximum of %d", docxMaxBlocks)
	}
	spec.Page.Size = normalizeDefault(strings.ToLower(spec.Page.Size), "a4")
	spec.Page.Orientation = normalizeDefault(strings.ToLower(spec.Page.Orientation), "portrait")
	if spec.Page.Size != "a4" {
		return fmt.Errorf("unsupported DOCX page size: %s", spec.Page.Size)
	}
	if spec.Page.Orientation != "portrait" && spec.Page.Orientation != "landscape" {
		return fmt.Errorf("unsupported DOCX page orientation: %s", spec.Page.Orientation)
	}
	if err := validateDocxParagraphStyle(spec.DefaultStyle); err != nil {
		return fmt.Errorf("default_style: %w", err)
	}
	for idx := range spec.Blocks {
		block := &spec.Blocks[idx]
		block.Type = strings.ToLower(strings.TrimSpace(block.Type))
		switch block.Type {
		case "heading":
			block.Runs = compactDocxRuns(block.Runs)
			if block.Level < 1 {
				block.Level = 1
			}
			if block.Level > 3 {
				return fmt.Errorf("blocks[%d].level must be between 1 and 3", idx)
			}
			if strings.TrimSpace(block.Text) == "" && len(block.Runs) == 0 {
				return fmt.Errorf("blocks[%d] heading requires text or non-empty runs", idx)
			}
			if err := validateDocxParagraphStyle(block.Style); err != nil {
				return fmt.Errorf("blocks[%d].style: %w", idx, err)
			}
			if err := validateDocxRuns(block.Runs); err != nil {
				return fmt.Errorf("blocks[%d].runs: %w", idx, err)
			}
		case "paragraph":
			block.Runs = compactDocxRuns(block.Runs)
			if strings.TrimSpace(block.Text) == "" && len(block.Runs) == 0 {
				return fmt.Errorf("blocks[%d] paragraph requires text or non-empty runs", idx)
			}
			if err := validateDocxParagraphStyle(block.Style); err != nil {
				return fmt.Errorf("blocks[%d].style: %w", idx, err)
			}
			if err := validateDocxRuns(block.Runs); err != nil {
				return fmt.Errorf("blocks[%d].runs: %w", idx, err)
			}
		case "table":
			if len(block.Headers) == 0 && len(block.Rows) == 0 {
				return fmt.Errorf("blocks[%d] table requires headers or rows", idx)
			}
			if len(block.Rows) > docxMaxTableRows {
				return fmt.Errorf("blocks[%d].rows exceeds maximum of %d", idx, docxMaxTableRows)
			}
			if err := validateDocxTableBlock(idx, block); err != nil {
				return err
			}
		case "page_break":
		default:
			return fmt.Errorf("unsupported DOCX block type: %s", block.Type)
		}
	}
	return nil
}

func validateDocxTableBlock(blockIndex int, block *DocxBlockSpec) error {
	colCount := len(block.Headers)
	for _, row := range block.Rows {
		if len(row) > colCount {
			colCount = len(row)
		}
	}
	if colCount == 0 {
		return fmt.Errorf("blocks[%d] table requires at least one column", blockIndex)
	}
	if colCount > docxMaxTableCols {
		return fmt.Errorf("blocks[%d] table columns exceed maximum of %d", blockIndex, docxMaxTableCols)
	}
	if len(block.ColumnWidths) > 0 && len(block.ColumnWidths) != colCount {
		return fmt.Errorf("blocks[%d].column_widths must match table column count", blockIndex)
	}
	if err := validateDocxTableCells(block.Headers); err != nil {
		return fmt.Errorf("blocks[%d].headers: %w", blockIndex, err)
	}
	for rowIndex, row := range block.Rows {
		if len(row) != colCount {
			return fmt.Errorf("blocks[%d].rows[%d] has %d cells; expected %d", blockIndex, rowIndex, len(row), colCount)
		}
		if err := validateDocxTableCells(row); err != nil {
			return fmt.Errorf("blocks[%d].rows[%d]: %w", blockIndex, rowIndex, err)
		}
	}
	if block.TableStyle.Alignment != "" {
		if _, err := docxJustificationValue(block.TableStyle.Alignment); err != nil {
			return fmt.Errorf("blocks[%d].table_style.alignment: %w", blockIndex, err)
		}
	}
	if block.TableStyle.BorderColor != "" {
		if _, err := normalizeDocxColor(block.TableStyle.BorderColor); err != nil {
			return fmt.Errorf("blocks[%d].table_style.border_color: %w", blockIndex, err)
		}
	}
	if block.TableStyle.Width != nil && *block.TableStyle.Width < 0 {
		return fmt.Errorf("blocks[%d].table_style.width cannot be negative", blockIndex)
	}
	return nil
}

func validateDocxTableCells(cells []DocxTableCellSpec) error {
	for idx := range cells {
		cell := &cells[idx]
		cell.Runs = compactDocxRuns(cell.Runs)
		if strings.TrimSpace(cell.Text) == "" && len(cell.Runs) == 0 {
			continue
		}
		if err := validateDocxParagraphStyle(cell.Style); err != nil {
			return fmt.Errorf("cell[%d].style: %w", idx, err)
		}
		if err := validateDocxRuns(cell.Runs); err != nil {
			return fmt.Errorf("cell[%d].runs: %w", idx, err)
		}
		if cell.BackgroundColor != "" {
			if _, err := normalizeDocxColor(cell.BackgroundColor); err != nil {
				return fmt.Errorf("cell[%d].background_color: %w", idx, err)
			}
		}
		if cell.VerticalAlign != "" {
			if _, err := docxVerticalAlignValue(cell.VerticalAlign); err != nil {
				return fmt.Errorf("cell[%d].vertical_align: %w", idx, err)
			}
		}
		if cell.ColSpan < 0 {
			return fmt.Errorf("cell[%d].col_span cannot be negative", idx)
		}
	}
	return nil
}

func validateDocxRuns(runs []DocxRunSpec) error {
	for idx := range runs {
		if err := validateDocxTextStyle(runs[idx].DocxTextStyleSpec); err != nil {
			return fmt.Errorf("run[%d]: %w", idx, err)
		}
	}
	return nil
}

func compactDocxRuns(runs []DocxRunSpec) []DocxRunSpec {
	if len(runs) == 0 {
		return runs
	}
	out := runs[:0]
	for _, run := range runs {
		if strings.TrimSpace(run.Text) == "" {
			continue
		}
		out = append(out, run)
	}
	return out
}

func validateDocxParagraphStyle(style DocxParagraphStyleSpec) error {
	if err := validateDocxTextStyle(style.DocxTextStyleSpec); err != nil {
		return err
	}
	if style.Alignment != "" {
		if _, err := docxJustificationValue(style.Alignment); err != nil {
			return err
		}
	}
	for name, value := range map[string]*float64{
		"line_spacing":      style.LineSpacing,
		"space_before":      style.SpaceBefore,
		"space_after":       style.SpaceAfter,
		"indent_left":       style.IndentLeft,
		"first_line_indent": style.FirstLineIndent,
	} {
		if value != nil && *value < 0 {
			return fmt.Errorf("%s cannot be negative", name)
		}
	}
	return nil
}

func validateDocxTextStyle(style DocxTextStyleSpec) error {
	if style.FontSize != nil && (*style.FontSize < 1 || *style.FontSize > 200) {
		return fmt.Errorf("font_size must be between 1 and 200")
	}
	if style.Color != "" {
		if _, err := normalizeDocxColor(style.Color); err != nil {
			return fmt.Errorf("color: %w", err)
		}
	}
	if style.Highlight != "" && !isDocxHighlightValue(style.Highlight) {
		return fmt.Errorf("unsupported highlight: %s", style.Highlight)
	}
	if _, err := docxUnderlineValue(style.Underline); err != nil {
		return err
	}
	return nil
}

func renderDocxHeading(doc *docx.Docx, spec *DocxDocumentSpec, block DocxBlockSpec) {
	style := block.Style
	if style.FontSize == nil {
		size := float64(map[int]int{1: 18, 2: 16, 3: 14}[block.Level])
		style.FontSize = &size
	}
	if style.Bold == nil {
		bold := true
		style.Bold = &bold
	}
	if style.Alignment == "" && block.Level == 1 {
		style.Alignment = "center"
	}
	renderDocxParagraph(doc.AddParagraph(), spec.DefaultStyle, style, block.Text, block.Runs)
}

func renderDocxParagraph(p *docx.Paragraph, defaultStyle DocxParagraphStyleSpec, style DocxParagraphStyleSpec, text string, runs []DocxRunSpec) {
	paragraphStyle := mergeDocxParagraphStyle(defaultStyle, style)
	applyDocxParagraphStyle(p, paragraphStyle)
	if len(runs) == 0 {
		runs = []DocxRunSpec{{Text: text}}
	}
	for _, runSpec := range runs {
		run := p.AddText(runSpec.Text)
		applyDocxRunStyle(run, mergeDocxTextStyle(paragraphStyle.DocxTextStyleSpec, runSpec.DocxTextStyleSpec))
	}
}

func renderDocxTable(doc *docx.Docx, spec *DocxDocumentSpec, block DocxBlockSpec) {
	rowCount := len(block.Rows)
	if len(block.Headers) > 0 {
		rowCount++
	}
	colCount := len(block.Headers)
	for _, row := range block.Rows {
		if len(row) > colCount {
			colCount = len(row)
		}
	}
	rowHeights := make([]int64, rowCount)
	colWidths := make([]int64, colCount)
	for idx, width := range block.ColumnWidths {
		colWidths[idx] = int64(pointsToTwips(width))
	}
	var tableWidth int64
	if block.TableStyle.Width != nil {
		tableWidth = int64(pointsToTwips(*block.TableStyle.Width))
	}
	borderColor, _ := normalizeDocxColor(block.TableStyle.BorderColor)
	if borderColor == "" {
		borderColor = "000000"
	}
	table := doc.AddTableTwips(rowHeights, colWidths, tableWidth, &docx.APITableBorderColors{
		Top: borderColor, Left: borderColor, Bottom: borderColor, Right: borderColor, InsideH: borderColor, InsideV: borderColor,
	})
	if !docxTableBorderEnabled(block) {
		noBorder := &docx.WTableBorder{Val: "nil", Size: 0, Space: 0, Color: "auto"}
		table.TableProperties.TableBorders = &docx.WTableBorders{
			Top: noBorder, Left: noBorder, Bottom: noBorder, Right: noBorder, InsideH: noBorder, InsideV: noBorder,
		}
	}
	if block.TableStyle.Alignment != "" {
		if jc, err := docxJustificationValue(block.TableStyle.Alignment); err == nil {
			table.Justification(jc)
		}
	}
	rowOffset := 0
	if len(block.Headers) > 0 {
		headerStyle := block.Style
		if headerStyle.Bold == nil {
			bold := true
			if block.HeaderBold != nil {
				bold = *block.HeaderBold
			}
			headerStyle.Bold = &bold
		}
		renderDocxTableRow(table.TableRows[0], spec.DefaultStyle, headerStyle, block.Headers)
		rowOffset = 1
	}
	for rowIndex, row := range block.Rows {
		renderDocxTableRow(table.TableRows[rowIndex+rowOffset], spec.DefaultStyle, block.Style, row)
	}
}

func docxTableBorderEnabled(block DocxBlockSpec) bool {
	if block.TableStyle.Border != nil {
		return *block.TableStyle.Border
	}
	if block.Border != nil {
		return *block.Border
	}
	return true
}

func renderDocxTableRow(row *docx.WTableRow, defaultStyle DocxParagraphStyleSpec, rowStyle DocxParagraphStyleSpec, cells []DocxTableCellSpec) {
	for idx := range cells {
		cell := cells[idx]
		target := row.TableCells[idx]
		if cell.BackgroundColor != "" {
			if fill, err := normalizeDocxColor(cell.BackgroundColor); err == nil {
				target.Shade("clear", "auto", fill)
			}
		}
		if cell.VerticalAlign != "" {
			if val, err := docxVerticalAlignValue(cell.VerticalAlign); err == nil {
				target.TableCellProperties.VAlign = &docx.WVerticalAlignment{Val: val}
			}
		}
		if cell.ColSpan > 1 {
			target.TableCellProperties.GridSpan = &docx.WGridSpan{Val: cell.ColSpan}
		}
		style := mergeDocxParagraphStyle(rowStyle, cell.Style)
		renderDocxParagraph(target.AddParagraph(), defaultStyle, style, cell.Text, cell.Runs)
	}
}

func appendDocxSection(doc *docx.Docx, page DocxPageSpec) {
	width, height := 11906, 16838
	if page.Orientation == "landscape" {
		width, height = height, width
	}
	doc.Document.Body.Items = append(doc.Document.Body.Items, &docx.SectPr{
		PgSz: &docx.PgSz{W: width, H: height},
		PgMar: &docx.PgMar{
			Top:    pointsToTwips(valueOrDefault(page.Margins.Top, 72)),
			Right:  pointsToTwips(valueOrDefault(page.Margins.Right, 72)),
			Bottom: pointsToTwips(valueOrDefault(page.Margins.Bottom, 72)),
			Left:   pointsToTwips(valueOrDefault(page.Margins.Left, 72)),
			Header: 720,
			Footer: 720,
			Gutter: 0,
		},
		Cols:    &docx.Cols{Space: 720},
		DocGrid: &docx.DocGrid{Type: "lines", LinePitch: 312},
	})
}

func applyDocxParagraphStyle(p *docx.Paragraph, style DocxParagraphStyleSpec) {
	if style.Alignment != "" {
		if jc, err := docxJustificationValue(style.Alignment); err == nil {
			p.Justification(jc)
		}
	}
	if p.Properties == nil {
		p.Properties = &docx.ParagraphProperties{}
	}
	if style.LineSpacing != nil || style.SpaceBefore != nil || style.SpaceAfter != nil {
		p.Properties.Spacing = &docx.Spacing{
			Val:      pointsToTwips(valueOrDefault(style.SpaceAfter, 0)),
			Before:   pointsToTwips(valueOrDefault(style.SpaceBefore, 0)),
			Line:     int(math.Round(valueOrDefault(style.LineSpacing, 1) * 240)),
			LineRule: "auto",
		}
	}
	if style.IndentLeft != nil || style.FirstLineIndent != nil {
		p.Properties.Ind = &docx.Ind{
			Left:      pointsToTwips(valueOrDefault(style.IndentLeft, 0)),
			FirstLine: pointsToTwips(valueOrDefault(style.FirstLineIndent, 0)),
		}
	}
}

func applyDocxRunStyle(run *docx.Run, style DocxTextStyleSpec) {
	font := normalizeDefault(style.FontFamily, docxDefaultFontFamily)
	eastAsia := normalizeDefault(style.EastAsiaFont, font)
	ascii := normalizeDefault(style.AsciiFont, font)
	run.Font(ascii, eastAsia, ascii, "eastAsia")
	size := docxHalfPointSize(valueOrDefault(style.FontSize, docxDefaultFontSize))
	run.Size(size).SizeCs(size)
	if valueOrDefaultBool(style.Bold, false) {
		run.Bold()
	}
	if valueOrDefaultBool(style.Italic, false) {
		run.Italic()
	}
	if underline, err := docxUnderlineValue(style.Underline); err == nil && underline != "" {
		run.Underline(underline)
	}
	if style.Color != "" {
		if color, err := normalizeDocxColor(style.Color); err == nil {
			run.Color(color)
		}
	}
	if style.Highlight != "" {
		run.Highlight(style.Highlight)
	}
}

func mergeDocxParagraphStyle(base, override DocxParagraphStyleSpec) DocxParagraphStyleSpec {
	out := base
	out.DocxTextStyleSpec = mergeDocxTextStyle(base.DocxTextStyleSpec, override.DocxTextStyleSpec)
	if override.Alignment != "" {
		out.Alignment = override.Alignment
	}
	if override.LineSpacing != nil {
		out.LineSpacing = override.LineSpacing
	}
	if override.SpaceBefore != nil {
		out.SpaceBefore = override.SpaceBefore
	}
	if override.SpaceAfter != nil {
		out.SpaceAfter = override.SpaceAfter
	}
	if override.IndentLeft != nil {
		out.IndentLeft = override.IndentLeft
	}
	if override.FirstLineIndent != nil {
		out.FirstLineIndent = override.FirstLineIndent
	}
	return out
}

func mergeDocxTextStyle(styles ...DocxTextStyleSpec) DocxTextStyleSpec {
	var out DocxTextStyleSpec
	for _, style := range styles {
		if style.FontFamily != "" {
			out.FontFamily = style.FontFamily
		}
		if style.EastAsiaFont != "" {
			out.EastAsiaFont = style.EastAsiaFont
		}
		if style.AsciiFont != "" {
			out.AsciiFont = style.AsciiFont
		}
		if style.FontSize != nil {
			out.FontSize = style.FontSize
		}
		if style.Bold != nil {
			out.Bold = style.Bold
		}
		if style.Italic != nil {
			out.Italic = style.Italic
		}
		if style.Underline != nil {
			out.Underline = style.Underline
		}
		if style.Color != "" {
			out.Color = style.Color
		}
		if style.Highlight != "" {
			out.Highlight = style.Highlight
		}
	}
	return out
}

func docxJustificationValue(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "left", "start":
		return "start", nil
	case "center", "centre":
		return "center", nil
	case "right", "end":
		return "end", nil
	case "justify", "both":
		return "both", nil
	case "distribute":
		return "distribute", nil
	default:
		return "", fmt.Errorf("unsupported alignment: %s", raw)
	}
}

func docxVerticalAlignValue(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "top":
		return "top", nil
	case "center", "middle":
		return "center", nil
	case "bottom":
		return "bottom", nil
	default:
		return "", fmt.Errorf("unsupported vertical alignment: %s", raw)
	}
}

func docxUnderlineValue(raw interface{}) (string, error) {
	switch value := raw.(type) {
	case nil:
		return "", nil
	case bool:
		if value {
			return "single", nil
		}
		return "", nil
	case string:
		value = strings.TrimSpace(value)
		if value == "" || value == "false" || value == "none" {
			return "", nil
		}
		for _, allowed := range []string{"single", "words", "double", "thick", "dotted", "dash", "dotDash", "dotDotDash", "wave", "dashLong", "wavyDouble"} {
			if value == allowed {
				return value, nil
			}
		}
		return "", fmt.Errorf("unsupported underline: %s", value)
	default:
		return "", fmt.Errorf("underline must be a boolean or string")
	}
}

func normalizeDocxColor(raw string) (string, error) {
	color := strings.TrimPrefix(strings.TrimSpace(raw), "#")
	if color == "" {
		return "", nil
	}
	if !docxHexColorPattern.MatchString(color) {
		return "", fmt.Errorf("expected RRGGBB hex color")
	}
	return strings.ToUpper(color), nil
}

func isDocxHighlightValue(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "black", "blue", "cyan", "green", "magenta", "red", "yellow", "white", "darkblue", "darkcyan", "darkgreen", "darkmagenta", "darkred", "darkyellow", "darkgray", "lightgray", "none":
		return true
	default:
		return false
	}
}

func docxHalfPointSize(points float64) string {
	return strconv.Itoa(int(math.Round(points * 2)))
}

func pointsToTwips(points float64) int {
	return int(math.Round(points * 20))
}

func valueOrDefault(value *float64, fallback float64) float64 {
	if value == nil {
		return fallback
	}
	return *value
}

func valueOrDefaultBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func normalizeDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func normalizeDocxSpacingAfter(data []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to inspect DOCX package: %w", err)
	}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for _, file := range reader.File {
		if err := copyDocxZipFile(writer, file); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to rewrite DOCX package: %w", err)
	}
	return out.Bytes(), nil
}

func copyDocxZipFile(writer *zip.Writer, file *zip.File) error {
	handle, err := file.Open()
	if err != nil {
		return fmt.Errorf("failed to open DOCX entry %s: %w", file.Name, err)
	}
	defer handle.Close()

	content, err := io.ReadAll(handle)
	if err != nil {
		return fmt.Errorf("failed to read DOCX entry %s: %w", file.Name, err)
	}
	if file.Name == "word/document.xml" {
		content = []byte(docxSpacingAfterPattern.ReplaceAllString(string(content), `<w:spacing$1 w:after="$2"$3>`))
	}
	header := file.FileHeader
	entry, err := writer.CreateHeader(&header)
	if err != nil {
		return fmt.Errorf("failed to create DOCX entry %s: %w", file.Name, err)
	}
	if _, err := entry.Write(content); err != nil {
		return fmt.Errorf("failed to write DOCX entry %s: %w", file.Name, err)
	}
	return nil
}
