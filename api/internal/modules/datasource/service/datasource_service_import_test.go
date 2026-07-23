package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/xuri/excelize/v2"
	"github.com/zgiai/zgi/api/internal/dto"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestParseExcelFileSkipUnmatchedColumns(t *testing.T) {
	file := buildImportWorkbook(t, []string{"name", "extra"}, []string{"Ada", "ignored"})
	columns := []dto.TableColumn{
		{Name: "name", Type: "text", IsRequired: true},
	}

	svc := &dataSourceService{}
	records, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, true)
	if err != nil {
		t.Fatalf("parseExcelFile() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if got := records[0]["name"]; got != "Ada" {
		t.Fatalf("records[0][name] = %v, want Ada", got)
	}
	if _, exists := records[0]["extra"]; exists {
		t.Fatalf("records[0] contains skipped field extra: %#v", records[0])
	}
}

func TestParseExcelFileSkipUnmatchedColumnsDropsRowsWithoutMatchedFields(t *testing.T) {
	file := buildImportWorkbook(t, []string{"extra", "name"}, []string{"ignored"})
	columns := []dto.TableColumn{
		{Name: "name", Type: "text"},
	}

	svc := &dataSourceService{}
	records, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, true)
	if err != nil {
		t.Fatalf("parseExcelFile() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("len(records) = %d, want 0; records = %#v", len(records), records)
	}
}

func TestParseExcelFileRejectsUnmatchedColumnsByDefault(t *testing.T) {
	file := buildImportWorkbook(t, []string{"name", "extra"}, []string{"Ada", "ignored"})
	columns := []dto.TableColumn{
		{Name: "name", Type: "text", IsRequired: true},
	}

	svc := &dataSourceService{}
	_, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, false)
	if err == nil {
		t.Fatal("parseExcelFile() error = nil, want unmatched column error")
	}
	if !strings.Contains(err.Error(), "column 'extra' does not exist in table") {
		t.Fatalf("parseExcelFile() error = %q, want unmatched column error", err.Error())
	}
}

func TestParseExcelFileRequiresMatchedRequiredColumns(t *testing.T) {
	file := buildImportWorkbook(t, []string{"notes", "extra"}, []string{"hello", "ignored"})
	columns := []dto.TableColumn{
		{Name: "name", Type: "text", IsRequired: true},
		{Name: "notes", Type: "text"},
	}

	svc := &dataSourceService{}
	_, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, true)
	if err == nil {
		t.Fatal("parseExcelFile() error = nil, want missing required column error")
	}
	if !strings.Contains(err.Error(), "missing required columns: name") {
		t.Fatalf("parseExcelFile() error = %q, want missing required column error", err.Error())
	}
}

func TestParseExcelFileMatchesDatabaseColumnName(t *testing.T) {
	file := buildImportWorkbook(t, []string{"user_id"}, []string{"u_001"})
	columns := []dto.TableColumn{
		{Name: "user_id", Type: "text", IsRequired: true},
	}

	svc := &dataSourceService{}
	records, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, false)
	if err != nil {
		t.Fatalf("parseExcelFile() error = %v", err)
	}
	if got := records[0]["user_id"]; got != "u_001" {
		t.Fatalf("records[0][user_id] = %v, want u_001", got)
	}
}

func TestParseExcelFileMatchesSourceColumnName(t *testing.T) {
	file := buildImportWorkbook(t, []string{"用户ID", "手机号"}, []string{"u_001", "13800000000"})
	userIDSource := "用户ID"
	phoneSource := "手机号"
	columns := []dto.TableColumn{
		{Name: "user_id", SourceColumnName: &userIDSource, Type: "text", IsRequired: true},
		{Name: "phone_number", SourceColumnName: &phoneSource, Type: "text"},
	}

	svc := &dataSourceService{}
	records, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, false)
	if err != nil {
		t.Fatalf("parseExcelFile() error = %v", err)
	}
	if got := records[0]["user_id"]; got != "u_001" {
		t.Fatalf("records[0][user_id] = %v, want u_001", got)
	}
	if got := records[0]["phone_number"]; got != "13800000000" {
		t.Fatalf("records[0][phone_number] = %v, want 13800000000", got)
	}
	if _, exists := records[0]["手机号"]; exists {
		t.Fatalf("records[0] contains source header key 手机号: %#v", records[0])
	}
}

