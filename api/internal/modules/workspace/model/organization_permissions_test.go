package model

import (
	"strings"
	"testing"
)

func TestExpandWorkspacePermissionCodesForCompatibility(t *testing.T) {
	expanded := ExpandWorkspacePermissionCodesForCompatibility([]WorkspacePermissionCode{
		WorkspacePermissionAgentManage,
	})

	expected := []WorkspacePermissionCode{
		WorkspacePermissionAgentManage,
		WorkspacePermissionAgentCreate,
		WorkspacePermissionWorkflowPublish,
	}
	for _, code := range expected {
		if !WorkspacePermissionCodesAllow(expanded, code) {
			t.Errorf("expanded permissions missing %s in %v", code, expanded)
		}
	}
}

func TestWorkspacePermissionCodesAllowDoesNotPromoteFineGrantToLegacyGrant(t *testing.T) {
	if WorkspacePermissionCodesAllow(
		[]WorkspacePermissionCode{WorkspacePermissionAgentCreate},
		WorkspacePermissionAgentManage,
	) {
		t.Fatal("fine-grained agent.create grant should not imply legacy agent.manage")
	}
}

func TestBuiltinMemberPermissionsIncludeCompatibleFineCodes(t *testing.T) {
	permissions := GetBuiltinGroupRolePermissionsByID(WorkspaceBuiltinRoleMemberID)

	expectedAllowed := []WorkspacePermissionCode{
		WorkspacePermissionWorkflowView,
		WorkspacePermissionDatabaseAIQueryRead,
		WorkspacePermissionFileUpload,
	}
	for _, code := range expectedAllowed {
		if !WorkspacePermissionCodesAllow(permissions, code) {
			t.Errorf("member role should allow %s", code)
		}
	}

	expectedDenied := []WorkspacePermissionCode{
		WorkspacePermissionWorkspaceMemberView,
		WorkspacePermissionWorkspaceMemberManage,
		WorkspacePermissionAgentCreate,
		WorkspacePermissionDatabaseRecordDelete,
	}
	for _, code := range expectedDenied {
		if WorkspacePermissionCodesAllow(permissions, code) {
			t.Errorf("member role should not allow %s", code)
		}
	}
}

func TestDefaultWorkspaceMemberPermissionStringsDoNotExposeCompatibilityPermissions(t *testing.T) {
	for _, role := range []WorkspaceMemberRole{WorkspaceRoleAdmin, WorkspaceRoleNormal} {
		permissions := DefaultWorkspaceMemberPermissionStrings(role, nil)

		if !containsWorkspacePermissionString(permissions, string(WorkspacePermissionDatabaseAIQueryRead)) {
			t.Fatalf("default permissions for role %s should include fine-grained ai query read: %#v", role, permissions)
		}
		if !containsWorkspacePermissionString(permissions, string(WorkspacePermissionFileUpload)) {
			t.Fatalf("default permissions for role %s should include fine-grained file upload: %#v", role, permissions)
		}
		assertNoCompatibilityWorkspacePermissions(t, permissions)
	}
}

func TestEffectiveWorkspaceMemberPermissionStringsDoesNotSynthesizeToolPermissions(t *testing.T) {
	permissions := EffectiveWorkspaceMemberPermissionStrings(
		WorkspaceRoleAdmin,
		nil,
		nil,
		WorkspaceMemberPermissionSourceDirect,
	)

	if WorkspacePermissionStringsAllow(permissions, WorkspacePermissionAgentCreate) {
		t.Fatalf("direct empty permissions should not grant configurable asset permissions: %#v", permissions)
	}
	assertNoRetiredWorkspacePermissions(t, permissions)
}

func TestEffectiveWorkspaceMemberPermissionStringsFallsBackForLegacyRole(t *testing.T) {
	permissions := EffectiveWorkspaceMemberPermissionStrings(
		WorkspaceRoleAdmin,
		nil,
		nil,
		"",
	)

	if WorkspacePermissionStringsAllow(permissions, WorkspacePermissionWorkspacePermissionManage) {
		t.Fatalf("effective permissions should not synthesize governance permissions: %#v", permissions)
	}
	if !WorkspaceMemberAllowsPermission(WorkspaceRoleAdmin, nil, nil, "", WorkspacePermissionWorkspacePermissionManage) {
		t.Fatal("admin role should still allow internal workspace governance checks")
	}
	assertNoRetiredWorkspacePermissions(t, permissions)
}

