package service

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	excelimportsvc "github.com/zgiai/zgi/api/internal/modules/datasource/service/excelimport"
)

func TestInsertExistingTableRecordsUsesBoundedBatches(t *testing.T) {
	records := makeValidatedRecords(1200)
	batchSizes := make([]int, 0, 3)

	result := insertExistingTableRecords(records, defaultExcelImportBatchSize, func(values []map[string]interface{}) (int, error) {
		batchSizes = append(batchSizes, len(values))
		return len(values), nil
	})

	if !reflect.DeepEqual(batchSizes, []int{500, 500, 200}) {
		t.Fatalf("batch sizes = %v, want [500 500 200]", batchSizes)
	}
	if result.ImportedRows != 1200 || len(result.FailedItems) != 0 {
		t.Fatalf("result = %#v, want 1200 imported and no failures", result)
	}
}

func TestInsertExistingTableRecordsFallsBackToRowsAndKeepsExcelRowIndex(t *testing.T) {
	records := makeValidatedRecords(1200)
	batchNumber := 0
	result := insertExistingTableRecords(records, defaultExcelImportBatchSize, func(values []map[string]interface{}) (int, error) {
		if len(values) > 1 {
			batchNumber++
			if batchNumber == 2 {
				return 0, errors.New("batch constraint failure")
			}
			return len(values), nil
		}
		if values[0]["sequence"] == 750 {
			return 0, errors.New("duplicate employee number")
		}
		return 1, nil
	})

	if result.ImportedRows != 1199 {
		t.Fatalf("imported rows = %d, want 1199", result.ImportedRows)
	}
	if len(result.FailedItems) != 1 {
		t.Fatalf("failed items = %#v, want one item", result.FailedItems)
	}
	failure := result.FailedItems[0]
	if failure.RowIndex != 752 || failure.ErrorCode != "database_insert_failed" || failure.ErrorMessage != "duplicate employee number" {
		t.Fatalf("failure = %#v", failure)
	}
	if status := existingTableImportStatus(result.ImportedRows, len(records), len(result.FailedItems)); status != dto.ExcelImportStatusPartialFailed {
		t.Fatalf("status = %q, want partial_failed", status)
	}
}

func TestInsertExistingTableRecordsReportsFailedWhenEveryRowFails(t *testing.T) {
	records := makeValidatedRecords(3)
	result := insertExistingTableRecords(records, defaultExcelImportBatchSize, func(values []map[string]interface{}) (int, error) {
		return 0, fmt.Errorf("insert rejected")
	})

	if result.ImportedRows != 0 || len(result.FailedItems) != 3 {
		t.Fatalf("result = %#v, want three failed rows", result)
	}
	if status := existingTableImportStatus(result.ImportedRows, len(records), len(result.FailedItems)); status != dto.ExcelImportStatusFailed {
		t.Fatalf("status = %q, want failed", status)
	}
}

func TestExistingTableImportStatusAllowsEmptyValidImport(t *testing.T) {
	if status := existingTableImportStatus(0, 0, 0); status != dto.ExcelImportStatusCompleted {
		t.Fatalf("status = %q, want completed", status)
	}
}

func TestPersistExistingTableImportAliasesOnlyAfterSuccessfulInsert(t *testing.T) {
	called := false
	if err := persistExistingTableImportAliases(0, func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("alias persistence should not run without imported rows")
	}

	wantErr := errors.New("alias storage unavailable")
	err := persistExistingTableImportAliases(1, func() error {
		called = true
		return wantErr
	})
	if !called || !errors.Is(err, wantErr) {
		t.Fatalf("called/error = %v/%v, want true/%v", called, err, wantErr)
	}
	if status := existingTableImportStatus(1, 1, 0); status != dto.ExcelImportStatusCompleted {
		t.Fatalf("alias failure must not change data status, got %q", status)
	}
}

func makeValidatedRecords(count int) []excelimportsvc.ValidatedRecord {
	records := make([]excelimportsvc.ValidatedRecord, count)
	for index := range records {
		records[index] = excelimportsvc.ValidatedRecord{
			RowIndex: index + 2,
			Values:   map[string]interface{}{"sequence": index},
		}
	}
	return records
}
