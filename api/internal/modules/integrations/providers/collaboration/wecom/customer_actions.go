package wecom

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	ActionCustomerFollowUserList         = "wecom.customer.follow_user.list"
	ActionCustomerListByOwner            = "wecom.customer.list_by_owner"
	ActionCustomerGet                    = "wecom.customer.get"
	ActionCustomerBatchGetByOwner        = "wecom.customer.batch_get_by_owner"
	ActionCustomerGroupList              = "wecom.customer_group.list"
	ActionCustomerGroupGet               = "wecom.customer_group.get"
	ActionCustomerMassMessageCreate      = "wecom.customer_mass_message.create_for_customers"
	ActionCustomerGroupMessageCreate     = "wecom.customer_mass_message.create_for_groups"
	ActionCustomerMassMessageHistoryList = "wecom.customer_mass_message.history.list"
	ActionCustomerMassMessageTaskList    = "wecom.customer_mass_message.task.list"
	ActionCustomerMassMessageResultList  = "wecom.customer_mass_message.send_result.list"

	ScopeCustomers           = "wecom:customers:read"
	ScopeCustomerGroups      = "wecom:customer_groups:read"
	ScopeCustomerMassMessage = "wecom:customer_mass_message:create"
)

const providerVisibleCoverage = "provider_visible_scope"

const maxCustomerGroupMassMessageTargets = 100

const (
	customerTokenVersion       = 1
	customerPageCursorPrefix   = "wcp1."
	customerCursorFollowUsers  = "follow_users"
	customerCursorOwnerClients = "customers_by_owner"
)

var customerProviderRestrictions = []interface{}{
	"authorized_app_visibility",
	"customer_contact_permissions",
	"provider_data_availability_rules",
}

type externalContactSummary struct {
	ExternalUserID string `json:"external_userid"`
	Name           string `json:"name"`
	Type           int    `json:"type"`
	CorpName       string `json:"corp_name"`
}

type externalContactFollowInfo struct {
	UserID     string `json:"userid"`
	Remark     string `json:"remark"`
	CreateTime int64  `json:"createtime"`
	AddWay     int    `json:"add_way"`
}

type externalContactDetail struct {
	apiEnvelope
	ExternalContact externalContactSummary      `json:"external_contact"`
	FollowUsers     []externalContactFollowInfo `json:"follow_user"`
	NextCursor      string                      `json:"next_cursor"`
}

type batchExternalContactItem struct {
	ExternalContact externalContactSummary    `json:"external_contact"`
	FollowInfo      externalContactFollowInfo `json:"follow_info"`
}

type customerPageCursorClaims struct {
	Version        int    `json:"v"`
	Kind           string `json:"kind"`
	QueryBind      string `json:"query_bind"`
	Offset         int    `json:"offset"`
	ConnectionBind string `json:"connection_bind"`
	TenantBind     string `json:"tenant_bind"`
	AgentBind      string `json:"agent_bind"`
}

