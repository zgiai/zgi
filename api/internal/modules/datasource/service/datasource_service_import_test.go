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
