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
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

type fakeToggleProviderService struct {
	service.ProviderService
	toggleErr error

	called         bool
	organizationID uuid.UUID
	provider       string
	isEnabled      bool
}

func (f *fakeToggleProviderService) ToggleProvider(ctx context.Context, organizationID uuid.UUID, provider string, isEnabled bool) error {
	f.called = true
	f.organizationID = organizationID
	f.provider = provider
	f.isEnabled = isEnabled
	return f.toggleErr
}

type providerRouteAccountService struct {
	interfaces.AccountService
	allowed             bool
	lastOrganizationID  string
	lastAccountID       string
	authorizationCalled bool
}

func (s *providerRouteAccountService) IsOrganizationAdminOrOwner(ctx context.Context, organizationID, accountID string) (bool, error) {
	s.authorizationCalled = true
	s.lastOrganizationID = organizationID
	s.lastAccountID = accountID
	return s.allowed, nil
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

func TestRegisterTenantProviderRoutesToggleProviderRequiresOrganizationAdminOrOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.NewString()
	fakeSvc := &fakeToggleProviderService{}
	accountSvc := &providerRouteAccountService{}

	router := gin.New()
	group := router.Group("/llm")
	group.Use(func(c *gin.Context) {
		c.Set("account_id", "acc-1")
		c.Set("organization_id", organizationID)
		c.Set("tenant_id", organizationID)
		c.Set("account_service", accountSvc)
		c.Next()
	})
	RegisterTenantProviderRoutes(group, NewProviderHandler(fakeSvc), nil)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/llm/providers/toggle", strings.NewReader(`{"provider":"test1","is_enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"code":"403001"`)
	require.True(t, accountSvc.authorizationCalled)
	require.Equal(t, organizationID, accountSvc.lastOrganizationID)
	require.Equal(t, "acc-1", accountSvc.lastAccountID)
	require.False(t, fakeSvc.called)
}

func TestRegisterTenantProviderRoutesToggleProviderAllowsOrganizationAdminOrOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.New()
	fakeSvc := &fakeToggleProviderService{}
	accountSvc := &providerRouteAccountService{allowed: true}

	router := gin.New()
	group := router.Group("/llm")
	group.Use(func(c *gin.Context) {
		c.Set("account_id", "acc-1")
		c.Set("organization_id", organizationID.String())
		c.Set("tenant_id", organizationID.String())
		c.Set("account_service", accountSvc)
		c.Next()
	})
	RegisterTenantProviderRoutes(group, NewProviderHandler(fakeSvc), nil)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/llm/providers/toggle", strings.NewReader(`{"provider":"test1","is_enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, accountSvc.authorizationCalled)
	require.True(t, fakeSvc.called)
	require.Equal(t, organizationID, fakeSvc.organizationID)
	require.Equal(t, "test1", fakeSvc.provider)
	require.False(t, fakeSvc.isEnabled)
}
