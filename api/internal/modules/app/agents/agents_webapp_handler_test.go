package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	file_model "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
)

func TestAgentsHandler_UpdateWebAppStatus_PassesContextAndRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &stubWebAppStatusHandlerService{
		resp: &dto.WebAppStatusResponse{
			AgentID:      "11111111-1111-1111-1111-111111111111",
			WebAppID:     "22222222-2222-2222-2222-222222222222",
			WebAppStatus: "inactive",
			UpdatedAt:    1234567890,
		},
	}
	router := newWebAppStatusHandlerTestRouter(service)

	req := httptest.NewRequest(http.MethodPatch, "/agents/11111111-1111-1111-1111-111111111111/webapp/status", bytes.NewBufferString(`{"status":"inactive","reason":"maintenance"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, service.called)
	require.Equal(t, "11111111-1111-1111-1111-111111111111", service.agentID)
	require.Equal(t, "inactive", service.req.Status)
	require.Equal(t, "maintenance", service.req.Reason)
	require.Equal(t, "99999999-9999-9999-9999-999999999999", service.accountID)
	require.Equal(t, "88888888-8888-8888-8888-888888888888", service.organizationID)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "0", body["code"])
	data, ok := body["data"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "inactive", data["web_app_status"])
}

func TestAgentsHandler_UpdateWebAppStatus_MapsInvalidStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &stubWebAppStatusHandlerService{err: errInvalidWebAppStatus}
	router := newWebAppStatusHandlerTestRouter(service)

	req := httptest.NewRequest(http.MethodPatch, "/agents/11111111-1111-1111-1111-111111111111/webapp/status", bytes.NewBufferString(`{"status":"archived"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.True(t, service.called)
}

func TestAgentsHandler_GetAgentRuntimeSurfaces_PassesContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &stubWebAppStatusHandlerService{
		runtimeSurfacesResp: &dto.AgentRuntimeSurfaceAuthorizationResponse{
			AgentID:        "11111111-1111-1111-1111-111111111111",
			WorkspaceID:    "22222222-2222-2222-2222-222222222222",
			OrganizationID: "88888888-8888-8888-8888-888888888888",
			Surfaces: []dto.AgentRuntimeSurfaceAuthorization{{
				Surface:             "webapp",
				Enabled:             true,
				CompatibilitySource: "legacy_agent_fields",
			}},
		},
	}
	handler := NewAgentsHandler(service, nil, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", "99999999-9999-9999-9999-999999999999")
		util.SetOrganizationID(c, "88888888-8888-8888-8888-888888888888")
		c.Next()
	})
	router.GET("/agents/:agent_id/runtime-surfaces", handler.GetAgentRuntimeSurfaces)

	req := httptest.NewRequest(http.MethodGet, "/agents/11111111-1111-1111-1111-111111111111/runtime-surfaces", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, service.runtimeSurfacesCalled)
	require.Equal(t, "11111111-1111-1111-1111-111111111111", service.runtimeSurfacesAgentID)
	require.Equal(t, "99999999-9999-9999-9999-999999999999", service.runtimeSurfacesAccountID)
	require.Equal(t, "88888888-8888-8888-8888-888888888888", service.runtimeSurfacesOrganizationID)
}

