// Package serveropsssh adapts the reusable server operations capability to the
// Connector CredentialDriver boundary for the Digital-Human-only SSH Server
// Operations contract (catalog.ServerOpsSSHDefinition).
//
// The adapter owns exactly three things: the credential-to-Connection
// translation (including the connection's non-secret privilege), the
// DigitalHuman-scope/argument gate, and the response mapping for the three
// read-only serverops actions and the four controlled service write actions.
// There is no deploy or arbitrary command path. It holds no credential, opens no
// network connection of its own, and delegates every remote interaction to the
// injected serverops capability. The container registers it through
// GetConnectorCandidateVerifier with serverops.New(serverops.Limits{}), which is
// constructor-only and never dials. Platform RiskHigh policy classifies all
// seven operations, so every DigitalHuman use requires fresh per-invocation
// approval with the exact command shown, a run without an interactive approver
// is refused, and the DigitalHuman surface gate is open only for DigitalHuman.
package serveropsssh

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

// SSH credential field names and action-argument names. They mirror the frozen
// catalog contract; any other key is rejected before a Connection is built.
const (
	fieldHost          = "host"
	fieldPort          = "port"
	fieldUsername      = "username"
	fieldPassword      = "password"
	fieldPrivateKey    = "private_key"
	fieldPassphrase    = "passphrase"
	fieldHostKeySHA256 = "host_key_sha256"
	fieldPrivilege     = "privilege"

	fieldServiceName  = "service_name"
	fieldCommand      = "command"
	fieldLines        = "lines"
	fieldSinceMinutes = "since_minutes"
)

// secretFieldNames are the catalog secret fields whose values must never reach a
// user-visible error or result.
var secretFieldNames = [...]string{fieldPassword, fieldPrivateKey, fieldPassphrase}

// Service is the narrow server operations capability the adapter consumes. The
// production implementation is *serverops.Service; tests inject a fake. Keeping
// the seam to these five methods prevents the adapter from reaching any other
// capability behavior.
type Service interface {
	ValidateConnection(context.Context, serverops.Connection) (*serverops.ValidationProfile, error)
	SystemSnapshot(context.Context, serverops.Connection) (*serverops.SystemSnapshot, error)
	ServiceStatus(context.Context, serverops.Connection, string) (*serverops.ServiceStatus, error)
	JournalTail(context.Context, serverops.Connection, string, serverops.JournalOptions) (*serverops.JournalTail, error)
	ChangeService(context.Context, serverops.Connection, string, string) (*serverops.ServiceChange, error)
	RunCommand(context.Context, serverops.Connection, string) (*serverops.CommandRun, error)
}

// HostKeyObservation is the secret-free result of one read-only host-key probe.
// It carries only three bounded, non-secret values: the canonical OpenSSH
// SHA-256 fingerprint observed in capture mode (empty for a strict probe), the
// redacted and bounded remote hostname, and a safe request id.
type HostKeyObservation struct {
	Fingerprint string
	Hostname    string
	RequestID   string
}

// HostKeyProbe is the read-only host-key discovery seam. It accepts the exact
// SSH credential field set and returns only a non-secret observation; it never
// seals, persists, or verifies a credential through the credential lifecycle.
type HostKeyProbe interface {
	ProbeHostKey(context.Context, map[string]string) (HostKeyObservation, error)
}

// Client is the immutable, concurrency-safe adapter. It never stores credential
// material: every call builds a request-scoped Connection and zeroes its own
// password, private-key, and passphrase copies before returning on both paths.
type Client struct {
	definition catalog.Definition
	schemas    *providers.Schemas
	service    Service
}

var _ providers.CredentialDriver = (*Client)(nil)

// The adapter is also the read-only host-key probe seam.
var _ HostKeyProbe = (*Client)(nil)

// The production capability must satisfy the injected seam.
var _ Service = (*serverops.Service)(nil)

// New builds the adapter from an injected server operations capability. A
// production caller passes the service built by serverops.New; tests pass a
// fake. New validates the frozen catalog definition and compiles its operation
// schemas; it never dials, stores a credential, or reaches a network.
func New(service Service) (*Client, error) {
	if service == nil {
		return nil, providers.Invalid()
	}
	definition := catalog.ServerOpsSSHDefinition()
	schemas, err := providers.NewSchemasForDefinition(definition)
	if err != nil {
		return nil, providers.Invalid()
	}
	return &Client{definition: definition, schemas: schemas, service: service}, nil
}

