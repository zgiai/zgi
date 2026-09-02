package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddSetupDefaultScopeMigrationAddsNullableUUIDs(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`ALTER TABLE "public"\."zgi_setups" ADD COLUMN "organization_id" uuid`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE "public"\."zgi_setups" ADD COLUMN "workspace_id" uuid`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upAddSetupDefaultScope(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	for _, want := range []string{
		`ALTER TABLE "public"."zgi_setups" ADD COLUMN "organization_id" uuid`,
		`ALTER TABLE "public"."zgi_setups" ADD COLUMN "workspace_id" uuid`,
	} {
		if !strings.Contains(statements, want) {
			t.Fatalf("setup default scope migration missing %q:\n%s", want, statements)
		}
	}
	for _, forbidden := range []string{"NOT NULL", "DEFAULT", "DROP COLUMN"} {
		if strings.Contains(statements, forbidden) {
			t.Fatalf("setup default scope migration must add nullable, data-preserving columns (%q):\n%s", forbidden, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
