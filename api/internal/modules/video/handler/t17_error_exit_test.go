package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	videoservice "github.com/zgiai/zgi/api/internal/modules/video/service"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

type stubVideoService struct {
	videoservice.Service
	getTaskErr error
}

func (s *stubVideoService) GetTask(context.Context, videoservice.Scope, string) (*videoservice.VideoTask, error) {
	if s.getTaskErr != nil {
		return nil, s.getTaskErr
	}
	return &videoservice.VideoTask{}, nil
}

func newVideoTestProjector(t *testing.T) *apptransport.Projector {
	t.Helper()
	definitions := append(appcatalog.DefaultDefinitions(), videoservice.CatalogDefinitions()...)
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(productCatalog)
	if err != nil {
		t.Fatal(err)
	}
	return projector
}

func newVideoTestContext(language string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/console/api/video-runtime/tasks/task-1", nil)
	context.Request.Header.Set("Accept-Language", language)
	context.Set("account_id", "11111111-1111-1111-1111-111111111111")
	context.Set("organization_id", "22222222-2222-2222-2222-222222222222")
	context.Header("X-Request-ID", "req-video")
	context.Params = gin.Params{{Key: "task_id", Value: "task-1"}}
	return context, recorder
}

// T17 acceptance 4: the video handler projects through the injected Projector
// and never returns the raw service error text.
func TestT17VideoHandlerProjectsAppError(t *testing.T) {
	handler := NewHandler(&stubVideoService{getTaskErr: videoservice.ErrTaskNotFound}, newVideoTestProjector(t))
	context, recorder := newVideoTestContext("zh-CN")

	handler.GetTask(context)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "video.task.not_found" {
		t.Fatalf("code = %v", body["code"])
	}
	if body["error_id"] == nil || body["error_id"] == "" {
		t.Fatal("missing error_id")
	}
	if body["request_id"] != "req-video" {
		t.Fatalf("request_id = %v", body["request_id"])
	}
	message, _ := body["message"].(string)
	if !strings.Contains(message, "视频") {
		t.Fatalf("message not localized: %q", message)
	}
}

func TestT17VideoHandlerDoesNotLeakRawError(t *testing.T) {
	handler := NewHandler(&stubVideoService{getTaskErr: errors.New("upstream secret sk-live-123")}, newVideoTestProjector(t))
	context, recorder := newVideoTestContext("en-US")

	handler.GetTask(context)

	if strings.Contains(recorder.Body.String(), "sk-live-123") {
		t.Fatalf("raw error leaked: %s", recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "video.task.failed" {
		t.Fatalf("code = %v", body["code"])
	}
	if body["retryable"] != true {
		t.Fatalf("retryable = %v", body["retryable"])
	}
}

func TestT17VideoHandlerRequiresProjector(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewHandler must reject a nil projector")
		}
	}()
	NewHandler(&stubVideoService{}, nil)
}
