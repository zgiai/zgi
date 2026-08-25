package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/image/registry"
	imageservice "github.com/zgiai/zgi/api/internal/modules/image/service"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

type routeBoundaryImageService struct {
	createTaskErr error
	listTasksErr  error
	getTaskErr    error
	cancelTaskErr error
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

func (s routeBoundaryImageService) ListTasks(context.Context, imageservice.Scope, imageservice.ListTasksQuery) (*imageservice.ListTasksResult, error) {
	if s.listTasksErr != nil {
		return nil, s.listTasksErr
	}
	return &imageservice.ListTasksResult{Data: []imageservice.ImageTask{}, Total: 0, HasMore: false}, nil
}

func (s routeBoundaryImageService) GetTask(context.Context, imageservice.Scope, string) (*imageservice.ImageTask, error) {
	if s.getTaskErr != nil {
		return nil, s.getTaskErr
	}
	return &imageservice.ImageTask{TaskID: "image-task-1", Status: "pending"}, nil
}

func (s routeBoundaryImageService) CancelTask(context.Context, imageservice.Scope, string) (*imageservice.ImageTask, error) {
	if s.cancelTaskErr != nil {
		return nil, s.cancelTaskErr
	}
	return &imageservice.ImageTask{TaskID: "image-task-1", Status: "cancelled"}, nil
}

func newImageRuntimeHandlerForTest(t *testing.T, svc imageservice.Service) *Handler {
	t.Helper()
	definitions := append(appcatalog.DefaultDefinitions(), imageservice.CatalogDefinitions()...)
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatalf("compose application error catalog: %v", err)
	}
	errorProjector, err := apptransport.NewProjector(productCatalog)
	if err != nil {
		t.Fatalf("create application error projector: %v", err)
	}
	return NewHandler(svc, errorProjector)
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

	newImageRuntimeHandlerForTest(t, routeBoundaryImageService{}).RegisterRoutes(router.Group(""))

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

func TestImageRuntimeRoutesProjectApplicationErrors(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		language string
		svc      routeBoundaryImageService
		status   int
		code     string
		message  string
	}{
		{
			name:     "create task conflict",
			method:   http.MethodPost,
			path:     "/image-runtime/generate",
			body:     `{}`,
			language: "en-US",
			svc: routeBoundaryImageService{createTaskErr: apperror.Wrap(
				imageservice.ErrTaskConflict,
				imageservice.AppCodeTaskConflict,
				apperror.WithOperation("image.task.create"),
			)},
			status:  http.StatusConflict,
			code:    "image.task.conflict",
			message: "Too many image generation tasks are running. Wait for one to finish and try again.",
		},
		{
			name:     "task not found",
			method:   http.MethodGet,
			path:     "/image-runtime/tasks/missing-task",
			language: "zh-Hans",
			svc: routeBoundaryImageService{getTaskErr: apperror.Wrap(
				imageservice.ErrTaskNotFound,
				imageservice.AppCodeTaskNotFound,
				apperror.WithOperation("image.task.get"),
			)},
			status:  http.StatusNotFound,
			code:    "image.task.not_found",
			message: "未找到图片生成任务，它可能已不存在。",
		},
		{
			name:     "search too long",
			method:   http.MethodGet,
			path:     "/image-runtime/tasks?search=long",
			language: "en-US",
			svc: routeBoundaryImageService{listTasksErr: apperror.Wrap(
				imageservice.ErrSearchTooLong,
				imageservice.AppCodeSearchTooLong,
				apperror.WithOperation("image.task.list"),
			)},
			status:  http.StatusBadRequest,
			code:    "image.search.too_long",
			message: "The image task search term is too long. Shorten it and try again.",
		},
		{
			name:     "invalid cursor",
			method:   http.MethodGet,
			path:     "/image-runtime/tasks?cursor=bad",
			language: "zh-CN",
			svc: routeBoundaryImageService{listTasksErr: apperror.Wrap(
				imageservice.ErrInvalidCursor,
				imageservice.AppCodeInvalidCursor,
				apperror.WithOperation("image.task.list"),
			)},
			status:  http.StatusBadRequest,
			code:    "image.cursor.invalid",
			message: "图片任务分页游标无效，请刷新后重试。",
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
			newImageRuntimeHandlerForTest(t, tt.svc).RegisterRoutes(router.Group(""))

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept-Language", tt.language)
			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.status {
				t.Fatalf("status = %d, want %d; body: %s", recorder.Code, tt.status, recorder.Body.String())
			}
			var body struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v; body: %s", err, recorder.Body.String())
			}
			if body.Code != tt.code || body.Message != tt.message {
				t.Fatalf("body = %#v, want code=%q message=%q", body, tt.code, tt.message)
			}
		})
	}
}
