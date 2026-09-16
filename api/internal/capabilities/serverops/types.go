package serverops

import (
	"errors"
	"time"
)

// Read-only action identities. These are the capability's only action ids: a
// caller selects an action by one of these constants instead of passing or
// parsing a command string.
const (
	ActionSystemSnapshot = "server.system.snapshot"
	ActionServiceStatus  = "server.service.status"
	ActionJournalTail    = "server.journal.tail"

	// SystemdServiceNamePattern accepts only an explicit systemd service unit
	// name. It excludes whitespace, "/", shell metacharacters, and bare command
	// names, so a caller can never smuggle a command line or file path through
	// the service_name argument.
	SystemdServiceNamePattern = `^[A-Za-z0-9][A-Za-z0-9:_.@\-]{0,126}\.service$`
)

// Runtime limits. Zero values become the documented defaults; negative values
// and values above the hard caps are rejected, never clamped.
const (
	defaultConnectTimeout = 10 * time.Second
	defaultCommandTimeout = 20 * time.Second
	defaultMaxOutputBytes = 1 << 20

	maxConnectTimeout = 30 * time.Second
	maxCommandTimeout = 60 * time.Second
	maxOutputBytesCap = 4 << 20
)

// Connection and credential bounds, applied before any parsing or factory
// construction.
const (
	maxHostBytes        = 255
	maxUsernameBytes    = 128
	maxPrivateKeyBytes  = 1 << 20
	maxPassphraseBytes  = 4096
	maxFingerprintBytes = 128
	maxRequestIDBytes   = 128
)

// Read-action bounds. They mirror the accepted output schemas so a parsed value
// can never exceed its declared bound.
const (
	// maxServiceNameLength bounds the echoed and requested unit name.
	maxServiceNameLength = 256
	// maxJournalLines bounds one journal tail response.
	maxJournalLines = 200
	// maxJournalLineLength bounds each journal line so a single record cannot
	// dominate the model context.
	maxJournalLineLength = 4096
	// maxDiskEntries bounds the disk list of a system snapshot.
	maxDiskEntries = 100
	// maxUptimeSeconds bounds uptime to one hundred years.
	maxUptimeSeconds = int64(3155760000)
	// maxLoadAverage bounds a reported one-minute load average.
	maxLoadAverage = float64(100000)
	// maxByteCount is the JSON-safe integer ceiling (2^53-1) used for byte and
	// capacity counters.
	maxByteCount = int64(9007199254740991)

	defaultJournalLines        = 100
	defaultJournalSinceMinutes = 60
	maxJournalSinceMinutes     = 1440
)

// Local error classes. They are the capability's only stable classification and
// callers compare them with errors.Is; the operator-only cause never changes the
// public class.
var (
	// ErrInvalidInput classifies rejected action input.
	ErrInvalidInput = errors.New("serverops: invalid input")
	// ErrConnectionInvalid classifies a rejected connection or credential.
	ErrConnectionInvalid = errors.New("serverops: connection configuration is invalid")
	// ErrResponseInvalid classifies corrupt or silently partial remote output.
	ErrResponseInvalid = errors.New("serverops: response is invalid")
	// ErrTimeout classifies a canceled or expired context.
	ErrTimeout = errors.New("serverops: request timed out")
	// ErrAuthInvalid classifies a failed host-key verification.
	ErrAuthInvalid = errors.New("serverops: authentication failed")
	// ErrProviderRejected classifies a non-zero remote exit status.
	ErrProviderRejected = errors.New("serverops: provider rejected the request")
	// ErrUpstream classifies any other transport or factory failure.
	ErrUpstream = errors.New("serverops: provider is unavailable")
)

// Error is the capability's public error envelope. Code is one of the local
// Err* sentinels, RequestID/AttemptCount carry non-secret attempt metadata, and
// the cause stays operator-only. Error() is always generic: it never echoes a
// host, command, remote output, or secret.
type Error struct {
	Code         error
	RequestID    string
	AttemptCount int

	cause error
}

