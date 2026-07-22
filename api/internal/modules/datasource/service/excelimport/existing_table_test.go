package excelimport

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
)

func TestMatchExistingTableColumnsUsesExactNamesAndCountsExtraFields(t *testing.T) {
	source := []dto.InferredExcelColumn{
		{SourceColumn: "员工姓名", SourceColumnIndex: 0, Type: "text"},
		{SourceColumn: "age", SourceColumnIndex: 1, Type: "integer"},
		{SourceColumn: "extra", SourceColumnIndex: 2, Type: "text"},
	}
	sourceColumnName := "员工姓名"
	target := []dto.TableColumn{
		{ID: "name-id", Name: "employee_name", SourceColumnName: &sourceColumnName, Type: "text"},
		{ID: "age-id", Name: "age", Type: "integer"},
	}

	mappings, rate := MatchExistingTableColumns(source, target, nil)
	if len(mappings) != 3 {
		t.Fatalf("got %d mappings, want 3", len(mappings))
	}
	if mappings[0].Status != dto.ExcelImportMatchExact || mappings[0].TargetColumnName != "employee_name" {
		t.Fatalf("first mapping = %#v, want exact employee_name", mappings[0])
	}
	if mappings[2].Status != dto.ExcelImportMatchUnmatched || mappings[2].Confirmed {
		t.Fatalf("extra source mapping = %#v, want unconfirmed unmatched", mappings[2])
	}
	if rate != 66.7 {
		t.Fatalf("overall match rate = %v, want 66.7", rate)
	}
}

func TestMatchExistingTableColumnsUsesConfirmedAlias(t *testing.T) {
	source := []dto.InferredExcelColumn{{SourceColumn: "联系电话", SourceColumnIndex: 0, Type: "text"}}
	target := []dto.TableColumn{{ID: "phone-id", Name: "phone", Type: "text"}}
	aliases := map[string]map[string]struct{}{
		NormalizeImportHeader("联系电话"): {"phone-id": {}},
	}

	mappings, rate := MatchExistingTableColumns(source, target, aliases)
	if mappings[0].Reason != "history_alias_exact" || mappings[0].Status != dto.ExcelImportMatchExact {
		t.Fatalf("mapping = %#v, want exact history alias", mappings[0])
	}
	if rate != 100 {
		t.Fatalf("overall match rate = %v, want 100", rate)
	}
}

func TestValidateExistingTableDraftRejectsUnconfirmedAndRequiredSkip(t *testing.T) {
	workbook := existingTableTestWorkbook()
	selection := existingTableTestSelection()

	_, err := ValidateExistingTableDraft(workbook, selection, dto.ExistingTableExcelImportDraftRequest{Mappings: []dto.ExistingTableExcelImportMapping{{
		SourceColumn:     existingTableStringPointer("姓名"),
		TargetColumnName: "name",
		TargetType:       "text",
		Status:           dto.ExcelImportMatchPossible,
		Action:           dto.ExcelImportMappingMap,
	}}})
	if err == nil {
		t.Fatal("expected unconfirmed mapping to fail")
	}

	_, err = ValidateExistingTableDraft(workbook, selection, dto.ExistingTableExcelImportDraftRequest{Mappings: []dto.ExistingTableExcelImportMapping{{
		TargetColumnName: "name",
		TargetType:       "text",
		TargetRequired:   true,
		Status:           dto.ExcelImportMatchUnmatched,
		Action:           dto.ExcelImportMappingSkip,
		Confirmed:        true,
	}}})
	if err == nil {
		t.Fatal("expected required target skip to fail")
	}
}

func TestValidateExistingTableDraftFailsWholeRowAndSeparatesSkippedRows(t *testing.T) {
	workbook := existingTableTestWorkbook()
	selection := existingTableTestSelection()
	nameIndex := 0
	ageIndex := 1
	fixedDepartment := "研发"
	draft := dto.ExistingTableExcelImportDraftRequest{
		Mappings: []dto.ExistingTableExcelImportMapping{
			{SourceColumn: existingTableStringPointer("姓名"), SourceColumnIndex: &nameIndex, TargetColumnName: "name", TargetType: "text", TargetRequired: true, Status: dto.ExcelImportMatchExact, Action: dto.ExcelImportMappingMap, Confirmed: true},
			{SourceColumn: existingTableStringPointer("年龄"), SourceColumnIndex: &ageIndex, TargetColumnName: "age", TargetType: "integer", Status: dto.ExcelImportMatchExact, Action: dto.ExcelImportMappingMap, Confirmed: true},
			{TargetColumnName: "department", TargetType: "text", TargetRequired: true, Status: dto.ExcelImportMatchUnmatched, Action: dto.ExcelImportMappingFixed, FixedValue: &fixedDepartment, Confirmed: true},
		},
		SkippedRows: []int{4},
	}

	result, err := ValidateExistingTableDraft(workbook, selection, draft)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRows != 3 || result.ValidRows != 1 || result.FailedRows != 1 || result.SkippedRows != 1 {
		t.Fatalf("counts = total:%d valid:%d failed:%d skipped:%d", result.TotalRows, result.ValidRows, result.FailedRows, result.SkippedRows)
	}
	if len(result.Records) != 1 || len(result.Errors) != 1 {
		t.Fatalf("records/errors = %d/%d, want 1/1", len(result.Records), len(result.Errors))
	}
	if result.Records[0]["department"] != "研发" {
		t.Fatalf("fixed department = %#v, want 研发", result.Records[0]["department"])
	}
}

func TestValidateExistingTableDraftReturnsEmptyErrorsForValidRows(t *testing.T) {
	workbook := &ParsedWorkbook{Sheets: []ParsedSheet{{
		Name:        "Sheet1",
		ColumnCount: 1,
		Rows:        [][]string{{"Name"}, {"Alice"}},
	}}}
	selection := existingTableTestSelection()
	nameIndex := 0
	draft := dto.ExistingTableExcelImportDraftRequest{Mappings: []dto.ExistingTableExcelImportMapping{{
		SourceColumn:      existingTableStringPointer("Name"),
		SourceColumnIndex: &nameIndex,
		TargetColumnName:  "name",
		TargetType:        "text",
		TargetRequired:    true,
		Status:            dto.ExcelImportMatchExact,
		Action:            dto.ExcelImportMappingMap,
		Confirmed:         true,
	}}}

	result, err := ValidateExistingTableDraft(workbook, selection, draft)
	if err != nil {
		t.Fatalf("ValidateExistingTableDraft returned error: %v", err)
	}
	if result.Errors == nil {
		t.Fatal("errors = nil, want empty slice for JSON array contract")
	}
}

func existingTableTestWorkbook() *ParsedWorkbook {
	return &ParsedWorkbook{Sheets: []ParsedSheet{{
		Name:        "Sheet1",
		ColumnCount: 2,
		Rows: [][]string{
			{"姓名", "年龄"},
			{"林清禾", "32"},
			{"周行远", "不是数字"},
			{"沈知夏", "27"},
		},
	}}}
}

func existingTableTestSelection() struct {
	SheetName string `json:"sheet_name"`
	HeaderRow int    `json:"header_row"`
	StartRow  int    `json:"start_row"`
} {
	return struct {
		SheetName string `json:"sheet_name"`
		HeaderRow int    `json:"header_row"`
		StartRow  int    `json:"start_row"`
	}{SheetName: "Sheet1", HeaderRow: 1, StartRow: 2}
}

func existingTableStringPointer(value string) *string {
	return &value
}
