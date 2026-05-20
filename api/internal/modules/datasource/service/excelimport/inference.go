package excelimport

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/zgiai/ginext/internal/dto"
)

const (
	defaultSampleSize = 500
	previewRowLimit   = 50
)

var nonASCIIIdentifierChars = regexp.MustCompile(`[^a-z0-9_]+`)

var reservedFieldNames = map[string]struct{}{
	"id":           {},
	"uuid":         {},
	"created_time": {},
	"updated_time": {},
}

var headerFieldNameDictionary = map[string]string{
	"备注":      "remark",
	"产品":      "product",
	"负责人":     "owner",
	"提出人":     "reporter",
	"创建人":     "creator",
	"更新人":     "updater",
	"创建时间":    "created_at",
	"更新时间":    "updated_at",
	"提交时间":    "submitted_at",
	"解决时间":    "resolved_at",
	"状态":      "status",
	"优先级":     "priority",
	"标题":      "title",
	"名称":      "name",
	"描述":      "description",
	"内容":      "content",
	"截图":      "screenshot",
	"复现步骤":    "reproduce_steps",
	"bug":     "bug",
	"bug描述":   "bug_description",
	"bug复现步骤": "bug_reproduce_steps",
	"bug截图":   "bug_screenshot",
	"bug提交时间": "bug_submitted_at",
	"bug解决时间": "bug_resolved_at",
	"bug状态":   "bug_status",
}

func AnalyzeWorkbook(wb *ParsedWorkbook, options AnalyzeOptions) (*AnalysisResult, error) {
	if wb == nil || len(wb.Sheets) == 0 {
		return nil, fmt.Errorf("workbook has no sheets")
	}
	if options.SampleSize <= 0 {
		options.SampleSize = defaultSampleSize
	}

	sheet, err := selectSheet(wb.Sheets, options.SheetName)
	if err != nil {
		return nil, err
	}
	if len(sheet.Rows) < 2 {
		return nil, fmt.Errorf("sheet %s must contain at least a header row and one data row", sheet.Name)
	}

	headerRow := detectHeaderRow(sheet.Rows)
	if options.HeaderRow != nil && *options.HeaderRow > 0 {
		headerRow = *options.HeaderRow
	}
	if headerRow < 1 || headerRow > len(sheet.Rows) {
		return nil, fmt.Errorf("header row %d is outside sheet range", headerRow)
	}
	startRow := headerRow + 1

	sourceHeaders := normalizeSourceHeaders(sheet.Rows[headerRow-1], sheet.ColumnCount)
	fieldNames := normalizeFieldNames(sourceHeaders)
	dataRows := sheet.Rows[startRow-1:]
	sampleRows := dataRows
	if len(sampleRows) > options.SampleSize {
		sampleRows = sampleRows[:options.SampleSize]
	}

	result := &AnalysisResult{}
	result.Selection.SheetName = sheet.Name
	result.Selection.HeaderRow = headerRow
	result.Selection.StartRow = startRow
	result.Sheets = toSheetDTOs(wb.Sheets)
	result.TotalRows = len(dataRows)
	result.Columns = inferColumns(sourceHeaders, fieldNames, sampleRows)
	result.PreviewRows = buildPreviewRows(fieldNames, dataRows, startRow)
	result.ValidRows = len(dataRows)
	result.Warnings = buildAnalysisWarnings(fieldNames)
	return result, nil
}

func selectSheet(sheets []ParsedSheet, name *string) (*ParsedSheet, error) {
	if name != nil && strings.TrimSpace(*name) != "" {
		for i := range sheets {
			if sheets[i].Name == *name {
				return &sheets[i], nil
			}
		}
		return nil, fmt.Errorf("sheet %s not found", *name)
	}
	for i := range sheets {
		if sheets[i].Recommended {
			return &sheets[i], nil
		}
	}
	return &sheets[0], nil
}

func toSheetDTOs(sheets []ParsedSheet) []dto.ExcelImportSheet {
	out := make([]dto.ExcelImportSheet, 0, len(sheets))
	for _, sheet := range sheets {
		out = append(out, dto.ExcelImportSheet{
			Name:        sheet.Name,
			RowCount:    sheet.RowCount,
			ColumnCount: sheet.ColumnCount,
			Hidden:      sheet.Hidden,
			Recommended: sheet.Recommended,
		})
	}
	return out
}

func detectHeaderRow(rows [][]string) int {
	limit := len(rows)
	if limit > 10 {
		limit = 10
	}
	bestRow := 1
	bestScore := -1
	for i := 0; i < limit; i++ {
		row := rows[i]
		nonEmpty := 0
		unique := make(map[string]struct{})
		textLike := 0
		for _, cell := range row {
			v := strings.TrimSpace(cell)
			if v == "" {
				continue
			}
			nonEmpty++
			unique[strings.ToLower(v)] = struct{}{}
			if !looksNumeric(v) && !looksTimestamp(v) {
				textLike++
			}
		}
		score := nonEmpty + len(unique) + textLike
		if i+1 < len(rows) && !isEmptyRow(rows[i+1]) {
			score += 3
		}
		if score > bestScore {
			bestScore = score
			bestRow = i + 1
		}
	}
	return bestRow
}

func normalizeSourceHeaders(header []string, columnCount int) []string {
	if columnCount < len(header) {
		columnCount = len(header)
	}
	out := make([]string, columnCount)
	for i := 0; i < columnCount; i++ {
		source := ""
		if i < len(header) {
			source = strings.TrimSpace(header[i])
		}
		if source == "" {
			source = fmt.Sprintf("column_%d", i+1)
		}
		out[i] = source
	}
	return out
}