// ConnectorID is the frozen member-facing connector identity.
func (c *Client) ConnectorID() string { return catalog.ServerOpsSSHConnectorID }

// VerifyCredentials validates the exact catalog credential shape and then runs
// the capability's fixed, read-only connectivity probe. It yields the non-secret
// stable account identity built by identityFrom (username, net.JoinHostPort host
// and port), the remote host's own reported hostname, all catalog operation ids,
// and the connection's non-secret privilege as evidence, so a confirmation
// preview can render the exact command without reading the credential.
func (c *Client) VerifyCredentials(ctx context.Context, input providers.CredentialInput) (providers.Verification, error) {
	if c == nil || c.service == nil || catalog.ValidateCredentialValues(c.definition, input.Values) != nil {
		return providers.Verification{}, providers.Invalid()
	}
	connection, release, err := connectionFrom(input.Values)
	if err != nil {
		return providers.Verification{}, err
	}
	defer release()

	profile, err := c.service.ValidateConnection(ctx, connection)
	if err != nil {
		return providers.Verification{}, capabilityFailure(err)
	}
	if profile == nil {
		return providers.Verification{}, responseFailure()
	}

	operations := make([]string, 0, len(c.definition.Operations))
	for _, operation := range c.definition.Operations {
		operations = append(operations, operation.ID)
	}
	identity := identityFrom(input.Values)
	return providers.Verification{
		AccountID:   identity,
		DisplayName: c.redactText(profile.Hostname, input.Values),
		Operations:  operations,
		RequestID:   providers.SafeRequestID(profile.RequestID, ""),
		Evidence: &core.ProviderEvidence{ServerOps: &core.ServerOpsEvidence{
			Version: 1, Identity: identity, Privilege: input.Values[fieldPrivilege],
		}},
	}, nil
}

// ProbeHostKey runs one read-only host-key probe for the exact SSH credential
// field set. It accepts host, port, username, and exactly one of password or
// private_key, with passphrase optional only for the latter, and host_key_sha256 optional: when the
// fingerprint is absent it opts the request-scoped Connection into capture mode
// and returns the observed canonical fingerprint, and when it is present it
// verifies strictly. It never seals or persists a credential and never calls
// VerifyCredentials.
func (c *Client) ProbeHostKey(ctx context.Context, values map[string]string) (HostKeyObservation, error) {
	if c == nil || c.service == nil || catalog.ValidateServerOpsSSHProbeValues(values) != nil {
		return HostKeyObservation{}, providers.Invalid()
	}
	connection, release, err := probeConnectionFrom(values)
	if err != nil {
		return HostKeyObservation{}, err
	}
	defer release()

	profile, err := c.service.ValidateConnection(ctx, connection)
	if err != nil {
		return HostKeyObservation{}, capabilityFailure(err)
	}
	if profile == nil {
		return HostKeyObservation{}, responseFailure()
	}
	// A capture probe must yield the observed fingerprint before it is exposed;
	// a missing observation fails closed rather than being presented as a
	// successful discovery.
	if connection.CaptureHostKey && profile.HostKeyFingerprint == "" {
		return HostKeyObservation{}, responseFailure()
	}
	return HostKeyObservation{
		Fingerprint: profile.HostKeyFingerprint,
		Hostname:    c.redactText(profile.Hostname, values),
		RequestID:   providers.SafeRequestID(profile.RequestID, ""),
	}, nil
}

