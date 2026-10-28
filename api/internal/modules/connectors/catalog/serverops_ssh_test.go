package catalog

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
)

// serverOpsSSHTestKey renders a bounded, realistic PEM/OpenSSH-shaped key with
// the requested line ending. The catalog contract validates the text shape, not
// the cryptographic material, so a fixture body is sufficient.
func serverOpsSSHTestKey(lineEnding string) string {
	return strings.Join([]string{
		"-----BEGIN OPENSSH PRIVATE KEY-----",
		"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW",
		"QyNTUxOQAAACBmaXh0dXJlLWtleS1tYXRlcmlhbC1ub3QtYS1yZWFsLXNlY3JldA==",
		"-----END OPENSSH PRIVATE KEY-----",
	}, lineEnding) + lineEnding
}

// serverOpsSSHTestKeyOfLength renders a valid-shaped key padded to exactly the
// requested total byte length.
func serverOpsSSHTestKeyOfLength(total int) string {
	const prefix = "-----BEGIN OPENSSH PRIVATE KEY-----\n"
	const suffix = "\n-----END OPENSSH PRIVATE KEY-----\n"
	return prefix + strings.Repeat("A", total-len(prefix)-len(suffix)) + suffix
}

func serverOpsSSHTestFingerprint() string {
	digest := sha256.Sum256([]byte("serverops-ssh-contract-test-host-key"))
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:])
}

func serverOpsSSHValidValues() map[string]string {
	return map[string]string{
		serverOpsSSHHostField:        "server.example.internal",
		serverOpsSSHPortField:        "22",
		serverOpsSSHUsernameField:    "deploy",
		serverOpsSSHPrivateKeyField:  serverOpsSSHTestKey("\n"),
		serverOpsSSHPassphraseField:  "correct horse battery staple",
		serverOpsSSHFingerprintField: serverOpsSSHTestFingerprint(),
	}
}

func cloneServerOpsSSHValues(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for name, value := range values {
		cloned[name] = value
	}
	return cloned
}

// TestServerOpsSSHDefinitionIsAdmittedDigitalHumanReadOnlyContract pins the
// static catalog contract. Catalog admission is contract-only: this test
// validates the definition without any runtime driver, execution gate, route,
// or approval, none of which this package provides.
func TestServerOpsSSHDefinitionIsAdmittedDigitalHumanReadOnlyContract(t *testing.T) {
	t.Parallel()

	definition := ServerOpsSSHDefinition()
	if err := Validate(definition); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !definition.IsDigitalHumanContract() || !definition.IsServerOpsSSHCredentials() || !definition.UsesCredentials() {
		t.Fatalf("contract families = dh:%v ssh:%v credentials:%v", definition.IsDigitalHumanContract(), definition.IsServerOpsSSHCredentials(), definition.UsesCredentials())
	}
	if definition.ExtensionSchemaVersion != ZGIMetadataSchemaVersion13 || definition.ContractVersion != 4 {
		t.Fatalf("contract version = %q/%d", definition.ExtensionSchemaVersion, definition.ContractVersion)
	}
	if definition.ID != ServerOpsSSHConnectorID || definition.PluginID != ServerOpsSSHPluginID ||
		definition.ProviderID != ServerOpsSSHProviderID || definition.DriverID != ServerOpsSSHDriverID ||
		definition.AuthMethodID != ServerOpsSSHAuthMethodID || definition.SetupMethod != SetupMethodSSHCredentials {
		t.Fatalf("identity = %+v", definition)
	}
	if !reflect.DeepEqual(definition.Surfaces, []Surface{SurfaceDigitalHuman}) || !definition.Supports(SurfaceDigitalHuman) {
		t.Fatalf("surfaces = %v", definition.Surfaces)
	}
	for _, surface := range FirstReleaseSurfaces() {
		if definition.Supports(surface) {
			t.Fatalf("Supports(%q) = true, want Digital-Human-only", surface)
		}
	}
	if len(definition.KeyFields) != 0 {
		t.Fatalf("key fields = %#v, want none", definition.KeyFields)
	}

	wantActions := []string{
		serverops.ActionSystemSnapshot, serverops.ActionServiceStatus, serverops.ActionJournalTail,
		serverops.ActionServiceRestart, serverops.ActionServiceStart, serverops.ActionServiceStop, serverops.ActionServiceReload,
	}
	if len(definition.Operations) != len(wantActions) {
		t.Fatalf("operations = %d, want %d", len(definition.Operations), len(wantActions))
	}
	seenTools := map[string]bool{}
	for index, action := range wantActions {
		operation := definition.Operations[index]
		if operation.ID != action {
			t.Fatalf("operation[%d] id = %q, want %q", index, operation.ID, action)
		}
		if seenTools[operation.ToolName] {
			t.Fatalf("duplicate tool name %q", operation.ToolName)
		}
		seenTools[operation.ToolName] = true
		if serverops.IsServiceWriteAction(action) {
			if operation.Effect != OperationEffectUpdate || operation.RiskHint != OperationRiskHintHigh || operation.Idempotent {
				t.Fatalf("write operation %q is not update/high-risk/non-idempotent: %+v", operation.ID, operation)
			}
			properties, _ := operation.InputSchema["properties"].(map[string]any)
			serviceName, _ := properties["service_name"].(map[string]any)
			if len(properties) != 1 || serviceName["pattern"] != serverops.SystemdServiceNamePattern {
				t.Fatalf("write operation %q input = %#v, want exactly the strict service_name", operation.ID, properties)
			}
			outputProperties, _ := operation.OutputSchema["properties"].(map[string]any)
			for _, name := range []string{"provider", "operation", "service_name", "load_state", "active_state", "sub_state"} {
				if _, ok := outputProperties[name]; !ok {
					t.Fatalf("write operation %q output lacks %q", operation.ID, name)
				}
			}
			if len(outputProperties) != 6 {
				t.Fatalf("write operation %q output = %#v", operation.ID, outputProperties)
			}
			if operationConst, _ := outputProperties["operation"].(map[string]any); operationConst["const"] != action {
				t.Fatalf("write operation %q output const = %#v", operation.ID, operationConst)
			}
		} else if operation.Effect != OperationEffectRead || operation.RiskHint != OperationRiskHintHigh || !operation.Idempotent {
			t.Fatalf("operation %q is not read-only/high-risk/idempotent: %+v", operation.ID, operation)
		}
		assertServerOpsSSHOperationIsNotLowRisk(t, operation)
		if operation.DataEgress || operation.SensitiveDataAllowed || operation.ExternalDestination != "" {
			t.Fatalf("operation %q has an egress contract: %+v", operation.ID, operation)
		}
		if operation.CredentialSubject != ServerOpsSSHCredentialSubject || len(operation.RequiredScopes) != 0 {
			t.Fatalf("operation %q permissions = %q/%v", operation.ID, operation.CredentialSubject, operation.RequiredScopes)
		}
		if operation.SkillID != serverops.SkillID {
			t.Fatalf("operation %q SkillID = %q, want %q", operation.ID, operation.SkillID, serverops.SkillID)
		}
		if len(operation.InputSchema) == 0 || len(operation.OutputSchema) == 0 {
			t.Fatalf("operation %q is missing a complete schema contract", operation.ID)
		}
		assertNoArbitraryCommandInput(t, operation)
	}

	// The snapshot tool takes no operator input beyond the scoped connection.
	if properties, _ := definition.Operations[0].InputSchema["properties"].(map[string]any); len(properties) != 0 {
		t.Fatalf("snapshot input properties = %#v, want none", properties)
	}
	statusProperties, _ := definition.Operations[1].InputSchema["properties"].(map[string]any)
	if len(statusProperties) != 1 {
		t.Fatalf("service status input properties = %#v", statusProperties)
	}
	serviceName, _ := statusProperties["service_name"].(map[string]any)
	if serviceName["pattern"] != serverops.SystemdServiceNamePattern {
		t.Fatalf("service_name pattern = %#v, want the runtime grammar", serviceName["pattern"])
	}
	journalProperties, _ := definition.Operations[2].InputSchema["properties"].(map[string]any)
	for _, name := range []string{"service_name", "lines", "since_minutes"} {
		if _, ok := journalProperties[name]; !ok {
			t.Fatalf("journal tail is missing parameter %q: %#v", name, journalProperties)
		}
	}
	if len(journalProperties) != 3 {
		t.Fatalf("journal tail parameters = %#v", journalProperties)
	}

	admitted, err := New(definition)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, ok := admitted.Get(ServerOpsSSHConnectorID); !ok {
		t.Fatal("admitted SSH contract is not retrievable")
	}
}

