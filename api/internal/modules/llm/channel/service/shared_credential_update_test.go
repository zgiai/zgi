package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	channeldto "github.com/zgiai/zgi/api/internal/modules/llm/channel/dto"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	channelrepo "github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	credentialdto "github.com/zgiai/zgi/api/internal/modules/llm/credential/dto"
	credentialrepo "github.com/zgiai/zgi/api/internal/modules/llm/credential/repository"
	credentialsvc "github.com/zgiai/zgi/api/internal/modules/llm/credential/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type sharedCredentialTestCrypto struct{}

func (sharedCredentialTestCrypto) Encrypt(plaintext string) (string, error) {
	return "encrypted:" + plaintext, nil
}

func (sharedCredentialTestCrypto) Decrypt(ciphertext string) (string, error) {
	return strings.TrimPrefix(ciphertext, "encrypted:"), nil
}

func openSharedCredentialUpdateTestDB(t *testing.T) *gorm.DB {
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
		type text NOT NULL,
		user_credential_id text,
		name text,
		provider text,
		models text,
		api_base_url text,
		native_protocols text,
		model_maps text,
		param_override text,
		header_override text,
		validation_report text,
		tags text,
		description text,
		priority integer NOT NULL DEFAULT 0,
		weight integer NOT NULL DEFAULT 1,
		is_enabled numeric NOT NULL DEFAULT 1,
		is_official numeric NOT NULL DEFAULT 0,
		auto_ban numeric NOT NULL DEFAULT 0,
		sync_mode text,
		last_synced_at datetime,
		balance numeric,
		currency text,
		created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
		deleted_at datetime
	)`).Error)
	return db
}

func TestUpdateRoute_RebindsOnlyEditedRouteWhenCredentialIsShared(t *testing.T) {
	ctx := context.Background()
	db := openSharedCredentialUpdateTestDB(t)
	organizationID := uuid.New()
	crypto := sharedCredentialTestCrypto{}
	credentialService := credentialsvc.NewTenantCredentialService(
		credentialrepo.NewTenantCredentialRepository(db),
		crypto,
		db,
	)
	sharedCredential, err := credentialService.Create(ctx, organizationID, &credentialdto.CreateTenantCredentialRequest{
		Name:            "shared qwen credential",
		ChannelProvider: "qwen",
		APIKey:          "sk-original-key",
		APIBaseURL:      "https://dashscope.aliyuncs.com/compatible-mode/v1",
	})
	require.NoError(t, err)

	editedRouteID := uuid.New()
	siblingRouteID := uuid.New()
	for _, routeID := range []uuid.UUID{editedRouteID, siblingRouteID} {
		require.NoError(t, db.Create(&channelmodel.LLMRoute{
			ID:              routeID,
			OrganizationID:  organizationID,
			Type:            shared.RouteTypePrivate,
			CredentialID:    &sharedCredential.ID,
			Name:            "Qwen Route",
			ChannelProvider: "qwen",
			APIBaseURL:      "https://dashscope.aliyuncs.com/compatible-mode/v1",
			Models:          []string{"qwen-plus"},
			Priority:        100,
			Weight:          100,
			IsEnabled:       true,
		}).Error)
	}

	routeRepository := channelrepo.NewTenantRouteRepository(db)
	svc := &channelService{
		tenantRouteRepo:   routeRepository,
		tenantCredService: credentialService,
		validator:         &fakeChannelValidator{},
		modelRepo: &fakeModelRepo{
			models: []*llmmodelmodel.LLMModel{{ID: uuid.New(), Model: "qwen-plus"}},
		},
		crypto: crypto,
	}
	replacementKey := "sk-replacement-key"

	_, err = svc.UpdateRoute(ctx, organizationID, editedRouteID, &channeldto.UpdateRouteRequest{
		APIKey: &replacementKey,
	})
	require.NoError(t, err)

	editedRoute, err := routeRepository.GetByID(ctx, organizationID, editedRouteID)
	require.NoError(t, err)
	siblingRoute, err := routeRepository.GetByID(ctx, organizationID, siblingRouteID)
	require.NoError(t, err)
	require.NotNil(t, editedRoute.CredentialID)
	require.NotNil(t, siblingRoute.CredentialID)
	require.NotEqual(t, sharedCredential.ID, *editedRoute.CredentialID)
	require.Equal(t, sharedCredential.ID, *siblingRoute.CredentialID)

	editedKey, err := credentialService.GetDecryptedAPIKey(ctx, organizationID, *editedRoute.CredentialID)
	require.NoError(t, err)
	siblingKey, err := credentialService.GetDecryptedAPIKey(ctx, organizationID, *siblingRoute.CredentialID)
	require.NoError(t, err)
	require.Equal(t, replacementKey, editedKey)
	require.Equal(t, "sk-original-key", siblingKey)

	staleReplacementID := uuid.New()
	staleRoute := *editedRoute
	staleRoute.CredentialID = &staleReplacementID
	bindingUpdater, ok := routeRepository.(channelrepo.CredentialBindingUpdater)
	require.True(t, ok)
	err = bindingUpdater.UpdateWithExpectedCredential(ctx, &staleRoute, &sharedCredential.ID)
	require.ErrorIs(t, err, channelrepo.ErrCredentialBindingChanged)

	reloadedEditedRoute, err := routeRepository.GetByID(ctx, organizationID, editedRouteID)
	require.NoError(t, err)
	require.Equal(t, *editedRoute.CredentialID, *reloadedEditedRoute.CredentialID)
}
