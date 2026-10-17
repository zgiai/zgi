package migrations

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/zgiai/zgi/api/internal/migrations/connectorfinalize"
	"gorm.io/gorm"
)

func TestPostRetirementChannelStatements(t *testing.T) {
	tables, triggers, err := postRetirementChannelStatements()
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range lateDigitalHumanChannelTables {
		if !strings.Contains(tables, "CREATE TABLE IF NOT EXISTS public."+table+" (") {
			t.Fatalf("late install omitted table %s", table)
		}
	}
	if strings.Contains(tables+triggers, "integration_connections") || strings.Contains(triggers, "trg_channel_installations_scope") {
		t.Fatal("late install reintroduced a retired connection dependency")
	}
	for _, table := range lateDigitalHumanChannelTables[1:] {
		if !strings.Contains(triggers, "CREATE TRIGGER trg_"+table+"_scope") {
			t.Fatalf("native scope trigger missing for %s", table)
		}
	}
	if strings.Count(createDigitalHumanChannelsSQL, " REFERENCES public.integration_connections(id) ON DELETE RESTRICT") != 2 {
		t.Fatal("ordinary historical migration was changed")
	}
}

func TestPostRetirementDigitalHumanPlanLeavesOrdinaryPathsUnchanged(t *testing.T) {
	plan := currentMigrationPlan("test")
	for _, applied := range []map[string]struct{}{
		{},
		{connectorfinalize.MigrationID: {}, migration202608191032439914ID: {}},
	} {
		got, adapted, err := postRetirementDigitalHumanPlan(nil, plan, applied, connectorfinalize.Options{})
		if err != nil || adapted || !slices.Equal(got.migrations, plan.migrations) {
			t.Fatalf("ordinary migration path changed: adapted=%v err=%v", adapted, err)
		}
	}
}

// These feature migrations were not part of the deployment that retired the
// legacy Connector tables. Freeze that supported history shape in the fixture.
var lateDigitalHumanFixturePendingIDs = []string{
	"20260813100000_add_data_source_access_policies",
	"20260816204500_allow_standalone_llm_runtime_types",
	"202608181104207370_create_digital_human_foundation",
	"202608181129156409_create_digital_human_persona_templates_and_bindings",
	"202608181152308612_create_digital_human_runs",
	"202608181328034308_backfill_digital_human_permissions",
	"202608190900000000_create_digital_human_memory",
	"202608191032439914_create_digital_human_channels",
	"202608191200000000_add_digital_human_memory_supersede_undo",
	"202608201932016586_add_default_subject_to_channel_installations",
	"202608210930000000_enable_digital_human_channel_progress",
	"202608220900000000_isolate_digital_human_channel_conversations",
	"20260822090000_create_text2sql_schema_indexes",
	"202608221300000000_index_digital_human_channel_self_messages",
	"202608231000000000_add_channel_event_preview",
	"202608231300000000_add_channel_installation_credentials",
	"202608251000000000_add_digital_human_tasks",
	"202608251100000000_allow_scheduled_channel_deliveries",
	"202608261000000000_repair_recurring_digital_human_task_status",
	"202608261100000000_add_digital_human_direct_skill_credentials",
	"202608270900000000_add_chat_runtime_inputs",
	"202608291000000000_add_channel_delivery_dismissal",
	"20260831090000_create_database_schema_index_settings",
	"20260901090000_create_database_schema_index_runs",
	"20260903183000_add_video_runtime_task_poster_url",
	"20260904120000_digital_human_connector_host",
}

