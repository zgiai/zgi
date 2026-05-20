package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0144ID = "20260418000144"

// M0144_drop_llm_route_foreign_keys removes legacy LLM foreign keys so the
// gateway relies on application-level validation and transactional cleanup.
func M0144_drop_llm_route_foreign_keys() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0144ID,
		Migrate: func(tx *gorm.DB) error {
			if tx.Dialector.Name() != "postgres" {
				return nil
			}

			operations := []struct {
				tableName   string
				columnName  string
				constraints []string
			}{
				{
					tableName:   "llm_system_channels",
					columnName:  "credential_id",
					constraints: []string{"llm_system_channels_credential_id_fkey"},
				},
				{
					tableName:   "llm_tenant_routes",
					columnName:  "user_credential_id",
					constraints: []string{"llm_tenant_routes_user_credential_id_fkey", "fk_route_credential", "fk_tenant_route_credential", "fk_llm_routes_credential"},
				},
				{
					tableName:   "llm_tenant_routes",
					columnName:  "system_channel_id",
					constraints: []string{"llm_tenant_routes_system_channel_id_fkey"},
				},
				{
					tableName:   "llm_routes",
					columnName:  "user_credential_id",
					constraints: []string{"llm_routes_user_credential_id_fkey", "fk_route_credential", "fk_tenant_route_credential", "fk_llm_routes_credential"},
				},
				{
					tableName:   "llm_routes",
					columnName:  "system_channel_id",
					constraints: []string{"llm_routes_system_channel_id_fkey", "llm_tenant_routes_system_channel_id_fkey"},
				},
			}

			for _, operation := range operations {
				if err := dropLLMForeignKeysIfPresent(tx, operation.tableName, operation.columnName, operation.constraints...); err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return fmt.Errorf("migration %s is irreversible", migration0144ID)
		},
	}
}

func dropLLMForeignKeysIfPresent(tx *gorm.DB, tableName, columnName string, constraints ...string) error {
	exists, err := tableExists(tx, tableName)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	columnPresent, err := columnExists(tx, tableName, columnName)
	if err != nil {
		return err
	}
	if !columnPresent {
		return nil
	}

	for _, constraintName := range constraints {
		if err := tx.Exec(
			fmt.Sprintf(`ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s`, tableName, constraintName),
		).Error; err != nil {
			return fmt.Errorf("drop constraint %s on %s: %w", constraintName, tableName, err)
		}
	}

	return nil
}
