package routes

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/config"
)

func TestRegisterConsoleInternalRoutes_PaymentEventRemoved(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setRouterTestConfig(t)

	r := gin.New()
	registerConsoleInternalRoutes(r, nil)

	body := []byte(`{"event_id":"evt-1","event_type":"PAYMENT_COMPLETED","payload":{"order_no":"ORD-1"}}`)
	req := httptest.NewRequest(http.MethodPost, "/console/api/internal/payment/event", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	signInternalRequest(req, "test-internal-key")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func setRouterTestConfig(t *testing.T) {
	t.Helper()
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Console: config.ConsoleConfig{InternalAPIKey: "test-internal-key"},
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
	})
}

func signInternalRequest(req *http.Request, secret string) {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	msg := ts + "|" + req.URL.Path
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	sig := hex.EncodeToString(mac.Sum(nil))

	req.Header.Set("X-Internal-Timestamp", ts)
	req.Header.Set("X-Internal-Signature", sig)
}
