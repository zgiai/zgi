package service

import (
	"bytes"
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/xuri/excelize/v2"
	"github.com/zgiai/zgi/api/internal/modules/datasource/model"
	"github.com/zgiai/zgi/api/internal/modules/datasource/repository"
	"github.com/zgiai/zgi/api/pkg/sql_base"
)

func TestGenerateTableTemplateExcelOnlyIncludesHeaderRow(t *testing.T) {
	const (
		organizationID = "org-1"
		dataSourceID   = "ds-1"
		tableID        = "table-1"
	)
	workspaceID := "workspace-1"
	db, mock := newExcelImportMockDB(t)
	mock.ExpectQuery(`SELECT \* FROM "data_source_import_jobs" WHERE organization_id = \$1 AND data_source_id = \$2 AND table_id = \$3 AND source_type = \$4 AND status = \$5 ORDER BY created_at DESC,"data_source_import_jobs"\."id" LIMIT \$6`).
		WithArgs(organizationID, dataSourceID, tableID, "schema", "completed", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	svc := &dataSourceService{
		repo: &templateDataSourceRepository{dataSource: &model.DataSource{
			ID:             dataSourceID,
			OrganizationID: organizationID,
			WorkspaceID:    &workspaceID,
		}},
		tableRepo: &templateTableRepository{table: &model.Table{
			ID:             tableID,
			OrganizationID: organizationID,
			DataSourceID:   dataSourceID,
			TableID:        42,
		}},
		sqlBase: &templateSQLBase{table: &sql_base.Table{
			ID: 42,
			Columns: []sql_base.Column{
				{ID: "column-name", Name: "name", DataType: "text"},
				{ID: "column-age", Name: "age", DataType: "integer"},
			},
		}},
		db: db,
	}

	content, err := svc.GenerateTableTemplateExcel(t.Context(), organizationID, dataSourceID, tableID)
	if err != nil {
		t.Fatalf("GenerateTableTemplateExcel() error = %v", err)
	}

	workbook, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("excelize.OpenReader() error = %v", err)
	}
	t.Cleanup(func() {
		_ = workbook.Close()
	})

	for cell, want := range map[string]string{"A1": "name", "B1": "age"} {
		got, err := workbook.GetCellValue("Sheet1", cell)
		if err != nil {
			t.Fatalf("GetCellValue(%q) error = %v", cell, err)
		}
		if got != want {
			t.Errorf("GetCellValue(%q) = %q, want %q", cell, got, want)
		}
	}
	for _, cell := range []string{"A2", "B2"} {
		got, err := workbook.GetCellValue("Sheet1", cell)
		if err != nil {
			t.Fatalf("GetCellValue(%q) error = %v", cell, err)
		}
		if got != "" {
			t.Errorf("GetCellValue(%q) = %q, want empty", cell, got)
		}
	}
	rows, err := workbook.GetRows("Sheet1")
	if err != nil {
		t.Fatalf("GetRows() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1 header row", len(rows))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations were not met: %v", err)
	}
}

type templateDataSourceRepository struct {
	repository.DataSourceRepository
	dataSource *model.DataSource
}

func (r *templateDataSourceRepository) FindByID(context.Context, string) (*model.DataSource, error) {
	return r.dataSource, nil
}

type templateTableRepository struct {
	repository.TableRepository
	table *model.Table
}

func (r *templateTableRepository) FindByID(context.Context, string) (*model.Table, error) {
	return r.table, nil
}

type templateSQLBase struct {
	sql_base.SQLBase
	table *sql_base.Table
}

func (s *templateSQLBase) GetTable(context.Context, int) (*sql_base.Table, error) {
	return s.table, nil
}
