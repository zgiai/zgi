package catalog

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
)

// This file declares the Digital-Human-only contract for the built-in SSH
// Server Operations Connector. BuiltinDefinitions admits it to the catalog, and
// the serveropsssh driver adapter and platform RiskHigh policy are wired around
// this frozen contract. The DigitalHuman execution surface gate
// (surfaceForTarget) is open only for DigitalHuman, so the admitted contract is
// reachable only through a user-set-up SSH credential, an exact bound
// DigitalHuman target, and fresh per-invocation approval. This file itself
// constructs no provider, driver, route, or approval and never dials a host, so
// catalog admission alone never dials.
//
// The contract exposes exactly the three read-only serverops capability actions
// and the four controlled service write actions (restart, start, stop, reload of
// one systemd unit) through a distinct ssh_credentials setup dialect. It
// declares no arbitrary command, deployment, or configuration-change operation,
// and it cannot be used to reach a host on its own.
//
// Effect is not a risk claim. Every operation, read or write, is declared
// OperationRiskHintHigh: DigitalHuman execution requires a mandatory per-use
// confirmation for this surface and fails closed whenever platform policy has
// not explicitly classified an operation, so a sensitive remote server read
// over SSH must not be presented as low-risk either.
//
// Release history. 1.0.0 published the three read actions and seven credential
// fields. 1.1.0 (ADR 0023) adds the four write actions and the optional,
// non-secret privilege field. Both releases stay accepted contracts, so a
// stored 1.0.0 definition keeps validating and is replaced through the ordinary
// compatible-update check; see serverOpsSSHAcceptedContracts.

// SSH Server Operations stable identities. The connector id is the exact
// member-facing identity; the provider/driver/auth identities are the frozen
// integration identity the wired driver adapter binds to.
const (
	ServerOpsSSHConnectorID  = "serverops_ssh"
	ServerOpsSSHPluginID     = "zgi.serverops-ssh"
	ServerOpsSSHProviderID   = "serverops"
	ServerOpsSSHDriverID     = "serverops-ssh-v1"
	ServerOpsSSHAuthMethodID = "serverops.ssh_credentials"
	// ServerOpsSSHCredentialSubject is the operation credential subject frozen
	// by the V4 permission digest.
	ServerOpsSSHCredentialSubject = "server"
)

// SetupMethodSSHCredentials is the dedicated SSH credential dialect. It is a
// new member of the closed setup-method set, so every previously published
// messaging, app-credential, mail, and Key dialect keeps its exact identity and
// contract digest.
const SetupMethodSSHCredentials SetupMethod = "ssh_credentials"

// SSH credential field names. The set is exact: no other key is admitted.
const (
	serverOpsSSHHostField        = "host"
	serverOpsSSHPortField        = "port"
	serverOpsSSHUsernameField    = "username"
	serverOpsSSHPasswordField    = "password"
	serverOpsSSHPrivateKeyField  = "private_key"
	serverOpsSSHPassphraseField  = "passphrase"
	serverOpsSSHFingerprintField = "host_key_sha256"
	serverOpsSSHPrivilegeField   = "privilege"
)

// ServerOpsSSHPrivilegeSudo is the only accepted privilege value. Omitting the
// privilege field runs every command as the SSH user; "sudo" runs the service
// write actions behind "sudo -n". Read actions never use sudo.
const ServerOpsSSHPrivilegeSudo = string(serverops.PrivilegeSudo)

// SSH credential bounds. They mirror the serverops connection bounds, except
// that private_key is bounded to 16 KiB here rather than the 1 MiB transport
// ceiling. That keeps the value inside the existing connection idempotency
// bound (16 KiB) and comfortably inside the 32 KiB credential route body.
const (
	serverOpsSSHHostMaxBytes        = 255
	serverOpsSSHPortMaxBytes        = 5
	serverOpsSSHUsernameMaxBytes    = 128
	serverOpsSSHPasswordMaxBytes    = 4096
	serverOpsSSHPrivateKeyMaxBytes  = 16 * 1024
	serverOpsSSHPassphraseMaxBytes  = 4096
	serverOpsSSHFingerprintMaxBytes = 128
	serverOpsSSHPrivilegeMaxBytes   = 4
	serverOpsSSHMinPort             = 1
	serverOpsSSHMaxPort             = 65535
	// serverOpsSSHFingerprintPrefix is the only accepted host-key fingerprint
	// family, matching serverops' canonical OpenSSH SHA-256 form.
	serverOpsSSHFingerprintPrefix = "SHA256:"
)

