package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddOrganizationLLMRoutingStrategyMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectQuery("(?s).*information_schema.columns.*table_name = \\$1.*column_name = \\$2.*").
		WithArgs("organizations", "llm_routing_strategy").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("(?is).*ALTER TABLE.*organizations.*llm_routing_strategy.*varchar\\(16\\).*DEFAULT 'stability'.*NOT NULL.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upAddOrganizationLLMRoutingStrategy(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	for _, fragment := range []string{"llm_routing_strategy", "varchar(16)", "DEFAULT 'stability'", "NOT NULL"} {
		if !strings.Contains(statements, fragment) {
			t.Fatalf("organization routing strategy migration missing %q:\n%s", fragment, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
