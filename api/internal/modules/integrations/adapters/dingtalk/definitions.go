package dingtalk

import (
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	IntegrationID = "dingtalk"
	DriverID      = "dingtalk-open-api"
	AuthMethodID  = "organization_dingtalk_internal_app"

	ActionDepartmentList   = "dingtalk.department.list"
	ActionDepartmentSearch = "dingtalk.department.search"
	ActionDepartmentGet    = "dingtalk.department.get"
	ActionDepartmentUsers  = "dingtalk.department.members.list"
	ActionContactSearch    = "dingtalk.contact.search"
	ActionUserGet          = "dingtalk.user.get"
	ActionRoleList         = "dingtalk.role.list"
	ActionRoleUsers        = "dingtalk.role.members.list"
	ActionAttendanceList   = "dingtalk.attendance.records.list"
	ActionMessageSendUser  = "dingtalk.message.send_user"
	ActionMessageSendDept  = "dingtalk.message.send_department"
	ActionMessageStatusGet = "dingtalk.message.delivery.get"

	ScopeContacts   = "dingtalk:contacts:read"
	ScopeAttendance = "dingtalk:attendance:read"
	ScopeSend       = "dingtalk:message:send"
)

func ProviderDefinition() integrations.ProviderDefinition {
	return integrations.ProviderDefinition{
		ID: IntegrationID, DriverID: DriverID, Name: "DingTalk", NameI18n: loc("DingTalk", "钉钉"),
		Description:     "Connect an organization-owned DingTalk internal application to resolve visible members and send traceable work notifications with explicit approval.",
		DescriptionI18n: loc("Connect an organization-owned DingTalk internal application to resolve visible members and send traceable work notifications with explicit approval.", "连接组织自建的钉钉企业内部应用，用于查找可见成员，并在明确确认后发送可追踪的工作通知。"),
		Author:          "ZGI", Icon: "message-square", Tags: []string{"communication", "enterprise", "external"},
		TagLabelsI18n: integrations.LocalizedLabelMap{
			"communication": loc("Communication", "沟通协作"), "enterprise": loc("Enterprise", "企业服务"), "external": loc("External", "外部服务"),
		},
		Categories:           []string{"communication"},
		CategoryLabelsI18n:   integrations.LocalizedLabelMap{"communication": loc("Communication", "沟通协作")},
		DocumentationURL:     "https://open.dingtalk.com/document/orgapp/overview-of-development-process",
		DocumentationURLI18n: loc("https://open.dingtalk.com/document/orgapp/overview-of-development-process", "https://open.dingtalk.com/document/orgapp/overview-of-development-process"),
		AuthMethods:          []integrations.AuthMethodDefinition{authMethod()},
		HealthProbe: integrations.HealthProbeDefinition{
			Supported: true, MayIncurCost: false,
			Description:     "Obtains an application token and reads the root department list without sending a notification.",
			DescriptionI18n: loc("Obtains an application token and reads the root department list without sending a notification.", "获取应用访问凭证并读取根部门列表，不会发送工作通知。"),
		},
		Scopes: []integrations.ProviderScopeDefinition{
			{ID: ScopeContacts, Label: "Read visible organization contacts", LabelI18n: loc("Read visible organization contacts", "读取应用可见范围内的组织通讯录"), Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead, Broad: true},
			{ID: ScopeAttendance, Label: "Read employee attendance records", LabelI18n: loc("Read employee attendance records", "读取员工考勤打卡记录"), Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead, Broad: true},
			{ID: ScopeSend, Label: "Send and inspect work notifications", LabelI18n: loc("Send and inspect work notifications", "发送并查询工作通知状态"), Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite, Broad: true},
		},
		Actions: actions(),
	}
}

