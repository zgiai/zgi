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
	ErrCapacityExceeded    = errors.New("agent runtime capacity exceeded")
	ErrAdmissionPolicy     = errors.New("agent runtime admission policy changed")
	ErrLeaseHeld           = errors.New("agent runtime lease is held by another owner")
	ErrCleanupClaimLost    = errors.New("agent runtime cleanup claim lost")
	ErrJobClaimLost        = errors.New("agent execution job claim lost")
	// ErrJobStartUnknown reports a job start whose request may have reached
	// the backend although no answer came back; the job may be running.
	ErrJobStartUnknown = errors.New("agent execution job start outcome unknown")
	ErrOutputLimit     = errors.New("agent execution output limit exceeded")
	ErrInvalidPath     = errors.New("invalid agent execution workspace path")
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
	Identity       *ExecutionIdentity
}

type Binding struct {
	ID                   string
	OrganizationID       string
	SubjectID            string
	Identity             *ExecutionIdentity `json:",omitempty"`
	AgentID              string
	ConversationID       string
	ProfileID            string
	NetworkPolicy        string
	NetworkMode          string
	HomeSnapshotID       string
	HomeSnapshotRef      string
	WorkspaceID          string
	Backend              string
	BackendRef           string
	WorkspaceRef         string
	CheckpointKey        string
	PendingCheckpointKey string
	IdempotencyKey       string
	RequestHash          string
	Status               BindingStatus
	Generation           int64
	FailureReason        string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	LastActivityAt       time.Time
	ExpiresAt            time.Time
	RetentionExpiresAt   time.Time
}

type CleanupTask struct {
	ID                string
	ResourceType      string
	ResourceID        string
	BindingID         string
	BindingGeneration int64
	Backend           string
	BackendRef        string
	WorkspaceRef      string
	Action            string
	Status            string
	Attempts          int
	LastError         string
	NotBefore         time.Time
	LeaseOwner        string
	LeaseExpiresAt    *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
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
	ID                string
	BindingID         string
	BindingGeneration int64
	OrganizationID    string
	SubjectID         string
	Backend           string
	BackendRef        string
	WorkspaceRef      string
	IdempotencyKey    string
	RequestHash       string
	Status            JobStatus
	Generation        int64
	Command           string
	Args              []string
	Environment       map[string]string
	WorkDir           string
	PTY               bool
	PTYColumns        uint16
	PTYRows           uint16
	Timeout           time.Duration
	ExitCode          *int
	FailureReason     string
	CancelRequested   bool
	ClaimOwner        string
	ClaimExpiresAt    *time.Time
	Attempts          int
	StdoutBytes       int64
	StderrBytes       int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
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
	// TerminationFailed requires quarantining compute, not deleting workspace files.
	TerminationFailed bool `json:"termination_failed,omitempty"`
}

// SystemPackageLedgerVersion is the fixed-version system package recovery contract.
const SystemPackageLedgerVersion = 2

// LanguagePackageLedgerVersion negotiates retained metadata and offline artifacts.
const LanguagePackageLedgerVersion = 2

