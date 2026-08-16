package wecom

import (
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	IntegrationID         = "wecom"
	DriverID              = "wecom-suite-open-api"
	AuthMethodID          = "organization_wecom_suite_app"
	ActionAppGet          = "wecom.app.get"
	ActionDepartmentList  = "wecom.department.list"
	ActionContactSearch   = "wecom.contact.search"
	ActionUserGet         = "wecom.user.get"
	ActionMessageSendUser = "wecom.message.send_user"
	ScopeApp              = "wecom:app"
	ScopeContacts         = "wecom:contacts:read"
	ScopeSend             = "wecom:message:send"
)

func ProviderDefinition() integrations.ProviderDefinition {
	return integrations.ProviderDefinition{
		ID: IntegrationID, DriverID: DriverID, Name: "WeCom", NameI18n: loc("WeCom", "企业微信"),
		Description:     "Install the ZGI WeCom third-party application for an organization, then resolve visible members and send application messages with explicit approval.",
		DescriptionI18n: loc("Install the ZGI WeCom third-party application for an organization, then resolve visible members and send application messages with explicit approval.", "为组织授权安装 ZGI 企业微信第三方应用，用于查找可见成员，并在明确确认后发送应用消息。"),
		Author:          "ZGI", Icon: "message-square", Tags: []string{"communication", "enterprise", "external"},
		TagLabelsI18n: integrations.LocalizedLabelMap{"communication": loc("Communication", "沟通协作"), "enterprise": loc("Enterprise", "企业服务"), "external": loc("External", "外部服务")},
		Categories:    []string{"communication"}, CategoryLabelsI18n: integrations.LocalizedLabelMap{"communication": loc("Communication", "沟通协作")},
		DocumentationURL: "https://developer.work.weixin.qq.com/document/path/90594", DocumentationURLI18n: loc("https://developer.work.weixin.qq.com/document/path/90594", "https://developer.work.weixin.qq.com/document/path/90594"),
		AuthMethods: []integrations.AuthMethodDefinition{authMethod()},
		HealthProbe: integrations.HealthProbeDefinition{Supported: true, MayIncurCost: false, Description: "Exchanges the permanent authorization for a short-lived enterprise token and reads the authorized application profile without sending a message.", DescriptionI18n: loc("Exchanges the permanent authorization for a short-lived enterprise token and reads the authorized application profile without sending a message.", "使用永久授权换取短期企业凭证并读取已授权应用资料，不会发送消息。")},
		Scopes: []integrations.ProviderScopeDefinition{
			{ID: ScopeApp, Label: "Read application identity", LabelI18n: loc("Read application identity", "读取应用身份"), Category: integrations.ProviderScopeCategoryIdentity, Access: integrations.ProviderScopeAccessIdentity},
			{ID: ScopeContacts, Label: "Read visible organization members", LabelI18n: loc("Read visible organization members", "读取应用可见范围内的组织成员"), Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead, Broad: true},
			{ID: ScopeSend, Label: "Send application messages", LabelI18n: loc("Send application messages", "发送应用消息"), Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite, Broad: true},
		}, Actions: actions(),
	}
}