// IsServerOpsSSHCredentials reports whether the definition is the exact SSH
// Server Operations built-in. It is deliberately scoped to the frozen version,
// setup dialect, and integration identity, so it can never widen validation for
// another connector that happens to carry the ssh_credentials setup method.
func (d Definition) IsServerOpsSSHCredentials() bool {
	return d.ExtensionSchemaVersion == ZGIMetadataSchemaVersion13 &&
		d.ContractVersion == 4 &&
		d.SetupMethod == SetupMethodSSHCredentials &&
		d.ID == ServerOpsSSHConnectorID &&
		d.ProviderID == ServerOpsSSHProviderID &&
		d.DriverID == ServerOpsSSHDriverID &&
		d.AuthMethodID == ServerOpsSSHAuthMethodID
}

// serverOpsSSHPluginVersion is the current published SSH contract release.
const serverOpsSSHPluginVersion = "1.1.0"

// ServerOpsSSHDefinition returns the static SSH Server Operations contract.
// Each call builds an independent value, and JSON round-tripping normalizes the
// schema numerics to the JSON-number form the operation-schema compiler
// requires, exactly as the other static definitions do. It is admitted to the
// catalog as the last built-in. Catalog admission and the driver adapter are
// wired, and the DigitalHuman execution surface gate admits only an exact bound
// DigitalHuman target under fresh per-invocation approval.
func ServerOpsSSHDefinition() Definition {
	operation := func(id, toolName, en, zh string, input, output map[string]any) Operation {
		return Operation{
			// The operations belong to the runtime-managed connector Skill, as every
			// other connector's do, so the DigitalHuman's connector binding
			// authorizes them; a Skill of their own was never enabled anywhere.
			ID: id, ToolName: toolName, SkillID: "connector-runtime",
			Name: en, NameI18n: LocalizedText{"en-US": en, "zh-Hans": zh},
			Description: en, DescriptionI18n: LocalizedText{"en-US": en, "zh-Hans": zh},
			InputSchema: input, OutputSchema: output,
			// Read-only, but High risk: the DigitalHuman surface always
			// requires per-use confirmation and fails closed for an
			// unclassified operation, so the hint must not imply a low-risk
			// read.
			Effect: OperationEffectRead, RiskHint: OperationRiskHintHigh, Idempotent: true,
			CredentialSubject: ServerOpsSSHCredentialSubject,
		}
	}
	// writeOperation changes the state of one systemd unit. It is an update of
	// existing server state, never idempotent, and High risk, so it inherits the
	// mandatory fresh per-call confirmation and the background-run refusal.
	writeOperation := func(id, toolName, en, zh string, input, output map[string]any) Operation {
		op := operation(id, toolName, en, zh, input, output)
		op.Effect, op.Idempotent = OperationEffectUpdate, false
		return op
	}
	text := func(maxLength int) map[string]any {
		return map[string]any{"type": "string", "maxLength": maxLength}
	}
	byteCount := func() map[string]any {
		// 2^53-1 is the JSON-safe integer ceiling used by the serverops
		// byte and capacity counters.
		return map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991}
	}
	serviceNameInput := func() map[string]any {
		return map[string]any{
			"type": "string", "minLength": 1, "maxLength": 256,
			"pattern": serverops.SystemdServiceNamePattern,
		}
	}

	d := Definition{
		ID: ServerOpsSSHConnectorID, PluginID: ServerOpsSSHPluginID, PluginVersion: serverOpsSSHPluginVersion,
		SourceDialect: SourceDialectAgentPluginsV1, SourceSchemaVersion: SourceSchemaVersion1,
		ProviderID: ServerOpsSSHProviderID, DriverID: ServerOpsSSHDriverID, AuthMethodID: ServerOpsSSHAuthMethodID,
		Name: "SSH Server Operations", NameI18n: LocalizedText{"en-US": "SSH Server Operations", "zh-Hans": "SSH 服务器运维"},
		Description:     "After per-use approval, inspect one Linux server over SSH and restart, start, stop, or reload its systemd services.",
		DescriptionI18n: LocalizedText{"en-US": "After per-use approval, inspect one Linux server over SSH and restart, start, stop, or reload its systemd services.", "zh-Hans": "逐次审批后，通过 SSH 检查单台 Linux 服务器，并重启、启动、停止或重载其 systemd 服务。"},
		Icon:            "serverops-ssh", Classification: ClassificationConnector,
		CapabilityTypes: []CapabilityType{CapabilityTypeAction},
		SetupMethod:     SetupMethodSSHCredentials, Ownership: OwnershipPersonal,
		Surfaces: []Surface{SurfaceDigitalHuman}, KeyFields: []KeyField{},
		ExtensionSchemaVersion: ZGIMetadataSchemaVersion13, ContractVersion: 4,
		Setup: &SetupDescriptor{
			Handler:            "serverops-ssh-credentials-v1",
			Methods:            []string{"manual"},
			AuthenticationKind: "ssh_password_or_private_key",
			VerificationPolicy: "host_key_pinned",
		},
		CredentialFields: []CredentialField{
			{Name: serverOpsSSHHostField, Label: "Host", LabelI18n: LocalizedText{"en-US": "Host", "zh-Hans": "主机"}, Required: true, Secret: false, Type: "string", MaxLength: serverOpsSSHHostMaxBytes},
			{Name: serverOpsSSHPortField, Label: "Port", LabelI18n: LocalizedText{"en-US": "Port", "zh-Hans": "端口"}, Required: true, Secret: false, Type: "string", MaxLength: serverOpsSSHPortMaxBytes},
			{Name: serverOpsSSHUsernameField, Label: "Username", LabelI18n: LocalizedText{"en-US": "Username", "zh-Hans": "用户名"}, Required: true, Secret: false, Type: "string", MaxLength: serverOpsSSHUsernameMaxBytes},
			{Name: serverOpsSSHPasswordField, Label: "SSH password", LabelI18n: LocalizedText{"en-US": "SSH password", "zh-Hans": "SSH 密码"}, Required: false, Secret: true, Type: "string", MaxLength: serverOpsSSHPasswordMaxBytes},
			{Name: serverOpsSSHPrivateKeyField, Label: "SSH private key", LabelI18n: LocalizedText{"en-US": "SSH private key", "zh-Hans": "SSH 私钥"}, Required: false, Secret: true, Type: "string", MaxLength: serverOpsSSHPrivateKeyMaxBytes},
			{Name: serverOpsSSHPassphraseField, Label: "Private key passphrase", LabelI18n: LocalizedText{"en-US": "Private key passphrase", "zh-Hans": "私钥口令"}, Required: false, Secret: true, Type: "string", MaxLength: serverOpsSSHPassphraseMaxBytes},
			{Name: serverOpsSSHFingerprintField, Label: "Host key SHA-256 fingerprint", LabelI18n: LocalizedText{"en-US": "Host key SHA-256 fingerprint", "zh-Hans": "主机密钥 SHA-256 指纹"}, Required: true, Secret: false, Type: "string", MaxLength: serverOpsSSHFingerprintMaxBytes},
			{Name: serverOpsSSHPrivilegeField, Label: "Command privilege", LabelI18n: LocalizedText{"en-US": "Command privilege", "zh-Hans": "命令权限"}, Required: false, Secret: false, Type: "string", Enum: []string{ServerOpsSSHPrivilegeSudo}, MaxLength: serverOpsSSHPrivilegeMaxBytes},
		},
	}

	d.Operations = []Operation{
		operation(
			serverops.ActionSystemSnapshot, "serverops_ssh_system_snapshot",
			"Read a host system snapshot", "读取主机系统快照",
			objectSchema(nil),
			objectSchema(map[string]any{
				"provider":         map[string]any{"const": ServerOpsSSHConnectorID},
				"operation":        map[string]any{"const": serverops.ActionSystemSnapshot},
				"hostname":         text(255),
				"operating_system": text(255),
				"kernel_version":   text(255),
				"uptime_seconds":   map[string]any{"type": "integer", "minimum": 0, "maximum": 3155760000},
				"load_average_1m":  map[string]any{"type": "number", "minimum": 0, "maximum": 100000},
				"memory": objectSchema(map[string]any{
					"total_bytes":     byteCount(),
					"available_bytes": byteCount(),
					"used_bytes":      byteCount(),
				}, "total_bytes", "available_bytes", "used_bytes"),
				"disks": map[string]any{"type": "array", "maxItems": 100, "items": objectSchema(map[string]any{
					"filesystem":      text(255),
					"mount_point":     text(1024),
					"total_bytes":     byteCount(),
					"available_bytes": byteCount(),
					"used_bytes":      byteCount(),
				}, "filesystem", "mount_point", "total_bytes", "available_bytes", "used_bytes")},
			}, "provider", "operation", "hostname", "operating_system", "kernel_version", "uptime_seconds", "load_average_1m", "memory", "disks"),
		),
		operation(
			serverops.ActionServiceStatus, "serverops_ssh_service_status",
			"Read a systemd service status", "读取 systemd 服务状态",
			objectSchema(map[string]any{"service_name": serviceNameInput()}, "service_name"),
			objectSchema(map[string]any{
				"provider":     map[string]any{"const": ServerOpsSSHConnectorID},
				"operation":    map[string]any{"const": serverops.ActionServiceStatus},
				"service_name": text(256),
				"load_state":   text(64),
				"active_state": text(64),
				"sub_state":    text(64),
				"description":  text(1024),
			}, "provider", "operation", "service_name", "load_state", "active_state", "sub_state", "description"),
		),
		operation(
			serverops.ActionJournalTail, "serverops_ssh_journal_tail",
			"Read a bounded service journal tail", "读取有界服务日志尾部",
			objectSchema(map[string]any{
				"service_name":  serviceNameInput(),
				"lines":         map[string]any{"type": "integer", "minimum": 1, "maximum": 200, "default": 100},
				"since_minutes": map[string]any{"type": "integer", "minimum": 1, "maximum": 1440, "default": 60},
			}, "service_name"),
			objectSchema(map[string]any{
				"provider":     map[string]any{"const": ServerOpsSSHConnectorID},
				"operation":    map[string]any{"const": serverops.ActionJournalTail},
				"service_name": text(256),
				"lines":        map[string]any{"type": "array", "maxItems": 200, "items": text(4096)},
				"truncated":    map[string]any{"type": "boolean"},
			}, "provider", "operation", "service_name", "lines", "truncated"),
		),
		writeOperation(
			serverops.ActionServiceRestart, "serverops_ssh_service_restart",
			"Restart a systemd service", "重启 systemd 服务",
			objectSchema(map[string]any{"service_name": serviceNameInput()}, "service_name"),
			serviceChangeOutput(serverops.ActionServiceRestart),
		),
		writeOperation(
			serverops.ActionServiceStart, "serverops_ssh_service_start",
			"Start a systemd service", "启动 systemd 服务",
			objectSchema(map[string]any{"service_name": serviceNameInput()}, "service_name"),
			serviceChangeOutput(serverops.ActionServiceStart),
		),
		writeOperation(
			serverops.ActionServiceStop, "serverops_ssh_service_stop",
			"Stop a systemd service", "停止 systemd 服务",
			objectSchema(map[string]any{"service_name": serviceNameInput()}, "service_name"),
			serviceChangeOutput(serverops.ActionServiceStop),
		),
		writeOperation(
			serverops.ActionServiceReload, "serverops_ssh_service_reload",
			"Reload a systemd service", "重载 systemd 服务",
			objectSchema(map[string]any{"service_name": serviceNameInput()}, "service_name"),
			serviceChangeOutput(serverops.ActionServiceReload),
		),
	}

	// JSON Schema expects JSON number/array types, including for static
	// definitions. This also detaches the returned value from the literals.
	encoded, _ := json.Marshal(d)
	_ = json.Unmarshal(encoded, &d)
	return d
}

