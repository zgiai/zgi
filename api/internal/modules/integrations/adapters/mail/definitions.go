package mail

import (
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	DriverID = "imap-smtp"

	IntegrationStandardMail = "standard-mail"

	ActionAccountGet    = "mail.account.get"
	ActionFolderList    = "mail.folder.list"
	ActionMessageSearch = "mail.message.search"
	ActionMessageGet    = "mail.message.get"
	ActionMessageSend   = "mail.message.send"
	ActionMessageReply  = "mail.message.reply"

	ScopeIdentity = "mail:identity"
	ScopeRead     = "mail:read"
	ScopeSend     = "mail:send"
)

type providerSpec struct {
	ID, Name, ChineseName, Description, ChineseDescription, DocumentationURL string
}

var providerSpecs = []providerSpec{
	{
		ID: IntegrationStandardMail, Name: "Standard Mail", ChineseName: "通用邮箱",
		Description:        "Connect a standards-based IMAP and SMTP mailbox with TLS to search, read, send, and reply to email.",
		ChineseDescription: "连接 QQ、网易或其他支持 IMAP/SMTP 的邮箱，可搜索、读取、发送和回复邮件。",
		DocumentationURL:   "https://www.rfc-editor.org/rfc/rfc9051",
	},
}

func ProviderDefinitions() []integrations.ProviderDefinition {
	definitions := make([]integrations.ProviderDefinition, 0, len(providerSpecs))
	for _, spec := range providerSpecs {
		definitions = append(definitions, providerDefinition(spec))
	}
	return definitions
}

func providerDefinition(spec providerSpec) integrations.ProviderDefinition {
	actions := actionsFor()
	return integrations.ProviderDefinition{
		ID: spec.ID, DriverID: DriverID, Name: spec.Name,
		NameI18n:    localized(spec.Name, spec.ChineseName),
		Description: spec.Description, DescriptionI18n: localized(spec.Description, spec.ChineseDescription),
		Author: "ZGI", Icon: "mail", Tags: []string{"email", "communication", "external"},
		TagLabelsI18n: integrations.LocalizedLabelMap{
			"email": localized("Email", "电子邮件"), "communication": localized("Communication", "沟通协作"), "external": localized("External", "外部服务"),
		},
		Categories:           []string{"communication"},
		CategoryLabelsI18n:   integrations.LocalizedLabelMap{"communication": localized("Communication", "沟通协作")},
		DocumentationURL:     spec.DocumentationURL,
		DocumentationURLI18n: localized(spec.DocumentationURL, spec.DocumentationURL),
		AuthMethods: []integrations.AuthMethodDefinition{
			presetMailAuthMethod(AccountQQAuthMethodID, integrations.ConnectionCredentialSourceAccount, "QQ Mail", "QQ 邮箱", qqSetupGuide()),
			presetMailAuthMethod(OrganizationQQAuthMethodID, integrations.ConnectionCredentialSourceOrganization, "Shared QQ Mail", "QQ 邮箱（共享）", qqSetupGuide()),
			presetMailAuthMethod(AccountNetEaseAuthMethodID, integrations.ConnectionCredentialSourceAccount, "NetEase Mail", "网易邮箱", netEaseSetupGuide()),
			presetMailAuthMethod(OrganizationNetEaseAuthMethodID, integrations.ConnectionCredentialSourceOrganization, "Shared NetEase Mail", "网易邮箱（共享）", netEaseSetupGuide()),
			customMailAuthMethod(spec, AccountCustomAuthMethodID, integrations.ConnectionCredentialSourceAccount),
			customMailAuthMethod(spec, OrganizationCustomAuthMethodID, integrations.ConnectionCredentialSourceOrganization),
		},
		HealthProbe: integrations.HealthProbeDefinition{
			Supported: true, MayIncurCost: false,
			Description:     "Authenticates to the configured mail service without sending email.",
			DescriptionI18n: localized("Authenticates to the configured mail service without sending email.", "仅验证邮箱服务登录状态，不会发送邮件。"),
		},
		Scopes: []integrations.ProviderScopeDefinition{
			{ID: ScopeIdentity, Label: "Verify mailbox identity", LabelI18n: localized("Verify mailbox identity", "验证邮箱身份"), Category: integrations.ProviderScopeCategoryIdentity, Access: integrations.ProviderScopeAccessIdentity},
			{ID: ScopeRead, Label: "Read mailbox messages", LabelI18n: localized("Read mailbox messages", "读取邮箱邮件"), Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead, Broad: true},
			{ID: ScopeSend, Label: "Send email", LabelI18n: localized("Send email", "发送邮件"), Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite, Broad: true},
		},
		Actions: actions,
	}
}

