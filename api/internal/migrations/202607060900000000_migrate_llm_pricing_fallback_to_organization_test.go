package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestMigrateLLMPricingFallbackToOrganizationUpgradesLegacyTable(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectQuery(`(?s)SELECT EXISTS .*information_schema\.tables`).
		WithArgs("llm_pricing_fallback_overrides").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT EXISTS .*information_schema\.columns`).
		WithArgs("llm_pricing_fallback_overrides", "organization_id").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`(?s).*`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)SELECT EXISTS .*information_schema\.columns`).
		WithArgs("llm_pricing_fallback_overrides", "id").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	for range 3 {
		mock.ExpectExec(`(?s).*`).WillReturnResult(sqlmock.NewResult(0, 0))
	}

	builder := mschema.New(db)
	if err := upMigrateLLMPricingFallbackToOrganization(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	for _, want := range []string{
		`ADD COLUMN "organization_id" uuid`,
		`ALTER COLUMN id SET DEFAULT public.uuid_generate_v4()::text`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_llm_pricing_fallback_org`,
		`INSERT INTO public.llm_pricing_fallback_overrides (id, organization_id`,
		`FROM public.organizations`,
		`ON CONFLICT (id) DO NOTHING`,
	} {
		if !strings.Contains(statements, want) {
			t.Fatalf("legacy pricing fallback migration missing %q:\n%s", want, statements)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "RENAME TO"} {
		if strings.Contains(statements, forbidden) {
			t.Fatalf("legacy pricing fallback migration must be non-destructive (%q):\n%s", forbidden, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateLLMPricingFallbackToOrganizationAcceptsCurrentTable(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectQuery(`(?s)SELECT EXISTS .*information_schema\.tables`).
		WithArgs("llm_pricing_fallback_overrides").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT EXISTS .*information_schema\.columns`).
		WithArgs("llm_pricing_fallback_overrides", "organization_id").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT EXISTS .*information_schema\.columns`).
		WithArgs("llm_pricing_fallback_overrides", "id").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`(?s)CREATE UNIQUE INDEX IF NOT EXISTS uq_llm_pricing_fallback_org`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upMigrateLLMPricingFallbackToOrganization(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	if strings.Contains(statements, `ADD COLUMN "organization_id"`) ||
		strings.Contains(statements, "INSERT INTO public.llm_pricing_fallback_overrides") {
		t.Fatalf("current pricing fallback schema must not be rewritten:\n%s", statements)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPricingFallbackOrganizationMigrationRemainsPendingAfterLaterMigrations(t *testing.T) {
	applied := make(map[string]struct{}, len(currentMigrationIDs()))
	for _, id := range currentMigrationIDs() {
		applied[id] = struct{}{}
	}
	delete(applied, migrationMigrateLLMPricingFallbackToOrganizationID)

	missing := missingPublicMigrationIDs(applied)
	if len(missing) != 1 || missing[0] != migrationMigrateLLMPricingFallbackToOrganizationID {
		t.Fatalf("expected missing pricing fallback organization migration, got %v", missing)
	}

	var hasLaterAppliedMigration bool
	for id := range applied {
		if id > migrationMigrateLLMPricingFallbackToOrganizationID {
			hasLaterAppliedMigration = true
			break
		}
	}
	if !hasLaterAppliedMigration {
		t.Fatal("expected later migrations to remain applied in regression setup")
	}
}
