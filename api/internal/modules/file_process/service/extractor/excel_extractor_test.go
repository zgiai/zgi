package extractor

import (
	"context"
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/extrame/xls"
	"github.com/xuri/excelize/v2"
)

func TestSafeXlsRowRecoversEmptySheetPanic(t *testing.T) {
	row, ok, panicValue := safeXlsRow(&xls.WorkSheet{Name: "Empty"}, 0)
	if ok {
		t.Fatalf("safeXlsRow ok = true, want false")
	}
	if row != nil {
		t.Fatalf("safeXlsRow row = %#v, want nil", row)
	}
	if panicValue == nil {
		t.Fatalf("safeXlsRow panicValue = nil, want recovered panic")
	}
}

func TestXlsRowValuesRecoversInvalidRowPanic(t *testing.T) {
	values, ok, panicValue := xlsRowValues(&xls.Row{})
	if ok {
		t.Fatalf("xlsRowValues ok = true, want false")
	}
	if values != nil {
		t.Fatalf("xlsRowValues values = %#v, want nil", values)
	}
	if panicValue == nil {
		t.Fatalf("xlsRowValues panicValue = nil, want recovered panic")
	}
}

func TestExcelExtractorDocumentsFromXlsSheetsSkipsEmptyAndKeepsValidSheets(t *testing.T) {
	extractor := NewExcelExtractor("mixed.xls")
	sheets := []legacyXlsSheetReader{
		fakeLegacyXlsSheet{
			sheetName: "Data1",
			rowLimit:  1,
			rows: map[int]fakeLegacyXlsRow{
				0: {cells: []string{"Name", "Amount"}},
				1: {cells: []string{"Alice", "10"}},
			},
		},
		fakeLegacyXlsSheet{
			sheetName: "EmptyMiddle",
			rowLimit:  0,
		},
		fakeLegacyXlsSheet{
			sheetName: "Data2",
			rowLimit:  1,
			rows: map[int]fakeLegacyXlsRow{
				0: {cells: []string{"Name", "Amount"}},
				1: {cells: []string{"Bob", "20"}},
			},
		},
	}

	documents := documentsFromFakeXlsSheets(t, extractor, sheets)
	if len(documents) != 2 {
		t.Fatalf("document count = %d, want 2", len(documents))
	}
	if got := documents[0].PageContent; got != `"Name":"Alice";"Amount":"10"` {
		t.Fatalf("first document content = %q", got)
	}
	if got := documents[1].PageContent; got != `"Name":"Bob";"Amount":"20"` {
		t.Fatalf("second document content = %q", got)
	}
}

func TestExcelExtractorDocumentsFromXlsSheetsOnlyEmptyReturnsNoDocuments(t *testing.T) {
	extractor := NewExcelExtractor("empty-only.xls")
	documents := documentsFromFakeXlsSheets(t, extractor, []legacyXlsSheetReader{
		fakeLegacyXlsSheet{
			sheetName: "Empty",
			rowLimit:  0,
		},
	})

	if len(documents) != 0 {
		t.Fatalf("document count = %d, want 0", len(documents))
	}
}

func TestExcelExtractorDocumentsFromXlsSheetsSkipsAbnormalSheetAndRow(t *testing.T) {
	extractor := NewExcelExtractor("abnormal.xls")
	sheets := []legacyXlsSheetReader{
		fakeLegacyXlsSheet{
			sheetName: "BadHeader",
			rowLimit:  1,
			rowPanics: map[int]interface{}{
				0: "bad header row",
			},
		},
		fakeLegacyXlsSheet{
			sheetName: "Recoverable",
			rowLimit:  2,
			rows: map[int]fakeLegacyXlsRow{
				0: {cells: []string{"Name", "Amount"}},
				2: {cells: []string{"Carol", "30"}},
			},
			rowPanics: map[int]interface{}{
				1: "bad data row",
			},
		},
	}

	documents := documentsFromFakeXlsSheets(t, extractor, sheets)
	if len(documents) != 1 {
		t.Fatalf("document count = %d, want 1", len(documents))
	}
	if got := documents[0].Metadata["sheet"]; got != "Recoverable" {
		t.Fatalf("document sheet = %v, want Recoverable", got)
	}
	if got := documents[0].PageContent; got != `"Name":"Carol";"Amount":"30"` {
		t.Fatalf("document content = %q", got)
	}
}

