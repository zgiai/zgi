package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/config"
	shortlinkcap "github.com/zgiai/zgi/api/internal/capabilities/shortlink"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	workflow_file "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	automationdefinition "github.com/zgiai/zgi/api/internal/modules/automation/service/definition"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	promptservice "github.com/zgiai/zgi/api/internal/modules/prompts/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/database"
	jwtpkg "github.com/zgiai/zgi/api/pkg/jwt"
	"github.com/zgiai/zgi/api/pkg/queue"
	"github.com/zgiai/zgi/api/pkg/response"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
	pkguuid "github.com/zgiai/zgi/api/pkg/uuid"
)

func TestWorkflowRoutes_WebAppConfigIsPublic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, _ := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/workflows/"+webAppID+"/config", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "0", body["code"])

	data, ok := body["data"].(map[string]any)
	require.True(t, ok)

	configData, ok := data["config"].(map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, configData["agent_id"])
	require.Equal(t, "WORKFLOW", configData["type"])
}

func TestWorkflowRoutes_WebAppConfigIgnoresInvalidAuthorizationHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, _ := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/workflows/"+webAppID+"/config", nil)
	req.Header.Set("Authorization", "Bearer "+uuid.NewString())
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "0", body["code"])
}

func TestWorkflowRoutes_WebAppConfigRejectsInactiveWebApp(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, _ := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/workflows/"+webAppID+"/config", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	requireWebAppOfflineResponse(t, w)
}

func TestWorkflowRoutes_WebAppConfigRejectsPersistedDisabledWebApp(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, _ := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/workflows/"+webAppID+"/config", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	requireWebAppOfflineResponse(t, w)
}

func TestWorkflowRoutes_RunStillRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, _ := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/console/api/workflows/"+webAppID+"/run", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "212012", body["code"])
}

func TestWorkflowRoutes_RunAcceptsUUIDBearerIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, userID := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/console/api/workflows/"+webAppID+"/run", nil)
	req.Header.Set("Authorization", "Bearer "+userID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "199001", body["code"])
}

func TestWorkflowRoutes_RunRejectsInactiveWebApp(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, userID := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/console/api/workflows/"+webAppID+"/run", nil)
	req.Header.Set("Authorization", "Bearer "+userID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	requireWebAppOfflineResponse(t, w)
}

func TestWorkflowRoutes_RunRejectsPersistedDisabledWebApp(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, userID := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/console/api/workflows/"+webAppID+"/run", nil)
	req.Header.Set("Authorization", "Bearer "+userID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	requireWebAppOfflineResponse(t, w)
}

func TestWorkflowRoutes_ConversationListAcceptsUUIDBearerIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, userID := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/workflows/"+webAppID+"/conversations?page=1&limit=20", nil)
	req.Header.Set("Authorization", "Bearer "+userID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "0", body["code"])

	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	require.Len(t, data["data"], 1)

	items, ok := data["data"].([]any)
	require.True(t, ok)
	item, ok := items[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "退款进度查询", item["name"])
	require.Equal(t, float64(1), item["dialogue_count"])
}

func TestWorkflowRoutes_ConversationListRejectsInactiveWebApp(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, userID := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/workflows/"+webAppID+"/conversations?page=1&limit=20", nil)
	req.Header.Set("Authorization", "Bearer "+userID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	requireWebAppOfflineResponse(t, w)
}

func TestWorkflowRoutes_ConversationListRejectsPersistedDisabledWebApp(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, userID := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/workflows/"+webAppID+"/conversations?page=1&limit=20", nil)
	req.Header.Set("Authorization", "Bearer "+userID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	requireWebAppOfflineResponse(t, w)
}

func TestWorkflowRoutes_ConversationDetailAcceptsOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, userID, conversationID := newWorkflowRoutesTestRouterWithConversation(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/workflows/"+webAppID+"/conversations/"+conversationID, nil)
	req.Header.Set("Authorization", "Bearer "+userID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "0", body["code"])

	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, conversationID, data["id"])
	require.Equal(t, "退款进度查询", data["name"])
	require.Equal(t, float64(1), data["dialogue_count"])
	require.IsType(t, []any{}, data["messages"])
}

func TestWorkflowRoutes_ConversationDetailRejectsOtherUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, _, conversationID := newWorkflowRoutesTestRouterWithConversation(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/workflows/"+webAppID+"/conversations/"+conversationID, nil)
	req.Header.Set("Authorization", "Bearer "+uuid.NewString())
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "403001", body["code"])
}

func TestWorkflowRoutes_ConversationDetailRejectsOtherAgentConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, userID, conversationID := newWorkflowRoutesTestRouterWithConversation(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/workflows/"+webAppID+"/conversations/"+conversationID, nil)
	req.Header.Set("Authorization", "Bearer "+userID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.NotEqual(t, http.StatusOK, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, strconv.Itoa(response.ErrAppNotFound.Code), body["code"])
}

func TestWorkflowRoutes_ConversationDeleteRejectsOtherUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, _, conversationID := newWorkflowRoutesTestRouterWithConversation(t)

	req := httptest.NewRequest(http.MethodDelete, "/console/api/workflows/"+webAppID+"/conversations/"+conversationID, nil)
	req.Header.Set("Authorization", "Bearer "+uuid.NewString())
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "403001", body["code"])

	var activeCount int64
	require.NoError(t, database.DB.Table("agents_conversations").
		Where("id = ? AND deleted_at IS NULL", conversationID).
		Count(&activeCount).Error)
	require.Equal(t, int64(1), activeCount)
}

