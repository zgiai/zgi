package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/llm/provider/service"
)

type fakeToggleProviderService struct {
	service.ProviderService
	toggleErr error

	organizationID uuid.UUID
	provider       string
	isEnabled      bool
}

func (f *fakeToggleProviderService) ToggleProvider(ctx context.Context, organizationID uuid.UUID, provider string, isEnabled bool) error {
	f.organizationID = organizationID
	f.provider = provider
	f.isEnabled = isEnabled
	return f.toggleErr
}

func TestProviderHandlerToggleProviderReturnsNotFoundForMissingProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.New()
	fakeSvc := &fakeToggleProviderService{toggleErr: service.ErrProviderNotFound}
	handler := NewProviderHandler(fakeSvc)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/providers/toggle", strings.NewReader(`{"provider":"missing","is_enabled":false}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("organization_id", organizationID.String())

	handler.ToggleProvider(ctx)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), "provider not found")
}

func TestProviderHandlerToggleProviderAllowsCustomProviderPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.New()
	fakeSvc := &fakeToggleProviderService{}
	handler := NewProviderHandler(fakeSvc)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/providers/toggle", strings.NewReader(`{"provider":"test1","is_enabled":false}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("organization_id", organizationID.String())

	handler.ToggleProvider(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, organizationID, fakeSvc.organizationID)
	require.Equal(t, "test1", fakeSvc.provider)
	require.False(t, fakeSvc.isEnabled)
}
