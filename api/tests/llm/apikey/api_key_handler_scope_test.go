package apikey_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	apikeydto "github.com/zgiai/ginext/internal/modules/llm/apikey/dto"
	apikeyhandler "github.com/zgiai/ginext/internal/modules/llm/apikey/handler"
)

type recordingAPIKeyService struct {
	getOrganizations    []string
	updateOrganizations []string
	deleteOrganizations []string
}

func (s *recordingAPIKeyService) CreateAPIKey(context.Context, *apikeydto.CreateAPIKeyRequest) (*apikeydto.CreateAPIKeyResponse, error) {
	return nil, nil
}

func (s *recordingAPIKeyService) GetAPIKey(_ context.Context, id string, organizationIDs []string) (*apikeydto.APIKeyResponse, error) {
	s.getOrganizations = append([]string(nil), organizationIDs...)
	return &apikeydto.APIKeyResponse{ID: id, OrganizationID: organizationIDs[0], Name: "key", Status: "active"}, nil
}

func (s *recordingAPIKeyService) ListAPIKeys(context.Context, *apikeydto.ListAPIKeyRequest) (*apikeydto.ListAPIKeyResponse, error) {
	return nil, nil
}

func (s *recordingAPIKeyService) UpdateAPIKey(_ context.Context, id string, organizationIDs []string, _ *apikeydto.UpdateAPIKeyRequest) (*apikeydto.APIKeyResponse, error) {
	s.updateOrganizations = append([]string(nil), organizationIDs...)
	return &apikeydto.APIKeyResponse{ID: id, OrganizationID: organizationIDs[0], Name: "key", Status: "active"}, nil
}

func (s *recordingAPIKeyService) DeleteAPIKey(_ context.Context, id string, organizationIDs []string) (*apikeydto.DeleteAPIKeyResponse, error) {
	s.deleteOrganizations = append([]string(nil), organizationIDs...)
	return &apikeydto.DeleteAPIKeyResponse{ID: id, Message: "API key deleted successfully"}, nil
}

func (s *recordingAPIKeyService) ValidateAPIKey(context.Context, string) (*apikeydto.ValidateAPIKeyResponse, error) {
	return nil, nil
}

func (s *recordingAPIKeyService) UpdateAccessedAt(context.Context, string) error {
	return nil
}

func TestAPIKeyHandlerPassesCurrentOrganizationScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &recordingAPIKeyService{}
	handler := apikeyhandler.NewAPIKeyHandler(svc, nil, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", "account-1")
		c.Set("organization_id", "org-1")
		c.Next()
	})
	router.GET("/api-keys/:id", handler.GetAPIKey)
	router.PUT("/api-keys/:id", handler.UpdateAPIKey)
	router.DELETE("/api-keys/:id", handler.DeleteAPIKey)

	performRequest(t, router, http.MethodGet, "/api-keys/key-1", "")
	assertStrings(t, svc.getOrganizations, []string{"org-1"})

	performRequest(t, router, http.MethodPut, "/api-keys/key-1", `{"name":"updated"}`)
	assertStrings(t, svc.updateOrganizations, []string{"org-1"})

	performRequest(t, router, http.MethodDelete, "/api-keys/key-1", "")
	assertStrings(t, svc.deleteOrganizations, []string{"org-1"})
}

func performRequest(t *testing.T, router *gin.Engine, method, path, body string) {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s status = %d body = %s", method, path, rec.Code, rec.Body.String())
	}
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d; got %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("value[%d] = %s, want %s; got %v", i, got[i], want[i], got)
		}
	}
}
