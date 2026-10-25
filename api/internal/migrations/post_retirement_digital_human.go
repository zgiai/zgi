package migrations

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/zgiai/zgi/api/internal/migrations/connectorfinalize"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

var lateDigitalHumanChannelTables = []string{
	"channel_installations", "external_conversation_mappings", "channel_bindings",
	"channel_inbound_events", "channel_delivery_outbox",
}

// postRetirementDigitalHumanPlan adapts only the missing historical channel
// creation step. The published migration sources and ordinary upgrade path
// remain unchanged. A retired deployment must never recreate legacy tables.
func postRetirementDigitalHumanPlan(db *gorm.DB, plan migrationPlan, applied map[string]struct{}, verification connectorfinalize.Options) (migrationPlan, bool, error) {
	if _, retired := applied[connectorfinalize.MigrationID]; !retired {
		return plan, false, nil
	}
	if _, created := applied[migration202608191032439914ID]; created {
		return plan, false, nil
	}
	if report, err := connectorfinalize.VerifyBeforeDigitalHumanInstall(db.Statement.Context, db, verification); err != nil {
		return plan, false, fmt.Errorf("verify retired schema before late DigitalHuman migrations (issues: %v): %w", report.Issues, err)
	}
	for _, table := range lateDigitalHumanChannelTables {
		if db.Migrator().HasTable(table) {
			return plan, false, fmt.Errorf("late DigitalHuman bootstrap requires absent channel tables; found %s without its creation migration", table)
		}
	}
	// A successor marker without the foundation is not a supported late install.
	for _, id := range []string{
		migration202608201932016586ID, migration202608210930000000ID,
		migration202608220900000000ID, migration202608221300000000ID,
		migration202608231000000000ID, migration202608231300000000ID,
		migration202608251000000000ID, migration202608251100000000ID,
		migration202608291000000000ID, "20260904120000_digital_human_connector_host",
	} {
		if _, done := applied[id]; done {
			return plan, false, fmt.Errorf("late DigitalHuman bootstrap has inconsistent history: %s is applied before channel creation", id)
		}
	}
	plan.migrations = slices.Clone(plan.migrations)
	for index, migration := range plan.migrations {
		if migration.ID == migration202608191032439914ID {
			adapted := *migration
			adapted.Migrate = createPostRetirementDigitalHumanChannels
			plan.migrations[index] = &adapted
			return plan, true, nil
		}
	}
	return plan, false, fmt.Errorf("late DigitalHuman bootstrap is missing its registered channel migration")
}

func postRetirementChannelStatements() (string, string, error) {
	// Reuse every historical column, index and native scope check. Only the two
	// already-retired foreign keys and the obsolete installation trigger differ.
	const functionStart = "\tCREATE OR REPLACE FUNCTION public.enforce_channel_scope_consistency()"
	const nativeTriggersStart = "\tDROP TRIGGER IF EXISTS trg_external_conversation_mappings_scope"
	const legacyForeignKey = " REFERENCES public.integration_connections(id) ON DELETE RESTRICT"
	tables, functionAndTriggers, found := strings.Cut(createDigitalHumanChannelsSQL, functionStart)
	if !found || strings.Count(tables, legacyForeignKey) != 2 {
		return "", "", fmt.Errorf("historical DigitalHuman channel schema changed; review the late-install adapter")
	}
	_, triggers, found := strings.Cut(functionAndTriggers, nativeTriggersStart)
	if !found {
		return "", "", fmt.Errorf("historical DigitalHuman channel triggers changed; review the late-install adapter")
	}
	tables = strings.ReplaceAll(tables, legacyForeignKey, "")
	triggers = nativeTriggersStart + triggers
	if strings.Contains(tables+triggers, "integration_connections") {
		return "", "", fmt.Errorf("late DigitalHuman channel schema still references retired Connections")
	}
	return tables, triggers, nil
}

func createPostRetirementDigitalHumanChannels(db *gorm.DB) error {
	tables, triggers, err := postRetirementChannelStatements()
	if err != nil {
		return err
	}
	builder := mschema.New(db)
	if err := builder.Raw(tables); err != nil {
		return err
	}
	if err := connectorfinalize.RestoreNativeChannelScope(db); err != nil {
		return err
	}
	return builder.Raw(triggers)
}

// Both verification passes use the locked physical connection. DDL can change
// SELECT * result shapes, so clear pgx's client and server caches before reusing
// verification queries inside the transaction. SQL DEALLOCATE alone is unsafe:
// it leaves the driver's statement cache referring to nonexistent statements.
func clearMigrationStatementCache(connection *gorm.DB) error {
	conn, ok := connection.Statement.ConnPool.(*sql.Conn)
	if !ok {
		return fmt.Errorf("late DigitalHuman verification requires a dedicated PostgreSQL connection")
	}
	return conn.Raw(func(driverConn any) error {
		pgxConn, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("late DigitalHuman verification requires the pgx PostgreSQL driver")
		}
		return pgxConn.Conn().DeallocateAll(connection.Statement.Context)
	})
}