// assertNoArbitraryCommandInput rejects any declared input that could carry a
// command, script, path, or deployment instruction.
func assertNoArbitraryCommandInput(t *testing.T, operation Operation) {
	t.Helper()
	properties, ok := operation.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("operation %q input schema has no properties object: %#v", operation.ID, operation.InputSchema)
	}
	for _, forbidden := range []string{"command", "command_line", "script", "shell", "argv", "path", "deploy", "restart", "write"} {
		if _, present := properties[forbidden]; present {
			t.Fatalf("operation %q declares forbidden input %q", operation.ID, forbidden)
		}
	}
	if allowed, present := operation.InputSchema["additionalProperties"]; !present || allowed != false {
		t.Fatalf("operation %q does not close its input schema: %#v", operation.ID, operation.InputSchema)
	}
}

// assertServerOpsSSHOperationIsNotLowRisk keeps the read-only operation Effect
// from being mislabeled as a low-risk read. DigitalHuman execution requires
// per-use confirmation and fails closed for any operation platform policy has
// not explicitly classified, so this deliberate remote-server read must carry a
// High hint rather than Low, Medium, or the "unclassified" non-claim.
func assertServerOpsSSHOperationIsNotLowRisk(t *testing.T, operation Operation) {
	t.Helper()
	if operation.RiskHint != OperationRiskHintHigh {
		t.Fatalf("operation %q risk hint = %q, want %q", operation.ID, operation.RiskHint, OperationRiskHintHigh)
	}
}

