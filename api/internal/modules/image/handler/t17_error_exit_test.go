package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	imageservice "github.com/zgiai/zgi/api/internal/modules/image/service"
	"github.com/zgiai/zgi/api/internal/util"
)

// T17 acceptance 4/5: unwrapped image sentinels are mapped to cataloged codes,
// and the projected body carries code/retryable/error_id/request_id.
func TestT17ImageHandlerProjectsLegacySentinels(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		status    int
		code      string
		retryable bool
	}{
		{"upstream failed", imageservice.ErrUpstreamFailed, http.StatusBadGateway, "image.upstream.failed", true},
		{"task timeout", imageservice.ErrTaskTimeout, http.StatusGatewayTimeout, "image.task.timeout", true},
		{"prompt required", imageservice.ErrPromptRequired, http.StatusBadRequest, "image.request.invalid", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				util.SetOrganizationID(c, uuid.NewString())
				c.Set("account_id", uuid.NewString())
				c.Header("X-Request-ID", "req-image")
				c.Next()
			})
			newImageRuntimeHandlerForTest(t, routeBoundaryImageService{createTaskErr: tc.err}).RegisterRoutes(router.Group(""))

			request := httptest.NewRequest(http.MethodPost, "/image-runtime/generate", strings.NewReader(`{"model":"gpt-image-2","prompt":"a cat"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept-Language", "en-US")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != tc.status {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, tc.status, recorder.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["code"] != tc.code {
				t.Fatalf("code = %v, want %q", body["code"], tc.code)
			}
			if body["retryable"] != tc.retryable {
				t.Fatalf("retryable = %v, want %v", body["retryable"], tc.retryable)
			}
			if body["error_id"] == nil || body["error_id"] == "" {
				t.Fatal("missing error_id")
			}
			if body["request_id"] != "req-image" {
				t.Fatalf("request_id = %v", body["request_id"])
			}
		})
	}
}
