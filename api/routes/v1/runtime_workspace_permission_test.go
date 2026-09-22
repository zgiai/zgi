package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/util"
)

func TestRuntimeWorkspaceManagerRequiredUsesExplicitWorkspaceService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &runtimeWorkspacePermissionService{allowed: true}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/console/api/runtime-workspaces", nil)
	ctx.Set("account_id", "account-1")
	util.SetWorkspaceScopeCompat(ctx, "workspace-1")

	runtimeWorkspaceManagerRequired(service)(ctx)

	if ctx.IsAborted() {
		t.Fatalf("authorized Runtime Workspace manager was rejected: %s", recorder.Body.String())
	}
	if service.workspaceID != "workspace-1" || service.accountID != "account-1" {
		t.Fatalf("permission scope = %q/%q", service.workspaceID, service.accountID)
	}
}

func TestRuntimeWorkspaceManagerRequiredRejectsDeniedAndMissingScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name        string
		workspaceID string
		accountID   string
		allowed     bool
	}{
		{name: "permission denied", workspaceID: "workspace-1", accountID: "account-1"},
		{name: "missing workspace", accountID: "account-1", allowed: true},
		{name: "missing account", workspaceID: "workspace-1", allowed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &runtimeWorkspacePermissionService{allowed: test.allowed}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/console/api/runtime-workspaces", nil)
			if test.accountID != "" {
				ctx.Set("account_id", test.accountID)
			}
			if test.workspaceID != "" {
				util.SetWorkspaceScopeCompat(ctx, test.workspaceID)
			}

			runtimeWorkspaceManagerRequired(service)(ctx)

			if !ctx.IsAborted() || recorder.Code != http.StatusForbidden {
				t.Fatalf("status/aborted = %d/%v, want %d/true", recorder.Code, ctx.IsAborted(), http.StatusForbidden)
			}
		})
	}
}

type runtimeWorkspacePermissionService struct {
	interfaces.WorkspaceManagementService
	allowed     bool
	workspaceID string
	accountID   string
}

func (service *runtimeWorkspacePermissionService) CheckPermission(_ context.Context, workspaceID, accountID string) bool {
	service.workspaceID = workspaceID
	service.accountID = accountID
	return service.allowed
}