func TestAgentsHandler_UpdateAgentRuntimeSurfaces_PassesContextAndRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &stubWebAppStatusHandlerService{
		runtimeSurfacesResp: &dto.AgentRuntimeSurfaceAuthorizationResponse{
			AgentID:        "11111111-1111-1111-1111-111111111111",
			WorkspaceID:    "22222222-2222-2222-2222-222222222222",
			OrganizationID: "88888888-8888-8888-8888-888888888888",
			Surfaces: []dto.AgentRuntimeSurfaceAuthorization{{
				Surface: "api",
				Enabled: false,
			}},
		},
	}
	handler := NewAgentsHandler(service, nil, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", "99999999-9999-9999-9999-999999999999")
		util.SetOrganizationID(c, "88888888-8888-8888-8888-888888888888")
		c.Next()
	})
	router.PATCH("/agents/:agent_id/runtime-surfaces", handler.UpdateAgentRuntimeSurfaces)

	req := httptest.NewRequest(http.MethodPatch, "/agents/11111111-1111-1111-1111-111111111111/runtime-surfaces", bytes.NewBufferString(`{"surfaces":[{"surface":"api","enabled":false}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, service.updateRuntimeSurfacesCalled)
	require.Equal(t, "11111111-1111-1111-1111-111111111111", service.updateRuntimeSurfacesAgentID)
	require.Equal(t, "99999999-9999-9999-9999-999999999999", service.updateRuntimeSurfacesAccountID)
	require.Equal(t, "88888888-8888-8888-8888-888888888888", service.updateRuntimeSurfacesOrganizationID)
	require.Len(t, service.updateRuntimeSurfacesReq.Surfaces, 1)
	require.Equal(t, "api", service.updateRuntimeSurfacesReq.Surfaces[0].Surface)
	require.False(t, service.updateRuntimeSurfacesReq.Surfaces[0].Enabled)
}

func TestAgentsHandlerMutationsRequireManageBeforeBindingRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name           string
		method         string
		path           string
		body           string
		register       func(*gin.Engine, *AgentsHandler)
		mutationCalled func(*stubWebAppStatusHandlerService) bool
	}{
		{
			name:   "runtime surfaces",
			method: http.MethodPatch,
			path:   "/agents/11111111-1111-1111-1111-111111111111/runtime-surfaces",
			body:   `{"surfaces":`,
			register: func(router *gin.Engine, handler *AgentsHandler) {
				router.PATCH("/agents/:agent_id/runtime-surfaces", handler.UpdateAgentRuntimeSurfaces)
			},
			mutationCalled: func(service *stubWebAppStatusHandlerService) bool {
				return service.updateRuntimeSurfacesCalled
			},
		},
		{
			name:   "config",
			method: http.MethodPut,
			path:   "/agents/11111111-1111-1111-1111-111111111111/config",
			body:   `{"system_prompt":`,
			register: func(router *gin.Engine, handler *AgentsHandler) {
				router.PUT("/agents/:agent_id/config", handler.UpdateAgentConfig)
			},
			mutationCalled: func(service *stubWebAppStatusHandlerService) bool {
				return service.updateAgentConfigCalled
			},
		},
		{
			name:   "memory slots",
			method: http.MethodPut,
			path:   "/agents/11111111-1111-1111-1111-111111111111/memory/slots",
			body:   `{"slots":`,
			register: func(router *gin.Engine, handler *AgentsHandler) {
				router.PUT("/agents/:agent_id/memory/slots", handler.ReplaceAgentMemorySlots)
			},
			mutationCalled: func(service *stubWebAppStatusHandlerService) bool {
				return service.replaceMemorySlotsCalled
			},
		},
		{
			name:   "memory values",
			method: http.MethodPut,
			path:   "/agents/11111111-1111-1111-1111-111111111111/memory/values",
			body:   `{"key":`,
			register: func(router *gin.Engine, handler *AgentsHandler) {
				router.PUT("/agents/:agent_id/memory/values", handler.UpdateAgentMemoryValue)
			},
			mutationCalled: func(service *stubWebAppStatusHandlerService) bool {
				return service.updateMemoryValueCalled
			},
		},
		{
			name:   "webapp status",
			method: http.MethodPatch,
			path:   "/agents/11111111-1111-1111-1111-111111111111/webapp/status",
			body:   `{"status":`,
			register: func(router *gin.Engine, handler *AgentsHandler) {
				router.PATCH("/agents/:agent_id/webapp/status", handler.UpdateWebAppStatus)
			},
			mutationCalled: func(service *stubWebAppStatusHandlerService) bool {
				return service.called
			},
		},
		{
			name:   "suggested questions",
			method: http.MethodPost,
			path:   "/agents/11111111-1111-1111-1111-111111111111/suggested-questions/generate",
			body:   `{"count":`,
			register: func(router *gin.Engine, handler *AgentsHandler) {
				router.POST("/agents/:agent_id/suggested-questions/generate", handler.GenerateAgentSuggestedQuestions)
			},
			mutationCalled: func(service *stubWebAppStatusHandlerService) bool {
				return service.generateSuggestedQuestionsCalled
			},
		},
		{
			name:   "published version rollback",
			method: http.MethodPost,
			path:   "/agents/11111111-1111-1111-1111-111111111111/published-versions/rollback",
			body:   `{"version_id":`,
			register: func(router *gin.Engine, handler *AgentsHandler) {
				router.POST("/agents/:agent_id/published-versions/rollback", handler.RollbackAgentPublishedVersion)
			},
			mutationCalled: func(service *stubWebAppStatusHandlerService) bool {
				return service.rollbackAgentPublishedVersionCalled
			},
		},
		{
			name:   "legacy update",
			method: http.MethodPut,
			path:   "/agents/11111111-1111-1111-1111-111111111111",
			body:   `{"name":`,
			register: func(router *gin.Engine, handler *AgentsHandler) {
				router.PUT("/agents/:agent_id", handler.UpdateAgent)
			},
			mutationCalled: func(service *stubWebAppStatusHandlerService) bool {
				return service.updateAgentCalled
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := &stubWebAppStatusHandlerService{requireManageAccessErr: runtimeservice.ErrPermissionDenied}
			handler := NewAgentsHandler(service, nil, nil, nil, nil)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("account_id", "99999999-9999-9999-9999-999999999999")
				util.SetOrganizationID(c, "88888888-8888-8888-8888-888888888888")
				c.Next()
			})
			tc.register(router, handler)

			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			var body map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, "403001", body["code"])
			require.True(t, service.requireManageAccessCalled)
			require.Equal(t, "11111111-1111-1111-1111-111111111111", service.requireManageAccessAgentID)
			require.Equal(t, "99999999-9999-9999-9999-999999999999", service.requireManageAccessAccountID)
			require.Equal(t, "88888888-8888-8888-8888-888888888888", service.requireManageAccessOrganizationID)
			require.False(t, tc.mutationCalled(service))
		})
	}
}

func TestAgentsHandlerChatAgentRequiresManageBeforeBindingRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &stubWebAppStatusHandlerService{requireManageAccessErr: runtimeservice.ErrPermissionDenied}
	handler := NewAgentsHandler(service, nil, nil, nil, nil, &noopChatRuntimeService{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", "99999999-9999-9999-9999-999999999999")
		util.SetOrganizationID(c, "88888888-8888-8888-8888-888888888888")
		c.Next()
	})
	router.POST("/agents/:agent_id/chat", handler.ChatAgent)

	req := httptest.NewRequest(http.MethodPost, "/agents/11111111-1111-1111-1111-111111111111/chat", bytes.NewBufferString(`{"query":`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "403001", body["code"])
	require.True(t, service.requireManageAccessCalled)
	require.Equal(t, "11111111-1111-1111-1111-111111111111", service.requireManageAccessAgentID)
	require.Equal(t, "99999999-9999-9999-9999-999999999999", service.requireManageAccessAccountID)
	require.Equal(t, "88888888-8888-8888-8888-888888888888", service.requireManageAccessOrganizationID)
	require.False(t, service.draftRuntimeConfigCalled)
}

func TestAgentsHandlerWorkflowBindingCandidatesRequireManageBeforeServiceCall(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &stubWebAppStatusHandlerService{requireManageAccessErr: runtimeservice.ErrPermissionDenied}
	handler := NewAgentsHandler(service, nil, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", "99999999-9999-9999-9999-999999999999")
		util.SetOrganizationID(c, "88888888-8888-8888-8888-888888888888")
		c.Next()
	})
	router.GET("/agents/:agent_id/workflow-bindings/candidates", handler.ListAgentWorkflowBindingCandidates)

	req := httptest.NewRequest(http.MethodGet, "/agents/11111111-1111-1111-1111-111111111111/workflow-bindings/candidates", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "403001", body["code"])
	require.True(t, service.requireManageAccessCalled)
	require.Equal(t, "11111111-1111-1111-1111-111111111111", service.requireManageAccessAgentID)
	require.Equal(t, "99999999-9999-9999-9999-999999999999", service.requireManageAccessAccountID)
	require.Equal(t, "88888888-8888-8888-8888-888888888888", service.requireManageAccessOrganizationID)
	require.False(t, service.workflowBindingCandidatesCalled)
}

func TestAgentsHandlerCreateRequiresManageBeforeBindingBusinessFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &stubWebAppStatusHandlerService{}
	organizationService := &createAgentPermissionOrganizationService{allowed: false}
	handler := NewAgentsHandler(service, nil, nil, organizationService, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", "99999999-9999-9999-9999-999999999999")
		util.SetOrganizationID(c, "88888888-8888-8888-8888-888888888888")
		c.Next()
	})
	router.POST("/agents", handler.CreateAgent)

	req := httptest.NewRequest(http.MethodPost, "/agents", bytes.NewBufferString(`{"workspace_id":"22222222-2222-2222-2222-222222222222"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "403001", body["code"])
	require.Equal(t, 1, organizationService.checkCalls)
	require.Equal(t, "88888888-8888-8888-8888-888888888888", organizationService.organizationID)
	require.Equal(t, "22222222-2222-2222-2222-222222222222", organizationService.workspaceID)
	require.Equal(t, "99999999-9999-9999-9999-999999999999", organizationService.accountID)
	require.Equal(t, workspace_model.WorkspacePermissionAgentManage, organizationService.permission)
	require.False(t, service.createAgentCalled)
}

func TestPublicAgentWebAppConfig_DoesNotExposeRuntimeSecrets(t *testing.T) {
	public := publicAgentWebAppConfig(&dto.AgentWebAppRuntimeConfigResponse{
		AgentID:     "agent-1",
		WebAppID:    "webapp-1",
		AgentType:   "AGENT",
		Name:        "Agent",
		Description: "Public description",
		Icon:        "AG",
		IconType:    "text",
		IconURL:     "",
		Version:     "20260526000000",
		VersionUUID: "version-1",
		Config: dto.AgentConfigResponse{
			SystemPrompt:        "secret prompt",
			ModelProvider:       "secret-provider",
			Model:               "secret-model",
			EnabledSkillIDs:     []string{"secret-skill"},
			KnowledgeDatasetIDs: []string{"secret-dataset"},
			DatabaseBindings: []dto.AgentDatabaseBinding{{
				DataSourceID:     "secret-database",
				TableIDs:         []string{"secret-table"},
				WritableTableIDs: []string{"secret-writable-table"},
			}},
			HomeTitle:          "Home",
			InputPlaceholder:   "Ask",
			SuggestedQuestions: []string{"Q1"},
			FileUpload:         true,
			SupportsVision:     true,
			AgentMemoryEnabled: true,
		},
	})

	encoded, err := json.Marshal(public)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "system_prompt")
	require.NotContains(t, string(encoded), "secret prompt")
	require.NotContains(t, string(encoded), "secret-model")
	require.NotContains(t, string(encoded), "secret-dataset")
	require.NotContains(t, string(encoded), "database_bindings")
	require.NotContains(t, string(encoded), "secret-database")
	require.NotContains(t, string(encoded), "secret-table")
	require.NotContains(t, string(encoded), "secret-writable-table")
	require.Contains(t, string(encoded), "Home")
	require.Contains(t, string(encoded), "file_upload_enabled")
	require.Contains(t, string(encoded), "supports_vision")
	require.Contains(t, string(encoded), "agent_memory_enabled")
}

func TestRequireAuthenticatedWebAppAgentWhenMemoryEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	published := &dto.AgentWebAppRuntimeConfigResponse{
		Config: dto.AgentConfigResponse{AgentMemoryEnabled: true},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webapps/webapp-1/chat", nil)

	require.False(t, requireAuthenticatedWebAppAgentWhenMemoryEnabled(c, published))
	require.Equal(t, http.StatusUnauthorized, w.Code)

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webapps/webapp-1/chat", nil)
	c.Set("is_authenticated", true)

	require.True(t, requireAuthenticatedWebAppAgentWhenMemoryEnabled(c, published))
}

func TestRequireAuthenticatedWebAppAgentFileAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webapps/webapp-1/files/upload", nil)

	require.False(t, requireAuthenticatedWebAppAgentFileAccess(c))
	require.Equal(t, http.StatusUnauthorized, w.Code)

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webapps/webapp-1/files/upload", nil)
	c.Set("is_authenticated", true)

	require.True(t, requireAuthenticatedWebAppAgentFileAccess(c))
}

func TestAgentsHandler_GetWebAppRuntimeConfig_MapsNotPublishedError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &stubWebAppStatusHandlerService{err: errAgentWebAppNotPublished}
	handler := NewAgentsHandler(service, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/webapps/:web_app_id/config", handler.GetWebAppRuntimeConfig)

	req := httptest.NewRequest(http.MethodGet, "/webapps/33333333-3333-3333-3333-333333333333/config", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "204009", body["code"])
}

func TestAgentsHandler_GetWebAppRuntimeCapability_ReturnsPublicOnlyContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &stubWebAppStatusHandlerService{
		publishedConfigResp: &dto.AgentWebAppRuntimeConfigResponse{
			AgentID:        "11111111-1111-1111-1111-111111111111",
			WebAppID:       "33333333-3333-3333-3333-333333333333",
			WorkspaceID:    "22222222-2222-2222-2222-222222222222",
			OrganizationID: "88888888-8888-8888-8888-888888888888",
			VersionUUID:    "44444444-4444-4444-4444-444444444444",
		},
	}
	handler := NewAgentsHandler(service, nil, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", "99999999-9999-9999-9999-999999999999")
		c.Set("is_authenticated", true)
		c.Next()
	})
	router.GET("/webapps/:web_app_id/capability", handler.GetWebAppRuntimeCapability)

	req := httptest.NewRequest(http.MethodGet, "/webapps/33333333-3333-3333-3333-333333333333/capability", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, service.publishedConfigCalled)
	require.Equal(t, "33333333-3333-3333-3333-333333333333", service.publishedConfigWebAppID)

	var body struct {
		Code string `json:"code"`
		Data struct {
			AgentID                string   `json:"agent_id"`
			WebAppID               string   `json:"web_app_id"`
			WorkspaceID            string   `json:"workspace_id"`
			OrganizationID         string   `json:"organization_id"`
			Surface                string   `json:"surface"`
			Allowed                bool     `json:"allowed"`
			Reason                 string   `json:"reason"`
			AuthMode               string   `json:"auth_mode"`
			PublicOnly             bool     `json:"public_only"`
			PrivateAudienceEnabled bool     `json:"private_audience_enabled"`
			SupportedSubjectTypes  []string `json:"supported_subject_types"`
			VersionUUID            string   `json:"version_uuid"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "0", body.Code)
	require.Equal(t, "11111111-1111-1111-1111-111111111111", body.Data.AgentID)
	require.Equal(t, "33333333-3333-3333-3333-333333333333", body.Data.WebAppID)
	require.Equal(t, "22222222-2222-2222-2222-222222222222", body.Data.WorkspaceID)
	require.Equal(t, "88888888-8888-8888-8888-888888888888", body.Data.OrganizationID)
	require.Equal(t, "webapp", body.Data.Surface)
	require.True(t, body.Data.Allowed)
	require.Equal(t, agentWebAppCapabilityReasonPublicCompatible, body.Data.Reason)
	require.Equal(t, "authenticated", body.Data.AuthMode)
	require.True(t, body.Data.PublicOnly)
	require.False(t, body.Data.PrivateAudienceEnabled)
	require.Equal(t, []string{"public"}, body.Data.SupportedSubjectTypes)
	require.Equal(t, "44444444-4444-4444-4444-444444444444", body.Data.VersionUUID)
}

func TestAgentsHandler_GetWebAppRuntimeCapability_MapsOfflineError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &stubWebAppStatusHandlerService{err: errAgentWebAppOffline}
	handler := NewAgentsHandler(service, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/webapps/:web_app_id/capability", handler.GetWebAppRuntimeCapability)

	req := httptest.NewRequest(http.MethodGet, "/webapps/33333333-3333-3333-3333-333333333333/capability", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.True(t, service.publishedConfigCalled)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "204008", body["code"])
}

func TestAgentsHandler_WebAppFileEndpointsRejectOfflineBeforeAuthOrFileValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		method         string
		path           string
		register       func(*gin.Engine, *AgentsHandler)
		fileConfigUsed func(*stubFileService) bool
		fileUploadUsed func(*stubFileService) bool
	}{
		{
			name:   "upload config",
			method: http.MethodGet,
			path:   "/webapps/33333333-3333-3333-3333-333333333333/files/upload",
			register: func(router *gin.Engine, handler *AgentsHandler) {
				router.GET("/webapps/:web_app_id/files/upload", handler.GetWebAppUploadConfig)
			},
			fileConfigUsed: func(fileService *stubFileService) bool {
				return fileService.getUploadConfigCalled
			},
		},
		{
			name:   "upload file",
			method: http.MethodPost,
			path:   "/webapps/33333333-3333-3333-3333-333333333333/files/upload",
			register: func(router *gin.Engine, handler *AgentsHandler) {
				router.POST("/webapps/:web_app_id/files/upload", handler.UploadWebAppFile)
			},
			fileUploadUsed: func(fileService *stubFileService) bool {
				return fileService.uploadFileCalled
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &stubWebAppStatusHandlerService{err: errAgentWebAppOffline}
			fileService := &stubFileService{}
			handler := NewAgentsHandler(service, nil, nil, nil, nil)
			handler.SetFileService(fileService)
			router := gin.New()
			tt.register(router, handler)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusForbidden, w.Code)
			require.True(t, service.publishedConfigCalled)
			require.Equal(t, "33333333-3333-3333-3333-333333333333", service.publishedConfigWebAppID)
			if tt.fileConfigUsed != nil {
				require.False(t, tt.fileConfigUsed(fileService))
			}
			if tt.fileUploadUsed != nil {
				require.False(t, tt.fileUploadUsed(fileService))
			}

			var body map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, "204008", body["code"])
		})
	}
}

func newWebAppStatusHandlerTestRouter(service AgentsService) *gin.Engine {
	handler := NewAgentsHandler(service, nil, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", "99999999-9999-9999-9999-999999999999")
		util.SetOrganizationID(c, "88888888-8888-8888-8888-888888888888")
		c.Next()
	})
	router.PATCH("/agents/:agent_id/webapp/status", handler.UpdateWebAppStatus)
	return router
}

type stubWebAppStatusHandlerService struct {
	resp *dto.WebAppStatusResponse
	err  error

	called                              bool
	agentID                             string
	req                                 dto.UpdateWebAppStatusRequest
	accountID                           string
	organizationID                      string
	runtimeSurfacesResp                 *dto.AgentRuntimeSurfaceAuthorizationResponse
	runtimeSurfacesCalled               bool
	runtimeSurfacesAgentID              string
	runtimeSurfacesAccountID            string
	runtimeSurfacesOrganizationID       string
	updateRuntimeSurfacesCalled         bool
	updateRuntimeSurfacesAgentID        string
	updateRuntimeSurfacesAccountID      string
	updateRuntimeSurfacesOrganizationID string
	updateRuntimeSurfacesReq            dto.UpdateAgentRuntimeSurfacesRequest
	requireManageAccessCalled           bool
	requireManageAccessAgentID          string
	requireManageAccessAccountID        string
	requireManageAccessOrganizationID   string
	requireManageAccessErr              error
	createAgentCalled                   bool
	draftRuntimeConfigCalled            bool
	updateAgentConfigCalled             bool
	workflowBindingCandidatesCalled     bool
	updateAgentCalled                   bool
	replaceMemorySlotsCalled            bool
	updateMemoryValueCalled             bool
	generateSuggestedQuestionsCalled    bool
	rollbackAgentPublishedVersionCalled bool
	publishedConfigCalled               bool
	publishedConfigWebAppID             string
	publishedConfigResp                 *dto.AgentWebAppRuntimeConfigResponse
}

func (s *stubWebAppStatusHandlerService) GetAgentsListWithPermissions(context.Context, string, dto.GetAgentsListRequest) (*dto.AgentsListResponse, error) {
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) GetRunnableWebApps(context.Context, string, dto.GetRunnableWebAppsRequest) (*dto.RunnableWebAppsResponse, error) {
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) CreateAgent(context.Context, string, interface{}, string) (interface{}, error) {
	s.createAgentCalled = true
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) GetAgent(context.Context, string) (interface{}, error) {
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) UpdateAgent(context.Context, string, interface{}) (interface{}, error) {
	s.updateAgentCalled = true
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) GetAgentConfig(context.Context, string, string) (*dto.AgentConfigResponse, error) {
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) GetAgentDraftRuntimeConfig(context.Context, string, string) (*dto.AgentDraftRuntimeConfigResponse, error) {
	s.draftRuntimeConfigCalled = true
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) RequireAgentManageAccess(ctx context.Context, agentID, accountID string) error {
	s.requireManageAccessCalled = true
	s.requireManageAccessAgentID = agentID
	s.requireManageAccessAccountID = accountID
	if v := ctx.Value("tenant_id"); v != nil {
		s.requireManageAccessOrganizationID, _ = v.(string)
	}
	return s.requireManageAccessErr
}

func (s *stubWebAppStatusHandlerService) GetAgentRuntimeSurfaces(ctx context.Context, agentID string, accountID string) (*dto.AgentRuntimeSurfaceAuthorizationResponse, error) {
	s.runtimeSurfacesCalled = true
	s.runtimeSurfacesAgentID = agentID
	s.runtimeSurfacesAccountID = accountID
	if v := ctx.Value("tenant_id"); v != nil {
		s.runtimeSurfacesOrganizationID, _ = v.(string)
	}
	return s.runtimeSurfacesResp, s.err
}

func (s *stubWebAppStatusHandlerService) UpdateAgentRuntimeSurfaces(ctx context.Context, agentID string, accountID string, req dto.UpdateAgentRuntimeSurfacesRequest) (*dto.AgentRuntimeSurfaceAuthorizationResponse, error) {
	s.updateRuntimeSurfacesCalled = true
	s.updateRuntimeSurfacesAgentID = agentID
	s.updateRuntimeSurfacesAccountID = accountID
	s.updateRuntimeSurfacesReq = req
	if v := ctx.Value("tenant_id"); v != nil {
		s.updateRuntimeSurfacesOrganizationID, _ = v.(string)
	}
	return s.runtimeSurfacesResp, s.err
}

func (s *stubWebAppStatusHandlerService) UpdateAgentConfig(context.Context, string, string, dto.AgentConfigRequest) (*dto.AgentConfigResponse, error) {
	s.updateAgentConfigCalled = true
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) ListAgentWorkflowBindingCandidates(context.Context, string, string) (*dto.AgentWorkflowBindingCandidatesResponse, error) {
	s.workflowBindingCandidatesCalled = true
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) ListAgentMemorySlots(context.Context, string, string) ([]dto.AgentMemorySlotConfig, error) {
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) ReplaceAgentMemorySlots(context.Context, string, string, []dto.AgentMemorySlotConfig) ([]dto.AgentMemorySlotConfig, error) {
	s.replaceMemorySlotsCalled = true
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) ListAgentMemoryValues(context.Context, string, string) (*dto.AgentMemoryValuesResponse, error) {
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) UpdateAgentMemoryValue(context.Context, string, string, dto.UpdateAgentMemoryValueRequest) (*dto.AgentMemoryValueResponse, error) {
	s.updateMemoryValueCalled = true
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) ClearAgentMemoryValue(context.Context, string, string, string) (*dto.AgentMemoryValueResponse, error) {
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) GenerateAgentSuggestedQuestions(context.Context, string, string, *dto.GenerateAgentSuggestedQuestionsRequest) (*dto.GenerateSuggestedQuestionsResponse, error) {
	s.generateSuggestedQuestionsCalled = true
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) PublishAgent(context.Context, string, string, dto.PublishAgentRequest) (*dto.PublishAgentResponse, error) {
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) ListAgentPublishedVersions(context.Context, string, string, int, int) (*dto.AgentPublishedVersionsResponse, error) {
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) RollbackAgentPublishedVersion(context.Context, string, string, dto.RollbackAgentPublishedVersionRequest) (*dto.AgentConfigResponse, error) {
	s.rollbackAgentPublishedVersionCalled = true
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) GetPublishedAgentWebAppConfig(_ context.Context, webAppID string) (*dto.AgentWebAppRuntimeConfigResponse, error) {
	s.publishedConfigCalled = true
	s.publishedConfigWebAppID = webAppID
	return s.publishedConfigResp, s.err
}

func (s *stubWebAppStatusHandlerService) UpdateWebAppStatus(ctx context.Context, agentID string, req dto.UpdateWebAppStatusRequest) (*dto.WebAppStatusResponse, error) {
	s.called = true
	s.agentID = agentID
	s.req = req
	if v := ctx.Value("account_id"); v != nil {
		s.accountID, _ = v.(string)
	}
	if v := ctx.Value("tenant_id"); v != nil {
		s.organizationID, _ = v.(string)
	}
	return s.resp, s.err
}

func (s *stubWebAppStatusHandlerService) DeleteAgent(context.Context, string) error {
	return nil
}

type noopChatRuntimeService struct {
	runtimeservice.Service
}

type stubFileService struct {
	interfaces.FileService
	getUploadConfigCalled bool
	uploadFileCalled      bool
}

func (s *stubFileService) GetUploadConfig() *interfaces.FileUploadConfigResponse {
	s.getUploadConfigCalled = true
	return &interfaces.FileUploadConfigResponse{}
}

func (s *stubFileService) UploadFile(ctx context.Context, filename string, content []byte, mimeType string, userID, tenantID string, userRole file_model.CreatedByRole, source *interfaces.FileSource, teamTenantID *string, isTemporary bool, isIcon bool) (*dto.UploadFile, error) {
	s.uploadFileCalled = true
	return &dto.UploadFile{ID: "file-1"}, nil
}

type createAgentPermissionOrganizationService struct {
	interfaces.OrganizationService

	allowed        bool
	checkCalls     int
	organizationID string
	workspaceID    string
	accountID      string
	permission     workspace_model.WorkspacePermissionCode
}

func (s *createAgentPermissionOrganizationService) CheckWorkspacePermission(_ context.Context, organizationID, workspaceID, accountID string, permission workspace_model.WorkspacePermissionCode) (bool, error) {
	s.checkCalls++
	s.organizationID = organizationID
	s.workspaceID = workspaceID
	s.accountID = accountID
	s.permission = permission
	return s.allowed, nil
}