func TestEffectiveWorkspaceMemberPermissionStringsExpandsLegacyCoarseGrant(t *testing.T) {
	permissions := EffectiveWorkspaceMemberPermissionStrings(
		WorkspaceRoleNormal,
		nil,
		[]string{string(WorkspacePermissionKnowledgeBaseManage)},
		WorkspaceMemberPermissionSourceDirect,
	)

	if !WorkspacePermissionStringsAllow(permissions, WorkspacePermissionKnowledgeBaseDocumentCreate) {
		t.Fatalf("knowledge_base.manage should expand to document create permission: %#v", permissions)
	}
	if WorkspacePermissionStringsAllow(permissions, WorkspacePermissionWorkspaceManage) {
		t.Fatalf("fine-grained expansion should not grant unrelated workspace.manage: %#v", permissions)
	}
}

func TestCanonicalWorkspacePermissionSnapshotStringsReplacesDeprecatedAssetCoarseCodes(t *testing.T) {
	permissions := CanonicalWorkspacePermissionSnapshotStrings([]string{
		string(WorkspacePermissionAgentManage),
		string(WorkspacePermissionKnowledgeBaseView),
		string(WorkspacePermissionDatabaseManage),
		string(WorkspacePermissionFileView),
		string(WorkspacePermissionFileUploadCreate),
		string(WorkspacePermissionAgentManage),
	})

	expected := []WorkspacePermissionCode{
		WorkspacePermissionAgentCreate,
		WorkspacePermissionWorkflowPublish,
		WorkspacePermissionKnowledgeBaseDocumentView,
		WorkspacePermissionDatabaseSchemaManage,
		WorkspacePermissionFileMetadataView,
		WorkspacePermissionFileUpload,
		WorkspacePermissionFileTextCreate,
	}
	for _, code := range expected {
		if !WorkspacePermissionStringsAllow(permissions, code) && !containsWorkspacePermissionString(permissions, string(code)) {
			t.Errorf("canonical permissions missing %s in %#v", code, permissions)
		}
	}

	deprecated := []WorkspacePermissionCode{
		WorkspacePermissionAgentManage,
		WorkspacePermissionKnowledgeBaseView,
		WorkspacePermissionDatabaseManage,
		WorkspacePermissionFileView,
	}
	for _, code := range deprecated {
		if containsWorkspacePermissionString(permissions, string(code)) {
			t.Errorf("canonical permissions should replace deprecated asset permission %s: %#v", code, permissions)
		}
	}
	assertNoCompatibilityWorkspacePermissions(t, permissions)
}

func TestCanonicalAssignableWorkspacePermissionSnapshotStringsExpandsCompatibilityPermissionsWithoutRetainingThem(t *testing.T) {
	permissions := CanonicalAssignableWorkspacePermissionSnapshotStrings([]string{
		string(WorkspacePermissionDatabaseDataEdit),
		string(WorkspacePermissionDatabaseAIQuery),
		string(WorkspacePermissionFileUploadCreate),
		string(WorkspacePermissionFileMoveCreate),
	})

	expected := []WorkspacePermissionCode{
		WorkspacePermissionDatabaseRecordCreate,
		WorkspacePermissionDatabaseRecordUpdate,
		WorkspacePermissionDatabaseRecordDelete,
		WorkspacePermissionDatabaseImportExecute,
		WorkspacePermissionDatabaseImportErrorsView,
		WorkspacePermissionDatabaseAIQueryRead,
		WorkspacePermissionFileUpload,
		WorkspacePermissionFileTextCreate,
		WorkspacePermissionFileMove,
		WorkspacePermissionFileFolderManage,
	}
	for _, code := range expected {
		if !containsWorkspacePermissionString(permissions, string(code)) {
			t.Errorf("assignable permissions missing %s after compatibility expansion: %#v", code, permissions)
		}
	}
	assertNoCompatibilityWorkspacePermissions(t, permissions)
}

func TestCanonicalAssignableWorkspacePermissionSnapshotStringsExcludesRetiredToolPermissions(t *testing.T) {
	permissions := CanonicalAssignableWorkspacePermissionSnapshotStrings([]string{
		string(WorkspacePermissionAgentCreate),
		"prompt.optimize",
		"prompt.playground",
		"content_parse.chunk.preview",
	})

	if !containsWorkspacePermissionString(permissions, string(WorkspacePermissionAgentCreate)) {
		t.Fatalf("assignable permissions should keep configurable permissions: %#v", permissions)
	}

	assertNoRetiredWorkspacePermissions(t, permissions)
}

func TestCanonicalAssignableWorkspacePermissionSnapshotStringsExcludesDashboardPermissions(t *testing.T) {
	permissions := CanonicalAssignableWorkspacePermissionSnapshotStrings([]string{
		"workspace.view",
		"workspace.member.view",
		string(WorkspacePermissionAgentCreate),
		"dashboard.view",
		"dashboard.stats.view",
		"dashboard.recent_work.view",
		"dashboard.models.view",
	})

	if !containsWorkspacePermissionString(permissions, string(WorkspacePermissionAgentCreate)) {
		t.Fatalf("assignable permissions should keep configurable asset permission: %#v", permissions)
	}
	assertNoRetiredWorkspacePermissions(t, permissions)
}

