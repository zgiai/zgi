package execution

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestConfirmationPreviewFrozenMailAndSecretExclusion(t *testing.T) {
	account := "sender@qq.com"
	connection := core.PersonalConnection{Name: "Work", ExternalAccountID: &account, EncryptedCredentials: "CREDENTIAL-SECRET"}
	request := ExecutionRequest{OperationID: "qqmail.message.send", Input: map[string]any{"to": []any{"first@example.com", "second@example.com"}, "subject": "Hello", "text": strings.Repeat("文", 3000), "app_password": "PRIVATE-KEY", "headers": map[string]any{"Authorization": "BEARER-SECRET"}}}
	preview := buildConfirmationPreview(request, connection, "")
	if !preview.Complete || preview.AccountIdentity != account || len(preview.Fields) != 3 || !preview.Fields[2].Truncated {
		t.Fatalf("invalid preview: %#v", preview)
	}
	if preview.Fields[0].Value != "first@example.com\nsecond@example.com" || len([]rune(preview.Fields[2].Value)) != 2048 {
		t.Fatal("targets or length changed")
	}
	data, _ := json.Marshal(preview)
	for _, secret := range []string{"PRIVATE-KEY", "BEARER-SECRET", "CREDENTIAL-SECRET", "app_password"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("secret escaped")
		}
	}
	connection.Name = "Changed"
	request.Input.(map[string]any)["subject"] = "Changed"
	if preview.AccountName != "Work" || preview.Fields[1].Value != "Hello" {
		t.Fatal("preview was not frozen")
	}
	for _, value := range []any{RuntimeConfirmation{Preview: preview}, FrozenConfirmationRequest{Preview: preview}} {
		encoded, _ := json.Marshal(value)
		if strings.Contains(string(encoded), account) || strings.Contains(string(encoded), "first@example") {
			t.Fatal("preview escaped public runtime projection")
		}
	}
}

func TestConfirmationPreviewRequiresResolvedContextAndCompleteTargets(t *testing.T) {
	account := "original-account"
	conn := core.PersonalConnection{ExternalAccountID: &account}
	req := ExecutionRequest{OperationID: "weixin.messages.reply", Input: map[string]any{"context_handle": "OPAQUE-HANDLE", "resolved_recipient": "FORGED", "text": "Hi"}}
	missing := buildConfirmationPreview(req, conn, "")
	if missing.Complete {
		t.Fatal("accepted client-supplied context target")
	}
	preview := buildConfirmationPreview(req, conn, "verified-recipient")
	if !preview.Complete || preview.Fields[0].Value != "verified-recipient" {
		t.Fatal("resolved recipient missing")
	}
	data, _ := json.Marshal(preview)
	if strings.Contains(string(data), "OPAQUE") || strings.Contains(string(data), "FORGED") {
		t.Fatal("opaque or forged target exposed")
	}
	req.OperationID = "unknown.operation"
	if buildConfirmationPreview(req, conn, "").Complete {
		t.Fatal("unknown operation admitted")
	}
	req.OperationID = "qqmail.message.send"
	req.Input = map[string]any{"to": []string{strings.Repeat("a", 9000)}, "subject": "x", "text": "body"}
	if buildConfirmationPreview(req, conn, "").Complete {
		t.Fatal("truncated recipient admitted")
	}
}

func TestConfirmationPreviewPreservesAllRecipientsAndAccount(t *testing.T) {
	account := "sender@example.com"
	conn := core.PersonalConnection{ExternalAccountID: &account}
	to := make([]string, 20)
	for i := range to {
		to[i] = strings.Repeat("a", 242) + "@example.com"
	}
	req := ExecutionRequest{OperationID: "gmail.message.send", Input: map[string]any{"to": to, "subject": "", "text": "body"}}
	p := buildConfirmationPreview(req, conn, "")
	if !p.Complete || p.Fields[0].Value != strings.Join(to, "\n") {
		t.Fatal("valid recipients were omitted")
	}
	for _, identity := range []string{strings.Repeat("a", 513), "sender\u202e@example.com"} {
		conn.ExternalAccountID = &identity
		if buildConfirmationPreview(req, conn, "").Complete {
			t.Fatal("altered account admitted")
		}
	}
	conn.ExternalAccountID = &account
	req.Input.(map[string]any)["to"] = []string{"recipient\u202e@example.com"}
	if buildConfirmationPreview(req, conn, "").Complete {
		t.Fatal("altered recipient admitted")
	}
}

