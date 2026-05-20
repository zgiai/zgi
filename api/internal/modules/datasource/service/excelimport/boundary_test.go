package excelimport

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
	"github.com/zgiai/ginext/internal/dto"
)

func TestParseWorkbookTrimsTrailingEmptyRowsAndRecommendsVisibleSheet(t *testing.T) {
	content := workbookBytes(t, map[string][][]string{
		"Small": {
			{"Name", "Amount"},
			{"A", "1"},
		},
		"Large": {
			{"Name", "Amount", "Status"},
			{"A", "1", "open"},
			{"B", "2", "closed"},
			{"", "", ""},
		},
	})

	wb, err := ParseWorkbook("sample.xlsx", content)
	if err != nil {
		t.Fatalf("ParseWorkbook returned error: %v", err)
	}
	if len(wb.Sheets) != 2 {
		t.Fatalf("sheet count = %d, want 2", len(wb.Sheets))
	}

	var large *ParsedSheet
	for i := range wb.Sheets {
		if wb.Sheets[i].Name == "Large" {
			large = &wb.Sheets[i]
		}
	}
	if large == nil {
		t.Fatalf("Large sheet not found")
	}
	if got := len(large.Rows); got != 3 {
		t.Fatalf("trimmed row count = %d, want 3", got)
	}
	if !large.Recommended {
		t.Fatalf("largest visible sheet should be recommended")
	}
}

func TestParseCSVAllowsRaggedRowsAndEmptyTrailingRows(t *testing.T) {
	wb, err := ParseWorkbook("ragged.csv", []byte("Name,Amount,Status\nA,1\nB,2,done\n,,\n"))
	if err != nil {
		t.Fatalf("ParseWorkbook returned error: %v", err)
	}
	sheet := wb.Sheets[0]
	if sheet.Name != "CSV" {
		t.Fatalf("sheet name = %q, want CSV", sheet.Name)
	}
	if got := len(sheet.Rows); got != 3 {
		t.Fatalf("trimmed row count = %d, want 3", got)
	}
	if got := sheet.ColumnCount; got != 3 {
		t.Fatalf("column count = %d, want 3", got)
	}
}

func TestValidateRowsSkipsInvalidRowsAndKeepsValidRows(t *testing.T) {
	wb := &ParsedWorkbook{
		SourceType: "excel",
		Sheets: []ParsedSheet{
			{
				Name:        "Sheet1",
				Rows:        [][]string{{"Name", "Amount", "Active"}, {"Alice", "10", "yes"}, {"Bob", "bad", "no"}, {"Carol", "12.5", "maybe"}},
				RowCount:    4,
				ColumnCount: 3,
				Recommended: true,
			},
		},
	}
	req := emptyConfirmRequest()
	req.Columns = []dto.InferredExcelColumn{
		{SourceColumnIndex: 0, Name: "name", Type: "text", IsRequired: true},
		{SourceColumnIndex: 1, Name: "amount", Type: "numeric", IsRequired: true},
		{SourceColumnIndex: 2, Name: "active", Type: "boolean", IsRequired: true},
	}

	result, err := ValidateRows(wb, req)
	if err != nil {
		t.Fatalf("ValidateRows returned error: %v", err)
	}
	if got := len(result.Records); got != 1 {
		t.Fatalf("valid records = %d, want 1", got)
	}
	if got := len(result.Errors); got != 4 {
		t.Fatalf("errors = %d, want 4", got)
	}
	if got := result.Records[0]["name"]; got != "Alice" {
		t.Fatalf("first valid record name = %v, want Alice", got)
	}
}

func TestValidateRowsEmptyRowPolicyError(t *testing.T) {
	wb := &ParsedWorkbook{
		SourceType: "excel",
		Sheets: []ParsedSheet{
			{
				Name:        "Sheet1",
				Rows:        [][]string{{"Name"}, {""}, {"Alice"}},
				RowCount:    3,
				ColumnCount: 1,
				Recommended: true,
			},
		},
	}
	req := emptyConfirmRequest()
	req.Options.EmptyRowPolicy = "error"
	req.Columns = []dto.InferredExcelColumn{
		{SourceColumnIndex: 0, Name: "name", Type: "text"},
	}

	result, err := ValidateRows(wb, req)
	if err != nil {
		t.Fatalf("ValidateRows returned error: %v", err)
	}
	if got := len(result.Errors); got != 1 {
		t.Fatalf("errors = %d, want 1", got)
	}
	if got := result.Errors[0].ErrorCode; got != "empty_row" {
		t.Fatalf("error code = %q, want empty_row", got)
	}
}

func TestValidateRowsFailFastStopsAtFirstInvalidRow(t *testing.T) {
	wb := &ParsedWorkbook{
		SourceType: "excel",
		Sheets: []ParsedSheet{
			{
				Name:        "Sheet1",
				Rows:        [][]string{{"Name", "Amount"}, {"Alice", "10"}, {"Bob", "bad"}, {"Carol", "30"}},
				RowCount:    4,
				ColumnCount: 2,
				Recommended: true,
			},
		},
	}
	req := emptyConfirmRequest()
	req.Options.ErrorPolicy = "fail_fast"
	req.Columns = []dto.InferredExcelColumn{
		{SourceColumnIndex: 0, Name: "name", Type: "text", IsRequired: true},
		{SourceColumnIndex: 1, Name: "amount", Type: "numeric", IsRequired: true},
	}

	result, err := ValidateRows(wb, req)
	if err != nil {
		t.Fatalf("ValidateRows returned error: %v", err)
	}
	if got := len(result.Records); got != 1 {
		t.Fatalf("valid records before first invalid row = %d, want 1", got)
	}
	if got := len(result.Errors); got != 2 {
		t.Fatalf("errors = %d, want invalid value and missing required", got)
	}
}

func workbookBytes(t *testing.T, sheets map[string][][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defaultSheet := f.GetSheetName(0)
	first := true
	for name, rows := range sheets {
		if first {
			if err := f.SetSheetName(defaultSheet, name); err != nil {
				t.Fatalf("SetSheetName: %v", err)
			}
			first = false
		} else if _, err := f.NewSheet(name); err != nil {
			t.Fatalf("NewSheet: %v", err)
		}
		for rowIndex, row := range rows {
			for colIndex, value := range row {
				cell, err := excelize.CoordinatesToCellName(colIndex+1, rowIndex+1)
				if err != nil {
					t.Fatalf("CoordinatesToCellName: %v", err)
				}
				if err := f.SetCellValue(name, cell, value); err != nil {
					t.Fatalf("SetCellValue: %v", err)
				}
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("Write workbook: %v", err)
	}
	return buf.Bytes()
}
