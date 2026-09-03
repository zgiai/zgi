package wecom

import (
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
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
	// Canonical member/application-message names are source-level aliases for
	// the stable action IDs. Keeping the registered IDs and tool names avoids
	// breaking existing connection policies, approvals, and saved agents.
	ActionMemberSearch       = ActionContactSearch
	ActionAppMessageTextSend = ActionMessageSendUser
	ScopeApp                 = "wecom:app"
	ScopeContacts            = "wecom:contacts:read"
	ScopeSend                = "wecom:message:send"
)

func ProviderDefinition() integrations.ProviderDefinition {
	return ProviderDefinitionForConfig(SuiteConfig{})
}

func ProviderDefinitionForConfig(config SuiteConfig) integrations.ProviderDefinition {
	definition := integrations.ProviderDefinition{
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
	if config.CustomerContactEnabled {
		customerScopes := customerActionScopes()
		definition.Scopes = append(definition.Scopes, customerScopes[:2]...)
		for _, action := range customerActions() {
			if isCustomerContactAction(action.ID) {
				definition.Actions = append(definition.Actions, action)
			}
		}
	}
	if config.CustomerContactEnabled && config.CustomerMassMessageEnabled {
		customerScopes := customerActionScopes()
		definition.Scopes = append(definition.Scopes, customerScopes[2])
		for _, action := range customerActions() {
			if isCustomerMassMessageAction(action.ID) {
				definition.Actions = append(definition.Actions, action)
			}
		}
	}
	return definition
}

func authMethod() integrations.AuthMethodDefinition {
	return integrations.AuthMethodDefinition{
		ID: AuthMethodID, Type: integrations.AuthMethodTypeProviderInstall, CredentialSource: integrations.ConnectionCredentialSourceOrganization, IdentityKind: integrations.AuthIdentityKindApplication,
		AcquisitionStrategy: integrations.AuthAcquisitionStrategyProviderInstallRedirect, LifecycleStrategy: integrations.AuthLifecycleStrategyProviderExchange, RequestAuthStrategy: integrations.RequestAuthStrategyProviderCustom,
		ScopeEvidence: integrations.AuthScopeEvidenceConnectorDeclared,
		Label:         "Authorize WeCom for this organization", LabelI18n: loc("Authorize WeCom for this organization", "授权企业微信组织连接"),
		Description:     "A WeCom system administrator reviews the requested permissions and installs the application. ZGI stores the resulting permanent authorization encrypted; no Corp ID, AgentID, or Secret is entered manually.",
		DescriptionI18n: loc("A WeCom system administrator reviews the requested permissions and installs the application. ZGI stores the resulting permanent authorization encrypted; no Corp ID, AgentID, or Secret is entered manually.", "由企业微信系统管理员确认企业权限与成员可见范围。企业微信会将此授权显示为安装第三方应用；ZGI 加密保存返回的永久授权，无需手工填写企业 ID、AgentID 或 Secret。"), Available: true,
		SetupGuide: &integrations.AuthSetupGuideDefinition{ConsoleURL: "https://open.work.weixin.qq.com/wwopen/developers/tools", DocumentationURL: "https://developer.work.weixin.qq.com/document/path/90597", ExpandedByDefault: true, Steps: []integrations.AuthSetupStepDefinition{
			{ID: "provider_configuration", Title: "Complete the service-provider application configuration", TitleI18n: loc("Complete the service-provider application configuration", "完成服务商第三方应用配置"), Description: "The ZGI deployment administrator configures the install callback, data callback, command callback, trusted domain, Suite ID, Suite Secret, Token, and EncodingAESKey.", DescriptionI18n: loc("The ZGI deployment administrator configures the install callback, data callback, command callback, trusted domain, Suite ID, Suite Secret, Token, and EncodingAESKey.", "由 ZGI 部署管理员配置安装完成回调、数据回调、指令回调、可信域名、Suite ID、Suite Secret、Token 和 EncodingAESKey。"), Action: integrations.AuthSetupStepActionOpenConsole},
			{ID: "start_install", Title: "Start authorization from ZGI", TitleI18n: loc("Start authorization from ZGI", "从 ZGI 发起企业授权"), Description: "Open the WeCom authorization page from this dialog. WeCom presents enterprise authorization as installing the third-party application; the state is single-use and expires after ten minutes.", DescriptionI18n: loc("Open the WeCom authorization page from this dialog. WeCom presents enterprise authorization as installing the third-party application; the state is single-use and expires after ten minutes.", "从此对话框打开企业微信授权页。企业微信会将企业授权显示为安装第三方应用；授权状态仅可使用一次，并会在十分钟后失效。")},
			{ID: "review_permissions", Title: "Review permissions and visibility", TitleI18n: loc("Review permissions and visibility", "审核权限与可见范围"), Description: "A WeCom system administrator confirms the long-term application permissions and member visibility before installing.", DescriptionI18n: loc("A WeCom system administrator confirms the long-term application permissions and member visibility before installing.", "企业微信系统管理员在安装前确认长期所需的应用权限与成员可见范围。")},
			{ID: "verify_connection", Title: "Return to ZGI and verify", TitleI18n: loc("Return to ZGI and verify", "返回 ZGI 并验证连接"), Description: "ZGI exchanges the one-time code for a permanent authorization, then uses short-lived enterprise tokens at runtime.", DescriptionI18n: loc("ZGI exchanges the one-time code for a permanent authorization, then uses short-lived enterprise tokens at runtime.", "ZGI 使用一次性临时码换取永久授权，运行时再按需换取短期企业凭证。")},
		}, Notices: []integrations.AuthSetupNoticeDefinition{
			{ID: "deployment_required", Level: integrations.AuthSetupNoticeLevelWarning, Text: "Installation is unavailable until the deployment has received a current Suite Ticket and all service-provider secrets are configured.", TextI18n: loc("Installation is unavailable until the deployment has received a current Suite Ticket and all service-provider secrets are configured.", "只有在部署已配置全部服务商密钥并收到最新 Suite Ticket 后，才可发起安装。")},
			{ID: "trusted_ip", Level: integrations.AuthSetupNoticeLevelInfo, Text: "WeCom API calls must originate from a public egress IP allowed in the service-provider platform.", TextI18n: loc("WeCom API calls must originate from a public egress IP allowed in the service-provider platform.", "企业微信 API 调用必须来自服务商平台已放行的公网出口 IP。")},
		}},
	}
}

func actions() []integrations.ActionDefinition {
	actions := []integrations.ActionDefinition{
		read(ActionAppGet, "get_wecom_application", "Get WeCom application", "获取企业微信应用", "Return the installed third-party application profile.", "返回已安装第三方应用的资料。", object(nil, nil), object(map[string]interface{}{"provider": str(64), "application": object(map[string]interface{}{"agent_id": str(64), "name": str(255), "square_logo_url": str(2048)}, []string{"agent_id", "name", "square_logo_url"})}, []string{"provider", "application"}), []string{ScopeApp}),
		read(ActionDepartmentList, "list_wecom_departments", "List WeCom departments", "列出企业微信部门", "List departments visible to the installed third-party application.", "列出已安装第三方应用可见范围内的部门。", object(map[string]interface{}{"department_id": titled(map[string]interface{}{"type": "integer", "minimum": 1}, "Department ID", "部门 ID")}, nil), object(map[string]interface{}{"provider": str(64), "departments": arr(object(map[string]interface{}{"id": map[string]interface{}{"type": "integer"}, "name": str(255), "parent_id": map[string]interface{}{"type": "integer"}}, []string{"id", "name", "parent_id"}), 200)}, []string{"provider", "departments"}), []string{ScopeContacts}),
		read(ActionContactSearch, "search_wecom_contacts", "Search WeCom members", "搜索企业微信成员", "Search active members in the application's visible range by name, name pinyin, or English name through WeCom's official provider search.", "通过企业微信官方服务商通讯录搜索，按姓名、姓名拼音或英文名搜索应用可见范围内的在职成员。", memberSearchInputSchema(), memberSearchOutputSchema(), []string{ScopeContacts}),
		read(ActionUserGet, "get_wecom_user", "Get WeCom member", "获取企业微信成员", "Read one member selected from a previous contact search.", "读取先前成员搜索中选择的一位成员。", object(map[string]interface{}{"recipient_ref": titled(nonblank(1024), "Recipient reference", "收件人引用")}, []string{"recipient_ref"}), object(map[string]interface{}{"provider": str(64), "member": object(map[string]interface{}{"recipient_ref": nonblank(1024), "name": str(255), "department_ids": arr(map[string]interface{}{"type": "integer"}, 50), "position": str(255), "status": map[string]interface{}{"type": "integer"}}, []string{"recipient_ref", "name", "department_ids", "position", "status"})}, []string{"provider", "member"}), []string{ScopeContacts}),
		write(ActionMessageSendUser, "send_wecom_user_message", "Send WeCom application message", "发送企业微信应用消息", "Send one plain-text application message to one or more members resolved by the official WeCom search after explicit approval.", "经明确确认后，向通过企业微信官方搜索解析出的一位或多位成员发送纯文本应用消息。", messageSendInputSchema(), messageSendOutputSchema(), []string{ScopeSend}),
	}
	for index := range actions {
		switch actions[index].ID {
		case ActionAppGet:
			actions[index].IntentAliases = []string{"wecom app profile", "企业微信应用资料", "企微应用资料"}
		case ActionDepartmentList:
			actions[index].IntentAliases = []string{"wecom departments", "企业微信部门", "企微部门"}
		case ActionContactSearch:
			actions[index].IntentAliases = []string{"search wecom members", "find wecom member", "wecom member search", "搜索企业微信成员", "查找企业微信成员", "按姓名查企业微信成员", "按拼音查企微成员", "查企微成员"}
		case ActionUserGet:
			actions[index].IntentAliases = []string{"wecom member details", "企业微信成员详情", "企微成员详情"}
		case ActionMessageSendUser:
			actions[index].IntentAliases = []string{"send wecom message", "企业微信发送消息", "企业微信发消息", "发送企微消息", "企微发消息"}
		}
	}
	return actions
}

func declaredGrantedScopes(config SuiteConfig) []string {
	scopes := []string{
		ScopeApp,
		ScopeContacts,
		ScopeSend,
	}
	if config.CustomerContactEnabled {
		scopes = append(scopes, ScopeCustomers, ScopeCustomerGroups)
	}
	if config.CustomerContactEnabled && config.CustomerMassMessageEnabled {
		scopes = append(scopes, ScopeCustomerMassMessage)
	}
	return scopes
}
func read(id, tool, en, zh, desc, zhDesc string, input, output map[string]interface{}, scopes []string) integrations.ActionDefinition {
	return integrations.ActionDefinition{ID: id, ToolName: tool, Name: en, NameI18n: loc(en, zh), Description: desc, DescriptionI18n: loc(desc, zhDesc), InputSchema: input, OutputSchema: output, Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow, DataEgress: true, ExternalDestination: "qyapi.weixin.qq.com", Idempotent: true, RequiredScopes: scopes, ScopeLabelsI18n: labels(scopes), SupportedAuthMethodIDs: []string{AuthMethodID}, DefaultPolicy: &integrations.DefaultActionPolicy{Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true}, SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent, tools.ToolInvokeFromWorkflow}}
}
func write(id, tool, en, zh, desc, zhDesc string, input, output map[string]interface{}, scopes []string) integrations.ActionDefinition {
	preparationDescription := "Resolve and confirm every intended member before sending."
	preparationDescriptionI18n := loc(preparationDescription, "发送前先查找并确认每一位目标成员。")
	return integrations.ActionDefinition{ID: id, ToolName: tool, Name: en, NameI18n: loc(en, zh), Description: desc, DescriptionI18n: loc(desc, zhDesc), InputSchema: input, OutputSchema: output, Effect: toolgovernance.EffectExternalSend, RiskLevel: toolgovernance.RiskLevelHigh, DataEgress: true, ExternalDestination: "qyapi.weixin.qq.com", Idempotent: false, RequiredScopes: scopes, ScopeLabelsI18n: labels(scopes), SupportedAuthMethodIDs: []string{AuthMethodID}, DefaultPolicy: &integrations.DefaultActionPolicy{Enabled: false, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: true}, SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat}, PreparationHints: []integrations.ActionPreparationHint{
		{ActionID: ActionContactSearch, Relation: integrations.ActionPreparationResolveTarget, TargetArguments: []string{"recipient_ref"}, ResultPaths: []string{"members[].recipient_ref"}, Description: preparationDescription, DescriptionI18n: preparationDescriptionI18n},
		{ActionID: ActionContactSearch, Relation: integrations.ActionPreparationResolveTarget, TargetArguments: []string{"recipient_refs"}, ResultPaths: []string{"members[].recipient_ref"}, Description: preparationDescription, DescriptionI18n: preparationDescriptionI18n},
	}, SuccessDeduplication: &integrations.SuccessDeduplicationDefinition{TargetArgumentPaths: []string{"recipient_ref", "recipient_refs"}}}
}

