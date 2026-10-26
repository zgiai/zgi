package v1

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/backgroundauth"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
)

type backgroundGrantRouteServiceStub struct {
	organizationID uuid.UUID
	workspaceID    uuid.UUID
	ownerID        uuid.UUID
	bindingID      uuid.UUID
	grantID        uuid.UUID
	operations     []string
	destinations   map[string]string
	expiresAt      time.Time
	maxInvocations int
	issueCalls     int
	listCalls      int
	revokeCalls    int
	grant          backgroundauth.Grant
	grants         []backgroundauth.Grant
	err            error
}

func (s *backgroundGrantRouteServiceStub) IssueGrant(_ context.Context, organizationID, workspaceID, ownerID, bindingID uuid.UUID, operations []string, destinations map[string]string, expiresAt time.Time, maxInvocations int) (backgroundauth.Grant, error) {
	s.issueCalls++
	s.organizationID, s.workspaceID, s.ownerID, s.bindingID = organizationID, workspaceID, ownerID, bindingID
	s.operations, s.destinations, s.expiresAt, s.maxInvocations = operations, destinations, expiresAt, maxInvocations
	return s.grant, s.err
}
func (s *backgroundGrantRouteServiceStub) ListGrants(_ context.Context, organizationID, workspaceID, ownerID uuid.UUID) ([]backgroundauth.Grant, error) {
	s.listCalls++
	s.organizationID, s.workspaceID, s.ownerID = organizationID, workspaceID, ownerID
	return s.grants, s.err
}
func (s *backgroundGrantRouteServiceStub) RevokeGrant(_ context.Context, organizationID, workspaceID, ownerID, grantID uuid.UUID) error {
	s.revokeCalls++
	s.organizationID, s.workspaceID, s.ownerID, s.grantID = organizationID, workspaceID, ownerID, grantID
	return s.err
}

type backgroundGrantPermissionStub struct {
	allowed    bool
	calls      int
	permission workspacemodel.WorkspacePermissionCode
}

func (s *backgroundGrantPermissionStub) CheckWorkspacePermission(_ context.Context, _, _, _ string, permission workspacemodel.WorkspacePermissionCode) (bool, error) {
	s.calls++
	s.permission = permission
	return s.allowed, nil
}

func TestConnectorBackgroundGrantRoutesUseAuthenticatedOwnerWorkspaceAndExplicitConstraints(t *testing.T) {
	organizationID, workspaceID, ownerID, bindingID, grantID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	expiresAt := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	service := &backgroundGrantRouteServiceStub{grant: backgroundauth.Grant{ID: grantID, Scope: backgroundauth.Scope{OrganizationID: organizationID,
		OwnerID: ownerID, WorkspaceID: workspaceID}, OperationsJSON: `["x-social.post.create"]`, DestinationsJSON: `{"account_id":"account-1"}`,
		ExpiresAt: expiresAt, MaxInvocations: 4}}
	permissions := &backgroundGrantPermissionStub{allowed: true}
	router := backgroundGrantRouteTestRouter(organizationID, workspaceID, ownerID, service, permissions)
	body := `{"operations":["x-social.post.create"],"destinations":{"account_id":"account-1"},"expires_at":"2026-10-21T12:00:00Z","max_invocations":4}`
	response := connectorRouteRequest(t, router, http.MethodPost, "/console/api/connector-target-bindings/"+bindingID.String()+"/background-grants", body, nil)
	if response.Code != http.StatusOK || service.issueCalls != 1 {
		t.Fatalf("issue status/body/calls = %d %s %d", response.Code, response.Body.String(), service.issueCalls)
	}
	if service.organizationID != organizationID || service.workspaceID != workspaceID || service.ownerID != ownerID || service.bindingID != bindingID ||
		len(service.operations) != 1 || service.destinations["account_id"] != "account-1" || service.maxInvocations != 4 || !service.expiresAt.Equal(expiresAt) {
		t.Fatalf("issue scope = %#v", service)
	}
	if permissions.permission != workspacemodel.WorkspacePermissionDigitalHumanUpdate || !strings.Contains(response.Body.String(), `"operations":["x-social.post.create"]`) ||
		!strings.Contains(response.Body.String(), `"destinations":{"account_id":"account-1"}`) {
		t.Fatalf("permission or response projection = %q, %s", permissions.permission, response.Body.String())
	}

	malicious := `{"organization_id":"` + uuid.NewString() + `","operations":["x-social.post.create"],"destinations":{},"expires_at":"2026-10-21T12:00:00Z","max_invocations":1}`
	response = connectorRouteRequest(t, router, http.MethodPost, "/console/api/connector-target-bindings/"+bindingID.String()+"/background-grants", malicious, nil)
	if response.Code == http.StatusOK || service.issueCalls != 1 {
		t.Fatalf("caller scope field accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestConnectorBackgroundGrantRoutesListAndRevokeOnlyAfterEditPermission(t *testing.T) {
	organizationID, workspaceID, ownerID, grantID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	grant := backgroundauth.Grant{ID: grantID, Scope: backgroundauth.Scope{OrganizationID: organizationID, OwnerID: ownerID, WorkspaceID: workspaceID},
		OperationsJSON: `["x-social.post.create"]`, DestinationsJSON: `{}`, ExpiresAt: time.Now().Add(time.Hour), MaxInvocations: 1}
	service := &backgroundGrantRouteServiceStub{grants: []backgroundauth.Grant{grant}}
	permissions := &backgroundGrantPermissionStub{allowed: false}
	router := backgroundGrantRouteTestRouter(organizationID, workspaceID, ownerID, service, permissions)
	response := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connector-background-grants", "", nil)
	if response.Code == http.StatusOK || service.listCalls != 0 {
		t.Fatalf("permission denial reached list: %d %s", response.Code, response.Body.String())
	}

	permissions.allowed = true
	response = connectorRouteRequest(t, router, http.MethodGet, "/console/api/connector-background-grants", "", nil)
	if response.Code != http.StatusOK || service.listCalls != 1 || !strings.Contains(response.Body.String(), grantID.String()) {
		t.Fatalf("list = %d %s calls=%d", response.Code, response.Body.String(), service.listCalls)
	}
	response = connectorRouteRequest(t, router, http.MethodDelete, "/console/api/connector-background-grants/"+grantID.String(), "", nil)
	if response.Code != http.StatusOK || service.revokeCalls != 1 || service.grantID != grantID {
		t.Fatalf("revoke = %d %s service=%#v", response.Code, response.Body.String(), service)
	}
}

func backgroundGrantRouteTestRouter(organizationID, workspaceID, ownerID uuid.UUID, service connectorBackgroundGrantService, permissions connectorBackgroundGrantPermissionService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	testRouter := gin.New()
	group := testRouter.Group("/console/api")
	group.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, organizationID.String())
		util.SetWorkspaceScopeCompat(c, workspaceID.String())
		c.Set("account_id", ownerID.String())
		c.Next()
	})
	(&connectorHandler{backgroundGrants: service, backgroundGrantPermissions: permissions}).registerBackgroundAuthorization(group)
	return testRouter
}