// InvokeCredentials executes exactly one of the eight serverops actions. It
// accepts a call only for a valid owner-scoped DigitalHuman ResourceScope and
// one of the exact serverops action ids, rejects any extra or invalid argument
// before a Connection is built or any SSH call is made, and delegates to the
// matching capability method. A service write action runs with the
// connection's own privilege; the read actions never use it.
func (c *Client) InvokeCredentials(ctx context.Context, operation string, input providers.CredentialInput, args map[string]any) (providers.Response, error) {
	if c == nil || c.service == nil {
		return providers.Response{}, providers.Invalid()
	}
	if !validDigitalHumanScope(input.ResourceScope) || !knownOperation(operation) {
		return providers.Response{}, providers.Invalid()
	}
	if catalog.ValidateCredentialValues(c.definition, input.Values) != nil {
		return providers.Response{}, providers.Invalid()
	}
	// Schema validation rejects an unknown operation, an extra key, a missing
	// required key, a wrong type, and an out-of-range integer. It runs before
	// the Connection is built and before any capability call, so invalid input
	// can never reach the remote host.
	values, err := c.schemas.Input(operation, args)
	if err != nil {
		return providers.Response{}, providers.Invalid()
	}
	connection, release, err := connectionFrom(input.Values)
	if err != nil {
		return providers.Response{}, err
	}
	defer release()

	var output map[string]any
	var requestID string
	switch operation {
	case serverops.ActionSystemSnapshot:
		result, callErr := c.service.SystemSnapshot(ctx, connection)
		if callErr != nil {
			return providers.Response{}, capabilityFailure(callErr)
		}
		if result == nil {
			return providers.Response{}, responseFailure()
		}
		output, requestID = systemSnapshotOutput(result), result.RequestID
	case serverops.ActionServiceStatus:
		result, callErr := c.service.ServiceStatus(ctx, connection, providers.Text(values, fieldServiceName))
		if callErr != nil {
			return providers.Response{}, capabilityFailure(callErr)
		}
		if result == nil {
			return providers.Response{}, responseFailure()
		}
		output, requestID = serviceStatusOutput(result), result.RequestID
	case serverops.ActionJournalTail:
		result, callErr := c.service.JournalTail(ctx, connection, providers.Text(values, fieldServiceName), serverops.JournalOptions{
			Lines:        providers.Number(values, fieldLines),
			SinceMinutes: providers.Number(values, fieldSinceMinutes),
		})
		if callErr != nil {
			return providers.Response{}, capabilityFailure(callErr)
		}
		if result == nil {
			return providers.Response{}, responseFailure()
		}
		output, requestID = journalTailOutput(result), result.RequestID
	case serverops.ActionServiceRestart, serverops.ActionServiceStart, serverops.ActionServiceStop, serverops.ActionServiceReload:
		result, callErr := c.service.ChangeService(ctx, connection, operation, providers.Text(values, fieldServiceName))
		if callErr != nil {
			return providers.Response{}, capabilityFailure(callErr)
		}
		if result == nil || result.Action != operation {
			return providers.Response{}, responseFailure()
		}
		output, requestID = serviceChangeOutput(result), result.RequestID
	case serverops.ActionCommandRun:
		result, callErr := c.service.RunCommand(ctx, connection, providers.Text(values, fieldCommand))
		if callErr != nil {
			return providers.Response{}, capabilityFailure(callErr)
		}
		if result == nil {
			return providers.Response{}, responseFailure()
		}
		output, requestID = commandRunOutput(result), result.RequestID
	default:
		return providers.Response{}, providers.Invalid()
	}

	// Remote text (a journal line, a hostname, a description) can echo a
	// submitted secret. Redact all catalog secret values before the result
	// leaves the adapter, then re-check the catalog output contract.
	for _, name := range secretFieldNames {
		if secret := input.Values[name]; secret != "" {
			providers.Redact(output, secret)
		}
	}
	if outputErr := c.schemas.Output(operation, output); outputErr != nil {
		return providers.Response{}, responseFailure()
	}
	return providers.Response{Value: output, RequestID: providers.SafeRequestID(requestID, "")}, nil
}

// knownOperation reports whether operation is one of the eight exact actions.
func knownOperation(operation string) bool {
	switch operation {
	case serverops.ActionSystemSnapshot, serverops.ActionServiceStatus, serverops.ActionJournalTail, serverops.ActionCommandRun:
		return true
	default:
		return serverops.IsServiceWriteAction(operation)
	}
}

// validDigitalHumanScope enforces the owner-scoped DigitalHuman boundary. The
// scope must name an organization, owner, connection, credential generation,
// binding instance, binding revision, and logical operation, and its target must
// be a well-formed DigitalHuman target. Anything else fails closed.
func validDigitalHumanScope(scope providers.ResourceScope) bool {
	return scope.OrganizationID != uuid.Nil &&
		scope.OwnerAccountID != uuid.Nil &&
		scope.ConnectionID != uuid.Nil &&
		scope.CredentialVersion >= 1 &&
		scope.BindingID != uuid.Nil &&
		scope.BindingRevision >= 1 &&
		strings.TrimSpace(scope.LogicalOperationID) != "" &&
		scope.Target.Type == core.TargetTypeDigitalHuman &&
		scope.Target.Validate() == nil
}