func authMethod() integrations.AuthMethodDefinition {
	return integrations.AuthMethodDefinition{
		ID: AuthMethodID, Type: integrations.AuthMethodTypeProviderInstall, CredentialSource: integrations.ConnectionCredentialSourceOrganization, IdentityKind: integrations.AuthIdentityKindApplication,
		AcquisitionStrategy: integrations.AuthAcquisitionStrategyProviderInstallRedirect, LifecycleStrategy: integrations.AuthLifecycleStrategyProviderExchange, RequestAuthStrategy: integrations.RequestAuthStrategyProviderCustom,
		ScopeEvidence: integrations.AuthScopeEvidenceConnectorDeclared,
		Label:         "Authorize and install the WeCom third-party application", LabelI18n: loc("Authorize and install the WeCom third-party application", "授权安装企业微信第三方应用"),
		Description:     "A WeCom system administrator reviews the requested permissions and installs the application. ZGI stores the resulting permanent authorization encrypted; no Corp ID, AgentID, or Secret is entered manually.",
		DescriptionI18n: loc("A WeCom system administrator reviews the requested permissions and installs the application. ZGI stores the resulting permanent authorization encrypted; no Corp ID, AgentID, or Secret is entered manually.", "由企业微信系统管理员审核权限并安装应用。ZGI 会加密保存返回的永久授权，无需手工填写企业 ID、AgentID 或 Secret。"), Available: true,
		SetupGuide: &integrations.AuthSetupGuideDefinition{ConsoleURL: "https://open.work.weixin.qq.com/wwopen/developers/tools", DocumentationURL: "https://developer.work.weixin.qq.com/document/path/90597", ExpandedByDefault: true, Steps: []integrations.AuthSetupStepDefinition{
			{ID: "provider_configuration", Title: "Complete the service-provider application configuration", TitleI18n: loc("Complete the service-provider application configuration", "完成服务商第三方应用配置"), Description: "The ZGI deployment administrator configures the install callback, data callback, command callback, trusted domain, Suite ID, Suite Secret, Token, and EncodingAESKey.", DescriptionI18n: loc("The ZGI deployment administrator configures the install callback, data callback, command callback, trusted domain, Suite ID, Suite Secret, Token, and EncodingAESKey.", "由 ZGI 部署管理员配置安装完成回调、数据回调、指令回调、可信域名、Suite ID、Suite Secret、Token 和 EncodingAESKey。"), Action: integrations.AuthSetupStepActionOpenConsole},
			{ID: "start_install", Title: "Start authorization from ZGI", TitleI18n: loc("Start authorization from ZGI", "从 ZGI 发起授权安装"), Description: "Open the WeCom authorization page from this dialog. The authorization state is single-use and expires after ten minutes.", DescriptionI18n: loc("Open the WeCom authorization page from this dialog. The authorization state is single-use and expires after ten minutes.", "从此对话框打开企业微信授权页；授权状态仅可使用一次，并会在十分钟后失效。")},
			{ID: "review_permissions", Title: "Review permissions and visibility", TitleI18n: loc("Review permissions and visibility", "审核权限与可见范围"), Description: "A WeCom system administrator confirms the long-term application permissions and member visibility before installing.", DescriptionI18n: loc("A WeCom system administrator confirms the long-term application permissions and member visibility before installing.", "企业微信系统管理员在安装前确认长期所需的应用权限与成员可见范围。")},
			{ID: "verify_connection", Title: "Return to ZGI and verify", TitleI18n: loc("Return to ZGI and verify", "返回 ZGI 并验证连接"), Description: "ZGI exchanges the one-time code for a permanent authorization, then uses short-lived enterprise tokens at runtime.", DescriptionI18n: loc("ZGI exchanges the one-time code for a permanent authorization, then uses short-lived enterprise tokens at runtime.", "ZGI 使用一次性临时码换取永久授权，运行时再按需换取短期企业凭证。")},
		}, Notices: []integrations.AuthSetupNoticeDefinition{
			{ID: "deployment_required", Level: integrations.AuthSetupNoticeLevelWarning, Text: "Installation is unavailable until the deployment has received a current Suite Ticket and all service-provider secrets are configured.", TextI18n: loc("Installation is unavailable until the deployment has received a current Suite Ticket and all service-provider secrets are configured.", "只有在部署已配置全部服务商密钥并收到最新 Suite Ticket 后，才可发起安装。")},
			{ID: "trusted_ip", Level: integrations.AuthSetupNoticeLevelInfo, Text: "WeCom API calls must originate from a public egress IP allowed in the service-provider platform.", TextI18n: loc("WeCom API calls must originate from a public egress IP allowed in the service-provider platform.", "企业微信 API 调用必须来自服务商平台已放行的公网出口 IP。")},
		}},
	}
}