func TestWorkflowRoutes_ConversationDeleteRejectsOtherAgentConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, userID, conversationID := newWorkflowRoutesTestRouterWithConversation(t)

	req := httptest.NewRequest(http.MethodDelete, "/console/api/workflows/"+webAppID+"/conversations/"+conversationID, nil)
	req.Header.Set("Authorization", "Bearer "+userID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.NotEqual(t, http.StatusOK, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, strconv.Itoa(response.ErrAppNotFound.Code), body["code"])

	var activeCount int64
	require.NoError(t, database.DB.Table("agents_conversations").
		Where("id = ? AND deleted_at IS NULL", conversationID).
		Count(&activeCount).Error)
	require.Equal(t, int64(1), activeCount)
}

func TestWorkflowRoutes_ConversationDeleteAcceptsOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, userID, conversationID := newWorkflowRoutesTestRouterWithConversation(t)

	req := httptest.NewRequest(http.MethodDelete, "/console/api/workflows/"+webAppID+"/conversations/"+conversationID, nil)
	req.Header.Set("Authorization", "Bearer "+userID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "0", body["code"])

	var activeCount int64
	require.NoError(t, database.DB.Table("agents_conversations").
		Where("id = ? AND deleted_at IS NULL", conversationID).
		Count(&activeCount).Error)
	require.Equal(t, int64(0), activeCount)
}

func TestWorkflowRoutes_PrecheckAcceptsUUIDBearerIdentityWithEmptyBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, userID := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/console/api/workflows/"+webAppID+"/precheck", nil)
	req.Header.Set("Authorization", "Bearer "+userID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "0", body["code"])

	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, data["contains_ai_credit_nodes"])
	require.Equal(t, "ok", data["status"])
	require.Empty(t, data["warnings"])
}

func TestWorkflowRoutes_PrecheckRejectsInactiveWebApp(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, userID := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/console/api/workflows/"+webAppID+"/precheck", nil)
	req.Header.Set("Authorization", "Bearer "+userID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	requireWebAppOfflineResponse(t, w)
}

func TestWorkflowRoutes_PrecheckRejectsPersistedDisabledWebApp(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, userID := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/console/api/workflows/"+webAppID+"/precheck", nil)
	req.Header.Set("Authorization", "Bearer "+userID)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	requireWebAppOfflineResponse(t, w)
}

func TestWorkflowRoutes_PrecheckIgnoresLegacyRequestBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, webAppID, userID := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/console/api/workflows/"+webAppID+"/precheck", strings.NewReader(`{"query":"","inputs":{"topic":"hello"},"response_mode":"streaming"}`))
	req.Header.Set("Authorization", "Bearer "+userID)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "0", body["code"])

	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, data["contains_ai_credit_nodes"])
}

func TestWorkflowRoutes_BuiltInWorkflowsRequireOrganizationAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, _ := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/built-in-workflows", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWorkflowRoutes_BuiltInWorkflowsAcceptOrganizationMemberWithoutWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, userID := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/built-in-workflows", nil)
	req.Header.Set("Authorization", workflowRoutesAuthHeader(t, userID))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "0", body["code"])

	items, ok := body["data"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)

	item, ok := items[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "imagegen_chat", item["scenario"])
}

func TestWorkflowRoutes_BuiltInWorkflowsFilterRuntimeGrantForOtherAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, userID := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/built-in-workflows", nil)
	req.Header.Set("Authorization", workflowRoutesAuthHeader(t, userID))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "0", body["code"])

	items, ok := body["data"].([]any)
	require.True(t, ok)
	require.Empty(t, items)
}

func TestWorkflowRoutes_BuiltInWorkflowsAllowRuntimeGrantForDepartmentMember(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, userID := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/built-in-workflows", nil)
	req.Header.Set("Authorization", workflowRoutesAuthHeader(t, userID))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "0", body["code"])

	items, ok := body["data"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)

	item, ok := items[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "imagegen_chat", item["scenario"])
}

func TestWorkflowRoutes_BuiltInWorkflowDetailRejectsRuntimeGrantForOtherAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, userID := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/built-in-workflows/imagegen_chat", nil)
	req.Header.Set("Authorization", workflowRoutesAuthHeader(t, userID))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "403001", body["code"])
}

func TestWorkflowRoutes_BuiltInWorkflowDetailAllowsRuntimeGrantForDepartmentMember(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, userID := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/built-in-workflows/imagegen_chat", nil)
	req.Header.Set("Authorization", workflowRoutesAuthHeader(t, userID))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "0", body["code"])

	item, ok := body["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "imagegen_chat", item["scenario"])
}

func TestWorkflowRoutes_BuiltInWorkflowRuntimeSurfacesReturnFallbackForAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, userID := newWorkflowRoutesTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/console/api/built-in-workflows/imagegen_chat/runtime-surfaces", nil)
	req.Header.Set("Authorization", workflowRoutesAuthHeader(t, userID))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, "0", body["code"])

	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "imagegen_chat", data["scenario"])
	require.NotEmpty(t, data["organization_id"])

	surfaces, ok := data["surfaces"].([]any)
	require.True(t, ok)
	require.Len(t, surfaces, 2)
	requireSurfaceEnabled(t, surfaces, "builtin_app", true)
	requireSurfaceEnabled(t, surfaces, "internal", true)
}

func TestWorkflowRoutes_BuiltInWorkflowRuntimeSurfacesUpdateAccountGrantForAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, userID := newWorkflowRoutesTestRouter(t)
	accountGrantID := "11111111-1111-1111-1111-111111111111"
	body := `{"surfaces":[{"surface":"builtin_app","enabled":true,"grants":[{"subject_type":"account","subject_id":"` + accountGrantID + `"}]}]}`

	req := httptest.NewRequest(http.MethodPatch, "/console/api/built-in-workflows/imagegen_chat/runtime-surfaces", strings.NewReader(body))
	req.Header.Set("Authorization", workflowRoutesAuthHeader(t, userID))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	resp := decodeJSONBody(t, w)
	require.Equal(t, "0", resp["code"])

	data, ok := resp["data"].(map[string]any)
	require.True(t, ok)
	surfaces, ok := data["surfaces"].([]any)
	require.True(t, ok)

	builtin := requireSurfaceEnabled(t, surfaces, "builtin_app", true)
	grants, ok := builtin["grants"].([]any)
	require.True(t, ok)
	require.Len(t, grants, 1)
	grant, ok := grants[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "account", grant["subject_type"])
	require.Equal(t, accountGrantID, grant["subject_id"])
}

