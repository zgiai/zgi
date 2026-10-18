package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestDigitalHumanWorkflowApprovalOriginMigrationContract(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public\.digital_human_runs DROP CONSTRAINT IF EXISTS digital_human_runs_waiting_check.*ADD CONSTRAINT digital_human_runs_waiting_check.*'workflow'`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upDigitalHumanWorkflowApprovalOrigin(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDigitalHumanWorkflowApprovalOriginPostgres(t *testing.T) {
	db := openExecutionFileTestPostgres(t)
	if err := db.Exec(`CREATE TABLE public.digital_human_runs (
 id uuid PRIMARY KEY, status text NOT NULL, waiting_reason text NOT NULL DEFAULT '', waiting_receipt uuid,
 surface text NOT NULL, conversation_id uuid, message_id uuid, waiting_approval_origin varchar(32) NOT NULL DEFAULT '')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := upDigitalHumanPlatformCLIApproval(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := upDigitalHumanWorkflowApprovalOrigin(mschema.New(db)); err != nil {
			t.Fatal(err)
		}
	}
	for _, surface := range []string{"digital_human_console", "digital_human_webapp", "digital_human_api"} {
		if err := db.Exec(`INSERT INTO digital_human_runs (id,status,waiting_reason,surface,conversation_id,message_id,waiting_approval_origin)
 VALUES (?, 'waiting', 'waiting_approval', ?, ?, ?, 'workflow')`, uuid.New(), surface, uuid.New(), uuid.New()).Error; err != nil {
			t.Fatalf("%s: %v", surface, err)
		}
	}
	for name, statement := range map[string]string{
		"background surface": `INSERT INTO digital_human_runs (id,status,waiting_reason,surface,conversation_id,message_id,waiting_approval_origin) VALUES (gen_random_uuid(),'waiting','waiting_approval','digital_human_task',gen_random_uuid(),gen_random_uuid(),'workflow')`,
		"missing message":    `INSERT INTO digital_human_runs (id,status,waiting_reason,surface,conversation_id,waiting_approval_origin) VALUES (gen_random_uuid(),'waiting','waiting_approval','digital_human_api',gen_random_uuid(),'workflow')`,
		"terminal marker":    `INSERT INTO digital_human_runs (id,status,surface,waiting_approval_origin) VALUES (gen_random_uuid(),'succeeded','digital_human_api','workflow')`,
		"unknown origin":     `INSERT INTO digital_human_runs (id,status,waiting_reason,surface,conversation_id,message_id,waiting_approval_origin) VALUES (gen_random_uuid(),'waiting','waiting_approval','digital_human_api',gen_random_uuid(),gen_random_uuid(),'other')`,
	} {
		if err := db.Exec(statement).Error; err == nil {
			t.Fatalf("%s was admitted", name)
		}
	}
}
