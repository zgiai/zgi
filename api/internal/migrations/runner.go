package migrations

import (
	"errors"
	"fmt"
	"log"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/migrations/connectorfinalize"
	"github.com/zgiai/zgi/api/pkg/database"
)

const advisoryLockKey = "zgi:migrations"

type RunOptions struct {
	DryRun bool
	NoLock bool
}

type RollbackOptions struct {
	ConfirmID string
	NoLock    bool
}

type MigrationStatus struct {
	ID      string
	Applied bool
}

func allMigrations() []*gormigrate.Migration {
	return registeredMigrations()
}

func migrationsForRun() []*gormigrate.Migration {
	migrations := allMigrations()
	if !config.ConnectorRetirementDeferred() {
		return migrations
	}
	filtered := make([]*gormigrate.Migration, 0, len(migrations))
	for _, migration := range migrations {
		if migration.ID == connectorfinalize.MigrationID {
			continue
		}
		filtered = append(filtered, migration)
	}
	return filtered
}

func migrationOptions(validateUnknownMigrations bool) *gormigrate.Options {
	options := *gormigrate.DefaultOptions
	options.ValidateUnknownMigrations = validateUnknownMigrations
	return &options
}

func Run() error {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		return err
	}

	db, err := database.InitDB(cfg.Database)
	if err != nil {
		log.Printf("Failed to initialize database: %v", err)
		return err
	}

	return RunWithDB(db)
}

func RunWithDB(db *gorm.DB) error {
	return RunWithOptions(db, RunOptions{})
}

func RunWithOptions(db *gorm.DB, options RunOptions) error {
	// Session advisory locks must be acquired and released on the same physical
	// PostgreSQL connection, including the final transaction's marker write.
	return db.Connection(func(conn *gorm.DB) error { return runOnConnection(conn.Session(&gorm.Session{NewDB: true}), options) })
}

func runOnConnection(db *gorm.DB, options RunOptions) error {
	normalizeMigrationTableColumns(db)

	unlock, err := migrationLock(db, options.NoLock)
	if err != nil {
		return err
	}
	if unlock != nil {
		defer unlock()
	}

	if options.DryRun {
		return PrintStatusWithDB(db)
	}

	plan, err := resolveMigrationPlan(db)
	if err != nil {
		log.Printf("failed to resolve migration plan: %v", err)
		return err
	}
	log.Printf("using migration plan %s with %d migrations", plan.name, len(plan.migrations))

	if plan.beforeRun != nil {
		if err := plan.beforeRun(db); err != nil {
			log.Printf("migration plan %s preflight failed: %v", plan.name, err)
			return err
		}
	}
	if config.ConnectorRetirementDeferred() {
		if err := db.Transaction(func(tx *gorm.DB) error {
			return connectorfinalize.RepairPendingSetupsCompatibility(tx)
		}); err != nil {
			log.Printf("deferred Connector compatibility repair failed: %v", err)
			return err
		}
	}

	if err := runMigrationChain(db, plan); err != nil {
		log.Printf("migrations failed: %v", err)
		return err
	}

	if plan.afterRun != nil {
		if err := plan.afterRun(db); err != nil {
			log.Printf("migration plan %s verification failed: %v", plan.name, err)
			return err
		}
	}

	log.Println("migrations completed successfully")
	return nil
}

func runMigrationChain(db *gorm.DB, plan migrationPlan) error {
	deferred := config.ConnectorRetirementDeferred()
	if deferred && plan.validateUnknownMigrations {
		applied, err := appliedMigrationIDs(db)
		if err != nil {
			return err
		}
		if err := validateDeferredMigrationHistoryForPlan(plan, applied, deferred); err != nil {
			return err
		}
	}
	// The destructive retirement migration is intentionally omitted from the
	// deferred run list. Validate its marker against the complete public list
	// ourselves so an already-retired database remains valid without allowing
	// arbitrary unknown migration IDs in a public-chain plan. The legacy bridge
	// permits historical IDs only after its schema preflight has succeeded;
	// preserve that policy here instead of rejecting those IDs a second time.
	options := migrationOptions(plan.validateUnknownMigrations && !deferred)
	m := gormigrate.New(db, options, plan.migrations)
	applied, err := appliedMigrationIDs(db)
	if err != nil {
		return err
	}
	if _, done := applied[connectorfinalize.MigrationID]; !done {
		if deferred {
			if _, checkpointDone := applied[connectorfinalize.CheckpointID]; !checkpointDone {
				if err := m.MigrateTo(connectorfinalize.CheckpointID); err != nil {
					return err
				}
			}
		} else {
			// Preserve historical transaction boundaries. Only the final maintenance
			// step and its migration marker require this additional atomic boundary.
			if err := m.MigrateTo(connectorfinalize.CheckpointID); err != nil {
				return err
			}
		}
		options.UseTransaction = true
		m = gormigrate.New(db, options, plan.migrations)
	}
	if err := m.Migrate(); err != nil {
		return err
	}
	// The credit columns must hold milli-points in BIGINT; a fractional column
	// would make every amount 1000x wrong, so refuse to serve with one.
	return assertMilliPointSchema(db)
}

