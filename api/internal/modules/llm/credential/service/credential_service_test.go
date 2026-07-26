package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential/dto"
	credentialrepo "github.com/zgiai/zgi/api/internal/modules/llm/credential/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type credentialTestCrypto struct{}

func (credentialTestCrypto) Encrypt(plaintext string) (string, error) {
	return "encrypted:" + plaintext, nil
}

func (credentialTestCrypto) Decrypt(ciphertext string) (string, error) {
	return ciphertext, nil
}

func openCredentialServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE llm_credentials (
		id text PRIMARY KEY,
		organization_id text NOT NULL,
		name text NOT NULL,
		provider text NOT NULL,
		api_key_ciphertext text NOT NULL,
		api_key_hash text,
		api_base_url text,
		is_active numeric NOT NULL DEFAULT 1,
		last_used_at datetime,
		expires_at datetime,
		metadata text,
		created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
		deleted_at datetime
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE llm_routes (
		id text PRIMARY KEY,
		organization_id text NOT NULL,
		user_credential_id text,
		deleted_at datetime
	)`).Error)
	return db
}

func TestAttachedCredentialConnectionCannotBeMutatedOrDeleted(t *testing.T) {
	ctx := context.Background()
	db := openCredentialServiceTestDB(t)
	organizationID := uuid.New()
	svc := NewTenantCredentialService(
		credentialrepo.NewTenantCredentialRepository(db),
		credentialTestCrypto{},
		db,
	)
	credential, err := svc.Create(ctx, organizationID, &dto.CreateTenantCredentialRequest{
		Name:            "shared",
		ChannelProvider: "openai-compatible",
		APIKey:          "original-key",
		APIBaseURL:      "https://api.example.com/v1",
	})
	require.NoError(t, err)
	routeID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO llm_routes (id, organization_id, user_credential_id) VALUES (?, ?, ?)`,
		routeID,
		organizationID,
		credential.ID,
	).Error)

	replacementKey := "replacement-key"
	_, err = svc.Update(ctx, organizationID, credential.ID, &dto.UpdateTenantCredentialRequest{
		APIKey: &replacementKey,
	})
	require.ErrorIs(t, err, ErrCredentialInUse)

	stored, err := svc.GetByID(ctx, organizationID, credential.ID)
	require.NoError(t, err)
	require.Equal(t, credential.APIKeyCiphertext, stored.APIKeyCiphertext)

	err = svc.Delete(ctx, organizationID, credential.ID)
	require.ErrorIs(t, err, ErrCredentialInUse)

	var routeCount int64
	require.NoError(t, db.Model(&channelmodel.LLMRoute{}).Where("id = ?", routeID).Count(&routeCount).Error)
	require.EqualValues(t, 1, routeCount)
}