func TestServerOpsSSHCredentialFieldContractAndSecretFlags(t *testing.T) {
	t.Parallel()

	definition := ServerOpsSSHDefinition()
	expected := []struct {
		name     string
		required bool
		secret   bool
		max      int
	}{
		{serverOpsSSHHostField, true, false, serverOpsSSHHostMaxBytes},
		{serverOpsSSHPortField, true, false, serverOpsSSHPortMaxBytes},
		{serverOpsSSHUsernameField, true, false, serverOpsSSHUsernameMaxBytes},
		{serverOpsSSHPasswordField, false, true, serverOpsSSHPasswordMaxBytes},
		{serverOpsSSHPrivateKeyField, false, true, 16 * 1024},
		{serverOpsSSHPassphraseField, false, true, serverOpsSSHPassphraseMaxBytes},
		{serverOpsSSHFingerprintField, true, false, serverOpsSSHFingerprintMaxBytes},
		{serverOpsSSHPrivilegeField, false, false, serverOpsSSHPrivilegeMaxBytes},
	}
	if len(definition.CredentialFields) != len(expected) {
		t.Fatalf("credential fields = %d, want %d", len(definition.CredentialFields), len(expected))
	}
	byName := make(map[string]CredentialField, len(definition.CredentialFields))
	for _, field := range definition.CredentialFields {
		byName[field.Name] = field
	}
	for _, want := range expected {
		field, ok := byName[want.name]
		if !ok {
			t.Fatalf("credential field %q is missing", want.name)
		}
		wantEnum := []string(nil)
		if want.name == serverOpsSSHPrivilegeField {
			wantEnum = []string{"sudo"}
		}
		if field.Required != want.required || field.Secret != want.secret || field.Type != "string" ||
			field.MaxLength != want.max || !slices.Equal(field.Enum, wantEnum) ||
			strings.TrimSpace(field.Label) == "" {
			t.Fatalf("credential field %q = %+v, want required=%v secret=%v max=%d", want.name, field, want.required, want.secret, want.max)
		}
	}
	if byName[serverOpsSSHPrivateKeyField].MaxLength != 16*1024 {
		t.Fatalf("private_key bound = %d, want 16 KiB", byName[serverOpsSSHPrivateKeyField].MaxLength)
	}
	for _, nonSecret := range []string{serverOpsSSHHostField, serverOpsSSHPortField, serverOpsSSHUsernameField, serverOpsSSHFingerprintField, serverOpsSSHPrivilegeField} {
		if byName[nonSecret].Secret {
			t.Fatalf("field %q must be non-secret", nonSecret)
		}
	}
}