type RuntimeCapabilities struct {
	PlatformProtocolVersion      int    `json:"platform_protocol_version,omitempty"`
	SystemPackageLedgerVersion   int    `json:"system_package_ledger_version,omitempty"`
	LanguagePackageLedgerVersion int    `json:"language_package_ledger_version,omitempty"`
	PackageSourceDigest          string `json:"package_source_digest,omitempty"`
	Shell                        bool   `json:"shell"`
	PTY                          bool   `json:"pty"`
	FileTransfer                 bool   `json:"file_transfer"`
	// DependencyPreparation permits user-space preparation. The approved mode
	// determines whether sources are staged offline or fetched through a
	// controlled HTTPS proxy; it never permits system package installation.
	DependencyPreparation bool `json:"dependency_preparation"`
	// DependencyPreparationMode identifies the approved source policy. An empty
	// value is retained for older workers and is interpreted as offline-bundled
	// when DependencyPreparation is true.
	DependencyPreparationMode string `json:"dependency_preparation_mode,omitempty"`
	// DependencyInstallation permits standard Skills to install user-space
	// Python/Node packages through the runtime's package-only network policy.
	// It is intentionally distinct from the legacy declaration-based
	// DependencyPreparation capability.
	DependencyInstallation bool   `json:"dependency_installation"`
	PackageNetworkMode     string `json:"package_network_mode,omitempty"`
	// PackageNetworkEnforced means the outer runtime provider enforces the
	// package-only egress boundary. Local gVisor reports this through its
	// worker bridge; hosted providers may enforce it with their own firewall.
	// It is separate from EgressProxyAddress because a provider-enforced direct
	// allowlist does not expose a proxy inside the sandbox.
	PackageNetworkEnforced bool   `json:"package_network_enforced,omitempty"`
	NetworkMode            string `json:"network_mode"`
	// EgressProxyAddress is a loopback-only proxy inside the isolated runtime.
	// It is empty when the runtime cannot provide controlled HTTPS egress.
	EgressProxyAddress string `json:"egress_proxy_address,omitempty"`
	MaxUploadBytes     int64  `json:"max_upload_bytes"`
	MaxDownloadBytes   int64  `json:"max_download_bytes"`
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

// WorkspaceArchiver is an internal, bounded transfer contract used to move a
// quiescent workspace between an execution environment and durable storage.
// It is deliberately separate from the user-facing file transfer API.
type WorkspaceArchiver interface {
	ExportWorkspace(context.Context, Allocation) (io.ReadCloser, error)
	ImportWorkspace(context.Context, Allocation, io.Reader) error
}

// WorkspaceCheckpointStore publishes immutable archives only after the full
// stream is durable. Open must verify the bytes while they are read.
type WorkspaceCheckpointStore interface {
	Save(context.Context, string, io.Reader) error
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}

type ActiveJobReader interface {
	HasActiveJobs(context.Context, string) (bool, error)
}

// Allocation contains only opaque backend handles. Product ownership and
// authorization remain in Binding and are never delegated to a backend.
type Allocation struct {
	BindingRef   string
	WorkspaceRef string
}

type CreateSpec struct {
	BindingID           string
	AllocationAttemptID string
	WorkspaceID         string
	OrganizationID      string
	SubjectID           string
	ProfileID           string
	NetworkPolicy       string
	NetworkMode         string
	HomeSnapshotID      string
	HomeSnapshotRef     string
}

// HomeSnapshot identifies an immutable, operator-approved base environment.
// Its backend reference is a prebuilt image or template, never a user session.
type HomeSnapshot struct {
	ID             string
	OrganizationID string
	SubjectID      string
	ProfileID      string
	Backend        string
	BackendRef     string
	Status         string
}

type HomeSnapshotReader interface {
	HomeSnapshot(context.Context, string) (HomeSnapshot, error)
}

// AllocationDiscoverer lets a backend locate compute created before the
// control-plane database could record the provider's opaque allocation ID.
// Discovery must include paused allocations and may return multiple matches.
type AllocationDiscoverer interface {
	FindAllocations(context.Context, string) ([]Allocation, error)
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
	BindingByIdempotency(context.Context, Scope, string) (Binding, error)
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
	TouchBinding(context.Context, string, int64, time.Time, time.Time, time.Time) (Binding, error)
}

type ExpiredBindingRepository interface {
	// Exclude suspending Bindings updated within IdleSuspendRetryDelay before
	// applying limit. UpdatedAt records the generation-fenced suspension attempt.
	ExpiredBindings(context.Context, time.Time, int) ([]Binding, error)
}

type RetainedBindingRepository interface {
	ExpiredRetainedBindings(context.Context, time.Time, int) ([]Binding, error)
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
