package migrations

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/migrations/baseline"
)

func TestAllMigrationsStartsAtPublicInitialSchema(t *testing.T) {
	got := allMigrations()
	if len(got) == 0 {
		t.Fatal("expected at least one public migration")
	}
	if got[0].ID != initialSchemaMigrationID {
		t.Fatalf("expected initial migration %s, got %s", initialSchemaMigrationID, got[0].ID)
	}
}

func TestRegisteredMigrationsAreValid(t *testing.T) {
	migrations := allMigrations()
	seen := make(map[string]struct{}, len(migrations))
	for _, migration := range migrations {
		if !migrationIDPattern.MatchString(migration.ID) {
			t.Fatalf("migration ID %q must match public migration ID format", migration.ID)
		}
		if migration.Migrate == nil {
			t.Fatalf("migration %s has nil Migrate function", migration.ID)
		}
		if _, exists := seen[migration.ID]; exists {
			t.Fatalf("duplicate migration ID %s", migration.ID)
		}
		seen[migration.ID] = struct{}{}
	}
	for i := 1; i < len(migrations); i++ {
		if migrations[i-1].ID > migrations[i].ID {
			t.Fatalf("migrations must be sorted by ID: %s before %s", migrations[i-1].ID, migrations[i].ID)
		}
	}
}

func TestMigrationIDPatternAcceptsPublicFormats(t *testing.T) {
	valid := []string{
		"20260520000000_initial_schema",
		"2026052000000012_legacy_short_suffix",
		"202605240013420827_create_audit_events",
	}
	for _, id := range valid {
		if !migrationIDPattern.MatchString(id) {
			t.Fatalf("expected migration ID %q to be valid", id)
		}
	}

	invalid := []string{
		"2026052000000_missing_digit",
		"20260524001342082_three_digit_suffix",
		"20260524001342082708_too_long_suffix",
		"202605240013420827_create-audit-events",
	}
	for _, id := range invalid {
		if migrationIDPattern.MatchString(id) {
			t.Fatalf("expected migration ID %q to be invalid", id)
		}
	}
}

func TestChooseMigrationPlanForFreshDatabase(t *testing.T) {
	plan, err := chooseMigrationPlan(migrationState{})
	if err != nil {
		t.Fatalf("choose migration plan: %v", err)
	}
	if plan.name != "fresh" {
		t.Fatalf("expected fresh plan, got %s", plan.name)
	}
	if !plan.validateUnknownMigrations {
		t.Fatal("fresh plan must validate unknown migrations")
	}
	if len(plan.migrations) == 0 || plan.migrations[0].ID != initialSchemaMigrationID {
		t.Fatalf("fresh plan must start with %s", initialSchemaMigrationID)
	}
}

func TestChooseMigrationPlanRejectsNonEmptyDatabaseWithoutHistory(t *testing.T) {
	_, err := chooseMigrationPlan(migrationState{hasApplicationData: true})
	if err == nil {
		t.Fatal("expected non-empty database without migration history to fail")
	}
	if !strings.Contains(err.Error(), "missing supported migration history") {
		t.Fatalf("expected supported history error, got %v", err)
	}
}

func TestChooseMigrationPlanSupportsLegacyShapeWithPrivateHistory(t *testing.T) {
	plan, err := chooseMigrationPlan(migrationState{
		hasMigrationsTable: true,
		hasApplicationData: true,
		supportsLegacy:     true,
		appliedIDs: map[string]struct{}{
			"pre_public_history": {},
		},
	})
	if err != nil {
		t.Fatalf("choose migration plan: %v", err)
	}
	if plan.name != "legacy-bridge" {
		t.Fatalf("expected legacy bridge plan, got %s", plan.name)
	}
	if plan.validateUnknownMigrations {
		t.Fatal("legacy bridge must allow historical migration IDs outside the public chain")
	}
	if plan.beforeRun == nil || plan.afterRun == nil {
		t.Fatal("legacy bridge must have preflight and post-run verification")
	}
}