func customerActions() []integrations.ActionDefinition {
	readCallers := []tools.ToolInvokeFrom{
		tools.ToolInvokeFromAIChat,
		tools.ToolInvokeFromAgent,
		tools.ToolInvokeFromWorkflow,
	}
	readAction := func(id, toolName, en, zh, description, zhDescription string, input, output map[string]interface{}, scopes []string) integrations.ActionDefinition {
		return integrations.ActionDefinition{
			ID: id, ToolName: toolName, Name: en, NameI18n: loc(en, zh),
			Description: description, DescriptionI18n: loc(description, zhDescription),
			InputSchema: input, OutputSchema: output, Effect: toolgovernance.EffectRead,
			RiskLevel: toolgovernance.RiskLevelLow, DataEgress: true,
			ExternalDestination: "qyapi.weixin.qq.com", Idempotent: true,
			RequiredScopes: scopes, ScopeLabelsI18n: labels(scopes),
			SupportedAuthMethodIDs: []string{AuthMethodID},
			DefaultPolicy: &integrations.DefaultActionPolicy{
				Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true,
			},
			SupportedCallers: readCallers,
		}
	}

	ownerRef := titled(nonblank(4096), "Member reference", "成员引用")
	customerRef := titled(nonblank(4096), "Customer reference", "客户引用")
	groupRef := titled(nonblank(4096), "Customer group reference", "客户群引用")
	coverageProps := map[string]interface{}{
		"coverage":              map[string]interface{}{"type": "string", "enum": []interface{}{providerVisibleCoverage}},
		"provider_restrictions": arr(str(64), 8),
	}
	memberRefItem := object(map[string]interface{}{"owner_ref": nonblank(4096)}, []string{"owner_ref"})
	customerRefItem := object(map[string]interface{}{"customer_ref": nonblank(4096)}, []string{"customer_ref"})
	groupListItem := object(map[string]interface{}{
		"group_ref": nonblank(4096), "status": map[string]interface{}{"type": "integer"},
	}, []string{"group_ref", "status"})
	followUserItem := object(map[string]interface{}{
		"owner_ref": nonblank(4096), "remark": str(255),
		"created_at": map[string]interface{}{"type": "integer"}, "add_way": map[string]interface{}{"type": "integer"},
	}, []string{"owner_ref", "remark", "created_at", "add_way"})
	customerItem := object(map[string]interface{}{
		"customer_ref": nonblank(4096), "name": str(255), "type": map[string]interface{}{"type": "integer"},
		"corp_name": str(255), "follow_users": arr(followUserItem, 500),
	}, []string{"customer_ref", "name", "type", "corp_name", "follow_users"})

	followOutputProps := cloneSchemaProperties(coverageProps)
	followOutputProps["owners"] = arr(memberRefItem, 500)
	followOutputProps["truncated"] = map[string]interface{}{"type": "boolean"}
	followOutputProps["next_cursor"] = str(2048)
	followOutputProps["is_last"] = map[string]interface{}{"type": "boolean"}

	listCustomerOutputProps := cloneSchemaProperties(coverageProps)
	listCustomerOutputProps["customer_refs"] = arr(customerRefItem, 500)
	listCustomerOutputProps["truncated"] = map[string]interface{}{"type": "boolean"}
	listCustomerOutputProps["next_cursor"] = str(2048)
	listCustomerOutputProps["is_last"] = map[string]interface{}{"type": "boolean"}

	getCustomerOutputProps := cloneSchemaProperties(coverageProps)
	getCustomerOutputProps["customer"] = customerItem
	getCustomerOutputProps["next_cursor"] = str(2048)
	getCustomerOutputProps["is_last"] = map[string]interface{}{"type": "boolean"}

	batchCustomerOutputProps := cloneSchemaProperties(coverageProps)
	batchCustomerOutputProps["customers"] = arr(customerItem, 100)
	batchCustomerOutputProps["next_cursor"] = str(2048)
	batchCustomerOutputProps["is_last"] = map[string]interface{}{"type": "boolean"}

	groupListOutputProps := cloneSchemaProperties(coverageProps)
	groupListOutputProps["groups"] = arr(groupListItem, 200)
	groupListOutputProps["next_cursor"] = str(2048)
	groupListOutputProps["is_last"] = map[string]interface{}{"type": "boolean"}

	groupMemberItem := object(map[string]interface{}{
		"subject_ref": nonblank(4096),
		"member_type": map[string]interface{}{"type": "string", "enum": []interface{}{"internal_member", "customer"}},
	}, []string{"subject_ref", "member_type"})
	groupDetail := object(map[string]interface{}{
		"group_ref": nonblank(4096), "name": str(255), "owner_ref": nonblank(4096),
		"created_at": map[string]interface{}{"type": "integer"}, "member_count": map[string]interface{}{"type": "integer", "minimum": 0},
		"members": arr(groupMemberItem, 1000),
	}, []string{"group_ref", "name", "owner_ref", "created_at", "member_count", "members"})
	groupGetOutputProps := cloneSchemaProperties(coverageProps)
	groupGetOutputProps["group"] = groupDetail
	taskRef := titled(nonblank(4096), "Mass-message task reference", "群发任务引用")
	taskListOutput := object(map[string]interface{}{
		"provider": str(64),
		"tasks": arr(object(map[string]interface{}{
			"sender_ref": nonblank(4096),
			"status":     map[string]interface{}{"type": "string", "enum": []interface{}{"pending_member_send", "sent", "unknown"}},
			"raw_status": map[string]interface{}{"type": "integer"},
			"sent_at":    map[string]interface{}{"type": "integer"},
		}, []string{"sender_ref", "status", "raw_status"}), 1000),
		"next_cursor": str(2048),
		"is_last":     map[string]interface{}{"type": "boolean"},
	}, []string{"provider", "tasks", "next_cursor", "is_last"})
	sendResultOutput := object(map[string]interface{}{
		"provider": str(64),
		"results": arr(object(map[string]interface{}{
			"target_ref":  nonblank(4096),
			"target_type": map[string]interface{}{"type": "string", "enum": []interface{}{"customer", "customer_group"}},
			"status":      map[string]interface{}{"type": "string", "enum": []interface{}{"pending", "sent", "failed_customer_not_friend", "failed_duplicate_mass_message", "unknown"}},
			"raw_status":  map[string]interface{}{"type": "integer"},
			"sent_at":     map[string]interface{}{"type": "integer"},
		}, []string{"target_ref", "target_type", "status", "raw_status"}), 1000),
		"next_cursor": str(2048),
		"is_last":     map[string]interface{}{"type": "boolean"},
	}, []string{"provider", "results", "next_cursor", "is_last"})

	actions := []integrations.ActionDefinition{
		readAction(
			ActionCustomerFollowUserList, "list_wecom_customer_follow_users", "List WeCom customer-contact members", "列出企业微信客户联系成员",
			"List provider-visible members configured for customer contact and return opaque member references.",
			"列出服务商当前可见且已配置客户联系功能的成员，并返回不透明成员引用。",
			object(map[string]interface{}{
				"max_results": titled(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 500, "default": 100}, "Maximum results", "最大结果数"),
				"cursor":      titled(str(2048), "Pagination cursor", "分页游标"),
			}, nil),
			object(followOutputProps, []string{"owners", "truncated", "next_cursor", "is_last", "coverage", "provider_restrictions"}), []string{ScopeCustomers},
		),
		readAction(
			ActionCustomerListByOwner, "list_wecom_customers_by_owner", "List WeCom customers by owner", "按成员列出企业微信客户",
			"List customers followed by one resolved member. A missing result means only that no customer is visible in the current provider scope.",
			"列出一位已解析成员所跟进的客户；未返回结果只表示当前服务商可见范围内未找到。",
			object(map[string]interface{}{
				"owner_ref":   ownerRef,
				"max_results": titled(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 500, "default": 100}, "Maximum results", "最大结果数"),
				"cursor":      titled(str(2048), "Pagination cursor", "分页游标"),
			}, []string{"owner_ref"}),
			object(listCustomerOutputProps, []string{"customer_refs", "truncated", "next_cursor", "is_last", "coverage", "provider_restrictions"}), []string{ScopeCustomers},
		),
		readAction(
			ActionCustomerGet, "get_wecom_customer", "Get WeCom customer", "获取企业微信客户",
			"Read one customer selected from a provider-visible result. The customer reference cannot be used as an internal member recipient.",
			"读取服务商可见结果中的一位客户；客户引用不能作为内部成员消息接收人。",
			object(map[string]interface{}{"customer_ref": customerRef, "cursor": titled(str(2048), "Pagination cursor", "分页游标")}, []string{"customer_ref"}),
			object(getCustomerOutputProps, []string{"customer", "next_cursor", "is_last", "coverage", "provider_restrictions"}), []string{ScopeCustomers},
		),
		readAction(
			ActionCustomerBatchGetByOwner, "batch_get_wecom_customers_by_owner", "Batch get WeCom customers by owner", "按成员批量获取企业微信客户",
			"Page through customer details for up to 100 resolved owner members.", "分页获取最多 100 位已解析成员所跟进的客户详情。",
			object(map[string]interface{}{
				"owner_refs": titled(map[string]interface{}{"type": "array", "minItems": 1, "maxItems": 100, "items": nonblank(4096)}, "Member references", "成员引用"),
				"cursor":     titled(str(2048), "Pagination cursor", "分页游标"),
				"limit":      titled(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 100, "default": 50}, "Page size", "分页数量"),
			}, []string{"owner_refs"}),
			object(batchCustomerOutputProps, []string{"customers", "next_cursor", "is_last", "coverage", "provider_restrictions"}), []string{ScopeCustomers},
		),
		readAction(
			ActionCustomerGroupList, "list_wecom_customer_groups", "List WeCom customer groups", "列出企业微信客户群",
			"List customer groups for explicitly resolved owner members. Omitting the owner scope is not allowed.",
			"列出明确指定群主成员的客户群；不允许省略群主范围进行全量拉取。",
			object(map[string]interface{}{
				"owner_refs":    titled(map[string]interface{}{"type": "array", "minItems": 1, "maxItems": 100, "items": nonblank(4096)}, "Owner member references", "群主成员引用"),
				"status_filter": titled(map[string]interface{}{"type": "integer", "minimum": 0, "maximum": 3, "default": 0}, "Follow-up status", "跟进状态"),
				"cursor":        titled(str(2048), "Pagination cursor", "分页游标"),
				"limit":         titled(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 200, "default": 100}, "Page size", "分页数量"),
			}, []string{"owner_refs"}),
			object(groupListOutputProps, []string{"groups", "next_cursor", "is_last", "coverage", "provider_restrictions"}), []string{ScopeCustomerGroups},
		),
		readAction(
			ActionCustomerGroupGet, "get_wecom_customer_group", "Get WeCom customer group", "获取企业微信客户群",
			"Read one customer group without requesting member display names, returning typed opaque member and customer references.",
			"读取一个客户群且不请求群成员姓名，返回带类型的不透明内部成员或客户引用。",
			object(map[string]interface{}{"group_ref": groupRef}, []string{"group_ref"}),
			object(groupGetOutputProps, []string{"group", "coverage", "provider_restrictions"}), []string{ScopeCustomerGroups},
		),
		readAction(
			ActionCustomerMassMessageTaskList, "list_wecom_customer_mass_message_tasks", "List WeCom mass-message member tasks", "列出企业微信群发成员任务",
			"Read the member-confirmation tasks for a previously created mass-message task.",
			"读取此前创建的群发任务所对应的成员确认任务。",
			object(map[string]interface{}{
				"task_ref": taskRef,
				"cursor":   titled(str(2048), "Pagination cursor", "分页游标"),
				"limit":    titled(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 1000, "default": 500}, "Page size", "分页数量"),
			}, []string{"task_ref"}), taskListOutput, []string{ScopeCustomerMassMessage},
		),
		readAction(
			ActionCustomerMassMessageResultList, "list_wecom_customer_mass_message_results", "List WeCom mass-message send results", "列出企业微信群发发送结果",
			"Read customer or customer-group send results for one member task.",
			"读取一位成员任务对应的客户或客户群发送结果。",
			object(map[string]interface{}{
				"task_ref":   taskRef,
				"sender_ref": ownerRef,
				"cursor":     titled(str(2048), "Pagination cursor", "分页游标"),
				"limit":      titled(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 1000, "default": 500}, "Page size", "分页数量"),
			}, []string{"task_ref", "sender_ref"}), sendResultOutput, []string{ScopeCustomerMassMessage},
		),
	}
	actions = append(actions, customerMassMessageHistoryAction())

	actions[0].IntentAliases = []string{"wecom customer contact members", "企业微信客户联系成员", "企微客户联系成员"}
	actions[1].IntentAliases = []string{"wecom customers by owner", "企业微信成员客户", "查询企微客户"}
	actions[2].IntentAliases = []string{"wecom customer details", "企业微信客户详情", "企微客户详情"}
	actions[3].IntentAliases = []string{"batch wecom customers", "批量企业微信客户", "批量企微客户"}
	actions[4].IntentAliases = []string{"wecom customer groups", "企业微信客户群", "企微客户群"}
	actions[5].IntentAliases = []string{"wecom customer group details", "企业微信客户群详情", "企微客户群详情"}
	actions[6].IntentAliases = []string{"wecom mass message tasks", "企业微信群发成员任务", "企微群发任务状态"}
	actions[7].IntentAliases = []string{"wecom mass message results", "企业微信群发发送结果", "企微群发结果"}

	actions = append(actions,
		customerMassMessageAction(false),
		customerMassMessageAction(true),
	)
	return actions
}

