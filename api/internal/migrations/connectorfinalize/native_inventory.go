package connectorfinalize

import (
	"fmt"

	"gorm.io/gorm"
)

const pendingSetupsMigrationID = "20260904160000_connector_pending_setups"

// Legacy names remain available to the retirement matrix tests and to the
// older maintenance helpers while the runtime resolver uses the explicit ID.
const (
	pendingSetupsTable     = "connector_pending_setups"
	pendingSetupsMigration = pendingSetupsMigrationID
)

func pendingSetupsDisposition(markerApplied, relationExists bool) (bool, error) {
	switch {
	case markerApplied == relationExists:
		return !markerApplied, nil
	case markerApplied:
		return false, fmt.Errorf("native table %s is missing although its creation migration %s is applied", pendingSetupsTable, pendingSetupsMigration)
	default:
		return false, fmt.Errorf("native table %s exists although its creation migration %s is not applied", pendingSetupsTable, pendingSetupsMigration)
	}
}

func nativeInventory(markerApplied, relationExists bool) (tables, deferred []string, err error) {
	deferPending, err := pendingSetupsDisposition(markerApplied, relationExists)
	if err != nil {
		return nil, nil, err
	}
	manifest := NativeTables()
	if !deferPending {
		return manifest, []string{}, nil
	}
	tables = make([]string, 0, len(manifest)-1)
	deferred = []string{pendingSetupsTable}
	for _, table := range manifest {
		if table != pendingSetupsTable {
			tables = append(tables, table)
		}
	}
	return tables, deferred, nil
}

func migrationRecorded(db *gorm.DB, id string) (bool, error) {
	var count int64
	if err := db.Table("migrations").Where("id = ?", id).Count(&count).Error; err != nil {
		return false, fmt.Errorf("inspect migration ledger for %s: %w", id, err)
	}
	return count > 0, nil
}

func publicRelationExists(db *gorm.DB, table string) (bool, error) {
	var count int64
	if err := db.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=?`, table).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("inspect relation %s: %w", table, err)
	}
	return count > 0, nil
}

func resolveNativeInventory(db *gorm.DB) (tables, deferred []string, err error) {
	markerApplied, err := migrationRecorded(db, pendingSetupsMigration)
	if err != nil {
		return nil, nil, err
	}
	relationExists, err := publicRelationExists(db, pendingSetupsTable)
	if err != nil {
		return nil, nil, err
	}
	return nativeInventory(markerApplied, relationExists)
}

func readNativeTables(db *gorm.DB, data snapshot, bytesRead *int) ([]TableProof, []string, error) {
	tables, deferred, err := resolveNativeInventory(db)
	if err != nil {
		return nil, nil, err
	}
	proofs, err := readTables(db, tables, data, bytesRead)
	if err != nil {
		return nil, nil, err
	}
	return proofs, deferred, nil
}

// nativeTablesAtCheckpoint preserves the closed inventory while accounting for
// the one native table introduced after retirement. Absence is valid only when
// its creating migration has not run; every other missing table still fails.
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
