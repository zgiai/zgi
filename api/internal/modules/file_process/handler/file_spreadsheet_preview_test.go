package handler

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestBuildDelimitedSpreadsheetPreview(t *testing.T) {
	preview, err := buildSpreadsheetPreview("people.csv", []byte("姓名,年龄\n杨一,23\n张三,89\n"), "csv")
	if err != nil {
		t.Fatalf("build preview: %v", err)
	}
	if preview.Engine != "csv" || len(preview.Sheets) != 1 {
		t.Fatalf("preview=%+v", preview)
	}
	sheet := preview.Sheets[0]
	if sheet.ColumnCount != 2 || sheet.TotalRowCount != 3 || len(sheet.Rows) != 3 {
		t.Fatalf("sheet=%+v", sheet)
	}
	if got := sheet.Rows[1].Cells[0]; got != "杨一" {
		t.Fatalf("row cell=%q", got)
	}
}

func TestBuildXLSXSpreadsheetPreview(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	if err := f.SetSheetRow(sheet, "A1", &[]any{"姓名", "年龄"}); err != nil {
		t.Fatalf("set header: %v", err)
	}
	if err := f.SetSheetRow(sheet, "A2", &[]any{"杨一", 23}); err != nil {
		t.Fatalf("set row: %v", err)
	}
	if err := f.Write(buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}

	preview, err := buildSpreadsheetPreview("people.xlsx", buf.Bytes(), "xlsx")
	if err != nil {
		t.Fatalf("build preview: %v", err)
	}
	if preview.Engine != "excelize" || len(preview.Sheets) != 1 {
		t.Fatalf("preview=%+v", preview)
	}
	sheetPreview := preview.Sheets[0]
	if sheetPreview.Rows[1].Cells[0] != "杨一" || sheetPreview.Rows[1].Cells[1] != "23" {
		t.Fatalf("rows=%+v", sheetPreview.Rows)
	}
}
