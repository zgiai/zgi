package excelimport

import (
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestParseXLSXNormalizesTypedDateBelowTitleRow(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	f.SetCellValue("Sheet1", "A1", "员工信息库")
	if err := f.MergeCell("Sheet1", "A1", "B1"); err != nil {
		t.Fatal(err)
	}
	f.SetCellValue("Sheet1", "A2", "工号")
	f.SetCellValue("Sheet1", "B2", "入职日期")
	f.SetCellValue("Sheet1", "A3", "10001")
	f.SetCellValue("Sheet1", "B3", time.Date(2023, time.April, 15, 0, 0, 0, 0, time.Local))
	style, err := f.NewStyle(&excelize.Style{NumFmt: 14})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStyle("Sheet1", "B3", "B3", style); err != nil {
		t.Fatal(err)
	}
	content, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}

	workbook, err := ParseWorkbook("employees.xlsx", content.Bytes())
	if err != nil {
		t.Fatalf("ParseWorkbook returned error: %v", err)
	}
	if got := workbook.Sheets[0].Rows[2][1]; got != "2023-04-15 00:00:00" {
		t.Fatalf("parsed date = %q, want 2023-04-15 00:00:00", got)
	}
	analysis, err := AnalyzeWorkbook(workbook, AnalyzeOptions{})
	if err != nil {
		t.Fatalf("AnalyzeWorkbook returned error: %v", err)
	}
	if analysis.Selection.HeaderRow != 2 {
		t.Fatalf("header row = %d, want 2", analysis.Selection.HeaderRow)
	}
	if got := analysis.Columns[1].Type; got != "timestamp" {
		t.Fatalf("date column type = %q, want timestamp", got)
	}
}