func presetMailAuthMethod(id string, source integrations.ConnectionCredentialSource, label, zhLabel string, guide *integrations.AuthSetupGuideDefinition) integrations.AuthMethodDefinition {
	fields := []integrations.CredentialFieldDefinition{
		credentialField("email_address", "Email address", "邮箱地址", integrations.CredentialFieldInputText, true, false),
		credentialField("app_password", "Authorization code", "授权码", integrations.CredentialFieldInputPassword, true, true),
	}
	fields[0].Description = "Enter the complete mailbox address. Server settings are selected automatically."
	fields[0].DescriptionI18n = localized(fields[0].Description, "填写完整邮箱地址，服务器配置会自动完成。")
	fields[0].Placeholder = "name@example.com"
	fields[0].PlaceholderI18n = localized("name@example.com", "name@example.com")
	fields[1].Description = "Use the provider-generated authorization code, never the normal mailbox password."
	fields[1].DescriptionI18n = localized(fields[1].Description, "填写邮箱服务商生成的授权码，不要填写日常邮箱登录密码。")
	fields[1].Placeholder = "Paste the authorization code"
	fields[1].PlaceholderI18n = localized(fields[1].Placeholder, "粘贴授权码")
	return integrations.AuthMethodDefinition{
		ID: id, Type: integrations.AuthMethodTypeCustomCredential, CredentialSource: source,
		IdentityKind:        integrations.AuthIdentityKindUser,
		AcquisitionStrategy: integrations.AuthAcquisitionStrategyManualForm,
		LifecycleStrategy:   integrations.AuthLifecycleStrategyStatic,
		RequestAuthStrategy: integrations.RequestAuthStrategyProviderCustom,
		ScopeEvidence:       integrations.AuthScopeEvidenceConnectorDeclared,
		Label:               label, LabelI18n: localized(label, zhLabel),
		Description:     "Enter only the mailbox address and provider-generated authorization code. IMAP, SMTP, ports, and TLS are configured automatically.",
		DescriptionI18n: localized("Enter only the mailbox address and provider-generated authorization code. IMAP, SMTP, ports, and TLS are configured automatically.", "只需填写邮箱地址和授权码；IMAP、SMTP、端口和 TLS 均由系统自动配置。"),
		Available:       true, Fields: fields, SetupGuide: guide,
	}
}

