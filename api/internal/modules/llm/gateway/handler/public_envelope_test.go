package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// assertPublicErrorEnvelope asserts the seven cataloged fields every public
// error exit must carry, plus the application-error header. protocol selects
// the shape: "anthropic" nests the identity under error, "gemini" uses a numeric
// code, and the OpenAI shape keeps the legacy param field.
func assertPublicErrorEnvelope(t *testing.T, recorder *httptest.ResponseRecorder, protocol, wantCode string, wantStatus int) {
	t.Helper()
	if recorder.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, wantStatus, recorder.Body.String())
	}
	if got := recorder.Header().Get("X-ZGI-App-Error-Code"); got != wantCode {
		t.Fatalf("X-ZGI-App-Error-Code = %q, want %q; body = %s", got, wantCode, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	for _, field := range []string{"code", "error_code", "message", "params", "retryable", "error_id", "request_id"} {
		if _, ok := body[field]; !ok {
			t.Fatalf("body is missing %q: %s", field, recorder.Body.String())
		}
	}
	if body["code"] != wantCode || body["error_code"] != wantCode {
		t.Fatalf("code/error_code = %v/%v, want %q", body["code"], body["error_code"], wantCode)
	}
	if body["request_id"] != "req-envelope-test" {
		t.Fatalf("request_id = %v, want req-envelope-test", body["request_id"])
	}
	if id, _ := body["error_id"].(string); id == "" {
		t.Fatalf("error_id is empty: %s", recorder.Body.String())
	}
	nested, _ := body["error"].(map[string]any)
	if nested == nil {
		t.Fatalf("body has no error object: %s", recorder.Body.String())
	}
	switch protocol {
	case "anthropic":
		if _, ok := nested["type"]; !ok {
			t.Fatalf("anthropic shape lost error.type: %s", recorder.Body.String())
		}
		if body["type"] != "error" {
			t.Fatalf("anthropic shape lost the top-level type: %s", recorder.Body.String())
		}
	case "gemini":
		if _, ok := nested["status"]; !ok {
			t.Fatalf("gemini shape lost error.status: %s", recorder.Body.String())
		}
	case "openai":
		if _, ok := nested["param"]; !ok {
			t.Fatalf("openai shape lost error.param: %s", recorder.Body.String())
		}
	}
}

func envelopeTestContext(t *testing.T, method, target string, anthropicVersion bool) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, nil)
	if anthropicVersion {
		c.Request.Header.Set("anthropic-version", "2023-06-01")
	}
	c.Set("request_id", "req-envelope-test")
	return c, recorder
}

// TestModelsDetailNotFoundCarriesThePublicEnvelope covers the two catalog
// detail 404 shapes. Both used to answer without the catalog identity, the
// X-ZGI-App-Error-Code header, or (for Anthropic) any envelope field at all.
func TestModelsDetailNotFoundCarriesThePublicEnvelope(t *testing.T) {
	t.Run("openai shape", func(t *testing.T) {
		c, recorder := envelopeTestContext(t, http.MethodGet, "/v1/models/nope", false)
		c.Params = gin.Params{{Key: "model_id", Value: "nope"}}
		writeModelsDetail(c, preCatalogTestModels())
		assertPublicErrorEnvelope(t, recorder, "openai", "llm.model.not_found", http.StatusNotFound)
	})

	t.Run("anthropic shape", func(t *testing.T) {
		c, recorder := envelopeTestContext(t, http.MethodGet, "/v1/models/nope", true)
		c.Params = gin.Params{{Key: "model_id", Value: "nope"}}
		writeModelsDetail(c, preCatalogTestModels())
		assertPublicErrorEnvelope(t, recorder, "anthropic", "llm.model.not_found", http.StatusNotFound)
	})
}

// TestAnthropicModelsListValidationCarriesThePublicEnvelope covers the
// limit/after_id/before_id 400s, which previously had no contract fields.
func TestAnthropicModelsListValidationCarriesThePublicEnvelope(t *testing.T) {
	c, recorder := envelopeTestContext(t, http.MethodGet, "/v1/models?limit=99999", true)
	writeModelsCatalog(c, preCatalogTestModels())
	assertPublicErrorEnvelope(t, recorder, "anthropic", "llm.request.invalid", http.StatusBadRequest)
}

// TestRawProtocolFallbackCarriesThePublicEnvelope covers an upstream failure
// that carried no body: every protocol shape must still publish the cataloged
// fields and header.
func TestRawProtocolFallbackCarriesThePublicEnvelope(t *testing.T) {
	cases := []struct {
		protocol string
		target   string
		status   int
		wantCode string
	}{
		{"openai", "/v1/chat/completions", http.StatusBadRequest, "llm.request.invalid"},
		{"anthropic", "/v1/messages", http.StatusBadRequest, "llm.request.invalid"},
		{"gemini", "/v1beta/models/x:generateContent", http.StatusBadRequest, "llm.request.invalid"},
		{"openai", "/v1/chat/completions", http.StatusUnauthorized, "llm.provider.auth_failed"},
		{"openai", "/v1/chat/completions", http.StatusTooManyRequests, "llm.provider.rate_limited"},
		{"openai", "/v1/chat/completions", http.StatusBadGateway, "llm.invocation.failed"},
	}
	for _, testCase := range cases {
		t.Run(testCase.protocol+" "+http.StatusText(testCase.status), func(t *testing.T) {
			c, recorder := envelopeTestContext(t, http.MethodPost, testCase.target, false)
			writeRawProtocolFallback(c, testCase.status, testCase.protocol)
			assertPublicErrorEnvelope(t, recorder, testCase.protocol, testCase.wantCode, testCase.status)
		})
	}
}
