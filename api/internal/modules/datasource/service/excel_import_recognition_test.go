package service

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
)

func TestNormalizeExcelRecognitionResultSanitizesNames(t *testing.T) {
	req := dto.RecognizeExcelImportRequest{
		Table: dto.RecognizeExcelImportTable{
			Name:        "fallback_table",
			Description: "fallback description",
		},
		Columns: []dto.InferredExcelColumn{
			{SourceColumnIndex: 0, SourceColumn: "合同编号", Name: "column_1", DisplayName: "合同编号", Type: "text"},
			{SourceColumnIndex: 1, SourceColumn: "客户名称", Name: "column_2", DisplayName: "客户名称", Type: "text"},
			{SourceColumnIndex: 2, SourceColumn: "客户联系人", Name: "column_3", DisplayName: "客户联系人", Type: "text"},
			{SourceColumnIndex: 3, SourceColumn: "备注", Name: "column_4", DisplayName: "备注", Type: "text"},
		},
	}

	var llmResult excelFieldRecognitionLLMResponse
	llmResult.Table.Name = "2026 合同表"
	llmResult.Table.Description = "合同台账"
	llmResult.Columns = append(llmResult.Columns,
		struct {
			SourceColumnIndex int    `json:"source_column_index"`
			SourceColumn      string `json:"source_column"`
			Name              string `json:"name"`
			DisplayName       string `json:"display_name"`
			Description       string `json:"description"`
		}{Name: "id", DisplayName: "合同编号", Description: "合同或订单编号"},
		struct {
			SourceColumnIndex int    `json:"source_column_index"`
			SourceColumn      string `json:"source_column"`
			Name              string `json:"name"`
			DisplayName       string `json:"display_name"`
			Description       string `json:"description"`
		}{Name: "customer name", DisplayName: "客户名称", Description: "客户公司名称"},
		struct {
			SourceColumnIndex int    `json:"source_column_index"`
			SourceColumn      string `json:"source_column"`
			Name              string `json:"name"`
			DisplayName       string `json:"display_name"`
			Description       string `json:"description"`
		}{Name: "customer-name", DisplayName: "客户联系人", Description: "客户侧联系人"},
		struct {
			SourceColumnIndex int    `json:"source_column_index"`
			SourceColumn      string `json:"source_column"`
			Name              string `json:"name"`
			DisplayName       string `json:"display_name"`
			Description       string `json:"description"`
		}{Name: "", DisplayName: "备注", Description: ""},
	)

	result, err := normalizeExcelRecognitionResult(req, llmResult, nil)
	if err != nil {
		t.Fatalf("normalizeExcelRecognitionResult returned error: %v", err)
	}
	if got := result.Table.Name; got != "fallback_table" {
		t.Fatalf("table name = %q, want fallback_table", got)
	}
	if got := result.Table.Description; got != "合同台账" {
		t.Fatalf("table description = %q, want 合同台账", got)
	}

	wantNames := []string{"id_value", "customer_name", "customer_name_2", "column_4"}
	for i, want := range wantNames {
		if got := result.Columns[i].Name; got != want {
			t.Fatalf("column %d name = %q, want %q", i, got, want)
		}
	}
	if got := result.Columns[3].Description; got != "" {
		t.Fatalf("blank model description should preserve input blank description, got %q", got)
	}
}

func TestNormalizeExcelRecognitionResultRejectsColumnCountMismatch(t *testing.T) {
	req := dto.RecognizeExcelImportRequest{
		Table: dto.RecognizeExcelImportTable{Name: "imported_table"},
		Columns: []dto.InferredExcelColumn{
			{SourceColumnIndex: 0, SourceColumn: "A", Name: "a", Type: "text"},
			{SourceColumnIndex: 1, SourceColumn: "B", Name: "b", Type: "text"},
		},
	}
	var llmResult excelFieldRecognitionLLMResponse
	llmResult.Columns = append(llmResult.Columns, struct {
		SourceColumnIndex int    `json:"source_column_index"`
		SourceColumn      string `json:"source_column"`
		Name              string `json:"name"`
		DisplayName       string `json:"display_name"`
		Description       string `json:"description"`
	}{Name: "a"})

	_, err := normalizeExcelRecognitionResult(req, llmResult, nil)
	if err == nil || !strings.Contains(err.Error(), "returned 1 columns, want 2") {
		t.Fatalf("error = %v, want column count mismatch", err)
	}
}

func TestNormalizeExcelRecognitionResultAvoidsExistingTableNames(t *testing.T) {
	req := dto.RecognizeExcelImportRequest{
		Table: dto.RecognizeExcelImportTable{Name: "invoice_records"},
		Columns: []dto.InferredExcelColumn{
			{SourceColumnIndex: 0, SourceColumn: "Invoice No", Name: "invoice_no", Type: "text"},
		},
	}
	var llmResult excelFieldRecognitionLLMResponse
	llmResult.Table.Name = "invoice_records"
	llmResult.Columns = append(llmResult.Columns, struct {
		SourceColumnIndex int    `json:"source_column_index"`
		SourceColumn      string `json:"source_column"`
		Name              string `json:"name"`
		DisplayName       string `json:"display_name"`
		Description       string `json:"description"`
	}{Name: "invoice_no"})

	result, err := normalizeExcelRecognitionResult(req, llmResult, map[string]struct{}{
		"invoice_records":   {},
		"invoice_records_2": {},
	})
	if err != nil {
		t.Fatalf("normalizeExcelRecognitionResult returned error: %v", err)
	}
	if got, want := result.Table.Name, "invoice_records_3"; got != want {
		t.Fatalf("table name = %q, want %q", got, want)
	}
}
