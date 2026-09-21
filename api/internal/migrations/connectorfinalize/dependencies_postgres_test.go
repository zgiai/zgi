package connectorfinalize

import (
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRetirementDependenciesPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_CONNECTOR_FINALIZE_TEST_DSN")
	if dsn == "" {
		t.Skip("requires explicit isolated ZGI_CONNECTOR_FINALIZE_TEST_DSN")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("open isolated PostgreSQL database")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	for _, sql := range []string{
		`CREATE TABLE public.retirement_test_accounts(id uuid PRIMARY KEY)`,
		`CREATE TABLE public.connector_target_bindings (id uuid PRIMARY KEY, compatibility text DEFAULT 'native', compatibility_state jsonb DEFAULT '{}', CONSTRAINT connector_target_bindings_compatibility_check CHECK(compatibility='native'), CONSTRAINT connector_target_bindings_compatibility_object_check CHECK(jsonb_typeof(compatibility_state)='object'))`,
		`CREATE TABLE public.integration_connections(id uuid PRIMARY KEY, account_id uuid REFERENCES public.retirement_test_accounts(id), payload text)`,
	} {
		if err := tx.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	check := func(wantIssue bool) {
		t.Helper()
		objects, err := retirementDependencies(tx)
		if err != nil {
			t.Fatal(err)
		}
		r := Report{}
		validateRetirementDependencies(objects, &r)
		if (len(r.Issues) > 0) != wantIssue {
			t.Fatalf("dependency gate issues=%d wantIssue=%v", len(r.Issues), wantIssue)
		}
	}
	check(false)
	for name, sql := range map[string]string{
		"view":                `CREATE VIEW public.retirement_test_view AS SELECT id FROM public.integration_connections`,
		"foreign_key":         `CREATE TABLE public.retirement_test_child(id uuid REFERENCES public.integration_connections(id))`,
		"row_type_function":   `CREATE FUNCTION public.retirement_test_function(public.integration_connections) RETURNS text LANGUAGE sql AS 'SELECT $1.payload'`,
		"compatibility_index": `CREATE INDEX retirement_test_compatibility ON public.connector_target_bindings(compatibility)`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := tx.SavePoint("dependency_case").Error; err != nil {
				t.Fatal(err)
			}
			defer tx.RollbackTo("dependency_case")
			if err := tx.Exec(sql).Error; err != nil {
				t.Fatal(err)
			}
			check(true)
		})
	}
}