func memberSearchInputSchema() map[string]interface{} {
	matchMode := titled(map[string]interface{}{"type": "string", "enum": []string{"fuzzy", "exact"}, "default": "fuzzy"}, "Match mode", "匹配方式")
	matchMode["enum_labels_i18n"] = map[string]map[string]string{
		integrations.LocaleEnglishUS:         {"fuzzy": "Fuzzy", "exact": "Exact name"},
		integrations.LocaleSimplifiedChinese: {"fuzzy": "模糊匹配", "exact": "精确姓名匹配"},
	}
	return object(map[string]interface{}{
		"query":         titled(nonblank(128), "Member name, pinyin, or English name", "成员姓名、拼音或英文名"),
		"match_mode":    matchMode,
		"cursor":        titled(str(512), "Pagination cursor", "分页游标"),
		"max_results":   titled(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 20, "default": 10}, "Maximum results", "最大结果数"),
		"department_id": titled(map[string]interface{}{"type": "integer", "minimum": 1, "deprecated": true}, "Deprecated department filter", "已弃用的部门过滤条件"),
	}, []string{"query"})
}

func memberSearchOutputSchema() map[string]interface{} {
	return object(map[string]interface{}{
		"provider": str(64),
		"members": arr(object(map[string]interface{}{
			"recipient_ref":  nonblank(1024),
			"name":           str(255),
			"department_ids": arr(map[string]interface{}{"type": "integer"}, 50),
		}, []string{"recipient_ref", "name", "department_ids"}), 20),
		"is_last":            map[string]interface{}{"type": "boolean"},
		"next_cursor":        str(512),
		"requires_selection": map[string]interface{}{"type": "boolean"},
	}, []string{"provider", "members", "is_last", "next_cursor", "requires_selection"})
}

