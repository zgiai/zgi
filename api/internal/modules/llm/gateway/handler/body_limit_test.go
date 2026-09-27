package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGatewayBodyLimitRejectsAllPublicProtocolsBeforeNextMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	paths := []struct{ path, marker string }{
		{"/v1/chat/completions", `"code":"llm.request.too_large"`},
		{"/v1/responses", `"code":"llm.request.too_large"`},
		{"/v1/messages", `"type":"error"`},
		{"/v1beta/models/gemini-test:generateContent", `"status":"RESOURCE_EXHAUSTED"`},
	}
	for _, test := range paths {
		t.Run(test.path, func(t *testing.T) {
			nextCalled := false
			router := gin.New()
			router.Use(GatewayBodyLimit(8))
			router.POST("/*path", func(c *gin.Context) {
				nextCalled = true
				c.Status(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(`{"input":"too large"}`))
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusRequestEntityTooLarge || nextCalled {
				t.Fatalf("status=%d next=%v body=%s", recorder.Code, nextCalled, recorder.Body.String())
			}
			if !strings.Contains(recorder.Header().Get("Content-Type"), "application/json") {
				t.Fatalf("content type = %q", recorder.Header().Get("Content-Type"))
			}
			if !strings.Contains(recorder.Body.String(), test.marker) {
				t.Fatalf("body=%s want marker=%s", recorder.Body.String(), test.marker)
			}
		})
	}
}

func TestGatewayBodyLimitRejectsChunkedBodyWithoutContentLength(t *testing.T) {
	nextCalled := false
	router := gin.New()
	router.Use(GatewayBodyLimit(4))
	router.POST("/v1/responses", func(c *gin.Context) { nextCalled = true })
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString("12345"))
	request.ContentLength = -1
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge || nextCalled {
		t.Fatalf("status=%d next=%v body=%s", recorder.Code, nextCalled, recorder.Body.String())
	}
}