func retiredDatabaseWithoutDigitalHuman(t *testing.T) *gorm.DB {
	t.Helper()
	db := finalizationTestDatabase(t)
	// Produce genuine retirement evidence, then remove only empty feature tables
	// in this isolated database to model a deployment predating DigitalHuman.
	options := migrationOptions(true)
	options.UseTransaction = true
	if err := gormigrate.New(db, options, allMigrations()).MigrateTo(connectorfinalize.MigrationID); err != nil {
		t.Fatalf("prepare retired database without DigitalHuman: %v", err)
	}
	if err := db.Exec(`DROP TABLE public.digital_humans, public.digital_human_versions,
		public.digital_human_persona_templates, public.digital_human_resource_bindings,
		public.digital_human_runs, public.digital_human_run_outbox,
		public.digital_human_memory_states, public.digital_human_memories,
		public.digital_human_memory_events, public.digital_human_memory_jobs,
		public.digital_human_memory_undo_records, public.digital_human_action_ledger,
		public.channel_installations, public.external_conversation_mappings,
		public.channel_bindings, public.channel_inbound_events, public.channel_delivery_outbox,
		public.channel_installation_credentials, public.automation_task_activities,
		public.digital_human_channel_task_policies, public.digital_human_direct_skill_credentials,
		public.chat_runtime_inputs, public.data_source_access_grants, public.data_source_access_policy_events,
		public.text2sql_schema_indexes, public.database_schema_index_settings, public.database_schema_index_runs RESTRICT`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DELETE FROM public.migrations WHERE id IN ?", lateDigitalHumanFixturePendingIDs).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE public.automation_tasks DROP CONSTRAINT automation_tasks_recurring_completion_check").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := connectorfinalize.VerifyBeforeDigitalHumanInstall(t.Context(), db, connectorfinalize.Options{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPostRetirementDigitalHumanPostgres(t *testing.T) {
	t.Run("late_install_and_repeat", func(t *testing.T) {
		db := retiredDatabaseWithoutDigitalHuman(t)
		if err := RunWithDB(db); err != nil {
			t.Fatal(err)
		}
		for _, table := range append(slices.Clone(lateDigitalHumanChannelTables), "digital_humans", "digital_human_versions", "digital_human_runs", "channel_installation_credentials", "chat_runtime_inputs", "connector_host_projection_outbox") {
			if !db.Migrator().HasTable(table) {
				t.Fatalf("late install omitted %s", table)
			}
		}
		for _, table := range connectorfinalize.RetiredTables() {
			if db.Migrator().HasTable(table) {
				t.Fatalf("late install recreated retired table %s", table)
			}
		}
		var function string
		if err := db.Raw("SELECT pg_get_functiondef('public.enforce_channel_scope_consistency()'::regprocedure)").Scan(&function).Error; err != nil {
			t.Fatal(err)
		}
		if strings.Contains(function, "integration_connections") || !strings.Contains(function, "channel binding subject scope mismatch") {
			t.Fatal("native channel scope validation was weakened or still uses legacy Connections")
		}
		if err := RunWithDB(db); err != nil {
			t.Fatalf("repeat migration failed: %v", err)
		}
		statuses, err := StatusWithDB(db)
		if err != nil {
			t.Fatal(err)
		}
		for _, status := range statuses {
			if !status.Applied {
				t.Fatalf("migration remains pending: %s", status.ID)
			}
		}
	})
	t.Run("failure_rolls_back_schema_and_markers", func(t *testing.T) {
		db := retiredDatabaseWithoutDigitalHuman(t)
		before, err := appliedMigrationIDs(db)
		if err != nil {
			t.Fatal(err)
		}
		plan := currentMigrationPlan("injected-failure")
		last := *plan.migrations[len(plan.migrations)-1]
		injected := errors.New("late install injected failure")
		last.Migrate = func(*gorm.DB) error { return injected }
		plan.migrations[len(plan.migrations)-1] = &last
		if err := runMigrationChain(db, plan); !errors.Is(err, injected) {
			t.Fatalf("expected injected failure, got %v", err)
		}
		after, err := appliedMigrationIDs(db)
		if err != nil || len(after) != len(before) || db.Migrator().HasTable("digital_humans") {
			t.Fatalf("failed late install leaked schema or migration markers: %v", err)
		}
	})
	t.Run("reject_partial_channel_schema", func(t *testing.T) {
		db := retiredDatabaseWithoutDigitalHuman(t)
		if err := db.Exec("CREATE TABLE public.channel_installations (id uuid PRIMARY KEY)").Error; err != nil {
			t.Fatal(err)
		}
		if err := RunWithDB(db); err == nil || db.Migrator().HasTable("digital_humans") {
			t.Fatal("partial channel schema was silently adopted")
		}
	})
	t.Run("reject_invalid_retirement_proof", func(t *testing.T) {
		db := retiredDatabaseWithoutDigitalHuman(t)
		if err := db.Exec("UPDATE public.connector_schema_readiness SET report_digest = ? WHERE id = 1", strings.Repeat("f", 64)).Error; err != nil {
			t.Fatal(err)
		}
		if err := RunWithDB(db); err == nil || db.Migrator().HasTable("digital_humans") {
			t.Fatal("invalid retirement proof was accepted")
		}
	})
	t.Run("reject_missing_table_with_applied_marker", func(t *testing.T) {
		db := retiredDatabaseWithoutDigitalHuman(t)
		if err := db.Exec("INSERT INTO public.migrations (id) VALUES (?)", "202608181104207370_create_digital_human_foundation").Error; err != nil {
			t.Fatal(err)
		}
		if err := RunWithDB(db); err == nil || db.Migrator().HasTable("channel_installations") {
			t.Fatal("missing previously installed DigitalHuman table was accepted")
		}
	})
}

func TestPostRetirementMigrationStatementCachePostgres(t *testing.T) {
	db := finalizationTestDatabase(t)
	if err := db.Connection(func(conn *gorm.DB) error {
		return conn.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("CREATE TEMP TABLE late_install_cache_test (id integer PRIMARY KEY); INSERT INTO late_install_cache_test VALUES (1)").Error; err != nil {
				return err
			}
			var before, after []map[string]any
			if err := tx.Raw("SELECT * FROM late_install_cache_test WHERE id = ?", 1).Scan(&before).Error; err != nil {
				return err
			}
			if err := tx.Exec("ALTER TABLE late_install_cache_test ADD COLUMN installed boolean DEFAULT true").Error; err != nil {
				return err
			}
			if err := clearMigrationStatementCache(conn); err != nil {
				return err
			}
			if err := tx.Raw("SELECT * FROM late_install_cache_test WHERE id = ?", 1).Scan(&after).Error; err != nil {
				return err
			}
			if len(before) != 1 || len(after) != 1 || after[0]["installed"] != true {
				t.Fatal("verification did not observe the new schema")
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
}
