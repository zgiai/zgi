package migrations

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/zgiai/zgi/api/internal/migrations/baseline"
	"gorm.io/gorm"
)

type migrationPlan struct {
	name                      string
	migrations                []*gormigrate.Migration
	validateUnknownMigrations bool
	beforeRun                 func(*gorm.DB) error
	afterRun                  func(*gorm.DB) error
}

type migrationState struct {
	hasMigrationsTable bool
	hasApplicationData bool
	appliedIDs         map[string]struct{}
	supportsLegacy     bool
	legacyShapeErr     error
}

var (
	baselineCreateTablePattern = regexp.MustCompile(`(?i)^CREATE\s+TABLE\s+public\.([a-z0-9_]+)\s*\(`)
	applicationProbeTables     = []string{
		"accounts",
		"organizations",
		"enterprise_groups",
		"workspaces",
		"tenants",
		"workflows",
		"llm_models",
	}
	legacyRequiredTables = []string{
		"accounts",
		"organizations",
		"workspaces",
		"workflows",
		"llm_models",
		"content_parse_artifacts",
		"content_parse_runs",
		"content_parse_chunking_runs",
		"data_sources",
		"data_source_tables",
		"datasets",
		"agents",
	}
	legacyRequiredColumnTypes = []requiredColumnType{
		{table: "accounts", column: "id", dataType: "uuid", udtName: "uuid"},
		{table: "content_parse_artifacts", column: "id", dataType: "uuid", udtName: "uuid"},
		{table: "content_parse_runs", column: "id", dataType: "uuid", udtName: "uuid"},
		{table: "content_parse_chunking_runs", column: "id", dataType: "uuid", udtName: "uuid"},
		{table: "data_sources", column: "id", dataType: "uuid", udtName: "uuid"},
		{table: "data_source_tables", column: "id", dataType: "uuid", udtName: "uuid"},
		{table: "datasets", column: "id", dataType: "uuid", udtName: "uuid"},
		{table: "agents", column: "id", dataType: "uuid", udtName: "uuid"},
	}
)

type requiredColumnType struct {
	table    string
	column   string
	dataType string
	udtName  string
}

func resolveMigrationPlan(db *gorm.DB) (migrationPlan, error) {
	state := migrationState{
		hasMigrationsTable: db.Migrator().HasTable("migrations"),
		hasApplicationData: hasApplicationSchema(db),
		appliedIDs:         map[string]struct{}{},
	}

	if state.hasMigrationsTable {
		applied, err := appliedMigrationIDs(db)
		if err != nil {
			return migrationPlan{}, err
		}
		state.appliedIDs = applied
	}

	if hasNonPublicMigrationIDs(state.appliedIDs) {
		if err := validateSupportedLegacyShape(db); err != nil {
			state.legacyShapeErr = err
		} else {
			state.supportsLegacy = true
		}
	}

	return chooseMigrationPlan(state)
}

func chooseMigrationPlan(state migrationState) (migrationPlan, error) {
	if !state.hasMigrationsTable || len(state.appliedIDs) == 0 {
		if state.hasApplicationData {
			return migrationPlan{}, fmt.Errorf("database contains application tables but is missing supported migration history")
		}
		return currentMigrationPlan("fresh"), nil
	}

	if hasNonPublicMigrationIDs(state.appliedIDs) {
		if state.supportsLegacy {
			return legacyBridgeMigrationPlan(), nil
		}
		if state.legacyShapeErr != nil {
			return migrationPlan{}, fmt.Errorf("database migration history is not in the public chain and schema shape is not a supported ZGI legacy database: %w", state.legacyShapeErr)
		}
		return migrationPlan{}, fmt.Errorf("database migration history is not in the public chain and schema shape is not a supported ZGI legacy database")
	}

	return currentMigrationPlan("current"), nil
}

func currentMigrationPlan(name string) migrationPlan {
	return migrationPlan{
		name:                      name,
		migrations:                allMigrations(),
		validateUnknownMigrations: true,
	}
}

func legacyBridgeMigrationPlan() migrationPlan {
	return migrationPlan{
		name:                      "legacy-bridge",
		migrations:                allMigrations(),
		validateUnknownMigrations: false,
		beforeRun: func(db *gorm.DB) error {
			if err := validateSupportedLegacyShape(db); err != nil {
				return err
			}
			return markInitialSchemaApplied(db)
		},
		afterRun: validatePublicBaselineTables,
	}
}

func currentMigrationIDs() []string {
	migrations := allMigrations()
	ids := make([]string, 0, len(migrations))
	for _, migration := range migrations {
		ids = append(ids, migration.ID)
	}
	return ids
}