func TestServerOpsSSHCredentialValues(t *testing.T) {
	t.Parallel()

	definition := ServerOpsSSHDefinition()
	if err := ValidateCredentialValues(definition, serverOpsSSHValidValues()); err != nil {
		t.Fatalf("valid values rejected: %v", err)
	}

	// The passphrase is optional by omission only. A supplied value must
	// already be nonblank and bounded, exactly as the connection fingerprint
	// contract requires, so an empty or whitespace-only value is rejected
	// rather than normalized away.
	withoutPassphrase := serverOpsSSHValidValues()
	delete(withoutPassphrase, serverOpsSSHPassphraseField)
	if err := ValidateCredentialValues(definition, withoutPassphrase); err != nil {
		t.Fatalf("omitted passphrase rejected: %v", err)
	}
	passwordValues := serverOpsSSHValidValues()
	delete(passwordValues, serverOpsSSHPrivateKeyField)
	delete(passwordValues, serverOpsSSHPassphraseField)
	passwordValues[serverOpsSSHPasswordField] = "server-password"
	if err := ValidateCredentialValues(definition, passwordValues); err != nil {
		t.Fatalf("password credentials rejected: %v", err)
	}
	for _, password := range []string{" leading-space", "trailing-space ", "password with spaces"} {
		values := cloneServerOpsSSHValues(passwordValues)
		values[serverOpsSSHPasswordField] = password
		if err := ValidateCredentialValues(definition, values); err != nil {
			t.Fatalf("password %q rejected: %v", password, err)
		}
	}
	for name, passphrase := range map[string]string{
		"empty passphrase":           "",
		"single space passphrase":    " ",
		"whitespace-only passphrase": " \t ",
	} {
		values := serverOpsSSHValidValues()
		values[serverOpsSSHPassphraseField] = passphrase
		if err := ValidateCredentialValues(definition, values); !errors.Is(err, ErrInvalidDefinition) {
			t.Fatalf("%s error = %v, want %v", name, err, ErrInvalidDefinition)
		}
	}

	// The privilege is optional by omission; the only supplied value is "sudo".
	withSudo := serverOpsSSHValidValues()
	withSudo[serverOpsSSHPrivilegeField] = "sudo"
	if err := ValidateCredentialValues(definition, withSudo); err != nil {
		t.Fatalf("sudo privilege rejected: %v", err)
	}
	if err := ValidateServerOpsSSHProbeValues(withSudo); err != nil {
		t.Fatalf("sudo privilege rejected by the probe contract: %v", err)
	}
	for _, privilege := range []string{"", " ", "root", "SUDO", " sudo", "sudo ", "sudo -n", "doas"} {
		values := serverOpsSSHValidValues()
		values[serverOpsSSHPrivilegeField] = privilege
		if err := ValidateCredentialValues(definition, values); !errors.Is(err, ErrInvalidDefinition) {
			t.Fatalf("privilege %q error = %v, want %v", privilege, err, ErrInvalidDefinition)
		}
	}

	// A multiline PEM/OpenSSH key is valid with LF or CRLF endings.
	for name, key := range map[string]string{
		"LF key":   serverOpsSSHTestKey("\n"),
		"CRLF key": serverOpsSSHTestKey("\r\n"),
		"boundary": serverOpsSSHTestKeyOfLength(serverOpsSSHPrivateKeyMaxBytes),
	} {
		values := serverOpsSSHValidValues()
		values[serverOpsSSHPrivateKeyField] = key
		if err := ValidateCredentialValues(definition, values); err != nil {
			t.Fatalf("%s rejected: %v", name, err)
		}
	}

	mutations := []struct {
		name   string
		mutate func(map[string]string)
	}{
		{name: "extra command key", mutate: func(v map[string]string) { v["command"] = "id" }},
		{name: "extra deploy key", mutate: func(v map[string]string) { v["deploy"] = "true" }},
		{name: "extra private key copy", mutate: func(v map[string]string) { v["private_key_pem"] = v[serverOpsSSHPrivateKeyField] }},
		{name: "missing host", mutate: func(v map[string]string) { delete(v, serverOpsSSHHostField) }},
		{name: "missing port", mutate: func(v map[string]string) { delete(v, serverOpsSSHPortField) }},
		{name: "missing username", mutate: func(v map[string]string) { delete(v, serverOpsSSHUsernameField) }},
		{name: "missing private key", mutate: func(v map[string]string) { delete(v, serverOpsSSHPrivateKeyField) }},
		{name: "both authentication methods", mutate: func(v map[string]string) { v[serverOpsSSHPasswordField] = "password" }},
		{name: "empty password", mutate: func(v map[string]string) {
			delete(v, serverOpsSSHPrivateKeyField)
			delete(v, serverOpsSSHPassphraseField)
			v[serverOpsSSHPasswordField] = ""
		}},
		{name: "control password", mutate: func(v map[string]string) {
			delete(v, serverOpsSSHPrivateKeyField)
			delete(v, serverOpsSSHPassphraseField)
			v[serverOpsSSHPasswordField] = "pass\tword"
		}},
		{name: "password with passphrase", mutate: func(v map[string]string) {
			delete(v, serverOpsSSHPrivateKeyField)
			v[serverOpsSSHPasswordField] = "password"
		}},
		{name: "missing fingerprint", mutate: func(v map[string]string) { delete(v, serverOpsSSHFingerprintField) }},
		{name: "blank private key", mutate: func(v map[string]string) { v[serverOpsSSHPrivateKeyField] = "   \n" }},
		{name: "oversized private key", mutate: func(v map[string]string) {
			v[serverOpsSSHPrivateKeyField] = serverOpsSSHTestKeyOfLength(serverOpsSSHPrivateKeyMaxBytes + 1)
		}},
		{name: "NUL private key", mutate: func(v map[string]string) { v[serverOpsSSHPrivateKeyField] = serverOpsSSHTestKey("\n") + "\x00" }},
		{name: "tab private key", mutate: func(v map[string]string) { v[serverOpsSSHPrivateKeyField] = "-----BEGIN\tKEY-----" }},
		{name: "control host", mutate: func(v map[string]string) { v[serverOpsSSHHostField] = "server\nexample" }},
		{name: "space host", mutate: func(v map[string]string) { v[serverOpsSSHHostField] = "server example" }},
		{name: "untrimmed host", mutate: func(v map[string]string) { v[serverOpsSSHHostField] = " server.example" }},
		{name: "port zero", mutate: func(v map[string]string) { v[serverOpsSSHPortField] = "0" }},
		{name: "port above range", mutate: func(v map[string]string) { v[serverOpsSSHPortField] = "65536" }},
		{name: "port leading zero", mutate: func(v map[string]string) { v[serverOpsSSHPortField] = "022" }},
		{name: "port signed", mutate: func(v map[string]string) { v[serverOpsSSHPortField] = "+22" }},
		{name: "port non numeric", mutate: func(v map[string]string) { v[serverOpsSSHPortField] = "ssh" }},
		{name: "control username", mutate: func(v map[string]string) { v[serverOpsSSHUsernameField] = "de\nploy" }},
		{name: "untrimmed username", mutate: func(v map[string]string) { v[serverOpsSSHUsernameField] = "deploy " }},
		{name: "control passphrase", mutate: func(v map[string]string) { v[serverOpsSSHPassphraseField] = "pass\nphrase" }},
		{name: "empty passphrase", mutate: func(v map[string]string) { v[serverOpsSSHPassphraseField] = "" }},
		{name: "space-only passphrase", mutate: func(v map[string]string) { v[serverOpsSSHPassphraseField] = " " }},
		{name: "whitespace-only passphrase", mutate: func(v map[string]string) { v[serverOpsSSHPassphraseField] = " \t " }},
		{name: "oversized passphrase", mutate: func(v map[string]string) {
			v[serverOpsSSHPassphraseField] = strings.Repeat("p", serverOpsSSHPassphraseMaxBytes+1)
		}},
		{name: "fingerprint without prefix", mutate: func(v map[string]string) {
			v[serverOpsSSHFingerprintField] = strings.TrimPrefix(v[serverOpsSSHFingerprintField], serverOpsSSHFingerprintPrefix)
		}},
		{name: "fingerprint short digest", mutate: func(v map[string]string) { v[serverOpsSSHFingerprintField] = "SHA256:AAAA" }},
		{name: "fingerprint with space", mutate: func(v map[string]string) { v[serverOpsSSHFingerprintField] = "SHA256:AA AA" }},
		{name: "fingerprint with newline", mutate: func(v map[string]string) { v[serverOpsSSHFingerprintField] = "SHA256:AAAA\n" }},
	}
	for _, testCase := range mutations {
		t.Run(testCase.name, func(t *testing.T) {
			values := cloneServerOpsSSHValues(serverOpsSSHValidValues())
			testCase.mutate(values)
			if err := ValidateCredentialValues(definition, values); !errors.Is(err, ErrInvalidDefinition) {
				t.Fatalf("ValidateCredentialValues() error = %v, want %v", err, ErrInvalidDefinition)
			}
		})
	}
}

