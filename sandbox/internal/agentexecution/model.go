// Package agentexecution owns the control-plane contract for stateful Agent
// execution environments. It deliberately does not depend on a concrete
// sandbox vendor or transport.
package agentexecution

import (
	"context"
	"errors"
	"io"
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
	ErrJobClaimLost        = errors.New("agent execution job claim lost")
	ErrOutputLimit         = errors.New("agent execution output limit exceeded")
	ErrInvalidPath         = errors.New("invalid agent execution workspace path")
)

type BindingStatus string

const (
	BindingStatusAllocating BindingStatus = "allocating"
	BindingStatusActive     BindingStatus = "active"
	BindingStatusSuspending BindingStatus = "suspending"
	BindingStatusSuspended  BindingStatus = "suspended"
	BindingStatusResuming   BindingStatus = "resuming"
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

type JobStatus string

const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCanceled  JobStatus = "canceled"
)

type Job struct {
	ID              string
	BindingID       string
	OrganizationID  string
	SubjectID       string
	Backend         string
	BackendRef      string
	WorkspaceRef    string
	IdempotencyKey  string
	RequestHash     string
	Status          JobStatus
	Generation      int64
	Command         string
	Args            []string
	Environment     map[string]string
	WorkDir         string
	PTY             bool
	PTYColumns      uint16
	PTYRows         uint16
	Timeout         time.Duration
	ExitCode        *int
	FailureReason   string
	CancelRequested bool
	ClaimOwner      string
	ClaimExpiresAt  *time.Time
	Attempts        int
	StdoutBytes     int64
	StderrBytes     int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
	StartedAt       *time.Time
	FinishedAt      *time.Time
}

type OutputStream string

const (
	OutputStreamStdout OutputStream = "stdout"
	OutputStreamStderr OutputStream = "stderr"
)

type OutputChunk struct {
	JobID     string       `json:"job_id,omitempty"`
	Sequence  int64        `json:"sequence"`
	Stream    OutputStream `json:"stream"`
	Data      []byte       `json:"data"`
	CreatedAt time.Time    `json:"created_at"`
}

type JobSpec struct {
	JobID       string
	Command     string
	Args        []string
	Environment map[string]string
	WorkDir     string
	PTY         bool
	PTYColumns  uint16
	PTYRows     uint16
	Timeout     time.Duration
}

type JobResult struct {
	ExitCode int    `json:"exit_code"`
	Canceled bool   `json:"canceled"`
	Error    string `json:"error,omitempty"`
}

type RuntimeCapabilities struct {
	Shell            bool   `json:"shell"`
	PTY              bool   `json:"pty"`
	FileTransfer     bool   `json:"file_transfer"`
	NetworkMode      string `json:"network_mode"`
	MaxUploadBytes   int64  `json:"max_upload_bytes"`
	MaxDownloadBytes int64  `json:"max_download_bytes"`
}

type FileInfo struct {
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	Mode        uint32    `json:"mode"`
	ModifiedAt  time.Time `json:"modified_at"`
	ContentType string    `json:"content_type"`
}

type RunningJob interface {
	Wait(context.Context) (JobResult, error)
	Cancel(context.Context) error
	ResizePTY(context.Context, uint16, uint16) error
}

// ExecutionBackend is the data-plane contract implemented by a runtime
// worker. StartJob must be idempotent for a given JobID because an expired
// worker claim can be recovered after a control-plane crash. Output callbacks
// must use stable, contiguous sequences beginning at zero and be delivered in
// sequence order; retries may replay a sequence with identical content.
type ExecutionBackend interface {
	Capabilities(context.Context, Allocation) (RuntimeCapabilities, error)
	StartJob(context.Context, Allocation, JobSpec, func(OutputChunk) error) (RunningJob, error)
	UploadFile(context.Context, Allocation, string, io.Reader, int64) (FileInfo, error)
	DownloadFile(context.Context, Allocation, string) (io.ReadCloser, FileInfo, error)
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
// SuspendBinding, ResumeBinding, and DestroyBinding must be idempotent because
// reconciliation can repeat them after a process crashes between the backend
// call and the generation-fenced database update. ResumeBinding may return new
// opaque handles when a provider recreates compute around retained storage.
type Backend interface {
	CreateBinding(context.Context, CreateSpec) (Allocation, error)
	SuspendBinding(context.Context, Allocation) error
	ResumeBinding(context.Context, CreateSpec, Allocation) (Allocation, error)
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

type BindingReader interface {
	Binding(context.Context, string) (Binding, error)
}

type JobRepository interface {
	CreateJob(context.Context, Job) (job Job, created bool, err error)
	Job(context.Context, string) (Job, error)
	ClaimJob(context.Context, string, time.Time, time.Time) (job Job, claimed bool, err error)
	RenewJobClaim(context.Context, string, string, time.Time, time.Time) error
	AppendJobOutput(context.Context, string, OutputChunk, int64, time.Time) (OutputChunk, error)
	JobCancellationRequested(context.Context, string) (bool, error)
	CompleteJob(context.Context, string, string, JobResult, time.Time) (Job, error)
	RequestJobCancellation(context.Context, string, int64, time.Time) (Job, error)
	JobOutput(context.Context, string, int64, int) ([]OutputChunk, error)
}
