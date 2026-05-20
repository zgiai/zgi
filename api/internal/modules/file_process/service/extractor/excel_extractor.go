package extractor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"

	"github.com/extrame/xls"
	"github.com/xuri/excelize/v2"
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
		sheet := xlsFile.GetSheet(i)
		if sheet == nil {
			continue
		}

		sheetName := sheet.Name
		headerRow := sheet.Row(0)
		if headerRow == nil {
			continue
		}

		headers := make([]string, headerRow.LastCol())
		for colIdx := 0; colIdx < headerRow.LastCol(); colIdx++ {
			headers[colIdx] = headerRow.Col(colIdx)
		}

		for rowIdx := 1; rowIdx <= int(sheet.MaxRow); rowIdx++ {
			row := sheet.Row(rowIdx)
			if row == nil {
				continue
			}

			var pageContent []string
			for colIdx := 0; colIdx < row.LastCol(); colIdx++ {
				cell := row.Col(colIdx)
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
	}

	return documents, nil
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