func customMailAuthMethod(spec providerSpec, id string, source integrations.ConnectionCredentialSource) integrations.AuthMethodDefinition {
	label := "Other mailbox (advanced)"
	zhLabel := "其他邮箱（高级设置）"
	if source == integrations.ConnectionCredentialSourceOrganization {
		label = "Other shared mailbox (advanced)"
		zhLabel = "其他共享邮箱（高级设置）"
	}
	fields := []integrations.CredentialFieldDefinition{
		credentialField("email_address", "Email address", "邮箱地址", integrations.CredentialFieldInputText, true, false),
		credentialField("username", "Login username", "登录用户名", integrations.CredentialFieldInputText, false, false),
		credentialField("app_password", "Authorization code / app password", "授权码 / 客户端专用密码", integrations.CredentialFieldInputPassword, true, true),
		credentialField("imap_host", "IMAP host", "IMAP 服务器", integrations.CredentialFieldInputText, true, false),
		credentialField("imap_port", "IMAP TLS port", "IMAP TLS 端口", integrations.CredentialFieldInputSelect, true, false),
		credentialField("smtp_host", "SMTP host", "SMTP 服务器", integrations.CredentialFieldInputText, true, false),
		credentialField("smtp_port", "SMTP port", "SMTP 端口", integrations.CredentialFieldInputSelect, true, false),
		credentialField("smtp_security", "SMTP security", "SMTP 安全方式", integrations.CredentialFieldInputSelect, true, false),
	}
	fields[4].Options = []integrations.CredentialFieldOption{
		{Value: "993", Label: "993", LabelI18n: localized("993", "993")},
	}
	fields[6].Options = []integrations.CredentialFieldOption{
		{Value: "465", Label: "465", LabelI18n: localized("465", "465")},
		{Value: "587", Label: "587", LabelI18n: localized("587", "587")},
	}
	fields[7].Options = []integrations.CredentialFieldOption{
		{Value: "implicit_tls", Label: "TLS (recommended)", LabelI18n: localized("TLS (recommended)", "TLS（推荐）")},
		{Value: "starttls", Label: "STARTTLS", LabelI18n: localized("STARTTLS", "STARTTLS")},
	}
	return integrations.AuthMethodDefinition{
		ID: id, Type: integrations.AuthMethodTypeCustomCredential, CredentialSource: source,
		IdentityKind:        integrations.AuthIdentityKindUser,
		AcquisitionStrategy: integrations.AuthAcquisitionStrategyManualForm,
		LifecycleStrategy:   integrations.AuthLifecycleStrategyStatic,
		RequestAuthStrategy: integrations.RequestAuthStrategyProviderCustom,
		ScopeEvidence:       integrations.AuthScopeEvidenceConnectorDeclared,
		Label:               label, LabelI18n: localized(label, zhLabel),
		Description:     "For enterprise or other mailboxes. Enter the provider's IMAP/SMTP endpoints and a dedicated authorization code or app password.",
		DescriptionI18n: localized("For enterprise or other mailboxes. Enter the provider's IMAP/SMTP endpoints and a dedicated authorization code or app password.", "适用于企业邮箱或其他邮箱，需要填写服务商提供的 IMAP/SMTP 地址和授权码。"),
		Available:       true, Fields: fields,
		SetupGuide: &integrations.AuthSetupGuideDefinition{
			DocumentationURL: spec.DocumentationURL,
			Steps: []integrations.AuthSetupStepDefinition{
				{ID: "enable_client_access", Title: "Enable mail client access", TitleI18n: localized("Enable mail client access", "启用邮箱客户端服务"), Description: "Enable IMAP and SMTP in the provider security settings, then copy its official TLS endpoints.", DescriptionI18n: localized("Enable IMAP and SMTP in the provider security settings, then copy its official TLS endpoints.", "在邮箱安全设置中启用 IMAP 和 SMTP，并复制服务商公布的 TLS 服务器地址。"), Action: integrations.AuthSetupStepActionOpenDocumentation},
				{ID: "create_app_password", Title: "Create an authorization code", TitleI18n: localized("Create an authorization code", "生成授权码"), Description: "Create a dedicated authorization code and store it as the app password. Revoking it disconnects ZGI.", DescriptionI18n: localized("Create a dedicated authorization code and store it as the app password. Revoking it disconnects ZGI.", "生成独立授权码并作为客户端专用密码保存；在服务商处撤销授权码即可断开 ZGI。")},
			},
			Notices: []integrations.AuthSetupNoticeDefinition{{ID: "no_login_password", Level: integrations.AuthSetupNoticeLevelWarning, Text: "Do not enter the mailbox login password.", TextI18n: localized("Do not enter the mailbox login password.", "不要填写邮箱登录密码。")}},
		},
	}
}

func qqSetupGuide() *integrations.AuthSetupGuideDefinition {
	return &integrations.AuthSetupGuideDefinition{
		ConsoleURL:        "https://mail.qq.com/",
		DocumentationURL:  "https://hiflow.tencent.com/document/applications/qq-mail/",
		ExpandedByDefault: true,
		Steps: []integrations.AuthSetupStepDefinition{
			{ID: "open_qq_settings", Title: "Open QQ Mail settings", TitleI18n: localized("Open QQ Mail settings", "打开 QQ 邮箱设置"), Description: "Sign in to QQ Mail and open Settings > Account.", DescriptionI18n: localized("Sign in to QQ Mail and open Settings > Account.", "登录 QQ 邮箱网页版，进入“设置 → 账号”。"), Action: integrations.AuthSetupStepActionOpenConsole},
			{ID: "enable_qq_protocols", Title: "Enable IMAP/SMTP", TitleI18n: localized("Enable IMAP/SMTP", "开启 IMAP/SMTP 服务"), Description: "Enable the IMAP/SMTP client service in the account security settings.", DescriptionI18n: localized("Enable the IMAP/SMTP client service in the account security settings.", "在账号安全设置中开启 IMAP/SMTP 客户端服务。")},
			{ID: "create_qq_code", Title: "Generate an authorization code", TitleI18n: localized("Generate an authorization code", "生成授权码"), Description: "Generate and copy a dedicated authorization code, then paste it below.", DescriptionI18n: localized("Generate and copy a dedicated authorization code, then paste it below.", "生成并复制独立授权码，然后粘贴到下方。"), Action: integrations.AuthSetupStepActionOpenDocumentation},
		},
		Notices: []integrations.AuthSetupNoticeDefinition{{ID: "no_qq_password", Level: integrations.AuthSetupNoticeLevelWarning, Text: "Use the QQ Mail authorization code, not the QQ or mailbox login password.", TextI18n: localized("Use the QQ Mail authorization code, not the QQ or mailbox login password.", "请填写 QQ 邮箱授权码，不要填写 QQ 密码或邮箱登录密码。")}},
	}
}