func (e *Error) Error() string {
	if e == nil {
		return "server operations execution failed"
	}
	return publicMessage(e.Code)
}

// Unwrap exposes the operator-only cause to errors.Is/errors.As.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Is matches the local error class so callers can classify without inspecting
// the operator-only cause.
func (e *Error) Is(target error) bool {
	if e == nil {
		return false
	}
	return errors.Is(e.Code, target)
}

func publicMessage(code error) string {
	switch {
	case errors.Is(code, ErrInvalidInput):
		return "server operations input is invalid"
	case errors.Is(code, ErrConnectionInvalid):
		return "server operations connection is invalid"
	case errors.Is(code, ErrResponseInvalid):
		return "server operations response is invalid"
	case errors.Is(code, ErrTimeout):
		return "server operations request timed out"
	case errors.Is(code, ErrAuthInvalid):
		return "server operations authentication failed"
	case errors.Is(code, ErrProviderRejected):
		return "server operations provider rejected the request"
	default:
		return "server operations provider is unavailable"
	}
}

func newError(code error, requestID string, cause error) *Error {
	attempt := 0
	if requestID != "" {
		attempt = 1
	}
	return &Error{Code: code, RequestID: requestID, AttemptCount: attempt, cause: cause}
}

// Limits bounds one execution. Zero values take the documented defaults and
// values above the hard caps are rejected.
type Limits struct {
	// ConnectTimeout bounds one dial plus SSH handshake. Zero means 10s.
	ConnectTimeout time.Duration
	// CommandTimeout bounds one remote read command. Zero means 20s.
	CommandTimeout time.Duration
	// MaxOutputBytes bounds the combined stdout/stderr budget. Zero means 1 MiB.
	MaxOutputBytes int
}

// Connection is the request-scoped SSH target plus its credential material.
//
// It deliberately carries no registry identity: integration id, driver id,
// auth-method id, credential source, and connection config belong to the caller
// that builds a Connection, and the capability never persists or learns them.
// PrivateKey and Passphrase are secret; the capability copies them, never
// mutates the caller's slices, and zeroes its own copies before returning.
type Connection struct {
	Host          string
	Port          int
	Username      string
	PrivateKey    []byte
	Passphrase    []byte
	HostKeySHA256 string
}

// Attempt is the non-secret execution metadata of one call.
type Attempt struct {
	RequestID    string
	AttemptCount int
}

// MemoryInfo is the validated memory record of a system snapshot.
type MemoryInfo struct {
	TotalBytes     int64
	AvailableBytes int64
	UsedBytes      int64
}

// DiskInfo is one validated disk record of a system snapshot.
type DiskInfo struct {
	MountPoint     string
	Filesystem     string
	TotalBytes     int64
	AvailableBytes int64
	UsedBytes      int64
}

// SystemSnapshot is the validated result of one system snapshot.
type SystemSnapshot struct {
	Attempt
	Hostname        string
	OperatingSystem string
	KernelVersion   string
	UptimeSeconds   int64
	LoadAverage1m   float64
	Memory          MemoryInfo
	Disks           []DiskInfo
}

// ServiceStatus is the validated result of one systemd service status read.
type ServiceStatus struct {
	Attempt
	ServiceName string
	LoadState   string
	ActiveState string
	SubState    string
	Description string
}

// JournalTail is the validated result of one journal tail read.
type JournalTail struct {
	Attempt
	ServiceName string
	Lines       []string
	Truncated   bool
}

// ValidationProfile is the secret-free result of one connection test. Hostname
// is the remote host's own reported name, never the configured SSH host.
type ValidationProfile struct {
	Attempt
	Hostname string
}

// JournalOptions selects the bounded journal window. A zero field takes the
// documented default; out-of-range values are rejected.
type JournalOptions struct {
	Lines        int
	SinceMinutes int
}