// serviceChangeOutput is the output schema of one service write action: the
// unit state read again after the change.
func serviceChangeOutput(action string) map[string]any {
	text := func(maxLength int) map[string]any {
		return map[string]any{"type": "string", "maxLength": maxLength}
	}
	return objectSchema(map[string]any{
		"provider":     map[string]any{"const": ServerOpsSSHConnectorID},
		"operation":    map[string]any{"const": action},
		"service_name": text(256),
		"load_state":   text(64),
		"active_state": text(64),
		"sub_state":    text(64),
	}, "provider", "operation", "service_name", "load_state", "active_state", "sub_state")
}

// The published 1.0.0 contract and the reviewed 1.0.0 -> 1.1.0 authentication
// change are pinned by frozen digests. They were taken from the accepted
// definitions, not derived at run time, so a change to the reconstruction below
// can never silently widen what a stored contract may be.
const (
	serverOpsSSHReleaseV100ContractDigest = "cf58703826e9ff787bc85410eb656ecd553e0a1eafcd518d5cfe89a0e0576627"
	serverOpsSSHReleaseV100AuthDigest     = "ff4c3728d1efb628765e64769ba46a90115a3ccbf645e2575fe36fcce4870a44"
	serverOpsSSHReleaseV110AuthDigest     = "f55fe95e4a857bab4ec485d3c44d9a5d5b39192250fca03799c5239b6838bfbf"
)

