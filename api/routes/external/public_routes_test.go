package external

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

func TestPublicDocumentationExposesDigitalHumanExternalContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterPublicRoutes(router.Group("/api"))

	docs := httptest.NewRecorder()
	router.ServeHTTP(docs, httptest.NewRequest(http.MethodGet, "/api/docs", nil))
	if docs.Code != http.StatusOK || !strings.Contains(docs.Body.String(), "/api/v1/digital-humans") {
		t.Fatalf("docs response = %d %s", docs.Code, docs.Body.String())
	}

	spec := httptest.NewRecorder()
	router.ServeHTTP(spec, httptest.NewRequest(http.MethodGet, "/api/openapi.yaml", nil))
	if spec.Code != http.StatusOK || !strings.Contains(spec.Header().Get("Content-Type"), "application/yaml") {
		t.Fatalf("OpenAPI response = %d %q", spec.Code, spec.Header().Get("Content-Type"))
	}
	var document map[string]any
	if err := yaml.Unmarshal(spec.Body.Bytes(), &document); err != nil {
		t.Fatalf("OpenAPI is invalid YAML: %v", err)
	}
	for _, required := range []string{
		"/digital-humans/chat:", "/digital-humans/runs/{run_id}/events:",
		"/digital-humans/conversations/{conversation_id}/messages:",
		"/digital-humans/files/upload:", "Last-Event-ID", "X-ZGI-Run-ID",
		"x-zgi-sse-event-data-schemas:", "run_accepted:", "run_end:",
	} {
		if !strings.Contains(spec.Body.String(), required) {
			t.Fatalf("OpenAPI document is missing %q", required)
		}
	}
	if strings.Contains(spec.Body.String(), "X-External-User-ID") || strings.Contains(spec.Body.String(), "end_user_id") {
		t.Fatal("DigitalHuman contract must use the user field instead of an identity header or renamed field")
	}
}