func TestWorkflowRoutes_BuiltInWorkflowRuntimeSurfacesRejectsAccountGrantOutsideOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, userID := newWorkflowRoutesTestRouter(t)
	accountGrantID := "99999999-9999-9999-9999-999999999998"
	body := `{"surfaces":[{"surface":"builtin_app","enabled":true,"grants":[{"subject_type":"account","subject_id":"` + accountGrantID + `"}]}]}`

	req := httptest.NewRequest(http.MethodPatch, "/console/api/built-in-workflows/imagegen_chat/runtime-surfaces", strings.NewReader(body))
	req.Header.Set("Authorization", workflowRoutesAuthHeader(t, userID))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeJSONBody(t, w)
	require.NotEqual(t, "0", resp["code"])
	require.Contains(t, w.Body.String(), "runtime grant account is not in organization")
}

func TestWorkflowRoutes_BuiltInWorkflowRuntimeSurfacesRejectsDepartmentGrantOutsideOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, userID := newWorkflowRoutesTestRouter(t)
	departmentGrantID := "99999999-9999-9999-9999-999999999997"
	body := `{"surfaces":[{"surface":"builtin_app","enabled":true,"grants":[{"subject_type":"department","subject_id":"` + departmentGrantID + `"}]}]}`

	req := httptest.NewRequest(http.MethodPatch, "/console/api/built-in-workflows/imagegen_chat/runtime-surfaces", strings.NewReader(body))
	req.Header.Set("Authorization", workflowRoutesAuthHeader(t, userID))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeJSONBody(t, w)
	require.NotEqual(t, "0", resp["code"])
	require.Contains(t, w.Body.String(), "runtime grant department is not in organization")
}

func TestWorkflowRoutes_BuiltInWorkflowRuntimeSurfacesRejectsNonAdminUpdate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, userID, _ := newWorkflowRoutesTestRouterWithConversationAndAccountService(t, workflowRoutesNonAdminAccountService{})
	body := `{"surfaces":[{"surface":"builtin_app","enabled":false}]}`

	req := httptest.NewRequest(http.MethodPatch, "/console/api/built-in-workflows/imagegen_chat/runtime-surfaces", strings.NewReader(body))
	req.Header.Set("Authorization", workflowRoutesAuthHeader(t, userID))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)

	resp := decodeJSONBody(t, w)
	require.Equal(t, "403001", resp["code"])
}

func TestWorkflowRoutes_DoNotRegisterLegacyWorkflowStatisticRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, _ := newWorkflowRoutesTestRouter(t)

	for _, route := range router.Routes() {
		require.NotContains(t, route.Path, "/statistics", "legacy workflow statistics routes must be restored deliberately with authorization")
		require.NotContains(t, route.Handler, "WorkflowStatisticHandler", "workflow statistics handler must not be wired without explicit authorization")
	}
}

func newWorkflowRoutesTestRouter(t *testing.T) (*gin.Engine, string, string) {
	t.Helper()

	router, webAppID, userID, _ := newWorkflowRoutesTestRouterWithConversation(t)
	return router, webAppID, userID
}

func newWorkflowRoutesTestRouterWithConversation(t *testing.T) (*gin.Engine, string, string, string) {
	t.Helper()
	return newWorkflowRoutesTestRouterWithConversationAndAccountService(t, workflowRoutesAccountService{})
}

func newWorkflowRoutesTestRouterWithConversationAndAccountService(t *testing.T, accountService interfaces.AccountService) (*gin.Engine, string, string, string) {
	t.Helper()
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Platform: config.PlatformConfig{Edition: "TEST"},
		JWT: config.JWTConfig{
			Secret:    "workflow-routes-test-secret",
			JWTExpire: time.Hour,
			Issuer:    "TEST",
		},
		Redis: config.RedisConfig{Host: "127.0.0.1", Port: 6379},
		Storage: config.StorageConfig{
			Type:            "local",
			OpenDALBasePath: t.TempDir(),
		},
		TaskQueue: config.TaskQueueConfig{
			Concurrency: 1,
			Retention:   time.Hour,
		},
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
		jwtpkg.Init(oldConfig)
	})
	jwtpkg.Init(config.GlobalConfig)

	db, mock := newWorkflowRoutesTestDB(t)

	oldDB := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = oldDB
	})

	webAppID, userID, conversationID := seedWorkflowRoutesTestData()
	expectWorkflowRoutesSQL(t, mock, webAppID, userID, conversationID)

	taskManager, err := queue.NewTaskManager(config.GlobalConfig)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = taskManager.Close()
	})

	scheduler, err := pkgscheduler.NewScheduler(config.GlobalConfig)
	require.NoError(t, err)

	router := gin.New()
	v1Group := router.Group("/console/api")
	RegisterWorkflowRoutes(v1Group, WorkflowRouteDeps{
		DB:                          db,
		AccountService:              accountService,
		FileService:                 workflowRoutesFileService{},
		ContentExtractor:            workflowRoutesContentExtractor{},
		QuotaService:                workflowRoutesQuotaService{},
		OrganizationService:         workflowRoutesOrganizationService{},
		LLMClient:                   struct{}{},
		ToolEngine:                  struct{}{},
		GraphFlowService:            &graphflow.Service{},
		PromptResolver:              workflowRoutesPromptService{},
		AutomationDefinitionService: workflowRoutesAutomationDefinitionService{},
		TaskManager:                 taskManager,
		TaskRegistry:                workflowRoutesTaskRegistry{},
		Scheduler:                   scheduler,
		EngineFactory:               &graph_engine.EngineFactory{},
		AutomationRunnerSetter:      workflowRoutesAutomationRunnerSetter{},
		ShortLinkService:            shortlinkcap.NewServiceWithDB(db),
	})
	return router, webAppID, userID, conversationID
}