// serverOpsSSHReleaseV100Definition reconstructs the published 1.0.0 contract:
// the current contract without the privilege field and the four write actions.
// It is accepted only while it matches the frozen 1.0.0 digest.
func serverOpsSSHReleaseV100Definition() Definition {
	d := ServerOpsSSHDefinition()
	d.PluginVersion = "1.0.0"
	d.Description = "After per-use approval, read-only inspection of one Linux server over SSH."
	d.DescriptionI18n = LocalizedText{"en-US": d.Description, "zh-Hans": "逐次审批后，通过 SSH 对单台 Linux 服务器进行只读检查。"}
	fields := make([]CredentialField, 0, len(d.CredentialFields))
	for _, field := range d.CredentialFields {
		if field.Name != serverOpsSSHPrivilegeField {
			fields = append(fields, field)
		}
	}
	d.CredentialFields = fields
	operations := make([]Operation, 0, len(d.Operations))
	for _, operation := range d.Operations {
		if !serverops.IsServiceWriteAction(operation.ID) {
			// 1.0.0 grouped its operations under a Skill of their own.
			operation.SkillID = serverops.SkillID
			operations = append(operations, operation)
		}
	}
	d.Operations = operations
	return d
}

// serverOpsSSHAcceptedContracts returns every accepted SSH contract, newest
// first. The 1.0.0 reconstruction is included only while it still matches the
// frozen digest; a drifted reconstruction is dropped, so the validator fails
// closed for a stored 1.0.0 definition instead of accepting an unreviewed shape.
func serverOpsSSHAcceptedContracts() []Definition {
	accepted := []Definition{ServerOpsSSHDefinition()}
	previous := serverOpsSSHReleaseV100Definition()
	digest, err := digestJSON(struct {
		Version  int `json:"version"`
		Contract any `json:"contract"`
	}{4, canonicalDefinitionContractV2(previous)})
	if err == nil && digest == serverOpsSSHReleaseV100ContractDigest {
		accepted = append(accepted, previous)
	}
	return accepted
}

