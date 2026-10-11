package connectorfinalize

import (
	"errors"
	"testing"
)

// advancedHeadID mirrors the schema1-ready installation this opt-in exists for:
// the applied head is later than CheckpointID and the finalizer marker is absent.
const advancedHeadID = "202609101110000000_add_digital_human_task_ownership"

const preCheckpointID = "20260902190000_detach_connector_runtime"

const postCheckpointID = "20260904160000_connector_pending_setups"

func TestAdvancedCheckpointValidationMatrix(t *testing.T) {
	correctLedger := []string{preCheckpointID, CheckpointID, postCheckpointID, advancedHeadID}
	for _, testCase := range []struct {
		name     string
		advanced string
		ledger   []string
		wantErr  error
	}{
		{
			name:     "empty_option_keeps_original_plan_behavior",
			advanced: "",
			ledger:   []string{preCheckpointID, CheckpointID, MigrationID, advancedHeadID},
		},
		{
			name:     "empty_option_accepts_empty_ledger",
			advanced: "",
			ledger:   []string{},
		},
		{
			name:     "correct_ledger_at_explicit_head",
			advanced: advancedHeadID,
			ledger:   correctLedger,
		},
		{
			name:     "correct_minimal_ledger",
			advanced: advancedHeadID,
			ledger:   []string{CheckpointID, advancedHeadID},
		},
		{
			name:     "wrong_head_is_rejected",
			advanced: advancedHeadID,
			ledger:   []string{preCheckpointID, CheckpointID, postCheckpointID},
			wantErr:  errAdvancedHeadMismatch,
		},
		{
			name:     "missing_historical_checkpoint_is_rejected",
			advanced: advancedHeadID,
			ledger:   []string{preCheckpointID, postCheckpointID, advancedHeadID},
			wantErr:  errAdvancedHistoryAbsent,
		},
		{
			name:     "applied_finalizer_marker_is_rejected",
			advanced: advancedHeadID,
			ledger:   []string{CheckpointID, MigrationID, advancedHeadID},
			wantErr:  errAdvancedAlreadyDone,
		},
		{
			name:     "illegal_id_shape_is_rejected",
			advanced: "2026_bad",
			ledger:   correctLedger,
			wantErr:  errAdvancedIDShape,
		},
		{
			name:     "unknown_name_shape_is_rejected",
			advanced: "202609101110000000_Add_Digital_Human",
			ledger:   correctLedger,
			wantErr:  errAdvancedIDShape,
		},
		{
			name:     "checkpoint_itself_is_not_later",
			advanced: CheckpointID,
			ledger:   correctLedger,
			wantErr:  errAdvancedOrder,
		},
		{
			name:     "earlier_migration_is_not_later",
			advanced: preCheckpointID,
			ledger:   correctLedger,
			wantErr:  errAdvancedOrder,
		},
		{
			name:     "empty_ledger_cannot_confirm_any_explicit_head",
			advanced: advancedHeadID,
			ledger:   []string{},
			wantErr:  errAdvancedEmptyLedger,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateAdvancedCheckpoint(testCase.advanced, testCase.ledger)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("validateAdvancedCheckpoint(%q) = %v, want %v", testCase.advanced, err, testCase.wantErr)
			}
		})
	}
}

// The sealed report must commit the whole applied history, not only the head, so
// a rewritten or backfilled ledger cannot reuse a reviewed plan digest even when
// the head is unchanged.
func TestReportDigestCommitsCompleteMigrationLedger(t *testing.T) {
	reviewed := Report{Version: ReportVersion, Checkpoint: advancedHeadID, MigrationLedgerDigest: migrationLedgerDigest([]string{CheckpointID, advancedHeadID})}
	rewritten := Report{Version: ReportVersion, Checkpoint: advancedHeadID, MigrationLedgerDigest: migrationLedgerDigest([]string{preCheckpointID, CheckpointID, advancedHeadID})}
	reviewed.seal()
	rewritten.seal()
	if reviewed.Checkpoint != rewritten.Checkpoint {
		t.Fatalf("head changed between reports: %q vs %q", reviewed.Checkpoint, rewritten.Checkpoint)
	}
	if reviewed.MigrationLedgerDigest == rewritten.MigrationLedgerDigest {
		t.Fatal("ledger digest ignored a historical entry")
	}
	if reviewed.Digest == rewritten.Digest {
		t.Fatal("sealed report digest does not commit the migration ledger digest")
	}
	unsealedLedger := Report{Version: ReportVersion, Checkpoint: advancedHeadID}
	unsealedLedger.seal()
	if unsealedLedger.Digest == reviewed.Digest {
		t.Fatal("sealed report digest ignores the migration ledger field")
	}
}

func TestMigrationLedgerDigestNormalizesEmptyHistory(t *testing.T) {
	if migrationLedgerDigest(nil) != migrationLedgerDigest([]string{}) {
		t.Fatal("nil and empty migration ledgers must commit identically")
	}
	if migrationLedgerDigest([]string{CheckpointID}) == migrationLedgerDigest([]string{CheckpointID, advancedHeadID}) {
		t.Fatal("ledger digest does not change with the applied history")
	}
}
