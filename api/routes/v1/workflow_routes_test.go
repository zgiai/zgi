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
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/queue"
	"github.com/zgiai/zgi/api/pkg/response"
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

func newWorkflowRoutesTestRouter(t *testing.T) (*gin.Engine, string, string) {
	t.Helper()

	router, webAppID, userID, _ := newWorkflowRoutesTestRouterWithConversation(t)
	return router, webAppID, userID
}

func newWorkflowRoutesTestRouterWithConversation(t *testing.T) (*gin.Engine, string, string, string) {
	t.Helper()

	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Platform: config.PlatformConfig{Edition: "TEST"},
		Storage: config.StorageConfig{
			Type:            "local",
			OpenDALBasePath: t.TempDir(),
		},
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
	})

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

	router := gin.New()
	v1Group := router.Group("/console/api")
	RegisterWorkflowRoutes(v1Group, nil, nil, nil, db, nil, nil, nil, nil, nil, nil, nil, nil, taskManager, workflowRoutesTaskRegistry{}, nil, nil, nil)
	return router, webAppID, userID, conversationID
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
	case "TestWorkflowRoutes_RunRejectsInactiveWebApp",
		"TestWorkflowRoutes_ConversationListRejectsInactiveWebApp",
		"TestWorkflowRoutes_PrecheckRejectsInactiveWebApp":
		expectAgentByWebAppWithStatus(mock, webAppID, "inactive")
	case "TestWorkflowRoutes_ConversationListAcceptsUUIDBearerIdentity":
		expectAgentByWebApp(mock, webAppID)
		expectConversationList(mock, userID)
	case "TestWorkflowRoutes_RunAcceptsUUIDBearerIdentity":
		expectAgentByWebApp(mock, webAppID)
		expectLatestWorkflow(mock)
	case "TestWorkflowRoutes_ConversationDetailAcceptsOwner":
		expectAgentByWebApp(mock, webAppID)
		expectConversation(mock, conversationID, userID)
		expectConversationMessages(mock, conversationID)
	case "TestWorkflowRoutes_ConversationDetailRejectsOtherUser",
		"TestWorkflowRoutes_ConversationDeleteRejectsOtherUser":
		expectAgentByWebApp(mock, webAppID)
		expectConversation(mock, conversationID, uuid.NewString())
		if t.Name() == "TestWorkflowRoutes_ConversationDeleteRejectsOtherUser" {
			expectActiveConversationCount(mock, conversationID, 1)
		}
	case "TestWorkflowRoutes_ConversationDeleteAcceptsOwner":
		expectAgentByWebApp(mock, webAppID)
		expectConversation(mock, conversationID, userID)
		expectDeleteConversation(mock)
		expectActiveConversationCount(mock, conversationID, 0)
	case "TestWorkflowRoutes_PrecheckAcceptsUUIDBearerIdentityWithEmptyBody",
		"TestWorkflowRoutes_PrecheckIgnoresLegacyRequestBody":
		expectAgentByWebApp(mock, webAppID)
		expectLatestWorkflow(mock)
	}
}

func expectAgentByWebApp(mock sqlmock.Sqlmock, webAppID string) {
	expectAgentByWebAppWithStatus(mock, webAppID, "active")
}

func expectAgentByWebAppWithStatus(mock sqlmock.Sqlmock, webAppID, webAppStatus string) {
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

func expectConversation(mock sqlmock.Sqlmock, conversationID, ownerID string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agents_conversations"`)).
		WithArgs(conversationID, 1).
		WillReturnRows(conversationRows(conversationID, ownerID))
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

func conversationRows(conversationID, ownerID string) *sqlmock.Rows {
	now := time.Now().UTC()
	agentID := uuid.New()
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
		agentID.String(),
		"advanced-chat",
		"hello conversation",
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

func decodeJSONBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func requireWebAppOfflineResponse(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()

	require.Equal(t, http.StatusForbidden, w.Code)

	body := decodeJSONBody(t, w)
	require.Equal(t, strconv.Itoa(response.ErrWebAppOffline.Code), body["code"])
	require.Equal(t, response.ErrWebAppOffline.Message, body["message"])
}