func customerMassMessageAction(group bool) integrations.ActionDefinition {
	id := ActionCustomerMassMessageCreate
	toolName := "create_wecom_customer_mass_message"
	en, zh := "Create WeCom customer mass-message task", "创建企业微信客户群发任务"
	description := "Create a member-confirmed mass-message task for selected customers. Success means task_created, not delivered."
	zhDescription := "为选定客户创建需成员确认发送的群发任务；成功仅表示任务已创建，不表示消息已送达。"
	referenceKey := "customer_refs"
	referenceTitle, referenceTitleZH := "Customer references", "客户引用"
	maxItems := 10000
	if group {
		id = ActionCustomerGroupMessageCreate
		toolName = "create_wecom_customer_group_mass_message"
		en, zh = "Create WeCom customer-group mass-message task", "创建企业微信客户群群发任务"
		description = "Create a member-confirmed mass-message task for selected customer groups. Success means task_created, not delivered."
		zhDescription = "为选定客户群创建需成员确认发送的群发任务；成功仅表示任务已创建，不表示消息已送达。"
		referenceKey = "group_refs"
		referenceTitle, referenceTitleZH = "Customer group references", "客户群引用"
		maxItems = maxCustomerGroupMassMessageTargets
	}
	properties := map[string]interface{}{
		referenceKey: titled(map[string]interface{}{"type": "array", "minItems": 1, "maxItems": maxItems, "items": nonblank(4096)}, referenceTitle, referenceTitleZH),
		"content":    titled(nonblank(4000), "Message text", "消息文本"),
	}
	required := []string{referenceKey, "content"}
	if group {
		properties["sender_ref"] = titled(nonblank(4096), "Sender member reference", "发送成员引用")
		required = append(required, "sender_ref")
	} else {
		properties["sender_ref"] = titled(nonblank(4096), "Optional sender member reference", "可选发送成员引用")
	}
	output := object(map[string]interface{}{
		"provider":         str(64),
		"operation_status": map[string]interface{}{"type": "string", "enum": []string{"succeeded", "partially_succeeded"}},
		"task": object(map[string]interface{}{
			"task_ref": nonblank(4096), "task_status": map[string]interface{}{"const": "created_pending_member_send"},
			"delivery_status":   map[string]interface{}{"const": "pending_member_send"},
			"provider_accepted": map[string]interface{}{"const": true},
			"accepted_count":    map[string]interface{}{"type": "integer", "minimum": 0},
			"rejected_count":    map[string]interface{}{"type": "integer", "minimum": 0},
			"rejected_refs":     arr(nonblank(4096), maxItems),
		}, []string{"task_ref", "task_status", "delivery_status", "provider_accepted", "accepted_count", "rejected_count", "rejected_refs"}),
	}, []string{"provider", "operation_status", "task"})
	action := integrations.ActionDefinition{
		ID: id, ToolName: toolName, Name: en, NameI18n: loc(en, zh),
		Description: description, DescriptionI18n: loc(description, zhDescription),
		InputSchema: object(properties, required), OutputSchema: output,
		Effect: toolgovernance.EffectExternalSend, RiskLevel: toolgovernance.RiskLevelHigh,
		DataEgress: true, ExternalDestination: "qyapi.weixin.qq.com", Idempotent: false,
		RequiredScopes: []string{ScopeCustomerMassMessage}, ScopeLabelsI18n: labels([]string{ScopeCustomerMassMessage}),
		SupportedAuthMethodIDs: []string{AuthMethodID},
		DefaultPolicy: &integrations.DefaultActionPolicy{
			Enabled: false, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: true,
		},
		SupportedCallers:     []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat},
		SuccessDeduplication: &integrations.SuccessDeduplicationDefinition{TargetArgumentPaths: []string{referenceKey}},
	}
	if group {
		action.IntentAliases = []string{"create wecom customer group mass message", "创建企业微信客户群群发", "创建企微客户群群发"}
		action.PreparationHints = []integrations.ActionPreparationHint{{
			ActionID: ActionCustomerGroupList, Relation: integrations.ActionPreparationResolveTarget,
			TargetArguments: []string{"group_refs"}, ResultPaths: []string{"groups[].group_ref"},
			Description:     "Resolve customer groups before creating the member-confirmed task.",
			DescriptionI18n: loc("Resolve customer groups before creating the member-confirmed task.", "创建需成员确认的任务前先解析客户群。"),
		}}
	} else {
		action.IntentAliases = []string{"create wecom customer mass message", "创建企业微信客户群发", "创建企微客户群发"}
		action.PreparationHints = []integrations.ActionPreparationHint{{
			ActionID: ActionCustomerListByOwner, Relation: integrations.ActionPreparationResolveTarget,
			TargetArguments: []string{"customer_refs"}, ResultPaths: []string{"customer_refs[].customer_ref"},
			Description:     "Resolve customers before creating the member-confirmed task.",
			DescriptionI18n: loc("Resolve customers before creating the member-confirmed task.", "创建需成员确认的任务前先解析客户。"),
		}}
	}
	return action
}