// identityFrom builds the stable, non-secret account identity
// "username@host:port" that the confirmation preview shows verbatim. The host
// and port are joined with net.JoinHostPort, so an IPv6 literal is bracketed
// exactly as the dial address is (for example "deploy@[2001:db8::1]:22").
//
// A username containing "@" would make the account/host boundary ambiguous, so
// it is rendered with strconv.Quote (for example `"deploy@corp"@host:22`). That
// rendering is injective: a quoted username always contains the "@" it was
// quoted for, while an unquoted username never contains one. The same username,
// host, and port always produce the same identity, so a reconnect recognizes
// the existing connection.
func identityFrom(values map[string]string) string {
	username := values[fieldUsername]
	if strings.Contains(username, "@") {
		username = strconv.Quote(username)
	}
	return username + "@" + net.JoinHostPort(values[fieldHost], values[fieldPort])
}

// connectionFrom translates validated credential values into one request-scoped
// capability Connection with host-key pinning. The returned release function
// zeroes the temporary []byte password, private-key, and passphrase copies; the caller
// must invoke it after the capability call returns, on both the success and the
// error path. The adapter never retains or persists the Connection.
func connectionFrom(values map[string]string) (serverops.Connection, func(), error) {
	port, err := strconv.Atoi(values[fieldPort])
	if err != nil || port < 1 || port > 65535 {
		return serverops.Connection{}, func() {}, providers.Invalid()
	}
	password := []byte(values[fieldPassword])
	privateKey := []byte(values[fieldPrivateKey])
	passphrase := []byte(values[fieldPassphrase])
	connection := serverops.Connection{
		Host:          values[fieldHost],
		Port:          port,
		Username:      values[fieldUsername],
		Password:      password,
		PrivateKey:    privateKey,
		Passphrase:    passphrase,
		HostKeySHA256: values[fieldHostKeySHA256],
		Privilege:     serverops.Privilege(values[fieldPrivilege]),
	}
	release := func() {
		clear(password)
		clear(privateKey)
		clear(passphrase)
	}
	return connection, release, nil
}

// probeConnectionFrom translates validated probe values into one request-scoped
// capability Connection. It reuses connectionFrom for the port parse and the
// temporary secret copies, then opts into host-key capture only when no
// fingerprint was supplied; a supplied fingerprint keeps strict verification.
// The returned release function zeroes the same temporary secret copies.
func probeConnectionFrom(values map[string]string) (serverops.Connection, func(), error) {
	connection, release, err := connectionFrom(values)
	if err != nil {
		return serverops.Connection{}, release, err
	}
	if values[fieldHostKeySHA256] == "" {
		connection.HostKeySHA256 = ""
		connection.CaptureHostKey = true
	}
	return connection, release, nil
}

