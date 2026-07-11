package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/channel/dto"
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

func TestRegisterTenantChannelRoutes_UpstreamStateRequiresAdminOrOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	group := router.Group("/llm")
	RegisterTenantChannelRoutes(group, &ChannelHandler{})

	for _, request := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/upstream-state/check"},
		{method: http.MethodPost, path: "/upstream-state/retry"},
		{method: http.MethodPut, path: "/upstream-state/settings", body: `{"warning_thresholds":[]}`},
	} {
		req := httptest.NewRequest(
			request.method,
			"/llm/channels/"+uuid.NewString()+request.path,
			strings.NewReader(request.body),
		)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s status = %d, want 403; body=%s", request.method, request.path, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), manageUpstreamStatePermissionMessage) {
			t.Fatalf("%s %s body = %s, want permission message", request.method, request.path, w.Body.String())
		}
	}
}

func TestRedactUpstreamStateKeepsOnlyGenericRisk(t *testing.T) {
	spendable := false
	channel := &dto.ChannelView{UpstreamState: &dto.UpstreamStateView{
		BalanceCapability:      "supported",
		BalanceScope:           "account_balance",
		Balances:               []dto.UpstreamBalanceAmountView{{Currency: "USD", Remaining: "2"}},
		Spendable:              &spendable,
		IsUnlimited:            true,
		Availability:           "exhausted",
		IsLow:                  true,
		IsStale:                true,
		BalanceObservedAt:      "2026-07-10T08:00:00Z",
		LastCheckAt:            "2026-07-10T08:00:00Z",
		LastCheckStatus:        "success",
		LastCheckErrorKind:     "provider_error",
		WarningThresholds:      []dto.UpstreamWarningThresholdView{{Currency: "USD", Amount: "5"}},
		SharedChannelCount:     2,
		BlockReason:            "balance_exhausted",
		CooldownUntil:          "2026-07-10T08:10:00Z",
		AvailabilityObservedAt: "2026-07-10T08:00:00Z",
		ManualRetryRequestedAt: "2026-07-10T08:05:00Z",
		ProviderErrorCode:      "Arrearage",
		ProviderErrorStatus:    400,
		WouldGuard:             true,
	}}

	redactUpstreamState(channel)
	state := channel.UpstreamState
	if len(state.Balances) != 0 || state.Spendable != nil || state.IsUnlimited || len(state.WarningThresholds) != 0 {
		t.Fatalf("concrete balance data was not redacted: %#v", state)
	}
	if state.BalanceScope != "" || state.BalanceObservedAt != "" || state.LastCheckAt != "" || state.SharedChannelCount != 0 {
		t.Fatalf("balance metadata was not redacted: %#v", state)
	}
	if state.BlockReason != "" || state.CooldownUntil != "" || state.WouldGuard {
		t.Fatalf("guard internals were not redacted: %#v", state)
	}
	if state.AvailabilityObservedAt != "" || state.ManualRetryRequestedAt != "" || state.ProviderErrorCode != "" || state.ProviderErrorStatus != 0 {
		t.Fatalf("provider evidence was not redacted: %#v", state)
	}
	if state.Availability != "exhausted" || !state.IsLow || !state.IsStale || state.LastCheckStatus != "success" {
		t.Fatalf("generic risk state was removed: %#v", state)
	}
}