func authMethod() integrations.AuthMethodDefinition {
	return integrations.AuthMethodDefinition{
		ID: AuthMethodID, Type: integrations.AuthMethodTypeCustomCredential,
		CredentialSource:    integrations.ConnectionCredentialSourceOrganization,
		IdentityKind:        integrations.AuthIdentityKindApplication,
		AcquisitionStrategy: integrations.AuthAcquisitionStrategyManualForm,
		LifecycleStrategy:   integrations.AuthLifecycleStrategyExchangeOnDemand,
		RequestAuthStrategy: integrations.RequestAuthStrategyProviderCustom,
		ScopeEvidence:       integrations.AuthScopeEvidenceConnectorDeclared,
		Label:               "Connect a DingTalk internal application", LabelI18n: loc("Connect a DingTalk internal application", "连接钉钉企业内部应用"),
		Description:     "Enter the AppKey, AppSecret, and AgentId of a dedicated internal application. The AppSecret is encrypted and never returned to the browser.",
		DescriptionI18n: loc("Enter the AppKey, AppSecret, and AgentId of a dedicated internal application. The AppSecret is encrypted and never returned to the browser.", "填写专用企业内部应用的 AppKey、AppSecret 和 AgentId。AppSecret 会加密保存，且不会返回浏览器。"),
		Available:       true,
		Fields: []integrations.CredentialFieldDefinition{
			field("app_key", "AppKey (Client ID)", "AppKey（Client ID）", "Copy the AppKey from the Basic Information page of the internal application.", "从企业内部应用的“基础信息”页面复制 AppKey。", integrations.CredentialFieldInputText, true, false),
			field("app_secret", "AppSecret (Client Secret)", "AppSecret（Client Secret）", "Use the AppSecret from the same internal application as the AppKey and AgentId.", "填写与 AppKey、AgentId 同一企业内部应用的 AppSecret。", integrations.CredentialFieldInputPassword, true, true),
			field("agent_id", "AgentId", "AgentId", "Copy the numeric AgentId from the application details. Do not use the AppKey or robot webhook ID.", "从应用详情复制数字 AgentId；不要填写 AppKey 或机器人 Webhook 标识。", integrations.CredentialFieldInputText, true, false),
		},
		SetupGuide: &integrations.AuthSetupGuideDefinition{
			ConsoleURL: "https://open-dev.dingtalk.com/", DocumentationURL: "https://open.dingtalk.com/document/orgapp/overview-of-development-process", ExpandedByDefault: true,
			Steps: []integrations.AuthSetupStepDefinition{
				{ID: "create_internal_app", Title: "Create an enterprise internal application", TitleI18n: loc("Create an enterprise internal application", "创建企业内部应用"), Description: "Choose the correct DingTalk organization and create a dedicated enterprise internal application. A group robot webhook is not compatible.", DescriptionI18n: loc("Choose the correct DingTalk organization and create a dedicated enterprise internal application. A group robot webhook is not compatible.", "选择正确的钉钉组织并创建专用企业内部应用；群机器人 Webhook 不能用于此连接。"), Action: integrations.AuthSetupStepActionOpenConsole},
				{ID: "copy_credentials", Title: "Copy all values from the same application", TitleI18n: loc("Copy all values from the same application", "从同一应用复制三项凭据"), Description: "Copy AppKey, AppSecret, and numeric AgentId from the application details. Do not mix values from different apps.", DescriptionI18n: loc("Copy AppKey, AppSecret, and numeric AgentId from the application details. Do not mix values from different apps.", "从应用详情复制 AppKey、AppSecret 和数字 AgentId；不要混用不同应用的数据。")},
				{ID: "grant_contact_permissions", Title: "Grant permissions required by verification", TitleI18n: loc("Grant permissions required by verification", "开通连接验证所需权限"), Description: "The connection test reads the root department list. Grant the internal app permission to read organization departments and contacts before testing.", DescriptionI18n: loc("The connection test reads the root department list. Grant the internal app permission to read organization departments and contacts before testing.", "连接测试会读取根部门列表。测试前，请为企业内部应用开通组织部门和通讯录读取权限。")},
				{ID: "grant_optional_permissions", Title: "Grant permissions for enabled actions", TitleI18n: loc("Grant permissions for enabled actions", "按启用操作补充权限"), Description: "Attendance actions need attendance read access; notification actions need send and status access. Grant only what you plan to enable.", DescriptionI18n: loc("Attendance actions need attendance read access; notification actions need send and status access. Grant only what you plan to enable.", "考勤操作需要考勤读取权限；工作通知操作需要发送和状态查询权限。仅开通准备启用的能力。")},
				{ID: "configure_visibility", Title: "Configure application visibility", TitleI18n: loc("Configure application visibility", "配置应用可见范围"), Description: "Include the departments and members ZGI needs to find. Members outside this range cannot be resolved or notified.", DescriptionI18n: loc("Include the departments and members ZGI needs to find. Members outside this range cannot be resolved or notified.", "将 ZGI 需要查找的部门和成员加入应用可见范围；范围外成员无法搜索或发送通知。")},
				{ID: "publish_application", Title: "Publish and enable the application", TitleI18n: loc("Publish and enable the application", "发布并启用应用"), Description: "Create and publish an application version, then confirm the internal app is enabled for the selected organization.", DescriptionI18n: loc("Create and publish an application version, then confirm the internal app is enabled for the selected organization.", "创建并发布应用版本，并确认企业内部应用已在所选组织中启用。")},
				{ID: "configure_ip_whitelist", Title: "Check the server IP whitelist", TitleI18n: loc("Check the server IP whitelist", "检查服务器出口 IP 白名单"), Description: "If source-IP restrictions are enabled, allow the public egress IP of the ZGI API server, not localhost or the browser IP.", DescriptionI18n: loc("If source-IP restrictions are enabled, allow the public egress IP of the ZGI API server, not localhost or the browser IP.", "如钉钉应用启用了来源 IP 限制，请加入 ZGI API 服务器公网出口 IP，而不是 localhost 或浏览器 IP。")},
				{ID: "save_and_verify", Title: "Save and let ZGI verify", TitleI18n: loc("Save and let ZGI verify", "保存并让 ZGI 自动验证"), Description: "Verification obtains an application token and reads the root department list. It does not send a notification.", DescriptionI18n: loc("Verification obtains an application token and reads the root department list. It does not send a notification.", "验证会获取应用访问凭证并读取根部门列表，不会发送工作通知。")},
			},
			Notices: []integrations.AuthSetupNoticeDefinition{
				{ID: "same_application", Level: integrations.AuthSetupNoticeLevelWarning, Text: "AppKey, AppSecret, and AgentId must come from the same enterprise internal application.", TextI18n: loc("AppKey, AppSecret, and AgentId must come from the same enterprise internal application.", "AppKey、AppSecret 和 AgentId 必须来自同一个企业内部应用。")},
				{ID: "contact_permission_required", Level: integrations.AuthSetupNoticeLevelWarning, Text: "Department and contact read access is required even when you only plan to send notifications, because ZGI verifies the application and resolves recipients first.", TextI18n: loc("Department and contact read access is required even when you only plan to send notifications, because ZGI verifies the application and resolves recipients first.", "即使只想发送通知，也必须开通部门和通讯录读取权限，因为 ZGI 需要先验证应用并解析接收人。")},
				{ID: "secret_write_only", Level: integrations.AuthSetupNoticeLevelInfo, Text: "The AppSecret is write-only, encrypted at rest, and never returned to the browser.", TextI18n: loc("The AppSecret is write-only, encrypted at rest, and never returned to the browser.", "AppSecret 仅可写入，会加密保存，之后不会返回浏览器。")},
			},
		},
	}
}

