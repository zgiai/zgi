package developeraccess

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	apikeyservice "github.com/zgiai/zgi/api/internal/modules/llm/apikey/service"
	accessmodel "github.com/zgiai/zgi/api/internal/modules/llm/developeraccess/model"
	gatewayhandler "github.com/zgiai/zgi/api/internal/modules/llm/gateway/handler"
	"github.com/zgiai/zgi/api/internal/util"
	"gorm.io/gorm"
)

func openQuotaAuthenticationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openDeveloperAccessTestDB(t)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	// These tests call both auth entry points in sequence. Serialize SQLite
	// connections so their asynchronous accessed_at writes cannot race the
	// next authoritative read with SQLITE_LOCKED. Keep the real repository.
	pool.SetMaxOpenConns(1)
	return db
}

func TestPersonalKeyCreationAndRotationDenyLegacyQuotaAuthentication(t *testing.T) {
	db := openQuotaAuthenticationTestDB(t)
	workspaceID, _, ownerID, _ := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	created, err := service.CreateKey(context.Background(), workspaceID, ownerID, CreateKeyInput{Name: "legacy guard"})
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := service.RotateKey(context.Background(), workspaceID, ownerID, created.ID, RotateKeyInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{created.ID, replacement.ID} {
		var stored apikeymodel.TenantAPIKey
		if err := db.First(&stored, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		// HasQuota deliberately retains the pre-principal quota semantics.
		// Both the rotated row and its replacement must fail that old gate.
		if stored.QuotaLimit == nil || *stored.QuotaLimit != 0 || stored.RemainQuota != 0 || stored.HasQuota() {
			t.Fatal("personal key permits legacy quota authentication")
		}
	}
}

func TestLegacyKeyQuotaAuthenticationIsUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limit := int64(100)
	for _, scenario := range []struct {
		name      string
		limit     *int64
		remaining int64
		valid     bool
	}{
		{"unlimited", nil, 0, true},
		{"available", &limit, 50, true},
		{"exhausted", &limit, 0, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			db := openQuotaAuthenticationTestDB(t)
			_, organizationID, _, _ := seedDeveloperWorkspace(t, db)
			secret := "legacy-fixture-" + uuid.NewString()
			key := apikeymodel.TenantAPIKey{OrganizationID: organizationID, Name: "legacy quota",
				Status: "active", KeyHash: util.HashAPIKey(secret), QuotaLimit: scenario.limit, RemainQuota: scenario.remaining}
			if err := db.Create(&key).Error; err != nil {
				t.Fatal(err)
			}
			repo := apikeyrepo.NewAPIKeyRepository(db)
			validation, err := apikeyservice.NewAPIKeyService(db, repo, nil, nil).ValidateAPIKey(context.Background(), secret)
			if err != nil {
				t.Fatal(err)
			}
			if validation.Valid != scenario.valid {
				t.Fatalf("legacy validation changed: valid=%v", validation.Valid)
			}
			router := gin.New()
			router.GET("/v1/models", gatewayhandler.LLMAPIKeyAuthMiddleware(repo), func(c *gin.Context) { c.Status(http.StatusNoContent) })
			request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			request.Header.Set("Authorization", "Bearer "+secret)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			want := http.StatusNoContent
			if !scenario.valid {
				want = http.StatusTooManyRequests
			}
			if response.Code != want {
				t.Fatalf("legacy gateway status=%d, want %d", response.Code, want)
			}
		})
	}
}

func TestPersonalQuotaUsesLiveGrantAtBothAuthenticationEntrypoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, scenario := range []struct {
		name        string
		legacyQuota any
		exhausted   bool
	}{
		{"guarded/available", int64(0), false},
		{"guarded/exhausted", int64(0), true},
		{"pre-guard/available", nil, false},
		{"pre-guard/exhausted", nil, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			exhausted := scenario.exhausted
			db := openQuotaAuthenticationTestDB(t)
			workspaceID, _, ownerID, _ := seedDeveloperWorkspace(t, db)
			repo := apikeyrepo.NewAPIKeyRepository(db)
			service := NewService(db, repo, nil)
			created, err := service.CreateKey(context.Background(), workspaceID, ownerID, CreateKeyInput{Name: "live grant"})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&apikeymodel.TenantAPIKey{}).Where("id = ?", created.ID).
				Updates(map[string]any{"quota_limit": scenario.legacyQuota, "remain_quota": int64(0)}).Error; err != nil {
				t.Fatal(err)
			}
			remaining := int64(100)
			if exhausted {
				remaining = 0
			}
			if err := db.Model(&accessmodel.Grant{}).Where("workspace_id = ?", workspaceID).
				Updates(map[string]any{"quota_limit": int64(100), "remain_quota": remaining}).Error; err != nil {
				t.Fatal(err)
			}
			validation, err := apikeyservice.NewAPIKeyService(db, repo, nil, nil).ValidateAPIKey(context.Background(), created.Secret)
			if err != nil {
				t.Fatal(err)
			}
			if validation.Valid == exhausted {
				t.Errorf("validation endpoint valid=%v, exhausted=%v", validation.Valid, exhausted)
			}
			router := gin.New()
			router.GET("/v1/models", gatewayhandler.LLMAPIKeyAuthMiddleware(repo), func(c *gin.Context) { c.Status(http.StatusNoContent) })
			request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			request.Header.Set("Authorization", "Bearer "+created.Secret)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			want := http.StatusNoContent
			if exhausted {
				want = http.StatusUnauthorized
			}
			if response.Code != want {
				t.Errorf("gateway status=%d, want %d", response.Code, want)
			}
		})
	}
}
