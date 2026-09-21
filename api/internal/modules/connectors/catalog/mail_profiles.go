package catalog

import (
	"encoding/json"
	"net/mail"
	"strings"
	"unicode"
)

const ZGIMetadataSchemaVersion15 = "1.5.0"

// MailProfile is a compiled provider contract, never a client-supplied server configuration.
type MailProfile struct {
	ID, Name, NameZH, DriverID, AuthMethodID, Handler, PasswordField string
	IMAPHost, SMTPHost, Domain                                       string
	RequireIMAPID                                                    bool
}

func MailProfileFor(id string) (MailProfile, bool) {
	switch id {
	case "gmail":
		return MailProfile{ID: id, Name: "Gmail", NameZH: "Gmail", DriverID: "gmail-mail-v1", AuthMethodID: "gmail.app_password", Handler: "gmail-app-password-v1", PasswordField: "app_password", IMAPHost: "imap.gmail.com", SMTPHost: "smtp.gmail.com"}, true
	case "qqmail":
		return MailProfile{ID: id, Name: "QQ Mail", NameZH: "QQ 邮箱", DriverID: "qqmail-mail-v1", AuthMethodID: "qqmail.authorization_code", Handler: "qqmail-authorization-code-v1", PasswordField: "authorization_code", IMAPHost: "imap.qq.com", SMTPHost: "smtp.qq.com", Domain: "qq.com"}, true
	case "netease163":
		return MailProfile{ID: id, Name: "163 Mail", NameZH: "163 邮箱", DriverID: "netease163-mail-v1", AuthMethodID: "netease163.authorization_code", Handler: "netease163-authorization-code-v1", PasswordField: "authorization_code", IMAPHost: "imap.163.com", SMTPHost: "smtp.163.com", Domain: "163.com", RequireIMAPID: true}, true
	default:
		return MailProfile{}, false
	}
}

func MatchMailProfile(connector, provider, driver, auth string) (MailProfile, bool) {
	p, ok := MailProfileFor(connector)
	return p, ok && p.ID == provider && p.DriverID == driver && p.AuthMethodID == auth
}

func MailOperationsFor(provider, usage string) []string {
	if _, ok := MailProfileFor(provider); !ok {
		return nil
	}
	var result []string
	if usage == "read" || usage == "read_send" {
		result = append(result, provider+".folder.list", provider+".message.list", provider+".message.get")
	}
	if usage == "send" || usage == "read_send" {
		result = append(result, provider+".message.send")
	}
	return result
}

func MailDefinition(id string) (Definition, bool) {
	p, ok := MailProfileFor(id)
	if !ok {
		return Definition{}, false
	}
	d := GmailDefinition()
	if id == "gmail" {
		return d, true
	}
	d.ID, d.ProviderID, d.PluginID = id, id, "zgi."+id
	d.DriverID, d.AuthMethodID, d.Icon = p.DriverID, p.AuthMethodID, id
	d.Name, d.NameI18n = p.Name, LocalizedText{"en-US": p.Name, "zh-Hans": p.NameZH}
	d.Description = "Read and explicitly send mail using a client authorization code."
	d.DescriptionI18n = LocalizedText{"en-US": d.Description, "zh-Hans": "使用邮箱客户端授权码读取邮件，并在确认后发送邮件。"}
	d.ContractVersion, d.ExtensionSchemaVersion = 6, ZGIMetadataSchemaVersion15
	d.Setup.Handler, d.Setup.AuthenticationKind = p.Handler, "authorization_code"
	d.CredentialFields[1].Name, d.CredentialFields[1].Label = p.PasswordField, "Client authorization code"
	d.CredentialFields[1].LabelI18n = LocalizedText{"en-US": "Client authorization code", "zh-Hans": "客户端授权码"}
	for i := range d.Operations {
		op := &d.Operations[i]
		op.ID = id + strings.TrimPrefix(op.ID, "gmail")
		op.ToolName = id + strings.TrimPrefix(op.ToolName, "gmail")
		if op.ExternalDestination != "" {
			op.ExternalDestination = id
		}
		op.OutputSchema = mailOutputFor(id, op.ID)
	}
	encoded, _ := json.Marshal(d)
	_ = json.Unmarshal(encoded, &d)
	return d, true
}

// NormalizeMailValuesFor preserves the authenticated login and credential case.
// Google app passwords and NetEase client codes may be copied with visual
// whitespace separators; those separators are removed before validation.
func NormalizeMailValuesFor(id string, values map[string]string) error {
	if id == "gmail" {
		return NormalizeMailValues(values)
	}
	p, ok := MailProfileFor(id)
	if !ok || len(values) != 2 {
		return invalid("mail_values")
	}
	raw, emailOK := values["email"]
	code, codeOK := values[p.PasswordField]
	if !emailOK || !codeOK || len(raw) > 256 || len(code) > 256 {
		return invalid("mail_values")
	}
	if strings.IndexFunc(raw, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) >= 0 {
		return invalid("mail_email")
	}
	email := strings.TrimSpace(raw)
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Name != "" || parsed.Address != email || len(email) > 254 {
		return invalid("mail_email")
	}
	at := strings.LastIndexByte(email, '@')
	if at < 1 || strings.ToLower(email[at+1:]) != p.Domain {
		return invalid("mail_email")
	}
	if p.ID == "netease163" {
		code = strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) || r == '\uFEFF' {
				return -1
			}
			return r
		}, code)
	} else {
		code = strings.TrimSpace(code)
	}
	if len(code) != 16 {
		return invalid("mail_password")
	}
	for _, r := range code {
		if r < 33 || r > 126 || p.RequireIMAPID && !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return invalid("mail_password")
		}
	}
	values["email"], values[p.PasswordField] = email[:at+1]+p.Domain, code
	return nil
}
