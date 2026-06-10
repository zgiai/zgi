package excelimport

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
)

func TestAnalyzeWorkbookKeepsSourceHeaderAndNormalizesFieldName(t *testing.T) {
	wb := &ParsedWorkbook{
		SourceType: "excel",
		Sheets: []ParsedSheet{
			{
				Name:        "Sheet1",
				Rows:        [][]string{{"Customer ID", "Amount"}, {"C-1", "12.5"}},
				RowCount:    2,
				ColumnCount: 2,
				Recommended: true,
			},
		},
	}

	result, err := AnalyzeWorkbook(wb, AnalyzeOptions{})
	if err != nil {
		t.Fatalf("AnalyzeWorkbook returned error: %v", err)
	}
	if got := result.Columns[0].SourceColumn; got != "Customer ID" {
		t.Fatalf("source column = %q, want Customer ID", got)
	}
	if got := result.Columns[0].SourceColumnIndex; got != 0 {
		t.Fatalf("source column index = %d, want 0", got)
	}
	if got := result.Columns[0].Name; got != "customer_id" {
		t.Fatalf("field name = %q, want customer_id", got)
	}
	if _, ok := result.PreviewRows[0].Values["customer_id"]; !ok {
		t.Fatalf("preview row should be keyed by normalized field name")
	}
}

func TestAnalyzeWorkbookTreatsSchemaLikeSheetAsDataTable(t *testing.T) {
	wb := &ParsedWorkbook{
		SourceType: "excel",
		Sheets: []ParsedSheet{
			{
				Name: "表结构字段",
				Rows: [][]string{
					{"字段名", "字段类型", "是否必填", "字段说明", "期望识别值"},
					{"合同编号", "Text", "是", "合同或订单编号", "HT-2026-0609-ZH-001"},
					{"发票号码", "Text", "是", "中文发票或服务单编号", "CN-INV-20260609-088"},
					{"客户名称", "Text", "是", "购买方或客户公司名称", "杭州星澜智能制造有限公司"},
					{"客户联系人", "Text", "否", "客户侧联系人", "林若晨"},
					{"联系电话", "Text", "否", "客户联系电话", "138-0571-6628"},
					{"供应商名称", "Text", "是", "服务提供方名称", "上海云启数据科技有限公司"},
					{"项目名称", "Text", "是", "服务或项目名称", "产线质量巡检数据入库服务"},
					{"服务地点", "Text", "否", "服务实施地点", "杭州滨江工厂"},
					{"服务周期", "Text", "否", "服务起止时间", "2026-06-01 至 2026-06-30"},
					{"开票日期", "Date", "是", "单据开具日期", "2026-06-09"},
					{"付款方式", "Text", "否", "付款账期或方式", "月结30天"},
					{"币种", "Text", "否", "金额币种", "人民币"},
					{"不含税金额", "Numeric", "是", "不含税总金额", "32600.00"},
					{"税率", "Text", "是", "适用税率", "6%"},
					{"税额", "Numeric", "是", "税额", "1956.00"},
					{"含税总额", "Numeric", "是", "含税应付总额", "34556.00"},
					{"审批人", "Text", "否", "内部审批人姓名", "周明远"},
					{"备注", "Text", "否", "补充说明", "请在7个工作日内完成验收确认。"},
				},
				RowCount:    19,
				ColumnCount: 5,
				Recommended: true,
			},
		},
	}

	result, err := AnalyzeWorkbook(wb, AnalyzeOptions{})
	if err != nil {
		t.Fatalf("AnalyzeWorkbook returned error: %v", err)
	}
	if got := len(result.Columns); got != 5 {
		t.Fatalf("columns = %d, want 5", got)
	}
	if got := result.TotalRows; got != 18 {
		t.Fatalf("total rows = %d, want 18", got)
	}
}

func TestValidateRowsUsesSourceColumnIndexForDuplicateHeaders(t *testing.T) {
	wb := &ParsedWorkbook{
		SourceType: "excel",
		Sheets: []ParsedSheet{
			{
				Name:        "Sheet1",
				Rows:        [][]string{{"Name", "Name"}, {"left", "right"}},
				RowCount:    2,
				ColumnCount: 2,
				Recommended: true,
			},
		},
	}

	req := emptyConfirmRequest()
	req.Selection.SheetName = "Sheet1"
	req.Selection.HeaderRow = 1
	req.Selection.StartRow = 2
	req.Columns = []dto.InferredExcelColumn{
		{SourceColumn: "Name", SourceColumnIndex: 0, Name: "name", Type: "text", IsRequired: true},
		{SourceColumn: "Name", SourceColumnIndex: 1, Name: "name_2", Type: "text", IsRequired: true},
	}

	result, err := ValidateRows(wb, req)
	if err != nil {
		t.Fatalf("ValidateRows returned error: %v", err)
	}
	if got := result.Records[0]["name"]; got != "left" {
		t.Fatalf("name = %v, want left", got)
	}
	if got := result.Records[0]["name_2"]; got != "right" {
		t.Fatalf("name_2 = %v, want right", got)
	}
}