type workflowRoutesAccountService struct{ interfaces.AccountService }

func (workflowRoutesAccountService) LoadUser(_ context.Context, userID string) (*auth_model.Account, error) {
	return &auth_model.Account{
		ID:     userID,
		Name:   "route test user",
		Status: auth_model.AccountStatusActive,
	}, nil
}

func (workflowRoutesAccountService) GetCurrentWorkspace(context.Context, string) (*workspace_model.Workspace, error) {
	return nil, nil
}

func (workflowRoutesAccountService) EnsureCurrentOrganizationID(context.Context, string) (string, error) {
	return uuid.NewString(), nil
}

func (workflowRoutesAccountService) IsOrganizationAdminOrOwner(context.Context, string, string) (bool, error) {
	return true, nil
}

type workflowRoutesNonAdminAccountService struct{ workflowRoutesAccountService }

func (workflowRoutesNonAdminAccountService) IsOrganizationAdminOrOwner(context.Context, string, string) (bool, error) {
	return false, nil
}

type workflowRoutesFileService struct{ interfaces.FileService }
type workflowRoutesContentExtractor struct{ workflow_file.ContentExtractor }
type workflowRoutesQuotaService struct{ interfaces.QuotaService }
type workflowRoutesOrganizationService struct{ interfaces.OrganizationService }

func (workflowRoutesOrganizationService) GetOrganizationByWorkspaceID(context.Context, string) (*workspace_model.Organization, error) {
	return &workspace_model.Organization{ID: uuid.NewString()}, nil
}

type workflowRoutesPromptService struct{ promptservice.PromptService }
type workflowRoutesAutomationDefinitionService struct{ automationdefinition.Service }

type workflowRoutesAutomationRunnerSetter struct{}

func (workflowRoutesAutomationRunnerSetter) SetAutomationWorkflowRunner(automationaction.AutomationWorkflowRunner) {
}

type workflowRoutesTaskRegistry struct{}

func (workflowRoutesTaskRegistry) Register(string, func(context.Context, *asynq.Task) error) bool {
	return true
}

func newWorkflowRoutesTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.MatchExpectationsInOrder(false)

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, mock.ExpectationsWereMet())
		_ = sqlDB.Close()
	})

	return db, mock
}

func seedWorkflowRoutesTestData() (string, string, string) {
	webAppID := uuid.New()
	userID := uuid.New()
	conversationID := uuid.New()
	return webAppID.String(), userID.String(), conversationID.String()
}

