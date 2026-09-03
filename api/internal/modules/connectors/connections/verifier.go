package connections

import (
	"context"

	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

var ErrCandidateContract = connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)

// CandidateVerificationRequest contains request-scoped secret material. It is
// never serializable or persistable; the lifecycle service must destroy the
// credential map after verification and candidate encryption finish.
type CandidateVerificationRequest struct {
	OrganizationID uuid.UUID
	OwnerAccountID uuid.UUID
	ConnectionID   uuid.UUID
	ConnectorID    string
	DriverID       string
	AuthMethodID   string
	Credentials    map[string]string `json:"-"`
	OperationIDs   []string
}

// CandidateVerificationResult is the non-secret evidence returned by one
// bounded provider verification.
type CandidateVerificationResult struct {
	ExternalAccountID   string
	ExternalDisplayName string
	UsableOperationIDs  []string
	ProviderRequestID   string
	CostUSD             *float64
	Evidence            *connectorcore.ProviderEvidence
}

type CandidateVerifier interface {
	Verify(context.Context, CandidateVerificationRequest) (CandidateVerificationResult, error)
}
