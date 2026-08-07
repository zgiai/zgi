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
	for _, header := range []string{"x-zgi-conversation-id", "x-zgi-message-id"} {
		if !strings.Contains(exposed, header) {
			t.Fatalf("Access-Control-Expose-Headers = %q, want %q", exposed, header)
		}
	}
}
