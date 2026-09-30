package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestConnectorChatPreferenceMigrationCreatesOnlyExplicitOwnerPreferences(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)CREATE FUNCTION public.connector_chat_defaults_valid.*CREATE TABLE public.connector_chat_preferences.*CREATE TABLE public.connector_chat_preference_mutations`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upCreateConnectorChatPreferences(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(createConnectorChatPreferencesSQL)
	for _, forbidden := range []string{"insert into", "update public.", "delete from", "connector_target_bindings", "encrypted_credentials", "api_key", "raw_key"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("migration guesses defaults, mutates history or stores secrets: %s", forbidden)
		}
	}
	for _, required := range []string{"PRIMARY KEY (organization_id, owner_account_id)", "count(*) <= 64", `'{"mode":"none"}'::jsonb`, "revision bigint NOT NULL CHECK (revision > 0)", "PRIMARY KEY (organization_id, owner_account_id, key_hash)", "fingerprint varchar(64)", "DEFAULT '{}'::jsonb"} {
		if !strings.Contains(createConnectorChatPreferencesSQL, required) {
			t.Fatalf("missing preference persistence guard: %s", required)
		}
	}
}
