package serverops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

// Private capability sentinels. They keep the operator-visible cause
// classifiable without leaking a credential, host, command, or remote output.
var (
	// errServerOpsConnection classifies a rejected connection or credential.
	errServerOpsConnection = errors.New("serverops: connection configuration is invalid")
	// errServerOpsFactory classifies a missing or unusable runner factory.
	errServerOpsFactory = errors.New("serverops: runner factory is unavailable")
	// errServerOpsRequestID classifies an unusable locally generated request id.
	errServerOpsRequestID = errors.New("serverops: request id is unavailable")
)

// runner is the private, non-exported transport seam. The concrete production
// implementation is *sshClient; tests inject a fake. It deliberately accepts an
// already-built trusted command, so a caller can never supply arbitrary input
// through it.
type runner interface {
	run(ctx context.Context, trustedCommand string) (sshRunResult, error)
}

// runnerFactory is the private synchronous construction seam. A factory must
// consume config synchronously during the call and must not retain raw
// Password, PrivateKey, or Passphrase bytes; the service zeroes them as soon
// as the call returns.
type runnerFactory func(config sshClientConfig) (runner, error)

// requestIDGenerator is the private request-id seam, injectable for
// deterministic tests.
type requestIDGenerator func() string

// Service is the immutable, concurrency-safe server operations capability.
// Every call constructs a fresh runner; the service itself never holds a
// connection or credential material.
type Service struct {
	limits       Limits
	newRunner    runnerFactory
	newRequestID requestIDGenerator
}

// New builds the production service. Zero limits normalize to the documented
// defaults; negative limits and limits above the hard caps are rejected.
func New(limits Limits) (*Service, error) {
	normalized, err := normalizeLimits(limits)
	if err != nil {
		return nil, err
	}
	return newService(normalized, productionRunnerFactory, uuid.NewString), nil
}

// newService assembles a service from already-normalized limits and the two
// private seams. It is the test-injection entry point.
func newService(limits Limits, factory runnerFactory, generateRequestID requestIDGenerator) *Service {
	return &Service{limits: limits, newRunner: factory, newRequestID: generateRequestID}
}

