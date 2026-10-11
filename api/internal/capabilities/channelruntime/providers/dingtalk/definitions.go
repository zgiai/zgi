package dingtalk

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

func text(max int) map[string]interface{} {
	return map[string]interface{}{"type": "string", "maxLength": max}
}
