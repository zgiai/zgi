package excelimport

import "github.com/zgiai/zgi/api/internal/dto"

type ParsedWorkbook struct {
	SourceType string
	Sheets     []ParsedSheet
}

type ParsedSheet struct {
	Name        string
	Rows        [][]string
	RowCount    int
	ColumnCount int
	Hidden      bool
	Recommended bool
}

type AnalyzeOptions struct {
	SheetName  *string
	HeaderRow  *int
	SampleSize int
}

type AnalysisResult struct {
	Selection struct {
		SheetName string
		HeaderRow int
		StartRow  int
	}
	Sheets      []dto.ExcelImportSheet
	Columns     []dto.InferredExcelColumn
	PreviewRows []dto.ExcelImportPreviewRow
	Warnings    []dto.ExcelImportWarning
	TotalRows   int
	ValidRows   int
}

type RowValidationResult struct {
	Records   []map[string]interface{}
	Errors    []dto.ExcelImportFailedItem
	TotalRows int
}
