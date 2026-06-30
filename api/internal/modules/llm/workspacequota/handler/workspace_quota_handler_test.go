package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/llm/workspacequota/dto"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type fakeWorkspaceQuotaService struct {
	getCalled      bool
	listCalled     bool
	organizationID string
	workspaceID    string
}

func (s *fakeWorkspaceQuotaService) ListWorkspaceQuotas(context.Context, string, *dto.ListWorkspaceQuotaRequest) (*dto.ListWorkspaceQuotaResponse, error) {
	s.listCalled = true
	return &dto.ListWorkspaceQuotaResponse{}, nil
}

func (s *fakeWorkspaceQuotaService) GetWorkspaceQuota(_ context.Context, organizationID, workspaceID string) (*dto.WorkspaceQuotaResponse, error) {
	s.getCalled = true
	s.organizationID = organizationID
	s.workspaceID = workspaceID
	return &dto.WorkspaceQuotaResponse{
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		RemainQuota:    3000,
		Configured:     true,
	}, nil
}

func (s *fakeWorkspaceQuotaService) UpdateWorkspaceQuota(context.Context, string, string, *dto.UpdateWorkspaceQuotaRequest) (*dto.WorkspaceQuotaResponse, error) {
	return &dto.WorkspaceQuotaResponse{}, nil
}

type fakeWorkspaceQuotaPermissionChecker struct {
	allowed        bool
	err            error
	organizationID string
	workspaceID    string
	accountID      string
	permissions    []workspacemodel.WorkspacePermissionCode
}

func (c *fakeWorkspaceQuotaPermissionChecker) CheckWorkspaceOrganizationAnyPermission(_ context.Context, organizationID, workspaceID, accountID string, permissionCodes ...workspacemodel.WorkspacePermissionCode) (bool, error) {
	c.organizationID = organizationID
	c.workspaceID = workspaceID
	c.accountID = accountID
	c.permissions = append([]workspacemodel.WorkspacePermissionCode(nil), permissionCodes...)
	return c.allowed, c.err
}

func TestGetWorkspaceQuotaAllowsWorkspaceViewPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	quotaSvc := &fakeWorkspaceQuotaService{}
	permissionChecker := &fakeWorkspaceQuotaPermissionChecker{allowed: true}
	h := NewWorkspaceQuotaHandler(quotaSvc, permissionChecker)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("organization_id", "org-1")
	c.Set("account_id", "account-1")
	c.Params = gin.Params{{Key: "workspace_id", Value: "ws-1"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/workspace-quotas/ws-1", nil)

	h.GetWorkspaceQuota(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !quotaSvc.getCalled {
		t.Fatal("expected quota service to be called")
	}
	if quotaSvc.organizationID != "org-1" || quotaSvc.workspaceID != "ws-1" {
		t.Fatalf("service request = (%q, %q), want (org-1, ws-1)", quotaSvc.organizationID, quotaSvc.workspaceID)
	}
	if permissionChecker.accountID != "account-1" || len(permissionChecker.permissions) != 1 || permissionChecker.permissions[0] != workspacemodel.WorkspacePermissionWorkspaceView {
		t.Fatalf("permission check = account %q permissions %v, want workspace.view", permissionChecker.accountID, permissionChecker.permissions)
	}
}

func TestGetWorkspaceQuotaRejectsMissingWorkspaceViewPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	quotaSvc := &fakeWorkspaceQuotaService{}
	permissionChecker := &fakeWorkspaceQuotaPermissionChecker{allowed: false}
	h := NewWorkspaceQuotaHandler(quotaSvc, permissionChecker)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("organization_id", "org-1")
	c.Set("account_id", "account-1")
	c.Params = gin.Params{{Key: "workspace_id", Value: "ws-1"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/workspace-quotas/ws-1", nil)

	h.GetWorkspaceQuota(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d body=%s", w.Code, w.Body.String())
	}
	if quotaSvc.getCalled {
		t.Fatal("quota service should not be called without workspace permission")
	}
}

func TestWorkspaceQuotaRoutesKeepSingleReadOutsideAdminGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewWorkspaceQuotaHandler(
		&fakeWorkspaceQuotaService{},
		&fakeWorkspaceQuotaPermissionChecker{allowed: true},
	)
	r := gin.New()

	readGroup := r.Group("")
	readGroup.Use(func(c *gin.Context) {
		c.Set("organization_id", "org-1")
		c.Set("account_id", "account-1")
		c.Next()
	})
	RegisterWorkspaceQuotaReadRoutes(readGroup, h)

	adminGroup := r.Group("")
	adminGroup.Use(func(c *gin.Context) {
		c.AbortWithStatus(http.StatusTeapot)
	})
	RegisterWorkspaceQuotaAdminRoutes(adminGroup, h)

	readRecorder := httptest.NewRecorder()
	readReq := httptest.NewRequest(http.MethodGet, "/workspace-quotas/ws-1", nil)
	r.ServeHTTP(readRecorder, readReq)
	if readRecorder.Code != http.StatusOK {
		t.Fatalf("single workspace quota read status = %d body=%s, want 200", readRecorder.Code, readRecorder.Body.String())
	}

	listRecorder := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/workspace-quotas", nil)
	r.ServeHTTP(listRecorder, listReq)
	if listRecorder.Code != http.StatusTeapot {
		t.Fatalf("workspace quota list status = %d, want admin middleware status %d", listRecorder.Code, http.StatusTeapot)
	}
}