func hasApplicationSchema(db *gorm.DB) bool {
	for _, table := range applicationProbeTables {
		if db.Migrator().HasTable(table) {
			return true
		}
	}
	return false
}

func hasNonPublicMigrationIDs(appliedIDs map[string]struct{}) bool {
	publicIDs := make(map[string]struct{}, len(allMigrations()))
	for _, id := range currentMigrationIDs() {
		publicIDs[id] = struct{}{}
	}
	for id := range appliedIDs {
		if _, ok := publicIDs[id]; !ok {
			return true
		}
	}
	return false
}

func validateSupportedLegacyShape(db *gorm.DB) error {
	missing, err := missingTables(db, legacyRequiredTables)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("legacy database is missing required tables: %s", strings.Join(missing, ", "))
	}
	if err := validateColumnTypes(db, legacyRequiredColumnTypes); err != nil {
		return err
	}
	return nil
}

func validateColumnTypes(db *gorm.DB, expected []requiredColumnType) error {
	var mismatches []string
	for _, requirement := range expected {
		actual, found, err := readColumnType(db, requirement.table, requirement.column)
		if err != nil {
			return err
		}
		if !found {
			mismatches = append(mismatches, fmt.Sprintf("%s.%s missing", requirement.table, requirement.column))
			continue
		}
		if actual.dataType != requirement.dataType || actual.udtName != requirement.udtName {
			mismatches = append(mismatches, fmt.Sprintf("%s.%s expected %s/%s got %s/%s", requirement.table, requirement.column, requirement.dataType, requirement.udtName, actual.dataType, actual.udtName))
		}
	}
	if len(mismatches) > 0 {
		return fmt.Errorf("legacy database has incompatible column types: %s", strings.Join(mismatches, ", "))
	}
	return nil
}

type columnType struct {
	dataType string
	udtName  string
}

func readColumnType(db *gorm.DB, table, column string) (columnType, bool, error) {
	var rows []struct {
		DataType string `gorm:"column:data_type"`
		UDTName  string `gorm:"column:udt_name"`
	}
	if err := db.Raw(`
		SELECT data_type, udt_name
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = ?
		  AND column_name = ?
	`, table, column).Scan(&rows).Error; err != nil {
		return columnType{}, false, fmt.Errorf("inspect public column %s.%s: %w", table, column, err)
	}
	if len(rows) == 0 {
		return columnType{}, false, nil
	}
	return columnType{dataType: rows[0].DataType, udtName: rows[0].UDTName}, true, nil
}

func validatePublicBaselineTables(db *gorm.DB) error {
	missing, err := missingTables(db, baselineTableNames())
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("legacy database is missing public baseline tables after bridge migration: %s", strings.Join(missing, ", "))
	}
	return nil
}

func missingTables(db *gorm.DB, expected []string) ([]string, error) {
	if len(expected) == 0 {
		return nil, nil
	}

	var actual []string
	if err := db.Raw(`
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_type = 'BASE TABLE'
		  AND table_name IN ?
	`, expected).Scan(&actual).Error; err != nil {
		return nil, fmt.Errorf("inspect public tables: %w", err)
	}

	actualSet := make(map[string]struct{}, len(actual))
	for _, table := range actual {
		actualSet[table] = struct{}{}
	}

	var missing []string
	for _, table := range expected {
		if _, ok := actualSet[table]; !ok {
			missing = append(missing, table)
		}
	}
	slices.Sort(missing)
	return missing, nil
}

func baselineTableNames() []string {
	seen := make(map[string]struct{})
	for _, file := range baseline.Files {
		for _, statement := range file.Statements {
			match := baselineCreateTablePattern.FindStringSubmatch(strings.TrimSpace(statement))
			if len(match) != 2 {
				continue
			}
			seen[match[1]] = struct{}{}
		}
	}

	tables := make([]string, 0, len(seen))
	for table := range seen {
		tables = append(tables, table)
	}
	slices.Sort(tables)
	return tables
}

func markInitialSchemaApplied(db *gorm.DB) error {
	var count int64
	if err := db.Table("migrations").Where("id = ?", initialSchemaMigrationID).Count(&count).Error; err != nil {
		return fmt.Errorf("check initial schema marker: %w", err)
	}
	if count > 0 {
		return nil
	}
	if err := db.Table("migrations").Create(map[string]any{"id": initialSchemaMigrationID}).Error; err != nil {
		return fmt.Errorf("write initial schema marker: %w", err)
	}
	return nil
}
