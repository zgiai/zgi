package excelimport

import (
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/extrame/xls"
	"github.com/xuri/excelize/v2"
)

func ParseWorkbook(fileName string, content []byte) (*ParsedWorkbook, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".csv":
		return parseCSV(content)
	case ".xls":
		return parseXLS(content)
	default:
		return parseXLSX(content)
	}
}

func parseXLSX(content []byte) (*ParsedWorkbook, error) {
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("failed to open workbook: %w", err)
	}
	defer f.Close()
	workbookProps, err := f.GetWorkbookProps()
	if err != nil {
		return nil, fmt.Errorf("failed to read workbook properties: %w", err)
	}
	use1904Format := workbookProps.Date1904 != nil && *workbookProps.Date1904

	wb := &ParsedWorkbook{SourceType: "excel"}
	for _, sheetName := range f.GetSheetList() {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			return nil, fmt.Errorf("failed to read sheet %s: %w", sheetName, err)
		}
		if err := normalizeXLSXDateCells(f, sheetName, rows, use1904Format); err != nil {
			return nil, fmt.Errorf("failed to normalize dates in sheet %s: %w", sheetName, err)
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

func normalizeXLSXDateCells(f *excelize.File, sheetName string, rows [][]string, use1904Format bool) error {
	dateStyles := make(map[int]bool)
	for rowIndex, row := range rows {
		for colIndex, value := range row {
			if strings.TrimSpace(value) == "" {
				continue
			}
			cell, err := excelize.CoordinatesToCellName(colIndex+1, rowIndex+1)
			if err != nil {
				return err
			}
			styleID, err := f.GetCellStyle(sheetName, cell)
			if err != nil {
				return err
			}
			isDate, known := dateStyles[styleID]
			if !known {
				style, err := f.GetStyle(styleID)
				if err != nil {
					return err
				}
				isDate = isExcelDateStyle(style)
				dateStyles[styleID] = isDate
			}
			if !isDate {
				continue
			}
			raw, err := f.GetCellValue(sheetName, cell, excelize.Options{RawCellValue: true})
			if err != nil {
				return err
			}
			serial, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				continue
			}
			parsed, err := excelize.ExcelDateToTime(serial, use1904Format)
			if err != nil {
				return err
			}
			rows[rowIndex][colIndex] = parsed.Format("2006-01-02 15:04:05")
		}
	}
	return nil
}

func isExcelDateStyle(style *excelize.Style) bool {
	if style == nil {
		return false
	}
	if isExcelBuiltInDateFormat(style.NumFmt) {
		return true
	}
	if style.CustomNumFmt == nil {
		return false
	}
	return isExcelCustomDateFormat(*style.CustomNumFmt)
}

func isExcelCustomDateFormat(format string) bool {
	hasDateToken := false
	for i := 0; i < len(format); {
		switch format[i] {
		case '"':
			next, ok := skipExcelFormatQuotedText(format, i+1)
			if !ok {
				return false
			}
			i = next
		case '\\', '_', '*':
			if i+1 >= len(format) {
				return false
			}
			i += 2
		case '[':
			end := strings.IndexByte(format[i+1:], ']')
			if end < 0 {
				return false
			}
			end += i + 1
			if isExcelElapsedTimeToken(format[i+1 : end]) {
				return false
			}
			i = end + 1
		default:
			switch format[i] | 0x20 {
			case 'y', 'm', 'd', 'h', 's':
				hasDateToken = true
			}
			i++
		}
	}
	return hasDateToken
}

func skipExcelFormatQuotedText(format string, start int) (int, bool) {
	for i := start; i < len(format); i++ {
		if format[i] != '"' {
			continue
		}
		if i+1 < len(format) && format[i+1] == '"' {
			i++
			continue
		}
		return i + 1, true
	}
	return 0, false
}

func isExcelElapsedTimeToken(token string) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" || !strings.ContainsRune("hms", rune(token[0])) {
		return false
	}
	for i := 1; i < len(token); i++ {
		if token[i] != token[0] {
			return false
		}
	}
	return true
}

func isExcelBuiltInDateFormat(numFmt int) bool {
	return (numFmt >= 14 && numFmt <= 22) ||
		(numFmt >= 27 && numFmt <= 36) ||
		numFmt == 45 || numFmt == 47 ||
		(numFmt >= 50 && numFmt <= 58)
}

