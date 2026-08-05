package wecom

import (
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	IntegrationID         = "wecom"
	DriverID              = "wecom-open-api"
	AuthMethodID          = "organization_wecom_custom_app"
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
		Description:     "Connect an organization-owned WeCom custom application to resolve members and send application messages with explicit approval.",
		DescriptionI18n: loc("Connect an organization-owned WeCom custom application to resolve members and send application messages with explicit approval.", "连接组织自建企业微信应用，用于查找成员并在明确确认后发送应用消息。"),
		Author:          "ZGI", Icon: "message-square", Tags: []string{"communication", "enterprise", "external"},
		TagLabelsI18n: integrations.LocalizedLabelMap{"communication": loc("Communication", "沟通协作"), "enterprise": loc("Enterprise", "企业服务"), "external": loc("External", "外部服务")},
		Categories:    []string{"communication"}, CategoryLabelsI18n: integrations.LocalizedLabelMap{"communication": loc("Communication", "沟通协作")},
		DocumentationURL: "https://developer.work.weixin.qq.com/document/path/91039", DocumentationURLI18n: loc("https://developer.work.weixin.qq.com/document/path/91039", "https://developer.work.weixin.qq.com/document/path/91039"),
		AuthMethods: []integrations.AuthMethodDefinition{authMethod()},
		HealthProbe: integrations.HealthProbeDefinition{Supported: true, MayIncurCost: false, Description: "Verifies the custom application and reads its bounded profile without sending a message.", DescriptionI18n: loc("Verifies the custom application and reads its bounded profile without sending a message.", "验证自建应用并读取受限的应用资料，不会发送消息。")},
		Scopes: []integrations.ProviderScopeDefinition{
			{ID: ScopeApp, Label: "Read application identity", LabelI18n: loc("Read application identity", "读取应用身份"), Category: integrations.ProviderScopeCategoryIdentity, Access: integrations.ProviderScopeAccessIdentity},
			{ID: ScopeContacts, Label: "Read visible organization members", LabelI18n: loc("Read visible organization members", "读取应用可见范围内的组织成员"), Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead, Broad: true},
			{ID: ScopeSend, Label: "Send application messages", LabelI18n: loc("Send application messages", "发送应用消息"), Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite, Broad: true},
		}, Actions: actions(),
	}
}

