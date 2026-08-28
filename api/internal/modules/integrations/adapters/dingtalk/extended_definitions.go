package dingtalk

import (
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

func extendedActions() []integrations.ActionDefinition {
	departmentSchema := object(map[string]interface{}{
		"department_ref": nonblank(2048),
		"name":           text(255),
		"parent_id":      map[string]interface{}{"type": "integer"},
	}, []string{"department_ref", "name", "parent_id"})
	memberSchema := object(map[string]interface{}{
		"recipient_ref": nonblank(2048),
		"name":          text(255),
		"title":         text(255),
		"active":        map[string]interface{}{"type": "boolean"},
	}, []string{"recipient_ref", "name", "title"})
	roleSchema := object(map[string]interface{}{
		"role_ref":   nonblank(2048),
		"name":       text(255),
		"group_name": text(255),
	}, []string{"role_ref", "name", "group_name"})

	attendance := read(
		ActionAttendanceList,
		"list_dingtalk_attendance_records",
		"List DingTalk attendance records",
		"查询钉钉考勤记录",
		"Read sanitized clock-in results for one previously resolved member within a period of at most seven days. Exact coordinates, addresses, photos, and remarks are never returned.",
		"读取一位已确认成员在最长七天内的脱敏打卡结果；不会返回精确经纬度、地址、照片或备注。",
		object(map[string]interface{}{
			"recipient_ref": titled(nonblank(2048), "Member reference", "成员引用"),
			"start_time":    titled(map[string]interface{}{"type": "string", "format": "date-time"}, "Start time", "开始时间"),
			"end_time":      titled(map[string]interface{}{"type": "string", "format": "date-time"}, "End time", "结束时间"),
		}, []string{"recipient_ref", "start_time", "end_time"}),
		object(map[string]interface{}{
			"provider":      nonblank(64),
			"recipient_ref": nonblank(2048),
			"records": array(object(map[string]interface{}{
				"work_time":       map[string]interface{}{"type": "string", "format": "date-time"},
				"planned_time":    map[string]interface{}{"type": "string", "format": "date-time"},
				"actual_time":     map[string]interface{}{"type": "string", "format": "date-time"},
				"check_type":      text(32),
				"time_result":     text(64),
				"location_result": text(64),
				"source_type":     text(64),
			}, []string{"work_time", "planned_time", "actual_time", "check_type", "time_result", "location_result", "source_type"}), 100),
		}, []string{"provider", "recipient_ref", "records"}),
		[]string{ScopeAttendance},
	)
	attendance.RiskLevel = toolgovernance.RiskLevelMedium

	sendDepartment := write(
		ActionMessageSendDept,
		"send_dingtalk_department_notification",
		"Send DingTalk department notification",
		"发送钉钉部门通知",
		"Submit one plain-text work notification to the members of a previously resolved department after explicit approval. A successful response means accepted for processing, not confirmed delivery.",
		"经明确确认后，向已解析部门的成员提交一条纯文本工作通知。成功响应仅表示钉钉已受理，不代表全部成员已送达。",
		object(map[string]interface{}{
			"department_ref": titled(nonblank(2048), "Department reference", "部门引用"),
			"content":        titled(nonblank(2048), "Notification content", "通知内容"),
		}, []string{"department_ref", "content"}),
		object(map[string]interface{}{
			"provider": nonblank(64),
			"notification": object(map[string]interface{}{
				"message_ref":       nonblank(2048),
				"department_ref":    nonblank(2048),
				"provider_accepted": map[string]interface{}{"const": true},
				"delivery_status":   map[string]interface{}{"const": "pending"},
			}, []string{"message_ref", "department_ref", "provider_accepted", "delivery_status"}),
		}, []string{"provider", "notification"}),
		[]string{ScopeSend},
	)
	sendDepartment.PreparationHints = []integrations.ActionPreparationHint{{
		ActionID: ActionDepartmentSearch, Relation: integrations.ActionPreparationResolveTarget,
		TargetArguments: []string{"department_ref"}, ResultPaths: []string{"departments[].department_ref"},
		Description:     "Resolve and disambiguate the intended DingTalk department before sending.",
		DescriptionI18n: loc("Resolve and disambiguate the intended DingTalk department before sending.", "发送前先搜索并确认目标钉钉部门；存在同名结果时必须消歧。"),
	}}
	sendDepartment.SuccessDeduplication = &integrations.SuccessDeduplicationDefinition{TargetArgumentPaths: []string{"department_ref"}}

	return []integrations.ActionDefinition{
		read(ActionDepartmentSearch, "search_dingtalk_departments", "Search DingTalk departments", "搜索钉钉部门", "Search visible departments by name and return connection-bound department references.", "按名称搜索应用可见的部门，并返回与当前连接绑定的部门引用。", object(map[string]interface{}{
			"query":       titled(nonblank(128), "Department name", "部门名称"),
			"max_results": titled(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 20, "default": 10}, "Maximum results", "最大结果数"),
		}, []string{"query"}), object(map[string]interface{}{"provider": nonblank(64), "departments": array(departmentSchema, 20), "has_more": map[string]interface{}{"type": "boolean"}}, []string{"provider", "departments", "has_more"}), []string{ScopeContacts}),
		read(ActionDepartmentGet, "get_dingtalk_department", "Get DingTalk department", "获取钉钉部门", "Read one department selected from a department search or list result.", "读取部门搜索或部门列表中选定的一个部门。", object(map[string]interface{}{
			"department_ref": titled(nonblank(2048), "Department reference", "部门引用"),
		}, []string{"department_ref"}), object(map[string]interface{}{"provider": nonblank(64), "department": departmentSchema}, []string{"provider", "department"}), []string{ScopeContacts}),
		read(ActionDepartmentUsers, "list_dingtalk_department_members", "List DingTalk department members", "列出钉钉部门成员", "List visible members in one selected department and return connection-bound member references.", "列出一个已选部门中的可见成员，并返回与当前连接绑定的成员引用。", object(map[string]interface{}{
			"department_ref": titled(nonblank(2048), "Department reference", "部门引用"),
			"max_results":    titled(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 100, "default": 50}, "Maximum results", "最大结果数"),
		}, []string{"department_ref"}), object(map[string]interface{}{"provider": nonblank(64), "members": array(memberSchema, 100), "has_more": map[string]interface{}{"type": "boolean"}}, []string{"provider", "members", "has_more"}), []string{ScopeContacts}),
		read(ActionRoleList, "list_dingtalk_roles", "List DingTalk roles", "列出钉钉角色", "List organization roles visible to the internal application.", "列出企业内部应用可见的组织角色。", object(map[string]interface{}{
			"max_results": titled(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 100, "default": 50}, "Maximum roles", "最大角色数"),
		}, nil), object(map[string]interface{}{"provider": nonblank(64), "roles": array(roleSchema, 100), "has_more": map[string]interface{}{"type": "boolean"}}, []string{"provider", "roles", "has_more"}), []string{ScopeContacts}),
		read(ActionRoleUsers, "list_dingtalk_role_members", "List DingTalk role members", "列出钉钉角色成员", "List members assigned to one selected organization role.", "列出分配给一个已选组织角色的成员。", object(map[string]interface{}{
			"role_ref":    titled(nonblank(2048), "Role reference", "角色引用"),
			"max_results": titled(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 100, "default": 50}, "Maximum results", "最大结果数"),
		}, []string{"role_ref"}), object(map[string]interface{}{"provider": nonblank(64), "members": array(memberSchema, 100), "has_more": map[string]interface{}{"type": "boolean"}}, []string{"provider", "members", "has_more"}), []string{ScopeContacts}),
		attendance,
		sendDepartment,
	}
}
