package migrations

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/pkg/database"
	"gorm.io/gorm"
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

func migrationOptions() *gormigrate.Options {
	options := *gormigrate.DefaultOptions
	options.ValidateUnknownMigrations = true
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

	m := gormigrate.New(db, migrationOptions(), allMigrations())
	if err := m.Migrate(); err != nil {
		log.Printf("migrations failed: %v", err)
		return err
	}

	log.Println("migrations completed successfully")
	return nil
}

func migrationLock(db *gorm.DB, noLock bool) (func(), error) {
	if noLock {
		if os.Getenv("ZGI_UNSAFE_NO_MIGRATION_LOCK") != "1" {
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

	m := gormigrate.New(db, migrationOptions(), allMigrations())
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