func TestValidateImportSchemaRejectsInvalidNames(t *testing.T) {
	req := emptyConfirmRequest()
	req.Table.Name = "bad-name"
	req.Columns = []dto.InferredExcelColumn{
		{SourceColumnIndex: 0, Name: "valid_name", Type: "text"},
	}
	if err := ValidateImportSchema(req); err == nil {
		t.Fatalf("ValidateImportSchema should reject invalid table name")
	}

	req.Table.Name = "valid_table"
	req.Columns = []dto.InferredExcelColumn{
		{SourceColumnIndex: 0, Name: "_field", Type: "text"},
	}
	if err := ValidateImportSchema(req); err == nil {
		t.Fatalf("ValidateImportSchema should reject field name starting with underscore")
	}

	req.Columns = []dto.InferredExcelColumn{
		{SourceColumnIndex: 0, Name: "id", Type: "text"},
	}
	if err := ValidateImportSchema(req); err == nil {
		t.Fatalf("ValidateImportSchema should reject reserved field name")
	}

	req.Columns = []dto.InferredExcelColumn{
		{SourceColumnIndex: 0, Name: "field", Type: "text"},
		{SourceColumnIndex: 1, Name: "field", Type: "text"},
	}
	if err := ValidateImportSchema(req); err == nil {
		t.Fatalf("ValidateImportSchema should reject duplicated field name")
	}
}

func emptyConfirmRequest() dto.ConfirmExcelImportRequest {
	var req dto.ConfirmExcelImportRequest
	req.Table.Name = "valid_table"
	req.Selection.SheetName = "Sheet1"
	req.Selection.HeaderRow = 1
	req.Selection.StartRow = 2
	req.Options.ErrorPolicy = "skip_invalid_rows"
	req.Options.EmptyRowPolicy = "skip"
	req.Options.BatchSize = 500
	return req
}

func TestAnalyzeWorkbookMapsChineseHeadersToDatabaseFieldNames(t *testing.T) {
	wb := &ParsedWorkbook{
		SourceType: "excel",
		Sheets: []ParsedSheet{
			{
				Name:        "BUG",
				Rows:        [][]string{{"备注", "bug描述", "bug描述"}, {"复现步骤", "图标显示不一致", "重复列"}},
				RowCount:    2,
				ColumnCount: 3,
				Recommended: true,
			},
		},
	}

	result, err := AnalyzeWorkbook(wb, AnalyzeOptions{})
	if err != nil {
		t.Fatalf("AnalyzeWorkbook returned error: %v", err)
	}

	want := []string{"remark", "bug_description", "bug_description_2"}
	for i, name := range want {
		if got := result.Columns[i].Name; got != name {
			t.Fatalf("column %d name = %q, want %q", i, got, name)
		}
		if _, ok := result.PreviewRows[0].Values[name]; !ok {
			t.Fatalf("preview row should contain key %q", name)
		}
	}
}

func TestAnalyzeWorkbookAvoidsSystemFieldNameCollisions(t *testing.T) {
	wb := &ParsedWorkbook{
		SourceType: "excel",
		Sheets: []ParsedSheet{
			{
				Name:        "Sheet1",
				Rows:        [][]string{{"ID", "uuid", "created_time", "updated_time"}, {"1", "u1", "2026-01-01", "2026-01-02"}},
				RowCount:    2,
				ColumnCount: 4,
				Recommended: true,
			},
		},
	}

	result, err := AnalyzeWorkbook(wb, AnalyzeOptions{})
	if err != nil {
		t.Fatalf("AnalyzeWorkbook returned error: %v", err)
	}

	want := []string{"id_value", "uuid_value", "created_time_value", "updated_time_value"}
	for i, name := range want {
		if got := result.Columns[i].Name; got != name {
			t.Fatalf("column %d name = %q, want %q", i, got, name)
		}
	}
}
