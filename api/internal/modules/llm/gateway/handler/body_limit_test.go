package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
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

// TestGatewayBodyLimitCarriesCatalogEnvelope pins the shell-layer 413/400 to the
// same public contract as every projected protocol error: the app-code header
// plus the seven top-level envelope fields, on every protocol shape.
func TestGatewayBodyLimitCarriesCatalogEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		path, body string
		limit      int64
		status     int
		code       string
	}{
		{"/v1/chat/completions", `{"input":"too large"}`, 8, http.StatusRequestEntityTooLarge, "llm.request.too_large"},
		{"/v1/messages", `{"input":"too large"}`, 8, http.StatusRequestEntityTooLarge, "llm.request.too_large"},
		{"/anthropic/v1/messages", `{"input":"too large"}`, 8, http.StatusRequestEntityTooLarge, "llm.request.too_large"},
		{"/v1beta/models/gemini-test:generateContent", `{"input":"too large"}`, 8, http.StatusRequestEntityTooLarge, "llm.request.too_large"},
	}
	for _, test := range cases {
		t.Run(test.path, func(t *testing.T) {
			router := gin.New()
			router.Use(withTestRequestID, GatewayBodyLimit(test.limit))
			router.POST("/*path", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body)))
			assertGatewayShellEnvelope(t, recorder, test.status, test.code)
		})
	}

	t.Run("unreadable body is a cataloged 400", func(t *testing.T) {
		router := gin.New()
		router.Use(withTestRequestID, GatewayBodyLimit(1024))
		router.POST("/*path", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		request := httptest.NewRequest(http.MethodPost, "/v1/messages", failingReader{})
		request.ContentLength = -1
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		assertGatewayShellEnvelope(t, recorder, http.StatusBadRequest, "llm.request.invalid")
	})
}

func withTestRequestID(c *gin.Context) {
	c.Header("X-Request-ID", "req-body-limit")
	c.Next()
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func assertGatewayShellEnvelope(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, status, recorder.Body.String())
	}
	if got := recorder.Header().Get("X-ZGI-App-Error-Code"); got != code {
		t.Fatalf("X-ZGI-App-Error-Code = %q, want %q", got, code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (%s)", err, recorder.Body.String())
	}
	for _, field := range []string{"code", "error_code", "message", "params", "retryable", "error_id", "request_id"} {
		if _, ok := body[field]; !ok {
			t.Fatalf("body is missing %q: %s", field, recorder.Body.String())
		}
	}
	if body["code"] != code || body["error_code"] != code || body["retryable"] != false {
		t.Fatalf("envelope = %v, want code %s retryable=false", body, code)
	}
}

func TestGatewayRequestBodyLimitDefaultsTo100MiB(t *testing.T) {
	if defaultGatewayRequestBodyLimit != 100<<20 {
		t.Fatalf("default limit = %d, want %d", defaultGatewayRequestBodyLimit, 100<<20)
	}
	if got := resolveGatewayRequestBodyLimit("", failOnWarn(t)); got != 100<<20 {
		t.Fatalf("empty setting = %d, want %d", got, 100<<20)
	}
}

func TestGatewayRequestBodyLimitUsesConfiguredMiB(t *testing.T) {
	cases := map[string]int64{"1": 1 << 20, "64": 64 << 20, " 200 ": 200 << 20}
	for raw, want := range cases {
		if got := resolveGatewayRequestBodyLimit(raw, failOnWarn(t)); got != want {
			t.Fatalf("setting %q = %d, want %d", raw, got, want)
		}
	}
}

func TestGatewayRequestBodyLimitInvalidFallsBackToDefaultAndWarns(t *testing.T) {
	for _, raw := range []string{"abc", "0", "-5", "1.5", "100MB", "9223372036854775807"} {
		t.Run(raw, func(t *testing.T) {
			var warnings []string
			warn := func(msg string, args ...interface{}) {
				warnings = append(warnings, msg)
				if !containsArg(args, GatewayRequestBodyLimitEnv) || !containsArg(args, raw) {
					t.Fatalf("warning args %v do not name env/value", args)
				}
			}
			if got := resolveGatewayRequestBodyLimit(raw, warn); got != defaultGatewayRequestBodyLimit {
				t.Fatalf("setting %q = %d, want default %d", raw, got, defaultGatewayRequestBodyLimit)
			}
			if len(warnings) != 1 {
				t.Fatalf("warnings = %v, want exactly one", warnings)
			}
		})
	}
}

// TestGatewayBodyLimitDefaultAcceptsAboveOld10MiB pins the ZGI-07 fix: a body
// just above the former 10 MiB ceiling reaches the next handler intact, and a
// declared length above the new default is still rejected before it is read.
func TestGatewayBodyLimitDefaultAcceptsAboveOld10MiB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var received int
	router := gin.New()
	router.Use(GatewayBodyLimit(0))
	router.POST("/*path", func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		received = len(body)
		c.Status(http.StatusNoContent)
	})

	size := 11 << 20
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(make([]byte, size))))
	if recorder.Code != http.StatusNoContent || received != size {
		t.Fatalf("status=%d received=%d want=%d", recorder.Code, received, size)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}"))
	request.ContentLength = defaultGatewayRequestBodyLimit + 1
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("over-default status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

// TestGatewayBodyLimitConfiguredLimitKeepsCatalogEnvelope checks that a limit
// resolved from the setting rejects with the same 413 contract as before.
func TestGatewayBodyLimitConfiguredLimitKeepsCatalogEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limit := resolveGatewayRequestBodyLimit("1", failOnWarn(t))
	nextCalled := false
	router := gin.New()
	router.Use(withTestRequestID, GatewayBodyLimit(limit))
	router.POST("/*path", func(c *gin.Context) { nextCalled = true })

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(make([]byte, (1<<20)+1)))
	request.ContentLength = -1
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if nextCalled {
		t.Fatal("next handler ran for an over-limit body")
	}
	assertGatewayShellEnvelope(t, recorder, http.StatusRequestEntityTooLarge, "llm.request.too_large")
}

func failOnWarn(t *testing.T) func(string, ...interface{}) {
	t.Helper()
	return func(msg string, args ...interface{}) { t.Fatalf("unexpected warning %q %v", msg, args) }
}

func containsArg(args []interface{}, want interface{}) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
