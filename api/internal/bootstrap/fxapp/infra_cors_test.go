package fxapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/config"
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

	engine := provideGinEngine(config.GlobalConfig, &SentryResource{}, &OpenTelemetryResource{})
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
