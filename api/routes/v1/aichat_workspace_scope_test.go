package v1

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"gorm.io/gorm"
)

type fakeAIChatCurrentWorkspaceProvider struct {
	workspace *model.Workspace
	err       error
	calls     int
}

func (provider *fakeAIChatCurrentWorkspaceProvider) GetCurrentWorkspace(context.Context, string) (*model.Workspace, error) {
	provider.calls++
	return provider.workspace, provider.err
}

type fakeAIChatWorkspaceProvider struct {
	workspaces       map[uuid.UUID]*model.Workspace
	memberships      map[uuid.UUID]bool
	err              error
	membershipError  error
	calls            int
	membershipChecks int
}

func (provider *fakeAIChatWorkspaceProvider) GetWorkspace(_ context.Context, workspaceID uuid.UUID) (*model.Workspace, error) {
	provider.calls++
	if provider.err != nil {
		return nil, provider.err
	}
	return provider.workspaces[workspaceID], nil
}

func (provider *fakeAIChatWorkspaceProvider) IsWorkspaceMember(_ context.Context, workspaceID, _ uuid.UUID) (bool, error) {
	provider.membershipChecks++
	if provider.membershipError != nil {
		return false, provider.membershipError
	}
	return provider.memberships[workspaceID], nil
}

func TestAIChatWorkspaceScopeResolvesWorkspaceScopedPreferences(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	connectionID := uuid.New()
	organizationIDRaw := organizationID.String()
	provider := &fakeAIChatCurrentWorkspaceProvider{workspace: &model.Workspace{
		ID:             workspaceID.String(),
		OrganizationID: &organizationIDRaw,
		Status:         model.WorkspaceStatusNormal,
	}}
	preferencesByWorkspace := map[uuid.UUID][]string{
		workspaceID: {"github:" + connectionID.String()},
	}
	var resolvedWorkspaceID string
	var resolvedPreferences []string
	workspaceProvider := &fakeAIChatWorkspaceProvider{memberships: map[uuid.UUID]bool{workspaceID: true}}
	resolver := aichatWorkspaceScopeResolver{workspaces: workspaceProvider, accounts: provider}
	router := newAIChatWorkspaceScopeTestRouter(resolver, organizationID, accountID, "", func(c *gin.Context) {
		resolvedWorkspaceID = util.GetWorkspaceID(c)
		parsedWorkspaceID, err := uuid.Parse(resolvedWorkspaceID)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		resolvedPreferences = preferencesByWorkspace[parsedWorkspaceID]
		c.Status(http.StatusNoContent)
	})

	response := performAIChatWorkspaceScopeRequest(router)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if resolvedWorkspaceID != workspaceID.String() {
		t.Fatalf("workspace scope = %q, want %q", resolvedWorkspaceID, workspaceID.String())
	}
	if len(resolvedPreferences) != 1 || resolvedPreferences[0] != "github:"+connectionID.String() {
		t.Fatalf("preferences = %#v, want workspace-scoped GitHub preference", resolvedPreferences)
	}
	if provider.calls != 1 {
		t.Fatalf("GetCurrentWorkspace calls = %d, want 1", provider.calls)
	}
}

