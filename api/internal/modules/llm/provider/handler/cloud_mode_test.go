package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProviderHandlerCreateGlobalDeniedWhenPlatformCatalogReadOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/providers", strings.NewReader(`{"name":"openai","provider_name":"OpenAI"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler := NewProviderHandler(nil)
	handler.SetPlatformCatalogReadOnly(true)
	handler.CreateGlobal(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "当前模式下平台模型目录不可写")
}
