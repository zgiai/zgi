package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/config"
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// gatewayRoutesTestAPIKeyRepo is never consulted: every request in these tests
// is rejected for a missing key header before the lookup.
type gatewayRoutesTestAPIKeyRepo struct {
	apikeyrepo.APIKeyRepository
}

func newGatewayRoutesTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previous := config.GlobalConfig
	config.GlobalConfig = &config.Config{}
	t.Cleanup(func() { config.GlobalConfig = previous })

	sqlDB, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	require.NoError(t, err)

	definitions := append(appcatalog.DefaultDefinitions(), llmerrors.CatalogDefinitions()...)
	definitions = append(definitions, llmerrors.BillingCatalogDefinitions()...)
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	require.NoError(t, err)

	router := gin.New()
	RegisterGatewayRoutes(router.Group(""), GatewayRouteDeps{
		DB:                      db,
		APIKeyRepo:              gatewayRoutesTestAPIKeyRepo{},
		ApplicationErrorCatalog: catalog,
	})
	return router
}

func TestGatewayRoutesRegisterPublicRerank(t *testing.T) {
	router := newGatewayRoutesTestRouter(t)

	registered := map[string]bool{}
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, want := range []string{
		"POST /v1/chat/completions",
		"POST /v1/embeddings",
		"POST /v1/rerank",
		"POST /v1/responses",
		"POST /v1/audio/transcriptions",
		"POST /v1/messages",
		"GET /v1/models",
	} {
		require.True(t, registered[want], "route %s is not registered", want)
	}
}

// A missing route answers gin's bare 404; a registered one behind the API key
// middleware answers 401 without a key, which is what the 2679 probe checks.
func TestGatewayRoutesRerankWithoutKeyIsUnauthorized(t *testing.T) {
	router := newGatewayRoutesTestRouter(t)

	for _, path := range []string{"/v1/rerank", "/v1/chat/completions", "/v1/embeddings"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusUnauthorized, recorder.Code, "%s body = %s", path, recorder.Body.String())
		require.Contains(t, recorder.Body.String(), "llm.api_key.invalid", path)
	}
}
