package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	platformconsole "github.com/zgiai/zgi/api/internal/infra/platform/console"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequireConsolePaymentProxy(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	t.Run("provider missing", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/payment", nil)

		ok := requireConsolePaymentProxy(c, nil)
		require.False(t, ok)
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	t.Run("provider present", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/payment", nil)

		ok := requireConsolePaymentProxy(c, platformconsole.NewRemote("http://localhost:9999", ""))
		require.True(t, ok)
	})
}
