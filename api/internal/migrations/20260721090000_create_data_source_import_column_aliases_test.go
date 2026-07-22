package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateDataSourceImportColumnAliasesMigrationIsRegistered(t *testing.T) {
	var found bool
	for _, migration := range registeredMigrations() {
		if migration.ID == migrationCreateDataSourceImportColumnAliasesID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("migration %s is not registered", migrationCreateDataSourceImportColumnAliasesID)
	}
}

func TestCreateDataSourceImportColumnAliasesMigrationDefinesScopedAliasStorage(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*CREATE TABLE IF NOT EXISTS public.data_source_import_column_aliases.*").
		WillReturnResult(sqlmock.NewResult(0, 0))
	builder := mschema.New(db)
	if err := upCreateDataSourceImportColumnAliases(builder); err != nil {
		t.Fatal(err)
	}
	source := strings.Join(builder.Statements(), "\n")
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS public.data_source_import_column_aliases",
		"UNIQUE INDEX IF NOT EXISTS idx_data_source_import_column_aliases_unique",
		"REFERENCES public.data_source_tables(id) ON DELETE CASCADE",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("migration source does not contain %q", want)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
