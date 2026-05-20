package middleware_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
	appmiddleware "github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
)

func TestRequestAccessAndAuditLogging(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logPath := filepath.Join(t.TempDir(), "app.log")
	l, err := logger.New(&config.Config{
		Log: config.LogConfig{
			Level:      "debug",
			Filename:   logPath,
			MaxSize:    10,
			MaxAge:     15,
			MaxBackups: 3,
			Compress:   false,
		},
	})
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	logger.SetLogger(l)
	t.Cleanup(logger.Sync)

	engine := gin.New()
	engine.Use(appmiddleware.RequestID())
	engine.Use(appmiddleware.Logger())
	engine.Use(appmiddleware.AuditLogger())
	engine.Use(appmiddleware.Recovery())
	engine.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	engine.POST("/console/api/items", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"created": true})
	})

	accessRequest := httptest.NewRequest(http.MethodGet, "/ok", nil)
	accessResponse := httptest.NewRecorder()
	engine.ServeHTTP(accessResponse, accessRequest)

	requestID := accessResponse.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Fatalf("X-Request-ID header is empty")
	}

	auditRequest := httptest.NewRequest(http.MethodPost, "/console/api/items", nil)
	auditResponse := httptest.NewRecorder()
	engine.ServeHTTP(auditResponse, auditRequest)
	logger.Sync()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", logPath, err)
	}

	text := string(content)
	if !strings.Contains(text, `"log_type":"access"`) {
		t.Fatalf("access log not found\n%s", text)
	}
	if !strings.Contains(text, requestID) {
		t.Fatalf("request_id %q not found\n%s", requestID, text)
	}
	if !strings.Contains(text, `"log_type":"audit"`) {
		t.Fatalf("audit log not found\n%s", text)
	}
}

func TestAccessLogIncludesRequestMetadataForSuccessfulRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logPath := filepath.Join(t.TempDir(), "app.log")
	l, err := logger.New(&config.Config{
		Log: config.LogConfig{
			Level:      "debug",
			Filename:   logPath,
			MaxSize:    10,
			MaxAge:     15,
			MaxBackups: 3,
			Compress:   false,
		},
	})
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	logger.SetLogger(l)
	t.Cleanup(logger.Sync)

	engine := gin.New()
	engine.Use(appmiddleware.RequestID())
	engine.Use(appmiddleware.Logger())
	engine.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	request := httptest.NewRequest(http.MethodGet, "/ok", nil)
	request.RemoteAddr = "192.0.2.10:12345"
	request.Header.Set("User-Agent", "test-agent")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	logger.Sync()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", logPath, err)
	}

	text := string(content)
	if !strings.Contains(text, `"route":"/ok"`) {
		t.Fatalf("route not found\n%s", text)
	}
	if !strings.Contains(text, `"user_agent":"test-agent"`) {
		t.Fatalf("successful request should include user_agent\n%s", text)
	}
	if !strings.Contains(text, `"path":"/ok"`) {
		t.Fatalf("successful request should include raw path\n%s", text)
	}
	if !strings.Contains(text, `"client_ip":"192.0.2.10"`) {
		t.Fatalf("successful request should include client_ip\n%s", text)
	}
}

