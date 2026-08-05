package mail

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestProviderDefinitionsRegisterAndExposeExpectedActions(t *testing.T) {
	registry := integrations.NewRegistry()
	adapter := New()
	for _, definition := range ProviderDefinitions() {
		if err := registry.Register(integrations.Registration{Definition: definition, Adapter: adapter, ConnectionTester: adapter, HealthProbe: adapter}); err != nil {
			t.Fatalf("register %s: %v", definition.ID, err)
		}
	}
	standard, ok := registry.ProviderDefinition(IntegrationStandardMail)
	if !ok || len(standard.Actions) != 6 {
		t.Fatalf("standard mail actions = %d, want 6", len(standard.Actions))
	}
	if len(standard.AuthMethods) != 6 {
		t.Fatalf("standard mail auth methods = %d, want 6", len(standard.AuthMethods))
	}
	if len(registry.ProviderDefinitions()) != 1 {
		t.Fatalf("mail provider count = %d, want 1", len(registry.ProviderDefinitions()))
	}
	for _, definition := range []integrations.ProviderDefinition{standard} {
		for _, action := range definition.Actions {
			if action.ID == ActionMessageSend || action.ID == ActionMessageReply {
				if action.DefaultPolicy == nil || !action.DefaultPolicy.Enabled || action.DefaultPolicy.ApprovalPolicy != "always_ask" || action.SuccessDeduplication == nil {
					t.Fatalf("write action %s must be enabled with mandatory confirmation and deduplication", action.ID)
				}
			}
		}
	}
}

func TestNativeMailOutputsMatchActionContractsAfterRuntimeNormalization(t *testing.T) {
	fixtures := map[string]map[string]interface{}{
		ActionAccountGet: {
			"provider": IntegrationStandardMail,
			"account":  map[string]interface{}{"email": "person@example.com", "display_name": "Example", "read_supported": true, "send_supported": true},
		},
		ActionFolderList: {
			"provider": IntegrationStandardMail,
			"folders":  []map[string]interface{}{{"name": "INBOX", "attributes": []string{"\\HasNoChildren"}}},
		},
		ActionMessageSearch: {
			"provider": IntegrationStandardMail,
			"messages": []map[string]interface{}{{"message_ref": "message-ref", "subject": "Subject", "from": "sender@example.com", "to": "person@example.com", "date": "2026-08-05T10:00:00Z", "unread": true, "size": 128}},
		},
		ActionMessageGet: {
			"provider": IntegrationStandardMail,
			"message":  map[string]interface{}{"message_ref": "message-ref", "message_id": "message-id", "subject": "Subject", "from": "sender@example.com", "to": "person@example.com", "cc": "", "date": "2026-08-05T10:00:00Z", "body_text": "Body", "attachments": []map[string]interface{}{{"filename": "report.pdf", "content_type": "application/pdf", "size": 1024}}},
		},
		ActionMessageSend: {
			"provider": IntegrationStandardMail,
			"message":  map[string]interface{}{"message_id": "message-id", "accepted_recipients": []string{"person@example.com"}, "smtp_accepted": true},
		},
		ActionMessageReply: {
			"provider": IntegrationStandardMail,
			"message":  map[string]interface{}{"message_id": "reply-id", "accepted_recipients": []string{"sender@example.com"}, "smtp_accepted": true},
		},
	}
	definition := ProviderDefinitions()[0]
	for _, action := range definition.Actions {
		t.Run(action.ID, func(t *testing.T) {
			output, ok := fixtures[action.ID]
			if !ok {
				t.Fatalf("missing native output fixture for %s", action.ID)
			}
			normalized, err := tools.NormalizeJSONValue(output)
			if err != nil {
				t.Fatal(err)
			}
			if err := tools.ValidateJSONSchemaValue(action.OutputSchema, normalized); err != nil {
				t.Fatalf("output contract: %v; output=%#v", err, normalized)
			}
		})
	}
}

func TestPresetAuthMethodsExposeOnlyAddressAndAuthorizationCode(t *testing.T) {
	definition := ProviderDefinitions()[0]
	for _, method := range definition.AuthMethods {
		if method.ID == AccountCustomAuthMethodID || method.ID == OrganizationCustomAuthMethodID {
			if len(method.Fields) != 8 {
				t.Fatalf("custom method %s field count = %d, want 8", method.ID, len(method.Fields))
			}
			continue
		}
		if len(method.Fields) != 2 || method.Fields[0].Key != "email_address" || method.Fields[1].Key != "app_password" {
			t.Fatalf("preset method %s fields = %#v", method.ID, method.Fields)
		}
		if method.SetupGuide == nil || !method.SetupGuide.ExpandedByDefault {
			t.Fatalf("preset method %s should expose an expanded setup guide", method.ID)
		}
	}
}

func TestResolveSettingsUsesQQPreset(t *testing.T) {
	connection := &integrations.ResolvedConnection{
		ID: "qq-connection", IntegrationID: IntegrationStandardMail, DriverID: DriverID,
		AuthMethodID: AccountQQAuthMethodID,
		Credentials:  map[string]string{"email_address": "person@qq.com", "app_password": "authorization-code"},
	}
	settings, err := resolveSettings(connection)
	if err != nil {
		t.Fatal(err)
	}
	if settings.IMAPHost != "imap.qq.com" || settings.SMTPHost != "smtp.qq.com" || settings.RequireIMAPID {
		t.Fatalf("unexpected QQ settings: %#v", settings)
	}
}

