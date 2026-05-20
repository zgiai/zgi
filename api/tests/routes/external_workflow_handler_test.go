package routes_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	external "github.com/zgiai/zgi/api/routes/external"
)

func TestExternalWorkflowStopTaskReturnsNotImplemented(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	handler := &external.ExternalWorkflowHandler{}
	router.POST("/api/v1/workflows/tasks/:task_id/stop", handler.StopWorkflowTask)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflows/tasks/task-1/stop", strings.NewReader(`{"user":"test-user"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}
