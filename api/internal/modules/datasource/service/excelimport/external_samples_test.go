package excelimport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalExcelSamplesAnalyze(t *testing.T) {
	dir := os.Getenv("EXCEL_IMPORT_SAMPLE_DIR")
	if dir == "" {
		t.Skip("set EXCEL_IMPORT_SAMPLE_DIR to run external Excel sample smoke tests")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	checked := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".xlsx") && !strings.HasSuffix(strings.ToLower(name), ".xls") {
			continue
		}
		checked++
		t.Run(name, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			wb, err := ParseWorkbook(name, content)
			if err != nil {
				t.Fatalf("ParseWorkbook: %v", err)
			}
			result, err := AnalyzeWorkbook(wb, AnalyzeOptions{SampleSize: 100})
			if err != nil {
				t.Fatalf("AnalyzeWorkbook: %v", err)
			}
			if len(result.Columns) == 0 {
				t.Fatalf("expected inferred columns")
			}
			if result.Selection.SheetName == "" {
				t.Fatalf("expected selected sheet")
			}
		})
	}
	if checked == 0 {
		t.Fatalf("no Excel files found in %s", dir)
	}
}
