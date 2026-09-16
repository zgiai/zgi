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

// lockInventoryTables is the closed fixed lock manifest. It is the retirement
// inventory plus the two surviving channel scope tables whose row counts the
// channel function transition inspects. The channel tables are lock-only: they
// are never added to a Source, Native or Host snapshot manifest and never
// receive retirement DDL, but Apply and FinalizeEmpty must hold ACCESS
// EXCLUSIVE on them before inspect reads the counts, otherwise a concurrent
// insert can appear between the observation and the transition.
func lockInventoryTables() []string {
	tables := make([]string, 0, len(RetiredTables())+len(NativeTables())+len(hostTables())+3)
	tables = append(tables, RetiredTables()...)
	tables = append(tables, NativeTables()...)
	tables = append(tables, hostTables()...)
	tables = append(tables, channelInstallationsTable, channelInstallationCredentialsTable)
	tables = append(tables, "migrations")
	return dedupeTables(tables)
}

// dedupeTables keeps the first occurrence of every relation so a future manifest
// change can never list one table twice in the fixed LOCK TABLE statement, and
// the resulting lock order stays deterministic.
func dedupeTables(tables []string) []string {
	seen := make(map[string]struct{}, len(tables))
	result := make([]string, 0, len(tables))
	for _, table := range tables {
		if _, ok := seen[table]; ok {
			continue
		}
		seen[table] = struct{}{}
		result = append(result, table)
	}
	return result
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
	tables := lockInventoryTables()
	// The fixed manifest includes tables introduced by later append-only
	// migrations. A fresh database has not created those relations yet, so
	// resolve the lock list against the current schema without weakening the
	// retirement capability or accepting caller-supplied table names.
	var existing []string
	if err := tx.Raw(`SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_name IN ?`, tables).Scan(&existing).Error; err != nil {
		return fmt.Errorf("inspect Connector inventory: %w", err)
	}
	existingSet := make(map[string]struct{}, len(existing))
	for _, table := range existing {
		existingSet[table] = struct{}{}
	}
	filtered := tables[:0]
	for _, table := range tables {
		if _, ok := existingSet[table]; ok {
			filtered = append(filtered, table)
		}
	}
	tables = filtered
	names := make([]string, 0, len(tables))
	for _, table := range tables {
		names = append(names, `public."`+table+`"`)
	}
	if err := tx.Exec("LOCK TABLE " + strings.Join(names, ", ") + " IN ACCESS EXCLUSIVE MODE").Error; err != nil {
		return fmt.Errorf("freeze Connector inventory; stop API and workers before maintenance: %w", err)
	}
	return nil
}
