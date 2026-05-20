package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterStatisticsRoutes_CurrentRoutesExist(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	group := router.Group("/console/api/llm")
	RegisterStatisticsRoutes(group, &StatisticsHandler{})

	paths := []string{
		"/console/api/llm/statistics/model-usage",
		"/console/api/llm/statistics/workspace-quota",
	}

	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Fatalf("expected route %s to be registered, got 404", path)
		}
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected route %s to return 401 without organization context, got %d body=%s", path, w.Code, w.Body.String())
		}
	}
}

func TestRegisterStatisticsRoutes_DeprecatedRoutesAreRemoved(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	group := router.Group("/console/api/llm")
	RegisterStatisticsRoutes(group, &StatisticsHandler{})

	paths := []string{
		"/console/api/llm/statistics/usage-details",
		"/console/api/llm/statistics/group",
		"/console/api/llm/statistics/model-consumption",
		"/console/api/llm/statistics/workspace-consumption",
		"/console/api/llm/statistics/gateway-key-consumption",
	}

	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected deprecated route %s to return 404, got %d body=%s", path, w.Code, w.Body.String())
		}
	}
}
