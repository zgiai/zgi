// Package agentexecution owns the control-plane contract for stateful Agent
// execution environments. It deliberately does not depend on a concrete
// sandbox vendor or transport.
package agentexecution

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound            = errors.New("agent runtime resource not found")
	ErrInvalidRequest      = errors.New("invalid agent runtime request")
	ErrIdempotencyConflict = errors.New("agent runtime idempotency key conflict")
	ErrGenerationConflict  = errors.New("agent runtime generation conflict")
	ErrInvalidTransition   = errors.New("invalid agent runtime state transition")
	ErrBackendUnavailable  = errors.New("agent runtime backend unavailable")
	ErrLeaseHeld           = errors.New("agent runtime lease is held by another owner")
	ErrCleanupClaimLost    = errors.New("agent runtime cleanup claim lost")
)

type BindingStatus string

const (
	BindingStatusAllocating BindingStatus = "allocating"
	BindingStatusActive     BindingStatus = "active"
	BindingStatusSuspending BindingStatus = "suspending"
	BindingStatusSuspended  BindingStatus = "suspended"
	BindingStatusDeleting   BindingStatus = "deleting"
	BindingStatusDeleted    BindingStatus = "deleted"
	BindingStatusFailed     BindingStatus = "failed"
)

type Scope struct {
	OrganizationID string
	SubjectID      string
}

type Binding struct {
	ID             string
	OrganizationID string
	SubjectID      string
	AgentID        string
	ConversationID string
	ProfileID      string
	HomeSnapshotID string
	WorkspaceID    string
	Backend        string
	BackendRef     string
	WorkspaceRef   string
	IdempotencyKey string
	RequestHash    string
	Status         BindingStatus
	Generation     int64
	FailureReason  string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastActivityAt time.Time
	ExpiresAt      time.Time
}

type CleanupTask struct {
	ID             string
	ResourceType   string
	ResourceID     string
	Backend        string
	BackendRef     string
	Action         string
	Status         string
	Attempts       int
	LastError      string
	NotBefore      time.Time
	LeaseOwner     string
	LeaseExpiresAt *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type AdmissionLease struct {
	ID         string
	BindingID  string
	OwnerID    string
	Generation int64
	ExpiresAt  time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Allocation contains only opaque backend handles. Product ownership and
// authorization remain in Binding and are never delegated to a backend.
type Allocation struct {
	BindingRef   string
	WorkspaceRef string
}

type CreateSpec struct {
	BindingID      string
	WorkspaceID    string
	OrganizationID string
	SubjectID      string
	ProfileID      string
	HomeSnapshotID string
}

// Backend is intentionally narrow and is owned by the control-plane caller.
// Local isolation, hosted runtimes, and future VM implementations conform to
// this contract without leaking provider credentials into persisted records.
// SuspendBinding and DestroyBinding must be idempotent because reconciliation
// can repeat them after a process crashes between the backend call and the
// generation-fenced database update.
type Backend interface {
	CreateBinding(context.Context, CreateSpec) (Allocation, error)
	SuspendBinding(context.Context, Allocation) error
	DestroyBinding(context.Context, Allocation) error
}

// Repository provides atomic idempotency reservation and generation-fenced
// updates. Implementations must return ErrGenerationConflict when a compare-
// and-swap update loses its race.
type Repository interface {
	ReserveBinding(context.Context, Binding) (binding Binding, created bool, err error)
	Binding(context.Context, string) (Binding, error)
	UpdateBinding(context.Context, Binding, int64) error
	EnqueueCleanup(context.Context, CleanupTask) error
}

type CleanupRepository interface {
	Repository
	ClaimCleanup(context.Context, string, time.Time, time.Time) (CleanupTask, bool, error)
	CompleteCleanup(context.Context, string, string, bool, string, time.Time, time.Time) error
}

type LeaseRepository interface {
	AcquireLease(context.Context, AdmissionLease, time.Time) (AdmissionLease, error)
	RenewLease(context.Context, string, string, int64, time.Time, time.Time) (AdmissionLease, error)
	ReleaseLease(context.Context, string, string, int64) error
}
