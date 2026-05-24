package excelimport

import (
	"bytes"
	"go/build"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
	"github.com/zgiai/zgi/api/internal/dto"
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

func TestParseWorkbookReadsLegacyXLS(t *testing.T) {
	content := legacyXLSFixtureBytes(t)
	wb, err := ParseWorkbook("legacy.xls", content)
	if err != nil {
		t.Fatalf("ParseWorkbook returned error: %v", err)
	}
	if wb.SourceType != "excel" {
		t.Fatalf("source type = %q, want excel", wb.SourceType)
	}
	if len(wb.Sheets) == 0 {
		t.Fatalf("expected at least one sheet")
	}
	sheet := wb.Sheets[0]
	if sheet.Name == "" {
		t.Fatalf("expected sheet name")
	}
	if len(sheet.Rows) == 0 {
		t.Fatalf("expected rows from legacy xls")
	}
	if sheet.ColumnCount == 0 {
		t.Fatalf("expected columns from legacy xls")
	}
}

func TestParseWorkbookReadsSpreadsheetMLXLS(t *testing.T) {
	content := []byte(`<?xml version="1.0"?>
<?mso-application progid="Excel.Sheet"?>
<Workbook xmlns="urn:schemas-microsoft-com:office:spreadsheet"
  xmlns:ss="urn:schemas-microsoft-com:office:spreadsheet">
  <Worksheet ss:Name="Intel Queue">
    <Table>
      <Row>
        <Cell><Data ss:Type="String">Intel ID</Data></Cell>
        <Cell><Data ss:Type="String">Risk Level</Data></Cell>
      </Row>
      <Row>
        <Cell><Data ss:Type="String">RI-XLS-2026-0001</Data></Cell>
        <Cell><Data ss:Type="String">High</Data></Cell>
      </Row>
    </Table>
  </Worksheet>
</Workbook>`)
	wb, err := ParseWorkbook("spreadsheetml.xls", content)
	if err != nil {
		t.Fatalf("ParseWorkbook returned error: %v", err)
	}
	if len(wb.Sheets) != 1 {
		t.Fatalf("sheet count = %d, want 1", len(wb.Sheets))
	}
	sheet := wb.Sheets[0]
	if sheet.Name != "Intel Queue" {
		t.Fatalf("sheet name = %q, want Intel Queue", sheet.Name)
	}
	if got := sheet.Rows[1][0]; got != "RI-XLS-2026-0001" {
		t.Fatalf("first data cell = %q, want RI-XLS-2026-0001", got)
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

func TestValidateRowsKeepsLocalTimestampWallTime(t *testing.T) {
	wb := &ParsedWorkbook{
		SourceType: "excel",
		Sheets: []ParsedSheet{
			{
				Name:        "Sheet1",
				Rows:        [][]string{{"Collection Time"}, {"2026-05-11 17:17:18"}},
				RowCount:    2,
				ColumnCount: 1,
				Recommended: true,
			},
		},
	}
	req := emptyConfirmRequest()
	req.Columns = []dto.InferredExcelColumn{
		{SourceColumnIndex: 0, Name: "collection_time", Type: "timestamp", IsRequired: true},
	}

	result, err := ValidateRows(wb, req)
	if err != nil {
		t.Fatalf("ValidateRows returned error: %v", err)
	}
	if got := result.Records[0]["collection_time"]; got != "2026-05-11 17:17:18" {
		t.Fatalf("collection_time = %#v, want original wall time", got)
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

func legacyXLSFixtureBytes(t *testing.T) []byte {
	t.Helper()
	modCache := os.Getenv("GOMODCACHE")
	if modCache == "" {
		modCache = filepath.Join(build.Default.GOPATH, "pkg", "mod")
	}
	path := filepath.Join(modCache, "github.com", "extrame", "xls@v0.0.1", "Table.xls")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return content
}
