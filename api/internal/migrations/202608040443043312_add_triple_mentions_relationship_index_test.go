package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddTripleMentionsRelationshipIndexMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*").WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := up202608040443043312(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, required := range []string{
		"idx_triple_mentions_kb_relationship_active",
		"(kb_id, relationship_id)",
		"is_deleted = false",
		"relationship_id IS NOT NULL",
	} {
		if !strings.Contains(statements, required) {
			t.Fatalf("relationship index migration missing %q:\n%s", required, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