const serverOpsSSHTestIdentity = "deploy@server.example.com:22"

func serverOpsSSHTestConnection(identity *string) core.PersonalConnection {
	return core.PersonalConnection{
		Name:                 "Production server",
		ExternalAccountID:    identity,
		EncryptedCredentials: "ENCRYPTED-PRIVATE-KEY-ENVELOPE",
	}
}

func serverOpsSSHTestRequest(operation string, input any) ExecutionRequest {
	return ExecutionRequest{
		ConnectorID: core.Identifier(catalog.ServerOpsSSHConnectorID),
		OperationID: core.Identifier(operation),
		Input:       input,
	}
}

func serverOpsSSHTestPtr(value string) *string { return &value }

func serverOpsSSHTestField(t *testing.T, preview *ConfirmationPreview, label string) ConfirmationPreviewField {
	t.Helper()
	for _, field := range preview.Fields {
		if field.Label == label {
			return field
		}
	}
	t.Fatalf("preview field %q missing: %#v", label, preview.Fields)
	return ConfirmationPreviewField{}
}

// serverOpsSSHTestSudoConnection is a verified connection whose non-secret
// evidence records the sudo privilege.
func serverOpsSSHTestSudoConnection() core.PersonalConnection {
	connection := serverOpsSSHTestConnection(serverOpsSSHTestPtr(serverOpsSSHTestIdentity))
	connection.ProviderEvidence = &core.ProviderEvidence{ServerOps: &core.ServerOpsEvidence{
		Version: 1, Identity: serverOpsSSHTestIdentity, Privilege: "sudo",
	}}
	return connection
}

const serverOpsSSHTestStatusCommand = "systemctl show --no-pager --property=LoadState --property=ActiveState " +
	"--property=SubState --property=Description -- 'nginx.service'"

