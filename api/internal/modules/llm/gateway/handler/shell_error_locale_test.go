package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// The two shell-layer rejections that run before the API key is resolved, an
// oversized body and a missing key header, used to answer in fixed English
// whatever Accept-Language said, while the unknown-key rejection beside them
// was already localized. Both now take the catalog message of their code.
func TestShellRejectionsAnswerInCallerLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	projector := testApplicationErrorProjector(t)
	router := gin.New()
	v1 := router.Group("/v1")
	v1.Use(GatewayBodyLimit(16, projector))
	v1.Use(LLMAPIKeyAuthMiddleware(nil, projector))
	v1.POST("/chat/completions", func(c *gin.Context) { c.Status(http.StatusOK) })

	call := func(language, auth, body string) (int, map[string]interface{}) {
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept-Language", language)
		if auth != "" {
			request.Header.Set("Authorization", auth)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		var payload map[string]interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode %s: %v", recorder.Body.String(), err)
		}
		return recorder.Code, payload
	}

	for _, tc := range []struct {
		name, auth, body string
		status           int
		code             string
		zh, en           string
	}{
		{"too large", "Bearer sk-x", strings.Repeat("a", 64), http.StatusRequestEntityTooLarge, "llm.request.too_large",
			"请求体过大，请缩减请求内容后重试。", "The request body is too large. Reduce the payload size and try again."},
		{"missing key header", "", "{}", http.StatusUnauthorized, "llm.api_key.invalid",
			"API 密钥无效，请检查密钥后重试。", "The API key is invalid. Check the key and try again."},
	} {
		status, zh := call("zh-CN", tc.auth, tc.body)
		if status != tc.status || zh["error_code"] != tc.code || zh["message"] != tc.zh {
			t.Fatalf("%s zh-CN = %d %v %q, want %d %s %q", tc.name, status, zh["error_code"], zh["message"], tc.status, tc.code, tc.zh)
		}
		_, en := call("en", tc.auth, tc.body)
		if en["message"] != tc.en {
			t.Fatalf("%s en message = %q, want %q", tc.name, en["message"], tc.en)
		}
	}
}
