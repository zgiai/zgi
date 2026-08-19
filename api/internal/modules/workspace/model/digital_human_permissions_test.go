package model

import "testing"

func TestDigitalHumanPermissionsAreIndependentAndAssignedByBuiltinRole(t *testing.T) {
	codes := []WorkspacePermissionCode{
		WorkspacePermissionDigitalHumanView,
		WorkspacePermissionDigitalHumanCreate,
		WorkspacePermissionDigitalHumanUpdate,
		WorkspacePermissionDigitalHumanDelete,
		WorkspacePermissionDigitalHumanPublish,
		WorkspacePermissionDigitalHumanRun,
		WorkspacePermissionDigitalHumanLogsView,
		WorkspacePermissionDigitalHumanMemoryView,
		WorkspacePermissionDigitalHumanMemoryManage,
	}
	for _, code := range codes {
		if !IsKnownWorkspacePermissionCode(code) {
			t.Errorf("permission %s is not registered", code)
		}
	}
	admin := GetBuiltinGroupRolePermissionsByID(WorkspaceBuiltinRoleAdminID)
	for _, code := range codes {
		if !WorkspacePermissionCodesAllow(admin, code) {
			t.Errorf("admin missing %s", code)
		}
	}
	member := GetBuiltinGroupRolePermissionsByID(WorkspaceBuiltinRoleMemberID)
	for _, code := range []WorkspacePermissionCode{
		WorkspacePermissionDigitalHumanView, WorkspacePermissionDigitalHumanRun, WorkspacePermissionDigitalHumanLogsView,
		WorkspacePermissionDigitalHumanMemoryView,
	} {
		if !WorkspacePermissionCodesAllow(member, code) {
			t.Errorf("member missing %s", code)
		}
	}
	for _, code := range []WorkspacePermissionCode{
		WorkspacePermissionDigitalHumanCreate, WorkspacePermissionDigitalHumanUpdate,
		WorkspacePermissionDigitalHumanDelete, WorkspacePermissionDigitalHumanPublish,
		WorkspacePermissionDigitalHumanMemoryManage,
	} {
		if WorkspacePermissionCodesAllow(member, code) {
			t.Errorf("member unexpectedly has %s", code)
		}
	}
	viewer := GetBuiltinGroupRolePermissionsByID(WorkspaceBuiltinRoleViewerID)
	if !WorkspacePermissionCodesAllow(viewer, WorkspacePermissionDigitalHumanView) ||
		!WorkspacePermissionCodesAllow(viewer, WorkspacePermissionDigitalHumanLogsView) ||
		!WorkspacePermissionCodesAllow(viewer, WorkspacePermissionDigitalHumanMemoryView) ||
		WorkspacePermissionCodesAllow(viewer, WorkspacePermissionDigitalHumanRun) {
		t.Fatalf("viewer DigitalHuman permissions = %v", viewer)
	}
	if WorkspacePermissionCodesAllow([]WorkspacePermissionCode{WorkspacePermissionAgentManage}, WorkspacePermissionDigitalHumanCreate) {
		t.Fatal("legacy agent.manage must not grant DigitalHuman permissions")
	}

	definitions := make(map[string][]string)
	for _, definition := range DefaultWorkspaceRoleTemplateDefinitions() {
		definitions[definition.SystemKey] = definition.Permissions
	}
	for _, code := range codes {
		if !WorkspacePermissionStringsAllow(definitions[WorkspaceDefaultRoleTemplateAdvancedKey], code) {
			t.Errorf("advanced default role missing %s", code)
		}
	}
	for _, code := range []WorkspacePermissionCode{
		WorkspacePermissionDigitalHumanView,
		WorkspacePermissionDigitalHumanCreate,
		WorkspacePermissionDigitalHumanUpdate,
		WorkspacePermissionDigitalHumanRun,
		WorkspacePermissionDigitalHumanLogsView,
		WorkspacePermissionDigitalHumanMemoryView,
		WorkspacePermissionDigitalHumanMemoryManage,
	} {
		if !WorkspacePermissionStringsAllow(definitions[WorkspaceDefaultRoleTemplateBasicKey], code) {
			t.Errorf("basic default role missing %s", code)
		}
	}
	for _, code := range []WorkspacePermissionCode{
		WorkspacePermissionDigitalHumanDelete,
		WorkspacePermissionDigitalHumanPublish,
	} {
		if WorkspacePermissionStringsAllow(definitions[WorkspaceDefaultRoleTemplateBasicKey], code) {
			t.Errorf("basic default role unexpectedly has %s", code)
		}
	}
	if !WorkspacePermissionStringsAllow(definitions[WorkspaceDefaultRoleTemplateReadonlyKey], WorkspacePermissionDigitalHumanView) ||
		!WorkspacePermissionStringsAllow(definitions[WorkspaceDefaultRoleTemplateReadonlyKey], WorkspacePermissionDigitalHumanLogsView) ||
		!WorkspacePermissionStringsAllow(definitions[WorkspaceDefaultRoleTemplateReadonlyKey], WorkspacePermissionDigitalHumanMemoryView) ||
		WorkspacePermissionStringsAllow(definitions[WorkspaceDefaultRoleTemplateReadonlyKey], WorkspacePermissionDigitalHumanRun) {
		t.Fatalf("readonly default role DigitalHuman permissions = %v", definitions[WorkspaceDefaultRoleTemplateReadonlyKey])
	}
}
