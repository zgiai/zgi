package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestDigitalHumanMemoryMigrationDefinesOwnedFencedAuditableMemory(t *testing.T) {
	compact := strings.ToLower(strings.Join(strings.Fields(createDigitalHumanMemorySQL), " "))
	required := []string{
		"create table if not exists public.digital_human_memory_states",
		"create table if not exists public.digital_human_memories",
		"create table if not exists public.digital_human_memory_events",
		"create table if not exists public.digital_human_memory_undo_records",
		"create table if not exists public.digital_human_memory_jobs",
		"create table if not exists public.digital_human_action_ledger",
		"auto_extraction_enabled boolean not null default false",
		"memory_epoch bigint not null default 0",
		"config_revision varchar(71) not null",
		"digital_human_memories_subject_key_unique",
		"digital_human_memory_events_operation_unique",
		"digital_human_memory_event_immutable",
		"enforce_digital_human_memory_scope",
	}
	for _, token := range required {
		if !strings.Contains(compact, token) {
			t.Errorf("migration is missing %q", token)
		}
	}
	for _, forbidden := range []string{"agent_id", "user_id", "user_scope", "access_token", "api_key", "password", "cookie"} {
		if strings.Contains(compact, forbidden) {
			t.Errorf("migration contains forbidden ownership or credential token %q", forbidden)
		}
	}
}

func TestDigitalHumanMemoryMigrationExecutesUpAndDown(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*CREATE TABLE IF NOT EXISTS public.digital_human_memory_states.*").
		WillReturnResult(sqlmock.NewResult(0, 0))
	upBuilder := mschema.New(db)
	if err := up202608190900000000(upBuilder); err != nil {
		t.Fatalf("up migration: %v", err)
	}

	mock.ExpectExec("(?s).*DROP TRIGGER IF EXISTS trg_digital_human_memory_events_immutable.*").
		WillReturnResult(sqlmock.NewResult(0, 0))
	for _, table := range []string{
		"digital_human_action_ledger", "digital_human_memory_jobs", "digital_human_memory_undo_records",
		"digital_human_memory_events", "digital_human_memories", "digital_human_memory_states",
	} {
		mock.ExpectExec(`DROP TABLE IF EXISTS "public"\."` + table + `"`).
			WillReturnResult(sqlmock.NewResult(0, 0))
	}
	downBuilder := mschema.New(db).AllowDestructive()
	if err := down202608190900000000(downBuilder); err != nil {
		t.Fatalf("down migration: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
