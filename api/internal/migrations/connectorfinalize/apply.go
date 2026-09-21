package connectorfinalize

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
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

// migrationLedgerLock freezes the complete applied migration ledger for the rest
// of the transaction. It is a closed statement: the relation is named literally
// here and no caller-supplied table name or SQL fragment is ever accepted.
// SHARE ROW EXCLUSIVE conflicts with every ledger INSERT/UPDATE/DELETE and with
// another holder, so NOWAIT fails closed instead of validating a checkpoint
// against a history a concurrent migration is still rewriting.
const migrationLedgerLock = `LOCK TABLE public.migrations IN SHARE ROW EXCLUSIVE MODE NOWAIT`

// errApplyReviewChanged is the original review rejection. It is returned for an
// invalid or mismatched reviewed report digest and for an applied head that does
// not match the checkpoint this Apply was authorized for.
var errApplyReviewChanged = errors.New("reviewed report or supported checkpoint changed")

func lockMigrationLedger(tx *gorm.DB) error {
	if err := tx.Exec(migrationLedgerLock).Error; err != nil {
		return fmt.Errorf("freeze the applied migration ledger before finalization: %w", err)
	}
	return nil
}

// requiresAdvancedLedgerLock reports whether this Apply is the explicit
// AdvancedCheckpoint opt-in that must freeze the applied migration ledger before
// it reads readiness. The default schema1 conversion and the default schema2
// repeat keep their original read/lock order and never take this lock.
func requiresAdvancedLedgerLock(option Options) bool {
	return option.AdvancedCheckpoint != ""
}

// requiredCheckpoint is the applied migration head Apply must observe. An empty
// AdvancedCheckpoint keeps the original strict CheckpointID contract; an
// explicit opt-in must match the exact reviewed migration ID.
func requiredCheckpoint(option Options) string {
	if option.AdvancedCheckpoint != "" {
		return option.AdvancedCheckpoint
	}
	return CheckpointID
}

// validateApplyReview re-checks the reviewed plan digest and the observed
// applied head before an archive, DDL or marker is written. ExpectedReport must
// be a legal SHA256 commitment to the freshly inspected plan digest.
func validateApplyReview(option Options, report Report, expectedReport string) error {
	if !sha256Digest(expectedReport) || report.Digest != expectedReport {
		return errApplyReviewChanged
	}
	if report.Checkpoint != requiredCheckpoint(option) {
		return errApplyReviewChanged
	}
	return nil
}

// validateApplyEntry rejects the explicit advanced checkpoint once the database
// already reports schema2. The default repeat path keeps calling verify.
func validateApplyEntry(option Options, schemaVersion int) error {
	if schemaVersion == 2 && option.AdvancedCheckpoint != "" {
		return errAdvancedAlreadyDone
	}
	return nil
}

// Apply commits the marker in the same transaction as conversion and DDL.
// A repeat checks the complete schema2 shape and proofs without adding records.
// An explicit AdvancedCheckpoint opts a schema1 database whose applied head has
// advanced past CheckpointID into the same conversion, but only after inspect
// re-validates the complete ordered ledger and the observed head equals that
// exact migration ID.
func Apply(ctx context.Context, db *gorm.DB, options ApplyOptions) (Report, error) {
	var report Report
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Only the explicit AdvancedCheckpoint opt-in freezes the applied
		// migration ledger, and that lock must be the first statement of the
		// transaction, before any SELECT, so readiness, inventory and the
		// advanced head are all read from one frozen migration generation. A
		// failed lock fails closed. Both default paths keep their original
		// read/lock order.
		if requiresAdvancedLedgerLock(options.Options) {
			if err := lockMigrationLedger(tx); err != nil {
				return err
			}
		}
		var version int
		if err := tx.Table("connector_schema_readiness").Select("schema_version").Where("id=1").Scan(&version).Error; err != nil {
			return err
		}
		if err := validateApplyEntry(options.Options, version); err != nil {
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
		if err := validateApplyReview(options.Options, r, options.ExpectedReport); err != nil {
			return err
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
	if err := applyChannelScopeFunctionTransition(tx); err != nil {
		return err
	}
	if err := retirementDDL(tx); err != nil {
		return err
	}
	// Compare immutable native material, including exact ciphertext bytes,
	// target restrictions, claims, pending confirmations and provider state.
	after := snapshot{}
	size := 0
	_, afterDeferred, err := readNativeTables(tx, after, &size)
	if err != nil {
		return err
	}
	if !slices.Equal(afterDeferred, report.DeferredNativeTables) {
		return fmt.Errorf("native deferred inventory changed during retirement")
	}
	deferred := make(map[string]bool, len(afterDeferred))
	for _, table := range afterDeferred {
		deferred[table] = true
	}
	for _, table := range NativeTables() {
		if deferred[table] {
			continue
		}
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