func cloneSchemaProperties(input map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(input)+4)
	for key, value := range input {
		out[key] = value
	}
	return out
}

func customerActionScopes() []integrations.ProviderScopeDefinition {
	return []integrations.ProviderScopeDefinition{
		{ID: ScopeCustomers, Label: "Read provider-visible customers", LabelI18n: loc("Read provider-visible customers", "读取服务商可见客户"), Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead, Broad: true},
		{ID: ScopeCustomerGroups, Label: "Read provider-visible customer groups", LabelI18n: loc("Read provider-visible customer groups", "读取服务商可见客户群"), Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead, Broad: true},
		{ID: ScopeCustomerMassMessage, Label: "Access customer mass-message tasks and history", LabelI18n: loc("Access customer mass-message tasks and history", "访问客户群发任务与历史"), Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite, Broad: true},
	}
}

func customerCoverageOutput() map[string]interface{} {
	return map[string]interface{}{
		"coverage":              providerVisibleCoverage,
		"provider_restrictions": append([]interface{}(nil), customerProviderRestrictions...),
	}
}

func customerCursorQueryBind(creds credentials, kind, value string) string {
	return strings.Join([]string{kind, creds.ConnectionID, creds.AuthCorpID, creds.AgentID, strings.TrimSpace(value)}, "\x00")
}

func (adapter *Adapter) encodeCustomerPageCursor(creds credentials, kind, query string, offset int) (string, error) {
	if offset < 1 {
		return "", fmt.Errorf("invalid customer page offset")
	}
	claims := customerPageCursorClaims{
		Version: customerTokenVersion, Kind: kind,
		QueryBind: adapter.subjectBinding("customer-page-query", query), Offset: offset,
		ConnectionBind: adapter.subjectBinding("connection", creds.ConnectionID),
		TenantBind:     adapter.subjectBinding("tenant", creds.AuthCorpID),
		AgentBind:      adapter.subjectBinding("agent", creds.AgentID),
	}
	return adapter.encodeCustomerToken(customerPageCursorPrefix, "customer-page-cursor", claims)
}

func (adapter *Adapter) decodeCustomerPageCursor(value string, creds credentials, kind, query string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	var claims customerPageCursorClaims
	if err := adapter.decodeCustomerToken(value, customerPageCursorPrefix, "customer-page-cursor", &claims); err != nil {
		return 0, err
	}
	if claims.Version != customerTokenVersion || claims.Kind != kind || claims.Offset < 1 ||
		claims.QueryBind != adapter.subjectBinding("customer-page-query", query) ||
		claims.ConnectionBind != adapter.subjectBinding("connection", creds.ConnectionID) ||
		claims.TenantBind != adapter.subjectBinding("tenant", creds.AuthCorpID) ||
		claims.AgentBind != adapter.subjectBinding("agent", creds.AgentID) {
		return 0, fmt.Errorf("customer page cursor binding mismatch")
	}
	return claims.Offset, nil
}

func (adapter *Adapter) encodeCustomerTaskRef(creds credentials, taskID string) (string, error) {
	return adapter.encodeSubjectRef(creds, SubjectTypeMassMessageTask, SubjectIDKindMsgID, taskID, "", "")
}

func (adapter *Adapter) decodeCustomerTaskRef(value string, creds credentials) (string, error) {
	claims, err := adapter.decodeSubjectRef(value, creds, SubjectTypeMassMessageTask, SubjectIDKindMsgID)
	if err != nil {
		return "", err
	}
	return claims.SubjectID, nil
}

