package connectorfinalize

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const MigrationID = "202609031410353961_finalize_connector_detachment"
const CheckpointID = "20260903120000_messaging_connector_protocol"

func lockInventoryTables() []string {
	tables := make([]string, 0, len(RetiredTables())+len(NativeTables())+len(hostTables())+3)
	tables = append(tables, RetiredTables()...)
	tables = append(tables, NativeTables()...)
	tables = append(tables, hostTables()...)
	tables = append(tables, channelInstallationsTable, channelInstallationCredentialsTable, "migrations")
	return dedupeTables(tables)
}

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

// channelScopeWithoutLegacyConnectionSQL removes the unreachable
// channel_installations branch left behind after that table moved to
// installation-owned credentials. The remaining triggers still use this
// function for mapping, binding and inbound-event scope validation.
const channelScopeWithoutLegacyConnectionSQL = `
CREATE OR REPLACE FUNCTION public.enforce_channel_scope_consistency()
RETURNS trigger AS $$
DECLARE
	parent_org uuid;
	parent_workspace uuid;
	parent_provider varchar(32);
	parent_connection uuid;
BEGIN
	SELECT organization_id, workspace_id, provider, connection_id
	INTO parent_org, parent_workspace, parent_provider, parent_connection
	FROM public.channel_installations WHERE id = NEW.installation_id AND deleted_at IS NULL;
	IF parent_org IS NULL OR parent_org <> NEW.organization_id OR parent_workspace <> NEW.workspace_id THEN
		RAISE EXCEPTION 'channel installation scope mismatch' USING ERRCODE = '23514';
	END IF;

	IF TG_TABLE_NAME = 'channel_bindings' THEN
		IF NOT EXISTS (
			SELECT 1 FROM public.external_conversation_mappings mapping
			WHERE mapping.id = NEW.conversation_mapping_id
			  AND mapping.installation_id = NEW.installation_id
			  AND mapping.organization_id = NEW.organization_id
			  AND mapping.workspace_id = NEW.workspace_id
		) THEN
			RAISE EXCEPTION 'channel binding conversation scope mismatch' USING ERRCODE = '23514';
		END IF;
		IF NEW.subject_type = 'digital_human' AND NOT EXISTS (
			SELECT 1 FROM public.digital_humans subject
			WHERE subject.id = NEW.subject_id
			  AND subject.organization_id = NEW.organization_id
			  AND subject.workspace_id = NEW.workspace_id
			  AND subject.deleted_at IS NULL
		) THEN
			RAISE EXCEPTION 'channel binding subject scope mismatch' USING ERRCODE = '23514';
		END IF;
	END IF;

	IF TG_TABLE_NAME = 'channel_inbound_events' THEN
		IF NEW.binding_id IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM public.channel_bindings binding
			WHERE binding.id = NEW.binding_id
			  AND binding.installation_id = NEW.installation_id
			  AND binding.organization_id = NEW.organization_id
			  AND binding.workspace_id = NEW.workspace_id
		) THEN
			RAISE EXCEPTION 'channel inbound binding scope mismatch' USING ERRCODE = '23514';
		END IF;
		IF NEW.invocation_run_id IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM public.digital_human_runs run
			WHERE run.id = NEW.invocation_run_id
			  AND run.organization_id = NEW.organization_id
			  AND run.workspace_id = NEW.workspace_id
		) THEN
			RAISE EXCEPTION 'channel inbound run scope mismatch' USING ERRCODE = '23514';
		END IF;
	END IF;

	IF TG_TABLE_NAME = 'channel_delivery_outbox' THEN
		IF NEW.provider <> parent_provider OR NEW.connection_id <> parent_connection THEN
			RAISE EXCEPTION 'channel delivery provider connection mismatch' USING ERRCODE = '23514';
		END IF;
		IF NOT EXISTS (
			SELECT 1 FROM public.channel_bindings binding
			WHERE binding.id = NEW.binding_id
			  AND binding.installation_id = NEW.installation_id
			  AND binding.organization_id = NEW.organization_id
			  AND binding.workspace_id = NEW.workspace_id
			  AND binding.subject_type = NEW.subject_type
			  AND binding.subject_id = NEW.subject_id
		) THEN
			RAISE EXCEPTION 'channel delivery binding scope mismatch' USING ERRCODE = '23514';
		END IF;
		IF NOT EXISTS (
			SELECT 1 FROM public.channel_inbound_events inbound
			WHERE inbound.id = NEW.inbound_event_id
			  AND inbound.installation_id = NEW.installation_id
			  AND inbound.binding_id = NEW.binding_id
			  AND inbound.invocation_run_id = NEW.run_id
			  AND inbound.organization_id = NEW.organization_id
			  AND inbound.workspace_id = NEW.workspace_id
		) THEN
			RAISE EXCEPTION 'channel delivery inbound event mismatch' USING ERRCODE = '23514';
		END IF;
		IF NOT EXISTS (
			SELECT 1 FROM public.digital_human_runs run
			WHERE run.id = NEW.run_id
			  AND run.organization_id = NEW.organization_id
			  AND run.workspace_id = NEW.workspace_id
			  AND run.digital_human_id = NEW.subject_id
		) THEN
			RAISE EXCEPTION 'channel delivery run subject mismatch' USING ERRCODE = '23514';
		END IF;
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
`

// retirementDDL is a closed structural capability. It takes no table, column
// or SQL argument. PostgreSQL checks RESTRICT against the complete fixed set,
// including foreign keys between two members of that set.
func retirementDDL(tx *gorm.DB) error {
	if err := tx.Exec(channelScopeWithoutLegacyConnectionSQL).Error; err != nil {
		return fmt.Errorf("detach DigitalHuman channel scope validation from legacy Connections: %w", err)
	}
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
	native, err := nativeTablesAtCheckpoint(tx)
	if err != nil {
		return err
	}
	tables := append(append(RetiredTables(), native...), hostTables()...)
	tables = append(tables, "migrations")
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