func TestResolveSettingsUsesNetEasePresetAndRequiresClientID(t *testing.T) {
	for _, test := range []struct {
		email, imapHost, smtpHost string
	}{
		{"person@163.com", "imap.163.com", "smtp.163.com"},
		{"person@126.com", "imap.126.com", "smtp.126.com"},
		{"person@yeah.net", "imap.yeah.net", "smtp.yeah.net"},
	} {
		connection := &integrations.ResolvedConnection{
			ID: "netease-connection", IntegrationID: IntegrationStandardMail, DriverID: DriverID,
			AuthMethodID: AccountNetEaseAuthMethodID,
			Credentials:  map[string]string{"email_address": test.email, "app_password": "authorization-code"},
		}
		settings, err := resolveSettings(connection)
		if err != nil {
			t.Fatalf("resolve %s: %v", test.email, err)
		}
		if settings.IMAPHost != test.imapHost || settings.SMTPHost != test.smtpHost || !settings.RequireIMAPID {
			t.Fatalf("unexpected NetEase settings for %s: %#v", test.email, settings)
		}
	}
}

func TestPresetAuthMethodRejectsMismatchedMailboxDomain(t *testing.T) {
	connection := &integrations.ResolvedConnection{
		ID: "wrong-provider", IntegrationID: IntegrationStandardMail, DriverID: DriverID,
		AuthMethodID: AccountQQAuthMethodID,
		Credentials:  map[string]string{"email_address": "person@163.com", "app_password": "authorization-code"},
	}
	if _, err := resolveSettings(connection); integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput {
		t.Fatalf("error = %v", err)
	}
}

func TestIMAPClientIDCommandDoesNotContainCredentials(t *testing.T) {
	command := imapClientIDCommand().Command()
	if command.Name != "ID" || len(command.Arguments) != 1 {
		t.Fatalf("unexpected ID command: %#v", command)
	}
	values, ok := command.Arguments[0].([]interface{})
	if !ok || len(values) != 6 {
		t.Fatalf("unexpected ID arguments: %#v", command.Arguments)
	}
	joined := strings.ToLower(strings.TrimSpace(strings.Join([]string{values[0].(string), values[1].(string), values[2].(string), values[3].(string), values[4].(string), values[5].(string)}, " ")))
	if strings.Contains(joined, "password") || strings.Contains(joined, "authorization") {
		t.Fatalf("ID command leaks credential metadata: %q", joined)
	}
}

func TestResolveSettingsUsesConfiguredStandardEndpoints(t *testing.T) {
	connection := standardConnection()
	settings, err := resolveSettings(connection)
	if err != nil {
		t.Fatal(err)
	}
	if settings.IMAPHost != "imap.example.com" || settings.IMAPPort != 993 || settings.SMTPHost != "smtp.example.com" || !settings.ReadSupported {
		t.Fatalf("unexpected settings: %#v", settings)
	}
}

func TestResolveSettingsRejectsUnsafeCustomSMTP(t *testing.T) {
	connection := standardConnection()
	connection.Credentials["smtp_host"] = "127.0.0.1"
	if _, err := resolveSettings(connection); integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput {
		t.Fatalf("error = %v", err)
	}
}

func TestMIMEMessageHidesBCCAndContainsStableHeaders(t *testing.T) {
	id, raw, err := buildMIMEMessage("sender@example.com", outboundMessage{To: []string{"to@example.com"}, CC: []string{"cc@example.com"}, BCC: []string{"hidden@example.com"}, Subject: "测试 subject", Body: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.HasPrefix(id, "<") || !strings.Contains(text, "Message-ID: "+id) || strings.Contains(text, "hidden@example.com") || !strings.Contains(text, "Content-Transfer-Encoding: base64") {
		t.Fatalf("unsafe or incomplete MIME:\n%s", text)
	}
}

func TestMessageReferenceRoundTripAndValidation(t *testing.T) {
	encoded := encodeMessageRef(messageReference{Version: 1, Connection: "connection-1", Folder: "INBOX", UIDValidity: 9, UID: 42})
	decoded, err := decodeMessageRef(encoded, "connection-1")
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Folder != "INBOX" || decoded.UID != 42 || decoded.UIDValidity != 9 {
		t.Fatalf("decoded = %#v", decoded)
	}
	if _, err := decodeMessageRef(base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"c":"connection-1","f":"INBOX","uv":0,"u":1}`)), "connection-1"); err == nil {
		t.Fatal("expected invalid UIDVALIDITY")
	}
	if _, err := decodeMessageRef(encoded, "connection-2"); err == nil {
		t.Fatal("expected connection-bound reference rejection")
	}
}

func TestResolveSettingsRejectsUnsafeCustomIMAP(t *testing.T) {
	connection := standardConnection()
	connection.Credentials["imap_host"] = "127.0.0.1"
	if _, err := resolveSettings(connection); integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput {
		t.Fatalf("error = %v", err)
	}
}

func TestValidatePublicMailHostRejectsLocalhost(t *testing.T) {
	_, err := resolvePublicMailHost(context.Background(), "localhost")
	if integrations.ErrorCode(err) != integrations.ErrorCodeAccessDenied {
		t.Fatalf("error = %v", err)
	}
}

func standardConnection() *integrations.ResolvedConnection {
	return &integrations.ResolvedConnection{
		ID: "connection-1", IntegrationID: IntegrationStandardMail, DriverID: DriverID,
		AuthMethodID: AccountCustomAuthMethodID,
		Credentials: map[string]string{
			"email_address": "person@example.com", "app_password": "authorization-code",
			"imap_host": "imap.example.com", "imap_port": "993",
			"smtp_host": "smtp.example.com", "smtp_port": "465", "smtp_security": "implicit_tls",
		},
	}
}
