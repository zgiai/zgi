package mail

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

const (
	AccountQQAuthMethodID           = "qq_mail_account"
	OrganizationQQAuthMethodID      = "organization_qq_mail_account"
	AccountNetEaseAuthMethodID      = "netease_mail_account"
	OrganizationNetEaseAuthMethodID = "organization_netease_mail_account"
	AccountCustomAuthMethodID       = "standard_smtp_account"
	OrganizationCustomAuthMethodID  = "organization_standard_smtp_account"
)

type mailPreset struct {
	ID            string
	DisplayName   string
	IMAPHost      string
	IMAPPort      int
	SMTPHost      string
	SMTPPort      int
	SMTPSecurity  string
	RequireIMAPID bool
}

var mailPresetsByDomain = map[string]mailPreset{
	"qq.com": {
		ID: "qq", DisplayName: "QQ Mail",
		IMAPHost: "imap.qq.com", IMAPPort: 993,
		SMTPHost: "smtp.qq.com", SMTPPort: 465, SMTPSecurity: "implicit_tls",
	},
	"163.com": {
		ID: "netease_163", DisplayName: "NetEase 163 Mail",
		IMAPHost: "imap.163.com", IMAPPort: 993,
		SMTPHost: "smtp.163.com", SMTPPort: 465, SMTPSecurity: "implicit_tls", RequireIMAPID: true,
	},
	"126.com": {
		ID: "netease_126", DisplayName: "NetEase 126 Mail",
		IMAPHost: "imap.126.com", IMAPPort: 993,
		SMTPHost: "smtp.126.com", SMTPPort: 465, SMTPSecurity: "implicit_tls", RequireIMAPID: true,
	},
	"yeah.net": {
		ID: "netease_yeah", DisplayName: "NetEase Yeah Mail",
		IMAPHost: "imap.yeah.net", IMAPPort: 993,
		SMTPHost: "smtp.yeah.net", SMTPPort: 465, SMTPSecurity: "implicit_tls", RequireIMAPID: true,
	},
}

func presetForAuthMethod(authMethodID, emailAddress string) (mailPreset, bool, error) {
	authMethodID = strings.ToLower(strings.TrimSpace(authMethodID))
	domain := emailDomain(emailAddress)
	preset, exists := mailPresetsByDomain[domain]
	switch authMethodID {
	case AccountQQAuthMethodID, OrganizationQQAuthMethodID:
		if !exists || preset.ID != "qq" {
			return mailPreset{}, true, integrations.NewError(integrations.ErrorCodeInvalidInput, "QQ Mail connection requires an @qq.com mailbox address", nil)
		}
		return preset, true, nil
	case AccountNetEaseAuthMethodID, OrganizationNetEaseAuthMethodID:
		if !exists || !strings.HasPrefix(preset.ID, "netease_") {
			return mailPreset{}, true, integrations.NewError(integrations.ErrorCodeInvalidInput, "NetEase Mail connection requires an @163.com, @126.com, or @yeah.net mailbox address", nil)
		}
		return preset, true, nil
	case AccountCustomAuthMethodID, OrganizationCustomAuthMethodID:
		return mailPreset{}, false, nil
	default:
		return mailPreset{}, false, integrations.NewError(integrations.ErrorCodeConnectionInvalid, "mail authentication method is invalid", nil)
	}
}

func emailDomain(emailAddress string) string {
	separator := strings.LastIndex(emailAddress, "@")
	if separator < 0 || separator == len(emailAddress)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(emailAddress[separator+1:]))
}

func allMailAuthMethodIDs() []string {
	return []string{
		AccountQQAuthMethodID,
		OrganizationQQAuthMethodID,
		AccountNetEaseAuthMethodID,
		OrganizationNetEaseAuthMethodID,
		AccountCustomAuthMethodID,
		OrganizationCustomAuthMethodID,
	}
}
