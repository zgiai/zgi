package excelimport

import (
	"testing"

	"github.com/zgiai/ginext/internal/dto"
)

func TestAnalyzeWorkbookKeepsSourceHeaderAndNormalizesFieldName(t *testing.T) {
	wb := &ParsedWorkbook{
		SourceType: "excel",
		Sheets: []ParsedSheet{
			{
				Name:        "Sheet1",
				Rows:        [][]string{{"Customer ID", "Amount"}, {"C-1", "12.5"}},
				RowCount:    2,
				ColumnCount: 2,
				Recommended: true,
			},
		},
	}

	result, err := AnalyzeWorkbook(wb, AnalyzeOptions{})
	if err != nil {
		t.Fatalf("AnalyzeWorkbook returned error: %v", err)
	}
	if got := result.Columns[0].SourceColumn; got != "Customer ID" {
		t.Fatalf("source column = %q, want Customer ID", got)
	}
	if got := result.Columns[0].SourceColumnIndex; got != 0 {
		t.Fatalf("source column index = %d, want 0", got)
	}
	if got := result.Columns[0].Name; got != "customer_id" {
		t.Fatalf("field name = %q, want customer_id", got)
	}
	if _, ok := result.PreviewRows[0].Values["customer_id"]; !ok {
		t.Fatalf("preview row should be keyed by normalized field name")
	}
}

func TestValidateRowsUsesSourceColumnIndexForDuplicateHeaders(t *testing.T) {
	wb := &ParsedWorkbook{
		SourceType: "excel",
		Sheets: []ParsedSheet{
			{
				Name:        "Sheet1",
				Rows:        [][]string{{"Name", "Name"}, {"left", "right"}},
				RowCount:    2,
				ColumnCount: 2,
				Recommended: true,
			},
		},
	}

	req := emptyConfirmRequest()
	req.Selection.SheetName = "Sheet1"
	req.Selection.HeaderRow = 1
	req.Selection.StartRow = 2
	req.Columns = []dto.InferredExcelColumn{
		{SourceColumn: "Name", SourceColumnIndex: 0, Name: "name", Type: "text", IsRequired: true},
		{SourceColumn: "Name", SourceColumnIndex: 1, Name: "name_2", Type: "text", IsRequired: true},
	}

	result, err := ValidateRows(wb, req)
	if err != nil {
		t.Fatalf("ValidateRows returned error: %v", err)
	}
	if got := result.Records[0]["name"]; got != "left" {
		t.Fatalf("name = %v, want left", got)
	}
	if got := result.Records[0]["name_2"]; got != "right" {
		t.Fatalf("name_2 = %v, want right", got)
	}
}

func TestValidateImportSchemaRejectsInvalidNames(t *testing.T) {
	req := emptyConfirmRequest()
	req.Table.Name = "bad-name"
	req.Columns = []dto.InferredExcelColumn{
		{SourceColumnIndex: 0, Name: "valid_name", Type: "text"},
	}
	if err := ValidateImportSchema(req); err == nil {
		t.Fatalf("ValidateImportSchema should reject invalid table name")
	}

	req.Table.Name = "valid_table"
	req.Columns = []dto.InferredExcelColumn{
		{SourceColumnIndex: 0, Name: "id", Type: "text"},
	}
	if err := ValidateImportSchema(req); err == nil {
		t.Fatalf("ValidateImportSchema should reject reserved field name")
	}

	req.Columns = []dto.InferredExcelColumn{
		{SourceColumnIndex: 0, Name: "field", Type: "text"},
		{SourceColumnIndex: 1, Name: "field", Type: "text"},
	}
	if err := ValidateImportSchema(req); err == nil {
		t.Fatalf("ValidateImportSchema should reject duplicated field name")
	}
}

func emptyConfirmRequest() dto.ConfirmExcelImportRequest {
	var req dto.ConfirmExcelImportRequest
	req.Table.Name = "valid_table"
	req.Selection.SheetName = "Sheet1"
	req.Selection.HeaderRow = 1
	req.Selection.StartRow = 2
	req.Options.ErrorPolicy = "skip_invalid_rows"
	req.Options.EmptyRowPolicy = "skip"
	req.Options.BatchSize = 500
	return req
}

func TestAnalyzeWorkbookMapsChineseHeadersToDatabaseFieldNames(t *testing.T) {
	wb := &ParsedWorkbook{
		SourceType: "excel",
		Sheets: []ParsedSheet{
			{
				Name:        "BUG",
				Rows:        [][]string{{"备注", "bug描述", "bug描述"}, {"复现步骤", "图标显示不一致", "重复列"}},
				RowCount:    2,
				ColumnCount: 3,
				Recommended: true,
			},
		},
	}

	result, err := AnalyzeWorkbook(wb, AnalyzeOptions{})
	if err != nil {
		t.Fatalf("AnalyzeWorkbook returned error: %v", err)
	}

	want := []string{"remark", "bug_description", "bug_description_2"}
	for i, name := range want {
		if got := result.Columns[i].Name; got != name {
			t.Fatalf("column %d name = %q, want %q", i, got, name)
		}
		if _, ok := result.PreviewRows[0].Values[name]; !ok {
			t.Fatalf("preview row should contain key %q", name)
		}
	}
}

func TestAnalyzeWorkbookAvoidsSystemFieldNameCollisions(t *testing.T) {
	wb := &ParsedWorkbook{
		SourceType: "excel",
		Sheets: []ParsedSheet{
			{
				Name:        "Sheet1",
				Rows:        [][]string{{"ID", "uuid", "created_time", "updated_time"}, {"1", "u1", "2026-01-01", "2026-01-02"}},
				RowCount:    2,
				ColumnCount: 4,
				Recommended: true,
			},
		},
	}

	result, err := AnalyzeWorkbook(wb, AnalyzeOptions{})
	if err != nil {
		t.Fatalf("AnalyzeWorkbook returned error: %v", err)
	}

	want := []string{"id_value", "uuid_value", "created_time_value", "updated_time_value"}
	for i, name := range want {
		if got := result.Columns[i].Name; got != name {
			t.Fatalf("column %d name = %q, want %q", i, got, name)
		}
	}
}