func authMethod() integrations.AuthMethodDefinition {
	return integrations.AuthMethodDefinition{
		ID: AuthMethodID, Type: integrations.AuthMethodTypeCustomCredential, CredentialSource: integrations.ConnectionCredentialSourceOrganization, IdentityKind: integrations.AuthIdentityKindApplication,
		AcquisitionStrategy: integrations.AuthAcquisitionStrategyManualForm, LifecycleStrategy: integrations.AuthLifecycleStrategyExchangeOnDemand, RequestAuthStrategy: integrations.RequestAuthStrategyProviderCustom,
		Label: "Connect an organization WeCom custom application", LabelI18n: loc("Connect an organization WeCom custom application", "连接组织企业微信自建应用"),
		Description:     "Enter the Corp ID, Agent ID, and application Secret from the WeCom administration console. The Secret is encrypted and never returned to the browser.",
		DescriptionI18n: loc("Enter the Corp ID, Agent ID, and application Secret from the WeCom administration console. The Secret is encrypted and never returned to the browser.", "填写企业微信管理后台的企业 ID、应用 AgentID 和应用 Secret；Secret 会加密保存且不会返回浏览器。"), Available: true,
		Fields: []integrations.CredentialFieldDefinition{
			field("corp_id", "Corp ID", "企业 ID", "Find this under My Enterprise > Enterprise Information. It is not the AgentID.", "在“我的企业 → 企业信息”中查看；它不是应用 AgentID。", integrations.CredentialFieldInputText, true, false),
			field("agent_id", "Agent ID", "应用 AgentID", "Copy the numeric AgentID from the same custom application as the Secret.", "从与 Secret 相同的自建应用详情中复制数字 AgentID。", integrations.CredentialFieldInputText, true, false),
			field("secret", "Application Secret", "应用 Secret", "Use the Secret of this custom application, not a contacts Secret, callback Token, or EncodingAESKey.", "填写该自建应用的 Secret，不要填写通讯录 Secret、回调 Token 或 EncodingAESKey。", integrations.CredentialFieldInputPassword, true, true),
		}, SetupGuide: &integrations.AuthSetupGuideDefinition{ConsoleURL: "https://work.weixin.qq.com/wework_admin/frame", DocumentationURL: "https://developer.work.weixin.qq.com/document/path/91039", ExpandedByDefault: true, Steps: []integrations.AuthSetupStepDefinition{
			{ID: "create_custom_app", Title: "Create a custom application", TitleI18n: loc("Create a custom application", "创建企业自建应用"), Description: "Create a dedicated custom application under Applications. A group robot webhook is not compatible.", DescriptionI18n: loc("Create a dedicated custom application under Applications. A group robot webhook is not compatible.", "在“应用管理”中创建专用自建应用；群机器人 Webhook 不能用于此连接。"), Action: integrations.AuthSetupStepActionOpenConsole},
			{ID: "copy_corp_id", Title: "Copy the Corp ID", TitleI18n: loc("Copy the Corp ID", "复制企业 ID"), Description: "Open My Enterprise > Enterprise Information and copy the Corp ID of the enterprise that owns the application.", DescriptionI18n: loc("Open My Enterprise > Enterprise Information and copy the Corp ID of the enterprise that owns the application.", "打开“我的企业 → 企业信息”，复制拥有该应用的同一企业的企业 ID。")},
			{ID: "copy_app_credentials", Title: "Copy AgentID and Secret from the same app", TitleI18n: loc("Copy AgentID and Secret from the same app", "从同一应用复制 AgentID 和 Secret"), Description: "Open the custom application details and copy its AgentID and Secret. Do not mix credentials from another application.", DescriptionI18n: loc("Open the custom application details and copy its AgentID and Secret. Do not mix credentials from another application.", "打开自建应用详情，复制 AgentID 和 Secret；不要混用其他应用的凭据。")},
			{ID: "configure_visibility", Title: "Configure the application visibility range", TitleI18n: loc("Configure the application visibility range", "配置应用可见范围"), Description: "Include every department or member ZGI needs to find. Hidden members cannot be searched or messaged.", DescriptionI18n: loc("Include every department or member ZGI needs to find. Hidden members cannot be searched or messaged.", "将 ZGI 需要查找的部门或成员加入应用可见范围；不可见成员无法搜索或发送消息。")},
			{ID: "configure_trusted_ip", Title: "Allow the ZGI server public egress IP", TitleI18n: loc("Allow the ZGI server public egress IP", "配置 ZGI 服务器出口 IP"), Description: "If trusted IP is required, add the public egress IP of the ZGI API server. This is not the browser address or localhost.", DescriptionI18n: loc("If trusted IP is required, add the public egress IP of the ZGI API server. This is not the browser address or localhost.", "如应用要求可信 IP，请加入 ZGI API 服务器的公网出口 IP，而不是浏览器地址或 localhost。")},
			{ID: "save_and_verify", Title: "Save and let ZGI verify", TitleI18n: loc("Save and let ZGI verify", "保存并让 ZGI 自动验证"), Description: "ZGI obtains an application token and reads the application profile. Verification does not send a message.", DescriptionI18n: loc("ZGI obtains an application token and reads the application profile. Verification does not send a message.", "ZGI 会获取应用访问凭证并读取应用资料；验证过程不会发送消息。")},
		}, Notices: []integrations.AuthSetupNoticeDefinition{
			{ID: "same_application", Level: integrations.AuthSetupNoticeLevelWarning, Text: "Corp ID, AgentID, and Secret must belong to the same enterprise and custom application.", TextI18n: loc("Corp ID, AgentID, and Secret must belong to the same enterprise and custom application.", "企业 ID、AgentID 和 Secret 必须属于同一企业的同一自建应用。")},
			{ID: "trusted_ip", Level: integrations.AuthSetupNoticeLevelInfo, Text: "WeCom error 60020 usually means the ZGI API server public egress IP is not trusted.", TextI18n: loc("WeCom error 60020 usually means the ZGI API server public egress IP is not trusted.", "企业微信错误码 60020 通常表示 ZGI API 服务器公网出口 IP 未加入可信 IP。")},
			{ID: "secret_write_only", Level: integrations.AuthSetupNoticeLevelInfo, Text: "The Secret is write-only, encrypted at rest, and never returned to the browser.", TextI18n: loc("The Secret is write-only, encrypted at rest, and never returned to the browser.", "Secret 仅可写入，会加密保存，之后不会返回浏览器。")},
		}},
	}
}
func field(key, en, zh, description, zhDescription string, input integrations.CredentialFieldInput, required, secret bool) integrations.CredentialFieldDefinition {
	return integrations.CredentialFieldDefinition{Key: key, Label: en, LabelI18n: loc(en, zh), Description: description, DescriptionI18n: loc(description, zhDescription), Input: input, Required: required, Secret: secret}
}

