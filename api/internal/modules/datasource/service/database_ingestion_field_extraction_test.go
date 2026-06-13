package service

import (
	"context"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/prompt"
)

func TestNormalizeFileConversionOutputMapsByColumnIDAndKeepsEvidence(t *testing.T) {
	columns := dto.GetTableColumnsResponse{Columns: []dto.TableColumn{
		{ID: "col_invoice", Name: "invoice_no", Type: "text"},
		{ID: "col_issue_date", Name: "issue_date", Type: "timestamp"},
	}}
	raw := `{
		"records": [{
			"fields": [
				{"column_id":"col_issue_date","value":"2026-06-09","evidence":"issue date 2026-06-09","confidence":0.91},
				{"column_id":"col_invoice","column_name":"wrong_name","value":"CN-INV-20260609-088","evidence":"invoice no CN-INV-20260609-088","confidence":0.98}
			]
		}]
	}`

	records, extraction, err := normalizeFileConversionOutput(raw, columns)
	if err != nil {
		t.Fatalf("normalizeFileConversionOutput returned error: %v", err)
	}
	if got := records[0]["invoice_no"]; got != "CN-INV-20260609-088" {
		t.Fatalf("invoice_no = %v, want CN-INV-20260609-088", got)
	}
	if got := records[0]["issue_date"]; got != "2026-06-09 00:00:00" {
		t.Fatalf("issue_date = %v, want 2026-06-09 00:00:00", got)
	}
	if extraction == nil || len(extraction.Records) != 1 || len(extraction.Records[0].Fields) != 2 {
		t.Fatalf("field extraction = %#v, want one record with two fields", extraction)
	}
	firstField := extraction.Records[0].Fields[0]
	if firstField.ColumnID != "col_issue_date" || firstField.ColumnName != "issue_date" {
		t.Fatalf("first field = %#v, want issue_date mapped by column_id", firstField)
	}
	if firstField.Evidence != "issue date 2026-06-09" {
		t.Fatalf("evidence = %q", firstField.Evidence)
	}
	if firstField.Confidence == nil || *firstField.Confidence != 0.91 {
		t.Fatalf("confidence = %#v, want 0.91", firstField.Confidence)
	}
	if firstField.RawValue != "2026-06-09" ||
		firstField.NormalizedValue != "2026-06-09 00:00:00" ||
		firstField.NormalizationStatus != fileIngestNormalizationNormalized {
		t.Fatalf("normalization metadata = %#v", firstField)
	}
}

func TestNormalizeFileConversionOutputNormalizesFieldValuesByColumnType(t *testing.T) {
	columns := dto.GetTableColumnsResponse{Columns: []dto.TableColumn{
		{ID: "col_area", Name: "area", Type: "numeric"},
		{ID: "col_rent", Name: "rent", Type: "numeric"},
		{ID: "col_enabled", Name: "enabled", Type: "boolean"},
		{ID: "col_due", Name: "due_date", Type: "timestamp"},
		{ID: "col_start", Name: "start_date", Type: "timestamp"},
		{ID: "col_invalid", Name: "invalid_amount", Type: "numeric"},
	}}
	raw := `{
		"records": [{
			"fields": [
				{"column_id":"col_area","value":"1.6 元/平米/天"},
				{"column_id":"col_rent","value":"13,860.26"},
				{"column_id":"col_enabled","value":"是"},
				{"column_id":"col_due","value":"45160"},
				{"column_id":"col_start","value":"2026年1月9日"},
				{"column_id":"col_invalid","value":"13860.26 至 166323.2"}
			]
		}]
	}`

	records, extraction, err := normalizeFileConversionOutput(raw, columns)
	if err != nil {
		t.Fatalf("normalizeFileConversionOutput returned error: %v", err)
	}
	if got := records[0]["area"]; got != 1.6 {
		t.Fatalf("area = %#v, want 1.6", got)
	}
	if got := records[0]["rent"]; got != 13860.26 {
		t.Fatalf("rent = %#v, want 13860.26", got)
	}
	if got := records[0]["enabled"]; got != true {
		t.Fatalf("enabled = %#v, want true", got)
	}
	if got := records[0]["due_date"]; got != nil {
		t.Fatalf("due_date = %#v, want nil for numeric date", got)
	}
	if got := records[0]["start_date"]; got != "2026-01-09 00:00:00" {
		t.Fatalf("start_date = %#v, want 2026-01-09 00:00:00", got)
	}
	if got := records[0]["invalid_amount"]; got != nil {
		t.Fatalf("invalid_amount = %#v, want nil for multiple numbers", got)
	}
	if extraction == nil || len(extraction.Records) != 1 || len(extraction.Records[0].Fields) != len(columns.Columns) {
		t.Fatalf("field extraction = %#v, want one complete record", extraction)
	}
	fields := extraction.Records[0].Fields
	if fields[0].NormalizationStatus != fileIngestNormalizationNormalized ||
		fields[0].RawValue != "1.6 元/平米/天" ||
		fields[0].NormalizedValue != 1.6 {
		t.Fatalf("area normalization = %#v", fields[0])
	}
	if fields[3].NormalizationStatus != fileIngestNormalizationInvalid ||
		!strings.Contains(fields[3].NormalizationReason, "numeric date") {
		t.Fatalf("due_date normalization = %#v", fields[3])
	}
	if fields[4].NormalizationStatus != fileIngestNormalizationNormalized ||
		fields[4].NormalizedValue != "2026-01-09 00:00:00" {
		t.Fatalf("start_date normalization = %#v", fields[4])
	}
	if fields[5].NormalizationStatus != fileIngestNormalizationInvalid ||
		!strings.Contains(fields[5].NormalizationReason, "multiple numeric") {
		t.Fatalf("invalid_amount normalization = %#v", fields[5])
	}
}

