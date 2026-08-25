package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/image/registry"
	imageservice "github.com/zgiai/zgi/api/internal/modules/image/service"
	"github.com/zgiai/zgi/api/internal/util"
)

type routeBoundaryImageService struct {
	createTaskErr error
	getTaskErr    error
}

func (routeBoundaryImageService) ListModels(context.Context, imageservice.Scope) ([]registry.ImageModel, error) {
	return []registry.ImageModel{}, nil
}

func (routeBoundaryImageService) Generate(context.Context, imageservice.Scope, imageservice.GenerateRequest) (*imageservice.GenerateResult, error) {
	return nil, nil
}

func (s routeBoundaryImageService) CreateTask(context.Context, imageservice.Scope, imageservice.GenerateRequest) (*imageservice.CreateTaskResult, error) {
	if s.createTaskErr != nil {
		return nil, s.createTaskErr
	}
	return &imageservice.CreateTaskResult{Task: imageservice.ImageTask{TaskID: "image-task-1", Status: "pending"}}, nil
}

func (routeBoundaryImageService) ListTasks(context.Context, imageservice.Scope, imageservice.ListTasksQuery) (*imageservice.ListTasksResult, error) {
	return &imageservice.ListTasksResult{Data: []imageservice.ImageTask{}, Total: 0, HasMore: false}, nil
}

func (s routeBoundaryImageService) GetTask(context.Context, imageservice.Scope, string) (*imageservice.ImageTask, error) {
	if s.getTaskErr != nil {
		return nil, s.getTaskErr
	}
	return &imageservice.ImageTask{TaskID: "image-task-1", Status: "pending"}, nil
}

func (routeBoundaryImageService) CancelTask(context.Context, imageservice.Scope, string) (*imageservice.ImageTask, error) {
	return &imageservice.ImageTask{TaskID: "image-task-1", Status: "cancelled"}, nil
}

func TestRegisterRoutesAllowsOrganizationScopedImageRuntimeRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	router.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, organizationID)
		c.Set("account_id", accountID)
		c.Next()
	})

	NewHandler(routeBoundaryImageService{}).RegisterRoutes(router.Group(""))

	modelsRecorder := httptest.NewRecorder()
	modelsRequest := httptest.NewRequest(http.MethodGet, "/image-runtime/models", nil)
	router.ServeHTTP(modelsRecorder, modelsRequest)
	if modelsRecorder.Code != http.StatusOK {
		t.Fatalf("models status = %d, want %d", modelsRecorder.Code, http.StatusOK)
	}
	generateRecorder := httptest.NewRecorder()
	generateRequest := httptest.NewRequest(http.MethodPost, "/image-runtime/generate", strings.NewReader(`{}`))
	generateRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(generateRecorder, generateRequest)
	if generateRecorder.Code != http.StatusOK {
		t.Fatalf("generate status = %d, want %d", generateRecorder.Code, http.StatusOK)
	}
	tasksRecorder := httptest.NewRecorder()
	tasksRequest := httptest.NewRequest(http.MethodGet, "/image-runtime/tasks", nil)
	router.ServeHTTP(tasksRecorder, tasksRequest)
	if tasksRecorder.Code != http.StatusOK {
		t.Fatalf("tasks status = %d, want %d", tasksRecorder.Code, http.StatusOK)
	}
	taskRecorder := httptest.NewRecorder()
	taskRequest := httptest.NewRequest(http.MethodGet, "/image-runtime/tasks/image-task-1", nil)
	router.ServeHTTP(taskRecorder, taskRequest)
	if taskRecorder.Code != http.StatusOK {
		t.Fatalf("task status = %d, want %d", taskRecorder.Code, http.StatusOK)
	}
	cancelRecorder := httptest.NewRecorder()
	cancelRequest := httptest.NewRequest(http.MethodPost, "/image-runtime/tasks/image-task-1/cancel", nil)
	router.ServeHTTP(cancelRecorder, cancelRequest)
	if cancelRecorder.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, want %d", cancelRecorder.Code, http.StatusOK)
	}
}

func TestImageRuntimeRoutesMapTaskErrors(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		svc    routeBoundaryImageService
		status int
		code   string
	}{
		{
			name:   "create task conflict",
			method: http.MethodPost,
			path:   "/image-runtime/generate",
			body:   `{}`,
			svc:    routeBoundaryImageService{createTaskErr: imageservice.ErrTaskConflict},
			status: http.StatusConflict,
			code:   "IMAGE_TASK_CONFLICT",
		},
		{
			name:   "task not found",
			method: http.MethodGet,
			path:   "/image-runtime/tasks/missing-task",
			svc:    routeBoundaryImageService{getTaskErr: imageservice.ErrTaskNotFound},
			status: http.StatusNotFound,
			code:   "IMAGE_TASK_NOT_FOUND",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				util.SetOrganizationID(c, uuid.NewString())
				c.Set("account_id", uuid.NewString())
				c.Next()
			})
			NewHandler(tt.svc).RegisterRoutes(router.Group(""))

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.status {
				t.Fatalf("status = %d, want %d; body: %s", recorder.Code, tt.status, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"code":"`+tt.code+`"`) {
				t.Fatalf("body = %s, want code %s", recorder.Body.String(), tt.code)
			}
		})
	}
}
