package agentmanagement

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestCreateAgentUsesExactCreatePermission(t *testing.T) {
	for _, tc := range []struct {
		permission workspacemodel.WorkspacePermissionCode
		allowed    bool
	}{
		{workspacemodel.WorkspacePermissionAgentCreate, true},
		{workspacemodel.WorkspacePermissionAgentManage, true},
		{workspacemodel.WorkspacePermissionAgentView, false},
		{workspacemodel.WorkspacePermissionAgentUpdate, false},
		{workspacemodel.WorkspacePermissionAgentDelete, false},
		{workspacemodel.WorkspacePermissionWorkflowCreate, false},
	} {
		t.Run(string(tc.permission), func(t *testing.T) {
			service := &fakeAgentManagementService{}
			perms := &fakeWorkspacePermissionService{granted: []workspacemodel.WorkspacePermissionCode{tc.permission}}
			tool := newCreateAgentTool(service, perms).ForkToolRuntime(&tools.ToolRuntime{
				TenantID: "org-1", InvokeFrom: tools.ToolInvokeFromAIChat,
				RuntimeParameters: map[string]interface{}{"organization_id": "org-1", "workspace_id": "workspace-1"},
			})
			_, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{"name": "Permission fixture"}, nil, nil, nil)
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v error=%v", tc.allowed, err)
			}
			wantCalls := 0
			if tc.allowed {
				wantCalls = 1
			}
			if service.createAgentCalls != wantCalls {
				t.Fatalf("CreateAgent calls=%d want=%d", service.createAgentCalls, wantCalls)
			}
			if perms.permissionCode != workspacemodel.WorkspacePermissionAgentCreate || perms.workspaceID != "workspace-1" || perms.organizationID != "org-1" || perms.accountID != "account-1" {
				t.Fatalf("wrong scoped permission check: %#v", perms)
			}
		})
	}
}