func expectWorkflowRoutesSQL(t *testing.T, mock sqlmock.Sqlmock, webAppID, userID, conversationID string) {
	t.Helper()

	switch t.Name() {
	case "TestWorkflowRoutes_WebAppConfigIsPublic",
		"TestWorkflowRoutes_WebAppConfigIgnoresInvalidAuthorizationHeader":
		expectAgentByWebApp(mock, webAppID)
		expectLatestWorkflow(mock)
	case "TestWorkflowRoutes_WebAppConfigRejectsInactiveWebApp":
		expectAgentByWebAppWithStatus(mock, webAppID, "inactive")
	case "TestWorkflowRoutes_WebAppConfigRejectsPersistedDisabledWebApp":
		expectAgentByWebAppWithRuntimeSurface(mock, webAppID, "active", false)
	case "TestWorkflowRoutes_RunRejectsInactiveWebApp",
		"TestWorkflowRoutes_ConversationListRejectsInactiveWebApp",
		"TestWorkflowRoutes_PrecheckRejectsInactiveWebApp":
		expectAgentByWebAppWithStatus(mock, webAppID, "inactive")
	case "TestWorkflowRoutes_RunRejectsPersistedDisabledWebApp",
		"TestWorkflowRoutes_ConversationListRejectsPersistedDisabledWebApp",
		"TestWorkflowRoutes_PrecheckRejectsPersistedDisabledWebApp":
		expectAgentByWebAppWithRuntimeSurface(mock, webAppID, "active", false)
	case "TestWorkflowRoutes_ConversationListAcceptsUUIDBearerIdentity":
		expectAgentByWebApp(mock, webAppID)
		expectConversationList(mock, userID)
	case "TestWorkflowRoutes_RunAcceptsUUIDBearerIdentity":
		expectAgentByWebApp(mock, webAppID)
		expectLatestWorkflow(mock)
	case "TestWorkflowRoutes_ConversationDetailAcceptsOwner":
		agentID := expectAgentByWebApp(mock, webAppID)
		expectConversationByAgent(mock, conversationID, agentID.String(), userID)
		expectConversationMessages(mock, conversationID)
	case "TestWorkflowRoutes_ConversationDetailRejectsOtherUser",
		"TestWorkflowRoutes_ConversationDeleteRejectsOtherUser":
		agentID := expectAgentByWebApp(mock, webAppID)
		expectConversationByAgent(mock, conversationID, agentID.String(), uuid.NewString())
		if t.Name() == "TestWorkflowRoutes_ConversationDeleteRejectsOtherUser" {
			expectActiveConversationCount(mock, conversationID, 1)
		}
	case "TestWorkflowRoutes_ConversationDetailRejectsOtherAgentConversation":
		agentID := expectAgentByWebApp(mock, webAppID)
		expectConversationByAgentNotFound(mock, conversationID, agentID.String())
	case "TestWorkflowRoutes_ConversationDeleteRejectsOtherAgentConversation":
		agentID := expectAgentByWebApp(mock, webAppID)
		expectConversationByAgentNotFound(mock, conversationID, agentID.String())
		expectActiveConversationCount(mock, conversationID, 1)
	case "TestWorkflowRoutes_ConversationDeleteAcceptsOwner":
		agentID := expectAgentByWebApp(mock, webAppID)
		expectConversationByAgent(mock, conversationID, agentID.String(), userID)
		expectDeleteConversation(mock)
		expectActiveConversationCount(mock, conversationID, 0)
	case "TestWorkflowRoutes_PrecheckAcceptsUUIDBearerIdentityWithEmptyBody",
		"TestWorkflowRoutes_PrecheckIgnoresLegacyRequestBody":
		expectAgentByWebApp(mock, webAppID)
		expectLatestWorkflow(mock)
	case "TestWorkflowRoutes_BuiltInWorkflowsAcceptOrganizationMemberWithoutWorkspace":
		agentID := expectBuiltInWorkflows(mock)
		expectBuiltInWorkflowAudienceDepartments(mock, nil)
		expectBuiltInWorkflowAudienceWorkspaces(mock, nil)
		expectBuiltInWorkflowRuntimeBatchFallback(mock, agentID)
	case "TestWorkflowRoutes_BuiltInWorkflowsFilterRuntimeGrantForOtherAccount":
		agentID := expectBuiltInWorkflows(mock)
		expectBuiltInWorkflowAudienceDepartments(mock, nil)
		expectBuiltInWorkflowAudienceWorkspaces(mock, nil)
		expectBuiltInWorkflowRuntimeBatchAccountGrant(mock, agentID, uuid.New())
	case "TestWorkflowRoutes_BuiltInWorkflowsAllowRuntimeGrantForDepartmentMember":
		departmentID := uuid.New()
		agentID := expectBuiltInWorkflows(mock)
		expectBuiltInWorkflowAudienceDepartments(mock, []uuid.UUID{departmentID})
		expectBuiltInWorkflowAudienceWorkspaces(mock, nil)
		expectBuiltInWorkflowRuntimeBatchDepartmentGrant(mock, agentID, departmentID)
	case "TestWorkflowRoutes_BuiltInWorkflowDetailRejectsRuntimeGrantForOtherAccount":
		agentID := expectBuiltInWorkflowByScenario(mock, "imagegen_chat")
		expectBuiltInWorkflowRuntimeAccountGrant(mock, agentID, uuid.New())
	case "TestWorkflowRoutes_BuiltInWorkflowDetailAllowsRuntimeGrantForDepartmentMember":
		agentID := expectBuiltInWorkflowByScenario(mock, "imagegen_chat")
		expectBuiltInWorkflowRuntimeDepartmentGrant(mock, agentID, uuid.New())
	case "TestWorkflowRoutes_BuiltInWorkflowRuntimeSurfacesReturnFallbackForAdmin":
		agentID := expectBuiltInWorkflowByScenario(mock, "imagegen_chat")
		expectBuiltInWorkflowRuntimeFallback(mock, agentID)
	case "TestWorkflowRoutes_BuiltInWorkflowRuntimeSurfacesUpdateAccountGrantForAdmin":
		agentID := expectBuiltInWorkflowByScenario(mock, "imagegen_chat")
		expectRuntimeGrantAccountInOrganization(mock, "11111111-1111-1111-1111-111111111111", 1)
		expectSaveBuiltInWorkflowRuntimeAccountGrant(mock, agentID)
		expectBuiltInWorkflowByScenario(mock, "imagegen_chat")
		expectBuiltInWorkflowRuntimeAccountGrant(mock, agentID, uuid.MustParse("11111111-1111-1111-1111-111111111111"))
	case "TestWorkflowRoutes_BuiltInWorkflowRuntimeSurfacesRejectsAccountGrantOutsideOrganization":
		expectBuiltInWorkflowByScenario(mock, "imagegen_chat")
		expectRuntimeGrantAccountInOrganization(mock, "99999999-9999-9999-9999-999999999998", 0)
	case "TestWorkflowRoutes_BuiltInWorkflowRuntimeSurfacesRejectsDepartmentGrantOutsideOrganization":
		expectBuiltInWorkflowByScenario(mock, "imagegen_chat")
		expectRuntimeGrantDepartmentInOrganization(mock, "99999999-9999-9999-9999-999999999997", 0)
	}
}

func expectAgentByWebApp(mock sqlmock.Sqlmock, webAppID string) uuid.UUID {
	return expectAgentByWebAppWithStatus(mock, webAppID, "active")
}

func expectAgentByWebAppWithStatus(mock sqlmock.Sqlmock, webAppID, webAppStatus string) uuid.UUID {
	return expectAgentByWebAppWithRuntimeSurface(mock, webAppID, webAppStatus, webAppStatus == "active")
}

func expectAgentByWebAppWithRuntimeSurface(mock sqlmock.Sqlmock, webAppID, webAppStatus string, surfaceEnabled bool) uuid.UUID {
	tenantID := uuid.New()
	agentID := uuid.New()
	createdBy := uuid.New()
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{
		"id",
		"tenant_id",
		"name",
		"description",
		"agent_type",
		"enable_api",
		"web_app_id",
		"web_app_status",
		"created_by",
		"created_at",
		"updated_at",
		"deleted_at",
	}).AddRow(
		agentID.String(),
		tenantID.String(),
		"public-workflow-app",
		"",
		"workflow",
		true,
		webAppID,
		webAppStatus,
		createdBy.String(),
		now,
		now,
		nil,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agents"`)).
		WithArgs(webAppID, 1).
		WillReturnRows(rows)
	if surfaceEnabled == (webAppStatus == "active") {
		expectAgentWebAppRuntimeFallback(mock, agentID)
	} else {
		expectAgentWebAppRuntimeSurface(mock, agentID, surfaceEnabled)
	}
	return agentID
}

