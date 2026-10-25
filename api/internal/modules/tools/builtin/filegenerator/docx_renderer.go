package filegenerator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const (
	docxDefaultFontFamily = "Arial"
	docxDefaultFontSize   = 12
	docxMaxBlocks         = 200
	docxMaxTableRows      = 200
	docxMaxTableCols      = 20

	docxA4WidthTwips       = 11906
	docxA4HeightTwips      = 16838
	docxDefaultMarginPoint = 72
)

var docxHexColorPattern = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)

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
	return parseDocxDocumentSpecValue(raw)
}

func parseDocxDocumentSpecValue(raw interface{}) (*DocxDocumentSpec, error) {
	data, err := structuredObjectJSON(raw, "document", 0)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var spec DocxDocumentSpec
	if err := decoder.Decode(&spec); err != nil {
		return nil, fmt.Errorf("document must be valid DOCX JSON: %w", err)
	}
	if err := requireJSONDecoderEOF(decoder, "document"); err != nil {
		return nil, err
	}
	if err := normalizeAndValidateDocxSpec(&spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

func renderDocx(content string) ([]byte, error) {
	var body strings.Builder
	for _, line := range splitDocumentLines(content) {
		body.WriteString("<w:p><w:r>")
		writeDocxRunText(&body, line)
		body.WriteString("</w:r></w:p>")
	}
	writeDocxSection(&body, DocxPageSpec{Orientation: "portrait"})
	return writeDocxPackage(body.String())
}

func renderRichDocx(spec *DocxDocumentSpec) ([]byte, error) {
	if spec == nil {
		return nil, fmt.Errorf("document spec is required")
	}
	var body strings.Builder
	for idx := range spec.Blocks {
		block := spec.Blocks[idx]
		switch block.Type {
		case "heading":
			renderDocxHeading(&body, spec, block)
		case "paragraph":
			renderDocxParagraph(&body, spec.DefaultStyle, block.Style, "", block.Text, block.Runs)
		case "table":
			renderDocxTable(&body, spec, block)
		case "page_break":
			body.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
		default:
			return nil, fmt.Errorf("unsupported DOCX block type: %s", block.Type)
		}
	}
	writeDocxSection(&body, spec.Page)
	return writeDocxPackage(body.String())
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

func renderDocxHeading(body *strings.Builder, spec *DocxDocumentSpec, block DocxBlockSpec) {
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
	styleID := "Heading" + strconv.Itoa(block.Level)
	renderDocxParagraph(body, spec.DefaultStyle, style, styleID, block.Text, block.Runs)
}

func renderDocxParagraph(body *strings.Builder, defaultStyle DocxParagraphStyleSpec, style DocxParagraphStyleSpec, styleID string, text string, runs []DocxRunSpec) {
	paragraphStyle := mergeDocxParagraphStyle(defaultStyle, style)
	body.WriteString("<w:p>")
	writeDocxParagraphProperties(body, paragraphStyle, styleID)
	if len(runs) == 0 {
		runs = []DocxRunSpec{{Text: text}}
	}
	for _, runSpec := range runs {
		body.WriteString("<w:r>")
		writeDocxRunProperties(body, mergeDocxTextStyle(paragraphStyle.DocxTextStyleSpec, runSpec.DocxTextStyleSpec))
		writeDocxRunText(body, runSpec.Text)
		body.WriteString("</w:r>")
	}
	body.WriteString("</w:p>")
}

func renderDocxTable(body *strings.Builder, spec *DocxDocumentSpec, block DocxBlockSpec) {
	colCount := len(block.Headers)
	for _, row := range block.Rows {
		if len(row) > colCount {
			colCount = len(row)
		}
	}
	var tableWidth int
	if block.TableStyle.Width != nil {
		tableWidth = pointsToTwips(*block.TableStyle.Width)
	}
	colWidths := docxTableColumnWidths(spec.Page, block.ColumnWidths, colCount, tableWidth)

	body.WriteString("<w:tbl><w:tblPr>")
	if tableWidth > 0 {
		fmt.Fprintf(body, `<w:tblW w:w="%d" w:type="dxa"/>`, tableWidth)
	} else {
		body.WriteString(`<w:tblW w:w="0" w:type="auto"/>`)
	}
	if block.TableStyle.Alignment != "" {
		if jc, err := docxJustificationValue(block.TableStyle.Alignment); err == nil {
			fmt.Fprintf(body, `<w:jc w:val="%s"/>`, jc)
		}
	}
	writeDocxTableBorders(body, block)
	body.WriteString(`<w:tblLayout w:type="fixed"/></w:tblPr><w:tblGrid>`)
	for _, width := range colWidths {
		fmt.Fprintf(body, `<w:gridCol w:w="%d"/>`, width)
	}
	body.WriteString("</w:tblGrid>")

	if len(block.Headers) > 0 {
		headerStyle := block.Style
		if headerStyle.Bold == nil {
			bold := true
			if block.HeaderBold != nil {
				bold = *block.HeaderBold
			}
			headerStyle.Bold = &bold
		}
		renderDocxTableRow(body, spec.DefaultStyle, headerStyle, block.Headers, colWidths, true)
	}
	for _, row := range block.Rows {
		renderDocxTableRow(body, spec.DefaultStyle, block.Style, row, colWidths, false)
	}
	body.WriteString("</w:tbl>")
}

// docxTableColumnWidths returns one width in twips per grid column. Explicit
// widths win; otherwise the table (or text area) width is split evenly.
func docxTableColumnWidths(page DocxPageSpec, explicit []float64, colCount int, tableWidth int) []int {
	widths := make([]int, colCount)
	if len(explicit) == colCount {
		for idx, width := range explicit {
			widths[idx] = pointsToTwips(width)
		}
		return widths
	}
	available := tableWidth
	if available <= 0 {
		pageWidth, _ := docxPageSizeTwips(page)
		available = pageWidth - pointsToTwips(valueOrDefault(page.Margins.Left, docxDefaultMarginPoint)) -
			pointsToTwips(valueOrDefault(page.Margins.Right, docxDefaultMarginPoint))
	}
	if colCount > 0 && available > 0 {
		for idx := range widths {
			widths[idx] = available / colCount
		}
	}
	return widths
}

func writeDocxTableBorders(body *strings.Builder, block DocxBlockSpec) {
	borderColor, _ := normalizeDocxColor(block.TableStyle.BorderColor)
	if borderColor == "" {
		borderColor = "000000"
	}
	body.WriteString("<w:tblBorders>")
	for _, edge := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		if docxTableBorderEnabled(block) {
			fmt.Fprintf(body, `<w:%s w:val="single" w:sz="4" w:space="0" w:color="%s"/>`, edge, borderColor)
		} else {
			fmt.Fprintf(body, `<w:%s w:val="nil"/>`, edge)
		}
	}
	body.WriteString("</w:tblBorders>")
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

func renderDocxTableRow(body *strings.Builder, defaultStyle DocxParagraphStyleSpec, rowStyle DocxParagraphStyleSpec, cells []DocxTableCellSpec, colWidths []int, header bool) {
	body.WriteString("<w:tr>")
	if header {
		body.WriteString("<w:trPr><w:tblHeader/></w:trPr>")
	}
	for idx := range cells {
		cell := cells[idx]
		span := 1
		if cell.ColSpan > 1 {
			span = cell.ColSpan
		}
		width := 0
		for col := idx; col < idx+span && col < len(colWidths); col++ {
			width += colWidths[col]
		}
		body.WriteString("<w:tc><w:tcPr>")
		fmt.Fprintf(body, `<w:tcW w:w="%d" w:type="dxa"/>`, width)
		if span > 1 {
			fmt.Fprintf(body, `<w:gridSpan w:val="%d"/>`, span)
		}
		if cell.BackgroundColor != "" {
			if fill, err := normalizeDocxColor(cell.BackgroundColor); err == nil {
				fmt.Fprintf(body, `<w:shd w:val="clear" w:color="auto" w:fill="%s"/>`, fill)
			}
		}
		if cell.VerticalAlign != "" {
			if val, err := docxVerticalAlignValue(cell.VerticalAlign); err == nil {
				fmt.Fprintf(body, `<w:vAlign w:val="%s"/>`, val)
			}
		}
		body.WriteString("</w:tcPr>")
		style := mergeDocxParagraphStyle(rowStyle, cell.Style)
		renderDocxParagraph(body, defaultStyle, style, "", cell.Text, cell.Runs)
		body.WriteString("</w:tc>")
	}
	body.WriteString("</w:tr>")
}

func docxPageSizeTwips(page DocxPageSpec) (int, int) {
	if page.Orientation == "landscape" {
		return docxA4HeightTwips, docxA4WidthTwips
	}
	return docxA4WidthTwips, docxA4HeightTwips
}

func writeDocxSection(body *strings.Builder, page DocxPageSpec) {
	width, height := docxPageSizeTwips(page)
	body.WriteString("<w:sectPr>")
	fmt.Fprintf(body, `<w:pgSz w:w="%d" w:h="%d"`, width, height)
	if page.Orientation == "landscape" {
		body.WriteString(` w:orient="landscape"`)
	}
	body.WriteString("/>")
	fmt.Fprintf(body, `<w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="720" w:footer="720" w:gutter="0"/>`,
		pointsToTwips(valueOrDefault(page.Margins.Top, docxDefaultMarginPoint)),
		pointsToTwips(valueOrDefault(page.Margins.Right, docxDefaultMarginPoint)),
		pointsToTwips(valueOrDefault(page.Margins.Bottom, docxDefaultMarginPoint)),
		pointsToTwips(valueOrDefault(page.Margins.Left, docxDefaultMarginPoint)),
	)
	body.WriteString(`<w:cols w:space="720"/><w:docGrid w:type="lines" w:linePitch="312"/></w:sectPr>`)
}

// writeDocxParagraphProperties writes w:pPr children in CT_PPr schema order.
func writeDocxParagraphProperties(body *strings.Builder, style DocxParagraphStyleSpec, styleID string) {
	var props strings.Builder
	if styleID != "" {
		fmt.Fprintf(&props, `<w:pStyle w:val="%s"/>`, styleID)
	}
	if style.LineSpacing != nil || style.SpaceBefore != nil || style.SpaceAfter != nil {
		fmt.Fprintf(&props, `<w:spacing w:before="%d" w:after="%d" w:line="%d" w:lineRule="auto"/>`,
			pointsToTwips(valueOrDefault(style.SpaceBefore, 0)),
			pointsToTwips(valueOrDefault(style.SpaceAfter, 0)),
			int(math.Round(valueOrDefault(style.LineSpacing, 1)*240)),
		)
	}
	if style.IndentLeft != nil || style.FirstLineIndent != nil {
		fmt.Fprintf(&props, `<w:ind w:left="%d" w:firstLine="%d"/>`,
			pointsToTwips(valueOrDefault(style.IndentLeft, 0)),
			pointsToTwips(valueOrDefault(style.FirstLineIndent, 0)),
		)
	}
	if style.Alignment != "" {
		if jc, err := docxJustificationValue(style.Alignment); err == nil {
			fmt.Fprintf(&props, `<w:jc w:val="%s"/>`, jc)
		}
	}
	if props.Len() > 0 {
		body.WriteString("<w:pPr>")
		body.WriteString(props.String())
		body.WriteString("</w:pPr>")
	}
}

// writeDocxRunProperties writes w:rPr children in CT_RPr schema order.
func writeDocxRunProperties(body *strings.Builder, style DocxTextStyleSpec) {
	font := normalizeDefault(style.FontFamily, docxDefaultFontFamily)
	eastAsia := normalizeDefault(style.EastAsiaFont, font)
	ascii := normalizeDefault(style.AsciiFont, font)
	size := docxHalfPointSize(valueOrDefault(style.FontSize, docxDefaultFontSize))

	body.WriteString("<w:rPr>")
	fmt.Fprintf(body, `<w:rFonts w:ascii="%s" w:eastAsia="%s" w:hAnsi="%s" w:hint="eastAsia"/>`,
		escapeDocxXML(ascii), escapeDocxXML(eastAsia), escapeDocxXML(ascii))
	if valueOrDefaultBool(style.Bold, false) {
		body.WriteString("<w:b/><w:bCs/>")
	}
	if valueOrDefaultBool(style.Italic, false) {
		body.WriteString("<w:i/><w:iCs/>")
	}
	if style.Color != "" {
		if color, err := normalizeDocxColor(style.Color); err == nil && color != "" {
			fmt.Fprintf(body, `<w:color w:val="%s"/>`, color)
		}
	}
	fmt.Fprintf(body, `<w:sz w:val="%s"/><w:szCs w:val="%s"/>`, size, size)
	if highlight, ok := docxHighlightValue(style.Highlight); ok {
		fmt.Fprintf(body, `<w:highlight w:val="%s"/>`, highlight)
	}
	if underline, err := docxUnderlineValue(style.Underline); err == nil && underline != "" {
		fmt.Fprintf(body, `<w:u w:val="%s"/>`, underline)
	}
	body.WriteString("</w:rPr>")
}

// writeDocxRunText writes run content, mapping tabs and line breaks to their
// WordprocessingML elements so they survive in Word.
func writeDocxRunText(body *strings.Builder, text string) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	for lineIndex, line := range strings.Split(text, "\n") {
		if lineIndex > 0 {
			body.WriteString("<w:br/>")
		}
		for segmentIndex, segment := range strings.Split(line, "\t") {
			if segmentIndex > 0 {
				body.WriteString("<w:tab/>")
			}
			if segment == "" {
				continue
			}
			body.WriteString(`<w:t xml:space="preserve">`)
			body.WriteString(escapeDocxXML(segment))
			body.WriteString("</w:t>")
		}
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
	_, ok := docxHighlightValue(raw)
	return ok
}

// docxHighlightValue maps a case-insensitive highlight name to the
// ST_HighlightColor spelling Word expects.
func docxHighlightValue(raw string) (string, bool) {
	switch value := strings.ToLower(strings.TrimSpace(raw)); value {
	case "black", "blue", "cyan", "green", "magenta", "red", "yellow", "white", "none":
		return value, true
	case "darkblue":
		return "darkBlue", true
	case "darkcyan":
		return "darkCyan", true
	case "darkgreen":
		return "darkGreen", true
	case "darkmagenta":
		return "darkMagenta", true
	case "darkred":
		return "darkRed", true
	case "darkyellow":
		return "darkYellow", true
	case "darkgray":
		return "darkGray", true
	case "lightgray":
		return "lightGray", true
	default:
		return "", false
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