// TestServerOpsSSHCredentialValuesDoesNotRewriteSecretMaps pins the requirement
// that validation only inspects a supplied value: a passphrase that is accepted
// after the blankness check keeps its exact bytes, including surrounding
// whitespace, and an accepted or rejected call never adds, removes, or rewrites
// a key. In particular the optional passphrase is never dropped to make a blank
// value look omitted.
func TestServerOpsSSHCredentialValuesDoesNotRewriteSecretMaps(t *testing.T) {
	t.Parallel()

	definition := ServerOpsSSHDefinition()
	values := serverOpsSSHValidValues()
	values[serverOpsSSHPassphraseField] = " padded secret passphrase "
	before := cloneServerOpsSSHValues(values)
	if err := ValidateCredentialValues(definition, values); err != nil {
		t.Fatalf("padded passphrase rejected: %v", err)
	}
	if !reflect.DeepEqual(values, before) {
		t.Fatalf("validation mutated the supplied maps: %#v, want %#v", values, before)
	}
	if values[serverOpsSSHPassphraseField] != " padded secret passphrase " {
		t.Fatalf("accepted passphrase was normalized to %q", values[serverOpsSSHPassphraseField])
	}

	rejected := serverOpsSSHValidValues()
	rejected[serverOpsSSHPassphraseField] = "   "
	rejectedBefore := cloneServerOpsSSHValues(rejected)
	if err := ValidateCredentialValues(definition, rejected); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("blank passphrase error = %v, want %v", err, ErrInvalidDefinition)
	}
	if !reflect.DeepEqual(rejected, rejectedBefore) {
		t.Fatalf("rejected validation mutated the supplied maps: %#v, want %#v", rejected, rejectedBefore)
	}
	if _, present := rejected[serverOpsSSHPassphraseField]; !present {
		t.Fatal("rejected validation dropped the supplied passphrase instead of leaving it explicit")
	}
}

func TestServerOpsSSHContractDigestPinsFieldAndOperationContracts(t *testing.T) {
	t.Parallel()

	definition := ServerOpsSSHDefinition()
	first, err := DefinitionContractDigest(definition)
	if err != nil {
		t.Fatal(err)
	}
	second, err := DefinitionContractDigest(ServerOpsSSHDefinition())
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !sha256HexPattern.MatchString(first) {
		t.Fatalf("canonical digests = %q / %q", first, second)
	}

	// Member-facing copy is not part of the frozen V4 contract.
	display := cloneDefinition(definition)
	display.Name = "Renamed SSH connector"
	display.NameI18n = LocalizedText{"en-US": "Renamed SSH connector", "zh-Hans": "重命名"}
	display.Description = "Updated member-facing copy"
	display.DescriptionI18n = LocalizedText{"en-US": "Updated member-facing copy", "zh-Hans": "更新后的描述"}
	display.CredentialFields[0].Label = "Renamed host"
	display.CredentialFields[0].LabelI18n = LocalizedText{"en-US": "Renamed host", "zh-Hans": "重命名主机"}
	display.Operations[0].Name = "Renamed operation"
	display.Operations[0].Description = "Updated operation copy"
	display.Operations[0].NameI18n = LocalizedText{"en-US": "Renamed operation", "zh-Hans": "重命名操作"}
	display.Operations[0].DescriptionI18n = LocalizedText{"en-US": "Updated operation copy", "zh-Hans": "更新后的操作描述"}
	if err := Validate(display); err != nil {
		t.Fatalf("display-only change rejected: %v", err)
	}
	displayDigest, err := DefinitionContractDigest(display)
	if err != nil || displayDigest != first {
		t.Fatalf("display-only digest = %q, %v; want %q", displayDigest, err, first)
	}
	if err := CheckCompatibleUpdate(definition, display); err != nil {
		t.Fatalf("display-only update rejected: %v", err)
	}

	// Skill ownership is admitted by provider shape but stays pinned by the
	// frozen operation digest, so a rewrite is still an incompatible update.
	skill := cloneDefinition(definition)
	skill.Operations[0].SkillID = "other-skill"
	if err := Validate(skill); err != nil {
		t.Fatalf("Skill ownership change rejected by shape validation: %v", err)
	}
	skillDigest, err := DefinitionContractDigest(skill)
	if err != nil || skillDigest == first {
		t.Fatalf("Skill rewrite digest = %q, %v; want a changed digest", skillDigest, err)
	}
	if err := CheckCompatibleUpdate(definition, skill); !errors.Is(err, ErrIncompatibleDefinitionUpdate) {
		t.Fatalf("Skill rewrite error = %v, want incompatible update", err)
	}

	// The additive four-surface form is a valid V4 definition, yet widening the
	// admitted Digital-Human-only surface set is still an incompatible update.
	widened := cloneDefinition(definition)
	widened.Surfaces = append(FirstReleaseSurfaces(), SurfaceDigitalHuman)
	if err := Validate(widened); err != nil {
		t.Fatalf("additive V4 surface set rejected: %v", err)
	}
	if err := CheckCompatibleUpdate(definition, widened); !errors.Is(err, ErrIncompatibleDefinitionUpdate) {
		t.Fatalf("surface widening error = %v, want incompatible update", err)
	}
}

func TestServerOpsSSHMutationRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Definition)
	}{
		{name: "connector id", mutate: func(d *Definition) { d.ID = "serverops_ssh_other" }},
		{name: "plugin id", mutate: func(d *Definition) { d.PluginID = "zgi.other-ssh" }},
		{name: "provider id", mutate: func(d *Definition) { d.ProviderID = "other" }},
		{name: "driver id", mutate: func(d *Definition) { d.DriverID = "serverops-ssh-v2" }},
		{name: "auth method id", mutate: func(d *Definition) { d.AuthMethodID = "serverops.ssh_key" }},
		{name: "setup method", mutate: func(d *Definition) { d.SetupMethod = SetupMethodKey }},
		{name: "extension schema version", mutate: func(d *Definition) { d.ExtensionSchemaVersion = ZGIMetadataSchemaVersion12 }},
		{name: "contract version", mutate: func(d *Definition) { d.ContractVersion = 5 }},
		{name: "classification", mutate: func(d *Definition) { d.Classification = ClassificationDataSource }},
		{name: "setup handler", mutate: func(d *Definition) { d.Setup.Handler = "other-handler" }},
		{name: "setup methods", mutate: func(d *Definition) { d.Setup.Methods = []string{"manual", "qr_connect"} }},
		{name: "setup auth kind", mutate: func(d *Definition) { d.Setup.AuthenticationKind = "password" }},
		{name: "setup verification", mutate: func(d *Definition) { d.Setup.VerificationPolicy = "tofu" }},
		{name: "extra credential field", mutate: func(d *Definition) {
			d.CredentialFields = append(d.CredentialFields, CredentialField{Name: "command", Label: "Command", Required: false, Type: "string", MaxLength: 128})
		}},
		{name: "credential field renamed", mutate: func(d *Definition) { d.CredentialFields[0].Name = "address" }},
		{name: "private key bound widened", mutate: func(d *Definition) { d.CredentialFields[3].MaxLength++ }},
		{name: "private key no longer secret", mutate: func(d *Definition) { d.CredentialFields[3].Secret = false }},
		{name: "passphrase made required", mutate: func(d *Definition) { d.CredentialFields[4].Required = true }},
		{name: "host made secret", mutate: func(d *Definition) { d.CredentialFields[0].Secret = true }},
		{name: "credential type changed", mutate: func(d *Definition) { d.CredentialFields[1].Type = "integer" }},
		{name: "credential enum added", mutate: func(d *Definition) { d.CredentialFields[1].Enum = []string{"22", "2222"} }},
		{name: "privilege enum widened", mutate: func(d *Definition) { d.CredentialFields[7].Enum = []string{"sudo", "root"} }},
		{name: "privilege made secret", mutate: func(d *Definition) { d.CredentialFields[7].Secret = true }},
		{name: "privilege made required", mutate: func(d *Definition) { d.CredentialFields[7].Required = true }},
		{name: "privilege removed", mutate: func(d *Definition) { d.CredentialFields = d.CredentialFields[:7] }},
		{name: "write operation made idempotent", mutate: func(d *Definition) { d.Operations[3].Idempotent = true }},
		{name: "write operation effect read", mutate: func(d *Definition) { d.Operations[4].Effect = OperationEffectRead }},
		{name: "write operation input widened", mutate: func(d *Definition) {
			d.Operations[5].InputSchema = objectSchema(map[string]any{"service_name": map[string]any{"type": "string"}, "verb": map[string]any{"type": "string"}}, "service_name")
		}},
		{name: "write operations removed", mutate: func(d *Definition) { d.Operations = d.Operations[:3] }},
		{name: "operation id rewritten", mutate: func(d *Definition) { d.Operations[0].ID = "server.command.run" }},
		{name: "operation tool rewritten", mutate: func(d *Definition) { d.Operations[0].ToolName = "run_command" }},
		{name: "operation effect write", mutate: func(d *Definition) { d.Operations[0].Effect = OperationEffectCreate }},
		{name: "operation effect update", mutate: func(d *Definition) { d.Operations[1].Effect = OperationEffectUpdate }},
		{name: "operation input schema widened", mutate: func(d *Definition) {
			d.Operations[1].InputSchema = objectSchema(map[string]any{"command": map[string]any{"type": "string"}}, "command")
		}},
		{name: "operation output schema widened", mutate: func(d *Definition) { d.Operations[2].OutputSchema = objectSchema(nil) }},
		{name: "operation egress", mutate: func(d *Definition) {
			d.Operations[0].DataEgress = true
			d.Operations[0].ExternalDestination = "example.com"
		}},
		{name: "operation risk downgraded to low", mutate: func(d *Definition) { d.Operations[0].RiskHint = OperationRiskHintLow }},
		{name: "operation risk unclassified", mutate: func(d *Definition) { d.Operations[0].RiskHint = OperationRiskHintUnclassified }},
		{name: "operation risk escalated to critical", mutate: func(d *Definition) { d.Operations[0].RiskHint = OperationRiskHintCritical }},
		{name: "operation credential subject", mutate: func(d *Definition) { d.Operations[0].CredentialSubject = "host" }},
		{name: "operation scopes added", mutate: func(d *Definition) { d.Operations[0].RequiredScopes = [][]string{{"read"}} }},
		{name: "operation removed", mutate: func(d *Definition) { d.Operations = d.Operations[:2] }},
		{name: "operation added", mutate: func(d *Definition) { d.Operations = append(d.Operations, additiveOperationFixture()) }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			current := ServerOpsSSHDefinition()
			candidate := cloneDefinition(current)
			testCase.mutate(&candidate)
			if err := Validate(candidate); !errors.Is(err, ErrInvalidDefinition) {
				t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidDefinition)
			}
			if err := CheckCompatibleUpdate(current, candidate); !errors.Is(err, ErrIncompatibleDefinitionUpdate) {
				t.Fatalf("CheckCompatibleUpdate() error = %v, want %v", err, ErrIncompatibleDefinitionUpdate)
			}
		})
	}
}

