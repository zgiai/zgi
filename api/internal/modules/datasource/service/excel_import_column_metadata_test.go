package service

import (
	"encoding/json"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
)

func TestExcelImportColumnMetadataFromSnapshot(t *testing.T) {
	t.Run("initial import snapshot", func(t *testing.T) {
		snapshot, err := json.Marshal([]dto.InferredExcelColumn{{
			Name:         "employee_id",
			DisplayName:  "员工编号",
			SourceColumn: "工号",
		}})
		if err != nil {
			t.Fatal(err)
		}

		metadata := excelImportColumnMetadataFromSnapshot(snapshot)
		if metadata["employee_id"].SourceColumnName != "工号" {
			t.Fatalf("expected original header, got %#v", metadata)
		}
	})

	t.Run("existing table import snapshot", func(t *testing.T) {
		source := "工号"
		snapshot, err := json.Marshal([]dto.TableColumn{{
			Name:             "employee_id",
			SourceColumnName: &source,
		}})
		if err != nil {
			t.Fatal(err)
		}

		metadata := excelImportColumnMetadataFromSnapshot(snapshot)
		if metadata["employee_id"].SourceColumnName != "工号" {
			t.Fatalf("expected preserved original header, got %#v", metadata)
		}
	})
}