func expectAgentWebAppRuntimeFallback(mock sqlmock.Sqlmock, agentID uuid.UUID) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND resource_id = $2 AND deleted_at IS NULL ORDER BY surface ASC`)).
		WithArgs(string(runtimeauth.PublishedRuntimeResourceAgent), agentID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"resource_type",
			"resource_id",
			"organization_id",
			"workspace_id",
			"surface",
			"enabled",
			"compatibility_source",
			"created_at",
			"updated_at",
			"deleted_at",
		}))
}

func expectAgentWebAppRuntimeSurface(mock sqlmock.Sqlmock, agentID uuid.UUID, enabled bool) {
	surfaceID := uuid.New()
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{
		"id",
		"resource_type",
		"resource_id",
		"organization_id",
		"workspace_id",
		"surface",
		"enabled",
		"compatibility_source",
		"created_at",
		"updated_at",
		"deleted_at",
	}).AddRow(
		surfaceID.String(),
		string(runtimeauth.PublishedRuntimeResourceAgent),
		agentID.String(),
		uuid.NewString(),
		nil,
		string(runtimeauth.PublishedRuntimeSurfaceWebApp),
		enabled,
		runtimeauth.PublishedRuntimeSourceGrant,
		now,
		now,
		nil,
	)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND resource_id = $2 AND deleted_at IS NULL ORDER BY surface ASC`)).
		WithArgs(string(runtimeauth.PublishedRuntimeResourceAgent), agentID).
		WillReturnRows(rows)

	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN \(.+\) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"surface_id",
			"subject_type",
			"subject_id",
			"enabled",
			"created_at",
			"updated_at",
			"deleted_at",
		}))
}

func expectLatestWorkflow(mock sqlmock.Sqlmock) {
	tenantID := uuid.New()
	agentID := uuid.New()
	createdBy := uuid.New()
	now := time.Now().UTC()
	graphJSON := `{"nodes":[{"data":{"type":"start","variables":[{"name":"topic","value_type":"string"}]}}],"edges":[]}`

	rows := sqlmock.NewRows([]string{
		"id",
		"tenant_id",
		"app_id",
		"agent_id",
		"type",
		"version",
		"graph",
		"features",
		"created_by",
		"created_at",
		"updated_at",
		"environment_variables",
		"conversation_variables",
		"internal",
	}).AddRow(
		uuid.NewString(),
		tenantID.String(),
		agentID.String(),
		agentID.String(),
		"workflow",
		"v1",
		graphJSON,
		`{"opening_statement":"hello"}`,
		createdBy.String(),
		now,
		now,
		"[]",
		"[]",
		false,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflows"`)).
		WithArgs(sqlmock.AnyArg(), "draft", 1).
		WillReturnRows(rows)
}

func expectConversationList(mock sqlmock.Sqlmock, userID string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "agents_conversations"`)).
		WithArgs(sqlmock.AnyArg(), "web-app", userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agents_conversations"`)).
		WithArgs(sqlmock.AnyArg(), "web-app", userID, 20).
		WillReturnRows(conversationRows(uuid.NewString(), userID))
}

func expectConversationByAgent(mock sqlmock.Sqlmock, conversationID, agentID, ownerID string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agents_conversations"`)).
		WithArgs(conversationID, agentID, 1).
		WillReturnRows(conversationRowsForAgent(conversationID, agentID, ownerID))
}

func expectConversationByAgentNotFound(mock sqlmock.Sqlmock, conversationID, agentID string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agents_conversations"`)).
		WithArgs(conversationID, agentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
}

func expectConversationMessages(mock sqlmock.Sqlmock, conversationID string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "agents_messages"`)).
		WithArgs(conversationID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agents_messages"`)).
		WithArgs(conversationID, 10000).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"agent_id",
			"conversation_id",
			"inputs",
			"query",
			"message",
			"answer",
			"currency",
			"status",
			"created_at",
			"updated_at",
		}))
}

func expectDeleteConversation(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "agents_conversations"`)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectActiveConversationCount(mock sqlmock.Sqlmock, conversationID string, count int64) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "agents_conversations"`)).
		WithArgs(conversationID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
}