func actions() []integrations.ActionDefinition {
	result := []integrations.ActionDefinition{
		read(ActionDepartmentList, "list_dingtalk_departments", "List DingTalk departments", "列出钉钉部门", "List departments visible to the internal application.", "列出企业内部应用可见范围内的部门。", object(map[string]interface{}{"department_id": titled(map[string]interface{}{"type": "integer", "minimum": 1, "default": 1}, "Parent department ID", "父部门 ID")}, nil), object(map[string]interface{}{"provider": nonblank(64), "departments": array(object(map[string]interface{}{"department_ref": nonblank(2048), "id": map[string]interface{}{"type": "integer"}, "name": text(255), "parent_id": map[string]interface{}{"type": "integer"}}, []string{"department_ref", "id", "name", "parent_id"}), 200), "empty_reason": map[string]interface{}{"type": "string", "enum": []string{"no_child_departments"}}}, []string{"provider", "departments"}), []string{ScopeContacts}),
		read(ActionContactSearch, "search_dingtalk_contacts", "Search DingTalk members", "搜索钉钉成员", "Search visible organization members by name and return connection-bound recipient references.", "按姓名搜索应用可见的组织成员，并返回与当前连接绑定的接收人引用。", object(map[string]interface{}{"query": titled(nonblank(128), "Member name", "成员姓名"), "max_results": titled(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 20, "default": 10}, "Maximum results", "最大结果数")}, []string{"query"}), object(map[string]interface{}{"provider": nonblank(64), "members": array(object(map[string]interface{}{"recipient_ref": nonblank(2048), "name": text(255), "title": text(255)}, []string{"recipient_ref", "name", "title"}), 20), "has_more": map[string]interface{}{"type": "boolean"}}, []string{"provider", "members", "has_more"}), []string{ScopeContacts}),
		read(ActionUserGet, "get_dingtalk_user", "Get DingTalk member", "获取钉钉成员", "Read one member selected from a previous contact search.", "读取先前成员搜索中选定的一位成员。", object(map[string]interface{}{"recipient_ref": titled(nonblank(2048), "Recipient reference", "接收人引用")}, []string{"recipient_ref"}), object(map[string]interface{}{"provider": nonblank(64), "member": object(map[string]interface{}{"recipient_ref": nonblank(2048), "name": text(255), "title": text(255), "department_ids": array(map[string]interface{}{"type": "integer"}, 50), "active": map[string]interface{}{"type": "boolean"}}, []string{"recipient_ref", "name", "title", "department_ids", "active"})}, []string{"provider", "member"}), []string{ScopeContacts}),
		write(ActionMessageSendUser, "send_dingtalk_work_notification", "Send DingTalk work notification", "发送钉钉工作通知", "Submit one plain-text work notification to a previously resolved member after explicit approval. A successful response means accepted for processing, not confirmed delivery.", "经明确确认后，向先前解析的成员提交一条纯文本工作通知。成功响应仅表示钉钉已受理，不代表已确认送达。", object(map[string]interface{}{"recipient_ref": titled(nonblank(2048), "Recipient reference", "接收人引用"), "content": titled(nonblank(2048), "Notification content", "通知内容")}, []string{"recipient_ref", "content"}), object(map[string]interface{}{"provider": nonblank(64), "notification": object(map[string]interface{}{"message_ref": nonblank(2048), "recipient_ref": nonblank(2048), "provider_accepted": map[string]interface{}{"const": true}, "delivery_status": map[string]interface{}{"const": "pending"}}, []string{"message_ref", "recipient_ref", "provider_accepted", "delivery_status"})}, []string{"provider", "notification"}), []string{ScopeSend}),
		read(ActionMessageStatusGet, "get_dingtalk_notification_status", "Get DingTalk notification status", "查询钉钉通知状态", "Query the delivery result of a work notification submitted by this connection.", "查询由当前连接提交的工作通知送达结果。", object(map[string]interface{}{"message_ref": titled(nonblank(2048), "Message reference", "消息引用")}, []string{"message_ref"}), object(map[string]interface{}{"provider": nonblank(64), "notification": object(map[string]interface{}{"message_ref": nonblank(2048), "target_type": map[string]interface{}{"type": "string", "enum": []string{"member", "department"}}, "delivery_status": map[string]interface{}{"type": "string", "enum": []string{"pending", "delivered_unread", "delivered_read", "partially_delivered", "failed"}}, "delivered_count": map[string]interface{}{"type": "integer", "minimum": 0}, "failed_count": map[string]interface{}{"type": "integer", "minimum": 0}, "failure_reason": text(255)}, []string{"message_ref", "target_type", "delivery_status", "delivered_count", "failed_count", "failure_reason"})}, []string{"provider", "notification"}), []string{ScopeSend}),
	}
	return append(result, extendedActions()...)
}

