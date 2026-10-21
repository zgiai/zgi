// Package providers defines the Connector-owned external protocol boundary.
package providers

import (
	"context"
	"errors"
	"github.com/google/uuid"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

type Verification struct {
	AccountID   string
	DisplayName string
	Operations  []string
	RequestID   string
	CostUSD     *float64
	Evidence    *core.ProviderEvidence
}
type Response struct {
	Value     map[string]any
	RequestID string
	CostUSD   *float64
}

// Driver identifies a platform-owned leaf implementation.
type Driver interface{ ConnectorID() string }

// KeyDriver receives only the operation, validated input and current request key.
// It cannot resolve user connections or take authorization decisions.
type KeyDriver interface {
	Driver
	Verify(context.Context, string) (Verification, error)
	Invoke(context.Context, string, string, map[string]any) (Response, error)
}

// CredentialInput never carries authorization or connection lookup powers.
// The lifecycle owns the disposable values and clears them after completion.
type CredentialInput struct {
	Values map[string]string `json:"-"`
	// Assigned by Runtime from the validated setup request, never tool arguments.
	MailUsage string `json:"-"`
	// Assigned by Runtime after native execution authorization, never from args.
	ResourceScope ResourceScope `json:"-"`
}

type ResourceScope struct {
	OrganizationID     uuid.UUID
	OwnerAccountID     uuid.UUID
	ConnectionID       uuid.UUID
	CredentialVersion  int
	BindingID          uuid.UUID
	BindingRevision    int
	LogicalOperationID string
	Target             core.TargetRef
}
type CredentialDriver interface {
	Driver
	VerifyCredentials(context.Context, CredentialInput) (Verification, error)
	InvokeCredentials(context.Context, string, CredentialInput, map[string]any) (Response, error)
}

type Failure struct {
	Code       core.ErrorCode
	Retryable  bool
	Unknown    bool
	RequestID  string
	Diagnostic *Diagnostic
}

// Diagnostic contains only bounded protocol facts. It never retains a URL,
// response body, credential, provider text, or a transport error string.
type Diagnostic = core.ProviderDiagnostic

func (failure *Failure) SafeDiagnostic() *core.ProviderDiagnostic {
	return failure.Diagnostic.SafeCopy()
}

type diagnosticPhaseKey struct{}

func WithDiagnosticPhase(ctx context.Context, phase string) context.Context {
	return context.WithValue(ctx, diagnosticPhaseKey{}, phase)
}

func (failure *Failure) Error() string { return string(failure.Code) }
func FailureOf(err error) *Failure {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure
	}
	return &Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: true}
}
func Invalid() error { return &Failure{Code: core.ErrorCodeInvalidInput} }
