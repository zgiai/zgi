package digitalhuman

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/util"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

type detailServiceStub struct {
	HandlerService
	owner uuid.UUID
	id    uuid.UUID
}

func (s detailServiceStub) Get(_ context.Context, scope Scope, id uuid.UUID) (*Detail, error) {
	if id != s.id || scope.WorkspaceID != s.owner {
		return nil, applicationError(ErrNotFound, "digital_human.get")
	}
	return &Detail{Definition: &Definition{ID: id, WorkspaceID: s.owner}, Draft: &Version{}}, nil
}

type locatorStub struct{ workspaceID uuid.UUID }

func (l locatorStub) DefinitionWorkspace(context.Context, uuid.UUID, uuid.UUID) (uuid.UUID, error) {
	if l.workspaceID == uuid.Nil {
		return uuid.Nil, ErrNotFound
	}
	return l.workspaceID, nil
}

// A DigitalHuman of another workspace is read like a prompt or dataset of
// another workspace: someone who may view it there gets its detail, with its
// workspace, so the console offers to switch; anyone else gets not found.
func TestDetailOfAnotherWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope := testScope()
	owner := uuid.New()
	id := uuid.New()
	service := detailServiceStub{owner: owner, id: id}

	cases := []struct {
		name       string
		locator    DefinitionWorkspaceLocator
		canView    bool
		wantStatus int
	}{
		{"viewer of the other workspace", locatorStub{workspaceID: owner}, true, http.StatusOK},
		{"no view permission there", locatorStub{workspaceID: owner}, false, http.StatusNotFound},
		{"not in the organization", locatorStub{}, true, http.StatusNotFound},
		{"no locator", nil, true, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var checked []uuid.UUID
			canView := func(_ context.Context, organizationID, workspaceID, actorID uuid.UUID) (bool, error) {
				if organizationID != scope.OrganizationID || actorID != scope.ActorID {
					t.Fatalf("checked %s for %s", organizationID, actorID)
				}
				checked = append(checked, workspaceID)
				return tc.canView, nil
			}
			router := otherWorkspaceRouter(t, service, scope, tc.locator, canView)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/digital-humans/"+id.String(), nil))
			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, tc.wantStatus, recorder.Body.String())
			}
			if tc.wantStatus != http.StatusOK {
				return
			}
			if len(checked) != 1 || checked[0] != owner {
				t.Fatalf("permission checked in %v, want the owning workspace", checked)
			}
			var body struct {
				Data Detail `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Data.Definition == nil || body.Data.Definition.WorkspaceID != owner {
				t.Fatalf("detail = %#v, want its own workspace", body.Data.Definition)
			}
		})
	}
}

func otherWorkspaceRouter(t *testing.T, service HandlerService, scope Scope, locator DefinitionWorkspaceLocator, canView CanViewInWorkspace) *gin.Engine {
	t.Helper()
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(catalog)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, scope.OrganizationID.String())
		util.SetWorkspaceID(c, scope.WorkspaceID.String())
		c.Set("account_id", scope.ActorID.String())
		c.Next()
	})
	NewHandler(service, projector).WithOtherWorkspaceDetail(locator, canView).RegisterRoutes(router.Group(""))
	return router
}
