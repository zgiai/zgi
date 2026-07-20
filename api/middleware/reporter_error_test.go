package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/observability"
)

type middlewareRecordingReporter struct {
	events []observability.Event
}

func (r *middlewareRecordingReporter) Name() string { return "test" }

func (r *middlewareRecordingReporter) Report(_ context.Context, event observability.Event) error {
	r.events = append(r.events, event)
	return nil
}

func (*middlewareRecordingReporter) Flush(context.Context) error { return nil }

func TestZGIErrorReporterUsesInjectedReporter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &middlewareRecordingReporter{}
	reporter := observability.NewZGIReporter(adapter)
	engine := gin.New()
	engine.Use(ZGIErrorReporter(reporter))
	engine.GET("/failed", func(c *gin.Context) {
		c.Status(http.StatusInternalServerError)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/failed", nil)
	engine.ServeHTTP(response, request)

	if len(adapter.events) != 1 {
		t.Fatalf("reported events = %d, want 1", len(adapter.events))
	}
	event := adapter.events[0]
	if event.Name != "http.request.failed" || event.Tags["http.route"] != "/failed" {
		t.Fatalf("reported event = %#v", event)
	}
}

func TestZGIErrorReporterSkipsExpectedClientErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &middlewareRecordingReporter{}
	reporter := observability.NewZGIReporter(adapter)
	engine := gin.New()
	engine.Use(ZGIErrorReporter(reporter))
	engine.GET("/missing", func(c *gin.Context) {
		c.Status(http.StatusNotFound)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/missing", nil)
	engine.ServeHTTP(response, request)

	if len(adapter.events) != 0 {
		t.Fatalf("reported events = %d, want 0", len(adapter.events))
	}
}