func netEaseSetupGuide() *integrations.AuthSetupGuideDefinition {
	return &integrations.AuthSetupGuideDefinition{
		ConsoleURL:        "https://email.163.com/",
		DocumentationURL:  "https://help.mail.126.com/faqDetail.do?code=d7a5dc8471cd0c0e8b4b8f4f8e49998b374173cfe9171305fa1ce630d7f67ac2a5feb28b66796d3b",
		ExpandedByDefault: true,
		Steps: []integrations.AuthSetupStepDefinition{
			{ID: "open_netease_settings", Title: "Open NetEase Mail settings", TitleI18n: localized("Open NetEase Mail settings", "打开网易邮箱设置"), Description: "Sign in to NetEase Mail and open Settings > POP3/SMTP/IMAP.", DescriptionI18n: localized("Sign in to NetEase Mail and open Settings > POP3/SMTP/IMAP.", "登录网易邮箱网页版，进入“设置 → POP3/SMTP/IMAP”。"), Action: integrations.AuthSetupStepActionOpenConsole},
			{ID: "enable_netease_protocols", Title: "Enable IMAP/SMTP", TitleI18n: localized("Enable IMAP/SMTP", "开启 IMAP/SMTP 服务"), Description: "Enable IMAP/SMTP and complete the provider's identity verification.", DescriptionI18n: localized("Enable IMAP/SMTP and complete the provider's identity verification.", "开启 IMAP/SMTP，并按页面提示完成身份验证。")},
			{ID: "create_netease_code", Title: "Create a client authorization password", TitleI18n: localized("Create a client authorization password", "新增客户端授权密码"), Description: "Copy the generated authorization password immediately; the provider displays it only once.", DescriptionI18n: localized("Copy the generated authorization password immediately; the provider displays it only once.", "立即复制生成的授权密码；服务商只会展示一次。"), Action: integrations.AuthSetupStepActionOpenDocumentation},
		},
		Notices: []integrations.AuthSetupNoticeDefinition{{ID: "no_netease_password", Level: integrations.AuthSetupNoticeLevelWarning, Text: "Use the NetEase client authorization password, not the webmail login password.", TextI18n: localized("Use the NetEase client authorization password, not the webmail login password.", "请填写网易客户端授权密码，不要填写网页版邮箱登录密码。")}},
	}
}

func credentialField(key, label, zh string, input integrations.CredentialFieldInput, required, secret bool) integrations.CredentialFieldDefinition {
	return integrations.CredentialFieldDefinition{Key: key, Label: label, LabelI18n: localized(label, zh), Input: input, Required: required, Secret: secret}
}

func actionsFor() []integrations.ActionDefinition {
	authIDs := allMailAuthMethodIDs()
	actions := []integrations.ActionDefinition{
		readAction(ActionAccountGet, "get_mail_account", "Get mail account", "获取邮箱账号", "Return the connected mailbox identity and supported protocol capabilities.", "返回已连接邮箱的身份和协议能力。", accountOutputSchema(), []string{ScopeIdentity}, authIDs),
		readAction(ActionFolderList, "list_mail_folders", "List mail folders", "列出邮箱文件夹", "List bounded IMAP mail folders.", "列出受限数量的 IMAP 邮箱文件夹。", foldersOutputSchema(), []string{ScopeRead}, authIDs),
		readActionWithInput(ActionMessageSearch, "search_mail_messages", "Search email", "搜索邮件", "Search a mailbox and return bounded message summaries.", "搜索邮箱并返回受限的邮件摘要。", searchInputSchema(), searchOutputSchema(), []string{ScopeRead}, authIDs),
		readActionWithInput(ActionMessageGet, "get_mail_message", "Read email", "读取邮件", "Read one message selected from a previous search result.", "读取先前搜索结果中的一封邮件。", messageRefInputSchema(), getOutputSchema(), []string{ScopeRead}, authIDs),
		writeAction(ActionMessageSend, "send_mail_message", "Send email", "发送邮件", "Send one plain-text email after explicit approval. SMTP acceptance does not guarantee final delivery.", "经明确确认后发送一封纯文本邮件；SMTP 接受不代表最终投递成功。", sendInputSchema(), sendOutputSchema(), []string{ScopeSend}, authIDs, []string{"to"}),
		writeAction(ActionMessageReply, "reply_mail_message", "Reply to email", "回复邮件", "Reply to one previously selected message after explicit approval.", "经明确确认后回复一封先前选择的邮件。", replyInputSchema(), sendOutputSchema(), []string{ScopeRead, ScopeSend}, authIDs, []string{"message_ref"}),
	}
	return actions
}

