package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

type currentWorkspaceAccountService struct {
	interfaces.AccountService
	workspace *workspacemodel.Workspace
}

func (s currentWorkspaceAccountService) GetCurrentWorkspace(context.Context, string) (*workspacemodel.Workspace, error) {
	return s.workspace, nil
}

func TestCurrentWorkspaceRequiredSetsWorkspaceScope(t *testing.T) {
	organizationID := "organization-1"
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", "account-1")
		util.SetOrganizationID(c, organizationID)
		c.Set(accountServiceKey, currentWorkspaceAccountService{
			workspace: &workspacemodel.Workspace{ID: "workspace-1", OrganizationID: &organizationID},
		})
		c.Next()
	})
	router.Use(CurrentWorkspaceRequired())
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, util.GetWorkspaceID(c))
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Body.String() != "workspace-1" {
		t.Fatalf("workspace id = %q, want %q", recorder.Body.String(), "workspace-1")
	}
}

func TestCurrentWorkspaceRequiredRejectsMissingWorkspace(t *testing.T) {
	recorder := performCurrentWorkspaceRequest(t, "organization-1", nil)
	if recorder.Code == http.StatusOK {
		t.Fatalf("status = %d, want non-OK", recorder.Code)
	}
}

func TestCurrentWorkspaceRequiredRejectsWorkspaceFromAnotherOrganization(t *testing.T) {
	workspaceOrganizationID := "organization-2"
	recorder := performCurrentWorkspaceRequest(t, "organization-1", &workspacemodel.Workspace{
		ID:             "workspace-1",
		OrganizationID: &workspaceOrganizationID,
	})

	var body response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != strconv.Itoa(response.ErrWorkspaceNotInOrganization.Code) {
		t.Fatalf("code = %q, want %d", body.Code, response.ErrWorkspaceNotInOrganization.Code)
	}
}

func performCurrentWorkspaceRequest(t *testing.T, organizationID string, workspace *workspacemodel.Workspace) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", "account-1")
		util.SetOrganizationID(c, organizationID)
		c.Set(accountServiceKey, currentWorkspaceAccountService{workspace: workspace})
		c.Next()
	})
	router.Use(CurrentWorkspaceRequired())
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(recorder, request)
	return recorder
}