func TestAIChatWorkspaceScopePrefersExplicitWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.New()
	accountID := uuid.New()
	explicitWorkspaceID := uuid.New()
	fallbackWorkspaceID := uuid.New()
	organizationIDRaw := organizationID.String()
	explicitWorkspace := &model.Workspace{
		ID:             explicitWorkspaceID.String(),
		Name:           "Explicit",
		Plan:           "basic",
		Status:         model.WorkspaceStatusNormal,
		OrganizationID: &organizationIDRaw,
	}
	workspaceProvider := &fakeAIChatWorkspaceProvider{
		workspaces:  map[uuid.UUID]*model.Workspace{explicitWorkspaceID: explicitWorkspace},
		memberships: map[uuid.UUID]bool{explicitWorkspaceID: true},
	}
	provider := &fakeAIChatCurrentWorkspaceProvider{workspace: &model.Workspace{
		ID:             fallbackWorkspaceID.String(),
		OrganizationID: &organizationIDRaw,
		Status:         model.WorkspaceStatusNormal,
	}}

	var resolvedWorkspaceID string
	resolver := aichatWorkspaceScopeResolver{workspaces: workspaceProvider, accounts: provider}
	router := newAIChatWorkspaceScopeTestRouter(resolver, organizationID, accountID, explicitWorkspaceID.String(), func(c *gin.Context) {
		resolvedWorkspaceID = util.GetWorkspaceID(c)
		c.Status(http.StatusNoContent)
	})
	response := performAIChatWorkspaceScopeRequest(router)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if resolvedWorkspaceID != explicitWorkspaceID.String() {
		t.Fatalf("workspace scope = %q, want explicit %q", resolvedWorkspaceID, explicitWorkspaceID.String())
	}
	if provider.calls != 0 {
		t.Fatalf("GetCurrentWorkspace calls = %d, want 0 when explicit scope exists", provider.calls)
	}
}

func TestAIChatWorkspaceScopeRejectsInvalidWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.New()
	otherOrganizationID := uuid.New()
	accountID := uuid.New()

	tests := []struct {
		name              string
		explicitWorkspace *model.Workspace
		explicitMember    bool
		explicitRaw       string
		currentWorkspace  *model.Workspace
		currentError      error
	}{
		{
			name: "explicit workspace from another organization",
			explicitWorkspace: &model.Workspace{
				ID:             uuid.NewString(),
				Status:         model.WorkspaceStatusNormal,
				OrganizationID: stringPointer(otherOrganizationID.String()),
			},
		},
		{
			name: "explicit archived workspace",
			explicitWorkspace: &model.Workspace{
				ID:             uuid.NewString(),
				Status:         model.WorkspaceStatusArchived,
				OrganizationID: stringPointer(organizationID.String()),
			},
		},
		{
			name: "explicit workspace without membership",
			explicitWorkspace: &model.Workspace{
				ID:             uuid.NewString(),
				Status:         model.WorkspaceStatusNormal,
				OrganizationID: stringPointer(organizationID.String()),
			},
		},
		{
			name:        "malformed explicit workspace",
			explicitRaw: "not-a-workspace-id",
		},
		{
			name:        "missing explicit workspace",
			explicitRaw: uuid.NewString(),
		},
		{
			name: "current workspace from another organization",
			currentWorkspace: &model.Workspace{
				ID:             uuid.NewString(),
				Status:         model.WorkspaceStatusNormal,
				OrganizationID: stringPointer(otherOrganizationID.String()),
			},
		},
		{
			name: "current archived workspace",
			currentWorkspace: &model.Workspace{
				ID:             uuid.NewString(),
				Status:         model.WorkspaceStatusArchived,
				OrganizationID: stringPointer(organizationID.String()),
			},
		},
		{
			name: "current workspace without membership",
			currentWorkspace: &model.Workspace{
				ID:             uuid.NewString(),
				Status:         model.WorkspaceStatusNormal,
				OrganizationID: stringPointer(organizationID.String()),
			},
		},
		{
			name:         "deleted current workspace",
			currentError: gorm.ErrRecordNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			explicitRaw := test.explicitRaw
			workspaceProvider := &fakeAIChatWorkspaceProvider{
				workspaces:  make(map[uuid.UUID]*model.Workspace),
				memberships: make(map[uuid.UUID]bool),
			}
			if test.explicitWorkspace != nil {
				test.explicitWorkspace.Name = "Workspace"
				test.explicitWorkspace.Plan = "basic"
				explicitRaw = test.explicitWorkspace.ID
				workspaceID := uuid.MustParse(test.explicitWorkspace.ID)
				workspaceProvider.workspaces[workspaceID] = test.explicitWorkspace
				workspaceProvider.memberships[workspaceID] = test.explicitMember
			}
			provider := &fakeAIChatCurrentWorkspaceProvider{workspace: test.currentWorkspace, err: test.currentError}
			downstreamCalled := false
			resolver := aichatWorkspaceScopeResolver{workspaces: workspaceProvider, accounts: provider}
			router := newAIChatWorkspaceScopeTestRouter(resolver, organizationID, accountID, explicitRaw, func(c *gin.Context) {
				downstreamCalled = true
				c.Status(http.StatusNoContent)
			})

			response := performAIChatWorkspaceScopeRequest(router)
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusNotFound, response.Body.String())
			}
			if downstreamCalled {
				t.Fatal("downstream handler ran for invalid workspace scope")
			}
		})
	}
}