func TestRecoveryWritesErrorLog(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logPath := filepath.Join(t.TempDir(), "app.log")
	l, err := logger.New(&config.Config{
		Log: config.LogConfig{
			Level:      "debug",
			Filename:   logPath,
			MaxSize:    10,
			MaxAge:     15,
			MaxBackups: 3,
			Compress:   false,
		},
	})
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	logger.SetLogger(l)
	t.Cleanup(logger.Sync)

	engine := gin.New()
	engine.Use(appmiddleware.RequestID())
	engine.Use(appmiddleware.Logger())
	engine.Use(appmiddleware.Recovery())
	engine.GET("/panic", func(c *gin.Context) {
		panic("boom")
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/panic", nil)
	engine.ServeHTTP(response, request)
	logger.Sync()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", logPath, err)
	}

	text := string(content)
	if !strings.Contains(text, `"log_type":"error"`) {
		t.Fatalf("error log not found\n%s", text)
	}
	if !strings.Contains(text, `"stacktrace":`) {
		t.Fatalf("stacktrace not found\n%s", text)
	}
	if !strings.Contains(text, `"request_id":"`) {
		t.Fatalf("request_id not found in recovery log\n%s", text)
	}
	recoveryEntry := findEntryByMessage(t, logPath, "panic recovered")
	caller, _ := recoveryEntry["caller"].(string)
	if strings.Contains(caller, "logger/logger.go:") {
		t.Fatalf("recovery caller still points to logger wrapper: %#v", recoveryEntry)
	}
	if response.Code == 0 {
		t.Fatalf("response code = 0, want non-zero")
	}
}

func TestAccessLogKeepsVerboseFieldsForClientErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logPath := filepath.Join(t.TempDir(), "app.log")
	l, err := logger.New(&config.Config{
		Log: config.LogConfig{
			Level:      "debug",
			Filename:   logPath,
			MaxSize:    10,
			MaxAge:     15,
			MaxBackups: 3,
			Compress:   false,
		},
	})
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	logger.SetLogger(l)
	t.Cleanup(logger.Sync)

	engine := gin.New()
	engine.Use(appmiddleware.RequestID())
	engine.Use(appmiddleware.Logger())
	engine.GET("/bad", func(c *gin.Context) {
		c.Status(http.StatusBadRequest)
	})

	request := httptest.NewRequest(http.MethodGet, "/bad", nil)
	request.RemoteAddr = "192.0.2.20:23456"
	request.Header.Set("User-Agent", "test-agent")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	logger.Sync()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", logPath, err)
	}

	text := string(content)
	if !strings.Contains(text, `"level":"warn"`) {
		t.Fatalf("client error access log should be warn level\n%s", text)
	}
	if !strings.Contains(text, `"user_agent":"test-agent"`) {
		t.Fatalf("client error access log should include user_agent\n%s", text)
	}
	if !strings.Contains(text, `"path":"/bad"`) {
		t.Fatalf("client error access log should include raw path\n%s", text)
	}
	if !strings.Contains(text, `"client_ip":"192.0.2.20"`) {
		t.Fatalf("client error access log should include client_ip\n%s", text)
	}
}

func TestContextErrorLogCarriesRequestIDWithoutStacktrace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logPath := filepath.Join(t.TempDir(), "app.log")
	l, err := logger.New(&config.Config{
		Log: config.LogConfig{
			Level:      "debug",
			Filename:   logPath,
			MaxSize:    10,
			MaxAge:     15,
			MaxBackups: 3,
			Compress:   false,
		},
	})
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	logger.SetLogger(l)
	t.Cleanup(logger.Sync)

	engine := gin.New()
	engine.Use(appmiddleware.RequestID())
	engine.Use(appmiddleware.Logger())
	engine.GET("/business-error", func(c *gin.Context) {
		logger.ErrorContext(c.Request.Context(), "business error", errors.New("validation failed"))
		c.Status(http.StatusBadRequest)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/business-error", nil)
	engine.ServeHTTP(response, request)
	logger.Sync()

	entry := findEntryByMessage(t, logPath, "business error")
	requestID := response.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Fatalf("X-Request-ID header is empty")
	}
	if entry["request_id"] != requestID {
		t.Fatalf("request_id = %v, want %q", entry["request_id"], requestID)
	}
	if _, exists := entry["stacktrace"]; exists {
		t.Fatalf("business error log should not include stacktrace: %#v", entry)
	}
}

func TestContextCriticalLogCarriesRequestIDAndStacktrace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logPath := filepath.Join(t.TempDir(), "app.log")
	l, err := logger.New(&config.Config{
		Log: config.LogConfig{
			Level:      "debug",
			Filename:   logPath,
			MaxSize:    10,
			MaxAge:     15,
			MaxBackups: 3,
			Compress:   false,
		},
	})
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}
	logger.SetLogger(l)
	t.Cleanup(logger.Sync)

	engine := gin.New()
	engine.Use(appmiddleware.RequestID())
	engine.Use(appmiddleware.Logger())
	engine.GET("/critical-error", func(c *gin.Context) {
		logger.CriticalContext(c.Request.Context(), "critical error", errors.New("boom"))
		c.Status(http.StatusInternalServerError)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/critical-error", nil)
	engine.ServeHTTP(response, request)
	logger.Sync()

	entry := findEntryByMessage(t, logPath, "critical error")
	requestID := response.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Fatalf("X-Request-ID header is empty")
	}
	if entry["request_id"] != requestID {
		t.Fatalf("request_id = %v, want %q", entry["request_id"], requestID)
	}
	if _, exists := entry["stacktrace"]; !exists {
		t.Fatalf("critical error log should include stacktrace: %#v", entry)
	}
}

func findEntryByMessage(t *testing.T, logPath, msg string) map[string]interface{} {
	t.Helper()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", logPath, err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("json.Unmarshal(%q) error = %v", line, err)
		}
		if entry["msg"] == msg {
			return entry
		}
	}

	t.Fatalf("log entry with msg %q not found", msg)
	return nil
}