func TestChooseMigrationPlanKeepsLegacyBridgeAfterBaselineMarker(t *testing.T) {
	plan, err := chooseMigrationPlan(migrationState{
		hasMigrationsTable: true,
		hasApplicationData: true,
		supportsLegacy:     true,
		appliedIDs: map[string]struct{}{
			"pre_public_history":     {},
			initialSchemaMigrationID: {},
		},
	})
	if err != nil {
		t.Fatalf("choose migration plan: %v", err)
	}
	if plan.name != "legacy-bridge" {
		t.Fatalf("expected legacy bridge plan, got %s", plan.name)
	}
	if plan.validateUnknownMigrations {
		t.Fatal("legacy bridge must keep allowing private migration IDs after baseline marker is written")
	}
}

func TestChooseMigrationPlanUsesCurrentPublicHistory(t *testing.T) {
	plan, err := chooseMigrationPlan(migrationState{
		hasMigrationsTable: true,
		hasApplicationData: true,
		appliedIDs: map[string]struct{}{
			initialSchemaMigrationID: {},
		},
	})
	if err != nil {
		t.Fatalf("choose migration plan: %v", err)
	}
	if plan.name != "current" {
		t.Fatalf("expected current plan, got %s", plan.name)
	}
	if !plan.validateUnknownMigrations {
		t.Fatal("current public history must validate unknown migrations")
	}
}

func TestChooseMigrationPlanRejectsUnsupportedLegacyShape(t *testing.T) {
	_, err := chooseMigrationPlan(migrationState{
		hasMigrationsTable: true,
		hasApplicationData: true,
		legacyShapeErr:     errors.New("legacy database is missing required tables: accounts"),
		appliedIDs: map[string]struct{}{
			"pre_public_history": {},
		},
	})
	if err == nil {
		t.Fatal("expected unsupported legacy shape to fail")
	}
	if !strings.Contains(err.Error(), "schema shape is not a supported ZGI legacy database") {
		t.Fatalf("expected unsupported shape error, got %v", err)
	}
	if !strings.Contains(err.Error(), "missing required tables") {
		t.Fatalf("expected missing tables context, got %v", err)
	}
}

func TestCheckStaticRules(t *testing.T) {
	result, err := Check(CheckOptions{})
	if err != nil {
		t.Fatalf("migration check failed: %v", err)
	}
	if result.MigrationCount != len(allMigrations()) {
		t.Fatalf("expected %d migrations checked, got %d", len(allMigrations()), result.MigrationCount)
	}
	if len(result.CheckedFiles) != len(allMigrations()) {
		t.Fatalf("expected %d migration files checked, got %d", len(allMigrations()), len(result.CheckedFiles))
	}
	if !result.PostgresCheckSkipped {
		t.Fatal("expected PostgreSQL execution to be skipped without a DSN")
	}
	if result.PostgresCheckRan {
		t.Fatal("PostgreSQL execution must not run without a DSN")
	}
}

func TestMigrationFilenameMatchesRegisteredID(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve caller path")
	}
	root := filepath.Dir(filename)

	for _, migration := range allMigrations() {
		if migration.ID == initialSchemaMigrationID {
			continue
		}
		path := filepath.Join(root, migration.ID+".go")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("migration %s must live in %s: %v", migration.ID, path, err)
		}
	}
}

func TestInitialSchemaSnapshotLooksComplete(t *testing.T) {
	text := readInitialSchemaText(t)

	if count := strings.Count(text, "CREATE TABLE public."); count < 140 {
		t.Fatalf("expected public baseline to contain at least 140 tables, got %d", count)
	}
	if count := strings.Count(text, "CREATE VIEW public."); count < 7 {
		t.Fatalf("expected public baseline to contain compatibility views, got %d", count)
	}
	if strings.Contains(text, "CREATE TABLE public.migrations") {
		t.Fatal("public baseline must not create the gormigrate migrations table")
	}
	if strings.Contains(text, "COPY ") || strings.Contains(text, "INSERT INTO ") {
		t.Fatal("public baseline must be schema-only and must not contain data")
	}
}

