package connectorfinalize

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type ApplyOptions struct {
	Options
	ExpectedReport string
	Archive        []byte
	ArchiveKey     []byte
}

func requiresArchive(data snapshot) bool {
	for table, rows := range data {
		if table != "connector_schema_readiness" && table != "connector_definitions" && table != "connector_definition_versions" && table != "connector_plugin_versions" && len(rows) > 0 {
			return true
		}
	}
	return false
}

// Apply commits the marker in the same transaction as conversion and DDL.
// A repeat checks the complete schema2 shape and proofs without adding records.
func Apply(ctx context.Context, db *gorm.DB, options ApplyOptions) (Report, error) {
	var report Report
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var version int
		if err := tx.Table("connector_schema_readiness").Select("schema_version").Where("id=1").Scan(&version).Error; err != nil {
			return err
		}
		if version == 2 {
			var err error
			report, err = verify(tx, options.Options, true)
			return err
		}
		if err := lockInventory(tx); err != nil {
			return err
		}
		r, data, err := inspect(tx, options.Options)
		if err != nil {
			return err
		}
		if r.Checkpoint != CheckpointID || !sha256Digest(options.ExpectedReport) || r.Digest != options.ExpectedReport {
			return fmt.Errorf("reviewed report or supported checkpoint changed")
		}
		if len(r.Issues) > 0 {
			return fmt.Errorf("retirement proof has %d unresolved issues", len(r.Issues))
		}
		if requiresArchive(data) {
			if err := verifyArchive(options.Archive, options.ArchiveKey, r, data); err != nil {
				return err
			}
		}
		if err := applySnapshot(tx, r, data, options.Archive); err != nil {
			return err
		}
		if err := tx.Table("migrations").Create(map[string]any{"id": MigrationID}).Error; err != nil {
			return fmt.Errorf("record retirement migration: %w", err)
		}
		report, err = verify(tx, options.Options, true)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	return report, err
}

// FinalizeEmpty is only called by the registered migration inside its outer
// transaction. Every source and host row must be empty; existing installations
// must use Plan/CreateArchive/Apply. gormigrate writes its marker before commit.
func FinalizeEmpty(tx *gorm.DB) error {
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return fmt.Errorf("retirement migration requires an outer transaction")
	}
	if err := lockInventory(tx); err != nil {
		return err
	}
	r, data, err := inspect(tx, Options{})
	if err != nil {
		return err
	}
	if r.Checkpoint != CheckpointID || len(r.Issues) > 0 || requiresArchive(data) || r.HostTransitions > 0 {
		return fmt.Errorf("Connector maintenance required: run connector-finalize plan, archive, apply and verify before restarting")
	}
	if err := applySnapshot(tx, r, data, nil); err != nil {
		return err
	}
	_, err = verify(tx, Options{}, false)
	return err
}

func applySnapshot(tx *gorm.DB, report Report, data snapshot, archive []byte) error {
	catalog, err := catalogChanges(data)
	if err != nil {
		return err
	}
	hosts, err := hostChanges(data)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := applyCatalogChanges(tx, catalog, now); err != nil {
		return err
	}
	if err := applyHostChanges(tx, hosts); err != nil {
		return err
	}
	for _, source := range data["connector_execution_route_claims"] {
		claim := find(data["connector_invocation_claims"], "receipt_id", text(source, "receipt_id"))
		if !historicalClaim(data, claim) {
			continue
		}
		if err := insertProof(tx, "retirement_claim_v2", text(claim, "receipt_id"), digest(claim), "digest_unavailable"); err != nil {
			return err
		}
	}
	if len(archive) > 0 {
		if err := insertProof(tx, "retirement_archive_v2", MigrationID, archiveDigest(archive), "sealed"); err != nil {
			return err
		}
	}
	for _, table := range RetiredTables() {
		for _, source := range data[table] {
			id := text(source, "id")
			if id == "" {
				id = text(source, "receipt_id")
			}
			if id == "" {
				id = digest(source)
			}
			if err := insertProof(tx, "retired_source_v2", digest([]string{table, id}), digest(source), "archived"); err != nil {
				return err
			}
		}
	}
	for _, table := range report.Source {
		if err := insertProof(tx, "retired_table_v2", table.Table, table.Digest, fmt.Sprintf("rows:%d", table.Rows)); err != nil {
			return err
		}
	}
	if err := retirementDDL(tx); err != nil {
		return err
	}
	// Compare immutable native material, including exact ciphertext bytes,
	// target restrictions, claims, pending confirmations and provider state.
	after := snapshot{}
	size := 0
	native, err := nativeTablesAtCheckpoint(tx)
	if err != nil {
		return err
	}
	if _, err := readTables(tx, native, after, &size); err != nil {
		return err
	}
	for _, table := range native {
		switch table {
		case "connector_definitions", "connector_definition_versions", "connector_plugin_versions", "connector_schema_readiness", "connector_conversion_records":
			continue
		}
		beforeRows := data[table]
		if table == "connector_target_bindings" {
			beforeRows = make([]row, 0, len(data[table]))
			for _, source := range data[table] {
				copy := object(cloneJSON(source))
				delete(copy, "compatibility")
				delete(copy, "compatibility_state")
				beforeRows = append(beforeRows, copy)
			}
		}
		if unorderedDigest(beforeRows) != unorderedDigest(after[table]) {
			return fmt.Errorf("retirement changed protected native data in %s", table)
		}
	}
	if err := insertProof(tx, "retirement_v2", MigrationID, report.Digest, "finalized"); err != nil {
		return err
	}
	updated := tx.Table("connector_schema_readiness").Where("id=1 AND schema_version=1 AND state='ready'").Updates(map[string]any{"schema_version": 2, "report_digest": report.Digest, "verified_at": now})
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		return fmt.Errorf("schema readiness changed")
	}
	return nil
}

func insertProof(tx *gorm.DB, kind, id, commitment, result string) error {
	if err := tx.Table("connector_conversion_records").Create(map[string]any{"source_kind": kind, "source_id": id, "source_digest": commitment, "result": result}).Error; err != nil {
		return fmt.Errorf("record retirement proof: %w", err)
	}
	return nil
}
