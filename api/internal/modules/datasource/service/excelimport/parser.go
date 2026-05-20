package excelimport

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

func ParseWorkbook(fileName string, content []byte) (*ParsedWorkbook, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext == ".csv" {
		return parseCSV(content)
	}
	return parseExcel(content)
}

func parseExcel(content []byte) (*ParsedWorkbook, error) {
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("failed to open workbook: %w", err)
	}
	defer f.Close()

	wb := &ParsedWorkbook{SourceType: "excel"}
	for _, sheetName := range f.GetSheetList() {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			return nil, fmt.Errorf("failed to read sheet %s: %w", sheetName, err)
		}
		visible, _ := f.GetSheetVisible(sheetName)
		sheet := ParsedSheet{
			Name:        sheetName,
			Rows:        trimTrailingEmptyRows(rows),
			Hidden:      !visible,
			RowCount:    len(rows),
			ColumnCount: maxColumnCount(rows),
		}
		wb.Sheets = append(wb.Sheets, sheet)
	}
	markRecommendedSheet(wb.Sheets)
	return wb, nil
}

func parseCSV(content []byte) (*ParsedWorkbook, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read csv: %w", err)
	}
	sheet := ParsedSheet{
		Name:        "CSV",
		Rows:        trimTrailingEmptyRows(rows),
		RowCount:    len(rows),
		ColumnCount: maxColumnCount(rows),
		Recommended: true,
	}
	return &ParsedWorkbook{SourceType: "csv", Sheets: []ParsedSheet{sheet}}, nil
}

func trimTrailingEmptyRows(rows [][]string) [][]string {
	end := len(rows)
	for end > 0 && isEmptyRow(rows[end-1]) {
		end--
	}
	return rows[:end]
}

func isEmptyRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

func maxColumnCount(rows [][]string) int {
	max := 0
	for _, row := range rows {
		if len(row) > max {
			max = len(row)
		}
	}
	return max
}

func markRecommendedSheet(sheets []ParsedSheet) {
	recommended := -1
	bestScore := -1
	for i, sheet := range sheets {
		if sheet.Hidden {
			continue
		}
		score := sheet.RowCount * sheet.ColumnCount
		if sheet.RowCount >= 2 && sheet.ColumnCount >= 2 && score > bestScore {
			recommended = i
			bestScore = score
		}
	}
	if recommended >= 0 {
		sheets[recommended].Recommended = true
	}
}
