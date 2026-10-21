package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateAgentCatalogTagsMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS public\.agent_catalog_tags.*organization_id uuid NOT NULL REFERENCES public\.organizations.*normalized_name varchar\(64\).*CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_catalog_tags_organization_name.*CREATE TABLE IF NOT EXISTS public\.agent_catalog_tag_bindings.*PRIMARY KEY \(agent_id, tag_id\).*idx_agent_catalog_tag_bindings_tag_agent`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := upCreateAgentCatalogTags(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateAgentCatalogTagsRollback(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)DROP TABLE IF EXISTS public\.agent_catalog_tag_bindings.*DROP TABLE IF EXISTS public\.agent_catalog_tags`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := downCreateAgentCatalogTags(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
