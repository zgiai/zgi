package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestDigitalHumanTaskApprovalOriginMigrationContract(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)DROP CONSTRAINT IF EXISTS digital_human_runs_waiting_check.*ADD CONSTRAINT digital_human_runs_waiting_check.*'task'`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upDigitalHumanTaskApprovalOrigin(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDigitalHumanTaskApprovalOriginPostgres(t *testing.T) {
	db := openExecutionFileTestPostgres(t)
	if err := db.Exec(`CREATE TABLE public.digital_human_runs (
 id uuid PRIMARY KEY, status text NOT NULL, waiting_reason text NOT NULL DEFAULT '', waiting_receipt uuid,
 surface text NOT NULL, conversation_id uuid, message_id uuid, waiting_approval_origin varchar(32) NOT NULL DEFAULT '')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := upDigitalHumanWorkflowApprovalOrigin(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := upDigitalHumanTaskApprovalOrigin(mschema.New(db)); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`INSERT INTO digital_human_runs (id,status,waiting_reason,surface,conversation_id,message_id,waiting_approval_origin)
 VALUES (?, 'waiting', 'waiting_approval', 'digital_human_api', ?, ?, 'task')`, uuid.New(), uuid.New(), uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO digital_human_runs (id,status,waiting_reason,surface,conversation_id,message_id,waiting_approval_origin)
 VALUES (gen_random_uuid(),'waiting','waiting_approval','digital_human_task',gen_random_uuid(),gen_random_uuid(),'task')`).Error; err == nil {
		t.Fatal("a background run held by a task approval was admitted")
	}
}
