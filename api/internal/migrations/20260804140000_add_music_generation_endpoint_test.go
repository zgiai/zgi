package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddMusicGenerationEndpointMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.llm_models.*ADD COLUMN IF NOT EXISTS music_generation boolean.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upAddMusicGenerationEndpoint(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	if !strings.Contains(statements, "ADD COLUMN IF NOT EXISTS music_generation boolean NOT NULL DEFAULT false") {
		t.Fatalf("music generation migration is incomplete:\n%s", statements)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