func TestParseExcelFileRejectsAmbiguousSourceColumnName(t *testing.T) {
	file := buildImportWorkbook(t, []string{"状态"}, []string{"启用"})
	statusSource := "状态"
	columns := []dto.TableColumn{
		{Name: "status", SourceColumnName: &statusSource, Type: "text"},
		{Name: "state", SourceColumnName: &statusSource, Type: "text"},
	}

	svc := &dataSourceService{}
	_, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, false)
	if err == nil {
		t.Fatal("parseExcelFile() error = nil, want ambiguous column error")
	}
	if !strings.Contains(err.Error(), "Excel 表头「状态」匹配到多个字段") {
		t.Fatalf("parseExcelFile() error = %q, want ambiguous column error", err.Error())
	}
}

func TestParseExcelFileRequiredColumnCanMatchSourceColumnName(t *testing.T) {
	file := buildImportWorkbook(t, []string{"用户ID"}, []string{"u_001"})
	userIDSource := "用户ID"
	columns := []dto.TableColumn{
		{Name: "user_id", SourceColumnName: &userIDSource, Type: "text", IsRequired: true},
	}

	svc := &dataSourceService{}
	records, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, false)
	if err != nil {
		t.Fatalf("parseExcelFile() error = %v", err)
	}
	if got := records[0]["user_id"]; got != "u_001" {
		t.Fatalf("records[0][user_id] = %v, want u_001", got)
	}
}

func TestParseExcelFileTrimsHeadersBeforeSourceColumnMatch(t *testing.T) {
	file := buildImportWorkbook(t, []string{" 用户ID "}, []string{"u_001"})
	userIDSource := "用户ID"
	columns := []dto.TableColumn{
		{Name: "user_id", SourceColumnName: &userIDSource, Type: "text", IsRequired: true},
	}

	svc := &dataSourceService{}
	records, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, false)
	if err != nil {
		t.Fatalf("parseExcelFile() error = %v", err)
	}
	if got := records[0]["user_id"]; got != "u_001" {
		t.Fatalf("records[0][user_id] = %v, want u_001", got)
	}
}

func TestParseExcelFileRejectsHeadersMatchingSameField(t *testing.T) {
	file := buildImportWorkbook(t, []string{"user_id", "用户ID"}, []string{"u_001", "u_002"})
	userIDSource := "用户ID"
	columns := []dto.TableColumn{
		{Name: "user_id", SourceColumnName: &userIDSource, Type: "text"},
	}

	svc := &dataSourceService{}
	_, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, false)
	if err == nil {
		t.Fatal("parseExcelFile() error = nil, want duplicate field match error")
	}
	if !strings.Contains(err.Error(), "同时匹配字段「user_id」") {
		t.Fatalf("parseExcelFile() error = %q, want duplicate field match error", err.Error())
	}
}

func TestParseExcelFileMatchesExplicitSourceColumnName(t *testing.T) {
	file := buildImportWorkbook(t, []string{"手机号"}, []string{"13800000000"})
	sourceColumnName := "手机号"
	columns := []dto.TableColumn{
		{Name: "phone_number", SourceColumnName: &sourceColumnName, Type: "text"},
	}

	svc := &dataSourceService{}
	records, err := svc.parseExcelFile(bytes.NewReader(file), "records.xlsx", columns, false)
	if err != nil {
		t.Fatalf("parseExcelFile() error = %v", err)
	}
	if got := records[0]["phone_number"]; got != "13800000000" {
		t.Fatalf("records[0][phone_number] = %v, want 13800000000", got)
	}
}

