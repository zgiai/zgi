package socialops

import (
	"github.com/google/uuid"
	"time"
)

// AccountConnection is credential-free setup evidence, never user-authored
// proof of ownership. Execution rechecks the platform before writing.
type AccountOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type AccountConnection struct {
	ConnectorID  string          `json:"connector_id"`
	ConnectionID uuid.UUID       `json:"connection_id"`
	Accounts     []AccountOption `json:"accounts"`
	VerifiedAt   time.Time       `json:"verified_at"`
}