func (adapter *Adapter) encodeCustomerToken(prefix, domain string, claims interface{}) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signature, err := adapter.signCustomerToken(domain, payload)
	if err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (adapter *Adapter) decodeCustomerToken(value, prefix, domain string, output interface{}) error {
	if !strings.HasPrefix(value, prefix) || len(value) > 4096 {
		return fmt.Errorf("invalid customer token")
	}
	parts := strings.Split(strings.TrimPrefix(value, prefix), ".")
	if len(parts) != 2 {
		return fmt.Errorf("invalid customer token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(payload) == 0 || len(payload) > 3072 {
		return fmt.Errorf("invalid customer token payload")
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(provided) != sha256.Size {
		return fmt.Errorf("invalid customer token signature")
	}
	expected, err := adapter.signCustomerToken(domain, payload)
	if err != nil || !hmac.Equal(provided, expected) {
		return fmt.Errorf("invalid customer token signature")
	}
	if err := json.Unmarshal(payload, output); err != nil {
		return fmt.Errorf("invalid customer token payload")
	}
	return nil
}

func (adapter *Adapter) signCustomerToken(domain string, payload []byte) ([]byte, error) {
	secret := strings.TrimSpace(adapter.suite.SuiteSecret)
	if secret == "" {
		return nil, fmt.Errorf("WeCom customer token signing key is unavailable")
	}
	key := sha256.Sum256([]byte("zgi/wecom/" + domain + "/v1\x00" + secret))
	digest := hmac.New(sha256.New, key[:])
	_, _ = digest.Write(payload)
	return digest.Sum(nil), nil
}

func (adapter *Adapter) executeCustomerAction(ctx context.Context, actionID string, creds credentials, input map[string]interface{}) (*integrations.ActionResult, bool, error) {
	if isCustomerContactAction(actionID) && !adapter.suite.CustomerContactEnabled {
		return nil, true, integrations.NewErrorWithReason(integrations.ErrorCodeDisabled, "wecom_customer_contact_disabled", "WeCom customer-contact actions are disabled by deployment configuration", nil)
	}
	if isCustomerMassMessageAction(actionID) && (!adapter.suite.CustomerContactEnabled || !adapter.suite.CustomerMassMessageEnabled) {
		return nil, true, integrations.NewErrorWithReason(integrations.ErrorCodeDisabled, "wecom_customer_mass_message_disabled", "WeCom customer mass-message actions are disabled by deployment configuration", nil)
	}
	switch actionID {
	case ActionCustomerFollowUserList:
		output, count, requestID, err := adapter.listCustomerFollowUsers(ctx, creds, input)
		return wecomResultWithRequestID(output, count, requestID), true, err
	case ActionCustomerListByOwner:
		output, count, requestID, err := adapter.listCustomersByOwner(ctx, creds, input)
		return wecomResultWithRequestID(output, count, requestID), true, err
	case ActionCustomerGet:
		output, requestID, err := adapter.getCustomer(ctx, creds, input)
		return wecomResultWithRequestID(output, 1, requestID), true, err
	case ActionCustomerBatchGetByOwner:
		output, count, requestID, err := adapter.batchGetCustomersByOwner(ctx, creds, input)
		return wecomResultWithRequestID(output, count, requestID), true, err
	case ActionCustomerGroupList:
		output, count, requestID, err := adapter.listCustomerGroups(ctx, creds, input)
		return wecomResultWithRequestID(output, count, requestID), true, err
	case ActionCustomerGroupGet:
		output, requestID, err := adapter.getCustomerGroup(ctx, creds, input)
		return wecomResultWithRequestID(output, 1, requestID), true, err
	case ActionCustomerMassMessageCreate:
		output, count, requestID, err := adapter.createCustomerMassMessage(ctx, creds, input, false)
		return wecomResultWithRequestID(output, count, requestID), true, err
	case ActionCustomerGroupMessageCreate:
		output, count, requestID, err := adapter.createCustomerMassMessage(ctx, creds, input, true)
		return wecomResultWithRequestID(output, count, requestID), true, err
	case ActionCustomerMassMessageHistoryList:
		output, count, requestID, err := adapter.listCustomerMassMessageHistory(ctx, creds, input)
		return wecomResultWithRequestID(output, count, requestID), true, err
	case ActionCustomerMassMessageTaskList:
		output, count, requestID, err := adapter.listCustomerMassMessageTasks(ctx, creds, input)
		return wecomResultWithRequestID(output, count, requestID), true, err
	case ActionCustomerMassMessageResultList:
		output, count, requestID, err := adapter.listCustomerMassMessageResults(ctx, creds, input)
		return wecomResultWithRequestID(output, count, requestID), true, err
	default:
		return nil, false, nil
	}
}

func isCustomerContactAction(actionID string) bool {
	switch actionID {
	case ActionCustomerFollowUserList, ActionCustomerListByOwner, ActionCustomerGet,
		ActionCustomerBatchGetByOwner, ActionCustomerGroupList, ActionCustomerGroupGet:
		return true
	default:
		return false
	}
}

func isCustomerMassMessageAction(actionID string) bool {
	switch actionID {
	case ActionCustomerMassMessageCreate, ActionCustomerGroupMessageCreate,
		ActionCustomerMassMessageHistoryList, ActionCustomerMassMessageTaskList, ActionCustomerMassMessageResultList:
		return true
	default:
		return false
	}
}

func (adapter *Adapter) listCustomerFollowUsers(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, string, error) {
	maxResults := inputInt(input, "max_results", 100)
	if maxResults < 1 {
		maxResults = 100
	}
	if maxResults > 500 {
		maxResults = 500
	}
	offset, err := adapter.decodeCustomerPageCursor(inputString(input, "cursor"), creds, customerCursorFollowUsers, customerCursorQueryBind(creds, customerCursorFollowUsers, ""))
	if err != nil {
		return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom customer-contact member cursor is invalid", err)
	}
	var response struct {
		apiEnvelope
		FollowUsers []string `json:"follow_user"`
	}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodGet, "/cgi-bin/externalcontact/get_follow_user_list", nil, nil, &response); err != nil {
		return nil, 0, "", err
	}
	end := min(offset+maxResults, len(response.FollowUsers))
	if offset > len(response.FollowUsers) {
		offset = len(response.FollowUsers)
		end = offset
	}
	owners := make([]map[string]interface{}, 0, end-offset)
	for _, userID := range response.FollowUsers[offset:end] {
		ref, err := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindUserID, userID, "", "")
		if err != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an invalid customer-contact member", err)
		}
		owners = append(owners, map[string]interface{}{"owner_ref": ref})
	}
	output := customerCoverageOutput()
	output["owners"] = owners
	isLast := end >= len(response.FollowUsers)
	output["truncated"] = !isLast
	output["is_last"] = isLast
	output["next_cursor"] = ""
	if !isLast {
		nextCursor, cursorErr := adapter.encodeCustomerPageCursor(creds, customerCursorFollowUsers, customerCursorQueryBind(creds, customerCursorFollowUsers, ""), end)
		if cursorErr != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom customer-contact member cursor could not be created", cursorErr)
		}
		output["next_cursor"] = nextCursor
	}
	return output, len(owners), firstNonEmpty(response.RequestID, response.RequestIDAlt), nil
}

func (adapter *Adapter) listCustomersByOwner(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, string, error) {
	owner, err := adapter.decodeSubjectRef(inputString(input, "owner_ref"), creds, SubjectTypeMember, SubjectIDKindUserID)
	if err != nil {
		return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom customer owner reference is invalid", err)
	}
	maxResults := inputInt(input, "max_results", 100)
	if maxResults < 1 {
		maxResults = 100
	}
	if maxResults > 500 {
		maxResults = 500
	}
	queryBind := customerCursorQueryBind(creds, customerCursorOwnerClients, owner.SubjectID)
	offset, err := adapter.decodeCustomerPageCursor(inputString(input, "cursor"), creds, customerCursorOwnerClients, queryBind)
	if err != nil {
		return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom customer list cursor is invalid", err)
	}
	var response struct {
		apiEnvelope
		ExternalUserIDs []string `json:"external_userid"`
	}
	query := url.Values{"userid": []string{owner.SubjectID}}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodGet, "/cgi-bin/externalcontact/list", query, nil, &response); err != nil {
		return nil, 0, "", err
	}
	end := min(offset+maxResults, len(response.ExternalUserIDs))
	if offset > len(response.ExternalUserIDs) {
		offset = len(response.ExternalUserIDs)
		end = offset
	}
	refs := make([]map[string]interface{}, 0, end-offset)
	for _, externalUserID := range response.ExternalUserIDs[offset:end] {
		ref, encodeErr := adapter.encodeSubjectRef(creds, SubjectTypeCustomer, SubjectIDKindExternalUserID, externalUserID, "", "")
		if encodeErr != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an invalid customer", encodeErr)
		}
		refs = append(refs, map[string]interface{}{"customer_ref": ref})
	}
	output := customerCoverageOutput()
	output["customer_refs"] = refs
	isLast := end >= len(response.ExternalUserIDs)
	output["truncated"] = !isLast
	output["is_last"] = isLast
	output["next_cursor"] = ""
	if !isLast {
		nextCursor, cursorErr := adapter.encodeCustomerPageCursor(creds, customerCursorOwnerClients, queryBind, end)
		if cursorErr != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom customer list cursor could not be created", cursorErr)
		}
		output["next_cursor"] = nextCursor
	}
	return output, len(refs), firstNonEmpty(response.RequestID, response.RequestIDAlt), nil
}