// reviewedServerOpsSSHPrivilegeExpansion admits the one reviewed SSH
// authentication change: the 1.0.0 credential contract to the 1.1.0 contract
// that adds the optional, non-secret privilege field. Both complete
// authentication contracts are pinned by frozen digests, so no other credential
// change is admitted; the ordinary checks still protect identities, surfaces,
// and every existing operation.
func reviewedServerOpsSSHPrivilegeExpansion(current, candidate Definition) bool {
	if !current.IsServerOpsSSHCredentials() || !candidate.IsServerOpsSSHCredentials() ||
		current.PluginVersion == candidate.PluginVersion {
		return false
	}
	oldDigest, err := digestJSON(canonicalAuthentication(current))
	if err != nil || oldDigest != serverOpsSSHReleaseV100AuthDigest {
		return false
	}
	newDigest, err := digestJSON(canonicalAuthentication(candidate))
	return err == nil && newDigest == serverOpsSSHReleaseV110AuthDigest
}

// reviewedServerOpsSSHSkillRelabel admits the one reviewed SSH operation change
// from the published 1.0.0 contract: its operations move from the
// "server-operations" Skill, which nothing ever enabled, to the runtime-managed
// "connector-runtime" Skill every other connector uses, so a DigitalHuman's
// connector binding authorizes them. Everything else about each operation must
// stay exactly the same.
func reviewedServerOpsSSHSkillRelabel(current Definition, operation, replacement Operation) bool {
	if current.ID != ServerOpsSSHConnectorID || current.PluginVersion != "1.0.0" ||
		operation.SkillID != serverops.SkillID || replacement.SkillID != "connector-runtime" {
		return false
	}
	digest, err := digestJSON(struct {
		Version  int `json:"version"`
		Contract any `json:"contract"`
	}{4, canonicalDefinitionContractV2(current)})
	if err != nil || digest != serverOpsSSHReleaseV100ContractDigest {
		return false
	}
	relabeled := replacement
	relabeled.SkillID = operation.SkillID
	left, leftErr := digestJSON(canonicalOperationContract(operation))
	right, rightErr := digestJSON(canonicalOperationContract(relabeled))
	return leftErr == nil && rightErr == nil && left == right
}

