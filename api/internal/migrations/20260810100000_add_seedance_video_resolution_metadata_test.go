package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestSeedanceMetadataMigrationsUseDataFixes(t *testing.T) {
	tests := []struct {
		name        string
		run         func(*mschema.Builder) error
		description string
	}{
		{
			name:        "references",
			run:         upUpdateSeedanceVideoReferenceMetadata,
			description: "DATA FIX: update Seedance video reference metadata",
		},
		{
			name:        "resolutions",
			run:         upAddSeedanceVideoResolutionMetadata,
			description: "DATA FIX: add Seedance video resolution metadata",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, mock := openMigrationMockDB(t)
			mock.ExpectExec("(?s)UPDATE public.llm_models.*").
				WillReturnResult(sqlmock.NewResult(0, 3))

			builder := mschema.New(db)
			if err := test.run(builder); err != nil {
				t.Fatal(err)
			}
			if statements := strings.Join(builder.Statements(), "\n"); !strings.Contains(statements, test.description) {
				t.Fatalf("migration did not record its data fix: %s", statements)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