func TestServerOpsSSHConfirmationPreviewCompleteForEveryAction(t *testing.T) {
	plain := serverOpsSSHTestConnection(serverOpsSSHTestPtr(serverOpsSSHTestIdentity))
	plainEvidence := serverOpsSSHTestConnection(serverOpsSSHTestPtr(serverOpsSSHTestIdentity))
	plainEvidence.ProviderEvidence = &core.ProviderEvidence{ServerOps: &core.ServerOpsEvidence{Version: 1, Identity: serverOpsSSHTestIdentity}}
	sudo := serverOpsSSHTestSudoConnection()
	snapshotCommand, err := serverops.CommandLine(serverops.ActionSystemSnapshot, nil, serverops.PrivilegeNone)
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name       string
		operation  string
		input      any
		connection core.PersonalConnection
		wantValues map[string]string
	}{
		{
			name: "system snapshot is fixed for an absent input", operation: serverops.ActionSystemSnapshot, input: nil, connection: plain,
			wantValues: map[string]string{"Action": serverops.ActionSystemSnapshot, "Target": serverOpsSSHTestIdentity, "Command": snapshotCommand},
		},
		{
			name: "system snapshot is fixed for an empty object", operation: serverops.ActionSystemSnapshot, input: map[string]any{}, connection: sudo,
			wantValues: map[string]string{"Action": serverops.ActionSystemSnapshot, "Target": serverOpsSSHTestIdentity, "Command": snapshotCommand},
		},
		{
			name: "service status shows the validated unit", operation: serverops.ActionServiceStatus, connection: plain,
			input:      map[string]any{"service_name": "nginx.service"},
			wantValues: map[string]string{"Service": "nginx.service", "Command": serverOpsSSHTestStatusCommand},
		},
		{
			name: "service status never uses sudo", operation: serverops.ActionServiceStatus, connection: sudo,
			input:      map[string]any{"service_name": "nginx.service"},
			wantValues: map[string]string{"Service": "nginx.service", "Command": serverOpsSSHTestStatusCommand},
		},
		{
			name: "journal tail defaults its bounded window", operation: serverops.ActionJournalTail, connection: plain,
			input: map[string]any{"service_name": "nginx.service"},
			wantValues: map[string]string{"Service": "nginx.service", "Lines": "100", "Since minutes": "60",
				"Command": "journalctl --no-pager --quiet --output=cat --unit='nginx.service' --lines=100 --since=-60min"},
		},
		{
			name: "journal tail keeps explicit bounds", operation: serverops.ActionJournalTail, connection: sudo,
			input: map[string]any{"service_name": "nginx.service", "lines": float64(50), "since_minutes": float64(15)},
			wantValues: map[string]string{"Service": "nginx.service", "Lines": "50", "Since minutes": "15",
				"Command": "journalctl --no-pager --quiet --output=cat --unit='nginx.service' --lines=50 --since=-15min"},
		},
		{
			name: "restart with sudo shows the sudo command", operation: serverops.ActionServiceRestart, connection: sudo,
			input: map[string]any{"service_name": "nginx.service"},
			wantValues: map[string]string{"Action": serverops.ActionServiceRestart, "Service": "nginx.service",
				"Command": "sudo -n systemctl restart -- 'nginx.service'"},
		},
		{
			name: "start without evidence runs as the SSH user", operation: serverops.ActionServiceStart, connection: plain,
			input: map[string]any{"service_name": "app@1.service"},
			wantValues: map[string]string{"Action": serverops.ActionServiceStart, "Service": "app@1.service",
				"Command": "systemctl start -- 'app@1.service'"},
		},
		{
			name: "stop with explicit unprivileged evidence", operation: serverops.ActionServiceStop, connection: plainEvidence,
			input: map[string]any{"service_name": "nginx.service"},
			wantValues: map[string]string{"Action": serverops.ActionServiceStop, "Service": "nginx.service",
				"Command": "systemctl stop -- 'nginx.service'"},
		},
		{
			name: "reload with sudo", operation: serverops.ActionServiceReload, connection: sudo,
			input: map[string]any{"service_name": "nginx.service"},
			wantValues: map[string]string{"Action": serverops.ActionServiceReload, "Service": "nginx.service",
				"Command": "sudo -n systemctl reload -- 'nginx.service'"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			preview := buildConfirmationPreview(
				serverOpsSSHTestRequest(testCase.operation, testCase.input),
				testCase.connection, "",
			)
			if !preview.Complete || preview.Kind != "resource" ||
				preview.AccountIdentity != serverOpsSSHTestIdentity || preview.AccountName != "Production server" ||
				len(preview.Fields) != len(testCase.wantValues) {
				t.Fatalf("preview = %#v", preview)
			}
			for label, want := range testCase.wantValues {
				field := serverOpsSSHTestField(t, preview, label)
				if field.Value != want || field.Truncated ||
					field.LabelI18n["en-US"] == "" || field.LabelI18n["zh-Hans"] == "" {
					t.Fatalf("field %q = %#v, want %q", label, field, want)
				}
				wantFormat := ""
				if label == "Command" {
					wantFormat = ConfirmationPreviewFormatCommand
				}
				if field.Format != wantFormat {
					t.Fatalf("field %q format = %q, want %q", label, field.Format, wantFormat)
				}
			}
			command := serverOpsSSHTestField(t, preview, "Command")
			if command.LabelI18n["zh-Hans"] != "执行命令" || preview.Fields[len(preview.Fields)-1].Label != "Command" {
				t.Fatalf("command field = %#v", command)
			}
			// The web parser requires kind=resource, a non-empty identity, and
			// at least one field before it will mark the preview complete.
			if !completeDigitalHumanPreview(preview) ||
				!digitalHumanApprovalPreviewAllowed(digitalHumanApprovalFreshConfirmation, preview) {
				t.Fatal("complete serverops preview rejected by the DigitalHuman rule")
			}
		})
	}
}