func readAction(id, tool, name, zh, description, zhDescription string, output map[string]interface{}, scopes, authIDs []string) integrations.ActionDefinition {
	return readActionWithInput(id, tool, name, zh, description, zhDescription, strictObject(nil, nil), output, scopes, authIDs)
}

func readActionWithInput(id, tool, name, zh, description, zhDescription string, input, output map[string]interface{}, scopes, authIDs []string) integrations.ActionDefinition {
	return integrations.ActionDefinition{
		ID: id, ToolName: tool, Name: name, NameI18n: localized(name, zh), Description: description, DescriptionI18n: localized(description, zhDescription),
		InputSchema: input, OutputSchema: output, Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
		DataEgress: true, ExternalDestination: "mail-service", Idempotent: true,
		RequiredScopes: scopes, ScopeLabelsI18n: scopeLabels(scopes), SupportedAuthMethodIDs: authIDs,
		DefaultPolicy:    &integrations.DefaultActionPolicy{Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true},
		SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent},
	}
}

func writeAction(id, tool, name, zh, description, zhDescription string, input, output map[string]interface{}, scopes, authIDs, targetPaths []string) integrations.ActionDefinition {
	return integrations.ActionDefinition{
		ID: id, ToolName: tool, Name: name, NameI18n: localized(name, zh), Description: description, DescriptionI18n: localized(description, zhDescription),
		InputSchema: input, OutputSchema: output, Effect: toolgovernance.EffectExternalSend, RiskLevel: toolgovernance.RiskLevelHigh,
		DataEgress: true, ExternalDestination: "mail-service", Idempotent: false,
		RequiredScopes: scopes, ScopeLabelsI18n: scopeLabels(scopes), SupportedAuthMethodIDs: authIDs,
		DefaultPolicy:        &integrations.DefaultActionPolicy{Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: true},
		SupportedCallers:     []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat},
		SuccessDeduplication: &integrations.SuccessDeduplicationDefinition{TargetArgumentPaths: targetPaths},
	}
}

func localized(en, zh string) integrations.LocalizedText {
	return integrations.LocalizedText{integrations.LocaleEnglishUS: en, integrations.LocaleSimplifiedChinese: zh}
}

func scopeLabels(scopes []string) integrations.LocalizedLabelMap {
	labels := integrations.LocalizedLabelMap{}
	for _, scope := range scopes {
		switch scope {
		case ScopeIdentity:
			labels[scope] = localized("Verify mailbox identity", "验证邮箱身份")
		case ScopeRead:
			labels[scope] = localized("Read mailbox messages", "读取邮箱邮件")
		case ScopeSend:
			labels[scope] = localized("Send email", "发送邮件")
		}
	}
	return labels
}

func strictObject(properties map[string]interface{}, required []string) map[string]interface{} {
	if properties == nil {
		properties = map[string]interface{}{}
	}
	value := map[string]interface{}{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		value["required"] = required
	}
	return value
}

func text(max int) map[string]interface{} {
	return map[string]interface{}{"type": "string", "maxLength": max}
}
func nonBlank(max int) map[string]interface{} {
	return map[string]interface{}{"type": "string", "minLength": 1, "maxLength": max}
}
func arrayOf(item map[string]interface{}, max int) map[string]interface{} {
	return map[string]interface{}{"type": "array", "maxItems": max, "items": item}
}
func titled(schema map[string]interface{}, en, zh string) map[string]interface{} {
	schema["title"] = en
	schema["title_i18n"] = map[string]interface{}{"en-US": en, "zh-Hans": zh}
	return schema
}

