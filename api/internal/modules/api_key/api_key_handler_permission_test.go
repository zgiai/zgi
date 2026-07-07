package APIKey

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestAPIKeyManagementRequiresAgentPermissionBeforeRequestHandling(t *testing.T) {
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	apiKeyID := "not-a-uuid"
	workspaceID := "22222222-2222-2222-2222-222222222222"
	tests := []struct {
		name           string
		method         string
		target         string
		body           string
		params         gin.Params
		call           func(*APIKeyHandler, *gin.Context)
		wantPermission workspace_model.WorkspacePermissionCode
	}{
		{
			name:           "list requires agent runtime access manage",
			method:         http.MethodGet,
			target:         "/agents/" + agentID.String() + "/api-keys",
			params:         gin.Params{{Key: "agent_id", Value: agentID.String()}},
			call:           (*APIKeyHandler).ListAPIKeys,
			wantPermission: workspace_model.WorkspacePermissionAgentRuntimeAccessManage,
		},
		{
			name:           "get requires agent runtime access manage before api key id validation",
			method:         http.MethodGet,
			target:         "/agents/" + agentID.String() + "/api-keys/" + apiKeyID,
			params:         gin.Params{{Key: "agent_id", Value: agentID.String()}, {Key: "api_key_id", Value: apiKeyID}},
			call:           (*APIKeyHandler).GetAPIKey,
			wantPermission: workspace_model.WorkspacePermissionAgentRuntimeAccessManage,
		},
		{
			name:           "usage logs require agent runtime access manage before api key id validation",
			method:         http.MethodGet,
			target:         "/agents/" + agentID.String() + "/api-keys/" + apiKeyID + "/usage",
			params:         gin.Params{{Key: "agent_id", Value: agentID.String()}, {Key: "api_key_id", Value: apiKeyID}},
			call:           (*APIKeyHandler).GetAPIKeyUsageLogs,
			wantPermission: workspace_model.WorkspacePermissionAgentRuntimeAccessManage,
		},
		{
			name:           "usage stats require agent runtime access manage before api key id validation",
			method:         http.MethodGet,
			target:         "/agents/" + agentID.String() + "/api-keys/" + apiKeyID + "/usage/stats",
			params:         gin.Params{{Key: "agent_id", Value: agentID.String()}, {Key: "api_key_id", Value: apiKeyID}},
			call:           (*APIKeyHandler).GetAPIKeyUsageStats,
			wantPermission: workspace_model.WorkspacePermissionAgentRuntimeAccessManage,
		},
		{
			name:           "create requires agent runtime access manage before body binding",
			method:         http.MethodPost,
			target:         "/agents/" + agentID.String() + "/api-keys",
			body:           `{"broken":`,
			params:         gin.Params{{Key: "agent_id", Value: agentID.String()}},
			call:           (*APIKeyHandler).CreateAPIKey,
			wantPermission: workspace_model.WorkspacePermissionAgentRuntimeAccessManage,
		},
		{
			name:           "update requires agent runtime access manage before body and api key id validation",
			method:         http.MethodPut,
			target:         "/agents/" + agentID.String() + "/api-keys/" + apiKeyID,
			body:           `{"broken":`,
			params:         gin.Params{{Key: "agent_id", Value: agentID.String()}, {Key: "api_key_id", Value: apiKeyID}},
			call:           (*APIKeyHandler).UpdateAPIKey,
			wantPermission: workspace_model.WorkspacePermissionAgentRuntimeAccessManage,
		},
		{
			name:           "delete requires agent runtime access manage before api key id validation",
			method:         http.MethodDelete,
			target:         "/agents/" + agentID.String() + "/api-keys/" + apiKeyID,
			params:         gin.Params{{Key: "agent_id", Value: agentID.String()}, {Key: "api_key_id", Value: apiKeyID}},
			call:           (*APIKeyHandler).DeleteAPIKey,
			wantPermission: workspace_model.WorkspacePermissionAgentRuntimeAccessManage,
		},
		{
			name:           "revoke requires agent runtime access manage before api key id validation",
			method:         http.MethodPost,
			target:         "/agents/" + agentID.String() + "/api-keys/" + apiKeyID + "/revoke",
			params:         gin.Params{{Key: "agent_id", Value: agentID.String()}, {Key: "api_key_id", Value: apiKeyID}},
			call:           (*APIKeyHandler).RevokeAPIKey,
			wantPermission: workspace_model.WorkspacePermissionAgentRuntimeAccessManage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &apiKeyPermissionRepository{}
			resolver := &apiKeyPermissionAgentWorkspaceResolver{
				workspaceID: workspaceID,
				agentType:   "AGENT",
			}
			permissionChecker := &apiKeyPermissionChecker{allowed: false}
			handler := &APIKeyHandler{
				apiKeyRepo:             repo,
				apiKeyUsageLogRepo:     &apiKeyPermissionUsageLogRepository{},
				organizationService:    permissionChecker,
				agentWorkspaceResolver: resolver,
			}
			ctx, recorder := newAPIKeyPermissionContext(tt.method, tt.target, tt.body, tt.params)

			tt.call(handler, ctx)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
			}
			requireAPIKeyResponseCode(t, recorder, response.ErrPermissionDenied)
			if resolver.lastAgentID != agentID {
				t.Fatalf("resolved agent id = %s, want %s", resolver.lastAgentID, agentID)
			}
			if permissionChecker.lastWorkspaceID != workspaceID {
				t.Fatalf("workspace checked = %q, want %q", permissionChecker.lastWorkspaceID, workspaceID)
			}
			if permissionChecker.lastPermission != tt.wantPermission {
				t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, tt.wantPermission)
			}
			if repo.calls != 0 {
				t.Fatalf("repository calls = %d, want 0", repo.calls)
			}
		})
	}
}

