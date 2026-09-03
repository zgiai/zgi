package connectorfinalize

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const MigrationID = "202609031410353961_finalize_connector_detachment"
const CheckpointID = "20260903120000_messaging_connector_protocol"

// retirementDDL is a closed structural capability. It takes no table, column
// or SQL argument. PostgreSQL checks RESTRICT against the complete fixed set,
// including foreign keys between two members of that set.
func retirementDDL(tx *gorm.DB) error {
	tables := make([]string, 0, len(RetiredTables()))
	for _, table := range RetiredTables() {
		tables = append(tables, `public."`+table+`"`)
	}
	if err := tx.Exec("DROP TABLE " + strings.Join(tables, ", ") + " RESTRICT").Error; err != nil {
		return fmt.Errorf("retire fixed Connector tables: %w", err)
	}
	for _, statement := range []string{
		`ALTER TABLE public.connector_target_bindings DROP CONSTRAINT connector_target_bindings_compatibility_check RESTRICT`,
		`ALTER TABLE public.connector_target_bindings DROP CONSTRAINT connector_target_bindings_compatibility_object_check RESTRICT`,
		`ALTER TABLE public.connector_target_bindings DROP COLUMN compatibility RESTRICT, DROP COLUMN compatibility_state RESTRICT`,
		`ALTER TABLE public.agent_resource_bindings DROP CONSTRAINT agent_resource_bindings_type_check RESTRICT`,
		`ALTER TABLE public.agent_resource_bindings ADD CONSTRAINT agent_resource_bindings_type_check CHECK (binding_type IN ('skill','knowledge_dataset','database','database_table','workflow'))`,
		`ALTER TABLE public.connector_confirmation_continuations VALIDATE CONSTRAINT connector_confirmation_continuations_native_claim_fk`,
	} {
		if err := tx.Exec(statement).Error; err != nil {
			return fmt.Errorf("retire fixed Connector compatibility structure: %w", err)
		}
	}
	return nil
}

func lockInventory(tx *gorm.DB) error {
	if err := tx.Exec(`SET LOCAL lock_timeout = '10s'`).Error; err != nil {
		return err
	}
	var locked bool
	if err := tx.Raw(`SELECT pg_try_advisory_xact_lock(hashtext('zgi:migrations'))`).Scan(&locked).Error; err != nil {
		return err
	}
	if !locked {
		return fmt.Errorf("another migration owns the maintenance lock")
	}
	tables := append(append(RetiredTables(), NativeTables()...), hostTables()...)
	tables = append(tables, "migrations")
	names := make([]string, 0, len(tables))
	for _, table := range tables {
		names = append(names, `public."`+table+`"`)
	}
	if err := tx.Exec("LOCK TABLE " + strings.Join(names, ", ") + " IN ACCESS EXCLUSIVE MODE").Error; err != nil {
		return fmt.Errorf("freeze Connector inventory; stop API and workers before maintenance: %w", err)
	}
	return nil
}