func TestExcelImportColumnMetadataIgnoresNewerNonSchemaJob(t *testing.T) {
	ctx := context.Background()
	const (
		organizationID = "org-1"
		dataSourceID   = "ds-1"
		tableID        = "table-1"
		accountID      = "account-1"
	)
	now := time.Now()
	phoneSource := "手机号"
	schemaSnapshot := []dto.InferredExcelColumn{
		{
			SourceColumn: phoneSource,
			Name:         "phone_number",
			Type:         "text",
		},
	}
	db, mock := newExcelImportMockDB(t)
	mock.ExpectQuery(`SELECT \* FROM "data_source_import_jobs" WHERE organization_id = \$1 AND data_source_id = \$2 AND table_id = \$3 AND source_type = \$4 AND status = \$5 ORDER BY created_at DESC,"data_source_import_jobs"\."id" LIMIT \$6`).
		WithArgs(organizationID, dataSourceID, tableID, "schema", "completed", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"organization_id",
			"data_source_id",
			"table_id",
			"source_type",
			"source_file_name",
			"status",
			"schema_snapshot",
			"created_by",
			"updated_by",
			"created_at",
			"updated_at",
		}).AddRow(
			"schema-job",
			organizationID,
			dataSourceID,
			tableID,
			"schema",
			"users",
			string(dto.ExcelImportStatusCompleted),
			[]byte(mustJSON(schemaSnapshot)),
			accountID,
			accountID,
			now.Add(-time.Minute),
			now.Add(-time.Minute),
		))

	svc := &dataSourceService{db: db}
	metadata, err := svc.getExcelImportColumnMetadata(ctx, organizationID, dataSourceID, tableID, now)
	if err != nil {
		t.Fatalf("getExcelImportColumnMetadata() error = %v", err)
	}
	phoneMetadata, ok := metadata["phone_number"]
	if !ok {
		t.Fatalf("metadata[phone_number] missing from %#v", metadata)
	}
	if got := phoneMetadata.SourceColumnName; got != phoneSource {
		t.Fatalf("metadata[phone_number].SourceColumnName = %q, want %q", got, phoneSource)
	}

	file := buildImportWorkbook(t, []string{phoneSource}, []string{"13800000000"})
	columns := []dto.TableColumn{
		{Name: "phone_number", SourceColumnName: &phoneMetadata.SourceColumnName, Type: "text"},
	}
	records, err := svc.parseExcelFile(bytes.NewReader(file), "users.xlsx", columns, false)
	if err != nil {
		t.Fatalf("parseExcelFile() error = %v", err)
	}
	if got := records[0]["phone_number"]; got != "13800000000" {
		t.Fatalf("records[0][phone_number] = %v, want 13800000000", got)
	}
	if _, exists := records[0][phoneSource]; exists {
		t.Fatalf("records[0] contains source header key %q: %#v", phoneSource, records[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

func TestExcelImportColumnMetadataKeepsSourceColumnName(t *testing.T) {
	ctx := context.Background()
	const (
		organizationID = "org-1"
		dataSourceID   = "ds-1"
		tableID        = "table-1"
		accountID      = "account-1"
	)
	now := time.Now()
	phoneSource := "手机号"
	schemaSnapshot := []dto.InferredExcelColumn{
		{
			SourceColumn: phoneSource,
			Name:         "phone_number",
			Type:         "text",
		},
	}
	db, mock := newExcelImportMockDB(t)
	mock.ExpectQuery(`SELECT \* FROM "data_source_import_jobs" WHERE organization_id = \$1 AND data_source_id = \$2 AND table_id = \$3 AND source_type = \$4 AND status = \$5 ORDER BY created_at DESC,"data_source_import_jobs"\."id" LIMIT \$6`).
		WithArgs(organizationID, dataSourceID, tableID, "schema", "completed", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"organization_id",
			"data_source_id",
			"table_id",
			"source_type",
			"source_file_name",
			"status",
			"schema_snapshot",
			"created_by",
			"updated_by",
			"created_at",
			"updated_at",
		}).AddRow(
			"source-schema-job",
			organizationID,
			dataSourceID,
			tableID,
			"schema",
			"users",
			string(dto.ExcelImportStatusCompleted),
			[]byte(mustJSON(schemaSnapshot)),
			accountID,
			accountID,
			now.Add(-time.Minute),
			now.Add(-time.Minute),
		))

	svc := &dataSourceService{db: db}
	metadata, err := svc.getExcelImportColumnMetadata(ctx, organizationID, dataSourceID, tableID, now)
	if err != nil {
		t.Fatalf("getExcelImportColumnMetadata() error = %v", err)
	}
	phoneMetadata, ok := metadata["phone_number"]
	if !ok {
		t.Fatalf("metadata[phone_number] missing from %#v", metadata)
	}
	if got := phoneMetadata.SourceColumnName; got != phoneSource {
		t.Fatalf("metadata[phone_number].SourceColumnName = %q, want %q", got, phoneSource)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

func TestExcelImportColumnMetadataFallsBackToLegacyImportJob(t *testing.T) {
	ctx := context.Background()
	const (
		organizationID = "org-1"
		dataSourceID   = "ds-1"
		tableID        = "table-1"
		accountID      = "account-1"
	)
	tableCreatedAt := time.Now()
	phoneSource := "手机号"
	schemaSnapshot := []dto.InferredExcelColumn{
		{
			SourceColumn: phoneSource,
			Name:         "phone_number",
			Type:         "text",
		},
	}
	db, mock := newExcelImportMockDB(t)
	mock.ExpectQuery(`SELECT \* FROM "data_source_import_jobs" WHERE organization_id = \$1 AND data_source_id = \$2 AND table_id = \$3 AND source_type = \$4 AND status = \$5 ORDER BY created_at DESC,"data_source_import_jobs"\."id" LIMIT \$6`).
		WithArgs(organizationID, dataSourceID, tableID, "schema", "completed", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"organization_id",
			"data_source_id",
			"table_id",
			"source_type",
			"source_file_name",
			"status",
			"schema_snapshot",
			"created_by",
			"updated_by",
			"created_at",
			"updated_at",
		}))
	mock.ExpectQuery(`SELECT \* FROM "data_source_import_jobs" WHERE organization_id = \$1 AND data_source_id = \$2 AND table_id = \$3 AND source_type IN \(\$4,\$5\) AND status IN \(\$6,\$7\) AND created_at <= \$8 ORDER BY created_at DESC,"data_source_import_jobs"\."id" LIMIT \$9`).
		WithArgs(organizationID, dataSourceID, tableID, "excel", "csv", "completed", "partial_failed", tableCreatedAt, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"organization_id",
			"data_source_id",
			"table_id",
			"source_type",
			"source_file_name",
			"status",
			"schema_snapshot",
			"created_by",
			"updated_by",
			"created_at",
			"updated_at",
		}).AddRow(
			"legacy-import-job",
			organizationID,
			dataSourceID,
			tableID,
			"excel",
			"users.xlsx",
			string(dto.ExcelImportStatusCompleted),
			[]byte(mustJSON(schemaSnapshot)),
			accountID,
			accountID,
			tableCreatedAt.Add(-time.Minute),
			tableCreatedAt,
		))

	svc := &dataSourceService{db: db}
	metadata, err := svc.getExcelImportColumnMetadata(ctx, organizationID, dataSourceID, tableID, tableCreatedAt)
	if err != nil {
		t.Fatalf("getExcelImportColumnMetadata() error = %v", err)
	}
	phoneMetadata, ok := metadata["phone_number"]
	if !ok {
		t.Fatalf("metadata[phone_number] missing from %#v", metadata)
	}
	if got := phoneMetadata.SourceColumnName; got != phoneSource {
		t.Fatalf("metadata[phone_number].SourceColumnName = %q, want %q", got, phoneSource)
	}

	file := buildImportWorkbook(t, []string{phoneSource}, []string{"13800000000"})
	columns := []dto.TableColumn{
		{Name: "phone_number", SourceColumnName: &phoneMetadata.SourceColumnName, Type: "text"},
	}
	records, err := svc.parseExcelFile(bytes.NewReader(file), "users.xlsx", columns, false)
	if err != nil {
		t.Fatalf("parseExcelFile() error = %v", err)
	}
	if got := records[0]["phone_number"]; got != "13800000000" {
		t.Fatalf("records[0][phone_number] = %v, want 13800000000", got)
	}
	if _, exists := records[0][phoneSource]; exists {
		t.Fatalf("records[0] contains source header key %q: %#v", phoneSource, records[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

func TestWithSourceColumnNamesMatchesHeadersBySourceColumnName(t *testing.T) {
	userIDSource := "用户ID"
	phoneSource := "手机号"
	columns := []dto.TableColumn{
		{Name: "phone_number", SourceColumnName: &phoneSource, Type: "text", IsRequired: true},
		{Name: "user_id", SourceColumnName: &userIDSource, Type: "integer", IsRequired: true},
	}

	got, err := withSourceColumnNames(columns, []string{"用户ID", "手机号"})
	if err != nil {
		t.Fatalf("withSourceColumnNames() error = %v", err)
	}

	if got[0].SourceColumnName == nil || *got[0].SourceColumnName != "手机号" {
		t.Fatalf("got[0].SourceColumnName = %v, want 手机号", got[0].SourceColumnName)
	}
	if got[1].SourceColumnName == nil || *got[1].SourceColumnName != "用户ID" {
		t.Fatalf("got[1].SourceColumnName = %v, want 用户ID", got[1].SourceColumnName)
	}
}

func TestWithSourceColumnNamesRejectsUnclearHeaderMatch(t *testing.T) {
	columns := []dto.TableColumn{
		{Name: "user_id", Type: "integer"},
		{Name: "mobile_phone", Type: "text"},
	}

	_, err := withSourceColumnNames(columns, []string{"用户ID", "手机号"})

	if err == nil {
		t.Fatal("withSourceColumnNames() error = nil, want unclear header match error")
	}
	if !strings.Contains(err.Error(), "无法唯一匹配 Excel 原始表头") {
		t.Fatalf("withSourceColumnNames() error = %q, want unclear header match error", err.Error())
	}
}

func TestLLMSourceColumnNameSupportsCurrentAndLegacyFields(t *testing.T) {
	tests := []struct {
		name             string
		sourceColumnName string
		displayName      string
		want             string
	}{
		{name: "current field", sourceColumnName: "工号", want: "工号"},
		{name: "legacy field", displayName: "工号", want: "工号"},
		{name: "current field wins", sourceColumnName: "员工编号", displayName: "工号", want: "员工编号"},
		{name: "trim values", sourceColumnName: "  工号  ", displayName: " 姓名 ", want: "工号"},
		{name: "empty fields", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := llmSourceColumnName(tt.sourceColumnName, tt.displayName); got != tt.want {
				t.Fatalf("llmSourceColumnName(%q, %q) = %q, want %q", tt.sourceColumnName, tt.displayName, got, tt.want)
			}
		})
	}
}

func TestLegacyLLMDisplayNameStillUsesStrictSourceHeaderMatching(t *testing.T) {
	var llmColumns []llmTableColumn
	if err := json.Unmarshal([]byte(`[{"Name":"employee_id","DisplayName":"工号","Type":"text","IsRequired":true,"Description":"员工工号"}]`), &llmColumns); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(llmColumns) != 1 {
		t.Fatalf("len(llmColumns) = %d, want 1", len(llmColumns))
	}

	sourceColumnName := llmSourceColumnName(llmColumns[0].SourceColumnName, llmColumns[0].DisplayName)
	columns := []dto.TableColumn{
		{Name: llmColumns[0].Name, SourceColumnName: &sourceColumnName, Type: llmColumns[0].Type},
	}

	got, err := withSourceColumnNames(columns, []string{"工号"})
	if err != nil {
		t.Fatalf("withSourceColumnNames() error = %v", err)
	}
	if got[0].SourceColumnName == nil || *got[0].SourceColumnName != "工号" {
		t.Fatalf("got[0].SourceColumnName = %v, want 工号", got[0].SourceColumnName)
	}

	_, err = withSourceColumnNames(columns, []string{"员工编号"})
	if err == nil {
		t.Fatal("withSourceColumnNames() error = nil, want mismatched legacy header error")
	}
}

func TestHasExplicitSourceColumnMetadata(t *testing.T) {
	emptySource := ""
	phoneSource := "手机号"
	tests := []struct {
		name    string
		columns []dto.TableColumn
		want    bool
	}{
		{name: "old client omitted source metadata", columns: []dto.TableColumn{{Name: "phone_number", Type: "text"}}, want: false},
		{name: "explicit clear", columns: []dto.TableColumn{{Name: "phone_number", SourceColumnName: &emptySource, Type: "text"}}, want: true},
		{name: "explicit source", columns: []dto.TableColumn{{Name: "phone_number", SourceColumnName: &phoneSource, Type: "text"}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasExplicitSourceColumnMetadata(tt.columns); got != tt.want {
				t.Fatalf("hasExplicitSourceColumnMetadata() = %v, want %v", got, tt.want)
			}
		})
	}
}
func TestTableColumnSourceSchemaUsesSourceColumnName(t *testing.T) {
	desc := "用户唯一标识"
	columns := []dto.TableColumn{
		{Name: "user_id", SourceColumnName: stringPtr("用户ID"), Type: "integer", IsRequired: true, Description: &desc},
		{Name: "notes", Type: "text"},
	}

	schema := tableColumnSourceSchema(columns)
	if len(schema) != 1 {
		t.Fatalf("len(schema) = %d, want 1", len(schema))
	}
	if schema[0].SourceColumn != "用户ID" || schema[0].Name != "user_id" {
		t.Fatalf("schema[0] = %#v, want source 用户ID and name user_id", schema[0])
	}
}
func TestTableColumnSourceSchemaReturnsEmptySnapshotWhenSourcesAreCleared(t *testing.T) {
	columns := []dto.TableColumn{
		{Name: "user_id", Type: "integer"},
		{Name: "phone_number", Type: "text"},
	}

	schema := tableColumnSourceSchema(columns)

	if len(schema) != 0 {
		t.Fatalf("len(schema) = %d, want 0", len(schema))
	}
}

func TestIsSQLMetaTableMissingRecognizesInternalAndExternalMissingTable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "internal", err: errors.New("sqlmeta: table not found"), want: true},
		{name: "external delete", err: errors.New("failed to delete table: 404 Not Found"), want: true},
		{name: "external get", err: errors.New("failed to get table: 404 Not Found"), want: true},
		{name: "other", err: errors.New("failed to delete table: 500 Internal Server Error"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSQLMetaTableMissing(tt.err); got != tt.want {
				t.Fatalf("isSQLMetaTableMissing() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTableSourceHeadersReadsExcelOriginalHeaders(t *testing.T) {
	file := buildImportWorkbook(t, []string{"用户ID", "手机号"}, []string{"1001", "13800138000"})

	headers, err := tableSourceHeaders("users.xlsx", file)
	if err != nil {
		t.Fatalf("tableSourceHeaders() error = %v", err)
	}
	if len(headers) != 2 || headers[0] != "用户ID" || headers[1] != "手机号" {
		t.Fatalf("tableSourceHeaders() = %#v, want [用户ID 手机号]", headers)
	}
}

func TestTableSourceHeadersDetectsHeaderBelowTitleRow(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "Sheet1"
	if err := f.SetCellValue(sheet, "C1", "员工信息库"); err != nil {
		t.Fatalf("SetCellValue(title) error = %v", err)
	}
	headers := []string{"工号", "性别", "部门", "岗位"}
	values := []string{"10001", "男", "研发部", "工程师"}
	for index, header := range headers {
		cell, err := excelize.CoordinatesToCellName(index+1, 2)
		if err != nil {
			t.Fatalf("CoordinatesToCellName(header) error = %v", err)
		}
		if err := f.SetCellValue(sheet, cell, header); err != nil {
			t.Fatalf("SetCellValue(header) error = %v", err)
		}
		cell, err = excelize.CoordinatesToCellName(index+1, 3)
		if err != nil {
			t.Fatalf("CoordinatesToCellName(value) error = %v", err)
		}
		if err := f.SetCellValue(sheet, cell, values[index]); err != nil {
			t.Fatalf("SetCellValue(value) error = %v", err)
		}
	}
	var buffer bytes.Buffer
	if err := f.Write(&buffer); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := tableSourceHeaders("employees.xlsx", buffer.Bytes())
	if err != nil {
		t.Fatalf("tableSourceHeaders() error = %v", err)
	}
	if len(got) < len(headers) {
		t.Fatalf("tableSourceHeaders() = %#v, want at least %#v", got, headers)
	}
	for index, want := range headers {
		if got[index] != want {
			t.Fatalf("tableSourceHeaders()[%d] = %q, want %q", index, got[index], want)
		}
	}
}

func TestTableSourceHeadersReadsCSVOriginalHeaders(t *testing.T) {
	content := []byte("用户ID,手机号\n1001,13800138000\n")

	headers, err := tableSourceHeaders("users.csv", content)
	if err != nil {
		t.Fatalf("tableSourceHeaders() error = %v", err)
	}
	if len(headers) != 2 || headers[0] != "用户ID" || headers[1] != "手机号" {
		t.Fatalf("tableSourceHeaders() = %#v, want [用户ID 手机号]", headers)
	}

	userIDSource := "用户ID"
	phoneSource := "手机号"
	columns := []dto.TableColumn{
		{Name: "user_id", SourceColumnName: &userIDSource, Type: "text"},
		{Name: "phone_number", SourceColumnName: &phoneSource, Type: "text"},
	}
	got, err := withSourceColumnNames(columns, headers)
	if err != nil {
		t.Fatalf("withSourceColumnNames() error = %v", err)
	}
	if got[1].SourceColumnName == nil || *got[1].SourceColumnName != "手机号" {
		t.Fatalf("got[1].SourceColumnName = %v, want 手机号", got[1].SourceColumnName)
	}

	schema := tableColumnSourceSchema(got)
	if len(schema) != 2 {
		t.Fatalf("len(schema) = %d, want 2", len(schema))
	}
	if schema[1].Name != "phone_number" || schema[1].SourceColumn != "手机号" {
		t.Fatalf("schema[1] = %#v, want phone_number from 手机号", schema[1])
	}
}
func buildImportWorkbook(t *testing.T, headers []string, values []string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "Sheet1"
	for index, header := range headers {
		cell, err := excelize.CoordinatesToCellName(index+1, 1)
		if err != nil {
			t.Fatalf("CoordinatesToCellName() error = %v", err)
		}
		if err := f.SetCellValue(sheet, cell, header); err != nil {
			t.Fatalf("SetCellValue(header) error = %v", err)
		}
	}
	for index, value := range values {
		cell, err := excelize.CoordinatesToCellName(index+1, 2)
		if err != nil {
			t.Fatalf("CoordinatesToCellName() error = %v", err)
		}
		if err := f.SetCellValue(sheet, cell, value); err != nil {
			t.Fatalf("SetCellValue(value) error = %v", err)
		}
	}
	buffer, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("WriteToBuffer() error = %v", err)
	}
	return buffer.Bytes()
}

func newExcelImportMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	return db, mock
}