// redactText removes the catalog secret values from a remote-supplied string and
// bounds it before it can become a display value.
func (c *Client) redactText(value string, values map[string]string) string {
	for _, name := range secretFieldNames {
		if secret := values[name]; secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	return providers.SafeText(value, 255)
}

// capabilityFailure projects a capability error onto the Connector failure
// contract. It never copies the operator-only cause, a remote error string, or a
// credential: only the stable code, retryability, and a chipless request id.
func capabilityFailure(err error) error {
	var capabilityErr *serverops.Error
	if !errors.As(err, &capabilityErr) || capabilityErr == nil {
		return providerFailure(core.ErrorCodeProviderUnavailable, false, "")
	}
	code := core.ErrorCodeProviderUnavailable
	retryable := false
	switch {
	case errors.Is(capabilityErr, serverops.ErrInvalidInput):
		code = core.ErrorCodeInvalidInput
	case errors.Is(capabilityErr, serverops.ErrConnectionInvalid):
		code = core.ErrorCodeInvalidAppCredentials
	case errors.Is(capabilityErr, serverops.ErrAuthInvalid):
		code = core.ErrorCodeInvalidAppCredentials
	case errors.Is(capabilityErr, serverops.ErrPermissionDenied):
		code = core.ErrorCodePermissionMissing
	case errors.Is(capabilityErr, serverops.ErrProviderRejected):
		code = core.ErrorCodeProviderUnavailable
	case errors.Is(capabilityErr, serverops.ErrResponseInvalid):
		code = core.ErrorCodeProviderResponseUnrecognized
	case errors.Is(capabilityErr, serverops.ErrTimeout):
		code = core.ErrorCodeProviderUnavailable
		retryable = true
	default:
		retryable = true
	}
	return providerFailure(code, retryable, capabilityErr.RequestID)
}

func providerFailure(code core.ErrorCode, retryable bool, requestID string) error {
	return &providers.Failure{Code: code, Retryable: retryable, RequestID: providers.SafeRequestID(requestID, "")}
}

func responseFailure() error {
	return providerFailure(core.ErrorCodeProviderResponseUnrecognized, false, "")
}

// systemSnapshotOutput maps a validated snapshot onto the frozen catalog output
// schema exactly: no key is added, omitted, or renamed.
func systemSnapshotOutput(result *serverops.SystemSnapshot) map[string]any {
	disks := make([]any, 0, len(result.Disks))
	for _, disk := range result.Disks {
		disks = append(disks, map[string]any{
			"filesystem":      disk.Filesystem,
			"mount_point":     disk.MountPoint,
			"total_bytes":     disk.TotalBytes,
			"available_bytes": disk.AvailableBytes,
			"used_bytes":      disk.UsedBytes,
		})
	}
	return map[string]any{
		"provider":         catalog.ServerOpsSSHConnectorID,
		"operation":        serverops.ActionSystemSnapshot,
		"hostname":         result.Hostname,
		"operating_system": result.OperatingSystem,
		"kernel_version":   result.KernelVersion,
		"uptime_seconds":   result.UptimeSeconds,
		"load_average_1m":  result.LoadAverage1m,
		"memory": map[string]any{
			"total_bytes":     result.Memory.TotalBytes,
			"available_bytes": result.Memory.AvailableBytes,
			"used_bytes":      result.Memory.UsedBytes,
		},
		"disks": disks,
	}
}

// serviceStatusOutput maps a validated service status onto the frozen schema.
func serviceStatusOutput(result *serverops.ServiceStatus) map[string]any {
	return map[string]any{
		"provider":     catalog.ServerOpsSSHConnectorID,
		"operation":    serverops.ActionServiceStatus,
		"service_name": result.ServiceName,
		"load_state":   result.LoadState,
		"active_state": result.ActiveState,
		"sub_state":    result.SubState,
		"description":  result.Description,
	}
}

// serviceChangeOutput maps a validated service write result onto the frozen
// schema: the unit state read right after the change.
func serviceChangeOutput(result *serverops.ServiceChange) map[string]any {
	return map[string]any{
		"provider":     catalog.ServerOpsSSHConnectorID,
		"operation":    result.Action,
		"service_name": result.ServiceName,
		"load_state":   result.LoadState,
		"active_state": result.ActiveState,
		"sub_state":    result.SubState,
	}
}

// commandRunOutput maps one command run onto the frozen schema. Its output is
// remote text and is redacted with the rest of the result.
func commandRunOutput(result *serverops.CommandRun) map[string]any {
	return map[string]any{
		"provider":  catalog.ServerOpsSSHConnectorID,
		"operation": serverops.ActionCommandRun,
		"command":   result.Command,
		"exit_code": result.ExitCode,
		"stdout":    result.Stdout,
		"stderr":    result.Stderr,
		"truncated": result.Truncated,
	}
}

// journalTailOutput maps a validated journal tail onto the frozen schema. The
// line slice is built as []any so the shared redactor can walk it.
func journalTailOutput(result *serverops.JournalTail) map[string]any {
	lines := make([]any, 0, len(result.Lines))
	for _, line := range result.Lines {
		lines = append(lines, line)
	}
	return map[string]any{
		"provider":     catalog.ServerOpsSSHConnectorID,
		"operation":    serverops.ActionJournalTail,
		"service_name": result.ServiceName,
		"lines":        lines,
		"truncated":    result.Truncated,
	}
}
