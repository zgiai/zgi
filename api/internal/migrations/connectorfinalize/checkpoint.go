package connectorfinalize

import (
	"errors"
	"fmt"
	"regexp"

	"gorm.io/gorm"
)

// migrationLedgerQuery reads the complete ordered applied-migration ledger. It
// selects IDs only, is parameter-free and never accepts a caller-supplied table
// or SQL fragment. The same constant is used by every caller.
const migrationLedgerQuery = `SELECT id::text FROM public.migrations ORDER BY id`

// migrationIDPattern matches the repository's timestamped migration ID shape:
// fourteen digits, an optional two- or four-digit extended suffix, then a
// lowercase name. tools/connector-migrate uses the same shape so both
// maintenance paths agree on what an explicit migration ID may look like.
var migrationIDPattern = regexp.MustCompile(`^\d{14}(\d{2}|\d{4})?_[a-z0-9_]+$`)

// The explicit opt-in fails closed on every disagreement between the requested
// ID and the applied ledger. Each condition keeps its own error so an operator
// can tell a typo from an unapplied history or an already-finished retirement.
var (
	errAdvancedEmptyLedger   = errors.New("migration ledger is empty; cannot verify the advanced checkpoint")
	errAdvancedIDShape       = errors.New("AdvancedCheckpoint requires an explicit migration ID")
	errAdvancedOrder         = errors.New("AdvancedCheckpoint must name a migration strictly later than " + CheckpointID)
	errAdvancedAlreadyDone   = errors.New("connector retirement is already applied; the advanced checkpoint cannot run")
	errAdvancedHistoryAbsent = errors.New("advanced checkpoint requires applied migration " + CheckpointID)
	errAdvancedHeadMismatch  = errors.New("advanced checkpoint does not match the applied migration head")
)

// migrationLedgerDigest commits the complete ordered ledger, not only its head,
// so a rewritten, removed or backfilled history cannot reuse a reviewed report
// digest. A nil and an empty ledger deliberately produce the same commitment.
func migrationLedgerDigest(ledger []string) string {
	if ledger == nil {
		ledger = []string{}
	}
	return digest(ledger)
}

// validateAdvancedCheckpoint re-evaluates the explicit opt-in against the full
// ordered ledger read in the same snapshot as the inventory. An empty option
// keeps the original Plan behavior: the ledger only supplies the head, and any
// head acceptance remains the caller's existing review decision.
func validateAdvancedCheckpoint(advanced string, ledger []string) error {
	if advanced == "" {
		return nil
	}
	if len(ledger) == 0 {
		return errAdvancedEmptyLedger
	}
	if !migrationIDPattern.MatchString(advanced) {
		return errAdvancedIDShape
	}
	if advanced <= CheckpointID {
		return errAdvancedOrder
	}
	if containsMigrationID(ledger, MigrationID) {
		return errAdvancedAlreadyDone
	}
	if !containsMigrationID(ledger, CheckpointID) {
		return errAdvancedHistoryAbsent
	}
	if ledger[len(ledger)-1] != advanced {
		return errAdvancedHeadMismatch
	}
	return nil
}

func containsMigrationID(ledger []string, id string) bool {
	for _, candidate := range ledger {
		if candidate == id {
			return true
		}
	}
	return false
}

// readMigrationLedger returns the complete ordered ID list. The caller decides
// whether an empty or surprising history is fatal; this function only reports
// read failures.
func readMigrationLedger(db *gorm.DB) ([]string, error) {
	rows, err := db.Raw(migrationLedgerQuery).Rows()
	if err != nil {
		return nil, fmt.Errorf("read applied migration ledger: %w", err)
	}
	defer rows.Close()
	ledger := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read applied migration ledger: %w", err)
		}
		ledger = append(ledger, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read applied migration ledger: %w", err)
	}
	return ledger, nil
}
