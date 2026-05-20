package migrations

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/go-gormigrate/gormigrate/v2"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

var (
	migrationIDPattern = regexp.MustCompile(`^\d{14}(\d{2})?_[a-z0-9_]+$`)
	registered         []*gormigrate.Migration
)

func registerMigration(migration *gormigrate.Migration) {
	if migration == nil {
		panic("migration registration is nil")
	}
	registered = append(registered, migration)
}

func registerSchemaMigration(id string, up func(*mschema.Builder) error, down func(*mschema.Builder) error) {
	migration := &gormigrate.Migration{
		ID: id,
		Migrate: func(tx *gorm.DB) error {
			return up(mschema.New(tx))
		},
	}
	if down != nil {
		migration.Rollback = func(tx *gorm.DB) error {
			return down(mschema.New(tx).AllowDestructive())
		}
	}
	registerMigration(migration)
}

func registeredMigrations() []*gormigrate.Migration {
	migrations := slices.Clone(registered)
	slices.SortFunc(migrations, func(a, b *gormigrate.Migration) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})

	seen := make(map[string]struct{}, len(migrations))
	for _, migration := range migrations {
		if migration.ID == "" {
			panic("migration ID is empty")
		}
		if !migrationIDPattern.MatchString(migration.ID) {
			panic(fmt.Sprintf("migration ID %q must match YYYYMMDDHHMMSS_slug", migration.ID))
		}
		if migration.Migrate == nil {
			panic(fmt.Sprintf("migration %s has nil Migrate function", migration.ID))
		}
		if _, exists := seen[migration.ID]; exists {
			panic(fmt.Sprintf("duplicate migration ID %s", migration.ID))
		}
		seen[migration.ID] = struct{}{}
	}
	return migrations
}