func actions() []integrations.ActionDefinition {
	return []integrations.ActionDefinition{
		read(ActionAppGet, "get_wecom_application", "Get WeCom application", "获取企业微信应用", "Return the connected custom application profile.", "返回已连接自建应用的资料。", object(nil, nil), object(map[string]interface{}{"provider": str(64), "application": object(map[string]interface{}{"agent_id": str(64), "name": str(255), "square_logo_url": str(2048)}, []string{"agent_id", "name", "square_logo_url"})}, []string{"provider", "application"}), []string{ScopeApp}),
		read(ActionDepartmentList, "list_wecom_departments", "List WeCom departments", "列出企业微信部门", "List departments visible to the custom application.", "列出自建应用可见范围内的部门。", object(map[string]interface{}{"department_id": titled(map[string]interface{}{"type": "integer", "minimum": 1}, "Department ID", "部门 ID")}, nil), object(map[string]interface{}{"provider": str(64), "departments": arr(object(map[string]interface{}{"id": map[string]interface{}{"type": "integer"}, "name": str(255), "parent_id": map[string]interface{}{"type": "integer"}}, []string{"id", "name", "parent_id"}), 200)}, []string{"provider", "departments"}), []string{ScopeContacts}),
		read(ActionContactSearch, "search_wecom_contacts", "Search WeCom members", "搜索企业微信成员", "Search visible members by name and return stable recipient references.", "按姓名搜索应用可见成员，并返回稳定的收件人引用。", object(map[string]interface{}{"query": titled(nonblank(128), "Member name", "成员姓名"), "department_id": titled(map[string]interface{}{"type": "integer", "minimum": 1, "default": 1}, "Department ID", "部门 ID"), "max_results": titled(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 20, "default": 10}, "Maximum results", "最大结果数")}, []string{"query"}), object(map[string]interface{}{"provider": str(64), "members": arr(object(map[string]interface{}{"recipient_ref": nonblank(1024), "name": str(255), "department_ids": arr(map[string]interface{}{"type": "integer"}, 50)}, []string{"recipient_ref", "name", "department_ids"}), 20)}, []string{"provider", "members"}), []string{ScopeContacts}),
		read(ActionUserGet, "get_wecom_user", "Get WeCom member", "获取企业微信成员", "Read one member selected from a previous contact search.", "读取先前成员搜索中选择的一位成员。", object(map[string]interface{}{"recipient_ref": titled(nonblank(1024), "Recipient reference", "收件人引用")}, []string{"recipient_ref"}), object(map[string]interface{}{"provider": str(64), "member": object(map[string]interface{}{"recipient_ref": nonblank(1024), "name": str(255), "department_ids": arr(map[string]interface{}{"type": "integer"}, 50), "position": str(255), "status": map[string]interface{}{"type": "integer"}}, []string{"recipient_ref", "name", "department_ids", "position", "status"})}, []string{"provider", "member"}), []string{ScopeContacts}),
		write(ActionMessageSendUser, "send_wecom_user_message", "Send WeCom application message", "发送企业微信应用消息", "Send one plain-text application message to a previously resolved member after explicit approval.", "经明确确认后，向先前解析出的成员发送一条纯文本应用消息。", object(map[string]interface{}{"recipient_ref": titled(nonblank(1024), "Recipient reference", "收件人引用"), "content": titled(nonblank(2048), "Message content", "消息内容")}, []string{"recipient_ref", "content"}), object(map[string]interface{}{"provider": str(64), "message": object(map[string]interface{}{"message_id": str(255), "recipient_ref": nonblank(1024), "provider_accepted": map[string]interface{}{"const": true}}, []string{"message_id", "recipient_ref", "provider_accepted"})}, []string{"provider", "message"}), []string{ScopeSend}),
	}
}
func read(id, tool, en, zh, desc, zhDesc string, input, output map[string]interface{}, scopes []string) integrations.ActionDefinition {
	return integrations.ActionDefinition{ID: id, ToolName: tool, Name: en, NameI18n: loc(en, zh), Description: desc, DescriptionI18n: loc(desc, zhDesc), InputSchema: input, OutputSchema: output, Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow, DataEgress: true, ExternalDestination: "qyapi.weixin.qq.com", Idempotent: true, RequiredScopes: scopes, ScopeLabelsI18n: labels(scopes), SupportedAuthMethodIDs: []string{AuthMethodID}, DefaultPolicy: &integrations.DefaultActionPolicy{Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true}, SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent}}
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