// validateServerOpsSSHContract is the tightly scoped provider-shape validator
// for the exact SSH built-in. It freezes the identity, version, setup
// descriptor, credential-field contract, and operation contract against the
// static definition. Member-facing copy stays free to change; Skill ownership
// stays free to change because it is frozen by the accepted version digest and
// the ordinary update check.
func validateServerOpsSSHContract(d Definition) error {
	var err error
	for _, canonical := range serverOpsSSHAcceptedContracts() {
		if err = validateServerOpsSSHContractAgainst(d, canonical); err == nil {
			return nil
		}
	}
	return err
}

// validateServerOpsSSHContractAgainst checks d against one accepted contract as
// a whole: fields and operations are never mixed across releases.
func validateServerOpsSSHContractAgainst(d, canonical Definition) error {
	if d.ID != canonical.ID || d.PluginID != canonical.PluginID || d.ContractVersion != canonical.ContractVersion ||
		d.ExtensionSchemaVersion != canonical.ExtensionSchemaVersion ||
		d.ProviderID != canonical.ProviderID || d.DriverID != canonical.DriverID ||
		d.AuthMethodID != canonical.AuthMethodID || d.SetupMethod != canonical.SetupMethod ||
		d.Ownership != canonical.Ownership || d.Classification != canonical.Classification ||
		len(d.KeyFields) != 0 || d.Setup == nil || !reflect.DeepEqual(d.Setup, canonical.Setup) ||
		len(d.CredentialFields) != len(canonical.CredentialFields) || len(d.Operations) != len(canonical.Operations) {
		return invalid("serverops_ssh_contract")
	}
	for _, expected := range canonical.CredentialFields {
		found := false
		for _, field := range d.CredentialFields {
			if field.Name != expected.Name {
				continue
			}
			if found || field.Required != expected.Required || field.Secret != expected.Secret ||
				field.Type != expected.Type || field.MaxLength != expected.MaxLength || !slices.Equal(field.Enum, expected.Enum) ||
				strings.TrimSpace(field.Label) == "" || !validLocalizedText(field.LabelI18n) ||
				!validLocalizedText(field.PlaceholderI18n) {
				return invalid("serverops_ssh_credentials")
			}
			found = true
		}
		if !found {
			return invalid("serverops_ssh_credentials")
		}
	}
	for _, operation := range d.Operations {
		matched := false
		for _, expected := range canonical.Operations {
			expected.SkillID = operation.SkillID
			left, leftErr := digestJSON(canonicalOperationContract(operation))
			right, rightErr := digestJSON(canonicalOperationContract(expected))
			if leftErr == nil && rightErr == nil && left == right &&
				operation.CredentialSubject == expected.CredentialSubject && len(operation.RequiredScopes) == 0 {
				matched = true
			}
		}
		if !matched {
			return invalid("serverops_ssh_operations")
		}
	}
	return nil
}

