package fxapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/observability"
)

func TestProvideGinEngine_AllowsSSEConnectionHeaderPreflight(t *testing.T) {
	previousConfig := config.GlobalConfig
	defer func() {
		config.GlobalConfig = previousConfig
	}()

	const origin = "https://c-cloud.zgi.im"
	config.GlobalConfig = &config.Config{
		Server: config.ServerConfig{
			Mode:             gin.TestMode,
			CORSAllowOrigins: []string{origin},
		},
	}

	engine := provideGinEngine(config.GlobalConfig, observability.NewZGIReporter(), &OpenTelemetryResource{})
	req := httptest.NewRequest(http.MethodOptions, "/console/api/aichat/chat", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "authorization,cache-control,connection,content-type")

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	allowHeaders := strings.ToLower(recorder.Header().Get("Access-Control-Allow-Headers"))
	if !strings.Contains(allowHeaders, "connection") {
		t.Fatalf("Access-Control-Allow-Headers = %q, want connection", allowHeaders)
	}
}

func TestProvideGinEngine_ExposesAgentSSEIdentityHeaders(t *testing.T) {
	previousConfig := config.GlobalConfig
	defer func() {
		config.GlobalConfig = previousConfig
	}()

	const origin = "https://c-cloud.zgi.im"
	config.GlobalConfig = &config.Config{
		Server: config.ServerConfig{
			Mode:             gin.TestMode,
			CORSAllowOrigins: []string{origin},
		},
	}

	engine := provideGinEngine(config.GlobalConfig, observability.NewZGIReporter(), &OpenTelemetryResource{})
	engine.GET("/stream", func(c *gin.Context) {
		c.Header("X-ZGI-Conversation-ID", "conversation-id")
		c.Header("X-ZGI-Message-ID", "message-id")
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	req.Header.Set("Origin", origin)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	exposed := strings.ToLower(recorder.Header().Get("Access-Control-Expose-Headers"))
	for _, header := range []string{"x-zgi-app-error-code", "x-zgi-conversation-id", "x-zgi-message-id"} {
		if !strings.Contains(exposed, header) {
			t.Fatalf("Access-Control-Expose-Headers = %q, want %q", exposed, header)
		}
	}
}

func TestProvideGinEngine_AllowsConnectorMutationPreflights(t *testing.T) {
	previousConfig := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = previousConfig })
	const origin = "http://localhost:3000"
	config.GlobalConfig = &config.Config{
		Server: config.ServerConfig{Mode: gin.TestMode, CORSAllowOrigins: []string{origin}},
	}
	engine := provideGinEngine(config.GlobalConfig, observability.NewZGIReporter(), &OpenTelemetryResource{})
	for _, test := range []struct {
		name, method, path, header string
	}{
		{"github key setup", http.MethodPost, "/console/api/connectors/github/key-setups", "Idempotency-Key"},
		{"search key setup", http.MethodPost, "/console/api/connectors/web-search/key-setups", "Idempotency-Key"},
		{"reconnect", http.MethodPost, "/console/api/connector-connections/connection-id/reconnect", "Idempotency-Key"},
		{"attach binding", http.MethodPost, "/console/api/connector-target-bindings", "Idempotency-Key"},
		{"forced removal", http.MethodDelete, "/console/api/connector-connections/connection-id", "X-Impact-Token"},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, test.path, nil)
			req.Header.Set("Origin", origin)
			req.Header.Set("Access-Control-Request-Method", test.method)
			req.Header.Set("Access-Control-Request-Headers", "authorization,content-type,"+strings.ToLower(test.header))
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusNoContent {
				t.Fatalf("preflight status = %d, want %d", recorder.Code, http.StatusNoContent)
			}
			if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != origin {
				t.Fatalf("allowed origin = %q, want %q", got, origin)
			}
			if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
				t.Fatalf("allowed credentials = %q, want true", got)
			}
			// A 204 alone is not enough: browsers reject the actual request if
			// any requested method or header is absent from the allowlists.
			if !corsHeaderListContains(recorder.Header().Get("Access-Control-Allow-Methods"), test.method) {
				t.Fatalf("preflight did not allow method %s", test.method)
			}
			for _, header := range []string{"Authorization", "Content-Type", test.header} {
				if !corsHeaderListContains(recorder.Header().Get("Access-Control-Allow-Headers"), header) {
					t.Errorf("preflight did not allow request header %s", header)
				}
			}
		})
	}
}

func TestProvideGinEngine_ConnectorCORSKeepsExplicitAllowlists(t *testing.T) {
	previousConfig := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = previousConfig })
	const origin = "http://localhost:3000"
	config.GlobalConfig = &config.Config{
		Server: config.ServerConfig{Mode: gin.TestMode, CORSAllowOrigins: []string{origin}},
	}
	engine := provideGinEngine(config.GlobalConfig, observability.NewZGIReporter(), &OpenTelemetryResource{})
	handlerCalled := false
	engine.POST("/console/api/connectors/github/key-setups", func(c *gin.Context) {
		handlerCalled = true
		c.Status(http.StatusNoContent)
	})
	for _, method := range []string{http.MethodOptions, http.MethodPost} {
		req := httptest.NewRequest(method, "/console/api/connectors/github/key-setups", nil)
		req.Header.Set("Origin", "https://untrusted.example")
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		req.Header.Set("Access-Control-Request-Headers", "idempotency-key")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusForbidden || handlerCalled {
			t.Fatalf("untrusted %s reached the handler or was not rejected: status=%d", method, recorder.Code)
		}
		if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("untrusted origin received an allow-origin header: %q", got)
		}
	}
	req := httptest.NewRequest(http.MethodOptions, "/console/api/connectors/github/key-setups", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "x-unrecognized-header")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	allowed := recorder.Header().Get("Access-Control-Allow-Headers")
	if corsHeaderListContains(allowed, "*") || corsHeaderListContains(allowed, "X-Unrecognized-Header") {
		t.Fatalf("CORS must not reflect arbitrary request headers: %q", allowed)
	}
}

func corsHeaderListContains(value, expected string) bool {
	for _, token := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(token), expected) {
			return true
		}
	}
	return false
}
