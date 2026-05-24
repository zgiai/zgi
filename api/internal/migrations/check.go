package migrations

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/zgiai/zgi/api/internal/migrations/baseline"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type CheckOptions struct {
	PostgresDSN string
}

type CheckResult struct {
	MigrationCount       int
	CheckedFiles         []string
	PostgresCheckSkipped bool
	PostgresCheckRan     bool
}

func Check(options CheckOptions) (CheckResult, error) {
	result := CheckResult{}

	if err := checkRegisteredMigrations(); err != nil {
		return result, err
	}

	files, err := checkMigrationFilenames()
	if err != nil {
		return result, err
	}
	result.CheckedFiles = files
	result.MigrationCount = len(allMigrations())

	if err := checkMigrationSourceSafety(files); err != nil {
		return result, err
	}

	if err := checkBaselineStatementSafety(); err != nil {
		return result, err
	}

	if strings.TrimSpace(options.PostgresDSN) == "" {
		result.PostgresCheckSkipped = true
		return result, nil
	}

	if err := checkFreshPostgres(options.PostgresDSN); err != nil {
		return result, err
	}
	result.PostgresCheckRan = true
	return result, nil
}

func checkRegisteredMigrations() error {
	migrations := allMigrations()
	seen := make(map[string]struct{}, len(migrations))
	for i, migration := range migrations {
		if migration == nil {
			return fmt.Errorf("migration at index %d is nil", i)
		}
		if !migrationIDPattern.MatchString(migration.ID) {
			return fmt.Errorf("migration ID %q must match public migration ID format", migration.ID)
		}
		if migration.Migrate == nil {
			return fmt.Errorf("migration %s has nil Migrate function", migration.ID)
		}
		if _, exists := seen[migration.ID]; exists {
			return fmt.Errorf("duplicate migration ID %s", migration.ID)
		}
		seen[migration.ID] = struct{}{}
		if i > 0 && migrations[i-1].ID > migration.ID {
			return fmt.Errorf("migrations must be sorted by ID: %s before %s", migrations[i-1].ID, migration.ID)
		}
	}
	return nil
}

func checkMigrationFilenames() ([]string, error) {
	root, err := migrationsDir()
	if err != nil {
		return nil, err
	}

	files, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil {
		return nil, err
	}
	slices.Sort(files)

	allowed := map[string]struct{}{
		"check.go":               {},
		"postgres_smoke_test.go": {},
		"registry.go":            {},
		"runner.go":              {},
		"runner_test.go":         {},
		"schema_executor.go":     {},
	}
	migrationIDs := make(map[string]struct{}, len(allMigrations()))
	for _, migration := range allMigrations() {
		migrationIDs[migration.ID] = struct{}{}
		expected := filepath.Join(root, migration.ID+".go")
		if _, err := os.Stat(expected); err != nil {
			return nil, fmt.Errorf("migration %s must live in %s: %w", migration.ID, expected, err)
		}
	}

	var checked []string
	for _, file := range files {
		name := filepath.Base(file)
		if _, ok := allowed[name]; ok || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		id := strings.TrimSuffix(name, ".go")
		if _, ok := migrationIDs[id]; !ok {
			return nil, fmt.Errorf("migration file %s does not match a registered migration ID", name)
		}
		checked = append(checked, file)
	}

	return checked, nil
}

func checkMigrationSourceSafety(files []string) error {
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", file, err)
		}
		normalized := strings.ToUpper(string(data))
		for _, forbidden := range []string{
			"ALLOWDESTRUCTIVE()",
			"DROP TABLE",
			"DROP COLUMN",
			"DROP SCHEMA",
			"TRUNCATE ",
			"DELETE FROM ",
			"UPDATE ",
		} {
			if strings.Contains(normalized, forbidden) {
				return fmt.Errorf("migration file %s contains forbidden token %q", filepath.Base(file), forbidden)
			}
		}
	}
	return nil
}

func checkBaselineStatementSafety() error {
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
					return fmt.Errorf("baseline file %s contains forbidden statement %q: %s", file.Name, forbidden, statementPreview(statement))
				}
			}
		}
	}
	return nil
}

func checkFreshPostgres(dsn string) error {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("connect PostgreSQL check database: %w", err)
	}
	if err := RunWithDB(db); err != nil {
		return fmt.Errorf("run migrations on fresh PostgreSQL check database: %w", err)
	}
	return nil
}

func migrationsDir() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve migrations directory")
	}
	return filepath.Dir(filename), nil
}