// validateServerOpsSSHCredentialValues checks the complete SSH credential
// schema without retaining any submitted value. The value map is never mutated
// or normalized: the optional passphrase may be absent, but any supplied value
// must already be nonblank and bounded.
func validateServerOpsSSHCredentialValues(values map[string]string) error {
	return validateServerOpsSSHValues(values, true)
}

// ValidateServerOpsSSHProbeValues checks the exact SSH probe credential schema
// without retaining any submitted value. It admits the same field set and the
// same required host/port/username plus exactly one authentication method as
// the credential contract, but host_key_sha256 is optional so a probe may opt
// into host-key capture; a supplied fingerprint must already be canonical. It
// is not part of the frozen connector contract, so ServerOpsSSHDefinition and
// its digest are unchanged.
func ValidateServerOpsSSHProbeValues(values map[string]string) error {
	return validateServerOpsSSHValues(values, false)
}

// validateServerOpsSSHValues is the shared SSH value validator. The value map
// is never mutated or normalized; fingerprintRequired selects the credential
// contract (required) or the probe contract (optional by omission).
func validateServerOpsSSHValues(values map[string]string, fingerprintRequired bool) error {
	allowed := map[string]bool{
		serverOpsSSHHostField: true, serverOpsSSHPortField: true, serverOpsSSHUsernameField: true,
		serverOpsSSHPasswordField: true, serverOpsSSHPrivateKeyField: true, serverOpsSSHPassphraseField: true, serverOpsSSHFingerprintField: true,
		serverOpsSSHPrivilegeField: true,
	}
	for name := range values {
		if !allowed[name] {
			return invalid("credential_values")
		}
	}
	host, ok := values[serverOpsSSHHostField]
	if !ok || !validServerOpsSSHTarget(host, serverOpsSSHHostMaxBytes, true) {
		return invalid("credential_host")
	}
	port, ok := values[serverOpsSSHPortField]
	if !ok || !validServerOpsSSHPort(port) {
		return invalid("credential_port")
	}
	username, ok := values[serverOpsSSHUsernameField]
	if !ok || !validServerOpsSSHTarget(username, serverOpsSSHUsernameMaxBytes, false) {
		return invalid("credential_username")
	}
	privateKey, hasPrivateKey := values[serverOpsSSHPrivateKeyField]
	password, hasPassword := values[serverOpsSSHPasswordField]
	if hasPrivateKey == hasPassword {
		return invalid("credential_authentication")
	}
	if hasPrivateKey && !validServerOpsSSHPrivateKey(privateKey) {
		return invalid("credential_private_key")
	}
	if hasPassword && !validServerOpsSSHPassword(password) {
		return invalid("credential_password")
	}
	// The passphrase is valid only with a private key and is optional only by
	// omission. A supplied blank value is rejected rather than normalized away.
	if passphrase, present := values[serverOpsSSHPassphraseField]; present {
		if !hasPrivateKey || !validServerOpsSSHPassphrase(passphrase) {
			return invalid("credential_passphrase")
		}
	}
	// The privilege is optional only by omission: omitted runs as the SSH user,
	// and the only supplied value accepted is exactly "sudo".
	if privilege, present := values[serverOpsSSHPrivilegeField]; present && privilege != ServerOpsSSHPrivilegeSudo {
		return invalid("credential_privilege")
	}
	if fingerprintRequired {
		fingerprint, ok := values[serverOpsSSHFingerprintField]
		if !ok || !validServerOpsSSHFingerprint(fingerprint) {
			return invalid("credential_host_key")
		}
		return nil
	}
	if fingerprint, present := values[serverOpsSSHFingerprintField]; present && !validServerOpsSSHFingerprint(fingerprint) {
		return invalid("credential_host_key")
	}
	return nil
}