func validateDeferredMigrationHistory(applied map[string]struct{}) error {
	return validateDeferredMigrationHistoryForPlan(migrationPlan{validateUnknownMigrations: true}, applied, true)
}

func validateDeferredMigrationHistoryForPlan(plan migrationPlan, applied map[string]struct{}, deferred bool) error {
	if !deferred || !plan.validateUnknownMigrations {
		return nil
	}
	known := make(map[string]struct{}, len(allMigrations()))
	for _, migration := range allMigrations() {
		known[migration.ID] = struct{}{}
	}
	for id := range applied {
		if _, ok := known[id]; !ok {
			return fmt.Errorf("deferred Connector retirement found unknown migration %s", id)
		}
	}
	return nil
}

// RunToConnectorCheckpoint supports the two published maintenance boundaries.
// It never skips unknown migrations or guesses a checkpoint from an ID prefix.
func RunToConnectorCheckpoint(db *gorm.DB, id string) error {
	if id != "20260902190000_detach_connector_runtime" && id != connectorfinalize.CheckpointID {
		return fmt.Errorf("unsupported Connector upgrade checkpoint")
	}
	return db.Connection(func(conn *gorm.DB) error {
		conn = conn.Session(&gorm.Session{NewDB: true})
		unlock, err := migrationLock(conn, false)
		if err != nil {
			return err
		}
		defer unlock()
		plan, err := resolveMigrationPlan(conn)
		if err != nil {
			return err
		}
		applied, err := appliedMigrationIDs(conn)
		if err != nil {
			return err
		}
		for appliedID := range applied {
			if appliedID > id && migrationIDPattern.MatchString(appliedID) {
				return fmt.Errorf("database has advanced beyond the requested checkpoint")
			}
		}
		if plan.beforeRun != nil {
			if err := plan.beforeRun(conn); err != nil {
				return err
			}
		}
		return gormigrate.New(conn, migrationOptions(plan.validateUnknownMigrations), plan.migrations).MigrateTo(id)
	})
}

func migrationLock(db *gorm.DB, noLock bool) (func(), error) {
	if noLock {
		if config.GetString("ZGI_UNSAFE_NO_MIGRATION_LOCK", "") != "1" {
			return nil, errors.New("disabling the migration lock requires ZGI_UNSAFE_NO_MIGRATION_LOCK=1")
		}
		return nil, nil
	}

	unlock, err := acquireMigrationLock(db)
	if err != nil {
		return nil, err
	}
	return unlock, nil
}

func Rollback() error {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		return err
	}

	db, err := database.InitDB(cfg.Database)
	if err != nil {
		log.Printf("Failed to initialize database: %v", err)
		return err
	}

	return RollbackWithOptions(db, RollbackOptions{})
}

func RollbackWithDB(db *gorm.DB) error {
	return RollbackWithOptions(db, RollbackOptions{})
}

func RollbackWithOptions(db *gorm.DB, options RollbackOptions) error {
	normalizeMigrationTableColumns(db)

	lastApplied, err := lastAppliedMigrationID(db)
	if err != nil {
		return err
	}
	if lastApplied == "" {
		return errors.New("no applied migration found to roll back")
	}
	if options.ConfirmID == "" {
		return fmt.Errorf("rollback requires explicit confirmation: pass --confirm %s", lastApplied)
	}
	if options.ConfirmID != lastApplied {
		return fmt.Errorf("rollback confirmation mismatch: latest applied migration is %s, got %s", lastApplied, options.ConfirmID)
	}

	unlock, err := migrationLock(db, options.NoLock)
	if err != nil {
		return err
	}
	if unlock != nil {
		defer unlock()
	}

	plan, err := resolveMigrationPlan(db)
	if err != nil {
		log.Printf("failed to resolve migration rollback plan: %v", err)
		return err
	}

	m := gormigrate.New(db, migrationOptions(plan.validateUnknownMigrations), plan.migrations)
	if err := m.RollbackLast(); err != nil {
		log.Printf("migrations rollback failed: %v", err)
		return err
	}

	log.Println("migrations rollback completed successfully")
	return nil
}

