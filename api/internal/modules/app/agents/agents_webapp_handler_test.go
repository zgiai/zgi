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
	"github.com/zgiai/zgi/api/internal/dto"
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

	called         bool
	agentID        string
	req            dto.UpdateWebAppStatusRequest
	accountID      string
	organizationID string
}

func (s *stubWebAppStatusHandlerService) GetAgentsList(context.Context, string, string, interface{}) (interface{}, error) {
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) GetAgentsListMultipleTenants(context.Context, string, []string, interface{}) (interface{}, error) {
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) GetInternalAgentsList(context.Context, string, []string, interface{}) (interface{}, error) {
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) GetAgentsListWithPermissions(context.Context, string, dto.GetAgentsListRequest) (*dto.AgentsListResponse, error) {
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) GetRunnableWebApps(context.Context, string, dto.GetRunnableWebAppsRequest) (*dto.RunnableWebAppsResponse, error) {
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) CreateAgent(context.Context, string, interface{}, string) (interface{}, error) {
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) GetAgent(context.Context, string) (interface{}, error) {
	return nil, nil
}

func (s *stubWebAppStatusHandlerService) UpdateAgent(context.Context, string, interface{}) (interface{}, error) {
	return nil, nil
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
