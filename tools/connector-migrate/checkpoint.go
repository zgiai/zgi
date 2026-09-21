package main

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

// The default historical path accepts exactly one migration head. The advanced
// path is an explicit opt-in for a schema1 database that already advanced past
// that head but has not run the API-owned retirement finalizer.
const (
	historicalCheckpointID = "20260902190000_detach_connector_runtime"
	finalizeMigrationID    = "202609031410353961_finalize_connector_detachment"
)

// migrationHistoryKey is a reserved snapshot key, not a database table. The
// complete ordered migration ledger is read in the same transaction snapshot as
// the conversion source and therefore contributes to the source digest.
const migrationHistoryKey = "connector_migration_history"

// migrationHistoryQuery reads IDs, never object names, and is parameter-free.
const migrationHistoryQuery = `SELECT coalesce(array_agg(id::text ORDER BY id), '{}'::text[]) FROM public.migrations`

// migrationLedgerLock serializes advanced apply against concurrent migration
// marker inserts. It is the first statement in the advanced apply transaction,
// before any SELECT and before the conversion advisory lock, so the complete
// migration ledger read into the source digest cannot change while apply holds
// the lock. Historical apply keeps its existing lock order and never runs this.
const migrationLedgerLock = "LOCK TABLE public.migrations IN SHARE ROW EXCLUSIVE MODE"

var migrationIDPattern = regexp.MustCompile(`^\d{14}(\d{2}|\d{4})?_[a-z0-9_]+$`)

type checkpointChoice struct {
	advanced bool
	id       string
}

var (
	errHistoricalSchema = errors.New("historical converter requires schema1; use the native connector-finalize command after retirement")
	errHistoricalHead   = errors.New("historical converter requires checkpoint " + historicalCheckpointID + "; ready current installations use connector-finalize")
	errEmptyLedger      = errors.New("migration ledger is empty; apply the native Connector schema migration before conversion")
	errAdvancedValue    = errors.New("--advanced-checkpoint requires an explicit migration ID")
	errAdvancedOrder    = errors.New("--advanced-checkpoint must name a migration strictly later than " + historicalCheckpointID)
	errAlreadyFinalized = errors.New("connector retirement is already applied; the historical converter cannot run")
	errHistoricalAbsent = errors.New("advanced checkpoint requires applied migration " + historicalCheckpointID)
	errAdvancedHead     = errors.New("advanced checkpoint does not match the applied migration head; nothing was applied")
)

// validate re-evaluates the complete checkpoint condition against a schema
// version and an ordered migration ledger. It is called before the transaction
// on the live connection and again inside the transaction snapshot, so apply
// rejects a history that changed while it waited for locks.
func (choice checkpointChoice) validate(schemaVersion int, history []string) error {
	if schemaVersion != 1 {
		return errHistoricalSchema
	}
	if len(history) == 0 {
		return errEmptyLedger
	}
	if !choice.advanced {
		if history[len(history)-1] != historicalCheckpointID {
			return errHistoricalHead
		}
		return nil
	}
	if strings.TrimSpace(choice.id) == "" || !migrationIDPattern.MatchString(choice.id) {
		return errAdvancedValue
	}
	if choice.id <= historicalCheckpointID {
		return errAdvancedOrder
	}
	for _, id := range history {
		if id == finalizeMigrationID {
			return errAlreadyFinalized
		}
	}
	if !containsID(history, historicalCheckpointID) {
		return errHistoricalAbsent
	}
	if history[len(history)-1] != choice.id {
		return errAdvancedHead
	}
	return nil
}

func containsID(history []string, id string) bool {
	for _, value := range history {
		if value == id {
			return true
		}
	}
	return false
}

// checkpointReader is the narrow read surface shared by *pgx.Conn and pgx.Tx.
type checkpointReader interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// readCheckpointState fails fast before the transaction. A missing readiness
// row or table is reported by validate as a schema mismatch so operators keep
// the historical guidance; the authoritative check runs again in the snapshot.
func readCheckpointState(ctx context.Context, reader checkpointReader) (int, []string, error) {
	var schemaVersion int
	_ = reader.QueryRow(ctx, "SELECT schema_version FROM public.connector_schema_readiness WHERE id=1").Scan(&schemaVersion)
	var history []string
	if err := reader.QueryRow(ctx, migrationHistoryQuery).Scan(&history); err != nil {
		return 0, nil, errors.New("cannot read the applied migration ledger")
	}
	return schemaVersion, history, nil
}

func snapshotSchemaVersion(snapshot databaseSnapshot) int {
	if len(snapshot["connector_schema_readiness"]) != 1 {
		return 0
	}
	return integer(snapshot["connector_schema_readiness"][0], "schema_version")
}

func migrationHistory(snapshot databaseSnapshot) []string {
	ids := make([]string, 0, len(snapshot[migrationHistoryKey]))
	for _, record := range snapshot[migrationHistoryKey] {
		if id := text(record, "id"); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
