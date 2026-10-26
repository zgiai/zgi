package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

type taskManagerPermissionService struct {
	interfaces.OrganizationService
	granted map[workspacemodel.WorkspacePermissionCode]bool
}

func (s taskManagerPermissionService) CheckWorkspacePermission(_ context.Context, _, _, _ string, code workspacemodel.WorkspacePermissionCode) (bool, error) {
	return s.granted[code], nil
}

func TestDigitalHumanPermissionMiddlewareRecordsTaskCenterManager(t *testing.T) {
	gin.SetMode(gin.TestMode)
	definitions, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, appcatalog.DefaultDefinitions()...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(definitions)
	if err != nil {
		t.Fatal(err)
	}
	viewer := map[workspacemodel.WorkspacePermissionCode]bool{workspacemodel.WorkspacePermissionDigitalHumanView: true, workspacemodel.WorkspacePermissionDigitalHumanRun: true}
	manager := map[workspacemodel.WorkspacePermissionCode]bool{workspacemodel.WorkspacePermissionDigitalHumanView: true, workspacemodel.WorkspacePermissionDigitalHumanUpdate: true, workspacemodel.WorkspacePermissionDigitalHumanRun: true}
	cases := []struct {
		name     string
		granted  map[workspacemodel.WorkspacePermissionCode]bool
		path     string
		wantSet  bool
		wantFlag bool
	}{
		{name: "viewer", granted: viewer, path: "/digital-humans/:id/tasks", wantSet: true, wantFlag: false},
		{name: "manager", granted: manager, path: "/digital-humans/:id/tasks", wantSet: true, wantFlag: true},
		{name: "personal view", granted: manager, path: "/digital-humans/:id/my-tasks", wantSet: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			var set, flag bool
			router.GET(tc.path, digitalHumanPermissionMiddleware(taskManagerPermissionService{granted: tc.granted}, projector), func(c *gin.Context) {
				var value any
				value, set = c.Get(digitalhuman.TaskManagerContextKey)
				flag, _ = value.(bool)
				c.Status(http.StatusNoContent)
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/digital-humans/abc/"+map[bool]string{true: "my-tasks", false: "tasks"}[tc.path == "/digital-humans/:id/my-tasks"], nil))
			if recorder.Code != http.StatusNoContent {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if set != tc.wantSet || flag != tc.wantFlag {
				t.Fatalf("manager flag set=%v value=%v, want set=%v value=%v", set, flag, tc.wantSet, tc.wantFlag)
			}
		})
	}
}