// productionRunnerFactory parses the key material once and returns an immutable
// SSH client. newSSHClient is the canonical parser for private key, passphrase,
// and host key fingerprint, and it retains no raw byte slice.
func productionRunnerFactory(config sshClientConfig) (runner, error) {
	client, err := newSSHClient(config, nil)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// normalizeLimits applies defaults and rejects out-of-contract limits.
func normalizeLimits(limits Limits) (Limits, error) {
	if limits.ConnectTimeout < 0 {
		return Limits{}, errors.New("serverops: connect timeout must not be negative")
	}
	if limits.CommandTimeout < 0 {
		return Limits{}, errors.New("serverops: command timeout must not be negative")
	}
	if limits.MaxOutputBytes < 0 {
		return Limits{}, errors.New("serverops: max output bytes must not be negative")
	}

	normalized := Limits{
		ConnectTimeout: limits.ConnectTimeout,
		CommandTimeout: limits.CommandTimeout,
		MaxOutputBytes: limits.MaxOutputBytes,
	}
	if normalized.ConnectTimeout == 0 {
		normalized.ConnectTimeout = defaultConnectTimeout
	}
	if normalized.CommandTimeout == 0 {
		normalized.CommandTimeout = defaultCommandTimeout
	}
	if normalized.MaxOutputBytes == 0 {
		normalized.MaxOutputBytes = defaultMaxOutputBytes
	}

	if normalized.ConnectTimeout > maxConnectTimeout {
		return Limits{}, fmt.Errorf("serverops: connect timeout exceeds the %s cap", maxConnectTimeout)
	}
	if normalized.CommandTimeout > maxCommandTimeout {
		return Limits{}, fmt.Errorf("serverops: command timeout exceeds the %s cap", maxCommandTimeout)
	}
	if normalized.MaxOutputBytes > maxOutputBytesCap {
		return Limits{}, fmt.Errorf("serverops: max output bytes exceeds the %d byte cap", maxOutputBytesCap)
	}
	return normalized, nil
}

// SystemSnapshot reads bounded host health facts. It takes no operator input.
func (s *Service) SystemSnapshot(ctx context.Context, connection Connection) (*SystemSnapshot, error) {
	// The action input is always valid before any credential work: the only
	// accepted command is the fixed snapshot builder.
	executionRunner, requestID, err := s.prepareRunner(connection)
	if err != nil {
		return nil, err
	}
	result, runErr := executionRunner.run(ctx, buildSystemSnapshotCommand())
	if runErr != nil {
		return nil, runError(requestID, runErr)
	}
	// Structured output must never be silently partial.
	if result.Truncated {
		return nil, newError(ErrResponseInvalid, requestID, fmt.Errorf("%w: system snapshot output was truncated", errReadActionOutput))
	}
	snapshot, err := parseSystemSnapshotOutput(result.Stdout)
	if err != nil {
		return nil, newError(ErrResponseInvalid, requestID, err)
	}
	disks := make([]DiskInfo, 0, len(snapshot.Disks))
	for _, disk := range snapshot.Disks {
		disks = append(disks, DiskInfo{
			MountPoint:     disk.MountPoint,
			Filesystem:     disk.Filesystem,
			TotalBytes:     disk.TotalBytes,
			AvailableBytes: disk.AvailableBytes,
			UsedBytes:      disk.UsedBytes,
		})
	}
	return &SystemSnapshot{
		Attempt:         Attempt{RequestID: requestID, AttemptCount: 1},
		Hostname:        snapshot.Hostname,
		OperatingSystem: snapshot.OperatingSystem,
		KernelVersion:   snapshot.KernelVersion,
		UptimeSeconds:   snapshot.UptimeSeconds,
		LoadAverage1m:   snapshot.LoadAverage1m,
		Memory: MemoryInfo{
			TotalBytes:     snapshot.Memory.TotalBytes,
			AvailableBytes: snapshot.Memory.AvailableBytes,
			UsedBytes:      snapshot.Memory.UsedBytes,
		},
		Disks: disks,
	}, nil
}

// ServiceStatus reads the state of one systemd service unit. The unit name is
// validated before any credential is parsed or any factory is invoked.
func (s *Service) ServiceStatus(ctx context.Context, connection Connection, serviceName string) (*ServiceStatus, error) {
	command, validatedName, err := buildServiceStatusCommand(map[string]any{"service_name": serviceName})
	if err != nil {
		return nil, newError(ErrInvalidInput, "", err)
	}
	executionRunner, requestID, err := s.prepareRunner(connection)
	if err != nil {
		return nil, err
	}
	result, runErr := executionRunner.run(ctx, command)
	if runErr != nil {
		return nil, runError(requestID, runErr)
	}
	if result.Truncated {
		return nil, newError(ErrResponseInvalid, requestID, fmt.Errorf("%w: service status output was truncated", errReadActionOutput))
	}
	status, err := parseServiceStatusOutput(result.Stdout)
	if err != nil {
		return nil, newError(ErrResponseInvalid, requestID, err)
	}
	return &ServiceStatus{
		Attempt:     Attempt{RequestID: requestID, AttemptCount: 1},
		ServiceName: validatedName,
		LoadState:   status.LoadState,
		ActiveState: status.ActiveState,
		SubState:    status.SubState,
		Description: status.Description,
	}, nil
}

// JournalTail reads a bounded window of one systemd service journal. The unit
// name and the numeric window are validated before any credential is parsed or
// any factory is invoked, and the transport truncation flag reaches the parser.
func (s *Service) JournalTail(ctx context.Context, connection Connection, serviceName string, options JournalOptions) (*JournalTail, error) {
	input := map[string]any{"service_name": serviceName}
	if options.Lines != 0 {
		input["lines"] = options.Lines
	}
	if options.SinceMinutes != 0 {
		input["since_minutes"] = options.SinceMinutes
	}
	command, validated, err := buildJournalTailCommand(input)
	if err != nil {
		return nil, newError(ErrInvalidInput, "", err)
	}
	executionRunner, requestID, err := s.prepareRunner(connection)
	if err != nil {
		return nil, err
	}
	result, runErr := executionRunner.run(ctx, command)
	if runErr != nil {
		return nil, runError(requestID, runErr)
	}
	journal, err := parseJournalTailOutput(result.Stdout, validated.Lines, result.Truncated)
	if err != nil {
		return nil, newError(ErrResponseInvalid, requestID, err)
	}
	return &JournalTail{
		Attempt:     Attempt{RequestID: requestID, AttemptCount: 1},
		ServiceName: validated.ServiceName,
		Lines:       journal.Lines,
		Truncated:   journal.Truncated,
	}, nil
}

// ValidateCredential parses the credential material locally. It never dials,
// never runs a command, and never retains the secret copies. A canceled context
// fails fast before the factory is invoked.
//
// An invalid credential or connection classifies as exactly one public class,
// ErrInvalidInput, so callers never see ErrInvalidInput and
// ErrConnectionInvalid match the same error. The operator-only cause is kept.
// A factory wiring failure stays ErrUpstream and is not masked as input.
func (s *Service) ValidateCredential(ctx context.Context, connection Connection) error {
	if err := ctx.Err(); err != nil {
		return newError(ErrTimeout, "", err)
	}
	if _, err := s.buildRunner(connection); err != nil {
		var capabilityErr *Error
		if errors.As(err, &capabilityErr) && errors.Is(capabilityErr.Code, ErrConnectionInvalid) {
			return newError(ErrInvalidInput, "", operatorCause(err))
		}
		return err
	}
	return nil
}

// ValidateConnection performs one auditable, read-only connectivity check.
//
// It accepts no operator input: the only command it can ever run is the exact
// fixed system snapshot command, so a connection test can never be turned into
// arbitrary command execution. The profile carries only the remote hostname and
// the non-secret attempt metadata.
func (s *Service) ValidateConnection(ctx context.Context, connection Connection) (*ValidationProfile, error) {
	if err := ctx.Err(); err != nil {
		return nil, newError(ErrTimeout, "", err)
	}
	executionRunner, err := s.buildRunner(connection)
	if err != nil {
		return nil, err
	}
	requestID := s.generateRequestID()
	if !validRequestID(requestID) {
		return nil, newError(ErrUpstream, "", errServerOpsRequestID)
	}
	result, runErr := executionRunner.run(ctx, buildSystemSnapshotCommand())
	if runErr != nil {
		return nil, runError(requestID, runErr)
	}
	if result.Truncated {
		return nil, newError(ErrResponseInvalid, requestID, fmt.Errorf("%w: system snapshot output was truncated", errReadActionOutput))
	}
	snapshot, err := parseSystemSnapshotOutput(result.Stdout)
	if err != nil {
		return nil, newError(ErrResponseInvalid, requestID, err)
	}
	profile := &ValidationProfile{
		Attempt:  Attempt{RequestID: requestID, AttemptCount: 1},
		Hostname: snapshot.Hostname,
	}
	// A capture probe must yield a canonical, bounded fingerprint before it is
	// exposed; a missing or malformed observation fails closed. A strict probe
	// never surfaces an observed value.
	if connection.CaptureHostKey {
		fingerprint, fingerprintErr := canonicalObservedHostKeyFingerprint(result.HostKeyFingerprint)
		if fingerprintErr != nil {
			return nil, newError(ErrResponseInvalid, requestID, fingerprintErr)
		}
		profile.HostKeyFingerprint = fingerprint
	}
	return profile, nil
}

// prepareRunner builds one fresh runner and one request id. Connection and
// credential failures never reach this point, and no raw secret survives it.
// Capture mode is a probe-only opt-in: a read action must never run with
// host-key verification switched to discovery, so a capture connection is
// rejected before any credential work or factory call.
func (s *Service) prepareRunner(connection Connection) (runner, string, error) {
	if connection.CaptureHostKey {
		return nil, "", newError(ErrConnectionInvalid, "", errServerOpsConnection)
	}
	executionRunner, err := s.buildRunner(connection)
	if err != nil {
		return nil, "", err
	}
	requestID := s.generateRequestID()
	if !validRequestID(requestID) {
		return nil, "", newError(ErrUpstream, "", errServerOpsRequestID)
	}
	return executionRunner, requestID, nil
}

// generateRequestID calls the injected generator, tolerating a nil seam.
func (s *Service) generateRequestID() string {
	if s.newRequestID == nil {
		return ""
	}
	return s.newRequestID()
}

// buildRunner validates the request-scoped connection, copies the key material,
// and constructs exactly one runner through the private factory seam. The raw
// private-key and passphrase copies are zeroed immediately after the synchronous
// factory returns, on both the success and error paths.
func (s *Service) buildRunner(connection Connection) (runner, error) {
	config, err := s.connectionConfig(connection)
	if err != nil {
		return nil, newError(ErrConnectionInvalid, "", err)
	}

	factory := s.newRunner
	if factory == nil {
		zeroBytes(config.Password)
		zeroBytes(config.PrivateKey)
		zeroBytes(config.Passphrase)
		return nil, newError(ErrUpstream, "", errServerOpsFactory)
	}

	created, factoryErr := factory(config)
	zeroBytes(config.Password)
	zeroBytes(config.PrivateKey)
	zeroBytes(config.Passphrase)

	if factoryErr != nil {
		if errors.Is(factoryErr, errSSHInvalidConfig) {
			return nil, newError(ErrConnectionInvalid, "", factoryErr)
		}
		return nil, newError(ErrUpstream, "", factoryErr)
	}
	if created == nil {
		return nil, newError(ErrUpstream, "", errServerOpsFactory)
	}
	return created, nil
}

// connectionConfig validates one request-scoped connection and returns the SSH
// client configuration for the factory. Every failure is the same generic
// sentinel so a rejected value can never be inferred from the error, and the
// caller's slices are copied, never retained or mutated.
//
// The text targets are fail-closed: an untrimmed or control-bearing host,
// username, or fingerprint is rejected instead of being normalized, so the
// target a caller displays or approves is exactly the target that is dialed.
// Only legal interior characters (for example a space inside a username) pass.
func (s *Service) connectionConfig(connection Connection) (sshClientConfig, error) {
	host := connection.Host
	if host == "" || hasSurroundingWhitespace(host) || len(host) > maxHostBytes ||
		hasASCIIControlOrSpace(host) {
		return sshClientConfig{}, errServerOpsConnection
	}
	username := connection.Username
	if username == "" || hasSurroundingWhitespace(username) || len(username) > maxUsernameBytes ||
		hasASCIIControl(username) {
		return sshClientConfig{}, errServerOpsConnection
	}
	if connection.Port < 1 || connection.Port > 65535 {
		return sshClientConfig{}, errServerOpsConnection
	}
	hasPassword := len(connection.Password) > 0
	hasPrivateKey := len(connection.PrivateKey) > 0
	if hasPassword == hasPrivateKey {
		return sshClientConfig{}, errServerOpsConnection
	}
	if hasPassword && (len(connection.Password) > maxPasswordBytes || hasASCIIControl(string(connection.Password)) || len(bytes.TrimSpace(connection.Password)) == 0) {
		return sshClientConfig{}, errServerOpsConnection
	}
	if hasPrivateKey && (len(connection.PrivateKey) > maxPrivateKeyBytes || len(bytes.TrimSpace(connection.PrivateKey)) == 0) {
		return sshClientConfig{}, errServerOpsConnection
	}
	// passphrase is optional only for an unencrypted/private key connection.
	if len(connection.Passphrase) > maxPassphraseBytes || (len(connection.Passphrase) > 0 && !hasPrivateKey) {
		return sshClientConfig{}, errServerOpsConnection
	}
	fingerprint := connection.HostKeySHA256
	if connection.CaptureHostKey {
		// Capture is opt-in host-key discovery, so a supplied fingerprint is a
		// contradictory request and is rejected rather than silently ignored.
		if fingerprint != "" {
			return sshClientConfig{}, errServerOpsConnection
		}
	} else if fingerprint == "" || hasSurroundingWhitespace(fingerprint) ||
		len(fingerprint) > maxFingerprintBytes || hasASCIIControlOrSpace(fingerprint) {
		return sshClientConfig{}, errServerOpsConnection
	}

	return sshClientConfig{
		Host:           host,
		Port:           connection.Port,
		Username:       username,
		Password:       append([]byte(nil), connection.Password...),
		PrivateKey:     append([]byte(nil), connection.PrivateKey...),
		Passphrase:     append([]byte(nil), connection.Passphrase...),
		HostKeySHA256:  fingerprint,
		CaptureHostKey: connection.CaptureHostKey,
		ConnectTimeout: s.limits.ConnectTimeout,
		CommandTimeout: s.limits.CommandTimeout,
		MaxOutputBytes: s.limits.MaxOutputBytes,
	}, nil
}

// runError classifies one runner failure into the local error classes. The
// wrapped cause stays available to operators through Error.Unwrap, while
// Error() remains generic.
func runError(requestID string, err error) error {
	var exitErr *ssh.ExitError
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return newError(ErrTimeout, requestID, err)
	case errors.Is(err, errSSHHostKeyMismatch):
		return newError(ErrAuthInvalid, requestID, err)
	case errors.Is(err, errSSHInvalidConfig):
		return newError(ErrConnectionInvalid, requestID, err)
	case errors.Is(err, errSSHInvalidCommand):
		return newError(ErrInvalidInput, requestID, err)
	case errors.As(err, &exitErr):
		return newError(ErrProviderRejected, requestID, err)
	default:
		return newError(ErrUpstream, requestID, err)
	}
}

