package catalog

import (
	"encoding/json"
	"net/mail"
	"reflect"
	"strings"
	"unicode"
)

const (
	ZGIMetadataSchemaVersion14             = "1.4.0"
	SetupMethodMailCredentials SetupMethod = "mail_credentials"
)

func (d Definition) IsMailCredentials() bool {
	return d.ExtensionSchemaVersion == ZGIMetadataSchemaVersion14 || d.ExtensionSchemaVersion == ZGIMetadataSchemaVersion15
}

func MailOperations(usage string) []string {
	return MailOperationsFor("gmail", usage)
}

func GmailDefinition() Definition {
	d := Definition{ID: "gmail", PluginID: "zgi.gmail", PluginVersion: "1.0.0", SourceDialect: SourceDialectAgentPluginsV1, SourceSchemaVersion: SourceSchemaVersion1,
		ProviderID: "gmail", DriverID: "gmail-mail-v1", AuthMethodID: "gmail.app_password", Icon: "gmail", Name: "Gmail", Description: "Read and explicitly send mail using a Google app password.",
		NameI18n: LocalizedText{"en-US": "Gmail", "zh-Hans": "Gmail"}, DescriptionI18n: LocalizedText{"en-US": "Read and explicitly send mail using a Google app password.", "zh-Hans": "使用 Google 应用专用密码读取邮件，并在确认后发送邮件。"},
		Classification: ClassificationConnector, CapabilityTypes: []CapabilityType{CapabilityTypeAction, CapabilityTypeData}, SetupMethod: SetupMethodMailCredentials, Ownership: OwnershipPersonal, Surfaces: FirstReleaseSurfaces(), KeyFields: []KeyField{},
		ExtensionSchemaVersion: ZGIMetadataSchemaVersion14, ContractVersion: 5,
		Setup: &SetupDescriptor{Handler: "gmail-app-password-v1", Methods: []string{"manual"}, AuthenticationKind: "app_password", VerificationPolicy: "selected_mail_protocols"},
		CredentialFields: []CredentialField{
			{Name: "email", Label: "Email address", LabelI18n: LocalizedText{"en-US": "Email address", "zh-Hans": "邮箱地址"}, Required: true, Type: "string", MaxLength: 254},
			{Name: "app_password", Label: "Google app password", LabelI18n: LocalizedText{"en-US": "Google app password", "zh-Hans": "Google 应用专用密码"}, Required: true, Secret: true, Type: "string", MaxLength: 256},
		},
	}
	for _, row := range []struct {
		id, en, zh string
		input      map[string]any
	}{
		{"folder.list", "List mail folders", "列出邮箱文件夹", objectSchema(nil)},
		{"message.list", "List mail messages", "列出邮件", objectSchema(map[string]any{"mailbox": textSchema(1024), "cursor": textSchema(8192), "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 50}})},
		{"message.get", "Read a mail message", "读取邮件正文", objectSchema(map[string]any{"message_handle": textSchema(8192)}, "message_handle")},
		{"message.send", "Send a mail message", "发送邮件", objectSchema(map[string]any{"to": map[string]any{"type": "array", "items": textSchema(254), "minItems": 1, "maxItems": 20}, "subject": map[string]any{"type": "string", "maxLength": 998}, "text": textSchema(262144)}, "to", "subject", "text")},
	} {
		op := messagingOperation("gmail", row.id, row.en, row.zh, "account", row.id == "message.send", row.input)
		op.OutputSchema = mailOutput(op.ID)
		d.Operations = append(d.Operations, op)
	}
	encoded, _ := json.Marshal(d)
	_ = json.Unmarshal(encoded, &d)
	return d
}

func mailOutput(id string) map[string]any {
	return mailOutputFor("gmail", id)
}

func mailOutputFor(provider, id string) map[string]any {
	props := map[string]any{"provider": map[string]any{"const": provider}, "operation": map[string]any{"const": id}}
	required := []string{"provider", "operation"}
	switch strings.TrimPrefix(id, provider+".") {
	case "folder.list":
		props["folders"] = map[string]any{"type": "array", "maxItems": 200, "items": objectSchema(map[string]any{"name": textSchema(1024)}, "name")}
		required = append(required, "folders")
	case "message.list":
		props["messages"] = map[string]any{"type": "array", "maxItems": 50, "items": objectSchema(map[string]any{"message_handle": textSchema(8192), "subject": map[string]any{"type": "string", "maxLength": 998}, "from": map[string]any{"type": "string", "maxLength": 1024}, "date": map[string]any{"type": "string", "maxLength": 64}}, "message_handle", "subject", "from", "date")}
		props["next_cursor"] = map[string]any{"type": "string", "maxLength": 8192}
		required = append(required, "messages", "next_cursor")
	case "message.get":
		props["text"] = map[string]any{"type": "string", "maxLength": 262144}
		props["truncated"] = map[string]any{"type": "boolean"}
		required = append(required, "text", "truncated")
	case "message.send":
		props["status"] = map[string]any{"const": "accepted_by_server"}
		props["message_id"] = textSchema(256)
		required = append(required, "status", "message_id")
	}
	return objectSchema(props, required...)
}

func validateMailContract(d Definition) error {
	c, ok := MailDefinition(d.ID)
	if !ok || d.ContractVersion != c.ContractVersion || d.ExtensionSchemaVersion != c.ExtensionSchemaVersion || d.ID != c.ID || d.ProviderID != c.ProviderID || d.DriverID != c.DriverID || d.AuthMethodID != c.AuthMethodID || d.SetupMethod != c.SetupMethod || len(d.KeyFields) != 0 || !reflect.DeepEqual(d.Setup, c.Setup) || len(d.CredentialFields) != 2 || len(d.Operations) != 4 {
		return invalid("mail_contract")
	}
	for _, want := range c.CredentialFields {
		count := 0
		for _, f := range d.CredentialFields {
			if f.Name == want.Name {
				count++
				if f.Required != want.Required || f.Secret != want.Secret || f.Type != want.Type || f.MaxLength != want.MaxLength || len(f.Enum) != 0 || strings.TrimSpace(f.Label) == "" || !validLocalizedText(f.LabelI18n) {
					return invalid("mail_fields")
				}
			}
		}
		if count != 1 {
			return invalid("mail_fields")
		}
	}
	for _, op := range d.Operations {
		found := false
		for _, want := range c.Operations {
			want.SkillID = op.SkillID
			a, _ := digestJSON(canonicalOperationContract(op))
			b, _ := digestJSON(canonicalOperationContract(want))
			if a == b && op.CredentialSubject == "account" && len(op.RequiredScopes) == 0 {
				found = true
			}
		}
		if !found {
			return invalid("mail_operations")
		}
	}
	return nil
}

// NormalizeMailValues mutates only this disposable credential map. It does not
// infer account equivalence from Gmail dots, plus tags, aliases or domain MX.
// Provider-specific credential validity is left to the real protocol verifier;
// this function only enforces safe input boundaries.
func NormalizeMailValues(values map[string]string) error {
	if len(values) != 2 {
		return invalid("mail_values")
	}
	email := strings.TrimSpace(values["email"])
	if len(email) > 254 || strings.IndexFunc(email, unicode.IsControl) >= 0 {
		return invalid("mail_email")
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Name != "" || address.Address != email {
		return invalid("mail_email")
	}
	at := strings.LastIndexByte(email, '@')
	if at < 1 || at == len(email)-1 {
		return invalid("mail_email")
	}
	email = email[:at+1] + strings.ToLower(email[at+1:])
	password := values["app_password"]
	if len(password) > 256 {
		return invalid("mail_password")
	}
	password = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n', '\u00a0':
			return -1
		}
		return r
	}, password)
	for _, r := range password {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return invalid("mail_password")
		}
	}
	values["email"], values["app_password"] = email, password
	return nil
}
