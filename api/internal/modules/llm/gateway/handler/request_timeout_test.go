package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/requesttimeout"
)

func serveGatewayRequestTimeout(t *testing.T, path string, header *string) (*httptest.ResponseRecorder, *time.Duration, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	nextCalled := false
	var requested *time.Duration
	router := gin.New()
	router.Use(GatewayRequestTimeout())
	router.POST("/*path", func(c *gin.Context) {
		nextCalled = true
		if timeout, ok := requesttimeout.Requested(c.Request.Context()); ok {
			requested = &timeout
		}
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
	if header != nil {
		request.Header.Set(requesttimeout.HeaderName, *header)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder, requested, nextCalled
}

func TestGatewayRequestTimeoutRecordsValidHeader(t *testing.T) {
	value := "2400"
	recorder, requested, next := serveGatewayRequestTimeout(t, "/v1/chat/completions", &value)
	if recorder.Code != http.StatusNoContent || !next {
		t.Fatalf("status=%d next=%v body=%s", recorder.Code, next, recorder.Body.String())
	}
	if requested == nil || *requested != 2400*time.Second {
		t.Fatalf("requested = %v, want 2400s", requested)
	}
}

func TestGatewayRequestTimeoutPassesWithoutHeader(t *testing.T) {
	recorder, requested, next := serveGatewayRequestTimeout(t, "/v1/chat/completions", nil)
	if recorder.Code != http.StatusNoContent || !next {
		t.Fatalf("status=%d next=%v", recorder.Code, next)
	}
	if requested != nil {
		t.Fatalf("requested = %s without a header", *requested)
	}
}

func TestGatewayRequestTimeoutRejectsInvalidHeader(t *testing.T) {
	paths := []struct{ path, marker string }{
		{"/v1/chat/completions", `"code":"llm.request.invalid"`},
		{"/v1/messages", `"type":"error"`},
		{"/anthropic/v1/messages", `"type":"error"`},
		{"/v1beta/models/gemini-test:generateContent", `"status":"INVALID_ARGUMENT"`},
	}
	for _, value := range []string{"", "0", "-5", "abc", "1.5", "60s", "3601"} {
		for _, test := range paths {
			raw := value
			recorder, _, next := serveGatewayRequestTimeout(t, test.path, &raw)
			if recorder.Code != http.StatusBadRequest || next {
				t.Fatalf("%s %q: status=%d next=%v body=%s", test.path, value, recorder.Code, next, recorder.Body.String())
			}
			body := recorder.Body.String()
			if !strings.Contains(body, test.marker) || !strings.Contains(body, requesttimeout.HeaderName) {
				t.Fatalf("%s %q: body = %s, want %s naming the header", test.path, value, body, test.marker)
			}
		}
	}
}
