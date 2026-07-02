package service

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"github.com/zgiai/zgi/api/internal/dto"
)

func TestParseExcelFileSkipUnmatchedColumns(t *testing.T) {
	file := buildImportWorkbook(t, []string{"name", "extra"}, []string{"Ada", "ignored"})
	columns := []dto.TableColumn{
		{Name: "name", Type: "text", IsRequired: true},
	}

	svc := &dataSourceService{}
	records, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, true)
	if err != nil {
		t.Fatalf("parseExcelFile() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if got := records[0]["name"]; got != "Ada" {
		t.Fatalf("records[0][name] = %v, want Ada", got)
	}
	if _, exists := records[0]["extra"]; exists {
		t.Fatalf("records[0] contains skipped field extra: %#v", records[0])
	}
}

func TestParseExcelFileSkipUnmatchedColumnsDropsRowsWithoutMatchedFields(t *testing.T) {
	file := buildImportWorkbook(t, []string{"extra", "name"}, []string{"ignored"})
	columns := []dto.TableColumn{
		{Name: "name", Type: "text"},
	}

	svc := &dataSourceService{}
	records, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, true)
	if err != nil {
		t.Fatalf("parseExcelFile() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("len(records) = %d, want 0; records = %#v", len(records), records)
	}
}

func TestParseExcelFileRejectsUnmatchedColumnsByDefault(t *testing.T) {
	file := buildImportWorkbook(t, []string{"name", "extra"}, []string{"Ada", "ignored"})
	columns := []dto.TableColumn{
		{Name: "name", Type: "text", IsRequired: true},
	}

	svc := &dataSourceService{}
	_, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, false)
	if err == nil {
		t.Fatal("parseExcelFile() error = nil, want unmatched column error")
	}
	if !strings.Contains(err.Error(), "column 'extra' does not exist in table") {
		t.Fatalf("parseExcelFile() error = %q, want unmatched column error", err.Error())
	}
}

func TestParseExcelFileRequiresMatchedRequiredColumns(t *testing.T) {
	file := buildImportWorkbook(t, []string{"notes", "extra"}, []string{"hello", "ignored"})
	columns := []dto.TableColumn{
		{Name: "name", Type: "text", IsRequired: true},
		{Name: "notes", Type: "text"},
	}

	svc := &dataSourceService{}
	_, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, true)
	if err == nil {
		t.Fatal("parseExcelFile() error = nil, want missing required column error")
	}
	if !strings.Contains(err.Error(), "missing required columns: name") {
		t.Fatalf("parseExcelFile() error = %q, want missing required column error", err.Error())
	}
}

func TestParseExcelFileMatchesDatabaseColumnName(t *testing.T) {
	file := buildImportWorkbook(t, []string{"user_id"}, []string{"u_001"})
	columns := []dto.TableColumn{
		{Name: "user_id", Type: "text", IsRequired: true},
	}

	svc := &dataSourceService{}
	records, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, false)
	if err != nil {
		t.Fatalf("parseExcelFile() error = %v", err)
	}
	if got := records[0]["user_id"]; got != "u_001" {
		t.Fatalf("records[0][user_id] = %v, want u_001", got)
	}
}

func TestParseExcelFileMatchesSourceColumnName(t *testing.T) {
	file := buildImportWorkbook(t, []string{"用户ID", "手机号"}, []string{"u_001", "13800000000"})
	userIDSource := "用户ID"
	phoneSource := "手机号"
	columns := []dto.TableColumn{
		{Name: "user_id", SourceColumnName: &userIDSource, Type: "text", IsRequired: true},
		{Name: "phone_number", SourceColumnName: &phoneSource, Type: "text"},
	}

	svc := &dataSourceService{}
	records, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, false)
	if err != nil {
		t.Fatalf("parseExcelFile() error = %v", err)
	}
	if got := records[0]["user_id"]; got != "u_001" {
		t.Fatalf("records[0][user_id] = %v, want u_001", got)
	}
	if got := records[0]["phone_number"]; got != "13800000000" {
		t.Fatalf("records[0][phone_number] = %v, want 13800000000", got)
	}
	if _, exists := records[0]["手机号"]; exists {
		t.Fatalf("records[0] contains source header key 手机号: %#v", records[0])
	}
}

func TestParseExcelFileRejectsAmbiguousSourceColumnName(t *testing.T) {
	file := buildImportWorkbook(t, []string{"状态"}, []string{"启用"})
	statusSource := "状态"
	columns := []dto.TableColumn{
		{Name: "status", SourceColumnName: &statusSource, Type: "text"},
		{Name: "state", SourceColumnName: &statusSource, Type: "text"},
	}

	svc := &dataSourceService{}
	_, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, false)
	if err == nil {
		t.Fatal("parseExcelFile() error = nil, want ambiguous column error")
	}
	if !strings.Contains(err.Error(), "Excel 表头「状态」匹配到多个字段") {
		t.Fatalf("parseExcelFile() error = %q, want ambiguous column error", err.Error())
	}
}

