package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
)

type currentWorkspaceAccountService struct {
	interfaces.AccountService
	workspace *workspacemodel.Workspace
}

func (s currentWorkspaceAccountService) GetCurrentWorkspace(context.Context, string) (*workspacemodel.Workspace, error) {
	return s.workspace, nil
}

func TestCurrentWorkspaceRequiredSetsWorkspaceScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", "account-1")
		c.Set(accountServiceKey, currentWorkspaceAccountService{
			workspace: &workspacemodel.Workspace{ID: "workspace-1"},
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