func actions() []integrations.ActionDefinition {
	return []integrations.ActionDefinition{
		read(ActionAppGet, "get_wecom_application", "Get WeCom application", "获取企业微信应用", "Return the installed third-party application profile.", "返回已安装第三方应用的资料。", object(nil, nil), object(map[string]interface{}{"provider": str(64), "application": object(map[string]interface{}{"agent_id": str(64), "name": str(255), "square_logo_url": str(2048)}, []string{"agent_id", "name", "square_logo_url"})}, []string{"provider", "application"}), []string{ScopeApp}),
		read(ActionDepartmentList, "list_wecom_departments", "List WeCom departments", "列出企业微信部门", "List departments visible to the installed third-party application.", "列出已安装第三方应用可见范围内的部门。", object(map[string]interface{}{"department_id": titled(map[string]interface{}{"type": "integer", "minimum": 1}, "Department ID", "部门 ID")}, nil), object(map[string]interface{}{"provider": str(64), "departments": arr(object(map[string]interface{}{"id": map[string]interface{}{"type": "integer"}, "name": str(255), "parent_id": map[string]interface{}{"type": "integer"}}, []string{"id", "name", "parent_id"}), 200)}, []string{"provider", "departments"}), []string{ScopeContacts}),
		read(ActionContactSearch, "search_wecom_contacts", "Search WeCom members", "搜索企业微信成员", "Search visible members by name and return stable recipient references.", "按姓名搜索应用可见成员，并返回稳定的收件人引用。", object(map[string]interface{}{"query": titled(nonblank(128), "Member name", "成员姓名"), "department_id": titled(map[string]interface{}{"type": "integer", "minimum": 1, "default": 1}, "Department ID", "部门 ID"), "max_results": titled(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 20, "default": 10}, "Maximum results", "最大结果数")}, []string{"query"}), object(map[string]interface{}{"provider": str(64), "members": arr(object(map[string]interface{}{"recipient_ref": nonblank(1024), "name": str(255), "department_ids": arr(map[string]interface{}{"type": "integer"}, 50)}, []string{"recipient_ref", "name", "department_ids"}), 20)}, []string{"provider", "members"}), []string{ScopeContacts}),
		read(ActionUserGet, "get_wecom_user", "Get WeCom member", "获取企业微信成员", "Read one member selected from a previous contact search.", "读取先前成员搜索中选择的一位成员。", object(map[string]interface{}{"recipient_ref": titled(nonblank(1024), "Recipient reference", "收件人引用")}, []string{"recipient_ref"}), object(map[string]interface{}{"provider": str(64), "member": object(map[string]interface{}{"recipient_ref": nonblank(1024), "name": str(255), "department_ids": arr(map[string]interface{}{"type": "integer"}, 50), "position": str(255), "status": map[string]interface{}{"type": "integer"}}, []string{"recipient_ref", "name", "department_ids", "position", "status"})}, []string{"provider", "member"}), []string{ScopeContacts}),
		write(ActionMessageSendUser, "send_wecom_user_message", "Send WeCom application message", "发送企业微信应用消息", "Send one plain-text application message to a previously resolved member after explicit approval.", "经明确确认后，向先前解析出的成员发送一条纯文本应用消息。", object(map[string]interface{}{"recipient_ref": titled(nonblank(1024), "Recipient reference", "收件人引用"), "content": titled(nonblank(2048), "Message content", "消息内容")}, []string{"recipient_ref", "content"}), object(map[string]interface{}{"provider": str(64), "message": object(map[string]interface{}{"message_id": str(255), "recipient_ref": nonblank(1024), "provider_accepted": map[string]interface{}{"const": true}}, []string{"message_id", "recipient_ref", "provider_accepted"})}, []string{"provider", "message"}), []string{ScopeSend}),
	}
}
func read(id, tool, en, zh, desc, zhDesc string, input, output map[string]interface{}, scopes []string) integrations.ActionDefinition {
	return integrations.ActionDefinition{ID: id, ToolName: tool, Name: en, NameI18n: loc(en, zh), Description: desc, DescriptionI18n: loc(desc, zhDesc), InputSchema: input, OutputSchema: output, Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow, DataEgress: true, ExternalDestination: "qyapi.weixin.qq.com", Idempotent: true, RequiredScopes: scopes, ScopeLabelsI18n: labels(scopes), SupportedAuthMethodIDs: []string{AuthMethodID}, DefaultPolicy: &integrations.DefaultActionPolicy{Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true}, SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent, tools.ToolInvokeFromWorkflow}}
}
func write(id, tool, en, zh, desc, zhDesc string, input, output map[string]interface{}, scopes []string) integrations.ActionDefinition {
	return integrations.ActionDefinition{ID: id, ToolName: tool, Name: en, NameI18n: loc(en, zh), Description: desc, DescriptionI18n: loc(desc, zhDesc), InputSchema: input, OutputSchema: output, Effect: toolgovernance.EffectExternalSend, RiskLevel: toolgovernance.RiskLevelHigh, DataEgress: true, ExternalDestination: "qyapi.weixin.qq.com", Idempotent: false, RequiredScopes: scopes, ScopeLabelsI18n: labels(scopes), SupportedAuthMethodIDs: []string{AuthMethodID}, DefaultPolicy: &integrations.DefaultActionPolicy{Enabled: false, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: true}, SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat}, PreparationHints: []integrations.ActionPreparationHint{{ActionID: ActionContactSearch, Relation: integrations.ActionPreparationResolveTarget, TargetArguments: []string{"recipient_ref"}, ResultPaths: []string{"members[].recipient_ref"}, Description: "Resolve the intended member before sending.", DescriptionI18n: loc("Resolve the intended member before sending.", "发送前先查找并确认目标成员。")}}, SuccessDeduplication: &integrations.SuccessDeduplicationDefinition{TargetArgumentPaths: []string{"recipient_ref"}}}
}
func loc(en, zh string) integrations.LocalizedText {
	return integrations.LocalizedText{integrations.LocaleEnglishUS: en, integrations.LocaleSimplifiedChinese: zh}
}
func labels(scopes []string) integrations.LocalizedLabelMap {
	out := integrations.LocalizedLabelMap{}
	for _, scope := range scopes {
		switch scope {
		case ScopeApp:
			out[scope] = loc("Read application identity", "读取应用身份")
		case ScopeContacts:
			out[scope] = loc("Read visible members", "读取可见成员")
		case ScopeSend:
			out[scope] = loc("Send application messages", "发送应用消息")
		}
	}
	return out
}
func object(props map[string]interface{}, required []string) map[string]interface{} {
	if props == nil {
		props = map[string]interface{}{}
	}
	value := map[string]interface{}{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		value["required"] = required
	}
	return value
}
func str(max int) map[string]interface{} {
	return map[string]interface{}{"type": "string", "maxLength": max}
}
func nonblank(max int) map[string]interface{} {
	return map[string]interface{}{"type": "string", "minLength": 1, "maxLength": max}
}
func arr(item map[string]interface{}, max int) map[string]interface{} {
	return map[string]interface{}{"type": "array", "maxItems": max, "items": item}
}
func titled(schema map[string]interface{}, en, zh string) map[string]interface{} {
	schema["title"] = en
	schema["title_i18n"] = map[string]interface{}{"en-US": en, "zh-Hans": zh}
	return schema
}