func TestExcelExtractorHandleXlsReadsLegacyWorkbook(t *testing.T) {
	sourcePath := filepath.Join(build.Default.GOPATH, "pkg", "mod", "github.com", "extrame", "xls@v0.0.1", "Table.xls")
	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", sourcePath, err)
	}

	filePath := filepath.Join(t.TempDir(), "legacy.xls")
	if err := os.WriteFile(filePath, raw, 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", filePath, err)
	}

	documents, err := NewExcelExtractor(filePath).handleXls(context.Background())
	if err != nil {
		t.Fatalf("handleXls returned error: %v", err)
	}
	if len(documents) == 0 {
		t.Fatalf("handleXls returned no documents, want extracted rows")
	}
	if documents[0].PageContent == "" {
		t.Fatalf("first document PageContent is empty")
	}
	if got := documents[0].Metadata["sheet"]; got == "" {
		t.Fatalf("first document sheet metadata = %v, want sheet name", got)
	}
}

func TestExcelExtractorHandleXlsxStillReadsWorkbook(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "sample.xlsx")
	f := excelize.NewFile()
	sheetName := "Sheet1"
	if err := f.SetCellValue(sheetName, "A1", "Name"); err != nil {
		t.Fatalf("SetCellValue header error = %v", err)
	}
	if err := f.SetCellValue(sheetName, "A2", "Alice"); err != nil {
		t.Fatalf("SetCellValue row error = %v", err)
	}
	if err := f.SaveAs(filePath); err != nil {
		t.Fatalf("SaveAs(%s) error = %v", filePath, err)
	}

	documents, err := NewExcelExtractor(filePath).handleXlsx(context.Background())
	if err != nil {
		t.Fatalf("handleXlsx returned error: %v", err)
	}
	if len(documents) != 1 {
		t.Fatalf("handleXlsx document count = %d, want 1", len(documents))
	}
	if !strings.Contains(documents[0].PageContent, `"Name":"Alice"`) {
		t.Fatalf("handleXlsx PageContent = %q, want Name/Alice content", documents[0].PageContent)
	}
}

type fakeLegacyXlsSheet struct {
	sheetName string
	rowLimit  int
	rows      map[int]fakeLegacyXlsRow
	rowPanics map[int]interface{}
}

func (s fakeLegacyXlsSheet) name() string {
	return s.sheetName
}

func (s fakeLegacyXlsSheet) maxRow() int {
	return s.rowLimit
}

func (s fakeLegacyXlsSheet) row(rowIndex int) (legacyXlsRowReader, bool, interface{}) {
	if panicValue, ok := s.rowPanics[rowIndex]; ok {
		return nil, false, panicValue
	}
	row, ok := s.rows[rowIndex]
	if !ok {
		return nil, false, nil
	}
	return row, true, nil
}

type fakeLegacyXlsRow struct {
	cells      []string
	panicValue interface{}
}

func (r fakeLegacyXlsRow) values() ([]string, bool, interface{}) {
	if r.panicValue != nil {
		return nil, false, r.panicValue
	}
	return r.cells, true, nil
}

func documentsFromFakeXlsSheets(t *testing.T, extractor *ExcelExtractor, sheets []legacyXlsSheetReader) []interfaceDocument {
	t.Helper()

	documents := make([]interfaceDocument, 0)
	for sheetIndex, sheet := range sheets {
		sheetDocuments, err := extractor.documentsFromXlsSheet(context.Background(), sheet, sheetIndex)
		if err != nil {
			t.Fatalf("documentsFromXlsSheet returned error: %v", err)
		}
		for _, document := range sheetDocuments {
			documents = append(documents, interfaceDocument{
				PageContent: document.PageContent,
				Metadata:    document.Metadata,
			})
		}
	}
	return documents
}

type interfaceDocument struct {
	PageContent string
	Metadata    map[string]interface{}
}