func expectBuiltInWorkflows(mock sqlmock.Sqlmock) uuid.UUID {
	agentID := pkguuid.GenerateBuiltInWorkflowUUID("imagegen_chat")
	workflowID := uuid.New()
	webAppID := uuid.New()
	iconType := "text"

	rows := sqlmock.NewRows([]string{
		"agent_id",
		"agent_name",
		"workflow_id",
		"web_app_id",
		"description",
		"agent_type",
		"icon",
		"icon_type",
	}).AddRow(
		agentID.String(),
		"Image generation",
		workflowID.String(),
		webAppID.String(),
		"Generate images",
		"WORKFLOW",
		nil,
		iconType,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WithArgs("00000000-0000-0000-0000-000000000000", "00000000-0000-0000-0000-000000000000").
		WillReturnRows(rows)
	return agentID
}

func expectBuiltInWorkflowByScenario(mock sqlmock.Sqlmock, scenario string) uuid.UUID {
	agentID := pkguuid.GenerateBuiltInWorkflowUUID(scenario)
	workflowID := uuid.New()
	webAppID := uuid.New()
	iconType := "text"

	rows := sqlmock.NewRows([]string{
		"agent_id",
		"agent_name",
		"workflow_id",
		"web_app_id",
		"description",
		"agent_type",
		"icon",
		"icon_type",
	}).AddRow(
		agentID.String(),
		"Image generation",
		workflowID.String(),
		webAppID.String(),
		"Generate images",
		"WORKFLOW",
		nil,
		iconType,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WithArgs(agentID, "00000000-0000-0000-0000-000000000000", "00000000-0000-0000-0000-000000000000", 1).
		WillReturnRows(rows)
	return agentID
}

func expectSaveBuiltInWorkflowRuntimeAccountGrant(mock sqlmock.Sqlmock, agentID uuid.UUID) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND resource_id = $2 AND surface = $3 AND deleted_at IS NULL LIMIT $4`)).
		WithArgs(string(runtimeauth.PublishedRuntimeResourceBuiltinWorkflow), agentID, string(runtimeauth.PublishedRuntimeSurfaceBuiltinApp), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"resource_type",
			"resource_id",
			"organization_id",
			"workspace_id",
			"surface",
			"enabled",
			"compatibility_source",
			"created_at",
			"updated_at",
			"deleted_at",
		}))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "published_runtime_surfaces"`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "published_runtime_surface_grants" SET "deleted_at"=`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "published_runtime_surface_grants"`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func expectRuntimeGrantAccountInOrganization(mock sqlmock.Sqlmock, accountID string, count int64) {
	mock.ExpectQuery(`SELECT count\(\*\) FROM "members" WHERE organization_id = \$1 AND account_id = \$2 AND status = \$3`).
		WithArgs(sqlmock.AnyArg(), accountID, workspace_model.OrganizationMemberStatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
}

func expectRuntimeGrantDepartmentInOrganization(mock sqlmock.Sqlmock, departmentID string, count int64) {
	mock.ExpectQuery(`SELECT count\(\*\) FROM "departments" WHERE group_id = \$1 AND id = \$2 AND status = \$3`).
		WithArgs(sqlmock.AnyArg(), departmentID, workspace_model.DepartmentStatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
}

func expectBuiltInWorkflowAudienceDepartments(mock sqlmock.Sqlmock, departmentIDs []uuid.UUID) {
	rows := sqlmock.NewRows([]string{"department_id"})
	for _, departmentID := range departmentIDs {
		rows.AddRow(departmentID.String())
	}
	mock.ExpectQuery(`SELECT department_members\.department_id FROM "department_members" JOIN departments ON departments\.id = department_members\.department_id WHERE department_members\.account_id = \$1 AND departments\.group_id = \$2 AND departments\.status = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), workspace_model.DepartmentStatusActive).
		WillReturnRows(rows)
}

func expectBuiltInWorkflowAudienceWorkspaces(mock sqlmock.Sqlmock, workspaceIDs []uuid.UUID) {
	rows := sqlmock.NewRows([]string{"workspace_id"})
	for _, workspaceID := range workspaceIDs {
		rows.AddRow(workspaceID.String())
	}
	mock.ExpectQuery(`SELECT workspace_members\.workspace_id FROM "workspace_members" JOIN workspaces ON workspaces\.id = workspace_members\.workspace_id WHERE workspace_members\.account_id = \$1 AND workspaces\.organization_id = \$2 AND workspaces\.status = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), workspace_model.WorkspaceStatusNormal).
		WillReturnRows(rows)
}

func expectBuiltInWorkflowRuntimeBatchFallback(mock sqlmock.Sqlmock, agentID uuid.UUID) {
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surfaces" WHERE resource_type = \$1 AND surface = \$2 AND organization_id = \$3 AND resource_id IN \(.+\) AND deleted_at IS NULL ORDER BY resource_id ASC`).
		WithArgs(string(runtimeauth.PublishedRuntimeResourceBuiltinWorkflow), string(runtimeauth.PublishedRuntimeSurfaceBuiltinApp), sqlmock.AnyArg(), agentID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"resource_type",
			"resource_id",
			"organization_id",
			"workspace_id",
			"surface",
			"enabled",
			"compatibility_source",
			"created_at",
			"updated_at",
			"deleted_at",
		}))
}

func expectBuiltInWorkflowRuntimeBatchAccountGrant(mock sqlmock.Sqlmock, agentID, accountID uuid.UUID) {
	expectBuiltInWorkflowRuntimeBatchGrant(mock, agentID, runtimeauth.PublishedRuntimeSubjectAccount, accountID)
}

func expectBuiltInWorkflowRuntimeBatchDepartmentGrant(mock sqlmock.Sqlmock, agentID, departmentID uuid.UUID) {
	expectBuiltInWorkflowRuntimeBatchGrant(mock, agentID, runtimeauth.PublishedRuntimeSubjectDepartment, departmentID)
}

func expectBuiltInWorkflowRuntimeBatchGrant(mock sqlmock.Sqlmock, agentID uuid.UUID, subjectType runtimeauth.PublishedRuntimeSubjectType, subjectID uuid.UUID) {
	surfaceID := uuid.New()
	now := time.Now().UTC()

	surfaceRows := sqlmock.NewRows([]string{
		"id",
		"resource_type",
		"resource_id",
		"organization_id",
		"workspace_id",
		"surface",
		"enabled",
		"compatibility_source",
		"created_at",
		"updated_at",
		"deleted_at",
	}).AddRow(
		surfaceID.String(),
		string(runtimeauth.PublishedRuntimeResourceBuiltinWorkflow),
		agentID.String(),
		uuid.NewString(),
		nil,
		string(runtimeauth.PublishedRuntimeSurfaceBuiltinApp),
		true,
		runtimeauth.PublishedRuntimeSourceGrant,
		now,
		now,
		nil,
	)

	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surfaces" WHERE resource_type = \$1 AND surface = \$2 AND organization_id = \$3 AND resource_id IN \(.+\) AND deleted_at IS NULL ORDER BY resource_id ASC`).
		WithArgs(string(runtimeauth.PublishedRuntimeResourceBuiltinWorkflow), string(runtimeauth.PublishedRuntimeSurfaceBuiltinApp), sqlmock.AnyArg(), agentID).
		WillReturnRows(surfaceRows)

	grantRows := sqlmock.NewRows([]string{
		"id",
		"surface_id",
		"subject_type",
		"subject_id",
		"enabled",
		"created_at",
		"updated_at",
		"deleted_at",
	}).AddRow(
		uuid.NewString(),
		surfaceID.String(),
		string(subjectType),
		subjectID.String(),
		true,
		now,
		now,
		nil,
	)
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN \(.+\) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`).
		WithArgs(surfaceID).
		WillReturnRows(grantRows)
}

func expectBuiltInWorkflowRuntimeFallback(mock sqlmock.Sqlmock, agentID uuid.UUID) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND resource_id = $2 AND deleted_at IS NULL ORDER BY surface ASC`)).
		WithArgs(string(runtimeauth.PublishedRuntimeResourceBuiltinWorkflow), agentID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"resource_type",
			"resource_id",
			"organization_id",
			"workspace_id",
			"surface",
			"enabled",
			"compatibility_source",
			"created_at",
			"updated_at",
			"deleted_at",
		}))
}

