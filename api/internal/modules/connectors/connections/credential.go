package connections

import "github.com/google/uuid"

// CredentialSealRequest is request-scoped and never serializable. Seal may
// produce an encrypted envelope before provider verification, but the service
// persists that envelope only after verification succeeds.
type CredentialSealRequest struct {
	OrganizationID    uuid.UUID
	ConnectionID      uuid.UUID
	ConnectorID       string
	CredentialVersion int
	Values            map[string]string `json:"-"`
}

type CredentialSealer interface {
	Seal(CredentialSealRequest) (string, error)
}
