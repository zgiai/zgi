package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestRegisterTenantChannelRoutes_AdjustRequiresAdminOrOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	group := router.Group("/llm")
	RegisterTenantChannelRoutes(group, &ChannelHandler{})

	channelID := uuid.New().String()
	req := httptest.NewRequest(
		http.MethodPost,
		"/llm/channels/"+channelID+"/wallet/adjust",
		strings.NewReader(`{"amount":100}`),
	)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 for non-admin adjust request, got %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"code":"403001"`) {
		t.Fatalf("expected permission denied error code 403001, got body=%s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), adjustChannelWalletPermissionMessage) {
		t.Fatalf("expected custom permission message, got body=%s", w.Body.String())
	}
}

func TestRegisterTenantChannelRoutes_TopUpRouteRemoved(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	group := router.Group("/llm")
	RegisterTenantChannelRoutes(group, &ChannelHandler{})

	channelID := uuid.New().String()
	req := httptest.NewRequest(
		http.MethodPost,
		"/llm/channels/"+channelID+"/wallet/topup",
		strings.NewReader(`{"amount":100}`),
	)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected removed topup route to return 404, got %d, body=%s", w.Code, w.Body.String())
	}
}