func TestParseExcelFileRequiredColumnCanMatchSourceColumnName(t *testing.T) {
	file := buildImportWorkbook(t, []string{"用户ID"}, []string{"u_001"})
	userIDSource := "用户ID"
	columns := []dto.TableColumn{
		{Name: "user_id", SourceColumnName: &userIDSource, Type: "text", IsRequired: true},
	}

	svc := &dataSourceService{}
	records, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, false)
	if err != nil {
		t.Fatalf("parseExcelFile() error = %v", err)
	}
	if got := records[0]["user_id"]; got != "u_001" {
		t.Fatalf("records[0][user_id] = %v, want u_001", got)
	}
}

func TestWithSourceColumnNamesAppliesOriginalHeaders(t *testing.T) {
	columns := []dto.TableColumn{
		{Name: "user_id", Type: "integer", IsRequired: true},
		{Name: "mobile_phone", Type: "text", IsRequired: true},
	}

	got := withSourceColumnNames(columns, []string{"用户ID", "手机号"})

	if got[0].SourceColumnName == nil || *got[0].SourceColumnName != "用户ID" {
		t.Fatalf("got[0].SourceColumnName = %v, want 用户ID", got[0].SourceColumnName)
	}
	if got[1].SourceColumnName == nil || *got[1].SourceColumnName != "手机号" {
		t.Fatalf("got[1].SourceColumnName = %v, want 手机号", got[1].SourceColumnName)
	}
	if got[1].DisplayName == nil || *got[1].DisplayName != "手机号" {
		t.Fatalf("got[1].DisplayName = %v, want 手机号", got[1].DisplayName)
	}
}

func TestWithSourceColumnNamesSkipsMismatchedHeaderCount(t *testing.T) {
	columns := []dto.TableColumn{{Name: "user_id", Type: "integer"}}

	got := withSourceColumnNames(columns, []string{"用户ID", "手机号"})

	if got[0].SourceColumnName != nil {
		t.Fatalf("got[0].SourceColumnName = %v, want nil", got[0].SourceColumnName)
	}
}

func TestTableColumnSourceSchemaUsesSourceColumnName(t *testing.T) {
	desc := "用户唯一标识"
	columns := []dto.TableColumn{
		{Name: "user_id", SourceColumnName: stringPtr("用户ID"), Type: "integer", IsRequired: true, Description: &desc},
		{Name: "notes", Type: "text"},
	}

	schema, ok := tableColumnSourceSchema(columns)
	if !ok {
		t.Fatal("tableColumnSourceSchema() ok = false, want true")
	}
	if len(schema) != 1 {
		t.Fatalf("len(schema) = %d, want 1", len(schema))
	}
	if schema[0].SourceColumn != "用户ID" || schema[0].Name != "user_id" {
		t.Fatalf("schema[0] = %#v, want source 用户ID and name user_id", schema[0])
	}
}
func TestExcelSourceHeadersReadsOriginalHeaders(t *testing.T) {
	file := buildImportWorkbook(t, []string{"用户ID", "手机号"}, []string{"1001", "13800138000"})

	headers, err := excelSourceHeaders("users.xlsx", file)
	if err != nil {
		t.Fatalf("excelSourceHeaders() error = %v", err)
	}
	if len(headers) != 2 || headers[0] != "用户ID" || headers[1] != "手机号" {
		t.Fatalf("excelSourceHeaders() = %#v, want [用户ID 手机号]", headers)
	}
}
func buildImportWorkbook(t *testing.T, headers []string, values []string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "Sheet1"
	for index, header := range headers {
		cell, err := excelize.CoordinatesToCellName(index+1, 1)
		if err != nil {
			t.Fatalf("CoordinatesToCellName() error = %v", err)
		}
		if err := f.SetCellValue(sheet, cell, header); err != nil {
			t.Fatalf("SetCellValue(header) error = %v", err)
		}
	}
	for index, value := range values {
		cell, err := excelize.CoordinatesToCellName(index+1, 2)
		if err != nil {
			t.Fatalf("CoordinatesToCellName() error = %v", err)
		}
		if err := f.SetCellValue(sheet, cell, value); err != nil {
			t.Fatalf("SetCellValue(value) error = %v", err)
		}
	}
	buffer, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("WriteToBuffer() error = %v", err)
	}
	return buffer.Bytes()
}