func lastAppliedMigrationID(db *gorm.DB) (string, error) {
	applied, err := appliedMigrationIDs(db)
	if err != nil {
		return "", err
	}
	migrations := allMigrations()
	for i := len(migrations) - 1; i >= 0; i-- {
		if _, ok := applied[migrations[i].ID]; ok {
			return migrations[i].ID, nil
		}
	}
	return "", nil
}

func Status() ([]MigrationStatus, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	db, err := database.InitDB(cfg.Database)
	if err != nil {
		return nil, err
	}
	return StatusWithDB(db)
}

func StatusWithDB(db *gorm.DB) ([]MigrationStatus, error) {
	migrations := allMigrations()
	applied, err := appliedMigrationIDs(db)
	if err != nil {
		return nil, err
	}

	statuses := make([]MigrationStatus, 0, len(migrations))
	for _, migration := range migrations {
		_, ok := applied[migration.ID]
		statuses = append(statuses, MigrationStatus{ID: migration.ID, Applied: ok})
	}
	return statuses, nil
}

func PrintStatus() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	db, err := database.InitDB(cfg.Database)
	if err != nil {
		return err
	}
	return PrintStatusWithDB(db)
}

func PrintStatusWithDB(db *gorm.DB) error {
	statuses, err := StatusWithDB(db)
	if err != nil {
		return err
	}
	for _, status := range statuses {
		state := "pending"
		if status.Applied {
			state = "applied"
		}
		fmt.Printf("%-8s %s\n", state, status.ID)
	}
	return nil
}

func appliedMigrationIDs(db *gorm.DB) (map[string]struct{}, error) {
	applied := make(map[string]struct{})
	if !db.Migrator().HasTable("migrations") {
		return applied, nil
	}

	var ids []string
	if err := db.Table("migrations").Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	for _, id := range ids {
		applied[id] = struct{}{}
	}
	return applied, nil
}

func acquireMigrationLock(db *gorm.DB) (func(), error) {
	var locked bool
	if err := db.Raw(`SELECT pg_try_advisory_lock(hashtext(?))`, advisoryLockKey).Scan(&locked).Error; err != nil {
		return nil, fmt.Errorf("acquire migration lock: %w", err)
	}
	if !locked {
		return nil, errors.New("another migration process is already running")
	}
	return func() {
		var unlocked bool
		if err := db.Raw(`SELECT pg_advisory_unlock(hashtext(?))`, advisoryLockKey).Scan(&unlocked).Error; err != nil {
			log.Printf("Warning: Failed to release migration lock: %v", err)
		}
	}, nil
}

func normalizeMigrationTableColumns(db *gorm.DB) {
	if !db.Migrator().HasTable("migrations") {
		return
	}

	if hasColumn(db, "migrations", "id") {
		if err := db.Exec(`ALTER TABLE "migrations" ALTER COLUMN "id" TYPE varchar(255)`).Error; err != nil {
			log.Printf("Warning: Failed to alter migrations.id type: %v", err)
		}
	}

	if hasColumn(db, "migrations", "migration") {
		if err := db.Exec(`ALTER TABLE "migrations" ALTER COLUMN "migration" DROP NOT NULL`).Error; err != nil {
			log.Printf("Warning: Failed to drop NOT NULL from migrations.migration: %v", err)
		}
	}

	if hasColumn(db, "migrations", "batch") {
		if err := db.Exec(`ALTER TABLE "migrations" ALTER COLUMN "batch" DROP NOT NULL`).Error; err != nil {
			log.Printf("Warning: Failed to drop NOT NULL from migrations.batch: %v", err)
		}
	}
}

func hasColumn(db *gorm.DB, table, column string) bool {
	var exists bool
	if err := db.Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = ?
			  AND column_name = ?
		)
	`, table, column).Scan(&exists).Error; err != nil {
		log.Printf("Warning: Failed to inspect column %s.%s: %v", table, column, err)
		return false
	}
	return exists
}