// validRequestID accepts a nonblank, bounded opaque request id.
func validRequestID(requestID string) bool {
	return strings.TrimSpace(requestID) != "" && len(requestID) <= maxRequestIDBytes
}

// operatorCause returns the operator-only cause of a capability error without
// its public classification, so an error that is re-classified into another
// public class cannot match two classes at once.
func operatorCause(err error) error {
	var capabilityErr *Error
	if errors.As(err, &capabilityErr) && capabilityErr.cause != nil {
		return capabilityErr.cause
	}
	return err
}

// hasSurroundingWhitespace reports whether value differs from its trimmed form.
// An untrimmed target is rejected rather than normalized, so a displayed or
// approved value can never silently become a different dialed target.
func hasSurroundingWhitespace(value string) bool {
	return value != strings.TrimSpace(value)
}

// hasASCIIControlOrSpace rejects any ASCII control byte or space, including a
// surrounding one, so a value carrying it can never be normalized into a valid
// target.
func hasASCIIControlOrSpace(value string) bool {
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character <= 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

// hasASCIIControl rejects any ASCII control byte. Spaces are allowed.
func hasASCIIControl(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] < 0x20 || value[index] == 0x7f {
			return true
		}
	}
	return false
}

// zeroBytes overwrites a request-scoped secret copy in place.
func zeroBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
