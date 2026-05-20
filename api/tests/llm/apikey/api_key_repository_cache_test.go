package apikey_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	gatewayhandler "github.com/zgiai/zgi/api/internal/modules/llm/gateway/handler"
	"github.com/zgiai/zgi/api/internal/util"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAPIKeyRepositoryGetByKeyHashCachesResult(t *testing.T) {
	ctx := context.Background()
	server := setupAPIKeyCacheRedis(t)
	db, mock, cleanup := openAPIKeyCacheMockDB(t)
	defer cleanup()

	repo := apikeyrepo.NewAPIKeyRepository(db)
	key := testTenantAPIKey("key-cache-read", "org-cache", "hash-cache-read", "active")
	expectAPIKeyLookup(mock, key)

	got, err := repo.GetByKeyHash(ctx, key.KeyHash)
	if err != nil {
		t.Fatalf("GetByKeyHash returned error: %v", err)
	}
	if got.ID != key.ID {
		t.Fatalf("key id = %s, want %s", got.ID, key.ID)
	}
	if !server.Exists(apiKeyCacheKey(key.KeyHash)) {
		t.Fatalf("expected cache key %s to exist", apiKeyCacheKey(key.KeyHash))
	}
	assertSQLExpectations(t, mock)
}

func TestAPIKeyRepositoryUpdateInvalidatesCachedKey(t *testing.T) {
	ctx := context.Background()
	server := setupAPIKeyCacheRedis(t)
	db, mock, cleanup := openAPIKeyCacheMockDB(t)
	defer cleanup()

	repo := apikeyrepo.NewAPIKeyRepository(db)
	key := testTenantAPIKey("key-update", "org-cache", "hash-update", "active")
	warmAPIKeyCache(t, ctx, repo, mock, server, key)

	key.Status = "disabled"
	expectAPIKeyUpdate(mock)
	if err := repo.Update(ctx, key); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if server.Exists(apiKeyCacheKey(key.KeyHash)) {
		t.Fatalf("expected cache key %s to be deleted", apiKeyCacheKey(key.KeyHash))
	}
	assertSQLExpectations(t, mock)
}

func TestAPIKeyRepositoryDeleteInvalidatesCachedKey(t *testing.T) {
	ctx := context.Background()
	server := setupAPIKeyCacheRedis(t)
	db, mock, cleanup := openAPIKeyCacheMockDB(t)
	defer cleanup()

	repo := apikeyrepo.NewAPIKeyRepository(db)
	key := testTenantAPIKey("key-delete", "org-cache", "hash-delete", "active")
	warmAPIKeyCache(t, ctx, repo, mock, server, key)

	expectAPIKeyHashLookupByID(mock, key)
	expectAPIKeySoftDelete(mock)
	if err := repo.Delete(ctx, key.ID, key.OrganizationID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if server.Exists(apiKeyCacheKey(key.KeyHash)) {
		t.Fatalf("expected cache key %s to be deleted", apiKeyCacheKey(key.KeyHash))
	}
	assertSQLExpectations(t, mock)
}

func TestAPIKeyRepositoryUpdateQuotaInvalidatesCachedKey(t *testing.T) {
	ctx := context.Background()
	server := setupAPIKeyCacheRedis(t)
	db, mock, cleanup := openAPIKeyCacheMockDB(t)
	defer cleanup()

	repo := apikeyrepo.NewAPIKeyRepository(db)
	key := testTenantAPIKey("key-quota", "org-cache", "hash-quota", "active")
	key.QuotaLimit = int64Ptr(100)
	key.RemainQuota = 100
	warmAPIKeyCache(t, ctx, repo, mock, server, key)

	expectAPIKeyQuotaUpdate(mock, key)
	if err := repo.UpdateQuota(ctx, key.ID, 10, -10); err != nil {
		t.Fatalf("UpdateQuota returned error: %v", err)
	}
	if server.Exists(apiKeyCacheKey(key.KeyHash)) {
		t.Fatalf("expected cache key %s to be deleted", apiKeyCacheKey(key.KeyHash))
	}
	assertSQLExpectations(t, mock)
}

func TestAPIKeyRepositoryCacheOperationsTolerateNilRedisClient(t *testing.T) {
	ctx := context.Background()
	oldClient := redisUtil.GetClient()
	redisUtil.SetClient(nil)
	t.Cleanup(func() {
		redisUtil.SetClient(oldClient)
	})

	db, mock, cleanup := openAPIKeyCacheMockDB(t)
	defer cleanup()

	repo := apikeyrepo.NewAPIKeyRepository(db)
	key := testTenantAPIKey("key-no-redis", "org-cache", "hash-no-redis", "active")

	expectAPIKeyLookup(mock, key)
	if _, err := repo.GetByKeyHash(ctx, key.KeyHash); err != nil {
		t.Fatalf("GetByKeyHash returned error: %v", err)
	}

	expectAPIKeyUpdate(mock)
	if err := repo.Update(ctx, key); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	expectAPIKeyHashLookupByID(mock, key)
	expectAPIKeySoftDelete(mock)
	if err := repo.Delete(ctx, key.ID, key.OrganizationID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	expectAPIKeyQuotaUpdate(mock, key)
	if err := repo.UpdateQuota(ctx, key.ID, 1, -1); err != nil {
		t.Fatalf("UpdateQuota returned error: %v", err)
	}

	assertSQLExpectations(t, mock)
}

func TestLLMAPIKeyAuthMiddlewareRejectsDisabledKeyAfterCacheInvalidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	server := setupAPIKeyCacheRedis(t)
	db, mock, cleanup := openAPIKeyCacheMockDB(t)
	defer cleanup()

	repo := apikeyrepo.NewAPIKeyRepository(db)
	rawKey := "sk-cache-invalidated"
	keyHash := util.HashAPIKey(rawKey)
	key := testTenantAPIKey("key-gateway", "org-cache", keyHash, "active")
	warmAPIKeyCache(t, ctx, repo, mock, server, key)

	key.Status = "disabled"
	expectAPIKeyUpdate(mock)
	if err := repo.Update(ctx, key); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if server.Exists(apiKeyCacheKey(key.KeyHash)) {
		t.Fatalf("expected cache key %s to be deleted", apiKeyCacheKey(key.KeyHash))
	}

	disabledKey := *key
	expectAPIKeyLookup(mock, &disabledKey)

	router := gin.New()
	router.Use(gatewayhandler.LLMAPIKeyAuthMiddleware(repo))
	router.GET("/v1/models", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	assertSQLExpectations(t, mock)
}

func setupAPIKeyCacheRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	oldClient := redisUtil.GetClient()
	redisUtil.SetClient(client)
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(oldClient)
	})

	return server
}

func openAPIKeyCacheMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open gorm: %v", err)
	}

	return db, mock, func() {
		_ = sqlDB.Close()
	}
}

func warmAPIKeyCache(t *testing.T, ctx context.Context, repo apikeyrepo.APIKeyRepository, mock sqlmock.Sqlmock, server *miniredis.Miniredis, key *apikeymodel.TenantAPIKey) {
	t.Helper()

	expectAPIKeyLookup(mock, key)
	if _, err := repo.GetByKeyHash(ctx, key.KeyHash); err != nil {
		t.Fatalf("GetByKeyHash returned error: %v", err)
	}
	if !server.Exists(apiKeyCacheKey(key.KeyHash)) {
		t.Fatalf("expected cache key %s to exist", apiKeyCacheKey(key.KeyHash))
	}
}

func expectAPIKeyLookup(mock sqlmock.Sqlmock, key *apikeymodel.TenantAPIKey) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "llm_organization_api_keys" WHERE key_hash = $1`)).
		WithArgs(key.KeyHash, 1).
		WillReturnRows(apiKeyRows(key))
}

func expectAPIKeyUpdate(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "llm_organization_api_keys" SET`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectAPIKeyHashLookupByID(mock sqlmock.Sqlmock, key *apikeymodel.TenantAPIKey) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "id","key_hash" FROM "llm_organization_api_keys"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "key_hash"}).AddRow(key.ID, key.KeyHash))
}

func expectAPIKeySoftDelete(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "llm_organization_api_keys" SET "deleted_at"=`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectAPIKeyQuotaUpdate(mock sqlmock.Sqlmock, key *apikeymodel.TenantAPIKey) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "llm_organization_api_keys" WHERE id = $1`)).
		WillReturnRows(apiKeyRows(key))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "llm_organization_api_keys" SET`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func apiKeyRows(key *apikeymodel.TenantAPIKey) *sqlmock.Rows {
	accessedAt := nullableTime(key.AccessedAt)
	expiresAt := nullableTime(key.ExpiresAt)
	quotaLimit := nullableInt64(key.QuotaLimit)
	modelLimits := nullableString(key.ModelLimits)

	return sqlmock.NewRows([]string{
		"id",
		"organization_id",
		"key",
		"key_hash",
		"name",
		"status",
		"is_internal",
		"created_at",
		"updated_at",
		"accessed_at",
		"expires_at",
		"deleted_at",
		"used_quota",
		"remain_quota",
		"quota_limit",
		"model_limits_enabled",
		"model_limits",
		"allow_ips",
	}).AddRow(
		key.ID,
		key.OrganizationID,
		key.Key,
		key.KeyHash,
		key.Name,
		key.Status,
		key.IsInternal,
		key.CreatedAt,
		key.UpdatedAt,
		accessedAt,
		expiresAt,
		nil,
		key.UsedQuota,
		key.RemainQuota,
		quotaLimit,
		key.ModelLimitsEnabled,
		modelLimits,
		key.AllowIPs,
	)
}

func testTenantAPIKey(id, organizationID, keyHash, status string) *apikeymodel.TenantAPIKey {
	now := time.Now().UTC().Truncate(time.Second)
	return &apikeymodel.TenantAPIKey{
		ID:             id,
		OrganizationID: organizationID,
		Key:            "encrypted-" + id,
		KeyHash:        keyHash,
		Name:           "test " + id,
		Status:         status,
		CreatedAt:      now,
		UpdatedAt:      now,
		RemainQuota:    100,
		AllowIPs:       "",
	}
}

func apiKeyCacheKey(keyHash string) string {
	return "llm:apikey:" + keyHash
}

func int64Ptr(value int64) *int64 {
	return &value
}

func nullableTime(value *time.Time) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt64(value *int64) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func nullableString(value *string) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func assertSQLExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}
