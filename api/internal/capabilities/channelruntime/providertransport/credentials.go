// Package providertransport contains the credential and diagnostic ports used
// by channel providers. It has no connection repository or action executor.
package providertransport

import "context"

// CredentialRequest is bound by the host to the current installation. Historic
// delivery connection locators must never be used as credential authority.
type CredentialRequest struct {
	OrganizationID string
	WorkspaceID    string
	InstallationID string
	ProviderID     string
	DriverID       string
}

type CredentialResolver interface {
	Resolve(context.Context, CredentialRequest) (*CredentialLease, error)
}

type CredentialLease struct {
	ID                string
	OrganizationID    string
	ProviderID        string
	DriverID          string
	AuthMethodID      string
	AccountID         string
	Credentials       map[string]string
	Config            map[string]any
	CredentialVersion int
	Revision          int
}

func (lease *CredentialLease) Destroy() {
	if lease == nil {
		return
	}
	for key := range lease.Credentials {
		lease.Credentials[key] = ""
		delete(lease.Credentials, key)
	}
	for key := range lease.Config {
		delete(lease.Config, key)
	}
}

type ContextResult struct {
	Output              map[string]interface{}
	ProviderRequestID   string
	ProviderDiagnostics ProviderDiagnostics
	ResultCount         int
	AttemptCount        int
}

// Historical error identities remain readable in persisted channel records.
const (
	ErrorCodeConnectionInvalid  = "integration_connection_invalid"
	ErrorCodeConnectionNotFound = "integration_connection_not_found"
)
