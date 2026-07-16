package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	gatewayhandler "github.com/zgiai/zgi/api/internal/modules/llm/gateway/handler"
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
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != "invalid_api_key" {
		t.Fatalf("error.code = %q, want invalid_api_key; body = %s", body.Error.Code, rec.Body.String())
	}
}

func TestLLMAPIKeyAuthMiddlewareDoesNotExposeRepositoryErrors(t *testing.T) {
	router := gatewayAuthTestRouter("sk-test")

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer sk-invalid")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if got := rec.Body.String(); got == "" || containsAny(got, "invalid key", "record not found") {
		t.Fatalf("body exposes repository error: %s", got)
	}
}

func TestLLMAPIKeyAuthMiddlewareUsesAnthropicErrorShape(t *testing.T) {
	router := gatewayAuthTestRouter("sk-test")

	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	var body struct {
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
		Error     struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Type != "error" || body.Error.Type != "authentication_error" || body.RequestID != "req-test" {
		t.Fatalf("unexpected Anthropic error body: %s", rec.Body.String())
	}
}

func gatewayAuthTestRouter(apiKey string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("request_id", "req-test")
		c.Header("X-Request-ID", "req-test")
		c.Next()
	})
	router.Use(gatewayhandler.LLMAPIKeyAuthMiddleware(fakeAPIKeyRepo{apiKey: apiKey}))
	for _, path := range []string{"/protected", "/v1/models", "/anthropic/v1/messages"} {
		router.Any(path, func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})
	}
	return router
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
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
