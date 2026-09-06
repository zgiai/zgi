package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/pkg/apperror"
)

type principalErrorRepository struct {
	apikeyrepo.APIKeyRepository
	err error
}

func (r principalErrorRepository) GetByKey(context.Context, string) (*apikeymodel.TenantAPIKey, error) {
	principal := "user"
	return &apikeymodel.TenantAPIKey{ID: "test-key", Status: "active", PrincipalType: &principal}, nil
}

func (r principalErrorRepository) ValidatePrincipalAccess(context.Context, *apikeymodel.TenantAPIKey) error {
	return r.err
}

func TestPrincipalQuotaMessagePreservesAuthenticationContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"/v1/models", "/v1/chat/completions", "/v1/messages", "/anthropic/v1/messages"} {
		for _, locale := range []string{"en-US", "zh-Hans"} {
			for _, exhausted := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/%s/quota=%v", path, locale, exhausted), func(t *testing.T) {
					cause := errors.New("private-database-detail")
					var failure error = cause
					if exhausted {
						failure = fmt.Errorf("wrapped: %w", apperror.Wrap(cause, llmerrors.AppCodeDeveloperQuotaExhausted))
					}
					router := gin.New()
					router.Use(func(c *gin.Context) { c.Header("X-Request-ID", "req-quota"); c.Next() })
					router.Use(LLMAPIKeyAuthMiddleware(principalErrorRepository{err: failure}, testApplicationErrorProjector(t)))
					router.Any(path, func(c *gin.Context) { t.Error("denied request reached provider handler") })
					request := httptest.NewRequest(http.MethodPost, path, nil)
					request.Header.Set("Authorization", "Bearer test-secret")
					request.Header.Set("Accept-Language", locale)
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					if response.Code != http.StatusUnauthorized || response.Header().Get("X-Request-ID") != "req-quota" {
						t.Fatalf("wire status/headers changed: %d %v", response.Code, response.Header())
					}
					var body map[string]any
					if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
						t.Fatal(err)
					}
					protocol, ok := body["error"].(map[string]any)
					if !ok {
						t.Fatalf("missing error: %v", body)
					}
					anthropic := path == "/v1/messages" || strings.HasPrefix(path, "/anthropic/")
					if anthropic {
						if len(body) != 3 || len(protocol) != 2 || body["type"] != "error" || body["request_id"] != "req-quota" || protocol["type"] != "authentication_error" {
							t.Fatalf("Anthropic contract changed: %v", body)
						}
					} else if len(body) != 1 || len(protocol) != 4 || protocol["type"] != "invalid_request_error" || protocol["code"] != "invalid_api_key" || protocol["param"] != nil {
						t.Fatalf("OpenAI contract changed: %v", body)
					}
					message, _ := protocol["message"].(string)
					if strings.Contains(response.Body.String(), "private-database-detail") {
						t.Fatal("cause leaked")
					}
					if !exhausted {
						if message != "API key access has been revoked" {
							t.Fatalf("unrelated legacy message changed: %q", message)
						}
					} else {
						want := "shared allowance"
						if locale == "zh-Hans" {
							want = "共享额度"
						}
						if !strings.Contains(message, want) || strings.Contains(message, "revoked") {
							t.Fatalf("wrong quota message: %q", message)
						}
					}
				})
			}
		}
	}
}

func TestPrincipalQuotaWithoutProjectorIsSafe(t *testing.T) {
	c := testProtocolContext(t, "/v1/models", "en-US")
	got := principalAccessProtocolError(c, apperror.New(llmerrors.AppCodeDeveloperQuotaExhausted), nil)
	if got.openAIStatus != 401 || got.openAICode != "invalid_api_key" || !strings.Contains(got.message, "no available quota") {
		t.Fatalf("unexpected fallback: %#v", got)
	}
}