func (adapter *Adapter) getCustomer(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, string, error) {
	claims, err := adapter.decodeSubjectRef(inputString(input, "customer_ref"), creds, SubjectTypeCustomer, SubjectIDKindExternalUserID)
	if err != nil {
		return nil, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom customer reference is invalid", err)
	}
	query := url.Values{"external_userid": []string{claims.SubjectID}}
	if cursor := strings.TrimSpace(inputString(input, "cursor")); cursor != "" {
		query.Set("cursor", cursor)
	}
	var response externalContactDetail
	if err := adapter.authorizedJSON(ctx, creds, http.MethodGet, "/cgi-bin/externalcontact/get", query, nil, &response); err != nil {
		return nil, "", err
	}
	customer, err := adapter.projectExternalContact(creds, response.ExternalContact, response.FollowUsers)
	if err != nil {
		return nil, "", err
	}
	output := customerCoverageOutput()
	output["customer"] = customer
	output["next_cursor"] = bounded(response.NextCursor, 2048)
	output["is_last"] = strings.TrimSpace(response.NextCursor) == ""
	return output, firstNonEmpty(response.RequestID, response.RequestIDAlt), nil
}

func (adapter *Adapter) batchGetCustomersByOwner(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, string, error) {
	ownerRefs := inputStringSlice(input, "owner_refs", 100)
	if len(ownerRefs) == 0 {
		return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom customer owner references are required", nil)
	}
	ownerIDs := make([]string, 0, len(ownerRefs))
	seenOwnerIDs := make(map[string]struct{}, len(ownerRefs))
	for _, ownerRef := range ownerRefs {
		claims, err := adapter.decodeSubjectRef(ownerRef, creds, SubjectTypeMember, SubjectIDKindUserID)
		if err != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom customer owner reference is invalid", err)
		}
		if _, duplicate := seenOwnerIDs[claims.SubjectID]; duplicate {
			continue
		}
		seenOwnerIDs[claims.SubjectID] = struct{}{}
		ownerIDs = append(ownerIDs, claims.SubjectID)
	}
	limit := inputInt(input, "limit", 50)
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	payload := map[string]interface{}{"userid_list": ownerIDs, "limit": limit}
	if cursor := strings.TrimSpace(inputString(input, "cursor")); cursor != "" {
		payload["cursor"] = cursor
	}
	var response struct {
		apiEnvelope
		ExternalContacts []batchExternalContactItem `json:"external_contact_list"`
		NextCursor       string                     `json:"next_cursor"`
	}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodPost, "/cgi-bin/externalcontact/batch/get_by_user", nil, payload, &response); err != nil {
		return nil, 0, "", err
	}
	customers := make([]map[string]interface{}, 0, len(response.ExternalContacts))
	for _, item := range response.ExternalContacts {
		projected, projectErr := adapter.projectExternalContact(creds, item.ExternalContact, []externalContactFollowInfo{item.FollowInfo})
		if projectErr != nil {
			return nil, 0, "", projectErr
		}
		customers = append(customers, projected)
	}
	output := customerCoverageOutput()
	output["customers"] = customers
	output["next_cursor"] = bounded(response.NextCursor, 2048)
	output["is_last"] = strings.TrimSpace(response.NextCursor) == ""
	return output, len(customers), firstNonEmpty(response.RequestID, response.RequestIDAlt), nil
}

func (adapter *Adapter) projectExternalContact(creds credentials, contact externalContactSummary, followUsers []externalContactFollowInfo) (map[string]interface{}, error) {
	ref, err := adapter.encodeSubjectRef(creds, SubjectTypeCustomer, SubjectIDKindExternalUserID, contact.ExternalUserID, "", "")
	if err != nil {
		return nil, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an invalid customer", err)
	}
	projectedFollowUsers := make([]map[string]interface{}, 0, len(followUsers))
	for _, follow := range followUsers {
		ownerRef, encodeErr := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindUserID, follow.UserID, "", "")
		if encodeErr != nil {
			return nil, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an invalid customer owner", encodeErr)
		}
		projectedFollowUsers = append(projectedFollowUsers, map[string]interface{}{
			"owner_ref": ownerRef, "remark": bounded(follow.Remark, 255),
			"created_at": follow.CreateTime, "add_way": follow.AddWay,
		})
	}
	return map[string]interface{}{
		"customer_ref": ref, "name": bounded(contact.Name, 255), "type": contact.Type,
		"corp_name": bounded(contact.CorpName, 255), "follow_users": projectedFollowUsers,
	}, nil
}

func (adapter *Adapter) listCustomerGroups(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, string, error) {
	ownerRefs := inputStringSlice(input, "owner_refs", 100)
	if len(ownerRefs) == 0 {
		return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom customer group owner references are required", nil)
	}
	ownerIDs := make([]string, 0, len(ownerRefs))
	seenOwnerIDs := make(map[string]struct{}, len(ownerRefs))
	for _, ownerRef := range ownerRefs {
		claims, err := adapter.decodeSubjectRef(ownerRef, creds, SubjectTypeMember, SubjectIDKindUserID)
		if err != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom customer group owner reference is invalid", err)
		}
		if _, duplicate := seenOwnerIDs[claims.SubjectID]; duplicate {
			continue
		}
		seenOwnerIDs[claims.SubjectID] = struct{}{}
		ownerIDs = append(ownerIDs, claims.SubjectID)
	}
	limit := inputInt(input, "limit", 100)
	if limit < 1 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	payload := map[string]interface{}{
		"status_filter": inputInt(input, "status_filter", 0),
		"owner_filter":  map[string]interface{}{"userid_list": ownerIDs},
		"limit":         limit,
	}
	if cursor := strings.TrimSpace(inputString(input, "cursor")); cursor != "" {
		payload["cursor"] = cursor
	}
	var response struct {
		apiEnvelope
		Groups []struct {
			ChatID string `json:"chat_id"`
			Status int    `json:"status"`
		} `json:"group_chat_list"`
		NextCursor string `json:"next_cursor"`
	}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodPost, "/cgi-bin/externalcontact/groupchat/list", nil, payload, &response); err != nil {
		return nil, 0, "", err
	}
	groups := make([]map[string]interface{}, 0, len(response.Groups))
	for _, group := range response.Groups {
		ref, err := adapter.encodeSubjectRef(creds, SubjectTypeCustomerGroup, SubjectIDKindChatID, group.ChatID, "", "")
		if err != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an invalid customer group", err)
		}
		groups = append(groups, map[string]interface{}{"group_ref": ref, "status": group.Status})
	}
	output := customerCoverageOutput()
	output["groups"] = groups
	output["next_cursor"] = bounded(response.NextCursor, 2048)
	output["is_last"] = strings.TrimSpace(response.NextCursor) == ""
	return output, len(groups), firstNonEmpty(response.RequestID, response.RequestIDAlt), nil
}

