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

func TestParseXLSXDoesNotNormalizeNumericCustomFormatsAsDates(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   string
	}{
		{name: "color", format: `[Red]#,##0`, want: "12,345"},
		{name: "quoted currency", format: `"USD" 0.00`, want: "USD 12345.00"},
		{name: "escaped date letter", format: `0.00\d`, want: "12345.00d"},
		{name: "condition", format: `[>=100]0.00`, want: "12345.00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := excelize.NewFile()
			defer f.Close()
			f.SetCellValue("Sheet1", "A1", "amount")
			f.SetCellValue("Sheet1", "A2", 12345)
			style, err := f.NewStyle(&excelize.Style{CustomNumFmt: &tt.format})
			if err != nil {
				t.Fatal(err)
			}
			if err := f.SetCellStyle("Sheet1", "A2", "A2", style); err != nil {
				t.Fatal(err)
			}
			content, err := f.WriteToBuffer()
			if err != nil {
				t.Fatal(err)
			}

			workbook, err := ParseWorkbook("amounts.xlsx", content.Bytes())
			if err != nil {
				t.Fatalf("ParseWorkbook returned error: %v", err)
			}
			if got := workbook.Sheets[0].Rows[1][0]; got != tt.want {
				t.Fatalf("parsed value = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseXLSXPreservesElapsedTimeValues(t *testing.T) {
	tests := []struct {
		name  string
		style *excelize.Style
		want  string
	}{
		{name: "custom elapsed hours", style: &excelize.Style{CustomNumFmt: stringPointer(`[h]:mm`)}, want: "36:00"},
		{name: "built-in elapsed hours", style: &excelize.Style{NumFmt: 46}, want: "36:00:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := excelize.NewFile()
			defer f.Close()
			f.SetCellValue("Sheet1", "A1", "duration")
			f.SetCellValue("Sheet1", "A2", 1.5)
			styleID, err := f.NewStyle(tt.style)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.SetCellStyle("Sheet1", "A2", "A2", styleID); err != nil {
				t.Fatal(err)
			}
			content, err := f.WriteToBuffer()
			if err != nil {
				t.Fatal(err)
			}

			workbook, err := ParseWorkbook("durations.xlsx", content.Bytes())
			if err != nil {
				t.Fatalf("ParseWorkbook returned error: %v", err)
			}
			if got := workbook.Sheets[0].Rows[1][0]; got != tt.want {
				t.Fatalf("parsed duration = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExcelCustomDateFormatScanner(t *testing.T) {
	tests := []struct {
		format string
		want   bool
	}{
		{format: `yyyy-mm-dd`, want: true},
		{format: `hh:mm:ss`, want: true},
		{format: `[h]:mm`, want: false},
		{format: `[m]`, want: false},
		{format: `[s]`, want: false},
		{format: `[Red]#,##0`, want: false},
		{format: `"USD" 0.00`, want: false},
		{format: `0.00\d`, want: false},
		{format: `[>=100]0.00`, want: false},
		{format: `[$-409]#,##0.00`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			if got := isExcelCustomDateFormat(tt.format); got != tt.want {
				t.Fatalf("isExcelCustomDateFormat(%q) = %v, want %v", tt.format, got, tt.want)
			}
		})
	}
}

func stringPointer(value string) *string {
	return &value
}