func TestServerOpsSSHConfirmationPreviewRejectsInvalidInputAndIdentity(t *testing.T) {
	validIdentity := serverOpsSSHTestIdentity
	for _, testCase := range []struct {
		name      string
		operation string
		input     any
		identity  *string
	}{
		{"missing identity", serverops.ActionSystemSnapshot, nil, nil},
		{"empty identity", serverops.ActionSystemSnapshot, nil, serverOpsSSHTestPtr("")},
		{"truncated identity", serverops.ActionSystemSnapshot, nil, serverOpsSSHTestPtr(strings.Repeat("a", 513))},
		{"altered identity", serverops.ActionSystemSnapshot, nil, serverOpsSSHTestPtr("deploy\u202e@server.example.com:22")},
		{"unknown action", "server.unknown", nil, &validIdentity},
		{"previous connector identity", "github.create_issue", nil, &validIdentity},
		{"snapshot extra argument", serverops.ActionSystemSnapshot, map[string]any{"command": "id"}, &validIdentity},
		{"snapshot non-object input", serverops.ActionSystemSnapshot, []any{"id"}, &validIdentity},
		{"status missing unit", serverops.ActionServiceStatus, map[string]any{}, &validIdentity},
		{"status extra argument", serverops.ActionServiceStatus, map[string]any{"service_name": "nginx.service", "command": "id"}, &validIdentity},
		{"status bare unit", serverops.ActionServiceStatus, map[string]any{"service_name": "nginx"}, &validIdentity},
		{"status injection", serverops.ActionServiceStatus, map[string]any{"service_name": "nginx.service; rm -rf /"}, &validIdentity},
		{"status wrong type", serverops.ActionServiceStatus, map[string]any{"service_name": 5}, &validIdentity},
		{"status blank unit", serverops.ActionServiceStatus, map[string]any{"service_name": ""}, &validIdentity},
		{"journal lines zero", serverops.ActionJournalTail, map[string]any{"service_name": "nginx.service", "lines": float64(0)}, &validIdentity},
		{"journal lines above cap", serverops.ActionJournalTail, map[string]any{"service_name": "nginx.service", "lines": float64(201)}, &validIdentity},
		{"journal lines wrong type", serverops.ActionJournalTail, map[string]any{"service_name": "nginx.service", "lines": "100"}, &validIdentity},
		{"journal since zero", serverops.ActionJournalTail, map[string]any{"service_name": "nginx.service", "since_minutes": float64(0)}, &validIdentity},
		{"journal since above cap", serverops.ActionJournalTail, map[string]any{"service_name": "nginx.service", "since_minutes": float64(1441)}, &validIdentity},
		{"journal extra argument", serverops.ActionJournalTail, map[string]any{"service_name": "nginx.service", "command": "id"}, &validIdentity},
		{"restart missing unit", serverops.ActionServiceRestart, map[string]any{}, &validIdentity},
		{"restart bare unit", serverops.ActionServiceRestart, map[string]any{"service_name": "nginx"}, &validIdentity},
		{"restart injection", serverops.ActionServiceRestart, map[string]any{"service_name": "nginx.service; reboot"}, &validIdentity},
		{"stop extra verb", serverops.ActionServiceStop, map[string]any{"service_name": "nginx.service", "verb": "mask"}, &validIdentity},
		{"start wrong type", serverops.ActionServiceStart, map[string]any{"service_name": 1}, &validIdentity},
		{"reload non-object input", serverops.ActionServiceReload, []any{"nginx.service"}, &validIdentity},
		{"unknown write action", "server.service.enable", map[string]any{"service_name": "nginx.service"}, &validIdentity},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			preview := buildConfirmationPreview(
				serverOpsSSHTestRequest(testCase.operation, testCase.input),
				serverOpsSSHTestConnection(testCase.identity), "",
			)
			if preview.Complete {
				t.Fatalf("invalid input admitted: %#v", preview)
			}
			if completeDigitalHumanPreview(preview) ||
				digitalHumanApprovalPreviewAllowed(digitalHumanApprovalFreshConfirmation, preview) {
				t.Fatalf("incomplete preview treated as complete: %#v", preview)
			}
		})
	}
}