func TestServerOpsSSHIsAdmittedLastAndExistingBuiltinsAreUnchanged(t *testing.T) {
	t.Parallel()

	definitions, err := BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	// The SSH contract is admitted to the catalog last, after the ten accepted
	// built-ins. Admission is contract-only: no runtime driver, execution gate,
	// route, or per-use approval is registered by this package, so the admitted
	// contract remains unreachable until a later driver registration batch.
	wantDigests := map[string]string{
		"github":     "0744103a6141e0a18a977b18b8234c9d90b7e3c866dc67bc1ff859770fd7382b",
		"github-mcp": "f70d1fb134f46cb0bd374b023c46f52541a39140358f23d6f4c470587ebe3e01",
		"web-search": "53b87c613241aaff73012b5d2f505fb7461c8e34c5d4d6c4982552fd5f9b356a",
		"feishu":     "d9db772907e21bf034a8ba78ea6a84f1fa829de397c5c0b0284996968452e2e3",
		"dingtalk":   "e22bf6cf6f3b5133c6bad0252e641625fc1c8b02d1ac8845385d95ad23803970",
		"wecom":      "e070f3aa5178737c895d61162f9eef929c76d556fcd044c58c59f3e9291e712b",
		"weixin":     "e25a84aa4081e2b169c8cab493c1fe4d9fa15714be5620a0c9de67b5a21af6de",
		"gmail":      "9a10ffe8439530bf48ea31e1c9d847cc0b9fa35ebf6e690624c03a041810ee8d",
		"qqmail":     "486d6156d6e824794b47dd15c9b921258d8e6ad0a50d76e00a53d701c969c994",
		"netease163": "d8ac059a4ed91922edc1e6577e4fa4bd791fd3bf60100b490e916ec8e5dc00b3",
	}
	if len(definitions) != len(wantDigests)+1+len(SocialMCPDefinitions()) {
		t.Fatalf("built-in count = %d, want %d", len(definitions), len(wantDigests)+1+len(SocialMCPDefinitions()))
	}
	admitted := definitions[len(definitions)-1]
	if admitted.ID != ServerOpsSSHConnectorID || !admitted.IsServerOpsSSHCredentials() || !alreadyV4DigitalHumanOnly(admitted) {
		t.Fatalf("last built-in is not the admitted SSH contract: %+v", admitted)
	}
	// Admitting the catalog entry must not drift the frozen contract digest.
	const wantServerOpsSSHDigest = "f3230d005f3cbf27affb0aee459bc8526a2984e140c2f5aab9589bd642a2e29d"
	if digest, err := DefinitionContractDigest(admitted); err != nil || digest != wantServerOpsSSHDigest {
		t.Fatalf("serverops_ssh contract digest = %q, %v; want %q", digest, err, wantServerOpsSSHDigest)
	}
	// The frozen integration identity is the one a future driver registration
	// batch must bind to; admitting the catalog entry registers no driver.
	if admitted.ProviderID != ServerOpsSSHProviderID || admitted.DriverID != ServerOpsSSHDriverID ||
		admitted.AuthMethodID != ServerOpsSSHAuthMethodID {
		t.Fatalf("SSH integration identity changed: %+v", admitted)
	}
	for _, definition := range definitions[:len(definitions)-1] {
		if IsSocialMCPConnector(definition.ID) {
			continue
		} // Covered by social_mcp_test; older contracts remain frozen below.
		want, ok := wantDigests[definition.ID]
		if !ok {
			t.Fatalf("unexpected built-in %q", definition.ID)
		}
		if definition.IsServerOpsSSHCredentials() {
			t.Fatal("the SSH contract must be admitted only once, as the last built-in")
		}
		// The new SSH predicate must not reclassify any existing dialect.
		if definition.UsesCredentials() != (definition.IsAppCredentials() || definition.IsMessagingCredentials() || definition.IsMailCredentials()) {
			t.Fatalf("credential dialect changed for %q", definition.ID)
		}
		digest, err := DefinitionContractDigest(definition)
		if err != nil {
			t.Fatalf("%s digest: %v", definition.ID, err)
		}
		if digest != want {
			t.Fatalf("%s contract digest = %q, want %q", definition.ID, digest, want)
		}
	}
	if _, ok := wantDigests[admitted.ID]; ok {
		t.Fatal("the SSH connector reuses an accepted built-in identity")
	}
}

