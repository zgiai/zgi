package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	apikeymodel "github.com/zgiai/ginext/internal/modules/llm/apikey/model"
	gatewayhandler "github.com/zgiai/ginext/internal/modules/llm/gateway/handler"
)

func TestLLMAPIKeyAuthMiddlewareAcceptsXAPIKey(t *testing.T) {
	router := gatewayAuthTestRouter("sk-test")

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("x-api-key", "sk-test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestLLMAPIKeyAuthMiddlewareRejectsConflictingHeaders(t *testing.T) {
	router := gatewayAuthTestRouter("sk-test")

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer sk-test")
	req.Header.Set("x-api-key", "sk-other")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func gatewayAuthTestRouter(apiKey string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gatewayhandler.LLMAPIKeyAuthMiddleware(fakeAPIKeyRepo{apiKey: apiKey}))
	router.GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return router
}

type fakeAPIKeyRepo struct {
	apiKey string
}

func (r fakeAPIKeyRepo) Create(context.Context, *apikeymodel.TenantAPIKey) error {
	return nil
}

func (r fakeAPIKeyRepo) GetByID(context.Context, string, string) (*apikeymodel.TenantAPIKey, error) {
	return nil, fmt.Errorf("not found")
}

func (r fakeAPIKeyRepo) GetByIDInOrganizations(context.Context, string, []string) (*apikeymodel.TenantAPIKey, error) {
	return nil, fmt.Errorf("not found")
}

func (r fakeAPIKeyRepo) GetByKey(_ context.Context, key string) (*apikeymodel.TenantAPIKey, error) {
	if key != r.apiKey {
		return nil, fmt.Errorf("invalid key")
	}
	return &apikeymodel.TenantAPIKey{
		ID:             "key-test",
		OrganizationID: "org-test",
		Status:         "active",
	}, nil
}

func (r fakeAPIKeyRepo) GetByKeyHash(context.Context, string) (*apikeymodel.TenantAPIKey, error) {
	return nil, fmt.Errorf("not found")
}

func (r fakeAPIKeyRepo) List(context.Context, string, map[string]interface{}, int, int) ([]*apikeymodel.TenantAPIKey, int64, error) {
	return nil, 0, nil
}

func (r fakeAPIKeyRepo) Update(context.Context, *apikeymodel.TenantAPIKey) error {
	return nil
}

func (r fakeAPIKeyRepo) Delete(context.Context, string, string) error {
	return nil
}

func (r fakeAPIKeyRepo) UpdateAccessedAt(context.Context, string) error {
	return nil
}

func (r fakeAPIKeyRepo) UpdateQuota(context.Context, string, int64, int64) error {
	return nil
}

func (r fakeAPIKeyRepo) CountByTenant(context.Context, string) (int64, error) {
	return 0, nil
}