func read(id, tool, en, zh, description, zhDescription string, input, output map[string]interface{}, scopes []string) integrations.ActionDefinition {
	destination := "oapi.dingtalk.com"
	if id == ActionContactSearch || id == ActionDepartmentSearch {
		destination = "api.dingtalk.com"
	}
	return integrations.ActionDefinition{
		ID: id, ToolName: tool, Name: en, NameI18n: loc(en, zh), Description: description, DescriptionI18n: loc(description, zhDescription),
		InputSchema: input, OutputSchema: output, Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
		DataEgress: true, ExternalDestination: destination, Idempotent: true,
		RequiredScopes: scopes, ScopeLabelsI18n: scopeLabels(scopes), SupportedAuthMethodIDs: []string{AuthMethodID},
		DefaultPolicy:    &integrations.DefaultActionPolicy{Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true},
		SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent},
	}
}

func write(id, tool, en, zh, description, zhDescription string, input, output map[string]interface{}, scopes []string) integrations.ActionDefinition {
	return integrations.ActionDefinition{
		ID: id, ToolName: tool, Name: en, NameI18n: loc(en, zh), Description: description, DescriptionI18n: loc(description, zhDescription),
		InputSchema: input, OutputSchema: output, Effect: toolgovernance.EffectExternalSend, RiskLevel: toolgovernance.RiskLevelHigh,
		DataEgress: true, ExternalDestination: "oapi.dingtalk.com", Idempotent: false,
		RequiredScopes: scopes, ScopeLabelsI18n: scopeLabels(scopes), SupportedAuthMethodIDs: []string{AuthMethodID},
		DefaultPolicy:        &integrations.DefaultActionPolicy{Enabled: false, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: true},
		SupportedCallers:     []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat},
		PreparationHints:     []integrations.ActionPreparationHint{{ActionID: ActionContactSearch, Relation: integrations.ActionPreparationResolveTarget, TargetArguments: []string{"recipient_ref"}, ResultPaths: []string{"members[].recipient_ref"}, Description: "Resolve and disambiguate the intended DingTalk member before sending.", DescriptionI18n: loc("Resolve and disambiguate the intended DingTalk member before sending.", "发送前先搜索并确认目标钉钉成员；存在同名结果时必须消歧。")}},
		SuccessDeduplication: &integrations.SuccessDeduplicationDefinition{TargetArgumentPaths: []string{"recipient_ref"}},
	}
}