func TestInitialSchemaSnapshotIsSafeForExistingData(t *testing.T) {
	for _, file := range baseline.Files {
		for _, statement := range file.Statements {
			normalized := strings.ToUpper(strings.Join(strings.Fields(statement), " "))
			for _, forbidden := range []string{
				"DROP TABLE",
				"DROP SCHEMA",
				"TRUNCATE ",
				"DELETE FROM ",
				"UPDATE ",
				"ALTER TABLE ONLY PUBLIC.MIGRATIONS",
			} {
				if strings.HasPrefix(normalized, forbidden) {
					t.Fatalf("public baseline must not contain destructive statement %q: %s", forbidden, statementPreview(statement))
				}
			}
		}
	}
}

func TestInitialSchemaFilesAreOrderedAndPresent(t *testing.T) {
	if len(baseline.Files) < 5 {
		t.Fatal("public baseline must be split into ordered schema chunks")
	}
	for i, file := range baseline.Files {
		if file.Name == "" {
			t.Fatal("initial schema chunk name must not be empty")
		}
		if len(file.Statements) == 0 {
			t.Fatalf("initial schema chunk %s must contain statements", file.Name)
		}
		if i > 0 && baseline.Files[i-1].Name >= file.Name {
			t.Fatalf("initial schema chunks must be listed in execution order: %s before %s", baseline.Files[i-1].Name, file.Name)
		}
	}
}

func TestBaselineTableNamesIncludesBridgeCriticalTables(t *testing.T) {
	tables := baselineTableNames()
	seen := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		seen[table] = struct{}{}
	}

	for _, table := range []string{
		"content_parse_chunk_artifact_sets",
		"data_library_document_assets",
		"data_library_database_asset_refs",
		"workflow_test_settings",
	} {
		if _, ok := seen[table]; !ok {
			t.Fatalf("expected baseline table list to include %s", table)
		}
	}
}

func TestWorkflowTestGenerationTasksMigrationDefinesActiveUniqueIndex(t *testing.T) {
	sql := strings.Join(strings.Fields(workflowTestGenerationTasksActiveIndexSQL), " ")
	for _, want := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_test_generation_tasks_active_agent",
		"ON public.workflow_test_generation_tasks (agent_id)",
		"WHERE status IN ('queued', 'running', 'canceling')",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("active generation task unique index SQL missing %q: %s", want, sql)
		}
	}
}

func TestWorkflowTestScenarioRecognitionTasksMigrationDefinesActiveUniqueIndex(t *testing.T) {
	sql := strings.Join(strings.Fields(workflowTestScenarioRecognitionTasksActiveIndexSQL), " ")
	for _, want := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_test_scenario_recognition_tasks_active_agent",
		"ON public.workflow_test_scenario_recognition_tasks (agent_id)",
		"WHERE status IN ('queued', 'running', 'canceling')",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("active scenario recognition task unique index SQL missing %q: %s", want, sql)
		}
	}
}

func TestDropAnnouncementRunNodeUniqueIndexMigrationSQL(t *testing.T) {
	upSQL := strings.Join(strings.Fields(dropAnnouncementRunNodeUniqueIndexSQL), " ")
	if !strings.Contains(upSQL, "DROP INDEX IF EXISTS public.idx_announcements_run_node") {
		t.Fatalf("drop announcement run/node index SQL missing expected drop: %s", upSQL)
	}

	guardSQL := strings.Join(strings.Fields(ensureNoDuplicateAnnouncementRunNodeSQL), " ")
	for _, want := range []string{
		"FROM public.announcements",
		"GROUP BY workflow_run_id, node_id",
		"HAVING COUNT(*) > 1",
		"RAISE EXCEPTION",
	} {
		if !strings.Contains(guardSQL, want) {
			t.Fatalf("announcement run/node rollback guard SQL missing %q: %s", want, guardSQL)
		}
	}

	recreateSQL := strings.Join(strings.Fields(recreateAnnouncementRunNodeUniqueIndexSQL), " ")
	for _, want := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_announcements_run_node",
		"ON public.announcements (workflow_run_id, node_id)",
	} {
		if !strings.Contains(recreateSQL, want) {
			t.Fatalf("announcement run/node recreate SQL missing %q: %s", want, recreateSQL)
		}
	}
}