func validServerOpsSSHPassword(value string) bool {
	return value != "" && strings.TrimSpace(value) != "" && len(value) <= serverOpsSSHPasswordMaxBytes && utf8.ValidString(value) && !hasASCIIControlByte(value)
}

func hasASCIIControlByte(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] < 0x20 || value[index] == 0x7f {
			return true
		}
	}
	return false
}

// validServerOpsSSHTarget accepts a fail-closed host, username, or fingerprint
// component: non-blank, exactly trimmed, bounded, and free of ASCII controls.
// A host additionally rejects spaces, matching the serverops connection rule.
// A value is never normalized into validity.
func validServerOpsSSHTarget(value string, maxBytes int, rejectSpace bool) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > maxBytes {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character < 0x20 || character == 0x7f || (rejectSpace && character == ' ') {
			return false
		}
	}
	return true
}

// validServerOpsSSHPort accepts only the canonical decimal spelling of an
// in-range TCP port. Signs, whitespace, leading zeros, and out-of-range values
// are rejected instead of being reinterpreted.
func validServerOpsSSHPort(value string) bool {
	if value == "" || len(value) > serverOpsSSHPortMaxBytes {
		return false
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < serverOpsSSHMinPort || port > serverOpsSSHMaxPort {
		return false
	}
	return strconv.Itoa(port) == value
}

// validServerOpsSSHPrivateKey accepts bounded, valid-UTF-8 PEM/OpenSSH key
// text. Line feeds and carriage returns are the only admitted control bytes, so
// a multiline key with LF or CRLF line endings is accepted while NUL, tabs, and
// every other control byte are rejected. The key is never trimmed or parsed
// here; only blankness is decided by TrimSpace.
func validServerOpsSSHPrivateKey(value string) bool {
	if len(value) == 0 || len(value) > serverOpsSSHPrivateKeyMaxBytes || strings.TrimSpace(value) == "" {
		return false
	}
	if !utf8.ValidString(value) {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character == '\n' || character == '\r' {
			continue
		}
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

// validServerOpsSSHPassphrase accepts only a supplied, nonblank, bounded,
// single-line passphrase. Omitting the field entirely is the documented
// "unencrypted key" form; a supplied empty or whitespace-only value is rejected
// exactly as connections.CommandHasher.RequestFingerprint rejects every blank
// supplied value. The value is only inspected, never trimmed or rewritten.
func validServerOpsSSHPassphrase(value string) bool {
	if strings.TrimSpace(value) == "" || len(value) > serverOpsSSHPassphraseMaxBytes || !utf8.ValidString(value) {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

// validServerOpsSSHFingerprint accepts only the canonical OpenSSH SHA-256
// fingerprint form: the "SHA256:" prefix followed by the unpadded standard
// base64 encoding of exactly 32 bytes. There is no MD5, skip, or TOFU form.
func validServerOpsSSHFingerprint(value string) bool {
	if value == "" || len(value) > serverOpsSSHFingerprintMaxBytes || !strings.HasPrefix(value, serverOpsSSHFingerprintPrefix) {
		return false
	}
	if !validServerOpsSSHTarget(value, serverOpsSSHFingerprintMaxBytes, true) {
		return false
	}
	encoded := strings.TrimPrefix(value, serverOpsSSHFingerprintPrefix)
	digest, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(digest) != sha256.Size {
		return false
	}
	return serverOpsSSHFingerprintPrefix+base64.RawStdEncoding.EncodeToString(digest) == value
}
