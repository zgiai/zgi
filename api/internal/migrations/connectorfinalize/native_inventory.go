package connectorfinalize

import (
	"fmt"

	"gorm.io/gorm"
)

// pendingSetupsTable is the only native table introduced by a migration that
// sorts after MigrationID. The finalizer therefore runs before the relation
// exists, and a database that has not applied the creating migration yet is a
// legitimate checkpoint rather than a partial retirement.
const (
	pendingSetupsTable     = "connector_pending_setups"
	pendingSetupsMigration = "20260904160000_connector_pending_setups"
)

// pendingSetupsDisposition decides whether the one post-finalizer native table
// may be skipped. It defers only while the creating migration has not been
// applied and the relation does not exist. Every other marker/relation
// disagreement proves a partially applied migration and must fail closed; no
// arbitrary missing table is ever ignored.
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

// nativeInventory splits the fixed native manifest into the tables that must be
// read now and the tables explicitly deferred to a later migration. Both
// returned slices are non-nil so the sealed report digest is deterministic.
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
		if table == pendingSetupsTable {
			continue
		}
		tables = append(tables, table)
	}
	return tables, deferred, nil
}

// migrationRecorded reports whether an append-only migration marker is present.
// The migrations table is already a hard dependency of inspect and verify.
func migrationRecorded(db *gorm.DB, id string) (bool, error) {
	var count int64
	if err := db.Table("migrations").Where("id = ?", id).Count(&count).Error; err != nil {
		return false, fmt.Errorf("inspect migration ledger for %s: %w", id, err)
	}
	return count > 0, nil
}

// publicRelationExists reports whether a public relation is currently visible
// to the transaction snapshot.
func publicRelationExists(db *gorm.DB, table string) (bool, error) {
	var count int64
	if err := db.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=?`, table).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("inspect relation %s: %w", table, err)
	}
	return count > 0, nil
}

// resolveNativeInventory applies the fixed deferred rule to the current
// transaction snapshot. Plan, Apply's before/after comparison and Verify all
// use this one resolver, so their inventories cannot disagree.
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

// readNativeTables reads the resolved native inventory and reports which fixed
// tables were deferred.
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
