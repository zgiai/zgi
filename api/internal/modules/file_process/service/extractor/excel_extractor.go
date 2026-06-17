package extractor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/pkg/logger"

	"github.com/extrame/xls"
	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
)

type ExcelExtractor struct {
	filePath string
}

func NewExcelExtractor(filePath string) *ExcelExtractor {
	return &ExcelExtractor{
		filePath: filePath,
	}
}

func (e *ExcelExtractor) Extract(ctx context.Context) (*dto.ExtractOutput, error) {
	documents, err := e.load(ctx)
	if err != nil {
		return nil, fmt.Errorf("error loading %s: %w", e.filePath, err)
	}

	return dto.NewExtractOutputFromDocumentsWithType("zgi:excel", "table", documents), nil
}

func (e *ExcelExtractor) load(ctx context.Context) ([]dto.Document, error) {
	fileExtension := strings.ToLower(filepath.Ext(e.filePath))

	switch fileExtension {
	case ".xlsx":
		documents, err := e.handleXlsx(ctx)
		if err != nil {
			return nil, err
		}
		return documents, nil
	case ".xls":
		documents, err := e.handleXls(ctx)
		if err != nil {
			return nil, err
		}
		return documents, nil
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", fileExtension)
	}
}

// handleXlsx processes .xlsx files
func (e *ExcelExtractor) handleXlsx(ctx context.Context) ([]dto.Document, error) {
	f, err := excelize.OpenFile(e.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open .xlsx file: %w", err)
	}
	defer f.Close()

	var documents []dto.Document

	sheetList := f.GetSheetList()

	for _, sheetName := range sheetList {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		rows, err := f.GetRows(sheetName)
		if err != nil {
			continue
		}

		if len(rows) == 0 {
			continue
		}

		headers := rows[0]

		for i := 1; i < len(rows); i++ {
			row := rows[i]
			if len(row) == 0 {
				continue
			}

			var pageContent []string
			for colIndex, cellValue := range row {
				if cellValue == "" {
					continue
				}

				var columnName string
				if colIndex < len(headers) && headers[colIndex] != "" {
					columnName = headers[colIndex]
				} else {
					// Generate column name if header is missing
					columnName = e.columnName(colIndex + 1)
				}

				// Check for hyperlink
				// TODO: Implement hyperlink detection if needed
				// The implementation check for hyperlinks in cells
				// This would require additional API calls in Go

				pageContent = append(pageContent, fmt.Sprintf("\"%s\":\"%s\"", columnName, cellValue))
			}

			if len(pageContent) > 0 {
				doc := dto.Document{
					PageContent: strings.Join(pageContent, ";"),
					Metadata: map[string]interface{}{
						"source": e.filePath,
						"sheet":  sheetName,
					},
				}
				documents = append(documents, doc)
			}
		}
	}

	return documents, nil
}

// handleXls processes .xls files
func (e *ExcelExtractor) handleXls(ctx context.Context) ([]dto.Document, error) {
	xlsFile, err := xls.Open(e.filePath, "utf-8")
	if err != nil {
		return nil, fmt.Errorf("failed to open .xls file: %w", err)
	}

	var documents []dto.Document

	for i := 0; i < xlsFile.NumSheets(); i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		sheet := xlsFile.GetSheet(i)
		if sheet == nil {
			continue
		}

		sheetDocuments, err := e.documentsFromXlsSheet(ctx, extrameXlsSheetReader{sheet: sheet}, i)
		if err != nil {
			return nil, err
		}
		documents = append(documents, sheetDocuments...)
	}

	return documents, nil
}

type legacyXlsSheetReader interface {
	name() string
	maxRow() int
	row(rowIndex int) (legacyXlsRowReader, bool, interface{})
}

type legacyXlsRowReader interface {
	values() ([]string, bool, interface{})
}

type extrameXlsSheetReader struct {
	sheet *xls.WorkSheet
}

func (s extrameXlsSheetReader) name() string {
	return s.sheet.Name
}

func (s extrameXlsSheetReader) maxRow() int {
	return int(s.sheet.MaxRow)
}

func (s extrameXlsSheetReader) row(rowIndex int) (legacyXlsRowReader, bool, interface{}) {
	row, ok, panicValue := safeXlsRow(s.sheet, rowIndex)
	if !ok {
		return nil, false, panicValue
	}
	return extrameXlsRowReader{row: row}, true, nil
}

type extrameXlsRowReader struct {
	row *xls.Row
}

func (r extrameXlsRowReader) values() ([]string, bool, interface{}) {
	return xlsRowValues(r.row)
}