func TestServerOpsSSHConfirmationPreviewKeepsVerifiedIdentityAndNoSecrets(t *testing.T) {
	identity := serverOpsSSHTestIdentity
	connection := core.PersonalConnection{
		Name:                 "Production server",
		ExternalAccountID:    &identity,
		EncryptedCredentials: "ENCRYPTED-PRIVATE-KEY-ENVELOPE",
		ProviderEvidence: &core.ProviderEvidence{Mail: &core.MailEvidence{
			AuthenticatedLogin: "mailbox@other.example.com",
		}},
	}
	request := serverOpsSSHTestRequest(serverops.ActionServiceStatus, map[string]any{
		"service_name": "nginx.service",
		"private_key":  "-----BEGIN OPENSSH PRIVATE KEY-----",
		"passphrase":   "PASSPHRASE-SENTINEL",
	})
	preview := buildConfirmationPreview(request, connection, "")
	if preview.Complete {
		t.Fatal("extra secret arguments were admitted")
	}
	// The connector's own verified identity must win over unrelated mail
	// evidence and must never be replaced by it.
	if preview.AccountIdentity != identity {
		t.Fatalf("account identity = %q, want %q", preview.AccountIdentity, identity)
	}
	encoded, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		"-----BEGIN OPENSSH PRIVATE KEY-----", "PASSPHRASE-SENTINEL",
		"ENCRYPTED-PRIVATE-KEY-ENVELOPE", "mailbox@other.example.com",
	} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("secret %q escaped into the preview", secret)
		}
	}

	// A secret-looking service_name is rejected rather than echoed.
	malformed := buildConfirmationPreview(
		serverOpsSSHTestRequest(serverops.ActionServiceStatus, map[string]any{
			"service_name": "-----BEGIN OPENSSH PRIVATE KEY-----",
		}),
		serverOpsSSHTestConnection(serverOpsSSHTestPtr(identity)), "",
	)
	malformedJSON, err := json.Marshal(malformed)
	if err != nil {
		t.Fatal(err)
	}
	if malformed.Complete || len(malformed.Fields) != 0 ||
		strings.Contains(string(malformedJSON), "BEGIN OPENSSH PRIVATE KEY") {
		t.Fatalf("malformed secret-like unit was echoed: %#v", malformed)
	}
}

func TestDigitalHumanIncompletePreviewPolicyFailsClosed(t *testing.T) {
	if surfaceForTarget(core.TargetTypeDigitalHuman) != catalog.SurfaceDigitalHuman {
		t.Fatal("DigitalHuman surface gate is closed")
	}
	if surfaceForTarget(core.TargetTypeChat) != catalog.SurfaceChat ||
		surfaceForTarget(core.TargetTypeAgent) != catalog.SurfaceAgent ||
		surfaceForTarget(core.TargetTypeWorkflow) != catalog.SurfaceWorkflow {
		t.Fatal("shared Chat/Agent/Workflow surfaces changed")
	}

	complete := &ConfirmationPreview{
		Kind: "resource", AccountIdentity: serverOpsSSHTestIdentity, Complete: true,
		Fields: []ConfirmationPreviewField{{Label: "Action", Value: serverops.ActionSystemSnapshot}},
	}
	if !digitalHumanApprovalPreviewAllowed(digitalHumanApprovalFreshConfirmation, complete) {
		t.Fatal("complete preview rejected by the DigitalHuman policy")
	}
	// The shared path keeps its existing preview semantics, including a nil
	// preview, so Chat, Agent, and Workflow behavior is unchanged.
	if !digitalHumanApprovalPreviewAllowed(digitalHumanApprovalShared, nil) {
		t.Fatal("shared preview policy changed")
	}
	for name, preview := range map[string]*ConfirmationPreview{
		"nil":              nil,
		"explicit false":   {Kind: "resource", AccountIdentity: serverOpsSSHTestIdentity, Complete: false, Fields: []ConfirmationPreviewField{{Label: "Action", Value: "x"}}},
		"missing identity": {Kind: "resource", AccountIdentity: "", Complete: true, Fields: []ConfirmationPreviewField{{Label: "Action", Value: "x"}}},
		"no fields":        {Kind: "resource", AccountIdentity: serverOpsSSHTestIdentity, Complete: true, Fields: []ConfirmationPreviewField{}},
	} {
		if digitalHumanApprovalPreviewAllowed(digitalHumanApprovalFreshConfirmation, preview) {
			t.Fatalf("incomplete %q preview admitted by the DigitalHuman policy", name)
		}
	}

	// A governed DigitalHuman whose verified target identity is missing cannot
	// produce a complete serverops preview, so the policy must deny it.
	missingIdentity := buildConfirmationPreview(
		serverOpsSSHTestRequest(serverops.ActionSystemSnapshot, nil),
		core.PersonalConnection{Name: "Production server"}, "",
	)
	if digitalHumanApprovalPreviewAllowed(digitalHumanApprovalFreshConfirmation, missingIdentity) {
		t.Fatal("incomplete serverops preview admitted by the DigitalHuman policy")
	}

	// The denial itself is fail-closed and audited: a denied decision is
	// written before returning, and no credential or provider is touched.
	request := validExecutionRequest()
	request.Target = validDigitalHumanTarget()
	request.OperationID = core.Identifier(serverops.ActionSystemSnapshot)
	request.ConnectorID = core.Identifier(catalog.ServerOpsSSHConnectorID)
	harness := newExecutionHarnessForRequest(t, request)
	result, err := harness.service.denyDigitalHumanIncompletePreview(context.Background(), harness.pinned, RiskHigh)
	if !core.IsErrorCode(err, core.ErrorCodeAccessDenied) || result.State != ExecutionStateBlocked {
		t.Fatalf("deny = %#v, %v", result, err)
	}
	if harness.credentials.calls != 0 || harness.provider.calls != 0 ||
		len(harness.audit.decisions) != 1 || len(harness.audit.outcomes) != 0 {
		t.Fatalf("denial reached credentials/provider/decisions/outcomes = %d/%d/%d/%d",
			harness.credentials.calls, harness.provider.calls, len(harness.audit.decisions), len(harness.audit.outcomes))
	}
	decision := harness.audit.decisions[0]
	if decision.Status != core.DecisionStatusDenied || decision.ReceiptID != string(harness.pinned.Receipt) ||
		decision.Target == nil || *decision.Target != request.Target {
		t.Fatalf("denial decision = %#v", decision)
	}
}