func TestExtractDatabaseIngestionTextToRecordsRejectsEmptyContentAtRecognitionStage(t *testing.T) {
	svc := &dataSourceService{}
	tableCtx := databaseIngestionTableContext{
		LLMOrganizationID: "org-1",
		Columns: dto.GetTableColumnsResponse{
			Columns: []dto.TableColumn{
				{ID: "col_invoice", Name: "invoice_no", Type: "text"},
			},
		},
	}

	result := svc.extractDatabaseIngestionTextToRecords(context.Background(), tableCtx, "acct-1", dto.ExtractTextToTableRecordsRequest{
		FileID:      "file-1",
		TableID:     "table-1",
		Content:     "   ",
		ContentHash: "content-hash-from-parse",
	})

	if result.Stage != fileIngestStageRecognition {
		t.Fatalf("stage = %q, want %q", result.Stage, fileIngestStageRecognition)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "parsed content is empty") {
		t.Fatalf("error = %#v, want parsed content is empty", result.Error)
	}
	if result.ContentHash != "content-hash-from-parse" {
		t.Fatalf("content hash = %q, want content-hash-from-parse", result.ContentHash)
	}
	if len(result.Columns) != 1 || result.Columns[0].Name != "invoice_no" {
		t.Fatalf("columns = %#v, want invoice_no column", result.Columns)
	}
}

func TestNormalizeFileConversionOutputRejectsUnknownColumnID(t *testing.T) {
	columns := dto.GetTableColumnsResponse{Columns: []dto.TableColumn{
		{ID: "col_invoice", Name: "invoice_no", Type: "text"},
	}}
	raw := `{"records":[{"fields":[{"column_id":"col_unknown","value":"x"}]}]}`

	_, _, err := normalizeFileConversionOutput(raw, columns)
	if err == nil || !strings.Contains(err.Error(), "unknown column_id") {
		t.Fatalf("error = %v, want unknown column_id", err)
	}
}

func TestNormalizeFileConversionOutputRejectsEmptyColumnID(t *testing.T) {
	columns := dto.GetTableColumnsResponse{Columns: []dto.TableColumn{
		{ID: "col_invoice", Name: "invoice_no", Type: "text"},
	}}
	raw := `{"records":[{"fields":[{"column_name":"invoice_no","value":"CN-INV-1"}]}]}`

	_, _, err := normalizeFileConversionOutput(raw, columns)
	if err == nil || !strings.Contains(err.Error(), "empty column_id") {
		t.Fatalf("error = %v, want empty column_id", err)
	}
}

func TestNormalizeFileConversionOutputAcceptsLegacyArrayRecords(t *testing.T) {
	columns := dto.GetTableColumnsResponse{Columns: []dto.TableColumn{
		{ID: "col_invoice", Name: "invoice_no", Type: "text"},
		{ID: "col_amount", Name: "total_amount", Type: "numeric"},
	}}
	raw := `[{"invoice_no":"CN-INV-1"}]`

	records, extraction, err := normalizeFileConversionOutput(raw, columns)
	if err != nil {
		t.Fatalf("normalizeFileConversionOutput returned error: %v", err)
	}
	if got := records[0]["invoice_no"]; got != "CN-INV-1" {
		t.Fatalf("invoice_no = %v, want CN-INV-1", got)
	}
	if got := records[0]["total_amount"]; got != nil {
		t.Fatalf("total_amount = %v, want nil", got)
	}
	if extraction == nil || len(extraction.Records) != 1 || len(extraction.Records[0].Fields) != 2 {
		t.Fatalf("field extraction = %#v, want compatible field details", extraction)
	}
	if got := extraction.Records[0].Fields[0].ColumnID; got != "col_invoice" {
		t.Fatalf("field column_id = %q, want col_invoice", got)
	}
}

func TestDatasourceFileConversionPromptUsesColumnIDAndEvidenceContract(t *testing.T) {
	tmpl, err := prompt.GetTemplate(prompt.DatasourceFileConversion)
	if err != nil {
		t.Fatalf("GetTemplate returned error: %v", err)
	}

	raw := tmpl.RawContent()
	for _, want := range []string{
		"Treat column_id as the only stable field identity",
		"Return values by column_id",
		"Do not put raw tabs, raw newlines, or other control characters inside JSON strings",
		"Every field object must use a column_id from the target schema",
		"Evidence must be a short source snippet copied from the parsed content",
		`"evidence":"short source snippet"`,
		`{"records":[]}`,
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("prompt template missing %q", want)
		}
	}
}