func TestCanonicalAssignableWorkspacePermissionSnapshotStringsExcludesUnknownPermissions(t *testing.T) {
	permissions := CanonicalAssignableWorkspacePermissionSnapshotStrings([]string{
		string(WorkspacePermissionKnowledgeBaseFolderManage),
		"knowledge_base.folder.manage",
		"database.unknown",
	})

	if !containsWorkspacePermissionString(permissions, string(WorkspacePermissionKnowledgeBaseFolderManage)) {
		t.Fatalf("assignable permissions should keep known folder permission: %#v", permissions)
	}
	if containsWorkspacePermissionString(permissions, "knowledge_base.folder.manage") {
		t.Fatalf("assignable permissions should drop unknown folder permission alias: %#v", permissions)
	}
	if containsWorkspacePermissionString(permissions, "database.unknown") {
		t.Fatalf("assignable permissions should drop unknown permission code: %#v", permissions)
	}
}

func TestEffectiveWorkspaceMemberPermissionStringsExcludesDashboardPermissions(t *testing.T) {
	permissions := EffectiveWorkspaceMemberPermissionStrings(
		WorkspaceRoleNormal,
		nil,
		[]string{
			string(WorkspacePermissionAgentCreate),
			"dashboard.view",
			"dashboard.stats.view",
			"dashboard.recent_work.view",
			"dashboard.models.view",
		},
		WorkspaceMemberPermissionSourceDirect,
	)

	if !containsWorkspacePermissionString(permissions, string(WorkspacePermissionAgentCreate)) {
		t.Fatalf("effective permissions should keep configurable asset permission: %#v", permissions)
	}
	assertNoDashboardWorkspacePermissions(t, permissions)
}

func TestEffectiveWorkspaceMemberPermissionStringsExcludesUnknownPermissions(t *testing.T) {
	permissions := EffectiveWorkspaceMemberPermissionStrings(
		WorkspaceRoleNormal,
		nil,
		[]string{
			string(WorkspacePermissionDatabaseRecordView),
			"knowledge_base.folder.manage",
		},
		WorkspaceMemberPermissionSourceDirect,
	)

	if !containsWorkspacePermissionString(permissions, string(WorkspacePermissionDatabaseRecordView)) {
		t.Fatalf("effective permissions should keep known permission: %#v", permissions)
	}
	if containsWorkspacePermissionString(permissions, "knowledge_base.folder.manage") {
		t.Fatalf("effective permissions should drop unknown permission code: %#v", permissions)
	}
}

func TestAllWorkspacePermissionCodesAreUniqueAndContainFineCodes(t *testing.T) {
	seen := make(map[WorkspacePermissionCode]struct{})
	for _, code := range AllWorkspacePermissionCodes() {
		if _, ok := seen[code]; ok {
			t.Fatalf("duplicate workspace permission code %s", code)
		}
		if isRetiredWorkspacePermission(code) {
			t.Fatalf("all workspace permission codes should not contain retired permission %s", code)
		}
		seen[code] = struct{}{}
	}

	expected := []WorkspacePermissionCode{
		WorkspacePermissionWorkflowRunDraft,
		WorkspacePermissionKnowledgeBaseDocumentCreate,
		WorkspacePermissionDatabaseImportExecute,
	}
	for _, code := range expected {
		if _, ok := seen[code]; !ok {
			t.Errorf("all workspace permission codes missing %s", code)
		}
	}
}

func assertNoDashboardWorkspacePermissions(t *testing.T, permissions []string) {
	t.Helper()
	for _, permission := range permissions {
		if strings.HasPrefix(permission, "dashboard.") {
			t.Fatalf("permissions should not contain dashboard permission %s: %#v", permission, permissions)
		}
	}
}

func assertNoRetiredWorkspacePermissions(t *testing.T, permissions []string) {
	t.Helper()
	for _, permission := range permissions {
		if strings.HasPrefix(permission, "prompt.") ||
			strings.HasPrefix(permission, "content_parse.") ||
			strings.HasPrefix(permission, "dashboard.") ||
			strings.HasPrefix(permission, "workspace.") {
			t.Fatalf("permissions should not contain retired permission %s: %#v", permission, permissions)
		}
	}
}

func assertNoCompatibilityWorkspacePermissions(t *testing.T, permissions []string) {
	t.Helper()
	for _, permission := range permissions {
		if IsWorkspaceCompatibilityPermission(WorkspacePermissionCode(permission)) {
			t.Fatalf("permissions should not contain compatibility-only permission %s: %#v", permission, permissions)
		}
	}
}

func containsWorkspacePermissionString(permissions []string, want string) bool {
	for _, permission := range permissions {
		if permission == want {
			return true
		}
	}
	return false
}