func TestLegacyBridgePreflightExcludesBackfilledTablesOnly(t *testing.T) {
	tables := baselineTableNamesExcluding(legacyBridgeBackfilledTables)
	seen := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		seen[table] = struct{}{}
	}

	for _, table := range legacyBridgeBackfilledTables {
		if _, ok := seen[table]; ok {
			t.Fatalf("expected legacy bridge preflight to allow %s to be backfilled by public migrations", table)
		}
	}
	if _, ok := seen["workflow_test_settings"]; !ok {
		t.Fatal("expected legacy bridge preflight to keep validating baseline tables not backfilled by public migrations")
	}
}

func TestLegacyBridgeBaselinePreflightRunsOnlyBeforeInitialMarker(t *testing.T) {
	if !shouldValidatePublicBaselineBeforeBridge(map[string]struct{}{}) {
		t.Fatal("expected first legacy bridge run to validate the public baseline shape")
	}
	if shouldValidatePublicBaselineBeforeBridge(map[string]struct{}{initialSchemaMigrationID: {}}) {
		t.Fatal("expected resumed legacy bridge run to trust migration history after the initial marker")
	}
	if shouldValidatePublicBaselineBeforeBridge(map[string]struct{}{
		initialSchemaMigrationID:                        {},
		migrationCreateChatRuntimeAndAgentVersionsID:    {},
		migration20260526090001ID:                       {},
		"closed_source_private_history_before_baseline": {},
	}) {
		t.Fatal("expected partially applied public migration history to skip initial baseline table checks")
	}
}

func TestLegacyBridgePostRunVerifiesPublicMigrationRecords(t *testing.T) {
	applied := make(map[string]struct{}, len(currentMigrationIDs()))
	for _, id := range currentMigrationIDs() {
		applied[id] = struct{}{}
	}
	if missing := missingPublicMigrationIDs(applied); len(missing) != 0 {
		t.Fatalf("expected complete public migration history, got missing %v", missing)
	}

	delete(applied, migration20260526090001ID)
	missing := missingPublicMigrationIDs(applied)
	if len(missing) != 1 || missing[0] != migration20260526090001ID {
		t.Fatalf("expected missing file extraction cache migration, got %v", missing)
	}
}

func TestDataLibraryFoundationMigrationGuardsExtractionArtifactConstraint(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve caller path")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(filename), migration202605231629280827ID+".go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)

	if !strings.Contains(text, "pg_constraint") {
		t.Fatal("data library extraction artifact foreign key must be guarded before ALTER TABLE")
	}
	if strings.Contains(text, "isDataLibraryDuplicateConstraintError") {
		t.Fatal("migration must not suppress duplicate constraint errors after executing ALTER TABLE")
	}
}

func TestLegacyBridgeDoesNotHardcodeClosedSourceMigrationIDs(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve caller path")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "legacy_bridge.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(data))
	privateMigrationIDPattern := regexp.MustCompile(`\b20\d{12,}(?:_[0-9]+)?\b`)
	if match := privateMigrationIDPattern.FindString(text); match != "" {
		t.Fatalf("legacy bridge must not expose closed-source migration ID %q", match)
	}
}

func TestNoClosedSourceMigrationHistoryFilesRemain(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve caller path")
	}
	root := filepath.Dir(filename)

	var historical []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, "m") && strings.HasSuffix(name, ".go") {
			historical = append(historical, filepath.ToSlash(path))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if len(historical) > 0 {
		t.Fatalf("closed-source migration history files must not remain: %s", strings.Join(historical, ", "))
	}
}

func readInitialSchemaText(t *testing.T) string {
	t.Helper()

	var builder strings.Builder
	for _, file := range baseline.Files {
		builder.WriteString(strings.Join(file.Statements, "\n"))
		builder.WriteByte('\n')
	}
	return builder.String()
}
