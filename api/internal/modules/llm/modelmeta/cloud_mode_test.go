package modelmeta

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHandlerSyncProviderDeniedWhenPlatformCatalogReadOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "provider", Value: "openai"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/modelmeta/sync/openai", nil)

	handler := NewHandler(nil)
	handler.SetPlatformCatalogReadOnly(true)
	handler.SyncProvider(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "当前模式下平台模型目录不可写")
}

func TestBindSyncProviderRequestAllowsEmptyBodyForFullSync(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/modelmeta/sync/openai", nil)

	req, err := bindSyncProviderRequest(ctx)
	require.NoError(t, err)
	require.Empty(t, req.Models)
}

func TestBindSyncProviderRequestParsesSelectedModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/modelmeta/sync/openai", strings.NewReader(`{"models":["gpt-4o","gpt-4o-mini"]}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	req, err := bindSyncProviderRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-4o", "gpt-4o-mini"}, req.Models)
}

func TestBindSyncProviderRequestRejectsInvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/modelmeta/sync/openai", strings.NewReader(`{"models":`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	_, err := bindSyncProviderRequest(ctx)
	require.Error(t, err)
}