func TestAIChatWorkspaceScopePreservesOrganizationModeWithoutCurrentWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.New()
	accountID := uuid.New()
	provider := &fakeAIChatCurrentWorkspaceProvider{}
	resolvedWorkspaceID := "unexpected"
	resolver := aichatWorkspaceScopeResolver{accounts: provider}
	router := newAIChatWorkspaceScopeTestRouter(resolver, organizationID, accountID, "", func(c *gin.Context) {
		resolvedWorkspaceID = util.GetWorkspaceID(c)
		c.Status(http.StatusNoContent)
	})

	response := performAIChatWorkspaceScopeRequest(router)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if resolvedWorkspaceID != "" {
		t.Fatalf("workspace scope = %q, want empty organization-mode scope", resolvedWorkspaceID)
	}
	if provider.calls != 1 {
		t.Fatalf("GetCurrentWorkspace calls = %d, want 1", provider.calls)
	}
}

func TestAIChatWorkspaceScopeFailsClosedOnCurrentWorkspaceLookupError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &fakeAIChatCurrentWorkspaceProvider{err: errors.New("database unavailable")}
	downstreamCalled := false
	resolver := aichatWorkspaceScopeResolver{accounts: provider}
	router := newAIChatWorkspaceScopeTestRouter(resolver, uuid.New(), uuid.New(), "", func(c *gin.Context) {
		downstreamCalled = true
	})

	response := performAIChatWorkspaceScopeRequest(router)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusInternalServerError, response.Body.String())
	}
	if downstreamCalled {
		t.Fatal("downstream handler ran after current workspace lookup failure")
	}
}

func newAIChatWorkspaceScopeTestRouter(
	resolver aichatWorkspaceScopeResolver,
	organizationID uuid.UUID,
	accountID uuid.UUID,
	explicitWorkspaceID string,
	handler gin.HandlerFunc,
) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, organizationID.String())
		c.Set("account_id", accountID.String())
		if explicitWorkspaceID != "" {
			util.SetWorkspaceID(c, explicitWorkspaceID)
		}
		c.Next()
	})
	router.Use(aichatWorkspaceScopeMiddlewareWithResolver(resolver))
	router.GET("/scope", handler)
	return router
}

func performAIChatWorkspaceScopeRequest(router http.Handler) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/scope", nil)
	router.ServeHTTP(recorder, request)
	return recorder
}

func attachIntegrationWorkspaceScopeForTest(handler *integrationHandler, organizationID, accountID, workspaceID uuid.UUID) {
	organizationIDRaw := organizationID.String()
	handler.workspaceScopeResolver = &aichatWorkspaceScopeResolver{
		workspaces: &fakeAIChatWorkspaceProvider{
			workspaces: map[uuid.UUID]*model.Workspace{
				workspaceID: {
					ID:             workspaceID.String(),
					Status:         model.WorkspaceStatusNormal,
					OrganizationID: &organizationIDRaw,
				},
			},
			memberships: map[uuid.UUID]bool{workspaceID: true},
		},
		accounts: &fakeAIChatCurrentWorkspaceProvider{},
	}
}

func stringPointer(value string) *string {
	return &value
}
