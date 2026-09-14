package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
	"github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	sharedredis "github.com/zgiai/zgi/api/pkg/redis"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetByKeyHashRestoresHashOnCacheHit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	if err := db.AutoMigrate(&model.TenantAPIKey{}); err != nil {
		t.Fatalf("migrate API key: %v", err)
	}

	miniRedis, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer miniRedis.Close()
	client := redisclient.NewClient(&redisclient.Options{Addr: miniRedis.Addr()})
	previousClient := sharedredis.GetClient()
	sharedredis.SetClient(client)
	t.Cleanup(func() {
		sharedredis.SetClient(previousClient)
		_ = client.Close()
	})

	keyHash := "f0a61e032f0e5dc1c295cde770c98d16a28eb0b17f6a0d1ee850bef543f22062"
	workspaceID, principalType, principalID, grantID := uuid.NewString(), "user", uuid.NewString(), uuid.NewString()
	stored := &model.TenantAPIKey{
		ID: uuid.NewString(), OrganizationID: uuid.NewString(), Name: "cached personal key", Status: "active",
		KeyHash: keyHash, KeyPrefix: "zgi_test", KeySuffix: "test", SecretVersion: 2,
		WorkspaceID: &workspaceID, PrincipalType: &principalType, PrincipalID: &principalID, AccessGrantID: &grantID,
	}
	if err := db.Create(stored).Error; err != nil {
		t.Fatalf("create API key: %v", err)
	}

	repo := NewAPIKeyRepository(db)
	first, err := repo.GetByKeyHash(context.Background(), keyHash)
	if err != nil {
		t.Fatalf("database lookup: %v", err)
	}
	if first.KeyHash != keyHash {
		t.Fatalf("database lookup key hash = %q", first.KeyHash)
	}
	second, err := repo.GetByKeyHash(context.Background(), keyHash)
	if err != nil {
		t.Fatalf("cache lookup: %v", err)
	}
	if second.KeyHash != keyHash {
		t.Fatalf("cache lookup lost key hash: got %q", second.KeyHash)
	}
	// An upgrade can change a previously cached unlimited personal key to
	// zero legacy quota. Old cache entries must expire before an old binary
	// can be admitted: changing the database alone does not purge Redis.
	if err := db.Model(stored).Updates(map[string]any{"quota_limit": int64(0), "remain_quota": int64(0)}).Error; err != nil {
		t.Fatal(err)
	}
	cached, err := repo.GetByKeyHash(context.Background(), keyHash)
	if err != nil {
		t.Fatal(err)
	}
	if !cached.HasQuota() {
		t.Fatal("fixture did not retain the pre-migration cache entry")
	}
	miniRedis.FastForward(apiKeyCacheTTL + time.Second)
	guarded, err := repo.GetByKeyHash(context.Background(), keyHash)
	if err != nil {
		t.Fatal(err)
	}
	if guarded.HasQuota() {
		t.Fatal("expired legacy cache did not reload the zero quota guard")
	}
	cachedGuard, err := repo.GetByKeyHash(context.Background(), keyHash)
	if err != nil {
		t.Fatal(err)
	}
	if cachedGuard.HasQuota() {
		t.Fatal("new cache entry lost the legacy quota guard")
	}
}
