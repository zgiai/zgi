package migrations

import (
	"io/fs"
	"os"
	"path/filepath"
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