func parseXLS(content []byte) (*ParsedWorkbook, error) {
	f, err := xls.OpenReader(bytes.NewReader(content), "utf-8")
	if err != nil {
		if xmlWorkbook, xmlErr := parseSpreadsheetML(content); xmlErr == nil {
			return xmlWorkbook, nil
		}
		return nil, fmt.Errorf("failed to open xls workbook: %w", err)
	}

	wb := &ParsedWorkbook{SourceType: "excel"}
	for sheetIndex := 0; sheetIndex < f.NumSheets(); sheetIndex++ {
		xlsSheet := f.GetSheet(sheetIndex)
		if xlsSheet == nil {
			continue
		}

		rows := make([][]string, 0, int(xlsSheet.MaxRow)+1)
		for rowIndex := 0; rowIndex <= int(xlsSheet.MaxRow); rowIndex++ {
			xlsRow := safeXLSRow(xlsSheet, rowIndex)
			if xlsRow == nil {
				rows = append(rows, nil)
				continue
			}

			row := make([]string, xlsRow.LastCol())
			for colIndex := 0; colIndex < xlsRow.LastCol(); colIndex++ {
				row[colIndex] = xlsRow.Col(colIndex)
			}
			rows = append(rows, row)
		}

		trimmedRows := trimTrailingEmptyRows(rows)
		sheet := ParsedSheet{
			Name:        xlsSheet.Name,
			Rows:        trimmedRows,
			Hidden:      false,
			RowCount:    len(rows),
			ColumnCount: maxColumnCount(rows),
		}
		wb.Sheets = append(wb.Sheets, sheet)
	}
	markRecommendedSheet(wb.Sheets)
	return wb, nil
}

func parseSpreadsheetML(content []byte) (*ParsedWorkbook, error) {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	wb := &ParsedWorkbook{SourceType: "excel"}
	var currentSheet *ParsedSheet
	var currentRow []string
	inData := false
	var data strings.Builder

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read spreadsheet xml: %w", err)
		}

		switch item := token.(type) {
		case xml.StartElement:
			switch item.Name.Local {
			case "Workbook":
			case "Worksheet":
				name := spreadsheetMLAttr(item, "Name")
				if name == "" {
					name = fmt.Sprintf("Sheet%d", len(wb.Sheets)+1)
				}
				wb.Sheets = append(wb.Sheets, ParsedSheet{Name: name})
				currentSheet = &wb.Sheets[len(wb.Sheets)-1]
			case "Row":
				if currentSheet == nil {
					continue
				}
				if rowIndex := spreadsheetMLPositiveIndex(item); rowIndex > 0 {
					for len(currentSheet.Rows) < rowIndex-1 {
						currentSheet.Rows = append(currentSheet.Rows, nil)
					}
				}
				currentRow = []string{}
			case "Cell":
				if currentRow == nil {
					continue
				}
				if colIndex := spreadsheetMLPositiveIndex(item); colIndex > 0 {
					for len(currentRow) < colIndex-1 {
						currentRow = append(currentRow, "")
					}
				}
			case "Data":
				if currentRow != nil {
					inData = true
					data.Reset()
				}
			}
		case xml.CharData:
			if inData {
				data.Write([]byte(item))
			}
		case xml.EndElement:
			switch item.Name.Local {
			case "Data":
				if inData {
					inData = false
				}
			case "Cell":
				if currentRow != nil {
					currentRow = append(currentRow, data.String())
					data.Reset()
				}
			case "Row":
				if currentSheet != nil && currentRow != nil {
					currentSheet.Rows = append(currentSheet.Rows, currentRow)
					currentRow = nil
				}
			case "Worksheet":
				if currentSheet != nil {
					currentSheet.Rows = trimTrailingEmptyRows(currentSheet.Rows)
					currentSheet.RowCount = len(currentSheet.Rows)
					currentSheet.ColumnCount = maxColumnCount(currentSheet.Rows)
					currentSheet = nil
				}
			}
		}
	}

	if len(wb.Sheets) == 0 {
		return nil, fmt.Errorf("spreadsheet xml has no worksheets")
	}
	markRecommendedSheet(wb.Sheets)
	return wb, nil
}

func spreadsheetMLAttr(element xml.StartElement, localName string) string {
	for _, attr := range element.Attr {
		if attr.Name.Local == localName {
			return attr.Value
		}
	}
	return ""
}

func spreadsheetMLPositiveIndex(element xml.StartElement) int {
	value := spreadsheetMLAttr(element, "Index")
	if value == "" {
		return 0
	}
	index, err := strconv.Atoi(value)
	if err != nil || index <= 0 {
		return 0
	}
	return index
}

func safeXLSRow(sheet *xls.WorkSheet, index int) (row *xls.Row) {
	defer func() {
		if recover() != nil {
			row = nil
		}
	}()
	return sheet.Row(index)
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
