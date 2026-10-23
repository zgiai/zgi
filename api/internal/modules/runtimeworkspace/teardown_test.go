package runtimeworkspace

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestTeardownAttachedWorkspacePurgesSubjectsAndDefersFileObjects(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	runtimeID, workspaceID, organizationID, subjectID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	fileID, relationshipID, conversationID := uuid.NewString(), uuid.New(), uuid.New()

	mock.ExpectQuery(`(?s)SELECT \* FROM "runtime_workspaces".*workspace_id = \$1.*FOR UPDATE`).
		WithArgs(workspaceID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"runtime_id", "workspace_id", "organization_id", "status", "mode"}).
			AddRow(runtimeID, workspaceID, organizationID, StatusDisabled, ModeAttached))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "id" FROM "user_subjects" WHERE issuer_type = $1 AND issuer_id = $2`)).
		WithArgs(SubjectIssuerType, runtimeID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(subjectID))
	mock.ExpectQuery(`(?s)SELECT files\.id, files\.key FROM upload_files AS files.*runtime_files\.runtime_workspace_id = \$1`).
		WithArgs(runtimeID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "key"}).AddRow(fileID, "upload_files/runtime/report.pdf"))
	mock.ExpectExec(`(?s)INSERT INTO file_object_deletions`).
		WithArgs(sqlmock.AnyArg(), "upload_files/runtime/report.pdf", now.Add(RuntimeFileObjectRetention), now, now).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`DELETE FROM upload_files WHERE id IN \(\$1\)`).WithArgs(fileID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "id" FROM "user_memory_relationships" WHERE subject_id IN ($1)`)).
		WithArgs(subjectID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(relationshipID))
	for _, table := range []string{"user_memory_extraction_jobs", "user_memory_undo_records", "user_memory_events", "user_memory_items"} {
		mock.ExpectExec(`DELETE FROM ` + table + ` WHERE relationship_id IN \(\$1\)`).WithArgs(relationshipID).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec(`DELETE FROM user_memory_relationships WHERE id IN \(\$1\)`).WithArgs(relationshipID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT id::text AS id, file_key FROM tool_files.*user_id IN \(\$1\).*account_id IN \(\$2\)`).
		WithArgs(subjectID, subjectID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "file_key"}).AddRow(uuid.NewString(), "tools/runtime/chart.png"))
	mock.ExpectExec(`(?s)INSERT INTO file_object_deletions`).
		WithArgs(sqlmock.AnyArg(), "tools/runtime/chart.png", now.Add(RuntimeFileObjectRetention), now, now).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`DELETE FROM tool_files WHERE id IN \(\$1\)`).WillReturnResult(sqlmock.NewResult(0, 1))
	for _, table := range []string{"agent_memory_values", "agent_memory_undo_records", "agent_memory_events", "agent_memory_extraction_jobs", "agent_memory_subject_states"} {
		mock.ExpectExec(`DELETE FROM ` + table + ` WHERE user_scope = 'end_user' AND user_id IN \(\$1\)`).WithArgs(subjectID).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectQuery(`(?s)SELECT files\.id::text AS id, files\.key FROM upload_files AS files.*agent_execution_uploads.*subject_id IN \(\$1\)`).
		WithArgs(subjectID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "key"}))
	mock.ExpectExec(`DELETE FROM agent_execution_uploads WHERE subject_id IN \(\$1\)`).WithArgs(subjectID).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "id" FROM "chat_runtime_conversations" WHERE account_id IN ($1)`)).
		WithArgs(subjectID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(conversationID))
	mock.ExpectExec(`DELETE FROM chat_runtime_messages WHERE conversation_id IN \(\$1\)`).WithArgs(conversationID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM chat_runtime_conversations WHERE id IN \(\$1\)`).WithArgs(conversationID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM user_subjects WHERE id IN \(\$1\)`).WithArgs(subjectID).WillReturnResult(sqlmock.NewResult(0, 1))
	for _, table := range []string{"runtime_workspace_end_user_credit_entries", "runtime_workspace_end_user_credit_lots", "runtime_workspace_end_user_credit_bills"} {
		mock.ExpectExec(`DELETE FROM ` + table + ` WHERE runtime_workspace_id = \$1`).WithArgs(runtimeID).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec(`DELETE FROM runtime_workspace_end_user_events WHERE runtime_workspace_id = \$1`).WithArgs(runtimeID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM runtime_workspace_end_users WHERE runtime_workspace_id = \$1`).WithArgs(runtimeID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM runtime_workspaces WHERE runtime_id = \$1`).WithArgs(runtimeID).WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := TeardownAttachedWorkspace(t.Context(), db, workspaceID, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.SubjectCount != 1 || result.ConversationCount != 1 || result.FileCount != 1 {
		t.Fatalf("teardown result = %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
