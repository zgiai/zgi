package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
)

func TestProcessingExecutorHandlerListsRegisteredExecutors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := service.NewDefaultProcessingExecutorRegistry()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("organization_id", "org-1")
		c.Next()
	})
	NewProcessingExecutorHandler(registry).RegisterRoutes(router.Group(""))

	req := httptest.NewRequest(http.MethodGet, "/data-library/processing-executors", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var payload struct {
		Data struct {
			Items []service.ProcessingRequestExecutorInfo `json:"items"`
			Total int                                     `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Data.Total != 4 || len(payload.Data.Items) != 4 {
		t.Fatalf("payload=%+v", payload.Data)
	}
	for _, item := range payload.Data.Items {
		if item.Enabled {
			t.Fatalf("expected disabled default executor: %+v", item)
		}
	}
}

func TestProcessingExecutorHandlerRejectsDisabledExecutorEnqueue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	processingSvc := &fakeProcessingRequestService{}
	router := newProcessingExecutorTestRouter(service.NewDefaultProcessingExecutorRegistry(), processingSvc)

	req := httptest.NewRequest(
		http.MethodPost,
		"/data-library/processing-executors/data-library-parse-disabled/enqueue",
		bytes.NewBufferString(`{"processing_request_id":"`+uuid.NewString()+`"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if processingSvc.lastEnqueueID != uuid.Nil {
		t.Fatalf("disabled executor should not enqueue request: id=%s", processingSvc.lastEnqueueID)
	}
}

func TestProcessingExecutorHandlerEnqueuesWithEnabledExecutor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestID := uuid.New()
	executor := &registeredHandlerProcessingExecutor{
		info: service.ProcessingRequestExecutorInfo{
			Key:          "parse-worker",
			TargetLevels: []string{model.DocumentProcessingLevelParse},
			Enabled:      true,
		},
	}
	registry, err := service.NewProcessingExecutorRegistry(executor)
	if err != nil {
		t.Fatalf("NewProcessingExecutorRegistry: %v", err)
	}
	processingSvc := &fakeProcessingRequestService{
		view: &service.ProcessingRequestView{
			ID:             requestID,
			OrganizationID: "org-1",
			Status:         model.ProcessingRequestStatusQueued,
		},
	}
	router := newProcessingExecutorTestRouter(registry, processingSvc)

	req := httptest.NewRequest(
		http.MethodPost,
		"/data-library/processing-executors/parse-worker/enqueue",
		bytes.NewBufferString(`{"processing_request_id":"`+requestID.String()+`"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if processingSvc.lastEnqueueOrganizationID != "org-1" ||
		processingSvc.lastEnqueueID != requestID ||
		processingSvc.lastEnqueueExecutor == nil ||
		processingSvc.lastEnqueueExecutor.Key() != "parse-worker" {
		t.Fatalf("enqueue org=%q id=%s executor=%v", processingSvc.lastEnqueueOrganizationID, processingSvc.lastEnqueueID, processingSvc.lastEnqueueExecutor)
	}
}

func TestProcessingExecutorHandlerClaimsWithEnabledExecutor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestID := uuid.New()
	executor := &registeredHandlerProcessingExecutor{
		info: service.ProcessingRequestExecutorInfo{
			Key:          "split-worker",
			TargetLevels: []string{model.DocumentProcessingLevelSplit},
			Enabled:      true,
		},
	}
	registry, err := service.NewProcessingExecutorRegistry(executor)
	if err != nil {
		t.Fatalf("NewProcessingExecutorRegistry: %v", err)
	}
	processingSvc := &fakeProcessingRequestService{
		view: &service.ProcessingRequestView{
			ID:             requestID,
			OrganizationID: "org-1",
			Status:         model.ProcessingRequestStatusRunning,
			ExecutorKey:    "split-worker",
		},
	}
	router := newProcessingExecutorTestRouter(registry, processingSvc)

	req := httptest.NewRequest(http.MethodPost, "/data-library/processing-executors/split-worker/claim", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if processingSvc.lastClaimOrganizationID != "org-1" ||
		processingSvc.lastClaimExecutor == nil ||
		processingSvc.lastClaimExecutor.Key() != "split-worker" {
		t.Fatalf("claim org=%q executor=%v", processingSvc.lastClaimOrganizationID, processingSvc.lastClaimExecutor)
	}
}

func TestProcessingExecutorHandlerRejectsDisabledExecutorClaim(t *testing.T) {
	gin.SetMode(gin.TestMode)
	processingSvc := &fakeProcessingRequestService{}
	router := newProcessingExecutorTestRouter(service.NewDefaultProcessingExecutorRegistry(), processingSvc)

	req := httptest.NewRequest(http.MethodPost, "/data-library/processing-executors/data-library-split-disabled/claim", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if processingSvc.lastClaimExecutor != nil || processingSvc.lastClaimExecutorKey != "" {
		t.Fatalf("disabled executor should not claim: executor=%v key=%q", processingSvc.lastClaimExecutor, processingSvc.lastClaimExecutorKey)
	}
}

func TestProcessingExecutorHandlerRejectsUnsupportedExecutorTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestID := uuid.New()
	executor := &registeredHandlerProcessingExecutor{
		info: service.ProcessingRequestExecutorInfo{
			Key:          "parse-worker",
			TargetLevels: []string{model.DocumentProcessingLevelParse},
			Enabled:      true,
		},
	}
	registry, err := service.NewProcessingExecutorRegistry(executor)
	if err != nil {
		t.Fatalf("NewProcessingExecutorRegistry: %v", err)
	}
	processingSvc := &fakeProcessingRequestService{
		err: service.ErrProcessingExecutorTargetUnsupported,
	}
	router := newProcessingExecutorTestRouter(registry, processingSvc)

	req := httptest.NewRequest(
		http.MethodPost,
		"/data-library/processing-executors/parse-worker/enqueue",
		bytes.NewBufferString(`{"processing_request_id":"`+requestID.String()+`"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func newProcessingExecutorTestRouter(registry *service.ProcessingExecutorRegistry, processingSvc service.ProcessingRequestService) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("organization_id", "org-1")
		c.Next()
	})
	NewProcessingExecutorHandler(registry, processingSvc).RegisterRoutes(router.Group(""))
	return router
}

type registeredHandlerProcessingExecutor struct {
	info service.ProcessingRequestExecutorInfo
}

func (e *registeredHandlerProcessingExecutor) Key() string {
	return e.info.Key
}

func (e *registeredHandlerProcessingExecutor) Info() service.ProcessingRequestExecutorInfo {
	return e.info
}

func (e *registeredHandlerProcessingExecutor) Enqueue(ctx context.Context, req service.ProcessingExecutionRequest) error {
	return nil
}

var _ service.RegisteredProcessingRequestExecutor = (*registeredHandlerProcessingExecutor)(nil)