func TestServerOpsSSHStoredV4ContractRoundTripPinsDigest(t *testing.T) {
	t.Parallel()

	definition := ServerOpsSSHDefinition()
	record, err := definitionAdmissionRecord(definition, testTime)
	if err != nil {
		t.Fatal(err)
	}
	row := storedDefinitionRow{
		ConnectorID: record.ConnectorID, PluginID: record.PluginID, PluginVersion: record.PluginVersion,
		PluginDigest: record.PluginDigest, SourceDialect: record.SourceDialect, SchemaVersion: record.SchemaVersion,
		ProviderID: record.ProviderID, DriverID: record.DriverID, Classification: record.Classification,
		Display: record.Display, KeySchema: record.KeySchema, CapabilitySummary: record.CapabilitySummary,
		ContractDigest: record.ContractDigest, Status: string(PluginDefinitionStatusActive),
	}
	loaded, err := row.definition()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.IsServerOpsSSHCredentials() || loaded.ContractVersion != 4 || len(loaded.CredentialFields) != 8 || len(loaded.Operations) != 7 {
		t.Fatalf("stored contract changed authentication family: %+v", loaded)
	}
	if err := CheckCompatibleUpdate(definition, loaded); err != nil {
		t.Fatalf("stored round trip is incompatible: %v", err)
	}
	// A forged contract digest must be rejected even when the stored columns
	// otherwise decode into a valid definition.
	row.ContractDigest = strings.Repeat("0", 64)
	if _, err := row.definition(); !errors.Is(err, ErrIncompatibleStoredDefinition) {
		t.Fatalf("tampered digest error = %v, want %v", err, ErrIncompatibleStoredDefinition)
	}
}

// serverOpsSSHRelease100Fixture is the published 1.0.0 contract, pinned to its
// frozen digest so a later reconstruction change cannot drift it.
func serverOpsSSHRelease100Fixture(t *testing.T) Definition {
	t.Helper()
	d := serverOpsSSHReleaseV100Definition()
	digest, err := DefinitionContractDigest(d)
	if err != nil || digest != "cf58703826e9ff787bc85410eb656ecd553e0a1eafcd518d5cfe89a0e0576627" {
		t.Fatalf("accepted 1.0.0 fixture drifted: %s, %v", digest, err)
	}
	if d.PluginVersion != "1.0.0" || len(d.CredentialFields) != 7 || len(d.Operations) != 3 {
		t.Fatalf("1.0.0 fixture shape = %s/%d/%d", d.PluginVersion, len(d.CredentialFields), len(d.Operations))
	}
	return d
}

// TestServerOpsSSHReleaseUpdateIsAcceptedThroughTheOrdinaryCheck pins the
// 1.0.0 -> 1.1.0 release: the stored 1.0.0 contract keeps validating, the new
// release is a compatible update of it (additive write operations plus the one
// reviewed authentication change), the bootstrap admits it through the existing
// lifecycle, and nothing older or mixed is admitted.
func TestServerOpsSSHReleaseUpdateIsAcceptedThroughTheOrdinaryCheck(t *testing.T) {
	previous := serverOpsSSHRelease100Fixture(t)
	current := ServerOpsSSHDefinition()
	if current.PluginVersion != "1.1.0" {
		t.Fatalf("current release = %q, want 1.1.0", current.PluginVersion)
	}
	if err := Validate(previous); err != nil {
		t.Fatalf("stored 1.0.0 contract no longer validates: %v", err)
	}
	oldAuth, _ := digestJSON(canonicalAuthentication(previous))
	newAuth, _ := digestJSON(canonicalAuthentication(current))
	if oldAuth != serverOpsSSHReleaseV100AuthDigest || newAuth != serverOpsSSHReleaseV110AuthDigest {
		t.Fatalf("reviewed authentication drifted: old=%s new=%s", oldAuth, newAuth)
	}
	if err := CheckCompatibleUpdate(previous, current); err != nil {
		t.Fatalf("1.0.0 -> 1.1.0 update rejected: %v", err)
	}
	// Downgrades and same-version content changes stay incompatible.
	if err := CheckCompatibleUpdate(current, previous); !errors.Is(err, ErrIncompatibleDefinitionUpdate) {
		t.Fatalf("downgrade error = %v, want incompatible update", err)
	}
	sameVersion := cloneDefinition(current)
	sameVersion.PluginVersion = previous.PluginVersion
	if reviewedServerOpsSSHPrivilegeExpansion(previous, sameVersion) {
		t.Fatal("the reviewed authentication change must require a new release")
	}
	assertFeishuTemplateBootstrap(t, previous, current)

	// Releases are never mixed: old credentials with new operations and new
	// credentials with old operations are both rejected.
	oldFieldsNewOps := cloneDefinition(current)
	oldFieldsNewOps.CredentialFields = cloneDefinition(previous).CredentialFields
	newFieldsOldOps := cloneDefinition(current)
	newFieldsOldOps.Operations = cloneDefinition(previous).Operations
	for name, candidate := range map[string]Definition{"old fields new ops": oldFieldsNewOps, "new fields old ops": newFieldsOldOps} {
		if err := Validate(candidate); !errors.Is(err, ErrInvalidDefinition) {
			t.Fatalf("%s: Validate() error = %v, want %v", name, err, ErrInvalidDefinition)
		}
	}

	// Any other authentication change on the same upgrade stays incompatible.
	widened := cloneDefinition(current)
	widened.CredentialFields[7].MaxLength = 8
	if reviewedServerOpsSSHPrivilegeExpansion(previous, widened) {
		t.Fatal("an unreviewed authentication contract was admitted")
	}
}