func sendInputSchema() map[string]interface{} {
	return strictObject(map[string]interface{}{
		"to": titled(nonBlank(4000), "Recipients", "收件人"), "cc": titled(text(4000), "CC", "抄送"), "bcc": titled(text(4000), "BCC", "密送"), "subject": titled(nonBlank(998), "Subject", "主题"), "body_text": titled(nonBlank(100000), "Plain-text body", "纯文本正文"),
	}, []string{"to", "subject", "body_text"})
}
func replyInputSchema() map[string]interface{} {
	return strictObject(map[string]interface{}{
		"message_ref": titled(nonBlank(2048), "Message reference", "邮件引用"), "body_text": titled(nonBlank(100000), "Plain-text reply", "纯文本回复"), "reply_all": titled(map[string]interface{}{"type": "boolean", "default": false}, "Reply all", "回复全部"),
	}, []string{"message_ref", "body_text"})
}
func searchInputSchema() map[string]interface{} {
	return strictObject(map[string]interface{}{
		"folder": titled(map[string]interface{}{"type": "string", "maxLength": 512, "default": "INBOX"}, "Mail folder", "邮箱文件夹"), "query": titled(text(500), "Search text", "搜索文本"), "from": titled(text(320), "Sender", "发件人"), "subject": titled(text(500), "Subject contains", "主题包含"),
		"unread_only": titled(map[string]interface{}{"type": "boolean", "default": false}, "Unread only", "仅未读"), "max_results": titled(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 20, "default": 10}, "Maximum results", "最大结果数"),
	}, nil)
}
func messageRefInputSchema() map[string]interface{} {
	return strictObject(map[string]interface{}{"message_ref": titled(nonBlank(2048), "Message reference", "邮件引用")}, []string{"message_ref"})
}
func accountOutputSchema() map[string]interface{} {
	return strictObject(map[string]interface{}{
		"provider": nonBlank(64), "account": strictObject(map[string]interface{}{"email": nonBlank(320), "display_name": text(255), "read_supported": map[string]interface{}{"type": "boolean"}, "send_supported": map[string]interface{}{"type": "boolean"}}, []string{"email", "display_name", "read_supported", "send_supported"}),
	}, []string{"provider", "account"})
}
func foldersOutputSchema() map[string]interface{} {
	return strictObject(map[string]interface{}{"provider": nonBlank(64), "folders": arrayOf(strictObject(map[string]interface{}{"name": nonBlank(512), "attributes": arrayOf(text(100), 20)}, []string{"name", "attributes"}), 100)}, []string{"provider", "folders"})
}
func searchOutputSchema() map[string]interface{} {
	return strictObject(map[string]interface{}{"provider": nonBlank(64), "messages": arrayOf(messageSummarySchema(), 20)}, []string{"provider", "messages"})
}
func messageSummarySchema() map[string]interface{} {
	return strictObject(map[string]interface{}{"message_ref": nonBlank(2048), "subject": text(998), "from": text(1000), "to": text(4000), "date": text(64), "unread": map[string]interface{}{"type": "boolean"}, "size": map[string]interface{}{"type": "integer", "minimum": 0}}, []string{"message_ref", "subject", "from", "to", "date", "unread", "size"})
}
func getOutputSchema() map[string]interface{} {
	return strictObject(map[string]interface{}{"provider": nonBlank(64), "message": strictObject(map[string]interface{}{"message_ref": nonBlank(2048), "message_id": text(998), "subject": text(998), "from": text(1000), "to": text(4000), "cc": text(4000), "date": text(64), "body_text": text(100000), "attachments": arrayOf(strictObject(map[string]interface{}{"filename": text(255), "content_type": text(255), "size": map[string]interface{}{"type": "integer", "minimum": 0}}, []string{"filename", "content_type", "size"}), 50)}, []string{"message_ref", "message_id", "subject", "from", "to", "cc", "date", "body_text", "attachments"})}, []string{"provider", "message"})
}
func sendOutputSchema() map[string]interface{} {
	return strictObject(map[string]interface{}{"provider": nonBlank(64), "message": strictObject(map[string]interface{}{"message_id": nonBlank(998), "accepted_recipients": arrayOf(nonBlank(320), 40), "smtp_accepted": map[string]interface{}{"const": true}}, []string{"message_id", "accepted_recipients", "smtp_accepted"})}, []string{"provider", "message"})
}
