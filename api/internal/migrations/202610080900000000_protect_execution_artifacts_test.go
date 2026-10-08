package migrations

import (
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestProtectExecutionArtifactsMigrationContract(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public\.tool_files.*ADD COLUMN IF NOT EXISTS execution_identity jsonb.*ck_tool_files_execution_owner.*CREATE INDEX IF NOT EXISTS idx_tool_files_execution_owner.*CREATE OR REPLACE FUNCTION public\.preserve_tool_file_execution_owner.*CREATE TRIGGER tr_tool_file_execution_owner`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upProtectExecutionArtifacts(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProtectExecutionArtifactsPostgres(t *testing.T) {
	rawDSN := os.Getenv("ZGI_EXECUTION_FILE_TEST_POSTGRES_ADMIN_DSN")
	if rawDSN == "" {
		t.Skip("set ZGI_EXECUTION_FILE_TEST_POSTGRES_ADMIN_DSN for isolated PostgreSQL validation")
	}
	dsn, err := url.Parse(rawDSN)
	if err != nil {
		t.Fatal("invalid test admin DSN")
	}
	admin, err := gorm.Open(postgres.Open(dsn.String()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	adminSQL, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminSQL.Close() })
	// The generated identifier contains only this test's prefix and hex digits.
	// Never migrate or delete the database selected by the supplied admin DSN.
	name := "zgi_execution_file_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec(`CREATE DATABASE "` + name + `"`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Exec(`DROP DATABASE "` + name + `"`).Error; err != nil {
			t.Error(err)
		}
	})
	dsn.Path = "/" + name
	db, err := gorm.Open(postgres.Open(dsn.String()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Exec(`CREATE TABLE tool_files (id uuid PRIMARY KEY, tenant_id uuid NOT NULL, user_id uuid NOT NULL, conversation_id uuid, file_key text NOT NULL, name text NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	legacy, protected, org, user, conversation, message := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := db.Exec(`INSERT INTO tool_files VALUES (?, ?, ?, ?, 'legacy/key', 'legacy.txt')`, legacy, org, user, conversation).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := upProtectExecutionArtifacts(mschema.New(db)); err != nil {
			t.Fatal(err)
		}
	}
	var legacyCount int64
	if err := db.Raw(`SELECT count(*) FROM tool_files WHERE id = ? AND execution_identity IS NULL AND execution_message_id IS NULL AND file_key = 'legacy/key'`, legacy).Scan(&legacyCount).Error; err != nil || legacyCount != 1 {
		t.Fatalf("legacy changed: count=%d err=%v", legacyCount, err)
	}
	owner, err := json.Marshal(map[string]any{"version": 1, "workspace_id": uuid.NewString(), "application_type": "digital_human", "application_id": uuid.NewString(), "entry": "webapp", "subject_kind": "account", "conversation_id": conversation.String(), "publication_ref": "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tool_files VALUES (?, ?, ?, ?, 'protected/key', 'result.txt', ?::jsonb, ?)`, protected, org, user, conversation, string(owner), message).Error; err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE tool_files SET execution_identity = NULL, execution_message_id = NULL WHERE id = ?`,
		`UPDATE tool_files SET execution_identity = jsonb_set(execution_identity, '{publication_ref}', '"v2"') WHERE id = ?`,
		`UPDATE tool_files SET execution_message_id = '00000000-0000-4000-8000-000000000001' WHERE id = ?`,
		`UPDATE tool_files SET tenant_id = '00000000-0000-4000-8000-000000000001' WHERE id = ?`,
		`UPDATE tool_files SET user_id = '00000000-0000-4000-8000-000000000001' WHERE id = ?`,
		`UPDATE tool_files SET conversation_id = '00000000-0000-4000-8000-000000000001' WHERE id = ?`,
		`UPDATE tool_files SET file_key = 'other/key' WHERE id = ?`,
	} {
		if err := db.Exec(statement, protected).Error; err == nil {
			t.Fatalf("owner mutation was accepted: %s", statement)
		}
	}
	if err := db.Exec(`UPDATE tool_files SET name = 'renamed.txt' WHERE id = ?`, protected).Error; err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{`null`, `{}`, `{"version":1}`, `{"version":2,"conversation_id":"` + conversation.String() + `"}`} {
		if err := db.Exec(`INSERT INTO tool_files VALUES (?, ?, ?, ?, 'invalid/key', 'invalid.txt', ?::jsonb, ?)`, uuid.New(), org, user, conversation, invalid, message).Error; err == nil {
			t.Fatalf("invalid protected identity accepted: %s", invalid)
		}
	}
}
