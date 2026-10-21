package migrations

import (
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestBackgroundExecutionArtifactsMigrationContract(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ADD CONSTRAINT ck_tool_files_execution_owner_v2.*scheduled_task.*DROP CONSTRAINT IF EXISTS ck_tool_files_execution_owner`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upBackgroundExecutionArtifacts(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBackgroundExecutionArtifactsPostgres(t *testing.T) {
	db := openExecutionFileTestPostgres(t)
	if err := db.Exec(`CREATE TABLE tool_files (id uuid PRIMARY KEY, tenant_id uuid NOT NULL, user_id uuid NOT NULL, conversation_id uuid, file_key text NOT NULL, name text NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := upProtectExecutionArtifacts(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := upBackgroundExecutionArtifacts(mschema.New(db)); err != nil {
			t.Fatal(err)
		}
	}
	conversation := uuid.New()
	for _, tc := range []struct {
		version          int
		app, entry, kind string
		valid            bool
	}{
		{1, "agent", "console", "account", true},
		{1, "digital_human", "webapp", "account", true},
		{2, "digital_human", "task", "scheduled_task", true},
		{2, "digital_human", "channel", "channel_conversation", true},
		{1, "digital_human", "task", "scheduled_task", false},
		{2, "agent", "task", "scheduled_task", false},
		{2, "digital_human", "task", "account", false},
		{2, "digital_human", "channel", "scheduled_task", false},
		{2, "digital_human", "console", "account", false},
	} {
		owner, err := json.Marshal(map[string]any{"version": tc.version, "workspace_id": uuid.NewString(), "application_type": tc.app,
			"application_id": uuid.NewString(), "entry": tc.entry, "subject_kind": tc.kind, "conversation_id": conversation.String(), "publication_ref": "v1"})
		if err != nil {
			t.Fatal(err)
		}
		id := uuid.New()
		err = db.Exec(`INSERT INTO tool_files VALUES (?, ?, ?, ?, 'protected/key', 'result.txt', ?::jsonb, ?)`, id, uuid.New(), uuid.New(), conversation, string(owner), uuid.New()).Error
		if (err == nil) != tc.valid {
			t.Fatalf("version=%d entry=%s kind=%s: %v", tc.version, tc.entry, tc.kind, err)
		}
		if tc.valid {
			if err := db.Exec(`UPDATE tool_files SET execution_identity=NULL, execution_message_id=NULL WHERE id=?`, id).Error; err == nil {
				t.Fatal("migration weakened immutable ownership")
			}
		}
	}
	if err := db.Exec(`INSERT INTO tool_files VALUES (?, ?, ?, NULL, 'legacy/key', 'legacy.txt', NULL, NULL)`, uuid.New(), uuid.New(), uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
}