func TestAPIKeyManagementPermissionFollowsAgentType(t *testing.T) {
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	workspaceID := "22222222-2222-2222-2222-222222222222"
	tests := []struct {
		name           string
		agentType      string
		wantPermission workspace_model.WorkspacePermissionCode
	}{
		{
			name:           "agent runtime",
			agentType:      "AGENT",
			wantPermission: workspace_model.WorkspacePermissionAgentRuntimeAccessManage,
		},
		{
			name:           "workflow runtime",
			agentType:      "WORKFLOW",
			wantPermission: workspace_model.WorkspacePermissionWorkflowRuntimeAccessManage,
		},
		{
			name:           "conversational workflow runtime",
			agentType:      "CONVERSATIONAL_WORKFLOW",
			wantPermission: workspace_model.WorkspacePermissionWorkflowRuntimeAccessManage,
		},
		{
			name:           "frontend conversational agent alias",
			agentType:      "CONVERSATIONAL_AGENT",
			wantPermission: workspace_model.WorkspacePermissionWorkflowRuntimeAccessManage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &apiKeyPermissionRepository{}
			permissionChecker := &apiKeyPermissionChecker{allowed: false}
			handler := &APIKeyHandler{
				apiKeyRepo:          repo,
				organizationService: permissionChecker,
				agentWorkspaceResolver: &apiKeyPermissionAgentWorkspaceResolver{
					workspaceID: workspaceID,
					agentType:   tt.agentType,
				},
			}
			ctx, recorder := newAPIKeyPermissionContext(http.MethodGet, "/agents/"+agentID.String()+"/api-keys", "", gin.Params{
				{Key: "agent_id", Value: agentID.String()},
			})

			handler.ListAPIKeys(ctx)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
			}
			if permissionChecker.lastPermission != tt.wantPermission {
				t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, tt.wantPermission)
			}
			if repo.calls != 0 {
				t.Fatalf("repository calls = %d, want 0", repo.calls)
			}
		})
	}
}

