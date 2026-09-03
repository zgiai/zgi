package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestConnectorPluginCatalogStateMigrationExecutesAndGuardsRollback(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE public.connector_plugin_versions.*CREATE TABLE public.connector_definition_versions.*INSERT INTO public.connector_plugin_versions.*INSERT INTO public.connector_definition_versions").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260902100000(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}

	mock.ExpectExec("(?s)cannot remove Connector plugin catalog state while accepted versions exist.*DROP TABLE IF EXISTS public.connector_plugin_versions").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down20260902100000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatalf("down migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConnectorPluginCatalogStateMigrationKeepsAcceptedVersionsAndOneActiveContract(t *testing.T) {
	sql := strings.Join(strings.Fields(createConnectorPluginCatalogStateSQL), " ")
	for _, want := range []string{
		"CREATE TABLE public.connector_plugin_versions",
		"PRIMARY KEY (plugin_id, plugin_version, plugin_digest)",
		"status IN ('active', 'superseded', 'disabled', 'unavailable')",
		"WHERE status = 'active'",
		"CREATE TABLE public.connector_definition_versions",
		"FOREIGN KEY (plugin_id, plugin_version, plugin_digest)",
		"PRIMARY KEY (connector_id, plugin_version, plugin_digest)",
		"CHECK (jsonb_typeof(definition) = 'object')",
		"idx_connector_definition_versions_one_active",
		"INSERT INTO public.connector_plugin_versions",
		"FROM public.connector_definitions",
		"jsonb_build_object( 'id', connector_id",
		"'operations', COALESCE(capability_summary->'operations', '[]'::jsonb)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration SQL is missing %q", want)
		}
	}
}
