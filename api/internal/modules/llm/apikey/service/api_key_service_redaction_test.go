package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/llm/apikey/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAPIKeyResponsesOnlyExposeFullKeyOnCreate(t *testing.T) {
	svc, _ := newAPIKeyRedactionTestService(t)
	ctx := context.Background()
	organizationID := "11111111-1111-1111-1111-111111111111"

	created, err := svc.CreateAPIKey(ctx, &dto.CreateAPIKeyRequest{
		OrganizationID: &organizationID,
		Name:           "production",
		Count:          1,
		QuotaType:      dto.QuotaTypeUnlimited,
	})
	require.NoError(t, err)
	require.Len(t, created.Keys, 1)

	fullKey := created.Keys[0].Key
	require.NotEmpty(t, fullKey)
	require.True(t, strings.HasPrefix(fullKey, "sk-"))
	require.NotEmpty(t, created.Keys[0].KeyMasked)

	keyID := created.Keys[0].ID
	organizationIDs := []string{organizationID}

	detail, err := svc.GetAPIKey(ctx, keyID, organizationIDs)
	require.NoError(t, err)
	requireRedactedAPIKeyResponse(t, detail, fullKey)

	list, err := svc.ListAPIKeys(ctx, &dto.ListAPIKeyRequest{
		OrganizationIDs: organizationIDs,
		Page:            1,
		Limit:           20,
	})
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	requireRedactedAPIKeyResponse(t, &list.Items[0], fullKey)

	updatedName := "renamed"
	updated, err := svc.UpdateAPIKey(ctx, keyID, organizationIDs, &dto.UpdateAPIKeyRequest{
		Name: &updatedName,
	})
	require.NoError(t, err)
	require.Equal(t, updatedName, updated.Name)
	requireRedactedAPIKeyResponse(t, updated, fullKey)
}

func TestUpdateAPIKeyCanClearQuotaLimitAndExpiration(t *testing.T) {
	svc, _ := newAPIKeyRedactionTestService(t)
	ctx := context.Background()
	organizationID := "11111111-1111-1111-1111-111111111111"
	quotaAmount := int64(100)
	expiresAt := time.Now().Add(time.Hour)

	created, err := svc.CreateAPIKey(ctx, &dto.CreateAPIKeyRequest{
		OrganizationID: &organizationID,
		Name:           "limited",
		Count:          1,
		QuotaType:      dto.QuotaTypeCustom,
		QuotaAmount:    &quotaAmount,
		ExpiresAt:      &expiresAt,
	})
	require.NoError(t, err)
	require.NotNil(t, created.Keys[0].QuotaLimit)
	require.NotNil(t, created.Keys[0].ExpiresAt)

	updated, err := svc.UpdateAPIKey(ctx, created.Keys[0].ID, []string{organizationID}, &dto.UpdateAPIKeyRequest{
		ClearQuotaLimit: true,
		ClearExpiresAt:  true,
	})
	require.NoError(t, err)
	require.Nil(t, updated.QuotaLimit)
	require.Nil(t, updated.ExpiresAt)
	require.Zero(t, updated.RemainQuota)
}

func requireRedactedAPIKeyResponse(t *testing.T, response *dto.APIKeyResponse, fullKey string) {
	t.Helper()

	require.Empty(t, response.Key)
	require.NotEmpty(t, response.KeyMasked)
	require.NotEqual(t, fullKey, response.KeyMasked)

	payload, err := json.Marshal(response)
	require.NoError(t, err)
	require.NotContains(t, string(payload), `"key":`)
	require.NotContains(t, string(payload), fullKey)
	require.Contains(t, string(payload), `"key_masked":`)
}

func newAPIKeyRedactionTestService(t *testing.T) (APIKeyService, *gorm.DB) {
	t.Helper()

	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Encryption: config.EncryptionConfig{
			APIKeyEncryptionKey: "12345678901234567890123456789012",
		},
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TenantAPIKey{}))

	repo := repository.NewAPIKeyRepository(db)
	return NewAPIKeyService(db, repo, nil, nil), db
}
