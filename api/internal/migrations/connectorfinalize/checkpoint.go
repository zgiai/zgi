package connectorfinalize

import (
	"errors"
	"fmt"
	"regexp"

	"gorm.io/gorm"
)

const DetachCheckpointID = "20260902190000_detach_connector_runtime"

const migrationLedgerQuery = `SELECT id::text FROM public.migrations ORDER BY id`

var migrationIDPattern = regexp.MustCompile(`^\d{14}(\d{2}|\d{4})?_[a-z0-9_]+$`)

var (
	errAdvancedEmptyLedger   = errors.New("migration ledger is empty; cannot verify the advanced checkpoint")
	errAdvancedIDShape       = errors.New("AdvancedCheckpoint requires an explicit migration ID")
	errAdvancedOrder         = errors.New("AdvancedCheckpoint must name a migration strictly later than " + CheckpointID)
	errAdvancedAlreadyDone   = errors.New("connector retirement is already applied; the advanced checkpoint cannot run")
	errAdvancedHistoryAbsent = errors.New("advanced checkpoint requires applied migration " + CheckpointID)
	errAdvancedHeadMismatch  = errors.New("advanced checkpoint does not match the applied migration head")
)

// CompatibleCheckpointMigration is an explicit exception for an unrelated
// migration already shipped on a legacy runtime. Unknown later IDs still fail
// closed; a name/prefix alone is not evidence of schema compatibility.
func CompatibleCheckpointMigration(id string) bool {
	switch id {
	case "20260903120000_add_chat_runtime_external_user", "20260903183000_add_video_runtime_task_poster_url":
		return true
	default:
		return false
	}
}

func migrationLedgerDigest(ledger []string) string {
	if ledger == nil {
		ledger = []string{}
	}
	return digest(ledger)
}

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

func preRetirementCheckpoint(ids []string) (string, error) {
	stages := []string{DetachCheckpointID, "20260903100000_create_connector_chat_preferences", "20260903110000_create_connector_onboarding", CheckpointID}
	allowed := map[string]bool{}
	for _, id := range stages {
		allowed[id] = true
	}
	applied := map[string]bool{}
	for _, id := range ids {
		if id == "" || applied[id] || (id > DetachCheckpointID && !allowed[id] && !CompatibleCheckpointMigration(id)) {
			return "", fmt.Errorf("unsupported pre-retirement migration history")
		}
		applied[id] = true
	}
	checkpoint := ""
	missing := false
	for _, id := range stages {
		if !applied[id] {
			missing = true
			continue
		}
		if missing {
			return "", fmt.Errorf("incomplete pre-retirement Connector migration history")
		}
		checkpoint = id
	}
	if checkpoint != CheckpointID {
		return "", fmt.Errorf("Connector finalization requires the messaging checkpoint")
	}
	return checkpoint, nil
}
