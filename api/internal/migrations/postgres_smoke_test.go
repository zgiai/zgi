package migrations

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/migrations/baseline"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPublicInitialSchemaAppliesToPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run PostgreSQL migration smoke test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}

	if err := RunWithDB(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	var tableCount int64
	if err := db.Raw(`
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_type = 'BASE TABLE'
		  AND table_name <> 'migrations'
	`).Scan(&tableCount).Error; err != nil {
		t.Fatalf("count public tables: %v", err)
	}
	if tableCount < 140 {
		t.Fatalf("expected initial schema to create at least 140 public tables, got %d", tableCount)
	}

	assertBaselineRelationsExist(t, db, "BASE TABLE", baselineRelationNames(t, `(?i)^CREATE\s+TABLE\s+public\.([a-z0-9_]+)\s*\(`))
	assertBaselineRelationsExist(t, db, "VIEW", baselineRelationNames(t, `(?i)^CREATE\s+VIEW\s+public\.([a-z0-9_]+)\s+AS\s+`))
	assertBaselineExtensionsExist(t, db, baselineRelationNames(t, `(?i)^CREATE\s+EXTENSION\s+IF\s+NOT\s+EXISTS\s+"?([a-z0-9_-]+)"?`))
	assertMigrationRecorded(t, db, initialSchemaMigrationID)
}

func baselineRelationNames(t *testing.T, pattern string) []string {
	t.Helper()

	re := regexp.MustCompile(pattern)
	seen := make(map[string]struct{})
	for _, file := range baseline.Files {
		for _, statement := range file.Statements {
			match := re.FindStringSubmatch(strings.TrimSpace(statement))
			if len(match) != 2 {
				continue
			}
			seen[match[1]] = struct{}{}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatalf("baseline pattern did not match any objects: %s", pattern)
	}
	return names
}

func assertBaselineRelationsExist(t *testing.T, db *gorm.DB, relationType string, expected []string) {
	t.Helper()

	var actual []string
	if err := db.Raw(`
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_type = ?
		ORDER BY table_name
	`, relationType).Scan(&actual).Error; err != nil {
		t.Fatalf("list public %s relations: %v", relationType, err)
	}

	actualSet := make(map[string]struct{}, len(actual))
	for _, name := range actual {
		actualSet[name] = struct{}{}
	}

	var missing []string
	for _, name := range expected {
		if _, ok := actualSet[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing public %s relations: %s", relationType, strings.Join(missing, ", "))
	}
}

func assertBaselineExtensionsExist(t *testing.T, db *gorm.DB, expected []string) {
	t.Helper()

	var actual []string
	if err := db.Raw(`
		SELECT extname
		FROM pg_extension
		WHERE extname IN ?
		ORDER BY extname
	`, expected).Scan(&actual).Error; err != nil {
		t.Fatalf("list PostgreSQL extensions: %v", err)
	}

	actualSet := make(map[string]struct{}, len(actual))
	for _, name := range actual {
		actualSet[name] = struct{}{}
	}

	var missing []string
	for _, name := range expected {
		if _, ok := actualSet[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing PostgreSQL extensions: %s", strings.Join(missing, ", "))
	}
}

func assertMigrationRecorded(t *testing.T, db *gorm.DB, migrationID string) {
	t.Helper()

	var count int64
	if err := db.Table("migrations").Where("id = ?", migrationID).Count(&count).Error; err != nil {
		t.Fatalf("read migration record %s: %v", migrationID, err)
	}
	if count != 1 {
		t.Fatalf("expected migration %s to be recorded once, got %d", migrationID, count)
	}
}