func expectBuiltInWorkflowRuntimeAccountGrant(mock sqlmock.Sqlmock, agentID, accountID uuid.UUID) {
	surfaceID := uuid.New()
	now := time.Now().UTC()

	surfaceRows := sqlmock.NewRows([]string{
		"id",
		"resource_type",
		"resource_id",
		"organization_id",
		"workspace_id",
		"surface",
		"enabled",
		"compatibility_source",
		"created_at",
		"updated_at",
		"deleted_at",
	}).AddRow(
		surfaceID.String(),
		string(runtimeauth.PublishedRuntimeResourceBuiltinWorkflow),
		agentID.String(),
		uuid.NewString(),
		nil,
		string(runtimeauth.PublishedRuntimeSurfaceBuiltinApp),
		true,
		runtimeauth.PublishedRuntimeSourceGrant,
		now,
		now,
		nil,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND resource_id = $2 AND deleted_at IS NULL ORDER BY surface ASC`)).
		WithArgs(string(runtimeauth.PublishedRuntimeResourceBuiltinWorkflow), agentID).
		WillReturnRows(surfaceRows)

	grantRows := sqlmock.NewRows([]string{
		"id",
		"surface_id",
		"subject_type",
		"subject_id",
		"enabled",
		"created_at",
		"updated_at",
		"deleted_at",
	}).AddRow(
		uuid.NewString(),
		surfaceID.String(),
		string(runtimeauth.PublishedRuntimeSubjectAccount),
		accountID.String(),
		true,
		now,
		now,
		nil,
	)
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN \(.+\) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`).
		WillReturnRows(grantRows)
}

func expectBuiltInWorkflowRuntimeDepartmentGrant(mock sqlmock.Sqlmock, agentID, departmentID uuid.UUID) {
	surfaceID := uuid.New()
	now := time.Now().UTC()

	surfaceRows := sqlmock.NewRows([]string{
		"id",
		"resource_type",
		"resource_id",
		"organization_id",
		"workspace_id",
		"surface",
		"enabled",
		"compatibility_source",
		"created_at",
		"updated_at",
		"deleted_at",
	}).AddRow(
		surfaceID.String(),
		string(runtimeauth.PublishedRuntimeResourceBuiltinWorkflow),
		agentID.String(),
		uuid.NewString(),
		nil,
		string(runtimeauth.PublishedRuntimeSurfaceBuiltinApp),
		true,
		runtimeauth.PublishedRuntimeSourceGrant,
		now,
		now,
		nil,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND resource_id = $2 AND deleted_at IS NULL ORDER BY surface ASC`)).
		WithArgs(string(runtimeauth.PublishedRuntimeResourceBuiltinWorkflow), agentID).
		WillReturnRows(surfaceRows)

	grantRows := sqlmock.NewRows([]string{
		"id",
		"surface_id",
		"subject_type",
		"subject_id",
		"enabled",
		"created_at",
		"updated_at",
		"deleted_at",
	}).AddRow(
		uuid.NewString(),
		surfaceID.String(),
		string(runtimeauth.PublishedRuntimeSubjectDepartment),
		departmentID.String(),
		true,
		now,
		now,
		nil,
	)
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN \(.+\) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`).
		WillReturnRows(grantRows)

	mock.ExpectQuery(`SELECT department_members\.department_id FROM "department_members" JOIN departments ON departments\.id = department_members\.department_id WHERE department_members\.account_id = \$1 AND departments\.group_id = \$2 AND departments\.status = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "active").
		WillReturnRows(sqlmock.NewRows([]string{"department_id"}).AddRow(departmentID.String()))
}

func conversationRows(conversationID, ownerID string) *sqlmock.Rows {
	return conversationRowsForAgent(conversationID, uuid.NewString(), ownerID)
}

func conversationRowsForAgent(conversationID, agentID, ownerID string) *sqlmock.Rows {
	now := time.Now().UTC()
	webAppID := uuid.NewString()

	return sqlmock.NewRows([]string{
		"id",
		"agent_id",
		"mode",
		"name",
		"inputs",
		"status",
		"invoke_from",
		"web_app_id",
		"from_source",
		"from_end_user_id",
		"from_account_id",
		"dialogue_count",
		"created_by",
		"created_at",
		"updated_at",
		"deleted_at",
	}).AddRow(
		conversationID,
		agentID,
		"advanced-chat",
		"退款进度查询",
		`{"query":"你好"}`,
		"normal",
		"web-app",
		webAppID,
		"end_user",
		ownerID,
		nil,
		1,
		ownerID,
		now,
		now,
		nil,
	)
}

func workflowRoutesAuthHeader(t *testing.T, userID string) string {
	t.Helper()

	token, err := jwtpkg.GenerateTokenFixed(userID, "route test user")
	require.NoError(t, err)
	return "Bearer " + token
}

func decodeJSONBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func requireSurfaceEnabled(t *testing.T, surfaces []any, surfaceName string, wantEnabled bool) map[string]any {
	t.Helper()

	for _, item := range surfaces {
		surface, ok := item.(map[string]any)
		require.True(t, ok)
		if surface["surface"] == surfaceName {
			require.Equal(t, wantEnabled, surface["enabled"])
			return surface
		}
	}

	t.Fatalf("surface %s not found in %v", surfaceName, surfaces)
	return nil
}

func requireWebAppOfflineResponse(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()

	require.Equal(t, http.StatusForbidden, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, strconv.Itoa(response.ErrWebAppOffline.Code), body["code"])
	require.Equal(t, response.ErrWebAppOffline.Message, body["message"])
}