func TestAPIKeyManagementWorkflowRoutesRequireWorkflowRuntimeAccess(t *testing.T) {
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	apiKeyID := "not-a-uuid"
	workspaceID := "22222222-2222-2222-2222-222222222222"
	tests := []struct {
		name   string
		method string
		target string
		body   string
		params gin.Params
		call   func(*APIKeyHandler, *gin.Context)
	}{
		{
			name:   "list",
			method: http.MethodGet,
			target: "/agents/" + agentID.String() + "/api-keys",
			params: gin.Params{{Key: "agent_id", Value: agentID.String()}},
			call:   (*APIKeyHandler).ListAPIKeys,
		},
		{
			name:   "get",
			method: http.MethodGet,
			target: "/agents/" + agentID.String() + "/api-keys/" + apiKeyID,
			params: gin.Params{{Key: "agent_id", Value: agentID.String()}, {Key: "api_key_id", Value: apiKeyID}},
			call:   (*APIKeyHandler).GetAPIKey,
		},
		{
			name:   "usage logs",
			method: http.MethodGet,
			target: "/agents/" + agentID.String() + "/api-keys/" + apiKeyID + "/usage",
			params: gin.Params{{Key: "agent_id", Value: agentID.String()}, {Key: "api_key_id", Value: apiKeyID}},
			call:   (*APIKeyHandler).GetAPIKeyUsageLogs,
		},
		{
			name:   "usage stats",
			method: http.MethodGet,
			target: "/agents/" + agentID.String() + "/api-keys/" + apiKeyID + "/usage/stats",
			params: gin.Params{{Key: "agent_id", Value: agentID.String()}, {Key: "api_key_id", Value: apiKeyID}},
			call:   (*APIKeyHandler).GetAPIKeyUsageStats,
		},
		{
			name:   "create",
			method: http.MethodPost,
			target: "/agents/" + agentID.String() + "/api-keys",
			body:   `{"broken":`,
			params: gin.Params{{Key: "agent_id", Value: agentID.String()}},
			call:   (*APIKeyHandler).CreateAPIKey,
		},
		{
			name:   "update",
			method: http.MethodPut,
			target: "/agents/" + agentID.String() + "/api-keys/" + apiKeyID,
			body:   `{"broken":`,
			params: gin.Params{{Key: "agent_id", Value: agentID.String()}, {Key: "api_key_id", Value: apiKeyID}},
			call:   (*APIKeyHandler).UpdateAPIKey,
		},
		{
			name:   "delete",
			method: http.MethodDelete,
			target: "/agents/" + agentID.String() + "/api-keys/" + apiKeyID,
			params: gin.Params{{Key: "agent_id", Value: agentID.String()}, {Key: "api_key_id", Value: apiKeyID}},
			call:   (*APIKeyHandler).DeleteAPIKey,
		},
		{
			name:   "revoke",
			method: http.MethodPost,
			target: "/agents/" + agentID.String() + "/api-keys/" + apiKeyID + "/revoke",
			params: gin.Params{{Key: "agent_id", Value: agentID.String()}, {Key: "api_key_id", Value: apiKeyID}},
			call:   (*APIKeyHandler).RevokeAPIKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &apiKeyPermissionRepository{}
			permissionChecker := &apiKeyPermissionChecker{allowed: false}
			handler := &APIKeyHandler{
				apiKeyRepo:             repo,
				apiKeyUsageLogRepo:     &apiKeyPermissionUsageLogRepository{},
				organizationService:    permissionChecker,
				agentWorkspaceResolver: &apiKeyPermissionAgentWorkspaceResolver{workspaceID: workspaceID, agentType: "WORKFLOW"},
			}
			ctx, recorder := newAPIKeyPermissionContext(tt.method, tt.target, tt.body, tt.params)

			tt.call(handler, ctx)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
			}
			requireAPIKeyResponseCode(t, recorder, response.ErrPermissionDenied)
			if permissionChecker.lastPermission != workspace_model.WorkspacePermissionWorkflowRuntimeAccessManage {
				t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspace_model.WorkspacePermissionWorkflowRuntimeAccessManage)
			}
			if repo.calls != 0 {
				t.Fatalf("repository calls = %d, want 0", repo.calls)
			}
		})
	}
}