// TestServerOpsSSHConfirmationPreviewRejectsForeignPrivilegeEvidence keeps the
// rendered command honest: privilege evidence that names another identity or an
// unsupported privilege makes the preview incomplete instead of guessing.
func TestServerOpsSSHConfirmationPreviewRejectsForeignPrivilegeEvidence(t *testing.T) {
	for name, evidence := range map[string]*core.ServerOpsEvidence{
		"other identity":        {Version: 1, Identity: "root@other.example.com:22", Privilege: "sudo"},
		"empty identity":        {Version: 1, Privilege: "sudo"},
		"unsupported privilege": {Version: 1, Identity: serverOpsSSHTestIdentity, Privilege: "root"},
		"unknown version":       {Version: 2, Identity: serverOpsSSHTestIdentity, Privilege: "sudo"},
	} {
		t.Run(name, func(t *testing.T) {
			connection := serverOpsSSHTestConnection(serverOpsSSHTestPtr(serverOpsSSHTestIdentity))
			connection.ProviderEvidence = &core.ProviderEvidence{ServerOps: evidence}
			for _, operation := range []string{serverops.ActionServiceRestart, serverops.ActionServiceStatus} {
				preview := buildConfirmationPreview(
					serverOpsSSHTestRequest(operation, map[string]any{"service_name": "nginx.service"}), connection, "",
				)
				if preview.Complete || completeDigitalHumanPreview(preview) {
					t.Fatalf("%s: preview with foreign evidence admitted: %#v", operation, preview)
				}
			}
		})
	}
}

// TestServerOpsSSHConfirmationPreviewCommandMatchesExecution pins that the
// previewed command is the capability's own rendering for the same input and
// privilege, so the approval card and the executed command cannot drift.
func TestServerOpsSSHConfirmationPreviewCommandMatchesExecution(t *testing.T) {
	input := map[string]any{"service_name": "worker@2.service"}
	for _, operation := range []string{
		serverops.ActionServiceRestart, serverops.ActionServiceStart, serverops.ActionServiceStop, serverops.ActionServiceReload,
	} {
		want, err := serverops.CommandLine(operation, input, serverops.PrivilegeSudo)
		if err != nil {
			t.Fatal(err)
		}
		preview := buildConfirmationPreview(serverOpsSSHTestRequest(operation, input), serverOpsSSHTestSudoConnection(), "")
		if !preview.Complete || serverOpsSSHTestField(t, preview, "Command").Value != want ||
			!strings.HasPrefix(want, "sudo -n systemctl ") {
			t.Fatalf("%s: preview = %#v, want command %q", operation, preview, want)
		}
		encoded, err := json.Marshal(preview)
		if err != nil || !strings.Contains(string(encoded), `"format":"command"`) {
			t.Fatalf("%s: encoded preview lacks the command format: %s", operation, encoded)
		}
	}
	// Fields without a format omit the key entirely.
	encoded, _ := json.Marshal(ConfirmationPreviewField{Label: "Service", Value: "x"})
	if strings.Contains(string(encoded), "format") {
		t.Fatalf("unformatted field encodes a format key: %s", encoded)
	}
}
