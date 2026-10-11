package consolesync

import (
	"strings"

	"github.com/google/uuid"
)

// parseUUIDOrZero converts a command id, returning the zero UUID when the console sent
// something malformed. The idempotency ledger still records the row because the console
// command id is the primary key of the ledger.
func parseUUIDOrZero(raw string) uuid.UUID {
	parsed, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil
	}
	return parsed
}

// isDuplicateKey reports whether the error is a unique-violation on the idempotency
// ledger. Both Postgres and SQLite phrase this differently, so match on the phrase.
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "unique violation")
}
