package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/statistics/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/statistics/service"
)

type fakeStatisticsService struct {
	modelUsageErr error
	modelUsageReq *dto.ModelUsageRequest
}

func (f *fakeStatisticsService) GetModelUsage(_ context.Context, _ string, req *dto.ModelUsageRequest) (*dto.ModelUsageResponse, error) {
	f.modelUsageReq = req
	return &dto.ModelUsageResponse{}, f.modelUsageErr
}

func (f *fakeStatisticsService) GetWorkspaceQuota(context.Context, string, *dto.WorkspaceQuotaRequest) (*dto.WorkspaceQuotaResponse, error) {
	return nil, nil
}

func TestGetModelUsage_ReturnsBadRequestForValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewStatisticsHandler(&fakeStatisticsService{
		modelUsageErr: service.ErrInvalidTimestamp,
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("organization_id", uuid.NewString())
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/console/api/llm/statistics/model-usage?start_time=1710000000000&end_time=1710086400",
		nil,
	)

	h.GetModelUsage(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestGetModelUsage_AllowsAIChatAppType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeSvc := &fakeStatisticsService{}
	h := NewStatisticsHandler(fakeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("organization_id", uuid.NewString())
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/console/api/llm/statistics/model-usage?start_time=1710000000&end_time=1710086400&app_type=aichat",
		nil,
	)

	h.GetModelUsage(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", w.Code, w.Body.String())
	}
	if fakeSvc.modelUsageReq == nil || fakeSvc.modelUsageReq.AppType == nil || *fakeSvc.modelUsageReq.AppType != "aichat" {
		t.Fatalf("app_type = %#v, want aichat", fakeSvc.modelUsageReq)
	}
}

func TestGetModelUsage_RejectsInvalidAppType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeSvc := &fakeStatisticsService{}
	h := NewStatisticsHandler(fakeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("organization_id", uuid.NewString())
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/console/api/llm/statistics/model-usage?start_time=1710000000&end_time=1710086400&app_type=invalid",
		nil,
	)

	h.GetModelUsage(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", w.Code, w.Body.String())
	}
	if fakeSvc.modelUsageReq != nil {
		t.Fatalf("service should not receive invalid app_type")
	}
}
