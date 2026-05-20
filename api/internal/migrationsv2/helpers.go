package migrationsv2

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

func migrationIDApplied(tx *gorm.DB, id string) (bool, error) {
	if !tx.Migrator().HasTable("migrations") {
		return false, nil
	}

	var count int64
	err := tx.Table("migrations").Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

func tableExists(tx *gorm.DB, table string) bool {
	return tx.Migrator().HasTable(table)
}

func viewExists(tx *gorm.DB, view string) (bool, error) {
	var count int64
	err := tx.Raw(`
		SELECT COUNT(*)
		FROM information_schema.views
		WHERE table_schema = current_schema()
		  AND table_name = ?
	`, view).Scan(&count).Error
	return count > 0, err
}

func indexExists(tx *gorm.DB, index string) (bool, error) {
	var count int64
	err := tx.Raw(`
		SELECT COUNT(*)
		FROM pg_indexes
		WHERE schemaname = current_schema()
		  AND indexname = ?
	`, index).Scan(&count).Error
	return count > 0, err
}

func constraintExists(tx *gorm.DB, constraint string) (bool, error) {
	var count int64
	err := tx.Raw(`
		SELECT COUNT(*)
		FROM information_schema.table_constraints
		WHERE constraint_schema = current_schema()
		  AND constraint_name = ?
	`, constraint).Scan(&count).Error
	return count > 0, err
}

func statementPreview(statement string) string {
	normalized := strings.Join(strings.Fields(statement), " ")
	if len(normalized) > 96 {
		return normalized[:96] + "..."
	}
	return normalized
}

func describeMissingItems(items []string) error {
	if len(items) == 0 {
		return nil
	}
	return fmt.Errorf("missing %s", strings.Join(items, ", "))
}