func (adapter *Adapter) getCustomerGroup(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, string, error) {
	claims, err := adapter.decodeSubjectRef(inputString(input, "group_ref"), creds, SubjectTypeCustomerGroup, SubjectIDKindChatID)
	if err != nil {
		return nil, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom customer group reference is invalid", err)
	}
	var response struct {
		apiEnvelope
		Group struct {
			ChatID     string `json:"chat_id"`
			Name       string `json:"name"`
			Owner      string `json:"owner"`
			CreateTime int64  `json:"create_time"`
			Members    []struct {
				UserID string `json:"userid"`
				Type   int    `json:"type"`
			} `json:"member_list"`
		} `json:"group_chat"`
	}
	payload := map[string]interface{}{"chat_id": claims.SubjectID, "need_name": 0}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodPost, "/cgi-bin/externalcontact/groupchat/get", nil, payload, &response); err != nil {
		return nil, "", err
	}
	groupRef, err := adapter.encodeSubjectRef(creds, SubjectTypeCustomerGroup, SubjectIDKindChatID, response.Group.ChatID, "", "")
	if err != nil {
		return nil, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an invalid customer group", err)
	}
	ownerRef, err := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindUserID, response.Group.Owner, "", "")
	if err != nil {
		return nil, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an invalid customer group owner", err)
	}
	members := make([]map[string]interface{}, 0, len(response.Group.Members))
	for _, member := range response.Group.Members {
		subjectType, idKind, memberType := SubjectTypeMember, SubjectIDKindUserID, "internal_member"
		if member.Type == 2 {
			subjectType, idKind, memberType = SubjectTypeCustomer, SubjectIDKindExternalUserID, "customer"
		} else if member.Type != 1 {
			continue
		}
		memberRef, encodeErr := adapter.encodeSubjectRef(creds, subjectType, idKind, member.UserID, "", "")
		if encodeErr != nil {
			return nil, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an invalid customer group member", encodeErr)
		}
		members = append(members, map[string]interface{}{"subject_ref": memberRef, "member_type": memberType})
	}
	output := customerCoverageOutput()
	output["group"] = map[string]interface{}{
		"group_ref": groupRef, "name": bounded(response.Group.Name, 255), "owner_ref": ownerRef,
		"created_at": response.Group.CreateTime, "member_count": len(members), "members": members,
	}
	return output, firstNonEmpty(response.RequestID, response.RequestIDAlt), nil
}

func (adapter *Adapter) createCustomerMassMessage(ctx context.Context, creds credentials, input map[string]interface{}, group bool) (map[string]interface{}, int, string, error) {
	referenceKey, subjectType, idKind, providerField, maxItems := "customer_refs", SubjectTypeCustomer, SubjectIDKindExternalUserID, "external_userid", 10000
	if group {
		referenceKey, subjectType, idKind, providerField, maxItems = "group_refs", SubjectTypeCustomerGroup, SubjectIDKindChatID, "chat_id_list", maxCustomerGroupMassMessageTargets
	}
	if group && inputStringArrayLength(input[referenceKey]) > maxItems {
		return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom customer-group mass-message target count exceeds the safe validation budget", nil)
	}
	refs := inputStringSlice(input, referenceKey, maxItems)
	if len(refs) == 0 {
		return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom mass-message recipients are required", nil)
	}
	providerIDs := make([]string, 0, len(refs))
	refByProviderID := make(map[string]string, len(refs))
	for _, ref := range refs {
		claims, err := adapter.decodeSubjectRef(ref, creds, subjectType, idKind)
		if err != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom mass-message recipient reference is invalid", err)
		}
		if _, duplicate := refByProviderID[claims.SubjectID]; duplicate {
			continue
		}
		providerIDs = append(providerIDs, claims.SubjectID)
		refByProviderID[claims.SubjectID] = ref
	}
	content := strings.TrimSpace(inputString(input, "content"))
	if content == "" || len([]byte(content)) > 4000 {
		return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom mass-message text must be between 1 and 4000 UTF-8 bytes", nil)
	}
	payload := map[string]interface{}{
		"chat_type": "single", providerField: providerIDs, "allow_select": false,
		"text": map[string]string{"content": content},
	}
	if group {
		payload["chat_type"] = "group"
		sender, err := adapter.decodeSubjectRef(inputString(input, "sender_ref"), creds, SubjectTypeMember, SubjectIDKindUserID)
		if err != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom mass-message sender reference is invalid", err)
		}
		for _, groupID := range providerIDs {
			ownerID, _, ownerErr := adapter.getCustomerGroupOwner(ctx, creds, groupID)
			if ownerErr != nil {
				return nil, 0, "", ownerErr
			}
			if !strings.EqualFold(ownerID, sender.SubjectID) {
				return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom customer group is not owned by the selected sender", nil)
			}
		}
		payload["sender"] = sender.SubjectID
	} else if senderRef := strings.TrimSpace(inputString(input, "sender_ref")); senderRef != "" {
		sender, err := adapter.decodeSubjectRef(senderRef, creds, SubjectTypeMember, SubjectIDKindUserID)
		if err != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom mass-message sender reference is invalid", err)
		}
		payload["sender"] = sender.SubjectID
	}
	var response struct {
		apiEnvelope
		MsgID    string   `json:"msgid"`
		FailList []string `json:"fail_list"`
	}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodPost, "/cgi-bin/externalcontact/add_msg_template", nil, payload, &response); err != nil {
		return nil, 0, "", err
	}
	if strings.TrimSpace(response.MsgID) == "" {
		return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom mass-message task response did not contain a task ID", nil)
	}
	rejectedRefs, err := validateMassMessageFailList(response.FailList, refByProviderID)
	if err != nil {
		return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom mass-message task returned an invalid failure list", err)
	}
	rejectedCount := len(rejectedRefs)
	acceptedCount := len(providerIDs) - rejectedCount
	if acceptedCount == 0 {
		return nil, 0, "", integrations.NewErrorWithReason(
			integrations.ErrorCodeProviderRejected,
			"wecom_all_mass_message_targets_rejected",
			"WeCom rejected all intended mass-message targets",
			nil,
		)
	}
	operationStatus := "succeeded"
	if rejectedCount > 0 {
		operationStatus = "partially_succeeded"
	}
	taskRef, err := adapter.encodeCustomerTaskRef(creds, response.MsgID)
	if err != nil {
		return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom mass-message task reference could not be created", err)
	}
	output := map[string]interface{}{
		"provider":         IntegrationID,
		"operation_status": operationStatus,
		"task": map[string]interface{}{
			"task_ref": taskRef, "task_status": "created_pending_member_send",
			"delivery_status": "pending_member_send", "provider_accepted": true,
			"accepted_count": acceptedCount, "rejected_count": rejectedCount, "rejected_refs": rejectedRefs,
		},
	}
	return output, acceptedCount, firstNonEmpty(response.RequestID, response.RequestIDAlt), nil
}

func validateMassMessageFailList(failList []string, refByProviderID map[string]string) ([]string, error) {
	rejected := make([]string, 0, len(failList))
	seen := make(map[string]struct{}, len(failList))
	for _, value := range failList {
		providerID := strings.TrimSpace(value)
		ref, exists := refByProviderID[providerID]
		if !exists || providerID == "" {
			return nil, fmt.Errorf("unknown failed target")
		}
		if _, duplicate := seen[providerID]; duplicate {
			return nil, fmt.Errorf("duplicate failed target")
		}
		seen[providerID] = struct{}{}
		rejected = append(rejected, ref)
	}
	return rejected, nil
}

