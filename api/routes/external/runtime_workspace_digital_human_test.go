package external

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/runtimeworkspace"
)

func TestRuntimeAppRouteServesTheBoundAppType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	serve := func(appType string, agent, digitalHuman gin.HandlerFunc) (int, string) {
		t.Helper()
		engine := gin.New()
		engine.GET("/op", func(c *gin.Context) {
			if appType != "" {
				c.Set(runtimeAppTypeContextKey, appType)
			}
			c.Next()
		}, runtimeAppRoute(nil, agent, digitalHuman))
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/op", nil))
		return recorder.Code, recorder.Body.String()
	}
	agent := func(c *gin.Context) { c.String(http.StatusOK, "agent") }
	digitalHuman := func(c *gin.Context) { c.String(http.StatusOK, "digital human") }

	if code, body := serve(runtimeworkspace.AppTypeAgent, agent, digitalHuman); code != http.StatusOK || body != "agent" {
		t.Fatalf("agent app = %d %q", code, body)
	}
	if code, body := serve("", agent, digitalHuman); code != http.StatusOK || body != "agent" {
		t.Fatalf("untyped app = %d %q, want the Agent handler", code, body)
	}
	if code, _ := serve(runtimeworkspace.AppTypeAgent, nil, digitalHuman); code == http.StatusOK {
		t.Fatal("an Agent reached a DigitalHuman-only operation")
	}
	// Without the DigitalHuman handler wired, a bound DigitalHuman answers
	// not found rather than reaching the Agent handler.
	if code, body := serve(runtimeworkspace.AppTypeDigitalHuman, agent, digitalHuman); code == http.StatusOK || body == "agent" {
		t.Fatalf("digital human app without its handler = %d %q", code, body)
	}
}