func field(key, en, zh, description, zhDescription string, input integrations.CredentialFieldInput, required, secret bool) integrations.CredentialFieldDefinition {
	return integrations.CredentialFieldDefinition{Key: key, Label: en, LabelI18n: loc(en, zh), Description: description, DescriptionI18n: loc(description, zhDescription), Input: input, Required: required, Secret: secret}
}
func loc(en, zh string) integrations.LocalizedText {
	return integrations.LocalizedText{integrations.LocaleEnglishUS: en, integrations.LocaleSimplifiedChinese: zh}
}
func scopeLabels(scopes []string) integrations.LocalizedLabelMap {
	result := integrations.LocalizedLabelMap{}
	for _, scope := range scopes {
		switch scope {
		case ScopeContacts:
			result[scope] = loc("Read visible contacts", "读取可见通讯录")
		case ScopeAttendance:
			result[scope] = loc("Read employee attendance records", "读取员工考勤记录")
		case ScopeSend:
			result[scope] = loc("Send and inspect work notifications", "发送并查询工作通知")
		}
	}
	return result
}
func object(properties map[string]interface{}, required []string) map[string]interface{} {
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
func nonblank(max int) map[string]interface{} {
	return map[string]interface{}{"type": "string", "minLength": 1, "maxLength": max}
}
func array(item map[string]interface{}, max int) map[string]interface{} {
	return map[string]interface{}{"type": "array", "maxItems": max, "items": item}
}
func titled(schema map[string]interface{}, en, zh string) map[string]interface{} {
	schema["title"] = en
	schema["title_i18n"] = map[string]interface{}{"en-US": en, "zh-Hans": zh}
	return schema
}