func TestAPIKeyManagementRequiresRouteAgentWorkspace(t *testing.T) {
	agentID := "11111111-1111-1111-1111-111111111111"
	repo := &apiKeyPermissionRepository{}
	permissionChecker := &apiKeyPermissionChecker{allowed: true}
	handler := &APIKeyHandler{
		apiKeyRepo:             repo,
		organizationService:    permissionChecker,
		agentWorkspaceResolver: &apiKeyPermissionAgentWorkspaceResolver{},
	}
	ctx, recorder := newAPIKeyPermissionContext(http.MethodGet, "/agents/"+agentID+"/api-keys", "", gin.Params{
		{Key: "agent_id", Value: agentID},
	})

	handler.ListAPIKeys(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	requireAPIKeyResponseCode(t, recorder, response.ErrPermissionDenied)
	if permissionChecker.checked {
		t.Fatalf("permission check should not run without a resolved agent workspace")
	}
	if repo.calls != 0 {
		t.Fatalf("repository calls = %d, want 0", repo.calls)
	}
}

func TestAPIKeyManagementUsesResolvedAgentWorkspaceForRepositoryScope(t *testing.T) {
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	organizationID := "33333333-3333-3333-3333-333333333333"
	repo := &apiKeyPermissionRepository{}
	handler := &APIKeyHandler{
		apiKeyRepo:             repo,
		organizationService:    &apiKeyPermissionChecker{allowed: true},
		agentWorkspaceResolver: &apiKeyPermissionAgentWorkspaceResolver{workspaceID: workspaceID.String()},
	}
	ctx, recorder := newAPIKeyPermissionContext(http.MethodGet, "/agents/"+agentID.String()+"/api-keys", "", gin.Params{
		{Key: "agent_id", Value: agentID.String()},
	})
	util.SetOrganizationScopeCompat(ctx, organizationID)

	handler.ListAPIKeys(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if repo.lastAgentID != agentID {
		t.Fatalf("repo agent id = %s, want %s", repo.lastAgentID, agentID)
	}
	if repo.lastTenantID != workspaceID {
		t.Fatalf("repo tenant id = %s, want resolved workspace %s", repo.lastTenantID, workspaceID)
	}
}

func newAPIKeyPermissionContext(method, target, body string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	if body == "" {
		body = "{}"
	}
	ctx.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = params
	ctx.Set("account_id", "account-1")
	util.SetOrganizationScopeCompat(ctx, "org-1")
	return ctx, recorder
}

func requireAPIKeyResponseCode(t *testing.T, recorder *httptest.ResponseRecorder, want response.ErrorCode) {
	t.Helper()
	var body response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v; body=%s", err, recorder.Body.String())
	}
	if body.Code != strconv.Itoa(want.Code) {
		t.Fatalf("response code = %s, want %d; body=%s", body.Code, want.Code, recorder.Body.String())
	}
}

type apiKeyPermissionAgentWorkspaceResolver struct {
	workspaceID string
	agentType   string
	lastAgentID uuid.UUID
}

func (r *apiKeyPermissionAgentWorkspaceResolver) ResolveAgentScope(_ context.Context, _ string, agentID uuid.UUID) (apiKeyAgentScope, error) {
	r.lastAgentID = agentID
	return apiKeyAgentScope{
		WorkspaceID: r.workspaceID,
		AgentType:   r.agentType,
	}, nil
}

type apiKeyPermissionChecker struct {
	allowed            bool
	checked            bool
	lastWorkspaceID    string
	lastPermission     workspace_model.WorkspacePermissionCode
	lastOrganizationID string
	lastAccountID      string
}

func (c *apiKeyPermissionChecker) CheckWorkspacePermission(_ context.Context, organizationID, workspaceID, accountID string, permissionCode workspace_model.WorkspacePermissionCode) (bool, error) {
	c.checked = true
	c.lastOrganizationID = organizationID
	c.lastWorkspaceID = workspaceID
	c.lastAccountID = accountID
	c.lastPermission = permissionCode
	return c.allowed, nil
}

type apiKeyPermissionRepository struct {
	calls        int
	lastAgentID  uuid.UUID
	lastTenantID uuid.UUID
}

func (r *apiKeyPermissionRepository) Create(context.Context, *APIKey) error {
	r.calls++
	return nil
}

func (r *apiKeyPermissionRepository) GetByID(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*APIKey, error) {
	r.calls++
	return nil, nil
}

func (r *apiKeyPermissionRepository) GetByKeyHash(context.Context, string) (*APIKey, error) {
	r.calls++
	return nil, nil
}

func (r *apiKeyPermissionRepository) List(_ context.Context, agentID, tenantID uuid.UUID) ([]*APIKey, error) {
	r.calls++
	r.lastAgentID = agentID
	r.lastTenantID = tenantID
	return []*APIKey{}, nil
}

func (r *apiKeyPermissionRepository) Update(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, map[string]interface{}) error {
	r.calls++
	return nil
}

func (r *apiKeyPermissionRepository) Delete(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	r.calls++
	return nil
}

func (r *apiKeyPermissionRepository) CountByAgent(context.Context, uuid.UUID, uuid.UUID) (int64, error) {
	r.calls++
	return 0, nil
}

func (r *apiKeyPermissionRepository) CheckNameExists(context.Context, string, uuid.UUID, uuid.UUID, *uuid.UUID) (bool, error) {
	r.calls++
	return false, nil
}

type apiKeyPermissionUsageLogRepository struct {
	calls int
}

func (r *apiKeyPermissionUsageLogRepository) Create(context.Context, *APIKeyUsageLog) error {
	r.calls++
	return nil
}

func (r *apiKeyPermissionUsageLogRepository) GetByAPIKeyID(context.Context, uuid.UUID, int, int, *time.Time, *time.Time) ([]*APIKeyUsageLog, int64, error) {
	r.calls++
	return nil, 0, nil
}

func (r *apiKeyPermissionUsageLogRepository) GetByAgentID(context.Context, uuid.UUID, int, int, *time.Time, *time.Time) ([]*APIKeyUsageLog, int64, error) {
	r.calls++
	return nil, 0, nil
}

func (r *apiKeyPermissionUsageLogRepository) GetByID(context.Context, uuid.UUID) (*APIKeyUsageLog, error) {
	r.calls++
	return nil, nil
}

func (r *apiKeyPermissionUsageLogRepository) GetTotalTokensUsed(context.Context, uuid.UUID, *time.Time, *time.Time) (int64, error) {
	r.calls++
	return 0, nil
}

func (r *apiKeyPermissionUsageLogRepository) GetUsageStats(context.Context, uuid.UUID, *time.Time, *time.Time) (map[string]interface{}, error) {
	r.calls++
	return map[string]interface{}{}, nil
}
