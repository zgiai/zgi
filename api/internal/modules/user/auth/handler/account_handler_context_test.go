package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
)

func TestAccountHandlerUpdateAccountContextOrganizationModeClearsWorkspace(t *testing.T) {
	organizationID := "org-1"
	service := &accountContextHandlerAccountService{
		updateResult: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &organizationID,
			CreatedAt:             time.Now(),
			UpdatedAt:             time.Now(),
		},
	}
	handler := NewAccountHandler(service, nil)

	body, err := json.Marshal(map[string]string{
		"mode":                    "organization",
		"current_organization_id": organizationID,
	})
	require.NoError(t, err)

	c, recorder := newAccountContextHandlerTestContext(http.MethodPut, "/account/context", body)
	c.Set("account_id", "acc-1")

	handler.UpdateAccountContext(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, service.updateCalled)
	require.NotNil(t, service.organizationIDArg)
	require.Equal(t, organizationID, *service.organizationIDArg)
	require.NotNil(t, service.workspaceIDArg)
	require.Empty(t, *service.workspaceIDArg)

	var payload struct {
		Code string `json:"code"`
		Data struct {
			AccountID             string  `json:"account_id"`
			Mode                  string  `json:"mode"`
			CurrentOrganizationID *string `json:"current_organization_id"`
			CurrentWorkspaceID    *string `json:"current_workspace_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, "0", payload.Code)
	require.Equal(t, "acc-1", payload.Data.AccountID)
	require.Equal(t, "organization", payload.Data.Mode)
	require.NotNil(t, payload.Data.CurrentOrganizationID)
	require.Equal(t, organizationID, *payload.Data.CurrentOrganizationID)
	require.Nil(t, payload.Data.CurrentWorkspaceID)
}

func TestAccountHandlerUpdateAccountContextWorkspaceModeRequiresWorkspace(t *testing.T) {
	service := &accountContextHandlerAccountService{}
	handler := NewAccountHandler(service, nil)

	body, err := json.Marshal(map[string]string{
		"mode": "workspace",
	})
	require.NoError(t, err)

	c, recorder := newAccountContextHandlerTestContext(http.MethodPut, "/account/context", body)
	c.Set("account_id", "acc-1")

	handler.UpdateAccountContext(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.False(t, service.updateCalled)
}

func TestAccountHandlerGetAccountContextReturnsMode(t *testing.T) {
	workspaceID := "ws-1"
	service := &accountContextHandlerAccountService{
		getResult: &auth_model.AccountContext{
			AccountID:          "acc-1",
			CurrentWorkspaceID: &workspaceID,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		},
	}
	handler := NewAccountHandler(service, nil)

	c, recorder := newAccountContextHandlerTestContext(http.MethodGet, "/account/context", nil)
	c.Set("account_id", "acc-1")

	handler.GetAccountContext(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var payload struct {
		Code string `json:"code"`
		Data struct {
			Mode               string  `json:"mode"`
			CurrentWorkspaceID *string `json:"current_workspace_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, "0", payload.Code)
	require.Equal(t, "workspace", payload.Data.Mode)
	require.NotNil(t, payload.Data.CurrentWorkspaceID)
	require.Equal(t, workspaceID, *payload.Data.CurrentWorkspaceID)
}

func TestAccountHandlerGetAccountContextReturnsOrganizationModeWithoutWorkspace(t *testing.T) {
	organizationID := "org-1"
	service := &accountContextHandlerAccountService{
		getResult: &auth_model.AccountContext{
			AccountID:             "acc-1",
			CurrentOrganizationID: &organizationID,
			CreatedAt:             time.Now(),
			UpdatedAt:             time.Now(),
		},
	}
	handler := NewAccountHandler(service, nil)

	c, recorder := newAccountContextHandlerTestContext(http.MethodGet, "/account/context", nil)
	c.Set("account_id", "acc-1")

	handler.GetAccountContext(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var payload struct {
		Code string `json:"code"`
		Data struct {
			Mode                  string  `json:"mode"`
			CurrentOrganizationID *string `json:"current_organization_id"`
			CurrentWorkspaceID    *string `json:"current_workspace_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, "0", payload.Code)
	require.Equal(t, "organization", payload.Data.Mode)
	require.NotNil(t, payload.Data.CurrentOrganizationID)
	require.Equal(t, organizationID, *payload.Data.CurrentOrganizationID)
	require.Nil(t, payload.Data.CurrentWorkspaceID)
}

func TestAccountHandlerGetAccountCapabilitiesReturnsContract(t *testing.T) {
	organizationID := "org-1"
	service := &accountContextHandlerAccountService{
		capabilitiesResult: &shared_dto.AccountCapabilitiesResponse{
			AccountID: "acc-1",
			Context: shared_dto.AccountCapabilityContext{
				Mode:                  "organization",
				CurrentOrganizationID: &organizationID,
			},
			Organization: shared_dto.AccountOrganizationCapabilities{
				ID:                   &organizationID,
				Role:                 "normal",
				IsMember:             true,
				CanAccessDashboard:   false,
				CanManageModelConfig: false,
				ProductSurfaces: shared_dto.AccountProductSurfaceCapabilities{
					Chat:     true,
					Image:    true,
					App:      true,
					Settings: true,
				},
			},
			Workspace: shared_dto.AccountWorkspaceCapabilities{
				RequiresWorkspace: true,
				Permissions:       []string{},
			},
			Routes: shared_dto.AccountRouteCapabilities{
				OrganizationScopeAllowed: true,
				WorkspaceScopeAllowed:    false,
				WorkspaceRequired:        true,
			},
			RuntimeAudience: shared_dto.AccountRuntimeAudienceCapability{
				AccountID:      "acc-1",
				OrganizationID: &organizationID,
				SubjectTypes:   []string{"organization", "account"},
				DepartmentIDs:  []string{},
				WorkspaceIDs:   []string{},
			},
			RuntimeSurfaces: map[string]shared_dto.AccountRuntimeSurfaceCapability{
				"webapp": {
					Enabled:           true,
					Mode:              "published_resource",
					GrantSubjectTypes: []string{"public", "organization", "department", "workspace", "account"},
				},
				"api": {
					Enabled:           true,
					Mode:              "api_key",
					GrantSubjectTypes: []string{"public"},
				},
				"app_center": {
					Enabled:           true,
					Mode:              "runtime_grant",
					GrantSubjectTypes: []string{"organization", "department", "workspace", "account"},
				},
				"builtin_app": {
					Enabled:           true,
					Mode:              "runtime_grant",
					GrantSubjectTypes: []string{"organization", "department", "workspace", "account"},
				},
				"internal": {
					Enabled:           true,
					Mode:              "internal_runtime",
					GrantSubjectTypes: []string{"internal"},
				},
			},
			RuntimeResourceLists: map[string]shared_dto.AccountRuntimeResourceListCapability{
				"app_center": {
					Enabled:      true,
					ResourceType: "agent",
					Surface:      "app_center",
					Mode:         "runtimeauth_candidate_filter",
					Endpoint:     "/console/api/agents/runnable-webapps",
				},
				"built_in_workflows": {
					Enabled:      true,
					ResourceType: "builtin_workflow",
					Surface:      "builtin_app",
					Mode:         "runtimeauth_candidate_filter",
					Endpoint:     "/console/api/built-in-workflows",
				},
			},
		},
	}
	handler := NewAccountHandler(service, nil)

	c, recorder := newAccountContextHandlerTestContext(http.MethodGet, "/account/capabilities", nil)
	c.Set("account_id", "acc-1")

	handler.GetAccountCapabilities(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var payload struct {
		Code string `json:"code"`
		Data struct {
			AccountID string `json:"account_id"`
			Context   struct {
				Mode                  string  `json:"mode"`
				CurrentOrganizationID *string `json:"current_organization_id"`
				CurrentWorkspaceID    *string `json:"current_workspace_id"`
			} `json:"context"`
			Organization struct {
				ID                   *string `json:"id"`
				Role                 string  `json:"role"`
				IsMember             bool    `json:"is_member"`
				CanAccessDashboard   bool    `json:"can_access_dashboard"`
				CanManageModelConfig bool    `json:"can_manage_model_config"`
				ProductSurfaces      struct {
					Chat     bool `json:"chat"`
					Image    bool `json:"image"`
					App      bool `json:"app"`
					Settings bool `json:"settings"`
				} `json:"product_surfaces"`
			} `json:"organization"`
			Workspace struct {
				ID                *string  `json:"id"`
				Available         bool     `json:"available"`
				RequiresWorkspace bool     `json:"requires_workspace"`
				CanView           bool     `json:"can_view"`
				Permissions       []string `json:"permissions"`
			} `json:"workspace"`
			Routes struct {
				OrganizationScopeAllowed bool `json:"organization_scope_allowed"`
				WorkspaceScopeAllowed    bool `json:"workspace_scope_allowed"`
				WorkspaceRequired        bool `json:"workspace_required"`
			} `json:"routes"`
			RuntimeAudience struct {
				AccountID      string   `json:"account_id"`
				OrganizationID *string  `json:"organization_id"`
				SubjectTypes   []string `json:"subject_types"`
				DepartmentIDs  []string `json:"department_ids"`
				WorkspaceIDs   []string `json:"workspace_ids"`
			} `json:"runtime_audience"`
			RuntimeSurfaces map[string]struct {
				Enabled           bool     `json:"enabled"`
				Mode              string   `json:"mode"`
				GrantSubjectTypes []string `json:"grant_subject_types"`
			} `json:"runtime_surfaces"`
			RuntimeResourceLists map[string]struct {
				Enabled      bool   `json:"enabled"`
				ResourceType string `json:"resource_type"`
				Surface      string `json:"surface"`
				Mode         string `json:"mode"`
				Endpoint     string `json:"endpoint"`
			} `json:"runtime_resource_lists"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, "0", payload.Code)
	require.Equal(t, "acc-1", payload.Data.AccountID)
	require.Equal(t, "organization", payload.Data.Context.Mode)
	require.NotNil(t, payload.Data.Context.CurrentOrganizationID)
	require.Equal(t, organizationID, *payload.Data.Context.CurrentOrganizationID)
	require.Nil(t, payload.Data.Context.CurrentWorkspaceID)
	require.NotNil(t, payload.Data.Organization.ID)
	require.Equal(t, organizationID, *payload.Data.Organization.ID)
	require.Equal(t, "normal", payload.Data.Organization.Role)
	require.True(t, payload.Data.Organization.IsMember)
	require.False(t, payload.Data.Organization.CanAccessDashboard)
	require.False(t, payload.Data.Organization.CanManageModelConfig)
	require.True(t, payload.Data.Organization.ProductSurfaces.Chat)
	require.True(t, payload.Data.Organization.ProductSurfaces.Image)
	require.True(t, payload.Data.Organization.ProductSurfaces.App)
	require.True(t, payload.Data.Organization.ProductSurfaces.Settings)
	require.Nil(t, payload.Data.Workspace.ID)
	require.False(t, payload.Data.Workspace.Available)
	require.True(t, payload.Data.Workspace.RequiresWorkspace)
	require.False(t, payload.Data.Workspace.CanView)
	require.Empty(t, payload.Data.Workspace.Permissions)
	require.True(t, payload.Data.Routes.OrganizationScopeAllowed)
	require.False(t, payload.Data.Routes.WorkspaceScopeAllowed)
	require.True(t, payload.Data.Routes.WorkspaceRequired)
	require.Equal(t, "acc-1", payload.Data.RuntimeAudience.AccountID)
	require.NotNil(t, payload.Data.RuntimeAudience.OrganizationID)
	require.Equal(t, organizationID, *payload.Data.RuntimeAudience.OrganizationID)
	require.ElementsMatch(t, []string{"organization", "account"}, payload.Data.RuntimeAudience.SubjectTypes)
	require.Empty(t, payload.Data.RuntimeAudience.DepartmentIDs)
	require.Empty(t, payload.Data.RuntimeAudience.WorkspaceIDs)
	require.Len(t, payload.Data.RuntimeSurfaces, 5)
	require.True(t, payload.Data.RuntimeSurfaces["webapp"].Enabled)
	require.Equal(t, "published_resource", payload.Data.RuntimeSurfaces["webapp"].Mode)
	require.ElementsMatch(t, []string{"public", "organization", "department", "workspace", "account"}, payload.Data.RuntimeSurfaces["webapp"].GrantSubjectTypes)
	require.True(t, payload.Data.RuntimeSurfaces["api"].Enabled)
	require.Equal(t, "api_key", payload.Data.RuntimeSurfaces["api"].Mode)
	require.Equal(t, []string{"public"}, payload.Data.RuntimeSurfaces["api"].GrantSubjectTypes)
	require.True(t, payload.Data.RuntimeSurfaces["app_center"].Enabled)
	require.Equal(t, "runtime_grant", payload.Data.RuntimeSurfaces["app_center"].Mode)
	require.ElementsMatch(t, []string{"organization", "department", "workspace", "account"}, payload.Data.RuntimeSurfaces["app_center"].GrantSubjectTypes)
	require.True(t, payload.Data.RuntimeSurfaces["builtin_app"].Enabled)
	require.Equal(t, "runtime_grant", payload.Data.RuntimeSurfaces["builtin_app"].Mode)
	require.ElementsMatch(t, []string{"organization", "department", "workspace", "account"}, payload.Data.RuntimeSurfaces["builtin_app"].GrantSubjectTypes)
	require.True(t, payload.Data.RuntimeSurfaces["internal"].Enabled)
	require.Equal(t, "internal_runtime", payload.Data.RuntimeSurfaces["internal"].Mode)
	require.Equal(t, []string{"internal"}, payload.Data.RuntimeSurfaces["internal"].GrantSubjectTypes)
	require.Len(t, payload.Data.RuntimeResourceLists, 2)
	require.True(t, payload.Data.RuntimeResourceLists["app_center"].Enabled)
	require.Equal(t, "agent", payload.Data.RuntimeResourceLists["app_center"].ResourceType)
	require.Equal(t, "app_center", payload.Data.RuntimeResourceLists["app_center"].Surface)
	require.Equal(t, "runtimeauth_candidate_filter", payload.Data.RuntimeResourceLists["app_center"].Mode)
	require.Equal(t, "/console/api/agents/runnable-webapps", payload.Data.RuntimeResourceLists["app_center"].Endpoint)
	require.True(t, payload.Data.RuntimeResourceLists["built_in_workflows"].Enabled)
	require.Equal(t, "builtin_workflow", payload.Data.RuntimeResourceLists["built_in_workflows"].ResourceType)
	require.Equal(t, "builtin_app", payload.Data.RuntimeResourceLists["built_in_workflows"].Surface)
	require.Equal(t, "runtimeauth_candidate_filter", payload.Data.RuntimeResourceLists["built_in_workflows"].Mode)
	require.Equal(t, "/console/api/built-in-workflows", payload.Data.RuntimeResourceLists["built_in_workflows"].Endpoint)
}

type accountContextHandlerAccountService struct {
	interfaces.AccountService
	getResult          *auth_model.AccountContext
	updateResult       *auth_model.AccountContext
	capabilitiesResult *shared_dto.AccountCapabilitiesResponse
	updateCalled       bool
	organizationIDArg  *string
	workspaceIDArg     *string
}

func (f *accountContextHandlerAccountService) GetAccountContext(ctx context.Context, accountID string) (*auth_model.AccountContext, error) {
	return f.getResult, nil
}

func (f *accountContextHandlerAccountService) GetAccountCapabilities(ctx context.Context, accountID string) (*shared_dto.AccountCapabilitiesResponse, error) {
	return f.capabilitiesResult, nil
}

func (f *accountContextHandlerAccountService) UpdateAccountContext(ctx context.Context, accountID string, organizationID, workspaceID *string) (*auth_model.AccountContext, error) {
	f.updateCalled = true
	f.organizationIDArg = copyAccountContextStringPtr(organizationID)
	f.workspaceIDArg = copyAccountContextStringPtr(workspaceID)
	return f.updateResult, nil
}

func copyAccountContextStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func newAccountContextHandlerTestContext(method, target string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(recorder)
	c.Request = request

	return c, recorder
}
