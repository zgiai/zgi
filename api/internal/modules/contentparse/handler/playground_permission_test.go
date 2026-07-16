package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	authmodel "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestPlaygroundRoutesRequireWorkspaceViewBeforeRequestHandling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name   string
		method string
		target string
	}{
		{name: "providers", method: http.MethodGet, target: "/content-parse/playground/providers"},
		{name: "parse", method: http.MethodPost, target: "/content-parse/playground/parse"},
		{name: "save", method: http.MethodPost, target: "/content-parse/playground/save"},
		{name: "runs", method: http.MethodGet, target: "/content-parse/playground/runs"},
		{name: "pdf render", method: http.MethodPost, target: "/content-parse/playground/pdf-render"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			organization := &playgroundPermissionOrganization{allowed: false}
			router := newPlaygroundPermissionRouter(organization, func(c *gin.Context) {
				util.SetOrganizationID(c, "org-1")
				util.SetWorkspaceScopeCompat(c, "workspace-1")
				c.Set("account_id", "account-1")
			})

			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.target, strings.NewReader("not request-shape-valid"))
			router.ServeHTTP(recorder, req)

			requirePlaygroundPermissionCode(t, recorder, response.ErrPermissionDenied)
			if !organization.checked {
				t.Fatal("expected workspace permission check")
			}
			if organization.organizationID != "org-1" || organization.workspaceID != "workspace-1" || organization.accountID != "account-1" {
				t.Fatalf("permission scope = (%q, %q, %q), want (org-1, workspace-1, account-1)", organization.organizationID, organization.workspaceID, organization.accountID)
			}
			if organization.permission != workspace_model.WorkspacePermissionWorkspaceView {
				t.Fatalf("permission = %q, want %q", organization.permission, workspace_model.WorkspacePermissionWorkspaceView)
			}
		})
	}
}

func TestPlaygroundRoutesRequireCurrentWorkspaceBeforeRequestHandling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	organization := &playgroundPermissionOrganization{allowed: true}
	router := newPlaygroundPermissionRouter(organization, func(c *gin.Context) {
		util.SetOrganizationID(c, "org-1")
		c.Set("account_id", "account-1")
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/content-parse/playground/parse", strings.NewReader("not multipart"))
	router.ServeHTTP(recorder, req)

	requirePlaygroundPermissionCode(t, recorder, response.ErrPermissionDenied)
	if organization.checked {
		t.Fatal("workspace permission service should not be called without a current workspace")
	}
}

func TestPlaygroundRoutesResolveExplicitWorkspaceIDBeforeRequestHandling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	organization := &playgroundPermissionOrganization{allowed: false}
	router := newPlaygroundPermissionRouter(organization, func(c *gin.Context) {
		util.SetOrganizationID(c, "org-1")
		c.Set("account_id", "account-1")
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/content-parse/playground/providers?workspace_id=workspace-query", nil)
	router.ServeHTTP(recorder, req)

	requirePlaygroundPermissionCode(t, recorder, response.ErrPermissionDenied)
	if !organization.checked {
		t.Fatal("expected workspace permission check")
	}
	if organization.workspaceID != "workspace-query" {
		t.Fatalf("workspace checked = %q, want workspace-query", organization.workspaceID)
	}
}

func TestPlaygroundRoutesResolveCurrentWorkspaceFromAccountContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspaceID := "workspace-from-account"
	organization := &playgroundPermissionOrganization{allowed: true}
	account := &playgroundPermissionAccount{
		context: &authmodel.AccountContext{
			AccountID:          "account-1",
			CurrentWorkspaceID: &workspaceID,
		},
	}
	router := newPlaygroundPermissionRouterWithAccount(organization, account, func(c *gin.Context) {
		util.SetOrganizationID(c, "org-1")
		c.Set("account_id", "account-1")
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/content-parse/playground/providers", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code == http.StatusForbidden {
		t.Fatalf("current workspace from account context should pass permission guard, body=%s", recorder.Body.String())
	}
	if !organization.checked {
		t.Fatal("expected workspace permission check")
	}
	if organization.workspaceID != workspaceID {
		t.Fatalf("workspace checked = %q, want %q", organization.workspaceID, workspaceID)
	}
}

func TestInternalPlaygroundRoutesBypassWorkspaceViewGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)

	organization := &playgroundPermissionOrganization{allowed: false}
	router := gin.New()
	group := router.Group("/console/api/internal")
	RegisterInternalRoutes(group, nil, nil, nil, nil, nil, &PlaygroundHandler{organization: organization})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/console/api/internal/content-parse/playground/providers", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code == http.StatusNotFound {
		t.Fatal("internal playground route should be registered")
	}
	if recorder.Code == http.StatusForbidden {
		t.Fatalf("internal playground route should bypass workspace permission guard, body=%s", recorder.Body.String())
	}
	if organization.checked {
		t.Fatal("internal playground route should not call workspace permission service")
	}
}

func newPlaygroundPermissionRouter(organization *playgroundPermissionOrganization, scope func(*gin.Context)) *gin.Engine {
	return newPlaygroundPermissionRouterWithAccount(organization, nil, scope)
}

func newPlaygroundPermissionRouterWithAccount(organization *playgroundPermissionOrganization, account playgroundAccountContextReader, scope func(*gin.Context)) *gin.Engine {
	router := gin.New()
	group := router.Group("/content-parse")
	group.Use(func(c *gin.Context) {
		if scope != nil {
			scope(c)
		}
		c.Next()
	})
	handler := &PlaygroundHandler{organization: organization, account: account}
	handler.RegisterRoutes(group)
	return router
}

func requirePlaygroundPermissionCode(t *testing.T, recorder *httptest.ResponseRecorder, want response.ErrorCode) {
	t.Helper()
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	wantCode := `"code":"` + strconv.Itoa(want.Code) + `"`
	if !strings.Contains(recorder.Body.String(), wantCode) {
		t.Fatalf("response body = %s, want code %s", recorder.Body.String(), wantCode)
	}
}

type playgroundPermissionOrganization struct {
	interfaces.OrganizationService

	allowed        bool
	checked        bool
	organizationID string
	workspaceID    string
	accountID      string
	permission     workspace_model.WorkspacePermissionCode
}

func (s *playgroundPermissionOrganization) CheckWorkspacePermission(_ context.Context, organizationID, workspaceID, accountID string, permissionCode workspace_model.WorkspacePermissionCode) (bool, error) {
	s.checked = true
	s.organizationID = organizationID
	s.workspaceID = workspaceID
	s.accountID = accountID
	s.permission = permissionCode
	return s.allowed, nil
}

type playgroundPermissionAccount struct {
	interfaces.AccountService

	context *authmodel.AccountContext
}

func (s *playgroundPermissionAccount) GetAccountContext(context.Context, string) (*authmodel.AccountContext, error) {
	return s.context, nil
}
