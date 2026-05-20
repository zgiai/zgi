package schema

import (
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestCompileCreateUsesLaravelStyleBlueprint(t *testing.T) {
	statements, err := CompileCreate("audit_events", func(table *Blueprint) {
		table.ID()
		table.UUID("organization_id").NotNull()
		table.String("event_type", 64).NotNull()
		table.JSONB("payload").DefaultSQL("'{}'::jsonb").NotNull()
		table.TimestampsTz()
		table.Index("idx_audit_events_org_created", "organization_id", "created_at")
	})
	if err != nil {
		t.Fatal(err)
	}

	got := strings.Join(statements, "\n")
	for _, want := range []string{
		`CREATE TABLE "public"."audit_events"`,
		`"id" uuid DEFAULT public.uuid_generate_v4() NOT NULL PRIMARY KEY`,
		`"event_type" varchar(64) NOT NULL`,
		`CREATE INDEX "idx_audit_events_org_created" ON "public"."audit_events" ("organization_id", "created_at")`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("compiled SQL missing %q:\n%s", want, got)
		}
	}
}

func TestCompileTableAddsColumnsIndexesAndForeignKeys(t *testing.T) {
	statements, err := CompileTable("audit_events", func(table *Blueprint) {
		table.UUID("account_id").Nullable()
		table.Index("idx_audit_events_account", "account_id")
		table.Foreign("fk_audit_events_account", []string{"account_id"}, "accounts", []string{"id"}).NullOnDelete()
	})
	if err != nil {
		t.Fatal(err)
	}

	got := strings.Join(statements, "\n")
	for _, want := range []string{
		`ALTER TABLE "public"."audit_events" ADD COLUMN "account_id" uuid`,
		`CREATE INDEX "idx_audit_events_account" ON "public"."audit_events" ("account_id")`,
		`ALTER TABLE "public"."audit_events" ADD CONSTRAINT "fk_audit_events_account" FOREIGN KEY ("account_id") REFERENCES "public"."accounts" ("id") ON DELETE SET NULL`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("compiled SQL missing %q:\n%s", want, got)
		}
	}
}

func TestBuilderRejectsUnsafeIdentifierShape(t *testing.T) {
	_, err := CompileTable("AuditEvents", func(table *Blueprint) {
		table.String("name")
	})
	if err == nil {
		t.Fatal("expected invalid identifier error")
	}
}

func TestHasColumnQueriesInformationSchema(t *testing.T) {
	db, mock := openMockDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS")).
		WithArgs("audit_events", "request_id").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	exists, err := New(db).HasColumn("audit_events", "request_id")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected column to exist")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWhenTableDoesntHaveColumnRunsCallbackOnlyWhenMissing(t *testing.T) {
	db, mock := openMockDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS")).
		WithArgs("audit_events", "request_id").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	called := false
	err := New(db).WhenTableDoesntHaveColumn("audit_events", "request_id", func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected callback to run")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDestructiveStatementsRequireExplicitOptIn(t *testing.T) {
	db, mock := openMockDB(t)

	err := New(db).DropIfExists("audit_events")
	if err == nil {
		t.Fatal("expected destructive drop table to be rejected")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAllowDestructiveExecutesDropTable(t *testing.T) {
	db, mock := openMockDB(t)
	mock.ExpectExec(regexp.QuoteMeta(`DROP TABLE IF EXISTS "public"."audit_events"`)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := New(db).AllowDestructive().DropIfExists("audit_events"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDropColumnRequiresExplicitOptIn(t *testing.T) {
	db, mock := openMockDB(t)

	err := New(db).Table("audit_events", func(table *Blueprint) {
		table.DropColumn("request_id")
	})
	if err == nil {
		t.Fatal("expected destructive drop column to be rejected")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRawUpdateRequiresExplicitOptIn(t *testing.T) {
	db, mock := openMockDB(t)

	err := New(db).Raw(`UPDATE "public"."audit_events" SET "event_type" = 'x'`)
	if err == nil {
		t.Fatal("expected destructive update to be rejected")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func openMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:       sqlDB,
		DriverName: "postgres",
	}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	mock.MatchExpectationsInOrder(false)
	return db, mock
}
