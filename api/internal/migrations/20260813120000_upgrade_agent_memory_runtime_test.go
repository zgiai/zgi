package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestUpgradeAgentMemoryRuntimeMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*revision.*source_kind.*agent_memory_subject_states.*agent_memory_agent_states.*agent_memory_extraction_jobs.*agent_memory_undo_records.*").WillReturnResult(sqlmock.NewResult(0, 0))
	builder := mschema.New(db)
	if err := upUpgradeAgentMemoryRuntime(builder); err != nil {
		t.Fatal(err)
	}
	statements := strings.Join(builder.Statements(), "\n")
	for _, expected := range []string{
		"source_completed_at", "memory_epoch",
		"idempotency_key", "resulting_revision", "agent_memory_value_legacy_write_guard",
		"agent_memory_event_content_guard", "operation_id", "idx_agent_memory_events_operation",
		"extraction_cutoff_at", "idx_agent_memory_jobs_terminal_cleanup",
		"draft_config_revision", "published_config_revision", "config_scope", "config_revision", "runtime_slots",
	} {
		if !strings.Contains(statements, expected) {
			t.Fatalf("migration statements missing %q:\n%s", expected, statements)
		}
	}
	if strings.Contains(statements, "UPDATE public.agent_memory_values") || strings.Contains(statements, "UPDATE public.agent_memory_events") {
		t.Fatal("migration must not perform an unbounded value or event backfill")
	}
	if strings.Contains(statements, "write_policy") {
		t.Fatal("automatic maintenance is agent-scoped and must not add a slot-level write policy")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
