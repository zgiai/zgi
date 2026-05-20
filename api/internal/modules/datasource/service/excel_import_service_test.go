package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/dto"
	excelimportmodel "github.com/zgiai/ginext/internal/modules/datasource/model/excelimport"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestConfirmExcelImportRejectsAlreadyTerminalOrImportingJobs(t *testing.T) {
	db := newExcelImportServiceTestDB(t)
	svc := &dataSourceService{db: db}

	for _, status := range []dto.ExcelImportStatus{
		dto.ExcelImportStatusCompleted,
		dto.ExcelImportStatusFailed,
		dto.ExcelImportStatusImporting,
	} {
		t.Run(string(status), func(t *testing.T) {
			tableID := uuid.NewString()
			job := &excelimportmodel.ImportJob{
				ID:             uuid.NewString(),
				OrganizationID: "org-1",
				DataSourceID:   "ds-1",
				TableID:        &tableID,
				SourceType:     "excel",
				SourceFileName: "sample.xlsx",
				Status:         string(status),
				CreatedBy:      "account-1",
				UpdatedBy:      "account-1",
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}
			if err := db.Create(job).Error; err != nil {
				t.Fatalf("create job: %v", err)
			}

			_, err := svc.ConfirmExcelImport(context.Background(), "org-1", "ds-1", "account-1", job.ID, validExcelImportConfirmRequest())
			if err == nil {
				t.Fatal("expected confirm to fail for non-review job")
			}

			var got excelimportmodel.ImportJob
			if err := db.First(&got, "id = ?", job.ID).Error; err != nil {
				t.Fatalf("reload job: %v", err)
			}
			if got.Status != string(status) {
				t.Fatalf("status changed to %q, want %q", got.Status, status)
			}
			if got.TableID == nil || *got.TableID != tableID {
				t.Fatalf("table_id changed to %v, want %s", got.TableID, tableID)
			}
		})
	}
}

func TestCountExcelImportFailedRowsDeduplicatesRowIndex(t *testing.T) {
	failedRows := countExcelImportFailedRows([]dto.ExcelImportFailedItem{
		{RowIndex: 2, ErrorCode: "invalid_integer"},
		{RowIndex: 2, ErrorCode: "missing_required"},
		{RowIndex: 5, ErrorCode: "invalid_boolean"},
	})
	if failedRows != 2 {
		t.Fatalf("failedRows = %d, want 2", failedRows)
	}
}

func newExcelImportServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:excel-import-service-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&excelimportmodel.ImportJob{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func validExcelImportConfirmRequest() dto.ConfirmExcelImportRequest {
	var req dto.ConfirmExcelImportRequest
	req.Selection.SheetName = "Sheet1"
	req.Selection.HeaderRow = 1
	req.Selection.StartRow = 2
	req.Table.Name = "imported_table"
	req.Columns = []dto.InferredExcelColumn{
		{SourceColumnIndex: 0, Name: "name", Type: "text"},
	}
	req.Options.ErrorPolicy = "skip_invalid_rows"
	req.Options.EmptyRowPolicy = "skip"
	req.Options.BatchSize = 100
	return req
}
