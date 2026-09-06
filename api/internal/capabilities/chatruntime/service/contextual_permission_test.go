package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestContextualAgentSkillDiscoveryUsesActionPermissions(t *testing.T) {
	workspaceID := uuid.New()
	for _, tc := range []struct {
		permission   workspacemodel.WorkspacePermissionCode
		read, mutate bool
	}{
		{workspacemodel.WorkspacePermissionAgentView, true, false},
		{workspacemodel.WorkspacePermissionAgentCreate, false, true},
		{workspacemodel.WorkspacePermissionAgentUpdate, false, true},
		{workspacemodel.WorkspacePermissionAgentDelete, false, true},
		{workspacemodel.WorkspacePermissionWorkflowCreate, false, false},
		{workspacemodel.WorkspacePermissionAgentMove, false, false},
	} {
		t.Run(string(tc.permission), func(t *testing.T) {
			perms := &skillConfigWorkspacePermissionService{allowed: map[workspacemodel.WorkspacePermissionCode]bool{tc.permission: true}}
			got := (&service{workspacePerms: perms}).trustedContextualAIChatSkillCapabilities(context.Background(), Scope{
				OrganizationID: uuid.New(), AccountID: uuid.New(), WorkspaceID: &workspaceID,
			}, contextualConsoleAgentsManageCapabilityPartsForTest())
			if got.AgentRead != tc.read || got.AgentManage != tc.mutate {
				t.Fatalf("capabilities=%+v want read=%v mutate=%v", got, tc.read, tc.mutate)
			}
			for _, code := range perms.codes {
				if code == workspacemodel.WorkspacePermissionAgentManage {
					t.Fatalf("checked retired aggregate permission: %s", code)
				}
			}
		})
	}
}

func TestContextualFileSkillDiscoveryUsesIndependentPermissions(t *testing.T) {
	workspaceID := uuid.New()
	for mask := 0; mask < 8; mask++ {
		perms := &skillConfigWorkspacePermissionService{allowed: map[workspacemodel.WorkspacePermissionCode]bool{
			workspacemodel.WorkspacePermissionFilePreview: mask&1 != 0,
			workspacemodel.WorkspacePermissionFileUpload:  mask&2 != 0,
			workspacemodel.WorkspacePermissionFileDelete:  mask&4 != 0,
		}}
		got := (&service{workspacePerms: perms}).trustedContextualAIChatSkillCapabilities(context.Background(), Scope{
			OrganizationID: uuid.New(), AccountID: uuid.New(), WorkspaceID: &workspaceID,
		}, contextualConsoleFilesAllCapabilityPartsForTest())
		if got.FileRead != (mask&1 != 0) || got.FileCreate != (mask&2 != 0) || got.FileDelete != (mask&4 != 0) {
			t.Fatalf("mask=%d capabilities=%+v", mask, got)
		}
	}
}
