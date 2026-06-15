package routes_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/middleware"
	external "github.com/zgiai/zgi/api/routes/external"
)

func TestExternalWorkflowStopTaskRequiresAPIKeyInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	handler := &external.ExternalWorkflowHandler{}
	router.POST("/api/v1/workflows/tasks/:task_id/stop", handler.StopWorkflowTask)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflows/tasks/task-1/stop", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestExternalWorkflowStopTaskDoesNotRequireUserBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("api_key_info", &middleware.APIKeyInfo{
			ID:       uuid.New(),
			AgentID:  uuid.New(),
			TenantID: uuid.New(),
		})
		c.Next()
	})
	handler := &external.ExternalWorkflowHandler{}
	router.POST("/api/v1/workflows/tasks/:task_id/stop", handler.StopWorkflowTask)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflows/tasks/task-1/stop", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusBadRequest {
		t.Fatalf("status code = %d, stop endpoint should not require user body", rec.Code)
	}
}
