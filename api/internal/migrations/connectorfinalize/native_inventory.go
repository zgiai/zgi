package connectorfinalize

import (
	"fmt"

	"gorm.io/gorm"
)

// nativeTablesAtCheckpoint keeps the fixed inventory required except for the
// pending-setups table, whose creation migration sorts after retirement. Its
// absence is valid only before that migration has run; a marker/schema mismatch
// is a partial migration and must fail instead of producing an empty proof.
func nativeTablesAtCheckpoint(db *gorm.DB) ([]string, error) {
	var state struct {
		Applied bool
		Present bool
	}
	if err := db.Raw(`SELECT
		EXISTS (SELECT 1 FROM public.migrations WHERE id = ?) AS applied,
		to_regclass('public.connector_pending_setups') IS NOT NULL AS present`, pendingSetupsMigrationID).Scan(&state).Error; err != nil {
		return nil, fmt.Errorf("inspect pending setup migration checkpoint: %w", err)
	}
	if state.Applied != state.Present {
		return nil, fmt.Errorf("connector_pending_setups table and creating migration marker disagree")
	}
	tables := make([]string, 0, len(NativeTables()))
	for _, table := range NativeTables() {
		if table != "connector_pending_setups" || state.Applied {
			tables = append(tables, table)
		}
	}
	return tables, nil
}