func (e *ExcelExtractor) documentsFromXlsSheet(ctx context.Context, sheet legacyXlsSheetReader, sheetIndex int) ([]dto.Document, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	sheetName := sheet.name()
	headerRow, ok, panicValue := sheet.row(0)
	if !ok {
		logSkippedXlsSheet(e.filePath, sheetName, sheetIndex, "header row unavailable", panicValue)
		return nil, nil
	}

	headers, ok, panicValue := headerRow.values()
	if !ok {
		logSkippedXlsSheet(e.filePath, sheetName, sheetIndex, "header row unreadable", panicValue)
		return nil, nil
	}
	if len(headers) == 0 {
		logSkippedXlsSheet(e.filePath, sheetName, sheetIndex, "empty sheet", nil)
		return nil, nil
	}

	var documents []dto.Document
	for rowIdx := 1; rowIdx <= sheet.maxRow(); rowIdx++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		row, ok, panicValue := sheet.row(rowIdx)
		if !ok {
			logSkippedXlsRow(e.filePath, sheetName, sheetIndex, rowIdx, "row unavailable", panicValue)
			continue
		}

		values, ok, panicValue := row.values()
		if !ok {
			logSkippedXlsRow(e.filePath, sheetName, sheetIndex, rowIdx, "row unreadable", panicValue)
			continue
		}
		if len(values) == 0 {
			continue
		}

		var pageContent []string
		for colIdx, cell := range values {
			if cell == "" {
				continue
			}

			var columnName string
			if colIdx < len(headers) && headers[colIdx] != "" {
				columnName = headers[colIdx]
			} else {
				columnName = e.columnName(colIdx + 1)
			}
			pageContent = append(pageContent, fmt.Sprintf("\"%s\":\"%s\"", columnName, cell))
		}

		if len(pageContent) > 0 {
			doc := dto.Document{
				PageContent: strings.Join(pageContent, ";"),
				Metadata: map[string]interface{}{
					"source": e.filePath,
					"sheet":  sheetName,
				},
			}
			documents = append(documents, doc)
		}
	}

	return documents, nil
}

func safeXlsRow(sheet *xls.WorkSheet, rowIndex int) (row *xls.Row, ok bool, panicValue interface{}) {
	defer func() {
		if recovered := recover(); recovered != nil {
			row = nil
			ok = false
			panicValue = recovered
		}
	}()

	row = sheet.Row(rowIndex)
	if row == nil {
		return nil, false, nil
	}
	return row, true, nil
}

func xlsRowValues(row *xls.Row) (values []string, ok bool, panicValue interface{}) {
	defer func() {
		if recovered := recover(); recovered != nil {
			values = nil
			ok = false
			panicValue = recovered
		}
	}()

	lastCol := row.LastCol()
	if lastCol <= 0 {
		return nil, true, nil
	}

	values = make([]string, lastCol)
	for colIdx := 0; colIdx < lastCol; colIdx++ {
		values[colIdx] = row.Col(colIdx)
	}
	return values, true, nil
}

func logSkippedXlsSheet(filePath, sheetName string, sheetIndex int, reason string, panicValue interface{}) {
	fields := []interface{}{
		zap.String("file_path", filePath),
		zap.String("sheet", sheetName),
		zap.Int("sheet_index", sheetIndex),
		zap.String("reason", reason),
	}
	if panicValue != nil {
		fields = append(fields, zap.Any("panic", panicValue))
	}
	logger.Warn("skipping .xls sheet during extraction", fields...)
}

func logSkippedXlsRow(filePath, sheetName string, sheetIndex int, rowIndex int, reason string, panicValue interface{}) {
	fields := []interface{}{
		zap.String("file_path", filePath),
		zap.String("sheet", sheetName),
		zap.Int("sheet_index", sheetIndex),
		zap.Int("row_index", rowIndex),
		zap.String("reason", reason),
	}
	if panicValue != nil {
		fields = append(fields, zap.Any("panic", panicValue))
	}
	logger.Warn("skipping .xls row during extraction", fields...)
}

// columnName converts column index to Excel column name (e.g., 1 -> A, 27 -> AA)
func (e *ExcelExtractor) columnName(index int) string {
	if index <= 0 {
		return ""
	}

	var result strings.Builder
	for index > 0 {
		index--
		result.WriteByte(byte('A' + (index % 26)))
		index /= 26
	}

	// Reverse the string since we built it backwards
	runes := []rune(result.String())
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}

	return string(runes)
}