func messageSendInputSchema() map[string]interface{} {
	schema := object(map[string]interface{}{
		"recipient_ref":  titled(nonblank(1024), "Recipient reference", "收件人引用"),
		"recipient_refs": titled(map[string]interface{}{"type": "array", "minItems": 1, "maxItems": maxMessageRecipients, "uniqueItems": true, "items": nonblank(1024)}, "Recipient references", "收件人引用列表"),
		"content":        titled(nonblank(2048), "Message content (maximum 2048 UTF-8 bytes)", "消息内容（最多 2048 个 UTF-8 字节）"),
		"safe":           titled(map[string]interface{}{"type": "boolean", "default": false}, "Confidential message", "保密消息"),
	}, []string{"content"})
	schema["oneOf"] = []interface{}{
		map[string]interface{}{"required": []string{"recipient_ref"}},
		map[string]interface{}{"required": []string{"recipient_refs"}},
	}
	return schema
}

func messageSendOutputSchema() map[string]interface{} {
	return object(map[string]interface{}{
		"provider":         str(64),
		"operation_status": map[string]interface{}{"type": "string", "enum": []string{"succeeded", "partially_succeeded"}},
		"message": object(map[string]interface{}{
			"message_id":            nonblank(255),
			"recipient_refs":        arr(nonblank(1024), maxMessageRecipients),
			"provider_accepted":     map[string]interface{}{"const": true},
			"requested_count":       map[string]interface{}{"type": "integer", "minimum": 1},
			"accepted_count":        map[string]interface{}{"type": "integer", "minimum": 1},
			"rejected_count":        map[string]interface{}{"type": "integer", "minimum": 0},
			"invalid_recipients":    arr(nonblank(1024), maxMessageRecipients),
			"unlicensed_recipients": arr(nonblank(1024), maxMessageRecipients),
			"invalid_party_count":   map[string]interface{}{"type": "integer", "minimum": 0},
			"invalid_tag_count":     map[string]interface{}{"type": "integer", "minimum": 0},
			"delivery_status":       map[string]interface{}{"type": "string", "const": "accepted_unknown_delivery"},
			"operation_status":      map[string]interface{}{"type": "string", "enum": []string{"succeeded", "partially_succeeded"}},
		}, []string{"message_id", "recipient_refs", "provider_accepted", "requested_count", "accepted_count", "rejected_count", "invalid_recipients", "unlicensed_recipients", "invalid_party_count", "invalid_tag_count", "delivery_status", "operation_status"}),
	}, []string{"provider", "operation_status", "message"})
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
		case ScopeCustomers:
			out[scope] = loc("Read provider-visible customers", "读取服务商可见客户")
		case ScopeCustomerGroups:
			out[scope] = loc("Read provider-visible customer groups", "读取服务商可见客户群")
		case ScopeCustomerMassMessage:
			out[scope] = loc("Access customer mass-message tasks and history", "访问客户群发任务与历史")
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
