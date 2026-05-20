package logger_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
)

func TestLoggerWritesStructuredEntries(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "app.log")
	l := mustNewLogger(t, logPath, 10)
	logger.SetLogger(l)
	t.Cleanup(logger.Sync)

	logger.Info("http request", "request_id", "req-1", "foo", "bar")
	logger.Info("zap fields", zap.String("request_id", "req-2"), zap.String("foo", "baz"))
	logger.Error("request failed", errors.New("boom"))
	logger.Sync()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", logPath, err)
	}

	text := string(content)
	assertContains(t, text, `"request_id":"req-1"`)
	assertContains(t, text, `"log_type":"app"`)
	assertContains(t, text, `"request_id":"req-2"`)
	assertContains(t, text, `"log_type":"error"`)
	assertContains(t, text, `"error":"boom"`)
}

func TestLoggerWritesJSONToStdout(t *testing.T) {
	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = originalStdout
		logger.SetLogger(zap.NewNop())
		_ = reader.Close()
	})

	l, err := logger.New(&config.Config{
		Log: config.LogConfig{
			Level:      "debug",
			Filename:   "",
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

	logger.Info("stdout json test", "request_id", "stdout-1")
	logger.Sync()
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}
	os.Stdout = originalStdout

	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll(stdout pipe) error = %v", err)
	}

	var entry map[string]interface{}
	line := strings.TrimSpace(string(output))
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("stdout log is not JSON: %v\n%s", err, line)
	}

	for _, key := range []string{"level", "ts", "msg", "caller"} {
		if _, exists := entry[key]; !exists {
			t.Fatalf("stdout JSON missing key %q: %#v", key, entry)
		}
	}
	if entry["msg"] != "stdout json test" {
		t.Fatalf("entry[msg] = %v, want stdout json test", entry["msg"])
	}
	if entry["request_id"] != "stdout-1" {
		t.Fatalf("entry[request_id] = %v, want stdout-1", entry["request_id"])
	}
}

func TestLoggerCallerPointsToCallSite(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "app.log")
	l := mustNewLogger(t, logPath, 10)
	logger.SetLogger(l)
	t.Cleanup(logger.Sync)

	logger.Info("caller info test")
	logger.Error("caller error test", errors.New("boom"))
	logger.Critical("caller critical test", errors.New("critical boom"))
	logger.Sync()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", logPath, err)
	}

	text := string(content)
	assertContains(t, text, `logger_test.go:`)
	assertNotContains(t, text, `"caller":"logger/logger.go:`)
}

func TestErrorContextIncludesRequestIDWithoutStacktrace(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "app.log")
	l := mustNewLogger(t, logPath, 10)
	logger.SetLogger(l)
	t.Cleanup(logger.Sync)

	ctx := logger.WithFields(context.Background(), zap.String("request_id", "req-ctx-1"))
	logger.ErrorContext(ctx, "context error", errors.New("boom"))
	logger.Sync()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", logPath, err)
	}

	text := string(content)
	assertContains(t, text, `"msg":"context error"`)
	assertContains(t, text, `"request_id":"req-ctx-1"`)
	assertContains(t, text, `"error":"boom"`)
	assertNotContains(t, text, `"stacktrace":`)
}

func TestInfoAndDebugContextIncludeRequestID(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "app.log")
	l := mustNewLogger(t, logPath, 10)
	logger.SetLogger(l)
	t.Cleanup(logger.Sync)

	ctx := logger.WithFields(context.Background(), zap.String("request_id", "req-ctx-info"))
	logger.InfoContext(ctx, "context info")
	logger.DebugContext(ctx, "context debug")
	logger.Sync()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", logPath, err)
	}

	text := string(content)
	assertContains(t, text, `"msg":"context info"`)
	assertContains(t, text, `"msg":"context debug"`)
	assertContains(t, text, `"request_id":"req-ctx-info"`)
}

func TestCriticalContextAddsStacktrace(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "app.log")
	l := mustNewLogger(t, logPath, 10)
	logger.SetLogger(l)
	t.Cleanup(logger.Sync)

	ctx := logger.WithFields(context.Background(), zap.String("request_id", "req-ctx-2"))
	logger.CriticalContext(ctx, "critical context", errors.New("boom"))
	logger.Sync()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", logPath, err)
	}

	text := string(content)
	assertContains(t, text, `"msg":"critical context"`)
	assertContains(t, text, `"request_id":"req-ctx-2"`)
	assertContains(t, text, `"stacktrace":`)
}

func TestLoggerRotatesFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "app.log")
	l := mustNewLogger(t, logPath, 1)
	logger.SetLogger(l)
	t.Cleanup(logger.Sync)

	largeMessage := strings.Repeat("x", 700*1024)
	logger.Info("large message", largeMessage)
	logger.Info("large message", largeMessage)
	logger.Sync()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("os.ReadDir(%q) error = %v", dir, err)
	}

	fileCount := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "app") {
			fileCount++
		}
	}

	if fileCount < 2 {
		t.Fatalf("rotated file count = %d, want >= 2", fileCount)
	}
}

func TestLoggerCreatesLogDirectory(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "nested", "logs", "app.log")
	l := mustNewLogger(t, logPath, 10)
	logger.SetLogger(l)
	t.Cleanup(logger.Sync)

	logger.Info("directory creation test")
	logger.Sync()

	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("os.Stat(%q) error = %v", logPath, err)
	}
}

func mustNewLogger(t *testing.T, logPath string, maxSize int) *zap.Logger {
	t.Helper()

	l, err := logger.New(&config.Config{
		Log: config.LogConfig{
			Level:      "debug",
			Filename:   logPath,
			MaxSize:    maxSize,
			MaxAge:     15,
			MaxBackups: 3,
			Compress:   false,
		},
	})
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}

	return l
}

func assertContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("text does not contain %q\nfull text:\n%s", want, text)
	}
}

func assertNotContains(t *testing.T, text, want string) {
	t.Helper()
	if strings.Contains(text, want) {
		t.Fatalf("text unexpectedly contains %q\nfull text:\n%s", want, text)
	}
}
