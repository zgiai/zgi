package digitalhuman

import (
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestExecutionConfigSerializerPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ZGI_MIGRATION_TEST_DSN"))
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN for execution config serializer integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	// A transaction-local table shadows any real table without mutating it.
	if err := tx.Exec("CREATE TEMP TABLE digital_human_versions (id uuid PRIMARY KEY, revision text NOT NULL, execution_runtime jsonb, created_at timestamptz, updated_at timestamptz) ON COMMIT DROP").Error; err != nil {
		t.Fatal(err)
	}
	// Exercise the actual model serializer and the repository's explicit JSON
	// update shape against PostgreSQL, not only SQLite's permissive JSON storage.
	version := Version{ID: uuid.New(), Revision: "serializer", ExecutionRuntime: &ExecutionRuntimeConfig{Enabled: true}}
	if err := tx.Clauses(clause.Returning{Columns: []clause.Column{{Name: "id"}}}).Select("id", "revision", "execution_runtime").Create(&version).Error; err != nil {
		t.Fatal(err)
	}
	var loaded Version
	if err := tx.Select("id", "revision", "execution_runtime").First(&loaded, "id = ?", version.ID).Error; err != nil || loaded.ExecutionRuntime == nil || !loaded.ExecutionRuntime.Enabled {
		t.Fatalf("read serialized opt-in: config=%+v err=%v", loaded.ExecutionRuntime, err)
	}
	for _, value := range []any{nil, datatypes.JSON(`{"enabled":true}`)} {
		if err := tx.Model(&Version{}).Where("id = ?", version.ID).Updates(map[string]any{"execution_runtime": value}).Error; err != nil {
			t.Fatal(err)
		}
		loaded = Version{}
		if err := tx.Select("id", "execution_runtime").First(&loaded, "id = ?", version.ID).Error; err != nil {
			t.Fatal(err)
		}
		if (loaded.ExecutionRuntime != nil) != (value != nil) {
			t.Fatalf("updated opt-in=%+v, JSON value=%v", loaded.ExecutionRuntime, value)
		}
	}
}
