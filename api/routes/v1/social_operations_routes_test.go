package v1

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"net/http"
	"testing"
)

type socialSetupRouteStub struct {
	scope         digitalhuman.Scope
	employee      uuid.UUID
	calls         int
	configuration socialops.Configuration
}

func (s *socialSetupRouteStub) GetConfiguration(_ context.Context, scope digitalhuman.Scope, id uuid.UUID) (socialops.Configuration, socialops.Record, error) {
	s.scope, s.employee = scope, id
	return socialops.Configuration{}, socialops.Record{}, nil
}
func (s *socialSetupRouteStub) SaveConfiguration(_ context.Context, scope digitalhuman.Scope, id uuid.UUID, revision int64, cfg socialops.Configuration) (socialops.Configuration, socialops.Record, error) {
	s.scope, s.employee, s.configuration = scope, id, cfg
	s.calls++
	return cfg, socialops.Record{Revision: revision + 1}, nil
}
func (s *socialSetupRouteStub) Preflight(_ context.Context, scope digitalhuman.Scope, id uuid.UUID) (socialops.PreflightReport, error) {
	s.scope, s.employee = scope, id
	return socialops.PreflightReport{Ready: false}, nil
}
func (s *socialSetupRouteStub) Setup(_ context.Context, scope digitalhuman.Scope, id uuid.UUID, revision int64, version uuid.UUID) (socialops.ProvisioningResult, error) {
	s.scope, s.employee = scope, id
	s.calls++
	return socialops.ProvisioningResult{}, nil
}
func TestSocialOperationsRoutesRejectInjectedAuthorityAndRestoreScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	org, workspace, owner, employee := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	router := gin.New()
	group := router.Group("/console/api")
	group.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, org.String())
		util.SetWorkspaceScopeCompat(c, workspace.String())
		c.Set("account_id", owner.String())
		c.Next()
	})
	service := &socialSetupRouteStub{}
	registerSocialOperationsRoutes(group, service)
	path := "/console/api/digital-humans/" + employee.String() + "/social-operations/configuration"
	for _, body := range []string{`{"expected_revision":0,"configuration":{"scope":{"owner_id":"forged"}}}`, `{"owner_id":"forged","expected_revision":0,"configuration":{}}`, `{"expected_revision":0,"configuration":{"employee_version_id":"` + uuid.NewString() + `"}}`} {
		response := connectorRouteRequest(t, router, http.MethodPut, path, body, nil)
		if response.Code == http.StatusOK || service.calls != 0 {
			t.Fatalf("authority accepted: %d %s", response.Code, response.Body.String())
		}
	}
	researchConnection := uuid.New()
	response := connectorRouteRequest(t, router, http.MethodPut, path, `{"expected_revision":0,"configuration":{"product":{"name":"Acme","description":"Product"},"audience":"Developers","research":{"connector_id":"web-search","connection_id":"`+researchConnection.String()+`"}}}`, nil)
	if response.Code != http.StatusOK || service.calls != 1 || service.scope.OrganizationID != org || service.scope.WorkspaceID != workspace || service.scope.ActorID != owner || service.employee != employee || service.configuration.Scope.OwnerID != uuid.Nil {
		t.Fatalf("scope or request: %d %s %#v", response.Code, response.Body.String(), service)
	}
	if service.configuration.Research.ConnectorID != "web-search" || service.configuration.Research.ConnectionID != researchConnection {
		t.Fatal("research selection dropped by DTO")
	}
}
func TestSocialOperationsSetupRequiresUpdatePermission(t *testing.T) {
	for _, path := range []string{"/digital-humans/:id/social-operations/preflight", "/digital-humans/:id/social-operations/setup"} {
		if actual := digitalHumanPermissionForRequest(http.MethodPost, path); actual != workspacemodel.WorkspacePermissionDigitalHumanUpdate {
			t.Fatalf("permission=%s", actual)
		}
	}
}
