package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestDigitalHumanPlatformCLIApprovalMigrationContract(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public\.digital_human_runs.*ADD COLUMN IF NOT EXISTS waiting_approval_origin.*DROP CONSTRAINT IF EXISTS digital_human_runs_waiting_check.*ADD CONSTRAINT digital_human_runs_waiting_check`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upDigitalHumanPlatformCLIApproval(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDigitalHumanPlatformCLIApprovalPostgres(t *testing.T) {
	db := openExecutionFileTestPostgres(t)
	if err := db.Exec(`CREATE TABLE public.digital_human_runs (
 id uuid PRIMARY KEY, status text NOT NULL, waiting_reason text NOT NULL DEFAULT '', waiting_receipt uuid,
 surface text NOT NULL, conversation_id uuid, message_id uuid,
 CONSTRAINT digital_human_runs_waiting_check CHECK (
 (status = 'waiting' AND waiting_reason = 'waiting_approval' AND waiting_receipt IS NOT NULL) OR
 (status <> 'waiting' AND waiting_reason = '' AND waiting_receipt IS NULL)))`).Error; err != nil {
		t.Fatal(err)
	}
	legacyID := uuid.New()
	if err := db.Exec(`INSERT INTO digital_human_runs (id,status,waiting_reason,waiting_receipt,surface)
 VALUES (?, 'waiting', 'waiting_approval', ?, 'digital_human_console')`, legacyID, uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := upDigitalHumanPlatformCLIApproval(mschema.New(db)); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM digital_human_runs WHERE id=? AND waiting_receipt IS NOT NULL AND waiting_approval_origin=''`, legacyID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("legacy receipt changed: %d, %v", count, err)
	}
	for _, surface := range []string{"digital_human_console", "digital_human_webapp", "digital_human_api"} {
		if err := db.Exec(`INSERT INTO digital_human_runs (id,status,waiting_reason,surface,conversation_id,message_id,waiting_approval_origin)
   VALUES (?, 'waiting', 'waiting_approval', ?, ?, ?, 'platform_cli')`, uuid.New(), surface, uuid.New(), uuid.New()).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, invalid := range []struct {
		name, status, reason, surface, origin string
		receipt, conversation, message        *uuid.UUID
	}{
		{name: "missing approval owner", status: "waiting", reason: "waiting_approval", surface: "digital_human_api"},
		{name: "background", status: "waiting", reason: "waiting_approval", surface: "digital_human_task", origin: "platform_cli", conversation: uuidPtrForApprovalTest(), message: uuidPtrForApprovalTest()},
		{name: "missing message", status: "waiting", reason: "waiting_approval", surface: "digital_human_api", origin: "platform_cli", conversation: uuidPtrForApprovalTest()},
		{name: "mixed receipt", status: "waiting", reason: "waiting_approval", surface: "digital_human_api", origin: "platform_cli", receipt: uuidPtrForApprovalTest(), conversation: uuidPtrForApprovalTest(), message: uuidPtrForApprovalTest()},
		{name: "terminal marker", status: "succeeded", surface: "digital_human_api", origin: "platform_cli"},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			if err := db.Exec(`INSERT INTO digital_human_runs (id,status,waiting_reason,waiting_receipt,surface,conversation_id,message_id,waiting_approval_origin) VALUES (?,?,?,?,?,?,?,?)`, uuid.New(), invalid.status, invalid.reason, invalid.receipt, invalid.surface, invalid.conversation, invalid.message, invalid.origin).Error; err == nil {
				t.Fatal("invalid approval state accepted")
			}
		})
	}
}

func uuidPtrForApprovalTest() *uuid.UUID { id := uuid.New(); return &id }
