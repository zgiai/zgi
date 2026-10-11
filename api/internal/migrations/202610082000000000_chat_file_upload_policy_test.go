package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestChatFileUploadPolicyMigrationIsAdditive(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("ALTER TABLE public.digital_human_versions ADD COLUMN IF NOT EXISTS file_upload_policy jsonb").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upChatFileUploadPolicy(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
