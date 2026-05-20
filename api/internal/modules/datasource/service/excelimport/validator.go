package excelimport

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
)

var (
	tableNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	fieldNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
	systemFieldNames = map[string]struct{}{
		"id":           {},
		"uuid":         {},
		"created_time": {},
		"updated_time": {},
	}
)

func ValidateImportSchema(req dto.ConfirmExcelImportRequest) error {
	tableName := strings.TrimSpace(req.Table.Name)
	if !tableNamePattern.MatchString(tableName) {
		return fmt.Errorf("table name must start with a lowercase letter and contain only lowercase letters, digits, and underscores")
	}

	seen := make(map[string]struct{})
	enabledCount := 0
	for _, col := range req.Columns {
		if !columnEnabled(col) {
			continue
		}
		enabledCount++
		name := strings.TrimSpace(col.Name)
		if !fieldNamePattern.MatchString(name) {
			return fmt.Errorf("field name %q must contain only lowercase letters, digits, and underscores", col.Name)
		}
		if _, reserved := systemFieldNames[name]; reserved {
			return fmt.Errorf("field name %q is reserved", name)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("field name %q is duplicated", name)
		}
		seen[name] = struct{}{}
		if col.SourceColumnIndex < 0 {
			return fmt.Errorf("source column index for field %q must be non-negative", name)
		}
	}
	if enabledCount == 0 {
		return fmt.Errorf("at least one column must be enabled")
	}
	return nil
}

func ValidateRows(wb *ParsedWorkbook, req dto.ConfirmExcelImportRequest) (*RowValidationResult, error) {
	sheet, err := selectSheet(wb.Sheets, &req.Selection.SheetName)
	if err != nil {
		return nil, err
	}
	if req.Selection.HeaderRow < 1 || req.Selection.HeaderRow > len(sheet.Rows) {
		return nil, fmt.Errorf("header row %d is outside sheet range", req.Selection.HeaderRow)
	}
	if req.Selection.StartRow < 1 || req.Selection.StartRow > len(sheet.Rows)+1 {
		return nil, fmt.Errorf("start row %d is outside sheet range", req.Selection.StartRow)
	}

	sourceByIndex := make(map[int]dto.InferredExcelColumn)
	for _, col := range req.Columns {
		if !columnEnabled(col) {
			continue
		}
		if col.SourceColumnIndex < 0 || col.SourceColumnIndex >= sheet.ColumnCount {
			return nil, fmt.Errorf("source column index %d for field %s is outside sheet range", col.SourceColumnIndex, col.Name)
		}
		sourceByIndex[col.SourceColumnIndex] = col
	}

	result := &RowValidationResult{}
	dataRows := sheet.Rows[req.Selection.StartRow-1:]
	result.TotalRows = len(dataRows)
	for rowOffset, row := range dataRows {
		rowIndex := req.Selection.StartRow + rowOffset
		if isEmptyRow(row) {
			if req.Options.EmptyRowPolicy == "error" {
				result.Errors = append(result.Errors, dto.ExcelImportFailedItem{
					RowIndex:     rowIndex,
					ErrorCode:    "empty_row",
					ErrorMessage: "Row is empty",
				})
			}
			continue
		}

		record := make(map[string]interface{})
		rowErrors := make([]dto.ExcelImportFailedItem, 0)
		for index, col := range sourceByIndex {
			raw := ""
			if index < len(row) {
				raw = strings.TrimSpace(row[index])
			}
			value, convertErr := convertValue(raw, col.Type, col.IsRequired)
			if convertErr != nil {
				columnName := col.Name
				rawValue := raw
				rowErrors = append(rowErrors, dto.ExcelImportFailedItem{
					RowIndex:     rowIndex,
					ColumnName:   &columnName,
					RawValue:     &rawValue,
					ErrorCode:    "invalid_" + col.Type,
					ErrorMessage: convertErr.Error(),
				})
				continue
			}
			record[col.Name] = value
		}
		for _, col := range req.Columns {
			if !columnEnabled(col) || !col.IsRequired {
				continue
			}
			if _, exists := record[col.Name]; !exists {
				columnName := col.Name
				rowErrors = append(rowErrors, dto.ExcelImportFailedItem{
					RowIndex:     rowIndex,
					ColumnName:   &columnName,
					ErrorCode:    "missing_required",
					ErrorMessage: fmt.Sprintf("Required field %s is missing", col.Name),
				})
			}
		}
		if len(rowErrors) > 0 {
			result.Errors = append(result.Errors, rowErrors...)
			if req.Options.ErrorPolicy == "fail_fast" {
				return result, nil
			}
			continue
		}
		result.Records = append(result.Records, record)
	}
	return result, nil
}

func columnEnabled(col dto.InferredExcelColumn) bool {
	if col.Enabled == nil {
		return true
	}
	return *col.Enabled
}

func convertValue(raw, typ string, required bool) (interface{}, error) {
	if raw == "" {
		if required {
			return nil, fmt.Errorf("value is required")
		}
		return nil, nil
	}

	switch typ {
	case "integer":
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("cannot convert %q to integer", raw)
		}
		return v, nil
	case "numeric":
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("cannot convert %q to numeric", raw)
		}
		return v, nil
	case "boolean":
		switch strings.ToLower(raw) {
		case "true", "yes", "y", "1", "on", "是":
			return true, nil
		case "false", "no", "n", "0", "off", "否":
			return false, nil
		default:
			return nil, fmt.Errorf("cannot convert %q to boolean", raw)
		}
	case "timestamp":
		for _, format := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
			if parsed, err := time.Parse(format, raw); err == nil {
				return parsed, nil
			}
		}
		return nil, fmt.Errorf("cannot convert %q to timestamp", raw)
	case "text":
		return raw, nil
	default:
		return raw, nil
	}
}