func (adapter *Adapter) getCustomerGroupOwner(ctx context.Context, creds credentials, chatID string) (string, string, error) {
	var response struct {
		apiEnvelope
		Group struct {
			ChatID string `json:"chat_id"`
			Owner  string `json:"owner"`
		} `json:"group_chat"`
	}
	payload := map[string]interface{}{"chat_id": chatID, "need_name": 0}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodPost, "/cgi-bin/externalcontact/groupchat/get", nil, payload, &response); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(response.Group.ChatID) != strings.TrimSpace(chatID) || !validProviderSubjectID(response.Group.Owner) {
		return "", "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an invalid customer group owner", nil)
	}
	return strings.TrimSpace(response.Group.Owner), firstNonEmpty(response.RequestID, response.RequestIDAlt), nil
}

func (adapter *Adapter) listCustomerMassMessageTasks(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, string, error) {
	taskID, err := adapter.decodeCustomerTaskRef(inputString(input, "task_ref"), creds)
	if err != nil {
		return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom mass-message task reference is invalid", err)
	}
	limit := inputInt(input, "limit", 500)
	if limit < 1 {
		limit = 500
	}
	if limit > 1000 {
		limit = 1000
	}
	payload := map[string]interface{}{"msgid": taskID, "limit": limit}
	if cursor := strings.TrimSpace(inputString(input, "cursor")); cursor != "" {
		payload["cursor"] = cursor
	}
	var response struct {
		apiEnvelope
		NextCursor string `json:"next_cursor"`
		Tasks      []struct {
			UserID   string `json:"userid"`
			Status   int    `json:"status"`
			SendTime *int64 `json:"send_time"`
		} `json:"task_list"`
	}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodPost, "/cgi-bin/externalcontact/get_groupmsg_task", nil, payload, &response); err != nil {
		return nil, 0, "", err
	}
	tasks := make([]map[string]interface{}, 0, len(response.Tasks))
	for _, item := range response.Tasks {
		senderRef, encodeErr := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindUserID, item.UserID, "", "")
		if encodeErr != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an invalid mass-message sender", encodeErr)
		}
		status := customerMassMessageTaskStatus(item.Status)
		projected := map[string]interface{}{"sender_ref": senderRef, "status": status, "raw_status": item.Status}
		if err := projectCustomerMassMessageSentAt(projected, status, item.SendTime); err != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an invalid mass-message task send time", err)
		}
		tasks = append(tasks, projected)
	}
	nextCursor := bounded(response.NextCursor, 2048)
	output := map[string]interface{}{
		"provider": IntegrationID, "tasks": tasks,
		"next_cursor": nextCursor, "is_last": strings.TrimSpace(nextCursor) == "",
	}
	return output, len(tasks), firstNonEmpty(response.RequestID, response.RequestIDAlt), nil
}

func (adapter *Adapter) listCustomerMassMessageResults(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, string, error) {
	taskID, err := adapter.decodeCustomerTaskRef(inputString(input, "task_ref"), creds)
	if err != nil {
		return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom mass-message task reference is invalid", err)
	}
	sender, err := adapter.decodeSubjectRef(inputString(input, "sender_ref"), creds, SubjectTypeMember, SubjectIDKindUserID)
	if err != nil {
		return nil, 0, "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom mass-message sender reference is invalid", err)
	}
	limit := inputInt(input, "limit", 500)
	if limit < 1 {
		limit = 500
	}
	if limit > 1000 {
		limit = 1000
	}
	payload := map[string]interface{}{"msgid": taskID, "userid": sender.SubjectID, "limit": limit}
	if cursor := strings.TrimSpace(inputString(input, "cursor")); cursor != "" {
		payload["cursor"] = cursor
	}
	var response struct {
		apiEnvelope
		NextCursor string `json:"next_cursor"`
		Results    []struct {
			UserID         string `json:"userid"`
			ExternalUserID string `json:"external_userid"`
			ChatID         string `json:"chat_id"`
			Status         int    `json:"status"`
			SendTime       *int64 `json:"send_time"`
		} `json:"send_list"`
	}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodPost, "/cgi-bin/externalcontact/get_groupmsg_send_result", nil, payload, &response); err != nil {
		return nil, 0, "", err
	}
	results := make([]map[string]interface{}, 0, len(response.Results))
	for _, item := range response.Results {
		if !strings.EqualFold(strings.TrimSpace(item.UserID), sender.SubjectID) {
			return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned a send result for an unexpected member", nil)
		}
		externalUserID := strings.TrimSpace(item.ExternalUserID)
		chatID := strings.TrimSpace(item.ChatID)
		if (externalUserID == "") == (chatID == "") {
			return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an ambiguous mass-message target", nil)
		}
		subjectType, idKind, providerID, targetType := SubjectTypeCustomer, SubjectIDKindExternalUserID, externalUserID, "customer"
		if chatID != "" {
			subjectType, idKind, providerID, targetType = SubjectTypeCustomerGroup, SubjectIDKindChatID, chatID, "customer_group"
		}
		targetRef, encodeErr := adapter.encodeSubjectRef(creds, subjectType, idKind, providerID, "", "")
		if encodeErr != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an invalid mass-message target", encodeErr)
		}
		status := customerMassMessageResultStatus(item.Status)
		projected := map[string]interface{}{
			"target_ref": targetRef, "target_type": targetType, "status": status, "raw_status": item.Status,
		}
		if err := projectCustomerMassMessageSentAt(projected, status, item.SendTime); err != nil {
			return nil, 0, "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an invalid mass-message result send time", err)
		}
		results = append(results, projected)
	}
	nextCursor := bounded(response.NextCursor, 2048)
	output := map[string]interface{}{
		"provider": IntegrationID, "results": results,
		"next_cursor": nextCursor, "is_last": strings.TrimSpace(nextCursor) == "",
	}
	return output, len(results), firstNonEmpty(response.RequestID, response.RequestIDAlt), nil
}

func inputStringSlice(input map[string]interface{}, key string, maxItems int) []string {
	values, _ := input[key].([]interface{})
	if len(values) == 0 {
		if typed, ok := input[key].([]string); ok {
			values = make([]interface{}, len(typed))
			for index := range typed {
				values[index] = typed[index]
			}
		}
	}
	out := make([]string, 0, min(len(values), maxItems))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		text, _ := value.(string)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if _, exists := seen[text]; exists {
			continue
		}
		seen[text] = struct{}{}
		out = append(out, text)
		if len(out) >= maxItems {
			break
		}
	}
	return out
}

func inputStringArrayLength(value interface{}) int {
	switch typed := value.(type) {
	case []interface{}:
		return len(typed)
	case []string:
		return len(typed)
	default:
		return 0
	}
}