func normalizeFieldNames(headers []string) []string {
	out := make([]string, len(headers))
	seen := make(map[string]int)
	for i, source := range headers {
		name := sanitizeFieldName(source)
		if name == "" {
			name = fmt.Sprintf("column_%d", i+1)
		}
		seen[name]++
		if seen[name] > 1 {
			name = fmt.Sprintf("%s_%d", name, seen[name])
		}
		out[i] = name
	}
	return out
}

func sanitizeFieldName(value string) string {
	trimmed := strings.TrimSpace(value)
	if mapped, ok := headerFieldNameDictionary[strings.ToLower(trimmed)]; ok {
		return mapped
	}
	if mapped, ok := headerFieldNameDictionary[trimmed]; ok {
		return mapped
	}

	var b strings.Builder
	for _, r := range strings.ToLower(trimmed) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || unicode.IsSpace(r):
			b.WriteRune('_')
		}
	}
	name := nonASCIIIdentifierChars.ReplaceAllString(b.String(), "_")
	name = strings.Trim(name, "_")
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	if name != "" && name[0] >= '0' && name[0] <= '9' {
		name = "col_" + name
	}
	if _, reserved := reservedFieldNames[name]; reserved {
		name += "_value"
	}
	return name
}

func inferColumns(sourceHeaders []string, fieldNames []string, rows [][]string) []dto.InferredExcelColumn {
	columns := make([]dto.InferredExcelColumn, 0, len(fieldNames))
	for colIndex, fieldName := range fieldNames {
		values := collectColumnValues(rows, colIndex)
		colType, confidence := inferType(values)
		nonEmpty := 0
		samples := make([]string, 0, 3)
		for _, value := range values {
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				continue
			}
			nonEmpty++
			if len(samples) < 3 {
				samples = append(samples, trimmed)
			}
		}
		isRequired := len(values) > 0 && nonEmpty == len(values)
		columns = append(columns, dto.InferredExcelColumn{
			SourceColumn:      sourceHeaders[colIndex],
			SourceColumnIndex: colIndex,
			Name:              fieldName,
			DisplayName:       sourceHeaders[colIndex],
			Type:              colType,
			IsRequired:        isRequired,
			Description:       "Imported from " + sourceHeaders[colIndex],
			Confidence:        confidence,
			SampleValues:      samples,
			Warnings:          nil,
		})
	}
	return columns
}

func collectColumnValues(rows [][]string, colIndex int) []string {
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		if colIndex < len(row) {
			values = append(values, strings.TrimSpace(row[colIndex]))
		} else {
			values = append(values, "")
		}
	}
	return values
}

func inferType(values []string) (string, float64) {
	nonEmpty := 0
	integers := 0
	numerics := 0
	booleans := 0
	timestamps := 0
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		nonEmpty++
		if looksInteger(v) {
			integers++
		}
		if looksNumeric(v) {
			numerics++
		}
		if looksBoolean(v) {
			booleans++
		}
		if looksTimestamp(v) {
			timestamps++
		}
	}
	if nonEmpty == 0 {
		return "text", 0.5
	}
	ratio := func(count int) float64 { return float64(count) / float64(nonEmpty) }
	switch {
	case ratio(booleans) >= 0.95:
		return "boolean", ratio(booleans)
	case ratio(integers) >= 0.95:
		return "integer", ratio(integers)
	case ratio(numerics) >= 0.95:
		return "numeric", ratio(numerics)
	case ratio(timestamps) >= 0.95:
		return "timestamp", ratio(timestamps)
	default:
		return "text", 0.8
	}
}

func buildPreviewRows(headers []string, rows [][]string, startRow int) []dto.ExcelImportPreviewRow {
	limit := len(rows)
	if limit > previewRowLimit {
		limit = previewRowLimit
	}
	out := make([]dto.ExcelImportPreviewRow, 0, limit)
	for i := 0; i < limit; i++ {
		values := make(map[string]interface{}, len(headers))
		for colIndex, header := range headers {
			if colIndex < len(rows[i]) {
				values[header] = rows[i][colIndex]
			} else {
				values[header] = ""
			}
		}
		out = append(out, dto.ExcelImportPreviewRow{RowIndex: startRow + i, Values: values})
	}
	return out
}

func buildAnalysisWarnings(headers []string) []dto.ExcelImportWarning {
	seen := make(map[string]struct{})
	warnings := make([]dto.ExcelImportWarning, 0)
	for _, header := range headers {
		if _, exists := seen[header]; exists {
			columnName := header
			warnings = append(warnings, dto.ExcelImportWarning{
				Code:       "duplicate_header_normalized",
				Message:    "Duplicate headers were normalized to unique field names.",
				ColumnName: &columnName,
			})
		}
		seen[header] = struct{}{}
	}
	return warnings
}

func looksInteger(value string) bool {
	if value == "" {
		return false
	}
	_, err := strconv.ParseInt(value, 10, 64)
	return err == nil
}

func looksNumeric(value string) bool {
	if value == "" {
		return false
	}
	_, err := strconv.ParseFloat(value, 64)
	return err == nil
}

func looksBoolean(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "false", "yes", "no", "y", "n", "1", "0", "on", "off", "是", "否":
		return true
	default:
		return false
	}
}

func looksTimestamp(value string) bool {
	v := strings.TrimSpace(value)
	if v == "" {
		return false
	}
	formats := []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02",
		"01/02/2006",
		"02/01/2006",
	}
	for _, format := range formats {
		if _, err := time.Parse(format, v); err == nil {
			return true
		}
	}
	return false
}
